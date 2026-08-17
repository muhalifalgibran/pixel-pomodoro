package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/canvas"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/habit"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/store"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/theme"
)

// chartHeight is how many text rows the fallback bar chart occupies. Each row is
// two pixels of half-block, so the chart resolves to 2*chartHeight steps.
const chartHeight = 5

// Contribution bar layout. The name column plus the bar and its suffix are sized
// to sit inside the HUD frame.
const (
	barNameWidth = 14
	// barSuffixWidth covers "  24/30   7d".
	barSuffixWidth = 13
	barMaxDays     = store.ChartDays
	barMinDays     = 7
)

// Shading glyphs for the contribution bar. Intensity is carried by the glyph as
// well as the colour, so the bar still reads in a terminal without truecolor.
var barGlyphs = [4]string{"·", "░", "▒", "█"}

// barDim scales the habit's colour for each level below "met", so a partial day
// reads as a fainter version of the same colour rather than a different one.
var barDim = [4]float64{1, 0.55, 0.8, 1}

// StatsReport is the [t] screen: totals, streak and a contribution bar per
// habit. It is plain text rather than pixel art — this screen is for reading.
// It is exported so `pomo -stats` prints exactly what the screen shows.
func StatsReport(pal theme.Palette, st store.Stats, habits []habit.Habit, progress map[string]store.HabitProgress, logPath string, width int) string {
	title := lipgloss.NewStyle().Foreground(lg(pal.Text)).Bold(true)
	accent := lipgloss.NewStyle().Foreground(lg(pal.Accent))
	dim := lipgloss.NewStyle().Foreground(lg(pal.AccentDim))
	faint := lipgloss.NewStyle().Foreground(lg(pal.TextDim))

	var b strings.Builder

	b.WriteString(title.Render("POMO STATS"))
	b.WriteString("\n\n")

	rows := [][2]string{
		{"Level", fmt.Sprintf("%d", st.Level)},
		{"XP", fmt.Sprintf("%d  (%d/%d into this level)", st.XP, st.XPIntoLevel, st.XPForLevel)},
		{"Streak", fmt.Sprintf("%d days", st.Streak)},
		{"Today", fmt.Sprintf("%d sessions, %s", st.TodaySessions, humanMins(st.TodayMins))},
		{"This week", humanMins(st.WeekMins)},
	}
	// Zen belongs to no habit, so it appears in no bar. Showing it here keeps
	// the time from looking like it went missing.
	if st.ZenWeekMins > 0 {
		rows = append(rows, [2]string{"Zen", humanMins(st.ZenWeekMins) + " this week"})
	}
	for _, r := range rows {
		b.WriteString("  " + faint.Render(fmt.Sprintf("%-10s", r[0])) + accent.Render(r[1]) + "\n")
	}

	if len(habits) > 0 {
		days := barDays(width)
		b.WriteString("\n" + title.Render(fmt.Sprintf("LAST %d DAYS", days)) + "\n\n")
		for _, h := range habits {
			b.WriteString(contributionBar(pal, h, progress[h.ID], days) + "\n")
		}
		b.WriteString("\n" + barKey(pal) + "\n")
	} else {
		// Without habits there are no goals to shade against, so fall back to
		// the plain activity chart.
		b.WriteString("\n  " + faint.Render(fmt.Sprintf("Last %d days", store.DaysCharted)) + "\n")
		b.WriteString(chart(pal, st.ByDay))
		b.WriteString("\n")
	}

	b.WriteString("\n  " + faint.Render("log  ") + dim.Render(logPath) + "\n")
	b.WriteString("\n  " + accent.Render("t") + faint.Render(" or ") + accent.Render("esc") + faint.Render(" back  ·  ") +
		accent.Render("q") + faint.Render(" quit"))

	return b.String()
}

// barDays is how many days fit in the available width. A narrow terminal shows
// a shorter stretch rather than a bar that overhangs the frame.
func barDays(width int) int {
	if width <= 0 {
		return barMaxDays
	}
	room := width - 2 - barNameWidth - barSuffixWidth
	return clampInt(room, barMinDays, barMaxDays)
}

