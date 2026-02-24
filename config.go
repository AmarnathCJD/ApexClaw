package main

import (
	"log"
	"os"
	"strconv"

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
	TelegramAPIID:    6,
	TelegramAPIHash:  "",
	TelegramBotToken: "",
	DefaultModel:     "GLM-4.7",
	OwnerID:          "",
}

func init() {
	if err := godotenv.Load(); err == nil {
		log.Printf("[ENV] loaded .env")
	}
	if v := os.Getenv("TELEGRAM_API_ID"); v != "" {
		if id, err := strconv.Atoi(v); err == nil {
			Cfg.TelegramAPIID = id
		}
	}
	if v := os.Getenv("TELEGRAM_API_HASH"); v != "" {
		Cfg.TelegramAPIHash = v
	}
	if v := os.Getenv("TELEGRAM_BOT_TOKEN"); v != "" {
		Cfg.TelegramBotToken = v
	}
	if v := os.Getenv("OWNER_ID"); v != "" {
		Cfg.OwnerID = v
	}
}
