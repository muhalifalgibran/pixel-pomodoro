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

// markAt appends sess and hands back the offset it started at, the way the
// checklist does before it lets you undo.
func markAt(t *testing.T, s *Store, sess Session) int64 {
	t.Helper()
	at, err := s.Size()
	if err != nil {
		t.Fatalf("Size() error = %v", err)
	}
	if err := s.Append(sess); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	return at
}

func TestRemoveLastDropsOnlyTheFinalLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.jsonl")
	s := New(path)

	markAt(t, s, focus(16, 9, 25))
	markAt(t, s, focus(16, 10, 25))
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	last := focus(16, 11, 50)
	at := markAt(t, s, last)
	if err := s.RemoveLast(at, last); err != nil {
		t.Fatalf("RemoveLast() error = %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("log after undo:\n%s\nwant it back to:\n%s", after, before)
	}
	got, _, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got) != 2 {
		t.Errorf("loaded %d sessions, want 2", len(got))
	}
}

func TestRemoveLastRefusesWhenTheLogMovedOn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.jsonl")
	s := New(path)

	mine := focus(16, 9, 25)
	at := markAt(t, s, mine)
	// Another pomo, or `pomo -log`, lands in between.
	if err := s.Append(focus(16, 10, 25)); err != nil {
		t.Fatal(err)
	}

	if err := s.RemoveLast(at, mine); !errors.Is(err, ErrLogMoved) {
		t.Fatalf("RemoveLast() error = %v, want ErrLogMoved", err)
	}
	got, _, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got) != 2 {
		t.Errorf("loaded %d sessions, want both left alone", len(got))
	}
}

// Load skips unparseable lines on purpose, so an undo that rebuilt the file
// from what Load returned would silently delete them. Truncating cannot.
func TestRemoveLastKeepsUnreadableLines(t *testing.T) {
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
	at := markAt(t, s, mine)
	if err := s.RemoveLast(at, mine); err != nil {
		t.Fatalf("RemoveLast() error = %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "this line is not json") {
		t.Errorf("undo ate the unparseable line:\n%s", raw)
	}
	got, skipped, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got) != 1 || skipped != 1 {
		t.Errorf("loaded %d sessions and skipped %d, want 1 and 1", len(got), skipped)
	}
}

func TestRemoveLastOnAnEmptyLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.jsonl")
	s := New(path)

	if err := s.RemoveLast(0, focus(16, 9, 25)); !errors.Is(err, ErrLogMoved) {
		t.Fatalf("RemoveLast() error = %v, want ErrLogMoved", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("undo created the log file")
	}
}
