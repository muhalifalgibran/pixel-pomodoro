package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/anim"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/config"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/habit"
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
	modeHabits
	modeHabitForm
	modeConfirm
	modeEditTask
	modeCheck
)

type tickMsg time.Time

// Options configures a Model beyond the user's config file.
type Options struct {
	Config config.Config
	Store  *store.Store
	// Habits persists the habit definitions. A nil store means habits are
	// unavailable, and the HUD falls back to a free-text task.
	Habits *habit.Store
	// HabitName preselects a habit by name, as `-habit work` does.
	HabitName string
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
	// Zen starts in the open-ended stopwatch.
	Zen bool
}

// Model is the Bubble Tea model for the HUD.
type Model struct {
	cfg   config.Config
	store *store.Store
	notif notify.Notifier

	timer *timer.State
	stats store.Stats

	habitStore *habit.Store
	habits     habit.List
	// progress is recomputed whenever a session lands, so the HUD and the
	// habit list never show a stale figure.
	progress map[string]store.HabitProgress
	// activeID is the habit sessions are logged against. Empty means none, and
	// the timer behaves as it did before habits existed.
	activeID    string
	habitCursor int
	// habitForm is the add/edit form; editingID is empty when adding.
	habitForm *form
	editingID string
	confirm   confirmPrompt
	// sessions is the replayed log, kept in memory so stats can be
	// recomputed after each append without re-reading the file.
	sessions []store.Session
	// lastMark is the check-off that [u] would take back, and is non-nil only
	// while it is still the newest thing this process appended. Holding it in
	// memory is what scopes undo to this run: a mis-press is worth taking back,
	// a week-old entry is history.
	lastMark *checkMark
	// checkStatus is the check-off screen's feedback line — what was logged, or
	// why an undo was refused.
	checkStatus string
	// progressDay is the calendar day progress was last computed for, so a run
	// left open across midnight notices.
	progressDay string

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
	// Zen is the open-ended stopwatch. It deliberately does not go through
	// timer.State: that machine counts down toward a configured length, and
	// bending it into an unbounded count-up would compromise the one component
	// here that is provably simple. While zen runs the timer is simply not
	// advanced, so leaving zen returns to it exactly where it was.
	zen        bool
	zenRunning bool
	zenElapsed time.Duration
	zenStart   time.Time

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

	var habits habit.List
	if opts.Habits != nil {
		habits, err = opts.Habits.Load()
		if err != nil {
			return nil, err
		}
	}

	// An explicit -habit wins over whatever was active last time.
	activeID := ""
	if opts.HabitName != "" {
		h, ok := habits.ByName(opts.HabitName)
		if !ok {
			return nil, unknownHabit(opts.HabitName, habits)
		}
		activeID = h.ID
		t.Task = h.Name
	}

	// Pick up where the last session was left off. The phase start is restored
	// too, so a session finished after resuming is logged against the time it
	// actually began rather than this launch.
	phaseStart := now
	resumed := false
	var zenResume store.Resume
	if opts.Config.Resume && !opts.Fresh && !opts.SkipToEnd {
		if saved, ok := opts.Store.LoadResume(now); ok {
			if snap, ok := saved.Snapshot(); ok {
				// An explicit -task or -habit overrides what was remembered.
				if opts.Task != "" {
					snap.Task = opts.Task
				}
				if err := t.Restore(snap); err == nil {
					resumed = true
					if !saved.PhaseStart.IsZero() {
						phaseStart = saved.PhaseStart
					}
					if saved.Zen {
						zenResume = saved
					}
					if activeID == "" && saved.Habit != "" {
						// Only honour a habit that still exists; a stale ID
						// would log sessions against nothing.
						if h, ok := habits.ByID(saved.Habit); ok && !h.Archived {
							activeID = h.ID
							snap.Task = h.Name
							t.Task = h.Name
						}
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
		habitStore: opts.Habits,
		habits:     habits,
		activeID:   activeID,
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
	switch {
	case opts.Zen:
		m.startZen(now)
	case zenResume.Zen && !opts.Fresh:
		// Zen was running when the user quit; pick it back up where it was.
		m.startZen(now)
		m.zenElapsed = time.Duration(zenResume.ZenElapsedS) * time.Second
		if !zenResume.ZenStart.IsZero() {
			m.zenStart = zenResume.ZenStart
		}
	}
	m.clk.set(m.clockText())
	m.refreshProgress(now)
	m.applyHabitTiming()
	return m, nil
}

// startZen enters the stopwatch, pausing the pomodoro where it stands.
func (m *Model) startZen(now time.Time) {
	m.zen = true
	m.zenRunning = true
	m.zenElapsed = 0
	m.zenStart = now
	m.timer.Running = false
	m.fadeTo(m.paletteTarget())
	m.steam.Clear()
	m.steamCredit = 0
}

// stopZen logs the stretch and hands the HUD back to the pomodoro.
//
// The session carries no habit ID: zen belongs to no goal, by design. It still
// earns XP and keeps the global streak alive, because the time was real.
func (m *Model) stopZen() {
	if mins := int(math.Round(m.zenElapsed.Minutes())); mins > 0 {
		m.appendSession(store.Session{
			Start: m.zenStart,
			Mins:  mins,
			Task:  "zen",
			Phase: store.PhaseZen,
			Done:  true,
		})
	}
	m.zen = false
	m.zenRunning = false
	m.zenElapsed = 0
	m.fadeTo(m.paletteTarget())
	m.phaseStart = time.Now()
}

// fadeTo starts a cross-fade toward a palette.
func (m *Model) fadeTo(pal theme.Palette) {
	m.palFrom = m.palette()
	m.palTo = pal
	m.palT = 0
}

// unknownHabit reports a name that matched nothing, listing what would have
// worked rather than leaving the user to guess.
func unknownHabit(name string, habits habit.List) error {
	known := habits.Names()
	if len(known) == 0 {
		return fmt.Errorf("no habit named %q, and none are defined yet", name)
	}
	return fmt.Errorf("no habit named %q; try one of: %s", name, strings.Join(known, ", "))
}

// refreshProgress recomputes the per-habit figures from the in-memory log.
func (m *Model) refreshProgress(now time.Time) {
	m.progress = store.Progress(m.sessions, m.habits.Active(), now)
	m.progressDay = sameDayKey(now)
}

// sameDayKey is the calendar day now falls in, for noticing that the figures on
// screen belong to yesterday.
func sameDayKey(now time.Time) string { return now.Format("2006-01-02") }

// activeHabit returns the habit sessions are being logged against.
func (m *Model) activeHabit() (habit.Habit, bool) {
	if m.activeID == "" {
		return habit.Habit{}, false
	}
	return m.habits.ByID(m.activeID)
}

// timerConfig is the global timing policy with the active habit's overrides
// applied. A habit leaves any of them at zero to inherit the global value.
func (m *Model) timerConfig() timer.Config {
	cfg := m.cfg.Timer()
	h, ok := m.activeHabit()
	if !ok {
		return cfg
	}
	if h.Focus > 0 {
		cfg.Focus = h.Focus
	}
	if h.Short > 0 {
		cfg.ShortBreak = h.Short
	}
	if h.Long > 0 {
		cfg.LongBreak = h.Long
	}
	return cfg
}

// applyHabitTiming rebuilds the timer against the active habit's lengths,
// keeping the position it already had where that still fits.
func (m *Model) applyHabitTiming() {
	cfg := m.timerConfig()
	if cfg == m.timer.Config() {
		return
	}
	snap := m.timer.Snapshot()
	t, err := timer.New(cfg)
	if err != nil {
		// A habit with a nonsense length should not take the program down; keep
		// the timer that already works.
		return
	}
	// Restore clamps a phase that the new lengths made shorter.
	if err := t.Restore(snap); err != nil {
		return
	}
	m.timer = t
}

// selectHabit makes a habit active. Anything meaningful already run in the
// current phase is logged as abandoned first, so switching mid-session neither
// loses the time nor credits it to the wrong habit.
func (m *Model) selectHabit(id string) {
	h, ok := m.habits.ByID(id)
	if !ok {
		return
	}
	if id != m.activeID {
		if elapsed := m.timer.Elapsed(); elapsed >= time.Minute {
			m.record(m.timer.Phase, elapsed, false)
		}
		m.activeID = id
		m.timer.Task = h.Name
		m.taskInput = h.Name
		m.timer.Reset()
		m.applyHabitTiming()
		m.phaseStart = time.Now()
	}
	m.timer.Running = true
	m.mode = modeNormal
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

	switch m.mode {
	case modeHabits:
		m.habitsKey(msg)
		return nil
	case modeCheck:
		m.checkKey(msg)
		return nil
	case modeHabitForm:
		m.formKey(msg)
		return nil
	case modeConfirm:
		m.confirmKey(msg)
		return nil
	}

	switch msg.String() {
	case "ctrl+c", "q":
		// A zen stretch in progress is logged rather than discarded; the time
		// was spent either way.
		if m.zen {
			m.stopZen()
		}
		m.saveResume()
		m.quitting = true
		return tea.Quit
	case "h":
		m.mode = modeHabits
		m.habitCursor = m.cursorForActive()
	case "l":
		m.mode = modeCheck
		m.habitCursor = m.cursorForActive()
		m.checkStatus = ""
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
	case "z":
		if m.zen {
			m.stopZen()
		} else {
			m.startZen(time.Now())
		}
	case " ":
		if m.zen {
			m.zenRunning = !m.zenRunning
		} else {
			m.timer.Toggle()
		}
	case "s":
		if m.zen {
			break
		}
		m.skip()
	case "r":
		if m.zen {
			break
		}
		m.timer.Reset()
		m.phaseStart = time.Now()
	case "e":
		// With a habit active the label is the habit's name, so editing it here
		// would silently desync the two. Names are changed in the habit form.
		if _, active := m.activeHabit(); !active {
			m.mode = modeEditTask
			m.taskInput = m.timer.Task
		}
	}
	return nil
}

// cursorForActive puts the habit list cursor on whatever is currently active,
// so opening the screen does not lose your place.
func (m *Model) cursorForActive() int { return m.cursorFor(m.activeID) }

func (m *Model) habitsKey(msg tea.KeyMsg) {
	active := m.habits.Active()

	switch msg.String() {
	case "ctrl+c", "q":
		m.saveResume()
		m.quitting = true
	case "esc", "h":
		m.mode = modeNormal
	case "j", "down":
		if len(active) > 0 {
			m.habitCursor = (m.habitCursor + 1) % len(active)
		}
	case "k", "up":
		if len(active) > 0 {
			m.habitCursor = (m.habitCursor - 1 + len(active)) % len(active)
		}
	case "enter":
		if m.habitCursor < len(active) {
			m.selectHabit(active[m.habitCursor].ID)
		}
	case "a":
		m.habitForm = newHabitForm(nil)
		m.editingID = ""
		m.mode = modeHabitForm
	case "E":
		if m.habitCursor < len(active) {
			h := active[m.habitCursor]
			m.habitForm = newHabitForm(&h)
			m.editingID = h.ID
			m.mode = modeHabitForm
		}
	case "d":
		if m.habitCursor < len(active) {
			m.askToRemove(active[m.habitCursor])
		}
	}
}

// checkMark is a check-off that can still be taken back: the exact session that
// was appended, and the byte the log stood at beforehand.
type checkMark struct {
	sess store.Session
	at   int64
}

// checkKey drives the [l] screen. It shares habitCursor with the habit list —
// both index the same slice, so moving between the two screens keeps your place.
func (m *Model) checkKey(msg tea.KeyMsg) {
	active := m.habits.Active()

	switch msg.String() {
	case "ctrl+c", "q":
		m.saveResume()
		m.quitting = true
	case "esc", "l":
		m.mode = modeNormal
	case "j", "down":
		if len(active) > 0 {
			m.habitCursor = (m.habitCursor + 1) % len(active)
			m.checkStatus = ""
		}
	case "k", "up":
		if len(active) > 0 {
			m.habitCursor = (m.habitCursor - 1 + len(active)) % len(active)
			m.checkStatus = ""
		}
	case " ":
		if m.habitCursor < len(active) {
			m.markDone(active[m.habitCursor])
		}
	case "u":
		m.undoMark()
	case "enter":
		if m.habitCursor < len(active) {
			m.selectHabit(active[m.habitCursor].ID)
		}
	}
}

// markDone credits a habit with one session's worth of work without the timer
// having run, for something already done. It goes through the same builder
// `pomo -log` uses, so the two record the same thing.
//
// The running timer is deliberately untouched, even when the marked habit is
// the active one: a mark asserts something about the past, and resetting the
// clock over it would throw away real elapsed time.
func (m *Model) markDone(h habit.Habit) {
	dur := store.SessionLength(h, m.cfg.Focus)
	sess, err := store.ManualSession(h, dur, time.Now())
	if err != nil {
		m.checkStatus = err.Error()
		return
	}

	// Taken before the append, so undo knows where the log stood.
	at, err := m.store.Size()
	if err != nil {
		at = -1
	}

	met := m.progress[h.ID].Met
	m.appendSession(sess)
	if at >= 0 {
		m.lastMark = &checkMark{sess: sess, at: at}
	}

	m.checkStatus = fmt.Sprintf("logged %s to %s — u to undo", habit.FormatMinutes(sess.Mins), h.Name)
	if !met && m.progress[h.ID].Met {
		m.checkStatus = h.Name + " — goal met"
	}
}

// undoMark takes back the last check-off, and only that one. It refuses rather
// than guesses if anything else has landed in the log since, so a session
// another pomo wrote can never be the thing that disappears.
func (m *Model) undoMark() {
	if m.lastMark == nil {
		m.checkStatus = "nothing to undo"
		return
	}
	if err := m.store.RemoveLast(m.lastMark.at, m.lastMark.sess); err != nil {
		m.lastMark = nil
		m.checkStatus = "the log has moved on — that one stays"
		return
	}

	name := m.lastMark.sess.Task
	m.lastMark = nil
	if n := len(m.sessions); n > 0 {
		m.sessions = m.sessions[:n-1]
	}
	now := time.Now()
	m.stats = store.Compute(m.sessions, now)
	m.refreshProgress(now)
	m.checkStatus = "took back the last " + name
}

func (m *Model) formKey(msg tea.KeyMsg) {
	switch m.habitForm.key(msg) {
	case formCancel:
		m.habitForm = nil
		m.mode = modeHabits
	case formSave:
		m.submitHabitForm()
	}
}

// submitHabitForm validates and persists, staying in the form on failure so
// nothing the user typed is thrown away.
func (m *Model) submitHabitForm() {
	h, err := habitFromForm(m.habitForm)
	if err != nil {
		m.habitForm.err = err.Error()
		return
	}

	if m.editingID == "" {
		added, err := m.habits.Add(h, time.Now())
		if err != nil {
			m.habitForm.err = err.Error()
			return
		}
		m.habitCursor = m.cursorFor(added.ID)
	} else {
		h.ID = m.editingID
		if err := m.habits.Update(h); err != nil {
			m.habitForm.err = err.Error()
			return
		}
		// The name may have changed, and the active label tracks it.
		if m.activeID == h.ID {
			m.timer.Task = h.Name
			m.applyHabitTiming()
		}
	}

	if err := m.saveHabits(); err != nil {
		m.habitForm.err = err.Error()
		return
	}
	m.habitForm = nil
	m.mode = modeHabits
}

// askToRemove sets up the delete confirmation. A habit with logged sessions is
// archived instead, so its history keeps a habit to point at.
func (m *Model) askToRemove(h habit.Habit) {
	logged := m.sessionsFor(h.ID)
	prompt := confirmPrompt{}

	if logged > 0 {
		prompt.message = "Archive " + h.Name + "?"
		prompt.detail = fmt.Sprintf(
			"%d logged session(s) stay in your history. It leaves the list but its past is kept.", logged)
		prompt.run = func() {
			_ = m.habits.Archive(h.ID)
			m.afterRemoval(h.ID)
		}
	} else {
		prompt.message = "Delete " + h.Name + "?"
		prompt.detail = "Nothing has been logged against it, so there is no history to keep."
		prompt.run = func() {
			_ = m.habits.Remove(h.ID)
			m.afterRemoval(h.ID)
		}
	}

	m.confirm = prompt
	m.mode = modeConfirm
}

// afterRemoval persists and repairs the cursor and active selection.
func (m *Model) afterRemoval(id string) {
	if m.activeID == id {
		m.activeID = ""
	}
	_ = m.saveHabits()
	if n := len(m.habits.Active()); m.habitCursor >= n {
		m.habitCursor = clampInt(n-1, 0, n)
	}
}

func (m *Model) confirmKey(msg tea.KeyMsg) {
	switch msg.String() {
	case "y", "Y":
		if m.confirm.run != nil {
			m.confirm.run()
		}
		m.confirm = confirmPrompt{}
		m.mode = modeHabits
	case "n", "N", "esc", "q":
		m.confirm = confirmPrompt{}
		m.mode = modeHabits
	}
}

// cursorFor puts the habit list cursor on a given habit.
func (m *Model) cursorFor(id string) int {
	for i, h := range m.habits.Active() {
		if h.ID == id {
			return i
		}
	}
	return 0
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

	// Progress is otherwise only recomputed when a session lands, so a pomo
	// left open overnight would keep showing yesterday's figures — and the
	// check-off screen is exactly where that reads as a bug, since a row would
	// sit there ticked until the first press of the new day snapped it back.
	if m.progressDay != "" && sameDayKey(now) != m.progressDay {
		m.stats = store.Compute(m.sessions, now)
		m.refreshProgress(now)
	}

	scaled := time.Duration(float64(dt) * m.tickScale)
	seconds := scaled.Seconds()
	m.elapsed += seconds

	if m.zen {
		if m.zenRunning {
			m.zenElapsed += scaled
		}
	} else {
		for _, ev := range m.timer.Advance(scaled) {
			// A phase that ended on its own ran its full length — and that
			// length comes from the timer's own config, not the global one. A
			// habit's focus override lives only in the timer, so reading the
			// global config here logged 25 minutes for a 50 minute session.
			m.record(ev.Ended, m.timer.Config().Duration(ev.Ended), ev.Completed)
			m.onPhaseChange(ev)
		}
	}

	m.clk.set(m.clockText())
	m.clk.update(seconds)

	if m.palT < 1 {
		m.palT = anim.Clamp01(m.palT + seconds/paletteFade.Seconds())
	}

	m.updateParticles(seconds)
}

func (m *Model) updateParticles(dt float64) {
	b := m.breath()
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
	r := store.NewResume(snap, m.activeID, m.phaseStart, time.Now())
	r.Zen = m.zen
	r.ZenElapsedS = int(m.zenElapsed / time.Second)
	r.ZenStart = m.zenStart
	_ = m.store.SaveResume(r)
}

// appendSession records one session in memory, on disk and in the derived
// figures. Both the pomodoro and zen paths go through it so none of the three
// can be updated without the others.
func (m *Model) appendSession(sess store.Session) {
	now := time.Now()
	// Undo only ever reaches the newest append, so a phase or a zen stretch
	// landing on top disarms it. markDone re-arms it straight after.
	m.lastMark = nil
	m.sessions = append(m.sessions, sess)
	m.stats = store.Compute(m.sessions, now)
	m.refreshProgress(now)
	// A failed write must not take the session down; the timer keeps running
	// and the user still sees their progress for this run.
	_ = m.store.Append(sess)
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
		Habit: m.activeID,
		Task:  m.timer.Task,
		Phase: phase.String(),
		Done:  completed,
	}
	m.phaseStart = time.Now()
	m.appendSession(sess)
}

// onPhaseChange fires the notification and starts the visual transition.
func (m *Model) onPhaseChange(ev timer.Event) {
	m.fadeTo(m.paletteTarget())

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

// onPhaseChange fades toward the new phase, but zen owns the palette while it
// runs, so a phase boundary crossed beforehand must not steal it back.
func (m *Model) paletteTarget() theme.Palette {
	if m.zen {
		return theme.Zen
	}
	return theme.For(m.timer.Phase)
}

// withLegend puts a screen's key hints under its content. Full-screen views are
// measured against the terminal, since they have all of it to use.
func withLegend(pal theme.Palette, body string, keys []string, width int) string {
	return strings.TrimRight(body, "\n") + "\n\n" +
		strings.Join(helpBlock(pal, keys, width), "\n")
}

// statsWidth is the width the stats screen may use. Before the first
// WindowSizeMsg the terminal size is unknown, so fall back to the frame width
// the HUD already assumes.
func (m *Model) statsWidth() int {
	if m.width > 0 {
		return m.width
	}
	return m.geom.BandW + 2
}

func (m *Model) clockText() string {
	if m.zen {
		return FormatElapsed(m.zenElapsed)
	}
	return FormatRemaining(m.timer.Remaining, m.cfg.ShowSeconds)
}

// View renders the HUD.
func (m *Model) View() string {
	if m.quitting {
		return ""
	}
	pal := m.palette()

	// Every screen's hints use the same three-row grid, so the block never
	// changes shape as you move between them.
	switch m.mode {
	case modeStats:
		return withLegend(pal,
			StatsReport(pal, m.stats, m.habits.Active(), m.progress, m.store.Path(), m.statsWidth()),
			statsKeys, m.statsWidth())
	case modeHabits:
		return withLegend(pal,
			habitsView(pal, m.habits.Active(), m.progress, m.habitCursor, m.activeID),
			habitsKeysFor(len(m.habits.Active()) > 0), m.statsWidth())
	case modeCheck:
		return withLegend(pal,
			checkView(pal, m.habits.Active(), m.progress, m.habitCursor, m.activeID, m.checkStatus),
			checkKeysFor(len(m.habits.Active()) > 0), m.statsWidth())
	case modeHabitForm:
		return withLegend(pal, m.habitForm.view(pal), formKeys, m.statsWidth())
	case modeConfirm:
		return withLegend(pal, m.confirm.view(pal), confirmKeys, m.statsWidth())
	}
	if m.width > 0 && (m.width < m.geom.BandW+2 || m.height < m.requiredHeight()) {
		return compactView(pal, m.timer, m.clockText(), m.stats)
	}
	return m.hudView(pal)
}
