package tools

import (
	"fmt"
	"strings"
)

var TGDownloadMediaFn func(chatID int64, messageID int32, savePath string) (string, error)

var TGGetChatInfoFn func(peer string) string

var TGForwardMsgFn func(fromChatID int64, msgID int32, toChatID int64) string

var TGDeleteMsgFn func(chatID int64, msgIDs []int32) string

var TGPinMsgFn func(chatID int64, msgID int32, silent bool) string

var TGReactFn func(chatID int64, msgID int32, emoji string) string

var TGGetReplyFn func(chatID int64, msgID int32) string

var TGGetMembersFn func(chatID int64, limit int) string

var TGBroadcastFn func(chatIDs []int64, text string) string

var TGDownload = &ToolDef{
	Name:        "tg_download",
	Description: "Download media/file from a Telegram message by its chat_id and message_id. Returns the local file path. Use this to save photos, documents, audio etc. sent in Telegram.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Description: "The Telegram chat ID containing the message", Required: true},
		{Name: "message_id", Description: "The message ID that contains the media to download", Required: true},
		{Name: "save_as", Description: "Optional local file path to save to (e.g. '/tmp/file.pdf'). Omit to auto-name.", Required: false},
	},
	Execute: func(args map[string]string) string {
		chatIDStr := strings.TrimSpace(args["chat_id"])
		msgIDStr := strings.TrimSpace(args["message_id"])
		savePath := strings.TrimSpace(args["save_as"])

		if chatIDStr == "" || msgIDStr == "" {
			return "Error: chat_id and message_id are required"
		}
		var chatID int64
		if _, err := fmt.Sscanf(chatIDStr, "%d", &chatID); err != nil {
			return fmt.Sprintf("Error: chat_id must be numeric. Got: %q", chatIDStr)
		}
		var msgID int32
		if _, err := fmt.Sscanf(msgIDStr, "%d", &msgID); err != nil {
			return fmt.Sprintf("Error: message_id must be numeric. Got: %q", msgIDStr)
		}
		if TGDownloadMediaFn == nil {
			return "Error: Telegram download not initialized"
		}
		localPath, err := TGDownloadMediaFn(chatID, msgID, savePath)
		if err != nil {
			return fmt.Sprintf("Error downloading: %v", err)
		}
		return fmt.Sprintf("Downloaded to: %s", localPath)
	},
}

var TGForwardMsg = &ToolDef{
	Name:        "tg_forward",
	Description: "Forward a Telegram message from one chat to another chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "from_chat_id", Description: "Chat ID where the original message is", Required: true},
		{Name: "message_id", Description: "ID of the message to forward", Required: true},
		{Name: "to_chat_id", Description: "Destination chat ID to forward the message to", Required: true},
	},
	Execute: func(args map[string]string) string {
		fromStr := strings.TrimSpace(args["from_chat_id"])
		msgStr := strings.TrimSpace(args["message_id"])
		toStr := strings.TrimSpace(args["to_chat_id"])

		if fromStr == "" || msgStr == "" || toStr == "" {
			return "Error: from_chat_id, message_id, and to_chat_id are required"
		}
		var fromID, toID int64
		var msgID int32
		if _, err := fmt.Sscanf(fromStr, "%d", &fromID); err != nil {
			return fmt.Sprintf("Error: from_chat_id must be numeric. Got: %q", fromStr)
		}
		if _, err := fmt.Sscanf(msgStr, "%d", &msgID); err != nil {
			return fmt.Sprintf("Error: message_id must be numeric. Got: %q", msgStr)
		}
		if _, err := fmt.Sscanf(toStr, "%d", &toID); err != nil {
			return fmt.Sprintf("Error: to_chat_id must be numeric. Got: %q", toStr)
		}
		if TGForwardMsgFn == nil {
			return "Error: Telegram forward not initialized"
		}
		return TGForwardMsgFn(fromID, msgID, toID)
	},
}

var TGDeleteMsg = &ToolDef{
	Name:        "tg_delete_msg",
	Description: "Delete one or more Telegram messages by message ID in a given chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Description: "Chat ID where the messages are", Required: true},
		{Name: "message_ids", Description: "Message ID(s) to delete, comma-separated (e.g. '123' or '123,124,125')", Required: true},
	},
	Execute: func(args map[string]string) string {
		chatStr := strings.TrimSpace(args["chat_id"])
		msgStr := strings.TrimSpace(args["message_ids"])

		if chatStr == "" || msgStr == "" {
			return "Error: chat_id and message_ids are required"
		}
		var chatID int64
		if _, err := fmt.Sscanf(chatStr, "%d", &chatID); err != nil {
			return fmt.Sprintf("Error: chat_id must be numeric. Got: %q", chatStr)
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
		return TGDeleteMsgFn(chatID, msgIDs)
	},
}

