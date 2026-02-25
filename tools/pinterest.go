package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

func pinterestHTMLGet(reqURL string) ([]byte, error) {
	client := &http.Client{
		Timeout: 20 * time.Second,
	}
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("HTTP %d: %.200s", resp.StatusCode, string(body))
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
}

func pinterestAPIGet(reqURL string) ([]byte, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36")
	req.Header.Set("X-App-Version", "e5cf318")
	req.Header.Set("X-Pinterest-Appstate", "active")
	req.Header.Set("X-Pinterest-Pws-Handler", "www/index.js")
	req.Header.Set("X-Pinterest-Source-Url", "/")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("HTTP %d: %.200s", resp.StatusCode, string(body))
	}
	return io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
}

var pinImgSizes = []string{"orig", "736x", "474x", "236x"}

func extractImgURL(images map[string]any) string {
	for _, size := range pinImgSizes {
		if entry, ok := images[size]; ok {
			if m, ok := entry.(map[string]any); ok {
				if u, ok := m["url"].(string); ok && u != "" {
					return u
				}
			}
		}
	}
	return ""
}

var pwsInitialRe = regexp.MustCompile(`id="__PWS_INITIAL_STRING__"[^>]*>([^<]+)<`)

func fetchPinterestImages(query string, lim int, offset int) ([]string, error) {

	searchURL := fmt.Sprintf("https://www.pinterest.com/search/pins/?q=%s&rs=typed", url.QueryEscape(query))

	body, err := pinterestHTMLGet(searchURL)
	if err != nil {
		return nil, err
	}

	html := string(body)

	matches := pwsInitialRe.FindStringSubmatch(html)
	if len(matches) < 2 {

		return extractImgURLsFromHTML(html, lim, offset)
	}

	jsonStr := matches[1]

	jsonStr = strings.ReplaceAll(jsonStr, "&amp;", "&")
	jsonStr = strings.ReplaceAll(jsonStr, "&#39;", "'")
	jsonStr = strings.ReplaceAll(jsonStr, "&quot;", `"`)
	jsonStr = strings.ReplaceAll(jsonStr, "&lt;", "<")
	jsonStr = strings.ReplaceAll(jsonStr, "&gt;", ">")

	return extractImgURLsFromPWSJSON(jsonStr, lim, offset)
}

func extractImgURLsFromPWSJSON(jsonStr string, lim, offset int) ([]string, error) {
	var root map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &root); err != nil {

		var inner string
		if err2 := json.Unmarshal([]byte(jsonStr), &inner); err2 == nil {
			if err3 := json.Unmarshal([]byte(inner), &root); err3 != nil {
				return nil, fmt.Errorf("PWS JSON parse failed: %v", err3)
			}
		} else {
			return nil, fmt.Errorf("PWS JSON parse failed: %v", err)
		}
	}

	var urls []string
	walkForImages(root, &urls)

	if len(urls) == 0 {
		return nil, nil
	}

	seen := make(map[string]bool)
	var deduped []string
	for _, u := range urls {
		if !seen[u] {
			seen[u] = true
			deduped = append(deduped, u)
		}
	}

	start := offset * lim
	if start >= len(deduped) {
		return nil, fmt.Errorf("no more results (offset %d, total %d)", offset, len(deduped))
	}
	end := start + lim
	if end > len(deduped) {
		end = len(deduped)
	}
	return deduped[start:end], nil
}

func walkForImages(v any, out *[]string) {
	switch node := v.(type) {
	case map[string]any:
		if images, ok := node["images"]; ok {
			if imgMap, ok := images.(map[string]any); ok {
				if u := extractImgURL(imgMap); u != "" {
					*out = append(*out, u)
					return
				}
			}
		}
		for _, child := range node {
			walkForImages(child, out)
		}
	case []any:
		for _, item := range node {
			walkForImages(item, out)
		}
	}
}

