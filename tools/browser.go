package tools

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

var cdpMu sync.Mutex
var cdpAllocCtx context.Context
var cdpAllocCancel context.CancelFunc
var cdpBrowserCtx context.Context
var cdpBrowserCancel context.CancelFunc
var cdpCurrentURL string

func getCDPCtx() (context.Context, error) {
	cdpMu.Lock()
	defer cdpMu.Unlock()
	if cdpBrowserCtx == nil {
		opts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("headless", true),
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("no-sandbox", true),
			chromedp.Flag("disable-dev-shm-usage", true),
			chromedp.WindowSize(1280, 900),
		)
		cdpAllocCtx, cdpAllocCancel = chromedp.NewExecAllocator(context.Background(), opts...)
		cdpBrowserCtx, cdpBrowserCancel = chromedp.NewContext(cdpAllocCtx,
			chromedp.WithLogf(func(f string, a ...any) {
				log.Printf("[CDP] "+f, a...)
			}),
		)

		if err := chromedp.Run(cdpBrowserCtx); err != nil {
			cdpBrowserCtx = nil
			cdpBrowserCancel()
			cdpAllocCancel()
			return nil, fmt.Errorf("failed to start browser: %v", err)
		}
	}
	return cdpBrowserCtx, nil
}

var BrowserOpen = &ToolDef{
	Name:        "browser_open",
	Description: "Open a URL in a real headless Chrome browser and return the visible page text. Use this for JS-heavy pages or anything requiring real browser rendering.",
	Args: []ToolArg{
		{Name: "url", Description: "URL to navigate to", Required: true},
	},
	Execute: func(args map[string]string) string {
		rawURL := args["url"]
		if rawURL == "" {
			return "Error: url is required"
		}
		ctx, err := getCDPCtx()
		if err != nil {
			return fmt.Sprintf("Error starting browser: %v", err)
		}
		tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		tabCtx, tabCancel := chromedp.NewContext(tctx)
		defer tabCancel()

		var title, text string
		if err := chromedp.Run(tabCtx,
			chromedp.Navigate(rawURL),
			chromedp.WaitReady("body", chromedp.ByQuery),
			chromedp.Title(&title),
			chromedp.Text("body", &text, chromedp.ByQuery, chromedp.NodeVisible),
		); err != nil {
			return fmt.Sprintf("Error navigating: %v", err)
		}
		cdpMu.Lock()
		cdpCurrentURL = rawURL
		cdpMu.Unlock()
		text = strings.TrimSpace(text)
		if len(text) > 6000 {
			text = text[:6000] + "\n...(truncated)"
		}
		return fmt.Sprintf("Title: %s\nURL: %s\n\n%s", title, rawURL, text)
	},
}

var BrowserClick = &ToolDef{
	Name:        "browser_click",
	Description: "Click an element in the current browser page by CSS selector.",
	Args: []ToolArg{
		{Name: "selector", Description: "CSS selector of the element to click (e.g. 'button#submit', 'a.login')", Required: true},
	},
	Execute: func(args map[string]string) string {
		sel := args["selector"]
		if sel == "" {
			return "Error: selector is required"
		}
		ctx, err := getCDPCtx()
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		tctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		tabCtx, tabCancel := chromedp.NewContext(tctx)
		defer tabCancel()
		if err := chromedp.Run(tabCtx,
			chromedp.Click(sel, chromedp.ByQuery),
		); err != nil {
			return fmt.Sprintf("Error clicking %q: %v", sel, err)
		}
		return fmt.Sprintf("Clicked: %s", sel)
	},
}

var BrowserType = &ToolDef{
	Name:        "browser_type",
	Description: "Type text into an input element in the current browser page.",
	Args: []ToolArg{
		{Name: "selector", Description: "CSS selector of the input field", Required: true},
		{Name: "text", Description: "Text to type into the field", Required: true},
	},
	Execute: func(args map[string]string) string {
		sel := args["selector"]
		text := args["text"]
		if sel == "" || text == "" {
			return "Error: selector and text are required"
		}
		ctx, err := getCDPCtx()
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		tctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		tabCtx, tabCancel := chromedp.NewContext(tctx)
		defer tabCancel()
		if err := chromedp.Run(tabCtx,
			chromedp.Clear(sel, chromedp.ByQuery),
			chromedp.SendKeys(sel, text, chromedp.ByQuery),
		); err != nil {
			return fmt.Sprintf("Error typing into %q: %v", sel, err)
		}
		return fmt.Sprintf("Typed %q into %s", text, sel)
	},
}

