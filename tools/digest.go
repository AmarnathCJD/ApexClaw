package tools

import (
	"fmt"
	"strings"
	"time"
)

var DailyDigest = &ToolDef{
	Name:        "daily_digest",
	Description: "Schedule a daily morning digest that auto-fetches news headlines, weather, and any notes/facts you've saved. Sends every day at the specified time.",
	Args: []ToolArg{
		{Name: "time", Description: "Time to send digest every day in HH:MM 24h IST format (e.g. '07:30')", Required: true},
		{Name: "city", Description: "City for weather in the digest (e.g. 'Mumbai')", Required: false},
		{Name: "topics", Description: "News topics to include, comma-separated (e.g. 'technology,crypto,india')", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		timeStr := strings.TrimSpace(args["time"])
		city := strings.TrimSpace(args["city"])
		topics := strings.TrimSpace(args["topics"])

		if timeStr == "" {
			return "Error: time is required (e.g. '07:30')"
		}

		var hour, min int
		if _, err := fmt.Sscanf(timeStr, "%d:%d", &hour, &min); err != nil || hour > 23 || min > 59 {
			return fmt.Sprintf("Error: invalid time %q — use HH:MM 24h format", timeStr)
		}

		ist := time.FixedZone("IST", 5*3600+30*60)
		now := time.Now().In(ist)

		next := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, ist)
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}

		var promptParts []string
		promptParts = append(promptParts, "Compose a concise morning digest. Include:")
		if city != "" {
			promptParts = append(promptParts, fmt.Sprintf("1. Current weather in %s (use weather tool)", city))
		} else {
			promptParts = append(promptParts, "1. General weather summary if location is known")
		}

		if topics != "" {
			topicList := strings.Join(strings.Split(topics, ","), ", ")
			promptParts = append(promptParts, fmt.Sprintf("2. Top 3-5 news headlines about: %s (use web_search)", topicList))
		} else {
			promptParts = append(promptParts, "2. Top 3-5 general news headlines (use web_search)")
		}

		promptParts = append(promptParts,
			"3. Any saved facts/notes relevant to today (use list_facts)",
			"4. A motivational thought or useful tip for the day",
			"Format nicely with HTML bold headers. Be concise.",
		)

		prompt := strings.Join(promptParts, "\n")

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

		ScheduleTaskFn("", "daily_digest", prompt, next.Format(time.RFC3339), "daily", userID, telegramID, 0, 0)

		return fmt.Sprintf(
			"Daily digest scheduled at %02d:%02d IST every day.\nFirst delivery: %s",
			hour, min, next.Format("02 Jan 2006 15:04 IST"),
		)
	},
}

var CronStatus = &ToolDef{
	Name:        "cron_status",
	Description: "Show all scheduled tasks with time remaining until next execution.",
	Args:        []ToolArg{},
	Execute: func(args map[string]string) string {
		if ListTasksFn == nil {
			return "Error: scheduler not initialized"
		}
		raw := ListTasksFn()
		if raw == "No scheduled tasks." {
			return raw
		}

		return raw
	},
}
