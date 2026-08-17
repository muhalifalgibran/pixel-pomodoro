package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/canvas"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/store"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/theme"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/timer"
)

// Frame glyphs. The heavy quadrants read as a chunky pixel border rather than
// a window chrome box.
const (
	frameTopLeft     = "▛"
	frameTop         = "▀"
	frameTopRight    = "▜"
	frameLeft        = "▌"
	frameRight       = "▐"
	frameBottomLeft  = "▙"
	frameBottom      = "▄"
	frameBottomRight = "▟"
)

func lg(c canvas.RGBA) lipgloss.Color {
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B))
}

// pad extends s with spaces to exactly width display cells, truncating if it
// overflows. Every HUD line goes through this so the frame edges stay aligned
// regardless of what the user typed as a task.
func pad(s string, width int) string {
	w := lipgloss.Width(s)
	if w > width {
		return truncate(s, width)
	}
	return s + strings.Repeat(" ", width-w)
}

// truncate cuts s to max display cells. It operates on the raw string, so it
// is only safe for unstyled text; styled segments are padded after styling.
func truncate(s string, max int) string {
	if runewidth.StringWidth(s) <= max {
		return s
	}
	if max <= 1 {
		return strings.Repeat("…", max)
	}
	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if w+rw > max-1 {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	return b.String() + "…"
}

// meter renders a proportional bar with distinct filled and empty runes.
func meter(filled, empty string, value, total, width int) string {
	if width <= 0 {
		return ""
	}
	n := 0
	if total > 0 {
		n = value * width / total
	}
	if n < 0 {
		n = 0
	}
	if n > width {
		n = width
	}
	return strings.Repeat(filled, n) + strings.Repeat(empty, width-n)
}

// meterFrac is meter for a 0..1 fraction.
func meterFrac(filled, empty string, frac float64, width int) string {
	return meter(filled, empty, int(frac*1000), 1000, width)
}

// statusBar is the top line: level, XP progress and streak.
func statusBar(pal theme.Palette, st store.Stats, width int) string {
	accent := lipgloss.NewStyle().Foreground(lg(pal.Accent))
	dim := lipgloss.NewStyle().Foreground(lg(pal.AccentDim))
	text := lipgloss.NewStyle().Foreground(lg(pal.Text))
	faint := lipgloss.NewStyle().Foreground(lg(pal.TextDim))

	const xpBarWidth = 8
	filled := 0
	if st.XPForLevel > 0 {
		filled = st.XPIntoLevel * xpBarWidth / st.XPForLevel
	}
	filled = clampInt(filled, 0, xpBarWidth)

	left := " " + text.Render(fmt.Sprintf("LV.%d", st.Level)) + "  " +
		accent.Render(strings.Repeat("▮", filled)) +
		dim.Render(strings.Repeat("▯", xpBarWidth-filled)) +
		" " + faint.Render(fmt.Sprintf("%dxp", st.XP))

	right := faint.Render("STREAK ") + accent.Render(fmt.Sprintf("%dd", st.Streak)) + " "

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return pad(left, width)
	}
	return left + strings.Repeat(" ", gap) + right
}

// progressBar is the line under the band: phase progress, phase name and the
// cycle dots.
func progressBar(pal theme.Palette, s *timer.State, width int) string {
	accent := lipgloss.NewStyle().Foreground(lg(pal.Accent))
	dim := lipgloss.NewStyle().Foreground(lg(pal.AccentDim))
	text := lipgloss.NewStyle().Foreground(lg(pal.Text))
	faint := lipgloss.NewStyle().Foreground(lg(pal.TextDim))

	const barWidth = 20
	frac := s.Progress()
	full := clampInt(int(frac*float64(barWidth)), 0, barWidth)
	bar := accent.Render(strings.Repeat("▰", full)) + dim.Render(strings.Repeat("▱", barWidth-full))

	label := strings.ToUpper(s.Phase.String())
	if !s.Running {
		label = "PAUSED"
	}

	every := s.Config().LongBreakEvery
	dots := make([]string, 0, every)
	for i := 0; i < every; i++ {
		if i < s.CycleIndex {
			dots = append(dots, accent.Render("●"))
		} else {
			dots = append(dots, dim.Render("○"))
		}
	}

	line := "  " + bar + "  " + text.Render(label) + "  " + strings.Join(dots, " ") +
		"  " + faint.Render(fmt.Sprintf("%d/%d", s.CycleIndex, every))
	return pad(line, width)
}

