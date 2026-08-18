// Package store persists finished sessions and derives every progress number
// from them. There is deliberately no second state file: XP, level and streak
// are recomputed by replaying the log, so they can never drift out of sync
// with the sessions that earned them.
package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/paths"
)

// Phase names as they appear in the log. Zen is the open-ended stopwatch, which
// is real work but belongs to no habit.
const (
	PhaseFocus = "focus"
	PhaseZen   = "zen"
)

// Session is one line of the log.
type Session struct {
	Start time.Time `json:"start"`
	Mins  int       `json:"mins"`
	// Habit is the stable habit ID this session counts toward. Empty means it
	// belongs to no habit — a zen session, or a free-text task.
	Habit string `json:"habit,omitempty"`
	// Task is the habit's display name, or a free-text label. Kept alongside
	// Habit so the log stays readable and greppable by hand.
	Task  string `json:"task,omitempty"`
	Phase string `json:"phase"`
	Done  bool   `json:"done"`
	// Manual records that pomo did not time this session itself, and which way
	// it arrived: ManualLogged for `pomo -log`, ManualSkipped for a press of
	// space on the checklist. Empty means the timer ran it.
	//
	// It is omitempty so every line written before it existed stays valid, and
	// so a timed session — the common case — carries no extra bytes.
	Manual string `json:"manual,omitempty"`
}

// How a session that pomo did not time itself came to be recorded.
const (
	// ManualLogged is work really done away from the terminal, told to pomo
	// afterwards with `pomo -log`. It counts for everything a timed session
	// does, because the time was genuinely spent.
	ManualLogged = "logged"
	// ManualSkipped is a press of space on the checklist: the habit's goal
	// moves, but no timer ran. It earns no XP, so the level cannot be raised
	// by pressing a key.
	ManualSkipped = "skipped"
)

// EarnsXP reports whether a session should count toward XP and level. Skipped
// time moves goals, streaks and the bars — it is still a habit you did — but
// letting it raise the level would make the level a count of keypresses.
func (s Session) EarnsXP() bool { return s.IsWork() && s.Manual != ManualSkipped }

// IsWork reports whether a session counts as focused work: a finished focus
// phase or a finished zen stretch, with time actually on the clock.
func (s Session) IsWork() bool {
	return s.Done && s.Mins > 0 && (s.Phase == PhaseFocus || s.Phase == PhaseZen)
}

// Store is an append-only session log.
type Store struct{ path string }

// New builds a store over the given file. The file is created on first write.
func New(path string) *Store { return &Store{path: path} }

// Path is the log's location, for display in the stats screen.
func (s *Store) Path() string { return s.path }

// DefaultPath is $XDG_DATA_HOME/pomo/sessions.jsonl, falling back to
// ~/.local/share/pomo/sessions.jsonl.
func DefaultPath() (string, error) { return paths.DataFile("sessions.jsonl") }

// Append writes one session. Each call is a single O_APPEND write of one line,
// so a crash mid-session cannot corrupt history already on disk.
func (s *Store) Append(sess Session) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	line, err := encodeSession(sess)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open session log: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("write session: %w", err)
	}
	return nil
}

// ErrNotInLog reports that the session the caller wanted to take back is no
// longer in the log, so there is nothing to remove.
var ErrNotInLog = errors.New("that session is no longer in the log")

// Remove drops the line sess wrote, wherever it now sits. Only a line that is
// still byte-for-byte what sess encoded to is touched, so a caller can only
// ever take back its own append and never somebody else's.
//
// This is the one operation that rewrites the file. It works on the raw lines
// rather than on what Load returned, because Load drops unparseable lines on
// purpose (see below) and rebuilding from it would quietly delete a user's
// damaged-but-present history. Every other line is copied through untouched,
// and the replacement lands by rename, so a crash leaves either the old file
// or the new one and never a half-written log.
//
// It removes the last match rather than the first: identical lines can only
// come from identical appends, and taking the newest keeps undo in the order
// the presses happened.
func (s *Store) Remove(sess Session) error {
	target, err := encodeSession(sess)
	if err != nil {
		return err
	}
	target = bytes.TrimRight(target, "\n")

	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return ErrNotInLog
	}
	if err != nil {
		return fmt.Errorf("read session log: %w", err)
	}

	// SplitAfter keeps the newline on each piece, so every other line is
	// written back exactly as it was found.
	lines := bytes.SplitAfter(data, []byte("\n"))
	at := -1
	for i, line := range lines {
		trimmed := bytes.TrimRight(line, "\n")
		if len(trimmed) > 0 && bytes.Equal(trimmed, target) {
			at = i
		}
	}
	if at < 0 {
		return ErrNotInLog
	}

	kept := make([]byte, 0, len(data)-len(target))
	for i, line := range lines {
		if i == at {
			continue
		}
		kept = append(kept, line...)
	}
	return s.replace(kept)
}

// replace swaps the log's contents atomically.
func (s *Store) replace(data []byte) error {
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".sessions-*.jsonl")
	if err != nil {
		return fmt.Errorf("create temporary session log: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write session log: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close session log: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("replace session log: %w", err)
	}
	return nil
}

// encodeSession renders one log line. Append and RemoveLast share it so the
// bytes RemoveLast matches against are the bytes Append wrote.
func encodeSession(sess Session) ([]byte, error) {
	line, err := json.Marshal(sess)
	if err != nil {
		return nil, fmt.Errorf("encode session: %w", err)
	}
	return append(line, '\n'), nil
}

