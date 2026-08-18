package store

import (
	"fmt"
	"math"
	"time"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/habit"
)

// SessionLength is how long one session of a habit is: its own focus override,
// or the global default where it sets none.
func SessionLength(h habit.Habit, fallback time.Duration) time.Duration {
	if h.Focus > 0 {
		return h.Focus
	}
	return fallback
}

// ManualSession builds the log entry for work done away from the timer, whether
// that came from `pomo -log` or from ticking the habit off on the [l] screen.
//
// Both go through here so the two can never disagree about what a manual "done"
// records. source says which of them it was — see ManualLogged and
// ManualSkipped — because the two mean different things: one is time really
// spent, the other is a goal moved without a clock running.
func ManualSession(h habit.Habit, dur time.Duration, when time.Time, source string) (Session, error) {
	if dur <= 0 {
		return Session{}, fmt.Errorf("duration must be more than zero")
	}
	mins := int(math.Round(dur.Minutes()))
	if mins <= 0 {
		return Session{}, fmt.Errorf("that rounds to no minutes at all")
	}
	return Session{
		Start:  when,
		Mins:   mins,
		Habit:  h.ID,
		Task:   h.Name,
		Phase:  PhaseFocus,
		Done:   true,
		Manual: source,
	}, nil
}
