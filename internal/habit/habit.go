// Package habit holds the habit definitions: a name, a daily or weekly target,
// and optional per-habit overrides for the timer and the accent colour.
//
// Habits are identified by a stable ID rather than by name. The name is edited
// freely in the TUI, and sessions record the ID, so renaming a habit keeps every
// session it ever earned.
package habit

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/canvas"
)

// Unit is what a goal counts.
type Unit int

const (
	// Sessions counts completed focus sessions, whatever their length.
	Sessions Unit = iota
	// Minutes counts focused minutes.
	Minutes
)

// Period is the window a goal is measured over.
type Period int

const (
	Daily Period = iota
	Weekly
)

// Goal is a target over a period.
type Goal struct {
	Target int    `json:"target"`
	Unit   Unit   `json:"unit"`
	Period Period `json:"period"`
}

// weeklySuffix marks a weekly goal in its text form.
const weeklySuffix = "/ week"

// String renders a goal the way the user would say it: "1 session", "4h",
// "5 sessions / week".
func (g Goal) String() string {
	var head string
	if g.Unit == Sessions {
		head = strconv.Itoa(g.Target) + " session"
		if g.Target != 1 {
			head += "s"
		}
	} else {
		head = FormatMinutes(g.Target)
	}
	if g.Period == Weekly {
		return head + " " + weeklySuffix
	}
	return head
}

// Short renders a goal compactly for the habit list, where the column is tight.
func (g Goal) Short() string {
	var head string
	if g.Unit == Sessions {
		head = strconv.Itoa(g.Target) + "x"
	} else {
		head = FormatMinutes(g.Target)
	}
	if g.Period == Weekly {
		return head + "/wk"
	}
	return head
}

// FormatMinutes renders a minute count as hours and minutes: 45 -> "45m",
// 240 -> "4h", 90 -> "1h 30m".
func FormatMinutes(m int) string {
	if m < 0 {
		m = 0
	}
	h, rem := m/60, m%60
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", rem)
	case rem == 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dh %dm", h, rem)
	}
}

// ParseGoal reads the text form back. It accepts the same shapes String
// produces, plus a bare number, which is read as sessions.
//
//	"1 session"          1 session  daily
//	"3 sessions"         3 sessions daily
//	"4h" / "90m"         minutes    daily
//	"5 sessions / week"  5 sessions weekly
//	"10h / week"         minutes    weekly
func ParseGoal(s string) (Goal, error) {
	text := strings.ToLower(strings.TrimSpace(s))
	if text == "" {
		return Goal{}, fmt.Errorf("goal is empty")
	}

	period := Daily
	// Accept "/ week", "/week" and a trailing "per week".
	for _, suffix := range []string{"/ week", "/week", "per week", "a week", "weekly"} {
		if trimmed, ok := cutSuffix(text, suffix); ok {
			period = Weekly
			text = strings.TrimSpace(trimmed)
			break
		}
	}
	if text == "" {
		return Goal{}, fmt.Errorf("goal has no amount")
	}

	// Sessions: a count followed by the word.
	for _, word := range []string{"sessions", "session"} {
		if trimmed, ok := cutSuffix(text, word); ok {
			n, err := strconv.Atoi(strings.TrimSpace(trimmed))
			if err != nil {
				return Goal{}, fmt.Errorf("%q is not a number of sessions", strings.TrimSpace(trimmed))
			}
			return Goal{Target: n, Unit: Sessions, Period: period}, nil
		}
	}

	// A bare number means sessions; "3" is a more natural way to say
	// "3 sessions" than to say "3 minutes".
	if n, err := strconv.Atoi(text); err == nil {
		return Goal{Target: n, Unit: Sessions, Period: period}, nil
	}

	// Otherwise a duration. ParseDuration rejects the spaced "1h 30m" form, so
	// close the gaps first.
	d, err := time.ParseDuration(strings.ReplaceAll(text, " ", ""))
	if err != nil {
		return Goal{}, fmt.Errorf("%q is neither a session count nor a duration like \"4h\"", s)
	}
	mins := int(d.Round(time.Minute) / time.Minute)
	return Goal{Target: mins, Unit: Minutes, Period: period}, nil
}

// cutSuffix is strings.CutSuffix with the whitespace tolerance this parser
// needs.
func cutSuffix(s, suffix string) (string, bool) {
	if strings.HasSuffix(s, suffix) {
		return strings.TrimSpace(strings.TrimSuffix(s, suffix)), true
	}
	return s, false
}

