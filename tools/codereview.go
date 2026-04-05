package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"apexclaw/model"
)

var CodeReview = &ToolDef{
	Name:        "code_review",
	Description: "Perform a deep AI-powered code review on a local folder or GitHub repo URL. Returns a structured Markdown report with issues, security findings, architecture notes, and actionable recommendations.",
	Args: []ToolArg{
		{Name: "path", Description: "Local folder path OR GitHub repo URL (e.g. github.com/user/repo or /home/user/myapp)", Required: true},
		{Name: "focus", Description: "Review focus: 'security', 'performance', 'architecture', 'bugs', 'all' (default: all)", Required: false},
		{Name: "depth", Description: "Review depth: 'quick' (top-level scan), 'deep' (default, full analysis)", Required: false},
		{Name: "lang", Description: "Hint the primary language (optional, auto-detected if omitted)", Required: false},
	},
	Execute: func(args map[string]string) string {
		path := args["path"]
		if path == "" {
			return "Error: path is required"
		}
		focus := args["focus"]
		if focus == "" {
			focus = "all"
		}
		depth := args["depth"]
		if depth == "" {
			depth = "deep"
		}
		lang := args["lang"]

		if strings.HasPrefix(path, "github.com/") || strings.HasPrefix(path, "https://github.com/") {
			return reviewGitHubRepo(path, focus, depth, lang)
		}
		return reviewLocalDir(path, focus, depth, lang)
	},
}

func reviewLocalDir(dir, focus, depth, lang string) string {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Sprintf("Error: cannot access path %q: %v", dir, err)
	}
	if !info.IsDir() {
		return fmt.Sprintf("Error: %q is not a directory", dir)
	}

	var files []string
	maxFiles := 30
	if depth == "quick" {
		maxFiles = 10
	}

	codeExts := map[string]bool{
		".go": true, ".py": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
		".rs": true, ".java": true, ".kt": true, ".cs": true, ".cpp": true, ".c": true,
		".rb": true, ".php": true, ".swift": true, ".dart": true, ".sh": true,
	}

	err = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || len(files) >= maxFiles {
			return nil
		}
		name := d.Name()
		if d.IsDir() && (name == "node_modules" || name == ".git" || name == "vendor" || name == "dist" || name == "build") {
			return filepath.SkipDir
		}
		if !d.IsDir() && codeExts[strings.ToLower(filepath.Ext(name))] {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return fmt.Sprintf("Error walking directory: %v", err)
	}
	if len(files) == 0 {
		return "No code files found in the specified directory."
	}

	var codeBuilder strings.Builder
	totalChars := 0
	maxChars := 12000
	if depth == "quick" {
		maxChars = 4000
	}

	for _, f := range files {
		if totalChars >= maxChars {
			break
		}
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		rel, _ := filepath.Rel(dir, f)
		snippet := string(data)
		remaining := maxChars - totalChars
		if len(snippet) > remaining {
			snippet = snippet[:remaining] + "\n... (truncated)"
		}
		fmt.Fprintf(&codeBuilder, "\n### File: %s\n```\n%s\n```\n", rel, snippet)
		totalChars += len(snippet)
	}

	return runCodeReviewLLM(codeBuilder.String(), dir, focus, lang, len(files))
}

