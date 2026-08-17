package ui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/config"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/habit"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/store"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/theme"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/timer"
)

func testModel(t *testing.T, mutate func(*config.Config)) (*Model, *store.Store) {
	t.Helper()
	cfg := config.Default()
	cfg.Focus = time.Minute
	cfg.ShortBreak = 30 * time.Second
	cfg.LongBreak = time.Minute
	cfg.Notify = false
	cfg.Sound = ""
	if mutate != nil {
		mutate(&cfg)
	}
	st := store.New(filepath.Join(t.TempDir(), "sessions.jsonl"))

	m, err := New(Options{Config: cfg, Store: st, TickScale: 1})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return m, st
}

// tick drives the model forward by d without touching the wall clock.
func tick(m *Model, d time.Duration) {
	m.advance(m.lastTick.Add(d))
}

func sizeTo(m *Model, w, h int) {
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
}

func press(m *Model, key string) {
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
}

func TestFormatRemaining(t *testing.T) {
	tests := []struct {
		d           time.Duration
		showSeconds bool
		want        string
	}{
		{25 * time.Minute, true, "25:00"},
		{90 * time.Second, true, "01:30"},
		{0, true, "00:00"},
		{-time.Second, true, "00:00"},
		// Sub-second remainders round up, so the display hits 00:00 exactly
		// when the phase ends instead of sitting there for a full second.
		{1500 * time.Millisecond, true, "00:02"},
		{500 * time.Millisecond, true, "00:01"},
		{25 * time.Minute, false, "25:--"},
		{45 * time.Second, false, "00:45"},
	}
	for _, tt := range tests {
		if got := FormatRemaining(tt.d, tt.showSeconds); got != tt.want {
			t.Errorf("FormatRemaining(%v, %v) = %q, want %q", tt.d, tt.showSeconds, got, tt.want)
		}
	}
}

// Every clock string must be the same display width, or the HUD reflows on
// each tick.
func TestClockCanvasWidthIsStable(t *testing.T) {
	w, h := clockCanvasSize("00:00")
	for _, s := range []string{"25:00", "11:11", "88:88", "09:59"} {
		gw, gh := clockCanvasSize(s)
		if gw != w || gh != h {
			t.Errorf("clockCanvasSize(%q) = %dx%d, want %dx%d", s, gw, gh, w, h)
		}
	}
}

func TestLayoutKeepsBlitsOnEvenRows(t *testing.T) {
	// Odd sprite and clock heights would otherwise produce an odd offset,
	// which re-pairs every half-block and shreds the art.
	for _, spriteH := range []int{20, 24, 30} {
		for _, clockH := range []int{9, 10, 11} {
			g := layout(24, spriteH, 29, clockH)
			if g.SpriteY%2 != 0 {
				t.Errorf("layout(sprite %d, clock %d).SpriteY = %d, want even", spriteH, clockH, g.SpriteY)
			}
			if g.ClockY%2 != 0 {
				t.Errorf("layout(sprite %d, clock %d).ClockY = %d, want even", spriteH, clockH, g.ClockY)
			}
			if g.BandH%2 != 0 {
				t.Errorf("layout(sprite %d, clock %d).BandH = %d, want even", spriteH, clockH, g.BandH)
			}
		}
	}
}

func TestClockRollStartsOnChangeAndSettles(t *testing.T) {
	var c clock
	c.set("01:00")
	if !c.settled() {
		t.Fatal("a freshly set clock should be settled")
	}

	c.set("00:59")
	if c.settled() {
		t.Fatal("changing digits did not start a roll")
	}

	// Advance past the roll duration.
	c.update(rollDuration.Seconds() * 1.5)
	if !c.settled() {
		t.Error("roll did not settle after its duration elapsed")
	}
}

func TestClockRollOnlyAnimatesChangedCharacters(t *testing.T) {
	var c clock
	c.set("12:34")
	c.set("12:35")

	// Only the last character changed.
	for i, r := range c.roll {
		settled := r >= 1
		if i == 4 && settled {
			t.Errorf("character %d changed but is not rolling", i)
		}
		if i != 4 && !settled {
			t.Errorf("character %d did not change but is rolling", i)
		}
	}
}

func TestClockStyleEscalatesInsideTheAlertWindow(t *testing.T) {
	pal := theme.Ember

	calm, alert := clockStyleFor(pal, time.Minute, true)
	if alert {
		t.Error("alert triggered a minute out")
	}
	if calm != pal.Clock {
		t.Error("calm style should be the palette's own clock style")
	}

	_, alert = clockStyleFor(pal, 3*time.Second, true)
	if !alert {
		t.Error("alert did not trigger near the end of the phase")
	}

	// A paused timer is not urgent, however little is left.
	if _, alert := clockStyleFor(pal, time.Second, false); alert {
		t.Error("a paused timer entered the alert state")
	}
}

func TestFullViewLinesAreAllTheSameWidth(t *testing.T) {
	m, _ := testModel(t, nil)
	sizeTo(m, 100, 40)

	lines := strings.Split(m.View(), "\n")
	if len(lines) < 5 {
		t.Fatalf("View() produced %d lines, want the full HUD", len(lines))
	}

	// The help grid sits outside the frame; every framed line must match, or
	// the border will not line up.
	want := lipgloss.Width(lines[0])
	for i, l := range lines[:len(lines)-helpRows] {
		if got := lipgloss.Width(l); got != want {
			t.Errorf("line %d is %d cells wide, want %d:\n%q", i, got, want, l)
		}
	}
	if want != m.geom.BandW+2 {
		t.Errorf("frame is %d cells wide, want band %d plus two border columns", want, m.geom.BandW)
	}
}

// A long task label must not push the frame's right edge out.
func TestFullViewSurvivesAnOverlongTask(t *testing.T) {
	m, _ := testModel(t, nil)
	sizeTo(m, 100, 40)
	m.timer.Task = strings.Repeat("very long task name ", 20)

	lines := strings.Split(m.View(), "\n")
	want := lipgloss.Width(lines[0])
	for i, l := range lines[:len(lines)-helpRows] {
		if got := lipgloss.Width(l); got != want {
			t.Errorf("line %d is %d cells wide, want %d", i, got, want)
		}
	}
}

func TestHelpBlockIsAGridOfColumns(t *testing.T) {
	rows := helpBlock(theme.Ember, timerKeys)

	if len(rows) != helpRows {
		t.Fatalf("helpBlock returned %d rows, want %d", len(rows), helpRows)
	}

	// Hints fill downward: three per column, so nine entries make three full
	// columns.
	wantContents := [helpRows][]string{
		{"space", "habits", "note"},
		{"skip", "stats", "quit"},
		{"reset", "zen", "hide"},
	}
	for i, want := range wantContents {
		for _, w := range want {
			if !strings.Contains(rows[i], w) {
				t.Errorf("row %d = %q, want it to contain %q", i, rows[i], w)
			}
		}
	}

	// Columns must line up, which means every row is the same width.
	want := lipgloss.Width(rows[0])
	for i, r := range rows {
		if got := lipgloss.Width(r); got != want {
			t.Errorf("row %d is %d cells wide, want %d — columns are ragged", i, got, want)
		}
	}
}

// The block must never be wider than the HUD it sits under.
func TestHelpBlockFitsUnderTheFrame(t *testing.T) {
	m, _ := testModel(t, nil)
	frameWidth := m.geom.BandW + 2
	pal := theme.Ember

	for _, editing := range []bool{false, true} {
		keys := timerKeys
		if editing {
			keys = editingKeys
		}
		for i, r := range helpBlock(pal, keys) {
			if got := lipgloss.Width(r); got > frameWidth {
				t.Errorf("editing=%v row %d is %d cells wide, wider than the %d-cell frame",
					editing, i, got, frameWidth)
			}
		}
	}
}

// Editing mode has fewer hints than rows; the block must still be helpRows
// tall so the layout does not jump when entering and leaving the task field.
func TestHelpBlockHeightIsStableAcrossModes(t *testing.T) {
	if got := len(helpBlock(theme.Ember, editingKeys)); got != helpRows {
		t.Errorf("editing help is %d rows, want %d", got, helpRows)
	}
}

func TestCompactViewOnASmallTerminal(t *testing.T) {
	m, _ := testModel(t, nil)
	sizeTo(m, 40, 12)

	out := m.View()

	if strings.Contains(out, frameTopLeft) {
		t.Error("small terminal still rendered the full frame")
	}
	if !strings.Contains(out, "01:00") {
		t.Errorf("compact view is missing the clock:\n%s", out)
	}
}

