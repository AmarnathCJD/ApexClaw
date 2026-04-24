package tools

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ValidateExternalURL rejects URLs that would let a tool reach internal or
// cloud-metadata endpoints (SSRF). It's applied to AI-driven HTTP tools like
// web_fetch and http_request where the model controls the target URL.
//
// Returns a non-nil error when the URL should be refused. Allow-list can be
// extended via the TOOL_HTTP_ALLOW_HOSTS env var (comma-separated hostnames)
// for self-hosted services the agent legitimately needs to reach.
func ValidateExternalURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("only http(s) schemes allowed, got %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("url has no host")
	}

	// Explicit allow-list (self-hosted services) bypasses the private-IP check.
	for _, allowed := range splitCSV(os.Getenv("TOOL_HTTP_ALLOW_HOSTS")) {
		if strings.EqualFold(host, allowed) {
			return nil
		}
	}

	// Explicit hostname deny-list for cloud metadata + loopback.
	lc := strings.ToLower(host)
	switch lc {
	case "localhost", "ip6-localhost", "ip6-loopback",
		"metadata", "metadata.google.internal",
		"metadata.internal":
		return fmt.Errorf("blocked host %q (metadata/loopback)", host)
	}

	// Resolve the host to IPs and reject any private/loopback/link-local/CGNAT/
	// multicast/unspecified address. Uses the default resolver with a short
	// timeout so a malicious DNS response can't hang the tool.
	ctx, cancel := contextWithTimeout()
	defer cancel()
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("dns lookup failed: %w", err)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return fmt.Errorf("blocked address %s for host %q", ip.String(), host)
		}
	}
	return nil
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func isBlockedIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsMulticast() ||
		ip.IsPrivate() {
		return true
	}
	// Cloud metadata + CGNAT ranges not covered by IsPrivate on older Go.
	blocked := []string{
		"169.254.169.254/32",
		"100.64.0.0/10",
		"fd00::/8",
		"fe80::/10",
	}
	for _, cidr := range blocked {
		_, n, err := net.ParseCIDR(cidr)
		if err == nil && n.Contains(ip) {
			return true
		}
	}
	return false
}

// pathSandboxRoot returns an absolute directory that file tools are confined
// to when set. Empty string means no sandbox (current behavior).
func pathSandboxRoot() string {
	v := strings.TrimSpace(os.Getenv("TOOL_FILE_SANDBOX"))
	if v == "" {
		return ""
	}
	abs, err := filepath.Abs(v)
	if err != nil {
		return v
	}
	return abs
}

// SafeFilePath rejects paths that escape the sandbox directory when one is
// configured. Returns the absolute path on success so callers don't need to
// re-resolve it.
func SafeFilePath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("path is required")
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	root := pathSandboxRoot()
	if root == "" {
		return abs, nil
	}
	// Resolve symlinks where possible so a symlink inside the sandbox can't
	// point at /etc/shadow.
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return "", fmt.Errorf("path %q escapes sandbox %q", raw, root)
	}
	return abs, nil
}

func contextWithTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 3*time.Second)
}
