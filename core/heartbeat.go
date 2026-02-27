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

	hbStore.mu.Lock()
	for i, existing := range hbStore.tasks {
		if existing.Label == t.Label {
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

func StartHeartbeat(client *telegram.Client) {
	heartbeatTGClient = client
	loadHeartbeatTasks()
	go func() {
		for {
			time.Sleep(15 * time.Second)
			runHeartbeatTick()
		}
	}()
	log.Printf("[HEARTBEAT] scheduler started (%d tasks loaded)", len(hbStore.tasks))
}

func runHeartbeatTick() {
	now := time.Now()
	hbStore.mu.Lock()
	var remaining []ScheduledTask
	var toRun []ScheduledTask
	for _, t := range hbStore.tasks {
		runAt, err := time.Parse(time.RFC3339, t.RunAt)
		if err != nil {
			log.Printf("[HEARTBEAT] bad run_at for task %q: %v — dropping", t.Label, err)
			continue
		}
		if now.After(runAt) || now.Equal(runAt) {
			toRun = append(toRun, t)
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
	log.Printf("[HEARTBEAT] firing task %q (prompt: %q) → chat=%d", t.Label, t.Prompt, t.TelegramID)
	ownerID := t.OwnerID
	if ownerID == "" {
		ownerID = Cfg.OwnerID
	}

	session := NewAgentSession(GlobalRegistry, Cfg.DefaultModel)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	reply, err := session.RunStream(ctx, ownerID, t.Prompt, nil)
	if err != nil {
		log.Printf("[HEARTBEAT] task %q error: %v", t.Label, err)
		return
	}
	if reply == "" {
		log.Printf("[HEARTBEAT] task %q produced empty reply", t.Label)
		return
	}
	if heartbeatTGClient == nil || t.TelegramID == 0 {
		log.Printf("[HEARTBEAT] task %q: no TG client or TelegramID=0, cannot deliver", t.Label)
		return
	}

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
	for _, t := range hbStore.tasks {
		repeat := t.Repeat
		if repeat == "" {
			repeat = "once"
		}
		fmt.Fprintf(&sb, "• <b>%s</b> — %s\n  next: %s | repeat: %s\n", t.Label, t.Prompt, t.RunAt, repeat)
	}
	return strings.TrimRight(sb.String(), "\n")
}
