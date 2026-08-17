package store

import (
	"testing"
	"time"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/habit"
)

// The 17th of August 2026 is a Monday, which makes the week boundaries in these
// tests easy to reason about.
func mon(day, hour int) time.Time { return time.Date(2026, 8, day, hour, 0, 0, 0, time.UTC) }

func hab(id, name string, target int, unit habit.Unit, period habit.Period) habit.Habit {
	return habit.Habit{
		ID:   id,
		Name: name,
		Goal: habit.Goal{Target: target, Unit: unit, Period: period},
	}
}

func work(day, hour, mins int, habitID, task string) Session {
	return Session{
		Start: mon(day, hour),
		Mins:  mins,
		Habit: habitID,
		Task:  task,
		Phase: PhaseFocus,
		Done:  true,
	}
}

func TestProgressMinutesGoal(t *testing.T) {
	h := hab("work", "work", 240, habit.Minutes, habit.Daily)
	sessions := []Session{
		work(17, 9, 120, "work", "work"),
		work(17, 11, 60, "work", "work"),
	}

	p := Progress(sessions, []habit.Habit{h}, mon(17, 20))["work"]

	if p.Value != 180 {
		t.Errorf("Value = %d, want 180", p.Value)
	}
	if p.Target != 240 {
		t.Errorf("Target = %d, want 240", p.Target)
	}
	if p.Met {
		t.Error("Met = true at 180 of 240")
	}
	if got, want := p.Fraction, 0.75; got != want {
		t.Errorf("Fraction = %v, want %v", got, want)
	}
}

func TestProgressSessionsGoal(t *testing.T) {
	h := hab("reading", "reading", 1, habit.Sessions, habit.Daily)
	// One short session is enough for a session-count goal.
	sessions := []Session{work(17, 9, 5, "reading", "reading")}

	p := Progress(sessions, []habit.Habit{h}, mon(17, 20))["reading"]

	if p.Value != 1 {
		t.Errorf("Value = %d, want 1 session", p.Value)
	}
	if !p.Met {
		t.Error("Met = false with the single required session logged")
	}
}

func TestMetIsInclusiveAtTheTarget(t *testing.T) {
	h := hab("work", "work", 240, habit.Minutes, habit.Daily)

	for _, tt := range []struct {
		mins int
		want bool
	}{
		{239, false},
		{240, true},
		{241, true},
	} {
		p := Progress([]Session{work(17, 9, tt.mins, "work", "work")}, []habit.Habit{h}, mon(17, 20))["work"]
		if p.Met != tt.want {
			t.Errorf("%d minutes against a 240 goal: Met = %v, want %v", tt.mins, p.Met, tt.want)
		}
	}
}

func TestProgressIgnoresOtherHabitsAndBreaks(t *testing.T) {
	work4h := hab("work", "work", 240, habit.Minutes, habit.Daily)
	reading := hab("reading", "reading", 1, habit.Sessions, habit.Daily)

	sessions := []Session{
		work(17, 9, 60, "work", "work"),
		work(17, 10, 60, "reading", "reading"),
		{Start: mon(17, 11), Mins: 15, Habit: "work", Phase: "short break", Done: true},
		{Start: mon(17, 12), Mins: 60, Habit: "work", Phase: PhaseFocus, Done: false}, // abandoned
	}

	got := Progress(sessions, []habit.Habit{work4h, reading}, mon(17, 20))

	if got["work"].Value != 60 {
		t.Errorf("work Value = %d, want only its own completed focus minutes", got["work"].Value)
	}
	if got["reading"].Value != 1 {
		t.Errorf("reading Value = %d, want 1", got["reading"].Value)
	}
}

// Sessions logged before the habit field existed carry only a task label.
func TestLegacySessionsAttributedByTaskName(t *testing.T) {
	h := hab("vibe-antarta", "Vibe Antarta", 60, habit.Minutes, habit.Daily)
	sessions := []Session{
		{Start: mon(17, 9), Mins: 30, Task: "vibe antarta", Phase: PhaseFocus, Done: true},
		{Start: mon(17, 10), Mins: 30, Task: "VIBE ANTARTA", Phase: PhaseFocus, Done: true},
	}

	p := Progress(sessions, []habit.Habit{h}, mon(17, 20))["vibe-antarta"]

	if p.Value != 60 {
		t.Errorf("Value = %d, want 60 — legacy lines should attribute by name, case-insensitively", p.Value)
	}
}

