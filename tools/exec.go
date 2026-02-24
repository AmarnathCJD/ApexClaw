package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"strings"
	"time"
)

var Exec = &ToolDef{
	Name:        "exec",
	Description: "Run a shell/system command. Returns combined stdout+stderr. Has a 30s timeout.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "cmd", Description: "Shell command to execute", Required: true},
		{Name: "timeout", Description: "Timeout in seconds (default 30, max 120)", Required: false},
	},
	Execute: func(args map[string]string) string {
		cmd := args["cmd"]
		if cmd == "" {
			return "Error: cmd is required"
		}
		timeoutSec := 30
		if t := args["timeout"]; t != "" {
			fmt.Sscanf(t, "%d", &timeoutSec)
		}
		if timeoutSec > 120 {
			timeoutSec = 120
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
		defer cancel()
		out, err := osexec.CommandContext(ctx, "bash", "-c", cmd).CombinedOutput()
		result := strings.TrimSpace(string(out))
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Sprintf("Timeout after %ds.\n%s", timeoutSec, result)
		}
		if err != nil {
			return fmt.Sprintf("Exit error: %v\n%s", err, result)
		}
		if len(result) > 4000 {
			result = result[:4000] + "\n...(truncated)"
		}
		if result == "" {
			return "(no output)"
		}
		return result
	},
}

var RunPython = &ToolDef{
	Name:        "run_python",
	Description: "Execute a Python code snippet. Writes to a temp file and runs with python3. Returns stdout+stderr. Timeout is 60s.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "code", Description: "Python code to execute", Required: true},
	},
	Execute: func(args map[string]string) string {
		code := args["code"]
		if code == "" {
			return "Error: code is required"
		}
		f, err := os.CreateTemp("", "apexclaw-*.py")
		if err != nil {
			return fmt.Sprintf("Error creating temp file: %v", err)
		}
		defer os.Remove(f.Name())
		if _, err := f.WriteString(code); err != nil {
			return fmt.Sprintf("Error writing script: %v", err)
		}
		f.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		var out bytes.Buffer
		c := osexec.CommandContext(ctx, "python3", f.Name())
		c.Stdout = &out
		c.Stderr = &out
		err = c.Run()

		result := strings.TrimSpace(out.String())
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Sprintf("Python timed out (60s).\n%s", result)
		}
		if err != nil {
			return fmt.Sprintf("Python error: %v\n%s", err, result)
		}
		if len(result) > 4000 {
			result = result[:4000] + "\n...(truncated)"
		}
		if result == "" {
			return "(no output)"
		}
		return result
	},
}
