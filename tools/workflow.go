package tools

import (
	"fmt"
	"log"
)

// Function pointers wired by core/register.go
var SetDeepWorkFn func(senderID string, maxSteps int, plan string) string
var SendProgressFn func(senderID string, status string)

var DeepWork = &ToolDef{
	Name:        "deep_work",
	Description: "Enter deep work mode for complex multi-step tasks. Raises the iteration limit to allow extended execution (up to 50 steps). Call this FIRST when a task needs many sequential tool calls (deploying, installing, browser workflows, ordering, etc.).",
	Args: []ToolArg{
		{Name: "plan", Description: "Brief plan of steps you will execute (helps track progress)", Required: true},
		{Name: "max_steps", Description: "Estimated tool calls needed (default: 30, max: 50)", Required: false},
	},
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
	Description: "Report progress on a multi-step task to the user in real-time. Use during deep_work to keep them informed.",
	Args: []ToolArg{
		{Name: "status", Description: "Current status message (e.g. 'Step 2/5: Installing Vercel CLI...')", Required: true},
		{Name: "percent", Description: "Estimated completion percentage 0-100 (optional)", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, senderID string) string {
		status := args["status"]
		if status == "" {
			return "Error: status is required"
		}

		if percent := args["percent"]; percent != "" {
			status = fmt.Sprintf("[%s%%] %s", percent, status)
		}

		if SendProgressFn != nil {
			SendProgressFn(senderID, status)
		}

		log.Printf("[PROGRESS] %s: %s", senderID, status)
		return fmt.Sprintf("Progress reported: %s", status)
	},
}
