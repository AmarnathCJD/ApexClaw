package core

import (
	"context"
	"fmt"
	"log"
	"mime"
	"os"
	"path/filepath"
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

var waBot *WhatsAppBot

func GetWhatsAppBot() *WhatsAppBot { return waBot }

func InitWhatsAppBot() (*WhatsAppBot, error) {
	b, err := NewWhatsAppBot()
	if err != nil {
		return nil, err
	}
	waBot = b
	return b, nil
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

func (b *WhatsAppBot) isGroup(chatJID types.JID) bool {
	return chatJID.Server == types.GroupServer
}

// isBotReplied returns true if the message is a reply to a message sent by this bot.
func (b *WhatsAppBot) isBotReplied(v *events.Message) bool {
	ext := v.Message.GetExtendedTextMessage()
	if ext == nil {
		return false
	}
	ctx := ext.GetContextInfo()
	if ctx == nil {
		return false
	}
	myJID := b.client.Store.ID
	if myJID == nil {
		return false
	}
	participant := ctx.GetParticipant()
	return strings.HasPrefix(participant, myJID.User+"@") || participant == myJID.User
}

func (b *WhatsAppBot) eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		if v.Info.IsFromMe {
			return
		}
		isGroup := b.isGroup(v.Info.Chat)
		ownerID := os.Getenv("WA_OWNER_ID")
		userID := v.Info.Sender.User
		if ownerID != "" && userID != ownerID {
			return
		}

		text := v.Message.GetConversation()
		if text == "" && v.Message.GetExtendedTextMessage() != nil {
			text = v.Message.GetExtendedTextMessage().GetText()
		}

		if text != "" {
			// In groups, only respond when the bot's own message was replied to
			if isGroup && !b.isBotReplied(v) {
				return
			}
			log.Printf("[WA] msg from %s (group=%v): %q", userID, isGroup, truncate(text, 80))
			chat := v.Info.Chat
			go func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[WA] handleText panic recovered: %v", r)
					}
				}()
				b.handleText(chat, userID, text, isGroup)
			}()
			return
		}

		// Handle incoming media
		if v.Message.GetImageMessage() != nil || v.Message.GetDocumentMessage() != nil ||
			v.Message.GetAudioMessage() != nil || v.Message.GetVideoMessage() != nil {
			if isGroup && !b.isBotReplied(v) {
				return
			}
			msg := v
			go func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[WA] handleIncomingMedia panic recovered: %v", r)
					}
				}()
				b.handleIncomingMedia(msg)
			}()
		}
	}
}

func (b *WhatsAppBot) handleIncomingMedia(v *events.Message) {
	userID := v.Info.Sender.User
	chatID := v.Info.Chat

	var data []byte
	var caption, mimeType, fileName string
	var err error

	dlCtx := context.Background()
	switch {
	case v.Message.GetImageMessage() != nil:
		img := v.Message.GetImageMessage()
		data, err = b.client.Download(dlCtx, img)
		caption, mimeType, fileName = img.GetCaption(), img.GetMimetype(), "image.jpg"
	case v.Message.GetDocumentMessage() != nil:
		doc := v.Message.GetDocumentMessage()
		data, err = b.client.Download(dlCtx, doc)
		caption, mimeType, fileName = doc.GetCaption(), doc.GetMimetype(), doc.GetFileName()
		if fileName == "" {
			fileName = "document"
		}
	case v.Message.GetAudioMessage() != nil:
		audio := v.Message.GetAudioMessage()
		data, err = b.client.Download(dlCtx, audio)
		mimeType, fileName = audio.GetMimetype(), "audio.ogg"
	case v.Message.GetVideoMessage() != nil:
		vid := v.Message.GetVideoMessage()
		data, err = b.client.Download(dlCtx, vid)
		caption, mimeType, fileName = vid.GetCaption(), vid.GetMimetype(), "video.mp4"
	}

	if err != nil {
		log.Printf("[WA] media download error: %v", err)
		return
	}

	ext := filepath.Ext(fileName)
	if ext == "" && mimeType != "" {
		if exts, _ := mime.ExtensionsByType(mimeType); len(exts) > 0 {
			ext = exts[0]
		}
	}
	tmp, err := os.CreateTemp("", "wa_media_*"+ext)
	if err != nil {
		return
	}
	tmp.Write(data)
	tmp.Close()
	defer os.Remove(tmp.Name())

	if caption == "" {
		caption = fmt.Sprintf("Process this file: %s", fileName)
	}
	msgCtxData := map[string]any{
		"sender_id": userID, "platform": "whatsapp",
		"chat_id": chatID.String(), "file_name": fileName, "file_path": tmp.Name(),
	}
	setTelegramContext(userID, msgCtxData)
	ctxPrefix := formatTGContext(msgCtxData)
	if ctxPrefix != "" {
		caption = ctxPrefix + "\n" + caption
	}

	b.client.SendChatPresence(context.Background(), chatID, types.ChatPresenceComposing, types.ChatPresenceMediaText)
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	session := GetOrCreateAgentSession("wa_" + userID)
	onChunk, _, done := b.newStreamHandler(chatID, "wa_"+userID)
	result, err := session.RunStream(timeoutCtx, "wa_"+userID, caption, onChunk)
	b.client.SendChatPresence(context.Background(), chatID, types.ChatPresencePaused, types.ChatPresenceMediaText)
	done()
	if err != nil {
		log.Printf("[WA] media agent error: %v", err)
		b.safeSendText(chatID, "Something went wrong processing the file.")
		return
	}
	result = cleanResultForWhatsApp(result)
	if result != "" && !strings.Contains(result, "[MAX_ITERATIONS]") {
		b.safeSendText(chatID, result)
	}
}

