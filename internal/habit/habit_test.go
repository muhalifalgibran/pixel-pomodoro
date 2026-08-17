package habit

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/canvas"
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

func TestAddAssignsAColourWhenNoneIsGiven(t *testing.T) {
	var l List
	h, err := l.Add(Habit{Name: "work", Goal: Goal{Target: 1}}, now())
	if err != nil {
		t.Fatal(err)
	}
	if h.Color == "" {
		t.Fatal("no colour was assigned")
	}
	// It is one of the curated set, not an arbitrary value.
	if !slicesContains(Colors, h.Color) {
		t.Errorf("Color = %q, want one of the palette %v", h.Color, Colors)
	}
	// And it was stored, so it is visible and editable in habits.json.
	if got, _ := l.ByID("work"); got.Color != h.Color {
		t.Errorf("stored colour = %q, want %q", got.Color, h.Color)
	}
}

func TestAddKeepsAnExplicitColour(t *testing.T) {
	var l List
	h, err := l.Add(Habit{Name: "work", Goal: Goal{Target: 1}, Color: "#123456"}, now())
	if err != nil {
		t.Fatal(err)
	}
	if h.Color != "#123456" {
		t.Errorf("Color = %q, want the explicit one kept", h.Color)
	}
}

// The first habits must all look different, or the colours are useless for
// telling one bar from another.
func TestAssignedColoursAreDistinctUntilThePaletteRunsOut(t *testing.T) {
	var l List
	seen := map[string]int{}
	for i := 0; i < len(Colors); i++ {
		h, err := l.Add(Habit{Name: "habit " + strconv.Itoa(i), Goal: Goal{Target: 1}}, now())
		if err != nil {
			t.Fatal(err)
		}
		seen[h.Color]++
	}
	if len(seen) != len(Colors) {
		t.Errorf("%d habits share %d colours, want all %d distinct", len(Colors), len(seen), len(Colors))
	}

	// Past the palette size it reuses rather than leaving one blank.
	h, err := l.Add(Habit{Name: "one too many", Goal: Goal{Target: 1}}, now())
	if err != nil {
		t.Fatal(err)
	}
	if h.Color == "" {
		t.Error("a habit past the palette size got no colour")
	}
}

// A colour that changed between launches would defeat the point of having one.
func TestColorForIsStable(t *testing.T) {
	first := ColorFor("vibe-antarta")
	for i := 0; i < 20; i++ {
		if got := ColorFor("vibe-antarta"); got != first {
			t.Fatalf("ColorFor is not deterministic: %q then %q", first, got)
		}
	}
	if !slicesContains(Colors, first) {
		t.Errorf("ColorFor returned %q, which is not in the palette", first)
	}
}

func TestColorForVariesByID(t *testing.T) {
	seen := map[string]bool{}
	for _, id := range []string{"work", "reading", "gym", "vibe-antarta", "writing", "study"} {
		seen[ColorFor(id)] = true
	}
	if len(seen) < 3 {
		t.Errorf("six habits produced only %d distinct colours; the spread is too narrow", len(seen))
	}
}

// Every palette entry has to be readable, which means parseable and not so dark
// it disappears against the panel.
func TestPaletteColoursAreUsable(t *testing.T) {
	for _, c := range Colors {
		col, err := canvas.ParseHex(c)
		if err != nil {
			t.Errorf("palette colour %q does not parse: %v", c, err)
			continue
		}
		// Rough perceived brightness. The HUD panel is around #141218, so
		// anything this dark would vanish into it.
		lum := 0.299*float64(col.R) + 0.587*float64(col.G) + 0.114*float64(col.B)
		if lum < 90 {
			t.Errorf("palette colour %q is too dark to read on the panel (luminance %.0f)", c, lum)
		}
	}
}

// Clearing the colour in the form asks for a new one, not for no colour.
func TestUpdateRefillsAClearedColour(t *testing.T) {
	var l List
	h, _ := l.Add(Habit{Name: "work", Goal: Goal{Target: 1}}, now())

	cleared := h
	cleared.Color = ""
	if err := l.Update(cleared); err != nil {
		t.Fatal(err)
	}

	got, _ := l.ByID("work")
	if got.Color == "" {
		t.Error("clearing the colour left the habit without one")
	}
}

func TestUpdateKeepsAnExplicitColour(t *testing.T) {
	var l List
	h, _ := l.Add(Habit{Name: "work", Goal: Goal{Target: 1}}, now())

	h.Color = "#abcdef"
	if err := l.Update(h); err != nil {
		t.Fatal(err)
	}
	if got, _ := l.ByID("work"); got.Color != "#abcdef" {
		t.Errorf("Color = %q, want the explicit one kept", got.Color)
	}
}

func slicesContains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