// Habit is one tracked habit.
type Habit struct {
	// ID is a stable slug assigned at creation. It never changes, so renaming
	// a habit does not orphan the sessions logged against it.
	ID   string `json:"id"`
	Name string `json:"name"`
	Goal Goal   `json:"goal"`

	// Color is "#rrggbb". Empty inherits the phase palette's accent.
	Color string `json:"color,omitempty"`

	// Focus, Short and Long override the global timer lengths. Zero inherits.
	Focus time.Duration `json:"focus_ns,omitempty"`
	Short time.Duration `json:"short_break_ns,omitempty"`
	Long  time.Duration `json:"long_break_ns,omitempty"`

	Created time.Time `json:"created"`
	// Archived hides a habit from the picker while keeping its history. A habit
	// with sessions is archived rather than deleted, so those sessions never
	// point at an ID nothing defines.
	Archived bool `json:"archived,omitempty"`
}

// Validate reports why a habit cannot be saved.
func (h Habit) Validate() error {
	if strings.TrimSpace(h.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if h.Goal.Target <= 0 {
		return fmt.Errorf("goal must be more than zero")
	}
	if h.Color != "" {
		if _, err := canvas.ParseHex(h.Color); err != nil {
			return fmt.Errorf("colour: %w", err)
		}
	}
	for _, d := range []struct {
		name string
		val  time.Duration
	}{
		{"focus", h.Focus},
		{"short break", h.Short},
		{"long break", h.Long},
	} {
		if d.val < 0 {
			return fmt.Errorf("%s length cannot be negative", d.name)
		}
	}
	return nil
}

// Slug builds an ID candidate from a name.
func Slug(name string) string {
	var b strings.Builder
	lastDash := true // suppresses a leading dash
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		// A name of nothing but punctuation still needs an addressable ID.
		return "habit"
	}
	return slug
}

// List is the saved habit collection. It is a struct rather than a bare slice
// so the file can carry a schema version.
type List struct {
	Version int     `json:"version"`
	Habits  []Habit `json:"habits"`
}

// CurrentVersion is the schema version written to disk.
const CurrentVersion = 1

// Active returns the habits that should appear in the picker, oldest first.
func (l List) Active() []Habit {
	out := make([]Habit, 0, len(l.Habits))
	for _, h := range l.Habits {
		if !h.Archived {
			out = append(out, h)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Created.Before(out[j].Created) })
	return out
}

// ByID finds a habit, archived or not.
func (l List) ByID(id string) (Habit, bool) {
	for _, h := range l.Habits {
		if h.ID == id {
			return h, true
		}
	}
	return Habit{}, false
}

// ByName finds an active habit by name, case-insensitively. It is how the CLI
// resolves `-habit work`.
func (l List) ByName(name string) (Habit, bool) {
	want := strings.ToLower(strings.TrimSpace(name))
	for _, h := range l.Active() {
		if strings.ToLower(h.Name) == want {
			return h, true
		}
	}
	return Habit{}, false
}

// Names lists the active habit names, for error messages that should tell the
// user what they could have typed.
func (l List) Names() []string {
	active := l.Active()
	out := make([]string, 0, len(active))
	for _, h := range active {
		out = append(out, h.Name)
	}
	return out
}

// NextID returns an unused ID derived from name, suffixing on collision so two
// habits named alike remain separately addressable.
func (l List) NextID(name string) string {
	base := Slug(name)
	if _, taken := l.ByID(base); !taken {
		return base
	}
	for n := 2; ; n++ {
		candidate := base + "-" + strconv.Itoa(n)
		if _, taken := l.ByID(candidate); !taken {
			return candidate
		}
	}
}

// Add appends a habit, assigning its ID and creation time.
func (l *List) Add(h Habit, now time.Time) (Habit, error) {
	h.Name = strings.TrimSpace(h.Name)
	if err := h.Validate(); err != nil {
		return Habit{}, err
	}
	h.ID = l.NextID(h.Name)
	h.Created = now
	l.Habits = append(l.Habits, h)
	return h, nil
}

// Update replaces a habit by ID, keeping its ID and creation time whatever the
// caller passed.
func (l *List) Update(h Habit) error {
	h.Name = strings.TrimSpace(h.Name)
	if err := h.Validate(); err != nil {
		return err
	}
	for i := range l.Habits {
		if l.Habits[i].ID == h.ID {
			h.Created = l.Habits[i].Created
			h.Archived = l.Habits[i].Archived
			l.Habits[i] = h
			return nil
		}
	}
	return fmt.Errorf("no habit with id %q", h.ID)
}

// Archive hides a habit without touching its history.
func (l *List) Archive(id string) error {
	for i := range l.Habits {
		if l.Habits[i].ID == id {
			l.Habits[i].Archived = true
			return nil
		}
	}
	return fmt.Errorf("no habit with id %q", id)
}

// Remove deletes a habit outright. Callers must only use this for a habit with
// no logged sessions; Archive is correct otherwise.
func (l *List) Remove(id string) error {
	for i := range l.Habits {
		if l.Habits[i].ID == id {
			l.Habits = append(l.Habits[:i], l.Habits[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("no habit with id %q", id)
}
