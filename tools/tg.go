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
var SendTGMsgFn func(
	peer string,
	text string,
	replyToID int32,
	replyQuote string,
	silent bool,
	scheduleAt string,
	selfDestructSeconds int,
	reactEmoji string,
	forwardFrom string,
	forwardMsgIDs []int32,
) string
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
var SendTGRichFn func(peer string, blocksJSON string) string
var TGCreateInviteFn func(peer string, expireDate int32, memberLimit int32) string
var TGGetProfilePhotosFn func(peer string, limit int) string
var TGBanUserFn func(peer string, userID string, deleteHistory bool, untilDate int32) string
var TGMuteUserFn func(peer string, userID string, untilDate int32) string
var TGKickUserFn func(peer string, userID string) string
var TGPromoteAdminFn func(peer string, userID string, rights map[string]bool, title string) string
var TGDemoteAdminFn func(peer string, userID string) string
var TGSendLocationFn func(peer string, lat, long float64) string
var TGGetFileFn func(peer string, msgID int32, savePath string) string

// GetTelegramContextFn returns per-user Telegram context (chat id, sender id,
// replied-to message id, etc.) — wired in core/register.go.
var GetTelegramContextFn func(userID string) map[string]any

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
	Description: "Send a Telegram message. Supports HTML formatting, replies (with quoted snippet), silent send, native scheduled send, client-side self-destruct, post-send reaction, and message forwarding. When forward_from and forward_msg_ids are set, text/reply_to are ignored.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "text", Type: ArgString, Description: "Message text (HTML allowed). Ignored when forwarding.", Required: false},
		{Name: "target", Type: ArgString, Description: "Destination chat ID, @username, or 'me'. Omit for current chat.", Required: false},
		{Name: "reply_to_id", Type: ArgInt, Description: "Message ID to reply to.", Required: false},
		{Name: "reply_quote", Type: ArgString, Description: "Snippet to show as native Telegram quoted reply. Requires reply_to_id.", Required: false},
		{Name: "silent", Type: ArgBool, Description: "Send without notification ping.", Required: false},
		{Name: "schedule_at", Type: ArgString, Description: "RFC3339 timestamp for native scheduled send (e.g. 2026-12-31T15:04:05Z).", Required: false},
		{Name: "self_destruct_seconds", Type: ArgInt, Description: "Auto-delete sent message after N seconds.", Required: false},
		{Name: "react_emoji", Type: ArgString, Description: "Emoji to react with on the replied-to message. Requires reply_to_id.", Required: false},
		{Name: "forward_from", Type: ArgString, Description: "Source chat to forward from. Ignores text/reply_to_id.", Required: false},
		{Name: "forward_msg_ids", Type: ArgList, Description: "Message IDs in forward_from to forward (comma-separated or array).", Required: false},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		target := resolveContextPeer(String(args, "target"), userID)
		if target == "" {
			return "Error: no current chat context"
		}
		if SendTGMsgFn == nil {
			return "Error: Telegram not initialized"
		}
		text := String(args, "text")
		replyToID, _ := Int(args, "reply_to_id")
		replyQuote := String(args, "reply_quote")
		silent := BoolOr(args, "silent", false)
		scheduleAt := String(args, "schedule_at")
		selfDestruct, _ := Int(args, "self_destruct_seconds")
		reactEmoji := String(args, "react_emoji")
		forwardFrom := String(args, "forward_from")
		// forward_msg_ids may come as []any or comma-separated string
		var fwdIDs []int32
		for _, n := range IntList(args, "forward_msg_ids") {
			if n > 0 {
				fwdIDs = append(fwdIDs, int32(n))
			}
		}
		if forwardFrom == "" && len(fwdIDs) == 0 && text == "" {
			return "Error: text is required (or use forward_from + forward_msg_ids)"
		}
		if replyQuote != "" && replyToID == 0 {
			return "Error: reply_quote requires reply_to_id"
		}
		if reactEmoji != "" && replyToID == 0 {
			return "Error: react_emoji requires reply_to_id"
		}
		return SendTGMsgFn(target, text, int32(replyToID), replyQuote, silent, scheduleAt, selfDestruct, reactEmoji, forwardFrom, fwdIDs)
	},
}

