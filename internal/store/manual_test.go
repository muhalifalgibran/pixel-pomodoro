package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/habit"
)

func TestSessionLengthPrefersTheHabitsOwnFocus(t *testing.T) {
	tests := []struct {
		name     string
		focus    time.Duration
		fallback time.Duration
		want     time.Duration
	}{
		{"own focus wins", 50 * time.Minute, 25 * time.Minute, 50 * time.Minute},
		{"unset falls back", 0, 25 * time.Minute, 25 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := habit.Habit{Name: "work", Focus: tt.focus}
			if got := SessionLength(h, tt.fallback); got != tt.want {
				t.Errorf("SessionLength() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestManualSessionRecordsACompletedFocus(t *testing.T) {
	h := habit.Habit{ID: "h1", Name: "work"}
	when := at(18, 9)

	tests := []struct {
		name     string
		dur      time.Duration
		wantMins int
		wantErr  string
	}{
		{"whole minutes", 50 * time.Minute, 50, ""},
		{"rounds up at the half", 90 * time.Second, 2, ""},
		{"half a minute still counts", 30 * time.Second, 1, ""},
		{"too short to round", 20 * time.Second, 0, "no minutes at all"},
		{"zero", 0, 0, "more than zero"},
		{"negative", -5 * time.Minute, 0, "more than zero"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess, err := ManualSession(h, tt.dur, when)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want it to mention %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ManualSession() error = %v", err)
			}
			if sess.Mins != tt.wantMins {
				t.Errorf("Mins = %d, want %d", sess.Mins, tt.wantMins)
			}
			if sess.Habit != "h1" || sess.Task != "work" {
				t.Errorf("session credits %q/%q, want h1/work", sess.Habit, sess.Task)
			}
			if !sess.IsWork() {
				t.Errorf("session does not count as work: %+v", sess)
			}
			if !sess.Start.Equal(when) {
				t.Errorf("Start = %v, want %v", sess.Start, when)
			}
		})
	}
}

func TestRemoveDropsOnlyThatLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.jsonl")
	s := New(path)

	for _, sess := range []Session{focus(16, 9, 25), focus(16, 10, 25)} {
		if err := s.Append(sess); err != nil {
			t.Fatal(err)
		}
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	mine := focus(16, 11, 50)
	if err := s.Append(mine); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(mine); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("log after remove:\n%s\nwant it back to:\n%s", after, before)
	}
}

// The point of rewriting rather than truncating: a session landing on top must
// not put the tick beyond reach.
func TestRemoveReachesPastALaterSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.jsonl")
	s := New(path)

	mine := focus(16, 9, 25)
	later := focus(16, 10, 50)
	for _, sess := range []Session{mine, later} {
		if err := s.Append(sess); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.Remove(mine); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	got, _, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("loaded %d sessions, want 1", len(got))
	}
	if got[0].Mins != 50 {
		t.Errorf("kept the %dm session, want the later 50m one to survive", got[0].Mins)
	}
}

func TestRemoveRefusesWhatItCannotFind(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.jsonl")
	s := New(path)

	if err := s.Remove(focus(16, 9, 25)); !errors.Is(err, ErrNotInLog) {
		t.Fatalf("Remove() on an empty log = %v, want ErrNotInLog", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("Remove created the log file")
	}

	if err := s.Append(focus(16, 9, 25)); err != nil {
		t.Fatal(err)
	}
	// Same shape, different minutes — not our line.
	if err := s.Remove(focus(16, 9, 26)); !errors.Is(err, ErrNotInLog) {
		t.Fatalf("Remove() of a session never written = %v, want ErrNotInLog", err)
	}
	got, _, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("loaded %d sessions, want the untouched one", len(got))
	}
}

// Load skips unparseable lines on purpose, so a remove that rebuilt the file
// from what Load returned would silently delete them.
func TestRemoveKeepsUnreadableLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.jsonl")
	s := New(path)

	if err := s.Append(focus(16, 9, 25)); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{ this line is not json\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	mine := focus(16, 10, 50)
	if err := s.Append(mine); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(mine); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "this line is not json") {
		t.Errorf("remove ate the unparseable line:\n%s", raw)
	}
	got, skipped, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || skipped != 1 {
		t.Errorf("loaded %d sessions and skipped %d, want 1 and 1", len(got), skipped)
	}
}

// Identical presses produce identical lines; removing the newest keeps undo in
// the order the presses happened.
func TestRemoveTakesTheLastOfIdenticalLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.jsonl")
	s := New(path)

	twin := focus(16, 9, 25)
	for i := 0; i < 3; i++ {
		if err := s.Append(twin); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Remove(twin); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	got, _, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("loaded %d sessions, want 2", len(got))
	}
}