// The reason stable IDs exist: renaming must not lose history.
func TestRenamingAHabitKeepsItsHistory(t *testing.T) {
	sessions := []Session{
		work(16, 9, 240, "work", "work"),
		work(17, 9, 240, "work", "work"),
	}

	before := Progress(sessions, []habit.Habit{hab("work", "work", 240, habit.Minutes, habit.Daily)}, mon(17, 20))["work"]
	// Same ID, new display name.
	after := Progress(sessions, []habit.Habit{hab("work", "deep work", 240, habit.Minutes, habit.Daily)}, mon(17, 20))["work"]

	if before.Streak != after.Streak || before.Value != after.Value {
		t.Errorf("renaming changed the numbers: %+v then %+v", before, after)
	}
	if after.Streak != 2 {
		t.Errorf("Streak = %d, want 2", after.Streak)
	}
}

// A session pointing at a habit that no longer exists counts toward nothing,
// rather than being silently reattributed by its stale label.
func TestSessionsForADeletedHabitAreDropped(t *testing.T) {
	other := hab("reading", "reading", 1, habit.Sessions, habit.Daily)
	sessions := []Session{work(17, 9, 60, "deleted-habit", "reading")}

	got := Progress(sessions, []habit.Habit{other}, mon(17, 20))

	if got["reading"].Value != 0 {
		t.Errorf("reading Value = %d, want 0 — a stale ID must not fall back to name matching", got["reading"].Value)
	}
}

// Zen belongs to no goal, even when its label collides with a habit name.
func TestZenNeverCountsTowardAHabit(t *testing.T) {
	h := hab("zen", "zen", 60, habit.Minutes, habit.Daily)
	sessions := []Session{
		{Start: mon(17, 9), Mins: 90, Task: "zen", Phase: PhaseZen, Done: true},
		{Start: mon(17, 11), Mins: 90, Habit: "zen", Task: "zen", Phase: PhaseZen, Done: true},
	}

	p := Progress(sessions, []habit.Habit{h}, mon(17, 20))["zen"]

	if p.Value != 0 {
		t.Errorf("Value = %d, want 0 — zen must never move a habit", p.Value)
	}
}

func TestDailyStreak(t *testing.T) {
	h := hab("work", "work", 240, habit.Minutes, habit.Daily)
	sessions := []Session{
		work(15, 9, 240, "work", "work"),
		work(16, 9, 240, "work", "work"),
		work(17, 9, 240, "work", "work"),
	}

	if got := Progress(sessions, []habit.Habit{h}, mon(17, 20))["work"].Streak; got != 3 {
		t.Errorf("Streak = %d, want 3", got)
	}
}

// A day that fell short of the goal ends the streak, even though work happened.
func TestStreakBreaksOnAShortfallNotJustAnEmptyDay(t *testing.T) {
	h := hab("work", "work", 240, habit.Minutes, habit.Daily)
	sessions := []Session{
		work(14, 9, 240, "work", "work"),
		work(15, 9, 75, "work", "work"), // showed up, missed the goal
		work(16, 9, 240, "work", "work"),
		work(17, 9, 240, "work", "work"),
	}

	if got := Progress(sessions, []habit.Habit{h}, mon(17, 20))["work"].Streak; got != 2 {
		t.Errorf("Streak = %d, want 2 — a short day should end the run", got)
	}
}

func TestStreakSurvivesAnUnstartedToday(t *testing.T) {
	h := hab("work", "work", 240, habit.Minutes, habit.Daily)
	sessions := []Session{
		work(15, 9, 240, "work", "work"),
		work(16, 9, 240, "work", "work"),
	}

	// 9am on the 17th, nothing logged yet.
	if got := Progress(sessions, []habit.Habit{h}, mon(17, 9))["work"].Streak; got != 2 {
		t.Errorf("Streak = %d, want 2 — an unstarted today must not read as broken", got)
	}
}

func TestStreakZeroAfterTwoMissedDays(t *testing.T) {
	h := hab("work", "work", 240, habit.Minutes, habit.Daily)
	sessions := []Session{work(15, 9, 240, "work", "work")}

	if got := Progress(sessions, []habit.Habit{h}, mon(18, 9))["work"].Streak; got != 0 {
		t.Errorf("Streak = %d, want 0", got)
	}
}

