// Package paths resolves where pomo keeps its files.
//
// Two directories, with a deliberate split:
//
//   - config holds what the user owns — config.toml and habits.json. These are
//     intent.
//   - data holds what pomo accumulates — sessions.jsonl and state.json. These
//     are history.
//
// Clearing your history should never cost you your habit definitions, which is
// why the two are not the same directory.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// appDir is the per-application subdirectory used under both roots.
const appDir = "pomo"

// ConfigFile returns the path to a file in pomo's config directory.
//
// XDG_CONFIG_HOME wins when set, so the location is predictable and overridable
// on every platform. Otherwise os.UserConfigDir picks the platform default:
// ~/.config on Linux, ~/Library/Application Support on macOS.
func ConfigFile(name string) (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, appDir, name), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config directory: %w", err)
	}
	return filepath.Join(dir, appDir, name), nil
}

// DataFile returns the path to a file in pomo's data directory: XDG_DATA_HOME
// when set, otherwise ~/.local/share.
func DataFile(name string) (string, error) {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, appDir, name), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", appDir, name), nil
}