// SendMedia uploads and sends a file (image/video/audio/document) to a WhatsApp chat.
func (b *WhatsAppBot) SendMedia(chatID types.JID, filePath, caption, mediaType string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	mimeType := mime.TypeByExtension(filepath.Ext(filePath))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	ctx := context.Background()
	switch mediaType {
	case "image":
		resp, err := b.client.Upload(ctx, data, whatsmeow.MediaImage)
		if err != nil {
			return err
		}
		_, err = b.client.SendMessage(ctx, chatID, &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
			Caption: proto.String(caption), Mimetype: proto.String(mimeType),
			URL: proto.String(resp.URL), DirectPath: proto.String(resp.DirectPath),
			MediaKey: resp.MediaKey, FileEncSHA256: resp.FileEncSHA256,
			FileSHA256: resp.FileSHA256, FileLength: proto.Uint64(resp.FileLength),
		}})
		return err
	case "video":
		resp, err := b.client.Upload(ctx, data, whatsmeow.MediaVideo)
		if err != nil {
			return err
		}
		_, err = b.client.SendMessage(ctx, chatID, &waE2E.Message{VideoMessage: &waE2E.VideoMessage{
			Caption: proto.String(caption), Mimetype: proto.String(mimeType),
			URL: proto.String(resp.URL), DirectPath: proto.String(resp.DirectPath),
			MediaKey: resp.MediaKey, FileEncSHA256: resp.FileEncSHA256,
			FileSHA256: resp.FileSHA256, FileLength: proto.Uint64(resp.FileLength),
		}})
		return err
	case "audio":
		resp, err := b.client.Upload(ctx, data, whatsmeow.MediaAudio)
		if err != nil {
			return err
		}
		_, err = b.client.SendMessage(ctx, chatID, &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
			Mimetype: proto.String(mimeType), URL: proto.String(resp.URL),
			DirectPath: proto.String(resp.DirectPath), MediaKey: resp.MediaKey,
			FileEncSHA256: resp.FileEncSHA256, FileSHA256: resp.FileSHA256,
			FileLength: proto.Uint64(resp.FileLength),
		}})
		return err
	default: // document
		resp, err := b.client.Upload(ctx, data, whatsmeow.MediaDocument)
		if err != nil {
			return err
		}
		_, err = b.client.SendMessage(ctx, chatID, &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
			Caption: proto.String(caption), FileName: proto.String(filepath.Base(filePath)),
			Mimetype: proto.String(mimeType), URL: proto.String(resp.URL),
			DirectPath: proto.String(resp.DirectPath), MediaKey: resp.MediaKey,
			FileEncSHA256: resp.FileEncSHA256, FileSHA256: resp.FileSHA256,
			FileLength: proto.Uint64(resp.FileLength),
		}})
		return err
	}
}

func (b *WhatsAppBot) handleText(chatID types.JID, userID string, text string, isGroup bool) {
	msgCtxData := map[string]any{
		"sender_id": userID,
		"platform":  "whatsapp",
		"chat_id":   chatID.String(),
		"is_group":  isGroup,
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
		b.safeSendText(chatID, "Something went wrong. Please try again.")
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

// WABotSendMessage sends a text message via the global WA bot to the given JID.
// jid can be a plain phone number ("9193786543210") or full JID.
func WABotSendMessage(jid, text string) string {
	if waBot == nil {
		return "Error: WhatsApp not connected"
	}
	if !strings.Contains(jid, "@") {
		jid = jid + "@s.whatsapp.net"
	}
	parsedJID, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Sprintf("Error: invalid JID %q: %v", jid, err)
	}
	_, err = waBot.client.SendMessage(context.Background(), parsedJID, &waE2E.Message{
		Conversation: proto.String(text),
	})
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return "Sent"
}

// WABotSendFile sends a media file via the global WA bot.
func WABotSendFile(jid, filePath, caption, mediaType string) string {
	if waBot == nil {
		return "Error: WhatsApp not connected"
	}
	if !strings.Contains(jid, "@") {
		jid = jid + "@s.whatsapp.net"
	}
	parsedJID, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Sprintf("Error: invalid JID %q: %v", jid, err)
	}
	if err := waBot.SendMedia(parsedJID, filePath, caption, mediaType); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return "Sent"
}

// WABotGetContacts returns a list of saved WA contacts.
func WABotGetContacts() string {
	if waBot == nil {
		return "Error: WhatsApp not connected"
	}
	contacts, err := waBot.client.Store.Contacts.GetAllContacts(context.Background())
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	if len(contacts) == 0 {
		return "No contacts found"
	}
	var sb strings.Builder
	count := 0
	for jid, info := range contacts {
		if count >= 100 {
			fmt.Fprintf(&sb, "... and %d more\n", len(contacts)-count)
			break
		}
		name := info.FullName
		if name == "" {
			name = info.PushName
		}
		if name == "" {
			name = jid.User
		}
		fmt.Fprintf(&sb, "%s: %s\n", name, jid.String())
		count++
	}
	return strings.TrimSpace(sb.String())
}

// WABotGetGroups returns joined WA groups.
func WABotGetGroups() string {
	if waBot == nil {
		return "Error: WhatsApp not connected"
	}
	groups, err := waBot.client.GetJoinedGroups(context.Background())
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	if len(groups) == 0 {
		return "Not in any groups"
	}
	var sb strings.Builder
	for _, g := range groups {
		fmt.Fprintf(&sb, "%s: %s\n", g.Name, g.JID.String())
	}
	return strings.TrimSpace(sb.String())
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
