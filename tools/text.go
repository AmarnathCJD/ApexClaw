package tools

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var TextProcess = &ToolDef{
	Name:        "text_process",
	Description: "Process text: count_words, count_chars, count_lines, upper, lower, reverse, trim, title",
	Args: []ToolArg{
		{Name: "text", Description: "Input text to process", Required: true},
		{Name: "op", Description: "Operation: count_words | count_chars | count_lines | upper | lower | reverse | trim | title", Required: true},
	},
	Execute: func(args map[string]string) string {
		text := args["text"]
		op := strings.ToLower(strings.TrimSpace(args["op"]))
		if text == "" {
			return "Error: text is required"
		}
		switch op {
		case "count_words":
			return fmt.Sprintf("%d words", len(strings.Fields(text)))
		case "count_chars":
			return fmt.Sprintf("%d characters (%d bytes)", utf8.RuneCountInString(text), len(text))
		case "count_lines":
			return fmt.Sprintf("%d lines", len(strings.Split(text, "\n")))
		case "upper":
			return strings.ToUpper(text)
		case "lower":
			return strings.ToLower(text)
		case "reverse":
			runes := []rune(text)
			for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
				runes[i], runes[j] = runes[j], runes[i]
			}
			return string(runes)
		case "trim":
			return strings.TrimSpace(text)
		case "title":
			return cases.Title(language.English).String(text)
		default:
			return fmt.Sprintf("Unknown op %q. Valid: count_words, count_chars, count_lines, upper, lower, reverse, trim, title", op)
		}
	},
}