var TGSendFile = &ToolDef{
	Name: "tg_send_file",
	Description: "Send a local file to a Telegram chat. Images (jpg/png/gif/webp) and videos (mp4/avi/mkv/mov/webm) " +
		"are sent as media by default. All other files are sent as documents. " +
		"Set doc=true to force document mode regardless of file type. Omit target for current chat.",
	Secure: true,
	Args: []ToolArg{
		{Name: "path", Type: ArgString, Description: "Absolute path of the file", Required: true},
		{Name: "caption", Type: ArgString, Description: "Optional caption", Required: false},
		{Name: "target", Type: ArgString, Description: "Chat ID, @username, or 'me'. Omit for current chat.", Required: false},
		{Name: "doc", Type: ArgBool, Description: "true to force send as document. Default: auto by extension.", Required: false},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		path := String(args, "path")
		if path == "" {
			return "Error: path is required"
		}
		target := resolveContextPeer(String(args, "target"), userID)
		if target == "" {
			return "Error: no current chat context"
		}
		if SendTGFileFn == nil {
			return "Error: Telegram not initialized"
		}
		var forceDoc bool
		if b, ok := Bool(args, "doc"); ok {
			forceDoc = b
		} else {
			forceDoc = !isMediaFile(path)
		}
		if r := SendTGFileFn(target, path, String(args, "caption"), forceDoc); r != "" {
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
		{Name: "path", Type: ArgString, Description: "Local path or Telegram FileID", Required: true},
		{Name: "caption", Type: ArgString, Description: "Optional caption", Required: false},
		{Name: "target", Type: ArgString, Description: "Chat ID, @username, or 'me'. Omit for current chat.", Required: false},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		path := String(args, "path")
		if path == "" {
			return "Error: path is required"
		}
		target := resolveContextPeer(String(args, "target"), userID)
		if target == "" {
			return "Error: no current chat context"
		}
		if SendTGPhotoFn == nil {
			return "Error: Telegram not initialized"
		}
		if r := SendTGPhotoFn(target, path, String(args, "caption")); r != "" {
			return r
		}
		return "Sent photo"
	},
}

var TGSendAlbum = &ToolDef{
	Name:        "tg_send_album",
	Description: "Send multiple photos/videos as an album (media group). Paths comma-separated or array. Omit target for current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "paths", Type: ArgList, Description: "List of local file paths or URLs (comma-separated or array)", Required: true},
		{Name: "caption", Type: ArgString, Description: "Optional caption for the album", Required: false},
		{Name: "target", Type: ArgString, Description: "Chat ID, @username, or 'me'. Omit for current chat.", Required: false},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		paths := List(args, "paths")
		if len(paths) == 0 {
			return "Error: paths is required"
		}
		target := resolveContextPeer(String(args, "target"), userID)
		if target == "" {
			return "Error: no current chat context"
		}
		if SendTGAlbumFn == nil {
			return "Error: Telegram not initialized"
		}
		// trim each path
		cleaned := make([]string, 0, len(paths))
		for _, p := range paths {
			if p = strings.TrimSpace(p); p != "" {
				cleaned = append(cleaned, p)
			}
		}
		if len(cleaned) == 0 {
			return "Error: no valid paths provided"
		}
		if r := SendTGAlbumFn(target, cleaned, String(args, "caption")); r != "" {
			return r
		}
		return fmt.Sprintf("Sent album (%d files)", len(cleaned))
	},
}

