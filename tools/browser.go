package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

var (
	rodMu      sync.Mutex
	rodBrowser *rod.Browser
	rodPage    *rod.Page
	rodPages   = make(map[string]*rod.Page)
	rodDataDir string
)

func getDataDir() string {
	return filepath.Join(os.TempDir(), "apexclaw-browser")
}

func getBrowser() (*rod.Browser, error) {
	rodMu.Lock()
	defer rodMu.Unlock()
	if rodBrowser != nil {
		return rodBrowser, nil
	}

	path, hasChrome := launcher.LookPath()
	if !hasChrome {
		return nil, fmt.Errorf("no Chrome/Chromium found. Install chromium or google-chrome")
	}

	u := launcher.New().
		Bin(path).
		UserDataDir(getDataDir()).
		Headless(true).
		Set("no-sandbox").
		Set("disable-gpu").
		Set("disable-dev-shm-usage").
		MustLaunch()

	rodBrowser = rod.New().ControlURL(u)
	if err := rodBrowser.Connect(); err != nil {
		rodBrowser = nil
		return nil, fmt.Errorf("browser connect failed: %v", err)
	}

	return rodBrowser, nil
}

func getPage() (*rod.Page, error) {
	browser, err := getBrowser()
	if err != nil {
		return nil, err
	}
	if rodPage != nil {
		return rodPage, nil
	}
	rodPage = stealth.MustPage(browser)
	rodPage.MustSetViewport(1280, 900, 1, false)
	return rodPage, nil
}

var BrowserOpen = &ToolDef{
	Name:        "browser_open",
	Description: "Navigate to a URL in a real headless Chrome browser (with stealth/anti-bot-detection). Returns page title and visible text. Persists cookies across sessions.",
	Args: []ToolArg{
		{Name: "url", Description: "URL to navigate to", Required: true},
		{Name: "wait_for", Description: "Optional CSS selector to wait for before returning (e.g. '#content', '.loaded')", Required: false},
	},
	Execute: func(args map[string]string) string {
		rawURL := args["url"]
		if rawURL == "" {
			return "Error: url is required"
		}

		page, err := getPage()
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}

		if err := page.Timeout(45 * time.Second).Navigate(rawURL); err != nil {
			return fmt.Sprintf("Error navigating to %s: %v", rawURL, err)
		}

		if err := page.Timeout(30 * time.Second).WaitStable(300 * time.Millisecond); err != nil {

		}

		if waitFor := args["wait_for"]; waitFor != "" {
			if err := page.Timeout(15*time.Second).MustWaitElementsMoreThan(waitFor, 0); err != nil {

			}
		}

		title := page.MustEval(`() => document.title`).String()
		text := page.MustEval(`() => document.body.innerText`).String()

		text = strings.TrimSpace(text)
		if len(text) > 8000 {
			text = text[:8000] + "\n...(truncated)"
		}
		return fmt.Sprintf("Title: %s\nURL: %s\n\n%s", title, rawURL, text)
	},
}

var BrowserClick = &ToolDef{
	Name:        "browser_click",
	Description: "Click an element on the current page. Supports CSS selectors or text-based matching.",
	Args: []ToolArg{
		{Name: "selector", Description: "CSS selector (e.g. 'button#submit', 'a.login')", Required: false},
		{Name: "text", Description: "Find and click element containing this text (alternative to selector)", Required: false},
	},
	Execute: func(args map[string]string) string {
		page, err := getPage()
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}

		sel := args["selector"]
		text := args["text"]
		if sel == "" && text == "" {
			return "Error: provide either selector or text"
		}

		if text != "" {

			xpath := fmt.Sprintf(`//*[contains(text(), '%s')]`, strings.ReplaceAll(text, "'", "\\'"))
			el, err := page.Timeout(10 * time.Second).ElementX(xpath)
			if err != nil {
				return fmt.Sprintf("Error: no element with text %q found: %v", text, err)
			}
			if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
				return fmt.Sprintf("Error clicking element with text %q: %v", text, err)
			}
			page.WaitStable(300 * time.Millisecond)
			return fmt.Sprintf("Clicked element containing: %q", text)
		}

		el, err := page.Timeout(10 * time.Second).Element(sel)
		if err != nil {
			return fmt.Sprintf("Error: selector %q not found: %v", sel, err)
		}
		if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
			return fmt.Sprintf("Error clicking %q: %v", sel, err)
		}
		page.WaitStable(300 * time.Millisecond)
		return fmt.Sprintf("Clicked: %s", sel)
	},
}

