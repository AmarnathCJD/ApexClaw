package main

import (
	"encoding/json"
	"log"
	"regexp"
	"strings"

	"apexclaw/model"
)

var toolCallRe = regexp.MustCompile(`(?s)<tool_call>(.*?)(?:/>|</tool_call>)`)
var attrRe = regexp.MustCompile(`(\w+)="([^"]*)"`)

func parseToolCall(text string) (funcName, argsJSON string, ok bool) {
	m := toolCallRe.FindStringSubmatch(text)
	if m == nil {
		return "", "", false
	}
	inner := strings.TrimSpace(m[1])
	parts := strings.SplitN(inner, " ", 2)
	funcName = parts[0]
	attrsStr := ""
	if len(parts) > 1 {
		attrsStr = parts[1]
	}
	attrs := attrRe.FindAllStringSubmatch(attrsStr, -1)
	kv := make(map[string]string, len(attrs))
	for _, a := range attrs {
		kv[a[1]] = a[2]
	}
	b, _ := json.Marshal(kv)
	return funcName, string(b), true
}

func main() {
	model.StartVersionUpdater()
	RegisterBuiltinTools(GlobalRegistry)

	log.Printf("[ApexClaw] starting (model: %s, tools: %d)", Cfg.DefaultModel, len(GlobalRegistry.List()))

	if Cfg.TelegramBotToken == "" {
		log.Fatal("[TG] TELEGRAM_BOT_TOKEN is not set")
	}

	bot, err := NewTelegramBot()
	if err != nil {
		log.Fatalf("[TG] bot init failed: %v", err)
	}

	log.Printf("[TG] bot starting...")
	if err := bot.Start(); err != nil {
		log.Fatalf("[TG] bot stopped: %v", err)
	}
}
