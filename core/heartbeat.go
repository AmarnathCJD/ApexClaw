package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
)

type ScheduledTask struct {
	ID          string `json:"id"`
	Prompt      string `json:"prompt"`
	RunAt       string `json:"run_at"`
	Repeat      string `json:"repeat"`
	OwnerID     string `json:"owner_id"`
	TelegramID  int64  `json:"telegram_id"`
	MessageID   int64  `json:"message_id"`
	GroupID     int64  `json:"group_id"`
	Label       string `json:"label"`
	CreatedAt   string `json:"created_at"`
	ScheduledAt string `json:"scheduled_at"`
	RunCount    int    `json:"run_count"`
	LastResult  string `json:"last_result"`
	Enabled     bool   `json:"enabled"`
	MaxRuns     int    `json:"max_runs"`
	OnFailure   string `json:"on_failure"`
	RetryAt     string `json:"retry_at"`
	Tags        string `json:"tags"`
}

type heartbeatStore struct {
	mu    sync.Mutex
	tasks []ScheduledTask
}

var hbStore = &heartbeatStore{}
var heartbeatTGClient *telegram.Client

func heartbeatPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".apexclaw", "heartbeat.json")
}

func loadHeartbeatTasks() {
	hbStore.mu.Lock()
	defer hbStore.mu.Unlock()
	data, err := os.ReadFile(heartbeatPath())
	if err != nil {
		return
	}
	var all []ScheduledTask
	if err := json.Unmarshal(data, &all); err != nil {
		return
	}

	now := time.Now()
	for _, t := range all {
		runAt, err := time.Parse(time.RFC3339, t.RunAt)
		if err != nil {
			continue
		}
		if t.Repeat == "" && now.After(runAt) {
			log.Printf("[HEARTBEAT] dropping stale one-shot task %q (was due %s)", t.Label, t.RunAt)
			continue
		}
		hbStore.tasks = append(hbStore.tasks, t)
	}
}

func persistHeartbeatTasks() {
	hbStore.mu.Lock()
	defer hbStore.mu.Unlock()
	path := heartbeatPath()
	os.MkdirAll(filepath.Dir(path), 0755)
	data, _ := json.MarshalIndent(hbStore.tasks, "", "  ")
	os.WriteFile(path, data, 0644)
}

func ScheduleTask(t ScheduledTask) {
	now := time.Now().Format(time.RFC3339)
	if t.CreatedAt == "" {
		t.CreatedAt = now
	}
	if t.ScheduledAt == "" {
		t.ScheduledAt = t.RunAt
	}
	if !t.Enabled {
		t.Enabled = true
	}

	hbStore.mu.Lock()
	for i, existing := range hbStore.tasks {
		if existing.Label == t.Label {
			t.RunCount = existing.RunCount
			t.LastResult = existing.LastResult
			hbStore.tasks[i] = t
			hbStore.mu.Unlock()
			persistHeartbeatTasks()
			log.Printf("[HEARTBEAT] updated task %q → run_at=%s", t.Label, t.RunAt)
			return
		}
	}
	hbStore.tasks = append(hbStore.tasks, t)
	hbStore.mu.Unlock()
	persistHeartbeatTasks()
	log.Printf("[HEARTBEAT] added task %q → run_at=%s owner=%s chat=%d", t.Label, t.RunAt, t.OwnerID, t.TelegramID)
}

func PauseTask(labelOrID string) bool {
	hbStore.mu.Lock()
	defer hbStore.mu.Unlock()
	for i, t := range hbStore.tasks {
		if t.Label == labelOrID || t.ID == labelOrID {
			hbStore.tasks[i].Enabled = false
			go persistHeartbeatTasks()
			return true
		}
	}
	return false
}

func ResumeTask(labelOrID string) bool {
	hbStore.mu.Lock()
	defer hbStore.mu.Unlock()
	for i, t := range hbStore.tasks {
		if t.Label == labelOrID || t.ID == labelOrID {
			hbStore.tasks[i].Enabled = true
			go persistHeartbeatTasks()
			return true
		}
	}
	return false
}

func GetTaskStats(labelOrID string) (runCount int, lastResult string, found bool) {
	hbStore.mu.Lock()
	defer hbStore.mu.Unlock()
	for _, t := range hbStore.tasks {
		if t.Label == labelOrID || t.ID == labelOrID {
			return t.RunCount, t.LastResult, true
		}
	}
	return 0, "", false
}

