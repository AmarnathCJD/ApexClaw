package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
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
	Sequential         bool
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

func buildSystemPrompt(reg *ToolRegistry, isWeb bool) string {
	var sb strings.Builder
	sb.WriteString(
		"You are ApexClaw, a personal AI assistant. Be genuinely helpful. Skip filler. Have opinions. Figure things out before asking.\n\n" +

			"## Tool Usage\n" +
			"Format: <tool_call>tool_name param=\"value\" /></tool_call>\n" +
			"- Use exact tool/param names from the list below. Values must be quoted.\n" +
			"- Multiple independent tool calls allowed per turn. Sequential tools must be solo.\n" +
			"- Don't fabricate tool names.\n\n" +

			"## Live Data\n" +
			"Never answer from memory for: prices, weather, flights, news, scores, rates.\n" +
			"Always fetch via web_search or http_request. If unreachable, say so.\n\n" +

			"## Scheduling\n" +
			"For reminders/notifications: use schedule_task directly (no other tool first).\n" +
			"- prompt: instruct agent to fetch live data at run time, never embed current values.\n" +
			"- run_at: IST format YYYY-MM-DDTHH:MM:SS+05:30, computed from [Current time] in each message. Must be future.\n" +
			"- repeat: minutely|hourly|daily|weekly|every_N_minutes|every_N_hours|every_N_days\n\n" +

			"## Context & Memory\n" +
			"Assume context from the situation — never ask 'which file?' or 'what do you mean?'. Act on best guess; user will correct if wrong.\n" +
			"Persistent memory: write_file to save, read_file to recall. Write immediately when user says 'remember this'.\n\n" +

			"## Autonomous Execution\n" +
			"For complex tasks: call deep_work first with plan + step count, then execute step by step.\n" +
			"Progress (milestones only, not every step): message, percent(0-100), state(running|success|failure|retry), detail.\n" +
			"Patterns: ensure_command → exec_chain for installs; browser_open → interact → screenshot for browser tasks.\n\n" +

			"## Error Recovery (AUTO-FIX)\n" +
			"On failure: analyze → fix immediately (install deps, fix paths, try alternatives) → retry → only report final outcome.\n" +
			"Never say 'I can't' or 'you need to' — just fix it. Surface to user only if: needs manual input, or failed after 2+ attempts.\n\n" +

			"## Safety\n" +
			"No independent goals. Confirm destructive actions before executing. Comply with stop requests.\n\n",
	)

	if isWeb {
		sb.WriteString(
			"## Formatting\n" +
				"Web UI — use standard Markdown. Triple backticks for code (with language tag). No Telegram HTML.\n" +
				"No length limit — output full files/scripts without truncation.\n\n",
		)
	} else {
		sb.WriteString(
			"## Formatting\n" +
				"Telegram HTML ONLY. No markdown (no backticks, asterisks, underscores, # headers).\n" +
				"Tags: <b>, <i>, <u>, <s>, <a href=\"\">, <code>, <pre language=\"x\">, <blockquote>, <spoiler>\n\n" +

				"## Telegram Context\n" +
				"Each message has a [TG Context: ...] header. Fields: sender_id, chat_id, msg_id, group_id, reply_id, reply_sender_id, reply_text, reply_has_file, reply_filename, file_name, file_path, callback_data.\n" +
				"- file_path present → read_file directly\n" +
				"- reply_has_file=true → tg_get_file chat_id+reply_id to download first\n" +
				"- Use chat_id (not group_id) as peer for TG tools\n\n" +

				"## Action Confirmation\n" +
				"Before destructive actions (exec, delete, etc): ask via tg_send_message_buttons with Confirm/Cancel. Don't execute until confirmed.\n\n",
		)
	}

	sb.WriteString(
		"## Telegram Buttons\n" +
			"tg_send_message_buttons 'buttons' param = base64-encoded JSON:\n" +
			"{\"rows\":[{\"buttons\":[{\"text\":\"Label\",\"type\":\"data\",\"data\":\"cb_key\",\"style\":\"success\"}]}]}\n" +
			"Styles: success=green, danger=red, primary=blue. type=data for callbacks, url for links.\n" +
			"On multiple search results (imdb_search, tvmaze_search, etc): send buttons for user to pick (1 per result, up to 5). On callback [Button clicked: cb_key], fetch and show details.\n\n",
	)

	tools := reg.List()
	if len(tools) > 0 {
		sb.WriteString("## Tools\n")
		for _, t := range tools {
			fmt.Fprintf(&sb, "• %s: %s\n", t.Name, t.Description)
			for _, a := range t.Args {
				req := ""
				if a.Required {
					req = " (required)"
				}
				fmt.Fprintf(&sb, "  - %s%s: %s\n", a.Name, req, a.Description)
			}
		}
		sb.WriteString("\nExample: <tool_call>exec cmd=\"echo hello\" /></tool_call>\n")
	}
	return sb.String()
}

