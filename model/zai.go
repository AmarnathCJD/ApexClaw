package model

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

const (
	zaiBase             = "https://chatglm.cn"
	zaiGuestEndpoint    = zaiBase + "/chatglm/user-api/guest/access"
	zaiStreamEndpoint   = zaiBase + "/chatglm/backend-api/assistant/stream"
	zaiUploadEndpoint   = zaiBase + "/chatglm/productivity-api/file/chat_upload"
	zaiImgEditEndpoint  = zaiBase + "/chatglm/drawing-api/v1/image/reference"
	zaiSignSalt         = "8a1317a7468aa3ad86e997d08f3f31cb"
	zaiUserAgent        = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36"
	zaiDefaultAssistant = "65940acff94777010aa6b796"
	zaiImageAssistant   = "65a232c082ff90a2ad2f15e2"
	zaiTokenTTL         = 20 * time.Minute
)

const (
	zaiTokenSoftCallCap = 8
	zaiTokenPerSemaCap  = 2
	zaiTokenPoolMax     = 12
	zaiTokenCooldown    = 90 * time.Second
	zaiTokenIdleTTL     = 15 * time.Minute
)

type zaiToken struct {
	token        string
	deviceID     string
	createdAt    time.Time
	lastUsed     time.Time
	callsServed  int
	sem          chan struct{}
	dead         bool
	cooldownDone time.Time
}

func newZaiToken(tok, dev string) *zaiToken {
	return &zaiToken{
		token:     tok,
		deviceID:  dev,
		createdAt: time.Now(),
		lastUsed:  time.Now(),
		sem:       make(chan struct{}, zaiTokenPerSemaCap),
	}
}

