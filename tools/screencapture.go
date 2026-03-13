package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var ScreenCapture = &ToolDef{
	Name:        "screen_capture",
	Description: "Take a screenshot of the desktop and optionally analyze it with AI vision. Returns image path and optional AI description of what's on screen.",
	Args: []ToolArg{
		{Name: "analyze", Description: "true/false — run AI vision analysis on the screenshot (default: false)", Required: false},
		{Name: "prompt", Description: "Custom prompt for AI analysis, e.g. 'What errors are visible?' or 'Describe the UI layout'", Required: false},
		{Name: "monitor", Description: "Which monitor to capture: 'all' (default), '1', '2', etc.", Required: false},
	},
	Execute: func(args map[string]string) string {
		analyze := strings.ToLower(args["analyze"]) == "true"
		prompt := args["prompt"]
		if prompt == "" {
			prompt = "Describe what is visible on the screen in detail. Note any interesting content, open apps, text, errors, or notable elements."
		}
		monitor := args["monitor"]
		if monitor == "" {
			monitor = "all"
		}
		return captureScreen(analyze, prompt, monitor)
	},
}

func captureScreen(analyze bool, prompt, monitor string) string {
	home, _ := os.UserHomeDir()
	outDir := filepath.Join(home, ".apexclaw", "screenshots")
	os.MkdirAll(outDir, 0755)

	ts := time.Now().Format("20060102_150405")
	outFile := filepath.Join(outDir, fmt.Sprintf("screen_%s.png", ts))

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		ps := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.Screen]::PrimaryScreen | Out-Null; Add-Type -AssemblyName System.Drawing; $bmp = New-Object System.Drawing.Bitmap([System.Windows.Forms.SystemInformation]::VirtualScreen.Width, [System.Windows.Forms.SystemInformation]::VirtualScreen.Height); $g = [System.Drawing.Graphics]::FromImage($bmp); $g.CopyFromScreen([System.Windows.Forms.SystemInformation]::VirtualScreen.Location, [System.Drawing.Point]::Empty, $bmp.Size); $bmp.Save('%s'); $g.Dispose(); $bmp.Dispose()`, outFile)
		cmd = exec.Command("powershell", "-NonInteractive", "-Command", ps)
	case "darwin":
		if monitor != "all" {
			cmd = exec.Command("screencapture", "-x", "-D", monitor, outFile)
		} else {
			cmd = exec.Command("screencapture", "-x", outFile)
		}
	default:
		cmd = exec.Command("bash", "-c", fmt.Sprintf("import -window root %s 2>/dev/null || scrot %s 2>/dev/null || gnome-screenshot -f %s 2>/dev/null", outFile, outFile, outFile))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd = exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("Error capturing screen: %v\n%s", err, string(out))
	}

	_, err = os.Stat(outFile)
	if err != nil {
		return fmt.Sprintf("Screenshot file not created: %v", err)
	}

	result := fmt.Sprintf("Screenshot saved: %s", outFile)

	if !analyze {
		return result
	}

	imgData, err := os.ReadFile(outFile)
	if err != nil {
		return result + "\nError reading screenshot for analysis: " + err.Error()
	}

	b64 := base64.StdEncoding.EncodeToString(imgData)
	analysis := analyzeScreenshotWithVision(b64, prompt)
	return fmt.Sprintf("%s\n\n## AI Vision Analysis\n%s", result, analysis)
}

func analyzeScreenshotWithVision(imageB64, prompt string) string {
	if ScreenAnalyzeFn != nil {
		return ScreenAnalyzeFn(imageB64, prompt)
	}
	return "(Vision analysis not available — ScreenAnalyzeFn not registered)"
}

var ScreenAnalyzeFn func(imageB64, prompt string) string
