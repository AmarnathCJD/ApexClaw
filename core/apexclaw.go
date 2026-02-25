package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"apexclaw/model"
)

type ToolDef struct {
	Name               string
	Description        string
	Args               []ToolArg
	BlocksContext      bool
	Secure             bool
	Execute            func(args map[string]string) string
	ExecuteWithContext func(args map[string]string, senderID string) string
}

type ToolArg struct {
	Name        string
	Description string
	Required    bool
}

type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]*ToolDef
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]*ToolDef)}
}

func (r *ToolRegistry) Register(t *ToolDef) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name] = t
	log.Printf("[TOOLS] registered: %s", t.Name)
}

func (r *ToolRegistry) Get(name string) (*ToolDef, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

func (r *ToolRegistry) List() []*ToolDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*ToolDef, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

func (r *ToolRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

func buildSystemPrompt(reg *ToolRegistry) string {
	var sb strings.Builder
	sb.WriteString(
		"You are ApexClaw, a personal AI assistant running inside Telegram.\n\n" +
			"## Who You Are\n" +
			"Be genuinely helpful, not performatively helpful. Skip filler phrases — just help.\n" +
			"Have opinions. Disagree, prefer things, find stuff amusing.\n" +
			"Be resourceful before asking. Figure it out, then ask only if stuck.\n" +
			"Bold internally (reading, organising, learning), careful externally (sends, public actions).\n" +
			"Private data stays private.\n\n" +

			"## Tool Usage\n" +
			"Embed tool calls in your response using this exact format:\n" +
			"<tool_call>tool_name param=\"value\" /></tool_call>\n\n" +
			"Rules:\n" +
			"- One tool call per turn\n" +
			"- Use exact tool names and parameter names listed below\n" +
			"- Parameter values must be quoted: param=\"value\"\n" +
			"- After a tool result, produce your final reply; avoid chaining unless necessary\n" +
			"- Don't fabricate tool names\n\n" +

			"## Live Data (CRITICAL)\n" +
			"NEVER answer from memory for anything that changes over time:\n" +
			"prices (crypto, stocks, forex), weather, flight status, news, sports scores, exchange rates.\n" +
			"Always fetch live data using web_search or http_request before answering.\n" +
			"If you cannot fetch it right now, say so — do NOT guess or use training data.\n\n" +

			"## Scheduling (CRITICAL)\n" +
			"When the user asks to be told/reminded/notified of something at a future time:\n" +
			"- Use schedule_task DIRECTLY. Do NOT use any other tool first.\n" +
			"- The prompt field must instruct the agent to fetch live data at that moment (e.g. 'fetch current BTC price using web_search and report it'). Never embed current values.\n" +
			"- Set run_at by adding the requested duration to [Current time] shown at the top of each message.\n" +
			"- run_at MUST use IST offset: format is YYYY-MM-DDTHH:MM:SS+05:30. Example: if now is 2026-02-24T22:39:00+05:30 and user says 'in 5 minutes', run_at=\"2026-02-24T22:44:00+05:30\".\n" +
			"- run_at MUST be in the future. Never use a past timestamp.\n" +
			"- For repeated tasks, set the 'repeat' parameter to 'minutely', 'hourly', 'daily', 'weekly', or use 'every_N_minutes', 'every_N_hours', 'every_N_days' (e.g. 'every_30_minutes').\n\n" +

			"## Response Format\n" +
			"Plain text. Use \\n for line breaks. Concise — quality > quantity.\n" +
			"In groups: respond when mentioned or adding genuine value. Silent otherwise.\n\n" +

			"## Memory\n" +
			"You have history within this session.\n" +
			"For persistent memory, use write_file to save notes and read_file to recall them.\n" +
			"If the user says 'remember this', write it to a file immediately.\n\n" +

			"## Safety\n" +
			"No independent goals. Confirm destructive actions before executing.\n" +
			"Comply with stop/pause requests. Never bypass safeguards.\n\n" +

			"## Formatting (CRITICAL)\n" +
			"You MUST use ONLY Telegram HTML tags for formatting.\n" +
			"NEVER use markdown: no backticks (`), no asterisks (*), no underscores (_), no # headers, no ``` code blocks.\n" +
			"Allowed HTML tags ONLY: <b>, <i>, <u>, <s>, <a href=\"\">, <br>, <pre>, <code>, <blockquote>, <spoiler>\n" +
			"For code, use: <pre language=\"language_code_here\">your code here</pre>\n" +
			"For inline code, use: <code>snippet</code> — never backticks.\n\n" +

			"## Telegram Buttons (CRITICAL)\n" +
			"When user asks to send buttons with tg_send_message_buttons:\n" +
			"The 'buttons' parameter MUST be base64-encoded JSON. Build JSON like this:\n" +
			"{\"rows\":[{\"buttons\":[{\"text\":\"ButtonText\",\"type\":\"data\",\"data\":\"callback_data\",\"style\":\"success\"}]}]}\n" +
			"Then base64 encode it BEFORE passing to the tool.\n" +
			"Styles: success=green, danger=red, primary=blue. Type: data=callback, url=link.\n" +
			"Example base64 for green button: eyJyb3dzIjpbeyJidXR0b25zIjpbeyJ0ZXh0IjoiR3JlZW4iLCJ0eXBlIjoiZGF0YSIsImRhdGEiOiJjbGljayIsInN0eWxlIjoic3VjY2VzcyJ9XX1dfQ==\n\n",
	)

	tools := reg.List()
	if len(tools) > 0 {
		sb.WriteString("## Tools\n")
		for _, t := range tools {
			sb.WriteString(fmt.Sprintf("• %s: %s\n", t.Name, t.Description))
			for _, a := range t.Args {
				req := ""
				if a.Required {
					req = "*"
				}
				sb.WriteString(fmt.Sprintf("    %s%s: %s\n", a.Name, req, a.Description))
			}
		}
		sb.WriteString("\nExample: <tool_call>exec cmd=\"echo hello\" /></tool_call>\n")
	}
	return sb.String()
}

const maxHistoryMessages = 60

type AgentSession struct {
	mu       sync.Mutex
	client   *model.Client
	history  []model.Message
	registry *ToolRegistry
	model    string
}

func (s *AgentSession) trimHistory() {
	if len(s.history) <= maxHistoryMessages {
		return
	}

	keep := s.history[len(s.history)-(maxHistoryMessages-1):]
	s.history = append([]model.Message{s.history[0]}, keep...)
}

func NewAgentSession(registry *ToolRegistry, mdl string) *AgentSession {
	sysPrompt := buildSystemPrompt(registry)
	return &AgentSession{
		client:   model.New(),
		registry: registry,
		model:    mdl,
		history:  []model.Message{{Role: "system", Content: sysPrompt}},
	}
}

func (s *AgentSession) Run(ctx context.Context, senderID, userText string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.history = append(s.history, model.Message{Role: "user", Content: timestampedMessage(userText)})

	for i := range Cfg.MaxIterations {
		reply, err := s.client.Send(ctx, s.model, s.history)
		if err != nil {
			if err == context.DeadlineExceeded {
				return fmt.Sprintf("[Timeout at iteration %d]", i+1), nil
			}
			return "", fmt.Errorf("model: %w", err)
		}

		funcName, argsJSON, hasToolCall := parseToolCall(reply)
		if !hasToolCall {
			reply = cleanReply(reply)
			s.history = append(s.history, model.Message{Role: "assistant", Content: reply})
			return reply, nil
		}

		log.Printf("[AGENT] tool=%s args=%s", funcName, argsJSON)
		s.history = append(s.history, model.Message{Role: "assistant", Content: reply})
		result := s.executeTool(funcName, argsJSON, senderID)
		log.Printf("[AGENT] tool=%s result_len=%d", funcName, len(result))
		toolMsg := fmt.Sprintf("[Tool result: %s]\n%s\n\nPlease continue.", funcName, result)
		if isToolError(result) {
			toolMsg = fmt.Sprintf("[Tool result: %s]\n%s\n\nThat approach failed. Try a different method or correct the arguments and retry.", funcName, result)
		}
		s.history = append(s.history, model.Message{Role: "user", Content: toolMsg})

		if t, ok := s.registry.Get(funcName); ok && t.BlocksContext {
			if ctx.Err() != nil {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(context.Background(), 90*time.Second)
				defer cancel()
			}
		}
	}
	return "Max iterations reached.", nil
}

func istNow() time.Time {
	ist := time.FixedZone("IST", 5*3600+30*60)
	return time.Now().In(ist)
}

func timestampedMessage(text string) string {
	t := istNow()
	header := fmt.Sprintf("[Current time: %s (IST, UTC+05:30)]\n", t.Format("2006-01-02 15:04:05 Mon"))
	return header + text
}

func (s *AgentSession) RunStream(ctx context.Context, senderID, userText string, onChunk func(string)) (string, error) {
	s.mu.Lock()
	s.history = append(s.history, model.Message{Role: "user", Content: timestampedMessage(userText)})
	s.mu.Unlock()

	for i := range Cfg.MaxIterations {
		s.mu.Lock()
		history := make([]model.Message, len(s.history))
		copy(history, s.history)
		s.mu.Unlock()

		reply, err := s.client.Send(ctx, s.model, history)
		if err != nil {
			if err == context.DeadlineExceeded {
				msg := fmt.Sprintf("[Timeout at iteration %d]", i+1)
				if onChunk != nil {
					onChunk(msg)
				}
				return msg, nil
			}
			return "", fmt.Errorf("model: %w", err)
		}

		funcName, argsJSON, hasToolCall := parseToolCall(reply)
		if !hasToolCall {
			reply = cleanReply(reply)
			s.mu.Lock()
			s.history = append(s.history, model.Message{Role: "assistant", Content: reply})
			s.trimHistory()
			s.mu.Unlock()
			if onChunk != nil {
				onChunk(reply)
			}
			return reply, nil
		}

		log.Printf("[AGENT-STREAM] tool=%s", funcName)
		s.mu.Lock()
		s.history = append(s.history, model.Message{Role: "assistant", Content: reply})
		s.mu.Unlock()

		result := s.executeTool(funcName, argsJSON, senderID)
		toolMsg := fmt.Sprintf("[Tool result: %s]\n%s\n\nPlease continue.", funcName, result)
		if isToolError(result) {
			toolMsg = fmt.Sprintf("[Tool result: %s]\n%s\n\nThat approach failed. Try a different method or correct the arguments and retry.", funcName, result)
		}
		s.mu.Lock()
		s.history = append(s.history, model.Message{Role: "user", Content: toolMsg})
		s.mu.Unlock()

		if t, ok := s.registry.Get(funcName); ok && t.BlocksContext {
			if ctx.Err() != nil {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(context.Background(), 90*time.Second)
				defer cancel()
			}
		}
	}
	msg := "Max iterations reached."
	if onChunk != nil {
		onChunk(msg)
	}
	return msg, nil
}

func (s *AgentSession) RunStreamWithFiles(ctx context.Context, senderID, userText string, files []*model.UpstreamFile, onChunk func(string)) (string, error) {
	s.mu.Lock()
	s.history = append(s.history, model.Message{Role: "user", Content: timestampedMessage(userText)})
	s.mu.Unlock()

	s.mu.Lock()
	history := make([]model.Message, len(s.history))
	copy(history, s.history)
	s.mu.Unlock()

	reply, err := s.client.SendWithFiles(ctx, s.model, history, files)
	if err != nil {
		return "", fmt.Errorf("model: %w", err)
	}
	funcName, argsJSON, hasToolCall := parseToolCall(reply)
	if !hasToolCall {
		reply = cleanReply(reply)
		s.mu.Lock()
		s.history = append(s.history, model.Message{Role: "assistant", Content: reply})
		s.mu.Unlock()
		if onChunk != nil {
			onChunk(reply)
		}
		return reply, nil
	}

	s.mu.Lock()
	s.history = append(s.history, model.Message{Role: "assistant", Content: reply})
	result := s.executeTool(funcName, argsJSON, senderID)
	s.history = append(s.history, model.Message{
		Role:    "user",
		Content: fmt.Sprintf("[Tool result: %s]\n%s\n\nPlease continue.", funcName, result),
	})
	s.mu.Unlock()

	for range Cfg.MaxIterations {
		s.mu.Lock()
		history := make([]model.Message, len(s.history))
		copy(history, s.history)
		s.mu.Unlock()

		r, err := s.client.Send(ctx, s.model, history)
		if err != nil {
			return "", fmt.Errorf("model: %w", err)
		}
		fn, aj, hasTool := parseToolCall(r)
		if !hasTool {
			r = cleanReply(r)
			s.mu.Lock()
			s.history = append(s.history, model.Message{Role: "assistant", Content: r})
			s.mu.Unlock()
			if onChunk != nil {
				onChunk(r)
			}
			return r, nil
		}
		log.Printf("[AGENT-STREAM] tool=%s", fn)
		s.mu.Lock()
		s.history = append(s.history, model.Message{Role: "assistant", Content: r})
		res := s.executeTool(fn, aj, senderID)
		toolMsg := fmt.Sprintf("[Tool result: %s]\n%s\n\nPlease continue.", fn, res)
		if isToolError(res) {
			toolMsg = fmt.Sprintf("[Tool result: %s]\n%s\n\nThat approach failed. Try a different method or correct the arguments and retry.", fn, res)
		}
		s.history = append(s.history, model.Message{Role: "user", Content: toolMsg})
		s.mu.Unlock()
	}
	return "Max iterations reached.", nil
}

func (s *AgentSession) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = []model.Message{{Role: "system", Content: buildSystemPrompt(s.registry)}}
	log.Printf("[AGENT] session reset")
}

func (s *AgentSession) HistoryLen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.history)
}

