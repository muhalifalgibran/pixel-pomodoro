package ui

import (
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/muhalifalgibran/pixel-pomodoro/assets"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/anim"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/canvas"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/sprite"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/store"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/theme"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/timer"
)

// hudView composites the timer HUD: the art band inside its pixel frame, with
// the status, progress and task lines, and the key legend beneath.
func (m *Model) hudView(pal theme.Palette) string {
	band := canvas.New(m.geom.BandW, m.geom.BandH)

	b := m.breath()
	osc := 2*anim.Pulse(m.elapsed, b.period) - 1
	// A one-pixel bob is half a cell, which is exactly the motion half-blocks
	// buy. Forcing the offset even quantised it to two-pixel jumps, and on
	// negative values Go's &^ rounds away from zero, so the mascot lurched
	// down twice as far as it rose.
	bob := int(math.Round(b.bob * osc))

	// Zen has no phase length, so there is nothing for the mascot to drain
	// against and it stays full.
	drain := m.timer.Progress()
	if m.zen {
		drain = 0
	}
	blitSquash(
		band, m.tomato.Canvas,
		m.geom.SpriteX, m.geom.SpriteY+bob,
		1+b.squash*osc,
		spriteTransform(m.tomato, pal, drain),
	)

	m.steam.Draw(band)

	// Zen has no boundary to escalate toward, so it never enters the alert
	// state however long it runs.
	style, alert := pal.Clock, false
	if !m.zen {
		style, alert = clockStyleFor(pal, m.timer.Remaining, m.timer.Running)
	}
	jitter := 0
	if alert {
		// Two-frame shake, fast enough to read as urgency.
		jitter = int(m.elapsed*8)%2*2 - 1
	}
	band.Blit(m.clk.draw(style, pal.Panel, alert, jitter), m.geom.ClockX, m.geom.ClockY)

	m.confetti.Draw(band)

	if m.cfg.Scanlines {
		applyScanlines(band)
	}

	rows := strings.Split(band.Render(pal.Panel), "\n")
	content := make([]string, 0, len(rows)+3)
	content = append(content, statusBar(pal, m.stats, m.habitStreak(), m.geom.BandW))
	content = append(content, rows...)

	switch {
	case m.zen:
		content = append(content, zenBar(pal, m.zenElapsed, m.zenRunning, m.geom.BandW))
		content = append(content, zenLine(pal, m.zenElapsed, m.geom.BandW))
	default:
		content = append(content, progressBar(pal, m.timer, m.geom.BandW))
		// A habit shows its own goal progress; without one the free-text task
		// line is what the timer had before habits existed.
		if h, ok := m.activeHabit(); ok && m.mode != modeEditTask {
			content = append(content, habitLine(pal, h, m.progress[h.ID], m.resumed, m.geom.BandW))
		} else {
			content = append(content, taskLine(pal, m.displayTask(), m.mode == modeEditTask, m.resumed, m.geom.BandW))
		}
	}

	out := frameLines(pal, m.geom.BandW, content)
	out = append(out, m.helpRowsFor(pal)...)
	return strings.Join(out, "\n")
}

// helpRowsFor returns the legend, the one-line hint, or the editing keys.
// While editing, the legend is shown regardless of the toggle: enter and esc
// are not guessable, and being stuck in a text field is worse than a few extra
// rows.
func (m *Model) helpRowsFor(pal theme.Palette) []string {
	if m.mode == modeEditTask {
		return helpBlock(pal, editingKeys)
	}
	if !m.showHelp {
		return helpHint(pal)
	}
	if m.zen {
		return helpBlock(pal, zenKeys)
	}
	return helpBlock(pal, timerKeys)
}

// requiredHeight is the rows the full HUD needs: two borders, the status bar,
// the art band, the progress and task lines, and however many rows the legend
// currently occupies.
func (m *Model) requiredHeight() int {
	help := 1
	if m.showHelp || m.mode == modeEditTask {
		help = helpRows
	}
	return 2 + 3 + m.geom.BandH/2 + help
}

// breath is the mascot's idle motion for whatever is running.
func (m *Model) breath() breath {
	if m.zen {
		return breathZen(m.zenRunning)
	}
	return breathFor(m.timer.Phase, m.timer.Running)
}

// habitStreak is the streak the status bar shows: the active habit's own run of
// met goals, or the global any-session streak when no habit is selected.
func (m *Model) habitStreak() int {
	if h, ok := m.activeHabit(); ok {
		return m.progress[h.ID].Streak
	}
	return m.stats.Streak
}

func (m *Model) displayTask() string {
	if m.mode == modeEditTask {
		return m.taskInput
	}
	return m.timer.Task
}

// compactView is the fallback for terminals too small for the art.
func compactView(pal theme.Palette, s *timer.State, clockText string, st store.Stats) string {
	text := lipgloss.NewStyle().Foreground(lg(pal.Text)).Bold(true)
	accent := lipgloss.NewStyle().Foreground(lg(pal.Accent))
	faint := lipgloss.NewStyle().Foreground(lg(pal.TextDim))

	label := strings.ToUpper(s.Phase.String())
	if !s.Running {
		label = "PAUSED"
	}
	lines := []string{
		text.Render(clockText) + "  " + accent.Render(label),
		faint.Render(meterFrac("▰", "▱", s.Progress(), 16)),
		faint.Render("LV." + strconv.Itoa(st.Level) + "  " + strconv.Itoa(st.Streak) + "d streak"),
		faint.Render("space pause · s skip · q quit"),
	}
	return strings.Join(lines, "\n")
}

// loadTomato parses the embedded mascot.
func loadTomato() (*sprite.Sprite, error) { return assets.Sprite("tomato") }
