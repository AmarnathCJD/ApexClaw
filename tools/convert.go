package tools

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

var UnitConvert = &ToolDef{
	Name:        "unit_convert",
	Description: "Convert between units: length (km/m/cm/mm/mile/yard/foot/inch), weight (kg/g/lb/oz/ton), temperature (C/F/K), speed (kmh/mph/ms/knot), data (B/KB/MB/GB/TB), area (m2/ft2/acre/hectare), volume (L/ml/gal/fl_oz).",
	Args: []ToolArg{
		{Name: "value", Description: "Numeric value to convert", Required: true},
		{Name: "from", Description: "Source unit (e.g. 'km', 'kg', 'C', 'mph', 'GB')", Required: true},
		{Name: "to", Description: "Target unit (e.g. 'mile', 'lb', 'F', 'kmh', 'MB')", Required: true},
	},
	Execute: func(args map[string]string) string {
		valStr := strings.TrimSpace(args["value"])
		from := strings.ToLower(strings.TrimSpace(args["from"]))
		to := strings.ToLower(strings.TrimSpace(args["to"]))

		val, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			return fmt.Sprintf("Error: invalid value %q", valStr)
		}

		result, err := convertUnit(val, from, to)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}

		if math.Abs(result) >= 0.001 && math.Abs(result) < 1e9 {
			return fmt.Sprintf("%s %s = %.6g %s", valStr, args["from"], result, args["to"])
		}
		return fmt.Sprintf("%s %s = %g %s", valStr, args["from"], result, args["to"])
	},
}

var unitToBase = map[string]struct {
	factor   float64
	category string
}{

	"km": {1000, "length"}, "m": {1, "length"}, "cm": {0.01, "length"},
	"mm": {0.001, "length"}, "mile": {1609.344, "length"}, "miles": {1609.344, "length"},
	"yard": {0.9144, "length"}, "yd": {0.9144, "length"},
	"foot": {0.3048, "length"}, "feet": {0.3048, "length"}, "ft": {0.3048, "length"},
	"inch": {0.0254, "length"}, "in": {0.0254, "length"}, "nm": {1852, "length"},

	"kg": {1000, "weight"}, "g": {1, "weight"}, "mg": {0.001, "weight"},
	"lb": {453.592, "weight"}, "lbs": {453.592, "weight"},
	"oz": {28.3495, "weight"}, "ton": {1e6, "weight"}, "tonne": {1e6, "weight"},

	"kmh": {1 / 3.6, "speed"}, "kph": {1 / 3.6, "speed"}, "km/h": {1 / 3.6, "speed"},
	"mph": {0.44704, "speed"}, "ms": {1, "speed"}, "m/s": {1, "speed"},
	"knot": {0.514444, "speed"}, "knots": {0.514444, "speed"},

	"b": {1, "data"}, "kb": {1024, "data"}, "mb": {1024 * 1024, "data"},
	"gb": {1024 * 1024 * 1024, "data"}, "tb": {1024 * 1024 * 1024 * 1024, "data"},
	"pb": {1024 * 1024 * 1024 * 1024 * 1024, "data"},

	"m2": {1, "area"}, "km2": {1e6, "area"}, "cm2": {0.0001, "area"},
	"ft2": {0.092903, "area"}, "acre": {4046.86, "area"}, "hectare": {10000, "area"}, "ha": {10000, "area"},

	"l": {1, "volume"}, "ml": {0.001, "volume"}, "cl": {0.01, "volume"},
	"gal": {3.78541, "volume"}, "gallon": {3.78541, "volume"},
	"fl_oz": {0.0295735, "volume"}, "cup": {0.236588, "volume"},
	"pint": {0.473176, "volume"}, "qt": {0.946353, "volume"},
}

func convertUnit(val float64, from, to string) (float64, error) {

	if isTemp(from) || isTemp(to) {
		return convertTemp(val, from, to)
	}

	fromInfo, ok1 := unitToBase[from]
	toInfo, ok2 := unitToBase[to]
	if !ok1 {
		return 0, fmt.Errorf("unknown unit %q", from)
	}
	if !ok2 {
		return 0, fmt.Errorf("unknown unit %q", to)
	}
	if fromInfo.category != toInfo.category {
		return 0, fmt.Errorf("cannot convert %s (%s) to %s (%s)", from, fromInfo.category, to, toInfo.category)
	}
	base := val * fromInfo.factor
	return base / toInfo.factor, nil
}

