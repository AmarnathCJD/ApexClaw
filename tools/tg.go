package tools

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/amarnathcjd/gogram/telegram"
)

// === Function Pointers (wired in core/register.go) ===

var SendTGFileFn func(peer string, filePath, caption string, forceDocument bool) string
var SendTGMsgFn func(peer string, text string) string
var SendTGPhotoFn func(peer string, pathOrFileID, caption string) string
var SendTGPhotoURLFn func(peer string, photoURL, caption string) string
var SendTGAlbumFn func(peer string, paths []string, caption string) string
var SetBotDpFn func(filePathOrURL string) string
var TGDownloadMediaFn func(peer string, messageID int32, savePath string) (string, error)
var TGGetChatInfoFn func(peer string) string
var TGResolvePeerFn func(peer string) (any, error)
var TGForwardMsgFn func(fromPeer string, msgID int32, toPeer string) string
var TGDeleteMsgFn func(peer string, msgIDs []int32) string
var TGPinMsgFn func(peer string, msgID int32, silent bool) string
var TGUnpinMsgFn func(peer string, msgID int32) string
var TGReactFn func(peer string, msgID int32, emoji string) string
var TGGetMembersFn func(peer string, limit int) string
var TGBroadcastFn func(peers []string, text string) string
var TGGetMessageFn func(peer string, msgID int32) string
var TGEditMessageFn func(peer string, msgID int32, newText string) string
var SendTGMessageWithButtonsFn func(peer string, text string, kb *telegram.ReplyInlineMarkup) string
var TGCreateInviteFn func(peer string, expireDate int32, memberLimit int32) string
var TGGetProfilePhotosFn func(peer string, limit int) string
var TGBanUserFn func(peer string, userID string, deleteHistory bool, untilDate int32) string
var TGMuteUserFn func(peer string, userID string, untilDate int32) string
var TGKickUserFn func(peer string, userID string) string
var TGPromoteAdminFn func(peer string, userID string, rights map[string]bool, title string) string
var TGDemoteAdminFn func(peer string, userID string) string
var TGSendLocationFn func(peer string, lat, long float64) string
var TGGetFileFn func(peer string, msgID int32, savePath string) string

// === Context Helpers ===

func resolveContextPeer(peerStr string, userID string) string {
	peerStr = strings.TrimSpace(peerStr)
	lower := strings.ToLower(peerStr)

	if GetTelegramContextFn == nil {
		return peerStr
	}
	ctx := GetTelegramContextFn(userID)
	if ctx == nil {
		return peerStr
	}

	if lower == "" || lower == "current" || lower == "here" || lower == "this" || lower == "chat" || lower == "group" {
		if v, ok := ctx["telegram_id"]; ok {
			return fmt.Sprintf("%d", v.(int64))
		}
	}

	if lower == "me" || lower == "self" || lower == "myself" || lower == "sender" {
		if v, ok := ctx["sender_id"]; ok {
			return v.(string)
		}
	}

	if lower == "them" || lower == "him" || lower == "her" || lower == "reply" || lower == "replied" || lower == "target" {
		if v, ok := ctx["reply_sender_id"]; ok {
			return v.(string)
		}
	}

	return peerStr
}

func resolveContextMessageID(idStr string, userID string) int32 {
	lower := strings.ToLower(strings.TrimSpace(idStr))
	if lower == "" || lower == "reply" || lower == "target" || lower == "this" {
		if GetTelegramContextFn != nil {
			ctx := GetTelegramContextFn(userID)
			if ctx != nil {
				if v, ok := ctx["reply_id"]; ok {
					return int32(v.(int64))
				}
				if v, ok := ctx["msg_id"]; ok {
					return int32(v.(int64))
				}
			}
		}
		return 0
	}
	var id int32
	fmt.Sscanf(idStr, "%d", &id)
	return id
}

func currentChatID(userID string) string {
	return resolveContextPeer("", userID)
}

// isMediaFile returns true for image/video extensions that should be sent as media (not document)
var mediaExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
	".mp4": true, ".avi": true, ".mkv": true, ".mov": true, ".webm": true,
}

