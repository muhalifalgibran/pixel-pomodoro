package ui

import (
	"fmt"
	"time"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/anim"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/canvas"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/pixfont"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/theme"
)

const (
	// rollDuration is how long a digit takes to slide over.
	rollDuration = 120 * time.Millisecond
	// rollDistance is the vertical travel of a rolling digit, one glyph plus
	// its margin, so the outgoing digit fully clears the window.
	rollDistance = pixfont.GlyphH + 2

	// alertWindow is when the countdown escalates.
	alertWindow = 10 * time.Second
	// colonPeriod is one full fade cycle of the colon, in seconds.
	colonPeriod = 1.0
)

// clock renders the countdown. It owns the odometer roll, which needs to
// remember what each character used to be.
type clock struct {
	text string
	prev []rune
	roll []float64 // per character, 0..1; 1 means settled

	elapsed float64 // seconds since start, drives the colon pulse
}

// FormatRemaining renders a duration as the clock string. Sub-second
// remainders round up so the display reaches 00:00 exactly when the phase
// ends, rather than sitting on 00:00 for a whole second beforehand.
func FormatRemaining(d time.Duration, showSeconds bool) string {
	if d < 0 {
		d = 0
	}
	total := int((d + time.Second - 1) / time.Second)
	mins, secs := total/60, total%60
	if showSeconds {
		return fmt.Sprintf("%02d:%02d", mins, secs)
	}
	// Minute-only display still switches to seconds for the final minute;
	// watching a static "01" tick away for sixty seconds reads as frozen.
	if mins > 0 {
		return fmt.Sprintf("%02d:--", mins)
	}
	return fmt.Sprintf("00:%02d", secs)
}

// FormatElapsed renders a count-up duration in exactly five glyphs, which is
// what keeps zen mode from reflowing the HUD.
//
// The pixel font is fixed-advance and the layout is sized once from a sample
// clock string, so a clock that grew a digit would move the whole frame. A
// stopwatch has no upper bound, so the units change instead of the width:
// MM:SS under an hour, HH:MM from an hour on. Five glyphs cannot say which,
// which is why the line beneath always spells the time out in full.
func FormatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d / time.Second)
	if total < 3600 {
		return fmt.Sprintf("%02d:%02d", total/60, total%60)
	}
	hours := total / 3600
	if hours > 99 {
		// A hundred hours in one sitting is not a session anyone is having, and
		// a sixth glyph would move the frame.
		hours = 99
	}
	return fmt.Sprintf("%02d:%02d", hours, (total%3600)/60)
}

// SpellElapsed writes a duration out in full, which is where the ambiguity in
// the five-glyph clock is resolved.
func SpellElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d / time.Second)
	h, m, sec := total/3600, (total%3600)/60, total%60
	switch {
	case h > 0:
		return fmt.Sprintf("%dh %dm %ds", h, m, sec)
	case m > 0:
		return fmt.Sprintf("%dm %ds", m, sec)
	default:
		return fmt.Sprintf("%ds", sec)
	}
}

// set updates the displayed text, starting a roll on every character that
// changed. A change in length resets rather than animates: the layout is
// fixed-width, so that only happens on a format switch.
func (c *clock) set(text string) {
	runes := []rune(text)
	if len(runes) != len(c.roll) {
		c.text = text
		c.prev = append(c.prev[:0], runes...)
		c.roll = make([]float64, len(runes))
		for i := range c.roll {
			c.roll[i] = 1
		}
		return
	}
	old := []rune(c.text)
	for i, r := range runes {
		if i < len(old) && old[i] != r {
			c.prev[i] = old[i]
			c.roll[i] = 0
		}
	}
	c.text = text
}

// update advances the roll and the colon pulse.
func (c *clock) update(dt float64) {
	c.elapsed += dt
	step := dt / rollDuration.Seconds()
	for i := range c.roll {
		if c.roll[i] < 1 {
			c.roll[i] = anim.Clamp01(c.roll[i] + step)
		}
	}
}

// settled reports whether every digit has finished rolling. Used by tests to
// assert the animation terminates.
func (c *clock) settled() bool {
	for _, t := range c.roll {
		if t < 1 {
			return false
		}
	}
	return true
}

// clockCanvasSize is the drawing window for a clock string: the ink plus one
// pixel of outline on each side, and enough rows to hold a glyph drawn at the
// even origin y=2 with its outline above and below.
func clockCanvasSize(text string) (w, h int) {
	return pixfont.Width(text) + 2, pixfont.GlyphH + 3
}

// clockGlyphOriginY is the even row a settled glyph is drawn on.
const clockGlyphOriginY = 2

// draw renders the clock into its own canvas. Rolling glyphs that slide past
// the edges are clipped by the canvas bounds, which is what keeps a digit from
// smearing into the rest of the HUD.
func (c *clock) draw(st pixfont.Style, panel canvas.RGBA, alert bool, jitter int) *canvas.Canvas {
	w, h := clockCanvasSize(c.text)
	if h%2 != 0 {
		h++
	}
	out := canvas.New(w, h)

	// One pixel in from the left leaves room for the outline. The vertical
	// origin must be even: glyphs are drawn for a specific half-block
	// pairing, and an odd row re-pairs every cell and shreds them. y=2 puts
	// the glyph on rows 2..8 with its outline on 1..9, inside a 10-row canvas.
	baseX, baseY := 1, clockGlyphOriginY

	colonStyle := c.colonStyle(st, panel, alert)

	x := baseX
	for i, r := range c.text {
		style := st
		if r == ':' {
			style = colonStyle
		}
		y := baseY
		if alert && r != ':' {
			y += jitter
		}

		if t := c.roll[i]; t < 1 {
			shift := int(anim.EaseOutCubic(t)*rollDistance + 0.5)
			// Outgoing digit slides up and out.
			pixfont.DrawGlyph(out, x, y-shift, c.prev[i], style)
			// Incoming digit follows it in from below.
			pixfont.DrawGlyph(out, x, y+rollDistance-shift, r, style)
		} else {
			pixfont.DrawGlyph(out, x, y, r, style)
		}
		x += pixfont.Advance(r)
	}
	return out
}

// colonStyle fades the colon once per second so the clock visibly breathes
// even when no digit is moving.
func (c *clock) colonStyle(st pixfont.Style, panel canvas.RGBA, alert bool) pixfont.Style {
	// Stay bright during the alert window; a fading colon reads as calm.
	if alert {
		return st
	}
	// Never fade below half, or the colon appears to drop out entirely.
	fade := 0.5 * anim.Pulse(c.elapsed, colonPeriod)
	out := st
	out.FaceTop = canvas.Lerp(st.FaceTop, panel, fade)
	out.FaceBottom = canvas.Lerp(st.FaceBottom, panel, fade)
	out.Bevel = canvas.Lerp(st.Bevel, panel, fade)
	return out
}

// clockStyleFor picks the clock palette, escalating to the alert style over
// the final seconds of a phase.
func clockStyleFor(pal theme.Palette, remaining time.Duration, running bool) (st pixfont.Style, alert bool) {
	if !running || remaining > alertWindow || remaining < 0 {
		return pal.Clock, false
	}
	// Cross-fade into the alert palette over the window so it escalates
	// rather than snapping.
	t := 1 - float64(remaining)/float64(alertWindow)
	return theme.LerpStyle(pal.Clock, theme.Alert, anim.EaseOutQuad(t)), true
}
