package tools

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// HTTPRequest is a general-purpose HTTP client tool with SSRF protections.
// Unlike web_fetch (which is GET-only and strips response detail), this
// surfaces status, selected headers, and the raw body so the model can talk
// to REST APIs, webhooks, and other public endpoints.
var HTTPRequest = &ToolDef{
	Name:        "http_request",
	Description: "Make an HTTP request and return the response. Supports custom method, headers, body, basic-auth, follow_redirects toggle, and timeout. Blocks SSRF (no localhost, private IPs, or cloud metadata endpoints). Use for talking to public APIs, webhooks, REST endpoints.",
	Secure:      false,
	MaxOutput:   32 * 1024,
	Timeout:     60 * time.Second,
	Args: []ToolArg{
		{Name: "url", Type: ArgString, Description: "Full URL including scheme (http/https)", Required: true},
		{Name: "method", Type: ArgString, Description: "HTTP method: GET, POST, PUT, PATCH, DELETE, HEAD (default GET)"},
		{Name: "headers", Type: ArgDict, Description: "Map of header name -> value"},
		{Name: "body", Type: ArgString, Description: "Request body (raw string). Defaults Content-Type to application/json for POST/PUT/PATCH when no Content-Type header is set."},
		{Name: "timeout_seconds", Type: ArgInt, Description: "Per-request timeout in seconds (default 30, max 120)"},
		{Name: "follow_redirects", Type: ArgBool, Description: "Follow 3xx redirects (default true)"},
		{Name: "auth_user", Type: ArgString, Description: "Basic auth username"},
		{Name: "auth_pass", Type: ArgString, Description: "Basic auth password"},
		{Name: "max_response_bytes", Type: ArgInt, Description: "Maximum response bytes to read (default 65536, max 1048576)"},
	},
	Execute: func(args map[string]any) string {
		rawURL := String(args, "url")
		if rawURL == "" {
			return "Error: url is required"
		}
		if err := ValidateExternalURL(rawURL); err != nil {
			return fmt.Sprintf("Error: %v", err)
		}

		method := strings.ToUpper(StringOr(args, "method", "GET"))
		switch method {
		case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD":
		default:
			return fmt.Sprintf("Error: unsupported method %q", method)
		}

		timeoutSec := IntOr(args, "timeout_seconds", 30)
		if timeoutSec <= 0 {
			timeoutSec = 30
		}
		if timeoutSec > 120 {
			timeoutSec = 120
		}

		maxBytes := IntOr(args, "max_response_bytes", 65536)
		if maxBytes <= 0 {
			maxBytes = 65536
		}
		if maxBytes > 1048576 {
			maxBytes = 1048576
		}

		followRedirects := BoolOr(args, "follow_redirects", true)

		body := String(args, "body")
		headers := Dict(args, "headers")

		var bodyReader io.Reader
		if body != "" {
			bodyReader = bytes.NewBufferString(body)
		}

		req, err := http.NewRequest(method, rawURL, bodyReader)
		if err != nil {
			return fmt.Sprintf("Error building request: %v", err)
		}

		// Apply caller-supplied headers and track whether Content-Type was set.
		hasContentType := false
		hasUserAgent := false
		for k, v := range headers {
			name := strings.TrimSpace(k)
			if name == "" {
				continue
			}
			val := fmt.Sprintf("%v", v)
			req.Header.Set(name, val)
			lower := strings.ToLower(name)
			if lower == "content-type" {
				hasContentType = true
			}
			if lower == "user-agent" {
				hasUserAgent = true
			}
		}

		// Default Content-Type for write methods with a body.
		if !hasContentType && body != "" {
			switch method {
			case "POST", "PUT", "PATCH":
				req.Header.Set("Content-Type", "application/json")
			}
		}
		if !hasUserAgent {
			req.Header.Set("User-Agent", "ApexClaw/1.0")
		}

		if user := String(args, "auth_user"); user != "" {
			req.SetBasicAuth(user, String(args, "auth_pass"))
		}

		client := &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
		if !followRedirects {
			client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		defer resp.Body.Close()

		// Read up to maxBytes+1 so we can detect truncation.
		limited := io.LimitReader(resp.Body, int64(maxBytes)+1)
		raw, err := io.ReadAll(limited)
		if err != nil {
			return fmt.Sprintf("Error reading response: %v", err)
		}
		truncated := false
		if len(raw) > maxBytes {
			raw = raw[:maxBytes]
			truncated = true
		}

		var sb strings.Builder
		if resp.StatusCode >= 400 {
			fmt.Fprintf(&sb, "Error: HTTP %d %s\n", resp.StatusCode, http.StatusText(resp.StatusCode))
		} else {
			fmt.Fprintf(&sb, "Status: %d %s\n", resp.StatusCode, http.StatusText(resp.StatusCode))
		}
		sb.WriteString("Headers:\n")
		for _, line := range formatInterestingHeaders(resp.Header) {
			sb.WriteString("  ")
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		sb.WriteString("\nBody:\n")
		sb.Write(raw)
		if truncated {
			fmt.Fprintf(&sb, "\n(... response truncated at %d bytes ...)", maxBytes)
		}
		return sb.String()
	},
}

// formatInterestingHeaders selects the response headers worth surfacing to
// the model and returns them as "Name: value" lines. Connection-level noise
// (Connection, Keep-Alive, Date, etc.) is dropped. Set-Cookie is collapsed
// to a count to avoid leaking session tokens into the model context.
func formatInterestingHeaders(h http.Header) []string {
	if len(h) == 0 {
		return []string{"(none)"}
	}
	// Canonical names we always want when present.
	keep := map[string]bool{
		"Content-Type":   true,
		"Content-Length": true,
		"Location":       true,
		"Server":         true,
		"X-Request-Id":   true,
		"Etag":           true,
		"Cache-Control":  true,
	}
	var lines []string
	for name, vals := range h {
		canonical := http.CanonicalHeaderKey(name)
		if canonical == "Set-Cookie" {
			lines = append(lines, fmt.Sprintf("Set-Cookie: (%d cookie(s))", len(vals)))
			continue
		}
		if keep[canonical] || strings.HasPrefix(canonical, "X-Ratelimit-") {
			lines = append(lines, fmt.Sprintf("%s: %s", canonical, strings.Join(vals, ", ")))
		}
	}
	if len(lines) == 0 {
		return []string{"(none of interest)"}
	}
	sort.Strings(lines)
	if len(lines) > 8 {
		lines = lines[:8]
	}
	return lines
}
