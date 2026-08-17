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
			{label: "name", hint: "work"},
			{label: "goal", hint: "4h, 90m, 1 session, 3 sessions / week"},
			{label: "colour", hint: "#ff7043 — optional"},
			{label: "focus", hint: "25m — optional, overrides the default"},
			{label: "break", hint: "5m — optional"},
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
