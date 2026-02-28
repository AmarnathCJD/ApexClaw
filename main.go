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
	core.StartConfigWatcher()
	log.Printf("[TOOLS] loaded: %d", len(core.GlobalRegistry.List()))

	go func() {
		if err := server.Start(core.Cfg.WebPort); err != nil {
			log.Printf("[Web] error: %v", err)
		}
	}()

	log.Printf("[ApexClaw] starting (model: %s)", core.Cfg.DefaultModel)

	if core.Cfg.TelegramBotToken == "" {
		log.Printf("[TG] Telegram not configured (optional) - use web UI at http://localhost:8080")
		idle()
		return
	}

	bot, err := core.NewTelegramBot()
	if err != nil {
		log.Printf("[TG] bot init failed: %v (continuing without Telegram)", err)
	}

	log.Printf("[TG] bot starting...")
	if err := bot.Start(); err != nil {
		log.Printf("[TG] bot stopped: %v", err)
	}
	idle()
}

func idle() {
	select {}
}
