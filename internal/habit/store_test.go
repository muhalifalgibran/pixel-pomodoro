package habit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(filepath.Join(t.TempDir(), "habits.json"))
}

func sampleList(t *testing.T) List {
	t.Helper()
	var l List
	if _, err := l.Add(Habit{
		Name:  "work",
		Goal:  Goal{Target: 240, Unit: Minutes, Period: Daily},
		Color: "#ff7043",
		Focus: 50 * time.Minute,
		Short: 10 * time.Minute,
	}, now()); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Add(Habit{
		Name: "reading",
		Goal: Goal{Target: 1, Unit: Sessions, Period: Daily},
	}, now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Add(Habit{
		Name: "gym",
		Goal: Goal{Target: 3, Unit: Sessions, Period: Weekly},
	}, now().Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	return l
}

func TestSaveLoadRoundTrip(t *testing.T) {
	s := testStore(t)
	want := sampleList(t)

	if err := s.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got.Version != CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, CurrentVersion)
	}
	if len(got.Habits) != len(want.Habits) {
		t.Fatalf("loaded %d habits, want %d", len(got.Habits), len(want.Habits))
	}
	for i := range want.Habits {
		w, g := want.Habits[i], got.Habits[i]
		if g.ID != w.ID || g.Name != w.Name || g.Goal != w.Goal || g.Color != w.Color {
			t.Errorf("habit %d = %+v, want %+v", i, g, w)
		}
		if g.Focus != w.Focus || g.Short != w.Short || g.Long != w.Long {
			t.Errorf("habit %d durations = %v/%v/%v, want %v/%v/%v",
				i, g.Focus, g.Short, g.Long, w.Focus, w.Short, w.Long)
		}
		if !g.Created.Equal(w.Created) {
			t.Errorf("habit %d Created = %v, want %v", i, g.Created, w.Created)
		}
	}
}

func TestLoadMissingFileIsEmptyNotAnError(t *testing.T) {
	got, err := testStore(t).Load()
	if err != nil {
		t.Fatalf("Load() on a missing file error = %v, want nil", err)
	}
	if len(got.Habits) != 0 {
		t.Errorf("loaded %d habits from nothing", len(got.Habits))
	}
	if got.Version != CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, CurrentVersion)
	}
}

// A corrupt file must be an error, not an empty list. Starting up as though the
// user had no habits would hand them a working program that has quietly lost
// everything, and they might then save over the file that still held it.
func TestLoadRefusesToSilentlyLoseACorruptFile(t *testing.T) {
	s := testStore(t)
	if err := os.MkdirAll(filepath.Dir(s.Path()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.Path(), []byte("{{{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := s.Load()
	if err == nil {
		t.Fatalf("Load() = %+v, want an error for a corrupt file", got)
	}
	if !strings.Contains(err.Error(), "will not overwrite") {
		t.Errorf("error = %v, want it to reassure the user the file is intact", err)
	}
}

func TestSaveIsAtomicAndLeavesNoTemporaryFiles(t *testing.T) {
	s := testStore(t)
	l := sampleList(t)

	for i := 0; i < 3; i++ {
		if err := s.Save(l); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
	}

	entries, err := os.ReadDir(filepath.Dir(s.Path()))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("directory holds %v, want only habits.json", names)
	}
}

func TestSaveCreatesTheDirectory(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "nested", "deeper", "habits.json"))
	if err := s.Save(sampleList(t)); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if _, err := os.Stat(s.Path()); err != nil {
		t.Errorf("habits.json was not created: %v", err)
	}
}

// A hand-edited file can carry duplicate or missing IDs, which would make
// habits unaddressable. Load repairs them rather than failing.
func TestLoadRepairsDuplicateAndMissingIDs(t *testing.T) {
	s := testStore(t)
	if err := os.MkdirAll(filepath.Dir(s.Path()), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{
	  "version": 1,
	  "habits": [
	    {"id": "work", "name": "work",    "goal": {"target": 1, "unit": 0, "period": 0}},
	    {"id": "work", "name": "wörk",    "goal": {"target": 1, "unit": 0, "period": 0}},
	    {"id": "",     "name": "reading", "goal": {"target": 1, "unit": 0, "period": 0}}
	  ]
	}`
	if err := os.WriteFile(s.Path(), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got.Habits) != 3 {
		t.Fatalf("loaded %d habits, want 3", len(got.Habits))
	}
	seen := map[string]bool{}
	for _, h := range got.Habits {
		if h.ID == "" {
			t.Errorf("habit %q still has no ID", h.Name)
		}
		if seen[h.ID] {
			t.Errorf("ID %q is still duplicated", h.ID)
		}
		seen[h.ID] = true
	}
}

func TestLoadFillsInAMissingVersion(t *testing.T) {
	s := testStore(t)
	if err := os.MkdirAll(filepath.Dir(s.Path()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.Path(), []byte(`{"habits":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Version != CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, CurrentVersion)
	}
}

// The file is meant to be readable and editable by hand, so keep it indented
// rather than one long line.
func TestSavedFileIsHumanReadable(t *testing.T) {
	s := testStore(t)
	if err := s.Save(sampleList(t)); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "\n  ") {
		t.Error("habits.json is not indented")
	}
	if !strings.HasSuffix(string(body), "\n") {
		t.Error("habits.json has no trailing newline")
	}
}
