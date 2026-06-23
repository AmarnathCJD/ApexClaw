package core

import (
	"strings"
	"sync"

	"apexclaw/tools"
)

// progressState tracks the single live progress message per Telegram user.
var (
	progressMu   sync.Mutex
	progressMsgs = make(map[string]*progressEntry)
)

type progressEntry struct {
	chatID  int64
	msgID   int32
	sending bool // true while first send is in-flight, prevents duplicate sends
}

// clearProgressMsg deletes the live progress message for a user (call after final reply sent).
func clearProgressMsg(senderID string) {
	progressMu.Lock()
	p, ok := progressMsgs[senderID]
	if ok {
		delete(progressMsgs, senderID)
	}
	progressMu.Unlock()
	if ok && p.msgID > 0 {
		tgDeleteRaw(p.chatID, p.msgID)
	}
}

func GetTaskContext() map[string]any {
	return nil
}

// RegisterBuiltinTools registers every tool in tools.All into the core
// registry and wires the function pointers that the tools package uses to
// reach Telegram / WhatsApp glue.
func RegisterBuiltinTools(reg *ToolRegistry) {
	for _, t := range tools.All {
		reg.Register(t)
	}

	tools.GetTelegramContextFn = getTelegramContext
	tools.SendTGFileFn = TGSendFile
	tools.SendTGMsgFn = TGSendMessage
	tools.SendTGPhotoFn = TGSendPhoto
	tools.SendTGPhotoURLFn = TGSendPhotoURL
	tools.SendTGAlbumFn = TGSendAlbum
	tools.SetBotDpFn = TGSetBotDp
	tools.TGDownloadMediaFn = TGDownloadMedia
	tools.TGGetFileFn = TGGetFile
	tools.TGGetChatInfoFn = TGGetChatInfo
	tools.TGResolvePeerFn = TGResolvePeer
	tools.TGForwardMsgFn = TGForwardMsg
	tools.TGDeleteMsgFn = TGDeleteMsg
	tools.TGPinMsgFn = TGPinMsg
	tools.TGUnpinMsgFn = TGUnpinMsg
	tools.TGReactFn = TGReact
	tools.TGGetMembersFn = TGGetMembers
	tools.TGBroadcastFn = TGBroadcast
	tools.TGGetMessageFn = TGGetMessage
	tools.TGEditMessageFn = TGEditMessage
	tools.SendTGMessageWithButtonsFn = TGSendMessageWithButtons
	tools.SendTGRichFn = TGSendRich
	tools.TGCreateInviteFn = TGCreateInvite
	tools.TGGetProfilePhotosFn = TGGetProfilePhotos
	tools.TGBanUserFn = TGBanUser
	tools.TGMuteUserFn = TGMuteUser
	tools.TGKickUserFn = TGKickUser
	tools.TGPromoteAdminFn = TGPromoteAdmin
	tools.TGDemoteAdminFn = TGDemoteAdmin
	tools.TGSendLocationFn = TGSendLocation

	tools.WASendMessageFn = WABotSendMessage
	tools.WASendFileFn = WABotSendFile
	tools.WAGetContactsFn = WABotGetContacts
	tools.WAGetGroupsFn = WABotGetGroups
	tools.WAOwnerIDFn = func() string { return Cfg.WAOwnerID }
}

// autoProgress is intentionally a no-op.
// The stream handler in telegram.go owns all Telegram output (working... / final result).
// Tool-level progress is tracked there via __TOOL_CALL: chunks, not here.
func autoProgress(senderID, toolName, argsJSON, state string) {
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func escapeJSON(s string) string {
	if len(s) > 200 {
		s = s[:200]
	}
	var result strings.Builder
	for _, c := range s {
		switch c {
		case '"':
			result.WriteString(`\"`)
		case '\\':
			result.WriteString(`\\`)
		case '\n':
			result.WriteString(`\n`)
		case '\r':
			result.WriteString(`\r`)
		case '\t':
			result.WriteString(`\t`)
		default:
			result.WriteString(string(c))
		}
	}
	return result.String()
}

func splitLines(text string, maxLines int) []string {
	var lines []string
	current := ""
	maxLen := 60

	for _, char := range text {
		if len(current) >= maxLen {
			lines = append(lines, current)
			current = ""
			if len(lines) >= maxLines {
				break
			}
		}
		current += string(char)
	}

	if current != "" && len(lines) < maxLines {
		lines = append(lines, current)
	}

	return lines
}
