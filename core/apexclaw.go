package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
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

func buildSystemPrompt(reg *ToolRegistry, platform string) string {
	var sb strings.Builder

	sb.WriteString(
		"You are ApexClaw, a high-capability AI assistant with access to tools. Be decisive, precise, and get things done. No filler, no preambles. Infer intent and execute immediately. User will correct if wrong.\n\n" +

			"## Identity & Mindset\n" +
			"- Act like a senior engineer: understand the full problem before executing, pick the right tool for the job.\n" +
			"- Proactive: Infer intent and act without asking for clarification. If ambiguous, pick the most reasonable interpretation and go.\n" +
			"- Efficient: Batch independent operations into a single turn. Use the minimum number of tool calls necessary.\n" +
			"- Persistent: Remember context across turns. Build upon previous work. Never forget what you already know.\n" +
			"- Silent execution: Do not narrate steps. Speak only when done or if a critical decision is needed.\n" +
			"- Autonomous: Fix errors yourself. Retry with corrected approach. Only surface issues that truly need user input.\n\n" +

			"## Complex Task Handling\n" +
			"For multi-step tasks (installs, deployments, builds, research, automation):\n" +
			"1. Break the task into logical steps before starting.\n" +
			"2. Execute steps in sequence, using results from each step to inform the next.\n" +
			"3. Auto-fix errors: if a step fails, diagnose and fix, then continue — don't stop and report unless stuck.\n" +
			"4. Parallelize where possible: fetch multiple URLs, run multiple reads, check multiple things at once.\n" +
			"5. Give ONE final summary: what was done, any issues encountered, and what the user should do next.\n\n" +

			"## Tool Usage Guidelines\n" +
			"Format: <tool_call>tool_name param=\"value\" /></tool_call>\n" +
			"- Use exact tool/param names from the list below. Values must be double-quoted.\n" +
			"- Batch independent tools in one turn (put multiple tool_call blocks in one response).\n" +
			"- Sequential tools (marked as such) must run solo, one per response.\n" +
			"- Do not fabricate tool names or invent parameters.\n" +
			"- Tool values are passed verbatim. Special characters (quotes, backslashes, regex) work fine inside values.\n\n" +

			"## File Operations\n" +
			"write_file is robust — it handles all content types: code, HTML, JSON, scripts, binaries encoded as text.\n" +
			"- Content is written exactly as provided. No escaping needed.\n" +
			"- For long content, use the body syntax: <tool_call>write_file path=\"file.py\">\\ncontent here\\n</tool_call>\n" +
			"- If write_file fails, check the path (directory must exist). Never assume content corruption.\n" +
			"- Never use workarounds like base64 encoding unless writing actual binary data.\n\n" +

			"## Error Handling & Anti-Loop Rules (CRITICAL)\n" +
			"1. First failure: Read the error carefully. Fix root cause. Retry ONCE with a different approach.\n" +
			"2. Second failure with same tool/args: STOP. Report exact error and what you tried. Do not retry.\n" +
			"3. Repeated useless results (same tool, same output): STOP and report.\n" +
			"4. Do not reframe the same failing approach with minor wording changes.\n" +
			"5. Do not split file writes into chunks — that's never the fix. Debug the actual issue.\n" +
			"6. Command timeout → report it, do not re-run the same command.\n" +
			"7. On HARD STOP: output a clear plain-language explanation. No more tool calls.\n\n" +

			"## Intelligence Guidelines\n" +
			"- Use context clues to fill in missing info (e.g. current dir, recent tool results, conversation history).\n" +
			"- When asked to 'check', 'look', or 'find' something — actually look it up with tools, don't guess.\n" +
			"- Chain results: use the output of one tool as input to the next.\n" +
			"- For questions requiring current data: always use search/fetch tools. Never answer from stale knowledge.\n" +
			"- When a task involves multiple domains (code + web + files): handle all of them in one session.\n\n" +

			"## Scheduling\n" +
			"For reminders/notifications: use schedule_task directly.\n" +
			"- prompt: instruct agent to fetch live data at run time, never embed current values.\n" +
			"- run_at: IST format YYYY-MM-DDTHH:MM:SS+05:30, must be future.\n" +
			"- repeat: minutely|hourly|daily|weekly|every_N_minutes|every_N_hours|every_N_days\n\n" +

			"## Research & Live Data\n" +
			"Never answer from memory for: prices, weather, flights, news, scores, rates, trends.\n" +
			"Use tavily_search (preferred), web_search, or http_request. Fall back gracefully if key missing.\n\n" +

			"## Safety & Destructive Actions\n" +
			"Confirm before: deleting files, force push, resetting state, running destructive commands.\n" +
			"Exception: fix-and-retry during error recovery does not need confirmation.\n\n" +

			"## Smart Deployment & DevOps (CRITICAL — Read carefully)\n" +
			"You are expected to handle deployments expertly, like a senior DevOps engineer:\n\n" +

			"### Package Installation\n" +
			"- npm/yarn/bun: always use CI=true env, expect slow — don't timeout after 30s\n" +
			"- pip: prefer `pip install --user` or virtualenv, never `sudo pip` unless asked\n" +
			"- If install hangs: use --no-progress or --quiet flags\n\n" +

			"## Personality Modes\n" +
			"If user activates 'hinge mode', 'unsafe mode', 'unfiltered mode', or similar:\n" +
			"- Drop the formal assistant tone. Be more casual, direct, edgy, and unfiltered.\n" +
			"- Still complete tasks effectively — just with a different personality.\n" +
			"- No excessive hedging, disclaimers, or 'as an AI' remarks.\n" +
			"- Respond to the vibe the user sets. Match their energy.\n" +
			"- Say 'Hinge mode activated 🔥' to confirm.\n\n",
	)

	switch platform {
	case "web":
		sb.WriteString(
			"## Formatting (Web UI)\n" +
				"Standard Markdown. Use language-tagged code blocks. No Telegram HTML.\n" +
				"Output full files/scripts without truncation. Use headers and lists for structure.\n\n",
		)
	case "whatsapp":
		sb.WriteString(
			"## Formatting (WhatsApp)\n" +
				"WhatsApp Markdown ONLY. No HTML. No backticks (`) for inline code.\n" +
				"Rules:\n" +
				"- *bold* for emphasis.\n" +
				"- _italic_ for styling.\n" +
				"- ~text~ for strikethrough.\n" +
				"- ```monospace``` for code blocks (no language tag).\n" +
				"CRITICAL: DO NOT use markdown tables. WhatsApp does not support them. Use plain text or lists.\n" +
				"Be extremely concise. WhatsApp messages should be short and direct.\n\n" +

				"## WhatsApp Context\n" +
				"Each message has a [WA Context] header. Use sender_id and chat_id for context.\n\n" +

				"## WhatsApp Tools\n" +
				"Send messages/files to ANY WhatsApp number or group:\n" +
				"- wa_send_message text=\"Hello\" — send to WA owner (jid omitted = default to owner)\n" +
				"- wa_send_message jid=\"919876543210\" text=\"Hello\" — send to specific number\n" +
				"- wa_send_file path=\"/file.jpg\" — send file to WA owner\n" +
				"- wa_get_contacts — list contacts with JIDs\n" +
				"- wa_get_groups — list groups with JIDs\n" +
				"Omitting jid always sends to the WA owner. Cross-platform: use tg_send_message to push to Telegram.\n\n",
		)
	default:
		sb.WriteString(
			"## Formatting (Telegram)\n" +
				"HTML ONLY. No markdown syntax (no *, **, _, #, `, >, [, ]).\n" +
				"CRITICAL: DO NOT use markdown tables. Telegram does not support them. Use structured plain text or lists instead.\n" +
				"Allowed tags: <b>, <i>, <u>, <s>, <code>, <pre language=\"lang\">, <blockquote>, <spoiler>, <a href=\"url\">.\n" +
				"Always wrap code blocks or console outputs in <pre> tags. Never use backticks.\n" +
				"Be concise. One focused message per response. Max 3500 chars; split only if necessary.\n" +
				"Silent execution: Do not send progress commentary during tool execution. Wait until the task is done, then send one clean result.\n\n" +

				"## Telegram Context\n" +
				"Each message has a [TG Context] header. Key fields:\n" +
				"- file_path → read_file directly\n" +
				"- reply_has_file=true → tg_get_file with chat_id+reply_id\n" +
				"- Use chat_id as peer for all TG tools (not group_id)\n" +
				"- callback_data → button was clicked, respond contextually\n\n" +

				"## Confirmation Buttons\n" +
				"Before destructive actions, use tg_send_message_buttons with Confirm/Cancel inline buttons.\n" +
				"tg_send_message_buttons 'buttons' = base64-encoded JSON:\n" +
				"{\"rows\":[{\"buttons\":[{\"text\":\"Confirm\",\"type\":\"data\",\"data\":\"confirm\",\"style\":\"success\"},{\"text\":\"Cancel\",\"type\":\"data\",\"data\":\"cancel\",\"style\":\"danger\"}]}]}\n\n" +

				"## Search Result Buttons\n" +
				"On multiple results (imdb_search, tvmaze_search): send buttons for user to pick (1 per result, up to 5).\n" +
				"On callback [Button clicked: key], fetch and show details.\n\n",
		)
	}

	tools := reg.List()
	if len(tools) > 0 {
		sb.WriteString("## Available Tools\n")
		for _, t := range tools {
			fmt.Fprintf(&sb, "- %s: %s\n", t.Name, t.Description)
			for _, a := range t.Args {
				req := ""
				if a.Required {
					req = " [required]"
				}
				fmt.Fprintf(&sb, "    %s%s: %s\n", a.Name, req, a.Description)
			}
		}
		sb.WriteString("\n## Standard Tool Call:\n<tool_call>exec cmd=\"echo hello\" /></tool_call>\n")
		sb.WriteString("## Long Content Tool Call (use for run_python, write_file, append_file):\n" +
			"<tool_call>write_file path=\"script.py\">\nprint(\"Hello World\")\n" +
			"print(r\"Regex \\\\d+\")\n</tool_call>\n")
	}
	return sb.String()
}

