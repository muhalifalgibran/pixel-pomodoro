// Package sprite parses .pix files into canvases. The format is plain text so
// art can be redrawn without recompiling.
//
//	# comment
//	size: 24 24
//	palette:
//	  . = transparent
//	  R = #e53935    trailing words are ignored, use them as notes
//	pixels:
//	  ...RRR...
//
// Parse errors always carry a line number: silently rendering malformed art is
// far worse than refusing to start.
package sprite

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/canvas"
)

// Sprite is parsed art plus the palette it was drawn with, kept so callers can
// recolor by palette key rather than by matching RGB values.
type Sprite struct {
	Name    string
	Canvas  *canvas.Canvas
	Palette map[byte]canvas.RGBA
	// Keys holds the palette key of every pixel, same layout as the canvas.
	// A zero byte means the pixel was transparent.
	Keys []byte
}

// KeyAt returns the palette key used at (x, y), or 0 when out of bounds or
// transparent.
func (s *Sprite) KeyAt(x, y int) byte {
	if x < 0 || y < 0 || x >= s.Canvas.W || y >= s.Canvas.H {
		return 0
	}
	return s.Keys[y*s.Canvas.W+x]
}

type parseState int

const (
	stateHeader parseState = iota
	statePalette
	statePixels
)

// Parse reads a .pix file. name is used only in error messages.
func Parse(name string, r io.Reader) (*Sprite, error) {
	var (
		sc      = bufio.NewScanner(r)
		state   = stateHeader
		w, h    = -1, -1
		palette = map[byte]canvas.RGBA{}
		rows    []string
		lineNo  int
	)

	for sc.Scan() {
		lineNo++
		raw := sc.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		switch {
		case strings.HasPrefix(line, "size:"):
			var err error
			w, h, err = parseSize(strings.TrimPrefix(line, "size:"))
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %w", name, lineNo, err)
			}
			continue
		case line == "palette:":
			state = statePalette
			continue
		case line == "pixels:":
			if w < 0 {
				return nil, fmt.Errorf("%s:%d: pixels: block before size:", name, lineNo)
			}
			state = statePixels
			continue
		}

		switch state {
		case statePalette:
			key, col, err := parsePaletteEntry(line)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %w", name, lineNo, err)
			}
			palette[key] = col
		case statePixels:
			// Use the raw line minus indentation so a trailing space cannot
			// silently widen a row.
			row := strings.TrimRight(strings.TrimLeft(raw, " \t"), " \t")
			if len(row) != w {
				return nil, fmt.Errorf("%s:%d: row is %d wide, size: declares %d", name, lineNo, len(row), w)
			}
			rows = append(rows, row)
		default:
			return nil, fmt.Errorf("%s:%d: unexpected %q outside palette:/pixels: block", name, lineNo, line)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}

	if w < 0 || h < 0 {
		return nil, fmt.Errorf("%s: missing size: header", name)
	}
	if len(rows) != h {
		return nil, fmt.Errorf("%s: got %d pixel rows, size: declares %d", name, len(rows), h)
	}

	cv := canvas.New(w, h)
	keys := make([]byte, w*h)
	for y, row := range rows {
		for x := 0; x < w; x++ {
			k := row[x]
			col, ok := palette[k]
			if !ok {
				return nil, fmt.Errorf("%s: pixel row %d col %d uses %q, which is not in the palette", name, y, x, string(k))
			}
			if !col.A {
				continue
			}
			cv.Set(x, y, col)
			keys[y*w+x] = k
		}
	}
	return &Sprite{Name: name, Canvas: cv, Palette: palette, Keys: keys}, nil
}

func parseSize(s string) (int, int, error) {
	f := strings.Fields(s)
	if len(f) != 2 {
		return 0, 0, fmt.Errorf("size: wants two numbers, got %q", strings.TrimSpace(s))
	}
	w, err := strconv.Atoi(f[0])
	if err != nil || w <= 0 {
		return 0, 0, fmt.Errorf("size: bad width %q", f[0])
	}
	h, err := strconv.Atoi(f[1])
	if err != nil || h <= 0 {
		return 0, 0, fmt.Errorf("size: bad height %q", f[1])
	}
	if h%2 != 0 {
		return 0, 0, fmt.Errorf("size: height %d must be even (half-block pairs)", h)
	}
	return w, h, nil
}

func parsePaletteEntry(line string) (byte, canvas.RGBA, error) {
	eq := strings.Index(line, "=")
	if eq < 0 {
		return 0, canvas.RGBA{}, fmt.Errorf("palette entry %q wants the form 'K = #rrggbb'", line)
	}
	key := strings.TrimSpace(line[:eq])
	if len(key) != 1 {
		return 0, canvas.RGBA{}, fmt.Errorf("palette key %q must be exactly one character", key)
	}
	rest := strings.Fields(line[eq+1:])
	if len(rest) == 0 {
		return 0, canvas.RGBA{}, fmt.Errorf("palette key %q has no color", key)
	}
	if rest[0] == "transparent" {
		return key[0], canvas.Transparent, nil
	}
	col, err := canvas.ParseHex(rest[0])
	if err != nil {
		return 0, canvas.RGBA{}, fmt.Errorf("palette key %q: %w", key, err)
	}
	return key[0], col, nil
}
