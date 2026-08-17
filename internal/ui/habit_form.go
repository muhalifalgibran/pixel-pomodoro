package ui

import (
	"fmt"
	"time"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/canvas"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/habit"
)

// Field positions in the habit form.
const (
	fieldName = iota
	fieldGoal
	fieldColor
	fieldFocus
	fieldBreak
)

// newHabitForm builds the add form, or the edit form when h is non-nil.
func newHabitForm(h *habit.Habit) *form {
	f := &form{
		title: "NEW HABIT",
		fields: []field{
			{
				label: "name",
				hint:  "what you call it",
				help:  []string{"work", "reading time", "vibe antarta"},
			},
			{
				label:   "goal",
				hint:    "how much, how often",
				preview: previewGoal,
				help: []string{
					"1 session          one focus session a day",
					"3 sessions         three a day",
					"4h                 four hours a day",
					"90m                ninety minutes a day",
					"3 sessions / week  three in a week",
					"10h / week         ten hours across the week",
				},
			},
			{
				label: "colour",
				hint:  "optional",
				help:  []string{"#ff7043  warm orange", "#7fb3c8  cool blue", "leave empty to use the theme"},
			},
			{
				label: "focus",
				hint:  "optional",
				help: []string{
					"50m  a longer focus phase for this habit",
					"leave empty to use the default (" + defaultFocusHint + ")",
				},
			},
			{
				label: "break",
				hint:  "optional",
				help:  []string{"10m  a longer break for this habit", "leave empty to use the default"},
			},
		},
	}
	if h != nil {
		f.title = "EDIT HABIT"
		f.fields[fieldName].value = h.Name
		f.fields[fieldGoal].value = h.Goal.String()
		f.fields[fieldColor].value = h.Color
		f.fields[fieldFocus].value = durationField(h.Focus)
		f.fields[fieldBreak].value = durationField(h.Short)
	}
	return f
}

// defaultFocusHint names the global default in the form, so "optional" says
// what you get by leaving it blank.
const defaultFocusHint = "25m"

// previewGoal interprets the goal field as it is typed. Showing what the input
// resolved to teaches the syntax faster than the examples do.
func previewGoal(v string) string {
	if v == "" {
		return ""
	}
	g, err := habit.ParseGoal(v)
	if err != nil {
		return "not a goal yet"
	}
	return g.Describe()
}

// durationField renders an optional duration, leaving zero blank rather than
// showing "0s" as though it were a real value.
func durationField(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	return d.String()
}

// habitFromForm validates the form and builds a habit from it. The returned
// error is written for the user, since it is shown in the form.
func habitFromForm(f *form) (habit.Habit, error) {
	var h habit.Habit

	h.Name = f.value(fieldName)
	if h.Name == "" {
		return h, fmt.Errorf("a habit needs a name")
	}

	goal, err := habit.ParseGoal(f.value(fieldGoal))
	if err != nil {
		return h, err
	}
	if goal.Target <= 0 {
		return h, fmt.Errorf("the goal must be more than zero")
	}
	h.Goal = goal

	if c := f.value(fieldColor); c != "" {
		if _, err := canvas.ParseHex(c); err != nil {
			return h, fmt.Errorf("colour must look like #ff7043")
		}
		h.Color = c
	}

	for _, d := range []struct {
		idx   int
		label string
		dst   *time.Duration
	}{
		{fieldFocus, "focus", &h.Focus},
		{fieldBreak, "break", &h.Short},
	} {
		raw := f.value(d.idx)
		if raw == "" {
			continue
		}
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return h, fmt.Errorf("%s must be a duration like 25m", d.label)
		}
		if parsed <= 0 {
			return h, fmt.Errorf("%s must be more than zero", d.label)
		}
		*d.dst = parsed
	}

	return h, nil
}

// sessionsFor counts how many logged sessions belong to a habit. A habit with
// history is archived rather than deleted, so its sessions never end up
// pointing at an ID nothing defines.
func (m *Model) sessionsFor(id string) int {
	n := 0
	for _, s := range m.sessions {
		if s.Habit == id {
			n++
		}
	}
	return n
}

// saveHabits persists the list and refreshes the derived figures.
func (m *Model) saveHabits() error {
	if m.habitStore == nil {
		return fmt.Errorf("habits are not available")
	}
	if err := m.habitStore.Save(m.habits); err != nil {
		return err
	}
	m.refreshProgress(time.Now())
	return nil
}
