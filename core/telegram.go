package core

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"maps"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
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

	inlineQueryMu sync.Mutex
	inlineQueries = make(map[string]string) // shortID -> full query text
)

func setTelegramContext(userID string, ctx map[string]any) {
	ctxMu.Lock()
	msgCtx[userID] = ctx
	ctxMu.Unlock()
}

func deleteTelegramContext(userID string) {
	ctxMu.Lock()
	delete(msgCtx, userID)
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

// formatTGContext returns a structured "[TG Context: ...]" line prepended to
// every user message that hits the agent. It surfaces everything the model
// might need to act without extra tool calls — chat id, msg id, group id,
// FULL replied-message text, replied sender display name, replied media kind,
// downloaded file path, and (for images) whether the image was already
// auto-attached to the ZAI session for vision.
func formatTGContext(ctx map[string]any) string {
	if len(ctx) == 0 {
		return ""
	}

	header := "TG Context"
	if v, ok := ctx["platform"]; ok && v == "whatsapp" {
		header = "WA Context"
	}

	var sb strings.Builder
	sb.WriteString("[" + header + "]\n")

	emit := func(k string, v any) {
		fmt.Fprintf(&sb, "- %s: %v\n", k, v)
	}
	emitQ := func(k string, v any) {
		// Quoted form for free-form text values (with newline-flattening)
		s := fmt.Sprintf("%v", v)
		s = strings.ReplaceAll(s, "\n", " ⏎ ")
		if len(s) > 300 {
			s = s[:300] + "…"
		}
		fmt.Fprintf(&sb, "- %s: %q\n", k, s)
	}

	if v, ok := ctx["sender_id"]; ok {
		emit("sender_id", v)
	}
	if v, ok := ctx["sender_name"]; ok {
		emit("sender_name", v)
	}
	if v, ok := ctx["sender_username"]; ok {
		emit("sender_username", "@"+fmt.Sprintf("%v", v))
	}
	if v, ok := ctx["telegram_id"]; ok {
		emit("chat_id", v)
	}
	if v, ok := ctx["group_id"]; ok {
		emit("group_id", v)
	}
	if v, ok := ctx["chat_type"]; ok {
		emit("chat_type", v)
	}
	if v, ok := ctx["msg_id"]; ok {
		emit("msg_id", v)
	}
	if v, ok := ctx["callback_data"]; ok {
		emit("callback_data", v)
	}

	// Inbound file (user attached a file to THIS message)
	if v, ok := ctx["file_name"]; ok {
		emit("file_name", v)
	}
	if v, ok := ctx["file_path"]; ok {
		emit("file_path", v)
	}

	// Replied-to message context (the meat — what makes "edit this", "what's
	// in this", "summarise that link", etc. work without tool round-trips).
	if v, ok := ctx["reply_id"]; ok {
		emit("reply_id", v)
	}
	if v, ok := ctx["reply_sender_name"]; ok {
		emit("reply_sender_name", v)
	}
	if v, ok := ctx["reply_sender_username"]; ok {
		emit("reply_sender_username", "@"+fmt.Sprintf("%v", v))
	}
	if v, ok := ctx["reply_sender_id"]; ok {
		emit("reply_sender_id", v)
	}
	if v, ok := ctx["reply_sender_is_bot"]; ok && v == true {
		emit("reply_sender_is_bot", true)
	}
	if v, ok := ctx["reply_text"]; ok && v != "" {
		emitQ("reply_text", v)
	}
	if v, ok := ctx["reply_media_type"]; ok {
		emit("reply_media_type", v)
	}
	if v, ok := ctx["reply_filename"]; ok {
		emit("reply_filename", v)
	}
	if v, ok := ctx["reply_file_path"]; ok {
		emit("reply_file_path", v)
	}
	if v, ok := ctx["reply_image_attached"]; ok && v == true {
		emit("reply_image_attached", "true (image is already loaded into your vision context — you can describe/edit it directly without uploading again)")
	}

	return strings.TrimRight(sb.String(), "\n")
}

// buildMsgContext returns a rich, AI-friendly context map describing the
// incoming Telegram message. When the message is a REPLY, we eagerly resolve:
//   - the replied-to message's full text content
//   - the replied-to sender's display name/username
//   - any media on the replied-to message (downloaded to a temp file)
//   - for images, the file is automatically uploaded to ZAI so the model can
//     see/edit it on its next chat turn (auto-attached via session.AttachZAIFile)
//
// This means the model never needs to call extra tools like tg_get_message
// or tg_download just to act on what the user replied to.
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
	if me := m.Sender; me != nil {
		if me.Username != "" {
			ctx["sender_username"] = me.Username
		}
		nm := strings.TrimSpace(me.FirstName + " " + me.LastName)
		if nm != "" {
			ctx["sender_name"] = nm
		}
	}

	if m.IsReply() {
		ctx["reply_id"] = int64(m.ReplyToMsgID())
		if r, err := m.GetReplyMessage(); err == nil && r != nil {
			ctx["reply_sender_id"] = fmt.Sprintf("%d", r.SenderID())
			if rs := r.Sender; rs != nil {
				if rs.Username != "" {
					ctx["reply_sender_username"] = rs.Username
				}
				nm := strings.TrimSpace(rs.FirstName + " " + rs.LastName)
				if nm != "" {
					ctx["reply_sender_name"] = nm
				}
				ctx["reply_sender_is_bot"] = rs.Bot
			}

			if txt := strings.TrimSpace(r.Text()); txt != "" {
				ctx["reply_text"] = txt
				ctx["reply_has_text"] = true
			}

			if r.IsMedia() {
				ctx["reply_has_file"] = true
				ctx["replied_id"] = int64(r.ID)
				if r.File != nil && r.File.Name != "" {
					ctx["reply_filename"] = r.File.Name
				}
				ctx["reply_media_type"] = mediaKind(r)
			}
		}
	}

	maps.Copy(ctx, extras)
	return ctx
}

