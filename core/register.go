package core

import (
	"encoding/json"
	"fmt"
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

func RegisterBuiltinTools(reg *ToolRegistry) {
	tools.ScheduleTaskFn = func(id, label, prompt, runAt, repeat, ownerID string, telegramID, messageID, groupID int64) {
		ScheduleTask(ScheduledTask{
			ID:         id,
			Label:      label,
			Prompt:     prompt,
			RunAt:      runAt,
			Repeat:     repeat,
			OwnerID:    ownerID,
			TelegramID: telegramID,
			MessageID:  messageID,
			GroupID:    groupID,
		})
	}
	tools.CancelTaskFn = CancelTask
	tools.ListTasksFn = ListHeartbeatTasks

	for _, t := range tools.All {
		reg.Register(&ToolDef{
			Name:               t.Name,
			Description:        t.Description,
			Args:               bridgeArgs(t.Args),
			Secure:             t.Secure,
			BlocksContext:      t.BlocksContext,
			Sequential:         t.Sequential,
			Execute:            t.Execute,
			ExecuteWithContext: t.ExecuteWithContext,
		})
	}

	tools.SetDeepWorkFn = func(senderID string, maxSteps int, plan string) string {
		agentSessions.RLock()
		var session *AgentSession
		for key, s := range agentSessions.m {
			if key == senderID || key == "web_"+senderID {
				session = s
				break
			}
		}
		agentSessions.RUnlock()

		if session == nil {
			return "Error: session not found"
		}
		session.SetDeepWork(maxSteps, plan)
		return fmt.Sprintf("Deep work activated! Plan: %s\nMax steps: %d\nYou now have extended iterations. Proceed with your plan.", plan, maxSteps)
	}

	tools.SendProgressFn = func(senderID string, percent int, message string, state string, detail string) (int64, error) {
		agentSessions.RLock()
		var session *AgentSession
		for key, s := range agentSessions.m {
			if key == senderID || key == "web_"+senderID {
				session = s
				break
			}
		}
		agentSessions.RUnlock()

		// Send WebUI progress
		if session != nil && session.streamCallback != nil {
			progressJSON := fmt.Sprintf(`{"message":"%s","percent":%d,"state":"%s","detail":"%s"}`,
				escapeJSON(message), percent, state, escapeJSON(detail))
			session.streamCallback(fmt.Sprintf("\x00PROGRESS:%s\x00", progressJSON))
		}

		// Edit-in-place Telegram progress message
		tgCtx := getTelegramContext(senderID)
		if tgCtx == nil {
			return 0, nil
		}
		chatID, ok := tgCtx["telegram_id"].(int64)
		if !ok {
			return 0, nil
		}

		var text strings.Builder
		fmt.Fprintf(&text, "[%s] <b>%s</b>", state, escapeHTML(message))
		if detail != "" && detail != "(no output)" {
			lines := splitLines(detail, 3)
			for _, line := range lines {
				fmt.Fprintf(&text, "\n<code>%s</code>", escapeHTML(line))
			}
		}

		progressMu.Lock()
		p := progressMsgs[senderID]
		if p != nil && (p.msgID > 0 || p.sending) && p.chatID == chatID {
			msgID := p.msgID
			progressMu.Unlock()
			if msgID > 0 {
				tgEditRaw(chatID, msgID, text.String())
			}
		} else {
			entry := &progressEntry{chatID: chatID, sending: true}
			progressMsgs[senderID] = entry
			progressMu.Unlock()

			newID := tgSendRaw(chatID, text.String())

			progressMu.Lock()
			if progressMsgs[senderID] == entry {
				entry.msgID = newID
				entry.sending = false
			}
			progressMu.Unlock()
		}

		return 0, nil
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
	tools.TGCreateInviteFn = TGCreateInvite
	tools.TGGetProfilePhotosFn = TGGetProfilePhotos
	tools.TGBanUserFn = TGBanUser
	tools.TGMuteUserFn = TGMuteUser
	tools.TGKickUserFn = TGKickUser
	tools.TGPromoteAdminFn = TGPromoteAdmin
	tools.TGDemoteAdminFn = TGDemoteAdmin
	tools.TGSendLocationFn = TGSendLocation
}

func bridgeArgs(args []tools.ToolArg) []ToolArg {
	out := make([]ToolArg, len(args))
	for i, a := range args {
		out[i] = ToolArg{
			Name:        a.Name,
			Description: a.Description,
			Required:    a.Required,
		}
	}
	return out
}

func repeatStr(s string, n int) string {
	var result strings.Builder
	for range n {
		result.WriteString(s)
	}
	return result.String()
}

// autoProgress fires a single-line progress update for automatic tool-execution tracking.
// Skips tools that manage progress themselves (progress, deep_work).
func autoProgress(senderID, toolName, argsJSON, state string) {
	if tools.SendProgressFn == nil {
		return
	}
	switch toolName {
	case "progress", "deep_work":
		return
	}

	// Extract a short detail label from the most meaningful arg
	detail := ""
	var args map[string]string
	if json.Unmarshal([]byte(argsJSON), &args) == nil {
		for _, key := range []string{"cmd", "url", "path", "query", "peer", "text", "file_path", "name"} {
			if v := args[key]; v != "" {
				if len(v) > 80 {
					v = v[:80] + "..."
				}
				detail = v
				break
			}
		}
	}

	tools.SendProgressFn(senderID, 0, toolName, state, detail)
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