var imgURLRe = regexp.MustCompile(`https://i\.pinimg\.com/(?:originals|736x|474x)/[^\s"'<>]+\.(?:jpg|jpeg|png|webp)`)

func extractImgURLsFromHTML(html string, lim, offset int) ([]string, error) {
	found := imgURLRe.FindAllString(html, -1)
	seen := make(map[string]bool)
	var urls []string
	for _, u := range found {

		if !seen[u] {
			seen[u] = true
			urls = append(urls, u)
		}
	}
	if len(urls) == 0 {
		return nil, nil
	}
	start := offset * lim
	if start >= len(urls) {
		return nil, fmt.Errorf("no more results")
	}
	end := start + lim
	if end > len(urls) {
		end = len(urls)
	}
	return urls[start:end], nil
}

func formatPin(pin map[string]any) string {
	id, _ := pin["id"].(string)
	desc := ""
	if d, ok := pin["description"].(string); ok {
		desc = strings.TrimSpace(d)
	}
	if desc == "" {
		if d, ok := pin["title"].(string); ok {
			desc = strings.TrimSpace(d)
		}
	}

	imgURL := ""
	if images, ok := pin["images"].(map[string]any); ok {
		imgURL = extractImgURL(images)
	}

	pinURL := fmt.Sprintf("https://www.pinterest.com/pin/%s/", id)

	board := ""
	if b, ok := pin["board"].(map[string]any); ok {
		if name, ok := b["name"].(string); ok {
			board = name
		}
	}

	var parts []string
	if id != "" {
		parts = append(parts, fmt.Sprintf("ID: %s", id))
	}
	if desc != "" {
		parts = append(parts, fmt.Sprintf("Desc: %s", desc))
	}
	if board != "" {
		parts = append(parts, fmt.Sprintf("Board: %s", board))
	}
	parts = append(parts, fmt.Sprintf("Pin: %s", pinURL))
	if imgURL != "" {
		parts = append(parts, fmt.Sprintf("Image: %s", imgURL))
	}
	return strings.Join(parts, "\n")
}

var PinterestSearch = &ToolDef{
	Name:        "pinterest_search",
	Description: "Search Pinterest for pins by keyword and send the images directly to Telegram. Great for wallpapers, recipes, fashion, art, interior design, etc.",
	Args: []ToolArg{
		{Name: "query", Description: "Search term (e.g. 'sunset wallpaper', 'minimalist interior', 'anime art')", Required: true},
		{Name: "count", Description: "Number of images to send (default 5, max 20)", Required: false},
		{Name: "offset", Description: "Page offset for pagination (default 0)", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		query := strings.TrimSpace(args["query"])
		if query == "" {
			return "Error: query is required"
		}

		count := 5
		if c := strings.TrimSpace(args["count"]); c != "" {
			var n int
			if _, err := fmt.Sscan(c, &n); err == nil && n > 0 {
				if n > 20 {
					n = 20
				}
				count = n
			}
		}

		offset := 0
		if o := strings.TrimSpace(args["offset"]); o != "" {
			var n int
			if _, err := fmt.Sscan(o, &n); err == nil && n >= 0 {
				offset = n
			}
		}

		urls, err := fetchPinterestImages(query, count, offset)
		if err != nil {
			return fmt.Sprintf("Pinterest search error: %v", err)
		}
		if len(urls) == 0 {
			return fmt.Sprintf("No Pinterest results found for %q", query)
		}

		var chatID int64
		if GetTelegramContextFn != nil {
			ctx := GetTelegramContextFn(userID)
			if ctx != nil {
				if v, ok := ctx["telegram_id"]; ok {
					chatID = v.(int64)
				}
			}
		}

		if chatID == 0 || SendTGAlbumURLsFn == nil {

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Pinterest: %q — %d images\n\n", query, len(urls)))
			for i, u := range urls {
				sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, u))
			}
			return strings.TrimSpace(sb.String())
		}

		const albumSize = 10
		sent := 0
		var errs []string
		for i := 0; i < len(urls); i += albumSize {
			end := i + albumSize
			if end > len(urls) {
				end = len(urls)
			}
			batch := urls[i:end]
			caption := fmt.Sprintf("📌 Pinterest: %q", query)
			if result := SendTGAlbumURLsFn(chatID, batch, caption); result != "" {
				errs = append(errs, result)
			} else {
				sent += len(batch)
			}
		}

		if len(errs) > 0 {
			return fmt.Sprintf("Sent %d/%d images. Errors:\n%s", sent, len(urls), strings.Join(errs, "\n"))
		}
		return fmt.Sprintf("Sent %d Pinterest images for %q", sent, query)
	},
}

