package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/store"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/theme"
)

// chartHeight is how many text rows the daily bar chart occupies. Each row is
// two pixels of half-block, so the chart resolves to 2*chartHeight steps.
const chartHeight = 5

// StatsReport is the [t] screen: totals, streak and a bar chart of the last
// two weeks. It is plain text rather than pixel art — this screen is for
// reading. It is exported so `pomo -stats` can print the same thing without
// starting the TUI.
func StatsReport(pal theme.Palette, st store.Stats, logPath string) string {
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
	for _, r := range rows {
		b.WriteString("  " + faint.Render(fmt.Sprintf("%-10s", r[0])) + accent.Render(r[1]) + "\n")
	}

	b.WriteString("\n  " + faint.Render(fmt.Sprintf("Last %d days", store.DaysCharted)) + "\n")
	b.WriteString(chart(pal, st.ByDay))
	b.WriteString("\n\n")

	b.WriteString("  " + faint.Render("log  ") + dim.Render(logPath) + "\n")
	b.WriteString("\n  " + accent.Render("t") + faint.Render(" or ") + accent.Render("esc") + faint.Render(" back  ·  ") +
		accent.Render("q") + faint.Render(" quit"))

	return b.String()
}

// chart draws a half-block column chart, one column per day.
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
