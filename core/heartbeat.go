package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
)

type ScheduledTask struct {
	ID          string `json:"id"`
	Prompt      string `json:"prompt"`
	RunAt       string `json:"run_at"`
	Repeat      string `json:"repeat"`
	OwnerID     string `json:"owner_id"`
	TelegramID  int64  `json:"telegram_id"`
	MessageID   int64  `json:"message_id"`
	GroupID     int64  `json:"group_id"`
	Label       string `json:"label"`
	CreatedAt   string `json:"created_at"`
	ScheduledAt string `json:"scheduled_at"`
}

type heartbeatStore struct {
	mu    sync.Mutex
	tasks []ScheduledTask
}

var hbStore = &heartbeatStore{}
var heartbeatTGClient *telegram.Client

func heartbeatPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".apexclaw", "heartbeat.json")
}

func loadHeartbeatTasks() {
	hbStore.mu.Lock()
	defer hbStore.mu.Unlock()
	data, err := os.ReadFile(heartbeatPath())
	if err != nil {
		return
	}
	var all []ScheduledTask
	if err := json.Unmarshal(data, &all); err != nil {
		return
	}

	now := time.Now()
	for _, t := range all {
		runAt, err := time.Parse(time.RFC3339, t.RunAt)
		if err != nil {
			continue
		}
		if t.Repeat == "" && now.After(runAt) {
			log.Printf("[HEARTBEAT] dropping stale one-shot task %q (was due %s)", t.Label, t.RunAt)
			continue
		}
		hbStore.tasks = append(hbStore.tasks, t)
	}
}

func persistHeartbeatTasks() {
	hbStore.mu.Lock()
	defer hbStore.mu.Unlock()
	path := heartbeatPath()
	os.MkdirAll(filepath.Dir(path), 0755)
	data, _ := json.MarshalIndent(hbStore.tasks, "", "  ")
	os.WriteFile(path, data, 0644)
}

func ScheduleTask(t ScheduledTask) {
	now := time.Now().Format(time.RFC3339)
	if t.CreatedAt == "" {
		t.CreatedAt = now
	}
	if t.ScheduledAt == "" {
		t.ScheduledAt = t.RunAt
	}

	hbStore.mu.Lock()
	for i, existing := range hbStore.tasks {
		if existing.Label == t.Label {
			hbStore.tasks[i] = t
			hbStore.mu.Unlock()
			persistHeartbeatTasks()
			log.Printf("[HEARTBEAT] updated task %q → run_at=%s", t.Label, t.RunAt)
			return
		}
	}
	hbStore.tasks = append(hbStore.tasks, t)
	hbStore.mu.Unlock()
	persistHeartbeatTasks()
	log.Printf("[HEARTBEAT] added task %q → run_at=%s owner=%s chat=%d", t.Label, t.RunAt, t.OwnerID, t.TelegramID)
}

func CancelTask(labelOrID string) bool {
	hbStore.mu.Lock()
	defer hbStore.mu.Unlock()
	for i, t := range hbStore.tasks {
		if t.Label == labelOrID || t.ID == labelOrID {
			hbStore.tasks = append(hbStore.tasks[:i], hbStore.tasks[i+1:]...)
			go persistHeartbeatTasks()
			return true
		}
	}
	return false
}

func StartHeartbeat(client *telegram.Client) {
	heartbeatTGClient = client
	loadHeartbeatTasks()
	go func() {
		for {
			time.Sleep(15 * time.Second)
			runHeartbeatTick()
		}
	}()
	log.Printf("[HEARTBEAT] scheduler started (%d tasks loaded)", len(hbStore.tasks))
}

func runHeartbeatTick() {
	now := time.Now()
	hbStore.mu.Lock()
	var remaining []ScheduledTask
	var toRun []ScheduledTask
	for _, t := range hbStore.tasks {
		runAt, err := time.Parse(time.RFC3339, t.RunAt)
		if err != nil {
			log.Printf("[HEARTBEAT] bad run_at for task %q: %v — dropping", t.Label, err)
			continue
		}
		if now.After(runAt) || now.Equal(runAt) {
			toRun = append(toRun, t)
			if t.Repeat != "" {
				nextRun := calcNextRun(runAt, now, t.Repeat)
				if nextRun.After(runAt) {
					t.RunAt = nextRun.Format(time.RFC3339)
					remaining = append(remaining, t)
				}
			}
		} else {
			remaining = append(remaining, t)
		}

	}
	hbStore.tasks = remaining
	hbStore.mu.Unlock()

	for _, t := range toRun {
		go fireHeartbeatTask(t)
	}
	if len(toRun) > 0 {
		persistHeartbeatTasks()
	}
}

