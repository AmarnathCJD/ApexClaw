package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"apexclaw/model"

	"github.com/amarnathcjd/gogram/telegram"
)

type TelegramBot struct {
	client      *telegram.Client
	botUsername string
}

var (
	ctxMu  sync.Mutex
	msgCtx = make(map[string]map[string]interface{})
)

func setTelegramContext(userID string, ctx map[string]interface{}) {
	ctxMu.Lock()
	msgCtx[userID] = ctx
	ctxMu.Unlock()
}

func getTelegramContext(userID string) map[string]interface{} {
	ctxMu.Lock()
	defer ctxMu.Unlock()
	if ctx, ok := msgCtx[userID]; ok {
		return ctx
	}
	return nil
}

func NewTelegramBot() (*TelegramBot, error) {
	if Cfg.TelegramAPIID == 0 || Cfg.TelegramAPIHash == "" || Cfg.TelegramBotToken == "" {
		return nil, fmt.Errorf("telegram not configured")
	}
	client, err := telegram.NewClient(telegram.ClientConfig{
		AppID:   int32(Cfg.TelegramAPIID),
		AppHash: Cfg.TelegramAPIHash,
	})
	if err != nil {
		return nil, fmt.Errorf("gogram init: %w", err)
	}
	return &TelegramBot{client: client}, nil
}

func (b *TelegramBot) Start() error {
	log.Printf("[TG] connecting bot...")
	if err := b.client.LoginBot(Cfg.TelegramBotToken); err != nil {
		return fmt.Errorf("bot login: %w", err)
	}
	me, _ := b.client.GetMe()
	if me != nil {
		log.Printf("[TG] logged in as @%s (%d)", me.Username, me.ID)
		b.botUsername = me.Username
	}

	StartHeartbeat(b.client)

	b.client.On(telegram.OnMessage, func(m *telegram.NewMessage) error {
		if m.Sender == nil || m.Sender.Bot {
			return nil
		}
		text := m.Text()
		if text == "" {
			return nil
		}
		return b.handleText(m, text)
	})

	b.client.On(telegram.OnMessage, func(m *telegram.NewMessage) error {
		if m.Sender == nil || m.Sender.Bot {
			return nil
		}
		if !m.IsMedia() {
			return nil
		}

		if m.Voice() != nil || m.Audio() != nil {
			return b.handleVoice(m)
		}
		return b.handlePhoto(m)
	})

	b.client.Idle()
	return nil
}

func (b *TelegramBot) handleText(m *telegram.NewMessage, text string) error {
	userID := strconv.FormatInt(m.Sender.ID, 10)
	if userID != Cfg.OwnerID {
		return nil
	}

	if !m.IsPrivate() {
		var isMentioned = false
		if strings.Contains(strings.ToLower(text), "apex") {
			isMentioned = true
		}

		if m.IsReply() {
			r, err := m.GetReplyMessage()
			if err != nil {
				return nil
			}
			if r.SenderID() == b.client.Me().ID {
				isMentioned = true
			}
		}

		if !isMentioned {
			return nil
		}
	}

	log.Printf("[TG] msg from %s (chat %d): %q", userID, m.ChatID(), truncate(text, 80))

	tgCtx := map[string]interface{}{
		"owner_id":    userID,
		"telegram_id": m.ChatID(),
		"message_id":  int64(m.ID),
	}
	if m.ChatID() < 0 {

		tgCtx["group_id"] = m.ChatID()
	}
	if m.IsReply() {
		tgCtx["reply_to_msg_id"] = int64(m.ReplyToMsgID())
	}
	setTelegramContext(userID, tgCtx)

	switch text {
	case "/start":
		_, err := m.Reply(
			"👋 Hey, I'm ApexClaw.\n" +
				"Chat normally — I have tools and I'll use them when needed.\n\n" +
				"/reset — clear history\n" +
				"/status — session info\n" +
				"/tasks — list scheduled tasks\n" +
				"/tools — list tools",
		)
		return err

	case "/reset":
		GetOrCreateAgentSession(userID).Reset()
		_, err := m.Reply("🔄 Conversation cleared.")
		return err

	case "/status":
		s := GetOrCreateAgentSession(userID)
		_, err := m.Reply(fmt.Sprintf(
			"History: %d msgs | Model: %s | Tools: %d",
			s.HistoryLen(), s.model, len(GlobalRegistry.List()),
		))
		return err

	case "/tasks":
		_, err := m.Reply(ListHeartbeatTasks())
		return err

	case "/tools":
		tools := GlobalRegistry.List()
		if len(tools) == 0 {
			_, err := m.Reply("No tools registered.")
			return err
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("🔧 %d tools:\n\n", len(tools)))
		for _, t := range tools {
			sb.WriteString(fmt.Sprintf("• %s — %s\n", t.Name, t.Description))
		}
		_, err := m.Reply(strings.TrimSpace(sb.String()))
		return err
	}

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	b.sendTyping(m)
	session := GetOrCreateAgentSession(userID)

	var buf strings.Builder
	_, err := session.RunStream(timeoutCtx, userID, text, func(chunk string) {
		buf.WriteString(chunk)
		if buf.Len() >= 800 || strings.Contains(chunk, "\n\n") {
			b.safeSend(m, buf.String())
			buf.Reset()
		}
	})

	if err != nil {
		log.Printf("[TG] agent error for %s: %v", userID, err)
		_, _ = m.Reply("⚠️ Something went wrong. Please try again.")
		return nil
	}
	if buf.Len() > 0 {
		b.safeSend(m, buf.String())
	}
	return nil
}

