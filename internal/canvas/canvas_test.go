package canvas

import (
	"strings"
	"testing"
)

var (
	red  = Opaque(0xff, 0x00, 0x00)
	blue = Opaque(0x00, 0x00, 0xff)
	bg   = Opaque(0x10, 0x10, 0x10)
)

func TestSilhouetteShapes(t *testing.T) {
	// One cell per shape: full, top-only, bottom-only, empty.
	c := New(4, 2)
	c.Set(0, 0, red)
	c.Set(0, 1, red)
	c.Set(1, 0, red)
	c.Set(2, 1, red)

	if got, want := c.Silhouette(), "█▀▄ "; got != want {
		t.Errorf("Silhouette() = %q, want %q", got, want)
	}
}

func TestSilhouetteRowsJoinWithNewline(t *testing.T) {
	c := New(1, 4)
	c.Set(0, 0, red)
	c.Set(0, 3, red)

	if got, want := c.Silhouette(), "▀\n▄"; got != want {
		t.Errorf("Silhouette() = %q, want %q", got, want)
	}
}

func TestRenderTransparentResolvesToBackground(t *testing.T) {
	c := New(1, 2) // entirely transparent

	got := c.Render(bg)
	want := "\x1b[38;2;16;16;16m\x1b[48;2;16;16;16m▀\x1b[0m"
	if got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestRenderPacksTopAndBottomPixels(t *testing.T) {
	c := New(1, 2)
	c.Set(0, 0, red)
	c.Set(0, 1, blue)

	got := c.Render(bg)
	want := "\x1b[38;2;255;0;0m\x1b[48;2;0;0;255m▀\x1b[0m"
	if got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestRenderCoalescesRunsWithoutChangingPixels(t *testing.T) {
	c := New(8, 2)
	for x := 0; x < 8; x++ {
		c.Set(x, 0, red)
		c.Set(x, 1, red)
	}

	got := c.Render(bg)
	if n := strings.Count(got, "\x1b[38;2;"); n != 1 {
		t.Errorf("uniform row emitted %d color escapes, want 1", n)
	}
	if n := strings.Count(got, "▀"); n != 8 {
		t.Errorf("emitted %d cells, want 8", n)
	}
}

func TestRenderRestartsColorStateEachRow(t *testing.T) {
	// Row 2 repeats row 1's colors. The reset at end of row 1 means row 2 must
	// still emit its own escape rather than inheriting.
	c := New(1, 4)
	for y := 0; y < 4; y++ {
		c.Set(0, y, red)
	}

	got := c.Render(bg)
	if n := strings.Count(got, "\x1b[38;2;"); n != 2 {
		t.Errorf("got %d color escapes across 2 rows, want 2", n)
	}
	if n := strings.Count(got, "\x1b[0m"); n != 2 {
		t.Errorf("got %d resets, want one per row", n)
	}
}

func TestBlitSkipsTransparentSourcePixels(t *testing.T) {
	dst := New(2, 2)
	dst.Set(0, 0, red)
	dst.Set(1, 0, red)

	src := New(2, 2)
	src.Set(1, 0, blue) // only this pixel is opaque

	dst.Blit(src, 0, 0)

	if got := dst.At(0, 0); got != red {
		t.Errorf("transparent source pixel overwrote destination: got %v, want %v", got, red)
	}
	if got := dst.At(1, 0); got != blue {
		t.Errorf("opaque source pixel did not land: got %v, want %v", got, blue)
	}
}

func TestBlitFuncCanDropPixels(t *testing.T) {
	dst := New(2, 2)
	src := New(2, 2)
	src.Set(0, 0, red)
	src.Set(1, 0, red)

	// Drop the left column, recolor the right.
	dst.BlitFunc(src, 0, 0, func(p RGBA, sx, sy int) RGBA {
		if sx == 0 {
			return Transparent
		}
		return blue
	})

	if got := dst.At(0, 0); got.A {
		t.Errorf("pixel returned as transparent was still written: %v", got)
	}
	if got := dst.At(1, 0); got != blue {
		t.Errorf("transformed pixel = %v, want %v", got, blue)
	}
}

func TestBlitClipsOutOfBounds(t *testing.T) {
	dst := New(2, 2)
	src := New(2, 2)
	src.Fill(red)

	dst.Blit(src, 1, 0) // half of src hangs off the right edge

	if got := dst.At(1, 0); got != red {
		t.Errorf("in-bounds pixel = %v, want %v", got, red)
	}
	// The out-of-bounds column is simply dropped; reaching here without a
	// panic is the assertion.
}

func TestNewRejectsOddHeight(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("New with odd height did not panic")
		}
	}()
	New(2, 3)
}