func isMediaFile(path string) bool {
	idx := strings.LastIndex(path, ".")
	if idx < 0 {
		return false
	}
	return mediaExts[strings.ToLower(path[idx:])]
}

// === Button types ===

type ButtonSpec struct {
	Text  string `json:"text"`
	Type  string `json:"type"`
	Data  string `json:"data"`
	URL   string `json:"url"`
	Style string `json:"style"`
}

type ButtonRowSpec struct {
	Buttons []ButtonSpec `json:"buttons"`
}

type ButtonsSpec struct {
	Rows []ButtonRowSpec `json:"rows"`
}

// === Tool Definitions ===

var TGSendMessage = &ToolDef{
	Name:        "tg_send_message",
	Description: "Send a text message to a Telegram chat. Omit target to send to current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "text", Description: "Message text (HTML formatting allowed)", Required: true},
		{Name: "target", Description: "Chat ID, @username, or 'me'. Omit for current chat.", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		text := strings.TrimSpace(args["text"])
		if text == "" {
			return "Error: text is required"
		}
		target := resolveContextPeer(args["target"], userID)
		if target == "" {
			return "Error: no current chat context"
		}
		if SendTGMsgFn == nil {
			return "Error: Telegram not initialized"
		}
		if r := SendTGMsgFn(target, text); r != "" {
			return r
		}
		return "Sent"
	},
}

