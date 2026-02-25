package core

import (
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
}

var Cfg = Config{
	TelegramAPIID:    0,
	TelegramAPIHash:  "",
	TelegramBotToken: "",
	DefaultModel:     "GLM-4.7",
	OwnerID:          "",
	MaxIterations:    10,
}

func init() {
	if err := godotenv.Load(); err == nil {
		log.Printf("[ENV] loaded .env")
	} else {
		log.Printf("[ENV] .env file not found")
	}

	requiredVars := []string{"TELEGRAM_API_ID", "TELEGRAM_API_HASH", "TELEGRAM_BOT_TOKEN", "OWNER_ID"}
	needsSetup := false
	for _, v := range requiredVars {
		if os.Getenv(v) == "" {
			needsSetup = true
			break
		}
	}

	if needsSetup {
		log.Printf("[SETUP] Missing required configuration, launching setup wizard...")
		if err := setup.InteractiveSetup(); err != nil {
			log.Fatalf("[SETUP] Setup failed: %v", err)
		}
		if err := godotenv.Load(); err != nil {
			log.Printf("[ENV] Error reloading .env: %v", err)
		}
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
}
