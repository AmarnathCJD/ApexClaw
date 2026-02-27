package tools

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/amarnathcjd/gogram/telegram"
)

// === Function Pointers for Core Implementation ===
// All accept peer strings (usernames, numeric IDs, or special values)
// Core functions handle ResolvePeer internally

var SendTGFileFn func(peer string, filePath, caption string) string
var SendTGMsgFn func(peer string, text string) string
var SendTGPhotoURLFn func(peer string, photoURL, caption string) string
var SendTGAlbumURLsFn func(peer string, photoURLs []string, caption string) string
var SetBotDpFn func(filePathOrURL string) string
var TGDownloadMediaFn func(peer string, messageID int32, savePath string) (string, error)
var TGGetChatInfoFn func(peer string) string
var TGResolvePeerFn func(peer string) (any, error)
var TGForwardMsgFn func(fromPeer string, msgID int32, toPeer string) string
var TGDeleteMsgFn func(peer string, msgIDs []int32) string
var TGPinMsgFn func(peer string, msgID int32, silent bool) string
var TGUnpinMsgFn func(peer string, msgID int32) string
var TGReactFn func(peer string, msgID int32, emoji string) string
var TGGetReplyFn func(peer string, msgID int32) string
var TGGetMembersFn func(peer string, limit int) string
var TGBroadcastFn func(peers []string, text string) string
var TGGetMessageFn func(peer string, msgID int32) string
var TGEditMessageFn func(peer string, msgID int32, newText string) string
var SendTGMessageWithButtonsFn func(peer string, text string, kb *telegram.ReplyInlineMarkup) string
var TGCreateInviteFn func(peer string, expireDate int32, memberLimit int32) string
var TGGetProfilePhotosFn func(peer string, limit int) string

// === Helper Types for Buttons ===

type ButtonSpec struct {
	Text  string `json:"text"`
	Type  string `json:"type"`  // "data" or "url"
	Data  string `json:"data"`  // callback data for type=data
	URL   string `json:"url"`   // url for type=url
	Style string `json:"style"` // "danger", "success", "primary"
}

type ButtonRowSpec struct {
	Buttons []ButtonSpec `json:"buttons"`
}

type ButtonsSpec struct {
	Rows []ButtonRowSpec `json:"rows"`
}

func resolveContextPeer(peerStr string, userID string) string {
	if peerStr == "" || peerStr == "current" {
		if GetTelegramContextFn != nil {
			ctx := GetTelegramContextFn(userID)
			if ctx != nil {
				if v, ok := ctx["telegram_id"]; ok {
					return fmt.Sprintf("%d", v.(int64))
				}
			}
		}
		return ""
	}

	if peerStr == "me" {
		if GetTelegramContextFn != nil {
			ctx := GetTelegramContextFn(userID)
			if ctx != nil {
				if v, ok := ctx["owner_id"]; ok {
					return v.(string)
				}
			}
		}
		return ""
	}

	return peerStr
}

var TGSendFile = &ToolDef{
	Name:        "tg_send_file",
	Description: "Send a local file to a Telegram chat. target can be a chat ID, @username, or 'me'. Omit target to use current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "path", Description: "Absolute or relative path of the file to send", Required: true},
		{Name: "caption", Description: "Optional caption for the file", Required: false},
		{Name: "target", Description: "Chat ID (numeric), @username, or 'me'. Omit for current chat.", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		path := strings.TrimSpace(args["path"])
		caption := strings.TrimSpace(args["caption"])
		target := strings.TrimSpace(args["target"])

		if path == "" {
			return "Error: path is required"
		}

		// Resolve context-based peers
		target = resolveContextPeer(target, userID)
		if target == "" {
			return "Error: no current chat context"
		}

		if SendTGFileFn == nil {
			return "Error: Telegram file sender not initialized"
		}
		// Pass peer string directly - core function handles ResolvePeer
		if result := SendTGFileFn(target, path, caption); result != "" {
			return result
		}
		return fmt.Sprintf("Sent %s", path)
	},
}

var TGSendMessage = &ToolDef{
	Name:        "tg_send_message",
	Description: "Send a text message to a Telegram chat. target can be a chat ID, @username, or 'me'. Omit target to use current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "text", Description: "The message text to send (HTML formatting allowed)", Required: true},
		{Name: "target", Description: "Chat ID (numeric), @username, or 'me'. Omit for current chat.", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		text := strings.TrimSpace(args["text"])
		target := strings.TrimSpace(args["target"])

		if text == "" {
			return "Error: text is required"
		}

		target = resolveContextPeer(target, userID)
		if target == "" {
			return "Error: no current chat context"
		}

		if SendTGMsgFn == nil {
			return "Error: Telegram sender not initialized"
		}
		if result := SendTGMsgFn(target, text); result != "" {
			return result
		}
		return "Sent message"
	},
}

