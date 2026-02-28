package core

import (
	"fmt"
	"strings"

	"apexclaw/tools"
)

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

		// Send/Edit Telegram progress message
		ctx := getTelegramContext(senderID)
		if ctx != nil {
			if chatID, ok := ctx["telegram_id"].(int64); ok {
				// Build progress text without emoji
				var text strings.Builder
				fmt.Fprintf(&text, "[%s] %s", state, message)
				if detail != "" && detail != "(no output)" {
					lines := splitLines(detail, 4)
					for _, line := range lines {
						fmt.Fprintf(&text, "\n> %s", line)
					}
				}

				// Check if we have a progress message ID in context
				progressMsgID := int32(0)
				if msgID, ok := ctx["progress_message_id"].(int32); ok {
					progressMsgID = msgID
				}

				// If we have a message ID, edit it; otherwise send new message
				if progressMsgID > 0 {
					_ = TGEditMessage(fmt.Sprintf("%d", chatID), progressMsgID, text.String())
				} else {
					// Send new message and store its ID for future edits
					_ = TGSendMessage(fmt.Sprintf("%d", chatID), text.String())
					// Note: TGSendMessage returns string, not message ID
					// Progress message ID will be tracked differently if needed
				}
			}
		}

		return 0, nil
	}

	tools.GetTelegramContextFn = getTelegramContext
	tools.SendTGFileFn = TGSendFile
	tools.SendTGMsgFn = TGSendMessage
	tools.SendTGPhotoFn = TGSendPhoto
	tools.SendTGPhotoURLFn = TGSendPhotoURL
	tools.SendTGAlbumURLsFn = TGSendAlbumURLs
	tools.SetBotDpFn = TGSetBotDp
	tools.TGDownloadMediaFn = TGDownloadMedia
	tools.TGGetChatInfoFn = TGGetChatInfo
	tools.TGResolvePeerFn = TGResolvePeer
	tools.TGForwardMsgFn = TGForwardMsg
	tools.TGDeleteMsgFn = TGDeleteMsg
	tools.TGPinMsgFn = TGPinMsg
	tools.TGUnpinMsgFn = TGUnpinMsg
	tools.TGReactFn = TGReact
	tools.TGGetReplyFn = TGGetReply
	tools.TGGetMembersFn = TGGetMembers
	tools.TGBroadcastFn = TGBroadcast
	tools.TGGetMessageFn = TGGetMessage
	tools.TGEditMessageFn = TGEditMessage
	tools.SendTGMessageWithButtonsFn = TGSendMessageWithButtons
	tools.TGCreateInviteFn = TGCreateInvite
	tools.TGGetProfilePhotosFn = TGGetProfilePhotos
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
