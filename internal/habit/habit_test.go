package habit

import (
	"strings"
	"testing"
	"time"
)

func TestGoalStringRoundTrip(t *testing.T) {
	goals := []Goal{
		{Target: 1, Unit: Sessions, Period: Daily},
		{Target: 3, Unit: Sessions, Period: Daily},
		{Target: 240, Unit: Minutes, Period: Daily},
		{Target: 90, Unit: Minutes, Period: Daily},
		{Target: 45, Unit: Minutes, Period: Daily},
		{Target: 5, Unit: Sessions, Period: Weekly},
		{Target: 600, Unit: Minutes, Period: Weekly},
	}
	for _, want := range goals {
		text := want.String()
		got, err := ParseGoal(text)
		if err != nil {
			t.Errorf("ParseGoal(%q) error = %v", text, err)
			continue
		}
		if got != want {
			t.Errorf("ParseGoal(%q) = %+v, want %+v", text, got, want)
		}
	}
}

func TestGoalStringPluralises(t *testing.T) {
	if got := (Goal{Target: 1, Unit: Sessions}).String(); got != "1 session" {
		t.Errorf("got %q, want %q", got, "1 session")
	}
	if got := (Goal{Target: 2, Unit: Sessions}).String(); got != "2 sessions" {
		t.Errorf("got %q, want %q", got, "2 sessions")
	}
}

