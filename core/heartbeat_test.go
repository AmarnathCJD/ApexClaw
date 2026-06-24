package core

import (
	"strings"
	"testing"
	"time"
)

func TestCalcNextRun_Minutely(t *testing.T) {
	now := time.Now()
	runAt := now.Add(-30 * time.Second)
	next := calcNextRun(runAt, now, "minutely")
	if !next.After(now) {
		t.Fatalf("next should be in the future, got %v", next)
	}
	delta := next.Sub(now)
	if delta < 0 || delta > time.Minute {
		t.Errorf("delta should be <= 1m, got %v", delta)
	}
}

func TestCalcNextRun_Monthly(t *testing.T) {
	now := time.Now()
	runAt := now.Add(-time.Hour)
	next := calcNextRun(runAt, now, "monthly")
	if !next.After(now) {
		t.Fatalf("next must be in the future, got %v", next)
	}
	if next.Sub(now) > 32*24*time.Hour || next.Sub(now) < 27*24*time.Hour {
		t.Errorf("monthly next should be ~1 month away, got delta %v", next.Sub(now))
	}
}

func TestCalcNextRun_EveryNUnits(t *testing.T) {
	now := time.Now()
	runAt := now.Add(-time.Hour)
	for _, tc := range []struct {
		repeat string
		want   time.Duration
	}{
		{"every_5_minutes", 5 * time.Minute},
		{"every_3_hours", 3 * time.Hour},
		{"every_2_days", 2 * 24 * time.Hour},
		{"every_1_weeks", 7 * 24 * time.Hour},
	} {
		next := calcNextRun(runAt, now, tc.repeat)
		if !next.After(now) {
			t.Errorf("%s: next should be in the future, got %v", tc.repeat, next)
			continue
		}
		delta := next.Sub(runAt)
		// Should be a multiple of `tc.want`. We allow up to 2x because the
		// loop in calcNextRun keeps adding until next > now.
		if delta < tc.want {
			t.Errorf("%s: delta %v should be >= %v", tc.repeat, delta, tc.want)
		}
	}
}

func TestCalcNextRunFor_Cron(t *testing.T) {
	task := ScheduledTask{
		Label:    "test",
		CronExpr: "*/15 * * * *",
	}
	now := time.Date(2026, 6, 23, 14, 7, 0, 0, time.UTC)
	next := calcNextRunFor(task, now, now)
	if !next.After(now) {
		t.Fatalf("cron next should advance, got %v", next)
	}
	if next.Minute()%15 != 0 {
		t.Errorf("cron */15 should land on a 15-min mark, got minute=%d", next.Minute())
	}
}

func TestCalcNextRunFor_CronInvalidFallsBackToRepeat(t *testing.T) {
	task := ScheduledTask{
		Label:    "test",
		CronExpr: "this is not cron",
		Repeat:   "hourly",
	}
	now := time.Now()
	runAt := now.Add(-time.Hour)
	next := calcNextRunFor(task, runAt, now)
	if !next.After(now) {
		t.Fatalf("invalid cron should fall back to repeat=hourly; got %v", next)
	}
}

func TestValidateCronExpr(t *testing.T) {
	good := []string{"*/15 * * * *", "0 9 * * 1-5", "0 0 1 * *", "@daily"}
	for _, g := range good {
		if err := validateCronExpr(g); err != nil {
			t.Errorf("expected %q to be valid, got %v", g, err)
		}
	}
	bad := []string{"not a cron", "* * *", "99 99 99 99 99"}
	for _, b := range bad {
		if err := validateCronExpr(b); err == nil {
			t.Errorf("expected %q to be invalid", b)
		}
	}
	if err := validateCronExpr(""); err != nil {
		t.Errorf("empty cron should be allowed, got %v", err)
	}
}

func TestHumanizeUntil(t *testing.T) {
	now := time.Date(2026, 6, 23, 14, 0, 0, 0, time.UTC)
	cases := []struct {
		offset time.Duration
		want   string
	}{
		{30 * time.Second, "in 30s"},
		{5 * time.Minute, "in 5m"},
		{2 * time.Hour, "in 2h"},
		{2*time.Hour + 30*time.Minute, "in 2h 30m"},
		{0, "now"},
	}
	for _, tc := range cases {
		got := humanizeUntil(now.Add(tc.offset), now)
		if got != tc.want {
			t.Errorf("offset %v: got %q want %q", tc.offset, got, tc.want)
		}
	}
	// "tomorrow at" path.
	got := humanizeUntil(now.Add(28*time.Hour), now)
	if !strings.HasPrefix(got, "tomorrow at ") {
		t.Errorf("28h offset should start with 'tomorrow at': got %q", got)
	}
}