var BrowserType = &ToolDef{
	Name:        "browser_type",
	Description: "Type text into an input field on the current page.",
	Args: []ToolArg{
		{Name: "selector", Description: "CSS selector of the input field", Required: true},
		{Name: "text", Description: "Text to type", Required: true},
		{Name: "clear", Description: "Clear field before typing (default: true)", Required: false},
		{Name: "submit", Description: "Press Enter after typing (default: false)", Required: false},
	},
	Execute: func(args map[string]string) string {
		sel := args["selector"]
		text := args["text"]
		if sel == "" || text == "" {
			return "Error: selector and text are required"
		}

		page, err := getPage()
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}

		el, err := page.Timeout(10 * time.Second).Element(sel)
		if err != nil {
			return fmt.Sprintf("Error: selector %q not found: %v", sel, err)
		}

		if args["clear"] != "false" {
			el.MustSelectAllText().MustInput("")
		}

		if err := el.Input(text); err != nil {
			return fmt.Sprintf("Error typing into %q: %v", sel, err)
		}

		if strings.EqualFold(args["submit"], "true") {
			el.Click(proto.InputMouseButtonLeft, 1)
			page.WaitStable(500 * time.Millisecond)
		}

		return fmt.Sprintf("Typed %q into %s", text, sel)
	},
}

var BrowserGetText = &ToolDef{
	Name:        "browser_get_text",
	Description: "Get the text content from the current page or a specific element.",
	Args: []ToolArg{
		{Name: "selector", Description: "CSS selector (default: body - entire page text)", Required: false},
	},
	Execute: func(args map[string]string) string {
		page, err := getPage()
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}

		sel := args["selector"]
		var text string
		if sel == "" || sel == "body" {
			text = page.MustEval(`() => document.body.innerText`).String()
		} else {
			el, err := page.Timeout(10 * time.Second).Element(sel)
			if err != nil {
				return fmt.Sprintf("Error: selector %q not found: %v", sel, err)
			}
			t, err := el.Text()
			if err != nil {
				return fmt.Sprintf("Error getting text from %q: %v", sel, err)
			}
			text = t
		}

		text = strings.TrimSpace(text)
		if len(text) > 8000 {
			text = text[:8000] + "\n...(truncated)"
		}
		if text == "" {
			return "(no text found)"
		}
		return text
	},
}

var BrowserEval = &ToolDef{
	Name:        "browser_eval",
	Description: "Execute JavaScript on the current page and return the result.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "js", Description: "JavaScript to evaluate (e.g. 'document.title', 'document.querySelectorAll(\"a\").length')", Required: true},
	},
	Execute: func(args map[string]string) string {
		js := args["js"]
		if js == "" {
			return "Error: js is required"
		}

		page, err := getPage()
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}

		result, err := page.Timeout(15 * time.Second).Eval(`() => {
			try { return String(eval(` + "`" + js + "`" + `)); } catch(e) { return "JS Error: " + e.message; }
		}`)
		if err != nil {
			return fmt.Sprintf("Error evaluating JS: %v", err)
		}

		text := result.Value.String()
		if len(text) > 8000 {
			text = text[:8000] + "\n...(truncated)"
		}
		return text
	},
}

