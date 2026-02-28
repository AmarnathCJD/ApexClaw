package tools

import (
	"fmt"
	"log"
)

// Function pointers wired by core/register.go
var SetDeepWorkFn func(senderID string, maxSteps int, plan string) string
var SendProgressFn func(senderID string, percent int, message string, state string, detail string) (int64, error)

var DeepWork = &ToolDef{
	Name:        "deep_work",
	Description: "Enter deep work mode for complex multi-step tasks. Raises the iteration limit to allow extended execution (up to 50 steps). Call this FIRST when a task needs many sequential tool calls (deploying, installing, browser workflows, ordering, etc.).",
	Args: []ToolArg{
		{Name: "plan", Description: "Brief plan of steps you will execute (helps track progress)", Required: true},
		{Name: "max_steps", Description: "Estimated tool calls needed (default: 30, max: 50)", Required: false},
	},
	Sequential: true,
	ExecuteWithContext: func(args map[string]string, senderID string) string {
		plan := args["plan"]
		if plan == "" {
			return "Error: plan is required"
		}

		maxSteps := 30
		if ms := args["max_steps"]; ms != "" {
			fmt.Sscanf(ms, "%d", &maxSteps)
		}
		if maxSteps < 5 {
			maxSteps = 5
		}
		if maxSteps > 50 {
			maxSteps = 50
		}

		if SetDeepWorkFn == nil {
			return "Error: deep work not available"
		}

		result := SetDeepWorkFn(senderID, maxSteps, plan)
		log.Printf("[DEEP_WORK] activated for %s: max_steps=%d plan=%q", senderID, maxSteps, plan)
		return result
	},
}

var Progress = &ToolDef{
	Name:        "progress",
	Description: "Report progress on a multi-step task with rich updates. States: 'running', 'success', 'failure', 'retry'. Updates both Telegram and WebUI in real-time.",
	Args: []ToolArg{
		{Name: "message", Description: "Main status message (e.g. 'Vercel CLI installation')", Required: true},
		{Name: "percent", Description: "Completion percentage 0-100 (optional)", Required: false},
		{Name: "state", Description: "Status: 'running', 'success', 'failure', 'retry' (optional, default: 'running')", Required: false},
		{Name: "detail", Description: "Detailed output/error message (optional)", Required: false},
	},
	Sequential: true,
	ExecuteWithContext: func(args map[string]string, senderID string) string {
		message := args["message"]
		if message == "" {
			return "Error: message is required"
		}

		percent := 0
		if p := args["percent"]; p != "" {
			fmt.Sscanf(p, "%d", &percent)
		}
		if percent < 0 {
			percent = 0
		}
		if percent > 100 {
			percent = 100
		}

		state := args["state"]
		if state == "" {
			state = "running"
		}
		switch state {
		case "running", "success", "failure", "retry":
		default:
			state = "running"
		}

		detail := args["detail"]

		if SendProgressFn != nil {
			msgID, err := SendProgressFn(senderID, percent, message, state, detail)
			if err == nil {
				log.Printf("[PROGRESS] %s [%d%%] %s (state=%s, tg_msg=%d)", senderID, percent, message, state, msgID)
				return ""
			}
			log.Printf("[PROGRESS] %s: send failed: %v", senderID, err)
		}

		return ""
	},
}
