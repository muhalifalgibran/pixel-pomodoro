//go:build darwin

package notify

import (
	"context"
	"os/exec"
	"time"
)

// deliverTimeout bounds the helper processes. osascript can hang if the
// notification centre is wedged; the timer must not care.
const deliverTimeout = 10 * time.Second

func deliver(title, body string) {
	script := `display notification "` + escapeAppleScript(body) +
		`" with title "` + escapeAppleScript(title) + `"`

	ctx, cancel := context.WithTimeout(context.Background(), deliverTimeout)
	defer cancel()
	// Errors are deliberately dropped: a failed notification is not worth
	// interrupting a focus session over.
	_ = exec.CommandContext(ctx, "osascript", "-e", script).Run()
}

func play(path string) {
	ctx, cancel := context.WithTimeout(context.Background(), deliverTimeout)
	defer cancel()
	_ = exec.CommandContext(ctx, "afplay", path).Run()
}