func (t *zaiToken) acquire(ctx context.Context) error {
	select {
	case t.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (t *zaiToken) release() { <-t.sem }

type zaiTokenPool struct {
	mu        sync.Mutex
	tokens    []*zaiToken
	lastIssue time.Time
	issueErr  time.Time
	backoff   time.Duration
}

var (
	zaiPool     = &zaiTokenPool{backoff: 500 * time.Millisecond}
	zaiHTTPOnce sync.Once
	zaiHTTP     *http.Client
)

func zaiClient() *http.Client {
	zaiHTTPOnce.Do(func() {
		transport := &http.Transport{
			MaxIdleConns:        10,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 20 * time.Second,
		}
		if raw := strings.TrimSpace(os.Getenv("PROXY")); raw != "" {
			if host, portStr, err := net.SplitHostPort(raw); err == nil {
				var auth *proxy.Auth
				if u := os.Getenv("PROXY_USERNAME"); u != "" {
					auth = &proxy.Auth{User: u, Password: os.Getenv("PROXY_PASSWORD")}
				}
				if dialer, derr := proxy.SOCKS5("tcp", net.JoinHostPort(host, portStr), auth, proxy.Direct); derr == nil {
					if cd, ok := dialer.(proxy.ContextDialer); ok {
						transport.DialContext = cd.DialContext
					} else {
						transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
							return dialer.Dial(network, addr)
						}
					}
				}
			}
		}
		zaiHTTP = &http.Client{Timeout: 120 * time.Second, Transport: transport}
	})
	return zaiHTTP
}

func zaiHex32() string {
	b := make([]byte, 16)
	if _, err := crand.Read(b); err != nil {
		now := time.Now().UnixNano()
		for i := range b {
			b[i] = byte(now >> (i % 8 * 8))
		}
	}
	return hex.EncodeToString(b)
}

func randomPublicIPv4() string {
	for {
		var b [4]byte
		if _, err := crand.Read(b[:]); err != nil {
			return "8.8.8.8"
		}
		if b[0] == 10 || b[0] == 127 || b[0] == 0 {
			continue
		}
		if b[0] == 172 && b[1] >= 16 && b[1] <= 31 {
			continue
		}
		if b[0] == 192 && b[1] == 168 {
			continue
		}
		if b[0] == 169 && b[1] == 254 {
			continue
		}
		if b[0] >= 224 {
			continue
		}
		return fmt.Sprintf("%d.%d.%d.%d", b[0], b[1], b[2], b[3])
	}
}

func zaiMangleTimestamp() string {
	ms := strconv.FormatInt(time.Now().UnixMilli(), 10)
	e := len(ms)
	if e < 2 {
		return ms
	}
	sum := 0
	for _, ch := range ms {
		sum += int(ch - '0')
	}
	i := sum - int(ms[e-2]-'0')
	mod := ((i % 10) + 10) % 10
	return ms[:e-2] + strconv.Itoa(mod) + ms[e-1:e]
}

func zaiSignHeaders() (ts, nonce, sign string) {
	ts = zaiMangleTimestamp()
	nonce = zaiHex32()
	digest := md5.Sum([]byte(ts + "-" + nonce + "-" + zaiSignSalt))
	sign = hex.EncodeToString(digest[:])
	return ts, nonce, sign
}

func zaiSetHeaders(req *http.Request, deviceID, token string) {
	ts, nonce, sign := zaiSignHeaders()
	req.Header.Set("Content-Type", "application/json;charset=utf-8")
	req.Header.Set("App-Name", "chatglm")
	req.Header.Set("X-Device-Id", deviceID)
	req.Header.Set("X-App-Platform", "pc")
	req.Header.Set("X-App-Version", "0.0.1")
	req.Header.Set("X-App-Fr", "browser_extension")
	req.Header.Set("X-Request-Id", zaiHex32())
	req.Header.Set("X-Exp-Groups", "")
	req.Header.Set("X-Device-Model", "")
	req.Header.Set("X-Device-Brand", "")
	req.Header.Set("X-Lang", "zh")
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Nonce", nonce)
	req.Header.Set("X-Sign", sign)
	req.Header.Set("X-Forwarded-For", randomPublicIPv4())
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Priority", "u=1, i")
	req.Header.Set("Sec-Ch-Ua", `"Microsoft Edge";v="143", "Chromium";v="143", "Not A(Brand";v="24"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Origin", zaiBase)
	req.Header.Set("Referer", zaiBase+"/")
	req.Header.Set("User-Agent", zaiUserAgent)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

var (
	errZaiAuth  = errors.New("zai: unauthorized")
	errZaiEmpty = errors.New("zai: empty response")
)

// isGuestQuotaExhaustedBody detects chatglm.cn's silent guest-quota throttle.
// The server returns HTTP 200 with a JSON envelope (NOT the usual SSE stream)
// of the shape: {"status":500,"message":"您已多次体验过对话, 请登录后继续使用",...}
// when a guest token has hit its ~10-call cap. We want to surface this as an
// auth error so the caller force-refreshes the token.
func isGuestQuotaExhaustedBody(raw []byte) bool {
	// Cheap shortcut: SSE responses start with "data:", not JSON braces.
	trimmed := bytes.TrimLeft(raw, " \r\n\t")
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return false
	}
	var env struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(trimmed, &env); err != nil {
		return false
	}
	if env.Status == 0 || env.Status == 200 {
		return false
	}
	// Quota-exhausted (and similar guest-throttle) messages always mention
	// 登录 (log in) and 体验 (try / experience) — match either Chinese token.
	return strings.Contains(env.Message, "登录") || strings.Contains(env.Message, "体验过")
}

func zaiFetchGuest(ctx context.Context) (string, string, error) {
	deviceID := zaiHex32()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, zaiGuestEndpoint, bytes.NewReader([]byte{}))
	if err != nil {
		return "", "", err
	}
	zaiSetHeaders(req, deviceID, "")
	req.Header.Set("X-App-Fr", "default")
	resp, err := zaiClient().Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("guest endpoint returned status %d", resp.StatusCode)
	}
	var out struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
		Result  struct {
			AccessToken string `json:"access_token"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", err
	}
	if out.Result.AccessToken == "" {
		return "", "", fmt.Errorf("guest endpoint: %s", out.Message)
	}
	return out.Result.AccessToken, deviceID, nil
}

func zaiGuestToken(ctx context.Context, force bool) (string, string, error) {
	t, err := zaiPool.checkout(ctx, force)
	if err != nil {
		return "", "", err
	}
	return t.token, t.deviceID, nil
}

func (p *zaiTokenPool) checkout(ctx context.Context, force bool) (*zaiToken, error) {
	p.mu.Lock()
	if force {
		for _, t := range p.tokens {
			t.dead = true
			t.cooldownDone = time.Now().Add(zaiTokenCooldown)
		}
	}
	p.evictLocked()
	for _, t := range p.tokens {
		if !t.dead && t.callsServed < zaiTokenSoftCallCap {
			t.lastUsed = time.Now()
			p.mu.Unlock()
			return t, nil
		}
	}
	if len(p.tokens) >= zaiTokenPoolMax {
		for i, t := range p.tokens {
			if t.dead {
				p.tokens = append(p.tokens[:i], p.tokens[i+1:]...)
				break
			}
		}
	}
	until := p.issueErr.Add(p.backoff)
	p.mu.Unlock()
	if w := time.Until(until); w > 0 {
		select {
		case <-time.After(w):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	tok, dev, err := zaiFetchGuest(ctx)
	p.mu.Lock()
	defer p.mu.Unlock()
	if err != nil {
		p.issueErr = time.Now()
		if p.backoff < 30*time.Second {
			p.backoff *= 2
		}
		return nil, fmt.Errorf("zai: token issue failed (backoff now %v): %w", p.backoff, err)
	}
	p.backoff = 500 * time.Millisecond
	p.lastIssue = time.Now()
	newT := newZaiToken(tok, dev)
	p.tokens = append(p.tokens, newT)
	return newT, nil
}

func (p *zaiTokenPool) markFailed(token string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, t := range p.tokens {
		if t.token == token {
			t.dead = true
			t.cooldownDone = time.Now().Add(zaiTokenCooldown)
			return
		}
	}
}

func (p *zaiTokenPool) recordSuccess(token string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, t := range p.tokens {
		if t.token == token {
			t.callsServed++
			t.lastUsed = time.Now()
			if t.callsServed >= zaiTokenSoftCallCap {
				t.dead = true
				t.cooldownDone = time.Now().Add(zaiTokenCooldown)
			}
			return
		}
	}
}

func (p *zaiTokenPool) evictLocked() {
	now := time.Now()
	kept := p.tokens[:0]
	for _, t := range p.tokens {
		if t.dead && now.After(t.cooldownDone) {
			continue
		}
		if !t.dead && now.Sub(t.lastUsed) > zaiTokenIdleTTL {
			continue
		}
		kept = append(kept, t)
	}
	p.tokens = kept
}

type zaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ZAIFile struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	FileSize int64  `json:"file_size"`
	ImageURL string `json:"image_url"`
}

type ZAIOpts struct {
	AssistantID    string
	ConversationID string
	ChatMode       string
	ChatModel      string
	IfPlusModel    bool
	Networking     bool
	Files          []ZAIFile
	CogView        map[string]any
}

type ZAIResult struct {
	Text           string
	ConversationID string
	ImageURLs      []string
}

func zaiChat(ctx context.Context, assistant string, messages []zaiMessage) (string, error) {
	res, err := ZAIChat(ctx, messages, ZAIOpts{AssistantID: assistant, Networking: true})
	if err != nil {
		return "", err
	}
	return res.Text, nil
}

func ZAIChat(ctx context.Context, messages []zaiMessage, opts ZAIOpts) (*ZAIResult, error) {
	if opts.AssistantID == "" {
		opts.AssistantID = zaiDefaultAssistant
	}
	const maxAttempts = 4
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		t, err := zaiPool.checkout(ctx, false)
		if err != nil {
			return nil, err
		}
		if err := t.acquire(ctx); err != nil {
			return nil, err
		}
		res, callErr := zaiDoChat(ctx, t.token, t.deviceID, messages, opts)
		t.release()
		if callErr == nil {
			zaiPool.recordSuccess(t.token)
			return res, nil
		}
		if errors.Is(callErr, errZaiAuth) {
			zaiPool.markFailed(t.token)
			lastErr = callErr
			continue
		}
		return res, callErr
	}
	return nil, fmt.Errorf("zai: exhausted %d token attempts: %w", maxAttempts, lastErr)
}

func zaiDoChat(ctx context.Context, token, deviceID string, messages []zaiMessage, opts ZAIOpts) (*ZAIResult, error) {
	streamMsgs := make([]map[string]any, 0, len(messages))
	for i, msg := range messages {
		content := []map[string]any{{"type": "text", "text": msg.Content}}
		if i == len(messages)-1 && msg.Role == "user" && len(opts.Files) > 0 {
			imgs := make([]map[string]any, 0, len(opts.Files))
			for idx, f := range opts.Files {
				imgs = append(imgs, map[string]any{
					"file_id":   f.FileID,
					"file_name": f.FileName,
					"file_size": f.FileSize,
					"image_url": f.ImageURL,
					"height":    0,
					"width":     0,
					"order":     idx,
				})
			}
			content = append(content, map[string]any{"type": "image", "image": imgs})
		}
		streamMsgs = append(streamMsgs, map[string]any{
			"role":    msg.Role,
			"content": content,
		})
	}

	cogview := map[string]any{"rm_label_watermark": false}
	for k, v := range opts.CogView {
		cogview[k] = v
	}

	body := map[string]any{
		"assistant_id":    opts.AssistantID,
		"chat_type":       "user_chat",
		"conversation_id": opts.ConversationID,
		"messages":        streamMsgs,
		"meta_data": map[string]any{
			"channel":             "",
			"chat_mode":           opts.ChatMode,
			"chat_model":          opts.ChatModel,
			"cogview":             cogview,
			"draft_id":            "",
			"if_plus_model":       opts.IfPlusModel,
			"input_question_type": "xxxx",
			"is_networking":       opts.Networking,
			"is_test":             false,
			"platform":            "pc",
			"quote_log_id":        "",
		},
		"project_id": "",
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, zaiStreamEndpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	zaiSetHeaders(req, deviceID, token)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := zaiClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, errZaiAuth
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		// chatglm.cn caps a single guest token at ~2 concurrent requests.
		// A 429 here means our parallel pressure on this token has spiked —
		// rotating to a fresh token is the right move (each token has its
		// own concurrency window).
		return nil, errZaiAuth
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 800))
		return nil, fmt.Errorf("upstream %d: %s", resp.StatusCode, string(body))
	}

	raw, _ := io.ReadAll(resp.Body)

	// chatglm.cn returns 200 OK with a JSON body of shape
	// {"status":500,"message":"您已多次体验过对话, 请登录后继续使用"}
	// when the guest token has exhausted its ~10-call quota. The body looks
	// like a non-stream JSON, NOT the SSE we'd normally parse. Detect this
	// shape and surface as auth-error so the caller force-rotates the token.
	if isGuestQuotaExhaustedBody(raw) {
		log.Printf("[ZAI] guest quota exhausted on this token — force-rotating")
		return nil, errZaiAuth
	}

	res, parseErr := zaiParseStream(bytes.NewReader(raw))
	if parseErr != nil {
		snip := string(raw)
		if len(snip) > 800 {
			snip = snip[:800] + "...[truncated]"
		}
		log.Printf("[ZAI] parse failed: %v | status=%d | content-type=%q | body=%s",
			parseErr, resp.StatusCode, resp.Header.Get("Content-Type"), snip)
	}
	return res, parseErr
}