var TGSendLocation = &ToolDef{
	Name:        "tg_send_location",
	Description: "Send a location pin to a Telegram chat. Omit target for current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "lat", Type: ArgFloat, Description: "Latitude (e.g. 37.7749)", Required: true},
		{Name: "long", Type: ArgFloat, Description: "Longitude (e.g. -122.4194)", Required: true},
		{Name: "target", Type: ArgString, Description: "Chat ID, @username, or 'me'. Omit for current chat.", Required: false},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		target := resolveContextPeer(String(args, "target"), userID)
		if target == "" {
			return "Error: no current chat context"
		}
		if TGSendLocationFn == nil {
			return "Error: Telegram not initialized"
		}
		lat, ok := Float(args, "lat")
		if !ok {
			return "Error: invalid lat"
		}
		long, ok := Float(args, "long")
		if !ok {
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
		{Name: "text", Type: ArgString, Description: "Message text", Required: true},
		{Name: "buttons", Type: ArgString, Description: "Buttons as BASE64-ENCODED JSON", Required: false},
		{Name: "target", Type: ArgString, Description: "Chat ID, @username, or 'me'. Omit for current chat.", Required: false},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		text := String(args, "text")
		if text == "" {
			return "Error: text is required"
		}
		target := resolveContextPeer(String(args, "target"), userID)
		if target == "" {
			return "Error: no current chat context"
		}
		if SendTGMessageWithButtonsFn == nil {
			return "Error: Telegram not initialized"
		}
		var kb *telegram.ReplyInlineMarkup
		if b64 := String(args, "buttons"); b64 != "" {
			kb = parseButtons(b64)
			if kb == nil {
				return "Error: failed to parse buttons"
			}
		}
		return SendTGMessageWithButtonsFn(target, text, kb)
	},
}

var TGSendRich = &ToolDef{
	Name: "tg_send_rich",
	Description: "Send a Telegram rich-page message with collapsible sections, tables, quotes, and paragraphs. " +
		"Use this when the answer is long or has multiple sections — fold details behind collapsibles so the chat stays clean. " +
		"blocks must be BASE64-ENCODED JSON array of block objects. Block types: " +
		"{\"type\":\"p\",\"text\":\"...\"}, " +
		"{\"type\":\"quote\",\"text\":\"...\"}, " +
		"{\"type\":\"divider\"}, " +
		"{\"type\":\"details\",\"title\":\"label\",\"open\":false,\"blocks\":[ ...nested blocks... ]}, " +
		"{\"type\":\"table\",\"header\":true,\"rows\":[[\"col1\",\"col2\"],[\"a\",\"b\"]]}.",
	Secure: true,
	Args: []ToolArg{
		{Name: "blocks", Type: ArgString, Description: "BASE64-ENCODED JSON array of block specs", Required: true},
		{Name: "target", Type: ArgString, Description: "Chat ID, @username, or 'me'. Omit for current chat.", Required: false},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		b64 := String(args, "blocks")
		if b64 == "" {
			return "Error: blocks is required"
		}
		target := resolveContextPeer(String(args, "target"), userID)
		if target == "" {
			return "Error: no current chat context"
		}
		decoded, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return fmt.Sprintf("Error decoding base64: %v", err)
		}
		if SendTGRichFn == nil {
			return "Error: Telegram not initialized"
		}
		if r := SendTGRichFn(target, string(decoded)); r != "" {
			return r
		}
		return "Sent"
	},
}

var SetBotDp = &ToolDef{
	Name:        "set_bot_dp",
	Description: "Set the bot profile picture. If reply has a photo, auto-uses it. Otherwise provide file path or URL.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "image", Type: ArgString, Description: "Local file path or image URL. Omit to use replied-to photo.", Required: false},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		image := String(args, "image")
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
		{Name: "chat_id", Type: ArgString, Description: "Chat ID or @username. Omit for current chat.", Required: false},
		{Name: "message_id", Type: ArgString, Description: "Message ID with media. Omit for replied message.", Required: false},
		{Name: "save_as", Type: ArgString, Description: "Optional local file path to save to", Required: false},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		chat := resolveContextPeer(String(args, "chat_id"), userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		msgID := resolveContextMessageID(String(args, "message_id"), userID)
		if msgID == 0 {
			return "Error: message_id required and could not be inferred"
		}
		if TGDownloadMediaFn == nil {
			return "Error: Telegram not initialized"
		}
		path, err := TGDownloadMediaFn(chat, msgID, String(args, "save_as"))
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
		{Name: "chat_id", Type: ArgString, Description: "Chat ID or @username. Omit for current chat.", Required: false},
		{Name: "message_id", Type: ArgString, Description: "Message ID with the file. Omit for replied message.", Required: false},
		{Name: "save_as", Type: ArgString, Description: "Optional save path", Required: false},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		chat := resolveContextPeer(String(args, "chat_id"), userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		msgID := resolveContextMessageID(String(args, "message_id"), userID)
		if msgID == 0 {
			return "Error: message_id required and could not be inferred"
		}
		if TGGetFileFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGGetFileFn(chat, msgID, String(args, "save_as"))
	},
}

var TGForwardMsg = &ToolDef{
	Name:        "tg_forward",
	Description: "Forward a message from one chat to another. Omit from/to for current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "from_chat_id", Type: ArgString, Description: "Source chat ID or @username. Omit for current chat.", Required: false},
		{Name: "message_id", Type: ArgInt, Description: "Message ID to forward", Required: true},
		{Name: "to_chat_id", Type: ArgString, Description: "Destination chat ID or @username. Omit for current chat.", Required: false},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		msgID, ok := Int(args, "message_id")
		if !ok || msgID == 0 {
			return "Error: message_id is required"
		}
		from := resolveContextPeer(String(args, "from_chat_id"), userID)
		to := resolveContextPeer(String(args, "to_chat_id"), userID)
		if from == "" || to == "" {
			return "Error: from/to chat could not be inferred"
		}
		if TGForwardMsgFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGForwardMsgFn(from, int32(msgID), to)
	},
}