var TGSendMessageWithButtons = &ToolDef{
	Name:        "tg_send_message_buttons",
	Description: "Send Telegram message with inline buttons. Buttons parameter should be base64-encoded JSON. Format: {\"rows\":[{\"buttons\":[{\"text\":\"Yes\",\"type\":\"data\",\"data\":\"yes\",\"style\":\"success\"}]}]}. Styles: success(green), danger(red), primary(blue). Type: data(callback) or url(link).",
	Secure:      true,
	Args: []ToolArg{
		{Name: "text", Description: "Message text to send", Required: true},
		{Name: "buttons", Description: "Buttons as BASE64-ENCODED JSON", Required: false},
		{Name: "target", Description: "Chat ID, @username, or 'me'. Omit for current chat.", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		text := strings.TrimSpace(args["text"])
		buttonsJSON := strings.TrimSpace(args["buttons"])
		target := strings.TrimSpace(args["target"])

		if text == "" {
			return "Error: text is required"
		}

		target = resolveContextPeer(target, userID)
		if target == "" {
			return "Error: no current chat context"
		}

		if SendTGMessageWithButtonsFn == nil {
			return "Error: button sender not initialized"
		}

		var kb *telegram.ReplyInlineMarkup
		if buttonsJSON != "" {
			kb = parseButtons(buttonsJSON)
			if kb == nil {
				return "Error: failed to parse buttons JSON"
			}
		}

		return SendTGMessageWithButtonsFn(target, text, kb)
	},
}

var SetBotDp = &ToolDef{
	Name:        "set_bot_dp",
	Description: "Set the bot/account profile picture. If replying to a photo, auto-uses that. Otherwise accepts a file path or image URL.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "image", Description: "Local file path or image URL. Omit to auto-use replied-to photo.", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		image := strings.TrimSpace(args["image"])

		if image == "" {
			if GetTelegramContextFn != nil && TGDownloadMediaFn != nil {
				ctx := GetTelegramContextFn(userID)
				if ctx != nil {
					replyMsgID, hasReply := ctx["reply_to_msg_id"]
					chatID, hasChat := ctx["telegram_id"]
					if hasReply && hasChat {
						msgID := int32(replyMsgID.(int64))
						peer := fmt.Sprintf("%d", chatID.(int64))
						localPath, err := TGDownloadMediaFn(peer, msgID, "")
						if err != nil {
							return fmt.Sprintf("Error downloading replied image: %v", err)
						}
						image = localPath
					}
				}
			}
			if image == "" {
				return "Error: no image provided and no replied-to message with media found"
			}
		}

		if SetBotDpFn == nil {
			return "Error: profile photo setter not initialized"
		}
		if result := SetBotDpFn(image); result != "" {
			return result
		}
		return "Profile photo updated successfully"
	},
}

var TGDownload = &ToolDef{
	Name:        "tg_download",
	Description: "Download media from a Telegram message by chat ID/username and message ID.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Description: "Chat ID (numeric), @username", Required: true},
		{Name: "message_id", Description: "The message ID containing the media", Required: true},
		{Name: "save_as", Description: "Optional local file path to save to", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		chatStr := strings.TrimSpace(args["chat_id"])
		msgIDStr := strings.TrimSpace(args["message_id"])
		savePath := strings.TrimSpace(args["save_as"])

		if chatStr == "" || msgIDStr == "" {
			return "Error: chat_id and message_id are required"
		}

		chatStr = resolveContextPeer(chatStr, userID)
		if chatStr == "" {
			return "Error: no current chat context"
		}

		var msgID int32
		if _, err := fmt.Sscanf(msgIDStr, "%d", &msgID); err != nil {
			return fmt.Sprintf("Error: message_id must be numeric. Got: %q", msgIDStr)
		}

		if TGDownloadMediaFn == nil {
			return "Error: Telegram download not initialized"
		}
		localPath, err := TGDownloadMediaFn(chatStr, msgID, savePath)
		if err != nil {
			return fmt.Sprintf("Error downloading: %v", err)
		}
		return fmt.Sprintf("Downloaded to: %s", localPath)
	},
}

