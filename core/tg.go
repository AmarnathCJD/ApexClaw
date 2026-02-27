package core

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
)

// TGSendFile sends a file to a Telegram chat (accepts peer string: ID, username, etc.)
func TGSendFile(peer string, filePath, caption string) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}

	resolvedPeer, err := TGResolvePeer(peer)
	if err != nil {
		return fmt.Sprintf("Error resolving peer: %v", err)
	}

	opts := &telegram.MediaOptions{ForceDocument: true}
	if caption != "" {
		opts.Caption = caption
	}
	if _, err := heartbeatTGClient.SendMedia(resolvedPeer, filePath, opts); err != nil {
		return fmt.Sprintf("Error sending file: %v", err)
	}
	return ""
}

// TGSendMessage sends a text message to a Telegram chat
func TGSendMessage(peer string, text string) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}

	resolvedPeer, err := TGResolvePeer(peer)
	if err != nil {
		return fmt.Sprintf("Error resolving peer: %v", err)
	}

	if _, err := heartbeatTGClient.SendMessage(resolvedPeer, text, &telegram.SendOptions{ParseMode: telegram.HTML}); err != nil {
		return fmt.Sprintf("Error sending message: %v", err)
	}
	return ""
}

// TGSendPhotoURL sends a photo from URL
func TGSendPhotoURL(peer string, photoURL, caption string) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}

	resolvedPeer, err := TGResolvePeer(peer)
	if err != nil {
		return fmt.Sprintf("Error resolving peer: %v", err)
	}

	opts := &telegram.MediaOptions{}
	if caption != "" {
		opts.Caption = caption
	}
	if _, err := heartbeatTGClient.SendMedia(resolvedPeer, photoURL, opts); err != nil {
		return fmt.Sprintf("Error sending photo: %v", err)
	}
	return ""
}

// TGSendAlbumURLs sends multiple photos as an album
func TGSendAlbumURLs(peer string, photoURLs []string, caption string) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}
	if len(photoURLs) == 0 {
		return "Error: no URLs provided"
	}

	resolvedPeer, err := TGResolvePeer(peer)
	if err != nil {
		return fmt.Sprintf("Error resolving peer: %v", err)
	}

	if len(photoURLs) == 1 {
		opts := &telegram.MediaOptions{}
		if caption != "" {
			opts.Caption = caption
		}
		if _, err := heartbeatTGClient.SendMedia(resolvedPeer, photoURLs[0], opts); err != nil {
			return fmt.Sprintf("Error sending photo: %v", err)
		}
		return ""
	}

	opts := &telegram.MediaOptions{}
	if caption != "" {
		opts.Caption = caption
	}

	_, err = heartbeatTGClient.SendAlbum(resolvedPeer, photoURLs, opts)
	if err != nil {
		return fmt.Sprintf("Error sending album: %v", err)
	}
	return ""
}

// TGSetBotDp sets the bot's profile picture
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

// downloadToTemp downloads a file from URL to temp
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

// TGDownloadMedia downloads media
func TGDownloadMedia(peer string, messageID int32, savePath string) (string, error) {
	if heartbeatTGClient == nil {
		return "", fmt.Errorf("Telegram client not ready")
	}

	chatID, err := heartbeatTGClient.ResolvePeer(peer)
	if err != nil {
		return "", fmt.Errorf("error resolving peer: %w", err)
	}

	msgs, err := heartbeatTGClient.GetMessages(chatID, &telegram.SearchOption{
		IDs: []int32{messageID},
	})
	if err != nil {
		return "", fmt.Errorf("GetMessages: %w", err)
	}
	if len(msgs) == 0 {
		return "", fmt.Errorf("message not found")
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

// TGGetChatInfo gets chat info
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
			return fmt.Sprintf("Error resolving peer: %v", resolveErr)
		}
	} else {
		peer, resolveErr = heartbeatTGClient.ResolveUsername(stripped)
		if resolveErr != nil {
			return fmt.Sprintf("Error resolving @%s: %v", stripped, resolveErr)
		}
	}

	return formatTGPeer(peer, peerStr)
}

// TGResolvePeer resolves a peer string
func TGResolvePeer(peerStr string) (any, error) {
	if heartbeatTGClient == nil {
		return nil, fmt.Errorf("Telegram client not ready")
	}

	return heartbeatTGClient.ResolvePeer(peerStr)
}

// formatTGPeer formats peer information
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
		return fmt.Sprintf("Group: %s\nID: %d\nMembers: %d", p.Title, p.ID, p.ParticipantsCount)
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
		return fmt.Sprintf("Peer %q: unknown type", label)
	}
}

