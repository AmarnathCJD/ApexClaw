package core

import (
	"os"
	"os/exec"
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
	tools.ScheduleTaskFn = func(id, label, prompt, runAt, repeat, ownerID, onFailure, tags string, maxRuns int, telegramID, messageID, groupID int64) {
		ScheduleTask(ScheduledTask{
			ID:         id,
			Label:      label,
			Prompt:     prompt,
			RunAt:      runAt,
			Repeat:     repeat,
			OwnerID:    ownerID,
			OnFailure:  onFailure,
			Tags:       tags,
			MaxRuns:    maxRuns,
			TelegramID: telegramID,
			MessageID:  messageID,
			GroupID:    groupID,
		})
	}
	tools.CancelTaskFn = CancelTask
	tools.PauseTaskFn = PauseTask
	tools.ResumeTaskFn = ResumeTask
	tools.ListTasksFn = ListHeartbeatTasks

	tools.GetDebugSession = func(senderID string) tools.DebugSessionInterface {
		agentSessions.RLock()
		session, ok := agentSessions.m[senderID]
		agentSessions.RUnlock()
		if ok {
			return session
		}
		return nil
	}

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

	tools.MonitorAlertFn = func(ownerID string, telegramID int64, label, url, diff string) {
		if heartbeatTGClient == nil || telegramID == 0 {
			return
		}
		msg := "<b>🔔 Monitor Alert: " + escapeHTML(label) + "</b>\n" +
			"URL: <code>" + escapeHTML(url) + "</code>\n" +
			"Change: " + escapeHTML(diff)
		heartbeatTGClient.SendMessage(telegramID, msg, nil)
	}

	tools.ScreenAnalyzeFn = func(imageB64, prompt string) string {
		return analyzeImageB64(imageB64, prompt)
	}

	tools.CustomToolRegisterFn = func(name, description, argsJSON, code, language string) {
		registerDynamicTool(reg, name, description, argsJSON, code, language)
	}
}

func registerDynamicTool(reg *ToolRegistry, name, description, argsJSON, code, language string) {
	def := &ToolDef{
		Name:        name,
		Description: description,
		Execute: func(args map[string]string) string {
			return runCustomTool(name, code, args)
		},
	}
	reg.Register(def)
}

func runCustomTool(name, code string, args map[string]string) string {
	argsJSON := "{}"
	if len(args) > 0 {
		parts := make([]string, 0, len(args))
		for k, v := range args {
			parts = append(parts, `"`+escapeJSON(k)+`":"`+escapeJSON(v)+`"`)
		}
		argsJSON = "{" + strings.Join(parts, ",") + "}"
	}
	runner := "import json\nargs = json.loads(r'''" + argsJSON + "''')\n" + code
	return execPythonCode(runner)
}

func execPythonCode(code string) string {
	f, err := os.CreateTemp("", "claw_dyn_*.py")
	if err != nil {
		return "Error: " + err.Error()
	}
	defer os.Remove(f.Name())
	f.WriteString(code)
	f.Close()
	out, err := exec.Command("python3", f.Name()).CombinedOutput()
	if err != nil {
		return "Error: " + err.Error() + "\n" + string(out)
	}
	return strings.TrimSpace(string(out))
}

func analyzeImageB64(imageB64, prompt string) string {
	return "(Vision analysis not yet integrated)"
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

// autoProgress is intentionally a no-op.
// The stream handler in telegram.go owns all Telegram output (working... / final result).
// Tool-level progress is tracked there via __TOOL_CALL: chunks, not here.
// The explicit `progress` tool (called by the AI) still works via SendProgressFn directly.
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
