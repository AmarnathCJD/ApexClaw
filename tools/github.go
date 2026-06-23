package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// runGh executes `gh <argv...>` in dir with the given timeout.
// Returns (stdout, errOrEmpty). errOrEmpty is non-empty when the command
// failed (including missing gh binary, timeout, or non-zero exit) and is
// formatted as an "Error: ..." string ready for the model.
func runGh(ctx context.Context, dir string, timeout time.Duration, argv ...string) (string, string) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	c := exec.CommandContext(cctx, "gh", argv...)
	c.Dir = dir
	c.Env = append(os.Environ(),
		"GH_PROMPT_DISABLED=1",
		"GH_NO_UPDATE_NOTIFIER=1",
	)

	var stdout, stderr strings.Builder
	c.Stdout = &stdout
	c.Stderr = &stderr

	err := c.Run()
	out := stdout.String()
	errOut := strings.TrimSpace(stderr.String())

	if cctx.Err() == context.DeadlineExceeded {
		return out, fmt.Sprintf("Error: gh %s timed out after %s", strings.Join(argv, " "), timeout)
	}
	if err != nil {
		// Detect "gh not installed"
		if _, lookErr := exec.LookPath("gh"); lookErr != nil {
			return "", "Error: GitHub CLI ('gh') is not installed. Install from https://cli.github.com/"
		}
		if errOut == "" {
			errOut = err.Error()
		}
		return out, fmt.Sprintf("Error: gh failed: %s", errOut)
	}
	return out, ""
}

// fmtDuration produces a compact "MmSSs" / "SSs" string for two timestamps.
func ghFmtDuration(start, end string) string {
	if start == "" || end == "" {
		return ""
	}
	ts, errS := time.Parse(time.RFC3339, start)
	te, errE := time.Parse(time.RFC3339, end)
	if errS != nil || errE != nil {
		return ""
	}
	d := te.Sub(ts)
	if d <= 0 {
		return ""
	}
	total := int(d.Seconds())
	if total < 60 {
		return fmt.Sprintf("%ds", total)
	}
	return fmt.Sprintf("%dm%02ds", total/60, total%60)
}

// truncateForOutput trims s to max bytes, appending an ellipsis hint.
func ghTruncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "\n... (truncated)"
}

// ---- gh_pr_view ----

type ghPRAuthor struct {
	Login string `json:"login"`
}

type ghPRCheck struct {
	Name        string `json:"name"`
	Status      string `json:"status"`     // QUEUED, IN_PROGRESS, COMPLETED
	Conclusion  string `json:"conclusion"` // SUCCESS, FAILURE, ...
	State       string `json:"state"`      // for older statuses (SUCCESS, FAILURE, PENDING)
	StartedAt   string `json:"startedAt"`
	CompletedAt string `json:"completedAt"`
}

type ghPRReview struct {
	Author      ghPRAuthor `json:"author"`
	State       string     `json:"state"`
	SubmittedAt string     `json:"submittedAt"`
	Body        string     `json:"body"`
}

type ghPRJSON struct {
	Number            int           `json:"number"`
	Title             string        `json:"title"`
	State             string        `json:"state"`
	Author            ghPRAuthor    `json:"author"`
	Body              string        `json:"body"`
	HeadRefName       string        `json:"headRefName"`
	BaseRefName       string        `json:"baseRefName"`
	URL               string        `json:"url"`
	StatusCheckRollup []ghPRCheck   `json:"statusCheckRollup"`
	ReviewDecision    string        `json:"reviewDecision"`
	Reviews           []ghPRReview  `json:"reviews"`
	Mergeable         string        `json:"mergeable"`
	IsDraft           bool          `json:"isDraft"`
}

