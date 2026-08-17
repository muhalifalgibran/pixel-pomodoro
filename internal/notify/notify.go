// Package notify sends phase-change notifications and plays the completion
// sound. Delivery is best-effort and always asynchronous: a slow or missing
// notification tool must never stall the render loop.
package notify

import "strings"

// Notifier is the notification policy. The zero value is silent.
type Notifier struct {
	// Enabled turns desktop notifications on.
	Enabled bool
	// Sound is a path to an audio file. Empty means no sound.
	Sound string
}

// Send delivers a notification and plays the sound, both in the background.
// It returns immediately.
func (n Notifier) Send(title, body string) {
	if n.Enabled {
		go deliver(title, body)
	}
	if n.Sound != "" {
		go play(n.Sound)
	}
}

// escapeAppleScript makes s safe to embed in an AppleScript string literal.
//
// The task label is user text and goes straight into a script body, so a stray
// quote would either break the script or let the text run as AppleScript.
// Backslashes and quotes are escaped, and newlines are flattened because a raw
// newline terminates the statement.
func escapeAppleScript(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", " ",
		"\r", " ",
	)
	return r.Replace(s)
}
