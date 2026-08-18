package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/canvas"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/habit"
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
func statusBar(pal theme.Palette, st store.Stats, streak, width int) string {
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

	right := faint.Render("STREAK ") + accent.Render(fmt.Sprintf("%dd", streak)) + " "

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

// resumedNote marks a session picked up from a previous run, so the clock
// starting at an odd number reads as intentional.
const resumedNote = "↻ resumed"

// habitGoalMeter is the width of the goal meter on the HUD's habit line.
const habitGoalMeter = 10

// habitLine replaces the task line when a habit is active: what you are working
// on, and how far into today's goal it is.
func habitLine(pal theme.Palette, h habit.Habit, p store.HabitProgress, resumed bool, width int) string {
	accent := lipgloss.NewStyle().Foreground(lg(habitAccent(pal, h)))
	dim := lipgloss.NewStyle().Foreground(lg(pal.AccentDim))
	text := lipgloss.NewStyle().Foreground(lg(pal.Text))
	faint := lipgloss.NewStyle().Foreground(lg(pal.TextDim))

	full := 0
	if p.Target > 0 {
		full = clampInt(int(p.Fraction*float64(habitGoalMeter)), 0, habitGoalMeter)
	}
	meter := accent.Render(strings.Repeat("▰", full)) +
		dim.Render(strings.Repeat("▱", habitGoalMeter-full))

	left := "  " + accent.Render("▶ ") + text.Render(truncate(h.Name, 18)) +
		"  " + faint.Render(progressText(h, p)) + "  " + meter

	right := ""
	if resumed {
		right = faint.Render(resumedNote) + " "
	}
	if right == "" {
		return pad(left, width)
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return pad(left, width)
	}
	return left + strings.Repeat(" ", gap) + right
}

// zenBar stands in for the progress bar during zen. There is no target, so
// there is no proportion to fill; a slow travelling pulse says "running"
// without implying an end.
func zenBar(pal theme.Palette, elapsed time.Duration, running bool, width int) string {
	accent := lipgloss.NewStyle().Foreground(lg(pal.Accent))
	dim := lipgloss.NewStyle().Foreground(lg(pal.AccentDim))
	text := lipgloss.NewStyle().Foreground(lg(pal.Text))

	const barWidth = 20
	label := "ZEN"
	if !running {
		label = "PAUSED"
	}

	cells := make([]string, barWidth)
	for i := range cells {
		cells[i] = dim.Render("▱")
	}
	if running {
		// One cell sweeping left to right, a second per step.
		at := int(elapsed/time.Second) % barWidth
		cells[at] = accent.Render("▰")
	}

	return pad("  "+strings.Join(cells, "")+"  "+text.Render(label), width)
}

// zenLine spells the elapsed time out in full.
//
// The big clock is five glyphs so the HUD never reflows, which means it cannot
// say whether "05:30" is five minutes or five hours. This line is where that is
// resolved.
func zenLine(pal theme.Palette, elapsed time.Duration, width int) string {
	accent := lipgloss.NewStyle().Foreground(lg(pal.Accent))
	faint := lipgloss.NewStyle().Foreground(lg(pal.TextDim))

	return pad("  "+accent.Render("◈ ")+faint.Render("zen · "+SpellElapsed(elapsed)), width)
}

// taskLine shows what the current session is for.
func taskLine(pal theme.Palette, task string, editing, resumed bool, width int) string {
	accent := lipgloss.NewStyle().Foreground(lg(pal.Accent))
	text := lipgloss.NewStyle().Foreground(lg(pal.Text))
	faint := lipgloss.NewStyle().Foreground(lg(pal.TextDim))

	note := ""
	// The note is suppressed while editing: the cursor should be the last
	// thing on the line.
	if resumed && !editing {
		note = faint.Render(resumedNote) + " "
	}

	// Reserve the marker, the leading spaces, the note and a little slack.
	room := width - 6 - lipgloss.Width(note)

	var left string
	switch {
	case editing:
		left = "  " + accent.Render("▶ ") + text.Render(truncate(task, room)) + accent.Render("█")
	case task == "":
		left = "  " + faint.Render("▶ no task — press e to name one")
	default:
		left = "  " + accent.Render("▶ ") + text.Render(truncate(task, room))
	}

	if note == "" {
		return pad(left, width)
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(note)
	if gap < 1 {
		return pad(left, width)
	}
	return left + strings.Repeat(" ", gap) + note
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

// helpHint stands in for the legend when it is hidden. Hiding it entirely would
// leave no way to discover the toggle again.
//
// A single hint is the shortest set there is, so it stays on one line like any
// other short set. The frame sits above the legend and never moves either way,
// and requiredHeight reserves the tallest legend regardless, so the collapsed
// state costs nothing.
func helpHint(pal theme.Palette) []string {
	return []string{helpInline(pal, []string{"/ keys"})}
}

// helpFits reports whether a key set would go on one line in the given width.
// Used by the tests to state the intent in terms of the rule rather than of a
// particular set's size.
func helpFits(pal theme.Palette, parts []string, width int) bool {
	return len(helpBlock(pal, parts, width)) == 1
}

// Key sets for each screen. Order matters: hints fill downward, so entries are
// grouped in threes to make full columns rather than stranding a primary key in
// a near-empty one.
var (
	// A tenth entry makes a four-column grid with a single cell in the last
	// column, so the least important key goes last rather than stranding a
	// primary one out there on its own.
	timerKeys = []string{
		"space pause", "s skip", "r reset",
		"h habits", "l log", "t stats",
		"z zen", "e note", "q quit",
		"/ hide",
	}
	// Zen deliberately does not advertise l. This set fits on one line with
	// nothing to spare, and a sixth entry would drop it into the grid; the key
	// still works, since it belongs to normal mode either way.
	zenKeys = []string{
		"space pause", "z stop", "t stats",
		"q quit", "/ hide",
	}
	editingKeys = []string{"enter save", "esc cancel"}

	habitsKeys = []string{
		"j/k move", "enter start", "a add",
		"E edit", "d delete", "esc back",
	}
	// The space entry is rewritten per row by checkKeysFor to name the amount
	// that press would credit; checkSpaceKey is where it sits.
	checkKeys = []string{
		"j/k move", "space skip 25m", "u undo",
		"enter start", "esc back", "q quit",
	}
	checkEmptyKeys  = []string{"h habits", "esc back"}
	habitsEmptyKeys = []string{"a add", "esc back"}
	formKeys        = []string{"tab field", "enter save", "esc cancel"}
	confirmKeys     = []string{"y yes", "n no", "esc cancel"}
	statsKeys       = []string{"t back", "esc back", "q quit"}
)

// helpBlock lays the key hints out on one line where they fit, and in columns
// where they do not.
//
// Deciding on the measured width rather than on a hint count is what keeps a
// legend from ever running past the space it has: a set that fits stays on one
// line, and the same set falls back to the grid on a narrower terminal instead
// of wrapping. width is the space available — the frame's width under the HUD,
// the terminal's on a full-screen view. A width of zero means unknown, and
// takes the grid as the safe choice.
func helpBlock(pal theme.Palette, parts []string, width int) []string {
	if len(parts) == 0 {
		return []string{""}
	}
	if line := helpInline(pal, parts); width > 0 && lipgloss.Width(line) <= width {
		return []string{line}
	}
	// helpRows is the preferred shape, but a cramped width needs a taller, and
	// therefore narrower, grid. Growing rows until it fits keeps the promise
	// that a legend never exceeds the space it was given.
	for rows := helpRows; rows < len(parts); rows++ {
		g := helpGrid(pal, parts, rows)
		if width <= 0 || widest(g) <= width {
			return g
		}
	}
	// One column is as narrow as it gets.
	return helpGrid(pal, parts, len(parts))
}

// widest is the display width of the longest row.
func widest(rows []string) int {
	w := 0
	for _, r := range rows {
		if n := lipgloss.Width(r); n > w {
			w = n
		}
	}
	return w
}

// helpInline renders a short key set as one line.
func helpInline(pal theme.Palette, parts []string) string {
	key := lipgloss.NewStyle().Foreground(lg(pal.Accent)).Bold(true)
	faint := lipgloss.NewStyle().Foreground(lg(pal.TextDim))

	out := make([]string, 0, len(parts))
	for _, p := range parts {
		word, rest, _ := strings.Cut(p, " ")
		out = append(out, key.Render(word)+faint.Render(" "+rest))
	}
	return " " + strings.Join(out, faint.Render("  ·  "))
}

// helpGrid lays a key set out in columns of rows, filling downward.
func helpGrid(pal theme.Palette, parts []string, rows int) []string {
	key := lipgloss.NewStyle().Foreground(lg(pal.Accent)).Bold(true)
	faint := lipgloss.NewStyle().Foreground(lg(pal.TextDim))

	rendered := make([]string, len(parts))
	widths := make([]int, len(parts))
	for i, p := range parts {
		word, rest, _ := strings.Cut(p, " ")
		rendered[i] = key.Render(word) + faint.Render(" "+rest)
		widths[i] = lipgloss.Width(rendered[i])
	}

	if rows < 1 {
		rows = 1
	}
	cols := (len(parts) + rows - 1) / rows
	colWidth := make([]int, cols)
	for i, w := range widths {
		if c := i / rows; w > colWidth[c] {
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

	out := make([]string, rows)
	for r := 0; r < rows; r++ {
		var b strings.Builder
		b.WriteString(" ")
		for c := 0; c < cols; c++ {
			i := c*rows + r
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