var BrowserScreenshot = &ToolDef{
	Name:        "browser_screenshot",
	Description: "Take a screenshot of the current page or a specific element. Saves as PNG.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "path", Description: "File path to save (default: temp file)", Required: false},
		{Name: "selector", Description: "CSS selector for element-level screenshot (default: full page)", Required: false},
		{Name: "full_page", Description: "Capture full scrollable page (default: false, viewport only)", Required: false},
	},
	Execute: func(args map[string]string) string {
		page, err := getPage()
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}

		savePath := args["path"]
		if savePath == "" {
			f, err := os.CreateTemp("", "apexclaw-screenshot-*.png")
			if err != nil {
				return fmt.Sprintf("Error creating temp file: %v", err)
			}
			f.Close()
			savePath = f.Name()
		}

		var buf []byte

		if sel := args["selector"]; sel != "" {
			el, err := page.Timeout(10 * time.Second).Element(sel)
			if err != nil {
				return fmt.Sprintf("Error: selector %q not found: %v", sel, err)
			}
			buf, err = el.Screenshot(proto.PageCaptureScreenshotFormatPng, 90)
			if err != nil {
				return fmt.Sprintf("Error taking element screenshot: %v", err)
			}
		} else if strings.EqualFold(args["full_page"], "true") {
			buf, err = page.Screenshot(true, &proto.PageCaptureScreenshot{
				Format: proto.PageCaptureScreenshotFormatPng,
			})
			if err != nil {
				return fmt.Sprintf("Error taking full page screenshot: %v", err)
			}
		} else {
			buf, err = page.Screenshot(false, &proto.PageCaptureScreenshot{
				Format: proto.PageCaptureScreenshotFormatPng,
			})
			if err != nil {
				return fmt.Sprintf("Error taking screenshot: %v", err)
			}
		}

		if len(buf) == 0 {
			return "Error: screenshot resulted in empty image"
		}
		if err := os.WriteFile(savePath, buf, 0644); err != nil {
			return fmt.Sprintf("Error saving: %v", err)
		}
		return fmt.Sprintf("Screenshot saved to: %s (%d bytes)", savePath, len(buf))
	},
}

var BrowserWait = &ToolDef{
	Name:        "browser_wait",
	Description: "Wait for an element to appear on the page, or wait for the page to stabilize. Use this when a page is loading or after clicking something.",
	Args: []ToolArg{
		{Name: "selector", Description: "CSS selector to wait for (omit to just wait for page stability)", Required: false},
		{Name: "timeout", Description: "Max wait time in seconds (default: 15)", Required: false},
	},
	Execute: func(args map[string]string) string {
		page, err := getPage()
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}

		timeoutSec := 15
		if t := args["timeout"]; t != "" {
			fmt.Sscanf(t, "%d", &timeoutSec)
		}
		if timeoutSec > 60 {
			timeoutSec = 60
		}
		timeout := time.Duration(timeoutSec) * time.Second

		sel := args["selector"]
		if sel != "" {
			_, err := page.Timeout(timeout).Element(sel)
			if err != nil {
				return fmt.Sprintf("Timeout: selector %q not found within %ds", sel, timeoutSec)
			}
			return fmt.Sprintf("Element %q found", sel)
		}

		if err := page.Timeout(timeout).WaitStable(500 * time.Millisecond); err != nil {
			return fmt.Sprintf("Page did not stabilize within %ds", timeoutSec)
		}
		return "Page is stable"
	},
}

var BrowserSelect = &ToolDef{
	Name:        "browser_select",
	Description: "Select an option from a dropdown/select element.",
	Args: []ToolArg{
		{Name: "selector", Description: "CSS selector of the <select> element", Required: true},
		{Name: "value", Description: "Option value to select", Required: false},
		{Name: "text", Description: "Option text to match (alternative to value)", Required: false},
	},
	Execute: func(args map[string]string) string {
		sel := args["selector"]
		if sel == "" {
			return "Error: selector is required"
		}

		page, err := getPage()
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}

		el, err := page.Timeout(10 * time.Second).Element(sel)
		if err != nil {
			return fmt.Sprintf("Error: selector %q not found: %v", sel, err)
		}

		if val := args["value"]; val != "" {
			err = el.Select([]string{val}, true, rod.SelectorTypeCSSSector)
			if err != nil {
				return fmt.Sprintf("Error selecting value %q: %v", val, err)
			}
			return fmt.Sprintf("Selected value: %s", val)
		}

		if text := args["text"]; text != "" {
			err = el.Select([]string{text}, true, rod.SelectorTypeText)
			if err != nil {
				return fmt.Sprintf("Error selecting text %q: %v", text, err)
			}
			return fmt.Sprintf("Selected text: %s", text)
		}

		return "Error: provide either value or text to select"
	},
}

