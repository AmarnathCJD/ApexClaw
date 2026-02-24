package tools

import (
	"fmt"
	"strings"
	"time"
)

var Datetime = &ToolDef{
	Name:        "datetime",
	Description: "Get the current date, time, day of week, and timezone",
	Args:        []ToolArg{},
	Execute: func(args map[string]string) string {
		now := time.Now()
		return fmt.Sprintf(
			"Date: %s\nTime: %s\nDay: %s\nTimezone: %s\nUnix: %d",
			now.Format("2006-01-02"),
			now.Format("15:04:05"),
			now.Weekday().String(),
			now.Format("MST"),
			now.Unix(),
		)
	},
}

var Timer = &ToolDef{
	Name:        "timer",
	Description: "Wait for a specified number of seconds (max 30)",
	Args: []ToolArg{
		{Name: "seconds", Description: "How many seconds to wait (max 30)", Required: true},
	},
	Execute: func(args map[string]string) string {
		secStr := args["seconds"]
		if secStr == "" {
			return "Error: seconds is required"
		}
		var sec int
		fmt.Sscanf(secStr, "%d", &sec)
		if sec <= 0 {
			return "Error: seconds must be positive"
		}
		if sec > 30 {
			sec = 30
		}
		time.Sleep(time.Duration(sec) * time.Second)
		return fmt.Sprintf("Waited %d second(s).", sec)
	},
}

var Echo = &ToolDef{
	Name:        "echo",
	Description: "Echo back the given text — useful for testing",
	Args: []ToolArg{
		{Name: "text", Description: "Text to echo back", Required: true},
	},
	Execute: func(args map[string]string) string {
		return strings.TrimSpace(args["text"])
	},
}