var TGSendFile = &ToolDef{
	Name: "tg_send_file",
	Description: "Send a local file to a Telegram chat. Images (jpg/png/gif/webp) and videos (mp4/avi/mkv/mov/webm) " +
		"are sent as media by default. All other files are sent as documents. " +
		"Set doc=true to force document mode regardless of file type. Omit target for current chat.",
	Secure: true,
	Args: []ToolArg{
		{Name: "path", Description: "Absolute path of the file", Required: true},
		{Name: "caption", Description: "Optional caption", Required: false},
		{Name: "target", Description: "Chat ID, @username, or 'me'. Omit for current chat.", Required: false},
		{Name: "doc", Description: "'true' to force send as document. Default: auto by extension.", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		path := strings.TrimSpace(args["path"])
		if path == "" {
			return "Error: path is required"
		}
		target := resolveContextPeer(args["target"], userID)
		if target == "" {
			return "Error: no current chat context"
		}
		if SendTGFileFn == nil {
			return "Error: Telegram not initialized"
		}
		docStr := strings.ToLower(strings.TrimSpace(args["doc"]))
		var forceDoc bool
		switch docStr {
		case "true":
			forceDoc = true
		case "false":
			forceDoc = false
		default:
			forceDoc = !isMediaFile(path)
		}
		if r := SendTGFileFn(target, path, strings.TrimSpace(args["caption"]), forceDoc); r != "" {
			return r
		}
		return fmt.Sprintf("Sent: %s", path)
	},
}

var TGSendPhoto = &ToolDef{
	Name:        "tg_send_photo",
	Description: "Send a photo from local path or Telegram FileID. Omit target for current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "path", Description: "Local path or Telegram FileID", Required: true},
		{Name: "caption", Description: "Optional caption", Required: false},
		{Name: "target", Description: "Chat ID, @username, or 'me'. Omit for current chat.", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		path := strings.TrimSpace(args["path"])
		if path == "" {
			return "Error: path is required"
		}
		target := resolveContextPeer(args["target"], userID)
		if target == "" {
			return "Error: no current chat context"
		}
		if SendTGPhotoFn == nil {
			return "Error: Telegram not initialized"
		}
		if r := SendTGPhotoFn(target, path, strings.TrimSpace(args["caption"])); r != "" {
			return r
		}
		return "Sent photo"
	},
}

var TGSendAlbum = &ToolDef{
	Name:        "tg_send_album",
	Description: "Send multiple photos/videos as an album (media group). Paths comma-separated. Omit target for current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "paths", Description: "Comma-separated list of local file paths or URLs", Required: true},
		{Name: "caption", Description: "Optional caption for the album", Required: false},
		{Name: "target", Description: "Chat ID, @username, or 'me'. Omit for current chat.", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		pathsStr := strings.TrimSpace(args["paths"])
		if pathsStr == "" {
			return "Error: paths is required"
		}
		target := resolveContextPeer(args["target"], userID)
		if target == "" {
			return "Error: no current chat context"
		}
		if SendTGAlbumFn == nil {
			return "Error: Telegram not initialized"
		}
		var paths []string
		for p := range strings.SplitSeq(pathsStr, ",") {
			if p = strings.TrimSpace(p); p != "" {
				paths = append(paths, p)
			}
		}
		if len(paths) == 0 {
			return "Error: no valid paths provided"
		}
		if r := SendTGAlbumFn(target, paths, strings.TrimSpace(args["caption"])); r != "" {
			return r
		}
		return fmt.Sprintf("Sent album (%d files)", len(paths))
	},
}

var TGSendLocation = &ToolDef{
	Name:        "tg_send_location",
	Description: "Send a location pin to a Telegram chat. Omit target for current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "lat", Description: "Latitude (e.g. 37.7749)", Required: true},
		{Name: "long", Description: "Longitude (e.g. -122.4194)", Required: true},
		{Name: "target", Description: "Chat ID, @username, or 'me'. Omit for current chat.", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		target := resolveContextPeer(args["target"], userID)
		if target == "" {
			return "Error: no current chat context"
		}
		if TGSendLocationFn == nil {
			return "Error: Telegram not initialized"
		}
		var lat, long float64
		if _, err := fmt.Sscanf(args["lat"], "%f", &lat); err != nil {
			return "Error: invalid lat"
		}
		if _, err := fmt.Sscanf(args["long"], "%f", &long); err != nil {
			return "Error: invalid long"
		}
		return TGSendLocationFn(target, lat, long)
	},
}

var TGSendMessageWithButtons = &ToolDef{
	Name: "tg_send_message_buttons",
	Description: "Send a Telegram message with inline buttons. buttons must be base64-encoded JSON. " +
		"Format: {\"rows\":[{\"buttons\":[{\"text\":\"Yes\",\"type\":\"data\",\"data\":\"yes\",\"style\":\"success\"}]}]}. " +
		"Styles: success(green), danger(red), primary(blue). Type: data(callback) or url(link).",
	Secure: true,
	Args: []ToolArg{
		{Name: "text", Description: "Message text", Required: true},
		{Name: "buttons", Description: "Buttons as BASE64-ENCODED JSON", Required: false},
		{Name: "target", Description: "Chat ID, @username, or 'me'. Omit for current chat.", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		text := strings.TrimSpace(args["text"])
		if text == "" {
			return "Error: text is required"
		}
		target := resolveContextPeer(args["target"], userID)
		if target == "" {
			return "Error: no current chat context"
		}
		if SendTGMessageWithButtonsFn == nil {
			return "Error: Telegram not initialized"
		}
		var kb *telegram.ReplyInlineMarkup
		if b64 := strings.TrimSpace(args["buttons"]); b64 != "" {
			kb = parseButtons(b64)
			if kb == nil {
				return "Error: failed to parse buttons"
			}
		}
		return SendTGMessageWithButtonsFn(target, text, kb)
	},
}

var SetBotDp = &ToolDef{
	Name:        "set_bot_dp",
	Description: "Set the bot profile picture. If reply has a photo, auto-uses it. Otherwise provide file path or URL.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "image", Description: "Local file path or image URL. Omit to use replied-to photo.", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		image := strings.TrimSpace(args["image"])
		if image == "" && GetTelegramContextFn != nil && TGDownloadMediaFn != nil {
			ctx := GetTelegramContextFn(userID)
			if ctx != nil {
				if repliedID, ok := ctx["replied_id"]; ok {
					if chatID, ok2 := ctx["telegram_id"]; ok2 {
						msgID := int32(repliedID.(int64))
						peer := fmt.Sprintf("%d", chatID.(int64))
						if local, err := TGDownloadMediaFn(peer, msgID, ""); err == nil {
							image = local
						}
					}
				}
			}
		}
		if image == "" {
			return "Error: no image provided and no replied-to message with media"
		}
		if SetBotDpFn == nil {
			return "Error: Telegram not initialized"
		}
		if r := SetBotDpFn(image); r != "" {
			return r
		}
		return "Profile photo updated"
	},
}

var TGDownload = &ToolDef{
	Name:        "tg_download",
	Description: "Download media from a Telegram message. Omit chat_id for current chat. Omit message_id to use replied message.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Description: "Chat ID or @username. Omit for current chat.", Required: false},
		{Name: "message_id", Description: "Message ID with media. Omit for replied message.", Required: false},
		{Name: "save_as", Description: "Optional local file path to save to", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		chat := resolveContextPeer(args["chat_id"], userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		msgID := resolveContextMessageID(args["message_id"], userID)
		if msgID == 0 {
			return "Error: message_id required and could not be inferred"
		}
		if TGDownloadMediaFn == nil {
			return "Error: Telegram not initialized"
		}
		path, err := TGDownloadMediaFn(chat, msgID, strings.TrimSpace(args["save_as"]))
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return fmt.Sprintf("Downloaded: %s", path)
	},
}

var TGGetFile = &ToolDef{
	Name:        "tg_get_file",
	Description: "Download a file from a specific message and return the local path. Use this to access files from replied messages before processing. Omit chat_id for current chat, omit message_id for replied message.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Description: "Chat ID or @username. Omit for current chat.", Required: false},
		{Name: "message_id", Description: "Message ID with the file. Omit for replied message.", Required: false},
		{Name: "save_as", Description: "Optional save path", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		chat := resolveContextPeer(args["chat_id"], userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		msgID := resolveContextMessageID(args["message_id"], userID)
		if msgID == 0 {
			return "Error: message_id required and could not be inferred"
		}
		if TGGetFileFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGGetFileFn(chat, msgID, strings.TrimSpace(args["save_as"]))
	},
}

var TGForwardMsg = &ToolDef{
	Name:        "tg_forward",
	Description: "Forward a message from one chat to another. Omit from/to for current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "from_chat_id", Description: "Source chat ID or @username. Omit for current chat.", Required: false},
		{Name: "message_id", Description: "Message ID to forward", Required: true},
		{Name: "to_chat_id", Description: "Destination chat ID or @username. Omit for current chat.", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		msgStr := strings.TrimSpace(args["message_id"])
		if msgStr == "" {
			return "Error: message_id is required"
		}
		from := resolveContextPeer(args["from_chat_id"], userID)
		to := resolveContextPeer(args["to_chat_id"], userID)
		if from == "" || to == "" {
			return "Error: from/to chat could not be inferred"
		}
		var msgID int32
		if _, err := fmt.Sscanf(msgStr, "%d", &msgID); err != nil {
			return "Error: message_id must be numeric"
		}
		if TGForwardMsgFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGForwardMsgFn(from, msgID, to)
	},
}

var TGDeleteMsg = &ToolDef{
	Name:        "tg_delete_msg",
	Description: "Delete messages from a chat. Omit chat_id for current chat. Omit message_ids to delete replied-to message.",
	Secure:      false,
	Args: []ToolArg{
		{Name: "chat_id", Description: "Chat ID or @username. Omit for current chat.", Required: false},
		{Name: "message_ids", Description: "Comma-separated message IDs. Omit to delete replied-to message.", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		chat := resolveContextPeer(args["chat_id"], userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		msgStr := strings.TrimSpace(args["message_ids"])
		var msgIDs []int32
		if msgStr == "" {
			id := resolveContextMessageID("", userID)
			if id == 0 {
				return "Error: no message to delete"
			}
			msgIDs = append(msgIDs, id)
		} else {
			for _, part := range strings.Split(msgStr, ",") {
				part = strings.TrimSpace(part)
				var id int32
				if _, err := fmt.Sscanf(part, "%d", &id); err != nil {
					return fmt.Sprintf("Error: invalid ID %q", part)
				}
				msgIDs = append(msgIDs, id)
			}
		}
		if TGDeleteMsgFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGDeleteMsgFn(chat, msgIDs)
	},
}

var TGPinMsg = &ToolDef{
	Name:        "tg_pin_msg",
	Description: "Pin a message in a chat. Omit chat_id for current chat. Omit message_id for replied-to message.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Description: "Chat ID or @username. Omit for current chat.", Required: false},
		{Name: "message_id", Description: "Message ID to pin. Omit for replied message.", Required: false},
		{Name: "silent", Description: "Pin silently (true/false, default false)", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		chat := resolveContextPeer(args["chat_id"], userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		msgID := resolveContextMessageID(args["message_id"], userID)
		if msgID == 0 {
			return "Error: message_id could not be inferred"
		}
		if TGPinMsgFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGPinMsgFn(chat, msgID, strings.EqualFold(args["silent"], "true"))
	},
}

var TGUnpinMsg = &ToolDef{
	Name:        "tg_unpin_msg",
	Description: "Unpin a message from a chat. Omit chat_id for current chat. Omit message_id for replied-to message.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Description: "Chat ID or @username. Omit for current chat.", Required: false},
		{Name: "message_id", Description: "Message ID to unpin. Omit for replied message.", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		chat := resolveContextPeer(args["chat_id"], userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		msgID := resolveContextMessageID(args["message_id"], userID)
		if msgID == 0 {
			return "Error: message_id could not be inferred"
		}
		if TGUnpinMsgFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGUnpinMsgFn(chat, msgID)
	},
}

var TGGetChatInfo = &ToolDef{
	Name:        "tg_get_chat_info",
	Description: "Get info about a Telegram user, group, or channel. Omit peer to use current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "peer", Description: "Chat/user ID (numeric) or @username. Omit for current chat.", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		peer := resolveContextPeer(args["peer"], userID)
		if peer == "" {
			return "Error: peer required"
		}
		if TGGetChatInfoFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGGetChatInfoFn(peer)
	},
}

var TGReact = &ToolDef{
	Name:        "tg_react",
	Description: "React to a message with an emoji. Omit chat_id/message_id to use context.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "emoji", Description: "Emoji reaction (e.g. '👍', '❤️', '🔥')", Required: true},
		{Name: "chat_id", Description: "Chat ID or @username. Omit for current chat.", Required: false},
		{Name: "message_id", Description: "Message ID. Omit for replied/current message.", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		emoji := strings.TrimSpace(args["emoji"])
		if emoji == "" {
			return "Error: emoji is required"
		}
		chat := resolveContextPeer(args["chat_id"], userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		msgID := resolveContextMessageID(args["message_id"], userID)
		if msgID == 0 {
			return "Error: message_id could not be inferred"
		}
		if TGReactFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGReactFn(chat, msgID, emoji)
	},
}

var TGGetMembers = &ToolDef{
	Name:        "tg_get_members",
	Description: "List members of a group or channel. Omit chat_id for current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Description: "Group/channel ID or @username. Omit for current.", Required: false},
		{Name: "limit", Description: "Max members to return (default 50, max 200)", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		chat := resolveContextPeer(args["chat_id"], userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		limit := 50
		if s := strings.TrimSpace(args["limit"]); s != "" {
			fmt.Sscanf(s, "%d", &limit)
			if limit <= 0 || limit > 200 {
				limit = 50
			}
		}
		if TGGetMembersFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGGetMembersFn(chat, limit)
	},
}

var TGBroadcast = &ToolDef{
	Name:        "tg_broadcast",
	Description: "Send the same message to multiple chats.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_ids", Description: "Comma-separated chat IDs or @usernames", Required: true},
		{Name: "text", Description: "Message text (HTML allowed)", Required: true},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		idsStr := strings.TrimSpace(args["chat_ids"])
		text := strings.TrimSpace(args["text"])
		if idsStr == "" || text == "" {
			return "Error: chat_ids and text are required"
		}
		var peers []string
		for p := range strings.SplitSeq(idsStr, ",") {
			if p = strings.TrimSpace(p); p != "" {
				peers = append(peers, p)
			}
		}
		if len(peers) == 0 {
			return "Error: no valid peers"
		}
		if TGBroadcastFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGBroadcastFn(peers, text)
	},
}

var TGGetMessage = &ToolDef{
	Name:        "tg_get_message",
	Description: "Fetch a specific message by ID. Omit chat_id for current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Description: "Chat ID or @username. Omit for current chat.", Required: false},
		{Name: "message_id", Description: "Message ID to fetch", Required: true},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		msgStr := strings.TrimSpace(args["message_id"])
		if msgStr == "" {
			return "Error: message_id is required"
		}
		chat := resolveContextPeer(args["chat_id"], userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		var msgID int32
		if _, err := fmt.Sscanf(msgStr, "%d", &msgID); err != nil {
			return "Error: message_id must be numeric"
		}
		if TGGetMessageFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGGetMessageFn(chat, msgID)
	},
}

var TGEditMessage = &ToolDef{
	Name:        "tg_edit_message",
	Description: "Edit a sent message. Omit chat_id for current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Description: "Chat ID or @username. Omit for current chat.", Required: false},
		{Name: "message_id", Description: "Message ID to edit", Required: true},
		{Name: "text", Description: "New message text (HTML allowed)", Required: true},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		msgStr := strings.TrimSpace(args["message_id"])
		text := strings.TrimSpace(args["text"])
		if msgStr == "" || text == "" {
			return "Error: message_id and text are required"
		}
		chat := resolveContextPeer(args["chat_id"], userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		var msgID int32
		if _, err := fmt.Sscanf(msgStr, "%d", &msgID); err != nil {
			return "Error: message_id must be numeric"
		}
		if TGEditMessageFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGEditMessageFn(chat, msgID, text)
	},
}

var TGCreateInvite = &ToolDef{
	Name:        "tg_create_invite",
	Description: "Create an invite link for a group/channel. Omit chat_id for current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Description: "Chat ID or @username. Omit for current.", Required: false},
		{Name: "expire_date", Description: "Expiration Unix timestamp (0 = never)", Required: false},
		{Name: "member_limit", Description: "Max members via link (0 = unlimited)", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		chat := resolveContextPeer(args["chat_id"], userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		var expiry, limit int32
		fmt.Sscanf(args["expire_date"], "%d", &expiry)
		fmt.Sscanf(args["member_limit"], "%d", &limit)
		if TGCreateInviteFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGCreateInviteFn(chat, expiry, limit)
	},
}

var TGGetProfilePhotos = &ToolDef{
	Name:        "tg_get_profile_photos",
	Description: "Get profile photos of a user. Defaults to 'me'. Supports IDs and @usernames.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "peer", Description: "User ID or @username. Omit for self.", Required: false},
		{Name: "limit", Description: "Max photos (default 10, max 100)", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		peer := strings.TrimSpace(args["peer"])
		if peer == "" {
			peer = "me"
		}
		peer = resolveContextPeer(peer, userID)
		if peer == "" {
			return "Error: peer required"
		}
		limit := 10
		if s := strings.TrimSpace(args["limit"]); s != "" {
			fmt.Sscanf(s, "%d", &limit)
			if limit <= 0 || limit > 100 {
				limit = 10
			}
		}
		if TGGetProfilePhotosFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGGetProfilePhotosFn(peer, limit)
	},
}

var TGBanUser = &ToolDef{
	Name:        "tg_ban_user",
	Description: "Ban a user from a group/channel. Optionally delete their message history and set ban duration. Omit chat_id for current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Description: "Group/channel ID or @username. Omit for current.", Required: false},
		{Name: "user_id", Description: "User ID or @username to ban", Required: true},
		{Name: "delete_history", Description: "Delete user's messages (true/false, default false)", Required: false},
		{Name: "until_date", Description: "Unix timestamp for ban expiry (0 = permanent)", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		chat := resolveContextPeer(args["chat_id"], userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		target := strings.TrimSpace(args["user_id"])
		if target == "" {
			return "Error: user_id is required"
		}
		deleteHistory := strings.EqualFold(args["delete_history"], "true")
		var untilDate int32
		fmt.Sscanf(args["until_date"], "%d", &untilDate)
		if TGBanUserFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGBanUserFn(chat, target, deleteHistory, untilDate)
	},
}

var TGMuteUser = &ToolDef{
	Name:        "tg_mute_user",
	Description: "Mute (restrict) a user in a group so they cannot send messages. Omit chat_id for current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Description: "Group/channel ID or @username. Omit for current.", Required: false},
		{Name: "user_id", Description: "User ID or @username to mute", Required: true},
		{Name: "until_date", Description: "Unix timestamp for mute expiry (0 = permanent)", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		chat := resolveContextPeer(args["chat_id"], userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		target := strings.TrimSpace(args["user_id"])
		if target == "" {
			return "Error: user_id is required"
		}
		var untilDate int32
		fmt.Sscanf(args["until_date"], "%d", &untilDate)
		if TGMuteUserFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGMuteUserFn(chat, target, untilDate)
	},
}

var TGKickUser = &ToolDef{
	Name:        "tg_kick_user",
	Description: "Kick (remove) a user from a group. They can rejoin via invite. Omit chat_id for current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Description: "Group/channel ID or @username. Omit for current.", Required: false},
		{Name: "user_id", Description: "User ID or @username to kick", Required: true},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		chat := resolveContextPeer(args["chat_id"], userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		target := strings.TrimSpace(args["user_id"])
		if target == "" {
			return "Error: user_id is required"
		}
		if TGKickUserFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGKickUserFn(chat, target)
	},
}

var TGPromoteAdmin = &ToolDef{
	Name:        "tg_promote_admin",
	Description: "Promote a user to admin in a group/channel with specific rights. Omit chat_id for current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Description: "Group/channel ID or @username. Omit for current.", Required: false},
		{Name: "user_id", Description: "User ID or @username to promote", Required: true},
		{Name: "title", Description: "Custom admin title (optional)", Required: false},
		{Name: "rights", Description: "JSON object of rights: {\"post_messages\":true,\"delete_messages\":true,\"ban_users\":true,\"invite_users\":true,\"pin_messages\":true,\"manage_call\":true}", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		chat := resolveContextPeer(args["chat_id"], userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		target := strings.TrimSpace(args["user_id"])
		if target == "" {
			return "Error: user_id is required"
		}
		rights := map[string]bool{}
		if r := strings.TrimSpace(args["rights"]); r != "" {
			_ = json.Unmarshal([]byte(r), &rights)
		}
		if TGPromoteAdminFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGPromoteAdminFn(chat, target, rights, strings.TrimSpace(args["title"]))
	},
}

var TGDemoteAdmin = &ToolDef{
	Name:        "tg_demote_admin",
	Description: "Remove admin rights from a user in a group/channel. Omit chat_id for current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Description: "Group/channel ID or @username. Omit for current.", Required: false},
		{Name: "user_id", Description: "User ID or @username to demote", Required: true},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		chat := resolveContextPeer(args["chat_id"], userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		target := strings.TrimSpace(args["user_id"])
		if target == "" {
			return "Error: user_id is required"
		}
		if TGDemoteAdminFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGDemoteAdminFn(chat, target)
	},
}

// === Button parsing ===

func parseButtons(buttonsB64 string) *telegram.ReplyInlineMarkup {
	jsonBytes, err := base64.StdEncoding.DecodeString(buttonsB64)
	if err != nil {
		return nil
	}
	var spec ButtonsSpec
	if err := json.Unmarshal(jsonBytes, &spec); err != nil {
		return nil
	}
	return parseButtonsSpec(&spec)
}

func parseButtonsSpec(spec *ButtonsSpec) *telegram.ReplyInlineMarkup {
	kb := telegram.NewKeyboard()
	for _, rowSpec := range spec.Rows {
		var rowBtns []telegram.KeyboardButton
		for _, btnSpec := range rowSpec.Buttons {
			var btn telegram.KeyboardButton
			switch btnSpec.Type {
			case "url":
				b := telegram.Button.URL(btnSpec.Text, btnSpec.URL)
				switch btnSpec.Style {
				case "success":
					b.Success()
				case "danger":
					b.Danger()
				default:
					b.Primary()
				}
				btn = b
			default: // "data"
				b := telegram.Button.Data(btnSpec.Text, btnSpec.Data)
				switch btnSpec.Style {
				case "success":
					b.Success()
				case "danger":
					b.Danger()
				default:
					b.Primary()
				}
				btn = b
			}
			rowBtns = append(rowBtns, btn)
		}
		if len(rowBtns) > 0 {
			kb.AddRow(rowBtns...)
		}
	}
	return kb.Build()
}