func TestWeeklyGoal(t *testing.T) {
	h := hab("gym", "gym", 3, habit.Sessions, habit.Weekly)
	// The 17th is a Monday, so these three all land in the same week.
	sessions := []Session{
		work(17, 9, 45, "gym", "gym"),
		work(19, 9, 45, "gym", "gym"),
		work(21, 9, 45, "gym", "gym"),
	}

	p := Progress(sessions, []habit.Habit{h}, mon(22, 20))["gym"]

	if p.Value != 3 {
		t.Errorf("Value = %d, want 3 sessions this week", p.Value)
	}
	if !p.Met {
		t.Error("Met = false with the weekly target reached")
	}
	if p.Window != ChartWeeks {
		t.Errorf("Window = %d, want %d weeks", p.Window, ChartWeeks)
	}
}

func TestWeeklyGoalIgnoresThePreviousWeek(t *testing.T) {
	h := hab("gym", "gym", 3, habit.Sessions, habit.Weekly)
	// Sunday the 16th is the week before Monday the 17th.
	sessions := []Session{
		work(16, 9, 45, "gym", "gym"),
		work(17, 9, 45, "gym", "gym"),
	}

	p := Progress(sessions, []habit.Habit{h}, mon(17, 20))["gym"]

	if p.Value != 1 {
		t.Errorf("Value = %d, want 1 — Sunday belongs to the previous week", p.Value)
	}
}

func TestWeeklyStreakCountsWeeks(t *testing.T) {
	h := hab("gym", "gym", 2, habit.Sessions, habit.Weekly)
	var sessions []Session
	// Three consecutive weeks, two sessions each, ending in the week of the 17th.
	for _, monday := range []int{3, 10, 17} {
		sessions = append(sessions,
			work(monday, 9, 45, "gym", "gym"),
			work(monday+1, 9, 45, "gym", "gym"),
		)
	}

	if got := Progress(sessions, []habit.Habit{h}, mon(19, 20))["gym"].Streak; got != 3 {
		t.Errorf("Streak = %d, want 3 weeks", got)
	}
}

func TestWeeklyStreakSurvivesAnUnfinishedWeek(t *testing.T) {
	h := hab("gym", "gym", 2, habit.Sessions, habit.Weekly)
	sessions := []Session{
		work(10, 9, 45, "gym", "gym"),
		work(11, 9, 45, "gym", "gym"),
	}

	// Monday the 17th: this week has nothing yet, last week met its goal.
	if got := Progress(sessions, []habit.Habit{h}, mon(17, 9))["gym"].Streak; got != 1 {
		t.Errorf("Streak = %d, want 1 — a week that has just begun must not break it", got)
	}
}

func TestWeekStartIsMonday(t *testing.T) {
	// The 17th is a Monday; every day of that week resolves back to it.
	for day := 17; day <= 23; day++ {
		got := weekStart(mon(day, 12))
		if got.Day() != 17 {
			t.Errorf("weekStart(Aug %d) = Aug %d, want Aug 17", day, got.Day())
		}
	}
	// Sunday the 16th belongs to the week starting Monday the 10th.
	if got := weekStart(mon(16, 12)); got.Day() != 10 {
		t.Errorf("weekStart(Sun Aug 16) = Aug %d, want Aug 10", got.Day())
	}
}

func TestDaysBarIsOldestFirstAndEndsToday(t *testing.T) {
	h := hab("work", "work", 240, habit.Minutes, habit.Daily)
	p := Progress([]Session{work(17, 9, 240, "work", "work")}, []habit.Habit{h}, mon(17, 20))["work"]

	if len(p.Days) != ChartDays {
		t.Fatalf("Days has %d cells, want %d", len(p.Days), ChartDays)
	}
	last := p.Days[len(p.Days)-1]
	if last.Date.Day() != 17 {
		t.Errorf("last cell is the %d, want today (17)", last.Date.Day())
	}
	if last.Level != LevelMet {
		t.Errorf("today's level = %d, want LevelMet", last.Level)
	}
	if p.Days[0].Date.After(last.Date) {
		t.Error("Days is not oldest-first")
	}
}

func TestShadingLevels(t *testing.T) {
	h := hab("work", "work", 100, habit.Minutes, habit.Daily)
	tests := []struct {
		mins int
		want int
	}{
		{0, LevelNone},
		{1, LevelLow}, // any work at all must show
		{49, LevelLow},
		{50, LevelMid}, // half the goal
		{99, LevelMid},
		{100, LevelMet},
		{200, LevelMet},
	}
	for _, tt := range tests {
		p := Progress([]Session{work(17, 9, tt.mins, "work", "work")}, []habit.Habit{h}, mon(17, 20))["work"]
		got := p.Days[len(p.Days)-1].Level
		if got != tt.want {
			t.Errorf("%d of 100 minutes: level = %d, want %d", tt.mins, got, tt.want)
		}
	}
}

