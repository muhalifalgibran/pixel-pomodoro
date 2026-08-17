package ui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/config"
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

	// The last line is the help text outside the frame; every framed line
	// must match, or the border will not line up.
	want := lipgloss.Width(lines[0])
	for i, l := range lines[:len(lines)-1] {
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
	for i, l := range lines[:len(lines)-1] {
		if got := lipgloss.Width(l); got != want {
			t.Errorf("line %d is %d cells wide, want %d", i, got, want)
		}
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

	tick(m, time.Second)
	if m.steam.Count() == 0 {
		t.Error("focus produced no steam")
	}

	press(m, "s") // move to a break
	tick(m, time.Second)
	tick(m, time.Second)
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
