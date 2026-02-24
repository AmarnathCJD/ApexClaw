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

var GitHubSearch = &ToolDef{
	Name:        "github_search",
	Description: "Search GitHub repositories or code. Type can be 'repositories', 'code', 'issues', or 'users'. No auth required for public results.",
	Args: []ToolArg{
		{Name: "query", Description: "Search query (e.g. 'language:go telegram bot', 'org:google stars:>1000')", Required: true},
		{Name: "type", Description: "What to search: 'repositories' (default), 'code', 'issues', 'users'", Required: false},
		{Name: "limit", Description: "Max results to return (default 5, max 10)", Required: false},
	},
	Execute: func(args map[string]string) string {
		query := args["query"]
		if query == "" {
			return "Error: query is required"
		}
		searchType := args["type"]
		if searchType == "" {
			searchType = "repositories"
		}
		limit := 5
		if args["limit"] != "" {
			fmt.Sscanf(args["limit"], "%d", &limit)
		}
		if limit > 10 {
			limit = 10
		}

		apiURL := fmt.Sprintf("https://api.github.com/search/%s?q=%s&per_page=%d",
			searchType, url.QueryEscape(query), limit)

		client := &http.Client{Timeout: 15 * time.Second}
		req, _ := http.NewRequest("GET", apiURL, nil)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "ApexClawAIAssistant/1.0")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)

		if resp.StatusCode != http.StatusOK {
			return fmt.Sprintf("GitHub API error (status %d): %s", resp.StatusCode, string(body))
		}

		var result struct {
			TotalCount int               `json:"total_count"`
			Items      []json.RawMessage `json:"items"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return fmt.Sprintf("Error parsing response: %v", err)
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("GitHub %s search for %q — %d total results\n\n", searchType, query, result.TotalCount))
		for i, item := range result.Items {
			var m map[string]any
			json.Unmarshal(item, &m)

			switch searchType {
			case "repositories":
				sb.WriteString(fmt.Sprintf("%d. %v\n   %v\n   ⭐ %v  |  Language: %v\n   %v\n\n",
					i+1, m["full_name"], m["description"], m["stargazers_count"], m["language"], m["html_url"]))
			case "code":
				sb.WriteString(fmt.Sprintf("%d. %v\n   Repo: %v\n   Path: %v\n   %v\n\n",
					i+1, m["name"], func() string {
						if r, ok := m["repository"].(map[string]any); ok {
							return fmt.Sprintf("%v", r["full_name"])
						}
						return "?"
					}(), m["path"], m["html_url"]))
			default:

				sb.WriteString(fmt.Sprintf("%d. %v  —  %v\n\n", i+1, m["name"], m["html_url"]))
			}
		}
		return strings.TrimSpace(sb.String())
	},
}

var GitHubReadFile = &ToolDef{
	Name:        "github_read_file",
	Description: "Read the raw contents of a file from a GitHub repository.",
	Args: []ToolArg{
		{Name: "repo", Description: "Repository in 'owner/repo' format (e.g. 'amarnathcjd/gogram')", Required: true},
		{Name: "path", Description: "File path within the repo (e.g. 'README.md' or 'internal/chat.go')", Required: true},
		{Name: "branch", Description: "Branch or commit ref (defaults to 'main')", Required: false},
	},
	Execute: func(args map[string]string) string {
		repo := args["repo"]
		path := args["path"]
		if repo == "" || path == "" {
			return "Error: repo and path are required"
		}
		branch := args["branch"]
		if branch == "" {
			branch = "main"
		}
		rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", repo, branch, path)

		client := &http.Client{Timeout: 15 * time.Second}
		req, _ := http.NewRequest("GET", rawURL, nil)
		req.Header.Set("User-Agent", "ApexClawAIAssistant/1.0")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {

			if branch == "main" {
				rawURL = fmt.Sprintf("https://raw.githubusercontent.com/%s/master/%s", repo, path)
				req2, _ := http.NewRequest("GET", rawURL, nil)
				req2.Header.Set("User-Agent", "ApexClawAIAssistant/1.0")
				resp2, err2 := client.Do(req2)
				if err2 == nil && resp2.StatusCode == http.StatusOK {
					body, _ := io.ReadAll(resp2.Body)
					resp2.Body.Close()
					result := string(body)
					if len(result) > 6000 {
						result = result[:6000] + "\n...(truncated)"
					}
					return result
				}
			}
			return fmt.Sprintf("File not found: %s/%s@%s", repo, path, branch)
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Sprintf("GitHub error (status %d): %s", resp.StatusCode, string(body))
		}

		body, _ := io.ReadAll(resp.Body)
		result := string(body)
		if len(result) > 6000 {
			result = result[:6000] + "\n...(truncated)"
		}
		return result
	},
}
