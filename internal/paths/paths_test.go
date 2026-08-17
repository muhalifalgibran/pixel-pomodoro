package paths

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigFileHonoursXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-config")

	got, err := ConfigFile("config.toml")
	if err != nil {
		t.Fatalf("ConfigFile() error = %v", err)
	}
	if want := filepath.Join("/tmp/xdg-config", "pomo", "config.toml"); got != want {
		t.Errorf("ConfigFile() = %q, want %q", got, want)
	}
}

func TestDataFileHonoursXDG(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/tmp/xdg-data")

	got, err := DataFile("sessions.jsonl")
	if err != nil {
		t.Fatalf("DataFile() error = %v", err)
	}
	if want := filepath.Join("/tmp/xdg-data", "pomo", "sessions.jsonl"); got != want {
		t.Errorf("DataFile() = %q, want %q", got, want)
	}
}

func TestFallbacksWhenXDGIsUnset(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")

	cfg, err := ConfigFile("config.toml")
	if err != nil {
		t.Fatalf("ConfigFile() error = %v", err)
	}
	data, err := DataFile("sessions.jsonl")
	if err != nil {
		t.Fatalf("DataFile() error = %v", err)
	}

	// Both land under the pomo subdirectory with the requested filename, and
	// neither is a bare relative path.
	for name, path := range map[string]string{"config": cfg, "data": data} {
		if !filepath.IsAbs(path) {
			t.Errorf("%s path %q is not absolute", name, path)
		}
		if filepath.Base(filepath.Dir(path)) != "pomo" {
			t.Errorf("%s path %q is not inside a pomo directory", name, path)
		}
	}
	if !strings.HasSuffix(data, filepath.Join(".local", "share", "pomo", "sessions.jsonl")) {
		t.Errorf("data fallback = %q, want it under ~/.local/share/pomo", data)
	}
}

// Config and data must not resolve to the same directory: clearing history
// should never take the habit definitions with it.
func TestConfigAndDataAreSeparate(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")

	cfg, err := ConfigFile("habits.json")
	if err != nil {
		t.Fatal(err)
	}
	data, err := DataFile("sessions.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(cfg) == filepath.Dir(data) {
		t.Errorf("config and data share a directory: %q", filepath.Dir(cfg))
	}
}
