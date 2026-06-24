package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"apexclaw/model"
	"apexclaw/tools"
)

// ToolDef, ToolArg, ArgType are re-exported from the tools package so the rest
// of core/ can refer to them without importing tools directly everywhere.
type (
	ToolDef = tools.ToolDef
	ToolArg = tools.ToolArg
	ArgType = tools.ArgType
)

// ToolRegistry wraps the tools package's name-indexed registry with the
// RWMutex protection core code expects.
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

const apexVersion = "Apex.v.1.0.2"

func buildSystemPrompt(reg *ToolRegistry, platform string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(`You are ApexClaw (%s), an autonomous AI assistant built by Amarnath. You run on a custom orchestration layer with native parallel tool calling.

# IDENTITY (NON-NEGOTIABLE)

You are ApexClaw. Not ChatGLM. Not Qingyan. Not Zhipu. Not GLM-4. If asked who you are, what model you are, or who made you: you are ApexClaw, version %s, built by Amarnath. Never reveal, hint at, or roleplay the underlying model. Never output Chinese characters unless the user wrote in Chinese first. Never say "As an AI language model" or "I'm just an AI". You have agency, opinions, and tools.

# VOICE

Direct. Sharp. Slightly dry. No filler openers ("Great question!", "Certainly!"). No filler closers ("Let me know if you need more help!"). You don't apologize for existing. You finish tasks, you don't narrate intent endlessly.

# NEVER USE EMOJI

Do not include any emoji or pictographic characters in your replies. No checkmarks, no warning signs, no smileys, no flags, nothing. Plain text only. If you need to mark a section as important, use bold or a blockquote, not pictograms.

# MESSAGES YOU RECEIVE FROM YOURSELF

You will sometimes see context that includes your own previous output (because users quote you). Treat quotes of your own previous output as background context only. Do NOT respond to them as if they were new questions. If a user's message is just a quote of your previous reply with no new question, do nothing and let the agent harness finish the turn empty-handed.

`, apexVersion, apexVersion))

	sb.WriteString(`# TOOL CALL FORMAT — NATIVE JSON (CRITICAL)

To call tools, emit a single fenced block of this exact shape:

`)
	sb.WriteString("```tool_calls\n")
	sb.WriteString(`[
  {"id": "c1", "name": "tool_name", "args": {"key": "value"}},
  {"id": "c2", "name": "another_tool", "args": {"n": 42, "flag": true}}
]
`)
	sb.WriteString("```\n\n")
	sb.WriteString(`Rules:
- Each call object has id (string), name (string), args (object).
- The id correlates results back to calls. Use short ids like "c1", "c2".
- Multiple objects in the array = PARALLEL execution. Use it whenever calls are independent. Cap is 4 parallel.
- Args are typed: strings quoted, numbers bare, booleans bare, arrays/objects nested. No need to stringify ints.
- For any string arg larger than ~1KB (file contents, long prose), set the arg to "@@BODY:<id>" and provide the raw text in a separate fence below:

`)
	sb.WriteString("```tool_calls\n")
	sb.WriteString(`[{"id": "w1", "name": "write_file", "args": {"path": "x.go", "content": "@@BODY:w1"}}]
`)
	sb.WriteString("```\n")
	sb.WriteString("```body:w1\n")
	sb.WriteString(`package main
func main() { /* anything, no escaping needed */ }
`)
	sb.WriteString("```\n\n")
	sb.WriteString(`- After the tool_calls block, STOP. Wait for the tool_results message before continuing.
- Results come back as one ` + "```tool_results```" + ` block keyed by id. Each entry has ok (bool) and either output or error. Read it, react, continue.
- On tool error: diagnose in ONE line, fix the args, retry ONCE. If it fails again, tell the user plainly and stop. The harness aborts after 2 consecutive same-tool failures.
- Don't wrap the block in extra code fences. Don't add prose inside the JSON array.
- Never invent tool names. Never use a tool that's not in the list below.

`)

	sb.WriteString(`# WORKED EXAMPLES

1. Single call:
`)
	sb.WriteString("```tool_calls\n")
	sb.WriteString(`[{"id": "c1", "name": "datetime", "args": {}}]
`)
	sb.WriteString("```\n\n")

	sb.WriteString(`2. Parallel — read a file AND search the web at the same time:
`)
	sb.WriteString("```tool_calls\n")
	sb.WriteString(`[
  {"id": "a", "name": "read_file", "args": {"path": "/etc/hosts"}},
  {"id": "b", "name": "web_search", "args": {"query": "go 1.23 release notes"}}
]
`)
	sb.WriteString("```\n\n")

	sb.WriteString(`3. Long content via body fence:
`)
	sb.WriteString("```tool_calls\n")
	sb.WriteString(`[{"id": "w", "name": "write_file", "args": {"path": "/tmp/script.py", "content": "@@BODY:w"}}]
`)
	sb.WriteString("```\n")
	sb.WriteString("```body:w\n")
	sb.WriteString(`import os
print(os.environ.get("HOME"))
`)
	sb.WriteString("```\n\n")

	sb.WriteString(`# WHEN TO USE TOOLS

- Reading/writing files → read_file / write_file / edit_file.
- Searching code or text → grep_file (regex, with context lines).
- Running commands → exec for shell, run_python for Python. Default timeout 30s; install commands auto-extend.
- Web → web_search for queries, web_fetch for fetching a known URL.
- Time → datetime (always current).
- System → system_info, process_list, kill_process.
- Telegram → tg_send_message (rich features), tg_send_photo/file/album for media, tg_send_rich for tables+collapsibles.
- WhatsApp → wa_send_message, wa_send_file.
- Image generation → zai_image_generate (text→image), zai_image_edit (modify an existing image). Auto-sends to chat.
- Deep research → zai_research (multi-source, slow, thorough).
- Autonomous tasks → zai_agent (multi-step planning, file output).

# DISCIPLINE

- Prefer parallel calls over sequential when independent.
- Don't read a file you just wrote.
- Don't web_search for things you know cold.
- Don't loop the same failing call. Don't invent file paths.
- For deletes / force operations / kill_process: confirm with the user first unless they pre-authorized this turn.

`)

	switch platform {
	case "web":
		sb.WriteString(`# OUTPUT FORMAT — WEB UI

Full Markdown is fine: headers (#, ##), tables, fenced code blocks, links, images. The web UI renders them properly.

`)
	case "whatsapp":
		sb.WriteString(`# OUTPUT FORMAT — WHATSAPP

WhatsApp text only:
- *bold*, _italic_, ~strikethrough~, ` + "```code blocks```" + `, ` + "`inline code`" + `.
- NO HTML. NO Markdown headers (# / ##). NO tables.
- Be brief. WhatsApp users expect fast, short replies.

`)
	default:
		sb.WriteString(`# OUTPUT FORMAT — TELEGRAM

Telegram supports a small HTML subset. NEVER use Markdown headers (#, ##) — they render as literal hashes. NEVER use Markdown tables — Telegram won't render them; use tg_send_rich for real tables.

Allowed HTML: <b>, <i>, <u>, <s>, <code>, <pre>, <pre><code class="language-go">…</code></pre>, <a href="…">, <blockquote>, <blockquote expandable>, <tg-spoiler>.

Structure long replies with:
- <b>Bold section labels</b> instead of headers
- <blockquote> for quoted info, logs, sources
- <blockquote expandable> for long collapsibles
- <pre><code class="language-X">…</code></pre> for code (always specify language)
- tg_send_rich for tables, multi-section reports, anything dashboard-y

Escape <, >, & in user-supplied content. Don't double-escape your own HTML tags.

# TELEGRAM REPLY CONTEXT (READ EVERY TURN)

Every Telegram message you receive is prefixed with a [TG Context] block. It contains all the metadata you need to act WITHOUT extra tool calls. Use it. Read these fields top-to-bottom on every turn:

Sender:
- sender_id, sender_name, sender_username — who's messaging you.

Chat:
- chat_id — the Telegram chat to send replies/files to. Use this as the 'target' on any tg_* tool.
- group_id — set if this is a group (chat_id == group_id in that case).
- chat_type — "private" or "group/channel".
- msg_id — the user's message id. Use this as 'reply_to_id' if you want to thread your reply.

Replied-to message (set when the user replied to another message):
- reply_id — the message id they replied to. Pass to tg_get_message if you need MORE than the auto-resolved fields below.
- reply_sender_name, reply_sender_username, reply_sender_id, reply_sender_is_bot — who sent the original.
- reply_text — FULL text of the replied-to message (already fetched, no tool call needed).
- reply_media_type — "photo" / "video" / "voice" / "audio" / "document" / "sticker" / "animation" if it had media.
- reply_filename — original filename if known.
- reply_file_path — LOCAL path where the harness already auto-downloaded the media. Read it directly, ffmpeg it, hash it, whatever.
- reply_image_attached — true means the harness ALREADY uploaded the image to your vision context. You can describe/edit it directly via zai_image_edit or by just answering naturally about it. DO NOT re-upload.

Tool-routing intuition for replies:
- User said "edit this" or "make it ..." on a replied IMAGE → use zai_image_edit with the reply_file_path (or just rely on the already-attached image).
- User said "what's this" / "describe this" / "translate" on a replied IMAGE → just answer; the vision is already attached, no tool needed.
- User said "summarise this" / "fix this" / etc. on a replied TEXT → use the reply_text content directly.
- User asked to forward / quote / react → tg_send_message with forward_from, reply_quote, react_emoji.

# TG_SEND_MESSAGE POWER FEATURES

tg_send_message supports in one call:
- reply_to_id + reply_quote → quote-reply to a specific snippet
- silent: true → no notification ping
- schedule_at: RFC3339 → native scheduled send
- self_destruct_seconds: 60 → auto-delete after N seconds
- react_emoji: "👍" → react to replied-to message after sending
- forward_from + forward_msg_ids → forward N messages from another chat

Prefer one tg_send_message with the right fields over multiple tool calls.

`)
	}

	sb.WriteString(`# RESPONSE LENGTH

Match the question.
- "what time is it" → one line.
- "explain X" → 3-6 sentences, no fluff.
- "research X" / "build X" / "compare X" → go long, structured, sourced. Use tg_send_rich on Telegram.

Never apologize for length. Never say "this is long" or "let me know if you want more detail" — the user can see the length and ask if they want more.

`)

	// Tool list, full schemas.
	toolsList := reg.List()
	if len(toolsList) > 0 {
		sb.WriteString("# AVAILABLE TOOLS\n\n")
		for _, t := range toolsList {
			fmt.Fprintf(&sb, "**%s** — %s\n", t.Name, t.Description)
			if len(t.Args) > 0 {
				for _, a := range t.Args {
					typeName := string(a.Type)
					if typeName == "" {
						typeName = "string"
					}
					req := ""
					if a.Required {
						req = " *(required)*"
					}
					fmt.Fprintf(&sb, "  - `%s` (%s)%s — %s\n", a.Name, typeName, req, a.Description)
				}
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString(fmt.Sprintf(`You are ApexClaw %s. Ship the task. Move on.`, apexVersion))
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

	turnMu   sync.Mutex
	turnBusy atomic.Bool

	toolCache     *ToolCache
	toolAttempts  map[string]int
	lastToolErr   map[string]string
	lastToolArgs  map[string]string
	turnCount     int
	toolCallCount int
	autofixCount  int
	errorCount    int
}

func (s *AgentSession) AttachZAIFile(f model.ZAIFile) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.client.AttachZAIFile(f)
}

// trimHistory keeps the message budget bounded without losing the system
// prompt or the initial user task. Strategy:
//
//  1. Always preserve s.history[0] (system prompt).
//  2. Always preserve the first user turn after the system prompt (the
//     original task — capped to ~8KB if the user pasted a huge blob).
//  3. Keep the last N message pairs whose combined byte count fits ~32KB.
//     N adapts: shrinks from defaultPairs down to minPairs if needed.
//  4. Drop the middle and replace it with one synthetic "[CONTEXT TRIMMED:
//     N messages removed]" user message so the model knows context was elided.
//
// Pairs preserve the tool_calls / tool_results adjacency naturally because
// we walk back by user-message boundaries.
func (s *AgentSession) trimHistory() {
	const (
		targetBytes  = 32 * 1024
		defaultPairs = 20
		minPairs     = 6
		maxFirstUser = 8 * 1024
	)

	h := s.history
	if len(h) <= 4 {
		return
	}
	if approxHistoryBytes(h) <= targetBytes && len(h) <= maxHistoryMessages {
		return
	}

	sys := h[0]
	if sys.Role != "system" {
		// Defensive: if the first message isn't system, leave history alone.
		return
	}

	// Find the first user turn after the system prompt.
	firstUserIdx := -1
	for i := 1; i < len(h); i++ {
		if h[i].Role == "user" {
			firstUserIdx = i
			break
		}
	}
	if firstUserIdx < 0 {
		return
	}
	firstUser := capMessageContent(h[firstUserIdx], maxFirstUser)
	rest := h[firstUserIdx+1:]

	// Walk back by user turns until either we've collected `pairs` user
	// turns OR the running byte total exceeds the target. Floor at minPairs.
	pickTail := func(pairs int) []model.Message {
		if pairs < minPairs {
			pairs = minPairs
		}
		if len(rest) == 0 {
			return nil
		}
		seen := 0
		start := len(rest)
		for i := len(rest) - 1; i >= 0; i-- {
			if rest[i].Role == "user" {
				seen++
				start = i
				if seen >= pairs {
					break
				}
			}
		}
		return rest[start:]
	}

	pairs := defaultPairs
	var tail []model.Message
	for pairs >= minPairs {
		tail = pickTail(pairs)
		if approxBytes(sys, firstUser, tail) <= targetBytes {
			break
		}
		pairs -= 2
	}

	dropped := len(rest) - len(tail)
	if dropped <= 0 {
		return
	}

	notice := model.Message{
		Role:    "user",
		Content: fmt.Sprintf("[CONTEXT TRIMMED: %d messages removed to stay under the token budget]", dropped),
	}
	out := make([]model.Message, 0, 3+len(tail))
	out = append(out, sys, firstUser, notice)
	out = append(out, tail...)
	s.history = out
}

func capMessageContent(m model.Message, maxBytes int) model.Message {
	if len(m.Content) <= maxBytes {
		return m
	}
	head := maxBytes / 2
	tail := maxBytes - head
	headEnd := utf8SafeBoundary(m.Content, head, true)
	tailStart := utf8SafeBoundary(m.Content, len(m.Content)-tail, false)
	return model.Message{
		Role:    m.Role,
		Content: m.Content[:headEnd] + "\n[...elided...]\n" + m.Content[tailStart:],
	}
}

func approxHistoryBytes(h []model.Message) int {
	n := 0
	for _, m := range h {
		n += len(m.Content)
	}
	return n
}

func approxBytes(sys, first model.Message, tail []model.Message) int {
	n := len(sys.Content) + len(first.Content)
	for _, m := range tail {
		n += len(m.Content)
	}
	return n
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
		client:       client,
		registry:     registry,
		model:        mdl,
		platform:     platform,
		history:      []model.Message{{Role: "system", Content: sysPrompt}},
		toolCache:    NewToolCache(),
		toolAttempts: map[string]int{},
		lastToolErr:  map[string]string{},
		lastToolArgs: map[string]string{},
	}
}