// TGForwardMsg forwards a message from one chat to another
func TGForwardMsg(fromPeer string, msgID int32, toPeer string) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}

	fromID, err := heartbeatTGClient.ResolvePeer(fromPeer)
	if err != nil {
		return fmt.Sprintf("Error resolving source: %v", err)
	}

	toID, err := heartbeatTGClient.ResolvePeer(toPeer)
	if err != nil {
		return fmt.Sprintf("Error resolving destination: %v", err)
	}

	_, err = heartbeatTGClient.Forward(toID, fromID, []int32{msgID})
	if err != nil {
		return fmt.Sprintf("Error forwarding: %v", err)
	}
	return fmt.Sprintf("Forwarded message %d", msgID)
}

// TGDeleteMsg deletes one or more messages from a chat
func TGDeleteMsg(peer string, msgIDs []int32) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}

	chatID, err := heartbeatTGClient.ResolvePeer(peer)
	if err != nil {
		return fmt.Sprintf("Error resolving peer: %v", err)
	}

	_, err = heartbeatTGClient.DeleteMessages(chatID, msgIDs)
	if err != nil {
		return fmt.Sprintf("Error deleting: %v", err)
	}
	return fmt.Sprintf("Deleted %d message(s)", len(msgIDs))
}

// TGPinMsg pins a message in a chat
func TGPinMsg(peer string, msgID int32, silent bool) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}

	chatID, err := heartbeatTGClient.ResolvePeer(peer)
	if err != nil {
		return fmt.Sprintf("Error resolving peer: %v", err)
	}

	_, err = heartbeatTGClient.PinMessage(chatID, msgID, &telegram.PinOptions{Silent: silent})
	if err != nil {
		return fmt.Sprintf("Error pinning: %v", err)
	}
	return fmt.Sprintf("Pinned message %d", msgID)
}

// TGUnpinMsg unpins a message from a chat
func TGUnpinMsg(peer string, msgID int32) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}

	chatID, err := heartbeatTGClient.ResolvePeer(peer)
	if err != nil {
		return fmt.Sprintf("Error resolving peer: %v", err)
	}

	_, err = heartbeatTGClient.UnpinMessage(chatID, msgID)
	if err != nil {
		return fmt.Sprintf("Error unpinning: %v", err)
	}
	return fmt.Sprintf("Unpinned message %d", msgID)
}

// TGReact adds an emoji reaction to a message
func TGReact(peer string, msgID int32, emoji string) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}

	chatID, err := heartbeatTGClient.ResolvePeer(peer)
	if err != nil {
		return fmt.Sprintf("Error resolving peer: %v", err)
	}

	if err := heartbeatTGClient.SendReaction(chatID, msgID, emoji); err != nil {
		return fmt.Sprintf("Error sending reaction: %v", err)
	}
	return fmt.Sprintf("Reacted with %s", emoji)
}

// TGGetReply fetches the full content of a message
func TGGetReply(peer string, msgID int32) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}

	chatID, err := heartbeatTGClient.ResolvePeer(peer)
	if err != nil {
		return fmt.Sprintf("Error resolving peer: %v", err)
	}

	msgs, err := heartbeatTGClient.GetMessages(chatID, &telegram.SearchOption{
		IDs: []int32{msgID},
	})
	if err != nil {
		return fmt.Sprintf("Error fetching message: %v", err)
	}
	if len(msgs) == 0 {
		return fmt.Sprintf("Message %d not found", msgID)
	}

	msg := msgs[0]
	var sb strings.Builder
	fmt.Fprintf(&sb, "Message ID: %d\n", msg.ID)

	if msg.Sender != nil {
		name := strings.TrimSpace(msg.Sender.FirstName + " " + msg.Sender.LastName)
		if msg.Sender.Username != "" {
			name += " (@" + msg.Sender.Username + ")"
		}
		fmt.Fprintf(&sb, "From: %s\n", name)
	} else if senderChat := msg.GetSenderChat(); senderChat != nil {
		fmt.Fprintf(&sb, "From channel: %s\n", senderChat.Title)
	}
	if msg.Text() != "" {
		fmt.Fprintf(&sb, "Text: %s\n", msg.Text())
	}
	if msg.IsMedia() {
		fmt.Fprintf(&sb, "Has media: true\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// TGGetMembers lists members of a group or channel
func TGGetMembers(peer string, limit int) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}

	chatID, err := heartbeatTGClient.ResolvePeer(peer)
	if err != nil {
		return fmt.Sprintf("Error resolving peer: %v", err)
	}

	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	members, _, err := heartbeatTGClient.GetChatMembers(chatID, &telegram.ParticipantOptions{Limit: int32(limit)})
	if err != nil {
		return fmt.Sprintf("Error fetching members: %v", err)
	}

	if len(members) == 0 {
		return "No members found"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Members (%d):\n\n", len(members))
	for i, member := range members {
		role := "member"
		switch member.Status {
		case telegram.Admin:
			role = "admin"
		case telegram.Creator:
			role = "creator"
		}
		username := ""
		if member.User.Username != "" {
			username = " (@" + member.User.Username + ")"
		}
		name := strings.TrimSpace(member.User.FirstName + " " + member.User.LastName)
		fmt.Fprintf(&sb, "%d. %s%s [%s]\n", i+1, name, username, role)
	}

	return strings.TrimRight(sb.String(), "\n")
}

// TGBroadcast sends the same message to multiple chats
func TGBroadcast(peers []string, text string) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}
	if len(peers) == 0 {
		return "Error: no chat IDs provided"
	}

	var successful, failed int
	for _, peer := range peers {
		chatID, err := heartbeatTGClient.ResolvePeer(peer)
		if err != nil {
			log.Printf("[TG] broadcast error for %q: %v", peer, err)
			failed++
			continue
		}
		if _, err := heartbeatTGClient.SendMessage(chatID, text, &telegram.SendOptions{ParseMode: telegram.HTML}); err != nil {
			log.Printf("[TG] broadcast error to %d: %v", chatID, err)
			failed++
		} else {
			successful++
		}
	}

	return fmt.Sprintf("Broadcast sent: %d successful, %d failed", successful, failed)
}