var GhPRView = &ToolDef{
	Name: "gh_pr_view",
	Description: "View a GitHub pull request with title, body, status, checks, and recent reviews. " +
		"Auto-detects current PR from branch if pr is omitted. Returns a clean human-readable summary.",
	Timeout:   30 * time.Second,
	MaxOutput: 32 * 1024,
	Args: []ToolArg{
		{Name: "repo", Type: ArgString, Description: "owner/name (default: current repo from cwd)", Required: false},
		{Name: "pr", Type: ArgString, Description: "PR number or branch name (default: current branch's PR)", Required: false},
		{Name: "include_diff", Type: ArgBool, Description: "Append the diff at the end (default: false)", Required: false},
		{Name: "cwd", Type: ArgString, Description: "Working directory", Required: false},
	},
	Execute: func(args map[string]any) string {
		dir, errMsg := resolveCwd(args)
		if errMsg != "" {
			return errMsg
		}
		repo := String(args, "repo")
		pr := String(args, "pr")
		includeDiff := BoolOr(args, "include_diff", false)

		fields := "number,title,state,author,body,headRefName,baseRefName,url,statusCheckRollup,reviewDecision,reviews,mergeable,isDraft"

		argv := []string{"pr", "view"}
		if pr != "" {
			argv = append(argv, pr)
		}
		if repo != "" {
			argv = append(argv, "--repo", repo)
		}
		argv = append(argv, "--json", fields)

		out, errStr := runGh(context.Background(), dir, 30*time.Second, argv...)
		if errStr != "" {
			return errStr
		}

		var info ghPRJSON
		if err := json.Unmarshal([]byte(out), &info); err != nil {
			return fmt.Sprintf("Error: failed to parse gh output: %v", err)
		}

		// Tally checks
		passing, failing, pending := 0, 0, 0
		for _, c := range info.StatusCheckRollup {
			concl := strings.ToUpper(c.Conclusion)
			state := strings.ToUpper(c.State)
			status := strings.ToUpper(c.Status)
			switch {
			case concl == "SUCCESS" || state == "SUCCESS":
				passing++
			case concl == "FAILURE" || concl == "TIMED_OUT" || concl == "CANCELLED" ||
				concl == "ACTION_REQUIRED" || state == "FAILURE" || state == "ERROR":
				failing++
			case status == "IN_PROGRESS" || status == "QUEUED" || state == "PENDING" || concl == "":
				pending++
			default:
				pending++
			}
		}

		draft := "no"
		if info.IsDraft {
			draft = "yes"
		}
		mergeable := strings.ToLower(info.Mergeable)
		if mergeable == "" {
			mergeable = "unknown"
		}
		reviewDecision := info.ReviewDecision
		if reviewDecision == "" {
			reviewDecision = "REVIEW_REQUIRED"
		}

		var b strings.Builder
		fmt.Fprintf(&b, "PR #%d — %s\n", info.Number, info.Title)
		fmt.Fprintf(&b, "Author: @%s | Branch: %s → %s | State: %s | Draft: %s | Mergeable: %s\n",
			info.Author.Login, info.HeadRefName, info.BaseRefName, info.State, draft, mergeable)
		fmt.Fprintf(&b, "URL: %s\n", info.URL)
		fmt.Fprintf(&b, "Review: %s\n", reviewDecision)
		fmt.Fprintf(&b, "Checks: %d passing, %d failing, %d pending\n", passing, failing, pending)

		// Recent reviews (last 5)
		if len(info.Reviews) > 0 {
			b.WriteString("---\nRecent reviews:\n")
			start := 0
			if len(info.Reviews) > 5 {
				start = len(info.Reviews) - 5
			}
			for _, r := range info.Reviews[start:] {
				snippet := strings.TrimSpace(r.Body)
				if len(snippet) > 200 {
					snippet = snippet[:200] + "..."
				}
				if snippet == "" {
					fmt.Fprintf(&b, "  @%s: %s\n", r.Author.Login, r.State)
				} else {
					fmt.Fprintf(&b, "  @%s: %s — %s\n", r.Author.Login, r.State, snippet)
				}
			}
		}

		b.WriteString("---\n")
		body := strings.TrimSpace(info.Body)
		if body == "" {
			b.WriteString("(no body)\n")
		} else {
			if len(body) > 2000 {
				body = body[:2000] + "\n... (body truncated at 2000 chars)"
			}
			b.WriteString(body)
			b.WriteString("\n")
		}

		if includeDiff {
			diffArgv := []string{"pr", "diff"}
			if pr != "" {
				diffArgv = append(diffArgv, pr)
			}
			if repo != "" {
				diffArgv = append(diffArgv, "--repo", repo)
			}
			diffOut, diffErr := runGh(context.Background(), dir, 30*time.Second, diffArgv...)
			b.WriteString("---\nDiff:\n")
			if diffErr != "" {
				b.WriteString(diffErr)
				b.WriteString("\n")
			} else {
				b.WriteString(ghTruncate(diffOut, 16*1024))
			}
		}

		return b.String()
	},
}

// ---- gh_run_view ----

type ghRunStep struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	Number     int    `json:"number"`
}

type ghRunJob struct {
	Name        string      `json:"name"`
	Status      string      `json:"status"`
	Conclusion  string      `json:"conclusion"`
	StartedAt   string      `json:"startedAt"`
	CompletedAt string      `json:"completedAt"`
	Steps       []ghRunStep `json:"steps"`
}

type ghRunJSON struct {
	WorkflowName string     `json:"workflowName"`
	Status       string     `json:"status"`
	Conclusion   string     `json:"conclusion"`
	CreatedAt    string     `json:"createdAt"`
	UpdatedAt    string     `json:"updatedAt"`
	Event        string     `json:"event"`
	HeadBranch   string     `json:"headBranch"`
	HeadSha      string     `json:"headSha"`
	URL          string     `json:"url"`
	Jobs         []ghRunJob `json:"jobs"`
}

type ghRunListEntry struct {
	DatabaseID   int64  `json:"databaseId"`
	WorkflowName string `json:"workflowName"`
	Status       string `json:"status"`
	Conclusion   string `json:"conclusion"`
}

