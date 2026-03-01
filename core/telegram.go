package core

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"maps"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

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

	inlineQueryMu sync.Mutex
	inlineQueries = make(map[string]string) // shortID -> full query text
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

func formatTGContext(ctx map[string]any) string {
	if len(ctx) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("[TG Context:")
	if v, ok := ctx["sender_id"]; ok {
		fmt.Fprintf(&sb, " sender_id=%v", v)
	}
	if v, ok := ctx["telegram_id"]; ok {
		fmt.Fprintf(&sb, " | chat_id=%v", v)
	}
	if v, ok := ctx["msg_id"]; ok {
		fmt.Fprintf(&sb, " | msg_id=%v", v)
	}
	if v, ok := ctx["group_id"]; ok {
		fmt.Fprintf(&sb, " | group_id=%v", v)
	}
	if v, ok := ctx["reply_id"]; ok {
		fmt.Fprintf(&sb, " | reply_id=%v", v)
	}
	if v, ok := ctx["reply_sender_id"]; ok {
		fmt.Fprintf(&sb, " | reply_sender_id=%v", v)
	}
	if v, ok := ctx["reply_text"]; ok && v != "" {
		text := fmt.Sprintf("%v", v)
		if len(text) > 100 {
			text = text[:100] + "..."
		}
		fmt.Fprintf(&sb, " | reply_text=%q", text)
	}
	if v, ok := ctx["reply_has_file"]; ok && v == true {
		sb.WriteString(" | reply_has_file=true")
		if fn, ok2 := ctx["reply_filename"]; ok2 {
			fmt.Fprintf(&sb, " | reply_filename=%v", fn)
		}
	}
	if v, ok := ctx["file_name"]; ok {
		fmt.Fprintf(&sb, " | file_name=%v", v)
	}
	if v, ok := ctx["file_path"]; ok {
		fmt.Fprintf(&sb, " | file_path=%v", v)
	}
	if v, ok := ctx["callback_data"]; ok {
		fmt.Fprintf(&sb, " | callback_data=%v", v)
	}
	sb.WriteString("]")
	return sb.String()
}