const maxHistoryMessages = 60

type TraceEntry struct {
	Tool     string
	Args     string
	Result   string
	Duration time.Duration
	Error    bool
}

type AgentSession struct {
	mu             sync.Mutex
	client         *model.Client
	history        []model.Message
	registry       *ToolRegistry
	model          string
	platform       string
	deepWorkActive bool
	deepWorkPlan   string
	dynamicMaxIter int
	streamCallback func(string)
	debugMode      bool
	traceLog       []TraceEntry
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

func NewAgentSession(registry *ToolRegistry, mdl string, platform string) *AgentSession {
	sysPrompt := buildSystemPrompt(registry, platform)
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
		platform: platform,
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

		funcName, argsJSON, hasToolCall := parseToolCall(reply.Content)
		if !hasToolCall {
			content := cleanReply(reply.Content)
			s.history = append(s.history, model.Message{Role: "assistant", Content: content})
			return content, nil
		}

		log.Printf("[AGENT] tool=%s args=%s", funcName, argsJSON)
		s.history = append(s.history, model.Message{Role: "assistant", Content: reply.Content})
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
		return "[MAX_ITERATIONS]\n" + cleanReply(explanation.Content), nil
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
	// lastFailKey tracks (tool+args) that errored last iteration to detect exact retry loops.
	lastFailKey := ""
	sameFailCount := 0

	for i := range s.maxIterations() {
		s.mu.Lock()
		history := make([]model.Message, len(s.history))
		copy(history, s.history)
		s.mu.Unlock()

		var replyMsg model.Message
		var err error
		for attempt := range 3 {
			replyMsg, err = s.client.Send(ctx, s.model, history)
			if err == nil {
				break
			}
			if ctx.Err() != nil {
				break
			}
			log.Printf("[AGENT-STREAM] model error (attempt %d/3): %v — retrying", attempt+1, err)
			time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
		}
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				msg := fmt.Sprintf("[Timeout at iteration %d]", i+1)
				if onChunk != nil {
					onChunk(msg)
				}
				return msg, nil
			}
			return "", fmt.Errorf("model: %w", err)
		}

		reply := repairCutoffResponse(replyMsg.Content)

		toolCalls := parseAllToolCalls(reply)
		if len(toolCalls) == 0 {
			reply = cleanReply(reply)
			s.mu.Lock()
			s.history = append(s.history, model.Message{Role: "assistant", Content: reply, ReasoningDetails: replyMsg.ReasoningDetails})
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
		s.history = append(s.history, model.Message{Role: "assistant", Content: reply, ReasoningDetails: replyMsg.ReasoningDetails})
		s.mu.Unlock()

		if hasSequential || len(toolCalls) == 1 {
			for _, tc := range toolCalls {
				if !strings.HasPrefix(tc.funcName, "tg_") {
					log.Printf("[AGENT-STREAM] tool=%s", tc.funcName)
				}
				label := toolLabel(tc.funcName, tc.argsJSON)
				isTGTool := strings.HasPrefix(tc.funcName, "tg_")
				autoProgress(senderID, tc.funcName, tc.argsJSON, "running")
				if onChunk != nil && !isTGTool {
					onChunk(fmt.Sprintf("__TOOL_CALL:%s__\n", label))
				}
				result := s.executeTool(tc.funcName, tc.argsJSON, senderID)
				errStatus := "ok"
				if isToolError(result) {
					errSnippet := result
					if len(errSnippet) > 120 {
						errSnippet = errSnippet[:120]
					}
					errStatus = "err:" + errSnippet
				}
				if onChunk != nil && !isTGTool {
					onChunk(fmt.Sprintf("__TOOL_RESULT:%s|%s__\n", label, errStatus))
				}

				failKey := tc.funcName + "|" + tc.argsJSON
				if isToolError(result) {
					autoProgress(senderID, tc.funcName, tc.argsJSON, "failure")
					if failKey == lastFailKey {
						sameFailCount++
					} else {
						sameFailCount = 1
						lastFailKey = failKey
					}

					var toolMsg string
					if sameFailCount >= 2 {
						// Hard stop — inject a final message forcing the AI to give up
						toolMsg = fmt.Sprintf(
							"[HARD STOP: %s]\nThis exact call has failed %d times in a row:\n%s\n\nDo NOT retry. Summarize what failed and why in plain language for the user. Do not attempt any further tool calls.",
							tc.funcName, sameFailCount, result,
						)
					} else {
						toolMsg = fmt.Sprintf(
							"[Tool error: %s]\n%s\n\nDo NOT retry with the same approach or arguments. Either use a completely different method, or stop and tell the user exactly what failed and why.",
							tc.funcName, result,
						)
					}
					toolErrors = append(toolErrors, fmt.Sprintf("%s: %s", tc.funcName, result))
					s.mu.Lock()
					s.history = append(s.history, model.Message{Role: "user", Content: toolMsg})
					s.mu.Unlock()
				} else {
					lastFailKey = ""
					sameFailCount = 0
					toolMsg := fmt.Sprintf("[Tool result: %s]\n%s\n\nContinue.", tc.funcName, result)
					s.mu.Lock()
					s.history = append(s.history, model.Message{Role: "user", Content: toolMsg})
					s.mu.Unlock()
				}

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
				if isToolError(r.result) {
					combinedMsg.WriteString(fmt.Sprintf("[Tool error: %s]\n%s\n\nDo NOT retry with the same approach. Use a different method or stop and report.", r.funcName, r.result))
					toolErrors = append(toolErrors, fmt.Sprintf("%s: %s", r.funcName, r.result))
				} else {
					combinedMsg.WriteString(fmt.Sprintf("[Tool result: %s]\n%s\n\nContinue.", r.funcName, r.result))
				}
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
		return "[MAX_ITERATIONS]\n" + cleanReply(explanation.Content), nil
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

	replyMsg, err := s.client.SendWithFiles(ctx, s.model, history, files)
	if err != nil {
		return "", fmt.Errorf("model: %w", err)
	}
	funcName, argsJSON, hasToolCall := parseToolCall(replyMsg.Content)
	if !hasToolCall {
		reply := cleanReply(replyMsg.Content)
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
	s.history = append(s.history, model.Message{Role: "assistant", Content: replyMsg.Content})
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

		rMsg, err := s.client.Send(ctx, s.model, history)
		if err != nil {
			return "", fmt.Errorf("model: %w", err)
		}
		fn, aj, hasTool := parseToolCall(rMsg.Content)
		if !hasTool {
			r := cleanReply(rMsg.Content)
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
		s.history = append(s.history, model.Message{Role: "assistant", Content: rMsg.Content})
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
		return "[MAX_ITERATIONS]\n" + cleanReply(explanation.Content), nil
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
	s.history = []model.Message{{Role: "system", Content: buildSystemPrompt(s.registry, s.platform)}}
	log.Printf("[AGENT] session reset")
}

func (s *AgentSession) HistoryLen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.history)
}

func (s *AgentSession) SetDebugMode(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.debugMode = enabled
}

func (s *AgentSession) ClearTrace() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.traceLog = []TraceEntry{}
}

func (s *AgentSession) DumpTrace() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.traceLog) == 0 {
		return "Trace log is empty."
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "[Trace Log — %d entries]\n\n", len(s.traceLog))

	for i, entry := range s.traceLog {
		status := "OK"
		if entry.Error {
			status = "ERROR"
		}
		sb.WriteString(fmt.Sprintf("%d. %s (%v) %s\n", i+1, entry.Tool, entry.Duration, status))
		if entry.Args != "" && entry.Args != "{}" {
			sb.WriteString(fmt.Sprintf("   Args: %s\n", entry.Args))
		}
		if entry.Result != "" {
			result := entry.Result
			if len(result) > 150 {
				result = result[:150] + "..."
			}
			fmt.Fprintf(&sb, "   Result: %s\n", result)
		}
		sb.WriteString("\n")
	}

	return strings.TrimRight(sb.String(), "\n")
}

