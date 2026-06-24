package model

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ToolCall is one parsed tool invocation emitted by the model.
type ToolCall struct {
	ID       string         // model-supplied correlation id (may be empty — agent assigns one)
	Name     string         // tool name
	Args     map[string]any // typed args, ready for the tool's Execute
	ArgsJSON string         // JSON-encoded Args, for legacy code that expects a string blob
}

// ToolResult is one tool execution's outcome, ready to embed in a
// `tool_results` reply back to the model.
type ToolResult struct {
	ID     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	Ok     bool   `json:"ok"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

var (
	reToolCallsFence = regexp.MustCompile("(?is)```tool_calls\\s*\\n?(.*?)```")
	reBodyFence      = regexp.MustCompile("(?is)```body:([A-Za-z0-9_\\-]+)\\s*\\n?(.*?)```")
	reGenericFence   = regexp.MustCompile("(?is)```(?:json)?\\s*\\n?(\\{[\\s\\S]*?\\}|\\[[\\s\\S]*?\\])\\s*```")
	rePlaceholder    = regexp.MustCompile(`^@@BODY:([A-Za-z0-9_\-]+)$`)
	reTrailingComma  = regexp.MustCompile(`,(\s*[}\]])`)
)

// ParseToolCalls extracts every tool call from an assistant message.
// Returns the parsed calls in source order plus any leftover prose
// (text outside the tool_calls / body fences), trimmed.
//
// Accepted shapes (in priority order):
//  1. ```tool_calls\n[ {name, args}, ... ]\n```  (preferred — explicit)
//  2. ```tool_calls\n{"tool_calls":[...]}\n```   (dict form some models emit)
//  3. ```json\n[ {name, args}, ... ]\n```         (generic JSON fence)
//  4. Bare top-level JSON array or object        (last resort)
//
// Long string args may use the placeholder shape "@@BODY:<id>" which is
// substituted with the contents of a paired ```body:<id>\n...\n``` fence
// so the model doesn't have to escape large blobs inside JSON.
func ParseToolCalls(text string) ([]ToolCall, string) {
	// Strip body fences first and remember their contents keyed by id.
	bodies := map[string]string{}
	work := reBodyFence.ReplaceAllStringFunc(text, func(m string) string {
		sub := reBodyFence.FindStringSubmatch(m)
		if len(sub) == 3 {
			body := sub[2]
			body = strings.TrimPrefix(body, "\r\n")
			body = strings.TrimPrefix(body, "\n")
			body = strings.TrimSuffix(body, "\n")
			bodies[sub[1]] = body
		}
		return ""
	})

	var calls []ToolCall
	var consumed []string

	// 1) Explicit tool_calls fences.
	if matches := reToolCallsFence.FindAllStringSubmatchIndex(work, -1); len(matches) > 0 {
		for _, m := range matches {
			body := work[m[2]:m[3]]
			consumed = append(consumed, work[m[0]:m[1]])
			if batch, err := parseJSONCalls(body); err == nil {
				calls = append(calls, batch...)
			}
		}
	}

	// 2) Generic ```json fence (only if explicit didn't match).
	if len(calls) == 0 {
		for _, m := range reGenericFence.FindAllStringSubmatchIndex(work, -1) {
			body := work[m[2]:m[3]]
			batch, err := parseJSONCalls(body)
			if err != nil || len(batch) == 0 {
				continue
			}
			calls = append(calls, batch...)
			consumed = append(consumed, work[m[0]:m[1]])
		}
	}

	// 3) Bare top-level JSON (no fences at all).
	if len(calls) == 0 {
		trimmed := strings.TrimSpace(work)
		if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") {
			if batch, err := parseJSONCalls(trimmed); err == nil && len(batch) > 0 {
				calls = batch
				consumed = append(consumed, trimmed)
			}
		}
	}

	// Inline body placeholders into the args.
	for i := range calls {
		calls[i].Args = inlineBodies(calls[i].Args, bodies).(map[string]any)
		if b, err := json.Marshal(calls[i].Args); err == nil {
			calls[i].ArgsJSON = string(b)
		} else {
			calls[i].ArgsJSON = "{}"
		}
	}

	// Compute commentary = original text minus the consumed fence regions
	// and body fences.
	commentary := text
	for _, c := range consumed {
		commentary = strings.Replace(commentary, c, "", 1)
	}
	commentary = reBodyFence.ReplaceAllString(commentary, "")
	return calls, strings.TrimSpace(commentary)
}

// parseJSONCalls accepts an array of call objects or a dict wrapping such
// an array, with a tolerant repair pass on parse failure.
func parseJSONCalls(body string) ([]ToolCall, error) {
	body = strings.TrimSpace(body)
	body = strings.TrimPrefix(body, "json\n")
	body = strings.TrimPrefix(body, "JSON\n")
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, fmt.Errorf("empty body")
	}

	try := func(s string) ([]ToolCall, error) {
		// Array form: [ {...}, {...} ]
		var arr []map[string]any
		if err := json.Unmarshal([]byte(s), &arr); err == nil {
			return callsFromArr(arr), nil
		}
		// Dict form: {"tool_calls":[...]} / {"calls":[...]} / {"tools":[...]}
		var obj map[string]any
		if err := json.Unmarshal([]byte(s), &obj); err == nil {
			for _, key := range []string{"tool_calls", "calls", "tools"} {
				if v, ok := obj[key]; ok {
					if list, ok := v.([]any); ok {
						out := make([]map[string]any, 0, len(list))
						for _, item := range list {
							if m, ok := item.(map[string]any); ok {
								out = append(out, m)
							}
						}
						return callsFromArr(out), nil
					}
				}
			}
			// Single call as a plain object.
			if _, ok := obj["name"]; ok {
				return callsFromArr([]map[string]any{obj}), nil
			}
		}
		return nil, fmt.Errorf("not parseable as array or dict")
	}

	if calls, err := try(body); err == nil {
		return calls, nil
	}
	if calls, err := try(tolerantFix(body)); err == nil {
		return calls, nil
	}
	if calls := parsePerObjectFallback(body); len(calls) > 0 {
		return calls, nil
	}
	return nil, fmt.Errorf("not parseable as array or dict")
}

