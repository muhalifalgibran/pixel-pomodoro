package sprite

import (
	"strings"
	"testing"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/canvas"
)

const good = `# a comment
size: 4 2
palette:
  . = transparent
  R = #ff0000   body
  b = #0000ff
pixels:
  .RRb
  b..R
`

func parseString(t *testing.T, src string) (*Sprite, error) {
	t.Helper()
	return Parse("test.pix", strings.NewReader(src))
}

func TestParseBuildsCanvasAndKeys(t *testing.T) {
	s, err := parseString(t, good)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if s.Canvas.W != 4 || s.Canvas.H != 2 {
		t.Fatalf("canvas is %dx%d, want 4x2", s.Canvas.W, s.Canvas.H)
	}
	if got, want := s.Canvas.At(1, 0), canvas.Opaque(0xff, 0, 0); got != want {
		t.Errorf("At(1,0) = %v, want %v", got, want)
	}
	if got := s.Canvas.At(0, 0); got.A {
		t.Errorf("At(0,0) = %v, want transparent", got)
	}
	if got, want := s.KeyAt(3, 0), byte('b'); got != want {
		t.Errorf("KeyAt(3,0) = %q, want %q", got, want)
	}
	if got := s.KeyAt(0, 0); got != 0 {
		t.Errorf("KeyAt on a transparent pixel = %q, want 0", got)
	}
	if got := s.KeyAt(99, 99); got != 0 {
		t.Errorf("KeyAt out of bounds = %q, want 0", got)
	}
}

func TestParseSilhouetteMatchesSource(t *testing.T) {
	s, err := parseString(t, good)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	// Rows ".RRb" over "b..R" pack to: bottom-only, top-only, top-only, full.
	if got, want := s.Canvas.Silhouette(), "▄▀▀█"; got != want {
		t.Errorf("Silhouette() = %q, want %q", got, want)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantSub string
	}{
		{
			name:    "row too wide",
			src:     "size: 4 2\npalette:\n  . = transparent\npixels:\n  .....\n  ....\n",
			wantSub: "test.pix:5: row is 5 wide, size: declares 4",
		},
		{
			name:    "row too narrow",
			src:     "size: 4 2\npalette:\n  . = transparent\npixels:\n  ...\n  ....\n",
			wantSub: "test.pix:5: row is 3 wide",
		},
		{
			name:    "wrong row count",
			src:     "size: 4 2\npalette:\n  . = transparent\npixels:\n  ....\n",
			wantSub: "got 1 pixel rows, size: declares 2",
		},
		{
			name:    "unknown palette char",
			src:     "size: 4 2\npalette:\n  . = transparent\npixels:\n  ..X.\n  ....\n",
			wantSub: `uses "X", which is not in the palette`,
		},
		{
			name:    "missing size header",
			src:     "palette:\n  . = transparent\n",
			wantSub: "missing size: header",
		},
		{
			name:    "pixels before size",
			src:     "pixels:\n  ....\n",
			wantSub: "test.pix:1: pixels: block before size:",
		},
		{
			name:    "odd height",
			src:     "size: 4 3\npalette:\n  . = transparent\npixels:\n  ....\n",
			wantSub: "height 3 must be even",
		},
		{
			name:    "bad hex",
			src:     "size: 4 2\npalette:\n  R = #ff00\npixels:\n  RRRR\n  RRRR\n",
			wantSub: `bad hex color "#ff00"`,
		},
		{
			name:    "multi-character palette key",
			src:     "size: 4 2\npalette:\n  RR = #ff0000\npixels:\n  ....\n  ....\n",
			wantSub: `palette key "RR" must be exactly one character`,
		},
		{
			name:    "palette entry without equals",
			src:     "size: 4 2\npalette:\n  R #ff0000\npixels:\n  ....\n  ....\n",
			wantSub: "wants the form",
		},
		{
			name:    "size with one number",
			src:     "size: 4\n",
			wantSub: "size: wants two numbers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseString(t, tt.src)
			if err == nil {
				t.Fatalf("Parse() succeeded, want error containing %q", tt.wantSub)
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("Parse() error = %q, want it to contain %q", err, tt.wantSub)
			}
		})
	}
}

func TestParseIgnoresTrailingWhitespaceOnPixelRows(t *testing.T) {
	src := "size: 2 2\npalette:\n  . = transparent\npixels:\n  ..  \n  ..\n"
	if _, err := parseString(t, src); err != nil {
		t.Errorf("trailing whitespace should not widen a row: %v", err)
	}
}
