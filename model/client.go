package model

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/corpix/uarand"
	"github.com/google/uuid"
)

type Message struct {
	Role    string         `json:"role"`
	Content string         `json:"content"`
	Files   []UpstreamFile `json:"-"`
}

type Client struct {
	http *http.Client
}

func New() *Client {
	return &Client{http: &http.Client{Timeout: 90 * time.Second}}
}

func (c *Client) Send(ctx context.Context, model string, messages []Message) (string, error) {
	return c.sendInternal(ctx, model, messages, nil)
}

func (c *Client) SendWithFiles(ctx context.Context, model string, messages []Message, files []*UpstreamFile) (string, error) {
	return c.sendInternal(ctx, model, messages, files)
}

func (c *Client) sendInternal(ctx context.Context, model string, messages []Message, files []*UpstreamFile) (string, error) {
	token, err := GetAnonymousToken()
	if err != nil {
		return "", fmt.Errorf("auth: %w", err)
	}
	resp, targetModel, err := makeUpstreamRequest(ctx, c.http, token, messages, model, files)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return "", fmt.Errorf("upstream %d: %s", resp.StatusCode, string(body))
	}
	_ = targetModel
	return collectNonStream(resp.Body)
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

	var upstreamMessages []map[string]any
	for _, m := range messages {
		upstreamMessages = append(upstreamMessages, map[string]any{
			"role":    m.Role,
			"content": m.Content,
		})
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
		"chat_id":                 chatID,
		"id":                      uuid.New().String(),
		"current_user_message_id": userMsgID,
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
	}

	bodyBytes, _ := json.Marshal(body)

	if len(files) > 0 {
		log.Printf("[MODEL] image request body: %s", string(bodyBytes))
	}

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
		Phase        string `json:"phase"`
		Done         bool   `json:"done"`
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
		if u.Data.Phase == "done" {
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

				if idx := strings.Index(ec, "</details>"); idx != -1 {
					after := ec[idx+len("</details>"):]
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

func extractLatestUserContent(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
}