var TGForwardMsg = &ToolDef{
	Name:        "tg_forward",
	Description: "Forward a message from one chat to another. Supports chat IDs and @usernames.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "from_chat_id", Description: "Source chat ID or @username", Required: true},
		{Name: "message_id", Description: "ID of message to forward", Required: true},
		{Name: "to_chat_id", Description: "Destination chat ID or @username", Required: true},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		fromStr := strings.TrimSpace(args["from_chat_id"])
		msgStr := strings.TrimSpace(args["message_id"])
		toStr := strings.TrimSpace(args["to_chat_id"])

		if fromStr == "" || msgStr == "" || toStr == "" {
			return "Error: from_chat_id, message_id, and to_chat_id are required"
		}

		var msgID int32
		if _, err := fmt.Sscanf(msgStr, "%d", &msgID); err != nil {
			return fmt.Sprintf("Error: message_id must be numeric. Got: %q", msgStr)
		}

		if TGForwardMsgFn == nil {
			return "Error: Telegram forward not initialized"
		}
		return TGForwardMsgFn(fromStr, msgID, toStr)
	},
}

var TGDeleteMsg = &ToolDef{
	Name:        "tg_delete_msg",
	Description: "Delete one or more messages from a chat. Supports chat IDs and @usernames.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Description: "Chat ID or @username", Required: true},
		{Name: "message_ids", Description: "Message IDs to delete, comma-separated (e.g. '123' or '123,124,125')", Required: true},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		chatStr := strings.TrimSpace(args["chat_id"])
		msgStr := strings.TrimSpace(args["message_ids"])

		if chatStr == "" || msgStr == "" {
			return "Error: chat_id and message_ids are required"
		}

		var msgIDs []int32
		for _, part := range strings.Split(msgStr, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			var id int32
			if _, err := fmt.Sscanf(part, "%d", &id); err != nil {
				return fmt.Sprintf("Error: invalid message_id %q", part)
			}
			msgIDs = append(msgIDs, id)
		}
		if len(msgIDs) == 0 {
			return "Error: no valid message IDs provided"
		}

		if TGDeleteMsgFn == nil {
			return "Error: Telegram delete not initialized"
		}
		return TGDeleteMsgFn(chatStr, msgIDs)
	},
}

var TGPinMsg = &ToolDef{
	Name:        "tg_pin_msg",
	Description: "Pin a message in a chat. Supports chat IDs and @usernames.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Description: "Chat ID or @username", Required: true},
		{Name: "message_id", Description: "Message ID to pin", Required: true},
		{Name: "silent", Description: "Pin silently without notifying (true/false, default false)", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		chatStr := strings.TrimSpace(args["chat_id"])
		msgStr := strings.TrimSpace(args["message_id"])
		silent := strings.EqualFold(strings.TrimSpace(args["silent"]), "true")

		if chatStr == "" || msgStr == "" {
			return "Error: chat_id and message_id are required"
		}

		var msgID int32
		if _, err := fmt.Sscanf(msgStr, "%d", &msgID); err != nil {
			return fmt.Sprintf("Error: message_id must be numeric. Got: %q", msgStr)
		}

		if TGPinMsgFn == nil {
			return "Error: Telegram pin not initialized"
		}
		return TGPinMsgFn(chatStr, msgID, silent)
	},
}

var TGUnpinMsg = &ToolDef{
	Name:        "tg_unpin_msg",
	Description: "Unpin a message from a chat. Supports chat IDs and @usernames.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Description: "Chat ID or @username", Required: true},
		{Name: "message_id", Description: "Message ID to unpin", Required: true},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		chatStr := strings.TrimSpace(args["chat_id"])
		msgStr := strings.TrimSpace(args["message_id"])

		if chatStr == "" || msgStr == "" {
			return "Error: chat_id and message_id are required"
		}

		var msgID int32
		if _, err := fmt.Sscanf(msgStr, "%d", &msgID); err != nil {
			return fmt.Sprintf("Error: message_id must be numeric. Got: %q", msgStr)
		}

		if TGUnpinMsgFn == nil {
			return "Error: Telegram unpin not initialized"
		}
		return TGUnpinMsgFn(chatStr, msgID)
	},
}

var TGGetChatInfo = &ToolDef{
	Name:        "tg_get_chat_info",
	Description: "Get info about a Telegram chat, group, channel, or user. Accepts numeric ID or @username.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "peer", Description: "Chat/user/channel ID (numeric) or @username", Required: true},
	},
	Execute: func(args map[string]string) string {
		peer := strings.TrimSpace(args["peer"])
		if peer == "" {
			return "Error: peer is required"
		}
		if TGGetChatInfoFn == nil {
			return "Error: Telegram info not initialized"
		}
		return TGGetChatInfoFn(peer)
	},
}

