// Package pixfont draws the clock. Glyphs are 5x7 bitmaps composited with
// four layers — ghost, outline, gradient face, bevel — which is what separates
// "pixel art" from "blocks of solid color".
package pixfont

import "github.com/muhalifalgibran/pixel-pomodoro/internal/canvas"

// Glyph geometry. Outline and bevel live outside the 5x7 box, so a glyph
// drawn at (x, y) touches x-1..x+GlyphW and y-1..y+GlyphH.
const (
	GlyphW = 5
	GlyphH = 7

	digitAdvance = 6 // 5px glyph + 1px gap; adjacent outlines share the gap
	colonAdvance = 4 // the colon never changes, so a tighter advance is safe
)

// Style is the palette for one clock rendering. Any layer left transparent is
// skipped, so a caller can opt out of ghosting or bevelling without a flag.
type Style struct {
	// FaceTop and FaceBottom are the vertical gradient endpoints.
	FaceTop, FaceBottom canvas.RGBA
	// Outline rings the glyph, hue-matched and dark.
	Outline canvas.RGBA
	// Bevel lights the top edge of every stroke.
	Bevel canvas.RGBA
	// Ghost is the unlit-segment mask drawn behind digits.
	Ghost canvas.RGBA
}

// Advance is the horizontal step from one glyph's origin to the next.
func Advance(r rune) int {
	switch r {
	case ':', ' ':
		return colonAdvance
	default:
		return digitAdvance
	}
}

// Width is the ink width of text: the distance from the first glyph's left
// edge to the last glyph's right edge, excluding the trailing gap. The outline
// adds one pixel on each side beyond this.
func Width(text string) int {
	w, n := 0, 0
	var last rune
	for _, r := range text {
		w += Advance(r)
		last, n = r, n+1
	}
	if n > 0 {
		w -= Advance(last) - GlyphW
	}
	return w
}

// Draw composites text onto dst with the first glyph's top-left at (x, y).
// Unknown runes are skipped without advancing, so a caller cannot silently
// shift the layout by passing a character the font lacks.
func Draw(dst *canvas.Canvas, x, y int, text string, st Style) {
	cx := x
	for _, r := range text {
		if !DrawGlyph(dst, cx, y, r, st) {
			continue
		}
		cx += Advance(r)
	}
}

// DrawGlyph composites a single glyph and reports whether the rune exists in
// the font.
func DrawGlyph(dst *canvas.Canvas, x, y int, r rune, st Style) bool {
	g, ok := mask(r)
	if !ok {
		return false
	}

	// Ghost sits behind everything: the unlit segments of the cell, faint.
	// Only digits get it — an '8' behind a colon would read as noise.
	if st.Ghost.A && isDigit(r) {
		if gh, ok := mask(ghostSource); ok {
			for gy := 0; gy < GlyphH; gy++ {
				for gx := 0; gx < GlyphW; gx++ {
					if on(gh, gx, gy) {
						dst.Set(x+gx, y+gy, st.Ghost)
					}
				}
			}
		}
	}

	// Outline rings the lit pixels. Drawn before the face so it can never eat
	// into the glyph itself.
	if st.Outline.A {
		for gy := 0; gy < GlyphH; gy++ {
			for gx := 0; gx < GlyphW; gx++ {
				if !on(g, gx, gy) {
					continue
				}
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						if dx == 0 && dy == 0 {
							continue
						}
						if !on(g, gx+dx, gy+dy) {
							dst.Set(x+gx+dx, y+gy+dy, st.Outline)
						}
					}
				}
			}
		}
	}

	// Face, shaded top to bottom.
	for gy := 0; gy < GlyphH; gy++ {
		t := float64(gy) / float64(GlyphH-1)
		col := canvas.Lerp(st.FaceTop, st.FaceBottom, t)
		for gx := 0; gx < GlyphW; gx++ {
			if on(g, gx, gy) {
				dst.Set(x+gx, y+gy, col)
			}
		}
	}

	// Bevel lights the top edge of each stroke: a lit pixel with nothing above
	// it. On a one-pixel stroke this catches horizontal bars entirely and only
	// the first pixel of a vertical run, which is what a light from above does.
	if st.Bevel.A {
		for gy := 0; gy < GlyphH; gy++ {
			for gx := 0; gx < GlyphW; gx++ {
				if on(g, gx, gy) && !on(g, gx, gy-1) {
					dst.Set(x+gx, y+gy, st.Bevel)
				}
			}
		}
	}
	return true
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }
