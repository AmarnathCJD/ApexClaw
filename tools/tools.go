package tools

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ArgType is the declared type of a tool argument.
type ArgType string

const (
	ArgString ArgType = "string"
	ArgInt    ArgType = "int"
	ArgBool   ArgType = "bool"
	ArgFloat  ArgType = "float"
	ArgList   ArgType = "list" // []string
	ArgDict   ArgType = "dict" // map[string]any
	ArgAny    ArgType = "any"  // pass through whatever JSON gave us
)

// ToolArg describes one argument the model may pass to a tool.
type ToolArg struct {
	Name        string
	Type        ArgType
	Description string
	Required    bool
	Default     any
	Enum        []string
}

// ToolDef is the canonical tool descriptor. Execute receives typed args
// (map[string]any) — strings stay strings, numbers stay numbers, booleans
// stay booleans, lists are []any, dicts are map[string]any.
type ToolDef struct {
	Name          string
	Description   string
	Args          []ToolArg
	Secure        bool
	Sequential    bool // serialize this tool (no parallel dispatch)
	BlocksContext bool // extends parent context after this tool runs (e.g. DeepWork)
	// MaxOutput caps the tool result in bytes before it goes back to the model.
	// 0 = use the agent-layer default (16KB). -1 = never truncate (raw passthrough).
	MaxOutput int
	// Timeout is the per-tool wall-clock cap enforced by the agent layer.
	// 0 = agent default (30s). -1 = no enforced timeout (tool manages its own).
	Timeout time.Duration
	// Execute is the simple form — no context.
	Execute func(args map[string]any) string
	// ExecuteWithContext receives the sender/user ID for context-aware tools.
	ExecuteWithContext func(args map[string]any, senderID string) string
}

// Registry is the global tool table. All registered tools are appended to
// the ordered slice (used for system-prompt emission, which preserves
// listing order) AND indexed by name (for fast lookup at dispatch time).
var (
	All    []*ToolDef
	byName = map[string]*ToolDef{}
)

// init registers all built-in tools. Ordered grouping matches the system
// prompt's "available tools" rendering — tools at the top of the list get
// preference in the model's reasoning.
func init() {
	for _, t := range []*ToolDef{
		// System control
		Exec,
		ExecChain,
		RunPython,
		SystemInfo,
		ProcessList,
		KillProcess,
		UpdateClaw,
		RestartClaw,
		KillClaw,

		// File ops
		ReadFile,
		WriteFile,
		EditFile,
		AppendFile,
		GrepFile,
		ListDir,
		CreateDir,
		DeleteFile,
		MoveFile,
		SearchFiles,
		ReadToolOutput,

		// Minimal web
		WebFetch,
		WebSearch,
		HTTPRequest,

		// Structured data
		JSONQuery,

		// Git
		GitStatus,
		GitDiff,
		GitLog,
		GitShow,
		GitBranchList,
		GitCommit,
		GitBranchCreate,
		GitCheckout,
		GitPull,
		GitPush,

		// GitHub (PR + CI)
		GhPRView,
		GhRunView,

		// Research
		ArxivSearch,
		HackerNewsTop,
		DNSLookup,

		ScheduleTaskTool,
		ListTasksTool,
		CancelTaskTool,
		CancelTasksByTagTool,
		PauseTaskTool,
		ResumeTaskTool,

		// Time
		Datetime,

		// Telegram
		TGSendMessage,
		TGSendFile,
		TGSendPhoto,
		TGSendAlbum,
		TGSendLocation,
		TGSendMessageWithButtons,
		TGSendRich,
		SetBotDp,
		TGDownload,
		TGGetFile,
		TGForwardMsg,
		TGDeleteMsg,
		TGPinMsg,
		TGUnpinMsg,
		TGGetChatInfo,
		TGReact,
		TGGetMembers,
		TGBroadcast,
		TGGetMessage,
		TGEditMessage,
		TGCreateInvite,
		TGGetProfilePhotos,
		TGBanUser,
		TGMuteUser,
		TGKickUser,
		TGPromoteAdmin,
		TGDemoteAdmin,

		// WhatsApp
		WASendMessage,
		WASendFile,
		WAGetContacts,
		WAGetGroups,

		// ChatGLM power tools
		ZAIImageGenerate,
		ZAIImageEdit,
		ZAIResearch,
		ZAIAgent,
	} {
		Register(t)
	}
}

// Register adds a tool. Safe to call from package init() blocks.
// Calling twice with the same name replaces the prior registration in place.
func Register(def *ToolDef) {
	if def == nil || def.Name == "" {
		return
	}
	if existing, ok := byName[def.Name]; ok {
		for i, t := range All {
			if t == existing {
				All[i] = def
				break
			}
		}
		byName[def.Name] = def
		return
	}
	All = append(All, def)
	byName[def.Name] = def
}

