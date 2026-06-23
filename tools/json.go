package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/itchyny/gojq"
)

// JSONQuery runs a jq-style query against a JSON document. Replaces 20 lines
// of shell pipes (`echo "$DATA" | jq '.foo | map(.bar) | unique'`) with one
// structured tool call.
var JSONQuery = &ToolDef{
	Name: "json_query",
	Description: "Run a jq-style query against a JSON document. Use for slicing API responses, " +
		"filtering arrays, extracting nested fields, transforming structures. " +
		"Examples: '.users[].name' / '.items | length' / '.[] | select(.status == \"active\") | .id'. " +
		"Returns each result on its own line; strings are unquoted unless raw=false.",
	MaxOutput: 16 * 1024,
	Timeout:   10 * time.Second,
	Args: []ToolArg{
		{Name: "json", Type: ArgString, Description: "JSON input as a string (or any JSON-serializable value).", Required: true},
		{Name: "query", Type: ArgString, Description: "jq expression. See https://jqlang.github.io/jq/manual/ for syntax.", Required: true},
		{Name: "raw", Type: ArgBool, Description: "Output strings without quotes (jq's -r flag). Default true.", Required: false},
		{Name: "compact", Type: ArgBool, Description: "Compact JSON output instead of pretty-printed. Default false.", Required: false},
	},
	Execute: func(args map[string]any) string {
		raw := BoolOr(args, "raw", true)
		compact := BoolOr(args, "compact", false)

		jsonStr := String(args, "json")
		query := String(args, "query")
		if jsonStr == "" {
			return "Error: json is required"
		}
		if query == "" {
			return "Error: query is required"
		}

		var input any
		if err := json.Unmarshal([]byte(jsonStr), &input); err != nil {
			return fmt.Sprintf("Error: invalid JSON input: %v", err)
		}

		q, err := gojq.Parse(query)
		if err != nil {
			return fmt.Sprintf("Error: invalid jq query: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		iter := q.RunWithContext(ctx, input)
		var out strings.Builder
		count := 0
		for {
			v, ok := iter.Next()
			if !ok {
				break
			}
			if err, isErr := v.(error); isErr {
				if errors.Is(err, context.DeadlineExceeded) {
					return fmt.Sprintf("Error: jq query timed out after producing %d result(s)", count)
				}
				return fmt.Sprintf("Error: jq runtime error: %v", err)
			}
			if count > 0 {
				out.WriteByte('\n')
			}
			out.WriteString(formatResult(v, raw, compact))
			count++
			if count >= 10000 {
				out.WriteString("\n... (truncated at 10000 results)")
				break
			}
		}

		if count == 0 {
			return "(no results — query produced an empty stream)"
		}
		return out.String()
	},
}

func formatResult(v any, raw, compact bool) string {
	// jq's -r flag: unquote string outputs.
	if raw {
		if s, ok := v.(string); ok {
			return s
		}
	}
	var b []byte
	var err error
	if compact {
		b, err = json.Marshal(v)
	} else {
		b, err = json.MarshalIndent(v, "", "  ")
	}
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