type zaiStreamContent struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Think string `json:"think"`
	Image []struct {
		ImageURL string `json:"image_url"`
	} `json:"image"`
}

type zaiStreamPart struct {
	LogicID string             `json:"logic_id"`
	Role    string             `json:"role"`
	Content []zaiStreamContent `json:"content"`
	Status  string             `json:"status"`
	Error   struct {
		InterveneType string `json:"intervene_type"`
		InterveneText string `json:"intervene_text"`
		RiskLevel     string `json:"risk_level"`
	} `json:"error"`
}

type zaiStreamEnvelope struct {
	Status         string          `json:"status"`
	ConversationID string          `json:"conversation_id"`
	Parts          []zaiStreamPart `json:"parts"`
}

var errZaiBlocked = errors.New("zai: content blocked by safety filter")

type zaiPartState struct {
	text   string
	think  string
	images []string
}

func dedupContiguousParagraphs(s string) string {
	if s == "" {
		return s
	}
	paras := strings.Split(s, "\n\n")
	out := make([]string, 0, len(paras))
	var prev string
	for _, p := range paras {
		norm := strings.TrimSpace(p)
		if norm != "" && norm == prev {
			continue
		}
		out = append(out, p)
		if norm != "" {
			prev = norm
		}
	}
	return strings.Join(out, "\n\n")
}

