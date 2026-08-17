package ui

import (
	"math"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/anim"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/canvas"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/sprite"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/theme"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/timer"
)

// Band geometry. The sprite sits left, the clock right; stacking them would
// need 26 rows and break the standard 80x24 terminal.
const (
	bandMargin  = 1
	bandGap     = 3
	spriteBandX = bandMargin
)

type geom struct {
	BandW, BandH     int
	SpriteX, SpriteY int
	ClockX, ClockY   int
}

func layout(spriteW, spriteH, clockW, clockH int) geom {
	bandH := spriteH
	if clockH > bandH {
		bandH = clockH
	}
	if bandH%2 != 0 {
		bandH++
	}
	g := geom{
		BandW:   bandMargin + spriteW + bandGap + clockW + bandMargin,
		BandH:   bandH,
		SpriteX: spriteBandX,
		SpriteY: 0,
		ClockX:  bandMargin + spriteW + bandGap,
		ClockY:  (bandH - clockH) / 2,
	}
	// Art is drawn for a specific half-block pairing, so both blits land on an
	// even row. An odd offset re-pairs every cell and mangles the sprite.
	g.SpriteY &^= 1
	g.ClockY &^= 1
	return g
}

// The spent portion of the mascot is desaturated and dimmed rather than
// tinted toward brown: blending toward a fixed dried color turned it into a
// mud blob, while keeping each pixel's own luminance preserves the shading.
//
// drainFeather ramps the effect in over several pixels. A hard edge reads as a
// bruise on the fruit; a soft one reads as a level dropping.
const (
	drainDesaturate = 0.35
	drainDim        = 0.62
	drainFeather    = 4.0
)

// isLeaf reports whether a palette key belongs to the calyx. The leaves do not
// drain: a tomato whose stem dries out reads as dying rather than as time
// passing.
func isLeaf(k byte) bool {
	switch k {
	case 's', 'g', 'G':
		return true
	}
	return false
}

// spriteTransform builds the per-pixel recolor for the mascot: the phase tint
// first, then the drain over the spent portion.
func spriteTransform(src *sprite.Sprite, pal theme.Palette, progress float64) func(canvas.RGBA, int, int) canvas.RGBA {
	dryBelow := progress * float64(src.Canvas.H)
	return func(c canvas.RGBA, sx, sy int) canvas.RGBA {
		if pal.SpriteTintStrength > 0 {
			c = canvas.Lerp(c, pal.SpriteTint, pal.SpriteTintStrength)
		}
		// How far above the fill line this pixel sits, feathered to 0..1.
		if t := anim.Clamp01((dryBelow - float64(sy)) / drainFeather); t > 0 && !isLeaf(src.KeyAt(sx, sy)) {
			c = canvas.Scale(canvas.Desaturate(c, drainDesaturate*t), 1-(1-drainDim)*t)
			c.A = true
		}
		return c
	}
}

// blitSquash draws src with a vertical scale, anchored at its base so the
// mascot squashes onto the ground rather than shrinking about its centre.
// Nearest-neighbor is the right choice here: interpolating would blur pixel
// art that was drawn one pixel at a time.
func blitSquash(dst, src *canvas.Canvas, x, y int, scaleY float64, f func(canvas.RGBA, int, int) canvas.RGBA) {
	if scaleY <= 0 {
		return
	}
	h := int(math.Round(float64(src.H) * scaleY))
	if h <= 0 {
		return
	}
	baseY := y + src.H - h // keep the feet planted

	for dy := 0; dy < h; dy++ {
		sy := int(float64(dy) / scaleY)
		if sy >= src.H {
			sy = src.H - 1
		}
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
			dst.Set(x+sx, baseY+dy, p)
		}
	}
}

// breath describes the mascot's idle motion for a phase.
type breath struct {
	period  float64 // seconds per cycle
	bob     float64 // vertical travel in pixels
	squash  float64 // peak vertical scale deviation
	steamHz float64 // steam wisps emitted per second, 0 for none
}

// Periods are in the range of calm human breathing, roughly 10-20 a minute.
// Focus stays the briskest of the three so it reads as alert, but the original
// 1.4s worked out at about 43 breaths a minute, which looked panicked rather
// than focused.
func breathFor(p timer.Phase, running bool) breath {
	if !running {
		// Paused: a slow shallow pulse. Freezing entirely reads as a hang.
		return breath{period: 5.0, bob: 0.6, squash: 0.012}
	}
	switch p {
	case timer.ShortBreak:
		return breath{period: 4.5, bob: 1.6, squash: 0.028}
	case timer.LongBreak:
		return breath{period: 5.5, bob: 2.0, squash: 0.03}
	default:
		return breath{period: 3.5, bob: 1.2, squash: 0.035, steamHz: 3}
	}
}

// breathZen is the calmest motion of all: no steam, the slowest cycle. Zen is
// open-ended, so nothing about it should suggest a deadline.
func breathZen(running bool) breath {
	if !running {
		return breath{period: 5.5, bob: 0.5, squash: 0.01}
	}
	return breath{period: 6.0, bob: 1.4, squash: 0.022}
}

// slowestBreath bounds how brisk any phase may be. A period under this reads
// as agitated on screen, which is the opposite of what a focus timer is for.
const slowestBreath = 3.0

// applyScanlines dims every other pixel row for a CRT feel.
func applyScanlines(c *canvas.Canvas) {
	for y := 1; y < c.H; y += 2 {
		for x := 0; x < c.W; x++ {
			p := c.At(x, y)
			if !p.A {
				continue
			}
			c.Set(x, y, canvas.Scale(p, 0.92))
		}
	}
}

// emitSteam releases a wisp from the shoulders of the mascot.
func emitSteam(sys *anim.System, g geom, spriteW int, col canvas.RGBA) {
	r := sys.Rand()
	// Bias toward the upper third, where steam would actually escape.
	x := float64(g.SpriteX) + 4 + r.Float64()*float64(spriteW-8)
	y := float64(g.SpriteY) + 2 + r.Float64()*4
	sys.Emit(anim.Particle{
		X:     x,
		Y:     y,
		VX:    (r.Float64() - 0.5) * 3,
		VY:    -(3 + r.Float64()*3),
		Life:  0.8 + r.Float64()*0.7,
		Color: col,
		Fade:  true,
	})
}

// confettiColors are deliberately off-palette: the burst should read as a
// reward, not as more HUD.
var confettiColors = []canvas.RGBA{
	canvas.MustParseHex("#ffd54f"),
	canvas.MustParseHex("#4dd0e1"),
	canvas.MustParseHex("#ff8a65"),
	canvas.MustParseHex("#aed581"),
	canvas.MustParseHex("#ba68c8"),
}

// burstConfetti fills the pool from the middle of the band.
func burstConfetti(sys *anim.System, g geom) {
	r := sys.Rand()
	originX := float64(g.BandW) / 2
	originY := float64(g.BandH) / 2
	for i := 0; i < sys.Cap(); i++ {
		angle := r.Float64() * 2 * math.Pi
		speed := 8 + r.Float64()*22
		sys.Emit(anim.Particle{
			X:     originX,
			Y:     originY,
			VX:    math.Cos(angle) * speed,
			VY:    math.Sin(angle)*speed - 6,
			Life:  0.9 + r.Float64()*0.7,
			Color: confettiColors[r.Intn(len(confettiColors))],
			Fade:  true,
		})
	}
}