func (b *TelegramBot) handleVoice(m *telegram.NewMessage) error {
	userID := strconv.FormatInt(m.Sender.ID, 10)
	if userID != Cfg.OwnerID {
		return nil
	}
	if !m.IsPrivate() {
		return nil
	}

	log.Printf("[TG] voice from %s (chat %d)", userID, m.ChatID())
	b.sendTyping(m)

	audioPath, err := m.Download()
	if err != nil {
		log.Printf("[TG] voice download error: %v", err)
		_, _ = m.Reply("⚠️ Failed to download voice message.")
		return nil
	}

	transcribed, err := transcribeAudio(audioPath)
	if err != nil {
		log.Printf("[TG] transcription error: %v", err)
		_, _ = m.Reply("⚠️ Could not transcribe voice message. Try typing your message.")
		return nil
	}

	log.Printf("[TG] transcribed: %q", transcribed)

	tgCtx := map[string]interface{}{
		"owner_id":    userID,
		"telegram_id": m.ChatID(),
		"message_id":  int64(m.ID),
	}
	if m.IsReply() {
		tgCtx["reply_to_msg_id"] = int64(m.ReplyToMsgID())
	}
	setTelegramContext(userID, tgCtx)

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	session := GetOrCreateAgentSession(userID)
	var buf strings.Builder
	_, err = session.RunStream(timeoutCtx, userID, transcribed, func(chunk string) {
		buf.WriteString(chunk)
		if buf.Len() >= 800 || strings.Contains(chunk, "\n\n") {
			b.safeSend(m, buf.String())
			buf.Reset()
		}
	})
	if err != nil {
		log.Printf("[TG] agent error for voice: %v", err)
		_, _ = m.Reply("⚠️ Something went wrong processing your voice message.")
		return nil
	}
	if buf.Len() > 0 {
		b.safeSend(m, buf.String())
	}
	return nil
}

