package core

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"apexclaw/model"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/joho/godotenv"
)

type TelegramBot struct {
	client      *telegram.Client
	botUsername string
}

var (
	ctxMu  sync.Mutex
	msgCtx = make(map[string]map[string]any)
)

func setTelegramContext(userID string, ctx map[string]any) {
	ctxMu.Lock()
	msgCtx[userID] = ctx
	ctxMu.Unlock()
}

func getTelegramContext(userID string) map[string]any {
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
	}, telegram.IsMedia)

	b.client.On(telegram.OnCallback, func(c *telegram.CallbackQuery) error {
		if c.Sender == nil {
			return nil
		}
		userID := strconv.FormatInt(c.SenderID, 10)
		if userID != Cfg.OwnerID {
			c.Answer("Access denied", &telegram.CallbackOptions{Alert: true})
			return nil
		}

		callbackData := c.DataString()
		log.Printf("[TG] callback from %s: %q", userID, callbackData)

		tgCtx := map[string]any{
			"owner_id":      userID,
			"sender_id":     userID,
			"my_id":         userID,
			"chat_id":       c.ChatID,
			"telegram_id":   c.ChatID,
			"message_id":    int64(c.MessageID),
			"callback_data": callbackData,
		}
		if c.ChatID < 0 {
			tgCtx["group_id"] = c.ChatID
		}
		setTelegramContext(userID, tgCtx)

		session := GetOrCreateAgentSession(userID)

		onChunk, flush := b.newStreamHandler(c.ChatID, int64(c.MessageID))
		_, err := session.RunStream(context.Background(), userID, fmt.Sprintf("[Button clicked: %s]", callbackData), onChunk)
		flush()

		if err != nil {
			c.Answer(fmt.Sprintf("Error: %v", err), &telegram.CallbackOptions{Alert: true})
			return nil
		}

		return nil
	})

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

	tgCtx := map[string]any{
		"owner_id":        userID,
		"sender_id":       userID,
		"my_id":           userID,
		"chat_id":         m.ChatID(),
		"telegram_id":     m.ChatID(),
		"message_id":      int64(m.ID),
		"is_private_chat": m.IsPrivate(),
		"chat_type":       "private",
	}
	if !m.IsPrivate() {
		tgCtx["chat_type"] = "group/channel"
		tgCtx["group_id"] = m.ChatID()
	}
	if m.IsReply() {
		tgCtx["reply_to_msg_id"] = int64(m.ReplyToMsgID())
		if r, err := m.GetReplyMessage(); err == nil {
			tgCtx["replied_to_user_id"] = fmt.Sprintf("%d", r.SenderID())
		}
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

	// Handle /webcode command
	parts := strings.Fields(text)
	if len(parts) > 0 && parts[0] == "/webcode" {
		return handleWebCodeCommand(m, parts)
	}

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	b.sendTyping(m)
	session := GetOrCreateAgentSession(userID)

	onChunk, flush := b.newStreamHandler(m.ChatID(), int64(m.ID))
	result, err := session.RunStream(timeoutCtx, userID, text, onChunk)

	if err != nil {
		flush()
		log.Printf("[TG] agent error for %s: %v", userID, err)
		_, _ = m.Reply("⚠️ Something went wrong. Please try again.")
		return nil
	}

	if strings.Contains(result, "[MAX_ITERATIONS]") {
		flush()
		explanation := strings.Replace(result, "[MAX_ITERATIONS]\n", "", 1)
		explanation = strings.TrimSpace(explanation)
		msg := "⚠️ <b>Couldn't complete the task:</b>\n\n" + explanation
		_, _ = m.Reply(msg, &telegram.SendOptions{ParseMode: telegram.HTML})
		return nil
	}

	flush()
	return nil
}

