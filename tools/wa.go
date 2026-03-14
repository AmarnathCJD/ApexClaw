package tools

import (
	"fmt"
	"strings"
)

// Function pointers wired in core/register.go
var WASendMessageFn func(jid, text string) string
var WASendFileFn func(jid, filePath, caption, mediaType string) string
var WAGetContactsFn func() string
var WAGetGroupsFn func() string
var WAOwnerIDFn func() string

func resolveWAJID(jid string) string {
	jid = strings.TrimSpace(jid)
	if jid == "" && WAOwnerIDFn != nil {
		jid = WAOwnerIDFn()
	}
	return jid
}

var WASendMessage = &ToolDef{
	Name:        "wa_send_message",
	Description: "Send a WhatsApp text message. jid: phone with country code e.g. '919876543210', or group JID. Omit jid to send to the WA owner.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "jid", Description: "Recipient phone number (digits only, country code) or group JID. Omit to send to WA owner.", Required: false},
		{Name: "text", Description: "Message text to send", Required: true},
	},
	ExecuteWithContext: func(args map[string]string, senderID string) string {
		jid := resolveWAJID(args["jid"])
		text := strings.TrimSpace(args["text"])
		if jid == "" {
			return "Error: jid required (no WA_OWNER_ID configured as fallback)"
		}
		if text == "" {
			return "Error: text is required"
		}
		if WASendMessageFn == nil {
			return "Error: WhatsApp not initialized"
		}
		return WASendMessageFn(jid, text)
	},
}

var WASendFile = &ToolDef{
	Name:        "wa_send_file",
	Description: "Send a file (image/video/audio/document) over WhatsApp. Omit jid to send to the WA owner.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "jid", Description: "Recipient phone number or group JID. Omit to send to WA owner.", Required: false},
		{Name: "path", Description: "Absolute local file path to send", Required: true},
		{Name: "caption", Description: "Optional caption for the file", Required: false},
		{Name: "type", Description: "Media type: image, video, audio, document (default: auto)", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, senderID string) string {
		jid := resolveWAJID(args["jid"])
		path := strings.TrimSpace(args["path"])
		if jid == "" {
			return "Error: jid required (no WA_OWNER_ID configured as fallback)"
		}
		if path == "" {
			return "Error: path is required"
		}
		if WASendFileFn == nil {
			return "Error: WhatsApp not initialized"
		}
		mediaType := strings.ToLower(strings.TrimSpace(args["type"]))
		if mediaType == "" {
			// Auto-detect from extension
			lower := strings.ToLower(path)
			switch {
			case strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg") ||
				strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".gif") ||
				strings.HasSuffix(lower, ".webp"):
				mediaType = "image"
			case strings.HasSuffix(lower, ".mp4") || strings.HasSuffix(lower, ".avi") ||
				strings.HasSuffix(lower, ".mkv") || strings.HasSuffix(lower, ".mov"):
				mediaType = "video"
			case strings.HasSuffix(lower, ".mp3") || strings.HasSuffix(lower, ".ogg") ||
				strings.HasSuffix(lower, ".wav") || strings.HasSuffix(lower, ".m4a"):
				mediaType = "audio"
			default:
				mediaType = "document"
			}
		}
		return WASendFileFn(jid, path, strings.TrimSpace(args["caption"]), mediaType)
	},
}

var WAGetContacts = &ToolDef{
	Name:        "wa_get_contacts",
	Description: "List saved WhatsApp contacts with their JIDs. Use JIDs from this list for wa_send_message.",
	Secure:      true,
	Args:        []ToolArg{},
	ExecuteWithContext: func(args map[string]string, senderID string) string {
		if WAGetContactsFn == nil {
			return "Error: WhatsApp not initialized"
		}
		return WAGetContactsFn()
	},
}

var WAGetGroups = &ToolDef{
	Name:        "wa_get_groups",
	Description: "List WhatsApp groups the bot is a member of, with their JIDs.",
	Secure:      true,
	Args:        []ToolArg{},
	ExecuteWithContext: func(args map[string]string, senderID string) string {
		if WAGetGroupsFn == nil {
			return "Error: WhatsApp not initialized"
		}
		return WAGetGroupsFn()
	},
}

// normalizeWAJID turns a plain phone number like "919876543210" into "919876543210@s.whatsapp.net"
func normalizeWAJID(jid string) string {
	jid = strings.TrimPrefix(jid, "+")
	if !strings.Contains(jid, "@") {
		return fmt.Sprintf("%s@s.whatsapp.net", jid)
	}
	return jid
}