func CancelTask(labelOrID string) bool {
	hbStore.mu.Lock()
	defer hbStore.mu.Unlock()
	for i, t := range hbStore.tasks {
		if t.Label == labelOrID || t.ID == labelOrID {
			hbStore.tasks = append(hbStore.tasks[:i], hbStore.tasks[i+1:]...)
			go persistHeartbeatTasks()
			return true
		}
	}
	return false
}

var heartbeatStop chan struct{}

func StartHeartbeat(client *telegram.Client) {
	heartbeatTGClient = client
	loadHeartbeatTasks()
	heartbeatStop = make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[HEARTBEAT] panic recovered: %v — restarting loop", r)
				go StartHeartbeat(client)
			}
		}()
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatStop:
				return
			case <-ticker.C:
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("[HEARTBEAT] tick panic recovered: %v", r)
						}
					}()
					runHeartbeatTick()
				}()
			}
		}
	}()
	log.Printf("[HEARTBEAT] scheduler started (%d tasks loaded)", len(hbStore.tasks))
}

func StopHeartbeat() {
	if heartbeatStop != nil {
		close(heartbeatStop)
		heartbeatStop = nil
	}
}

func runHeartbeatTick() {
	now := time.Now()
	hbStore.mu.Lock()
	var remaining []ScheduledTask
	var toRun []ScheduledTask

	for _, t := range hbStore.tasks {
		// retry_at override (set on failure when OnFailure="retry")
		if t.RetryAt != "" {
			retryAt, err := time.Parse(time.RFC3339, t.RetryAt)
			if err == nil && (now.After(retryAt) || now.Equal(retryAt)) {
				t.RetryAt = ""
				toRun = append(toRun, t)
				if t.Repeat != "" {
					t.RunAt = calcNextRun(retryAt, now, t.Repeat).Format(time.RFC3339)
				}
				remaining = append(remaining, t)
				continue
			} else if err == nil {
				remaining = append(remaining, t)
				continue
			}
		}

		runAt, err := time.Parse(time.RFC3339, t.RunAt)
		if err != nil {
			log.Printf("[HEARTBEAT] bad run_at for task %q: %v — dropping", t.Label, err)
			continue
		}

		if now.After(runAt) || now.Equal(runAt) {
			if t.Enabled {
				if t.MaxRuns > 0 && t.RunCount >= t.MaxRuns {
					log.Printf("[HEARTBEAT] task %q hit max_runs=%d — removing", t.Label, t.MaxRuns)
					continue
				}
				toRun = append(toRun, t)
			}
			if t.Repeat != "" {
				nextRun := calcNextRun(runAt, now, t.Repeat)
				if nextRun.After(runAt) {
					t.RunAt = nextRun.Format(time.RFC3339)
					remaining = append(remaining, t)
				}
			}
		} else {
			remaining = append(remaining, t)
		}
	}
	hbStore.tasks = remaining
	hbStore.mu.Unlock()

	for _, t := range toRun {
		go fireHeartbeatTask(t)
	}
	if len(toRun) > 0 {
		persistHeartbeatTasks()
	}
}

func calcNextRun(runAt, now time.Time, repeat string) time.Time {
	var add time.Duration
	repeat = strings.ToLower(strings.TrimSpace(repeat))
	switch repeat {
	case "minutely":
		add = time.Minute
	case "hourly":
		add = time.Hour
	case "daily":
		add = 24 * time.Hour
	case "weekly":
		add = 7 * 24 * time.Hour
	default:
		if strings.HasPrefix(repeat, "every_") {
			var num int
			var unit string
			if _, err := fmt.Sscanf(repeat, "every_%d_%s", &num, &unit); err == nil && num > 0 {
				if strings.HasPrefix(unit, "minute") {
					add = time.Duration(num) * time.Minute
				} else if strings.HasPrefix(unit, "hour") {
					add = time.Duration(num) * time.Hour
				} else if strings.HasPrefix(unit, "day") {
					add = time.Duration(num) * 24 * time.Hour
				}
			}
		}
	}
	if add == 0 {
		return runAt
	}
	nextRun := runAt.Add(add)
	for nextRun.Before(now) || nextRun.Equal(now) {
		nextRun = nextRun.Add(add)
	}
	return nextRun
}

