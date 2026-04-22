package model

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/corpix/uarand"
	"github.com/google/uuid"
)

type Message struct {
	Role             string         `json:"role"`
	Content          string         `json:"content"`
	ReasoningDetails any            `json:"reasoning_details,omitempty"`
	Files            []UpstreamFile `json:"-"`
}

type Client struct {
	http *http.Client
}

func New() *Client {
	transport := &http.Transport{
		Dial: func(network, addr string) (net.Conn, error) {
			return net.Dial(network, addr)
		},
	}
	return &Client{http: &http.Client{Timeout: 5 * time.Minute, Transport: transport}}
}

func NewWithCustomDialer(dialer *net.Dialer) *Client {
	transport := &http.Transport{
		Dial: func(network, addr string) (net.Conn, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return dialer.DialContext(ctx, network, addr)
		},
	}
	return &Client{http: &http.Client{Timeout: 5 * time.Minute, Transport: transport}}
}

const (
	maxRetries  = 3
	retryBaseMs = 1000
)

var retryableHTTPCodes = map[int]bool{429: true, 500: true, 502: true, 503: true}

func (c *Client) Send(ctx context.Context, model string, messages []Message) (Message, error) {
	return c.sendWithRetry(ctx, model, messages, nil)
}

func (c *Client) SendWithFiles(ctx context.Context, model string, messages []Message, files []*UpstreamFile) (Message, error) {
	return c.sendWithRetry(ctx, model, messages, files)
}

func (c *Client) sendWithRetry(ctx context.Context, model string, messages []Message, files []*UpstreamFile) (Message, error) {
	var lastErr error
	for attempt := range maxRetries {
		if attempt > 0 {
			delay := time.Duration(retryBaseMs*(1<<uint(attempt-1))) * time.Millisecond
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return Message{}, ctx.Err()
			}
			log.Printf("[MODEL] retry attempt %d after %v (last err: %v)", attempt+1, delay, lastErr)
		}
		result, err := c.sendInternal(ctx, model, messages, files)
		if err == nil {
			return result, nil
		}
		lastErr = err
		errStr := err.Error()
		isRetryable := false
		for code := range retryableHTTPCodes {
			if strings.Contains(errStr, fmt.Sprintf("upstream %d", code)) {
				isRetryable = true
				break
			}
		}
		if !isRetryable {
			var netErr *net.OpError
			if errors.As(err, &netErr) {
				isRetryable = true
			}
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return Message{}, err
		}
		if strings.Contains(errStr, "upstream 401") {
			ClearTokenCache()
		}
		if !isRetryable {
			return Message{}, err
		}
	}
	return Message{}, fmt.Errorf("all %d retries failed: %w", maxRetries, lastErr)
}

func (c *Client) sendInternal(ctx context.Context, mdl string, messages []Message, files []*UpstreamFile) (Message, error) {
	provider := GetActiveProvider()
	if provider == "" || provider == "zai" || provider == "glm" {
		ps := GetProviderSettings("zai")
		active := mdl
		if active == "" {
			active = ps.Model
		}
		log.Printf("[MODEL] provider=zai model=%s msgs=%d", active, len(messages))
		return c.sendInternalZAI(ctx, mdl, messages, files)
	}

	ps := GetProviderSettings(provider)
	active := mdl
	if active == "" {
		active = ps.Model
	}
	log.Printf("[MODEL] provider=%s model=%s msgs=%d", provider, active, len(messages))

	switch provider {
	case "nvidia":
		return c.sendInternalOpenAICompat(ctx, mdl, messages, files)
	case "openrouter":
		return c.sendInternalOpenRouter(ctx, mdl, messages, files)
	case "groq":
		return c.sendInternalGroq(ctx, mdl, messages, files)
	default:
		return Message{}, fmt.Errorf("unsupported AI_PROVIDER: %s", provider)
	}
}