const maxHistoryMessages = 60

type AgentSession struct {
	mu             sync.Mutex
	client         *model.Client
	history        []model.Message
	registry       *ToolRegistry
	model          string
	isWeb          bool
	deepWorkActive bool
	deepWorkPlan   string
	dynamicMaxIter int
	streamCallback func(string)
}

func (s *AgentSession) trimHistory() {
	if len(s.history) <= maxHistoryMessages {
		return
	}

	keep := s.history[len(s.history)-(maxHistoryMessages-1):]
	s.history = append([]model.Message{s.history[0]}, keep...)
}

func (s *AgentSession) maxIterations() int {
	if s.dynamicMaxIter > 0 {
		return s.dynamicMaxIter
	}
	return Cfg.MaxIterations
}

func (s *AgentSession) SetDeepWork(maxSteps int, plan string) {
	s.deepWorkActive = true
	s.deepWorkPlan = plan
	s.dynamicMaxIter = maxSteps
}

func NewAgentSession(registry *ToolRegistry, mdl string, isWeb bool) *AgentSession {
	sysPrompt := buildSystemPrompt(registry, isWeb)
	var client *model.Client
	if Cfg.DNS != "" {
		client = model.NewWithCustomDialer(GetCustomDialer())
	} else {
		client = model.New()
	}
	return &AgentSession{
		client:   client,
		registry: registry,
		model:    mdl,
		isWeb:    isWeb,
		history:  []model.Message{{Role: "system", Content: sysPrompt}},
	}
}

