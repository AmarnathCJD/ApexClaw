package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
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
		if err := ValidateExternalURL(rawURL); err != nil {
			return fmt.Sprintf("Error: %v", err)
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
			fmt.Fprintf(&sb, "Answer: %s\n\n", result.Answer)
		}
		if result.AbstractText != "" {
			fmt.Fprintf(&sb, "Summary: %s\nSource: %s\n\n", result.AbstractText, result.AbstractURL)
		}
		if len(result.RelatedTopics) > 0 {
			sb.WriteString("Related:\n")
			limit := min(len(result.RelatedTopics), 5)
			for _, t := range result.RelatedTopics[:limit] {
				if t.Text != "" {
					fmt.Fprintf(&sb, "• %s\n  %s\n", t.Text, t.FirstURL)
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

var TavilySearch = &ToolDef{
	Name:        "tavily_search",
	Description: "Search the web using Tavily API with advanced options (requires TAVILY_KEY env var)",
	Args: []ToolArg{
		{Name: "query", Description: "Search query string", Required: true},
		{Name: "topic", Description: "Topic type: 'general' or 'news' (default: general)", Required: false},
		{Name: "search_depth", Description: "Search depth: 'basic' or 'advanced' (default: basic)", Required: false},
		{Name: "max_results", Description: "Max results to return, 1-10 (default: 5)", Required: false},
		{Name: "include_answer", Description: "Include AI-generated answer: 'true' or 'false' (default: false)", Required: false},
		{Name: "include_raw_content", Description: "Include raw page content: 'true' or 'false' (default: false)", Required: false},
	},
	Execute: func(args map[string]string) string {
		apiKey := os.Getenv("TAVILY_KEY")
		if apiKey == "" {
			return "Error: TAVILY_KEY environment variable not configured. Set it to use Tavily search."
		}

		query := args["query"]
		if query == "" {
			return "Error: query is required"
		}

		topic := args["topic"]
		if topic == "" {
			topic = "general"
		}
		searchDepth := args["search_depth"]
		if searchDepth == "" {
			searchDepth = "basic"
		}
		maxResults := 5
		if m := args["max_results"]; m != "" {
			fmt.Sscanf(m, "%d", &maxResults)
			if maxResults < 1 || maxResults > 10 {
				maxResults = 5
			}
		}
		includeAnswer := args["include_answer"] == "true"
		includeRaw := args["include_raw_content"] == "true"

		payload := map[string]interface{}{
			"query":               query,
			"auto_parameters":     false,
			"topic":               topic,
			"search_depth":        searchDepth,
			"chunks_per_source":   3,
			"max_results":         maxResults,
			"include_answer":      includeAnswer,
			"include_raw_content": includeRaw,
			"include_images":      false,
			"include_favicon":     false,
		}

		body, err := json.Marshal(payload)
		if err != nil {
			return fmt.Sprintf("Error encoding request: %v", err)
		}

		client := &http.Client{Timeout: 30 * time.Second}
		req, _ := http.NewRequest("POST", "https://api.tavily.com/search", bytes.NewBuffer(body))
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("Error calling Tavily API: %v", err)
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)

		var result struct {
			Answer  string `json:"answer"`
			Results []struct {
				Title   string  `json:"title"`
				URL     string  `json:"url"`
				Content string  `json:"content"`
				Score   float64 `json:"score"`
			} `json:"results"`
		}

		if err := json.Unmarshal(respBody, &result); err != nil {
			return fmt.Sprintf("Error parsing response: %v", err)
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Tavily Search: %s\n\n", query))

		if result.Answer != "" && includeAnswer {
			sb.WriteString(fmt.Sprintf("Answer: %s\n\n", result.Answer))
		}

		if len(result.Results) > 0 {
			sb.WriteString("Results:\n")
			for i, r := range result.Results {
				fmt.Fprintf(&sb, "%d. <b>%s</b>\n", i+1, r.Title)
				fmt.Fprintf(&sb, "   URL: %s\n", r.URL)
				if r.Content != "" && !includeRaw {
					content := r.Content
					if len(content) > 200 {
						content = content[:200] + "..."
					}
					fmt.Fprintf(&sb, "   %s\n", content)
				} else if includeRaw && r.Content != "" {
					fmt.Fprintf(&sb, "   Content: %s\n", r.Content)
				}
				sb.WriteString("\n")
			}
		} else {
			sb.WriteString("No results found.\n")
		}

		return strings.TrimSpace(sb.String())
	},
}

var TavilyExtract = &ToolDef{
	Name:        "tavily_extract",
	Description: "Extract and process content from URLs using Tavily API (requires TAVILY_KEY env var)",
	Args: []ToolArg{
		{Name: "urls", Description: "Comma-separated URLs to extract from", Required: true},
		{Name: "query", Description: "Query to guide extraction (optional)", Required: false},
		{Name: "extract_depth", Description: "Extract depth: 'basic' or 'advanced' (default: basic)", Required: false},
		{Name: "chunks_per_source", Description: "Chunks per source, 1-10 (default: 3)", Required: false},
		{Name: "format", Description: "Output format: 'markdown' or 'raw' (default: markdown)", Required: false},
	},
	Execute: func(args map[string]string) string {
		apiKey := os.Getenv("TAVILY_KEY")
		if apiKey == "" {
			return "Error: TAVILY_KEY environment variable not configured. Set it to use Tavily extract."
		}

		urls := args["urls"]
		if urls == "" {
			return "Error: urls is required"
		}

		extractDepth := args["extract_depth"]
		if extractDepth == "" {
			extractDepth = "basic"
		}
		chunksPerSource := 3
		if c := args["chunks_per_source"]; c != "" {
			fmt.Sscanf(c, "%d", &chunksPerSource)
			if chunksPerSource < 1 || chunksPerSource > 10 {
				chunksPerSource = 3
			}
		}
		format := args["format"]
		if format == "" {
			format = "markdown"
		}
		query := args["query"]

		payload := map[string]interface{}{
			"urls":              urls,
			"chunks_per_source": chunksPerSource,
			"extract_depth":     extractDepth,
			"include_images":    false,
			"include_favicon":   false,
			"format":            format,
		}
		if query != "" {
			payload["query"] = query
		}

		body, err := json.Marshal(payload)
		if err != nil {
			return fmt.Sprintf("Error encoding request: %v", err)
		}

		client := &http.Client{Timeout: 30 * time.Second}
		req, _ := http.NewRequest("POST", "https://api.tavily.com/extract", bytes.NewBuffer(body))
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("Error calling Tavily API: %v", err)
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)

		var result struct {
			Results []struct {
				URL     string `json:"url"`
				Content string `json:"content"`
			} `json:"results"`
		}

		if err := json.Unmarshal(respBody, &result); err != nil {
			return fmt.Sprintf("Error parsing response: %v", err)
		}

		var sb strings.Builder
		sb.WriteString("Extracted Content:\n\n")

		for _, r := range result.Results {
			sb.WriteString(fmt.Sprintf("<b>Source: %s</b>\n", r.URL))
			content := r.Content
			if len(content) > 2000 {
				content = content[:2000] + "\n...(truncated)"
			}
			sb.WriteString(content + "\n\n")
		}

		if len(result.Results) == 0 {
			return "No content extracted from the provided URLs."
		}

		return strings.TrimSpace(sb.String())
	},
}

var TavilyResearch = &ToolDef{
	Name:        "tavily_research",
	Description: "Advanced research using Tavily with structured output schema (requires TAVILY_KEY env var)",
	Args: []ToolArg{
		{Name: "query", Description: "Research query", Required: true},
		{Name: "model", Description: "Model to use: 'auto' or specific model (default: auto)", Required: false},
		{Name: "stream", Description: "Stream results: 'true' or 'false' (default: false)", Required: false},
		{Name: "citation_format", Description: "Citation format: 'numbered' or 'inline' (default: numbered)", Required: false},
	},
	Execute: func(args map[string]string) string {
		apiKey := os.Getenv("TAVILY_KEY")
		if apiKey == "" {
			return "Error: TAVILY_KEY environment variable not configured. Set it to use Tavily research."
		}

		query := args["query"]
		if query == "" {
			return "Error: query is required"
		}

		model := args["model"]
		if model == "" {
			model = "auto"
		}
		stream := args["stream"] == "true"
		citationFormat := args["citation_format"]
		if citationFormat == "" {
			citationFormat = "numbered"
		}

		payload := map[string]interface{}{
			"input":           query,
			"model":           model,
			"stream":          stream,
			"citation_format": citationFormat,
		}

		body, err := json.Marshal(payload)
		if err != nil {
			return fmt.Sprintf("Error encoding request: %v", err)
		}

		client := &http.Client{Timeout: 60 * time.Second}
		req, _ := http.NewRequest("POST", "https://api.tavily.com/research", bytes.NewBuffer(body))
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("Error calling Tavily API: %v", err)
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)

		var result struct {
			Result string `json:"result"`
			Error  string `json:"error"`
		}

		if err := json.Unmarshal(respBody, &result); err != nil {
			return fmt.Sprintf("Error parsing response: %v", err)
		}

		if result.Error != "" {
			return fmt.Sprintf("Tavily API error: %s", result.Error)
		}

		if result.Result == "" {
			return "No results from research API."
		}

		output := result.Result
		if len(output) > 3000 {
			output = output[:3000] + "\n\n...(truncated, full result available upon request)"
		}

		return output
	},
}
