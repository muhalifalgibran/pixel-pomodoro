package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/timer"
)

// ResumeWindow is how long a saved position stays valid. Past it the file is
// ignored: coming back a week later and landing mid-phase in a session you
// have forgotten about is worse than starting clean.
const ResumeWindow = 12 * time.Hour

// Resume is the in-flight timer position, written when you quit and read when
// you start.
//
// This is deliberately separate from sessions.jsonl. That log is an
// append-only record of finished work and the single source of XP, level and
// streak; this file is throwaway state for one unfinished phase, and deleting
// it costs nothing but your place.
type Resume struct {
	Phase      string `json:"phase"`
	RemainingS int    `json:"remaining_seconds"`
	Running    bool   `json:"running"`
	CycleIndex int    `json:"cycle_index"`
	Completed  int    `json:"completed"`
	Task       string `json:"task,omitempty"`
	// Habit is the stable ID of the habit that was active, so relaunching
	// resumes the same one rather than dropping back to nothing.
	Habit string `json:"habit,omitempty"`
	// Zen records the open-ended stopwatch, which does not go through the
	// timer at all and so cannot be described by the snapshot above.
	Zen         bool      `json:"zen,omitempty"`
	ZenElapsedS int       `json:"zen_elapsed_seconds,omitempty"`
	ZenStart    time.Time `json:"zen_start,omitempty"`
	// PhaseStart is when the current phase began, so a session finished after
	// resuming is logged with its real start time.
	PhaseStart time.Time `json:"phase_start"`
	SavedAt    time.Time `json:"saved_at"`
}

// ResumePath is the state file, alongside the session log.
func (s *Store) ResumePath() string {
	return filepath.Join(filepath.Dir(s.path), "state.json")
}

// SaveResume writes the current position, replacing any previous one.
//
// The write goes to a temporary file and is then renamed, which is atomic on
// the same filesystem. A half-written state file would otherwise be
// indistinguishable from a valid one and could restore a nonsense position.
func (s *Store) SaveResume(r Resume) error {
	path := s.ResumePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("encode resume state: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), ".state-*.json")
	if err != nil {
		return fmt.Errorf("create temporary state file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write resume state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close resume state: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace resume state: %w", err)
	}
	return nil
}

// LoadResume reads the saved position. It reports false when there is nothing
// usable — no file, unreadable JSON, or a position older than ResumeWindow.
// None of those are errors: they all just mean "start fresh".
func (s *Store) LoadResume(now time.Time) (Resume, bool) {
	data, err := os.ReadFile(s.ResumePath())
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			// An unreadable state file should not stop the timer starting.
			return Resume{}, false
		}
		return Resume{}, false
	}

	var r Resume
	if err := json.Unmarshal(data, &r); err != nil {
		return Resume{}, false
	}
	if r.RemainingS <= 0 {
		return Resume{}, false
	}
	if _, ok := timer.ParsePhase(r.Phase); !ok {
		return Resume{}, false
	}
	// A clock that moved backwards, or a file from the future, is not
	// trustworthy either.
	age := now.Sub(r.SavedAt)
	if age < 0 || age > ResumeWindow {
		return Resume{}, false
	}
	return r, true
}

// ClearResume removes the saved position.
func (s *Store) ClearResume() error {
	err := os.Remove(s.ResumePath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove resume state: %w", err)
	}
	return nil
}

// Snapshot converts a Resume into the timer's own type.
func (r Resume) Snapshot() (timer.Snapshot, bool) {
	phase, ok := timer.ParsePhase(r.Phase)
	if !ok {
		return timer.Snapshot{}, false
	}
	return timer.Snapshot{
		Phase:      phase,
		Remaining:  time.Duration(r.RemainingS) * time.Second,
		Running:    r.Running,
		CycleIndex: r.CycleIndex,
		Completed:  r.Completed,
		Task:       r.Task,
	}, true
}

// NewResume builds the persisted form of a snapshot.
func NewResume(snap timer.Snapshot, habitID string, phaseStart, now time.Time) Resume {
	return Resume{
		Habit:      habitID,
		Phase:      snap.Phase.String(),
		RemainingS: int(snap.Remaining.Round(time.Second) / time.Second),
		Running:    snap.Running,
		CycleIndex: snap.CycleIndex,
		Completed:  snap.Completed,
		Task:       snap.Task,
		PhaseStart: phaseStart,
		SavedAt:    now,
	}
}