var TGReact = &ToolDef{
	Name:        "tg_react",
	Description: "React to a message with an emoji. Omit chat/message IDs to use context.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "emoji", Description: "Emoji reaction (e.g. '👍', '❤️', '🔥')", Required: true},
		{Name: "chat_id", Description: "Chat ID or @username. Omit for current chat.", Required: false},
		{Name: "message_id", Description: "Message ID. Omit to react to replied-to message.", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		emoji := strings.TrimSpace(args["emoji"])
		if emoji == "" {
			return "Error: emoji is required"
		}

		var chatStr string
		var msgID int32

		chatStr = strings.TrimSpace(args["chat_id"])
		if chatStr == "" && GetTelegramContextFn != nil {
			if ctx := GetTelegramContextFn(userID); ctx != nil {
				if v, ok := ctx["telegram_id"]; ok {
					chatStr = fmt.Sprintf("%d", v.(int64))
				}
			}
		}
		if chatStr == "" {
			return "Error: no chat context — specify chat_id"
		}

		msgStr := strings.TrimSpace(args["message_id"])
		if msgStr != "" {
			if _, err := fmt.Sscanf(msgStr, "%d", &msgID); err != nil {
				return fmt.Sprintf("Error: invalid message_id %q", msgStr)
			}
		} else if GetTelegramContextFn != nil {
			if ctx := GetTelegramContextFn(userID); ctx != nil {
				if v, ok := ctx["reply_to_msg_id"]; ok {
					msgID = int32(v.(int64))
				}
				if msgID == 0 {
					if v, ok := ctx["message_id"]; ok {
						msgID = int32(v.(int64))
					}
				}
			}
		}
		if msgID == 0 {
			return "Error: no message_id — reply to a message or specify message_id"
		}

		if TGReactFn == nil {
			return "Error: Telegram react not initialized"
		}
		return TGReactFn(chatStr, msgID, emoji)
	},
}

var TGGetReply = &ToolDef{
	Name:        "tg_get_reply",
	Description: "Fetch the full content of a replied-to message. Returns text, sender info, and media type.",
	Secure:      true,
	Args:        []ToolArg{},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		if GetTelegramContextFn == nil {
			return "Error: Telegram context not initialized"
		}
		ctx := GetTelegramContextFn(userID)
		if ctx == nil {
			return "Error: no Telegram context"
		}
		replyMsgVal, hasReply := ctx["reply_to_msg_id"]
		chatIDVal, hasChat := ctx["telegram_id"]
		if !hasReply || !hasChat {
			return "Error: not replying to any message"
		}
		msgID := int32(replyMsgVal.(int64))
		chatStr := fmt.Sprintf("%d", chatIDVal.(int64))
		if TGGetReplyFn == nil {
			return "Error: tg_get_reply not initialized"
		}
		return TGGetReplyFn(chatStr, msgID)
	},
}

var TGGetMembers = &ToolDef{
	Name:        "tg_get_members",
	Description: "List members of a group or channel. Supports chat IDs and @usernames.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Description: "Group/channel ID or @username", Required: true},
		{Name: "limit", Description: "Max members to return (default 50, max 200)", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		chatStr := strings.TrimSpace(args["chat_id"])
		if chatStr == "" {
			return "Error: chat_id is required"
		}

		limit := 50
		if lstr := strings.TrimSpace(args["limit"]); lstr != "" {
			if _, err := fmt.Sscanf(lstr, "%d", &limit); err != nil || limit <= 0 {
				limit = 50
			}
			if limit > 200 {
				limit = 200
			}
		}

		if TGGetMembersFn == nil {
			return "Error: tg_get_members not initialized"
		}
		return TGGetMembersFn(chatStr, limit)
	},
}

var TGBroadcast = &ToolDef{
	Name:        "tg_broadcast",
	Description: "Send the same message to multiple chats. Supports numeric IDs and @usernames.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_ids", Description: "Chat IDs/usernames, comma-separated (e.g. '123,-100987654,@username')", Required: true},
		{Name: "text", Description: "Message text to send (HTML formatting allowed)", Required: true},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		idsStr := strings.TrimSpace(args["chat_ids"])
		text := strings.TrimSpace(args["text"])
		if idsStr == "" || text == "" {
			return "Error: chat_ids and text are required"
		}

		var chatPeers []string
		for _, part := range strings.Split(idsStr, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				chatPeers = append(chatPeers, part)
			}
		}
		if len(chatPeers) == 0 {
			return "Error: no valid chat IDs provided"
		}

		if TGBroadcastFn == nil {
			return "Error: tg_broadcast not initialized"
		}
		return TGBroadcastFn(chatPeers, text)
	},
}