var GhRunView = &ToolDef{
	Name: "gh_run_view",
	Description: "View a GitHub Actions CI run — status, conclusion, job breakdown, failing step (if any). " +
		"Auto-detects last run on current branch if run is omitted.",
	Timeout:   45 * time.Second,
	MaxOutput: 32 * 1024,
	Args: []ToolArg{
		{Name: "repo", Type: ArgString, Description: "owner/name (default: current repo from cwd)", Required: false},
		{Name: "run", Type: ArgString, Description: "Run ID (default: latest run on branch)", Required: false},
		{Name: "branch", Type: ArgString, Description: "Branch name (default: current branch). Used when run is omitted.", Required: false},
		{Name: "log_failures", Type: ArgBool, Description: "Fetch + tail logs from failing jobs (default: false)", Required: false},
		{Name: "cwd", Type: ArgString, Description: "Working directory", Required: false},
	},
	Execute: func(args map[string]any) string {
		dir, errMsg := resolveCwd(args)
		if errMsg != "" {
			return errMsg
		}
		repo := String(args, "repo")
		runID := String(args, "run")
		branch := String(args, "branch")
		logFailures := BoolOr(args, "log_failures", false)

		// Resolve run ID if not provided.
		if runID == "" {
			listArgv := []string{"run", "list", "--limit", "1", "--json", "databaseId,workflowName,status,conclusion"}
			if branch == "" {
				branch = currentBranch(dir)
			}
			if branch != "" {
				listArgv = append(listArgv, "--branch", branch)
			}
			if repo != "" {
				listArgv = append(listArgv, "--repo", repo)
			}
			out, errStr := runGh(context.Background(), dir, 30*time.Second, listArgv...)
			if errStr != "" {
				return errStr
			}
			var entries []ghRunListEntry
			if err := json.Unmarshal([]byte(out), &entries); err != nil {
				return fmt.Sprintf("Error: failed to parse gh run list output: %v", err)
			}
			if len(entries) == 0 {
				if branch != "" {
					return fmt.Sprintf("No GitHub Actions runs found on branch %q.", branch)
				}
				return "No GitHub Actions runs found."
			}
			runID = fmt.Sprintf("%d", entries[0].DatabaseID)
		}

		// View the run.
		viewArgv := []string{"run", "view", runID, "--json",
			"workflowName,status,conclusion,createdAt,updatedAt,event,headBranch,headSha,url,jobs"}
		if repo != "" {
			viewArgv = append(viewArgv, "--repo", repo)
		}
		out, errStr := runGh(context.Background(), dir, 30*time.Second, viewArgv...)
		if errStr != "" {
			return errStr
		}

		var info ghRunJSON
		if err := json.Unmarshal([]byte(out), &info); err != nil {
			return fmt.Sprintf("Error: failed to parse gh run view output: %v", err)
		}

		shortSha := info.HeadSha
		if len(shortSha) > 7 {
			shortSha = shortSha[:7]
		}

		var b strings.Builder
		fmt.Fprintf(&b, "Workflow: %s  Status: %s  Conclusion: %s\n",
			info.WorkflowName, info.Status, info.Conclusion)
		fmt.Fprintf(&b, "Event: %s | Branch: %s | SHA: %s | URL: %s\n",
			info.Event, info.HeadBranch, shortSha, info.URL)
		fmt.Fprintf(&b, "Started: %s  Ended: %s\n", info.CreatedAt, info.UpdatedAt)
		b.WriteString("---\nJobs:\n")

		hasFailures := false
		for _, j := range info.Jobs {
			mark := "•"
			concl := strings.ToLower(j.Conclusion)
			switch concl {
			case "success":
				mark = "✓"
			case "failure", "timed_out", "cancelled", "action_required":
				mark = "✗"
				hasFailures = true
			case "skipped":
				mark = "-"
			default:
				if strings.ToLower(j.Status) == "in_progress" || strings.ToLower(j.Status) == "queued" {
					mark = "…"
				}
			}
			label := j.Conclusion
			if label == "" {
				label = j.Status
			}
			dur := ghFmtDuration(j.StartedAt, j.CompletedAt)
			if dur != "" {
				fmt.Fprintf(&b, "  %s %s (%s, %s)\n", mark, j.Name, strings.ToLower(label), dur)
			} else {
				fmt.Fprintf(&b, "  %s %s (%s)\n", mark, j.Name, strings.ToLower(label))
			}
			// List failing steps inline.
			if mark == "✗" {
				for _, s := range j.Steps {
					if strings.EqualFold(s.Conclusion, "failure") {
						fmt.Fprintf(&b, "     - %q failed\n", s.Name)
					}
				}
			}
		}

		if logFailures && hasFailures {
			logArgv := []string{"run", "view", runID, "--log-failed"}
			if repo != "" {
				logArgv = append(logArgv, "--repo", repo)
			}
			logOut, logErr := runGh(context.Background(), dir, 45*time.Second, logArgv...)
			b.WriteString("---\nFailing logs (tail):\n")
			if logErr != "" {
				b.WriteString(logErr)
				b.WriteString("\n")
			} else {
				// Tail to ~200 lines.
				lines := strings.Split(logOut, "\n")
				if len(lines) > 200 {
					lines = lines[len(lines)-200:]
				}
				b.WriteString(strings.Join(lines, "\n"))
			}
		}

		return b.String()
	},
}
