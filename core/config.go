package core

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"log"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"apexclaw/model"
	"apexclaw/setup"

	"github.com/fsnotify/fsnotify"
	"github.com/joho/godotenv"
)

type Config struct {
	DefaultModel string `json:"default_model"`

	TelegramAPIID    int
	TelegramAPIHash  string
	TelegramBotToken string
	OwnerID          string
	MaxIterations    int

	WebPort       string
	WebLoginCode  string
	WebJWTSecret  string
	WebFirstLogin bool

	DNS string
}

var Cfg = Config{
	TelegramAPIID:    0,
	TelegramAPIHash:  "",
	TelegramBotToken: "",
	DefaultModel:     "GLM-4.7",
	OwnerID:          "",
	MaxIterations:    10,
	WebPort:          ":8080",
	WebLoginCode:     "123456",
	WebJWTSecret:     "",
	WebFirstLogin:    true,
}

func init() {
	if err := godotenv.Load(); err == nil {
		log.Printf("[ENV] loaded .env")
	} else {
		log.Printf("[ENV] .env file not found")
	}

	if err := setup.InteractiveSetup(); err != nil {
		log.Printf("[SETUP] %v", err)
	}
	if err := godotenv.Load(); err != nil {
		log.Printf("[ENV] Error reloading .env: %v", err)
	}

	apiIdStr := os.Getenv("TELEGRAM_API_ID")
	if id, err := strconv.Atoi(apiIdStr); err == nil {
		Cfg.TelegramAPIID = id
	}

	Cfg.TelegramAPIHash = os.Getenv("TELEGRAM_API_HASH")
	Cfg.TelegramBotToken = os.Getenv("TELEGRAM_BOT_TOKEN")
	Cfg.OwnerID = os.Getenv("OWNER_ID")

	if maxIter := os.Getenv("MAX_ITERATIONS"); maxIter != "" {
		if n, err := strconv.Atoi(maxIter); err == nil && n > 0 {
			Cfg.MaxIterations = n
		}
	}

	if model := os.Getenv("DEFAULT_MODEL"); model != "" {
		Cfg.DefaultModel = model
	}

	if port := os.Getenv("WEB_PORT"); port != "" {
		Cfg.WebPort = port
	}

	if code := os.Getenv("WEB_LOGIN_CODE"); code != "" {
		Cfg.WebLoginCode = code
	}
	if secret := os.Getenv("WEB_JWT_SECRET"); secret != "" {
		Cfg.WebJWTSecret = secret
	} else {
		Cfg.WebJWTSecret = generateJWTSecret()
		envMap, _ := godotenv.Read()
		if envMap == nil {
			envMap = make(map[string]string)
		}
		envMap["WEB_JWT_SECRET"] = Cfg.WebJWTSecret
		godotenv.Write(envMap, ".env")
		log.Printf("[AUTH] Generated new JWT secret")
	}

	Cfg.WebFirstLogin = true
	if firstLogin := os.Getenv("WEB_FIRST_LOGIN"); firstLogin == "false" {
		Cfg.WebFirstLogin = false
	}

	if dns := os.Getenv("DNS"); dns != "" {
		Cfg.DNS = dns
		UpdateDNSResolver()
		log.Printf("[DNS] Using custom DNS: %s", Cfg.DNS)
	}

	log.Printf("[Web] Default login code: %s (WEB_FIRST_LOGIN=%v)", Cfg.WebLoginCode, Cfg.WebFirstLogin)
}

func generateJWTSecret() string {
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("[AUTH] Failed to generate JWT secret: %v", err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

type PersistedSession struct {
	SessionID string          `json:"session_id"`
	SavedAt   time.Time       `json:"saved_at"`
	History   []model.Message `json:"history"`
}

var (
	sessionStoreDir   = "sessions"
	lastReloadTime    time.Time
	reloadDebounceDur = 500 * time.Millisecond
	BroadcastReloadFn func()
	sessionStoreMu    sync.RWMutex
	savedSessions     = make(map[string]*PersistedSession)
)

func SaveSession(sessionID string, history []model.Message) error {
	if len(history) <= 1 {
		return nil
	}
	os.MkdirAll(sessionStoreDir, 0755)
	ps := &PersistedSession{
		SessionID: sessionID,
		SavedAt:   time.Now(),
		History:   history,
	}
	sessionStoreMu.Lock()
	savedSessions[sessionID] = ps
	sessionStoreMu.Unlock()
	return nil
}

func LoadSession(sessionID string) []model.Message {
	sessionStoreMu.RLock()
	ps, ok := savedSessions[sessionID]
	sessionStoreMu.RUnlock()
	if !ok || len(ps.History) == 0 {
		return nil
	}
	return ps.History
}

func reloadSafeConfig() {
	envMap, err := godotenv.Read()
	if err != nil {
		log.Printf("[CONFIG] hot-reload: failed to read .env: %v", err)
		return
	}
	if maxIter, ok := envMap["MAX_ITERATIONS"]; ok {
		if n, err := strconv.Atoi(maxIter); err == nil && n > 0 {
			Cfg.MaxIterations = n
		}
	}
	if model, ok := envMap["DEFAULT_MODEL"]; ok && model != "" {
		Cfg.DefaultModel = model
	}
	if code, ok := envMap["WEB_LOGIN_CODE"]; ok && code != "" {
		Cfg.WebLoginCode = code
	}
	if fl, ok := envMap["WEB_FIRST_LOGIN"]; ok {
		Cfg.WebFirstLogin = fl != "false"
	}
	if dns, ok := envMap["DNS"]; ok && dns != "" {
		Cfg.DNS = dns
		UpdateDNSResolver()
		log.Printf("[DNS] Updated DNS: %s", Cfg.DNS)
	}
	log.Printf("[CONFIG] hot-reload complete: model=%s max_iter=%d", Cfg.DefaultModel, Cfg.MaxIterations)
}

func ReloadConfig() {
	reloadSafeConfig()
	if BroadcastReloadFn != nil {
		BroadcastReloadFn()
	}
}

func StartConfigWatcher() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("[CONFIG] watcher init failed: %v", err)
		return
	}
	if err := watcher.Add(".env"); err != nil {
		log.Printf("[CONFIG] cannot watch .env: %v", err)
		watcher.Close()
		return
	}
	go func() {
		defer watcher.Close()
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
					now := time.Now()
					if now.Sub(lastReloadTime) < reloadDebounceDur {
						continue
					}
					lastReloadTime = now
					log.Printf("[CONFIG] .env changed (%s), reloading safe fields...", event.Op)
					ReloadConfig()
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("[CONFIG] watcher error: %v", err)
			}
		}
	}()
	log.Printf("[CONFIG] watching .env for hot-reload")
}

var (
	customDialer *net.Dialer
	dialerMu     sync.RWMutex
)

func GetCustomDialer() *net.Dialer {
	dialerMu.RLock()
	defer dialerMu.RUnlock()
	if customDialer != nil {
		return customDialer
	}
	return &net.Dialer{}
}

func UpdateDNSResolver() {
	dialerMu.Lock()
	defer dialerMu.Unlock()

	if Cfg.DNS == "" {
		customDialer = nil
		return
	}

	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := &net.Dialer{}
			return d.DialContext(ctx, network, Cfg.DNS+":53")
		},
	}

	customDialer = &net.Dialer{
		Resolver: resolver,
	}

	net.DefaultResolver = resolver
}

func DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return GetCustomDialer().DialContext(ctx, network, address)
}
