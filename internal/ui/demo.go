package ui

import (
	"strings"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/canvas"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/config"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/pixfont"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/theme"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/timer"
)

// demoPhases is what the demo renders: one band per palette, each at a
// different point in its phase so the mascot's drain is visible.
var demoPhases = []struct {
	phase    timer.Phase
	progress float64
}{
	{timer.Focus, 0.25},
	{timer.ShortBreak, 0.60},
	{timer.LongBreak, 0.85},
}

// demoBand composes a single HUD band: mascot on the left, clock on the right.
// mono drops the clock's outline layer, which would otherwise make every glyph
// a solid blob in a silhouette.
func demoBand(cfg config.Config, text string, phase timer.Phase, progress float64, mono bool) (*canvas.Canvas, theme.Palette, error) {
	tomato, err := loadTomato()
	if err != nil {
		return nil, theme.Palette{}, err
	}
	pal := theme.For(phase)

	clockW, clockH := clockCanvasSize(text)
	g := layout(tomato.Canvas.W, tomato.Canvas.H, clockW, clockH)

	band := canvas.New(g.BandW, g.BandH)
	band.BlitFunc(tomato.Canvas, g.SpriteX, g.SpriteY, spriteTransform(tomato, pal, progress))

	style := pal.Clock
	if mono {
		style = pixfont.Style{FaceTop: pal.Clock.FaceTop, FaceBottom: pal.Clock.FaceBottom}
	}
	clock := canvas.New(clockW, clockH+clockH%2)
	pixfont.Draw(clock, 1, clockGlyphOriginY, text, style)
	band.Blit(clock, g.ClockX, g.ClockY)

	if cfg.Scanlines && !mono {
		applyScanlines(band)
	}
	return band, pal, nil
}

// DemoArt renders the mascot and the clock once, without starting a timer.
// This is the fast loop for iterating on sprites and glyphs: edit the .pix file
// or a bitmap, rerun, look.
func DemoArt(cfg config.Config, text string, mono bool) (string, error) {
	var out []string
	for _, d := range demoPhases {
		band, pal, err := demoBand(cfg, text, d.phase, d.progress, mono)
		if err != nil {
			return "", err
		}
		out = append(out, "── "+pal.Name+" ──")
		if mono {
			out = append(out, band.Silhouette())
		} else {
			out = append(out, band.Render(pal.Panel))
		}
		out = append(out, "")
	}
	return strings.Join(out, "\n"), nil
}

// DemoSVG renders the same bands as a standalone SVG, one per palette. A
// terminal screenshot cannot be committed to a repository and block characters
// in a README show only the silhouette, so the documentation needs a real
// image of what the art actually looks like.
func DemoSVG(cfg config.Config, text string, pixel int) (string, error) {
	if pixel < 1 {
		pixel = 1
	}

	bands := make([]*canvas.Canvas, 0, len(demoPhases))
	pals := make([]theme.Palette, 0, len(demoPhases))
	for _, d := range demoPhases {
		band, pal, err := demoBand(cfg, text, d.phase, d.progress, false)
		if err != nil {
			return "", err
		}
		bands = append(bands, band)
		pals = append(pals, pal)
	}

	const gap = 2 // pixels of breathing room between bands
	width, height := 0, 0
	for _, b := range bands {
		if b.W > width {
			width = b.W
		}
		height += b.H + gap
	}
	height -= gap

	var b strings.Builder
	b.WriteString(svgHeader(width*pixel, height*pixel))
	// A neutral backdrop so the three panel colors read as distinct bands
	// rather than blending into whatever the page behind them is.
	b.WriteString(`<rect width="100%" height="100%" fill="#0b0a0f"/>`)
	b.WriteString("\n")

	y := 0
	for i, band := range bands {
		writeCanvasSVG(&b, band, pals[i].Panel, 0, y, pixel)
		y += band.H + gap
	}
	b.WriteString("</svg>\n")
	return b.String(), nil
}

func svgHeader(w, h int) string {
	return `<svg xmlns="http://www.w3.org/2000/svg" width="` + itoa(w) +
		`" height="` + itoa(h) + `" viewBox="0 0 ` + itoa(w) + " " + itoa(h) +
		`" shape-rendering="crispEdges" role="img" aria-label="pomo HUD art">` + "\n"
}

// writeCanvasSVG emits one rect per horizontal run of identical color. Emitting
// a rect per pixel would work but triples the file size for no visual gain.
func writeCanvasSVG(b *strings.Builder, c *canvas.Canvas, bg canvas.RGBA, ox, oy, pixel int) {
	for y := 0; y < c.H; y++ {
		x := 0
		for x < c.W {
			col := resolvePixel(c.At(x, y), bg)
			run := 1
			for x+run < c.W && resolvePixel(c.At(x+run, y), bg) == col {
				run++
			}
			b.WriteString(`<rect x="`)
			b.WriteString(itoa((ox + x) * pixel))
			b.WriteString(`" y="`)
			b.WriteString(itoa((oy + y) * pixel))
			b.WriteString(`" width="`)
			b.WriteString(itoa(run * pixel))
			b.WriteString(`" height="`)
			b.WriteString(itoa(pixel))
			b.WriteString(`" fill="`)
			b.WriteString(hexOf(col))
			b.WriteString(`"/>`)
			x += run
		}
		b.WriteString("\n")
	}
}

func resolvePixel(p, bg canvas.RGBA) canvas.RGBA {
	if p.A {
		return p
	}
	return canvas.RGBA{R: bg.R, G: bg.G, B: bg.B, A: true}
}

const hexDigits = "0123456789abcdef"

func hexOf(c canvas.RGBA) string {
	out := []byte("#000000")
	for i, v := range []uint8{c.R, c.G, c.B} {
		out[1+i*2] = hexDigits[v>>4]
		out[2+i*2] = hexDigits[v&0x0f]
	}
	return string(out)
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
