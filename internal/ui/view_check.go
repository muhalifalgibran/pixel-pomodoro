package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/habit"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/store"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/theme"
)

// Column widths for the check-off list. Like the habit list's, they are fixed
// so the columns line up down the screen, and they sum with the cursor and the
// gap before the streak to the width of the HUD frame:
// 2 + 4 + 21 + 18 + 10 + 2 + 3 = 60.
//
// There is no goal column here, unlike the habit list. progressText already
// spells the target out — "0 / 1 today", "1h 40m / 4h" — so a separate "4h"
// would only say it twice, and the room is better spent on the name.
const (
	checkBoxWidth  = 4
	checkNameWidth = 21
	checkProgWidth = 18
	checkMeterWide = 10
)

// TodayReport renders the check-off list without a cursor, for `pomo -today`.
// It is the same rows the [l] screen shows, so the two can never drift.
func TodayReport(pal theme.Palette, habits []habit.Habit, progress map[string]store.HabitProgress) string {
	return checkView(pal, habits, progress, -1, "", "")
}

// checkView is the [l] screen: today's habits as a checklist you can tick
// without running the timer, with a cursor for picking one. A negative cursor
// draws no selection.
func checkView(pal theme.Palette, habits []habit.Habit, progress map[string]store.HabitProgress, cursor int, activeID, status string) string {
	title := lipgloss.NewStyle().Foreground(lg(pal.Text)).Bold(true)
	accent := lipgloss.NewStyle().Foreground(lg(pal.Accent))
	faint := lipgloss.NewStyle().Foreground(lg(pal.TextDim))

	var b strings.Builder
	b.WriteString(title.Render("TODAY"))
	if n := len(habits); n > 0 {
		b.WriteString("  " + faint.Render(fmt.Sprintf("%d of %d done", metCount(habits, progress), n)))
	}
	b.WriteString("\n\n")

	if len(habits) == 0 {
		b.WriteString("  " + faint.Render("Nothing to tick off yet. Add a habit with") + " " +
			accent.Render("h") + " " + faint.Render("then") + " " + accent.Render("a") + ".\n")
		return b.String()
	}

	for i, h := range habits {
		b.WriteString(checkRow(pal, h, progress[h.ID], i == cursor, h.ID == activeID))
		b.WriteString("\n")
	}
	if status != "" {
		b.WriteString("\n  " + faint.Render(status) + "\n")
	}
	return b.String()
}

// metCount is how many of the listed habits have already met their goal, which
// is the one figure this screen has that the habit list does not.
func metCount(habits []habit.Habit, progress map[string]store.HabitProgress) int {
	n := 0
	for _, h := range habits {
		if progress[h.ID].Met {
			n++
		}
	}
	return n
}

// checkKeysFor picks the screen's key set. The ticking keys are suppressed when
// there is nothing to tick.
func checkKeysFor(any bool) []string {
	if !any {
		return checkEmptyKeys
	}
	return checkKeys
}

// checkRow renders one line of the check-off list.
func checkRow(pal theme.Palette, h habit.Habit, p store.HabitProgress, selected, active bool) string {
	accent := lipgloss.NewStyle().Foreground(lg(habitAccent(pal, h)))
	dim := lipgloss.NewStyle().Foreground(lg(pal.AccentDim))
	text := lipgloss.NewStyle().Foreground(lg(pal.Text))
	faint := lipgloss.NewStyle().Foreground(lg(pal.TextDim))

	cursor := "  "
	if selected {
		cursor = accent.Render("▸ ")
	}

	// ASCII rather than a check glyph: a ✔ is a width gamble across terminals,
	// and these rows are laid out to the cell.
	box := dim.Render(padPlain("[ ]", checkBoxWidth))
	if p.Met {
		box = accent.Render(padPlain("[x]", checkBoxWidth))
	}

	// A met habit reads in its own colour all the way across. That is this
	// screen's celebration: confetti lives in the HUD's pixel band, which this
	// view does not draw into, so bursting it here would show the user nothing.
	nameStyle := text
	switch {
	case p.Met:
		nameStyle = accent
	case !active:
		nameStyle = faint
	}

	return cursor + box +
		nameStyle.Render(padPlain(truncate(h.Name, checkNameWidth), checkNameWidth)) +
		accent.Render(padPlain(progressText(h, p), checkProgWidth)) +
		meterBar(pal, h, p, checkMeterWide) + "  " + faint.Render(streakText(h, p))
}
