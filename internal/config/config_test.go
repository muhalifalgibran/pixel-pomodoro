package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "absent.toml"))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil for a missing file", err)
	}
	if got != Default() {
		t.Errorf("Load() = %+v, want the defaults", got)
	}
}

func TestLoadOverridesOnlyTheKeysPresent(t *testing.T) {
	path := writeConfig(t, "focus = \"50m\"\ntheme = \"indigo\"\n")

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Focus != 50*time.Minute {
		t.Errorf("Focus = %v, want 50m", got.Focus)
	}
	if got.Theme != "indigo" {
		t.Errorf("Theme = %q, want indigo", got.Theme)
	}
	// Untouched keys must keep their defaults.
	if got.ShortBreak != Default().ShortBreak {
		t.Errorf("ShortBreak = %v, want the default %v", got.ShortBreak, Default().ShortBreak)
	}
	if got.Notify != Default().Notify {
		t.Errorf("Notify = %v, want the default %v", got.Notify, Default().Notify)
	}
}

// A pointer field is what makes "explicitly false" different from "absent".
func TestLoadCanSetABoolToFalse(t *testing.T) {
	path := writeConfig(t, "auto_start_breaks = false\nnotify = false\nscanlines = false\n")

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.AutoStartBreaks {
		t.Error("auto_start_breaks = false was ignored")
	}
	if got.Notify {
		t.Error("notify = false was ignored")
	}
	if got.Scanlines {
		t.Error("scanlines = false was ignored")
	}
}

func TestLoadRejectsABadDuration(t *testing.T) {
	path := writeConfig(t, `focus = "25 minutes"`)

	if _, err := Load(path); err == nil {
		t.Error("Load() accepted an unparseable duration")
	}
}

func TestLoadRejectsMalformedTOML(t *testing.T) {
	path := writeConfig(t, "focus = \n")

	if _, err := Load(path); err == nil {
		t.Error("Load() accepted malformed TOML")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{"default", func(*Config) {}, false},
		{"zero focus", func(c *Config) { c.Focus = 0 }, true},
		{"negative break", func(c *Config) { c.ShortBreak = -time.Minute }, true},
		{"zero long_break_every", func(c *Config) { c.LongBreakEvery = 0 }, true},
		{"unknown theme", func(c *Config) { c.Theme = "chartreuse" }, true},
		{"mint theme", func(c *Config) { c.Theme = "mint" }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Default()
			tt.mutate(&c)
			if err := c.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestTimerProjection(t *testing.T) {
	c := Default()
	c.Focus = 50 * time.Minute
	c.LongBreakEvery = 3

	tc := c.Timer()

	if tc.Focus != 50*time.Minute {
		t.Errorf("Timer().Focus = %v, want 50m", tc.Focus)
	}
	if tc.LongBreakEvery != 3 {
		t.Errorf("Timer().LongBreakEvery = %d, want 3", tc.LongBreakEvery)
	}
	if err := tc.Validate(); err != nil {
		t.Errorf("projected timer config is invalid: %v", err)
	}
}

func TestDurationUnmarshal(t *testing.T) {
	var d Duration
	if err := d.UnmarshalText([]byte("1h30m")); err != nil {
		t.Fatalf("UnmarshalText() error = %v", err)
	}
	if time.Duration(d) != 90*time.Minute {
		t.Errorf("parsed %v, want 90m", time.Duration(d))
	}
	if err := d.UnmarshalText([]byte("nope")); err == nil {
		t.Error("UnmarshalText accepted a non-duration")
	}
}