func calcNextRun(runAt, now time.Time, repeat string) time.Time {
	var add time.Duration
	repeat = strings.ToLower(strings.TrimSpace(repeat))
	switch repeat {
	case "minutely":
		add = time.Minute
	case "hourly":
		add = time.Hour
	case "daily":
		add = 24 * time.Hour
	case "weekly":
		add = 7 * 24 * time.Hour
	default:
		if strings.HasPrefix(repeat, "every_") {
			var num int
			var unit string
			if _, err := fmt.Sscanf(repeat, "every_%d_%s", &num, &unit); err == nil && num > 0 {
				if strings.HasPrefix(unit, "minute") {
					add = time.Duration(num) * time.Minute
				} else if strings.HasPrefix(unit, "hour") {
					add = time.Duration(num) * time.Hour
				} else if strings.HasPrefix(unit, "day") {
					add = time.Duration(num) * 24 * time.Hour
				}
			}
		}
	}
	if add == 0 {
		return runAt
	}
	nextRun := runAt.Add(add)
	for nextRun.Before(now) || nextRun.Equal(now) {
		nextRun = nextRun.Add(add)
	}
	return nextRun
}

func fireHeartbeatTask(t ScheduledTask) {
	log.Printf("[HEARTBEAT] firing task %q (prompt: %q) → chat=%d", t.Label, t.Prompt, t.TelegramID)
	ownerID := t.OwnerID
	if ownerID == "" {
		ownerID = Cfg.OwnerID
	}

	session := NewAgentSession(GlobalRegistry, Cfg.DefaultModel)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	reply, err := session.RunStream(ctx, ownerID, t.Prompt, nil)
	if err != nil {
		log.Printf("[HEARTBEAT] task %q error: %v", t.Label, err)
		return
	}
	if reply == "" {
		log.Printf("[HEARTBEAT] task %q produced empty reply", t.Label)
		return
	}
	if heartbeatTGClient == nil || t.TelegramID == 0 {
		log.Printf("[HEARTBEAT] task %q: no TG client or TelegramID=0, cannot deliver", t.Label)
		return
	}

	opts := &telegram.SendOptions{ParseMode: telegram.HTML}
	if t.MessageID != 0 {
		opts.ReplyID = int32(t.MessageID)
	}
	if _, err := heartbeatTGClient.SendMessage(t.TelegramID, reply, opts); err != nil {
		log.Printf("[HEARTBEAT] send error for task %q: %v", t.Label, err)
	}
}

func TGSendFile(chatID int64, filePath, caption string) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}
	opts := &telegram.MediaOptions{ForceDocument: true}
	if caption != "" {
		opts.Caption = caption
	}
	if _, err := heartbeatTGClient.SendMedia(chatID, filePath, opts); err != nil {
		return fmt.Sprintf("Error sending file: %v", err)
	}
	return ""
}

func TGSendMessage(chatID int64, text string) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}
	if _, err := heartbeatTGClient.SendMessage(chatID, text, &telegram.SendOptions{ParseMode: telegram.HTML}); err != nil {
		return fmt.Sprintf("Error sending message: %v", err)
	}
	return ""
}

func TGSendPhotoURL(chatID int64, photoURL, caption string) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}
	opts := &telegram.MediaOptions{}
	if caption != "" {
		opts.Caption = caption
	}
	if _, err := heartbeatTGClient.SendMedia(chatID, photoURL, opts); err != nil {
		return fmt.Sprintf("Error sending photo: %v", err)
	}
	return ""
}