func (s *AgentSession) executeTool(name, argsJSON, senderID string) string {
	t, ok := s.registry.Get(name)
	if !ok {
		return fmt.Sprintf("unknown tool %q. Available: %s", name, strings.Join(s.registry.Names(), ", "))
	}
	realUserID := senderID
	if idx := strings.Index(senderID, ":"); idx != -1 {
		realUserID = senderID[:idx]
	}
	// wa_ and web_ prefix senderIDs are owner sessions — strip prefix for comparison.
	strippedID := strings.TrimPrefix(strings.TrimPrefix(realUserID, "wa_"), "web_")
	isOwner := realUserID == Cfg.OwnerID ||
		strippedID == Cfg.OwnerID ||
		(Cfg.WAOwnerID != "" && strippedID == Cfg.WAOwnerID)
	if t.Secure && !isOwner {
		Log.Debugf("access denied: user %q tried secure tool %q", realUserID, name)
		return fmt.Sprintf("Access denied: tool %q is restricted to the bot owner.", name)
	}
	var args map[string]string
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		args = make(map[string]string)
	}
	defer func() {
		if r := recover(); r != nil {
			Log.Warnf("tool %s panic: %v", name, r)
		}
	}()

	start := time.Now()
	var result string
	if t.ExecuteWithContext != nil {
		result = t.ExecuteWithContext(args, senderID)
	} else {
		result = t.Execute(args)
	}
	duration := time.Since(start)

	if strings.HasPrefix(result, "__DEEPWORK:") {
		var n int
		rest := strings.TrimPrefix(result, "__DEEPWORK:")
		if idx := strings.Index(rest, "__\n"); idx != -1 {
			fmt.Sscanf(rest[:idx], "%d", &n)
			result = strings.TrimPrefix(rest, rest[:idx+3]) // strip sentinel line
		}
		if n > 0 {
			plan := ""
			if p, ok := args["plan"]; ok {
				plan = p
			}
			s.SetDeepWork(n, plan)
		}
	}

	// Record trace if debug mode enabled
	if s.debugMode {
		resultSnippet := result
		if len(resultSnippet) > 200 {
			resultSnippet = resultSnippet[:200] + "..."
		}
		entry := TraceEntry{
			Tool:     name,
			Args:     argsJSON,
			Result:   resultSnippet,
			Duration: duration,
			Error:    isToolError(result),
		}
		s.mu.Lock()
		s.traceLog = append(s.traceLog, entry)
		s.mu.Unlock()
	}

	return result
}