// A weekly target spreads over seven days for shading, so a normal day's work
// does not look like a failure on the bar.
func TestWeeklyShadingUsesADailyShare(t *testing.T) {
	h := hab("gym", "gym", 700, habit.Minutes, habit.Weekly) // 100 minutes a day
	p := Progress([]Session{work(17, 9, 100, "gym", "gym")}, []habit.Habit{h}, mon(17, 20))["gym"]

	if got := p.Days[len(p.Days)-1].Level; got != LevelMet {
		t.Errorf("level = %d, want LevelMet: 100 minutes is a full day's share of 700 a week", got)
	}
	// The period figures stay weekly regardless.
	if p.Met {
		t.Error("Met = true; one day is not a week's worth")
	}
}

func TestMetCountWithinTheWindow(t *testing.T) {
	h := hab("work", "work", 240, habit.Minutes, habit.Daily)
	sessions := []Session{
		work(15, 9, 240, "work", "work"),
		work(16, 9, 100, "work", "work"), // short
		work(17, 9, 240, "work", "work"),
	}

	p := Progress(sessions, []habit.Habit{h}, mon(17, 20))["work"]

	if p.MetCount != 2 {
		t.Errorf("MetCount = %d, want 2", p.MetCount)
	}
	if p.Window != ChartDays {
		t.Errorf("Window = %d, want %d", p.Window, ChartDays)
	}
}

// The same DST hazard the global streak is already pinned against.
func TestProgressUsesLocalCalendarDaysAcrossDST(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	h := hab("work", "work", 60, habit.Minutes, habit.Daily)
	// DST springs forward on 2026-03-08, making that local day 23 hours long.
	sessions := []Session{
		{Start: time.Date(2026, 3, 7, 22, 0, 0, 0, loc), Mins: 60, Habit: "work", Phase: PhaseFocus, Done: true},
		{Start: time.Date(2026, 3, 8, 22, 0, 0, 0, loc), Mins: 60, Habit: "work", Phase: PhaseFocus, Done: true},
		{Start: time.Date(2026, 3, 9, 22, 0, 0, 0, loc), Mins: 60, Habit: "work", Phase: PhaseFocus, Done: true},
	}

	got := Progress(sessions, []habit.Habit{h}, time.Date(2026, 3, 9, 23, 30, 0, 0, loc))["work"].Streak
	if got != 3 {
		t.Errorf("Streak = %d, want 3 across the DST boundary", got)
	}
}

func TestProgressOnAnEmptyLog(t *testing.T) {
	h := hab("work", "work", 240, habit.Minutes, habit.Daily)

	p := Progress(nil, []habit.Habit{h}, mon(17, 9))["work"]

	if p.Value != 0 || p.Met || p.Streak != 0 || p.MetCount != 0 {
		t.Errorf("progress on an empty log = %+v, want all zero", p)
	}
	if len(p.Days) != ChartDays {
		t.Errorf("Days has %d cells, want a full bar of %d", len(p.Days), ChartDays)
	}
}

func TestProgressCoversEveryHabit(t *testing.T) {
	habits := []habit.Habit{
		hab("work", "work", 240, habit.Minutes, habit.Daily),
		hab("reading", "reading", 1, habit.Sessions, habit.Daily),
		hab("gym", "gym", 3, habit.Sessions, habit.Weekly),
	}

	got := Progress(nil, habits, mon(17, 9))

	if len(got) != len(habits) {
		t.Errorf("Progress returned %d entries, want one per habit (%d)", len(got), len(habits))
	}
	for _, h := range habits {
		if _, ok := got[h.ID]; !ok {
			t.Errorf("no progress for %q", h.ID)
		}
	}
}

func TestZenTotalsAreBrokenOut(t *testing.T) {
	sessions := []Session{
		{Start: mon(17, 9), Mins: 60, Habit: "work", Task: "work", Phase: PhaseFocus, Done: true},
		{Start: mon(17, 11), Mins: 90, Task: "zen", Phase: PhaseZen, Done: true},
	}

	st := Compute(sessions, mon(17, 20))

	if st.XP != 150 {
		t.Errorf("XP = %d, want 150 — zen is real work and should earn it", st.XP)
	}
	if st.ZenTodayMins != 90 {
		t.Errorf("ZenTodayMins = %d, want 90", st.ZenTodayMins)
	}
	if st.ZenWeekMins != 90 {
		t.Errorf("ZenWeekMins = %d, want 90", st.ZenWeekMins)
	}
	if st.Streak != 1 {
		t.Errorf("Streak = %d, want 1 — zen keeps the global streak alive", st.Streak)
	}
}