func TestKeysPauseSkipAndSwitchScreens(t *testing.T) {
	m, _ := testModel(t, nil)
	m.timer.Running = true

	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if m.timer.Running {
		t.Error("space did not pause")
	}
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !m.timer.Running {
		t.Error("space did not resume")
	}

	press(m, "s")
	if m.timer.Phase != timer.ShortBreak {
		t.Errorf("s left the timer in %v, want a short break", m.timer.Phase)
	}

	press(m, "t")
	if m.mode != modeStats {
		t.Error("t did not open the stats screen")
	}
	press(m, "t")
	if m.mode != modeNormal {
		t.Error("t did not close the stats screen")
	}
}

func TestTaskEditing(t *testing.T) {
	m, _ := testModel(t, nil)

	press(m, "e")
	if m.mode != modeEditTask {
		t.Fatal("e did not enter task editing")
	}

	press(m, "h")
	press(m, "i")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.mode != modeNormal {
		t.Error("enter did not leave task editing")
	}
	if m.timer.Task != "hi" {
		t.Errorf("Task = %q, want %q", m.timer.Task, "hi")
	}
}

func TestTaskEditingCancelRestoresThePreviousLabel(t *testing.T) {
	m, _ := testModel(t, nil)
	m.timer.Task = "original"

	press(m, "e")
	press(m, "x")
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.timer.Task != "original" {
		t.Errorf("Task = %q, want the edit to have been discarded", m.timer.Task)
	}
}

// While editing, keys must reach the text field rather than the timer, or
// typing "space" would pause and typing "s" would skip.
func TestEditingSwallowsCommandKeys(t *testing.T) {
	m, _ := testModel(t, nil)
	m.timer.Running = true
	press(m, "e")

	press(m, "s")
	m.Update(tea.KeyMsg{Type: tea.KeySpace})

	if !m.timer.Running {
		t.Error("space paused the timer while editing the task")
	}
	if m.timer.Phase != timer.Focus {
		t.Error("s skipped the phase while editing the task")
	}
}

func TestCompletingAPhaseAppendsToTheLog(t *testing.T) {
	m, st := testModel(t, nil)
	m.timer.Running = true
	m.timer.Task = "render loop"

	tick(m, 61*time.Second)

	sessions, skipped, err := st.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0", skipped)
	}
	if len(sessions) != 1 {
		t.Fatalf("logged %d sessions, want 1", len(sessions))
	}
	got := sessions[0]
	if got.Phase != "focus" || !got.Done || got.Mins != 1 || got.Task != "render loop" {
		t.Errorf("logged %+v, want a completed 1m focus labelled \"render loop\"", got)
	}
	if m.stats.XP != 1 {
		t.Errorf("XP = %d, want the completed minute to have been counted", m.stats.XP)
	}
}

func TestSkippingLogsWhatActuallyRan(t *testing.T) {
	m, st := testModel(t, nil)
	m.timer.Running = true

	tick(m, 40*time.Second)
	press(m, "s")

	sessions, _, err := st.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("logged %d sessions, want 1", len(sessions))
	}
	got := sessions[0]
	if got.Done {
		t.Error("skipped session was logged as completed")
	}
	if got.Mins != 1 {
		t.Errorf("Mins = %d, want the 40s that actually ran to round to 1", got.Mins)
	}
	// A skipped focus earns nothing.
	if m.stats.XP != 0 {
		t.Errorf("XP = %d, want 0 for a skipped session", m.stats.XP)
	}
}

func TestVeryShortPhasesAreNotLogged(t *testing.T) {
	m, st := testModel(t, nil)
	m.timer.Running = true

	// Skip almost immediately: nothing meaningful happened.
	tick(m, 2*time.Second)
	press(m, "s")

	sessions, _, err := st.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("logged %d sessions, want none for a two-second phase", len(sessions))
	}
}

func TestTickScaleFastForwards(t *testing.T) {
	cfg := config.Default()
	cfg.Focus = time.Minute
	cfg.Notify = false
	cfg.Sound = ""
	st := store.New(filepath.Join(t.TempDir(), "sessions.jsonl"))

	m, err := New(Options{Config: cfg, Store: st, TickScale: 60})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	m.timer.Running = true

	// One real second at 60x is a full minute of timer time.
	tick(m, time.Second)

	if m.timer.Phase == timer.Focus {
		t.Error("tick-scale did not fast-forward past the focus phase")
	}
}

func TestSkipToEndStartsAtTheBoundary(t *testing.T) {
	cfg := config.Default()
	cfg.Notify = false
	cfg.Sound = ""
	st := store.New(filepath.Join(t.TempDir(), "sessions.jsonl"))

	m, err := New(Options{Config: cfg, Store: st, SkipToEnd: true, StartRunning: true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if m.timer.Remaining != time.Second {
		t.Errorf("Remaining = %v, want 1s", m.timer.Remaining)
	}
	if !m.timer.Running {
		t.Error("skip-to-end did not start the timer")
	}
}

func TestPhaseChangeCrossFadesThePalette(t *testing.T) {
	// A short phase and small ticks, so the boundary is crossed by a tick
	// shorter than the fade itself. One huge catch-up tick would legitimately
	// complete the fade in the same step.
	m, _ := testModel(t, func(c *config.Config) { c.Focus = 500 * time.Millisecond })
	m.timer.Running = true

	tick(m, 400*time.Millisecond) // still in focus
	tick(m, 150*time.Millisecond) // crosses into the break

	if m.timer.Phase == timer.Focus {
		t.Fatal("timer did not reach the break")
	}
	if m.palT >= 1 {
		t.Error("phase change did not start a palette cross-fade")
	}
	if m.palTo.Name != theme.Mint.Name {
		t.Errorf("fading toward %q, want mint for a short break", m.palTo.Name)
	}

	// The fade must finish rather than hang partway.
	tick(m, paletteFade*2)
	if m.palT < 1 {
		t.Errorf("palT = %v, want the cross-fade to have completed", m.palT)
	}
}

func TestSteamOnlyDuringFocus(t *testing.T) {
	m, _ := testModel(t, nil)
	m.timer.Running = true

	// Tick at the real frame interval. One big jump would age every wisp past
	// its lifetime in a single step and reap them all.
	for i := 0; i < 20; i++ {
		tick(m, frameInterval)
	}
	if m.steam.Count() == 0 {
		t.Error("focus produced no steam")
	}

	press(m, "s") // move to a break, which clears the pool
	for i := 0; i < 40; i++ {
		tick(m, frameInterval)
	}
	if m.steam.Count() != 0 {
		t.Errorf("steam still running during a break: %d particles", m.steam.Count())
	}
}

func TestStatsViewRendersWithoutData(t *testing.T) {
	m, _ := testModel(t, nil)
	press(m, "t")

	out := m.View()

	for _, want := range []string{"POMO STATS", "Level", "Streak", "Last 14 days"} {
		if !strings.Contains(out, want) {
			t.Errorf("stats view is missing %q:\n%s", want, out)
		}
	}
}

func TestStatsChartMarksEveryWorkedDay(t *testing.T) {
	now := time.Date(2026, 8, 16, 20, 0, 0, 0, time.UTC)
	sessions := []store.Session{
		{Start: now.AddDate(0, 0, -1), Mins: 300, Phase: "focus", Done: true}, // a big day
		{Start: now, Mins: 1, Phase: "focus", Done: true},                     // a tiny one
	}
	st := store.Compute(sessions, now)

	out := chart(theme.Ember, st.ByDay)

	// The tiny day must still show something; rounding it to zero would
	// misreport a day that was worked as a day that was not.
	if strings.Count(out, "▄")+strings.Count(out, "█") < 2 {
		t.Errorf("chart lost a worked day:\n%s", out)
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		in   string
		max  int
		want string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"truncate me please", 8, "truncat…"},
		{"x", 1, "x"},
	}
	for _, tt := range tests {
		got := truncate(tt.in, tt.max)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.in, tt.max, got, tt.want)
		}
		if lipgloss.Width(got) > tt.max {
			t.Errorf("truncate(%q, %d) = %q, which is %d cells wide", tt.in, tt.max, got, lipgloss.Width(got))
		}
	}
}

