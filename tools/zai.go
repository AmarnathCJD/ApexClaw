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

var ZAIImageGenerate = &ToolDef{
	Name:        "zai_image_generate",
	Description: "Generate an image with chatglm.cn's image model from a text prompt. Auto-sends the result(s) to the current Telegram chat. Use for image creation requests like 'draw me X' or 'generate a picture of Y'.",
	Secure:      true,
	Timeout:     -1, // tool manages its own 5-minute timeout internally
	Args: []ToolArg{
		{Name: "prompt", Type: ArgString, Description: "Image description (English or Chinese). Be specific about style, subject, lighting.", Required: true},
		{Name: "aspect_ratio", Type: ArgString, Description: "Aspect ratio: 1:1, 16:9, 9:16, 4:3, 3:4. Default 1:1.", Required: false},
		{Name: "style", Type: ArgString, Description: "Style hint (e.g. 'photorealistic', 'anime', 'oil painting'). Optional.", Required: false},
		{Name: "target", Type: ArgString, Description: "Chat to send image to. Omit for current chat.", Required: false},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		prompt := String(args, "prompt")
		if prompt == "" {
			return "Error: prompt is required"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		res, err := model.ZAIImageGenerate(ctx, prompt, model.ZAIImageOpts{
			AspectRatio: String(args, "aspect_ratio"),
			Style:       String(args, "style"),
		})
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		if len(res.ImageURLs) == 0 {
			return "Error: no images returned"
		}

		target := resolveContextPeer(String(args, "target"), userID)
		if target != "" && SendTGPhotoURLFn != nil {
			for i, url := range res.ImageURLs {
				caption := ""
				if i == 0 {
					caption = prompt
				}
				SendTGPhotoURLFn(target, url, caption)
			}
			return fmt.Sprintf("Sent %d image(s) for prompt: %s", len(res.ImageURLs), prompt)
		}
		return strings.Join(res.ImageURLs, "\n")
	},
}

var ZAIImageEdit = &ToolDef{
	Name:        "zai_image_edit",
	Description: "Edit an existing image with chatglm.cn's image edit model. Provide a local path or URL plus an edit instruction. Auto-sends the edited image to the current Telegram chat.",
	Secure:      true,
	Timeout:     -1,
	Args: []ToolArg{
		{Name: "image", Type: ArgString, Description: "Local file path or HTTP(S) URL of the source image", Required: true},
		{Name: "prompt", Type: ArgString, Description: "Edit instruction (e.g. 'remove the background', 'make the dress red')", Required: true},
		{Name: "aspect_ratio", Type: ArgString, Description: "Output aspect ratio (default 1:1)", Required: false},
		{Name: "target", Type: ArgString, Description: "Chat to send to. Omit for current chat.", Required: false},
	},
	ExecuteWithContext: func(args map[string]any, userID string) string {
		image := String(args, "image")
		prompt := String(args, "prompt")
		if image == "" || prompt == "" {
			return "Error: image and prompt are required"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		localPath := image
		var cleanup string
		if strings.HasPrefix(image, "http://") || strings.HasPrefix(image, "https://") {
			tmp, err := downloadToTemp(ctx, image)
			if err != nil {
				return fmt.Sprintf("Error downloading source: %v", err)
			}
			localPath = tmp
			cleanup = tmp
		}
		if cleanup != "" {
			defer os.Remove(cleanup)
		}

		f, err := model.ZAIUpload(ctx, localPath)
		if err != nil {
			return fmt.Sprintf("Error uploading: %v", err)
		}
		aspect := String(args, "aspect_ratio")
		url, err := model.ZAIImageEdit(ctx, f, prompt, aspect, "")
		if err != nil {
			return fmt.Sprintf("Error editing: %v", err)
		}
		target := resolveContextPeer(String(args, "target"), userID)
		if target != "" && SendTGPhotoURLFn != nil {
			SendTGPhotoURLFn(target, url, prompt)
			return fmt.Sprintf("Sent edited image: %s", prompt)
		}
		return url
	},
}

var ZAIResearch = &ToolDef{
	Name:        "zai_research",
	Description: "Run a deep multi-source research query through chatglm.cn's research mode. Returns a thorough multi-section report (can be 5-20k chars). Slower than normal chat — use for genuinely complex research questions.",
	Timeout:     -1,
	MaxOutput:   32 * 1024, // research reports are intentionally long
	Args: []ToolArg{
		{Name: "query", Type: ArgString, Description: "Research question or topic. Be specific about scope and what aspects to cover.", Required: true},
	},
	Execute: func(args map[string]any) string {
		query := String(args, "query")
		if query == "" {
			return "Error: query is required"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		res, err := model.ZAIChatText(ctx, query, model.ZAIOpts{
			ChatMode:   "engine_research",
			Networking: true,
		})
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return res.Text
	},
}

var ZAIAgent = &ToolDef{
	Name:        "zai_agent",
	Description: "Run a complex multi-step task through chatglm.cn's autonomous agent mode. The remote agent reasons, plans, and produces a structured deliverable (plans, docs, analyses). Slower than normal chat.",
	Timeout:     -1,
	MaxOutput:   16 * 1024,
	Args: []ToolArg{
		{Name: "task", Type: ArgString, Description: "Task description. Be specific about the deliverable wanted (a plan, a report, an analysis, etc).", Required: true},
	},
	Execute: func(args map[string]any) string {
		task := String(args, "task")
		if task == "" {
			return "Error: task is required"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		res, err := model.ZAIChatText(ctx, task, model.ZAIOpts{
			ChatMode:   "engine_agent",
			Networking: true,
		})
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return res.Text
	},
}

func downloadToTemp(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %d", resp.StatusCode)
	}
	ext := ".jpg"
	if ct := resp.Header.Get("Content-Type"); strings.Contains(ct, "png") {
		ext = ".png"
	} else if strings.Contains(ct, "webp") {
		ext = ".webp"
	}
	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("zai_dl_%d%s", time.Now().UnixNano(), ext))
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return tmp, nil
}
