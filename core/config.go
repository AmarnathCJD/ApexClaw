package core

import (
	"crypto/rand"
	"encoding/base64"
	"log"
	"os"
	"strconv"

	"apexclaw/setup"

	"github.com/joho/godotenv"
)

type Config struct {
	DefaultModel string `json:"default_model"`

	TelegramAPIID    int
	TelegramAPIHash  string
	TelegramBotToken string
	OwnerID          string
	MaxIterations    int

	WebLoginCode  string
	WebJWTSecret  string
	WebFirstLogin bool
}

var Cfg = Config{
	TelegramAPIID:    0,
	TelegramAPIHash:  "",
	TelegramBotToken: "",
	DefaultModel:     "GLM-4.7",
	OwnerID:          "",
	MaxIterations:    10,
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

	// Offer setup wizard to user
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

	log.Printf("[Web] Default login code: %s (WEB_FIRST_LOGIN=%v)", Cfg.WebLoginCode, Cfg.WebFirstLogin)
}

// generateJWTSecret creates a secure random JWT secret
func generateJWTSecret() string {
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("[AUTH] Failed to generate JWT secret: %v", err)
	}
	return base64.StdEncoding.EncodeToString(b)
}
