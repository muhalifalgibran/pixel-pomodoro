// Package timer holds the pomodoro state machine. It never reads the clock and
// never touches the filesystem: callers feed it elapsed durations. That keeps
// every transition reachable from a test without sleeping.
package timer

import (
	"fmt"
	"time"
)

// Phase is what the timer is currently counting down.
type Phase int

const (
	Focus Phase = iota
	ShortBreak
	LongBreak
)

func (p Phase) String() string {
	switch p {
	case Focus:
		return "focus"
	case ShortBreak:
		return "short break"
	case LongBreak:
		return "long break"
	default:
		return fmt.Sprintf("phase(%d)", int(p))
	}
}

// IsBreak reports whether the phase is either kind of break.
func (p Phase) IsBreak() bool { return p == ShortBreak || p == LongBreak }

// Config is the timing policy. Durations must be positive.
type Config struct {
	Focus           time.Duration
	ShortBreak      time.Duration
	LongBreak       time.Duration
	LongBreakEvery  int // focus sessions per long break
	AutoStartBreaks bool
	AutoStartFocus  bool
}

// DefaultConfig is the classic 25/5/15 cycle.
func DefaultConfig() Config {
	return Config{
		Focus:           25 * time.Minute,
		ShortBreak:      5 * time.Minute,
		LongBreak:       15 * time.Minute,
		LongBreakEvery:  4,
		AutoStartBreaks: true,
		AutoStartFocus:  false,
	}
}

// Validate reports why a config cannot drive the state machine. A zero
// duration would let Advance spin forever, so it is rejected outright.
func (c Config) Validate() error {
	for _, d := range []struct {
		name string
		val  time.Duration
	}{
		{"focus", c.Focus},
		{"short_break", c.ShortBreak},
		{"long_break", c.LongBreak},
	} {
		if d.val <= 0 {
			return fmt.Errorf("%s duration must be positive, got %s", d.name, d.val)
		}
	}
	if c.LongBreakEvery < 1 {
		return fmt.Errorf("long_break_every must be at least 1, got %d", c.LongBreakEvery)
	}
	return nil
}

// Duration is the configured length of a phase.
func (c Config) Duration(p Phase) time.Duration {
	switch p {
	case ShortBreak:
		return c.ShortBreak
	case LongBreak:
		return c.LongBreak
	default:
		return c.Focus
	}
}

// Event records a phase boundary. Completed distinguishes a phase that ran out
// from one the user skipped, which matters for both the session log and
// whether a notification should fire.
type Event struct {
	Ended     Phase
	Next      Phase
	Completed bool
}

// State is the live timer.
type State struct {
	cfg Config

	Phase     Phase
	Remaining time.Duration
	Running   bool

	// CycleIndex counts completed focus sessions in the current set and
	// resets after a long break. Completed counts them for the whole run.
	CycleIndex int
	Completed  int

	Task string
}

// maxTransitionsPerAdvance bounds the catch-up loop. Reaching it means the
// machine was handed an absurd elapsed value; stopping beats spinning.
const maxTransitionsPerAdvance = 64

// New builds a timer sitting at the start of a focus phase, not yet running.
func New(cfg Config) (*State, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &State{cfg: cfg, Phase: Focus, Remaining: cfg.Focus}, nil
}

// Config returns the timing policy in force.
func (s *State) Config() Config { return s.cfg }

// Total is the full length of the current phase.
func (s *State) Total() time.Duration { return s.cfg.Duration(s.Phase) }

// Elapsed is how far into the current phase the timer has run.
func (s *State) Elapsed() time.Duration { return s.Total() - s.Remaining }

// Progress is Elapsed as a fraction in [0,1].
func (s *State) Progress() float64 {
	total := s.Total()
	if total <= 0 {
		return 0
	}
	p := float64(s.Elapsed()) / float64(total)
	switch {
	case p < 0:
		return 0
	case p > 1:
		return 1
	}
	return p
}

// Advance moves the timer forward by elapsed and returns every phase boundary
// crossed, oldest first. A paused timer ignores elapsed entirely.
//
// Callers must pass real wall-clock deltas rather than counting ticks: ticks
// drift, and a suspended laptop hands back one enormous delta that has to
// resolve into several transitions at once.
func (s *State) Advance(elapsed time.Duration) []Event {
	if !s.Running || elapsed <= 0 {
		return nil
	}
	var events []Event
	for n := 0; elapsed > 0 && s.Running; n++ {
		if n >= maxTransitionsPerAdvance {
			break
		}
		if elapsed < s.Remaining {
			s.Remaining -= elapsed
			return events
		}
		elapsed -= s.Remaining
		events = append(events, s.endPhase(true))
	}
	return events
}

// Skip ends the current phase early. The phase is recorded as not completed,
// so it earns no XP and fires no completion sound.
func (s *State) Skip() Event { return s.endPhase(false) }

// Toggle pauses or resumes.
func (s *State) Toggle() { s.Running = !s.Running }

// Reset restarts the current phase without disturbing the cycle count. It is
// the "I got interrupted, start this one over" action, not a session wipe.
func (s *State) Reset() {
	s.Remaining = s.Total()
	s.Running = false
}

// endPhase performs the transition and returns the event describing it.
func (s *State) endPhase(completed bool) Event {
	ended := s.Phase

	// Only a focus session that actually ran to the end advances the cycle.
	// Skipping one must not earn progress toward a long break.
	if ended == Focus && completed {
		s.Completed++
		s.CycleIndex++
	}

	next := Focus
	if ended == Focus {
		if s.CycleIndex > 0 && s.CycleIndex%s.cfg.LongBreakEvery == 0 {
			next = LongBreak
		} else {
			next = ShortBreak
		}
	} else if ended == LongBreak {
		s.CycleIndex = 0
	}

	s.Phase = next
	s.Remaining = s.cfg.Duration(next)
	if next == Focus {
		s.Running = s.cfg.AutoStartFocus
	} else {
		s.Running = s.cfg.AutoStartBreaks
	}

	return Event{Ended: ended, Next: next, Completed: completed}
}
