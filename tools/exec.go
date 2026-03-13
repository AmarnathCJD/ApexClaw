package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	osexec "os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var Exec = &ToolDef{
	Name:        "exec",
	Description: "Run a shell/system command. Returns combined stdout+stderr. Auto-detects long-running commands (npm install, pip install, etc) and increases timeout.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "cmd", Description: "Shell command to execute", Required: true},
		{Name: "timeout", Description: "Timeout in seconds (default: auto-detect, min 30, max 600)", Required: false},
	},
	Execute: func(args map[string]string) string {
		cmd := args["cmd"]
		if cmd == "" {
			return "Error: cmd is required"
		}

		timeoutSec := 30
		if strings.Contains(cmd, "npm install") || strings.Contains(cmd, "npm i ") ||
			strings.Contains(cmd, "pip install") || strings.Contains(cmd, "pip3 install") ||
			strings.Contains(cmd, "go get") || strings.Contains(cmd, "cargo") ||
			strings.Contains(cmd, "apt-get") || strings.Contains(cmd, "brew") ||
			strings.Contains(cmd, "yarn install") || strings.Contains(cmd, "bun install") {
			timeoutSec = 300
		}

		if t := args["timeout"]; t != "" {
			if parsedT, err := strconv.Atoi(t); err == nil {
				timeoutSec = parsedT
			}
		}
		if timeoutSec < 30 {
			timeoutSec = 30
		}
		if timeoutSec > 600 {
			timeoutSec = 600
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
		defer cancel()

		envVars := os.Environ()
		envVars = append(envVars, "CI=true", "NPM_CONFIG_PROGRESS=false", "DEBIAN_FRONTEND=noninteractive")

		var out []byte
		var err error
		if runtime.GOOS == "windows" {
			c := osexec.CommandContext(ctx, "cmd", "/c", cmd)
			c.Env = envVars
			out, err = c.CombinedOutput()
		} else {
			c := osexec.CommandContext(ctx, "sh", "-c", cmd)
			c.Env = envVars
			out, err = c.CombinedOutput()
		}

		result := strings.TrimSpace(string(out))
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Sprintf("Error: Timeout after %ds.\n%s", timeoutSec, result)
		}
		if err != nil {
			return fmt.Sprintf("Error: Exit error: %v\n%s", err, result)
		}
		if len(result) > 8000 {
			result = result[:8000] + "\n...(truncated)"
		}
		if result == "" {
			return "(completed)"
		}
		return result
	},
}

func runShellCmd(cmd string, timeoutSec int) (string, error, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	envVars := os.Environ()
	envVars = append(envVars, "CI=true", "NPM_CONFIG_PROGRESS=false", "DEBIAN_FRONTEND=noninteractive")

	var c *osexec.Cmd
	if runtime.GOOS == "windows" {
		c = osexec.CommandContext(ctx, "cmd", "/c", cmd)
	} else {
		c = osexec.CommandContext(ctx, "sh", "-c", cmd)
	}
	c.Env = envVars

	out, err := c.CombinedOutput()
	result := strings.TrimSpace(string(out))
	if ctx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("timeout after %ds", timeoutSec), true
	}
	return result, err, false
}

var ExecChain = &ToolDef{
	Name:        "exec_chain",
	Description: "Execute multiple shell commands in sequence. Returns all outputs. Stops on first error by default. Saves iterations for multi-step CLI tasks.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "commands", Description: "JSON array of commands: [\"cmd1\", \"cmd2\", \"cmd3\"]", Required: true},
		{Name: "timeout", Description: "Timeout per command in seconds (default: 60, max: 300)", Required: false},
		{Name: "stop_on_error", Description: "Stop on first error (default: true)", Required: false},
	},
	Execute: func(args map[string]string) string {
		cmdsJSON := args["commands"]
		if cmdsJSON == "" {
			return "Error: commands is required"
		}

		var commands []string
		if err := json.Unmarshal([]byte(cmdsJSON), &commands); err != nil {
			return fmt.Sprintf("Error parsing commands JSON: %v", err)
		}
		if len(commands) == 0 {
			return "Error: commands array is empty"
		}
		if len(commands) > 20 {
			return "Error: max 20 commands per chain"
		}

		timeoutSec := 60
		if t := args["timeout"]; t != "" {
			if parsedT, err := strconv.Atoi(t); err == nil {
				timeoutSec = parsedT
			}
		}
		if timeoutSec < 30 {
			timeoutSec = 30
		}
		if timeoutSec > 600 {
			timeoutSec = 600
		}

		stopOnError := args["stop_on_error"] != "false"

		var results []string
		total := len(commands)

		for i, cmd := range commands {
			cmdTimeout := timeoutSec
			if strings.Contains(cmd, "npm install") || strings.Contains(cmd, "npm i ") ||
				strings.Contains(cmd, "pip install") || strings.Contains(cmd, "pip3 install") ||
				strings.Contains(cmd, "go get") || strings.Contains(cmd, "cargo") ||
				strings.Contains(cmd, "apt-get") || strings.Contains(cmd, "brew") ||
				strings.Contains(cmd, "yarn install") || strings.Contains(cmd, "bun install") {
				cmdTimeout = 300
			}

			start := time.Now()
			result, cmdErr, timedOut := runShellCmd(cmd, cmdTimeout)
			elapsed := time.Since(start)

			if timedOut {
				results = append(results, fmt.Sprintf("[%d/%d] TIMEOUT after %ds\n%s\n%s", i+1, total, cmdTimeout, cmd, result))
				if stopOnError {
					break
				}
				continue
			}

			if cmdErr != nil {
				results = append(results, fmt.Sprintf("[%d/%d] FAILED (%.1fs)\n%s\n%s", i+1, total, elapsed.Seconds(), cmd, result))
				if stopOnError {
					break
				}
				continue
			}

			output := result
			if len(output) > 2000 {
				output = output[:2000] + "...(truncated)"
			}
			if output == "" {
				output = "(ok)"
			}
			results = append(results, fmt.Sprintf("[%d/%d] OK (%.1fs)\n%s", i+1, total, elapsed.Seconds(), cmd))
			if output != "(ok)" {
				results[len(results)-1] += fmt.Sprintf("\n%s", output)
			}
		}

		return strings.Join(results, "\n\n")
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
		if len(result) > 8000 {
			result = result[:8000] + "\n...(truncated)"
		}
		if result == "" {
			return "(no output)"
		}
		return result
	},
}
