package main

import (
	"log"

	"apexclaw/core"
	"apexclaw/model"
	"apexclaw/server"
)

func main() {
	model.StartVersionUpdater()
	core.RegisterBuiltinTools(core.GlobalRegistry)
	log.Printf("[TOOLS] loaded: %d", len(core.GlobalRegistry.List()))

	go func() {
		if err := server.Start(":8080"); err != nil {
			log.Printf("[Web] error: %v", err)
		}
	}()

	log.Printf("[ApexClaw] starting (model: %s)", core.Cfg.DefaultModel)

	if core.Cfg.TelegramBotToken == "" {
		log.Printf("[TG] Telegram not configured (optional) - use web UI at http://localhost:8080")
		return
	}

	bot, err := core.NewTelegramBot()
	if err != nil {
		log.Printf("[TG] bot init failed: %v (continuing without Telegram)", err)
		return
	}

	log.Printf("[TG] bot starting...")
	if err := bot.Start(); err != nil {
		log.Printf("[TG] bot stopped: %v", err)
	}
}