var TGGetMessage = &ToolDef{
	Name:        "tg_get_message",
	Description: "Fetch a single message by ID from a chat. Supports chat IDs and @usernames.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Description: "Chat ID or @username", Required: true},
		{Name: "message_id", Description: "Message ID to fetch", Required: true},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		chatStr := strings.TrimSpace(args["chat_id"])
		msgStr := strings.TrimSpace(args["message_id"])

		if chatStr == "" || msgStr == "" {
			return "Error: chat_id and message_id are required"
		}

		var msgID int32
		if _, err := fmt.Sscanf(msgStr, "%d", &msgID); err != nil {
			return fmt.Sprintf("Error: message_id must be numeric. Got: %q", msgStr)
		}

		if TGGetMessageFn == nil {
			return "Error: tg_get_message not initialized"
		}
		return TGGetMessageFn(chatStr, msgID)
	},
}

var TGEditMessage = &ToolDef{
	Name:        "tg_edit_message",
	Description: "Edit a previously sent message. Supports chat IDs and @usernames.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Description: "Chat ID or @username", Required: true},
		{Name: "message_id", Description: "Message ID to edit", Required: true},
		{Name: "text", Description: "New message text (HTML formatting allowed)", Required: true},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		chatStr := strings.TrimSpace(args["chat_id"])
		msgStr := strings.TrimSpace(args["message_id"])
		text := strings.TrimSpace(args["text"])

		if chatStr == "" || msgStr == "" || text == "" {
			return "Error: chat_id, message_id, and text are required"
		}

		var msgID int32
		if _, err := fmt.Sscanf(msgStr, "%d", &msgID); err != nil {
			return fmt.Sprintf("Error: message_id must be numeric. Got: %q", msgStr)
		}

		if TGEditMessageFn == nil {
			return "Error: tg_edit_message not initialized"
		}
		return TGEditMessageFn(chatStr, msgID, text)
	},
}

var TGCreateInvite = &ToolDef{
	Name:        "tg_create_invite",
	Description: "Create an invite link for a chat. Optionally set expiration and member limit.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Description: "Chat ID or @username", Required: true},
		{Name: "expire_date", Description: "Expiration timestamp (Unix), 0 for never (default 0)", Required: false},
		{Name: "member_limit", Description: "Max members allowed to join via link, 0 for unlimited (default 0)", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		chatStr := strings.TrimSpace(args["chat_id"])
		if chatStr == "" {
			return "Error: chat_id is required"
		}

		var expireDate, memberLimit int32
		if expStr := strings.TrimSpace(args["expire_date"]); expStr != "" {
			var e int64
			if _, err := fmt.Sscanf(expStr, "%d", &e); err == nil {
				expireDate = int32(e)
			}
		}
		if limStr := strings.TrimSpace(args["member_limit"]); limStr != "" {
			if _, err := fmt.Sscanf(limStr, "%d", &memberLimit); err == nil {
				memberLimit = int32(memberLimit)
			}
		}

		if TGCreateInviteFn == nil {
			return "Error: tg_create_invite not initialized"
		}
		return TGCreateInviteFn(chatStr, expireDate, memberLimit)
	},
}

var TGGetProfilePhotos = &ToolDef{
	Name:        "tg_get_profile_photos",
	Description: "Get profile photos of a user or channel. Supports IDs and @usernames.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "peer", Description: "User/channel ID or @username", Required: true},
		{Name: "limit", Description: "Max photos to return (default 10, max 100)", Required: false},
	},
	Execute: func(args map[string]string) string {
		peer := strings.TrimSpace(args["peer"])
		if peer == "" {
			return "Error: peer is required"
		}

		limit := 10
		if limStr := strings.TrimSpace(args["limit"]); limStr != "" {
			if _, err := fmt.Sscanf(limStr, "%d", &limit); err == nil && limit > 0 && limit <= 100 {
			} else {
				limit = 10
			}
		}

		if TGGetProfilePhotosFn == nil {
			return "Error: tg_get_profile_photos not initialized"
		}
		return TGGetProfilePhotosFn(peer, limit)
	},
}

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
		var rowButtons []telegram.KeyboardButton

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
			case "data":
				fallthrough
			default:
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

			rowButtons = append(rowButtons, btn)
		}

		if len(rowButtons) > 0 {
			kb.AddRow(rowButtons...)
		}
	}

	return kb.Build()
}
