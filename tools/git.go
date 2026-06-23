package tools

import (
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"time"
)

// resolveCwd validates the cwd arg. If empty, falls back to os.Getwd().
// Returns absolute path or an error string (prefixed with "Error:").
func resolveCwd(args map[string]any) (string, string) {
	cwd := String(args, "cwd")
	if cwd == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Sprintf("Error: cannot determine working directory: %v", err)
		}
		return wd, ""
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Sprintf("Error: invalid cwd %q: %v", cwd, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Sprintf("Error: cwd does not exist: %s", abs)
	}
	if !info.IsDir() {
		return "", fmt.Sprintf("Error: cwd is not a directory: %s", abs)
	}
	return abs, ""
}

// runGit executes `git <argv...>` in dir with the given timeout.
// Returns combined stdout+stderr. On error, the output is prefixed with "Error: ...".
func runGit(dir string, timeout time.Duration, argv ...string) string {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	c := osexec.CommandContext(ctx, "git", argv...)
	c.Dir = dir
	c.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=echo",
		"GIT_PAGER=cat",
	)

	out, err := c.CombinedOutput()
	result := strings.TrimSpace(string(out))

	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Sprintf("Error: git %s timed out after %s\n%s",
			strings.Join(argv, " "), timeout, result)
	}
	if err != nil {
		if result == "" {
			return fmt.Sprintf("Error: git %s failed: %v", strings.Join(argv, " "), err)
		}
		return fmt.Sprintf("Error: git %s failed: %v\n%s",
			strings.Join(argv, " "), err, result)
	}
	if result == "" {
		return "(ok, no output)"
	}
	return result
}

