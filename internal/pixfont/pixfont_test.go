package pixfont

import (
	"testing"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/canvas"
)

var testStyle = Style{
	FaceTop:    canvas.Opaque(0xff, 0x00, 0x00),
	FaceBottom: canvas.Opaque(0x80, 0x00, 0x00),
	Outline:    canvas.Opaque(0x10, 0x00, 0x00),
	Bevel:      canvas.Opaque(0xff, 0xff, 0xff),
	Ghost:      canvas.Opaque(0x20, 0x20, 0x20),
}

func faceOnly() Style {
	return Style{FaceTop: testStyle.FaceTop, FaceBottom: testStyle.FaceBottom}
}

func TestGlyphsHaveUniformGeometry(t *testing.T) {
	for r, g := range glyphs {
		if len(g) != GlyphH {
			t.Errorf("glyph %q has %d rows, want %d", r, len(g), GlyphH)
			continue
		}
		for y, row := range g {
			if len(row) != GlyphW {
				t.Errorf("glyph %q row %d is %d wide, want %d", r, y, len(row), GlyphW)
			}
		}
	}
}

func TestDigitsAndColonExist(t *testing.T) {
	for _, r := range "0123456789: " {
		if _, ok := mask(r); !ok {
			t.Errorf("font is missing %q", r)
		}
	}
}

func TestAllDigitsAreDistinct(t *testing.T) {
	seen := map[string]rune{}
	for _, r := range "0123456789" {
		g, _ := mask(r)
		key := ""
		for _, row := range g {
			key += row
		}
		if prev, dup := seen[key]; dup {
			t.Errorf("glyphs %q and %q have identical bitmaps", prev, r)
		}
		seen[key] = r
	}
}

// The clock must not reflow as digits tick over. Every digit therefore has to
// occupy the same advance.
func TestDigitAdvanceIsUniform(t *testing.T) {
	for _, r := range "0123456789" {
		if got := Advance(r); got != digitAdvance {
			t.Errorf("Advance(%q) = %d, want %d", r, got, digitAdvance)
		}
	}
}

func TestWidthIsStableAcrossDigitChanges(t *testing.T) {
	want := Width("11:11")
	for _, s := range []string{"88:88", "00:00", "23:41", "59:59"} {
		if got := Width(s); got != want {
			t.Errorf("Width(%q) = %d, want %d — the clock would reflow", s, got, want)
		}
	}
}

func TestWidthExcludesTrailingGap(t *testing.T) {
	if got, want := Width("8"), GlyphW; got != want {
		t.Errorf("Width(%q) = %d, want %d", "8", got, want)
	}
	if got, want := Width("88"), digitAdvance+GlyphW; got != want {
		t.Errorf("Width(%q) = %d, want %d", "88", got, want)
	}
	if got := Width(""); got != 0 {
		t.Errorf("Width(\"\") = %d, want 0", got)
	}
}

func TestDrawGlyphReportsUnknownRunes(t *testing.T) {
	c := canvas.New(16, 16)
	if DrawGlyph(c, 1, 1, 'Z', faceOnly()) {
		t.Error("DrawGlyph reported success for a rune the font lacks")
	}
	for y := 0; y < c.H; y++ {
		for x := 0; x < c.W; x++ {
			if c.At(x, y).A {
				t.Fatalf("unknown rune painted pixel at (%d,%d)", x, y)
			}
		}
	}
}

func TestDrawSkipsUnknownRunesWithoutShiftingLayout(t *testing.T) {
	withBad := canvas.New(32, 16)
	withoutBad := canvas.New(32, 16)
	Draw(withBad, 1, 2, "1Z1", faceOnly())
	Draw(withoutBad, 1, 2, "11", faceOnly())

	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			if withBad.At(x, y) != withoutBad.At(x, y) {
				t.Fatalf("unknown rune shifted layout at (%d,%d)", x, y)
			}
		}
	}
}

func TestFaceIsGradientTopToBottom(t *testing.T) {
	c := canvas.New(16, 16)
	DrawGlyph(c, 2, 2, '8', faceOnly())

	// '8' lights (1,0) on its top row and (1,6) on its bottom row.
	top := c.At(2+1, 2+0)
	bottom := c.At(2+1, 2+6)
	if top != testStyle.FaceTop {
		t.Errorf("top row = %v, want FaceTop %v", top, testStyle.FaceTop)
	}
	if bottom != testStyle.FaceBottom {
		t.Errorf("bottom row = %v, want FaceBottom %v", bottom, testStyle.FaceBottom)
	}
	if top == bottom {
		t.Error("face is flat, expected a vertical gradient")
	}
}

func TestOutlineSurroundsTheGlyphWithoutEatingIt(t *testing.T) {
	c := canvas.New(16, 16)
	st := faceOnly()
	st.Outline = testStyle.Outline
	DrawGlyph(c, 2, 2, '8', st)

	// Directly above the top-left lit pixel of '8' — outside the bitmap.
	if got := c.At(2+1, 2-1); got != testStyle.Outline {
		t.Errorf("pixel above the glyph = %v, want outline %v", got, testStyle.Outline)
	}
	// A lit pixel must still be face-colored, not overwritten by the outline.
	if got := c.At(2+1, 2+0); got == testStyle.Outline {
		t.Error("outline overwrote a lit glyph pixel")
	}
	// The hole in the middle of '8' is off, so it takes outline, not face.
	if got := c.At(2+2, 2+1); got != testStyle.Outline {
		t.Errorf("counter of '8' = %v, want outline %v", got, testStyle.Outline)
	}
}

func TestBevelLightsTopEdgesOnly(t *testing.T) {
	c := canvas.New(16, 16)
	st := faceOnly()
	st.Bevel = testStyle.Bevel
	DrawGlyph(c, 2, 2, '1', st)

	// '1' has a vertical stem in column 2. Its topmost pixel is row 0.
	if got := c.At(2+2, 2+0); got != testStyle.Bevel {
		t.Errorf("top of the stem = %v, want bevel %v", got, testStyle.Bevel)
	}
	// Further down the same stem there is a lit pixel above, so no bevel.
	if got := c.At(2+2, 2+3); got == testStyle.Bevel {
		t.Error("bevel leaked down the stem; a 1px stroke would become all bevel")
	}
}

func TestGhostAppearsBehindDigitsOnly(t *testing.T) {
	digit := canvas.New(16, 16)
	DrawGlyph(digit, 2, 2, '1', testStyle)
	// '8' lights (4,1) and '1' has nothing within a pixel of it, so the ghost
	// survives there. Ghost segments that do touch the glyph are legitimately
	// covered by the outline.
	if got := digit.At(2+4, 2+1); got != testStyle.Ghost {
		t.Errorf("unlit segment behind '1' = %v, want ghost %v", got, testStyle.Ghost)
	}

	colon := canvas.New(16, 16)
	DrawGlyph(colon, 2, 2, ':', testStyle)
	for y := 0; y < GlyphH; y++ {
		for x := 0; x < GlyphW; x++ {
			if colon.At(2+x, 2+y) == testStyle.Ghost {
				t.Fatalf("colon drew a ghost segment at (%d,%d); an '8' behind a colon is noise", x, y)
			}
		}
	}
}

func TestDrawPlacesGlyphsAtTheAdvance(t *testing.T) {
	c := canvas.New(32, 16)
	Draw(c, 1, 2, "11", faceOnly())

	// Both stems are in glyph column 2.
	if got := c.At(1+2, 2); !got.A {
		t.Error("first glyph is missing")
	}
	if got := c.At(1+digitAdvance+2, 2); !got.A {
		t.Error("second glyph is not at one advance from the first")
	}
}