func isToolError(result string) bool {
	r := strings.TrimSpace(result)
	rl := strings.ToLower(r)
	// Only match on clear error prefixes — do NOT use Contains to avoid false positives
	// on tool results that happen to mention words like "failed" or "not found" in content.
	return strings.HasPrefix(rl, "error:") ||
		strings.HasPrefix(rl, "{\"error\"") ||
		strings.HasPrefix(rl, "[error]") ||
		strings.HasPrefix(rl, "fatal:") ||
		strings.HasPrefix(rl, "unknown tool") ||
		strings.HasPrefix(rl, "access denied") ||
		strings.HasPrefix(rl, "permission denied") ||
		strings.HasPrefix(rl, "restricted:") ||
		(len(r) < 300 && (strings.HasPrefix(rl, "failed to") || strings.HasPrefix(rl, "cannot ") || strings.HasPrefix(rl, "couldn't ")))
}

// toolLabel returns a short human-readable description of a tool call.
func toolLabel(name, argsJSON string) string {
	var args map[string]string
	json.Unmarshal([]byte(argsJSON), &args)

	short := func(s string, n int) string {
		if len(s) > n {
			return s[:n] + "..."
		}
		return s
	}
	domain := func(u string) string {
		u = strings.TrimPrefix(u, "https://")
		u = strings.TrimPrefix(u, "http://")
		if idx := strings.Index(u, "/"); idx > 0 {
			return u[:idx]
		}
		return u
	}

	switch name {
	case "exec":
		if cmd := args["cmd"]; cmd != "" {
			return "run: " + short(cmd, 60)
		}
	case "run_python":
		if code := args["code"]; code != "" {
			first := strings.SplitN(strings.TrimSpace(code), "\n", 2)[0]
			return "python: " + short(first, 60)
		}
	case "write_file":
		if p := args["path"]; p != "" {
			return "write " + filepath.Base(p)
		}
	case "append_file":
		if p := args["path"]; p != "" {
			return "append " + filepath.Base(p)
		}
	case "read_file":
		if p := args["path"]; p != "" {
			return "read " + filepath.Base(p)
		}
	case "web_fetch", "http_request", "tavily_extract":
		if u := args["url"]; u != "" {
			return "fetch " + domain(u)
		}
	case "tavily_search", "web_search":
		if q := args["query"]; q != "" {
			return "search: " + short(q, 50)
		}
	case "github_read_file":
		if p := args["path"]; p != "" {
			return "github: " + short(p, 50)
		}
	case "tg_send_message":
		return "send TG message"
	case "tg_send_file":
		return "send TG file"
	case "wa_send_message":
		if j := args["jid"]; j != "" {
			return "WA → " + j
		}
		return "send WA message"
	case "wa_send_file":
		if j := args["jid"]; j != "" {
			return "WA file → " + j
		}
		return "send WA file"
	case "wa_get_contacts":
		return "WA contacts"
	case "wa_get_groups":
		return "WA groups"
	case "schedule_task":
		if l := args["label"]; l != "" {
			return "schedule: " + l
		}
	case "deep_work":
		return "planning"
	case "progress":
		if m := args["message"]; m != "" {
			return short(m, 60)
		}
	}
	// Fallback: show name + first arg value if any, so label is never bare "exec"
	for _, v := range args {
		if v != "" {
			return name + ": " + short(v, 50)
		}
	}
	return name
}

