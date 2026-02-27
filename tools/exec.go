package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	osexec "os/exec"
	"runtime"
	"strings"
	"time"
)

var Exec = &ToolDef{
	Name:        "exec",
	Description: "Run a shell/system command. Returns combined stdout+stderr.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "cmd", Description: "Shell command to execute", Required: true},
		{Name: "timeout", Description: "Timeout in seconds (default 30, max 300)", Required: false},
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
		if timeoutSec > 300 {
			timeoutSec = 300
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
		defer cancel()

		var out []byte
		var err error
		if runtime.GOOS == "windows" {
			out, err = osexec.CommandContext(ctx, "cmd", "/c", cmd).CombinedOutput()
		} else {
			out, err = osexec.CommandContext(ctx, "sh", "-c", cmd).CombinedOutput()
		}

		result := strings.TrimSpace(string(out))
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Sprintf("Timeout after %ds.\n%s", timeoutSec, result)
		}
		if err != nil {
			return fmt.Sprintf("Exit error: %v\n%s", err, result)
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

func runShellCmd(cmd string, timeoutSec int) (string, error, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	var out []byte
	var err error
	if runtime.GOOS == "windows" {
		out, err = osexec.CommandContext(ctx, "cmd", "/c", cmd).CombinedOutput()
	} else {
		out, err = osexec.CommandContext(ctx, "sh", "-c", cmd).CombinedOutput()
	}

	result := strings.TrimSpace(string(out))
	if ctx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("timeout after %ds", timeoutSec), true
	}
	return result, err, false
}

var EnsureCommand = &ToolDef{
	Name:        "ensure_command",
	Description: "Check if a CLI tool is installed. If not, attempt to install it. Returns the path and version if available.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "command", Description: "Command name to check (e.g. 'vercel', 'ffmpeg', 'netlify')", Required: true},
		{Name: "install_via", Description: "How to install if missing: 'npm', 'pip', 'apt', 'brew', 'auto' (default: auto)", Required: false},
		{Name: "package", Description: "Package name if different from command (e.g. command='vercel' package='vercel')", Required: false},
	},
	Execute: func(args map[string]string) string {
		command := args["command"]
		if command == "" {
			return "Error: command is required"
		}

		pkg := args["package"]
		if pkg == "" {
			pkg = command
		}
		installVia := strings.ToLower(args["install_via"])
		if installVia == "" {
			installVia = "auto"
		}

		path, err := osexec.LookPath(command)
		if err == nil {
			version, _, _ := runShellCmd(command+" --version", 10)
			if version == "" {
				version = "(version unknown)"
			}
			return fmt.Sprintf("Found: %s\nPath: %s\nVersion: %s", command, path, version)
		}

		if installVia == "auto" {
			type installer struct {
				name string
				cmd  string
			}
			installers := []installer{
				{"npm", "npm install -g " + pkg},
				{"pip", "pip install " + pkg},
			}
			switch runtime.GOOS {
			case "linux":
				installers = append(installers, installer{"apt", "sudo apt-get install -y " + pkg})
			case "darwin":
				installers = append(installers, installer{"brew", "brew install " + pkg})
			}

			var tried []string
			for _, inst := range installers {
				if _, err := osexec.LookPath(strings.Split(inst.cmd, " ")[0]); err != nil {
					continue
				}
				tried = append(tried, inst.name)
				_, cmdErr, _ := runShellCmd(inst.cmd, 120)
				if cmdErr == nil {
					newPath, err := osexec.LookPath(command)
					if err == nil {
						version, _, _ := runShellCmd(command+" --version", 10)
						return fmt.Sprintf("Installed via %s!\nPath: %s\nVersion: %s", inst.name, newPath, version)
					}
				}
			}
			return fmt.Sprintf("Error: could not install %q. Tried: %s. Install manually.", command, strings.Join(tried, ", "))
		}

		var installCmd string
		switch installVia {
		case "npm":
			installCmd = "npm install -g " + pkg
		case "pip":
			installCmd = "pip install " + pkg
		case "apt":
			installCmd = "sudo apt-get install -y " + pkg
		case "brew":
			installCmd = "brew install " + pkg
		default:
			return fmt.Sprintf("Error: unknown install method %q. Use: npm, pip, apt, brew, auto", installVia)
		}

		result, cmdErr, timedOut := runShellCmd(installCmd, 120)
		if timedOut {
			return fmt.Sprintf("Error: install timed out after 120s.\n%s", result)
		}
		if cmdErr != nil {
			return fmt.Sprintf("Error installing via %s: %v\n%s", installVia, cmdErr, result)
		}

		newPath, err := osexec.LookPath(command)
		if err != nil {
			return fmt.Sprintf("Install command succeeded but %q still not found in PATH.\n%s", command, result)
		}
		version, _, _ := runShellCmd(command+" --version", 10)
		return fmt.Sprintf("Installed via %s!\nPath: %s\nVersion: %s", installVia, newPath, version)
	},
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
			fmt.Sscanf(t, "%d", &timeoutSec)
		}
		if timeoutSec > 300 {
			timeoutSec = 300
		}

		stopOnError := args["stop_on_error"] != "false"

		var results []string
		total := len(commands)

		for i, cmd := range commands {
			start := time.Now()
			result, cmdErr, timedOut := runShellCmd(cmd, timeoutSec)
			elapsed := time.Since(start)

			if timedOut {
				results = append(results, fmt.Sprintf("[%d/%d] %s → TIMEOUT (%.1fs)\n%s", i+1, total, cmd, elapsed.Seconds(), result))
				if stopOnError {
					break
				}
				continue
			}

			if cmdErr != nil {
				results = append(results, fmt.Sprintf("[%d/%d] %s → FAIL (%.1fs)\n%s", i+1, total, cmd, elapsed.Seconds(), result))
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
			results = append(results, fmt.Sprintf("[%d/%d] %s → OK (%.1fs)\n%s", i+1, total, cmd, elapsed.Seconds(), output))
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