func (c *Client) sendInternalZAI(ctx context.Context, model string, messages []Message, files []*UpstreamFile) (Message, error) {
	token, err := GetAnonymousToken()
	if err != nil {
		return Message{}, fmt.Errorf("auth: %w", err)
	}
	resp, targetModel, err := makeUpstreamRequest(ctx, c.http, token, messages, model, files)
	if err != nil {
		return Message{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return Message{}, fmt.Errorf("upstream %d: %s", resp.StatusCode, string(body))
	}
	_ = targetModel
	content, err := collectNonStream(resp.Body)
	return Message{Role: "assistant", Content: content}, err
}

func (c *Client) sendInternalOpenAICompat(ctx context.Context, model string, messages []Message, files []*UpstreamFile) (Message, error) {
	ps := GetProviderSettings("nvidia")
	if ps.APIKey == "" {
		return Message{}, fmt.Errorf("missing NVIDIA_API_KEY")
	}

	apiURL := ps.APIURL
	if apiURL == "" {
		apiURL = "https://integrate.api.nvidia.com/v1/chat/completions"
	}
	if model == "" {
		model = ps.Model
	}

	imageURLs := collectImageURLs(messages, files)
	body := map[string]any{
		"model":             model,
		"messages":          toOpenAIMessages(messages, imageURLs),
		"temperature":       ps.Temperature,
		"top_p":             ps.TopP,
		"frequency_penalty": 0,
		"presence_penalty":  0,
		"max_tokens":        ps.MaxTokens,
		"stream":            ps.Stream,
		"chat_template_kwargs": map[string]any{
			"enable_thinking": ps.EnableThinking,
		},
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return Message{}, err
	}
	req.Header.Set("Authorization", "Bearer "+ps.APIKey)
	if ps.Stream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return Message{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1000))
		return Message{}, fmt.Errorf("upstream %d: %s", resp.StatusCode, string(body))
	}

	var content string
	if ps.Stream {
		content, err = collectOpenAIStream(resp.Body)
	} else {
		content, err = collectOpenAINonStream(resp.Body)
	}
	return Message{Role: "assistant", Content: content}, err
}

func (c *Client) sendInternalGroq(ctx context.Context, model string, messages []Message, files []*UpstreamFile) (Message, error) {
	ps := GetProviderSettings("groq")
	if ps.APIKey == "" {
		return Message{}, fmt.Errorf("missing GROQ_API_KEY")
	}

	apiURL := ps.APIURL
	if apiURL == "" {
		apiURL = "https://api.groq.com/openai/v1/chat/completions"
	}
	if model == "" {
		model = ps.Model
	}

	imageURLs := collectImageURLs(messages, files)
	body := map[string]any{
		"model":                 model,
		"messages":              toOpenAIMessages(messages, imageURLs),
		"temperature":           ps.Temperature,
		"top_p":                 ps.TopP,
		"max_completion_tokens": ps.MaxTokens,
		"stream":                ps.Stream,
		"reasoning_effort":      ps.ReasoningEffort,
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return Message{}, err
	}
	req.Header.Set("Authorization", "Bearer "+ps.APIKey)
	req.Header.Set("Content-Type", "application/json")
	if ps.Stream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return Message{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1000))
		return Message{}, fmt.Errorf("upstream %d: %s", resp.StatusCode, string(body))
	}

	var content string
	if ps.Stream {
		content, err = collectOpenAIStream(resp.Body)
	} else {
		content, err = collectOpenAINonStream(resp.Body)
	}
	return Message{Role: "assistant", Content: content}, err
}

func toOpenAIMessages(messages []Message, imageURLs []string) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	lastUserIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(strings.TrimSpace(messages[i].Role), "user") {
			lastUserIdx = i
			break
		}
	}

	for i, m := range messages {
		role := strings.TrimSpace(m.Role)
		if role == "" {
			continue
		}

		entry := map[string]any{"role": role}
		if strings.EqualFold(role, "user") && len(imageURLs) > 0 && i == lastUserIdx {
			parts := make([]map[string]any, 0, 1+len(imageURLs))
			if strings.TrimSpace(m.Content) != "" {
				parts = append(parts, map[string]any{"type": "text", "text": m.Content})
			}
			for _, img := range imageURLs {
				parts = append(parts, map[string]any{
					"type":      "image_url",
					"image_url": map[string]any{"url": img},
				})
			}
			entry["content"] = parts
		} else {
			entry["content"] = m.Content
		}

		if m.ReasoningDetails != nil {
			entry["reasoning_details"] = m.ReasoningDetails
		}

		out = append(out, entry)
	}

	if len(out) == 0 && len(imageURLs) > 0 {
		parts := make([]map[string]any, 0, len(imageURLs)+1)
		parts = append(parts, map[string]any{"type": "text", "text": "Describe this image"})
		for _, img := range imageURLs {
			parts = append(parts, map[string]any{
				"type":      "image_url",
				"image_url": map[string]any{"url": img},
			})
		}
		out = append(out, map[string]any{"role": "user", "content": parts})
	} else if len(out) == 0 {
		out = append(out, map[string]any{"role": "user", "content": "hi"})
	}
	return out
}

