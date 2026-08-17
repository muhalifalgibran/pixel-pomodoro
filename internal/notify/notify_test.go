package notify

import "testing"

// The task label is user text spliced into an AppleScript literal. If a quote
// survives unescaped, the script either fails to parse or runs whatever the
// user typed after it.
func TestEscapeAppleScript(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "render loop", "render loop"},
		{"double quote", `say "hi"`, `say \"hi\"`},
		{"backslash", `c:\path`, `c:\\path`},
		{"backslash before quote", `\"`, `\\\"`},
		{"newline flattened", "line one\nline two", "line one line two"},
		{"carriage return flattened", "a\rb", "a b"},
		{
			name: "script injection attempt",
			in:   `x" with title "pwned`,
			want: `x\" with title \"pwned`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeAppleScript(tt.in); got != tt.want {
				t.Errorf("escapeAppleScript(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestZeroNotifierIsSilent(t *testing.T) {
	// Nothing to assert beyond "does not panic and does not block": a zero
	// Notifier must be safe to call on every phase change.
	var n Notifier
	n.Send("pomo", "focus finished")
}