// contributionBar renders one habit's recent history, one cell per day.
func contributionBar(pal theme.Palette, h habit.Habit, p store.HabitProgress, days int) string {
	base := habitAccent(pal, h)
	text := lipgloss.NewStyle().Foreground(lg(pal.Text))
	faint := lipgloss.NewStyle().Foreground(lg(pal.TextDim))

	cells := p.Days
	if len(cells) > days {
		// Keep the most recent days; the tail is what the user cares about.
		cells = cells[len(cells)-days:]
	}

	var bar strings.Builder
	for _, c := range cells {
		level := clampInt(c.Level, 0, len(barGlyphs)-1)
		col := canvas.Scale(base, barDim[level])
		if level == store.LevelNone {
			col = pal.AccentDim
		}
		bar.WriteString(lipgloss.NewStyle().Foreground(lg(col)).Render(barGlyphs[level]))
	}

	unit := "d"
	if h.Goal.Period == habit.Weekly {
		unit = "w"
	}
	streak := "  –"
	if p.Streak > 0 {
		streak = fmt.Sprintf("%3s", fmt.Sprintf("%d%s", p.Streak, unit))
	}

	return "  " + text.Render(padPlain(truncate(h.Name, barNameWidth), barNameWidth)) +
		bar.String() +
		faint.Render(fmt.Sprintf("  %2d/%-2d ", p.MetCount, p.Window)) +
		faint.Render(streak)
}

// barKey explains the shading, since the glyphs alone are not self-evident.
func barKey(pal theme.Palette) string {
	accent := lipgloss.NewStyle().Foreground(lg(pal.Accent))
	dim := lipgloss.NewStyle().Foreground(lg(pal.AccentDim))
	faint := lipgloss.NewStyle().Foreground(lg(pal.TextDim))

	return "  " +
		dim.Render(barGlyphs[0]) + faint.Render(" none   ") +
		lipgloss.NewStyle().Foreground(lg(canvas.Scale(pal.Accent, barDim[1]))).Render(barGlyphs[1]) +
		faint.Render(" under half   ") +
		lipgloss.NewStyle().Foreground(lg(canvas.Scale(pal.Accent, barDim[2]))).Render(barGlyphs[2]) +
		faint.Render(" close   ") +
		accent.Render(barGlyphs[3]) + faint.Render(" goal met")
}

// chart draws a half-block column chart, one column per day. It is the
// no-habits fallback: without goals there is nothing to shade against.
func chart(pal theme.Palette, days []store.DayTotal) string {
	if len(days) == 0 {
		return ""
	}
	accent := lipgloss.NewStyle().Foreground(lg(pal.Accent))
	dim := lipgloss.NewStyle().Foreground(lg(pal.AccentDim))

	peak := 0
	for _, d := range days {
		if d.Mins > peak {
			peak = d.Mins
		}
	}
	if peak == 0 {
		return "  " + dim.Render(strings.Repeat("· ", len(days)))
	}

	steps := chartHeight * 2 // two pixels per text row

	// Height in half-block steps for each column.
	heights := make([]int, len(days))
	for i, d := range days {
		h := d.Mins * steps / peak
		if d.Mins > 0 && h == 0 {
			h = 1 // a day with any work must not render as empty
		}
		heights[i] = h
	}

	var b strings.Builder
	for row := 0; row < chartHeight; row++ {
		b.WriteString("  ")
		// Steps covered by this row, counting from the top.
		top := (chartHeight - row) * 2
		for _, h := range heights {
			switch {
			case h >= top:
				b.WriteString(accent.Render("█"))
			case h == top-1:
				b.WriteString(accent.Render("▄"))
			default:
				b.WriteString(" ")
			}
			b.WriteString(" ")
		}
		b.WriteString("\n")
	}

	// Day-of-month labels, two cells per column to match the bars.
	b.WriteString("  ")
	for _, d := range days {
		b.WriteString(dim.Render(fmt.Sprintf("%-2d", d.Date.Day()%100)))
	}
	return b.String()
}

func humanMins(m int) string {
	if m < 60 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dh %02dm", m/60, m%60)
}
