package tools

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

var SystemInfo = &ToolDef{
	Name:        "system_info",
	Description: "Get system information: OS, CPU cores, Go runtime memory stats, and (Windows) RAM usage",
	Args:        []ToolArg{},
	Execute: func(args map[string]any) string {
		var sb strings.Builder
		sb.WriteString("System Information\n")
		sb.WriteString(strings.Repeat("─", 36) + "\n")
		sb.WriteString(fmt.Sprintf("OS:           %s/%s\n", runtime.GOOS, runtime.GOARCH))
		sb.WriteString(fmt.Sprintf("CPU Cores:    %d\n", runtime.NumCPU()))
		sb.WriteString(fmt.Sprintf("Go Version:   %s\n", runtime.Version()))
		sb.WriteString(fmt.Sprintf("Goroutines:   %d\n", runtime.NumGoroutine()))

		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		sb.WriteString(fmt.Sprintf("Heap Alloc:   %s\n", sysFormatBytes(mem.HeapAlloc)))
		sb.WriteString(fmt.Sprintf("Heap Sys:     %s\n", sysFormatBytes(mem.HeapSys)))
		sb.WriteString(fmt.Sprintf("Total Alloc:  %s\n", sysFormatBytes(mem.TotalAlloc)))
		sb.WriteString(fmt.Sprintf("GC Runs:      %d\n", mem.NumGC))

		if runtime.GOOS == "windows" {
			out, err := exec.Command("wmic", "OS", "get", "FreePhysicalMemory,TotalVisibleMemorySize", "/Value").Output()
			if err == nil {
				for _, line := range strings.Split(string(out), "\n") {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "FreePhysicalMemory=") {
						var kb uint64
						fmt.Sscanf(strings.TrimPrefix(line, "FreePhysicalMemory="), "%d", &kb)
						sb.WriteString(fmt.Sprintf("Free RAM:     %s\n", sysFormatBytes(kb*1024)))
					} else if strings.HasPrefix(line, "TotalVisibleMemorySize=") {
						var kb uint64
						fmt.Sscanf(strings.TrimPrefix(line, "TotalVisibleMemorySize="), "%d", &kb)
						sb.WriteString(fmt.Sprintf("Total RAM:    %s\n", sysFormatBytes(kb*1024)))
					}
				}
			}
		}

		return strings.TrimRight(sb.String(), "\n")
	},
}

func sysFormatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

var ProcessList = &ToolDef{
	Name:        "process_list",
	Description: "List running processes with their PID and memory usage (sudo only)",
	Secure:      true,
	Args: []ToolArg{
		{Name: "filter", Type: ArgString, Description: "Optional name filter to search for specific processes (case-insensitive)", Required: false},
	},
	Execute: func(args map[string]any) string {
		filter := strings.ToLower(String(args, "filter"))

		var out []byte
		var err error
		if runtime.GOOS == "windows" {
			out, err = exec.Command("tasklist", "/FO", "CSV", "/NH").Output()
		} else {
			out, err = exec.Command("ps", "aux").Output()
		}
		if err != nil {
			return fmt.Sprintf("Error listing processes: %v", err)
		}

		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		var results []string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if filter != "" && !strings.Contains(strings.ToLower(line), filter) {
				continue
			}
			results = append(results, line)
			if len(results) >= 50 {
				results = append(results, fmt.Sprintf("...(showing first 50 of %d total)", len(lines)))
				break
			}
		}
		if len(results) == 0 {
			if filter != "" {
				return fmt.Sprintf("No processes found matching: %s", filter)
			}
			return "No processes found"
		}
		return strings.Join(results, "\n")
	},
}

