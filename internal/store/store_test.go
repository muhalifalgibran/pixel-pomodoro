package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

var utc = time.UTC

func at(day int, hour int) time.Time {
	return time.Date(2026, 8, day, hour, 0, 0, 0, utc)
}

func focus(day, hour, mins int) Session {
	return Session{Start: at(day, hour), Mins: mins, Phase: "focus", Done: true}
}

func TestAppendAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "sessions.jsonl")
	s := New(path)

	want := []Session{
		{Start: at(16, 9), Mins: 25, Task: "render loop", Phase: "focus", Done: true},
		{Start: at(16, 10), Mins: 5, Phase: "short break", Done: true},
	}
	for _, sess := range want {
		if err := s.Append(sess); err != nil {
			t.Fatalf("Append() error = %v", err)
		}
	}

	got, skipped, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0", skipped)
	}
	if len(got) != len(want) {
		t.Fatalf("loaded %d sessions, want %d", len(got), len(want))
	}
	for i := range want {
		if !got[i].Start.Equal(want[i].Start) || got[i].Mins != want[i].Mins || got[i].Task != want[i].Task {
			t.Errorf("session %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestLoadMissingFileIsNotAnError(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "sessions.jsonl"))

	got, skipped, err := s.Load()
	if err != nil {
		t.Fatalf("Load() on a missing log error = %v, want nil", err)
	}
	if len(got) != 0 || skipped != 0 {
		t.Errorf("got %d sessions and %d skipped, want 0 and 0", len(got), skipped)
	}
}

func TestLoadToleratesATruncatedTrailingLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.jsonl")
	s := New(path)
	if err := s.Append(focus(16, 9, 25)); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	// Simulate a hard kill mid-write.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := f.WriteString(`{"start":"2026-08-16T10:0`); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()

	got, skipped, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got) != 1 {
		t.Errorf("loaded %d sessions, want the 1 intact one", len(got))
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
}

func TestComputeXPCountsOnlyCompletedFocus(t *testing.T) {
	now := at(16, 20)
	sessions := []Session{
		focus(16, 9, 25),
		focus(16, 10, 25),
		{Start: at(16, 11), Mins: 25, Phase: "focus", Done: false},     // abandoned
		{Start: at(16, 12), Mins: 15, Phase: "long break", Done: true}, // break
		{Start: at(16, 13), Mins: 0, Phase: "focus", Done: true},       // zero length
	}

	st := Compute(sessions, now)

	if st.XP != 50 {
		t.Errorf("XP = %d, want 50 (breaks, skips and zero-length sessions earn nothing)", st.XP)
	}
	if st.TodaySessions != 2 {
		t.Errorf("TodaySessions = %d, want 2", st.TodaySessions)
	}
	if st.TodayMins != 50 {
		t.Errorf("TodayMins = %d, want 50", st.TodayMins)
	}
}

func TestLevelCurve(t *testing.T) {
	tests := []struct {
		xp    int
		level int
	}{
		{0, 1},
		{24, 1},
		{25, 2}, // 25*1^2
		{99, 2},
		{100, 3}, // 25*2^2
		{224, 3},
		{225, 4}, // 25*3^2
	}
	for _, tt := range tests {
		if got := LevelForXP(tt.xp); got != tt.level {
			t.Errorf("LevelForXP(%d) = %d, want %d", tt.xp, got, tt.level)
		}
	}
	// The boundaries must agree with the inverse.
	for level := 1; level <= 8; level++ {
		start := XPForLevelStart(level)
		if got := LevelForXP(start); got != level {
			t.Errorf("XP %d starts level %d but LevelForXP says %d", start, level, got)
		}
		if level > 1 {
			if got := LevelForXP(start - 1); got != level-1 {
				t.Errorf("XP %d should still be level %d, got %d", start-1, level-1, got)
			}
		}
	}
}

func TestComputeXPBarSpansTheCurrentLevel(t *testing.T) {
	st := Compute([]Session{focus(16, 9, 60)}, at(16, 20)) // 60 XP -> level 2 (25..100)

	if st.Level != 2 {
		t.Fatalf("Level = %d, want 2", st.Level)
	}
	if st.XPIntoLevel != 35 {
		t.Errorf("XPIntoLevel = %d, want 35", st.XPIntoLevel)
	}
	if st.XPForLevel != 75 {
		t.Errorf("XPForLevel = %d, want 75 (100-25)", st.XPForLevel)
	}
}

func TestStreakCountsConsecutiveDays(t *testing.T) {
	now := at(16, 20)
	sessions := []Session{
		focus(14, 9, 25),
		focus(15, 9, 25),
		focus(16, 9, 25),
	}

	if got := Compute(sessions, now).Streak; got != 3 {
		t.Errorf("Streak = %d, want 3", got)
	}
}

func TestStreakBreaksOnAGapDay(t *testing.T) {
	now := at(16, 20)
	sessions := []Session{
		focus(12, 9, 25),
		focus(13, 9, 25),
		// nothing on the 14th
		focus(15, 9, 25),
		focus(16, 9, 25),
	}

	if got := Compute(sessions, now).Streak; got != 2 {
		t.Errorf("Streak = %d, want 2 — the gap on the 14th ends the run", got)
	}
}

func TestStreakSurvivesADayNotYetStarted(t *testing.T) {
	// It is 9am on the 17th and nothing is logged yet, but yesterday counted.
	now := time.Date(2026, 8, 17, 9, 0, 0, 0, utc)
	sessions := []Session{focus(15, 9, 25), focus(16, 9, 25)}

	if got := Compute(sessions, now).Streak; got != 2 {
		t.Errorf("Streak = %d, want 2 — an unstarted today must not read as broken", got)
	}
}

func TestStreakZeroWhenTheGapIsTwoDays(t *testing.T) {
	now := at(18, 9)
	sessions := []Session{focus(15, 9, 25), focus(16, 9, 25)}

	if got := Compute(sessions, now).Streak; got != 0 {
		t.Errorf("Streak = %d, want 0", got)
	}
}

func TestStreakZeroOnAnEmptyLog(t *testing.T) {
	if got := Compute(nil, at(16, 9)).Streak; got != 0 {
		t.Errorf("Streak = %d, want 0", got)
	}
}

// A session is bucketed by its local calendar day. Around a DST change the
// naive "divide by 24h" approach lands sessions on the wrong day, which would
// silently break a streak.
func TestDayBucketingUsesLocalCalendarDaysAcrossDST(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	// US DST springs forward on 2026-03-08; that local day is only 23h long.
	sessions := []Session{
		{Start: time.Date(2026, 3, 7, 22, 0, 0, 0, loc), Mins: 25, Phase: "focus", Done: true},
		{Start: time.Date(2026, 3, 8, 22, 0, 0, 0, loc), Mins: 25, Phase: "focus", Done: true},
		{Start: time.Date(2026, 3, 9, 22, 0, 0, 0, loc), Mins: 25, Phase: "focus", Done: true},
	}
	now := time.Date(2026, 3, 9, 23, 0, 0, 0, loc)

	if got := Compute(sessions, now).Streak; got != 3 {
		t.Errorf("Streak = %d, want 3 across the DST boundary", got)
	}
}

func TestWeekMinsCoversSevenDaysIncludingToday(t *testing.T) {
	now := at(16, 20)
	sessions := []Session{
		focus(9, 9, 25),  // 7 days back, outside the window
		focus(10, 9, 25), // exactly 6 days back, inside
		focus(16, 9, 25), // today
	}

	if got := Compute(sessions, now).WeekMins; got != 50 {
		t.Errorf("WeekMins = %d, want 50", got)
	}
}

func TestByDayIsOldestFirstAndEndsToday(t *testing.T) {
	now := at(16, 20)
	st := Compute([]Session{focus(16, 9, 25)}, now)

	if len(st.ByDay) != DaysCharted {
		t.Fatalf("ByDay has %d entries, want %d", len(st.ByDay), DaysCharted)
	}
	last := st.ByDay[len(st.ByDay)-1]
	if last.Date.Day() != 16 {
		t.Errorf("last chart day is the %d, want today (16)", last.Date.Day())
	}
	if last.Mins != 25 {
		t.Errorf("today's chart total = %d, want 25", last.Mins)
	}
	if st.ByDay[0].Date.After(last.Date) {
		t.Error("ByDay is not oldest-first")
	}
}

func TestRecentTasksAreNewestFirstAndDeduplicated(t *testing.T) {
	sessions := []Session{
		{Start: at(14, 9), Task: "alpha", Phase: "focus", Done: true, Mins: 25},
		{Start: at(15, 9), Task: "beta", Phase: "focus", Done: true, Mins: 25},
		{Start: at(16, 9), Task: "alpha", Phase: "focus", Done: true, Mins: 25},
		{Start: at(16, 10), Phase: "focus", Done: true, Mins: 25}, // unlabelled
	}

	got := RecentTasks(sessions, 5)

	want := []string{"alpha", "beta"}
	if len(got) != len(want) {
		t.Fatalf("RecentTasks() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("RecentTasks()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRecentTasksRespectsLimit(t *testing.T) {
	sessions := []Session{
		{Start: at(14, 9), Task: "a"},
		{Start: at(15, 9), Task: "b"},
		{Start: at(16, 9), Task: "c"},
	}
	if got := RecentTasks(sessions, 2); len(got) != 2 {
		t.Errorf("RecentTasks(limit 2) returned %d entries", len(got))
	}
}

func TestDefaultPathHonorsXDG(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/tmp/xdg")

	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}
	if want := filepath.Join("/tmp/xdg", "pomo", "sessions.jsonl"); got != want {
		t.Errorf("DefaultPath() = %q, want %q", got, want)
	}
}
