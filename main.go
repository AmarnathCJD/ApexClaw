package main

import (
	"log"

	"apexclaw/core"
	"apexclaw/server"
)

func main() {
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
	} else {
		bot, err := core.NewTelegramBot()
		if err != nil {
			log.Printf("[TG] bot init failed: %v (continuing without Telegram)", err)
		} else {
			log.Printf("[TG] bot starting...")
			if err := bot.Start(); err != nil {
				log.Printf("[TG] bot stopped: %v", err)
			}
		}
	}

	if core.Cfg.WAOwnerID == "" {
		log.Printf("[WA] WhatsApp not configured (optional) - set WA_OWNER_ID in .env to enable")
	} else {
		waBot, err := core.InitWhatsAppBot()
		if err != nil {
			log.Printf("[WA] bot init failed: %v", err)
		} else {
			log.Printf("[WA] bot starting...")
			go func() {
				if err := waBot.Start(); err != nil {
					log.Printf("[WA] bot stopped: %v", err)
				}
			}()
		}
	}

	idle()
}

func idle() {
	select {}
}