func buildMsgContext(m *telegram.NewMessage, userID string, extras map[string]any) map[string]any {
	ctx := map[string]any{
		"sender_id":       userID,
		"telegram_id":     m.ChatID(),
		"msg_id":          int64(m.ID),
		"is_private_chat": m.IsPrivate(),
		"chat_type":       "private",
	}
	if !m.IsPrivate() {
		ctx["chat_type"] = "group/channel"
		ctx["group_id"] = m.ChatID()
	}
	if m.IsReply() {
		ctx["reply_id"] = int64(m.ReplyToMsgID())
		if r, err := m.GetReplyMessage(); err == nil {
			ctx["reply_sender_id"] = fmt.Sprintf("%d", r.SenderID())
			if r.IsMedia() {
				ctx["reply_has_file"] = true
				ctx["replied_id"] = int64(r.ID)
				if r.File != nil && r.File.Name != "" {
					ctx["reply_filename"] = r.File.Name
				}
			}
		}
	}
	maps.Copy(ctx, extras)
	return ctx
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

	b.client.OnCommand("start", b.handleStart)
	b.client.OnCommand("reset", b.handleReset)
	b.client.OnCommand("status", b.handleStatus)
	b.client.OnCommand("tasks", b.handleTasks)
	b.client.OnCommand("tools", b.handleTools)
	b.client.OnCommand("addsudo", b.handleAddSudo)
	b.client.OnCommand("rmsudo", b.handleRmSudo)
	b.client.OnCommand("listsudo", b.handleListSudo)
	b.client.OnCommand("webcode", b.handleWebCode)

	b.client.On(telegram.OnMessage, func(m *telegram.NewMessage) error {
		if m.Sender == nil || m.Sender.Bot {
			return nil
		}
		text := m.Text()
		if text == "" || strings.HasPrefix(text, "/") {
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
		return b.handleFile(m)
	}, telegram.IsMedia)

	b.client.OnInlineQuery(string(telegram.OnInline), func(iq *telegram.InlineQuery) error {
		userID := strconv.FormatInt(iq.SenderID, 10)
		if !IsSudo(userID) {
			return nil
		}
		query := strings.TrimSpace(iq.Query)
		if query == "" {
			return nil
		}
		shortID := fmt.Sprintf("%d_%d", iq.SenderID, iq.QueryID)
		inlineQueryMu.Lock()
		inlineQueries[shortID] = query
		inlineQueryMu.Unlock()

		builder := iq.Builder()
		builder.Article(
			"Ask ApexClaw",
			query,
			"[processing]",
			&telegram.ArticleOptions{ID: shortID, ReplyMarkup: telegram.InlineData("[PROCESSING]", "[PROCESSING]")},
		)
		_, err := iq.Answer(builder.Results(), &telegram.InlineSendOptions{CacheTime: 0})
		return err
	})

	b.client.OnChosenInline(func(is *telegram.InlineSend) error {
		userID := strconv.FormatInt(is.SenderID, 10)
		if !IsSudo(userID) {
			return nil
		}
		shortID := is.ID
		inlineQueryMu.Lock()
		query := inlineQueries[shortID]
		delete(inlineQueries, shortID)
		inlineQueryMu.Unlock()
		if query == "" {
			return nil
		}
		log.Printf("[TG] inline send from %s: %q", userID, truncate(query, 80))

		ctx := map[string]any{
			"sender_id":       userID,
			"telegram_id":     is.ChatID(),
			"msg_id":          int64(is.MessageID()),
			"is_private_chat": true,
			"chat_type":       "private",
			"inline_query":    query,
		}
		setTelegramContext(userID, ctx)
		ctxPrefix := formatTGContext(ctx)
		fullMsg := query
		if ctxPrefix != "" {
			fullMsg = ctxPrefix + "\n" + query
		}

		timeoutCtx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
		defer cancel()

		session := GetOrCreateAgentSession(userID)

		result, err := session.RunStream(timeoutCtx, userID, fullMsg, func(string) {})
		if err != nil {
			log.Printf("[TG] inline agent error for %s: %v", userID, err)
			is.Edit("⚠️ Something went wrong processing your query.")
			return nil
		}

		result = cleanResultForTelegram(result)
		if result == "" {
			result = "Done."
		}
		_, err = is.Edit(result, &telegram.SendOptions{ParseMode: telegram.HTML})
		return nil
	})

	b.client.On(telegram.OnCallback, func(c *telegram.CallbackQuery) error {
		if c.Sender == nil {
			return nil
		}
		userID := strconv.FormatInt(c.SenderID, 10)
		if !IsSudo(userID) {
			c.Answer("Access denied", &telegram.CallbackOptions{Alert: true})
			return nil
		}

		if strings.EqualFold(c.DataString(), "[PROCESSING]") {
			c.Answer("Please wait for the previous request to complete.", &telegram.CallbackOptions{Alert: true})
			return nil
		}

		callbackData := c.DataString()
		log.Printf("[TG] callback from %s: %q", userID, callbackData)

		ctx := map[string]any{
			"sender_id":       userID,
			"telegram_id":     c.ChatID,
			"msg_id":          int64(c.MessageID),
			"callback_data":   callbackData,
			"is_private_chat": c.IsPrivate(),
			"chat_type":       "private",
		}
		if !c.IsPrivate() {
			ctx["chat_type"] = "group/channel"
			ctx["group_id"] = c.ChatID
		}
		setTelegramContext(userID, ctx)
		cbCtxPrefix := formatTGContext(ctx)
		cbMsg := fmt.Sprintf("[Button clicked: %s]", callbackData)
		if cbCtxPrefix != "" {
			cbMsg = cbCtxPrefix + "\n" + cbMsg
		}

		session := GetOrCreateAgentSession(userID)
		onChunk, _, done := b.newStreamHandler(c.ChatID, int64(c.MessageID), userID)
		_, err := session.RunStream(context.Background(), userID, cbMsg, onChunk)
		done()

		if err != nil {
			c.Answer(fmt.Sprintf("Error: %v", err), &telegram.CallbackOptions{Alert: true})
		}
		return nil
	})

	return nil
}

func (b *TelegramBot) handleText(m *telegram.NewMessage, text string) error {
	userID := strconv.FormatInt(m.SenderID(), 10)
	if !IsSudo(userID) {
		return nil
	}

	if !m.IsPrivate() {
		mentioned := strings.Contains(strings.ToLower(text), "apex")
		if !mentioned && m.IsReply() {
			if r, err := m.GetReplyMessage(); err == nil && r.SenderID() == b.client.Me().ID {
				mentioned = true
			}
		}
		if !mentioned {
			return nil
		}
	}

	log.Printf("[TG] msg from %s (chat %d): %q", userID, m.ChatID(), truncate(text, 80))
	msgCtxData := buildMsgContext(m, userID, nil)
	setTelegramContext(userID, msgCtxData)
	ctxPrefix := formatTGContext(msgCtxData)
	if ctxPrefix != "" {
		text = ctxPrefix + "\n" + text
	}

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	b.sendTyping(m)
	session := GetOrCreateAgentSession(userID)
	onChunk, _, done := b.newStreamHandler(m.ChatID(), int64(m.ID), userID)
	result, err := session.RunStream(timeoutCtx, userID, text, onChunk)

	if err != nil {
		done()
		log.Printf("[TG] agent error for %s: %v", userID, err)
		_, _ = m.Reply("⚠️ Something went wrong. Please try again.")
		return nil
	}

	result = cleanResultForTelegram(result)

	if strings.Contains(result, "[MAX_ITERATIONS]") {
		done()
		explanation := strings.TrimSpace(strings.Replace(result, "[MAX_ITERATIONS]\n", "", 1))
		if explanation == "" {
			explanation = "Hit iteration limit before completing the task."
		}
		_, _ = m.Reply(explanation, &telegram.SendOptions{ParseMode: telegram.HTML})
		return nil
	}

	done()
	return nil
}

func (b *TelegramBot) handleVoice(m *telegram.NewMessage) error {
	userID := strconv.FormatInt(m.Sender.ID, 10)
	if !IsSudo(userID) {
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
	voiceMsgCtx := buildMsgContext(m, userID, nil)
	setTelegramContext(userID, voiceMsgCtx)
	voiceCtxPrefix := formatTGContext(voiceMsgCtx)
	if voiceCtxPrefix != "" {
		transcribed = voiceCtxPrefix + "\n" + transcribed
	}

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	session := GetOrCreateAgentSession(userID)
	onChunk, _, done := b.newStreamHandler(m.ChatID(), int64(m.ID), userID)
	_, err = session.RunStream(timeoutCtx, userID, transcribed, onChunk)
	done()

	if err != nil {
		log.Printf("[TG] agent error for voice: %v", err)
		_, _ = m.Reply("⚠️ Something went wrong processing your voice message.")
	}
	return nil
}

func (b *TelegramBot) handleFile(m *telegram.NewMessage) error {
	userID := strconv.FormatInt(m.Sender.ID, 10)
	if !IsSudo(userID) {
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

	fileName := m.File.Name
	log.Printf("[TG] file from %s (chat %d, name: %s)", userID, m.ChatID(), fileName)
	b.sendTyping(m)

	filePath, err := m.Download()
	if err != nil {
		log.Printf("[TG] file download error: %v", err)
		_, _ = m.Reply("⚠️ Failed to download your file.")
		return nil
	}
	defer os.Remove(filePath)

	caption := m.Text()
	if caption == "" {
		caption = fmt.Sprintf("Process this file: %s", fileName)
	}

	fileMsgCtx := buildMsgContext(m, userID, map[string]any{
		"file_name": fileName,
		"file_path": filePath,
	})
	setTelegramContext(userID, fileMsgCtx)
	fileCtxPrefix := formatTGContext(fileMsgCtx)
	if fileCtxPrefix != "" {
		caption = fileCtxPrefix + "\n" + caption
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	session := GetOrCreateAgentSession(userID)
	if _, err = session.Run(ctx, userID, caption); err != nil {
		log.Printf("[TG] agent error for file: %v", err)
		_, _ = m.Reply("⚠️ Something went wrong processing the file.")
	}
	return nil
}

func cleanResultForTelegram(result string) string {
	// Strip \x00PROGRESS:...\x00 blocks first
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
	lines := strings.Split(result, "\n")
	var cleaned []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "PROGRESS:") ||
			strings.HasPrefix(trimmed, "{\"message\":") ||
			strings.HasPrefix(trimmed, "<tool_call>") ||
			strings.Contains(trimmed, "</tool_call>") ||
			trimmed == "" {
			continue
		}
		cleaned = append(cleaned, line)
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

func (b *TelegramBot) safeSend(m *telegram.NewMessage, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	text = telegram.HTMLToMarkdownV2(text)
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

// newStreamHandler returns (onChunk, flush, done).
// flush: sends buffered text mid-stream (does NOT clear progress).
// done: final call — clears progress message then flushes remaining buffer.
func (b *TelegramBot) newStreamHandler(chatID int64, replyToMsgID int64, senderID string) (func(string), func(), func()) {
	var buf strings.Builder

	flush := func() {
		if buf.Len() == 0 {
			return
		}
		b.safeSendText(chatID, replyToMsgID, buf.String())
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

// ── Bot commands ──────────────────────────────────────────────────────────────

func (b *TelegramBot) handleStart(m *telegram.NewMessage) error {
	userID := strconv.FormatInt(m.SenderID(), 10)
	if !IsSudo(userID) {
		return nil
	}
	msg := "👋 Hey, I'm ApexClaw.\n" +
		"Chat normally — I have tools and I'll use them when needed.\n\n" +
		"/reset — clear history\n" +
		"/status — session info\n" +
		"/tasks — list scheduled tasks\n" +
		"/tools — list tools"
	if userID == Cfg.OwnerID {
		msg += "\n\n🛠️ Sudo Management:\n" +
			"/addsudo — Add a sudo user\n" +
			"/rmsudo — Remove a sudo user\n" +
			"/listsudo — List all sudo users"
	}
	_, err := m.Reply(msg)
	return err
}

func (b *TelegramBot) handleReset(m *telegram.NewMessage) error {
	userID := strconv.FormatInt(m.SenderID(), 10)
	if !IsSudo(userID) {
		return nil
	}
	GetOrCreateAgentSession(userID).Reset()
	_, err := m.Reply("🔄 Conversation cleared.")
	return err
}

func (b *TelegramBot) handleStatus(m *telegram.NewMessage) error {
	userID := strconv.FormatInt(m.SenderID(), 10)
	if !IsSudo(userID) {
		return nil
	}
	s := GetOrCreateAgentSession(userID)
	_, err := m.Reply(fmt.Sprintf(
		"History: %d msgs | Model: %s | Tools: %d",
		s.HistoryLen(), s.model, len(GlobalRegistry.List()),
	))
	return err
}

func (b *TelegramBot) handleTasks(m *telegram.NewMessage) error {
	userID := strconv.FormatInt(m.SenderID(), 10)
	if !IsSudo(userID) {
		return nil
	}
	_, err := m.Reply(ListHeartbeatTasks())
	return err
}

func (b *TelegramBot) handleTools(m *telegram.NewMessage) error {
	userID := strconv.FormatInt(m.SenderID(), 10)
	if !IsSudo(userID) {
		return nil
	}
	tools := GlobalRegistry.List()
	if len(tools) == 0 {
		_, err := m.Reply("No tools registered.")
		return err
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "🔧 %d tools:\n\n", len(tools))
	for _, t := range tools {
		fmt.Fprintf(&sb, "%s, ", t.Name)
	}
	_, err := m.Reply(strings.TrimSpace(sb.String()))
	return err
}

func (b *TelegramBot) handleAddSudo(m *telegram.NewMessage) error {
	return b.handleSudoCommands(m, strings.Fields(m.Text()))
}

func (b *TelegramBot) handleRmSudo(m *telegram.NewMessage) error {
	return b.handleSudoCommands(m, strings.Fields(m.Text()))
}

func (b *TelegramBot) handleListSudo(m *telegram.NewMessage) error {
	return b.handleSudoCommands(m, strings.Fields(m.Text()))
}

func (b *TelegramBot) handleWebCode(m *telegram.NewMessage) error {
	userID := strconv.FormatInt(m.SenderID(), 10)
	if !IsSudo(userID) {
		return nil
	}
	return handleWebCodeCommand(m, strings.Fields(m.Text()))
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
		_, err := m.Reply(fmt.Sprintf("✅ Web login code changed!\nOld: `%s`\nNew: `%s`", oldCode, newCode))
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
		_, err := m.Reply(fmt.Sprintf("🎲 Random web login code generated!\nOld: `%s`\nNew: `%s`", oldCode, newCode))
		return err

	default:
		_, err := m.Reply("Unknown subcommand. Use: /webcode show | set <code> | random")
		return err
	}
}

func (b *TelegramBot) handleSudoCommands(m *telegram.NewMessage, parts []string) error {
	userID := strconv.FormatInt(m.SenderID(), 10)
	if userID != Cfg.OwnerID {
		return nil
	}

	cmd := parts[0]
	if strings.Contains(cmd, "listsudo") {
		if len(Cfg.SudoIDs) == 0 {
			_, err := m.Reply("No sudo users added.")
			return err
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "👑 <b>Owner:</b> <code>%s</code>\n", Cfg.OwnerID)
		fmt.Fprintf(&sb, "🛠️ <b>Sudo Users (%d):</b>\n", len(Cfg.SudoIDs))
		for _, id := range Cfg.SudoIDs {
			fmt.Fprintf(&sb, "• <code>%s</code>\n", id)
		}
		_, err := m.Reply(sb.String(), &telegram.SendOptions{ParseMode: telegram.HTML})
		return err
	}

	var targetID string
	if m.IsReply() {
		r, _ := m.GetReplyMessage()
		if r != nil {
			targetID = strconv.FormatInt(r.SenderID(), 10)
		}
	} else if len(parts) > 1 {
		arg := parts[1]
		if _, err := strconv.ParseInt(arg, 10, 64); err == nil {
			targetID = arg
		} else {
			peer, err := TGResolvePeer(arg)
			if err == nil {
				if u, ok := peer.(*telegram.UserObj); ok {
					targetID = strconv.FormatInt(u.ID, 10)
				}
			}
		}
	}

	if targetID == "" {
		_, err := m.Reply(fmt.Sprintf("Usage: %s <id/username> or reply to a message", cmd))
		return err
	}
	if targetID == Cfg.OwnerID {
		_, err := m.Reply("❌ That's the owner!")
		return err
	}

	envMap, _ := godotenv.Read()
	if envMap == nil {
		envMap = make(map[string]string)
	}

	currentSudos := Cfg.SudoIDs
	newSudos := []string{}

	if strings.Contains(cmd, "addsudo") {
		if slices.Contains(currentSudos, targetID) {
			_, err := m.Reply(fmt.Sprintf("✅ user <code>%s</code> is already a sudo user.", targetID), &telegram.SendOptions{ParseMode: telegram.HTML})
			return err
		}
		newSudos = append(currentSudos, targetID)
		_, _ = m.Reply(fmt.Sprintf("✅ Added <code>%s</code> to sudo users.", targetID), &telegram.SendOptions{ParseMode: telegram.HTML})
	} else if strings.Contains(cmd, "rmsudo") {
		found := false
		for _, s := range currentSudos {
			if s != targetID {
				newSudos = append(newSudos, s)
			} else {
				found = true
			}
		}
		if !found {
			_, err := m.Reply(fmt.Sprintf("❌ user <code>%s</code> is not a sudo user.", targetID), &telegram.SendOptions{ParseMode: telegram.HTML})
			return err
		}
		_, _ = m.Reply(fmt.Sprintf("✅ Removed <code>%s</code> from sudo users.", targetID), &telegram.SendOptions{ParseMode: telegram.HTML})
	}

	Cfg.SudoIDs = newSudos
	envMap["SUDO_IDS"] = strings.Join(newSudos, " ")
	godotenv.Write(envMap, ".env")
	return nil
}

func GenerateRandomCode() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(900000))
	return fmt.Sprintf("%06d", n.Int64()+100000)
}
