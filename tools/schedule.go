package tools

import (
	"fmt"
	"strings"
	"time"
)

var (
	ScheduleTaskFn      func(label, prompt, runAt, repeat, cronExpr, ownerID, onFailure, tags string, maxRuns int, telegramID, messageID, groupID int64) error
	CancelTaskFn        func(labelOrID string) bool
	CancelTasksByTagFn  func(tag string) int
	PauseTaskFn         func(labelOrID string) bool
	ResumeTaskFn        func(labelOrID string) bool
	ListTasksFn         func() string
	ListTasksByTagFn    func(tagFilter string) string
	DryRunScheduleFn    func(label, prompt, runAt, repeat, cronExpr string, count int) (string, error)
)

var ScheduleTaskTool = &ToolDef{
	Name: "schedule_task",
	Description: "Schedule a task to run at a future time (one-shot or recurring). Use this for reminders, periodic checks, status pings. " +
		"Either set repeat (minutely/hourly/daily/weekly/monthly/every_N_unit) for simple recurrence, " +
		"or set cron_expr (5-field standard cron) for advanced schedules like '0 9 * * 1-5' (9am weekdays).",
	Secure:  true,
	Timeout: 5 * time.Second,
	Args: []ToolArg{
		{Name: "label", Type: ArgString, Required: true, Description: "Short unique name for the task. Used to cancel/pause it later. Existing tasks with the same label are updated in place."},
		{Name: "prompt", Type: ArgString, Required: true, Description: "Instruction the agent will execute when this task fires. Be concrete — the agent re-runs this prompt against live data."},
		{Name: "run_at", Type: ArgString, Required: false, Description: "RFC3339 timestamp for the first fire, e.g. 2026-12-31T15:04:05+05:30. For cron schedules this is computed automatically if omitted."},
		{Name: "repeat", Type: ArgString, Required: false, Description: "Recurrence keyword: minutely | hourly | daily | weekly | monthly | every_N_minutes | every_N_hours | every_N_days | every_N_weeks. Mutually exclusive with cron_expr."},
		{Name: "cron_expr", Type: ArgString, Required: false, Description: "Standard 5-field cron expression (minute hour dom month dow). Example: '*/15 * * * *' for every 15 minutes, '0 9 * * 1-5' for 9am Mon-Fri."},
		{Name: "max_runs", Type: ArgInt, Required: false, Description: "Cap total fires. 0 or omitted means unlimited (for one-shot tasks leave at 0 — they auto-clean after firing)."},
		{Name: "on_failure", Type: ArgString, Required: false, Description: "What to do on agent error: 'retry' (retry once in 5 min), 'skip' (ignore the failure and stay scheduled), or '' (default = drop the task)."},
		{Name: "tags", Type: ArgString, Required: false, Description: "Comma-separated tags. Used for bulk operations: cancel_tasks_by_tag, list filtered by tag."},
		{Name: "dry_run", Type: ArgBool, Required: false, Description: "Preview the next few firing times without actually scheduling. Useful to verify a cron expression."},
		{Name: "dry_run_count", Type: ArgInt, Required: false, Description: "How many future firings to preview when dry_run is true. Default 3, max 10."},
	},
	ExecuteWithContext: func(args map[string]any, senderID string) string {
		label := String(args, "label")
		prompt := String(args, "prompt")
		if label == "" {
			return "Error: label is required"
		}
		if prompt == "" {
			return "Error: prompt is required"
		}
		runAt := String(args, "run_at")
		repeat := String(args, "repeat")
		cronExpr := String(args, "cron_expr")
		if repeat != "" && cronExpr != "" {
			return "Error: cannot set both repeat and cron_expr — pick one"
		}

		if BoolOr(args, "dry_run", false) {
			if DryRunScheduleFn == nil {
				return "Error: dry-run not available in this build"
			}
			out, err := DryRunScheduleFn(label, prompt, runAt, repeat, cronExpr, IntOr(args, "dry_run_count", 3))
			if err != nil {
				return "Error: " + err.Error()
			}
			return out
		}

		if ScheduleTaskFn == nil {
			return "Error: scheduling not initialized"
		}
		maxRuns := IntOr(args, "max_runs", 0)
		onFailure := String(args, "on_failure")
		tags := String(args, "tags")

		ctx := getTelegramContextOrEmpty(senderID)
		telegramID := int64Of(ctx, "telegram_id")
		messageID := int64Of(ctx, "msg_id")
		groupID := int64Of(ctx, "group_id")

		if err := ScheduleTaskFn(label, prompt, runAt, repeat, cronExpr, senderID, onFailure, tags, maxRuns, telegramID, messageID, groupID); err != nil {
			return "Error: " + err.Error()
		}
		schedKind := repeat
		if cronExpr != "" {
			schedKind = "cron: " + cronExpr
		} else if schedKind == "" {
			schedKind = "once"
		}
		when := runAt
		if when == "" {
			when = "computed from cron expression"
		}
		return fmt.Sprintf("Scheduled %q (%s) → next fire %s", label, schedKind, when)
	},
}