var BrowserScroll = &ToolDef{
	Name:        "browser_scroll",
	Description: "Scroll the page down/up or to a specific element. Useful for lazy-loaded content.",
	Args: []ToolArg{
		{Name: "selector", Description: "CSS selector to scroll to (optional)", Required: false},
		{Name: "direction", Description: "Scroll direction: 'down', 'up', 'bottom', 'top' (default: down)", Required: false},
	},
	Execute: func(args map[string]string) string {
		page, err := getPage()
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}

		if sel := args["selector"]; sel != "" {
			el, err := page.Timeout(10 * time.Second).Element(sel)
			if err != nil {
				return fmt.Sprintf("Error: selector %q not found: %v", sel, err)
			}
			err = el.ScrollIntoView()
			if err != nil {
				return fmt.Sprintf("Error scrolling to %q: %v", sel, err)
			}
			return fmt.Sprintf("Scrolled to: %s", sel)
		}

		dir := strings.ToLower(args["direction"])
		switch dir {
		case "up":
			page.MustEval(`() => window.scrollBy(0, -500)`)
		case "top":
			page.MustEval(`() => window.scrollTo(0, 0)`)
		case "bottom":
			page.MustEval(`() => window.scrollTo(0, document.body.scrollHeight)`)
		default:
			page.MustEval(`() => window.scrollBy(0, 500)`)
		}

		page.WaitStable(300 * time.Millisecond)
		return fmt.Sprintf("Scrolled %s", dir)
	},
}

var BrowserTabs = &ToolDef{
	Name:        "browser_tabs",
	Description: "Manage browser tabs: list open tabs, switch between them, open new tabs, or close tabs.",
	Args: []ToolArg{
		{Name: "action", Description: "Action: 'list', 'new', 'switch', 'close' (default: list)", Required: false},
		{Name: "name", Description: "Tab name for switch/close, or URL for new tab", Required: false},
	},
	Execute: func(args map[string]string) string {
		browser, err := getBrowser()
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}

		action := strings.ToLower(args["action"])
		name := args["name"]

		switch action {
		case "new":
			url := name
			if url == "" {
				url = "about:blank"
			}
			newPage := stealth.MustPage(browser)
			newPage.MustSetViewport(1280, 900, 1, false)
			if url != "about:blank" {
				if err := newPage.Navigate(url); err != nil {
					return fmt.Sprintf("Error navigating new tab: %v", err)
				}
				newPage.WaitStable(300 * time.Millisecond)
			}

			tabName := name
			if tabName == "" {
				tabName = fmt.Sprintf("tab_%d", len(rodPages)+1)
			}
			rodPages[tabName] = newPage
			rodPage = newPage
			return fmt.Sprintf("Opened new tab: %s", tabName)

		case "switch":
			if name == "" {
				return "Error: name is required for switch"
			}
			p, ok := rodPages[name]
			if !ok {
				return fmt.Sprintf("Error: no tab named %q. Use action=list to see tabs.", name)
			}
			rodPage = p
			return fmt.Sprintf("Switched to tab: %s", name)

		case "close":
			if name == "" {
				return "Error: name is required for close"
			}
			p, ok := rodPages[name]
			if !ok {
				return fmt.Sprintf("Error: no tab named %q", name)
			}
			p.MustClose()
			delete(rodPages, name)
			if rodPage == p {
				rodPage = nil
			}
			return fmt.Sprintf("Closed tab: %s", name)

		default:
			var tabs []string
			for n, p := range rodPages {
				info := p.MustInfo()
				tabs = append(tabs, fmt.Sprintf("  %s → %s", n, info.URL))
			}
			if rodPage != nil {
				info := rodPage.MustInfo()
				tabs = append([]string{fmt.Sprintf("  [active] → %s", info.URL)}, tabs...)
			}
			if len(tabs) == 0 {
				return "No tabs open"
			}
			return "Open tabs:\n" + strings.Join(tabs, "\n")
		}
	},
}

