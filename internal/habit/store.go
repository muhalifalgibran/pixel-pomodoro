package habit

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/paths"
)

// Store persists the habit list.
//
// The file lives in the config directory rather than beside sessions.jsonl on
// purpose: habit definitions are intent, sessions are history. Clearing your
// history should not wipe your habit list.
//
// It is JSON rather than TOML because the program rewrites this file. Rewriting
// a hand-authored config.toml would destroy the user's comments, so config.toml
// stays hand-written and pomo never touches it.
type Store struct{ path string }

// NewStore builds a store over the given file.
func NewStore(path string) *Store { return &Store{path: path} }

// Path is the file's location, for display.
func (s *Store) Path() string { return s.path }

// DefaultPath is habits.json in pomo's config directory.
func DefaultPath() (string, error) { return paths.ConfigFile("habits.json") }

// Load reads the habit list. A missing file yields an empty list rather than an
// error: running with no habits at all is the first-launch case and must work.
//
// A file that exists but cannot be parsed *is* an error. Silently starting with
// an empty list would present the user with a working program that has quietly
// lost every habit they defined, and they might then save over the file that
// still held them.
func (s *Store) Load() (List, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return List{Version: CurrentVersion}, nil
	}
	if err != nil {
		return List{}, fmt.Errorf("read %s: %w", s.path, err)
	}

	var l List
	if err := json.Unmarshal(data, &l); err != nil {
		return List{}, fmt.Errorf("parse %s: %w\nfix or move the file; pomo will not overwrite it", s.path, err)
	}
	if l.Version == 0 {
		l.Version = CurrentVersion
	}
	// Guard against a hand-edited file with duplicate or missing IDs, which
	// would make habits unaddressable.
	seen := map[string]bool{}
	for i := range l.Habits {
		if l.Habits[i].ID == "" || seen[l.Habits[i].ID] {
			l.Habits[i].ID = l.NextID(l.Habits[i].Name)
		}
		seen[l.Habits[i].ID] = true
	}
	return l, nil
}

// Save writes the habit list.
//
// The write goes to a temporary file in the same directory and is then renamed,
// which is atomic on one filesystem. Writing in place would leave a truncated
// habits.json if the process died mid-write, and a truncated file is
// indistinguishable from a corrupt one.
func (s *Store) Save(l List) error {
	if l.Version == 0 {
		l.Version = CurrentVersion
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return fmt.Errorf("encode habits: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".habits-*.json")
	if err != nil {
		return fmt.Errorf("create temporary habits file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write habits: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close habits: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("replace %s: %w", s.path, err)
	}
	return nil
}