// mediaKind returns a short label for the media type on a message, used to
// teach the model what kind of attachment the user replied to.
func mediaKind(m *telegram.NewMessage) string {
	switch {
	case m.Photo() != nil:
		return "photo"
	case m.Video() != nil:
		return "video"
	case m.Voice() != nil:
		return "voice"
	case m.Audio() != nil:
		return "audio"
	case m.Sticker() != nil:
		return "sticker"
	case m.Document() != nil:
		return "document"
	case m.Animation() != nil:
		return "animation"
	}
	return "file"
}

func NewTelegramBot() (*TelegramBot, error) {
	if Cfg.TelegramAPIID == 0 || Cfg.TelegramAPIHash == "" || Cfg.TelegramBotToken == "" {
		return nil, fmt.Errorf("telegram not configured")
	}
	client, err := telegram.NewClient(telegram.ClientConfig{
		AppID:   int32(Cfg.TelegramAPIID),
		AppHash: Cfg.TelegramAPIHash,
		Proxy: &telegram.Socks5Proxy{
			BaseProxy: telegram.BaseProxy{
				Host: "103.214.23.203",
				Port: 1080,
			},
			Username: "tgproxy",
			Password: "0000",
		},
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
	b.client.OnCommand("settings", b.handleSettings)

	b.client.On(telegram.OnMessage, func(m *telegram.NewMessage) error {
		if m.Sender == nil || m.Sender.Bot || m.IsOutgoing() {
			return nil
		}
		if m.IsMedia() {
			return nil
		}
		text := m.Text()
		if text == "" || strings.HasPrefix(text, "/") {
			return nil
		}
		return b.handleText(m, text)
	})

	b.client.On(telegram.OnMessage, func(m *telegram.NewMessage) error {
		if m.Sender == nil || m.Sender.Bot || m.IsOutgoing() {
			return nil
		}
		if !m.IsMedia() {
			return nil
		}
		if m.Voice() != nil || m.Audio() != nil {
			return b.handleVoice(m)
		}
		return b.handleFile(m)
	}, telegram.HasMedia)

	b.client.OnInlineQuery(string(telegram.OnInline), func(iq *telegram.InlineQuery) error {
		userID := strconv.FormatInt(iq.SenderID, 10)
		if !IsSudo(userID) {
			builder := iq.Builder()
			builder.Article(
				"Ask ApexClaw",
				"You are not authorized to use this bot.",
				"Deploy your own: <pre language=\"bash\">curl -fsSL https://claw.gogram.fun | bash</pre>\n\nThen run: <pre language=\"bash\">apexclaw</pre>",
				&telegram.ArticleOptions{ID: "unauthorized", ReplyMarkup: telegram.InlineData("[unauthorized]", "[unauthorized]")},
			)
			_, err := iq.Answer(builder.Results(), &telegram.InlineSendOptions{CacheTime: 0})
			return err
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
			is.Edit("Error: Something went wrong processing your query.")
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

		// Handle /settings inline UI
		if after, ok := strings.CutPrefix(callbackData, "__SET:"); ok {
			b.handleSettingsCallbackData(c, after)
			return nil
		}

		// Handle max-iterations continue/stop buttons
		if callbackData == "__MAX_ITER_STOP__" {
			c.Edit("🛑 Stopped.", &telegram.SendOptions{ParseMode: telegram.HTML})
			c.Answer("Stopped.")
			return nil
		}
		if callbackData == "__MAX_ITER_CONTINUE__" {
			c.Edit("▶️ Continuing...", &telegram.SendOptions{ParseMode: telegram.HTML})
			c.Answer("Resuming...")
			session := GetOrCreateAgentSession(userID)
			onChunk, _, done := b.newStreamHandler(c.ChatID, int64(c.MessageID), userID)
			cbCtx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
			defer cancel()
			result, err := session.RunStream(cbCtx, userID, "Please continue from where you left off and complete the task.", onChunk)
			if strings.Contains(result, "[MAX_ITERATIONS]") {
				done()
				explanation := strings.TrimSpace(strings.Replace(result, "[MAX_ITERATIONS]\n", "", 1))
				if explanation == "" {
					explanation = "Hit the iteration limit again."
				}
				b.sendMaxIterButtons(c.ChatID, int64(c.MessageID), userID, explanation)
				return nil
			}
			done()
			if err != nil {
				c.Answer(fmt.Sprintf("Error: %v", err), &telegram.CallbackOptions{Alert: true})
			}
			return nil
		}

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
		cbCtx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
		defer cancel()
		_, err := session.RunStream(cbCtx, userID, cbMsg, onChunk)
		done()

		if err != nil {
			c.Answer(fmt.Sprintf("Error: %v", err), &telegram.CallbackOptions{Alert: true})
		}
		return nil
	})

	b.client.OnGuestChat(b.handleGuestChat)

	return nil
}

func (b *TelegramBot) handleGuestChat(g *telegram.GuestChatQuery) error {
	if g.Message == nil {
		return nil
	}
	text := strings.TrimSpace(g.Message.Text())
	if b.botUsername != "" {
		text = strings.TrimSpace(strings.TrimPrefix(text, "@"+b.botUsername))
	}
	if text == "" {
		_, err := g.Article("ApexClaw", "ask me something", "Send a message with @"+b.botUsername+" followed by your question.")
		return err
	}

	senderID := int64(0)
	if g.Message.Sender != nil {
		senderID = g.Message.Sender.ID
	}
	userID := strconv.FormatInt(senderID, 10)
	if !IsSudo(userID) {
		_, err := g.Article("ApexClaw", "unauthorized", "You are not authorized to use this bot. Deploy your own: curl -fsSL https://claw.gogram.fun | bash")
		return err
	}

	log.Printf("[TG] guest chat from %s (chat %d): %q", userID, g.Message.ChatID(), truncate(text, 80))

	ctxData := map[string]any{
		"sender_id":       userID,
		"telegram_id":     g.Message.ChatID(),
		"msg_id":          int64(g.Message.ID),
		"is_private_chat": false,
		"chat_type":       "guest",
		"guest_chat":      true,
	}
	setTelegramContext(userID, ctxData)
	defer deleteTelegramContext(userID)

	prefix := formatTGContext(ctxData)
	prompt := text
	if prefix != "" {
		prompt = prefix + "\n" + text
	}

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	session := GetOrCreateAgentSession(userID)
	result, err := session.RunStream(timeoutCtx, userID, prompt, func(string) {})
	if err != nil {
		log.Printf("[TG] guest agent error for %s: %v", userID, err)
		_, aErr := g.Article("ApexClaw", "error", "Sorry — something went wrong. Please try again.")
		return aErr
	}

	result = cleanResultForTelegram(result)
	result = strings.TrimSpace(result)
	if strings.Contains(result, "[MAX_ITERATIONS]") {
		result = strings.TrimSpace(strings.Replace(result, "[MAX_ITERATIONS]\n", "", 1))
		if result == "" {
			result = "Hit the iteration limit before completing the task."
		}
	}
	if result == "" {
		result = "Done."
	}
	if len(result) > 4000 {
		result = result[:4000] + "…"
	}

	desc := strings.TrimSpace(strings.SplitN(result, "\n", 2)[0])
	if len(desc) > 100 {
		desc = desc[:100] + "…"
	}

	_, err = g.Article("ApexClaw", desc, result, &telegram.ArticleOptions{ParseMode: telegram.HTML})
	return err
}

var (
	mentionApexRe = regexp.MustCompile(`(?i)\bapex(claw)?\b`)
	echoOpenerRe  = regexp.MustCompile(`(?i)^\s*apexclaw\s*:\s*`)
)

var recentlyHandled = struct {
	sync.Mutex
	ids map[string]time.Time
}{ids: make(map[string]time.Time)}

func alreadyHandled(chatID int64, msgID int32) bool {
	key := fmt.Sprintf("%d:%d", chatID, msgID)
	recentlyHandled.Lock()
	defer recentlyHandled.Unlock()
	now := time.Now()
	for k, t := range recentlyHandled.ids {
		if now.Sub(t) > 2*time.Minute {
			delete(recentlyHandled.ids, k)
		}
	}
	if _, ok := recentlyHandled.ids[key]; ok {
		return true
	}
	recentlyHandled.ids[key] = now
	return false
}

func (b *TelegramBot) isBotMentioned(text string) bool {
	if uname := strings.TrimSpace(b.botUsername); uname != "" {
		needle := "@" + strings.ToLower(uname)
		if strings.Contains(strings.ToLower(text), needle) {
			return true
		}
	}
	return mentionApexRe.MatchString(text)
}

func looksLikeBotEcho(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	return echoOpenerRe.MatchString(t)
}

func (b *TelegramBot) handleText(m *telegram.NewMessage, text string) error {
	if alreadyHandled(m.ChatID(), m.ID) {
		log.Printf("[TG] dropping duplicate event for chat=%d msg=%d", m.ChatID(), m.ID)
		return nil
	}
	userID := strconv.FormatInt(m.SenderID(), 10)
	if !IsSudo(userID) {
		return nil
	}

	if !m.IsPrivate() {
		mentioned := b.isBotMentioned(text)
		if !mentioned && m.IsReply() {
			if r, err := m.GetReplyMessage(); err == nil && r.SenderID() == b.client.Me().ID {
				mentioned = true
			}
		}
		if !mentioned {
			return nil
		}
	}

	if looksLikeBotEcho(text) {
		log.Printf("[TG] ignoring message that appears to be a quote of bot output: %q", truncate(text, 80))
		return nil
	}

	if existing := tryGetAgentSession(userID); existing != nil && existing.IsBusy() {
		log.Printf("[TG] dropping message from %s — session is mid-turn (%q)", userID, truncate(text, 60))
		return nil
	}

	log.Printf("[TG] msg from %s (chat %d): %q", userID, m.ChatID(), truncate(text, 80))
	requestID := fmt.Sprintf("%s:%d:%d", userID, m.ChatID(), m.ID)
	msgCtxData := buildMsgContext(m, userID, nil)

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	b.sendTyping(m)
	session := GetOrCreateAgentSession(userID)

	// If the user replied to a media message, eagerly download it and (for images)
	// attach to the ZAI session so the model can see/edit it without a separate
	// tool call. This lets the user say "edit this" or "what's in this" in a reply
	// and have it Just Work.
	if v, ok := msgCtxData["reply_has_file"]; ok && v == true {
		if r, err := m.GetReplyMessage(); err == nil && r != nil {
			if path, derr := r.Download(); derr == nil && path != "" {
				msgCtxData["reply_file_path"] = path
				log.Printf("[TG] auto-downloaded replied media to %s", path)
				if isImageFile(path) {
					upCtx, upCancel := context.WithTimeout(timeoutCtx, 30*time.Second)
					if zf, uerr := model.ZAIUpload(upCtx, path); uerr == nil && zf != nil {
						session.AttachZAIFile(*zf)
						msgCtxData["reply_image_attached"] = true
						log.Printf("[TG] attached replied image to zai session (file_id=%s)", zf.FileID)
					} else if uerr != nil {
						log.Printf("[TG] zai upload of replied image failed: %v", uerr)
					}
					upCancel()
				}
			} else if derr != nil {
				log.Printf("[TG] failed to download replied media: %v", derr)
			}
		}
	}

	setTelegramContext(requestID, msgCtxData)
	defer deleteTelegramContext(requestID)

	ctxPrefix := formatTGContext(msgCtxData)
	if ctxPrefix != "" {
		text = ctxPrefix + "\n" + text
	}

	onChunk, _, done := b.newStreamHandler(m.ChatID(), int64(m.ID), requestID)
	result, err := session.RunStream(timeoutCtx, requestID, text, onChunk)

	if err != nil {
		done()
		log.Printf("[TG] agent error for %s: %v", userID, err)
		b.safeSendText(m.ChatID(), 0, friendlyErrorMessage(err))
		return nil
	}

	result = cleanResultForTelegram(result)

	if strings.Contains(result, "[MAX_ITERATIONS]") {
		done() // clear progress message first
		explanation := strings.TrimSpace(strings.Replace(result, "[MAX_ITERATIONS]\n", "", 1))
		if explanation == "" {
			explanation = "Hit the iteration limit before completing the task."
		}
		b.sendMaxIterButtons(m.ChatID(), int64(m.ID), userID, explanation)
		return nil
	}

	done()
	return nil
}

func (b *TelegramBot) sendMaxIterButtons(chatID, replyToMsgID int64, userID, explanation string) {
	text := explanation + "\n\n<i>Reached the step limit. Would you like to continue?</i>"
	kb := telegram.NewKeyboard()
	kb.AddRow(
		telegram.Button.Data("▶️ Continue", "__MAX_ITER_CONTINUE__").Success(),
		telegram.Button.Data("🛑 Stop", "__MAX_ITER_STOP__").Danger(),
	)
	opts := &telegram.SendOptions{ParseMode: telegram.HTML, ReplyMarkup: kb.Build()}
	if replyToMsgID > 0 {
		opts.ReplyID = int32(replyToMsgID)
	}
	b.client.SendMessage(chatID, text, opts)
}

func (b *TelegramBot) handleVoice(m *telegram.NewMessage) error {
	if alreadyHandled(m.ChatID(), m.ID) {
		log.Printf("[TG] dropping duplicate voice event for chat=%d msg=%d", m.ChatID(), m.ID)
		return nil
	}
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
		_, _ = m.Reply("Error: Failed to download voice message.")
		return nil
	}
	defer os.Remove(audioPath)

	transcribed, err := transcribeAudio(audioPath)
	if err != nil {
		log.Printf("[TG] transcription error: %v", err)
		_, _ = m.Reply("Error: Could not transcribe voice message. Try typing your message.")
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
		_, _ = m.Reply(friendlyErrorMessage(err))
	}
	return nil
}

func (b *TelegramBot) handleFile(m *telegram.NewMessage) error {
	if alreadyHandled(m.ChatID(), m.ID) {
		log.Printf("[TG] dropping duplicate file event for chat=%d msg=%d", m.ChatID(), m.ID)
		return nil
	}
	userID := strconv.FormatInt(m.SenderID(), 10)
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
	b.sendTyping(m)

	filePath, err := m.Download()
	if err != nil {
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

	if isImageFile(filePath) {
		upCtx, upCancel := context.WithTimeout(ctx, 30*time.Second)
		zf, uerr := model.ZAIUpload(upCtx, filePath)
		upCancel()
		if uerr == nil && zf != nil {
			session.AttachZAIFile(*zf)
			log.Printf("[TG] attached %s to zai session (file_id=%s)", fileName, zf.FileID)
		} else if uerr != nil {
			log.Printf("[TG] zai upload failed for %s: %v", fileName, uerr)
		}
	}

	if _, err = session.Run(ctx, userID, caption); err != nil {
		log.Printf("[TG] agent error for file: %v", err)
		_, _ = m.Reply(friendlyErrorMessage(err))
	}
	return nil
}

func isImageFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp":
		return true
	}
	return false
}

func friendlyErrorMessage(err error) string {
	if err == nil {
		return "Something went wrong. Please try again."
	}
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "zai: content blocked"):
		return "⚠️ The AI service rejected this content (safety filter). Try rephrasing."
	case strings.Contains(s, "zai: empty response"):
		return "⚠️ The AI returned an empty response — the conversation has been reset. Try again."
	case strings.Contains(s, "zai: unauthorized"):
		return "⚠️ The AI session expired. Try again."
	case strings.Contains(s, "upstream 429"):
		return "⚠️ Rate-limited by the AI service. Wait a few seconds and retry."
	case strings.Contains(s, "upstream 5"):
		return "⚠️ The AI service is having issues. Try again in a moment."
	case strings.Contains(s, "context deadline exceeded"), strings.Contains(s, "timeout"):
		return "⏱️ The request timed out. Try a shorter prompt or retry."
	case strings.Contains(s, "max_iterations"):
		return "⚠️ Hit the iteration limit. The task may need to be broken down."
	}
	return "Something went wrong. Please try again."
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
	prevLine := ""
	for _, line := range lines {
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
	result = strings.Join(cleaned, "\n")
	result = stripMarkdown(result)
	return strings.TrimSpace(result)
}