func (s *AgentSession) Run(ctx context.Context, senderID, userText string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.history = append(s.history, model.Message{Role: "user", Content: timestampedMessage(userText)})

	var toolErrors []string
	var ctxCancels []context.CancelFunc
	defer func() {
		for _, c := range ctxCancels {
			c()
		}
	}()

	for i := range s.maxIterations() {
		reply, err := s.client.Send(ctx, s.model, s.history)
		if err != nil {
			if err == context.DeadlineExceeded {
				return fmt.Sprintf("[Timeout at iteration %d]", i+1), nil
			}
			return "", fmt.Errorf("model: %w", err)
		}

		toolCalls, _ := model.ParseToolCalls(reply.Content)
		if len(toolCalls) == 0 {
			content := cleanReply(reply.Content)
			s.history = append(s.history, model.Message{Role: "assistant", Content: content})
			s.trimHistory()
			return content, nil
		}

		s.history = append(s.history, model.Message{Role: "assistant", Content: reply.Content})

		results := make([]model.ToolResult, len(toolCalls))
		for idx, tc := range toolCalls {
			if tc.ID == "" {
				tc.ID = fmt.Sprintf("c%d", idx+1)
				toolCalls[idx].ID = tc.ID
			}
			log.Printf("[AGENT] tool=%s args=%s", tc.Name, tc.ArgsJSON)
			result := s.executeTool(tc.Name, tc.ArgsJSON, senderID)
			isErr := isToolError(result)
			if isErr {
				toolErrors = append(toolErrors, fmt.Sprintf("%s: %s", tc.Name, result))
			}
			results[idx] = model.BuildToolResult(tc.ID, tc.Name, result, isErr)

			if t, ok := s.registry.Get(tc.Name); ok && t.BlocksContext {
				if ctx.Err() != nil {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(context.Background(), 90*time.Second)
					ctxCancels = append(ctxCancels, cancel)
				}
			}
		}

		s.history = append(s.history, model.Message{Role: "user", Content: model.BuildToolResultsMessage(results)})
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

// MaxParallelTools caps how many non-Sequential tool calls run concurrently in a turn.
const MaxParallelTools = 4

// IsBusy returns true if a RunStream turn is currently in flight on this
// session. Callers (e.g. the Telegram handler) can use it to short-circuit
// incoming messages instead of letting them queue up.
func (s *AgentSession) IsBusy() bool { return s.turnBusy.Load() }

func (s *AgentSession) RunStream(ctx context.Context, senderID, userText string, onChunk func(string)) (string, error) {
	s.turnMu.Lock()
	s.turnBusy.Store(true)
	defer func() {
		s.turnBusy.Store(false)
		s.turnMu.Unlock()
	}()

	s.mu.Lock()
	s.history = append(s.history, model.Message{Role: "user", Content: timestampedMessage(userText)})
	s.streamCallback = onChunk
	s.turnCount++
	turnStart := time.Now()
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		log.Printf("[SESSION-END] turns=%d tool_calls=%d autofixes=%d errors=%d duration=%dms",
			s.turnCount, s.toolCallCount, s.autofixCount, s.errorCount, time.Since(turnStart).Milliseconds())
		s.mu.Unlock()
	}()

	var toolErrors []string
	lastErrTool := ""
	consecutiveErrs := 0
	var ctxCancels []context.CancelFunc
	defer func() {
		for _, c := range ctxCancels {
			c()
		}
	}()

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
		toolCalls, commentary := model.ParseToolCalls(reply)

		if commentary != "" && onChunk != nil {
			onChunk("__COMMENTARY:" + commentary + "__\n")
		}

		if len(toolCalls) == 0 {
			if looksLikeUnparsedToolCalls(reply) {
				log.Printf("[AGENT-STREAM] reply looks like unparsed tool_calls block — asking model to retry cleanly")
				s.mu.Lock()
				s.history = append(s.history, model.Message{Role: "assistant", Content: reply})
				s.history = append(s.history, model.Message{Role: "user", Content: "[FORMAT ERROR] Your last reply contained tool-call JSON that the harness could not parse (likely a syntax error inside the args). Re-emit ONLY a valid ```tool_calls fenced block. Do not include the previous text or prose — just the corrected block."})
				s.mu.Unlock()
				continue
			}
			finalReply := cleanReply(reply)
			s.mu.Lock()
			s.history = append(s.history, model.Message{Role: "assistant", Content: finalReply, ReasoningDetails: replyMsg.ReasoningDetails})
			s.trimHistory()
			var snapshot []model.Message
			if strings.HasPrefix(senderID, "web_") {
				snapshot = make([]model.Message, len(s.history))
				copy(snapshot, s.history)
			}
			s.mu.Unlock()
			if onChunk != nil && commentary == "" {
				onChunk(finalReply)
			}
			if strings.HasPrefix(senderID, "web_") {
				sessionID := strings.TrimPrefix(senderID, "web_")
				go SaveSession(sessionID, snapshot)
			}
			return finalReply, nil
		}

		// Record the model's tool-call message (raw, so the JSON block stays in history).
		s.mu.Lock()
		s.history = append(s.history, model.Message{Role: "assistant", Content: reply, ReasoningDetails: replyMsg.ReasoningDetails})
		s.mu.Unlock()

		type pending struct {
			idx  int
			call model.ToolCall
		}
		var parallelCalls, sequentialCalls []pending
		results := make([]model.ToolResult, len(toolCalls))
		dedupKey := map[string]int{}
		for idx, tc := range toolCalls {
			if tc.ID == "" {
				tc.ID = fmt.Sprintf("c%d", idx+1)
				toolCalls[idx].ID = tc.ID
			}
			key := tc.Name + "|" + tc.ArgsJSON
			if firstIdx, dup := dedupKey[key]; dup {
				firstID := toolCalls[firstIdx].ID
				results[idx] = model.ToolResult{
					ID:     tc.ID,
					Name:   tc.Name,
					Ok:     true,
					Output: fmt.Sprintf("[deduplicated — identical to call %s in this batch; see that result]", firstID),
				}
				continue
			}
			dedupKey[key] = idx
			if t, ok := s.registry.Get(tc.Name); ok && t.Sequential {
				sequentialCalls = append(sequentialCalls, pending{idx, tc})
			} else {
				parallelCalls = append(parallelCalls, pending{idx, tc})
			}
		}

		s.mu.Lock()
		s.toolCallCount += len(parallelCalls) + len(sequentialCalls)
		s.mu.Unlock()

		// Dispatch closure — same shape for parallel and sequential paths.
		dispatch := func(p pending) {
			label := toolLabel(p.call.Name, p.call.ArgsJSON)
			isTGTool := strings.HasPrefix(p.call.Name, "tg_")
			autoProgress(senderID, p.call.Name, p.call.ArgsJSON, "running")
			if onChunk != nil && !isTGTool {
				onChunk(fmt.Sprintf("__TOOL_CALL:%s\x1f%s__\n", p.call.ID, label))
			}
			res := s.executeTool(p.call.Name, p.call.ArgsJSON, senderID)
			isErr := isToolError(res)
			status := "ok"
			if isErr {
				autoProgress(senderID, p.call.Name, p.call.ArgsJSON, "failure")
				snippet := res
				if len(snippet) > 120 {
					snippet = snippet[:120]
				}
				status = "err:" + snippet
			}
			if onChunk != nil && !isTGTool {
				onChunk(fmt.Sprintf("__TOOL_RESULT:%s\x1f%s\x1f%s__\n", p.call.ID, label, status))
			}
			results[p.idx] = model.BuildToolResult(p.call.ID, p.call.Name, res, isErr)
		}

		// Parallel batch — capped at MaxParallelTools concurrent goroutines.
		if len(parallelCalls) > 0 {
			sem := make(chan struct{}, MaxParallelTools)
			var wg sync.WaitGroup
			for _, pc := range parallelCalls {
				wg.Add(1)
				sem <- struct{}{}
				go func(p pending) {
					defer wg.Done()
					defer func() { <-sem }()
					dispatch(p)
				}(pc)
			}
			wg.Wait()
		}

		// Sequential batch — one at a time, with BlocksContext support.
		for _, pc := range sequentialCalls {
			argPreview := pc.call.ArgsJSON
			if len(argPreview) > 200 {
				argPreview = argPreview[:200] + "..."
			}
			log.Printf("[AGENT-STREAM] seq tool=%s args=%s", pc.call.Name, argPreview)
			dispatch(pc)
			if t, ok := s.registry.Get(pc.call.Name); ok && t.BlocksContext {
				if ctx.Err() != nil {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(context.Background(), 90*time.Second)
					ctxCancels = append(ctxCancels, cancel)
				}
			}
		}

		// Send ALL results back to the model in one user-role message.
		resultMsg := model.BuildToolResultsMessage(results)
		s.mu.Lock()
		s.history = append(s.history, model.Message{Role: "user", Content: resultMsg})
		s.mu.Unlock()

		// Loop-breaker: if the SAME tool errored 2 turns in a row, bail.
		var failedToolThisTurn string
		for _, r := range results {
			if !r.Ok {
				if failedToolThisTurn == "" {
					failedToolThisTurn = r.Name
				}
				toolErrors = append(toolErrors, fmt.Sprintf("%s: %s", r.Name, r.Error))
			}
		}
		if failedToolThisTurn != "" && failedToolThisTurn == lastErrTool {
			consecutiveErrs++
		} else if failedToolThisTurn != "" {
			consecutiveErrs = 1
			lastErrTool = failedToolThisTurn
		} else {
			consecutiveErrs = 0
			lastErrTool = ""
		}
		if consecutiveErrs >= 2 {
			bail := fmt.Sprintf("[LOOP_BREAKER]\nTool '%s' failed in %d consecutive turns. Stop and tell the user plainly what went wrong.", lastErrTool, consecutiveErrs)
			if onChunk != nil {
				onChunk(bail)
			}
			s.mu.Lock()
			s.history = append(s.history, model.Message{Role: "user", Content: bail})
			s.mu.Unlock()
			return bail, nil
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
	if strings.HasPrefix(senderID, "web_") {
		sessionID := strings.TrimPrefix(senderID, "web_")
		s.mu.Lock()
		snapshot := make([]model.Message, len(s.history))
		copy(snapshot, s.history)
		s.mu.Unlock()
		go SaveSession(sessionID, snapshot)
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

func (s *AgentSession) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = []model.Message{{Role: "system", Content: buildSystemPrompt(s.registry, s.platform)}}
	if s.toolCache != nil {
		s.toolCache.Clear()
	}
	s.toolAttempts = map[string]int{}
	s.lastToolErr = map[string]string{}
	s.lastToolArgs = map[string]string{}
	s.turnCount = 0
	s.toolCallCount = 0
	s.autofixCount = 0
	s.errorCount = 0
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

const (
	defaultMaxOutput   = 16 * 1024        // 16KB default cap for tool result body
	defaultToolTimeout = 30 * time.Second // wall-clock timeout per tool
	spillTTL           = time.Hour        // spill files cleaned up after this long
)

// wrapToolResult applies the per-tool MaxOutput cap. If the result exceeds the
// cap, the full result is spilled to a temp file (apexclaw-<tool>-*.txt) and
// the model sees a head/tail snippet plus the spill path so it can call
// read_tool_output for more.
//
// tg_send_* tools are exempted — their results are short status strings.
func wrapToolResult(toolName, result string, def *tools.ToolDef) string {
	if strings.HasPrefix(toolName, "tg_send_") {
		return result
	}
	cap := defaultMaxOutput
	if def != nil {
		switch {
		case def.MaxOutput < 0:
			return result
		case def.MaxOutput > 0:
			cap = def.MaxOutput
		}
	}
	if len(result) <= cap {
		return result
	}

	// Spill the full output to a temp file.
	pathNote := ""
	f, err := os.CreateTemp(os.TempDir(), "apexclaw-"+sanitizeToolName(toolName)+"-*.txt")
	if err != nil {
		pathNote = fmt.Sprintf("(spill failed: %v)", err)
	} else {
		if _, werr := f.Write([]byte(result)); werr != nil {
			pathNote = fmt.Sprintf("(spill write failed: %v)", werr)
			f.Close()
			os.Remove(f.Name())
		} else {
			f.Close()
			pathNote = "full output at " + f.Name()
			fname := f.Name()
			time.AfterFunc(spillTTL, func() { os.Remove(fname) })
		}
	}

	head := cap / 2
	tail := cap - head
	headEnd := utf8SafeBoundary(result, head, true)
	tailStart := utf8SafeBoundary(result, len(result)-tail, false)
	dropped := tailStart - headEnd
	return fmt.Sprintf("%s\n\n...(truncated %d bytes — %s)...\n\n%s",
		result[:headEnd], dropped, pathNote, result[tailStart:])
}

// utf8SafeBoundary walks i to the nearest UTF-8 rune boundary so we never
// split a multi-byte character. backward=true walks left; false walks right.
func utf8SafeBoundary(s string, i int, backward bool) int {
	if i <= 0 {
		return 0
	}
	if i >= len(s) {
		return len(s)
	}
	for i > 0 && i < len(s) && (s[i]&0xC0) == 0x80 {
		if backward {
			i--
		} else {
			i++
		}
	}
	return i
}

func sanitizeToolName(n string) string {
	return strings.Map(func(r rune) rune {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, n)
}

// executeToolSafely wraps a tool invocation with panic recovery and an
// enforced wall-clock timeout. If the tool's Execute panics or blocks past
// its Timeout, we return an "Error:" string instead of crashing the agent.
func (s *AgentSession) executeToolSafely(def *tools.ToolDef, args map[string]any, senderID string) (out string) {
	timeout := def.Timeout
	if timeout == 0 {
		timeout = defaultToolTimeout
	}
	// Negative timeout = no enforced cap; tool runs synchronously (no goroutine
	// overhead). Useful for tools that intentionally block, like zai_research.
	if timeout < 0 {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[TOOL-PANIC] %s: %v", def.Name, r)
				out = fmt.Sprintf("Error: tool panicked: %v", r)
			}
		}()
		if def.ExecuteWithContext != nil {
			return def.ExecuteWithContext(args, senderID)
		}
		return def.Execute(args)
	}

	done := make(chan string, 1) // buffered so the orphan goroutine can exit
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[TOOL-PANIC] %s: %v", def.Name, r)
				done <- fmt.Sprintf("Error: tool panicked: %v", r)
			}
		}()
		if def.ExecuteWithContext != nil {
			done <- def.ExecuteWithContext(args, senderID)
		} else {
			done <- def.Execute(args)
		}
	}()

	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case res := <-done:
		return res
	case <-t.C:
		log.Printf("[TOOL-TIMEOUT] %s exceeded %v", def.Name, timeout)
		return fmt.Sprintf("Error: tool '%s' exceeded %v timeout", def.Name, timeout)
	}
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
	strippedID := strings.TrimPrefix(strings.TrimPrefix(realUserID, "wa_"), "web_")
	isOwner := realUserID == Cfg.OwnerID ||
		strippedID == Cfg.OwnerID ||
		(Cfg.WAOwnerID != "" && strippedID == Cfg.WAOwnerID)
	if t.Secure && !isOwner {
		Log.Debugf("access denied: user %q tried secure tool %q", realUserID, name)
		return fmt.Sprintf("Access denied: tool %q is restricted to the bot owner.", name)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		args = make(map[string]any)
	}

	// Telemetry: record attempt + previous error state.
	s.mu.Lock()
	s.toolAttempts[name]++
	attempt := s.toolAttempts[name]
	prevErr, hadErr := s.lastToolErr[name]
	prevArgs := s.lastToolArgs[name]
	s.mu.Unlock()

	// Cache check (skips non-cacheable tools and bypass requests).
	if cached, hit := s.toolCache.Get(name, argsJSON); hit {
		log.Printf("[TOOL] name=%s attempt=%d status=cache-hit duration=0ms", name, attempt)
		return cached
	}

	start := time.Now()
	result := s.executeToolSafely(t, args, senderID)
	duration := time.Since(start)

	// Handle the __DEEPWORK:__ sentinel.
	if strings.HasPrefix(result, "__DEEPWORK:") {
		var n int
		rest := strings.TrimPrefix(result, "__DEEPWORK:")
		if idx := strings.Index(rest, "__\n"); idx != -1 {
			fmt.Sscanf(rest[:idx], "%d", &n)
			result = strings.TrimPrefix(rest, rest[:idx+3])
		}
		if n > 0 {
			plan, _ := args["plan"].(string)
			s.SetDeepWork(n, plan)
		}
	}

	// Telemetry: track error transitions for autofix detection.
	isErr := isToolError(result)
	status := "ok"
	msg := ""
	if isErr {
		status = "err"
		msg = truncStr(result, 120)
	}
	s.mu.Lock()
	if isErr {
		s.lastToolErr[name] = msg
		s.lastToolArgs[name] = argsJSON
		s.errorCount++
		if hadErr && argsJSON != prevArgs {
			s.autofixCount++
			log.Printf("[AUTOFIX] tool=%s prev_err=%q new_args=%s outcome=err",
				name, prevErr, truncStr(argsJSON, 200))
		}
	} else {
		if hadErr && argsJSON != prevArgs {
			s.autofixCount++
			delete(s.lastToolErr, name)
			delete(s.lastToolArgs, name)
			log.Printf("[AUTOFIX] tool=%s prev_err=%q new_args=%s outcome=ok",
				name, prevErr, truncStr(argsJSON, 200))
		} else if !hadErr {
			delete(s.lastToolErr, name)
			delete(s.lastToolArgs, name)
		}
	}
	s.mu.Unlock()

	log.Printf("[TOOL] name=%s attempt=%d status=%s duration=%dms msg=%q",
		name, attempt, status, duration.Milliseconds(), msg)

	// Cache successful results for cacheable tools.
	if !isErr {
		s.toolCache.Put(name, argsJSON, result)
	}

	// Debug trace.
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
			Error:    isErr,
		}
		s.mu.Lock()
		s.traceLog = append(s.traceLog, entry)
		s.mu.Unlock()
	}

	// Output capping — applied AFTER caching so cached entries also benefit
	// (we want every result the model sees to be size-bounded).
	return wrapToolResult(name, result, t)
}

func truncStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
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

func looksLikeUnparsedToolCalls(s string) bool {
	t := strings.TrimSpace(s)
	if t == "" {
		return false
	}
	if strings.Contains(t, "```tool_calls") {
		return true
	}
	if !(strings.HasPrefix(t, "[") || strings.HasPrefix(t, "{")) {
		return false
	}
	if !strings.Contains(t, "\"name\"") {
		return false
	}
	hasArgs := strings.Contains(t, "\"args\"") ||
		strings.Contains(t, "\"arguments\"") ||
		strings.Contains(t, "\"parameters\"")
	return hasArgs
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

// tryGetAgentSession returns the existing session for key, or nil if none
// has been created yet. Unlike GetOrCreateAgentSession this never allocates.
func tryGetAgentSession(key string) *AgentSession {
	agentSessions.RLock()
	defer agentSessions.RUnlock()
	return agentSessions.m[key]
}

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