// currentBranch returns the current branch name or "" if detection fails.
func currentBranch(dir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c := osexec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := c.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ---- read-only tools ----

var GitStatus = &ToolDef{
	Name:        "git_status",
	Description: "Show git working tree status (short + branch info). Read-only.",
	Secure:      false,
	Timeout:     30 * time.Second,
	MaxOutput:   16 * 1024,
	Args: []ToolArg{
		{Name: "cwd", Type: ArgString, Description: "Repo directory (default: current working directory)", Required: false},
	},
	Execute: func(args map[string]any) string {
		dir, errMsg := resolveCwd(args)
		if errMsg != "" {
			return errMsg
		}
		return runGit(dir, 30*time.Second, "status", "--short", "--branch")
	},
}

var GitDiff = &ToolDef{
	Name:        "git_diff",
	Description: "Show git diff. Optionally limit to a single file, or show staged changes only. Read-only.",
	Secure:      false,
	Timeout:     30 * time.Second,
	MaxOutput:   32 * 1024,
	Args: []ToolArg{
		{Name: "cwd", Type: ArgString, Description: "Repo directory (default: current working directory)", Required: false},
		{Name: "file", Type: ArgString, Description: "Restrict diff to a single file path", Required: false},
		{Name: "staged", Type: ArgBool, Description: "Show staged (cached) diff instead of unstaged (default: false)", Required: false},
	},
	Execute: func(args map[string]any) string {
		dir, errMsg := resolveCwd(args)
		if errMsg != "" {
			return errMsg
		}
		argv := []string{"diff"}
		if BoolOr(args, "staged", false) {
			argv = append(argv, "--staged")
		}
		if file := String(args, "file"); file != "" {
			argv = append(argv, "--", file)
		}
		return runGit(dir, 30*time.Second, argv...)
	},
}

var GitLog = &ToolDef{
	Name:        "git_log",
	Description: "Show git commit history. Read-only.",
	Secure:      false,
	Timeout:     30 * time.Second,
	MaxOutput:   16 * 1024,
	Args: []ToolArg{
		{Name: "cwd", Type: ArgString, Description: "Repo directory (default: current working directory)", Required: false},
		{Name: "count", Type: ArgInt, Description: "Number of commits to show (default 20, max 200)", Required: false},
		{Name: "oneline", Type: ArgBool, Description: "Use --oneline compact format (default: true)", Required: false},
		{Name: "file", Type: ArgString, Description: "Restrict log to a single file path", Required: false},
	},
	Execute: func(args map[string]any) string {
		dir, errMsg := resolveCwd(args)
		if errMsg != "" {
			return errMsg
		}
		count := IntOr(args, "count", 20)
		if count < 1 {
			count = 20
		}
		if count > 200 {
			count = 200
		}
		argv := []string{"log", fmt.Sprintf("-n%d", count)}
		if BoolOr(args, "oneline", true) {
			argv = append(argv, "--oneline")
		}
		if file := String(args, "file"); file != "" {
			argv = append(argv, "--", file)
		}
		return runGit(dir, 30*time.Second, argv...)
	},
}

var GitShow = &ToolDef{
	Name:        "git_show",
	Description: "Show a git commit (metadata + optional diff). Read-only.",
	Secure:      false,
	Timeout:     30 * time.Second,
	MaxOutput:   32 * 1024,
	Args: []ToolArg{
		{Name: "cwd", Type: ArgString, Description: "Repo directory (default: current working directory)", Required: false},
		{Name: "ref", Type: ArgString, Description: "Commit/ref to show (default: HEAD)", Required: false},
		{Name: "show_diff", Type: ArgBool, Description: "Include diff (default: true). When false, shows metadata only.", Required: false},
	},
	Execute: func(args map[string]any) string {
		dir, errMsg := resolveCwd(args)
		if errMsg != "" {
			return errMsg
		}
		ref := StringOr(args, "ref", "HEAD")
		argv := []string{"show"}
		if !BoolOr(args, "show_diff", true) {
			argv = append(argv, "--no-patch")
		}
		argv = append(argv, ref)
		return runGit(dir, 30*time.Second, argv...)
	},
}

var GitBranchList = &ToolDef{
	Name:        "git_branch_list",
	Description: "List git branches. Read-only.",
	Secure:      false,
	Timeout:     30 * time.Second,
	MaxOutput:   8 * 1024,
	Args: []ToolArg{
		{Name: "cwd", Type: ArgString, Description: "Repo directory (default: current working directory)", Required: false},
		{Name: "all", Type: ArgBool, Description: "Include remote branches (git branch -a). Default: false (local only)", Required: false},
	},
	Execute: func(args map[string]any) string {
		dir, errMsg := resolveCwd(args)
		if errMsg != "" {
			return errMsg
		}
		argv := []string{"branch"}
		if BoolOr(args, "all", false) {
			argv = append(argv, "-a")
		}
		return runGit(dir, 30*time.Second, argv...)
	},
}

// ---- write tools (owner-only) ----

var GitCommit = &ToolDef{
	Name:        "git_commit",
	Description: "Create a git commit. Optionally stages all changes first (add_all). Owner-only.",
	Secure:      true,
	Timeout:     30 * time.Second,
	MaxOutput:   16 * 1024,
	Args: []ToolArg{
		{Name: "cwd", Type: ArgString, Description: "Repo directory (default: current working directory)", Required: false},
		{Name: "message", Type: ArgString, Description: "Commit message", Required: true},
		{Name: "add_all", Type: ArgBool, Description: "Run `git add -A` before committing (default: false)", Required: false},
	},
	Execute: func(args map[string]any) string {
		message := String(args, "message")
		if message == "" {
			return "Error: message is required"
		}
		dir, errMsg := resolveCwd(args)
		if errMsg != "" {
			return errMsg
		}
		var outputs []string
		if BoolOr(args, "add_all", false) {
			addOut := runGit(dir, 30*time.Second, "add", "-A")
			outputs = append(outputs, "$ git add -A\n"+addOut)
			if strings.HasPrefix(addOut, "Error:") {
				return strings.Join(outputs, "\n\n")
			}
		}
		commitOut := runGit(dir, 30*time.Second, "commit", "-m", message)
		outputs = append(outputs, "$ git commit -m ...\n"+commitOut)
		return strings.Join(outputs, "\n\n")
	},
}

var GitBranchCreate = &ToolDef{
	Name:        "git_branch_create",
	Description: "Create a new git branch. By default also checks out the new branch. Owner-only.",
	Secure:      true,
	Timeout:     30 * time.Second,
	MaxOutput:   8 * 1024,
	Args: []ToolArg{
		{Name: "cwd", Type: ArgString, Description: "Repo directory (default: current working directory)", Required: false},
		{Name: "name", Type: ArgString, Description: "New branch name", Required: true},
		{Name: "from", Type: ArgString, Description: "Base ref to branch from (default: current HEAD)", Required: false},
		{Name: "checkout", Type: ArgBool, Description: "Check out the new branch after creating (default: true)", Required: false},
	},
	Execute: func(args map[string]any) string {
		name := String(args, "name")
		if name == "" {
			return "Error: name is required"
		}
		dir, errMsg := resolveCwd(args)
		if errMsg != "" {
			return errMsg
		}
		from := String(args, "from")
		checkout := BoolOr(args, "checkout", true)

		var argv []string
		if checkout {
			argv = []string{"checkout", "-b", name}
			if from != "" {
				argv = append(argv, from)
			}
		} else {
			argv = []string{"branch", name}
			if from != "" {
				argv = append(argv, from)
			}
		}
		return runGit(dir, 30*time.Second, argv...)
	},
}

var GitCheckout = &ToolDef{
	Name:        "git_checkout",
	Description: "Check out a git ref (branch, tag, or commit). Owner-only.",
	Secure:      true,
	Timeout:     30 * time.Second,
	MaxOutput:   8 * 1024,
	Args: []ToolArg{
		{Name: "cwd", Type: ArgString, Description: "Repo directory (default: current working directory)", Required: false},
		{Name: "ref", Type: ArgString, Description: "Branch, tag, or commit to check out", Required: true},
		{Name: "force", Type: ArgBool, Description: "Use -f to discard local changes (default: false)", Required: false},
	},
	Execute: func(args map[string]any) string {
		ref := String(args, "ref")
		if ref == "" {
			return "Error: ref is required"
		}
		dir, errMsg := resolveCwd(args)
		if errMsg != "" {
			return errMsg
		}
		argv := []string{"checkout"}
		if BoolOr(args, "force", false) {
			argv = append(argv, "-f")
		}
		argv = append(argv, ref)
		return runGit(dir, 30*time.Second, argv...)
	},
}

var GitPull = &ToolDef{
	Name:        "git_pull",
	Description: "Pull changes from a remote. Owner-only.",
	Secure:      true,
	Timeout:     90 * time.Second,
	MaxOutput:   16 * 1024,
	Args: []ToolArg{
		{Name: "cwd", Type: ArgString, Description: "Repo directory (default: current working directory)", Required: false},
		{Name: "remote", Type: ArgString, Description: "Remote name (default: origin)", Required: false},
		{Name: "branch", Type: ArgString, Description: "Branch to pull (default: current branch)", Required: false},
		{Name: "rebase", Type: ArgBool, Description: "Use --rebase instead of merge (default: false)", Required: false},
	},
	Execute: func(args map[string]any) string {
		dir, errMsg := resolveCwd(args)
		if errMsg != "" {
			return errMsg
		}
		remote := StringOr(args, "remote", "origin")
		branch := String(args, "branch")
		if branch == "" {
			branch = currentBranch(dir)
		}
		argv := []string{"pull"}
		if BoolOr(args, "rebase", false) {
			argv = append(argv, "--rebase")
		}
		argv = append(argv, remote)
		if branch != "" {
			argv = append(argv, branch)
		}
		return runGit(dir, 90*time.Second, argv...)
	},
}

var GitPush = &ToolDef{
	Name:        "git_push",
	Description: "Push changes to a remote. Owner-only.",
	Secure:      true,
	Timeout:     90 * time.Second,
	MaxOutput:   16 * 1024,
	Args: []ToolArg{
		{Name: "cwd", Type: ArgString, Description: "Repo directory (default: current working directory)", Required: false},
		{Name: "remote", Type: ArgString, Description: "Remote name (default: origin)", Required: false},
		{Name: "branch", Type: ArgString, Description: "Branch to push (default: current branch)", Required: false},
		{Name: "force", Type: ArgBool, Description: "Use --force (default: false). Dangerous on shared branches.", Required: false},
		{Name: "set_upstream", Type: ArgBool, Description: "Use -u to set upstream tracking (default: false)", Required: false},
	},
	Execute: func(args map[string]any) string {
		dir, errMsg := resolveCwd(args)
		if errMsg != "" {
			return errMsg
		}
		remote := StringOr(args, "remote", "origin")
		branch := String(args, "branch")
		if branch == "" {
			branch = currentBranch(dir)
		}
		argv := []string{"push"}
		if BoolOr(args, "force", false) {
			argv = append(argv, "--force")
		}
		if BoolOr(args, "set_upstream", false) {
			argv = append(argv, "-u")
		}
		argv = append(argv, remote)
		if branch != "" {
			argv = append(argv, branch)
		}
		return runGit(dir, 90*time.Second, argv...)
	},
}