// Get returns a tool by name. ok = false if not registered.
func Get(name string) (*ToolDef, bool) {
	d, ok := byName[name]
	return d, ok
}

// Names returns the registered tool names, in registration order.
func Names() []string {
	out := make([]string, 0, len(All))
	for _, t := range All {
		out = append(out, t.Name)
	}
	return out
}

// ---- typed-arg readers ----
//
// Each reader is forgiving: if the model emitted the "wrong" JSON type
// (e.g. "true" string instead of bool true), we coerce when sensible.
// Use the (value, ok) return to distinguish "missing or invalid" from a
// real zero value.

func String(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case nil:
		return ""
	case bool, float64, int, int64:
		return fmt.Sprintf("%v", t)
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func StringOr(args map[string]any, key, fallback string) string {
	s := String(args, key)
	if s == "" {
		return fallback
	}
	return s
}

func Int(args map[string]any, key string) (int, bool) {
	v, ok := args[key]
	if !ok {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return int(t), true
	case int:
		return t, true
	case int64:
		return int(t), true
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, false
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

func IntOr(args map[string]any, key string, fallback int) int {
	if n, ok := Int(args, key); ok {
		return n
	}
	return fallback
}

func Int64(args map[string]any, key string) (int64, bool) {
	v, ok := args[key]
	if !ok {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case int:
		return int64(t), true
	case int64:
		return t, true
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, false
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

func Bool(args map[string]any, key string) (bool, bool) {
	v, ok := args[key]
	if !ok {
		return false, false
	}
	switch t := v.(type) {
	case bool:
		return t, true
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		switch s {
		case "true", "1", "yes", "on", "t", "y":
			return true, true
		case "false", "0", "no", "off", "f", "n":
			return false, true
		case "":
			return false, false
		}
	case float64:
		return t != 0, true
	}
	return false, false
}

func BoolOr(args map[string]any, key string, fallback bool) bool {
	if b, ok := Bool(args, key); ok {
		return b
	}
	return fallback
}

func Float(args map[string]any, key string) (float64, bool) {
	v, ok := args[key]
	if !ok {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	}
	return 0, false
}

func FloatOr(args map[string]any, key string, fallback float64) float64 {
	if f, ok := Float(args, key); ok {
		return f
	}
	return fallback
}

// List returns the value at key as []string. Coerces:
//   - []any of strings/numbers/bools
//   - comma-separated single string
//   - single string -> single-element list
func List(args map[string]any, key string) []string {
	v, ok := args[key]
	if !ok || v == nil {
		return nil
	}
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			s := fmt.Sprintf("%v", x)
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return t
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return nil
		}
		if strings.Contains(s, ",") {
			parts := strings.Split(s, ",")
			out := make([]string, 0, len(parts))
			for _, p := range parts {
				if p = strings.TrimSpace(p); p != "" {
					out = append(out, p)
				}
			}
			return out
		}
		return []string{s}
	}
	return nil
}

// IntList returns the value at key as []int. Coerces from string lists too.
func IntList(args map[string]any, key string) []int {
	v, ok := args[key]
	if !ok || v == nil {
		return nil
	}
	switch t := v.(type) {
	case []any:
		out := make([]int, 0, len(t))
		for _, x := range t {
			switch n := x.(type) {
			case float64:
				out = append(out, int(n))
			case int:
				out = append(out, n)
			case string:
				if p, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
					out = append(out, p)
				}
			}
		}
		return out
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return nil
		}
		var out []int
		for _, p := range strings.Split(s, ",") {
			if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
				out = append(out, n)
			}
		}
		return out
	}
	return nil
}

// Dict returns args[key] as a map[string]any.
func Dict(args map[string]any, key string) map[string]any {
	v, ok := args[key]
	if !ok {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
		var out map[string]any
		if json.Unmarshal([]byte(s), &out) == nil {
			return out
		}
	}
	return nil
}

// Require validates that all Required args are present and non-empty.
// Returns an error message string for embedding directly in tool results.
func Require(args map[string]any, def *ToolDef) string {
	if def == nil {
		return ""
	}
	var missing []string
	for _, a := range def.Args {
		if !a.Required {
			continue
		}
		v, ok := args[a.Name]
		if !ok || v == nil {
			missing = append(missing, a.Name)
			continue
		}
		if s, isStr := v.(string); isStr && strings.TrimSpace(s) == "" {
			missing = append(missing, a.Name)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return fmt.Sprintf("Error: missing required arg(s): %s", strings.Join(missing, ", "))
}

// JSON marshals an arbitrary value for embedding in tool output.
func JSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return string(b)
}

// JSONPretty is JSON with indent.
func JSONPretty(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return string(b)
}
