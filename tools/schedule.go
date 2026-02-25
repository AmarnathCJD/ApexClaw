package tools

import (
	"fmt"
	"time"
)

var ScheduleTaskFn func(id, label, prompt, runAt, repeat, ownerID string, telegramID, messageID, groupID int64)
var CancelTaskFn func(labelOrID string) bool
var ListTasksFn func() string
var GetTelegramContextFn func(userID string) map[string]interface{}

var ScheduleTask = &ToolDef{
	Name:        "schedule_task",
	Description: "Schedule a proactive task: the bot will run the given prompt at the specified time and send you the response automatically. Great for reminders, monitoring, periodic summaries.",
	Args: []ToolArg{
		{Name: "label", Description: "Short human-readable name for this task (e.g. 'morning_briefing')", Required: true},
		{Name: "prompt", Description: "The prompt or instruction the bot should run at the scheduled time (e.g. 'Check weather in Kochi and summarize')", Required: true},
		{Name: "run_at", Description: "When to first run, in ISO 8601 / RFC3339 format (e.g. '2026-02-25T08:00:00+05:30')", Required: true},
		{Name: "repeat", Description: "Repeat interval: 'once', 'minutely', 'hourly', 'daily', 'weekly', or 'every_N_minutes' / 'every_N_hours' / 'every_N_days'. Default: 'once'", Required: false},
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
			return fmt.Sprintf("Error: run_at must be in RFC3339 format (e.g. 2026-02-25T08:00:00+05:30). Got: %q", runAt)
		}
		if !runAtParsed.After(time.Now()) {
			ist := time.FixedZone("IST", 5*3600+30*60)
			return fmt.Sprintf("Error: run_at %q is in the past. Current time is %s. Recalculate and use a future timestamp.", runAt, time.Now().In(ist).Format(time.RFC3339))
		}

		if ScheduleTaskFn == nil {
			return "Error: scheduler not initialized"
		}

		var ownerID string
		var telegramID, messageID, groupID int64
		if GetTelegramContextFn != nil {
			ctx := GetTelegramContextFn(userID)
			if ctx != nil {
				if v, ok := ctx["owner_id"]; ok {
					ownerID = v.(string)
				}
				if v, ok := ctx["telegram_id"]; ok {
					telegramID = v.(int64)
				}
				if v, ok := ctx["message_id"]; ok {
					messageID = v.(int64)
				}
				if v, ok := ctx["group_id"]; ok {
					groupID = v.(int64)
				}
			}
		}

		ScheduleTaskFn("", label, prompt, runAt, repeat, ownerID, telegramID, messageID, groupID)
		repeatStr := "once"
		if repeat != "" {
			repeatStr = repeat
		}
		return fmt.Sprintf("Task %q scheduled for %s (repeat: %s)", label, runAt, repeatStr)
	},
}

var CancelTask = &ToolDef{
	Name:        "cancel_task",
	Description: "Cancel a previously scheduled task by its label name.",
	Args: []ToolArg{
		{Name: "label", Description: "The label of the task to cancel", Required: true},
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

var ListTasks = &ToolDef{
	Name:        "list_tasks",
	Description: "List all currently scheduled heartbeat tasks.",
	Args:        []ToolArg{},
	Execute: func(args map[string]string) string {
		if ListTasksFn == nil {
			return "Error: scheduler not initialized"
		}
		return ListTasksFn()
	},
}
