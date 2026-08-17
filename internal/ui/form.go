package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/theme"
)

// formAction is what a keypress asked the form to do.
type formAction int

const (
	formContinue formAction = iota
	formSave
	formCancel
)

// field is one labelled text input.
type field struct {
	label string
	value string
	// hint shows when the field is empty, to say what belongs there without
	// needing a separate help screen.
	hint string
}

// form is a small vertical group of text fields. It is deliberately minimal:
// the habit editor is the only form here, and a full input framework would be
// more machinery than the job needs.
type form struct {
	title  string
	fields []field
	cursor int
	// err is the last validation failure, shown under the fields so the user
	// can see what to fix without losing what they typed.
	err string
}

// value returns a field's trimmed contents.
func (f *form) value(i int) string {
	if i < 0 || i >= len(f.fields) {
		return ""
	}
	return strings.TrimSpace(f.fields[i].value)
}

// key routes a keypress. Editing keys are handled here; save and cancel are
// reported back so the caller decides what they mean.
func (f *form) key(msg tea.KeyMsg) formAction {
	switch msg.Type {
	case tea.KeyEnter:
		return formSave
	case tea.KeyEsc:
		return formCancel
	case tea.KeyTab, tea.KeyDown:
		f.move(1)
	case tea.KeyShiftTab, tea.KeyUp:
		f.move(-1)
	case tea.KeyBackspace:
		r := []rune(f.fields[f.cursor].value)
		if len(r) > 0 {
			f.fields[f.cursor].value = string(r[:len(r)-1])
		}
	case tea.KeyRunes, tea.KeySpace:
		// A space arrives as KeySpace with Runes already holding " ", so
		// appending the rune is enough.
		f.fields[f.cursor].value += string(msg.Runes)
	}
	return formContinue
}

func (f *form) move(by int) {
	if len(f.fields) == 0 {
		return
	}
	f.cursor = (f.cursor + by + len(f.fields)) % len(f.fields)
}

// formRows is the height a form occupies, so the caller can reason about
// whether it fits.
func (f *form) rows() int { return len(f.fields) + 4 }

// view renders the form.
func (f *form) view(pal theme.Palette) string {
	title := lipgloss.NewStyle().Foreground(lg(pal.Text)).Bold(true)
	accent := lipgloss.NewStyle().Foreground(lg(pal.Accent))
	text := lipgloss.NewStyle().Foreground(lg(pal.Text))
	faint := lipgloss.NewStyle().Foreground(lg(pal.TextDim))
	warn := lipgloss.NewStyle().Foreground(lg(pal.Accent)).Bold(true)

	const labelWidth = 10

	var b strings.Builder
	b.WriteString(title.Render(f.title))
	b.WriteString("\n\n")

	for i, fl := range f.fields {
		marker := "  "
		if i == f.cursor {
			marker = accent.Render("▸ ")
		}
		body := text.Render(fl.value)
		if i == f.cursor {
			body += accent.Render("█")
		} else if fl.value == "" {
			body = faint.Render(fl.hint)
		}
		b.WriteString(marker + faint.Render(padPlain(fl.label, labelWidth)) + body + "\n")
	}

	if f.err != "" {
		b.WriteString("\n  " + warn.Render("× "+f.err) + "\n")
	}
	return b.String()
}

// confirmPrompt is a yes/no question with the action it will take.
type confirmPrompt struct {
	message string
	detail  string
	// run performs the action. It is only called on an explicit yes.
	run func()
}

// view renders the prompt.
func (c confirmPrompt) view(pal theme.Palette) string {
	title := lipgloss.NewStyle().Foreground(lg(pal.Text)).Bold(true)
	accent := lipgloss.NewStyle().Foreground(lg(pal.Accent))
	faint := lipgloss.NewStyle().Foreground(lg(pal.TextDim))

	var b strings.Builder
	b.WriteString(title.Render(c.message))
	b.WriteString("\n\n")
	if c.detail != "" {
		b.WriteString("  " + faint.Render(c.detail) + "\n\n")
	}
	b.WriteString("  " + accent.Render("y") + faint.Render(" yes   ") +
		accent.Render("n") + faint.Render(" or ") + accent.Render("esc") + faint.Render(" no") + "\n")
	return b.String()
}