var allowedTagsRe = regexp.MustCompile(`(?i)(</?(?:b|strong|i|em|u|ins|s|strike|del|code|pre|blockquote|spoiler)>|<a href="[^"]*">|<code class="[^"]*">|<pre language="[^"]*">|<span class="tg-spoiler">|</span>)`)

func stripMarkdown(s string) string {
	s = regexp.MustCompile(`(?m)(?:^\s*\|.*\|\s*$\r?\n?)+`).ReplaceAllStringFunc(s, func(table string) string {
		return "<pre>\n" + strings.TrimSpace(table) + "\n</pre>\n"
	})

	s = regexp.MustCompile(`(?s)\*\*(.*?)\*\*`).ReplaceAllString(s, "<b>$1</b>")
	s = regexp.MustCompile(`(?s)__(.*?)__`).ReplaceAllString(s, "<b>$1</b>")
	s = regexp.MustCompile(`(?s)\*(.*?)\*`).ReplaceAllString(s, "<i>$1</i>")
	s = regexp.MustCompile("(?s)```[a-zA-Z0-9_+-]*\n?(.*?)```").ReplaceAllString(s, "<pre>$1</pre>")
	s = regexp.MustCompile("(?s)`([^`]+)`").ReplaceAllString(s, "<code>$1</code>")
	s = regexp.MustCompile(`(?m)^#+\s+(.*)$`).ReplaceAllString(s, "<b>$1</b>")
	s = regexp.MustCompile(`(?:\[([^\]]+)\])\(([^)]+)\)`).ReplaceAllString(s, "<a href=\"$2\">$1</a>")
	s = strings.ReplaceAll(s, "`", "")

	var mapping []string
	protected := allowedTagsRe.ReplaceAllStringFunc(s, func(tag string) string {
		mapping = append(mapping, tag)
		return fmt.Sprintf("____TG_TAG_%d____", len(mapping)-1)
	})

	escaped := html.EscapeString(protected)

	for i, tag := range mapping {
		placeholder := fmt.Sprintf("____TG_TAG_%d____", i)
		escaped = strings.Replace(escaped, placeholder, tag, 1)
	}

	escaped = regexp.MustCompile(`\n{3,}`).ReplaceAllString(escaped, "\n\n")

	return strings.TrimSpace(escaped)
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

// isTGSendTool returns true for tool names that directly deliver a message to
// the Telegram chat. When one of these succeeds, the agent's final text
// response is suppressed to prevent a redundant second message.
var tgSendTools = map[string]bool{
	"tg_send_message":         true,
	"tg_send_file":            true,
	"tg_send_photo":           true,
	"tg_send_photo_url":       true,
	"tg_send_album":           true,
	"tg_broadcast":            true,
	"tg_send_message_buttons": true,
	"tg_send_location":        true,
}

func isTGSendTool(label string) bool {
	// label may be "tool_name" or "tool_name(args...)" — check prefix
	for name := range tgSendTools {
		if label == name || strings.HasPrefix(label, name+"(") {
			return true
		}
	}
	return false
}

func (b *TelegramBot) newStreamHandler(chatID int64, replyToMsgID int64, senderID string) (func(string), func(), func()) {
	type stepEntry struct {
		id     string
		label  string
		status string // "running" | "done" | "failed"
		errMsg string // populated when status == "failed"
	}

	var (
		progressMsgID int32
		steps         []stepEntry
		commentary    []string
		lastEditAt    time.Time
		lastEditText  string
		finalBuf      strings.Builder
		mu            sync.Mutex
		sentDirect    bool // true if a tg_send_* tool successfully ran
	)

	buildProgressText := func() string {
		var sb strings.Builder

		// Show recent commentary inline as italic.
		if len(commentary) > 0 {
			tail := commentary
			if len(tail) > 2 {
				tail = tail[len(tail)-2:]
			}
			for _, c := range tail {
				c = strings.TrimSpace(c)
				if c == "" {
					continue
				}
				if len(c) > 200 {
					c = c[:200] + "…"
				}
				fmt.Fprintf(&sb, "<i>%s</i>\n\n", escapeHTML(c))
			}
		}

		if len(steps) == 0 {
			if sb.Len() == 0 {
				return "<i>Thinking…</i>"
			}
			return strings.TrimRight(sb.String(), "\n")
		}

		// Group identical labels — show "× N" when the same tool runs multiple times.
		type rendered struct {
			label  string
			status string
			errMsg string
			count  int
		}
		var rows []rendered
		showFrom := 0
		if len(steps) > 6 {
			showFrom = len(steps) - 6
		}
		for _, st := range steps[showFrom:] {
			if len(rows) > 0 && rows[len(rows)-1].label == st.label && rows[len(rows)-1].status == st.status {
				rows[len(rows)-1].count++
				continue
			}
			rows = append(rows, rendered{label: st.label, status: st.status, errMsg: st.errMsg, count: 1})
		}

		for _, r := range rows {
			suffix := ""
			if r.count > 1 {
				suffix = fmt.Sprintf(" × %d", r.count)
			}
			switch r.status {
			case "running":
				fmt.Fprintf(&sb, "⏳ <i>%s</i>%s\n", escapeHTML(r.label), suffix)
			case "done":
				fmt.Fprintf(&sb, "✅ %s%s\n", escapeHTML(r.label), suffix)
			case "failed":
				errText := r.errMsg
				if len(errText) > 90 {
					errText = errText[:90] + "…"
				}
				fmt.Fprintf(&sb, "❌ %s%s\n", escapeHTML(r.label), suffix)
				if errText != "" {
					fmt.Fprintf(&sb, "   <code>%s</code>\n", escapeHTML(errText))
				}
			}
		}
		return strings.TrimRight(sb.String(), "\n")
	}

	// editProgress: smart cadence — always render terminal events (done/failed),
	// otherwise debounce to every 1.5s OR when the rendered text actually changes.
	editProgress := func(force bool) {
		mu.Lock()
		defer mu.Unlock()
		text := buildProgressText()

		if progressMsgID == 0 {
			opts := &telegram.SendOptions{ParseMode: telegram.HTML}
			if replyToMsgID > 0 {
				opts.ReplyID = int32(replyToMsgID)
			}
			m, err := b.client.SendMessage(chatID, text, opts)
			if err == nil {
				progressMsgID = int32(m.ID)
				lastEditAt = time.Now()
				lastEditText = text
			}
			return
		}
		if text == lastEditText {
			return
		}
		shouldEdit := force || time.Since(lastEditAt) > 1500*time.Millisecond
		if shouldEdit {
			b.client.EditMessage(chatID, progressMsgID, text, &telegram.SendOptions{ParseMode: telegram.HTML})
			lastEditAt = time.Now()
			lastEditText = text
		}
	}

	onChunk := func(chunk string) {
		// __COMMENTARY:<text>__\n — the model's prose around tool calls.
		if after, ok := strings.CutPrefix(chunk, "__COMMENTARY:"); ok {
			text := strings.TrimSuffix(after, "__\n")
			text = strings.TrimSpace(text)
			if text == "" {
				return
			}
			mu.Lock()
			commentary = append(commentary, text)
			mu.Unlock()
			editProgress(false)
			return
		}
		// __TOOL_CALL:<id>\x1f<label>__\n  (unit-separator delimited so labels can contain '|')
		if after, ok := strings.CutPrefix(chunk, "__TOOL_CALL:"); ok {
			raw := strings.TrimSuffix(after, "__\n")
			id, label, found := strings.Cut(raw, "\x1f")
			if !found {
				id, label, _ = strings.Cut(raw, "|")
			}
			mu.Lock()
			steps = append(steps, stepEntry{id: id, label: label, status: "running"})
			mu.Unlock()
			editProgress(false)
			return
		}
		// __TOOL_RESULT:<id>\x1f<label>\x1f<status>__\n
		if after, ok := strings.CutPrefix(chunk, "__TOOL_RESULT:"); ok {
			raw := strings.TrimSuffix(after, "__\n")
			parts := strings.SplitN(raw, "\x1f", 3)
			if len(parts) < 3 {
				parts = strings.SplitN(raw, "|", 3)
				if len(parts) < 3 {
					return
				}
			}
			id, label, statusRaw := parts[0], parts[1], parts[2]

			hasErr := false
			mu.Lock()
			if statusRaw == "ok" && isTGSendTool(label) {
				sentDirect = true
			}
			// Match by id first (more reliable across parallel calls), fall back to label.
			matched := false
			for i := len(steps) - 1; i >= 0; i-- {
				if steps[i].status != "running" {
					continue
				}
				if (id != "" && steps[i].id == id) || steps[i].label == label {
					if statusRaw == "ok" {
						steps[i].status = "done"
					} else {
						steps[i].status = "failed"
						steps[i].errMsg = strings.TrimPrefix(statusRaw, "err:")
						hasErr = true
					}
					matched = true
					break
				}
			}
			_ = matched
			mu.Unlock()
			editProgress(hasErr)
			return
		}

		// Strip \x00PROGRESS:...\x00 markers (from web UI progress tool)
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
		mu.Lock()
		finalBuf.WriteString(chunk)
		finalBuf.WriteString("\n")
		mu.Unlock()
	}

	flush := func() {}

	done := func() {
		clearProgressMsg(senderID)

		mu.Lock()
		msgID := progressMsgID
		result := strings.TrimSpace(finalBuf.String())
		alreadySent := sentDirect
		mu.Unlock()

		const maxLen = 3800
		editOpts := &telegram.SendOptions{ParseMode: telegram.HTML}

		if alreadySent {
			if msgID != 0 {
				b.client.EditMessage(chatID, msgID, "<i>done</i>", editOpts)
			}
			return
		}

		if result == "" {
			if msgID != 0 {
				b.client.EditMessage(chatID, msgID, "<i>done</i>", editOpts)
			}
			return
		}

		result = stripMarkdown(result)

		if msgID != 0 && len(result) <= maxLen {
			if _, err := b.client.EditMessage(chatID, msgID, result, editOpts); err == nil {
				return
			}
		}

		first := true
		for len(result) > 0 {
			chunk := result
			if len(chunk) > maxLen {
				cut := strings.LastIndex(result[:maxLen], "\n")
				if cut < 100 {
					cut = maxLen
				}
				chunk = result[:cut]
				result = strings.TrimSpace(result[cut:])
			} else {
				result = ""
			}
			if first && msgID != 0 {
				if _, err := b.client.EditMessage(chatID, msgID, chunk, editOpts); err == nil {
					first = false
					continue
				}
				msgID = 0
			}
			b.safeSendText(chatID, replyToMsgID, chunk)
			first = false
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
		msg += "\n\nSudo Management:\n" +
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
	_, err := m.Reply("Conversation cleared.")
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
			_, err := m.Reply("Error: Code must be exactly 6 digits.")
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
		_, err := m.Reply(fmt.Sprintf("Web login code changed!\nOld: `%s`\nNew: `%s`", oldCode, newCode))
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

// ─── /settings command & inline UI ───────────────────────────────────────────

func (b *TelegramBot) handleSettings(m *telegram.NewMessage) error {
	userID := strconv.FormatInt(m.SenderID(), 10)
	if !IsSudo(userID) {
		return nil
	}
	text, kb := buildSettingsMenu()
	_, err := m.Reply(text, &telegram.SendOptions{ParseMode: telegram.HTML, ReplyMarkup: kb})
	return err
}

func buildSettingsMenu() (string, *telegram.ReplyInlineMarkup) {
	provider := model.GetActiveProvider()
	ps := model.GetProviderSettings(provider)
	hasKey := ps.APIKey != ""

	keyStatus := "❌ no key"
	if provider == "zai" {
		keyStatus = "✅ built-in"
	} else if hasKey {
		keyStatus = "✅ set"
	}

	text := fmt.Sprintf(
		"⚙️ <b>Settings</b>\n\n"+
			"Provider: <b>%s</b>\n"+
			"Model: <b>%s</b>\n"+
			"API Key: %s\n"+
			"Max Tokens: <b>%d</b>\n"+
			"Temperature: <b>%.2f</b>",
		provider, ps.Model, keyStatus, ps.MaxTokens, ps.Temperature,
	)

	kb := telegram.NewKeyboard()
	kb.AddRow(
		telegram.Button.Data("🔌 Provider", "__SET:provider__"),
		telegram.Button.Data("🤖 Model", "__SET:model__"),
	)
	kb.AddRow(
		telegram.Button.Data("📊 Max Tokens", "__SET:maxtokens__"),
		telegram.Button.Data("🌡 Temperature", "__SET:temperature__"),
	)
	kb.AddRow(telegram.Button.Data("❌ Close", "__SET:close__"))
	return text, kb.Build()
}

func (b *TelegramBot) handleSettingsCallbackData(c *telegram.CallbackQuery, raw string) {
	// Strip trailing __ (used in button data to make callbacks unique)
	sub := strings.TrimSuffix(raw, "__")
	provider := model.GetActiveProvider()
	c.Answer("")

	switch {
	case sub == "provider":
		text := "🔌 <b>Select Provider</b>"
		kb := telegram.NewKeyboard()
		var row []telegram.KeyboardButton
		for i, p := range model.KnownProviders {
			label := p
			if p == provider {
				label = "✅ " + p
			}
			row = append(row, telegram.Button.Data(label, "__SET:setprov:"+p))
			if len(row) == 2 || i == len(model.KnownProviders)-1 {
				kb.AddRow(row...)
				row = nil
			}
		}
		kb.AddRow(telegram.Button.Data("« Back", "__SET:back__"))
		c.Edit(text, &telegram.SendOptions{ParseMode: telegram.HTML, ReplyMarkup: kb.Build()})

	case sub == "model":
		models := model.KnownModels[provider]
		cur := model.GetProviderSettings(provider).Model
		text := fmt.Sprintf("🤖 <b>Select Model</b>\nProvider: <b>%s</b>", provider)
		kb := telegram.NewKeyboard()
		for _, mdl := range models {
			label := mdl
			if mdl == cur {
				label = "✅ " + mdl
			}
			kb.AddRow(telegram.Button.Data(label, "__SET:setmodel:"+mdl))
		}
		kb.AddRow(telegram.Button.Data("« Back", "__SET:back__"))
		c.Edit(text, &telegram.SendOptions{ParseMode: telegram.HTML, ReplyMarkup: kb.Build()})

	case sub == "maxtokens":
		cur := model.GetProviderSettings(provider).MaxTokens
		text := fmt.Sprintf("📊 <b>Max Tokens</b> (current: %d)", cur)
		kb := telegram.NewKeyboard()
		presets := []int{2048, 4096, 8192, 16384, 32768}
		var row []telegram.KeyboardButton
		for i, v := range presets {
			label := fmt.Sprintf("%d", v)
			if v == cur {
				label = "✅ " + label
			}
			row = append(row, telegram.Button.Data(label, fmt.Sprintf("__SET:setmaxtok:%d", v)))
			if len(row) == 3 || i == len(presets)-1 {
				kb.AddRow(row...)
				row = nil
			}
		}
		kb.AddRow(telegram.Button.Data("« Back", "__SET:back__"))
		c.Edit(text, &telegram.SendOptions{ParseMode: telegram.HTML, ReplyMarkup: kb.Build()})

	case sub == "temperature":
		cur := model.GetProviderSettings(provider).Temperature
		text := fmt.Sprintf("🌡 <b>Temperature</b> (current: %.2f)", cur)
		kb := telegram.NewKeyboard()
		presets := []float64{0.0, 0.3, 0.5, 0.7, 1.0, 1.2}
		var row []telegram.KeyboardButton
		for i, v := range presets {
			label := fmt.Sprintf("%.1f", v)
			if v == cur {
				label = "✅ " + label
			}
			row = append(row, telegram.Button.Data(label, fmt.Sprintf("__SET:settemp:%.1f", v)))
			if len(row) == 3 || i == len(presets)-1 {
				kb.AddRow(row...)
				row = nil
			}
		}
		kb.AddRow(telegram.Button.Data("« Back", "__SET:back__"))
		c.Edit(text, &telegram.SendOptions{ParseMode: telegram.HTML, ReplyMarkup: kb.Build()})

	case strings.HasPrefix(sub, "setprov:"):
		newProv := strings.TrimPrefix(sub, "setprov:")
		model.SetProvider(newProv)
		settingsEditMenu(c, sub)

	case strings.HasPrefix(sub, "setmodel:"):
		newModel := strings.TrimPrefix(sub, "setmodel:")
		model.SetProviderModel(provider, newModel)
		Cfg.DefaultModel = newModel
		settingsEditMenu(c, sub)

	case strings.HasPrefix(sub, "setmaxtok:"):
		val, _ := strconv.Atoi(strings.TrimPrefix(sub, "setmaxtok:"))
		if val > 0 {
			model.UpdateProviderSettings(provider, func(ps *model.ProviderSettings) {
				ps.MaxTokens = val
			})
		}
		settingsEditMenu(c, sub)

	case strings.HasPrefix(sub, "settemp:"):
		var val float64
		fmt.Sscanf(strings.TrimPrefix(sub, "settemp:"), "%f", &val)
		model.UpdateProviderSettings(provider, func(ps *model.ProviderSettings) {
			ps.Temperature = val
		})
		settingsEditMenu(c, sub)

	case sub == "back":
		settingsEditMenu(c, sub)

	case sub == "close":
		if _, err := c.Delete(); err != nil {
			log.Printf("[SETTINGS] delete error: %v", err)
		}
	}
}

func settingsEdit(c *telegram.CallbackQuery, action, text string, kb *telegram.ReplyInlineMarkup) {
	_, err := c.Edit(text, &telegram.SendOptions{ParseMode: telegram.HTML, ReplyMarkup: kb})
	if err != nil {
		log.Printf("[SETTINGS] edit(%s) error: %v (chatID=%d msgID=%d)", action, err, c.ChatID, c.MessageID)
	}
}

func settingsEditMenu(c *telegram.CallbackQuery, action string) {
	text, kb := buildSettingsMenu()
	settingsEdit(c, action, text, kb)
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
		fmt.Fprintf(&sb, "<b>Sudo Users (%d):</b>\n", len(Cfg.SudoIDs))
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
		_, err := m.Reply("Error: That's the owner!")
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
			_, err := m.Reply(fmt.Sprintf("User <code>%s</code> is already a sudo user.", targetID), &telegram.SendOptions{ParseMode: telegram.HTML})
			return err
		}
		newSudos = append(currentSudos, targetID)
		_, _ = m.Reply(fmt.Sprintf("Added <code>%s</code> to sudo users.", targetID), &telegram.SendOptions{ParseMode: telegram.HTML})
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
			_, err := m.Reply(fmt.Sprintf("Error: User <code>%s</code> is not a sudo user.", targetID), &telegram.SendOptions{ParseMode: telegram.HTML})
			return err
		}
		_, _ = m.Reply(fmt.Sprintf("Removed <code>%s</code> from sudo users.", targetID), &telegram.SendOptions{ParseMode: telegram.HTML})
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