var KillProcess = &ToolDef{
	Name:        "kill_process",
	Description: "Kill a running process by PID or name (sudo only)",
	Secure:      true,
	Args: []ToolArg{
		{Name: "pid", Type: ArgInt, Description: "Process ID to kill", Required: false},
		{Name: "name", Type: ArgString, Description: "Process name to kill (e.g. 'notepad.exe')", Required: false},
	},
	Execute: func(args map[string]any) string {
		pid, hasPid := Int(args, "pid")
		name := String(args, "name")
		if !hasPid && name == "" {
			return "Error: pid or name is required"
		}

		pidStr := ""
		if hasPid {
			pidStr = fmt.Sprintf("%d", pid)
		}

		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			if hasPid {
				cmd = exec.Command("taskkill", "/F", "/PID", pidStr)
			} else {
				cmd = exec.Command("taskkill", "/F", "/IM", name)
			}
		} else {
			if hasPid {
				cmd = exec.Command("kill", "-9", pidStr)
			} else {
				cmd = exec.Command("pkill", "-9", name)
			}
		}

		out, err := cmd.CombinedOutput()
		result := strings.TrimSpace(string(out))
		if err != nil {
			if result != "" {
				return fmt.Sprintf("Error: %v\n%s", err, result)
			}
			return fmt.Sprintf("Error: %v", err)
		}
		if result != "" {
			return result
		}
		if hasPid {
			return fmt.Sprintf("Process %s killed successfully", pidStr)
		}
		return fmt.Sprintf("Process '%s' killed successfully", name)
	},
}

var UpdateClaw = &ToolDef{
	Name:        "update_claw",
	Description: "Update ApexClaw. Uses git pull/build if in a git repo, otherwise tells you how to update. (sudo only)",
	Secure:      true,
	Args:        []ToolArg{},
	Execute: func(args map[string]any) string {
		var sb strings.Builder
		sb.WriteString("Update initiated...\n")

		if _, err := os.Stat(".git"); err == nil {
			sb.WriteString("Detected Git repository. Running git pull...\n")
			cmdPull := exec.Command("git", "pull")
			outPull, err := cmdPull.CombinedOutput()
			sb.WriteString("Result: " + strings.TrimSpace(string(outPull)) + "\n")
			if err != nil {
				return sb.String() + "\nUpdate failed during git pull."
			}

			sb.WriteString("Rebuilding apexclaw...\n")
			binName := "apexclaw"
			if runtime.GOOS == "windows" {
				binName = "apexclaw.exe"
			}
			cmdBuild := exec.Command("go", "build", "-o", binName, ".")
			outBuild, err := cmdBuild.CombinedOutput()
			if len(outBuild) > 0 {
				sb.WriteString("Build Output: " + strings.TrimSpace(string(outBuild)) + "\n")
			}
			if err != nil {
				return sb.String() + "\nBuild failed."
			}

			sb.WriteString("\nUpdate successful! Use restart_claw to reload.")
		} else {
			sb.WriteString("Not a git repository. Attempting binary update via curl one-liner...\n")
			cmdUpdate := exec.Command("sh", "-c", "curl -fsSL https://claw.gogram.fun | bash")
			if runtime.GOOS == "windows" {
				cmdUpdate = exec.Command("cmd", "/C", "curl -fsSL https://claw.gogram.fun | bash")
			}
			out, err := cmdUpdate.CombinedOutput()
			sb.WriteString("Result:\n" + strings.TrimSpace(string(out)) + "\n")
			if err != nil {
				return sb.String() + "\nUpdate failed."
			}
			sb.WriteString("\nBinary updated! Use restart_claw to reload.")
		}

		return sb.String()
	},
}

var RestartClaw = &ToolDef{
	Name:        "restart_claw",
	Description: "Restarts the ApexClaw process (sudo only)",
	Secure:      true,
	Args:        []ToolArg{},
	Execute: func(args map[string]any) string {
		binName := "./apexclaw"
		if runtime.GOOS == "windows" {
			binName = "apexclaw.exe"
		}

		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.Command("cmd", "/C", "start", binName)
		} else {
			cmd = exec.Command("sh", "-c", binName+" &")
		}

		err := cmd.Start()
		if err != nil {
			return fmt.Sprintf("Error starting new process: %v", err)
		}

		go func() {
			time.Sleep(1 * time.Second)
			os.Exit(0)
		}()

		return "Restarting ApexClaw... bot will be back in a moment."
	},
}

var KillClaw = &ToolDef{
	Name:        "kill_claw",
	Description: "Immediately shuts down the ApexClaw process (sudo only)",
	Secure:      true,
	Args:        []ToolArg{},
	Execute: func(args map[string]any) string {
		go func() {
			time.Sleep(500 * time.Millisecond)
			os.Exit(0)
		}()
		return "Shutting down ApexClaw. Use your terminal or host manager to restart it."
	},
}
