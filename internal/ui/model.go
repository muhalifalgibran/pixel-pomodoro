package ui

import (
	"math"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/muhalifalgibran/pixel-pomodoro/assets"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/anim"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/canvas"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/config"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/notify"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/sprite"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/store"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/theme"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/timer"
)

const (
	// frameInterval is the render tick. 20fps is smooth enough for the
	// breathing and particles without burning a core.
	frameInterval = 50 * time.Millisecond
	// paletteFade is how long a phase change takes to cross-fade.
	paletteFade = 400 * time.Millisecond

	steamPoolSize    = 48
	confettiPoolSize = 90
	confettiHold     = 1500 * time.Millisecond
)

type mode int

const (
	modeNormal mode = iota
	modeStats
	modeEditTask
)

type tickMsg time.Time

// Options configures a Model beyond the user's config file.
type Options struct {
	Config config.Config
	Store  *store.Store
	// Task pre-labels the first session.
	Task string
	// TickScale multiplies elapsed time. 1 is real time; higher values
	// fast-forward, which is how a full cycle gets verified in seconds.
	TickScale float64
	// StartRunning begins counting down immediately.
	StartRunning bool
	// SkipToEnd starts one second from the end of the phase, so the
	// completion path — notification, sound, log append, confetti — can be
	// verified without waiting it out.
	SkipToEnd bool
	// Fresh ignores any saved position and starts a new focus phase.
	Fresh bool
}

// Model is the Bubble Tea model for the HUD.
type Model struct {
	cfg   config.Config
	store *store.Store
	notif notify.Notifier

	timer *timer.State
	stats store.Stats
	// sessions is the replayed log, kept in memory so stats can be
	// recomputed after each append without re-reading the file.
	sessions []store.Session

	tomato *sprite.Sprite
	geom   geom
	clk    clock

	steam        *anim.System
	confetti     *anim.System
	steamCredit  float64
	confettiLeft time.Duration

	// palFrom and palTo bracket the phase cross-fade; palT is its progress.
	palFrom, palTo theme.Palette
	palT           float64

	elapsed   float64 // seconds of wall time, drives the idle pulses
	lastTick  time.Time
	tickScale float64

	// phaseStart is when the current phase began, for the session log. It
	// survives a quit, so a session finished after resuming is logged against
	// the time it actually started.
	phaseStart time.Time
	// resumed records that this run picked up a saved position, so the HUD can
	// say so rather than leaving the user wondering why the clock is odd.
	resumed bool

	mode      mode
	taskInput string
	// showHelp toggles the key legend. It starts visible so the keys are
	// discoverable, and "/" collapses it to a single hint line.
	showHelp bool

	width, height int
	quitting      bool
}

// New builds the model. It replays the session log so level, XP and streak are
// live from the first frame.
func New(opts Options) (*Model, error) {
	if err := opts.Config.Validate(); err != nil {
		return nil, err
	}
	t, err := timer.New(opts.Config.Timer())
	if err != nil {
		return nil, err
	}
	t.Task = opts.Task
	t.Running = opts.StartRunning

	now := time.Now()

	// Pick up where the last session was left off. The phase start is restored
	// too, so a session finished after resuming is logged against the time it
	// actually began rather than this launch.
	phaseStart := now
	resumed := false
	if opts.Config.Resume && !opts.Fresh && !opts.SkipToEnd {
		if saved, ok := opts.Store.LoadResume(now); ok {
			if snap, ok := saved.Snapshot(); ok {
				// An explicit -task overrides the remembered label.
				if opts.Task != "" {
					snap.Task = opts.Task
				}
				if err := t.Restore(snap); err == nil {
					resumed = true
					if !saved.PhaseStart.IsZero() {
						phaseStart = saved.PhaseStart
					}
				}
			}
		}
	}

	if opts.SkipToEnd && t.Remaining > time.Second {
		t.Remaining = time.Second
	}

	tomato, err := loadTomato()
	if err != nil {
		return nil, err
	}

	sessions, _, err := opts.Store.Load()
	if err != nil {
		return nil, err
	}

	scale := opts.TickScale
	if scale <= 0 {
		scale = 1
	}

	pal := theme.For(t.Phase)
	clockW, clockH := clockCanvasSize(FormatRemaining(t.Remaining, opts.Config.ShowSeconds))

	m := &Model{
		cfg:   opts.Config,
		store: opts.Store,
		notif: notify.Notifier{
			Enabled: opts.Config.Notify,
			Sound:   opts.Config.Sound,
		},
		timer:      t,
		sessions:   sessions,
		stats:      store.Compute(sessions, now),
		tomato:     tomato,
		geom:       layout(tomato.Canvas.W, tomato.Canvas.H, clockW, clockH),
		steam:      anim.NewSystem(steamPoolSize, now.UnixNano()),
		confetti:   anim.NewSystem(confettiPoolSize, now.UnixNano()+1),
		palFrom:    pal,
		palTo:      pal,
		palT:       1,
		lastTick:   now,
		tickScale:  scale,
		phaseStart: phaseStart,
		resumed:    resumed,
		taskInput:  t.Task,
		showHelp:   true,
	}
	m.confetti.Gravity = 34
	m.confetti.Drag = 0.6
	m.clk.set(m.clockText())
	return m, nil
}