// Load reads the whole log. Unparseable lines are skipped and counted rather
// than failing the load: a half-written trailing line from a hard kill must
// not cost the user their history.
func (s *Store) Load() (sessions []Session, skipped int, err error) {
	f, err := os.Open(s.path)
	if os.IsNotExist(err) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("open session log: %w", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var sess Session
		if err := json.Unmarshal(line, &sess); err != nil {
			skipped++
			continue
		}
		sessions = append(sessions, sess)
	}
	if err := sc.Err(); err != nil {
		return sessions, skipped, fmt.Errorf("read session log: %w", err)
	}
	return sessions, skipped, nil
}

// Stats is everything the HUD and the stats screen display.
type Stats struct {
	XP     int // total completed focus minutes
	Level  int
	Streak int // consecutive days with at least one completed focus session

	// XPIntoLevel and XPForLevel drive the XP bar: progress through the
	// current level rather than absolute XP.
	XPIntoLevel int
	XPForLevel  int

	TodaySessions int
	TodayMins     int
	WeekMins      int

	// Zen totals are broken out because zen time belongs to no habit: it earns
	// XP and keeps the global streak alive, but appears in no habit's bar, so
	// the stats screen has to account for it separately or it looks like the
	// time went missing.
	ZenTodayMins int
	ZenWeekMins  int

	// Skipped totals are the share of the above that no timer ran, so the
	// stats screen can say how much of a day was actually sat through. They
	// are counted in TodayMins and WeekMins, and in no habit's goal any
	// differently — only XP leaves them out.
	SkippedTodayMins int
	SkippedWeekMins  int

	// ByDay holds the last DaysCharted days of completed focus minutes,
	// oldest first, for the stats screen's bar chart.
	ByDay []DayTotal
}

// DayTotal is one bar of the stats chart.
type DayTotal struct {
	Date time.Time
	Mins int
}

// DaysCharted is how far back the stats screen's chart reaches.
const DaysCharted = 14

// xpPerLevelUnit shapes the level curve: level n begins at 25*(n-1)^2 XP, so
// levels start quick and stretch out.
const xpPerLevelUnit = 25

// LevelForXP returns the level a total XP earns, starting at 1.
func LevelForXP(xp int) int {
	if xp < 0 {
		return 1
	}
	return int(math.Floor(math.Sqrt(float64(xp)/xpPerLevelUnit))) + 1
}

// XPForLevelStart is the XP at which a level begins.
func XPForLevelStart(level int) int {
	if level <= 1 {
		return 0
	}
	n := level - 1
	return xpPerLevelUnit * n * n
}

// Compute derives every statistic from the log. now is passed in rather than
// read so the day-boundary behavior is testable.
func Compute(sessions []Session, now time.Time) Stats {
	var st Stats

	loc := now.Location()
	today := civil(now)
	weekStart := today.AddDate(0, 0, -6)

	// Completed focus minutes per local day. Everything else is derived from
	// this one map.
	perDay := map[string]int{}
	perDaySessions := map[string]int{}

	for _, s := range sessions {
		if !s.IsWork() {
			continue
		}
		if s.EarnsXP() {
			st.XP += s.Mins
		}

		day := civil(s.Start.In(loc))
		key := dayKey(day)
		perDay[key] += s.Mins
		perDaySessions[key]++

		inWeek := !day.Before(weekStart) && !day.After(today)
		if day.Equal(today) {
			st.TodayMins += s.Mins
			st.TodaySessions++
		}
		if inWeek {
			st.WeekMins += s.Mins
		}
		if s.Phase == PhaseZen {
			if day.Equal(today) {
				st.ZenTodayMins += s.Mins
			}
			if inWeek {
				st.ZenWeekMins += s.Mins
			}
		}
		if s.Manual == ManualSkipped {
			if day.Equal(today) {
				st.SkippedTodayMins += s.Mins
			}
			if inWeek {
				st.SkippedWeekMins += s.Mins
			}
		}
	}

	st.Level = LevelForXP(st.XP)
	start := XPForLevelStart(st.Level)
	st.XPIntoLevel = st.XP - start
	st.XPForLevel = XPForLevelStart(st.Level+1) - start
	st.Streak = streak(perDay, today)

	st.ByDay = make([]DayTotal, 0, DaysCharted)
	for i := DaysCharted - 1; i >= 0; i-- {
		d := today.AddDate(0, 0, -i)
		st.ByDay = append(st.ByDay, DayTotal{Date: d, Mins: perDay[dayKey(d)]})
	}
	return st
}

// streak walks back from today, or from yesterday when today has not started
// yet — a streak should not appear broken just because it is 9am.
func streak(perDay map[string]int, today time.Time) int {
	day := today
	if perDay[dayKey(day)] == 0 {
		day = day.AddDate(0, 0, -1)
		if perDay[dayKey(day)] == 0 {
			return 0
		}
	}
	n := 0
	for perDay[dayKey(day)] > 0 {
		n++
		day = day.AddDate(0, 0, -1)
	}
	return n
}

// civil truncates to a local calendar day, anchored at noon. Noon rather than
// midnight because AddDate on a midnight timestamp can land on the wrong day
// across a DST transition.
func civil(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 12, 0, 0, 0, t.Location())
}

func dayKey(t time.Time) string { return t.Format("2006-01-02") }

// RecentTasks returns the most recently used task labels, newest first, for
// offering the user something to reuse when starting a session.
func RecentTasks(sessions []Session, limit int) []string {
	type seen struct {
		task string
		when time.Time
	}
	latest := map[string]time.Time{}
	for _, s := range sessions {
		if s.Task == "" {
			continue
		}
		if prev, ok := latest[s.Task]; !ok || s.Start.After(prev) {
			latest[s.Task] = s.Start
		}
	}
	all := make([]seen, 0, len(latest))
	for task, when := range latest {
		all = append(all, seen{task, when})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].when.After(all[j].when) })

	out := make([]string, 0, limit)
	for i := 0; i < len(all) && i < limit; i++ {
		out = append(out, all[i].task)
	}
	return out
}
