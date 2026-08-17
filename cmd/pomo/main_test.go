package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/config"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/habit"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/store"
)

func logFixture(t *testing.T) (config.Config, *store.Store, *habit.Store) {
	t.Helper()
	dir := t.TempDir()

	hs := habit.NewStore(filepath.Join(dir, "habits.json"))
	var l habit.List
	work := habit.Habit{
		Name:  "work",
		Goal:  habit.Goal{Target: 240, Unit: habit.Minutes, Period: habit.Daily},
		Focus: 50 * time.Minute,
	}
	if _, err := l.Add(work, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Add(habit.Habit{
		Name: "reading time",
		Goal: habit.Goal{Target: 1, Unit: habit.Sessions, Period: habit.Daily},
	}, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := hs.Save(l); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	return cfg, store.New(filepath.Join(dir, "sessions.jsonl")), hs
}

func now() time.Time { return time.Date(2026, 8, 17, 15, 0, 0, 0, time.UTC) }

func TestLogSessionWithADuration(t *testing.T) {
	cfg, st, hs := logFixture(t)
	var out bytes.Buffer

	if err := logSession(&out, cfg, st, hs, []string{"work", "90m"}, "", now()); err != nil {
		t.Fatalf("logSession() error = %v", err)
	}

	sessions, _, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("logged %d sessions, want 1", len(sessions))
	}
	got := sessions[0]
	if got.Habit != "work" || got.Mins != 90 || !got.Done || got.Phase != store.PhaseFocus {
		t.Errorf("logged %+v, want a completed 90 minute focus against work", got)
	}
	if !strings.Contains(out.String(), "1h 30m") {
		t.Errorf("output does not confirm the amount:\n%s", out.String())
	}
}

// Omitting the duration logs one session at the habit's own focus length, which
// is what a session-count goal needs.
func TestLogSessionWithoutADurationUsesTheHabitsFocusLength(t *testing.T) {
	cfg, st, hs := logFixture(t)
	var out bytes.Buffer

	if err := logSession(&out, cfg, st, hs, []string{"work"}, "", now()); err != nil {
		t.Fatalf("logSession() error = %v", err)
	}

	sessions, _, _ := st.Load()
	if got := sessions[0].Mins; got != 50 {
		t.Errorf("mins = %d, want the habit's 50m focus length", got)
	}
}

func TestLogSessionFallsBackToTheGlobalFocusLength(t *testing.T) {
	cfg, st, hs := logFixture(t)
	var out bytes.Buffer

	// "reading time" sets no override.
	if err := logSession(&out, cfg, st, hs, []string{"reading time"}, "", now()); err != nil {
		t.Fatalf("logSession() error = %v", err)
	}

	sessions, _, _ := st.Load()
	want := int(cfg.Focus / time.Minute)
	if got := sessions[0].Mins; got != want {
		t.Errorf("mins = %d, want the global default %d", got, want)
	}
}

func TestLogSessionIsCaseInsensitive(t *testing.T) {
	cfg, st, hs := logFixture(t)
	var out bytes.Buffer

	if err := logSession(&out, cfg, st, hs, []string{"WORK", "10m"}, "", now()); err != nil {
		t.Fatalf("logSession() error = %v", err)
	}
	sessions, _, _ := st.Load()
	if len(sessions) != 1 {
		t.Error("a differently cased habit name was not matched")
	}
}

func TestLogSessionBackdates(t *testing.T) {
	cfg, st, hs := logFixture(t)
	var out bytes.Buffer

	if err := logSession(&out, cfg, st, hs, []string{"work", "4h"}, "2026-08-14", now()); err != nil {
		t.Fatalf("logSession() error = %v", err)
	}

	sessions, _, _ := st.Load()
	if got := sessions[0].Start.Day(); got != 14 {
		t.Errorf("logged on the %d, want the 14th", got)
	}
	// Anchored at noon so the entry cannot slide into a neighbouring day.
	if got := sessions[0].Start.Hour(); got != 12 {
		t.Errorf("logged at %d:00, want noon", got)
	}
}

func TestLogSessionErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		date    string
		wantSub string
	}{
		{"no arguments", nil, "", "usage"},
		{"unknown habit", []string{"nope"}, "", "no habit named"},
		{"bad duration", []string{"work", "banana"}, "", "not a duration"},
		{"zero duration", []string{"work", "0s"}, "", "more than zero"},
		// A negative duration parses fine, so it is the positivity check that
		// catches it, not the parser.
		{"negative duration", []string{"work", "-5m"}, "", "more than zero"},
		{"too many arguments", []string{"work", "10m", "extra"}, "", "unexpected argument"},
		{"bad date", []string{"work", "10m"}, "yesterday", "not a date"},
		{"sub-minute duration", []string{"work", "20s"}, "", "no minutes at all"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, st, hs := logFixture(t)
			var out bytes.Buffer

			err := logSession(&out, cfg, st, hs, tt.args, tt.date, now())
			if err == nil {
				t.Fatalf("logSession() succeeded, want an error containing %q", tt.wantSub)
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantSub)
			}
			// Nothing should have been written on a failure.
			if sessions, _, _ := st.Load(); len(sessions) != 0 {
				t.Errorf("a failed -log still wrote %d session(s)", len(sessions))
			}
		})
	}
}

// An unknown name should say what would have worked rather than leaving the
// user to guess.
func TestLogSessionListsKnownHabitsOnAMiss(t *testing.T) {
	cfg, st, hs := logFixture(t)
	var out bytes.Buffer

	err := logSession(&out, cfg, st, hs, []string{"nope"}, "", now())
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"work", "reading time"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err, want)
		}
	}
}

func TestResolveVersionFallsBackToDev(t *testing.T) {
	// The test binary carries no ldflags and no release build info.
	if got := resolveVersion(); got == "" {
		t.Error("resolveVersion() returned an empty string")
	}
}