func parsePerObjectFallback(body string) []ToolCall {
	chunks := splitTopLevelObjects(body)
	if len(chunks) == 0 {
		return nil
	}
	var out []ToolCall
	for _, c := range chunks {
		var obj map[string]any
		if json.Unmarshal([]byte(c), &obj) != nil {
			if fixed := tolerantFix(c); fixed != c {
				if json.Unmarshal([]byte(fixed), &obj) != nil {
					continue
				}
			} else {
				continue
			}
		}
		if _, ok := obj["name"]; !ok {
			continue
		}
		got := callsFromArr([]map[string]any{obj})
		out = append(out, got...)
	}
	return out
}

func splitTopLevelObjects(s string) []string {
	var out []string
	depth := 0
	inStr := false
	esc := false
	start := -1
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if esc {
			esc = false
			continue
		}
		if ch == '\\' && inStr {
			esc = true
			continue
		}
		if ch == '"' {
			inStr = !inStr
			continue
		}
		if inStr {
			continue
		}
		switch ch {
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 && start >= 0 {
				out = append(out, s[start:i+1])
				start = -1
			}
			if depth < 0 {
				depth = 0
			}
		}
	}
	return out
}

// callsFromArr coerces a generic array of objects into typed ToolCalls.
// Accepts alternative field names ("tool"/"function" for name,
// "arguments"/"input"/"parameters" for args) some models drift toward.
func callsFromArr(arr []map[string]any) []ToolCall {
	out := make([]ToolCall, 0, len(arr))
	for _, obj := range arr {
		tc := ToolCall{}
		if v, ok := obj["id"].(string); ok {
			tc.ID = v
		}
		for _, k := range []string{"name", "tool", "function"} {
			if v, ok := obj[k].(string); ok && v != "" {
				tc.Name = v
				break
			}
		}
		for _, k := range []string{"args", "arguments", "input", "parameters"} {
			if v, ok := obj[k].(map[string]any); ok {
				tc.Args = v
				break
			}
			if v, ok := obj[k].(string); ok && strings.TrimSpace(v) != "" {
				var parsed map[string]any
				if json.Unmarshal([]byte(v), &parsed) == nil {
					tc.Args = parsed
					break
				}
			}
		}
		if tc.Args == nil {
			tc.Args = map[string]any{}
		}
		if tc.Name == "" {
			continue
		}
		out = append(out, tc)
	}
	return out
}

// tolerantFix strips trailing commas and tries to find the longest
// fully-balanced prefix so we can recover from truncated JSON.
func tolerantFix(s string) string {
	s = reTrailingComma.ReplaceAllString(s, "$1")

	depth := 0
	inStr := false
	esc := false
	lastGoodEnd := -1
	for i, r := range s {
		if esc {
			esc = false
			continue
		}
		if r == '\\' {
			esc = true
			continue
		}
		if r == '"' {
			inStr = !inStr
			continue
		}
		if inStr {
			continue
		}
		switch r {
		case '[', '{':
			depth++
		case ']', '}':
			depth--
			if depth == 0 {
				lastGoodEnd = i + 1
			}
		}
	}
	if lastGoodEnd > 0 && lastGoodEnd < len(s) {
		return s[:lastGoodEnd]
	}
	if !inStr {
		var closers strings.Builder
		for depth > 0 {
			closers.WriteByte(']')
			depth--
		}
		return s + closers.String()
	}
	return s
}

// inlineBodies walks args recursively and substitutes "@@BODY:<id>" strings
// with the matching body contents verbatim. Substitution is anchored — the
// entire string value must equal "@@BODY:<id>" for the swap to fire.
func inlineBodies(v any, bodies map[string]string) any {
	switch t := v.(type) {
	case map[string]any:
		for k, vv := range t {
			t[k] = inlineBodies(vv, bodies)
		}
		return t
	case []any:
		for i, vv := range t {
			t[i] = inlineBodies(vv, bodies)
		}
		return t
	case string:
		if m := rePlaceholder.FindStringSubmatch(t); len(m) == 2 {
			if body, ok := bodies[m[1]]; ok {
				return body
			}
		}
		return t
	}
	return v
}

// BuildToolResult creates one result entry from a tool execution.
// content is the tool's output OR error message; isError says which one.
func BuildToolResult(callID, name, content string, isError bool) ToolResult {
	r := ToolResult{ID: callID, Name: name, Ok: !isError}
	if isError {
		r.Error = content
	} else {
		r.Output = content
	}
	return r
}

// BuildToolResultsMessage emits a single ```tool_results``` fenced JSON
// block containing every result in dispatch order. This goes back to the
// model as a user-role message so the model can react to all results in
// one read.
func BuildToolResultsMessage(results []ToolResult) string {
	b, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "```tool_results\n[]\n```"
	}
	return "```tool_results\n" + string(b) + "\n```"
}