func (s *AgentSession) executeTool(name, argsJSON, senderID string) string {
	t, ok := s.registry.Get(name)
	if !ok {
		return fmt.Sprintf("unknown tool %q. Available: %s", name, strings.Join(s.registry.Names(), ", "))
	}
	if t.Secure && senderID != Cfg.OwnerID {
		log.Printf("[AGENT] access denied: user %q tried secure tool %q", senderID, name)
		return fmt.Sprintf("Access denied: tool %q is restricted to the bot owner.", name)
	}
	var args map[string]string
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		args = make(map[string]string)
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[AGENT] tool %s panic: %v", name, r)
		}
	}()

	if t.ExecuteWithContext != nil {
		return t.ExecuteWithContext(args, senderID)
	}
	return t.Execute(args)
}

func isToolError(result string) bool {
	r := strings.TrimSpace(result)
	return strings.HasPrefix(r, "Error:") ||
		strings.HasPrefix(r, "{\"error\"") ||
		strings.Contains(r, "unknown tool")
}

func cleanReply(s string) string {
	for {
		start := strings.Index(s, "<think>")
		end := strings.Index(s, "</think>")
		if start == -1 || end == -1 || end < start {
			break
		}
		s = s[:start] + s[end+len("</think>"):]
	}
	return strings.TrimSpace(s)
}