// Init starts the render loop.
func (m *Model) Init() tea.Cmd { return tickCmd() }

func tickCmd() tea.Cmd {
	return tea.Tick(frameInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Update handles input and the render tick.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		return m, m.handleKey(msg)

	case tickMsg:
		m.advance(time.Time(msg))
		if m.quitting {
			return m, tea.Quit
		}
		return m, tickCmd()
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) tea.Cmd {
	if m.mode == modeEditTask {
		m.editKey(msg)
		return nil
	}

	switch msg.String() {
	case "ctrl+c", "q":
		m.saveResume()
		m.quitting = true
		return tea.Quit
	case "t":
		if m.mode == modeStats {
			m.mode = modeNormal
		} else {
			m.mode = modeStats
		}
	case "/":
		m.showHelp = !m.showHelp
	case "esc":
		m.mode = modeNormal
	case " ":
		m.timer.Toggle()
	case "s":
		m.skip()
	case "r":
		m.timer.Reset()
		m.phaseStart = time.Now()
	case "e":
		m.mode = modeEditTask
		m.taskInput = m.timer.Task
	}
	return nil
}

func (m *Model) editKey(msg tea.KeyMsg) {
	switch msg.Type {
	case tea.KeyEnter:
		m.timer.Task = strings.TrimSpace(m.taskInput)
		m.mode = modeNormal
	case tea.KeyEsc:
		m.taskInput = m.timer.Task
		m.mode = modeNormal
	case tea.KeyBackspace:
		r := []rune(m.taskInput)
		if len(r) > 0 {
			m.taskInput = string(r[:len(r)-1])
		}
	case tea.KeyRunes, tea.KeySpace:
		// A space arrives as KeySpace with Runes already holding " ", so
		// appending the rune is enough. Adding one for the key type as well
		// inserted a double space on every word break.
		m.taskInput += string(msg.Runes)
	}
}

// skip ends the current phase early, logging what actually ran.
func (m *Model) skip() {
	elapsed := m.timer.Elapsed()
	ended := m.timer.Phase
	ev := m.timer.Skip()
	m.record(ended, elapsed, false)
	m.onPhaseChange(ev)
}

// advance moves the world forward to now.
func (m *Model) advance(now time.Time) {
	dt := now.Sub(m.lastTick)
	if dt < 0 {
		dt = 0
	}
	m.lastTick = now

	scaled := time.Duration(float64(dt) * m.tickScale)
	seconds := scaled.Seconds()
	m.elapsed += seconds

	for _, ev := range m.timer.Advance(scaled) {
		// A phase that ended on its own ran its full configured length.
		m.record(ev.Ended, m.cfg.Timer().Duration(ev.Ended), ev.Completed)
		m.onPhaseChange(ev)
	}

	m.clk.set(m.clockText())
	m.clk.update(seconds)

	if m.palT < 1 {
		m.palT = anim.Clamp01(m.palT + seconds/paletteFade.Seconds())
	}

	m.updateParticles(seconds)
}

func (m *Model) updateParticles(dt float64) {
	b := breathFor(m.timer.Phase, m.timer.Running)
	if b.steamHz > 0 {
		m.steamCredit += b.steamHz * dt
		for m.steamCredit >= 1 {
			m.steamCredit--
			emitSteam(m.steam, m.geom, m.tomato.Canvas.W, m.palette().Accent)
		}
	} else {
		m.steamCredit = 0
	}
	m.steam.Update(dt)

	if m.confettiLeft > 0 {
		m.confettiLeft -= time.Duration(dt * float64(time.Second))
		if m.confettiLeft <= 0 {
			m.confettiLeft = 0
			m.confetti.Clear()
		}
	}
	m.confetti.Update(dt)
}

// saveResume writes the current position so the next launch can pick it up.
// A phase that has only just started is not worth remembering, and neither is
// one about to end.
func (m *Model) saveResume() {
	if !m.cfg.Resume {
		return
	}
	snap := m.timer.Snapshot()
	if snap.Remaining < time.Second {
		_ = m.store.ClearResume()
		return
	}
	// Errors are dropped on purpose: failing to save your place must not stop
	// the program exiting.
	_ = m.store.SaveResume(store.NewResume(snap, m.phaseStart, time.Now()))
}

// record appends a finished phase to the log and refreshes the derived stats.
func (m *Model) record(phase timer.Phase, ran time.Duration, completed bool) {
	mins := int(math.Round(ran.Minutes()))
	if mins <= 0 {
		// Nothing meaningful happened; logging a zero-minute row would only
		// add noise to the history.
		m.phaseStart = time.Now()
		return
	}
	sess := store.Session{
		Start: m.phaseStart,
		Mins:  mins,
		Task:  m.timer.Task,
		Phase: phase.String(),
		Done:  completed,
	}
	m.phaseStart = time.Now()

	m.sessions = append(m.sessions, sess)
	m.stats = store.Compute(m.sessions, time.Now())
	// A failed write must not take the session down; the timer keeps running
	// and the user still sees their progress for this run.
	_ = m.store.Append(sess)
}

// onPhaseChange fires the notification and starts the visual transition.
func (m *Model) onPhaseChange(ev timer.Event) {
	m.palFrom = m.palette()
	m.palTo = theme.For(ev.Next)
	m.palT = 0

	// The saved position now describes a phase that is over. Rewrite it
	// immediately rather than waiting for a clean quit, so a crash cannot
	// resurrect a session that already finished.
	m.resumed = false
	m.saveResume()

	m.steam.Clear()
	m.steamCredit = 0

	if ev.Completed {
		burstConfetti(m.confetti, m.geom)
		m.confettiLeft = confettiHold
		m.notif.Send("pomo", phaseMessage(ev))
	}
}

func phaseMessage(ev timer.Event) string {
	switch ev.Ended {
	case timer.Focus:
		return "Focus done — take a " + ev.Next.String()
	default:
		return strings.ToUpper(ev.Ended.String()[:1]) + ev.Ended.String()[1:] + " over — back to focus"
	}
}

func (m *Model) palette() theme.Palette {
	return theme.Lerp(m.palFrom, m.palTo, anim.EaseOutQuad(m.palT))
}

func (m *Model) clockText() string {
	return FormatRemaining(m.timer.Remaining, m.cfg.ShowSeconds)
}

// View renders the HUD.
func (m *Model) View() string {
	if m.quitting {
		return ""
	}
	pal := m.palette()

	if m.mode == modeStats {
		return StatsReport(pal, m.stats, m.store.Path())
	}
	if m.width > 0 && (m.width < m.geom.BandW+2 || m.height < m.requiredHeight()) {
		return compactView(pal, m.timer, m.clockText(), m.stats)
	}
	return m.fullView(pal)
}

func (m *Model) fullView(pal theme.Palette) string {
	band := canvas.New(m.geom.BandW, m.geom.BandH)

	b := breathFor(m.timer.Phase, m.timer.Running)
	osc := 2*anim.Pulse(m.elapsed, b.period) - 1
	// A one-pixel bob is half a cell, which is exactly the motion half-blocks
	// buy. Forcing the offset even quantised it to two-pixel jumps, and on
	// negative values Go's &^ rounds away from zero, so the mascot lurched
	// down twice as far as it rose.
	bob := int(math.Round(b.bob * osc))

	blitSquash(
		band, m.tomato.Canvas,
		m.geom.SpriteX, m.geom.SpriteY+bob,
		1+b.squash*osc,
		spriteTransform(m.tomato, pal, m.timer.Progress()),
	)

	m.steam.Draw(band)

	style, alert := clockStyleFor(pal, m.timer.Remaining, m.timer.Running)
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
	content = append(content, statusBar(pal, m.stats, m.geom.BandW))
	content = append(content, rows...)
	content = append(content, progressBar(pal, m.timer, m.geom.BandW))
	content = append(content, taskLine(pal, m.displayTask(), m.mode == modeEditTask, m.resumed, m.geom.BandW))

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
		return helpBlock(pal, true)
	}
	if !m.showHelp {
		return helpHint(pal)
	}
	return helpBlock(pal, false)
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
