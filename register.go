package main

import "apexclaw/tools"

func GetTaskContext() map[string]interface{} {
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
			Execute:            t.Execute,
			ExecuteWithContext: t.ExecuteWithContext,
		})
	}

	tools.GetTelegramContextFn = getTelegramContext
	tools.SendTGFileFn = TGSendFile
	tools.SendTGMsgFn = TGSendMessage
	tools.TGDownloadMediaFn = TGDownloadMedia
	tools.TGGetChatInfoFn = func(peer string) string { return TGGetChatInfo(peer) }
	tools.TGForwardMsgFn = TGForwardMsg
	tools.TGDeleteMsgFn = TGDeleteMsg
	tools.TGPinMsgFn = TGPinMsg
	tools.SendTGPhotoURLFn = TGSendPhotoURL
	tools.SendTGAlbumURLsFn = TGSendAlbumURLs
	tools.SetBotDpFn = TGSetBotDp
	tools.TGReactFn = TGReact
	tools.TGGetReplyFn = TGGetReply
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
