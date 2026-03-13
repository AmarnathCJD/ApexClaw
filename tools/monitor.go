package tools

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type MonitorEntry struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	Label       string `json:"label"`
	Interval    string `json:"interval"`
	LastHash    string `json:"last_hash"`
	LastChecked string `json:"last_checked"`
	LastContent string `json:"last_content"`
	HitCount    int    `json:"hit_count"`
	Enabled     bool   `json:"enabled"`
	OwnerID     string `json:"owner_id"`
	TelegramID  int64  `json:"telegram_id"`
	CreatedAt   string `json:"created_at"`
}

type monitorStore struct {
	mu      sync.Mutex
	entries []MonitorEntry
}

var monStore = &monitorStore{}

var MonitorAlertFn func(ownerID string, telegramID int64, label, url, diff string)

func monitorPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".apexclaw", "monitors.json")
}

func loadMonitors() {
	monStore.mu.Lock()
	defer monStore.mu.Unlock()
	data, err := os.ReadFile(monitorPath())
	if err != nil {
		return
	}
	json.Unmarshal(data, &monStore.entries)
}

func saveMonitors() {
	monStore.mu.Lock()
	defer monStore.mu.Unlock()
	path := monitorPath()
	os.MkdirAll(filepath.Dir(path), 0755)
	data, _ := json.MarshalIndent(monStore.entries, "", "  ")
	os.WriteFile(path, data, 0644)
}

func StartMonitor() {
	loadMonitors()
	go func() {
		for {
			time.Sleep(60 * time.Second)
			runMonitorTick()
		}
	}()
}

func runMonitorTick() {
	monStore.mu.Lock()
	entries := make([]MonitorEntry, len(monStore.entries))
	copy(entries, monStore.entries)
	monStore.mu.Unlock()

	for _, e := range entries {
		if !e.Enabled {
			continue
		}
		interval := parseMonitorInterval(e.Interval)
		if e.LastChecked != "" {
			last, err := time.Parse(time.RFC3339, e.LastChecked)
			if err == nil && time.Since(last) < interval {
				continue
			}
		}
		go checkMonitorEntry(e)
	}
}

func parseMonitorInterval(s string) time.Duration {
	switch strings.ToLower(s) {
	case "1m", "1min":
		return time.Minute
	case "5m", "5min":
		return 5 * time.Minute
	case "15m", "15min":
		return 15 * time.Minute
	case "30m", "30min":
		return 30 * time.Minute
	case "1h", "hourly":
		return time.Hour
	case "6h":
		return 6 * time.Hour
	case "12h":
		return 12 * time.Hour
	case "24h", "daily":
		return 24 * time.Hour
	default:
		return time.Hour
	}
}

func checkMonitorEntry(e MonitorEntry) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", e.URL, nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", "ApexClaw-Monitor/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return
	}

	content := stripHTMLTags(string(body))
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
	now := time.Now().Format(time.RFC3339)

	monStore.mu.Lock()
	for i, ent := range monStore.entries {
		if ent.ID != e.ID {
			continue
		}
		prevHash := monStore.entries[i].LastHash
		prevContent := monStore.entries[i].LastContent
		monStore.entries[i].LastChecked = now
		monStore.entries[i].LastHash = hash
		monStore.entries[i].LastContent = content[:min256(len(content), 500)]

		if prevHash != "" && prevHash != hash {
			monStore.entries[i].HitCount++
			diff := buildTextDiff(prevContent, content[:min256(len(content), 500)])
			monStore.mu.Unlock()
			saveMonitors()
			if MonitorAlertFn != nil {
				MonitorAlertFn(e.OwnerID, e.TelegramID, e.Label, e.URL, diff)
			}
			return
		}
		monStore.mu.Unlock()
		saveMonitors()
		return
	}
	monStore.mu.Unlock()
}

func stripHTMLTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
		} else if r == '>' {
			inTag = false
			b.WriteRune(' ')
		} else if !inTag {
			b.WriteRune(r)
		}
	}
	result := strings.Join(strings.Fields(b.String()), " ")
	return result
}

func buildTextDiff(old, new string) string {
	oldWords := strings.Fields(old)
	newWords := strings.Fields(new)
	oldSet := make(map[string]bool)
	newSet := make(map[string]bool)
	for _, w := range oldWords {
		oldSet[w] = true
	}
	for _, w := range newWords {
		newSet[w] = true
	}
	var added, removed []string
	for w := range newSet {
		if !oldSet[w] {
			added = append(added, w)
		}
	}
	for w := range oldSet {
		if !newSet[w] {
			removed = append(removed, w)
		}
	}
	if len(added) > 10 {
		added = added[:10]
	}
	if len(removed) > 10 {
		removed = removed[:10]
	}
	var parts []string
	if len(added) > 0 {
		parts = append(parts, fmt.Sprintf("New: %s", strings.Join(added, ", ")))
	}
	if len(removed) > 0 {
		parts = append(parts, fmt.Sprintf("Removed: %s", strings.Join(removed, ", ")))
	}
	if len(parts) == 0 {
		return "Content changed (structure/whitespace)"
	}
	return strings.Join(parts, " | ")
}

