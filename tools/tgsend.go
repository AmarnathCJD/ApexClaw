package tools

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

var SendTGFileFn func(chatID int64, filePath, caption string) string

var SendTGMsgFn func(chatID int64, text string) string

var SendTGPhotoURLFn func(chatID int64, photoURL, caption string) string

var SendTGAlbumURLsFn func(chatID int64, photoURLs []string, caption string) string

var SetBotDpFn func(filePathOrURL string) string

var TGSendFile = &ToolDef{
	Name:        "tg_send_file",
	Description: "Send a local file to a Telegram chat. If target is not specified, sends to the current chat. target can be a chat ID (numeric) or 'me' for your personal DM.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "path", Description: "Absolute or relative path of the file to send", Required: true},
		{Name: "caption", Description: "Optional caption for the file", Required: false},
		{Name: "target", Description: "Chat ID to send to (numeric), or 'me' for your DM. Omit to use the current chat.", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		path := strings.TrimSpace(args["path"])
		caption := strings.TrimSpace(args["caption"])
		target := strings.TrimSpace(args["target"])

		if path == "" {
			return "Error: path is required"
		}

		info, err := os.Stat(path)
		if err != nil {
			return fmt.Sprintf("Error: file not found: %s", path)
		}
		if info.IsDir() {
			return fmt.Sprintf("Error: %s is a directory, not a file", path)
		}

		var chatID int64
		switch target {
		case "", "current":
			if GetTelegramContextFn != nil {
				ctx := GetTelegramContextFn(userID)
				if ctx != nil {
					if v, ok := ctx["telegram_id"]; ok {
						chatID = v.(int64)
					}
				}
			}
			if chatID == 0 {
				return "Error: no current chat context — specify a target chat ID explicitly"
			}
		case "me":
			if GetTelegramContextFn != nil {
				ctx := GetTelegramContextFn(userID)
				if ctx != nil {
					if v, ok := ctx["owner_id"]; ok {
						ownerStr := v.(string)
						if id, err := strconv.ParseInt(ownerStr, 10, 64); err == nil {
							chatID = id
						}
					}
				}
			}
			if chatID == 0 {
				return "Error: could not resolve 'me' — owner ID not set"
			}
		default:
			id, err := strconv.ParseInt(target, 10, 64)
			if err != nil {
				return fmt.Sprintf("Error: target must be a numeric chat ID or 'me'. Got: %q", target)
			}
			chatID = id
		}

		if SendTGFileFn == nil {
			return "Error: Telegram file sender not initialized"
		}
		if result := SendTGFileFn(chatID, path, caption); result != "" {
			return result
		}
		return fmt.Sprintf("Sent %s to chat %d", path, chatID)
	},
}

var TGSendMessage = &ToolDef{
	Name:        "tg_send_message",
	Description: "Send a Telegram message to a specific chat. If target is not specified, sends to the current chat. target can be a chat ID or 'me'.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "text", Description: "The message text to send (HTML formatting allowed)", Required: true},
		{Name: "target", Description: "Chat ID to send to (numeric), or 'me' for your DM. Omit to use the current chat.", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		text := strings.TrimSpace(args["text"])
		target := strings.TrimSpace(args["target"])

		if text == "" {
			return "Error: text is required"
		}

		var chatID int64
		switch target {
		case "", "current":
			if GetTelegramContextFn != nil {
				ctx := GetTelegramContextFn(userID)
				if ctx != nil {
					if v, ok := ctx["telegram_id"]; ok {
						chatID = v.(int64)
					}
				}
			}
			if chatID == 0 {
				return "Error: no current chat context — specify a target chat ID explicitly"
			}
		case "me":
			if GetTelegramContextFn != nil {
				ctx := GetTelegramContextFn(userID)
				if ctx != nil {
					if v, ok := ctx["owner_id"]; ok {
						ownerStr := v.(string)
						if id, err := strconv.ParseInt(ownerStr, 10, 64); err == nil {
							chatID = id
						}
					}
				}
			}
			if chatID == 0 {
				return "Error: could not resolve 'me'"
			}
		default:
			id, err := strconv.ParseInt(target, 10, 64)
			if err != nil {
				return fmt.Sprintf("Error: target must be a numeric chat ID or 'me'. Got: %q", target)
			}
			chatID = id
		}

		if SendTGMsgFn == nil {
			return "Error: Telegram sender not initialized"
		}
		if result := SendTGMsgFn(chatID, text); result != "" {
			return result
		}
		return fmt.Sprintf("Sent message to chat %d", chatID)
	},
}

var SetBotDp = &ToolDef{
	Name:        "set_bot_dp",
	Description: "Set the bot/account profile picture (DP). If the user replied to a photo/image message, automatically uses that image. Otherwise accepts a local file path or image URL. Downloads and uploads as the new profile photo.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "image", Description: "Local file path or image URL. Omit to auto-use the replied-to message's photo.", Required: false},
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
						cid := chatID.(int64)
						localPath, err := TGDownloadMediaFn(cid, msgID, "")
						if err != nil {
							return fmt.Sprintf("Error downloading replied image: %v", err)
						}
						image = localPath
					}
				}
			}
			if image == "" {
				return "Error: no image provided and no replied-to message with media found. Reply to a photo or provide an image URL."
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