func (b *TelegramBot) handleVoice(m *telegram.NewMessage) error {
	userID := strconv.FormatInt(m.Sender.ID, 10)
	if userID != Cfg.OwnerID {
		return nil
	}
	if !m.IsPrivate() {
		if !m.IsReply() {
			return nil
		}
		r, err := m.GetReplyMessage()
		if err != nil || r.SenderID() != b.client.Me().ID {
			return nil
		}
	}

	log.Printf("[TG] voice from %s (chat %d)", userID, m.ChatID())
	b.sendTyping(m)

	audioPath, err := m.Download()
	if err != nil {
		log.Printf("[TG] voice download error: %v", err)
		_, _ = m.Reply("⚠️ Failed to download voice message.")
		return nil
	}
	defer os.Remove(audioPath)

	transcribed, err := transcribeAudio(audioPath)
	if err != nil {
		log.Printf("[TG] transcription error: %v", err)
		_, _ = m.Reply("⚠️ Could not transcribe voice message. Try typing your message.")
		return nil
	}

	log.Printf("[TG] transcribed: %q", transcribed)

	tgCtx := map[string]any{
		"owner_id":    userID,
		"sender_id":   userID,
		"my_id":       userID,
		"chat_id":     m.ChatID(),
		"telegram_id": m.ChatID(),
		"message_id":  int64(m.ID),
	}
	if m.IsReply() {
		tgCtx["reply_to_msg_id"] = int64(m.ReplyToMsgID())
		if r, err := m.GetReplyMessage(); err == nil {
			tgCtx["replied_to_user_id"] = fmt.Sprintf("%d", r.SenderID())
		}
	}
	setTelegramContext(userID, tgCtx)

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	session := GetOrCreateAgentSession(userID)

	onChunk, flush := b.newStreamHandler(m.ChatID(), int64(m.ID))
	_, err = session.RunStream(timeoutCtx, userID, transcribed, onChunk)

	flush()

	if err != nil {
		log.Printf("[TG] agent error for voice: %v", err)
		_, _ = m.Reply("⚠️ Something went wrong processing your voice message.")
		return nil
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
	defer os.Remove(photoLocation)

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

	tgCtx := map[string]any{
		"owner_id":    userID,
		"sender_id":   userID,
		"my_id":       userID,
		"chat_id":     m.ChatID(),
		"telegram_id": m.ChatID(),
		"message_id":  int64(m.ID),
	}
	if m.ChatID() < 0 {
		tgCtx["group_id"] = m.ChatID()
	}
	if m.IsReply() {
		tgCtx["reply_to_msg_id"] = int64(m.ReplyToMsgID())
		if r, err := m.GetReplyMessage(); err == nil {
			tgCtx["replied_to_user_id"] = fmt.Sprintf("%d", r.SenderID())
		}
	}
	setTelegramContext(userID, tgCtx)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	session := GetOrCreateAgentSession(userID)

	onChunk, flush := b.newStreamHandler(m.ChatID(), int64(m.ID))
	_, err = session.RunStreamWithFiles(ctx, userID, caption, []*model.UpstreamFile{uploadedFile}, onChunk)
	flush()

	if err != nil {
		log.Printf("[TG] agent error for image: %v", err)
		_, _ = m.Reply("⚠️ Something went wrong analyzing the image.")
		return nil
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
	flacPath := filePath + ".flac"

	cmd := exec.Command("ffmpeg", "-y", "-i", filePath, "-ar", "16000", "-ac", "1", "-c:a", "flac", flacPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg conversion failed: %v\nOutput: %s", err, string(out))
	}
	defer os.Remove(flacPath)

	flacBytes, err := os.ReadFile(flacPath)
	if err != nil {
		return "", fmt.Errorf("failed to read flac file: %w", err)
	}

	url := "https://www.google.com/speech-api/v2/recognize?client=chromium&lang=en-US&key=AIzaSyBOti4mM-6x9WDnZIjIeyEU21OpBXqWBgw"
	req, err := http.NewRequest("POST", url, bytes.NewReader(flacBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "audio/x-flac; rate=16000")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("google stt request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	lines := strings.SplitSeq(string(bodyBytes), "\n")
	for line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var result struct {
			Result []struct {
				Alternative []struct {
					Transcript string `json:"transcript"`
				} `json:"alternative"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(line), &result); err == nil {
			if len(result.Result) > 0 && len(result.Result[0].Alternative) > 0 {
				return result.Result[0].Alternative[0].Transcript, nil
			}
		}
	}

	return "", fmt.Errorf("no transcript found in response: %s", string(bodyBytes))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func (b *TelegramBot) safeSendText(chatID int64, replyToMsgID int64, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	opts := &telegram.SendOptions{ParseMode: telegram.HTML}
	if replyToMsgID > 0 {
		opts.ReplyID = int32(replyToMsgID)
	}
	if _, err := b.client.SendMessage(chatID, text, opts); err != nil {
		plain := strings.NewReplacer(
			"<b>", "", "</b>", "", "<i>", "", "</i>", "",
			"<code>", "", "</code>", "", "<pre>", "", "</pre>", "",
		).Replace(text)
		opts.ParseMode = ""
		b.client.SendMessage(chatID, plain, opts)
	}
}

func (b *TelegramBot) newStreamHandler(chatID int64, replyToMsgID int64) (func(string), func()) {
	var buf strings.Builder

	flush := func() {
		if buf.Len() == 0 {
			return
		}
		b.safeSendText(chatID, replyToMsgID, buf.String())
		buf.Reset()
	}

	onChunk := func(chunk string) {
		if strings.HasPrefix(chunk, "__TOOL_CALL:") || strings.HasPrefix(chunk, "__TOOL_RESULT:") {
			return
		}

		buf.WriteString(chunk)
		if buf.Len() >= 800 || strings.Contains(chunk, "\n\n") {
			flush()
		}
	}

	return onChunk, flush
}

func handleWebCodeCommand(m *telegram.NewMessage, parts []string) error {
	if len(parts) == 1 {
		_, err := m.Reply(
			"🔐 Web Login Code Commands:\n\n" +
				"/webcode show — Show current code\n" +
				"/webcode set <newcode> — Set specific 6-digit code\n" +
				"/webcode random — Generate random code",
		)
		return err
	}

	switch parts[1] {
	case "show":
		_, err := m.Reply(fmt.Sprintf("🔐 Current web login code: `%s`", Cfg.WebLoginCode))
		return err

	case "set":
		if len(parts) < 3 {
			_, err := m.Reply("Usage: /webcode set <6-digit-code>")
			return err
		}
		newCode := parts[2]
		if !regexp.MustCompile(`^\d{6}$`).MatchString(newCode) {
			_, err := m.Reply("❌ Code must be exactly 6 digits.")
			return err
		}
		oldCode := Cfg.WebLoginCode
		Cfg.WebLoginCode = newCode
		envMap, _ := godotenv.Read()
		if envMap == nil {
			envMap = make(map[string]string)
		}
		envMap["WEB_LOGIN_CODE"] = newCode
		envMap["WEB_FIRST_LOGIN"] = "false"
		godotenv.Write(envMap, ".env")
		_, err := m.Reply(fmt.Sprintf(
			"✅ Web login code changed!\nOld: `%s`\nNew: `%s`",
			oldCode, newCode,
		))
		return err

	case "random":
		newCode := GenerateRandomCode()
		oldCode := Cfg.WebLoginCode
		Cfg.WebLoginCode = newCode
		envMap, _ := godotenv.Read()
		if envMap == nil {
			envMap = make(map[string]string)
		}
		envMap["WEB_LOGIN_CODE"] = newCode
		envMap["WEB_FIRST_LOGIN"] = "false"
		godotenv.Write(envMap, ".env")
		_, err := m.Reply(fmt.Sprintf(
			"🎲 Random web login code generated!\nOld: `%s`\nNew: `%s`",
			oldCode, newCode,
		))
		return err

	default:
		_, err := m.Reply("Unknown subcommand. Use: /webcode show | set <code> | random")
		return err
	}
}

func GenerateRandomCode() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(900000))
	return fmt.Sprintf("%06d", n.Int64()+100000)
}
