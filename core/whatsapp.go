package core

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

type WhatsAppBot struct {
	client *whatsmeow.Client
}

func NewWhatsAppBot() (*WhatsAppBot, error) {
	ctx := context.Background()

	dbLog := waLog.Stdout("Database", "ERROR", true)
	container, err := sqlstore.New(ctx, "sqlite3", "file:wasession.db?_foreign_keys=on", dbLog)
	if err != nil {
		return nil, fmt.Errorf("failed to init wa store: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get first device: %w", err)
	}

	clientLog := waLog.Stdout("Client", "ERROR", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)

	return &WhatsAppBot{client: client}, nil
}

func (b *WhatsAppBot) Start() error {
	b.client.AddEventHandler(b.eventHandler)

	if b.client.Store.ID == nil {
		qrChan, _ := b.client.GetQRChannel(context.Background())
		err := b.client.Connect()
		if err != nil {
			return fmt.Errorf("failed to connect wa: %w", err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				log.Printf("[WA] Scan the QR code above with WhatsApp to login")
			} else {
				log.Printf("[WA] Login event: %s", evt.Event)
			}
		}
	} else {
		err := b.client.Connect()
		if err != nil {
			return fmt.Errorf("failed to connect wa: %w", err)
		}
		log.Printf("[WA] Logged in successfully")
	}

	return nil
}

func (b *WhatsAppBot) eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		if v.Info.IsFromMe {
			return
		}

		text := v.Message.GetConversation()
		if text == "" && v.Message.GetExtendedTextMessage() != nil {
			text = v.Message.GetExtendedTextMessage().GetText()
		}

		if text == "" {
			return
		}

		userID := v.Info.Sender.User
		WA_OWNER_ID := os.Getenv("WA_OWNER_ID")
		if userID != WA_OWNER_ID {
			return
		}

		log.Printf("[WA] msg from %s: %q", userID, truncate(text, 80))
		b.handleText(v.Info.Chat, userID, text)
	}
}

func (b *WhatsAppBot) handleText(chatID types.JID, userID string, text string) {
	msgCtxData := map[string]any{
		"sender_id": userID,
		"platform":  "whatsapp",
		"chat_id":   chatID.String(),
	}
	setTelegramContext(userID, msgCtxData)
	ctxPrefix := formatTGContext(msgCtxData)
	if ctxPrefix != "" {
		text = ctxPrefix + "\n" + text
	}

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	b.client.SendPresence(context.Background(), types.PresenceAvailable)
	b.client.SendChatPresence(context.Background(), chatID, types.ChatPresenceComposing, types.ChatPresenceMediaText)

	session := GetOrCreateAgentSession("wa_" + userID)
	onChunk, _, done := b.newStreamHandler(chatID, "wa_"+userID)
	result, err := session.RunStream(timeoutCtx, "wa_"+userID, text, onChunk)

	b.client.SendChatPresence(context.Background(), chatID, types.ChatPresencePaused, types.ChatPresenceMediaText)

	if err != nil {
		done()
		log.Printf("[WA] agent error for %s: %v", userID, err)
		b.safeSendText(chatID, "⚠️ Something went wrong. Please try again.")
		return
	}

	result = cleanResultForWhatsApp(result)

	if strings.Contains(result, "[MAX_ITERATIONS]") {
		done()
		explanation := strings.TrimSpace(strings.Replace(result, "[MAX_ITERATIONS]\n", "", 1))
		if explanation == "" {
			explanation = "Hit iteration limit before completing the task."
		}
		b.safeSendText(chatID, explanation)
		return
	}

	done()
}

func (b *WhatsAppBot) safeSendText(chatID types.JID, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	text = cleanResultForWhatsApp(text)
	_, _ = b.client.SendMessage(context.Background(), chatID, &waE2E.Message{
		Conversation: proto.String(text),
	})
}

func (b *WhatsAppBot) newStreamHandler(chatID types.JID, senderID string) (func(string), func(), func()) {
	var buf strings.Builder

	flush := func() {
		if buf.Len() == 0 {
			return
		}
		b.safeSendText(chatID, buf.String())
		buf.Reset()
	}

	done := func() {
		clearProgressMsg(senderID)
		flush()
	}

	onChunk := func(chunk string) {
		if strings.HasPrefix(chunk, "__TOOL_CALL:") || strings.HasPrefix(chunk, "__TOOL_RESULT:") {
			return
		}
		// Strip \x00PROGRESS:...\x00 blocks
		for {
			start := strings.Index(chunk, "\x00PROGRESS:")
			if start == -1 {
				break
			}
			end := strings.Index(chunk[start+1:], "\x00")
			if end == -1 {
				chunk = chunk[:start]
				break
			}
			chunk = chunk[:start] + chunk[start+1+end+1:]
		}
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			return
		}
		buf.WriteString(chunk)
		if buf.Len() >= 800 || strings.Contains(chunk, "\n\n") {
			flush()
		}
	}

	return onChunk, flush, done
}

func cleanResultForWhatsApp(result string) string {
	for {
		start := strings.Index(result, "\x00PROGRESS:")
		if start == -1 {
			break
		}
		end := strings.Index(result[start+1:], "\x00")
		if end == -1 {
			result = result[:start]
			break
		}
		result = result[:start] + result[start+1+end+1:]
	}
	var cleaned []string
	prevLine := ""
	for _, line := range strings.Split(result, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "PROGRESS:") ||
			strings.HasPrefix(trimmed, "{\"message\":") ||
			strings.HasPrefix(trimmed, "<tool_call>") ||
			strings.Contains(trimmed, "</tool_call>") ||
			trimmed == "" {
			continue
		}
		if trimmed == prevLine {
			continue
		}
		prevLine = trimmed
		cleaned = append(cleaned, line)
	}
	return strings.Join(cleaned, "\n")
}
