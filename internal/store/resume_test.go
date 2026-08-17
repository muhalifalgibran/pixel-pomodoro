package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/timer"
)

func resumeStore(t *testing.T) *Store {
	t.Helper()
	return New(filepath.Join(t.TempDir(), "sessions.jsonl"))
}

func sampleSnapshot() timer.Snapshot {
	return timer.Snapshot{
		Phase:      timer.Focus,
		Remaining:  12*time.Minute + 34*time.Second,
		Running:    true,
		CycleIndex: 2,
		Completed:  6,
		Task:       "render loop",
	}
}

func TestResumeRoundTrip(t *testing.T) {
	s := resumeStore(t)
	now := time.Date(2026, 8, 17, 15, 0, 0, 0, time.UTC)
	start := now.Add(-13 * time.Minute)

	if err := s.SaveResume(NewResume(sampleSnapshot(), "", start, now)); err != nil {
		t.Fatalf("SaveResume() error = %v", err)
	}

	got, ok := s.LoadResume(now.Add(time.Minute))
	if !ok {
		t.Fatal("LoadResume() reported nothing to resume")
	}
	snap, ok := got.Snapshot()
	if !ok {
		t.Fatal("Snapshot() rejected a round-tripped phase")
	}

	want := sampleSnapshot()
	if snap.Phase != want.Phase || snap.Remaining != want.Remaining ||
		snap.Running != want.Running || snap.CycleIndex != want.CycleIndex ||
		snap.Completed != want.Completed || snap.Task != want.Task {
		t.Errorf("round trip = %+v, want %+v", snap, want)
	}
	if !got.PhaseStart.Equal(start) {
		t.Errorf("PhaseStart = %v, want %v", got.PhaseStart, start)
	}
}

func TestResumeStoredNextToTheSessionLog(t *testing.T) {
	s := resumeStore(t)
	if got, want := filepath.Dir(s.ResumePath()), filepath.Dir(s.Path()); got != want {
		t.Errorf("resume state lives in %s, want %s", got, want)
	}
	if filepath.Base(s.ResumePath()) == filepath.Base(s.Path()) {
		t.Error("resume state and the session log share a filename")
	}
}

func TestLoadResumeMissingFile(t *testing.T) {
	if _, ok := resumeStore(t).LoadResume(time.Now()); ok {
		t.Error("LoadResume() found something in an empty data directory")
	}
}

func TestLoadResumeIgnoresStaleState(t *testing.T) {
	s := resumeStore(t)
	saved := time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)
	if err := s.SaveResume(NewResume(sampleSnapshot(), "", saved, saved)); err != nil {
		t.Fatalf("SaveResume() error = %v", err)
	}

	// Just inside the window.
	if _, ok := s.LoadResume(saved.Add(ResumeWindow - time.Minute)); !ok {
		t.Error("state inside the window was discarded")
	}
	// Just outside it.
	if _, ok := s.LoadResume(saved.Add(ResumeWindow + time.Minute)); ok {
		t.Error("state older than the window was resumed")
	}
}

// A file dated in the future means the clock moved; trusting it could restore
// a position that never existed.
func TestLoadResumeIgnoresFutureTimestamps(t *testing.T) {
	s := resumeStore(t)
	now := time.Date(2026, 8, 17, 15, 0, 0, 0, time.UTC)
	if err := s.SaveResume(NewResume(sampleSnapshot(), "", now, now.Add(time.Hour))); err != nil {
		t.Fatalf("SaveResume() error = %v", err)
	}
	if _, ok := s.LoadResume(now); ok {
		t.Error("state saved in the future was resumed")
	}
}

func TestLoadResumeRejectsUnusableFiles(t *testing.T) {
	now := time.Date(2026, 8, 17, 15, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		body string
	}{
		{"not json", "{{{"},
		{"empty", ""},
		{"unknown phase", `{"phase":"siesta","remaining_seconds":60,"saved_at":"2026-08-17T14:59:00Z"}`},
		{"zero remaining", `{"phase":"focus","remaining_seconds":0,"saved_at":"2026-08-17T14:59:00Z"}`},
		{"negative remaining", `{"phase":"focus","remaining_seconds":-5,"saved_at":"2026-08-17T14:59:00Z"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := resumeStore(t)
			if err := os.MkdirAll(filepath.Dir(s.ResumePath()), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(s.ResumePath(), []byte(tt.body), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			if _, ok := s.LoadResume(now); ok {
				t.Errorf("LoadResume() accepted %s state", tt.name)
			}
		})
	}
}

func TestClearResume(t *testing.T) {
	s := resumeStore(t)
	now := time.Now()
	if err := s.SaveResume(NewResume(sampleSnapshot(), "", now, now)); err != nil {
		t.Fatalf("SaveResume() error = %v", err)
	}
	if err := s.ClearResume(); err != nil {
		t.Fatalf("ClearResume() error = %v", err)
	}
	if _, ok := s.LoadResume(now); ok {
		t.Error("state survived ClearResume()")
	}
	// Clearing again is not an error.
	if err := s.ClearResume(); err != nil {
		t.Errorf("second ClearResume() error = %v, want nil", err)
	}
}

// The save must be atomic: a reader should never observe a partial file.
func TestSaveResumeLeavesNoTemporaryFiles(t *testing.T) {
	s := resumeStore(t)
	now := time.Now()
	for i := 0; i < 3; i++ {
		if err := s.SaveResume(NewResume(sampleSnapshot(), "", now, now)); err != nil {
			t.Fatalf("SaveResume() error = %v", err)
		}
	}

	entries, err := os.ReadDir(filepath.Dir(s.ResumePath()))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("data directory holds %v, want just the state file", names)
	}
}

func TestResumeSurvivesRepeatedSaves(t *testing.T) {
	s := resumeStore(t)
	now := time.Now()

	first := sampleSnapshot()
	if err := s.SaveResume(NewResume(first, "", now, now)); err != nil {
		t.Fatalf("SaveResume() error = %v", err)
	}
	second := first
	second.Remaining = 90 * time.Second
	second.Phase = timer.ShortBreak
	if err := s.SaveResume(NewResume(second, "", now, now)); err != nil {
		t.Fatalf("SaveResume() error = %v", err)
	}

	got, ok := s.LoadResume(now)
	if !ok {
		t.Fatal("LoadResume() found nothing")
	}
	snap, _ := got.Snapshot()
	if snap.Phase != timer.ShortBreak || snap.Remaining != 90*time.Second {
		t.Errorf("loaded %+v, want the second save", snap)
	}
}
