// Package theme holds the per-phase color palettes. The mascot is faceless, so
// palette and motion are what carry the mood — this file is where "focus feels
// different from break" actually lives.
package theme

import (
	"github.com/muhalifalgibran/pixel-pomodoro/internal/canvas"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/pixfont"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/timer"
)

// Palette is every color one phase needs.
type Palette struct {
	Name string

	// Panel is the HUD background the art composites against.
	Panel canvas.RGBA
	// Frame is the pixel border; FrameDim is its shadowed side.
	Frame, FrameDim canvas.RGBA
	// Accent drives the progress bar fill, cycle dots and XP bar.
	Accent, AccentDim canvas.RGBA
	// Text is the label color; TextDim is for secondary chrome.
	Text, TextDim canvas.RGBA

	// Clock is the pixfont style for this phase.
	Clock pixfont.Style

	// SpriteTint cools or warms the mascot to match the phase.
	SpriteTint canvas.RGBA
	// SpriteTintStrength is how far the mascot is pulled toward SpriteTint,
	// in [0,1]. Keep it low: a heavy tint stops the tomato reading as a
	// tomato, and the panel, frame and clock already carry the phase.
	SpriteTintStrength float64
}

var hex = canvas.MustParseHex

// Ember is the focus palette: hot, close, slightly tense.
var Ember = Palette{
	Name:      "ember",
	Panel:     hex("#141218"),
	Frame:     hex("#5c2b1e"),
	FrameDim:  hex("#2e1610"),
	Accent:    hex("#ff7043"),
	AccentDim: hex("#4a2018"),
	Text:      hex("#ffd9c9"),
	TextDim:   hex("#8a6055"),
	Clock: pixfont.Style{
		FaceTop:    hex("#ff8a65"),
		FaceBottom: hex("#c62828"),
		Outline:    hex("#3d0a0a"),
		Bevel:      hex("#ffd9c9"),
		Ghost:      hex("#241618"),
	},
	SpriteTint:         hex("#ff7043"),
	SpriteTintStrength: 0,
}

// Mint is the short-break palette: cool, open, restful.
var Mint = Palette{
	Name:      "mint",
	Panel:     hex("#101816"),
	Frame:     hex("#1e5c4a"),
	FrameDim:  hex("#0f2e25"),
	Accent:    hex("#4dd0a7"),
	AccentDim: hex("#183a31"),
	Text:      hex("#c9f5e6"),
	TextDim:   hex("#55887a"),
	Clock: pixfont.Style{
		FaceTop:    hex("#7ff0c6"),
		FaceBottom: hex("#1e8f6d"),
		Outline:    hex("#062a20"),
		Bevel:      hex("#dffff4"),
		Ghost:      hex("#152a24"),
	},
	SpriteTint:         hex("#4dd0a7"),
	SpriteTintStrength: 0.18,
}

// Indigo is the long-break palette: the calmest of the three.
var Indigo = Palette{
	Name:      "indigo",
	Panel:     hex("#121020"),
	Frame:     hex("#3a3480"),
	FrameDim:  hex("#1c1940"),
	Accent:    hex("#8b7cf0"),
	AccentDim: hex("#241f4a"),
	Text:      hex("#ddd8ff"),
	TextDim:   hex("#6a6396"),
	Clock: pixfont.Style{
		FaceTop:    hex("#b3a8ff"),
		FaceBottom: hex("#5b4bc4"),
		Outline:    hex("#140f33"),
		Bevel:      hex("#efeaff"),
		Ghost:      hex("#1b1735"),
	},
	SpriteTint:         hex("#8b7cf0"),
	SpriteTintStrength: 0.22,
}

// Zen is the open-ended stopwatch's palette: the calmest of the set. Focus is
// meant to feel brisk; zen is the opposite, so the colours are cool and low
// contrast and nothing about it urges you along.
var Zen = Palette{
	Name:      "zen",
	Panel:     hex("#0e1114"),
	Frame:     hex("#2f3b46"),
	FrameDim:  hex("#18202a"),
	Accent:    hex("#7fb3c8"),
	AccentDim: hex("#24323c"),
	Text:      hex("#d5e6ee"),
	TextDim:   hex("#5f7480"),
	Clock: pixfont.Style{
		FaceTop:    hex("#cfe8f2"),
		FaceBottom: hex("#4a7f95"),
		Outline:    hex("#0b1a21"),
		Bevel:      hex("#ffffff"),
		Ghost:      hex("#162026"),
	},
	SpriteTint:         hex("#7fb3c8"),
	SpriteTintStrength: 0.15,
}

// Alert overrides the clock during the last seconds of a phase. It is applied
// on top of whatever phase palette is active, so the HUD stays put and only
// the countdown escalates.
var Alert = pixfont.Style{
	FaceTop:    hex("#fff3c4"),
	FaceBottom: hex("#ffa000"),
	Outline:    hex("#4a2600"),
	Bevel:      hex("#ffffff"),
	Ghost:      hex("#2a2010"),
}

// For returns the palette a phase should use.
func For(p timer.Phase) Palette {
	switch p {
	case timer.ShortBreak:
		return Mint
	case timer.LongBreak:
		return Indigo
	default:
		return Ember
	}
}

// Lerp blends two palettes. Phase changes run through this over a few hundred
// milliseconds so the color shift reads as a fade rather than a jump.
func Lerp(a, b Palette, t float64) Palette {
	switch {
	case t <= 0:
		return a
	case t >= 1:
		return b
	}
	out := b
	out.Panel = canvas.Lerp(a.Panel, b.Panel, t)
	out.Frame = canvas.Lerp(a.Frame, b.Frame, t)
	out.FrameDim = canvas.Lerp(a.FrameDim, b.FrameDim, t)
	out.Accent = canvas.Lerp(a.Accent, b.Accent, t)
	out.AccentDim = canvas.Lerp(a.AccentDim, b.AccentDim, t)
	out.Text = canvas.Lerp(a.Text, b.Text, t)
	out.TextDim = canvas.Lerp(a.TextDim, b.TextDim, t)
	out.Clock = LerpStyle(a.Clock, b.Clock, t)
	out.SpriteTint = canvas.Lerp(a.SpriteTint, b.SpriteTint, t)
	out.SpriteTintStrength = a.SpriteTintStrength + (b.SpriteTintStrength-a.SpriteTintStrength)*t
	return out
}

// LerpStyle blends two clock styles.
func LerpStyle(a, b pixfont.Style, t float64) pixfont.Style {
	return pixfont.Style{
		FaceTop:    canvas.Lerp(a.FaceTop, b.FaceTop, t),
		FaceBottom: canvas.Lerp(a.FaceBottom, b.FaceBottom, t),
		Outline:    canvas.Lerp(a.Outline, b.Outline, t),
		Bevel:      canvas.Lerp(a.Bevel, b.Bevel, t),
		Ghost:      canvas.Lerp(a.Ghost, b.Ghost, t),
	}
}

// ByName looks up a palette for the config file's theme key.
func ByName(name string) (Palette, bool) {
	switch name {
	case "ember":
		return Ember, true
	case "mint":
		return Mint, true
	case "indigo":
		return Indigo, true
	case "zen":
		return Zen, true
	}
	return Palette{}, false
}
