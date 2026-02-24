package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var WebFetch = &ToolDef{
	Name:        "web_fetch",
	Description: "Fetch the plain-text content of a URL (no JavaScript execution)",
	Args: []ToolArg{
		{Name: "url", Description: "The full URL to fetch", Required: true},
	},
	Execute: func(args map[string]string) string {
		rawURL := args["url"]
		if rawURL == "" {
			return "Error: url is required"
		}
		if _, err := url.ParseRequestURI(rawURL); err != nil {
			return fmt.Sprintf("Error: invalid URL: %v", err)
		}
		client := &http.Client{Timeout: 20 * time.Second}
		req, err := http.NewRequest("GET", rawURL, nil)
		if err != nil {
			return fmt.Sprintf("Error building request: %v", err)
		}
		req.Header.Set("User-Agent", "ApexClaw/1.0")
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("Error fetching URL: %v", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
		if err != nil {
			return fmt.Sprintf("Error reading body: %v", err)
		}
		text := strings.TrimSpace(string(body))
		if len(text) > 6000 {
			text = text[:6000] + "\n...(truncated)"
		}
		return fmt.Sprintf("HTTP %d\n\n%s", resp.StatusCode, text)
	},
}

var WebSearch = &ToolDef{
	Name:        "web_search",
	Description: "Search the web using DuckDuckGo and return top results",
	Args: []ToolArg{
		{Name: "query", Description: "Search query string", Required: true},
	},
	Execute: func(args map[string]string) string {
		query := args["query"]
		if query == "" {
			return "Error: query is required"
		}

		apiURL := fmt.Sprintf(
			"https://api.duckduckgo.com/?q=%s&format=json&no_html=1&skip_disambig=1",
			url.QueryEscape(query),
		)

		client := &http.Client{Timeout: 15 * time.Second}
		req, _ := http.NewRequest("GET", apiURL, nil)
		req.Header.Set("User-Agent", "ApexClaw/1.0")
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("Search error: %v", err)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)

		var result struct {
			AbstractText  string `json:"AbstractText"`
			AbstractURL   string `json:"AbstractURL"`
			Answer        string `json:"Answer"`
			RelatedTopics []struct {
				Text     string `json:"Text"`
				FirstURL string `json:"FirstURL"`
			} `json:"RelatedTopics"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			return fmt.Sprintf("Error parsing results: %v", err)
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Search: %s\n\n", query))

		if result.Answer != "" {
			sb.WriteString(fmt.Sprintf("Answer: %s\n\n", result.Answer))
		}
		if result.AbstractText != "" {
			sb.WriteString(fmt.Sprintf("Summary: %s\nSource: %s\n\n", result.AbstractText, result.AbstractURL))
		}
		if len(result.RelatedTopics) > 0 {
			sb.WriteString("Related:\n")
			limit := min(len(result.RelatedTopics), 5)
			for _, t := range result.RelatedTopics[:limit] {
				if t.Text != "" {
					sb.WriteString(fmt.Sprintf("• %s\n  %s\n", t.Text, t.FirstURL))
				}
			}
		}

		out := strings.TrimSpace(sb.String())
		if out == fmt.Sprintf("Search: %s", query) {
			return "No results found. Try a different query."
		}
		return out
	},
}
