package core

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	DefaultModel string `json:"default_model"`

	TelegramAPIID    int
	TelegramAPIHash  string
	TelegramBotToken string
	OwnerID          string
}

var Cfg = Config{
	TelegramAPIID:    0,
	TelegramAPIHash:  "",
	TelegramBotToken: "",
	DefaultModel:     "GLM-4.7",
	OwnerID:          "",
}

func promptEnv(key, promptMsg string) string {
	fmt.Printf("%s: ", promptMsg)
	reader := bufio.NewReader(os.Stdin)
	val, _ := reader.ReadString('\n')
	return strings.TrimSpace(val)
}

func init() {
	if err := godotenv.Load(); err == nil {
		log.Printf("[ENV] loaded .env")
	} else {
		log.Printf("[ENV] .env file not found, creating a new one")
	}

	envUpdated := false
	apiIdStr := os.Getenv("TELEGRAM_API_ID")
	if apiIdStr == "" {
		apiIdStr = promptEnv("TELEGRAM_API_ID", "Please enter your Telegram API ID")
		os.Setenv("TELEGRAM_API_ID", apiIdStr)
		envUpdated = true
	}
	if id, err := strconv.Atoi(apiIdStr); err == nil {
		Cfg.TelegramAPIID = id
	}

	Cfg.TelegramAPIHash = os.Getenv("TELEGRAM_API_HASH")
	if Cfg.TelegramAPIHash == "" {
		Cfg.TelegramAPIHash = promptEnv("TELEGRAM_API_HASH", "Please enter your Telegram API Hash")
		os.Setenv("TELEGRAM_API_HASH", Cfg.TelegramAPIHash)
		envUpdated = true
	}

	Cfg.TelegramBotToken = os.Getenv("TELEGRAM_BOT_TOKEN")
	if Cfg.TelegramBotToken == "" {
		Cfg.TelegramBotToken = promptEnv("TELEGRAM_BOT_TOKEN", "Please enter your Telegram Bot Token")
		os.Setenv("TELEGRAM_BOT_TOKEN", Cfg.TelegramBotToken)
		envUpdated = true
	}

	Cfg.OwnerID = os.Getenv("OWNER_ID")
	if Cfg.OwnerID == "" {
		Cfg.OwnerID = promptEnv("OWNER_ID", "Please enter your numeric Owner ID (Telegram Chat ID limit access)")
		os.Setenv("OWNER_ID", Cfg.OwnerID)
		envUpdated = true
	}

	if envUpdated {
		f, err := os.OpenFile(".env", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			fmt.Fprintf(f, "\nTELEGRAM_API_ID=%d\n", Cfg.TelegramAPIID)
			fmt.Fprintf(f, "TELEGRAM_API_HASH=%s\n", Cfg.TelegramAPIHash)
			fmt.Fprintf(f, "TELEGRAM_BOT_TOKEN=%s\n", Cfg.TelegramBotToken)
			fmt.Fprintf(f, "OWNER_ID=%s\n", Cfg.OwnerID)
			f.Close()
			log.Printf("[ENV] Saved new credentials to .env")
		} else {
			log.Printf("[ENV] Error saving .env file: %v", err)
		}
	}
}