var TGDeleteMsg = &ToolDef{
	Name:        "tg_delete_msg",
	Description: "Delete messages from a chat. Omit chat_id for current chat. Omit message_ids to delete replied-to message.",
	Secure:      false,
	Args: []ToolArg{
		{Name: "chat_id", Type: ArgString, Description: "Chat ID or @username. Omit for current chat.", Required: false},
		{Name: "message_ids", Type: ArgList, Description: "Message IDs (comma-separated or array). Omit to delete replied-to message.", Required: false},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		chat := resolveContextPeer(String(args, "chat_id"), userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		var msgIDs []int32
		ids := IntList(args, "message_ids")
		if len(ids) == 0 {
			id := resolveContextMessageID("", userID)
			if id == 0 {
				return "Error: no message to delete"
			}
			msgIDs = append(msgIDs, id)
		} else {
			for _, id := range ids {
				if id > 0 {
					msgIDs = append(msgIDs, int32(id))
				}
			}
		}
		if len(msgIDs) == 0 {
			return "Error: no valid message IDs"
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
		{Name: "chat_id", Type: ArgString, Description: "Chat ID or @username. Omit for current chat.", Required: false},
		{Name: "message_id", Type: ArgString, Description: "Message ID to pin. Omit for replied message.", Required: false},
		{Name: "silent", Type: ArgBool, Description: "Pin silently (default false)", Required: false},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		chat := resolveContextPeer(String(args, "chat_id"), userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		msgID := resolveContextMessageID(String(args, "message_id"), userID)
		if msgID == 0 {
			return "Error: message_id could not be inferred"
		}
		if TGPinMsgFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGPinMsgFn(chat, msgID, BoolOr(args, "silent", false))
	},
}

var TGUnpinMsg = &ToolDef{
	Name:        "tg_unpin_msg",
	Description: "Unpin a message from a chat. Omit chat_id for current chat. Omit message_id for replied-to message.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Type: ArgString, Description: "Chat ID or @username. Omit for current chat.", Required: false},
		{Name: "message_id", Type: ArgString, Description: "Message ID to unpin. Omit for replied message.", Required: false},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		chat := resolveContextPeer(String(args, "chat_id"), userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		msgID := resolveContextMessageID(String(args, "message_id"), userID)
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
		{Name: "peer", Type: ArgString, Description: "Chat/user ID (numeric) or @username. Omit for current chat.", Required: false},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		peer := resolveContextPeer(String(args, "peer"), userID)
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
		{Name: "emoji", Type: ArgString, Description: "Emoji reaction (e.g. '👍', '❤️', '🔥')", Required: true},
		{Name: "chat_id", Type: ArgString, Description: "Chat ID or @username. Omit for current chat.", Required: false},
		{Name: "message_id", Type: ArgString, Description: "Message ID. Omit for replied/current message.", Required: false},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		emoji := String(args, "emoji")
		if emoji == "" {
			return "Error: emoji is required"
		}
		chat := resolveContextPeer(String(args, "chat_id"), userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		msgID := resolveContextMessageID(String(args, "message_id"), userID)
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
		{Name: "chat_id", Type: ArgString, Description: "Group/channel ID or @username. Omit for current.", Required: false},
		{Name: "limit", Type: ArgInt, Description: "Max members to return (default 50, max 200)", Required: false},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		chat := resolveContextPeer(String(args, "chat_id"), userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		limit := IntOr(args, "limit", 50)
		if limit <= 0 || limit > 200 {
			limit = 50
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
		{Name: "chat_ids", Type: ArgList, Description: "Chat IDs or @usernames (comma-separated or array)", Required: true},
		{Name: "text", Type: ArgString, Description: "Message text (HTML allowed)", Required: true},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		peers := List(args, "chat_ids")
		text := String(args, "text")
		if len(peers) == 0 || text == "" {
			return "Error: chat_ids and text are required"
		}
		// trim each peer
		cleaned := make([]string, 0, len(peers))
		for _, p := range peers {
			if p = strings.TrimSpace(p); p != "" {
				cleaned = append(cleaned, p)
			}
		}
		if len(cleaned) == 0 {
			return "Error: no valid peers"
		}
		if TGBroadcastFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGBroadcastFn(cleaned, text)
	},
}

var TGGetMessage = &ToolDef{
	Name:        "tg_get_message",
	Description: "Fetch a specific message by ID. Omit chat_id for current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Type: ArgString, Description: "Chat ID or @username. Omit for current chat.", Required: false},
		{Name: "message_id", Type: ArgInt, Description: "Message ID to fetch", Required: true},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		msgID, ok := Int(args, "message_id")
		if !ok || msgID == 0 {
			return "Error: message_id is required"
		}
		chat := resolveContextPeer(String(args, "chat_id"), userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		if TGGetMessageFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGGetMessageFn(chat, int32(msgID))
	},
}

var TGEditMessage = &ToolDef{
	Name:        "tg_edit_message",
	Description: "Edit a sent message. Omit chat_id for current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Type: ArgString, Description: "Chat ID or @username. Omit for current chat.", Required: false},
		{Name: "message_id", Type: ArgInt, Description: "Message ID to edit", Required: true},
		{Name: "text", Type: ArgString, Description: "New message text (HTML allowed)", Required: true},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		msgID, ok := Int(args, "message_id")
		text := String(args, "text")
		if !ok || msgID == 0 || text == "" {
			return "Error: message_id and text are required"
		}
		chat := resolveContextPeer(String(args, "chat_id"), userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		if TGEditMessageFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGEditMessageFn(chat, int32(msgID), text)
	},
}

var TGCreateInvite = &ToolDef{
	Name:        "tg_create_invite",
	Description: "Create an invite link for a group/channel. Omit chat_id for current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Type: ArgString, Description: "Chat ID or @username. Omit for current.", Required: false},
		{Name: "expire_date", Type: ArgInt, Description: "Expiration Unix timestamp (0 = never)", Required: false},
		{Name: "member_limit", Type: ArgInt, Description: "Max members via link (0 = unlimited)", Required: false},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		chat := resolveContextPeer(String(args, "chat_id"), userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		expiry := IntOr(args, "expire_date", 0)
		limit := IntOr(args, "member_limit", 0)
		if TGCreateInviteFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGCreateInviteFn(chat, int32(expiry), int32(limit))
	},
}

var TGGetProfilePhotos = &ToolDef{
	Name:        "tg_get_profile_photos",
	Description: "Get profile photos of a user. Defaults to 'me'. Supports IDs and @usernames.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "peer", Type: ArgString, Description: "User ID or @username. Omit for self.", Required: false},
		{Name: "limit", Type: ArgInt, Description: "Max photos (default 10, max 100)", Required: false},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		peer := String(args, "peer")
		if peer == "" {
			peer = "me"
		}
		peer = resolveContextPeer(peer, userID)
		if peer == "" {
			return "Error: peer required"
		}
		limit := IntOr(args, "limit", 10)
		if limit <= 0 || limit > 100 {
			limit = 10
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
		{Name: "chat_id", Type: ArgString, Description: "Group/channel ID or @username. Omit for current.", Required: false},
		{Name: "user_id", Type: ArgString, Description: "User ID or @username to ban", Required: true},
		{Name: "delete_history", Type: ArgBool, Description: "Delete user's messages (default false)", Required: false},
		{Name: "until_date", Type: ArgInt, Description: "Unix timestamp for ban expiry (0 = permanent)", Required: false},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		chat := resolveContextPeer(String(args, "chat_id"), userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		target := String(args, "user_id")
		if target == "" {
			return "Error: user_id is required"
		}
		deleteHistory := BoolOr(args, "delete_history", false)
		untilDate := IntOr(args, "until_date", 0)
		if TGBanUserFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGBanUserFn(chat, target, deleteHistory, int32(untilDate))
	},
}

var TGMuteUser = &ToolDef{
	Name:        "tg_mute_user",
	Description: "Mute (restrict) a user in a group so they cannot send messages. Omit chat_id for current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Type: ArgString, Description: "Group/channel ID or @username. Omit for current.", Required: false},
		{Name: "user_id", Type: ArgString, Description: "User ID or @username to mute", Required: true},
		{Name: "until_date", Type: ArgInt, Description: "Unix timestamp for mute expiry (0 = permanent)", Required: false},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		chat := resolveContextPeer(String(args, "chat_id"), userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		target := String(args, "user_id")
		if target == "" {
			return "Error: user_id is required"
		}
		untilDate := IntOr(args, "until_date", 0)
		if TGMuteUserFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGMuteUserFn(chat, target, int32(untilDate))
	},
}

var TGKickUser = &ToolDef{
	Name:        "tg_kick_user",
	Description: "Kick (remove) a user from a group. They can rejoin via invite. Omit chat_id for current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Type: ArgString, Description: "Group/channel ID or @username. Omit for current.", Required: false},
		{Name: "user_id", Type: ArgString, Description: "User ID or @username to kick", Required: true},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		chat := resolveContextPeer(String(args, "chat_id"), userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		target := String(args, "user_id")
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
		{Name: "chat_id", Type: ArgString, Description: "Group/channel ID or @username. Omit for current.", Required: false},
		{Name: "user_id", Type: ArgString, Description: "User ID or @username to promote", Required: true},
		{Name: "title", Type: ArgString, Description: "Custom admin title (optional)", Required: false},
		{Name: "rights", Type: ArgDict, Description: "Rights map: {\"post_messages\":true,\"delete_messages\":true,\"ban_users\":true,\"invite_users\":true,\"pin_messages\":true,\"manage_call\":true}", Required: false},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		chat := resolveContextPeer(String(args, "chat_id"), userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		target := String(args, "user_id")
		if target == "" {
			return "Error: user_id is required"
		}
		rights := map[string]bool{}
		if m := Dict(args, "rights"); m != nil {
			for k, v := range m {
				switch t := v.(type) {
				case bool:
					rights[k] = t
				case string:
					rights[k] = strings.EqualFold(strings.TrimSpace(t), "true")
				case float64:
					rights[k] = t != 0
				}
			}
		} else if r := String(args, "rights"); r != "" {
			// fall back: try unmarshalling as JSON string
			_ = json.Unmarshal([]byte(r), &rights)
		}
		if TGPromoteAdminFn == nil {
			return "Error: Telegram not initialized"
		}
		return TGPromoteAdminFn(chat, target, rights, String(args, "title"))
	},
}

var TGDemoteAdmin = &ToolDef{
	Name:        "tg_demote_admin",
	Description: "Remove admin rights from a user in a group/channel. Omit chat_id for current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Type: ArgString, Description: "Group/channel ID or @username. Omit for current.", Required: false},
		{Name: "user_id", Type: ArgString, Description: "User ID or @username to demote", Required: true},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		chat := resolveContextPeer(String(args, "chat_id"), userID)
		if chat == "" {
			return "Error: no current chat context"
		}
		target := String(args, "user_id")
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
