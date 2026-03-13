package tools

import (
	"fmt"
	"time"
)

var ScheduleTaskFn func(id, label, prompt, runAt, repeat, ownerID, onFailure, tags string, maxRuns int, telegramID, messageID, groupID int64)
var CancelTaskFn func(labelOrID string) bool
var PauseTaskFn func(labelOrID string) bool
var ResumeTaskFn func(labelOrID string) bool
var ListTasksFn func() string
var GetTelegramContextFn func(userID string) map[string]any

var ScheduleTask = &ToolDef{
	Name:        "schedule_task",
	Description: "Schedule a proactive task: the bot runs the given prompt at the specified time and delivers the result. Supports repeating, pausing, max-run limits, and failure handling.",
	Args: []ToolArg{
		{Name: "label", Description: "Short unique name for this task (e.g. 'morning_briefing')", Required: true},
		{Name: "prompt", Description: "Instruction the bot runs at the scheduled time (fetch live data — never embed current values)", Required: true},
		{Name: "run_at", Description: "When to first run, RFC3339 format (e.g. '2026-02-25T08:00:00+05:30')", Required: true},
		{Name: "repeat", Description: "once|minutely|hourly|daily|weekly|every_N_minutes|every_N_hours|every_N_days (default: once)", Required: false},
		{Name: "max_runs", Description: "Auto-cancel after this many executions (0 = unlimited)", Required: false},
		{Name: "on_failure", Description: "What to do if task fails: 'skip' (default), 'retry' (retry in 5 min), 'disable' (pause and notify)", Required: false},
		{Name: "tags", Description: "Optional comma-separated tags for grouping/filtering tasks", Required: false},
	},
	Execute: func(args map[string]string) string {
		return "Error: schedule_task requires context"
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		label := args["label"]
		prompt := args["prompt"]
		runAt := args["run_at"]
		repeat := args["repeat"]
		if label == "" || prompt == "" || runAt == "" {
			return "Error: label, prompt, and run_at are required"
		}
		if repeat == "" || repeat == "once" {
			repeat = ""
		}

		runAtParsed, err := time.Parse(time.RFC3339, runAt)
		if err != nil {
			return fmt.Sprintf("Error: run_at must be RFC3339 (e.g. 2026-02-25T08:00:00+05:30). Got: %q", runAt)
		}
		if !runAtParsed.After(time.Now()) {
			ist := time.FixedZone("IST", 5*3600+30*60)
			return fmt.Sprintf("Error: run_at %q is in the past. Current time: %s", runAt, time.Now().In(ist).Format(time.RFC3339))
		}

		if ScheduleTaskFn == nil {
			return "Error: scheduler not initialized"
		}

		maxRuns := 0
		if v := args["max_runs"]; v != "" {
			fmt.Sscanf(v, "%d", &maxRuns)
		}
		onFailure := args["on_failure"]
		tags := args["tags"]

		var ownerID string
		var telegramID, messageID, groupID int64
		if GetTelegramContextFn != nil {
			ctx := GetTelegramContextFn(userID)
			if ctx != nil {
				if v, ok := ctx["owner_id"]; ok {
					ownerID, _ = v.(string)
				}
				if v, ok := ctx["telegram_id"]; ok {
					telegramID, _ = v.(int64)
				}
				if v, ok := ctx["message_id"]; ok {
					messageID, _ = v.(int64)
				}
				if v, ok := ctx["group_id"]; ok {
					groupID, _ = v.(int64)
				}
				if ownerID == "" {
					if v, ok := ctx["sender_id"]; ok {
						ownerID, _ = v.(string)
					}
				}
				if telegramID == 0 {
					telegramID, _ = ctx["telegram_id"].(int64)
				}
			}
		}

		ScheduleTaskFn("", label, prompt, runAt, repeat, ownerID, onFailure, tags, maxRuns, telegramID, messageID, groupID)
		repeatStr := "once"
		if repeat != "" {
			repeatStr = repeat
		}
		extras := ""
		if maxRuns > 0 {
			extras += fmt.Sprintf(", max %d runs", maxRuns)
		}
		if onFailure != "" {
			extras += fmt.Sprintf(", on_failure=%s", onFailure)
		}
		return fmt.Sprintf("Task %q scheduled for %s (%s%s)", label, runAt, repeatStr, extras)
	},
}

var CancelTask = &ToolDef{
	Name:        "cancel_task",
	Description: "Permanently cancel and remove a scheduled task by label.",
	Args: []ToolArg{
		{Name: "label", Description: "The task label to cancel", Required: true},
	},
	Execute: func(args map[string]string) string {
		label := args["label"]
		if label == "" {
			return "Error: label is required"
		}
		if CancelTaskFn == nil {
			return "Error: scheduler not initialized"
		}
		if CancelTaskFn(label) {
			return fmt.Sprintf("Task %q cancelled.", label)
		}
		return fmt.Sprintf("No task found with label %q.", label)
	},
}

var PauseTask = &ToolDef{
	Name:        "pause_task",
	Description: "Pause a scheduled task (keeps it but skips execution until resumed).",
	Args: []ToolArg{
		{Name: "label", Description: "The task label to pause", Required: true},
	},
	Execute: func(args map[string]string) string {
		if PauseTaskFn == nil {
			return "Error: scheduler not initialized"
		}
		if PauseTaskFn(args["label"]) {
			return fmt.Sprintf("Task %q paused.", args["label"])
		}
		return fmt.Sprintf("No task found with label %q.", args["label"])
	},
}

var ResumeTask = &ToolDef{
	Name:        "resume_task",
	Description: "Resume a previously paused scheduled task.",
	Args: []ToolArg{
		{Name: "label", Description: "The task label to resume", Required: true},
	},
	Execute: func(args map[string]string) string {
		if ResumeTaskFn == nil {
			return "Error: scheduler not initialized"
		}
		if ResumeTaskFn(args["label"]) {
			return fmt.Sprintf("Task %q resumed.", args["label"])
		}
		return fmt.Sprintf("No task found with label %q.", args["label"])
	},
}

var ListTasks = &ToolDef{
	Name:        "list_tasks",
	Description: "List all scheduled tasks with their status, next run time, and run count.",
	Args:        []ToolArg{},
	Execute: func(args map[string]string) string {
		if ListTasksFn == nil {
			return "Error: scheduler not initialized"
		}
		return ListTasksFn()
	},
}