var BrowserCookies = &ToolDef{
	Name:        "browser_cookies",
	Description: "Manage browser cookies: get, set, or clear cookies. Cookies persist across sessions via user data dir.",
	Args: []ToolArg{
		{Name: "action", Description: "Action: 'get', 'set', 'clear' (default: get)", Required: false},
		{Name: "domain", Description: "Domain filter for get/clear (e.g. '.google.com')", Required: false},
		{Name: "name", Description: "Cookie name (for set)", Required: false},
		{Name: "value", Description: "Cookie value (for set)", Required: false},
	},
	Execute: func(args map[string]string) string {
		page, err := getPage()
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}

		action := strings.ToLower(args["action"])
		switch action {
		case "set":
			name := args["name"]
			value := args["value"]
			domain := args["domain"]
			if name == "" || value == "" || domain == "" {
				return "Error: name, value, and domain are required for set"
			}
			err := page.SetCookies([]*proto.NetworkCookieParam{{
				Name:   name,
				Value:  value,
				Domain: domain,
			}})
			if err != nil {
				return fmt.Sprintf("Error setting cookie: %v", err)
			}
			return fmt.Sprintf("Cookie set: %s=%s (domain: %s)", name, value, domain)
		default:
			cookies, err := page.Cookies([]string{})
			if err != nil {
				return fmt.Sprintf("Error getting cookies: %v", err)
			}
			domain := args["domain"]
			var result []string
			for _, c := range cookies {
				if domain != "" && !strings.Contains(c.Domain, domain) {
					continue
				}
				result = append(result, fmt.Sprintf("  %s=%s (domain: %s, expires: %s)", c.Name, c.Value, c.Domain, time.Unix(int64(c.Expires), 0).Format("2006-01-02")))
			}
			if len(result) == 0 {
				return "No cookies found"
			}
			if len(result) > 50 {
				result = result[:50]
				result = append(result, "  ...(truncated)")
			}
			return fmt.Sprintf("Cookies (%d):\n%s", len(result), strings.Join(result, "\n"))
		}
	},
}

var BrowserFormFill = &ToolDef{
	Name:        "browser_form_fill",
	Description: "Fill multiple form fields at once. Saves iterations vs typing one at a time. Pass a JSON mapping of CSS selectors to values.",
	Args: []ToolArg{
		{Name: "fields", Description: "JSON object: {\"#email\": \"user@example.com\", \"#password\": \"pass123\", \"#name\": \"John\"}", Required: true},
		{Name: "submit", Description: "CSS selector of submit button to click after filling (optional)", Required: false},
	},
	Execute: func(args map[string]string) string {
		fieldsJSON := args["fields"]
		if fieldsJSON == "" {
			return "Error: fields is required"
		}

		var fields map[string]string
		if err := json.Unmarshal([]byte(fieldsJSON), &fields); err != nil {
			return fmt.Sprintf("Error parsing fields JSON: %v", err)
		}

		page, err := getPage()
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}

		var filled []string
		for sel, val := range fields {
			el, err := page.Timeout(10 * time.Second).Element(sel)
			if err != nil {
				return fmt.Sprintf("Error: field %q not found: %v", sel, err)
			}
			el.MustSelectAllText().MustInput("")
			if err := el.Input(val); err != nil {
				return fmt.Sprintf("Error typing into %q: %v", sel, err)
			}
			filled = append(filled, sel)
		}

		if submitSel := args["submit"]; submitSel != "" {
			el, err := page.Timeout(10 * time.Second).Element(submitSel)
			if err != nil {
				return fmt.Sprintf("Filled %d fields but submit button %q not found: %v", len(filled), submitSel, err)
			}
			el.MustClick()
			page.WaitStable(500 * time.Millisecond)
			return fmt.Sprintf("Filled %d fields and submitted via %s", len(filled), submitSel)
		}

		return fmt.Sprintf("Filled %d fields: %s", len(filled), strings.Join(filled, ", "))
	},
}

var BrowserPDF = &ToolDef{
	Name:        "browser_pdf",
	Description: "Save the current page as a PDF file. Useful for saving receipts, confirmations, or any page content.",
	Args: []ToolArg{
		{Name: "path", Description: "File path to save PDF (default: temp file)", Required: false},
	},
	Execute: func(args map[string]string) string {
		page, err := getPage()
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}

		savePath := args["path"]
		if savePath == "" {
			f, err := os.CreateTemp("", "apexclaw-page-*.pdf")
			if err != nil {
				return fmt.Sprintf("Error creating temp file: %v", err)
			}
			f.Close()
			savePath = f.Name()
		}

		reader, err := page.PDF(&proto.PagePrintToPDF{
			PrintBackground: true,
		})
		if err != nil {
			return fmt.Sprintf("Error generating PDF: %v", err)
		}

		buf := make([]byte, 0)
		tmp := make([]byte, 4096)
		for {
			n, readErr := reader.Read(tmp)
			if n > 0 {
				buf = append(buf, tmp[:n]...)
			}
			if readErr != nil {
				break
			}
		}

		if err := os.WriteFile(savePath, buf, 0644); err != nil {
			return fmt.Sprintf("Error saving PDF: %v", err)
		}
		return fmt.Sprintf("PDF saved to: %s (%d bytes)", savePath, len(buf))
	},
}