// taskLine shows what the current session is for.
func taskLine(pal theme.Palette, task string, editing bool, width int) string {
	accent := lipgloss.NewStyle().Foreground(lg(pal.Accent))
	text := lipgloss.NewStyle().Foreground(lg(pal.Text))
	faint := lipgloss.NewStyle().Foreground(lg(pal.TextDim))

	// Reserve the marker, the two leading spaces and a little slack.
	room := width - 6
	switch {
	case editing:
		return pad("  "+accent.Render("▶ ")+text.Render(truncate(task, room))+accent.Render("█"), width)
	case task == "":
		return pad("  "+faint.Render("▶ no task — press e to name one"), width)
	default:
		return pad("  "+accent.Render("▶ ")+text.Render(truncate(task, room)), width)
	}
}

// frameLines wraps content rows in the pixel border. Content rows must already
// be exactly width display cells wide.
func frameLines(pal theme.Palette, width int, rows []string) []string {
	edge := lipgloss.NewStyle().Foreground(lg(pal.Frame))
	dim := lipgloss.NewStyle().Foreground(lg(pal.FrameDim))

	out := make([]string, 0, len(rows)+2)
	out = append(out, edge.Render(frameTopLeft+strings.Repeat(frameTop, width)+frameTopRight))
	for _, r := range rows {
		out = append(out, edge.Render(frameLeft)+r+dim.Render(frameRight))
	}
	out = append(out, dim.Render(frameBottomLeft+strings.Repeat(frameBottom, width)+frameBottomRight))
	return out
}

// helpRows is the height of the key-hint block below the frame. Hints fill
// downward: the first helpRows entries form the first column, the next the
// second, and so on.
const helpRows = 3

// helpColumnGap separates the columns. Wide enough that the eye reads columns
// rather than one run-on line.
const helpColumnGap = 4

// helpBlock lays the key hints out as a grid. A single long line pushed the
// hints wider than the HUD frame above them; columns keep the block inside the
// frame's footprint and group related keys vertically.
func helpBlock(pal theme.Palette, editing bool) []string {
	key := lipgloss.NewStyle().Foreground(lg(pal.Accent)).Bold(true)
	faint := lipgloss.NewStyle().Foreground(lg(pal.TextDim))

	parts := []string{"space pause", "s skip", "r reset", "e task", "t stats", "q quit"}
	if editing {
		parts = []string{"enter save", "esc cancel"}
	}

	rendered := make([]string, len(parts))
	widths := make([]int, len(parts))
	for i, p := range parts {
		word, rest, _ := strings.Cut(p, " ")
		rendered[i] = key.Render(word) + faint.Render(" "+rest)
		widths[i] = lipgloss.Width(rendered[i])
	}

	cols := (len(parts) + helpRows - 1) / helpRows
	colWidth := make([]int, cols)
	for i, w := range widths {
		if c := i / helpRows; w > colWidth[c] {
			colWidth[c] = w
		}
	}

	// Every cell is padded to its column width, including the last, so the
	// block is a clean rectangle. Trailing spaces also overwrite whatever the
	// previous frame left on those cells.
	total := 1
	for c, w := range colWidth {
		total += w
		if c < cols-1 {
			total += helpColumnGap
		}
	}

	out := make([]string, helpRows)
	for r := 0; r < helpRows; r++ {
		var b strings.Builder
		b.WriteString(" ")
		for c := 0; c < cols; c++ {
			i := c*helpRows + r
			if i >= len(parts) {
				break
			}
			b.WriteString(rendered[i])
			gap := colWidth[c] - widths[i]
			if c < cols-1 {
				gap += helpColumnGap
			}
			b.WriteString(strings.Repeat(" ", gap))
		}
		out[r] = pad(b.String(), total)
	}
	return out
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