// TGGetMessage fetches a single message by ID
func TGGetMessage(peer string, msgID int32) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}

	chatID, err := heartbeatTGClient.ResolvePeer(peer)
	if err != nil {
		return fmt.Sprintf("Error resolving peer: %v", err)
	}

	msgs, err := heartbeatTGClient.GetMessages(chatID, &telegram.SearchOption{
		IDs: []int32{msgID},
	})
	if err != nil {
		return fmt.Sprintf("Error fetching message: %v", err)
	}
	if len(msgs) == 0 {
		return fmt.Sprintf("Message %d not found", msgID)
	}

	msg := msgs[0]
	var sb strings.Builder
	fmt.Fprintf(&sb, "Message ID: %d\n", msg.ID)
	if msg.Sender != nil {
		name := strings.TrimSpace(msg.Sender.FirstName + " " + msg.Sender.LastName)
		if msg.Sender.Username != "" {
			name += " (@" + msg.Sender.Username + ")"
		}
		fmt.Fprintf(&sb, "From: %s (ID: %d)\n", name, msg.SenderID())
	}
	if msg.Text() != "" {
		fmt.Fprintf(&sb, "Text:\n%s\n", msg.Text())
	}
	if msg.IsMedia() {
		fmt.Fprintf(&sb, "Media: true\n")
	}
	fmt.Fprintf(&sb, "Date: %s\n", time.Unix(int64(msg.Date()), 0).Format("02 Jan 2006 15:04:05 MST"))

	return strings.TrimRight(sb.String(), "\n")
}

// TGEditMessage edits a previously sent message
func TGEditMessage(peer string, msgID int32, newText string) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}

	chatID, err := heartbeatTGClient.ResolvePeer(peer)
	if err != nil {
		return fmt.Sprintf("Error resolving peer: %v", err)
	}

	_, err = heartbeatTGClient.EditMessage(chatID, msgID, newText, &telegram.SendOptions{ParseMode: telegram.HTML})
	if err != nil {
		return fmt.Sprintf("Error editing message: %v", err)
	}
	return fmt.Sprintf("Edited message %d", msgID)
}

// TGSendMessageWithButtons sends a message with inline keyboard buttons
func TGSendMessageWithButtons(peer string, text string, kb *telegram.ReplyInlineMarkup) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not initialized"
	}

	chatID, err := heartbeatTGClient.ResolvePeer(peer)
	if err != nil {
		return fmt.Sprintf("Error resolving peer: %v", err)
	}

	_, err = heartbeatTGClient.SendMessage(chatID, text, &telegram.SendOptions{
		ReplyMarkup: kb,
	})
	if err != nil {
		return fmt.Sprintf("Error sending message: %v", err)
	}

	return "Message sent"
}

// TGCreateInvite creates an invite link for a chat
func TGCreateInvite(peer string, expireDate int32, memberLimit int32) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}

	inv, err := heartbeatTGClient.ExportInvite(peer)
	if err != nil {
		return fmt.Sprintf("Error creating invite: %v", err)
	}
	return inv.Link
}

// TGGetProfilePhotos gets profile photos of a user
func TGGetProfilePhotos(peer string, limit int) string {
	if heartbeatTGClient == nil {
		return "Error: Telegram client not ready"
	}

	userID, err := heartbeatTGClient.ResolvePeer(peer)
	if err != nil {
		return fmt.Sprintf("Error resolving peer: %v", err)
	}

	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	// Get profile photos
	opts := &telegram.PhotosOptions{Limit: int32(limit)}
	photos, err := heartbeatTGClient.GetProfilePhotos(userID, opts)
	if err != nil {
		return fmt.Sprintf("Error fetching profile photos: %v", err)
	}

	if len(photos) == 0 {
		return "No profile photos found"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Profile Photos (%d):\n\n", len(photos))
	for i := range photos {
		fmt.Fprintf(&sb, "%d. Photo fetched\n", i+1)
	}

	return strings.TrimRight(sb.String(), "\n")
}
