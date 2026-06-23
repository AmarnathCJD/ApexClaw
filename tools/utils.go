package tools

import (
	"fmt"
	"time"
)

var Datetime = &ToolDef{
	Name:        "datetime",
	Description: "Get the current date, time, day of week, and timezone",
	Args:        []ToolArg{},
	Execute: func(args map[string]any) string {
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
