package tools

import (
	"fmt"
	"time"
)

var Datetime = &ToolDef{
	Name:        "datetime",
	Description: "Get the current date, time, day of week, and timezone. Optional tz arg (IANA name like 'America/New_York' or 'UTC').",
	Args: []ToolArg{
		{Name: "tz", Type: ArgString, Description: "Optional IANA timezone name. Defaults to server local.", Required: false},
	},
	Execute: func(args map[string]any) string {
		now := time.Now()
		if tz := String(args, "tz"); tz != "" {
			if loc, err := time.LoadLocation(tz); err == nil {
				now = now.In(loc)
			}
		}
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