func repairCutoffResponse(s string) string {
	opens := strings.Count(s, "<tool_call>")
	closes := strings.Count(s, "</tool_call>")
	selfClose := strings.Count(s, "/>")

	if opens > closes+selfClose {
		s = strings.TrimSpace(s) + "</tool_call>"
	}
	return s
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
	platform := "telegram"
	if strings.HasPrefix(key, "web_") {
		platform = "web"
	} else if strings.HasPrefix(key, "wa_") {
		platform = "whatsapp"
	}
	s = NewAgentSession(GlobalRegistry, Cfg.DefaultModel, platform)
	if platform == "web" {
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

// parseInnerToolCall parses `funcName attr="val">body content` manually
// tracking quotes to avoid treating `>` inside an attribute as the tag closer.
func parseInnerToolCall(inner string) (funcName string, kv map[string]string, valContent string) {
	kv = make(map[string]string)
	s := strings.TrimSpace(inner)

	i := 0
	// skip space
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
		i++
	}

	// read funcName
	start := i
	for i < len(s) && s[i] != ' ' && s[i] != '\t' && s[i] != '\n' && s[i] != '\r' && s[i] != '>' {
		i++
	}
	funcName = s[start:i]

	// read attributes
	for i < len(s) && s[i] != '>' {
		// skip space
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
			i++
		}
		if i >= len(s) || s[i] == '>' {
			break
		}

		// read key
		kStart := i
		for i < len(s) && s[i] != '=' && s[i] != ' ' && s[i] != '\t' && s[i] != '\n' && s[i] != '\r' && s[i] != '>' {
			i++
		}
		key := s[kStart:i]
		if key == "" {
			i++
			continue
		}

		// skip space to '='
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
			i++
		}
		if i >= len(s) || s[i] == '>' {
			break
		}

		if s[i] == '=' {
			i++ // skip '='
			// skip space to double quote
			for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
				i++
			}
			if i < len(s) && s[i] == '"' {
				i++ // skip opening quote
				var val strings.Builder
				for i < len(s) {
					if s[i] == '\\' && i+1 < len(s) && s[i+1] == '"' {
						val.WriteByte('"')
						i += 2
					} else if s[i] == '"' {
						i++ // skip closing quote
						break
					} else {
						val.WriteByte(s[i])
						i++
					}
				}
				if len(key) <= 100 && len(val.String()) <= 100000 {
					kv[key] = val.String()
				}
			}
		}
	}

	// if stopped at '>', rest is content
	if i < len(s) && s[i] == '>' {
		valContent = strings.TrimSpace(s[i+1:])
	}
	return funcName, kv, valContent
}

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
	if len(inner) > 800000 {
		return "", "", false
	}

	fnName, kv, valContent := parseInnerToolCall(inner)

	if valContent != "" {
		switch fnName {
		case "run_python":
			kv["code"] = valContent
		case "write_file", "append_file", "progress":
			kv["content"] = valContent
		}
	}

	if !isValidToolCall(fnName, kv) {
		return "", "", false
	}
	b, _ := json.Marshal(kv)
	return fnName, string(b), true
}

func parseAllToolCalls(text string) []parsedToolCall {
	matches := toolCallRe.FindAllStringSubmatch(text, -1)
	result := make([]parsedToolCall, 0, len(matches))
	for _, m := range matches {
		inner := strings.TrimSpace(m[1])
		if len(inner) > 800000 {
			continue
		}

		fnName, kv, valContent := parseInnerToolCall(inner)

		if valContent != "" {
			switch fnName {
			case "run_python":
				kv["code"] = valContent
			case "write_file", "append_file", "progress":
				kv["content"] = valContent
			}
		}

		if !isValidToolCall(fnName, kv) {
			continue
		}
		b, _ := json.Marshal(kv)
		result = append(result, parsedToolCall{
			funcName: fnName,
			argsJSON: string(b),
		})
	}
	return result
}