func isTemp(u string) bool {
	return u == "c" || u == "f" || u == "k" || u == "°c" || u == "°f"
}

func convertTemp(val float64, from, to string) (float64, error) {

	var celsius float64
	switch from {
	case "c", "°c":
		celsius = val
	case "f", "°f":
		celsius = (val - 32) * 5 / 9
	case "k":
		celsius = val - 273.15
	default:
		return 0, fmt.Errorf("unknown temperature unit %q", from)
	}
	switch to {
	case "c", "°c":
		return celsius, nil
	case "f", "°f":
		return celsius*9/5 + 32, nil
	case "k":
		return celsius + 273.15, nil
	default:
		return 0, fmt.Errorf("unknown temperature unit %q", to)
	}
}

var TimezoneConvert = &ToolDef{
	Name:        "timezone_convert",
	Description: "Convert a date/time from one timezone to another. Supports timezone names like 'Asia/Kolkata', 'America/New_York', 'Europe/London', 'UTC', 'IST', 'EST', 'PST', etc.",
	Args: []ToolArg{
		{Name: "time", Description: "Time to convert (e.g. '2026-02-25 14:30', '15:45', 'now'). Defaults to now.", Required: false},
		{Name: "from", Description: "Source timezone (e.g. 'Asia/Kolkata', 'UTC', 'IST')", Required: true},
		{Name: "to", Description: "Target timezone (e.g. 'America/New_York', 'Europe/London')", Required: true},
	},
	Execute: func(args map[string]string) string {
		timeStr := strings.TrimSpace(args["time"])
		fromZone := strings.TrimSpace(args["from"])
		toZone := strings.TrimSpace(args["to"])

		fromZone = resolveTimezone(fromZone)
		toZone = resolveTimezone(toZone)

		fromLoc, err := time.LoadLocation(fromZone)
		if err != nil {
			return fmt.Sprintf("Error: unknown source timezone %q: %v", fromZone, err)
		}
		toLoc, err := time.LoadLocation(toZone)
		if err != nil {
			return fmt.Sprintf("Error: unknown target timezone %q: %v", toZone, err)
		}

		var t time.Time
		if timeStr == "" || strings.EqualFold(timeStr, "now") {
			t = time.Now().In(fromLoc)
		} else {

			formats := []string{
				"2006-01-02 15:04:05",
				"2006-01-02 15:04",
				"2006-01-02T15:04:05",
				"2006-01-02T15:04",
				"02 Jan 2006 15:04",
				"15:04:05",
				"15:04",
			}
			parsed := false
			for _, f := range formats {

				if !strings.Contains(f, "2006") {
					today := time.Now().In(fromLoc).Format("2006-01-02")
					t, err = time.ParseInLocation("2006-01-02 "+f, today+" "+timeStr, fromLoc)
				} else {
					t, err = time.ParseInLocation(f, timeStr, fromLoc)
				}
				if err == nil {
					parsed = true
					break
				}
			}
			if !parsed {
				return fmt.Sprintf("Error: cannot parse time %q — use format like '2026-02-25 14:30' or '15:45'", timeStr)
			}
		}

		converted := t.In(toLoc)
		return fmt.Sprintf(
			"%s → %s\n%s  =  %s",
			args["from"], args["to"],
			t.Format("02 Jan 2006 15:04:05 MST"),
			converted.Format("02 Jan 2006 15:04:05 MST"),
		)
	},
}

func resolveTimezone(tz string) string {
	aliases := map[string]string{
		"IST":  "Asia/Kolkata",
		"EST":  "America/New_York",
		"EDT":  "America/New_York",
		"CST":  "America/Chicago",
		"CDT":  "America/Chicago",
		"MST":  "America/Denver",
		"MDT":  "America/Denver",
		"PST":  "America/Los_Angeles",
		"PDT":  "America/Los_Angeles",
		"GMT":  "UTC",
		"BST":  "Europe/London",
		"CET":  "Europe/Paris",
		"CEST": "Europe/Paris",
		"JST":  "Asia/Tokyo",
		"CST8": "Asia/Shanghai",
		"AEST": "Australia/Sydney",
		"SGT":  "Asia/Singapore",
		"GST":  "Asia/Dubai",
	}
	if mapped, ok := aliases[strings.ToUpper(tz)]; ok {
		return mapped
	}
	return tz
}
