package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

var DownloadYtdlp = &ToolDef{
	Name:        "download_ytdlp",
	Description: "Download video or audio using yt-dlp if it is installed on the system map.",
	Args: []ToolArg{
		{Name: "url", Description: "URL to download", Required: true},
		{Name: "audio_only", Description: "Set to 'true' to extract audio only", Required: false},
		{Name: "options", Description: "Extra command line flags (e.g. '-f best')", Required: false},
	},
	Execute: func(args map[string]string) string {
		url := strings.TrimSpace(args["url"])
		if url == "" {
			return "Error: url is required"
		}

		if _, err := exec.LookPath("yt-dlp"); err != nil {
			return "Error: yt-dlp is not installed or not in PATH."
		}

		var cmdArgs []string
		if args["audio_only"] == "true" {
			cmdArgs = append(cmdArgs, "-x", "--audio-format", "mp3")
		}
		if opts := strings.TrimSpace(args["options"]); opts != "" {
			cmdArgs = append(cmdArgs, strings.Split(opts, " ")...)
		}
		cmdArgs = append(cmdArgs, url)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		cmd := exec.CommandContext(ctx, "yt-dlp", cmdArgs...)
		out, err := cmd.CombinedOutput()

		res := string(out)
		if len(res) > 4000 {
			res = res[len(res)-4000:]
		}

		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return fmt.Sprintf("Timeout (5m).\n...%s", res)
			}
			return fmt.Sprintf("Error: %v\n...%s", err, res)
		}
		return fmt.Sprintf("Success:\n...%s", res)
	},
}

var DownloadAria2c = &ToolDef{
	Name:        "download_aria2c",
	Description: "Download files using aria2c if it is installed on the system map.",
	Args: []ToolArg{
		{Name: "url", Description: "URL to download", Required: true},
		{Name: "options", Description: "Extra command line flags (e.g. '-x 16')", Required: false},
	},
	Execute: func(args map[string]string) string {
		url := strings.TrimSpace(args["url"])
		if url == "" {
			return "Error: url is required"
		}

		if _, err := exec.LookPath("aria2c"); err != nil {
			return "Error: aria2c is not installed or not in PATH."
		}

		var cmdArgs []string
		if opts := strings.TrimSpace(args["options"]); opts != "" {
			cmdArgs = append(cmdArgs, strings.Split(opts, " ")...)
		}
		cmdArgs = append(cmdArgs, url)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		cmd := exec.CommandContext(ctx, "aria2c", cmdArgs...)
		out, err := cmd.CombinedOutput()

		res := string(out)
		if len(res) > 4000 {
			res = res[len(res)-4000:]
		}

		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return fmt.Sprintf("Timeout (5m).\n...%s", res)
			}
			return fmt.Sprintf("Error: %v\n...%s", err, res)
		}
		return fmt.Sprintf("Success:\n...%s", res)
	},
}