func zaiParseStream(r io.Reader) (*ZAIResult, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	res := &ZAIResult{}
	seenImg := map[string]bool{}
	parts := map[string]*zaiPartState{}
	var order []string
	var lastFrame string
	frames := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		frames++
		lastFrame = data
		var env zaiStreamEnvelope
		if json.Unmarshal([]byte(data), &env) != nil {
			continue
		}
		if env.ConversationID != "" {
			res.ConversationID = env.ConversationID
		}
		for i, part := range env.Parts {
			if part.Status == "intervene" || part.Error.InterveneType != "" {
				msg := part.Error.InterveneText
				if msg == "" {
					msg = "content rejected"
				}
				return nil, fmt.Errorf("%w: %s", errZaiBlocked, msg)
			}
			if part.Role != "" && part.Role != "assistant" {
				continue
			}
			key := part.LogicID
			if key == "" {
				key = fmt.Sprintf("__pos%d", i)
			}
			st, ok := parts[key]
			if !ok {
				st = &zaiPartState{}
				parts[key] = st
				order = append(order, key)
			}
			hasText, hasThink := false, false
			for _, c := range part.Content {
				if c.Type == "text" && c.Text != "" {
					hasText = true
				}
				if c.Type == "think" && (c.Think != "" || c.Text != "") {
					hasThink = true
				}
			}
			if hasText {
				st.text = ""
			}
			if hasThink {
				st.think = ""
			}
			for _, c := range part.Content {
				switch c.Type {
				case "text":
					if c.Text != "" {
						st.text = c.Text
					}
				case "think":
					if c.Think != "" {
						st.think = c.Think
					} else if c.Text != "" {
						st.think = c.Text
					}
				case "image":
					for _, img := range c.Image {
						if img.ImageURL != "" && !seenImg[img.ImageURL] {
							seenImg[img.ImageURL] = true
							st.images = append(st.images, img.ImageURL)
						}
					}
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	var texts []string
	seenText := map[string]bool{}
	for _, k := range order {
		st := parts[k]
		t := strings.TrimSpace(st.text)
		if t != "" && !seenText[t] {
			texts = append(texts, t)
			seenText[t] = true
		}
		res.ImageURLs = append(res.ImageURLs, st.images...)
	}
	res.Text = dedupContiguousParagraphs(strings.TrimSpace(strings.Join(texts, "\n\n")))

	if res.Text == "" && len(res.ImageURLs) == 0 {
		var thinkFallback string
		for _, k := range order {
			if t := strings.TrimSpace(parts[k].think); t != "" {
				thinkFallback = t
				break
			}
		}
		if thinkFallback != "" {
			res.Text = thinkFallback
			return res, nil
		}
		if frames > 0 {
			snip := lastFrame
			if len(snip) > 400 {
				snip = snip[:400] + "..."
			}
			return nil, fmt.Errorf("%w (frames=%d parts=%d last=%s)", errZaiEmpty, frames, len(order), snip)
		}
		return nil, errZaiEmpty
	}
	return res, nil
}

func ZAIChatText(ctx context.Context, userText string, opts ZAIOpts) (*ZAIResult, error) {
	return ZAIChat(ctx, []zaiMessage{{Role: "user", Content: userText}}, opts)
}

func ZAIUpload(ctx context.Context, path string) (*ZAIFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	return zaiUploadReader(ctx, f, filepath.Base(path), st.Size())
}

func ZAIUploadBytes(ctx context.Context, data []byte, name string) (*ZAIFile, error) {
	return zaiUploadReader(ctx, bytes.NewReader(data), name, int64(len(data)))
}

func zaiUploadReader(ctx context.Context, r io.Reader, name string, size int64) (*ZAIFile, error) {
	_ = size
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	const maxAttempts = 4
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		t, err := zaiPool.checkout(ctx, false)
		if err != nil {
			return nil, err
		}
		if err := t.acquire(ctx); err != nil {
			return nil, err
		}
		res, callErr := zaiUploadDo(ctx, t.token, t.deviceID, bytes.NewReader(body), name)
		t.release()
		if callErr == nil {
			zaiPool.recordSuccess(t.token)
			return res, nil
		}
		if errors.Is(callErr, errZaiAuth) {
			zaiPool.markFailed(t.token)
			lastErr = callErr
			continue
		}
		return res, callErr
	}
	return nil, fmt.Errorf("zai: upload exhausted %d token attempts: %w", maxAttempts, lastErr)
}

func zaiUploadDo(ctx context.Context, token, deviceID string, r io.Reader, name string) (*ZAIFile, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", name)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, r); err != nil {
		return nil, err
	}
	w.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, zaiUploadEndpoint, &buf)
	if err != nil {
		return nil, err
	}
	zaiSetHeaders(req, deviceID, token)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	resp, err := zaiClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, errZaiAuth
	}
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upload %d: %s", resp.StatusCode, truncStr(string(body), 200))
	}

	var out struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
		Result  struct {
			SourceID string `json:"source_id"`
			FileID   string `json:"file_id"`
			FileName string `json:"file_name"`
			FileSize int64  `json:"file_size"`
			FileURL  string `json:"file_url"`
			FileType string `json:"file_type"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	id := out.Result.SourceID
	if id == "" {
		id = out.Result.FileID
	}
	if id == "" || out.Result.FileURL == "" {
		return nil, fmt.Errorf("upload: %s (body=%s)", out.Message, truncStr(string(body), 300))
	}
	return &ZAIFile{
		FileID:   id,
		FileName: out.Result.FileName,
		FileSize: out.Result.FileSize,
		ImageURL: out.Result.FileURL,
	}, nil
}

type ZAIImageOpts struct {
	AspectRatio string
	Style       string
	Scene       string
	ChatModel   string
}

func ZAIImageGenerate(ctx context.Context, prompt string, opts ZAIImageOpts) (*ZAIResult, error) {
	if opts.AspectRatio == "" {
		opts.AspectRatio = "1:1"
	}
	if opts.Style == "" {
		opts.Style = "none"
	}
	if opts.Scene == "" {
		opts.Scene = "none"
	}
	return ZAIChat(ctx, []zaiMessage{{Role: "user", Content: prompt}}, ZAIOpts{
		AssistantID: zaiImageAssistant,
		Networking:  false,
		CogView: map[string]any{
			"aspect_ratio": opts.AspectRatio,
			"style":        opts.Style,
			"scene":        opts.Scene,
			"chat_model":   opts.ChatModel,
		},
	})
}

func ZAIImageEdit(ctx context.Context, file *ZAIFile, prompt, aspectRatio, conversationID string) (string, error) {
	if aspectRatio == "" {
		aspectRatio = "1:1"
	}
	const maxAttempts = 4
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		t, err := zaiPool.checkout(ctx, false)
		if err != nil {
			return "", err
		}
		if err := t.acquire(ctx); err != nil {
			return "", err
		}
		url, callErr := zaiImageEditDo(ctx, t.token, t.deviceID, file, prompt, aspectRatio, conversationID)
		t.release()
		if callErr == nil {
			zaiPool.recordSuccess(t.token)
			return url, nil
		}
		if errors.Is(callErr, errZaiAuth) {
			zaiPool.markFailed(t.token)
			lastErr = callErr
			continue
		}
		return url, callErr
	}
	return "", fmt.Errorf("zai: image-edit exhausted %d token attempts: %w", maxAttempts, lastErr)
}

func zaiImageEditDo(ctx context.Context, token, deviceID string, file *ZAIFile, prompt, aspectRatio, conversationID string) (string, error) {
	body := map[string]any{
		"aspect_ratio":       aspectRatio,
		"assistant_id":       zaiImageAssistant,
		"category":           1,
		"conversation_id":    conversationID,
		"file_id":            file.FileID,
		"history_id":         "",
		"if_plus_model":      false,
		"image_url":          file.ImageURL,
		"prompt":             prompt,
		"rm_label_watermark": false,
		"title":              "【智能编辑】 " + prompt,
		"type":               "Intelligent Editing",
	}
	payload, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, zaiImgEditEndpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	zaiSetHeaders(req, deviceID, token)

	resp, err := zaiClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", errZaiAuth
	}
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("image-edit %d: %s", resp.StatusCode, truncStr(string(rb), 200))
	}

	var out struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
		Result  struct {
			ImageURL       string `json:"image_url"`
			ConversationID string `json:"conversation_id"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rb, &out); err != nil {
		return "", err
	}
	if out.Result.ImageURL == "" {
		return "", fmt.Errorf("image-edit empty: %s", out.Message)
	}
	return out.Result.ImageURL, nil
}

func truncStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