func reviewGitHubRepo(repoURL, focus, depth, lang string) string {
	repoURL = strings.TrimPrefix(repoURL, "https://")
	repoURL = strings.TrimPrefix(repoURL, "http://")
	parts := strings.SplitN(strings.TrimPrefix(repoURL, "github.com/"), "/", 3)
	if len(parts) < 2 {
		return "Error: invalid GitHub URL. Expected github.com/owner/repo"
	}
	owner, repo := parts[0], parts[1]

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/git/trees/HEAD?recursive=1", owner, repo)
	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Sprintf("Error fetching repo tree: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 128*1024))

	codeExts := []string{".go", ".py", ".js", ".ts", ".jsx", ".tsx", ".rs", ".java", ".kt", ".rb", ".php"}
	var filePaths []string
	bodyStr := string(body)
	for _, ext := range codeExts {
		ext = `"` + ext + `"`
		idx := 0
		for {
			pos := strings.Index(bodyStr[idx:], `"path":"`)
			if pos < 0 {
				break
			}
			pos += idx + 8
			end := strings.Index(bodyStr[pos:], `"`)
			if end < 0 {
				break
			}
			path := bodyStr[pos : pos+end]
			if strings.HasSuffix(path, strings.Trim(ext, `"`)) {
				filePaths = append(filePaths, path)
			}
			idx = pos + end
		}
	}

	maxFiles := 20
	if depth == "quick" {
		maxFiles = 6
	}
	if len(filePaths) > maxFiles {
		filePaths = filePaths[:maxFiles]
	}

	var codeBuilder strings.Builder
	totalChars := 0
	maxChars := 10000
	for _, path := range filePaths {
		if totalChars >= maxChars {
			break
		}
		rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/HEAD/%s", owner, repo, path)
		req, _ := http.NewRequest("GET", rawURL, nil)
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		remaining := maxChars - totalChars
		snippet := string(data)
		if len(snippet) > remaining {
			snippet = snippet[:remaining]
		}
		fmt.Fprintf(&codeBuilder, "\n### File: %s\n```\n%s\n```\n", path, snippet)
		totalChars += len(snippet)
	}

	label := fmt.Sprintf("github.com/%s/%s", owner, repo)
	return runCodeReviewLLM(codeBuilder.String(), label, focus, lang, len(filePaths))
}

func runCodeReviewLLM(code, location, focus, lang string, fileCount int) string {
	focusDesc := map[string]string{
		"security":     "Focus heavily on: SQL injection, XSS, auth bypasses, hardcoded secrets, insecure deserialization, SSRF, path traversal, and other OWASP Top 10 issues.",
		"performance":  "Focus heavily on: N+1 queries, inefficient algorithms (O(n²)+), memory leaks, unnecessary allocations, blocking I/O, missing caching opportunities, and bottlenecks.",
		"architecture": "Focus heavily on: separation of concerns, coupling/cohesion, SOLID principles, testability, scalability design patterns, and structural anti-patterns.",
		"bugs":         "Focus heavily on: off-by-one errors, null/nil dereferences, race conditions, incorrect error handling, logic bugs, and edge cases.",
		"all":          "Cover all categories: security vulnerabilities, performance issues, architecture problems, potential bugs, code quality, and maintainability.",
	}
	focusStr := focusDesc[focus]
	if focusStr == "" {
		focusStr = focusDesc["all"]
	}
	langHint := ""
	if lang != "" {
		langHint = fmt.Sprintf("Primary language: %s\n", lang)
	}

	prompt := fmt.Sprintf(`You are a senior software engineer performing a deep, expert-level code review.

Repository/Project: %s
Files analyzed: %d
%s
Review Focus: %s

## Code to Review:
%s

## Your Task:
Produce a structured Markdown code review report with these sections:

### 🔍 Executive Summary
2-3 sentence overview of the codebase quality, main concerns, and overall impression.

### 🚨 Critical Issues (Must Fix)
List issues that could cause security vulnerabilities, data loss, crashes, or major bugs.
Format: **[CATEGORY]** Description → File:line (if known) → Recommended fix

### ⚠️ Warnings (Should Fix)
Important but non-critical issues: performance problems, code smells, missing error handling.

### 💡 Suggestions (Nice to Have)
Architecture improvements, refactoring opportunities, better patterns to consider.

### 🔒 Security Analysis
Specific security assessment: auth, input validation, secrets, API exposure.

### 📊 Code Quality Score
Rate each dimension 1-10:
- Security: X/10
- Performance: X/10
- Maintainability: X/10
- Test Coverage (inferred): X/10
- Overall: X/10

### 🚀 Top 3 Action Items
The 3 most important things to do next, in priority order.

Be specific, actionable, and reference actual code when possible. Don't be generic.`, location, fileCount, langHint, focusStr, code)

	client := model.New()
	messages := []model.Message{{Role: "user", Content: prompt}}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	reply, err := client.Send(ctx, "claude-sonnet-4-6", messages)
	if err != nil {
		return fmt.Sprintf("Error: code review failed: %v", err)
	}
	return strings.TrimSpace(reply.Content)
}