var ListTasksTool = &ToolDef{
	Name:        "list_tasks",
	Description: "List all scheduled tasks (or only those matching a tag filter). Returns label, next-fire time, schedule type, run count, and last result.",
	Timeout:     5 * time.Second,
	MaxOutput:   24 * 1024,
	Args: []ToolArg{
		{Name: "tag", Type: ArgString, Required: false, Description: "Optional tag filter. Comma-separated to match any of the given tags."},
	},
	Execute: func(args map[string]any) string {
		tag := String(args, "tag")
		if tag != "" {
			if ListTasksByTagFn != nil {
				return ListTasksByTagFn(tag)
			}
			return "Error: filtered list not available"
		}
		if ListTasksFn == nil {
			return "Error: scheduling not initialized"
		}
		return ListTasksFn()
	},
}

var CancelTaskTool = &ToolDef{
	Name:        "cancel_task",
	Description: "Cancel a scheduled task by its label or ID. The task is removed entirely. Use cancel_tasks_by_tag for bulk removal.",
	Secure:      true,
	Timeout:     5 * time.Second,
	Args: []ToolArg{
		{Name: "label_or_id", Type: ArgString, Required: true, Description: "Label or numeric ID of the task to cancel."},
	},
	Execute: func(args map[string]any) string {
		key := String(args, "label_or_id")
		if key == "" {
			return "Error: label_or_id is required"
		}
		if CancelTaskFn == nil {
			return "Error: scheduling not initialized"
		}
		if !CancelTaskFn(key) {
			return fmt.Sprintf("No task found with label/ID %q", key)
		}
		return fmt.Sprintf("Cancelled task %q", key)
	},
}

var CancelTasksByTagTool = &ToolDef{
	Name:        "cancel_tasks_by_tag",
	Description: "Bulk-cancel every scheduled task whose tags contain the given substring. Useful for tearing down a whole flow at once.",
	Secure:      true,
	Timeout:     5 * time.Second,
	Args: []ToolArg{
		{Name: "tag", Type: ArgString, Required: true, Description: "Tag substring to match (case-insensitive)."},
	},
	Execute: func(args map[string]any) string {
		tag := String(args, "tag")
		if tag == "" {
			return "Error: tag is required"
		}
		if CancelTasksByTagFn == nil {
			return "Error: scheduling not initialized"
		}
		n := CancelTasksByTagFn(tag)
		if n == 0 {
			return fmt.Sprintf("No tasks matched tag %q", tag)
		}
		return fmt.Sprintf("Cancelled %d task(s) matching tag %q", n, tag)
	},
}

var PauseTaskTool = &ToolDef{
	Name:        "pause_task",
	Description: "Pause a scheduled task without removing it. The task stays in the queue but does NOT fire until resumed.",
	Secure:      true,
	Timeout:     5 * time.Second,
	Args: []ToolArg{
		{Name: "label_or_id", Type: ArgString, Required: true, Description: "Label or numeric ID of the task to pause."},
	},
	Execute: func(args map[string]any) string {
		key := String(args, "label_or_id")
		if key == "" {
			return "Error: label_or_id is required"
		}
		if PauseTaskFn == nil {
			return "Error: scheduling not initialized"
		}
		if !PauseTaskFn(key) {
			return fmt.Sprintf("No task found with label/ID %q", key)
		}
		return fmt.Sprintf("Paused task %q", key)
	},
}

var ResumeTaskTool = &ToolDef{
	Name:        "resume_task",
	Description: "Resume a previously-paused scheduled task. The task will fire at its next scheduled time.",
	Secure:      true,
	Timeout:     5 * time.Second,
	Args: []ToolArg{
		{Name: "label_or_id", Type: ArgString, Required: true, Description: "Label or numeric ID of the task to resume."},
	},
	Execute: func(args map[string]any) string {
		key := String(args, "label_or_id")
		if key == "" {
			return "Error: label_or_id is required"
		}
		if ResumeTaskFn == nil {
			return "Error: scheduling not initialized"
		}
		if !ResumeTaskFn(key) {
			return fmt.Sprintf("No task found with label/ID %q", key)
		}
		return fmt.Sprintf("Resumed task %q", key)
	},
}

func getTelegramContextOrEmpty(senderID string) map[string]any {
	if GetTelegramContextFn == nil {
		return nil
	}
	return GetTelegramContextFn(senderID)
}

func int64Of(m map[string]any, key string) int64 {
	if m == nil {
		return 0
	}
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	case string:
		s := strings.TrimSpace(t)
		var n int64
		fmt.Sscanf(s, "%d", &n)
		return n
	}
	return 0
}