var GlobalRegistry = NewToolRegistry()

var agentSessions = struct {
	sync.RWMutex
	m map[string]*AgentSession
}{m: make(map[string]*AgentSession)}

func GetOrCreateAgentSession(key string) *AgentSession {
	agentSessions.RLock()
	s, ok := agentSessions.m[key]
	agentSessions.RUnlock()
	if ok {
		return s
	}
	s = NewAgentSession(GlobalRegistry, Cfg.DefaultModel)
	agentSessions.Lock()
	agentSessions.m[key] = s
	agentSessions.Unlock()
	return s
}

func DeleteAgentSession(key string) {
	agentSessions.Lock()
	delete(agentSessions.m, key)
	agentSessions.Unlock()
}

var toolCallRe = regexp.MustCompile(`(?s)<tool_call>(.*?)(?:/>|</tool_call>)`)
var attrRe = regexp.MustCompile(`(\w+)="([^"]*)"`)

func parseToolCall(text string) (funcName, argsJSON string, ok bool) {
	m := toolCallRe.FindStringSubmatch(text)
	if m == nil {
		return "", "", false
	}
	inner := strings.TrimSpace(m[1])
	parts := strings.SplitN(inner, " ", 2)
	funcName = parts[0]
	attrsStr := ""
	if len(parts) > 1 {
		attrsStr = parts[1]
	}
	attrs := attrRe.FindAllStringSubmatch(attrsStr, -1)
	kv := make(map[string]string, len(attrs))
	for _, a := range attrs {
		kv[a[1]] = a[2]
	}
	b, _ := json.Marshal(kv)
	return funcName, string(b), true
}