func TGSendAlbumURLs(chatID int64, photoURLs []string, caption string) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}
	if len(photoURLs) == 0 {
		return "Error: no URLs provided"
	}

	if len(photoURLs) == 1 {
		return TGSendPhotoURL(chatID, photoURLs[0], caption)
	}
	opts := &telegram.MediaOptions{}
	if caption != "" {
		opts.Caption = caption
	}

	_, err := heartbeatTGClient.SendAlbum(chatID, photoURLs, opts)
	if err != nil {
		return fmt.Sprintf("Error sending album: %v", err)
	}
	return ""
}

func TGSetBotDp(filePathOrURL string) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}

	localPath := filePathOrURL
	if strings.HasPrefix(filePathOrURL, "http://") || strings.HasPrefix(filePathOrURL, "https://") {
		tmp, err := downloadToTemp(filePathOrURL)
		if err != nil {
			return fmt.Sprintf("Error downloading image: %v", err)
		}
		defer func() { _ = os.Remove(tmp) }()
		localPath = tmp
	}

	inputFile, err := heartbeatTGClient.UploadFile(localPath)
	if err != nil {
		return fmt.Sprintf("Error uploading file: %v", err)
	}

	_, err = heartbeatTGClient.PhotosUploadProfilePhoto(&telegram.PhotosUploadProfilePhotoParams{
		File: inputFile,
	})
	if err != nil {
		return fmt.Sprintf("Error setting profile photo: %v", err)
	}
	return ""
}

func downloadToTemp(rawURL string) (string, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	ext := ".jpg"
	ct := resp.Header.Get("Content-Type")
	switch {
	case strings.Contains(ct, "png"):
		ext = ".png"
	case strings.Contains(ct, "gif"):
		ext = ".gif"
	case strings.Contains(ct, "webp"):
		ext = ".webp"
	}

	f, err := os.CreateTemp("", "tgdp-*"+ext)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", err
	}
	return f.Name(), nil
}

func TGDownloadMedia(chatID int64, messageID int32, savePath string) (string, error) {
	if heartbeatTGClient == nil {
		return "", fmt.Errorf("Telegram client not ready")
	}
	msgs, err := heartbeatTGClient.GetMessages(chatID, &telegram.SearchOption{
		IDs: []int32{messageID},
	})
	if err != nil {
		return "", fmt.Errorf("GetMessages: %w", err)
	}
	if len(msgs) == 0 {
		return "", fmt.Errorf("message %d not found in chat %d", messageID, chatID)
	}
	msg := msgs[0]
	opts := &telegram.DownloadOptions{}
	if savePath != "" {
		opts.FileName = savePath
	}
	path, err := heartbeatTGClient.DownloadMedia(msg.Media(), opts)
	if err != nil {
		return "", fmt.Errorf("DownloadMedia: %w", err)
	}
	return path, nil
}

func TGGetChatInfo(peerStr string) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}

	stripped := strings.TrimPrefix(peerStr, "@")
	var peer any
	var resolveErr error

	isNumeric := true
	for i, c := range peerStr {
		if c == '-' && i == 0 {
			continue
		}
		if c < '0' || c > '9' {
			isNumeric = false
			break
		}
	}

	if isNumeric {
		var chatID int64
		if _, err := fmt.Sscanf(peerStr, "%d", &chatID); err != nil {
			return fmt.Sprintf("Error: invalid peer ID %q", peerStr)
		}
		peer, resolveErr = heartbeatTGClient.GetPeer(chatID)
		if resolveErr != nil {
			return fmt.Sprintf("Error resolving peer %d: %v", chatID, resolveErr)
		}
	} else {
		peer, resolveErr = heartbeatTGClient.ResolveUsername(stripped)
		if resolveErr != nil {
			return fmt.Sprintf("Error resolving @%s: %v", stripped, resolveErr)
		}
	}

	return formatTGPeer(peer, peerStr)
}