var BrowserGetText = &ToolDef{
	Name:        "browser_get_text",
	Description: "Get the inner text of a specific element on the current browser page using a CSS selector. Use 'body' to get all visible text.",
	Args: []ToolArg{
		{Name: "selector", Description: "CSS selector (e.g. '#result', '.price', 'body')", Required: true},
	},
	Execute: func(args map[string]string) string {
		sel := args["selector"]
		if sel == "" {
			sel = "body"
		}
		ctx, err := getCDPCtx()
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		tctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		tabCtx, tabCancel := chromedp.NewContext(tctx)
		defer tabCancel()
		var text string
		if err := chromedp.Run(tabCtx,
			chromedp.Text(sel, &text, chromedp.ByQuery),
		); err != nil {
			return fmt.Sprintf("Error getting text for %q: %v", sel, err)
		}
		text = strings.TrimSpace(text)
		if len(text) > 6000 {
			text = text[:6000] + "\n...(truncated)"
		}
		return text
	},
}

var BrowserEval = &ToolDef{
	Name:        "browser_eval",
	Description: "Evaluate a JavaScript expression in the current browser page and return the result as a string. Useful for interacting with page state.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "js", Description: "JavaScript expression to evaluate (e.g. 'document.title', 'document.querySelector(\".price\").innerText')", Required: true},
	},
	Execute: func(args map[string]string) string {
		js := args["js"]
		if js == "" {
			return "Error: js is required"
		}
		ctx, err := getCDPCtx()
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		tctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		tabCtx, tabCancel := chromedp.NewContext(tctx)
		defer tabCancel()
		var result any
		if err := chromedp.Run(tabCtx,
			chromedp.Evaluate(js, &result),
		); err != nil {
			return fmt.Sprintf("Error evaluating JS: %v", err)
		}
		return fmt.Sprintf("%v", result)
	},
}

var BrowserScreenshot = &ToolDef{
	Name:        "browser_screenshot",
	Description: "Take a screenshot of the current browser page and save it to a file.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "path", Description: "File path to save the screenshot (e.g. '/tmp/screen.png'). Defaults to a temp file.", Required: false},
		{Name: "full_page", Description: "Capture full page (true) or viewport (false). Default: false", Required: false},
	},
	Execute: func(args map[string]string) string {
		savePath := args["path"]
		if savePath == "" {
			f, err := os.CreateTemp("", "apexclaw-screenshot-*.png")
			if err != nil {
				return fmt.Sprintf("Error creating temp file: %v", err)
			}
			f.Close()
			savePath = f.Name()
		}

		fullPage := strings.ToLower(strings.TrimSpace(args["full_page"])) == "true"

		ctx, err := getCDPCtx()
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}

		tctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()

		// Use the main browser context, not a new tab context
		var buf []byte
		if fullPage {
			// Full page screenshot
			if err := chromedp.Run(tctx,
				chromedp.FullScreenshot(&buf, 90),
			); err != nil {
				return fmt.Sprintf("Error taking full screenshot: %v", err)
			}
		} else {
			// Viewport screenshot
			if err := chromedp.Run(tctx,
				chromedp.CaptureScreenshot(&buf),
			); err != nil {
				return fmt.Sprintf("Error taking screenshot: %v", err)
			}
		}

		if len(buf) == 0 {
			return "Error: screenshot resulted in empty image. Browser window may not be visible or page may not be loaded."
		}

		if err := os.WriteFile(savePath, buf, 0644); err != nil {
			return fmt.Sprintf("Error saving screenshot: %v", err)
		}
		return fmt.Sprintf("Screenshot saved to: %s (%d bytes)", savePath, len(buf))
	},
}
