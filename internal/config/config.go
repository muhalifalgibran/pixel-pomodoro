// Package config resolves settings from three sources, in increasing
// precedence: built-in defaults, the TOML config file, then CLI flags.
package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/paths"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/theme"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/timer"
)

// Config is the fully resolved settings.
type Config struct {
	Focus           time.Duration
	ShortBreak      time.Duration
	LongBreak       time.Duration
	LongBreakEvery  int
	AutoStartBreaks bool
	AutoStartFocus  bool

	ShowSeconds bool
	Scanlines   bool
	Theme       string

	// Resume restores the timer to wherever it was when you last quit.
	Resume bool

	Notify bool
	Sound  string
}

// defaultSound is macOS's built-in chime. On other platforms notify is a
// no-op, so the value is simply unused rather than wrong.
const defaultSound = "/System/Library/Sounds/Glass.aiff"

// Default is the classic 25/5/15 cycle with the full HUD treatment.
func Default() Config {
	return Config{
		Focus:           25 * time.Minute,
		ShortBreak:      5 * time.Minute,
		LongBreak:       15 * time.Minute,
		LongBreakEvery:  4,
		AutoStartBreaks: true,
		AutoStartFocus:  false,
		ShowSeconds:     true,
		Scanlines:       true,
		Resume:          true,
		Theme:           "ember",
		Notify:          true,
		Sound:           defaultSound,
	}
}

// DefaultPath is config.toml in pomo's config directory.
func DefaultPath() (string, error) { return paths.ConfigFile("config.toml") }

// Duration wraps time.Duration so TOML can carry "25m" instead of a
// nanosecond count.
type Duration time.Duration

// UnmarshalText parses a Go duration string.
func (d *Duration) UnmarshalText(text []byte) error {
	v, err := time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("%q is not a duration, try a form like \"25m\"", text)
	}
	*d = Duration(v)
	return nil
}

// file mirrors the TOML document. Every field is a pointer so an absent key is
// distinguishable from one deliberately set to a zero value.
type file struct {
	Focus           *Duration `toml:"focus"`
	ShortBreak      *Duration `toml:"short_break"`
	LongBreak       *Duration `toml:"long_break"`
	LongBreakEvery  *int      `toml:"long_break_every"`
	AutoStartBreaks *bool     `toml:"auto_start_breaks"`
	AutoStartFocus  *bool     `toml:"auto_start_focus"`
	ShowSeconds     *bool     `toml:"show_seconds"`
	Resume          *bool     `toml:"resume"`
	Scanlines       *bool     `toml:"scanlines"`
	Theme           *string   `toml:"theme"`
	Notify          *bool     `toml:"notify"`
	Sound           *string   `toml:"sound"`
}

// Load reads path over the defaults. A missing file is not an error: running
// with no config at all is the common case.
func Load(path string) (Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("read %s: %w", path, err)
	}

	var f file
	if _, err := toml.Decode(string(data), &f); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}

	if f.Focus != nil {
		cfg.Focus = time.Duration(*f.Focus)
	}
	if f.ShortBreak != nil {
		cfg.ShortBreak = time.Duration(*f.ShortBreak)
	}
	if f.LongBreak != nil {
		cfg.LongBreak = time.Duration(*f.LongBreak)
	}
	if f.LongBreakEvery != nil {
		cfg.LongBreakEvery = *f.LongBreakEvery
	}
	if f.AutoStartBreaks != nil {
		cfg.AutoStartBreaks = *f.AutoStartBreaks
	}
	if f.AutoStartFocus != nil {
		cfg.AutoStartFocus = *f.AutoStartFocus
	}
	if f.ShowSeconds != nil {
		cfg.ShowSeconds = *f.ShowSeconds
	}
	if f.Resume != nil {
		cfg.Resume = *f.Resume
	}
	if f.Scanlines != nil {
		cfg.Scanlines = *f.Scanlines
	}
	if f.Theme != nil {
		cfg.Theme = *f.Theme
	}
	if f.Notify != nil {
		cfg.Notify = *f.Notify
	}
	if f.Sound != nil {
		cfg.Sound = *f.Sound
	}
	return cfg, nil
}

// Validate reports settings the program cannot run with.
func (c Config) Validate() error {
	if err := c.Timer().Validate(); err != nil {
		return err
	}
	if _, ok := theme.ByName(c.Theme); !ok {
		return fmt.Errorf("unknown theme %q, want one of: ember, mint, indigo, zen", c.Theme)
	}
	return nil
}

// Timer projects the timing settings onto the state machine's config.
func (c Config) Timer() timer.Config {
	return timer.Config{
		Focus:           c.Focus,
		ShortBreak:      c.ShortBreak,
		LongBreak:       c.LongBreak,
		LongBreakEvery:  c.LongBreakEvery,
		AutoStartBreaks: c.AutoStartBreaks,
		AutoStartFocus:  c.AutoStartFocus,
	}
}