var PinterestGetPin = &ToolDef{
	Name:        "pinterest_get_pin",
	Description: "Get details and send the full-resolution image for a specific Pinterest pin by its ID or URL.",
	Args: []ToolArg{
		{Name: "pin_id", Description: "Pinterest pin ID or full URL (e.g. '123456789' or 'https://pinterest.com/pin/123456/')", Required: true},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		pinID := strings.TrimSpace(args["pin_id"])
		if pinID == "" {
			return "Error: pin_id is required"
		}

		if strings.Contains(pinID, "pinterest.com/pin/") {
			parts := strings.Split(pinID, "/pin/")
			pinID = strings.Trim(parts[len(parts)-1], "/")
		}

		sourceURL := "/pin/" + pinID + "/"
		data := fmt.Sprintf(`{"options":{"id":"%s","field_set_key":"detailed"},"context":{}}`, pinID)

		params := url.Values{}
		params.Set("source_url", sourceURL)
		params.Set("data", data)

		reqURL := "https://www.pinterest.com/resource/PinResource/get/?" + params.Encode()

		body, err := pinterestAPIGet(reqURL)
		if err != nil {
			return fmt.Sprintf("Pinterest fetch error: %v", err)
		}

		var resp struct {
			ResourceResponse struct {
				Data map[string]any `json:"data"`
			} `json:"resource_response"`
		}

		if err := json.Unmarshal(body, &resp); err != nil {
			return fmt.Sprintf("Parse error: %v", err)
		}

		pin := resp.ResourceResponse.Data
		if len(pin) == 0 {
			return fmt.Sprintf("Pin %s not found", pinID)
		}

		imgURL := ""
		if images, ok := pin["images"].(map[string]any); ok {
			imgURL = extractImgURL(images)
		}

		desc := ""
		if d, ok := pin["description"].(string); ok {
			desc = strings.TrimSpace(d)
		}
		creator := ""
		if pinner, ok := pin["pinner"].(map[string]any); ok {
			name, _ := pinner["full_name"].(string)
			uname, _ := pinner["username"].(string)
			if name != "" {
				creator = fmt.Sprintf(" by %s (@%s)", name, uname)
			}
		}
		caption := fmt.Sprintf("📌 Pin %s%s", pinID, creator)
		if desc != "" && len(desc) < 200 {
			caption += "\n" + desc
		}

		var chatID int64
		if GetTelegramContextFn != nil {
			ctx := GetTelegramContextFn(userID)
			if ctx != nil {
				if v, ok := ctx["telegram_id"]; ok {
					chatID = v.(int64)
				}
			}
		}

		if imgURL != "" && chatID != 0 && SendTGPhotoURLFn != nil {
			if result := SendTGPhotoURLFn(chatID, imgURL, caption); result != "" {
				return fmt.Sprintf("Fetched pin but failed to send image: %s\nURL: %s", result, imgURL)
			}
			return fmt.Sprintf("Sent pin %s image to chat", pinID)
		}

		var sb strings.Builder
		sb.WriteString(formatPin(pin))
		if link, ok := pin["link"].(string); ok && link != "" {
			sb.WriteString(fmt.Sprintf("\nSource: %s", link))
		}
		return strings.TrimSpace(sb.String())
	},
}
