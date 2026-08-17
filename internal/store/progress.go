package store

import (
	"strings"
	"time"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/habit"
)

// ChartDays is how many days the contribution bar shows.
const ChartDays = 30

// ChartWeeks is the window for a weekly habit's met-count, roughly the same
// stretch of time as ChartDays.
const ChartWeeks = 4

// Shading levels for one cell of the contribution bar.
const (
	LevelNone = iota // nothing logged
	LevelLow         // under half the day's share
	LevelMid         // at least half, short of the goal
	LevelMet         // goal reached
)

// DayCell is one square of a habit's contribution bar.
type DayCell struct {
	Date     time.Time
	Sessions int
	Mins     int
	// Fraction is the day's value against the day's expected share of the goal.
	// For a weekly goal that share is the target spread over seven days, so the
	// bar reads as daily activity while the streak stays weekly.
	Fraction float64
	Level    int
}

// HabitProgress is everything the habit list and the stats screen need for one
// habit.
type HabitProgress struct {
	HabitID string
	Unit    habit.Unit
	Period  habit.Period

	// Value and Target describe the current period: today for a daily goal,
	// this week for a weekly one.
	Value    int
	Target   int
	Met      bool
	Fraction float64

	// Streak counts consecutive met periods, days or weeks to match Period.
	Streak int
	// MetCount and Window count met periods inside the charted stretch.
	MetCount int
	Window   int

	// Days is the contribution bar, oldest first, always ChartDays long.
	Days []DayCell
}

// tally is one habit's activity on one day.
type tally struct {
	sessions int
	mins     int
}

// Progress derives per-habit progress from the log.
//
// now is passed in rather than read so every day and week boundary is testable.
func Progress(sessions []Session, habits []habit.Habit, now time.Time) map[string]HabitProgress {
	loc := now.Location()
	today := civil(now)

	byName := make(map[string]string, len(habits))
	known := make(map[string]bool, len(habits))
	for _, h := range habits {
		known[h.ID] = true
		byName[strings.ToLower(strings.TrimSpace(h.Name))] = h.ID
	}

	// One pass over the log: (habit, day) -> tally.
	per := make(map[string]map[string]tally, len(habits))
	for _, s := range sessions {
		if !s.IsWork() {
			continue
		}
		id := attribute(s, known, byName)
		if id == "" {
			continue
		}
		day := dayKey(civil(s.Start.In(loc)))
		days, ok := per[id]
		if !ok {
			days = map[string]tally{}
			per[id] = days
		}
		t := days[day]
		t.sessions++
		t.mins += s.Mins
		days[day] = t
	}

	out := make(map[string]HabitProgress, len(habits))
	for _, h := range habits {
		out[h.ID] = progressFor(h, per[h.ID], today)
	}
	return out
}

// attribute decides which habit a session counts toward.
//
// A session written by a current version carries the habit ID. Older lines
// predate the field and are matched on their task label instead, which is why
// no migration pass is needed — nothing on disk has to be rewritten.
//
// Zen is never attributed, even if its label happens to match a habit name: the
// whole point of zen is that it belongs to no goal.
func attribute(s Session, known map[string]bool, byName map[string]string) string {
	if s.Phase == PhaseZen {
		return ""
	}
	if s.Habit != "" {
		if known[s.Habit] {
			return s.Habit
		}
		// The habit was deleted outright. Nothing to count it toward.
		return ""
	}
	return byName[strings.ToLower(strings.TrimSpace(s.Task))]
}

// progressFor evaluates one habit against its own goal.
func progressFor(h habit.Habit, days map[string]tally, today time.Time) HabitProgress {
	p := HabitProgress{
		HabitID: h.ID,
		Unit:    h.Goal.Unit,
		Period:  h.Goal.Period,
		Target:  h.Goal.Target,
	}

	value := func(t tally) int {
		if h.Goal.Unit == habit.Sessions {
			return t.sessions
		}
		return t.mins
	}
	dayValue := func(d time.Time) int { return value(days[dayKey(d)]) }

	// The share of the goal a single day is expected to carry. A weekly target
	// spreads over seven days so the bar still reads as daily activity.
	share := float64(h.Goal.Target)
	if h.Goal.Period == habit.Weekly {
		share = float64(h.Goal.Target) / 7
	}

	p.Days = make([]DayCell, 0, ChartDays)
	for i := ChartDays - 1; i >= 0; i-- {
		d := today.AddDate(0, 0, -i)
		t := days[dayKey(d)]
		cell := DayCell{Date: d, Sessions: t.sessions, Mins: t.mins}
		if share > 0 {
			cell.Fraction = float64(value(t)) / share
		}
		cell.Level = levelFor(value(t), cell.Fraction)
		p.Days = append(p.Days, cell)
	}

	switch h.Goal.Period {
	case habit.Weekly:
		weekTotal := func(anchor time.Time) int {
			start := weekStart(anchor)
			sum := 0
			for i := 0; i < 7; i++ {
				sum += dayValue(start.AddDate(0, 0, i))
			}
			return sum
		}
		p.Value = weekTotal(today)
		p.Window = ChartWeeks
		met := func(anchor time.Time) bool { return weekTotal(anchor) >= h.Goal.Target }
		p.Streak = streakBack(met, weekStart(today), func(t time.Time) time.Time { return t.AddDate(0, 0, -7) })
		for i := 0; i < ChartWeeks; i++ {
			if met(weekStart(today).AddDate(0, 0, -7*i)) {
				p.MetCount++
			}
		}
	default:
		p.Value = dayValue(today)
		p.Window = ChartDays
		met := func(d time.Time) bool { return dayValue(d) >= h.Goal.Target }
		p.Streak = streakBack(met, today, func(t time.Time) time.Time { return t.AddDate(0, 0, -1) })
		for _, c := range p.Days {
			if c.Level == LevelMet {
				p.MetCount++
			}
		}
	}

	p.Met = p.Value >= h.Goal.Target
	if h.Goal.Target > 0 {
		p.Fraction = float64(p.Value) / float64(h.Goal.Target)
	}
	return p
}

// levelFor maps a day's activity onto a shading level. Any activity at all
// shows: a day that was worked must never render as an empty one.
func levelFor(value int, fraction float64) int {
	switch {
	case value <= 0:
		return LevelNone
	case fraction >= 1:
		return LevelMet
	case fraction >= 0.5:
		return LevelMid
	default:
		return LevelLow
	}
}

// streakBack counts consecutive met periods ending at the current one.
//
// A current period that is not met yet does not break the run — it is only 9am,
// and reporting a broken streak because today has not happened yet would be
// both wrong and discouraging. This mirrors the rule the global streak already
// uses.
func streakBack(met func(time.Time) bool, current time.Time, prev func(time.Time) time.Time) int {
	at := current
	if !met(at) {
		at = prev(at)
		if !met(at) {
			return 0
		}
	}
	n := 0
	for met(at) {
		n++
		at = prev(at)
	}
	return n
}

// weekStart is the Monday of the week containing t, at noon like civil().
// Monday because a habit week that resets mid-weekend reads as arbitrary.
func weekStart(t time.Time) time.Time {
	d := civil(t)
	// time.Weekday has Sunday as 0; shift so Monday is 0.
	offset := (int(d.Weekday()) + 6) % 7
	return d.AddDate(0, 0, -offset)
}
