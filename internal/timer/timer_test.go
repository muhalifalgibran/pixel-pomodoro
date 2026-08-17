package timer

import (
	"testing"
	"time"
)

func testConfig() Config {
	return Config{
		Focus:           25 * time.Minute,
		ShortBreak:      5 * time.Minute,
		LongBreak:       15 * time.Minute,
		LongBreakEvery:  4,
		AutoStartBreaks: true,
		AutoStartFocus:  true,
	}
}

func newRunning(t *testing.T, cfg Config) *State {
	t.Helper()
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	s.Running = true
	return s
}

func TestNewStartsPausedAtFullFocus(t *testing.T) {
	s, err := New(testConfig())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if s.Phase != Focus {
		t.Errorf("Phase = %v, want Focus", s.Phase)
	}
	if s.Remaining != 25*time.Minute {
		t.Errorf("Remaining = %v, want 25m", s.Remaining)
	}
	if s.Running {
		t.Error("a fresh timer should not be running")
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{"default is valid", func(*Config) {}, false},
		{"zero focus", func(c *Config) { c.Focus = 0 }, true},
		{"negative short break", func(c *Config) { c.ShortBreak = -time.Minute }, true},
		{"zero long break", func(c *Config) { c.LongBreak = 0 }, true},
		{"zero long_break_every", func(c *Config) { c.LongBreakEvery = 0 }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testConfig()
			tt.mutate(&c)
			err := c.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestAdvanceIgnoredWhilePaused(t *testing.T) {
	s, _ := New(testConfig())
	events := s.Advance(10 * time.Minute)

	if len(events) != 0 {
		t.Errorf("paused timer emitted %d events, want 0", len(events))
	}
	if s.Remaining != 25*time.Minute {
		t.Errorf("paused timer moved to %v, want 25m", s.Remaining)
	}
}

func TestAdvanceCountsDownWithoutTransition(t *testing.T) {
	s := newRunning(t, testConfig())

	if events := s.Advance(10 * time.Minute); len(events) != 0 {
		t.Errorf("got %d events mid-phase, want 0", len(events))
	}
	if s.Remaining != 15*time.Minute {
		t.Errorf("Remaining = %v, want 15m", s.Remaining)
	}
}

func TestAdvanceExactlyToZeroEndsThePhase(t *testing.T) {
	s := newRunning(t, testConfig())

	events := s.Advance(25 * time.Minute)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].Ended != Focus || events[0].Next != ShortBreak || !events[0].Completed {
		t.Errorf("event = %+v, want focus -> short break, completed", events[0])
	}
	if s.Phase != ShortBreak {
		t.Errorf("Phase = %v, want ShortBreak", s.Phase)
	}
	if s.Remaining != 5*time.Minute {
		t.Errorf("Remaining = %v, want a full 5m break", s.Remaining)
	}
}

func TestFullSetReachesLongBreakOnTheFourthFocus(t *testing.T) {
	s := newRunning(t, testConfig())

	var order []string
	// Four focus sessions and the breaks between them.
	for i := 0; i < 8; i++ {
		for _, e := range s.Advance(s.Remaining) {
			order = append(order, e.Ended.String()+" -> "+e.Next.String())
		}
	}

	want := []string{
		"focus -> short break",
		"short break -> focus",
		"focus -> short break",
		"short break -> focus",
		"focus -> short break",
		"short break -> focus",
		"focus -> long break",
		"long break -> focus",
	}
	if len(order) != len(want) {
		t.Fatalf("got %d transitions %v, want %d", len(order), order, len(want))
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("transition %d = %q, want %q", i, order[i], want[i])
		}
	}
	if s.CycleIndex != 0 {
		t.Errorf("CycleIndex = %d after a long break, want 0", s.CycleIndex)
	}
	if s.Completed != 4 {
		t.Errorf("Completed = %d, want 4", s.Completed)
	}
}

func TestAdvanceAcrossSleepFiresEveryCrossedBoundary(t *testing.T) {
	s := newRunning(t, testConfig())

	// The laptop slept for an hour, which lands exactly on a boundary:
	// 25m focus + 5m break + 25m focus + 5m break = 60m.
	events := s.Advance(time.Hour)

	if len(events) != 4 {
		t.Fatalf("got %d events for a 1h jump, want 4: %+v", len(events), events)
	}
	if s.Phase != Focus {
		t.Errorf("Phase = %v, want Focus", s.Phase)
	}
	if s.Completed != 2 {
		t.Errorf("Completed = %d, want 2", s.Completed)
	}
}

func TestAdvanceIsBoundedForAbsurdInput(t *testing.T) {
	s := newRunning(t, testConfig())

	events := s.Advance(1000 * time.Hour)

	if len(events) > maxTransitionsPerAdvance {
		t.Errorf("got %d events, want at most %d", len(events), maxTransitionsPerAdvance)
	}
	if s.Remaining < 0 {
		t.Errorf("Remaining went negative: %v", s.Remaining)
	}
}

func TestAdvanceStopsAtABoundaryWhenAutoStartIsOff(t *testing.T) {
	cfg := testConfig()
	cfg.AutoStartBreaks = false
	s := newRunning(t, cfg)

	// An hour of elapsed time, but the break does not auto-start, so the
	// surplus must be discarded rather than eaten by later phases.
	events := s.Advance(time.Hour)

	if len(events) != 1 {
		t.Fatalf("got %d events, want 1 — the timer should have stopped at the break", len(events))
	}
	if s.Running {
		t.Error("timer auto-started a break with auto_start_breaks off")
	}
	if s.Remaining != 5*time.Minute {
		t.Errorf("Remaining = %v, want a full 5m break", s.Remaining)
	}
}

func TestAutoStartFocusOffPausesAfterABreak(t *testing.T) {
	cfg := testConfig()
	cfg.AutoStartFocus = false
	s := newRunning(t, cfg)

	s.Advance(25 * time.Minute) // focus -> break, break auto-starts
	if !s.Running {
		t.Fatal("break should have auto-started")
	}
	s.Advance(5 * time.Minute) // break -> focus, focus must not auto-start

	if s.Running {
		t.Error("focus auto-started with auto_start_focus off")
	}
	if s.Phase != Focus {
		t.Errorf("Phase = %v, want Focus", s.Phase)
	}
}

func TestSkipDoesNotEarnCycleProgress(t *testing.T) {
	s := newRunning(t, testConfig())

	e := s.Skip()

	if e.Completed {
		t.Error("skipped phase reported as completed")
	}
	if s.Completed != 0 {
		t.Errorf("Completed = %d after a skip, want 0", s.Completed)
	}
	if s.CycleIndex != 0 {
		t.Errorf("CycleIndex = %d after a skip, want 0", s.CycleIndex)
	}
	if s.Phase != ShortBreak {
		t.Errorf("Phase = %v, want ShortBreak", s.Phase)
	}
}

func TestSkippingEveryFocusNeverReachesALongBreak(t *testing.T) {
	s := newRunning(t, testConfig())

	for i := 0; i < 10; i++ {
		if e := s.Skip(); e.Next == LongBreak {
			t.Fatalf("skip %d produced a long break; skipped work should not earn one", i)
		}
	}
}

func TestTogglePausesAndResumes(t *testing.T) {
	s := newRunning(t, testConfig())

	s.Toggle()
	if s.Running {
		t.Error("Toggle did not pause a running timer")
	}
	s.Advance(time.Minute)
	if s.Remaining != 25*time.Minute {
		t.Errorf("paused timer advanced to %v", s.Remaining)
	}

	s.Toggle()
	if !s.Running {
		t.Error("Toggle did not resume a paused timer")
	}
	s.Advance(time.Minute)
	if s.Remaining != 24*time.Minute {
		t.Errorf("Remaining = %v, want 24m", s.Remaining)
	}
}

func TestResetRestartsThePhaseAndKeepsTheCycle(t *testing.T) {
	s := newRunning(t, testConfig())
	s.Advance(25 * time.Minute) // complete one focus, land in a break
	s.Advance(2 * time.Minute)

	s.Reset()

	if s.Phase != ShortBreak {
		t.Errorf("Phase = %v, want the phase to survive a reset", s.Phase)
	}
	if s.Remaining != 5*time.Minute {
		t.Errorf("Remaining = %v, want a full 5m", s.Remaining)
	}
	if s.Running {
		t.Error("Reset should leave the timer paused")
	}
	if s.Completed != 1 {
		t.Errorf("Completed = %d, want the cycle count to survive a reset", s.Completed)
	}
}

func TestProgress(t *testing.T) {
	s := newRunning(t, testConfig())

	if got := s.Progress(); got != 0 {
		t.Errorf("Progress() at the start = %v, want 0", got)
	}
	s.Advance(5 * time.Minute)
	if got, want := s.Progress(), 0.2; got != want {
		t.Errorf("Progress() = %v, want %v", got, want)
	}
	if got, want := s.Elapsed(), 5*time.Minute; got != want {
		t.Errorf("Elapsed() = %v, want %v", got, want)
	}
}

func TestLongBreakEveryOne(t *testing.T) {
	cfg := testConfig()
	cfg.LongBreakEvery = 1
	s := newRunning(t, cfg)

	events := s.Advance(cfg.Focus)

	if len(events) != 1 || events[0].Next != LongBreak {
		t.Errorf("events = %+v, want a single focus -> long break", events)
	}
}

func TestPhaseString(t *testing.T) {
	for _, tt := range []struct {
		p    Phase
		want string
	}{
		{Focus, "focus"},
		{ShortBreak, "short break"},
		{LongBreak, "long break"},
	} {
		if got := tt.p.String(); got != tt.want {
			t.Errorf("Phase(%d).String() = %q, want %q", int(tt.p), got, tt.want)
		}
	}
	if Focus.IsBreak() {
		t.Error("Focus.IsBreak() = true")
	}
	if !ShortBreak.IsBreak() || !LongBreak.IsBreak() {
		t.Error("breaks should report IsBreak() = true")
	}
}

func TestParsePhaseRoundTripsString(t *testing.T) {
	for _, p := range []Phase{Focus, ShortBreak, LongBreak} {
		got, ok := ParsePhase(p.String())
		if !ok {
			t.Errorf("ParsePhase(%q) reported the phase as unknown", p.String())
			continue
		}
		if got != p {
			t.Errorf("ParsePhase(%q) = %v, want %v", p.String(), got, p)
		}
	}
	if _, ok := ParsePhase("siesta"); ok {
		t.Error("ParsePhase accepted a phase that does not exist")
	}
}

func TestSnapshotAndRestoreRoundTrip(t *testing.T) {
	s := newRunning(t, testConfig())
	s.Task = "render loop"
	s.Advance(9 * time.Minute)

	snap := s.Snapshot()

	restored, _ := New(testConfig())
	if err := restored.Restore(snap); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	if restored.Phase != s.Phase || restored.Remaining != s.Remaining ||
		restored.Running != s.Running || restored.CycleIndex != s.CycleIndex ||
		restored.Completed != s.Completed || restored.Task != s.Task {
		t.Errorf("restored %+v, want it to match %+v", restored, s)
	}
}

func TestRestoreRejectsAnImpossiblePosition(t *testing.T) {
	tests := []struct {
		name string
		snap Snapshot
	}{
		{"zero remaining", Snapshot{Phase: Focus, Remaining: 0}},
		{"negative remaining", Snapshot{Phase: Focus, Remaining: -time.Minute}},
		{"unknown phase", Snapshot{Phase: Phase(42), Remaining: time.Minute}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := New(testConfig())
			if err := s.Restore(tt.snap); err == nil {
				t.Error("Restore() accepted an impossible position")
			}
		})
	}
}

// Shortening a phase in config must not strand a saved session above the new
// length; clamping lets the user resume at the new duration.
func TestRestoreClampsToTheCurrentPhaseLength(t *testing.T) {
	cfg := testConfig()
	cfg.Focus = 10 * time.Minute
	s, _ := New(cfg)

	if err := s.Restore(Snapshot{Phase: Focus, Remaining: 25 * time.Minute}); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if s.Remaining != 10*time.Minute {
		t.Errorf("Remaining = %v, want it clamped to the configured 10m", s.Remaining)
	}
}

// A hand-edited cycle index must not put the machine somewhere it can never
// reach on its own.
func TestRestoreNormalisesAnOutOfRangeCycle(t *testing.T) {
	s, _ := New(testConfig()) // long_break_every = 4

	if err := s.Restore(Snapshot{Phase: Focus, Remaining: time.Minute, CycleIndex: 99, Completed: -3}); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if s.CycleIndex != 0 {
		t.Errorf("CycleIndex = %d, want it normalised to 0", s.CycleIndex)
	}
	if s.Completed != 0 {
		t.Errorf("Completed = %d, want it normalised to 0", s.Completed)
	}
}