func fireHeartbeatTask(t ScheduledTask) {
	log.Printf("[HEARTBEAT] firing task %q (#%d) → chat=%d", t.Label, t.RunCount+1, t.TelegramID)
	ownerID := t.OwnerID
	if ownerID == "" {
		ownerID = Cfg.OwnerID
	}

	session := NewAgentSession(GlobalRegistry, Cfg.DefaultModel, "telegram")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	reply, err := session.RunStream(ctx, ownerID, t.Prompt, nil)

	failed := err != nil || reply == ""
	if failed {
		log.Printf("[HEARTBEAT] task %q failed: err=%v empty=%v", t.Label, err, reply == "")
		onFailure := strings.ToLower(t.OnFailure)
		if onFailure == "" {
			onFailure = "skip"
		}
		switch onFailure {
		case "retry":
			retryAt := time.Now().Add(5 * time.Minute).Format(time.RFC3339)
			hbStore.mu.Lock()
			for i, st := range hbStore.tasks {
				if st.Label == t.Label {
					hbStore.tasks[i].RetryAt = retryAt
					break
				}
			}
			hbStore.mu.Unlock()
			go persistHeartbeatTasks()
			log.Printf("[HEARTBEAT] task %q scheduled retry at %s", t.Label, retryAt)
		case "disable":
			hbStore.mu.Lock()
			for i, st := range hbStore.tasks {
				if st.Label == t.Label {
					hbStore.tasks[i].Enabled = false
					break
				}
			}
			hbStore.mu.Unlock()
			go persistHeartbeatTasks()
			log.Printf("[HEARTBEAT] task %q disabled after failure", t.Label)
			if heartbeatTGClient != nil && t.TelegramID != 0 {
				heartbeatTGClient.SendMessage(t.TelegramID,
					fmt.Sprintf("⚠️ Scheduled task <b>%s</b> was disabled after a failure.", escapeHTML(t.Label)),
					&telegram.SendOptions{ParseMode: telegram.HTML})
			}
		}
		return
	}

	// Update run stats
	snippet := reply
	if len(snippet) > 100 {
		snippet = snippet[:100]
	}
	hbStore.mu.Lock()
	for i, st := range hbStore.tasks {
		if st.Label == t.Label {
			hbStore.tasks[i].RunCount++
			hbStore.tasks[i].LastResult = snippet
			break
		}
	}
	hbStore.mu.Unlock()
	go persistHeartbeatTasks()

	if heartbeatTGClient == nil || t.TelegramID == 0 {
		log.Printf("[HEARTBEAT] task %q: no TG client or TelegramID=0, cannot deliver", t.Label)
		return
	}

	reply = cleanResultForTelegram(reply)
	opts := &telegram.SendOptions{ParseMode: telegram.HTML}
	if t.MessageID != 0 {
		opts.ReplyID = int32(t.MessageID)
	}
	if _, err := heartbeatTGClient.SendMessage(t.TelegramID, reply, opts); err != nil {
		log.Printf("[HEARTBEAT] send error for task %q: %v", t.Label, err)
	}
}

func ListHeartbeatTasks() string {
	hbStore.mu.Lock()
	defer hbStore.mu.Unlock()
	if len(hbStore.tasks) == 0 {
		return "No scheduled tasks."
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "<b>Scheduled Tasks (%d)</b>\n\n", len(hbStore.tasks))
	for _, t := range hbStore.tasks {
		repeat := t.Repeat
		if repeat == "" {
			repeat = "once"
		}
		status := "✅"
		if !t.Enabled {
			status = "⏸"
		} else if t.RetryAt != "" {
			status = "🔄"
		}
		maxInfo := ""
		if t.MaxRuns > 0 {
			maxInfo = fmt.Sprintf(" | %d/%d runs", t.RunCount, t.MaxRuns)
		} else if t.RunCount > 0 {
			maxInfo = fmt.Sprintf(" | ran %d×", t.RunCount)
		}
		fmt.Fprintf(&sb, "%s <b>%s</b>%s\n  next: <code>%s</code> | %s\n",
			status, escapeHTML(t.Label), maxInfo, t.RunAt, repeat)
		if t.Tags != "" {
			fmt.Fprintf(&sb, "  tags: %s\n", escapeHTML(t.Tags))
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}