func TestScheduleTask_RequiresLabelAndPrompt(t *testing.T) {
	resetHbStore()
	err := ScheduleTask(ScheduledTask{Prompt: "x", RunAt: time.Now().Format(time.RFC3339)})
	if err == nil || !strings.Contains(err.Error(), "label") {
		t.Errorf("missing label should error mentioning 'label', got %v", err)
	}
	err = ScheduleTask(ScheduledTask{Label: "x", RunAt: time.Now().Format(time.RFC3339)})
	if err == nil || !strings.Contains(err.Error(), "prompt") {
		t.Errorf("missing prompt should error mentioning 'prompt', got %v", err)
	}
}

func TestScheduleTask_RejectsBadRunAt(t *testing.T) {
	resetHbStore()
	err := ScheduleTask(ScheduledTask{Label: "x", Prompt: "y", RunAt: "tomorrow 9am"})
	if err == nil || !strings.Contains(err.Error(), "RFC3339") {
		t.Errorf("bad run_at should error mentioning RFC3339, got %v", err)
	}
}

func TestScheduleTask_RejectsBadCron(t *testing.T) {
	resetHbStore()
	err := ScheduleTask(ScheduledTask{Label: "x", Prompt: "y", CronExpr: "garbage"})
	if err == nil || !strings.Contains(err.Error(), "cron") {
		t.Errorf("bad cron should error mentioning cron, got %v", err)
	}
}

func TestScheduleTask_CronWithoutRunAtComputes(t *testing.T) {
	resetHbStore()
	err := ScheduleTask(ScheduledTask{
		Label:    "every15",
		Prompt:   "ping",
		CronExpr: "*/15 * * * *",
	})
	if err != nil {
		t.Fatalf("valid cron schedule should succeed: %v", err)
	}
	hbStore.mu.Lock()
	defer hbStore.mu.Unlock()
	if len(hbStore.tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(hbStore.tasks))
	}
	if hbStore.tasks[0].RunAt == "" {
		t.Error("run_at should have been auto-computed from cron expression")
	}
}

func TestCancelTasksByTag(t *testing.T) {
	resetHbStore()
	for _, label := range []string{"a", "b", "c"} {
		_ = ScheduleTask(ScheduledTask{
			Label:  label,
			Prompt: "x",
			RunAt:  time.Now().Add(time.Hour).Format(time.RFC3339),
			Tags:   "fleet,urgent",
		})
	}
	_ = ScheduleTask(ScheduledTask{
		Label:  "lonely",
		Prompt: "x",
		RunAt:  time.Now().Add(time.Hour).Format(time.RFC3339),
		Tags:   "personal",
	})
	n := CancelTasksByTag("fleet")
	if n != 3 {
		t.Errorf("expected 3 tasks cancelled by tag, got %d", n)
	}
	hbStore.mu.Lock()
	defer hbStore.mu.Unlock()
	if len(hbStore.tasks) != 1 || hbStore.tasks[0].Label != "lonely" {
		t.Errorf("expected only 'lonely' to remain, got %d tasks", len(hbStore.tasks))
	}
}

func TestDryRunSchedule(t *testing.T) {
	task := ScheduledTask{
		Label:    "x",
		Prompt:   "y",
		CronExpr: "0 9 * * *",
		RunAt:    time.Now().Format(time.RFC3339),
	}
	out, err := DryRunSchedule(task, 3)
	if err != nil {
		t.Fatalf("dry-run should succeed: %v", err)
	}
	// Should list 3 future fires.
	count := strings.Count(out, "\n")
	if count < 3 {
		t.Errorf("dry-run should list >= 3 firings, got\n%s", out)
	}
}

func resetHbStore() {
	hbStore.mu.Lock()
	hbStore.tasks = nil
	hbStore.mu.Unlock()
}
