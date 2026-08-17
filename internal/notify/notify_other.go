//go:build !darwin

package notify

// Notifications and sound are macOS-only for now. The stubs keep the rest of
// the program cross-platform: `go build` and the tests work everywhere, the
// timer simply runs quietly.

func deliver(title, body string) {}

func play(path string) {}
