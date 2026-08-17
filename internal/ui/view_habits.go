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

// Column widths for the habit list. Fixed so the columns line up down the
// screen rather than drifting with the longest name.
//
// They sum, with the cursor and the gap before the streak, to the width of the
// HUD frame: 2 + 16 + 11 + 16 + 10 + 2 + 3 = 60. Widening any of them without
// narrowing another pushes the list past the frame it sits under.
const (
	habitNameWidth = 16
	habitGoalWidth = 11
	habitProgWidth = 16
	habitMeterWide = 10
)

// HabitsReport renders the habit list without a cursor, for `pomo -habits`. It
// is the same rows the [h] screen shows, so the two can never drift.
func HabitsReport(pal theme.Palette, habits []habit.Habit, progress map[string]store.HabitProgress) string {
	return habitsView(pal, habits, progress, -1, "")
}

// habitsView is the [h] screen: every habit with its current progress, and a
// cursor for picking one. A negative cursor draws no selection.
func habitsView(pal theme.Palette, habits []habit.Habit, progress map[string]store.HabitProgress, cursor int, activeID string) string {
	title := lipgloss.NewStyle().Foreground(lg(pal.Text)).Bold(true)
	accent := lipgloss.NewStyle().Foreground(lg(pal.Accent))
	faint := lipgloss.NewStyle().Foreground(lg(pal.TextDim))

	var b strings.Builder
	b.WriteString(title.Render("HABITS"))
	b.WriteString("\n\n")

	if len(habits) == 0 {
		b.WriteString("  " + faint.Render("No habits yet.") + "\n")
		b.WriteString("  " + faint.Render("A habit is a name and a daily target, like ") +
			accent.Render("work") + faint.Render(" for ") + accent.Render("4h") +
			faint.Render(" a day.") + "\n\n")
		b.WriteString("  " + accent.Render("a") + faint.Render(" add one") +
			faint.Render("   ") + accent.Render("esc") + faint.Render(" back") + "\n")
		return b.String()
	}

	for i, h := range habits {
		b.WriteString(habitRow(pal, h, progress[h.ID], i == cursor, h.ID == activeID))
		b.WriteString("\n")
	}
	return b.String()
}

// habitsLegend is the key hints for the habit list. The picking keys are
// suppressed when there is nothing to pick.
func habitsLegend(pal theme.Palette, any bool) string {
	if !any {
		return keyHints(pal, []string{"a add", "esc back"})
	}
	return keyHints(pal, []string{
		"j/k move", "enter start", "a add", "E edit", "d delete", "esc back",
	})
}

// formLegend is the key hints for the add/edit form.
func formLegend(pal theme.Palette) string {
	return keyHints(pal, []string{"tab field", "enter save", "esc cancel"})
}

// keyHints renders one line of "key description" pairs, styling the key.
func keyHints(pal theme.Palette, parts []string) string {
	key := lipgloss.NewStyle().Foreground(lg(pal.Accent)).Bold(true)
	faint := lipgloss.NewStyle().Foreground(lg(pal.TextDim))

	out := make([]string, 0, len(parts))
	for _, p := range parts {
		word, rest, _ := strings.Cut(p, " ")
		out = append(out, key.Render(word)+faint.Render(" "+rest))
	}
	return "  " + strings.Join(out, faint.Render("   "))
}

// habitRow renders one line of the habit list.
func habitRow(pal theme.Palette, h habit.Habit, p store.HabitProgress, selected, active bool) string {
	accent := lipgloss.NewStyle().Foreground(lg(habitAccent(pal, h)))
	dim := lipgloss.NewStyle().Foreground(lg(pal.AccentDim))
	text := lipgloss.NewStyle().Foreground(lg(pal.Text))
	faint := lipgloss.NewStyle().Foreground(lg(pal.TextDim))

	cursor := "  "
	if selected {
		cursor = accent.Render("▸ ")
	}

	name := truncate(h.Name, habitNameWidth)
	nameStyle := text
	if !active {
		nameStyle = faint
	}

	full := 0
	if p.Target > 0 {
		full = clampInt(int(p.Fraction*float64(habitMeterWide)), 0, habitMeterWide)
	}
	meter := accent.Render(strings.Repeat("▰", full)) +
		dim.Render(strings.Repeat("▱", habitMeterWide-full))

	streak := "  –"
	if p.Streak > 0 {
		streak = fmt.Sprintf("%3s", fmt.Sprintf("%dd", p.Streak))
		if h.Goal.Period == habit.Weekly {
			streak = fmt.Sprintf("%3s", fmt.Sprintf("%dw", p.Streak))
		}
	}

	return cursor +
		nameStyle.Render(padPlain(name, habitNameWidth)) +
		faint.Render(padPlain(h.Goal.Short(), habitGoalWidth)) +
		accent.Render(padPlain(progressText(h, p), habitProgWidth)) +
		meter + "  " + faint.Render(streak)
}

// progressText spells out where a habit stands in its current period.
func progressText(h habit.Habit, p store.HabitProgress) string {
	if p.Met {
		return "done"
	}
	switch h.Goal.Unit {
	case habit.Sessions:
		unit := "today"
		if h.Goal.Period == habit.Weekly {
			// "this wk" rather than "this week" so a two-digit weekly target
			// still fits the column.
			unit = "this wk"
		}
		return fmt.Sprintf("%d / %d %s", p.Value, p.Target, unit)
	default:
		return fmt.Sprintf("%s / %s", habit.FormatMinutes(p.Value), habit.FormatMinutes(p.Target))
	}
}

// habitAccent is the colour to draw a habit in: its own if set, otherwise the
// palette's. A colour that fails to parse falls back rather than erroring —
// habits are validated on save, so reaching here means the file was hand-edited
// and a wrong colour should not stop the screen rendering.
func habitAccent(pal theme.Palette, h habit.Habit) canvas.RGBA {
	if h.Color != "" {
		if c, err := canvas.ParseHex(h.Color); err == nil {
			return c
		}
	}
	return pal.Accent
}

// padPlain pads unstyled text to a width. Styling happens after padding, so the
// escape sequences never count toward the column.
func padPlain(s string, width int) string {
	if w := lipgloss.Width(s); w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return truncate(s, width)
}