func collectImageURLs(messages []Message, files []*UpstreamFile) []string {
	urls := make([]string, 0, len(files)+1)
	seen := map[string]bool{}

	addURL := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if !(strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") || strings.HasPrefix(v, "data:image/")) {
			return
		}
		if seen[v] {
			return
		}
		seen[v] = true
		urls = append(urls, v)
	}

	for _, f := range files {
		if f == nil {
			continue
		}
		if f.Media != "" && !strings.Contains(strings.ToLower(f.Media), "image") {
			continue
		}
		addURL(f.URL)
		if len(f.File) > 0 {
			var raw map[string]any
			if err := json.Unmarshal(f.File, &raw); err == nil {
				if v, ok := raw["url"].(string); ok {
					addURL(v)
				}
				if v, ok := raw["download_url"].(string); ok {
					addURL(v)
				}
				if v, ok := raw["public_url"].(string); ok {
					addURL(v)
				}
			}
		}
	}

	for _, m := range messages {
		for _, f := range m.Files {
			if f.Media != "" && !strings.Contains(strings.ToLower(f.Media), "image") {
				continue
			}
			addURL(f.URL)
		}
	}

	return urls
}

func envBool(name string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if v == "" {
		return fallback
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func envInt(name string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n <= 0 {
		return fallback
	}
	return n
}

func envFloat(name string, fallback float64) float64 {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	var n float64
	if _, err := fmt.Sscanf(v, "%f", &n); err != nil {
		return fallback
	}
	return n
}

func makeUpstreamRequest(ctx context.Context, client *http.Client, token string, messages []Message, model string, files []*UpstreamFile) (*http.Response, string, error) {
	payload, err := DecodeJWTPayload(token)
	if err != nil || payload == nil {
		return nil, "", fmt.Errorf("invalid token")
	}

	userID := payload.ID
	chatID := uuid.New().String()
	timestamp := time.Now().UnixMilli()
	requestID := uuid.New().String()
	userMsgID := uuid.New().String()

	targetModel := GetTargetModel(model)
	latestUserContent := extractLatestUserContent(messages)
	signature := GenerateSignature(userID, requestID, latestUserContent, timestamp)

	enableThinking := IsThinkingModel(model)
	autoWebSearch := IsSearchModel(model)

	var systemTexts []string
	var nonSystemMessages []Message
	for _, m := range messages {
		if m.Role == "system" {
			if m.Content != "" {
				systemTexts = append(systemTexts, m.Content)
			}
		} else {
			nonSystemMessages = append(nonSystemMessages, m)
		}
	}

	var upstreamMessages []map[string]any
	for _, m := range nonSystemMessages {
		upstreamMessages = append(upstreamMessages, map[string]any{
			"role":    m.Role,
			"content": m.Content,
		})
	}

	if len(systemTexts) > 0 {
		combined := strings.Join(systemTexts, "\n\n")
		systemPair := []map[string]any{
			{"role": "user", "content": "[System Instructions]\n" + combined},
			{"role": "assistant", "content": "Understood. I will follow these instructions."},
		}
		upstreamMessages = append(systemPair, upstreamMessages...)
	}

	body := map[string]any{
		"stream":           true,
		"model":            targetModel,
		"messages":         upstreamMessages,
		"signature_prompt": latestUserContent,
		"params":           map[string]any{},
		"features": map[string]any{
			"image_generation": false,
			"web_search":       false,
			"auto_web_search":  autoWebSearch,
			"preview_mode":     true,
			"enable_thinking":  enableThinking,
		},
		"chat_id": chatID,
		"id":      uuid.New().String(),
	}

	if len(files) > 0 {
		var filesData []map[string]any
		for _, f := range files {
			var rawMap map[string]any
			if err := json.Unmarshal(f.File, &rawMap); err == nil {
				rawMap["type"] = f.Type
				rawMap["itemId"] = f.ItemID
				rawMap["media"] = f.Media
				rawMap["ref_user_msg_id"] = userMsgID
				filesData = append(filesData, rawMap)
			} else {
				filesData = append(filesData, map[string]any{
					"type":            f.Type,
					"id":              f.ID,
					"url":             f.URL,
					"name":            f.Name,
					"status":          f.Status,
					"size":            f.Size,
					"error":           f.Error,
					"itemId":          f.ItemID,
					"media":           f.Media,
					"ref_user_msg_id": userMsgID,
				})
			}
		}
		body["files"] = filesData
		body["current_user_message_id"] = userMsgID
	}

	bodyBytes, _ := json.Marshal(body)

	url := fmt.Sprintf(
		"https://chat.z.ai/api/v2/chat/completions?timestamp=%d&requestId=%s&user_id=%s&version=0.0.1&platform=web&token=%s&current_url=%s&pathname=%s&signature_timestamp=%d",
		timestamp, requestID, userID, token,
		fmt.Sprintf("https://chat.z.ai/c/%s", chatID),
		fmt.Sprintf("/c/%s", chatID),
		timestamp,
	)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-FE-Version", GetFeVersion())
	req.Header.Set("X-Signature", signature)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Origin", "https://chat.z.ai")
	req.Header.Set("Referer", fmt.Sprintf("https://chat.z.ai/c/%s", uuid.New().String()))
	req.Header.Set("User-Agent", uarand.GetRandom())

	resp, err := client.Do(req)
	return resp, targetModel, err
}

type upstreamData struct {
	Type string `json:"type"`
	Data struct {
		DeltaContent string `json:"delta_content"`
		EditContent  string `json:"edit_content"`
		Content      string `json:"content"`
		Phase        string `json:"phase"`
		Done         bool   `json:"done"`
		Error        *struct {
			Code   string `json:"code"`
			Detail string `json:"detail"`
		} `json:"error"`
	} `json:"data"`
}

func (u *upstreamData) getEditContent() string {
	ec := u.Data.EditContent
	if ec == "" {
		return ""
	}
	if len(ec) > 0 && ec[0] == '"' {
		var unescaped string
		if err := json.Unmarshal([]byte(ec), &unescaped); err == nil {
			return unescaped
		}
	}
	return ec
}

func collectNonStream(body io.Reader) (string, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var chunks []string
	totalOutputLen := 0

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			break
		}
		var u upstreamData
		if err := json.Unmarshal([]byte(payload), &u); err != nil {
			continue
		}
		if u.Data.Error != nil {
			if u.Data.Error.Code != "" && u.Data.Error.Detail != "" {
				return "", fmt.Errorf("model %s: %s", u.Data.Error.Code, u.Data.Error.Detail)
			}
			if u.Data.Error.Detail != "" {
				return "", fmt.Errorf("model error: %s", u.Data.Error.Detail)
			}
			if u.Data.Error.Code != "" {
				return "", fmt.Errorf("model error: %s", u.Data.Error.Code)
			}
			return "", fmt.Errorf("model error in stream response")
		}

		if u.Data.Phase == "" && u.Data.Content != "" {
			chunks = append(chunks, u.Data.Content)
			continue
		}

		if u.Data.Phase == "done" || u.Data.Done {
			break
		}
		if u.Data.Phase == "thinking" {
			continue
		}

		ec := u.getEditContent()

		if ec != "" {
			if strings.Contains(ec, `"search_result"`) ||
				strings.Contains(ec, `"search_image"`) ||
				strings.Contains(ec, `"mcp"`) {
				continue
			}
		}

		switch u.Data.Phase {
		case "answer":
			if u.Data.DeltaContent != "" {
				chunks = append(chunks, u.Data.DeltaContent)
			} else if ec != "" && strings.Contains(ec, "</details>") {
				if _, after, ok := strings.Cut(ec, "</details>"); ok {
					after := after
					after = strings.TrimPrefix(after, "\n")
					if after != "" {
						chunks = append(chunks, after)
					}
				}
			}
		case "other", "tool_call":
			if ec != "" {
				runes := []rune(ec)
				if len(runes) > totalOutputLen {
					newPart := string(runes[totalOutputLen:])
					totalOutputLen = len(runes)
					chunks = append(chunks, newPart)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[MODEL] scanner error: %v", err)
	}

	result := strings.TrimSpace(strings.Join(chunks, ""))
	if result == "" {
		return "", fmt.Errorf("empty response from model")
	}
	return result, nil
}

func collectOpenAIStream(body io.Reader) (string, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024)

	var chunks []string

	type openAIResponse struct {
		Error *struct {
			Code    string `json:"code"`
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			break
		}

		var chunk openAIResponse
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}

		if chunk.Error != nil {
			if chunk.Error.Code != "" && chunk.Error.Message != "" {
				return "", fmt.Errorf("provider %s: %s", chunk.Error.Code, chunk.Error.Message)
			}
			if chunk.Error.Type != "" && chunk.Error.Message != "" {
				return "", fmt.Errorf("provider %s: %s", chunk.Error.Type, chunk.Error.Message)
			}
			if chunk.Error.Message != "" {
				return "", fmt.Errorf("provider error: %s", chunk.Error.Message)
			}
			return "", fmt.Errorf("provider error in stream response")
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		if c := chunk.Choices[0].Delta.Content; c != "" {
			chunks = append(chunks, c)
			continue
		}
		if c := chunk.Choices[0].Message.Content; c != "" {
			chunks = append(chunks, c)
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("openai stream scanner: %w", err)
	}

	result := strings.TrimSpace(strings.Join(chunks, ""))
	if result == "" {
		return "", fmt.Errorf("empty response from provider")
	}
	return result, nil
}

func collectOpenAINonStream(body io.Reader) (string, error) {
	type openAIResponse struct {
		Error *struct {
			Code    string `json:"code"`
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	var resp openAIResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return "", fmt.Errorf("decode provider response: %w", err)
	}
	if resp.Error != nil {
		if resp.Error.Code != "" && resp.Error.Message != "" {
			return "", fmt.Errorf("provider %s: %s", resp.Error.Code, resp.Error.Message)
		}
		if resp.Error.Type != "" && resp.Error.Message != "" {
			return "", fmt.Errorf("provider %s: %s", resp.Error.Type, resp.Error.Message)
		}
		if resp.Error.Message != "" {
			return "", fmt.Errorf("provider error: %s", resp.Error.Message)
		}
		return "", fmt.Errorf("provider error")
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("empty response from provider")
	}
	result := strings.TrimSpace(resp.Choices[0].Message.Content)
	if result == "" {
		return "", fmt.Errorf("empty response from provider")
	}
	return result, nil
}

func extractLatestUserContent(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
}

func (c *Client) sendInternalOpenRouter(ctx context.Context, model string, messages []Message, files []*UpstreamFile) (Message, error) {
	ps := GetProviderSettings("openrouter")
	if ps.APIKey == "" {
		return Message{}, fmt.Errorf("missing OPENROUTER_API_KEY")
	}

	apiURL := "https://openrouter.ai/api/v1/chat/completions"
	if model == "" {
		model = ps.Model
	}

	imageURLs := collectImageURLs(messages, files)
	body := map[string]any{
		"model":     model,
		"messages":  toOpenAIMessages(messages, imageURLs),
		"reasoning": map[string]bool{"enabled": true},
		"stream":    ps.Stream,
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return Message{}, err
	}
	req.Header.Set("Authorization", "Bearer "+ps.APIKey)
	req.Header.Set("Content-Type", "application/json")
	if ps.Stream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return Message{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1000))
		return Message{}, fmt.Errorf("upstream %d: %s", resp.StatusCode, string(body))
	}

	if ps.Stream {
		return collectOpenAIStreamWithReasoning(resp.Body)
	}
	return collectOpenAINonStreamWithReasoning(resp.Body)
}

func collectOpenAIStreamWithReasoning(body io.Reader) (Message, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024)

	var chunks []string
	var reasoningDetails any

	type openAIResponse struct {
		Error *struct {
			Code    string `json:"code"`
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
		Choices []struct {
			Delta struct {
				Content          string `json:"content"`
				ReasoningDetails any    `json:"reasoning_details"`
			} `json:"delta"`
			Message struct {
				Content          string `json:"content"`
				ReasoningDetails any    `json:"reasoning_details"`
			} `json:"message"`
		} `json:"choices"`
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}

		var chunk openAIResponse
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}

		if chunk.Error != nil {
			msg := chunk.Error.Message
			if chunk.Error.Code != "" {
				return Message{}, fmt.Errorf("provider %s: %s", chunk.Error.Code, msg)
			}
			return Message{}, fmt.Errorf("provider error: %s", msg)
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		c := chunk.Choices[0].Delta.Content
		if c == "" {
			c = chunk.Choices[0].Message.Content
		}
		if c != "" {
			chunks = append(chunks, c)
		}

		r := chunk.Choices[0].Delta.ReasoningDetails
		if r == nil {
			r = chunk.Choices[0].Message.ReasoningDetails
		}
		if r != nil {
			reasoningDetails = r
		}
	}

	if err := scanner.Err(); err != nil {
		return Message{}, err
	}

	result := strings.TrimSpace(strings.Join(chunks, ""))
	return Message{Role: "assistant", Content: result, ReasoningDetails: reasoningDetails}, nil
}

func collectOpenAINonStreamWithReasoning(body io.Reader) (Message, error) {
	type openAIResponse struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningDetails any    `json:"reasoning_details"`
			} `json:"message"`
		} `json:"choices"`
	}

	var resp openAIResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return Message{}, err
	}
	if resp.Error != nil {
		return Message{}, fmt.Errorf("provider error: %s", resp.Error.Message)
	}
	if len(resp.Choices) == 0 {
		return Message{}, fmt.Errorf("empty response")
	}
	msg := resp.Choices[0].Message
	return Message{Role: "assistant", Content: msg.Content, ReasoningDetails: msg.ReasoningDetails}, nil
}
