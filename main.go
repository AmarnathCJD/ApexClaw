package main

import (
	"log"

	"apexclaw/core"
	"apexclaw/model"
)

func main() {
	model.StartVersionUpdater()
	core.RegisterBuiltinTools(core.GlobalRegistry)

	log.Printf("[ApexClaw] starting (model: %s, tools: %d)", core.Cfg.DefaultModel, len(core.GlobalRegistry.List()))

	if core.Cfg.TelegramBotToken == "" {
		log.Fatal("[TG] TELEGRAM_BOT_TOKEN is not set")
	}

	bot, err := core.NewTelegramBot()
	if err != nil {
		log.Fatalf("[TG] bot init failed: %v", err)
	}

	log.Printf("[TG] bot starting...")
	if err := bot.Start(); err != nil {
		log.Fatalf("[TG] bot stopped: %v", err)
	}
}