func (b *TelegramBot) handlePhoto(m *telegram.NewMessage) error {
	userID := strconv.FormatInt(m.Sender.ID, 10)

	if userID != Cfg.OwnerID {
		return nil
	}

	if !m.IsPrivate() {
		if m.IsReply() {
			r, err := m.GetReplyMessage()
			if err != nil {
				return nil
			}
			if r.SenderID() != b.client.Me().ID {
				return nil
			}
		} else {
			return nil
		}
	}

	log.Printf("[TG] photo from %s (chat %d)", userID, m.ChatID())
	b.sendTyping(m)

	photoLocation, err := m.Download()
	if err != nil {
		log.Printf("[TG] photo download error: %v", err)
		_, _ = m.Reply("⚠️ Failed to download your photo.")
		return nil
	}

	caption := m.Text()
	if caption == "" {
		caption = "What is in this image? Describe it in detail."
	}

	token, err := model.GetAnonymousToken()
	if err != nil {
		log.Printf("[TG] token error: %v", err)
		_, _ = m.Reply("⚠️ Failed to get auth token.")
		return nil
	}

	photoBytes, err := os.ReadFile(photoLocation)
	if err != nil {
		log.Printf("[TG] photo read error: %v", err)
		_, _ = m.Reply("⚠️ Failed to read your photo.")
		return nil
	}

	uploadedFile, err := model.UploadImageData(token, photoBytes, "photo.jpg")
	if err != nil {
		log.Printf("[TG] image upload error: %v", err)
		_, _ = m.Reply("⚠️ Failed to upload image to model.")
		return nil
	}

	log.Printf("[TG] image uploaded: id=%s", uploadedFile.ID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	session := GetOrCreateAgentSession(userID)

	var buf strings.Builder
	_, err = session.RunStreamWithFiles(ctx, userID, caption, []*model.UpstreamFile{uploadedFile}, func(chunk string) {
		buf.WriteString(chunk)
		if buf.Len() >= 800 || strings.Contains(chunk, "\n\n") {
			b.safeSend(m, buf.String())
			buf.Reset()
		}
	})

	if err != nil {
		log.Printf("[TG] agent error for image: %v", err)
		_, _ = m.Reply("⚠️ Something went wrong analyzing the image.")
		return nil
	}
	if buf.Len() > 0 {
		b.safeSend(m, buf.String())
	}
	return nil
}

func (b *TelegramBot) safeSend(m *telegram.NewMessage, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	if _, err := m.Reply(text, &telegram.SendOptions{ParseMode: telegram.HTML}); err != nil {
		plain := strings.NewReplacer(
			"<b>", "", "</b>", "", "<i>", "", "</i>", "",
			"<code>", "", "</code>", "", "<pre>", "", "</pre>", "",
		).Replace(text)
		m.Reply(plain)
	}
}

func (b *TelegramBot) sendTyping(m *telegram.NewMessage) {
	b.client.SendAction(m.ChatID(), "typing")
}

func transcribeAudio(filePath string) (string, error) {
	apiKey := os.Getenv("ASSEMBLYAI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("ASSEMBLYAI_API_KEY not set — set it to enable voice transcription")
	}

	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open audio: %w", err)
	}
	defer f.Close()

	uploadReq, err := http.NewRequest("POST", "https://api.assemblyai.com/v2/upload", f)
	if err != nil {
		return "", err
	}
	uploadReq.Header.Set("authorization", apiKey)
	uploadReq.Header.Set("content-type", "application/octet-stream")

	client := &http.Client{Timeout: 60 * time.Second}
	uploadResp, err := client.Do(uploadReq)
	if err != nil {
		return "", fmt.Errorf("upload: %w", err)
	}
	defer uploadResp.Body.Close()
	uploadBody, _ := io.ReadAll(uploadResp.Body)

	var uploadResult struct {
		UploadURL string `json:"upload_url"`
	}
	if err := json.Unmarshal(uploadBody, &uploadResult); err != nil || uploadResult.UploadURL == "" {
		return "", fmt.Errorf("upload response parse error: %s", string(uploadBody))
	}

	transcriptReqBody, _ := json.Marshal(map[string]string{
		"audio_url":     uploadResult.UploadURL,
		"language_code": "en",
	})
	tReq, err := http.NewRequest("POST", "https://api.assemblyai.com/v2/transcript", bytes.NewReader(transcriptReqBody))
	if err != nil {
		return "", err
	}
	tReq.Header.Set("authorization", apiKey)
	tReq.Header.Set("content-type", "application/json")

	tResp, err := client.Do(tReq)
	if err != nil {
		return "", fmt.Errorf("transcript request: %w", err)
	}
	defer tResp.Body.Close()
	tBody, _ := io.ReadAll(tResp.Body)

	var tResult struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(tBody, &tResult); err != nil || tResult.ID == "" {
		return "", fmt.Errorf("transcript ID parse error: %s", string(tBody))
	}

	pollURL := "https://api.assemblyai.com/v2/transcript/" + tResult.ID
	for i := 0; i < 20; i++ {
		time.Sleep(3 * time.Second)
		pReq, _ := http.NewRequest("GET", pollURL, nil)
		pReq.Header.Set("authorization", apiKey)
		pResp, err := client.Do(pReq)
		if err != nil {
			continue
		}
		pBody, _ := io.ReadAll(pResp.Body)
		pResp.Body.Close()

		var poll struct {
			Status string `json:"status"`
			Text   string `json:"text"`
			Error  string `json:"error"`
		}
		if err := json.Unmarshal(pBody, &poll); err != nil {
			continue
		}
		switch poll.Status {
		case "completed":
			return poll.Text, nil
		case "error":
			return "", fmt.Errorf("transcription failed: %s", poll.Error)
		}
	}
	return "", fmt.Errorf("transcription timed out")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