func TestParseGoal(t *testing.T) {
	tests := []struct {
		in   string
		want Goal
	}{
		{"1 session", Goal{Target: 1, Unit: Sessions, Period: Daily}},
		{"3 sessions", Goal{Target: 3, Unit: Sessions, Period: Daily}},
		{"  2   SESSIONS  ", Goal{Target: 2, Unit: Sessions, Period: Daily}},
		{"4h", Goal{Target: 240, Unit: Minutes, Period: Daily}},
		{"90m", Goal{Target: 90, Unit: Minutes, Period: Daily}},
		{"1h30m", Goal{Target: 90, Unit: Minutes, Period: Daily}},
		{"1h 30m", Goal{Target: 90, Unit: Minutes, Period: Daily}},
		// A bare number reads as sessions: "3" means three sessions far more
		// often than it means three minutes.
		{"3", Goal{Target: 3, Unit: Sessions, Period: Daily}},
		{"5 sessions / week", Goal{Target: 5, Unit: Sessions, Period: Weekly}},
		{"5 sessions /week", Goal{Target: 5, Unit: Sessions, Period: Weekly}},
		{"5 sessions per week", Goal{Target: 5, Unit: Sessions, Period: Weekly}},
		{"3 a week", Goal{Target: 3, Unit: Sessions, Period: Weekly}},
		{"10h / week", Goal{Target: 600, Unit: Minutes, Period: Weekly}},
	}
	for _, tt := range tests {
		got, err := ParseGoal(tt.in)
		if err != nil {
			t.Errorf("ParseGoal(%q) error = %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseGoal(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestParseGoalRejectsNonsense(t *testing.T) {
	for _, in := range []string{"", "   ", "banana", "some sessions", "/ week", "4 hours of work"} {
		if got, err := ParseGoal(in); err == nil {
			t.Errorf("ParseGoal(%q) = %+v, want an error", in, got)
		}
	}
}

func TestFormatMinutes(t *testing.T) {
	tests := []struct {
		m    int
		want string
	}{
		{0, "0m"},
		{45, "45m"},
		{60, "1h"},
		{90, "1h 30m"},
		{240, "4h"},
		{-5, "0m"},
	}
	for _, tt := range tests {
		if got := FormatMinutes(tt.m); got != tt.want {
			t.Errorf("FormatMinutes(%d) = %q, want %q", tt.m, got, tt.want)
		}
	}
}

func TestSlug(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"work", "work"},
		{"vibe antarta", "vibe-antarta"},
		{"Reading Time", "reading-time"},
		{"  spaced  out  ", "spaced-out"},
		{"deep/work: 2024", "deep-work-2024"},
		{"!!!", "habit"},
		{"", "habit"},
	}
	for _, tt := range tests {
		if got := Slug(tt.in); got != tt.want {
			t.Errorf("Slug(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSlugIsStable(t *testing.T) {
	// The ID is derived once at creation, so the same name must always produce
	// the same slug.
	for i := 0; i < 5; i++ {
		if got := Slug("vibe antarta"); got != "vibe-antarta" {
			t.Fatalf("Slug is not deterministic: got %q", got)
		}
	}
}

func TestValidate(t *testing.T) {
	base := Habit{Name: "work", Goal: Goal{Target: 240, Unit: Minutes}}
	if err := base.Validate(); err != nil {
		t.Fatalf("a valid habit was rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Habit)
	}{
		{"empty name", func(h *Habit) { h.Name = "" }},
		{"whitespace name", func(h *Habit) { h.Name = "   " }},
		{"zero target", func(h *Habit) { h.Goal.Target = 0 }},
		{"negative target", func(h *Habit) { h.Goal.Target = -1 }},
		{"bad colour", func(h *Habit) { h.Color = "not-a-colour" }},
		{"short colour", func(h *Habit) { h.Color = "#fff" }},
		{"negative focus", func(h *Habit) { h.Focus = -time.Minute }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := base
			tt.mutate(&h)
			if err := h.Validate(); err == nil {
				t.Error("Validate() accepted an invalid habit")
			}
		})
	}

	// A valid colour is accepted.
	h := base
	h.Color = "#ff7043"
	if err := h.Validate(); err != nil {
		t.Errorf("a valid colour was rejected: %v", err)
	}
}

func now() time.Time { return time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC) }

func TestAddAssignsIDAndCreated(t *testing.T) {
	var l List
	h, err := l.Add(Habit{Name: "  work  ", Goal: Goal{Target: 240, Unit: Minutes}}, now())
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if h.ID != "work" {
		t.Errorf("ID = %q, want %q", h.ID, "work")
	}
	if h.Name != "work" {
		t.Errorf("Name = %q, want the name trimmed", h.Name)
	}
	if !h.Created.Equal(now()) {
		t.Errorf("Created = %v, want %v", h.Created, now())
	}
	if len(l.Habits) != 1 {
		t.Errorf("list holds %d habits, want 1", len(l.Habits))
	}
}

func TestAddRejectsInvalid(t *testing.T) {
	var l List
	if _, err := l.Add(Habit{Name: "", Goal: Goal{Target: 1}}, now()); err == nil {
		t.Error("Add() accepted a habit with no name")
	}
	if len(l.Habits) != 0 {
		t.Error("an invalid habit was still appended")
	}
}

func TestNextIDSuffixesOnCollision(t *testing.T) {
	var l List
	first, _ := l.Add(Habit{Name: "work", Goal: Goal{Target: 1}}, now())
	second, _ := l.Add(Habit{Name: "work", Goal: Goal{Target: 1}}, now())
	third, _ := l.Add(Habit{Name: "Work", Goal: Goal{Target: 1}}, now())

	if first.ID != "work" || second.ID != "work-2" || third.ID != "work-3" {
		t.Errorf("IDs = %q, %q, %q; want work, work-2, work-3", first.ID, second.ID, third.ID)
	}
}

// The point of stable IDs: a rename must not change how sessions are attributed.
func TestUpdateKeepsIDAndCreated(t *testing.T) {
	var l List
	h, _ := l.Add(Habit{Name: "work", Goal: Goal{Target: 240, Unit: Minutes}}, now())

	renamed := h
	renamed.Name = "deep work"
	renamed.Created = time.Time{} // a caller passing junk must not win
	if err := l.Update(renamed); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, ok := l.ByID("work")
	if !ok {
		t.Fatal("the habit is no longer addressable by its original ID")
	}
	if got.Name != "deep work" {
		t.Errorf("Name = %q, want the rename applied", got.Name)
	}
	if !got.Created.Equal(now()) {
		t.Errorf("Created = %v, want the original %v", got.Created, now())
	}
}

func TestUpdateRejectsUnknownAndInvalid(t *testing.T) {
	var l List
	l.Add(Habit{Name: "work", Goal: Goal{Target: 1}}, now())

	if err := l.Update(Habit{ID: "nope", Name: "x", Goal: Goal{Target: 1}}); err == nil {
		t.Error("Update() accepted an unknown ID")
	}
	if err := l.Update(Habit{ID: "work", Name: "", Goal: Goal{Target: 1}}); err == nil {
		t.Error("Update() accepted an invalid habit")
	}
}

func TestArchiveHidesFromActiveButKeepsLookup(t *testing.T) {
	var l List
	l.Add(Habit{Name: "work", Goal: Goal{Target: 1}}, now())
	l.Add(Habit{Name: "reading", Goal: Goal{Target: 1}}, now().Add(time.Minute))

	if err := l.Archive("work"); err != nil {
		t.Fatalf("Archive() error = %v", err)
	}

	active := l.Active()
	if len(active) != 1 || active[0].ID != "reading" {
		t.Errorf("Active() = %+v, want only reading", active)
	}
	// Still resolvable, so its sessions are never orphaned.
	if _, ok := l.ByID("work"); !ok {
		t.Error("an archived habit is no longer resolvable by ID")
	}
	if err := l.Archive("nope"); err == nil {
		t.Error("Archive() accepted an unknown ID")
	}
}

func TestRemove(t *testing.T) {
	var l List
	l.Add(Habit{Name: "work", Goal: Goal{Target: 1}}, now())
	l.Add(Habit{Name: "reading", Goal: Goal{Target: 1}}, now())

	if err := l.Remove("work"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, ok := l.ByID("work"); ok {
		t.Error("the habit survived Remove()")
	}
	if len(l.Habits) != 1 {
		t.Errorf("list holds %d habits, want 1", len(l.Habits))
	}
	if err := l.Remove("nope"); err == nil {
		t.Error("Remove() accepted an unknown ID")
	}
}

func TestActiveIsOldestFirst(t *testing.T) {
	var l List
	l.Add(Habit{Name: "third", Goal: Goal{Target: 1}}, now().Add(2*time.Hour))
	l.Add(Habit{Name: "first", Goal: Goal{Target: 1}}, now())
	l.Add(Habit{Name: "second", Goal: Goal{Target: 1}}, now().Add(time.Hour))

	got := l.Active()
	want := []string{"first", "second", "third"}
	if len(got) != len(want) {
		t.Fatalf("Active() returned %d habits, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Name != want[i] {
			t.Errorf("Active()[%d] = %q, want %q", i, got[i].Name, want[i])
		}
	}
}

func TestByNameIsCaseInsensitiveAndSkipsArchived(t *testing.T) {
	var l List
	l.Add(Habit{Name: "Vibe Antarta", Goal: Goal{Target: 1}}, now())
	l.Add(Habit{Name: "gone", Goal: Goal{Target: 1}}, now())
	l.Archive("gone")

	if got, ok := l.ByName("vibe antarta"); !ok || got.ID != "vibe-antarta" {
		t.Errorf("ByName() = %+v, %v; want the habit found", got, ok)
	}
	if _, ok := l.ByName("gone"); ok {
		t.Error("ByName() returned an archived habit")
	}
	if _, ok := l.ByName("absent"); ok {
		t.Error("ByName() invented a habit")
	}
}

func TestNames(t *testing.T) {
	var l List
	l.Add(Habit{Name: "work", Goal: Goal{Target: 1}}, now())
	l.Add(Habit{Name: "reading", Goal: Goal{Target: 1}}, now().Add(time.Minute))
	l.Add(Habit{Name: "hidden", Goal: Goal{Target: 1}}, now().Add(2*time.Minute))
	l.Archive("hidden")

	if got, want := strings.Join(l.Names(), ", "), "work, reading"; got != want {
		t.Errorf("Names() = %q, want %q", got, want)
	}
}