func min256(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var MonitorAdd = &ToolDef{
	Name:        "monitor_add",
	Description: "Watch a URL for content changes. Get alerted via Telegram whenever the page changes. Supports intervals: 5m, 15m, 30m, 1h, 6h, 12h, daily.",
	Args: []ToolArg{
		{Name: "url", Description: "URL to monitor", Required: true},
		{Name: "label", Description: "Short name for this monitor (e.g. 'bitcoin_price')", Required: true},
		{Name: "interval", Description: "Check interval: 5m, 15m, 30m, 1h, 6h, 12h, daily (default: 1h)", Required: false},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		url := args["url"]
		label := args["label"]
		interval := args["interval"]
		if url == "" || label == "" {
			return "Error: url and label are required"
		}
		if interval == "" {
			interval = "1h"
		}

		var telegramID int64
		var ownerID string
		if GetTelegramContextFn != nil {
			ctx := GetTelegramContextFn(userID)
			if ctx != nil {
				telegramID, _ = ctx["telegram_id"].(int64)
				ownerID, _ = ctx["owner_id"].(string)
			}
		}
		if ownerID == "" {
			ownerID = userID
		}

		id := fmt.Sprintf("mon_%d", time.Now().UnixNano())
		entry := MonitorEntry{
			ID:         id,
			URL:        url,
			Label:      label,
			Interval:   interval,
			Enabled:    true,
			OwnerID:    ownerID,
			TelegramID: telegramID,
			CreatedAt:  time.Now().Format(time.RFC3339),
		}

		monStore.mu.Lock()
		for i, e := range monStore.entries {
			if e.Label == label && e.OwnerID == ownerID {
				monStore.entries[i] = entry
				monStore.mu.Unlock()
				saveMonitors()
				return fmt.Sprintf("Monitor %q updated → checking every %s", label, interval)
			}
		}
		monStore.entries = append(monStore.entries, entry)
		monStore.mu.Unlock()
		saveMonitors()
		return fmt.Sprintf("Monitor %q added → checking every %s. You'll be alerted when the page changes.", label, interval)
	},
	Execute: func(args map[string]string) string {
		return "Error: monitor_add requires context"
	},
}

var MonitorList = &ToolDef{
	Name:        "monitor_list",
	Description: "List all active URL monitors with their status and last check time.",
	Args:        []ToolArg{},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		monStore.mu.Lock()
		defer monStore.mu.Unlock()

		var ownerID string
		if GetTelegramContextFn != nil {
			ctx := GetTelegramContextFn(userID)
			if ctx != nil {
				ownerID, _ = ctx["owner_id"].(string)
			}
		}

		var mine []MonitorEntry
		for _, e := range monStore.entries {
			if e.OwnerID == ownerID || e.OwnerID == userID {
				mine = append(mine, e)
			}
		}
		if len(mine) == 0 {
			return "No active monitors. Use monitor_add to start watching URLs."
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "Active Monitors (%d)\n\n", len(mine))
		for _, e := range mine {
			status := "✅"
			if !e.Enabled {
				status = "⏸"
			}
			last := "never"
			if e.LastChecked != "" {
				if t, err := time.Parse(time.RFC3339, e.LastChecked); err == nil {
					last = fmt.Sprintf("%s ago", formatDuration(time.Since(t)))
				}
			}
			fmt.Fprintf(&sb, "%s %s | %s | checked %s | %d changes\n  %s\n",
				status, e.Label, e.Interval, last, e.HitCount, e.URL)
		}
		return strings.TrimRight(sb.String(), "\n")
	},
	Execute: func(args map[string]string) string {
		return "Error: requires context"
	},
}

var MonitorRemove = &ToolDef{
	Name:        "monitor_remove",
	Description: "Stop monitoring a URL by label.",
	Args: []ToolArg{
		{Name: "label", Description: "The monitor label to remove", Required: true},
	},
	ExecuteWithContext: func(args map[string]string, userID string) string {
		label := args["label"]
		if label == "" {
			return "Error: label is required"
		}
		monStore.mu.Lock()
		defer monStore.mu.Unlock()
		for i, e := range monStore.entries {
			if e.Label == label {
				monStore.entries = append(monStore.entries[:i], monStore.entries[i+1:]...)
				go saveMonitors()
				return fmt.Sprintf("Monitor %q removed.", label)
			}
		}
		return fmt.Sprintf("No monitor found with label %q.", label)
	},
	Execute: func(args map[string]string) string {
		return "Error: requires context"
	},
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
