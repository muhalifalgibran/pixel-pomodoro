// Package canvas provides a pixel buffer that renders to a terminal using
// half-block characters. Each cell holds two vertical pixels: the top pixel is
// the foreground of "▀" and the bottom pixel is the background.
package canvas

import (
	"fmt"
	"strconv"
	"strings"
)

// RGBA is a pixel. A reports whether the pixel is opaque; transparent pixels
// resolve to the background color at render time.
type RGBA struct {
	R, G, B uint8
	A       bool
}

// Opaque builds a visible pixel.
func Opaque(r, g, b uint8) RGBA { return RGBA{R: r, G: g, B: b, A: true} }

// Transparent is the zero pixel.
var Transparent = RGBA{}

// ParseHex converts "#rrggbb" to an opaque color.
func ParseHex(s string) (RGBA, error) {
	h := strings.TrimPrefix(s, "#")
	if len(h) != 6 {
		return RGBA{}, fmt.Errorf("bad hex color %q, want #rrggbb", s)
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return RGBA{}, fmt.Errorf("bad hex color %q: %w", s, err)
	}
	return Opaque(uint8(v>>16), uint8(v>>8), uint8(v)), nil
}

// MustParseHex is ParseHex for compile-time color literals, where a bad value
// is a programming error rather than user input.
func MustParseHex(s string) RGBA {
	c, err := ParseHex(s)
	if err != nil {
		panic(err)
	}
	return c
}

// Lerp blends a to b. t is clamped to [0,1]. A transparent endpoint blends as
// if it were the other color, so fading toward "nothing" does not darken.
func Lerp(a, b RGBA, t float64) RGBA {
	switch {
	case t <= 0:
		return a
	case t >= 1:
		return b
	}
	mix := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t + 0.5) }
	return RGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: true}
}

// Desaturate pulls a color toward its own luminance. amount is clamped to
// [0,1]; 1 is fully grey. Unlike blending toward a fixed grey, this keeps the
// pixel's original brightness, so shading survives.
func Desaturate(c RGBA, amount float64) RGBA {
	y := 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)
	grey := RGBA{R: uint8(y + 0.5), G: uint8(y + 0.5), B: uint8(y + 0.5), A: c.A}
	out := Lerp(c, grey, amount)
	out.A = c.A
	return out
}

// Scale multiplies a color's channels, for dimming without changing hue.
func Scale(c RGBA, f float64) RGBA {
	clamp := func(v float64) uint8 {
		switch {
		case v <= 0:
			return 0
		case v >= 255:
			return 255
		}
		return uint8(v + 0.5)
	}
	return RGBA{R: clamp(float64(c.R) * f), G: clamp(float64(c.G) * f), B: clamp(float64(c.B) * f), A: c.A}
}

// Canvas is a fixed-size pixel buffer. H must be even so every cell holds a
// complete pixel pair.
type Canvas struct {
	W, H int
	px   []RGBA
}

// New allocates a canvas. It panics on a non-even height, which is always a
// programming error rather than user input.
func New(w, h int) *Canvas {
	if h%2 != 0 {
		panic("canvas: height must be even")
	}
	return &Canvas{W: w, H: h, px: make([]RGBA, w*h)}
}

func (c *Canvas) inBounds(x, y int) bool { return x >= 0 && y >= 0 && x < c.W && y < c.H }

// At returns the pixel at (x, y), or Transparent when out of bounds.
func (c *Canvas) At(x, y int) RGBA {
	if !c.inBounds(x, y) {
		return Transparent
	}
	return c.px[y*c.W+x]
}

// Set writes a pixel, ignoring out-of-bounds writes so callers can draw shapes
// that overhang the canvas without clipping by hand.
func (c *Canvas) Set(x, y int, col RGBA) {
	if !c.inBounds(x, y) {
		return
	}
	c.px[y*c.W+x] = col
}

// Fill paints every pixel.
func (c *Canvas) Fill(col RGBA) {
	for i := range c.px {
		c.px[i] = col
	}
}

// Blit copies src onto c with its top-left at (x, y). Transparent source
// pixels leave the destination untouched.
func (c *Canvas) Blit(src *Canvas, x, y int) {
	c.BlitFunc(src, x, y, nil)
}

// BlitFunc is Blit with a per-pixel transform applied to the source color. It
// is how the mascot drain and palette tinting are done: one sprite, no
// per-frame art. A nil f copies unchanged.
func (c *Canvas) BlitFunc(src *Canvas, x, y int, f func(src RGBA, sx, sy int) RGBA) {
	for sy := 0; sy < src.H; sy++ {
		for sx := 0; sx < src.W; sx++ {
			p := src.At(sx, sy)
			if !p.A {
				continue
			}
			if f != nil {
				p = f(p, sx, sy)
				if !p.A {
					continue
				}
			}
			c.Set(x+sx, y+sy, p)
		}
	}
}

const upperHalf = "▀"

// Render emits the canvas as ANSI text, one line per pixel row pair. Runs of
// identical color pairs share a single escape sequence, which keeps a full
// redraw cheap enough for a 20fps loop.
func (c *Canvas) Render(bg RGBA) string {
	buf := make([]byte, 0, c.W*c.H/2*8)
	var lastTop, lastBot RGBA
	for y := 0; y < c.H; y += 2 {
		primed := false
		for x := 0; x < c.W; x++ {
			top := resolve(c.At(x, y), bg)
			bot := resolve(c.At(x, y+1), bg)
			if !primed || top != lastTop || bot != lastBot {
				buf = appendSGR(buf, top, bot)
				lastTop, lastBot, primed = top, bot, true
			}
			buf = append(buf, upperHalf...)
		}
		buf = append(buf, "\x1b[0m"...)
		if y+2 < c.H {
			buf = append(buf, '\n')
		}
	}
	return string(buf)
}

// Silhouette renders opacity only, using the four half-block shapes. It drops
// color entirely, which makes it readable in a plain terminal capture and
// stable enough to use as golden-test output.
func (c *Canvas) Silhouette() string {
	buf := make([]byte, 0, c.W*c.H/2*4)
	for y := 0; y < c.H; y += 2 {
		for x := 0; x < c.W; x++ {
			top, bot := c.At(x, y).A, c.At(x, y+1).A
			switch {
			case top && bot:
				buf = append(buf, "█"...)
			case top:
				buf = append(buf, "▀"...)
			case bot:
				buf = append(buf, "▄"...)
			default:
				buf = append(buf, ' ')
			}
		}
		if y+2 < c.H {
			buf = append(buf, '\n')
		}
	}
	return string(buf)
}

func resolve(p, bg RGBA) RGBA {
	if p.A {
		return p
	}
	return RGBA{R: bg.R, G: bg.G, B: bg.B, A: true}
}

func appendSGR(buf []byte, fg, bg RGBA) []byte {
	buf = append(buf, "\x1b[38;2;"...)
	buf = appendTriple(buf, fg)
	buf = append(buf, "m\x1b[48;2;"...)
	buf = appendTriple(buf, bg)
	return append(buf, 'm')
}

func appendTriple(buf []byte, c RGBA) []byte {
	buf = strconv.AppendUint(buf, uint64(c.R), 10)
	buf = append(buf, ';')
	buf = strconv.AppendUint(buf, uint64(c.G), 10)
	buf = append(buf, ';')
	return strconv.AppendUint(buf, uint64(c.B), 10)
}