var TGPinMsg = &ToolDef{
	Name:        "tg_pin_msg",
	Description: "Pin a message in a Telegram chat/group/channel.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Description: "Chat ID where the message is", Required: true},
		{Name: "message_id", Description: "Message ID to pin", Required: true},
		{Name: "silent", Description: "Pin silently without notifying members (true/false, default false)", Required: false},
	},
	Execute: func(args map[string]string) string {
		chatStr := strings.TrimSpace(args["chat_id"])
		msgStr := strings.TrimSpace(args["message_id"])
		silent := strings.EqualFold(strings.TrimSpace(args["silent"]), "true")

		if chatStr == "" || msgStr == "" {
			return "Error: chat_id and message_id are required"
		}
		var chatID int64
		var msgID int32
		if _, err := fmt.Sscanf(chatStr, "%d", &chatID); err != nil {
			return fmt.Sprintf("Error: chat_id must be numeric. Got: %q", chatStr)
		}
		if _, err := fmt.Sscanf(msgStr, "%d", &msgID); err != nil {
			return fmt.Sprintf("Error: message_id must be numeric. Got: %q", msgStr)
		}
		if TGPinMsgFn == nil {
			return "Error: Telegram pin not initialized"
		}
		return TGPinMsgFn(chatID, msgID, silent)
	},
}

var TGGetChatInfo = &ToolDef{
	Name:        "tg_get_chat_info",
	Description: "Get info about a Telegram chat, group, channel, or user. Accepts a numeric chat ID (e.g. -100123456789) or a @username (e.g. '@durov'). Returns name, username, type, member count.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "peer", Description: "Numeric Telegram chat/user/channel ID, or @username (e.g. '@telegram')", Required: true},
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
	Description: "React to a Telegram message with an emoji. If message_id is omitted, reacts to the replied-to message in the current chat.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "emoji", Description: "Emoji reaction to send (e.g. '👍', '❤️', '🔥', '😂')", Required: true},
		{Name: "chat_id", Description: "Chat ID of the message to react to. Omit to use current chat.", Required: false},
		{Name: "message_id", Description: "Message ID to react to. Omit to react to the replied-to message.", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		emoji := strings.TrimSpace(args["emoji"])
		if emoji == "" {
			return "Error: emoji is required"
		}

		var chatID int64
		var msgID int32

		chatStr := strings.TrimSpace(args["chat_id"])
		msgStr := strings.TrimSpace(args["message_id"])

		if chatStr != "" {
			if _, err := fmt.Sscanf(chatStr, "%d", &chatID); err != nil {
				return fmt.Sprintf("Error: invalid chat_id %q", chatStr)
			}
		} else if GetTelegramContextFn != nil {
			if ctx := GetTelegramContextFn(userID); ctx != nil {
				if v, ok := ctx["telegram_id"]; ok {
					chatID = v.(int64)
				}
			}
		}
		if chatID == 0 {
			return "Error: no chat context — specify chat_id"
		}

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
		return TGReactFn(chatID, msgID, emoji)
	},
}

var TGGetReply = &ToolDef{
	Name:        "tg_get_reply",
	Description: "Fetch the full content of the message the user replied to. Returns its text, sender info, and media type if any. Useful when the user says 'translate this', 'summarize this', etc. about a replied-to message.",
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
		chatID := chatIDVal.(int64)
		if TGGetReplyFn == nil {
			return "Error: tg_get_reply not initialized"
		}
		return TGGetReplyFn(chatID, msgID)
	},
}

var TGGetMembers = &ToolDef{
	Name:        "tg_get_members",
	Description: "List members of a Telegram group or channel. Returns usernames, names, and roles.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_id", Description: "Numeric group/channel ID (e.g. -100123456789)", Required: true},
		{Name: "limit", Description: "Max number of members to return (default 50, max 200)", Required: false},
	},
	Execute: func(args map[string]string) string {
		chatStr := strings.TrimSpace(args["chat_id"])
		if chatStr == "" {
			return "Error: chat_id is required"
		}
		var chatID int64
		if _, err := fmt.Sscanf(chatStr, "%d", &chatID); err != nil {
			return fmt.Sprintf("Error: chat_id must be numeric. Got: %q", chatStr)
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
		return TGGetMembersFn(chatID, limit)
	},
}

var TGBroadcast = &ToolDef{
	Name:        "tg_broadcast",
	Description: "Send the same message to multiple Telegram chats at once. Useful for announcements.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "chat_ids", Description: "Comma-separated list of chat IDs to send to (e.g. '123456,-100987654')", Required: true},
		{Name: "text", Description: "Message text to send to all chats (HTML formatting allowed)", Required: true},
	},
	Execute: func(args map[string]string) string {
		idsStr := strings.TrimSpace(args["chat_ids"])
		text := strings.TrimSpace(args["text"])
		if idsStr == "" || text == "" {
			return "Error: chat_ids and text are required"
		}
		var chatIDs []int64
		for _, part := range strings.Split(idsStr, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			var id int64
			if _, err := fmt.Sscanf(part, "%d", &id); err != nil {
				return fmt.Sprintf("Error: invalid chat_id %q", part)
			}
			chatIDs = append(chatIDs, id)
		}
		if len(chatIDs) == 0 {
			return "Error: no valid chat IDs provided"
		}
		if TGBroadcastFn == nil {
			return "Error: tg_broadcast not initialized"
		}
		return TGBroadcastFn(chatIDs, text)
	},
}