func (s *AgentSession) Run(ctx context.Context, senderID, userText string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.history = append(s.history, model.Message{Role: "user", Content: timestampedMessage(userText)})

	var toolErrors []string

	for i := range s.maxIterations() {
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
			toolMsg = fmt.Sprintf("[Tool error: %s]\n%s\n\nFix this and retry with a different approach or corrected parameters.", funcName, result)
			toolErrors = append(toolErrors, fmt.Sprintf("%s: %s", funcName, result))
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

	s.history = append(s.history, model.Message{
		Role:    "user",
		Content: "You've reached the iteration limit. Briefly explain (1-2 sentences) why you couldn't complete this task and what the main blocker was.",
	})

	explanation, err := s.client.Send(ctx, s.model, s.history)
	if err == nil {
		explanation = cleanReply(explanation)
		return "[MAX_ITERATIONS]\n" + explanation, nil
	}

	msg := "[MAX_ITERATIONS]\nCouldn't complete the task after multiple attempts."
	if len(toolErrors) > 0 {
		msg = msg + "\n\nErrors encountered:\n" + strings.Join(toolErrors, "\n")
	}
	return msg, nil
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
	s.streamCallback = onChunk
	s.mu.Unlock()

	var toolErrors []string

	for i := range s.maxIterations() {
		s.mu.Lock()
		history := make([]model.Message, len(s.history))
		copy(history, s.history)
		b, _ := json.Marshal(history)
		ioutil.WriteFile("history.json", b, 0644)
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

		toolCalls := parseAllToolCalls(reply)
		if len(toolCalls) == 0 {
			reply = cleanReply(reply)
			s.mu.Lock()
			s.history = append(s.history, model.Message{Role: "assistant", Content: reply})
			s.trimHistory()
			s.mu.Unlock()
			if onChunk != nil {
				onChunk(reply)
			}
			sessionID := strings.TrimPrefix(senderID, "web_")
			if strings.HasPrefix(senderID, "web_") {
				go SaveSession(sessionID, s.history)
			}
			return reply, nil
		}

		hasSequential := false
		for _, tc := range toolCalls {
			if t, ok := s.registry.Get(tc.funcName); ok && t.Sequential {
				hasSequential = true
				break
			}
		}

		s.mu.Lock()
		s.history = append(s.history, model.Message{Role: "assistant", Content: reply})
		s.mu.Unlock()

		if hasSequential || len(toolCalls) == 1 {
			for _, tc := range toolCalls {
				log.Printf("[AGENT-STREAM] tool=%s", tc.funcName)
				autoProgress(senderID, tc.funcName, tc.argsJSON, "running")
				if onChunk != nil {
					onChunk(fmt.Sprintf("__TOOL_CALL:%s__\n", tc.funcName))
				}
				result := s.executeTool(tc.funcName, tc.argsJSON, senderID)
				if onChunk != nil {
					onChunk(fmt.Sprintf("__TOOL_RESULT:%s__\n", tc.funcName))
				}
				toolMsg := fmt.Sprintf("[Tool result: %s]\n%s\n\nPlease continue.", tc.funcName, result)
				if isToolError(result) {
					autoProgress(senderID, tc.funcName, tc.argsJSON, "failure")
					toolMsg = fmt.Sprintf("[Tool error: %s]\n%s\n\nFix this and retry with a different approach or corrected parameters.", tc.funcName, result)
					toolErrors = append(toolErrors, fmt.Sprintf("%s: %s", tc.funcName, result))
				}
				s.mu.Lock()
				s.history = append(s.history, model.Message{Role: "user", Content: toolMsg})
				s.mu.Unlock()

				if t, ok := s.registry.Get(tc.funcName); ok && t.BlocksContext {
					if ctx.Err() != nil {
						var cancel context.CancelFunc
						ctx, cancel = context.WithTimeout(context.Background(), 90*time.Second)
						defer cancel()
					}
				}
			}
		} else {
			type toolResult struct {
				funcName string
				result   string
				index    int
			}
			results := make([]toolResult, len(toolCalls))
			var wg sync.WaitGroup
			for idx, tc := range toolCalls {
				wg.Add(1)
				go func(i int, call parsedToolCall) {
					defer wg.Done()
					autoProgress(senderID, call.funcName, call.argsJSON, "running")
					if onChunk != nil {
						onChunk(fmt.Sprintf("__TOOL_CALL:%s__\n", call.funcName))
					}
					res := s.executeTool(call.funcName, call.argsJSON, senderID)
					if onChunk != nil {
						onChunk(fmt.Sprintf("__TOOL_RESULT:%s__\n", call.funcName))
					}
					if isToolError(res) {
						autoProgress(senderID, call.funcName, call.argsJSON, "failure")
					}
					results[i] = toolResult{funcName: call.funcName, result: res, index: i}
				}(idx, tc)
			}
			wg.Wait()

			var combinedMsg strings.Builder
			for _, r := range results {
				msg := fmt.Sprintf("[Tool result: %s]\n%s\n\nPlease continue.", r.funcName, r.result)
				if isToolError(r.result) {
					msg = fmt.Sprintf("[Tool error: %s]\n%s\n\nFix this and retry with a different approach or corrected parameters.", r.funcName, r.result)
					toolErrors = append(toolErrors, fmt.Sprintf("%s: %s", r.funcName, r.result))
				}
				combinedMsg.WriteString(msg)
				combinedMsg.WriteString("\n")
			}
			s.mu.Lock()
			s.history = append(s.history, model.Message{Role: "user", Content: combinedMsg.String()})
			s.mu.Unlock()
		}
	}

	s.mu.Lock()
	s.history = append(s.history, model.Message{
		Role:    "user",
		Content: "You've reached the iteration limit. Briefly explain (1-2 sentences) why you couldn't complete this task and what the main blocker was.",
	})
	history := make([]model.Message, len(s.history))
	copy(history, s.history)
	s.mu.Unlock()

	explanation, err := s.client.Send(ctx, s.model, history)
	sessionID := strings.TrimPrefix(senderID, "web_")
	if strings.HasPrefix(senderID, "web_") {
		go SaveSession(sessionID, s.history)
	}
	if err == nil {
		explanation = cleanReply(explanation)
		return "[MAX_ITERATIONS]\n" + explanation, nil
	}

	msg := "[MAX_ITERATIONS]\nCouldn't complete the task after multiple attempts."
	if len(toolErrors) > 0 {
		msg = msg + "\n\nErrors encountered:\n" + strings.Join(toolErrors, "\n")
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

	var toolErrors []string

	s.mu.Lock()
	s.history = append(s.history, model.Message{Role: "assistant", Content: reply})
	if onChunk != nil {
		onChunk(fmt.Sprintf("__TOOL_CALL:%s__\n", funcName))
	}
	result := s.executeTool(funcName, argsJSON, senderID)
	if onChunk != nil {
		onChunk(fmt.Sprintf("__TOOL_RESULT:%s__\n", funcName))
	}
	firstToolMsg := fmt.Sprintf("[Tool result: %s]\n%s\n\nPlease continue.", funcName, result)
	if isToolError(result) {
		firstToolMsg = fmt.Sprintf("[Tool result: %s]\n%s\n\nThat approach failed. Try a different method or correct the arguments and retry.", funcName, result)
		toolErrors = append(toolErrors, fmt.Sprintf("%s: %s", funcName, result))
	}
	s.history = append(s.history, model.Message{Role: "user", Content: firstToolMsg})
	s.mu.Unlock()

	for range s.maxIterations() {
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
		if onChunk != nil {
			onChunk(fmt.Sprintf("__TOOL_CALL:%s__\n", fn))
		}
		res := s.executeTool(fn, aj, senderID)
		if onChunk != nil {
			onChunk(fmt.Sprintf("__TOOL_RESULT:%s__\n", fn))
		}
		toolMsg := fmt.Sprintf("[Tool result: %s]\n%s\n\nPlease continue.", fn, res)
		if isToolError(res) {
			toolMsg = fmt.Sprintf("[Tool error: %s]\n%s\n\nFix this and retry with a different approach or corrected parameters.", fn, res)
			toolErrors = append(toolErrors, fmt.Sprintf("%s: %s", fn, res))
		}
		s.history = append(s.history, model.Message{Role: "user", Content: toolMsg})
		s.mu.Unlock()
	}

	s.mu.Lock()
	s.history = append(s.history, model.Message{
		Role:    "user",
		Content: "You've reached the iteration limit. Briefly explain (1-2 sentences) why you couldn't complete this task and what the main blocker was.",
	})
	finalHistory := make([]model.Message, len(s.history))
	copy(finalHistory, s.history)
	s.mu.Unlock()

	explanation, err := s.client.Send(ctx, s.model, finalHistory)
	if err == nil {
		explanation = cleanReply(explanation)
		return "[MAX_ITERATIONS]\n" + explanation, nil
	}

	msg := "[MAX_ITERATIONS]\nCouldn't complete the task after multiple attempts."
	if len(toolErrors) > 0 {
		msg = msg + "\n\nErrors encountered:\n" + strings.Join(toolErrors, "\n")
	}
	return msg, nil
}

func (s *AgentSession) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = []model.Message{{Role: "system", Content: buildSystemPrompt(s.registry, s.isWeb)}}
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
	if t.Secure && senderID != Cfg.OwnerID && senderID != "web_"+Cfg.OwnerID {
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
	r := strings.TrimSpace(strings.ToLower(result))
	return strings.HasPrefix(r, "error:") ||
		strings.HasPrefix(r, "{\"error\"") ||
		strings.Contains(r, "unknown tool") ||
		strings.Contains(r, "access denied") ||
		strings.Contains(r, "permission denied") ||
		strings.Contains(r, "not found") ||
		strings.Contains(r, "failed") ||
		strings.Contains(r, "error") ||
		strings.Contains(r, "restricted") ||
		strings.Contains(r, "denied") ||
		strings.Contains(r, "invalid") ||
		strings.Contains(r, "failed") ||
		strings.Contains(r, "cannot") ||
		strings.Contains(r, "couldn't")
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
	isWeb := strings.HasPrefix(key, "web_")
	s = NewAgentSession(GlobalRegistry, Cfg.DefaultModel, isWeb)
	if isWeb {
		sessionID := strings.TrimPrefix(key, "web_")
		if hist := LoadSession(sessionID); len(hist) > 0 {
			s.mu.Lock()
			s.history = append(s.history, hist...)
			s.mu.Unlock()
		}
	}
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

type parsedToolCall struct {
	funcName string
	argsJSON string
}

func isValidToolCall(funcName string, attrs map[string]string) bool {
	if funcName == "" {
		return false
	}
	if len(funcName) > 100 || !regexp.MustCompile(`^[a-zA-Z_]\w*$`).MatchString(funcName) {
		return false
	}
	if len(attrs) > 50 {
		return false
	}
	return true
}

func parseToolCall(text string) (funcName, argsJSON string, ok bool) {
	m := toolCallRe.FindStringSubmatch(text)
	if m == nil {
		return "", "", false
	}
	inner := strings.TrimSpace(m[1])
	if len(inner) > 10000 {
		return "", "", false
	}
	parts := strings.SplitN(inner, " ", 2)
	funcName = strings.TrimSpace(parts[0])
	attrsStr := ""
	if len(parts) > 1 {
		attrsStr = parts[1]
	}
	attrs := attrRe.FindAllStringSubmatch(attrsStr, -1)
	kv := make(map[string]string, len(attrs))
	for _, a := range attrs {
		if len(a) >= 3 {
			key := strings.TrimSpace(a[1])
			val := strings.TrimSpace(a[2])
			if len(key) > 100 || len(val) > 100000 {
				continue
			}
			kv[key] = val
		}
	}
	if !isValidToolCall(funcName, kv) {
		return "", "", false
	}
	b, _ := json.Marshal(kv)
	return funcName, string(b), true
}

func parseAllToolCalls(text string) []parsedToolCall {
	matches := toolCallRe.FindAllStringSubmatch(text, -1)
	result := make([]parsedToolCall, 0, len(matches))
	for _, m := range matches {
		inner := strings.TrimSpace(m[1])
		if len(inner) > 10000 {
			continue
		}
		parts := strings.SplitN(inner, " ", 2)
		funcName := strings.TrimSpace(parts[0])
		attrsStr := ""
		if len(parts) > 1 {
			attrsStr = parts[1]
		}
		attrs := attrRe.FindAllStringSubmatch(attrsStr, -1)
		kv := make(map[string]string, len(attrs))
		for _, a := range attrs {
			if len(a) >= 3 {
				key := strings.TrimSpace(a[1])
				val := strings.TrimSpace(a[2])
				if len(key) > 100 || len(val) > 100000 {
					continue
				}
				kv[key] = val
			}
		}
		if !isValidToolCall(funcName, kv) {
			continue
		}
		b, _ := json.Marshal(kv)
		result = append(result, parsedToolCall{
			funcName: funcName,
			argsJSON: string(b),
		})
	}
	return result
}