func TestPadFixesWidthInBothDirections(t *testing.T) {
	if got := pad("hi", 6); lipgloss.Width(got) != 6 {
		t.Errorf("pad short string = %q, width %d, want 6", got, lipgloss.Width(got))
	}
	if got := pad("far too long for this", 6); lipgloss.Width(got) != 6 {
		t.Errorf("pad long string = %q, width %d, want 6", got, lipgloss.Width(got))
	}
}

func TestMeter(t *testing.T) {
	if got, want := meter("#", ".", 5, 10, 10), "#####....."; got != want {
		t.Errorf("meter() = %q, want %q", got, want)
	}
	if got, want := meter("#", ".", 20, 10, 4), "####"; got != want {
		t.Errorf("meter() over 100%% = %q, want %q", got, want)
	}
	if got, want := meter("#", ".", 1, 0, 4), "...."; got != want {
		t.Errorf("meter() with a zero total = %q, want %q", got, want)
	}
}

// The behaviour that matters: quit mid-phase, come back, pick up where you
// left off.
func TestQuitThenRelaunchResumesWhereItLeftOff(t *testing.T) {
	cfg := config.Default()
	cfg.Focus = 25 * time.Minute
	cfg.Notify = false
	cfg.Sound = ""
	st := store.New(filepath.Join(t.TempDir(), "sessions.jsonl"))

	first, err := New(Options{Config: cfg, Store: st, Task: "render loop", StartRunning: true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	tick(first, 9*time.Minute)
	press(first, "q")

	second, err := New(Options{Config: cfg, Store: st, StartRunning: true})
	if err != nil {
		t.Fatalf("New() on relaunch error = %v", err)
	}

	if got, want := second.timer.Remaining, 16*time.Minute; got != want {
		t.Errorf("Remaining = %v, want %v", got, want)
	}
	if second.timer.Task != "render loop" {
		t.Errorf("Task = %q, want the label to survive the quit", second.timer.Task)
	}
	if !second.resumed {
		t.Error("relaunch did not report itself as resumed")
	}
	if !strings.Contains(second.View(), "resumed") {
		t.Error("the HUD does not say the session was resumed")
	}
}

func TestResumeRestoresPhaseAndCycle(t *testing.T) {
	cfg := config.Default()
	cfg.Focus = time.Minute
	cfg.ShortBreak = 5 * time.Minute
	cfg.Notify = false
	cfg.Sound = ""
	st := store.New(filepath.Join(t.TempDir(), "sessions.jsonl"))

	first, _ := New(Options{Config: cfg, Store: st, StartRunning: true})
	tick(first, 61*time.Second) // finish the focus, land in a break
	tick(first, time.Minute)
	press(first, "q")

	second, _ := New(Options{Config: cfg, Store: st, StartRunning: true})

	if second.timer.Phase != timer.ShortBreak {
		t.Errorf("Phase = %v, want the break to be resumed", second.timer.Phase)
	}
	if second.timer.CycleIndex != 1 {
		t.Errorf("CycleIndex = %d, want 1", second.timer.CycleIndex)
	}
	// The 61s tick spent 60s finishing the focus and 1s on the break, then a
	// further minute ran, leaving 5m - 1s - 1m.
	if got, want := second.timer.Remaining, 3*time.Minute+59*time.Second; got != want {
		t.Errorf("Remaining = %v, want %v", got, want)
	}
}

// A session started before a quit must be logged against when it really began,
// not when the program was relaunched.
func TestResumedSessionLogsItsOriginalStartTime(t *testing.T) {
	cfg := config.Default()
	cfg.Focus = time.Minute
	cfg.Notify = false
	cfg.Sound = ""
	st := store.New(filepath.Join(t.TempDir(), "sessions.jsonl"))

	first, _ := New(Options{Config: cfg, Store: st, StartRunning: true})
	originalStart := first.phaseStart
	tick(first, 30*time.Second)
	press(first, "q")

	second, _ := New(Options{Config: cfg, Store: st, StartRunning: true})
	if !second.phaseStart.Equal(originalStart.Round(0)) && second.phaseStart.Sub(originalStart).Abs() > time.Second {
		t.Errorf("phaseStart = %v, want it close to the original %v", second.phaseStart, originalStart)
	}
}

func TestFreshIgnoresTheSavedPosition(t *testing.T) {
	cfg := config.Default()
	cfg.Notify = false
	cfg.Sound = ""
	st := store.New(filepath.Join(t.TempDir(), "sessions.jsonl"))

	first, _ := New(Options{Config: cfg, Store: st, Task: "old task", StartRunning: true})
	tick(first, 9*time.Minute)
	press(first, "q")

	second, err := New(Options{Config: cfg, Store: st, Fresh: true, StartRunning: true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if second.timer.Remaining != cfg.Focus {
		t.Errorf("Remaining = %v, want a full phase %v", second.timer.Remaining, cfg.Focus)
	}
	if second.resumed {
		t.Error("-fresh still reported a resume")
	}
}

func TestResumeDisabledInConfig(t *testing.T) {
	cfg := config.Default()
	cfg.Resume = false
	cfg.Notify = false
	cfg.Sound = ""
	st := store.New(filepath.Join(t.TempDir(), "sessions.jsonl"))

	first, _ := New(Options{Config: cfg, Store: st, StartRunning: true})
	tick(first, 9*time.Minute)
	press(first, "q")

	if _, ok := st.LoadResume(time.Now()); ok {
		t.Error("resume = false still wrote a state file")
	}
	second, _ := New(Options{Config: cfg, Store: st, StartRunning: true})
	if second.timer.Remaining != cfg.Focus {
		t.Errorf("Remaining = %v, want a full phase", second.timer.Remaining)
	}
}

// An explicit -task is the user speaking now; it should win over the label
// remembered from last time.
func TestExplicitTaskOverridesTheRememberedOne(t *testing.T) {
	cfg := config.Default()
	cfg.Notify = false
	cfg.Sound = ""
	st := store.New(filepath.Join(t.TempDir(), "sessions.jsonl"))

	first, _ := New(Options{Config: cfg, Store: st, Task: "old task", StartRunning: true})
	tick(first, time.Minute)
	press(first, "q")

	second, _ := New(Options{Config: cfg, Store: st, Task: "new task", StartRunning: true})
	if second.timer.Task != "new task" {
		t.Errorf("Task = %q, want the flag to win", second.timer.Task)
	}
	if !second.resumed {
		t.Error("overriding the task should not cancel the resume")
	}
}

// Finishing a phase must overwrite the saved position, so a crash afterwards
// cannot resurrect a session that already completed.
func TestCompletingAPhaseRewritesTheSavedPosition(t *testing.T) {
	cfg := config.Default()
	cfg.Focus = time.Minute
	cfg.ShortBreak = 5 * time.Minute
	cfg.Notify = false
	cfg.Sound = ""
	st := store.New(filepath.Join(t.TempDir(), "sessions.jsonl"))

	m, _ := New(Options{Config: cfg, Store: st, StartRunning: true})
	tick(m, 61*time.Second) // focus completes, break begins

	saved, ok := st.LoadResume(time.Now())
	if !ok {
		t.Fatal("no position was saved at the phase boundary")
	}
	if saved.Phase != timer.ShortBreak.String() {
		t.Errorf("saved phase = %q, want the new break", saved.Phase)
	}
}

func TestNothingIsSavedForAPhaseAboutToEnd(t *testing.T) {
	cfg := config.Default()
	cfg.Notify = false
	cfg.Sound = ""
	st := store.New(filepath.Join(t.TempDir(), "sessions.jsonl"))

	m, _ := New(Options{Config: cfg, Store: st, SkipToEnd: true, StartRunning: true})
	tick(m, 900*time.Millisecond) // 1s phase, so 100ms is left
	press(m, "q")

	if _, ok := st.LoadResume(time.Now()); ok {
		t.Error("a phase with under a second left was saved as resumable")
	}
}

func TestSlashTogglesTheLegend(t *testing.T) {
	m, _ := testModel(t, nil)
	sizeTo(m, 100, 40)

	if !m.showHelp {
		t.Fatal("the legend should start visible so the keys are discoverable")
	}
	full := strings.Split(m.View(), "\n")

	press(m, "/")
	if m.showHelp {
		t.Fatal("/ did not hide the legend")
	}
	collapsed := strings.Split(m.View(), "\n")

	if len(collapsed) >= len(full) {
		t.Errorf("collapsed view is %d lines, want fewer than %d", len(collapsed), len(full))
	}
	if strings.Contains(collapsed[len(collapsed)-1], "pause") {
		t.Error("the legend is still showing after being hidden")
	}
	// Hiding it entirely would leave no way back.
	if !strings.Contains(collapsed[len(collapsed)-1], "keys") {
		t.Errorf("no hint about the toggle once hidden: %q", collapsed[len(collapsed)-1])
	}

	press(m, "/")
	if !m.showHelp {
		t.Error("/ did not bring the legend back")
	}
}

// Enter and esc are not guessable, so the legend must appear while editing
// even when the user has hidden it.
func TestEditingShowsTheKeysEvenWhenHidden(t *testing.T) {
	m, _ := testModel(t, nil)
	sizeTo(m, 100, 40)
	press(m, "/")

	press(m, "e")
	out := m.View()

	if !strings.Contains(out, "save") || !strings.Contains(out, "cancel") {
		t.Errorf("editing keys are missing while the legend is hidden:\n%s", out)
	}
}

func TestRequiredHeightTracksTheLegend(t *testing.T) {
	m, _ := testModel(t, nil)

	shown := m.requiredHeight()
	press(m, "/")
	hidden := m.requiredHeight()

	if hidden >= shown {
		t.Errorf("hiding the legend did not reduce the required height: %d then %d", shown, hidden)
	}
	if got, want := shown-hidden, helpRows-1; got != want {
		t.Errorf("height changed by %d, want %d", got, want)
	}
}

// Space arrives as KeySpace with Runes already holding " ". Appending both
// produced a double space on every word break.
func TestTypingSpacesInATaskLabel(t *testing.T) {
	m, _ := testModel(t, nil)
	press(m, "e")

	for _, r := range "vibe code pomo" {
		if r == ' ' {
			m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
			continue
		}
		press(m, string(r))
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.timer.Task != "vibe code pomo" {
		t.Errorf("Task = %q, want %q", m.timer.Task, "vibe code pomo")
	}
}

// The mascot should read as breathing, not panting. A period much under a few
// seconds looks agitated on screen.
func TestBreathingIsCalmInEveryPhase(t *testing.T) {
	for _, tt := range []struct {
		name    string
		phase   timer.Phase
		running bool
	}{
		{"focus", timer.Focus, true},
		{"short break", timer.ShortBreak, true},
		{"long break", timer.LongBreak, true},
		{"paused", timer.Focus, false},
	} {
		b := breathFor(tt.phase, tt.running)
		if b.period < slowestBreath {
			t.Errorf("%s breathes with a %.1fs period, want at least %.1fs",
				tt.name, b.period, slowestBreath)
		}
		if b.bob <= 0 || b.squash <= 0 {
			t.Errorf("%s has no idle motion (%+v); a frozen mascot reads as a hang", tt.name, b)
		}
	}
}

// Focus stays the briskest so it reads as alert rather than restful.
func TestFocusBreathesFasterThanBreaks(t *testing.T) {
	focus := breathFor(timer.Focus, true)
	for _, p := range []timer.Phase{timer.ShortBreak, timer.LongBreak} {
		if got := breathFor(p, true); focus.period >= got.period {
			t.Errorf("focus period %.1fs is not brisker than %v's %.1fs", focus.period, p, got.period)
		}
	}
}

// Only focus gives off steam; a break should be still.
func TestSteamIsFocusOnly(t *testing.T) {
	if breathFor(timer.Focus, true).steamHz <= 0 {
		t.Error("focus produces no steam")
	}
	for _, p := range []timer.Phase{timer.ShortBreak, timer.LongBreak} {
		if got := breathFor(p, true).steamHz; got != 0 {
			t.Errorf("%v emits steam at %v Hz, want none", p, got)
		}
	}
	if breathFor(timer.Focus, false).steamHz != 0 {
		t.Error("a paused timer still emits steam")
	}
}

// --- habits ---

func habitModel(t *testing.T, goals ...habit.Habit) (*Model, *store.Store, *habit.Store) {
	t.Helper()
	dir := t.TempDir()
	hs := habit.NewStore(filepath.Join(dir, "habits.json"))

	var l habit.List
	base := time.Now().Add(-time.Hour)
	for i, h := range goals {
		if _, err := l.Add(h, base.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatalf("Add(%q): %v", h.Name, err)
		}
	}
	if err := hs.Save(l); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Notify = false
	cfg.Sound = ""
	st := store.New(filepath.Join(dir, "sessions.jsonl"))

	m, err := New(Options{Config: cfg, Store: st, Habits: hs, TickScale: 1})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return m, st, hs
}

func minutesHabit(name string, mins int) habit.Habit {
	return habit.Habit{Name: name, Goal: habit.Goal{Target: mins, Unit: habit.Minutes, Period: habit.Daily}}
}

func sessionsHabit(name string, n int) habit.Habit {
	return habit.Habit{Name: name, Goal: habit.Goal{Target: n, Unit: habit.Sessions, Period: habit.Daily}}
}

func TestHabitsScreenListsEveryHabit(t *testing.T) {
	m, _, _ := habitModel(t, minutesHabit("work", 240), sessionsHabit("reading", 1))
	sizeTo(m, 100, 40)

	press(m, "h")
	if m.mode != modeHabits {
		t.Fatal("h did not open the habits screen")
	}

	out := m.View()
	for _, want := range []string{"HABITS", "work", "reading", "4h"} {
		if !strings.Contains(out, want) {
			t.Errorf("habits screen is missing %q:\n%s", want, out)
		}
	}
}

func TestHabitsCursorMovesAndWraps(t *testing.T) {
	m, _, _ := habitModel(t, minutesHabit("work", 240), sessionsHabit("reading", 1))
	press(m, "h")

	if m.habitCursor != 0 {
		t.Fatalf("cursor starts at %d, want 0", m.habitCursor)
	}
	press(m, "j")
	if m.habitCursor != 1 {
		t.Errorf("cursor = %d after j, want 1", m.habitCursor)
	}
	press(m, "j")
	if m.habitCursor != 0 {
		t.Errorf("cursor = %d, want it to wrap to 0", m.habitCursor)
	}
	press(m, "k")
	if m.habitCursor != 1 {
		t.Errorf("cursor = %d after k from the top, want it to wrap to 1", m.habitCursor)
	}
}

func TestSelectingAHabitStartsItAndLogsAgainstIt(t *testing.T) {
	m, st, _ := habitModel(t, minutesHabit("work", 240))
	press(m, "h")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.mode != modeNormal {
		t.Error("selecting a habit did not return to the HUD")
	}
	if m.activeID != "work" {
		t.Fatalf("activeID = %q, want work", m.activeID)
	}
	if !m.timer.Running {
		t.Error("selecting a habit did not start the timer")
	}
	if m.timer.Task != "work" {
		t.Errorf("Task = %q, want the habit's name", m.timer.Task)
	}

	// Run a focus phase to completion and check where it landed.
	tick(m, m.timer.Remaining+time.Second)

	sessions, _, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) == 0 {
		t.Fatal("no session was logged")
	}
	if got := sessions[0].Habit; got != "work" {
		t.Errorf("logged habit = %q, want work", got)
	}
}

func TestHabitProgressReachesTheHUD(t *testing.T) {
	m, _, _ := habitModel(t, minutesHabit("work", 240))
	sizeTo(m, 100, 40)
	press(m, "h")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Complete one 25 minute focus.
	tick(m, m.timer.Remaining+time.Second)

	if got := m.progress["work"].Value; got != 25 {
		t.Errorf("progress Value = %d, want 25 minutes", got)
	}
	out := m.View()
	if !strings.Contains(out, "work") {
		t.Errorf("HUD does not name the active habit:\n%s", out)
	}
	// The habit line spells out progress against the goal.
	if !strings.Contains(out, "25m / 4h") {
		t.Errorf("HUD does not show goal progress:\n%s", out)
	}
}

// Switching habits mid-phase must neither lose the time nor credit it to the
// habit being switched to.
func TestSwitchingHabitsLogsTheAbandonedTimeToTheOldOne(t *testing.T) {
	m, st, _ := habitModel(t, minutesHabit("work", 240), sessionsHabit("reading", 1))
	press(m, "h")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // work
	tick(m, 5*time.Minute)

	press(m, "h")
	press(m, "j")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // reading

	if m.activeID != "reading" {
		t.Fatalf("activeID = %q, want reading", m.activeID)
	}
	sessions, _, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("logged %d sessions, want the abandoned one", len(sessions))
	}
	if sessions[0].Habit != "work" {
		t.Errorf("abandoned time went to %q, want work", sessions[0].Habit)
	}
	if sessions[0].Done {
		t.Error("abandoned time was logged as completed")
	}
	// The new habit starts clean.
	if m.timer.Elapsed() != 0 {
		t.Errorf("new habit started %v in", m.timer.Elapsed())
	}
}

func TestPerHabitTimerLengthsApply(t *testing.T) {
	long := minutesHabit("work", 240)
	long.Focus = 50 * time.Minute
	long.Short = 10 * time.Minute
	m, _, _ := habitModel(t, long, sessionsHabit("reading", 1))

	press(m, "h")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if got := m.timer.Config().Focus; got != 50*time.Minute {
		t.Errorf("focus = %v, want the habit's 50m override", got)
	}
	if got := m.timer.Config().ShortBreak; got != 10*time.Minute {
		t.Errorf("short break = %v, want the habit's 10m override", got)
	}

	// A habit with no overrides inherits the global lengths.
	press(m, "h")
	press(m, "j")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.timer.Config().Focus; got != config.Default().Focus {
		t.Errorf("focus = %v, want the global default %v", got, config.Default().Focus)
	}
}

func TestStatusBarStreakFollowsTheActiveHabit(t *testing.T) {
	m, _, _ := habitModel(t, minutesHabit("work", 240))

	// No habit selected: the global any-session streak.
	if got, want := m.habitStreak(), m.stats.Streak; got != want {
		t.Errorf("streak = %d, want the global %d with no habit active", got, want)
	}

	press(m, "h")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got, want := m.habitStreak(), m.progress["work"].Streak; got != want {
		t.Errorf("streak = %d, want the habit's %d", got, want)
	}
}

func TestEmptyHabitsScreenPointsAtAdding(t *testing.T) {
	m, _, _ := habitModel(t)
	sizeTo(m, 100, 40)

	press(m, "h")
	out := m.View()

	if !strings.Contains(out, "No habits yet") {
		t.Errorf("empty state is missing:\n%s", out)
	}
	if !strings.Contains(out, "add") {
		t.Errorf("empty state does not point at adding one:\n%s", out)
	}
}

// With no habits at all the timer must behave exactly as it did before habits
// existed. This must not become a tool you configure before it runs.
func TestHUDWorksWithNoHabits(t *testing.T) {
	m, _, _ := habitModel(t)
	sizeTo(m, 100, 40)
	m.timer.Task = "free text"

	out := m.View()

	if !strings.Contains(out, "free text") {
		t.Errorf("free-text task line is gone:\n%s", out)
	}
	if _, active := m.activeHabit(); active {
		t.Error("a habit is active despite none being defined")
	}
}

// The label belongs to the habit, so e must not let it drift out of sync.
func TestNoteKeyIsInertWhileAHabitIsActive(t *testing.T) {
	m, _, _ := habitModel(t, minutesHabit("work", 240))
	press(m, "h")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	press(m, "e")

	if m.mode == modeEditTask {
		t.Error("e opened the task editor while a habit was active")
	}
	if m.timer.Task != "work" {
		t.Errorf("Task = %q, want it still tied to the habit", m.timer.Task)
	}
}

func TestActiveHabitSurvivesAQuitAndRelaunch(t *testing.T) {
	m, st, hs := habitModel(t, minutesHabit("work", 240))
	press(m, "h")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tick(m, 5*time.Minute)
	press(m, "q")

	cfg := config.Default()
	cfg.Notify = false
	cfg.Sound = ""
	second, err := New(Options{Config: cfg, Store: st, Habits: hs, StartRunning: true})
	if err != nil {
		t.Fatalf("New() on relaunch error = %v", err)
	}

	if second.activeID != "work" {
		t.Errorf("activeID = %q, want the habit to be resumed too", second.activeID)
	}
}

func TestHabitFlagSelectsByName(t *testing.T) {
	dir := t.TempDir()
	hs := habit.NewStore(filepath.Join(dir, "habits.json"))
	var l habit.List
	l.Add(minutesHabit("Vibe Antarta", 60), time.Now())
	if err := hs.Save(l); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Notify = false
	cfg.Sound = ""
	st := store.New(filepath.Join(dir, "sessions.jsonl"))

	m, err := New(Options{Config: cfg, Store: st, Habits: hs, HabitName: "vibe antarta"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if m.activeID != "vibe-antarta" {
		t.Errorf("activeID = %q, want vibe-antarta", m.activeID)
	}

	// An unknown name fails, and says what would have worked.
	_, err = New(Options{Config: cfg, Store: st, Habits: hs, HabitName: "nope"})
	if err == nil {
		t.Fatal("New() accepted an unknown habit name")
	}
	if !strings.Contains(err.Error(), "Vibe Antarta") {
		t.Errorf("error = %v, want it to list the known habits", err)
	}
}

func TestHabitRowsFitTheFrame(t *testing.T) {
	m, _, _ := habitModel(t,
		minutesHabit("a habit with a very long name indeed", 240),
		sessionsHabit("gym", 3),
	)
	press(m, "h")

	for i, line := range strings.Split(habitsView(theme.Ember, m.habits.Active(), m.progress, 0, ""), "\n") {
		if got := lipgloss.Width(line); got > m.geom.BandW+2 {
			t.Errorf("habit row %d is %d cells wide, wider than the %d-cell frame",
				i, got, m.geom.BandW+2)
		}
	}
}

// --- habit form ---

// typeInto sends a string a rune at a time, as a real keyboard would.
func typeInto(m *Model, s string) {
	for _, r := range s {
		if r == ' ' {
			m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
			continue
		}
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func tab(m *Model)   { m.Update(tea.KeyMsg{Type: tea.KeyTab}) }
func enter(m *Model) { m.Update(tea.KeyMsg{Type: tea.KeyEnter}) }
func esc(m *Model)   { m.Update(tea.KeyMsg{Type: tea.KeyEsc}) }

func TestAddingAHabitThroughTheForm(t *testing.T) {
	m, _, hs := habitModel(t)
	press(m, "h")
	press(m, "a")
	if m.mode != modeHabitForm {
		t.Fatal("a did not open the form")
	}

	typeInto(m, "deep work")
	tab(m)
	typeInto(m, "4h")
	enter(m)

	if m.mode != modeHabits {
		t.Fatalf("mode = %v after saving, want the habit list; error was %q", m.mode, m.habitForm.err)
	}
	active := m.habits.Active()
	if len(active) != 1 {
		t.Fatalf("list holds %d habits, want 1", len(active))
	}
	if active[0].Name != "deep work" {
		t.Errorf("Name = %q, want %q", active[0].Name, "deep work")
	}
	if active[0].Goal.Target != 240 || active[0].Goal.Unit != habit.Minutes {
		t.Errorf("Goal = %+v, want 240 minutes", active[0].Goal)
	}

	// And it persisted, not just landed in memory.
	saved, err := hs.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Active()) != 1 {
		t.Errorf("habits.json holds %d habits, want 1", len(saved.Active()))
	}
}

func TestFormKeepsWhatYouTypedOnAValidationError(t *testing.T) {
	m, _, _ := habitModel(t)
	press(m, "h")
	press(m, "a")

	typeInto(m, "reading")
	tab(m)
	typeInto(m, "banana")
	enter(m)

	if m.mode != modeHabitForm {
		t.Fatal("an invalid goal was accepted")
	}
	if m.habitForm.err == "" {
		t.Error("no error was shown")
	}
	if got := m.habitForm.value(fieldName); got != "reading" {
		t.Errorf("name field = %q, want it preserved", got)
	}
	if len(m.habits.Active()) != 0 {
		t.Error("an invalid habit was saved anyway")
	}
}

func TestFormRejectsAnEmptyName(t *testing.T) {
	m, _, _ := habitModel(t)
	press(m, "h")
	press(m, "a")
	tab(m)
	typeInto(m, "4h")
	enter(m)

	if m.mode != modeHabitForm {
		t.Error("a habit with no name was accepted")
	}
}

func TestFormRejectsABadColour(t *testing.T) {
	m, _, _ := habitModel(t)
	press(m, "h")
	press(m, "a")
	typeInto(m, "work")
	tab(m)
	typeInto(m, "4h")
	tab(m)
	typeInto(m, "reddish")
	enter(m)

	if m.mode != modeHabitForm {
		t.Fatal("an unparseable colour was accepted")
	}
	if !strings.Contains(m.habitForm.err, "colour") {
		t.Errorf("error = %q, want it to name the colour", m.habitForm.err)
	}
}

func TestFormCancelDiscards(t *testing.T) {
	m, _, _ := habitModel(t)
	press(m, "h")
	press(m, "a")
	typeInto(m, "throwaway")
	esc(m)

	if m.mode != modeHabits {
		t.Error("esc did not leave the form")
	}
	if len(m.habits.Active()) != 0 {
		t.Error("a cancelled habit was saved")
	}
}

func TestEditingAHabitKeepsItsIDAndHistory(t *testing.T) {
	m, _, _ := habitModel(t, minutesHabit("work", 240))
	press(m, "h")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // make it active
	press(m, "h")
	press(m, "E")
	if m.mode != modeHabitForm {
		t.Fatal("E did not open the edit form")
	}
	// The form arrives pre-filled.
	if got := m.habitForm.value(fieldName); got != "work" {
		t.Fatalf("name field = %q, want it prefilled", got)
	}
	if got := m.habitForm.value(fieldGoal); got != "4h" {
		t.Errorf("goal field = %q, want %q", got, "4h")
	}

	// Rename it.
	for range "work" {
		m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	typeInto(m, "deep work")
	enter(m)

	h, ok := m.habits.ByID("work")
	if !ok {
		t.Fatal("the habit lost its original ID, which would orphan its sessions")
	}
	if h.Name != "deep work" {
		t.Errorf("Name = %q, want the rename applied", h.Name)
	}
	// The active label follows the rename.
	if m.timer.Task != "deep work" {
		t.Errorf("Task = %q, want it to track the renamed habit", m.timer.Task)
	}
}

// A habit with history is archived so its sessions keep something to point at.
func TestDeletingAHabitWithHistoryArchivesIt(t *testing.T) {
	m, _, hs := habitModel(t, minutesHabit("work", 240))
	press(m, "h")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tick(m, m.timer.Remaining+time.Second) // log a session
	press(m, "h")
	press(m, "d")

	if m.mode != modeConfirm {
		t.Fatal("d did not ask first")
	}
	if !strings.Contains(m.confirm.message, "Archive") {
		t.Errorf("prompt = %q, want it to say archive for a habit with history", m.confirm.message)
	}
	press(m, "y")

	if h, ok := m.habits.ByID("work"); !ok {
		t.Error("the habit was removed outright, orphaning its sessions")
	} else if !h.Archived {
		t.Error("the habit was not archived")
	}
	if len(m.habits.Active()) != 0 {
		t.Error("an archived habit is still in the picker")
	}
	saved, _ := hs.Load()
	if h, ok := saved.ByID("work"); !ok || !h.Archived {
		t.Error("the archive was not persisted")
	}
}

func TestDeletingAnUnusedHabitRemovesIt(t *testing.T) {
	m, _, _ := habitModel(t, minutesHabit("work", 240))
	press(m, "h")
	press(m, "d")

	if !strings.Contains(m.confirm.message, "Delete") {
		t.Errorf("prompt = %q, want it to say delete for a habit with no history", m.confirm.message)
	}
	press(m, "y")

	if _, ok := m.habits.ByID("work"); ok {
		t.Error("the habit survived deletion")
	}
}

func TestConfirmCanBeDeclined(t *testing.T) {
	m, _, _ := habitModel(t, minutesHabit("work", 240))
	press(m, "h")
	press(m, "d")
	press(m, "n")

	if m.mode != modeHabits {
		t.Error("n did not return to the list")
	}
	if _, ok := m.habits.ByID("work"); !ok {
		t.Error("the habit was removed despite declining")
	}
}

func TestRemovingTheActiveHabitClearsIt(t *testing.T) {
	m, _, _ := habitModel(t, minutesHabit("work", 240))
	press(m, "h")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	press(m, "h")
	press(m, "d")
	press(m, "y")

	if m.activeID != "" {
		t.Errorf("activeID = %q, want it cleared with the habit gone", m.activeID)
	}
	if _, active := m.activeHabit(); active {
		t.Error("a removed habit is still reported active")
	}
}

func TestFormAndConfirmScreensRender(t *testing.T) {
	m, _, _ := habitModel(t, minutesHabit("work", 240))
	sizeTo(m, 100, 40)

	press(m, "h")
	press(m, "a")
	out := m.View()
	for _, want := range []string{"NEW HABIT", "name", "goal", "colour", "save", "cancel"} {
		if !strings.Contains(out, want) {
			t.Errorf("form is missing %q:\n%s", want, out)
		}
	}

	esc(m)
	press(m, "d")
	out = m.View()
	for _, want := range []string{"work", "yes", "no"} {
		if !strings.Contains(out, want) {
			t.Errorf("confirm prompt is missing %q:\n%s", want, out)
		}
	}
}

func TestFormFieldNavigationWraps(t *testing.T) {
	f := newHabitForm(nil)
	last := len(f.fields) - 1

	f.move(-1)
	if f.cursor != last {
		t.Errorf("cursor = %d after moving back from the first field, want %d", f.cursor, last)
	}
	f.move(1)
	if f.cursor != 0 {
		t.Errorf("cursor = %d, want it to wrap to 0", f.cursor)
	}
}

func TestEditFormPrefillsOptionalFieldsOnlyWhenSet(t *testing.T) {
	h := minutesHabit("work", 240)
	h.ID = "work"
	h.Focus = 50 * time.Minute

	f := newHabitForm(&h)

	if got := f.value(fieldFocus); got != "50m0s" {
		t.Errorf("focus field = %q, want the override shown", got)
	}
	// An unset break must be blank, not "0s".
	if got := f.value(fieldBreak); got != "" {
		t.Errorf("break field = %q, want it empty", got)
	}
}

// --- contribution bars ---

func TestContributionBarShowsOneCellPerDay(t *testing.T) {
	h := minutesHabit("work", 240)
	h.ID = "work"
	p := store.HabitProgress{Window: 30, Days: make([]store.DayCell, store.ChartDays)}
	for i := range p.Days {
		p.Days[i].Level = store.LevelMet
	}

	line := contributionBar(theme.Ember, h, p, store.ChartDays)

	if got := strings.Count(line, barGlyphs[store.LevelMet]); got != store.ChartDays {
		t.Errorf("bar holds %d met cells, want %d", got, store.ChartDays)
	}
}

func TestContributionBarUsesADistinctGlyphPerLevel(t *testing.T) {
	h := minutesHabit("work", 240)
	h.ID = "work"
	p := store.HabitProgress{
		Window: 4,
		Days: []store.DayCell{
			{Level: store.LevelNone},
			{Level: store.LevelLow},
			{Level: store.LevelMid},
			{Level: store.LevelMet},
		},
	}

	line := contributionBar(theme.Ember, h, p, 4)

	// Each glyph appears, so intensity reads without relying on colour.
	for level, glyph := range barGlyphs {
		if !strings.Contains(line, glyph) {
			t.Errorf("level %d glyph %q is missing from %q", level, glyph, line)
		}
	}
}

func TestContributionBarKeepsTheMostRecentDays(t *testing.T) {
	h := minutesHabit("work", 240)
	h.ID = "work"
	p := store.HabitProgress{Window: 30, Days: make([]store.DayCell, store.ChartDays)}
	// Only the final day was met.
	p.Days[len(p.Days)-1].Level = store.LevelMet

	line := contributionBar(theme.Ember, h, p, 7)

	if !strings.Contains(line, barGlyphs[store.LevelMet]) {
		t.Errorf("a truncated bar dropped the most recent day: %q", line)
	}
}

func TestBarDaysShrinksForNarrowTerminals(t *testing.T) {
	if got := barDays(200); got != barMaxDays {
		t.Errorf("barDays(200) = %d, want the full %d", got, barMaxDays)
	}
	if got := barDays(40); got >= barMaxDays {
		t.Errorf("barDays(40) = %d, want fewer than %d", got, barMaxDays)
	}
	if got := barDays(10); got != barMinDays {
		t.Errorf("barDays(10) = %d, want the floor of %d", got, barMinDays)
	}
	// An unknown width falls back to the full stretch rather than the floor.
	if got := barDays(0); got != barMaxDays {
		t.Errorf("barDays(0) = %d, want %d", got, barMaxDays)
	}
}

// The bars must fit under the frame at any terminal size, which is the failure
// the key legend had.
func TestContributionBarsFitTheFrame(t *testing.T) {
	m, _, _ := habitModel(t, minutesHabit("a very long habit name here", 240), sessionsHabit("gym", 3))

	for _, width := range []int{40, 60, 80, 120} {
		days := barDays(width)
		for _, h := range m.habits.Active() {
			line := contributionBar(theme.Ember, h, m.progress[h.ID], days)
			if got := lipgloss.Width(line); got > width {
				t.Errorf("at width %d the bar for %q is %d cells wide", width, h.Name, got)
			}
		}
	}
}

func TestStatsScreenShowsABarPerHabit(t *testing.T) {
	m, _, _ := habitModel(t, minutesHabit("work", 240), sessionsHabit("reading", 1))
	sizeTo(m, 100, 40)

	press(m, "t")
	out := m.View()

	for _, want := range []string{"LAST", "work", "reading", "goal met"} {
		if !strings.Contains(out, want) {
			t.Errorf("stats screen is missing %q:\n%s", want, out)
		}
	}
}

// Without habits there are no goals to shade against, so the plain activity
// chart is the honest fallback.
func TestStatsScreenFallsBackToTheChartWithNoHabits(t *testing.T) {
	m, _, _ := habitModel(t)
	sizeTo(m, 100, 40)

	press(m, "t")
	out := m.View()

	if strings.Contains(out, "goal met") {
		t.Error("the goal-shading key is shown with no goals defined")
	}
	if !strings.Contains(out, "Last") {
		t.Errorf("the fallback chart is missing:\n%s", out)
	}
}

func TestStatsScreenReportsZenSeparately(t *testing.T) {
	m, _, _ := habitModel(t, minutesHabit("work", 240))
	sizeTo(m, 100, 40)
	m.stats.ZenWeekMins = 80

	press(m, "t")
	out := m.View()

	if !strings.Contains(out, "Zen") {
		t.Errorf("zen time is not accounted for:\n%s", out)
	}
	if !strings.Contains(out, "1h 20m") {
		t.Errorf("zen total is wrong:\n%s", out)
	}
}

// --- zen mode ---

// The five-glyph invariant is what keeps zen from reflowing the HUD.
func TestElapsedClockIsAlwaysFiveGlyphs(t *testing.T) {
	for _, d := range []time.Duration{
		0, time.Second, 59 * time.Second,
		time.Minute, 59*time.Minute + 59*time.Second,
		time.Hour, time.Hour + time.Minute,
		9 * time.Hour, 25 * time.Hour, 99 * time.Hour,
		200 * time.Hour, // clamped rather than growing a glyph
		-time.Second,
	} {
		got := FormatElapsed(d)
		if len([]rune(got)) != 5 {
			t.Errorf("FormatElapsed(%v) = %q, which is %d glyphs, not 5", d, got, len([]rune(got)))
		}
		if w, _ := clockCanvasSize(got); w != mustClockWidth(t) {
			t.Errorf("FormatElapsed(%v) = %q renders %d wide, want %d", d, got, w, mustClockWidth(t))
		}
	}
}

func mustClockWidth(t *testing.T) int {
	t.Helper()
	w, _ := clockCanvasSize("00:00")
	return w
}

// The units switch at exactly an hour, since that is where MM:SS would need a
// sixth glyph.
func TestElapsedClockSwitchesUnitsAtOneHour(t *testing.T) {
	if got, want := FormatElapsed(59*time.Minute+59*time.Second), "59:59"; got != want {
		t.Errorf("just under an hour = %q, want %q", got, want)
	}
	if got, want := FormatElapsed(time.Hour), "01:00"; got != want {
		t.Errorf("exactly an hour = %q, want %q (HH:MM)", got, want)
	}
	if got, want := FormatElapsed(time.Hour+30*time.Minute), "01:30"; got != want {
		t.Errorf("90 minutes = %q, want %q", got, want)
	}
}

// Five glyphs cannot say whether 01:30 is an hour and a half or ninety seconds,
// so the spelled-out form has to be unambiguous.
func TestSpellElapsed(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{5 * time.Second, "5s"},
		{90 * time.Second, "1m 30s"},
		{time.Hour + 40*time.Minute + 12*time.Second, "1h 40m 12s"},
		{-time.Second, "0s"},
	}
	for _, tt := range tests {
		if got := SpellElapsed(tt.d); got != tt.want {
			t.Errorf("SpellElapsed(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func zenModel(t *testing.T) (*Model, *store.Store) {
	t.Helper()
	cfg := config.Default()
	cfg.Notify = false
	cfg.Sound = ""
	st := store.New(filepath.Join(t.TempDir(), "sessions.jsonl"))
	m, err := New(Options{Config: cfg, Store: st, StartRunning: true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return m, st
}

func TestZenTogglesAndCountsUp(t *testing.T) {
	m, _ := zenModel(t)
	sizeTo(m, 100, 40)

	press(m, "z")
	if !m.zen {
		t.Fatal("z did not enter zen")
	}
	if m.timer.Running {
		t.Error("the pomodoro is still running during zen")
	}

	tick(m, 90*time.Second)
	if m.zenElapsed != 90*time.Second {
		t.Errorf("zenElapsed = %v, want 90s", m.zenElapsed)
	}
	if got := m.clockText(); got != "01:30" {
		t.Errorf("clock = %q, want 01:30 counting up", got)
	}
}

// Leaving zen must hand the pomodoro back exactly as it was.
func TestZenLeavesThePomodoroUntouched(t *testing.T) {
	m, _ := zenModel(t)
	tick(m, 5*time.Minute) // run some focus first
	before := m.timer.Snapshot()

	press(m, "z")
	tick(m, 20*time.Minute) // long enough to have finished the phase
	press(m, "z")

	after := m.timer.Snapshot()
	if after.Phase != before.Phase {
		t.Errorf("phase moved from %v to %v during zen", before.Phase, after.Phase)
	}
	if after.Remaining != before.Remaining {
		t.Errorf("remaining moved from %v to %v during zen", before.Remaining, after.Remaining)
	}
	if after.CycleIndex != before.CycleIndex {
		t.Errorf("cycle moved from %d to %d during zen", before.CycleIndex, after.CycleIndex)
	}
}

func TestZenLogsWithNoHabitAndEarnsXP(t *testing.T) {
	m, st := zenModel(t)
	press(m, "z")
	tick(m, 30*time.Minute)
	press(m, "z")

	if m.zen {
		t.Error("z did not leave zen")
	}
	sessions, _, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("logged %d sessions, want 1", len(sessions))
	}
	got := sessions[0]
	if got.Phase != store.PhaseZen {
		t.Errorf("phase = %q, want %q", got.Phase, store.PhaseZen)
	}
	if got.Habit != "" {
		t.Errorf("habit = %q, want empty — zen belongs to no goal", got.Habit)
	}
	if got.Mins != 30 {
		t.Errorf("mins = %d, want 30", got.Mins)
	}
	if m.stats.XP != 30 {
		t.Errorf("XP = %d, want 30 — zen is real work", m.stats.XP)
	}
}

func TestZenMovesNoHabit(t *testing.T) {
	m, _, _ := habitModel(t, minutesHabit("work", 240))
	press(m, "h")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // work active

	press(m, "z")
	tick(m, 45*time.Minute)
	press(m, "z")

	if got := m.progress["work"].Value; got != 0 {
		t.Errorf("work progress = %d, want 0 — zen must not feed a habit", got)
	}
}

func TestZenPausesWithSpace(t *testing.T) {
	m, _ := zenModel(t)
	press(m, "z")
	tick(m, time.Minute)

	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if m.zenRunning {
		t.Fatal("space did not pause zen")
	}
	tick(m, 5*time.Minute)
	if m.zenElapsed != time.Minute {
		t.Errorf("zenElapsed = %v, want it frozen at 1m", m.zenElapsed)
	}

	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	tick(m, time.Minute)
	if m.zenElapsed != 2*time.Minute {
		t.Errorf("zenElapsed = %v, want 2m after resuming", m.zenElapsed)
	}
}

// Skip and reset belong to the pomodoro; there is no phase to skip in zen.
func TestSkipAndResetAreInertInZen(t *testing.T) {
	m, st := zenModel(t)
	press(m, "z")
	tick(m, time.Minute)

	press(m, "s")
	press(m, "r")

	if !m.zen {
		t.Error("s or r knocked the model out of zen")
	}
	sessions, _, _ := st.Load()
	if len(sessions) != 0 {
		t.Errorf("logged %d sessions, want none", len(sessions))
	}
}

func TestQuittingLogsAZenStretchInProgress(t *testing.T) {
	m, st := zenModel(t)
	press(m, "z")
	tick(m, 20*time.Minute)
	press(m, "q")

	sessions, _, _ := st.Load()
	if len(sessions) != 1 {
		t.Fatalf("logged %d sessions, want the zen stretch to be kept", len(sessions))
	}
	if sessions[0].Phase != store.PhaseZen {
		t.Errorf("phase = %q, want zen", sessions[0].Phase)
	}
}

func TestZenSurvivesAQuitAndResumes(t *testing.T) {
	cfg := config.Default()
	cfg.Notify = false
	cfg.Sound = ""
	st := store.New(filepath.Join(t.TempDir(), "sessions.jsonl"))

	first, err := New(Options{Config: cfg, Store: st, StartRunning: true, Zen: true})
	if err != nil {
		t.Fatal(err)
	}
	tick(first, 10*time.Minute)
	// Save without stopping, the way a crash or a bare save would.
	first.saveResume()

	second, err := New(Options{Config: cfg, Store: st, StartRunning: true})
	if err != nil {
		t.Fatal(err)
	}
	if !second.zen {
		t.Fatal("zen did not resume")
	}
	if second.zenElapsed != 10*time.Minute {
		t.Errorf("zenElapsed = %v, want 10m", second.zenElapsed)
	}
}

func TestZenViewSpellsOutTheTimeAndDropsPomodoroChrome(t *testing.T) {
	m, _ := zenModel(t)
	sizeTo(m, 100, 40)
	press(m, "z")
	tick(m, time.Hour+40*time.Minute+12*time.Second)

	out := m.View()

	if !strings.Contains(out, "1h 40m 12s") {
		t.Errorf("zen view does not spell the elapsed time out:\n%s", out)
	}
	if !strings.Contains(out, "ZEN") {
		t.Errorf("zen view does not say it is in zen:\n%s", out)
	}
	// The cycle dots and phase label belong to the pomodoro.
	if strings.Contains(out, "FOCUS") {
		t.Error("zen view still shows the focus phase label")
	}
	// The legend swaps to zen's keys.
	if !strings.Contains(out, "stop") {
		t.Errorf("zen legend is missing:\n%s", out)
	}
}

func TestZenUsesItsOwnPaletteAndNoSteam(t *testing.T) {
	m, _ := zenModel(t)
	press(m, "z")

	if got := m.paletteTarget().Name; got != "zen" {
		t.Errorf("palette target = %q, want zen", got)
	}
	if got := m.breath().steamHz; got != 0 {
		t.Errorf("zen emits steam at %v Hz, want none", got)
	}
	if m.breath().period < breathFor(timer.Focus, true).period {
		t.Error("zen breathes faster than focus; it should be the calmest")
	}
}

// A count-up clock must never enter the alert state: there is no boundary.
func TestZenNeverAlerts(t *testing.T) {
	m, _ := zenModel(t)
	sizeTo(m, 100, 40)
	press(m, "z")

	// Run right past where a pomodoro would be shaking.
	tick(m, 3*time.Hour)
	lines := strings.Split(m.View(), "\n")
	want := lipgloss.Width(lines[0])
	for i, l := range lines[:len(lines)-helpRows] {
		if got := lipgloss.Width(l); got != want {
			t.Errorf("line %d is %d cells wide, want %d — zen reflowed the HUD", i, got, want)
		}
	}
}

// A habit's focus override lives in the timer, not the global config. Reading
// the wrong one logged 25 minutes for a completed 50 minute session, so the
// habit's progress advanced by half of what was actually worked.
func TestCompletedPhaseLogsTheHabitsOwnLength(t *testing.T) {
	long := minutesHabit("work", 240)
	long.Focus = 50 * time.Minute
	m, st, _ := habitModel(t, long)

	press(m, "h")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.timer.Config().Focus; got != 50*time.Minute {
		t.Fatalf("timer focus = %v, want the 50m override", got)
	}

	tick(m, m.timer.Remaining+time.Second)

	sessions, _, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) == 0 {
		t.Fatal("no session was logged")
	}
	if got := sessions[0].Mins; got != 50 {
		t.Errorf("logged %d minutes, want the habit's 50", got)
	}
	if got := m.progress["work"].Value; got != 50 {
		t.Errorf("habit progress = %d, want 50", got)
	}
}

// The same applies to a break with its own override.
func TestCompletedBreakLogsTheHabitsOwnLength(t *testing.T) {
	h := minutesHabit("work", 240)
	h.Focus = time.Minute
	h.Short = 10 * time.Minute
	m, st, _ := habitModel(t, h)

	press(m, "h")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tick(m, m.timer.Remaining+time.Second) // finish focus, break auto-starts
	tick(m, m.timer.Remaining+time.Second) // finish the break

	sessions, _, _ := st.Load()
	if len(sessions) < 2 {
		t.Fatalf("logged %d sessions, want the focus and the break", len(sessions))
	}
	if got := sessions[1].Mins; got != 10 {
		t.Errorf("break logged %d minutes, want the habit's 10", got)
	}
}

// The goal syntax is the one thing in the form that has to be learned, so the
// examples must be visible while the field is being filled in — the earlier
// version hid the hint the moment the field took focus.
func TestGoalFieldShowsItsExamplesWhileFocused(t *testing.T) {
	f := newHabitForm(nil)
	f.cursor = fieldGoal

	out := f.view(theme.Ember)

	for _, want := range []string{
		"1 session", "3 sessions", "4h", "90m", "/ week", "for example",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("goal help is missing %q:\n%s", want, out)
		}
	}
	// And still visible once something has been typed.
	f.fields[fieldGoal].value = "4h"
	if !strings.Contains(f.view(theme.Ember), "for example") {
		t.Error("the examples vanished once the field had a value")
	}
}

func TestGoalPreviewInterpretsWhatWasTyped(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"4h", "4h a day"},
		{"90m", "1h 30m a day"},
		{"1 session", "1 session a day"},
		{"3", "3 sessions a day"},
		{"3 sessions / week", "3 sessions a week"},
		{"10h / week", "10h a week"},
		{"banana", "not a goal yet"},
		{"4", "4 sessions a day"},
	}
	for _, tt := range tests {
		if got := previewGoal(tt.in); got != tt.want {
			t.Errorf("previewGoal(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestGoalPreviewReachesTheForm(t *testing.T) {
	f := newHabitForm(nil)
	f.cursor = fieldGoal
	f.fields[fieldGoal].value = "3 sessions / week"

	out := f.view(theme.Ember)

	if !strings.Contains(out, "3 sessions a week") {
		t.Errorf("the form does not say what the goal was read as:\n%s", out)
	}
}

// Every field earns some guidance; an unexplained field is a field people
// leave blank because they do not know what it wants.
func TestEveryFormFieldHasAHintAndExamples(t *testing.T) {
	for i, fl := range newHabitForm(nil).fields {
		if fl.hint == "" {
			t.Errorf("field %d (%q) has no placeholder", i, fl.label)
		}
		if len(fl.help) == 0 {
			t.Errorf("field %d (%q) has no examples", i, fl.label)
		}
	}
}

func TestOptionalFieldsSayTheyAreOptional(t *testing.T) {
	f := newHabitForm(nil)
	for _, i := range []int{fieldColor, fieldFocus, fieldBreak} {
		if !strings.Contains(f.fields[i].hint, "optional") {
			t.Errorf("field %q does not say it is optional", f.fields[i].label)
		}
	}
	// And the required ones do not.
	for _, i := range []int{fieldName, fieldGoal} {
		if strings.Contains(f.fields[i].hint, "optional") {
			t.Errorf("required field %q is labelled optional", f.fields[i].label)
		}
	}
}

func TestGoalDescribe(t *testing.T) {
	tests := []struct {
		goal habit.Goal
		want string
	}{
		{habit.Goal{Target: 1, Unit: habit.Sessions, Period: habit.Daily}, "1 session a day"},
		{habit.Goal{Target: 3, Unit: habit.Sessions, Period: habit.Daily}, "3 sessions a day"},
		{habit.Goal{Target: 240, Unit: habit.Minutes, Period: habit.Daily}, "4h a day"},
		{habit.Goal{Target: 3, Unit: habit.Sessions, Period: habit.Weekly}, "3 sessions a week"},
		{habit.Goal{Target: 600, Unit: habit.Minutes, Period: habit.Weekly}, "10h a week"},
	}
	for _, tt := range tests {
		if got := tt.goal.Describe(); got != tt.want {
			t.Errorf("Describe(%+v) = %q, want %q", tt.goal, got, tt.want)
		}
	}
}