func formatTGPeer(peer any, label string) string {
	switch p := peer.(type) {
	case *telegram.UserObj:
		name := strings.TrimSpace(p.FirstName + " " + p.LastName)
		username := ""
		if p.Username != "" {
			username = " (@" + p.Username + ")"
		}
		var flags []string
		if p.Bot {
			flags = append(flags, "bot")
		}
		if p.Verified {
			flags = append(flags, "verified")
		}
		if p.Premium {
			flags = append(flags, "premium")
		}
		extra := ""
		if len(flags) > 0 {
			extra = " [" + strings.Join(flags, ", ") + "]"
		}
		return fmt.Sprintf("User: %s%s%s\nID: %d", name, username, extra, p.ID)
	case *telegram.ChatObj:
		return fmt.Sprintf(
			"Group: %s\nID: %d\nMembers: %d\nCreated: %s",
			p.Title, p.ID, p.ParticipantsCount,
			time.Unix(int64(p.Date), 0).Format("02 Jan 2006"),
		)
	case *telegram.Channel:
		members := ""
		if p.ParticipantsCount > 0 {
			members = fmt.Sprintf("\nMembers: %d", p.ParticipantsCount)
		}
		username := ""
		if p.Username != "" {
			username = " (@" + p.Username + ")"
		}
		kind := "Channel"
		if p.Megagroup {
			kind = "Supergroup"
		}
		return fmt.Sprintf("%s: %s%s\nID: %d%s", kind, p.Title, username, p.ID, members)
	default:
		return fmt.Sprintf("Peer %q: unknown type %T", label, peer)
	}
}

func TGForwardMsg(fromChatID int64, msgID int32, toChatID int64) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}
	_, err := heartbeatTGClient.Forward(toChatID, fromChatID, []int32{msgID})
	if err != nil {
		return fmt.Sprintf("Error forwarding: %v", err)
	}
	return fmt.Sprintf("Forwarded message %d from %d to %d", msgID, fromChatID, toChatID)
}

func TGDeleteMsg(chatID int64, msgIDs []int32) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}
	_, err := heartbeatTGClient.DeleteMessages(chatID, msgIDs)
	if err != nil {
		return fmt.Sprintf("Error deleting: %v", err)
	}
	return fmt.Sprintf("Deleted %d message(s)", len(msgIDs))
}

func TGPinMsg(chatID int64, msgID int32, silent bool) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}
	_, err := heartbeatTGClient.PinMessage(chatID, msgID, &telegram.PinOptions{Silent: silent})
	if err != nil {
		return fmt.Sprintf("Error pinning: %v", err)
	}
	return fmt.Sprintf("Pinned message %d in chat %d", msgID, chatID)
}

func TGReact(chatID int64, msgID int32, emoji string) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}
	if err := heartbeatTGClient.SendReaction(chatID, msgID, emoji); err != nil {
		return fmt.Sprintf("Error sending reaction: %v", err)
	}
	return fmt.Sprintf("Reacted with %s to message %d", emoji, msgID)
}

func TGGetReply(chatID int64, msgID int32) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}
	msgs, err := heartbeatTGClient.GetMessages(chatID, &telegram.SearchOption{
		IDs: []int32{msgID},
	})
	if err != nil {
		return fmt.Sprintf("Error fetching message: %v", err)
	}
	if len(msgs) == 0 {
		return fmt.Sprintf("Message %d not found in chat %d", msgID, chatID)
	}
	msg := msgs[0]
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Message ID: %d\n", msg.ID))

	if msg.Sender != nil {
		name := strings.TrimSpace(msg.Sender.FirstName + " " + msg.Sender.LastName)
		if msg.Sender.Username != "" {
			name += " (@" + msg.Sender.Username + ")"
		}
		sb.WriteString(fmt.Sprintf("From: %s\n", name))
	} else if senderChat := msg.GetSenderChat(); senderChat != nil {
		sb.WriteString(fmt.Sprintf("From channel: %s\n", senderChat.Title))
	}
	if msg.Text() != "" {
		sb.WriteString(fmt.Sprintf("Text: %s\n", msg.Text()))
	}
	if msg.IsMedia() {
		sb.WriteString(fmt.Sprintf("Has media: true (type: %T)\n", msg.Media()))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func ListHeartbeatTasks() string {
	hbStore.mu.Lock()
	defer hbStore.mu.Unlock()
	if len(hbStore.tasks) == 0 {
		return "No scheduled tasks."
	}
	var sb strings.Builder
	for _, t := range hbStore.tasks {
		repeat := t.Repeat
		if repeat == "" {
			repeat = "once"
		}
		fmt.Fprintf(&sb, "• <b>%s</b> — %s\n  next: %s | repeat: %s\n", t.Label, t.Prompt, t.RunAt, repeat)
	}
	return strings.TrimRight(sb.String(), "\n")
}
