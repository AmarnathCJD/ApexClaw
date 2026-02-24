package tools

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

var PomodoroFn func(sessions int, workMin, breakMin int, ownerID string, telegramID int64) string

var Pomodoro = &ToolDef{
	Name:        "pomodoro",
	Description: "Start a Pomodoro work/break timer chain. Schedules alternating work and break reminders via the heartbeat system.",
	Args: []ToolArg{
		{Name: "sessions", Description: "Number of work sessions (default 4)", Required: false},
		{Name: "work_min", Description: "Work session length in minutes (default 25)", Required: false},
		{Name: "break_min", Description: "Break length in minutes (default 5)", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		sessions := 4
		workMin := 25
		breakMin := 5

		if v := strings.TrimSpace(args["sessions"]); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 12 {
				sessions = n
			}
		}
		if v := strings.TrimSpace(args["work_min"]); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 120 {
				workMin = n
			}
		}
		if v := strings.TrimSpace(args["break_min"]); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 60 {
				breakMin = n
			}
		}

		var telegramID int64
		if GetTelegramContextFn != nil {
			ctx := GetTelegramContextFn(userID)
			if ctx != nil {
				if v, ok := ctx["telegram_id"]; ok {
					telegramID = v.(int64)
				}
			}
		}

		if ScheduleTaskFn == nil {
			return "Error: scheduler not initialized"
		}

		ist := time.FixedZone("IST", 5*3600+30*60)
		now := time.Now().In(ist)
		offset := time.Duration(0)

		var scheduled []string
		for i := 1; i <= sessions; i++ {
			offset += time.Duration(workMin) * time.Minute
			workAt := now.Add(offset).Format(time.RFC3339)
			workLabel := fmt.Sprintf("pomodoro_work_%d_end", i)
			workPrompt := fmt.Sprintf(
				"Pomodoro: Work session %d of %d is complete! Time for a %d-minute break. "+
					"Send an encouraging message telling the user their session is done and break starts now.",
				i, sessions, breakMin,
			)
			ScheduleTaskFn("", workLabel, workPrompt, workAt, "", userID, telegramID, 0, 0)
			scheduled = append(scheduled, fmt.Sprintf("Session %d ends at %s", i, now.Add(offset).Format("15:04")))

			if i < sessions {
				offset += time.Duration(breakMin) * time.Minute
				breakAt := now.Add(offset).Format(time.RFC3339)
				breakLabel := fmt.Sprintf("pomodoro_break_%d_end", i)
				breakPrompt := fmt.Sprintf(
					"Pomodoro: Break %d of %d is over! Time to start work session %d. "+
						"Send a motivating message telling the user break is done and to get back to work.",
					i, sessions-1, i+1,
				)
				ScheduleTaskFn("", breakLabel, breakPrompt, breakAt, "", userID, telegramID, 0, 0)
				scheduled = append(scheduled, fmt.Sprintf("Break %d ends at %s", i, now.Add(offset).Format("15:04")))
			}
		}

		totalMin := sessions*workMin + (sessions-1)*breakMin
		return fmt.Sprintf(
			"Pomodoro started: %d sessions × %dmin work + %dmin break = ~%dmin total.\n%s",
			sessions, workMin, breakMin, totalMin,
			strings.Join(scheduled, "\n"),
		)
	},
}
