// Command pomo is a pixel-art pomodoro timer for the terminal.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/config"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/habit"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/selfupdate"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/store"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/theme"
	"github.com/muhalifalgibran/pixel-pomodoro/internal/ui"
)

// version is stamped in at release time with -ldflags. A source build leaves
// it as "dev", which is the honest answer for a binary built from a working
// tree that may not match any tag.
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "pomo:", err)
		os.Exit(1)
	}
}

// resolveVersion prefers the linker-stamped tag, then falls back to whatever
// the module system recorded. `go install ...@latest` produces a binary with
// no ldflags but a populated build info, so without this it would report
// "dev" despite being a tagged release.
func resolveVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

type flags struct {
	configPath string

	focus      time.Duration
	shortBreak time.Duration
	longBreak  time.Duration
	every      int
	themeName  string
	task       string
	habitName  string

	noSound     bool
	noNotify    bool
	noScanlines bool
	noSeconds   bool
	paused      bool
	fresh       bool
	zen         bool

	// Development and verification affordances. They ship deliberately: they
	// are what makes the timer debuggable without waiting 25 minutes.
	demo       bool
	mono       bool
	demoText   string
	svgPath    string
	svgPixel   int
	statsOnly  bool
	habitsOnly bool
	logSession bool
	logDate    string
	tickScale  float64
	skipToEnd  bool
}

func run() error {
	defaultConfigPath, err := config.DefaultPath()
	if err != nil {
		return err
	}

	var f flags
	flag.StringVar(&f.configPath, "config", defaultConfigPath, "path to config.toml")
	flag.DurationVar(&f.focus, "focus", 25*time.Minute, "focus phase length")
	flag.DurationVar(&f.focus, "f", 25*time.Minute, "focus phase length (shorthand)")
	flag.DurationVar(&f.shortBreak, "short-break", 5*time.Minute, "short break length")
	flag.DurationVar(&f.shortBreak, "b", 5*time.Minute, "short break length (shorthand)")
	flag.DurationVar(&f.longBreak, "long-break", 15*time.Minute, "long break length")
	flag.IntVar(&f.every, "long-break-every", 4, "focus sessions per long break")
	flag.StringVar(&f.themeName, "theme", "ember", "palette: ember, mint or indigo")
	flag.StringVar(&f.task, "task", "", "free-text label for this session")
	flag.StringVar(&f.habitName, "habit", "", "start on a named habit")

	flag.BoolVar(&f.noSound, "no-sound", false, "disable the completion sound")
	flag.BoolVar(&f.noNotify, "no-notify", false, "disable desktop notifications")
	flag.BoolVar(&f.noScanlines, "no-scanlines", false, "disable the CRT scanline dim")
	flag.BoolVar(&f.noSeconds, "no-seconds", false, "show minutes only until the final minute")
	flag.BoolVar(&f.paused, "paused", false, "start paused instead of counting down immediately")
	flag.BoolVar(&f.fresh, "fresh", false, "ignore the saved position and start a new focus phase")
	flag.BoolVar(&f.zen, "zen", false, "start the open-ended stopwatch, attached to no habit or goal")

	flag.BoolVar(&f.demo, "demo", false, "render the art once and exit, for iterating on sprites")
	flag.BoolVar(&f.mono, "mono", false, "with -demo, print opacity silhouettes instead of color")
	flag.StringVar(&f.demoText, "text", "23:41", "with -demo, the clock string to render")
	flag.StringVar(&f.svgPath, "svg", "", "with -demo, write the art to this SVG file instead of the terminal")
	flag.IntVar(&f.svgPixel, "svg-pixel", 6, "with -demo -svg, the size in SVG units of one art pixel")
	flag.BoolVar(&f.statsOnly, "stats", false, "print stats and exit")
	flag.BoolVar(&f.habitsOnly, "habits", false, "print the habit list and progress, then exit")
	flag.BoolVar(&f.logSession, "log", false, "log work against a habit without the timer: -log <habit> [duration]")
	flag.StringVar(&f.logDate, "log-date", "", "with -log, backdate the entry (YYYY-MM-DD)")
	flag.Float64Var(&f.tickScale, "tick-scale", 1, "multiply elapsed time; 60 fast-forwards a cycle into seconds")
	flag.BoolVar(&f.skipToEnd, "skip-to-end", false, "start one second from the end of a 1m phase, to verify completion")

	showVersion := flag.Bool("version", false, "print the version and exit")
	update := flag.Bool("update", false, "download and install the latest release")
	assumeYes := flag.Bool("y", false, "with -update, do not ask for confirmation")
	flag.Parse()

	if *showVersion {
		fmt.Printf("pomo %s\n", resolveVersion())
		return nil
	}

	if *update {
		return runUpdate(*assumeYes)
	}

	cfg, err := config.Load(f.configPath)
	if err != nil {
		return err
	}
	applyFlags(&cfg, f)
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("%w (config: %s)", err, f.configPath)
	}

	if f.demo {
		return runDemo(cfg, f)
	}

	logPath, err := store.DefaultPath()
	if err != nil {
		return err
	}
	st := store.New(logPath)

	habitPath, err := habit.DefaultPath()
	if err != nil {
		return err
	}
	habits := habit.NewStore(habitPath)

	if f.statsOnly {
		return printStats(cfg, st, habits)
	}
	if f.habitsOnly {
		return printHabits(cfg, st, habits)
	}
	if f.logSession {
		return logSession(os.Stdout, cfg, st, habits, flag.Args(), f.logDate, time.Now())
	}

	// Launching a timer means you want it running; -paused is for when you
	// want to set the task up first.
	model, err := ui.New(ui.Options{
		Config:       cfg,
		Store:        st,
		Habits:       habits,
		HabitName:    f.habitName,
		Task:         f.task,
		TickScale:    f.tickScale,
		StartRunning: !f.paused,
		SkipToEnd:    f.skipToEnd,
		Fresh:        f.fresh,
		Zen:          f.zen,
	})
	if err != nil {
		return err
	}

	_, err = tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}

// applyFlags overrides the config with flags the user actually passed.
// Checking flag.Visit rather than comparing against defaults is what keeps
// "flag set to the same value as the default" from being indistinguishable
// from "flag absent", which would silently override the config file.
func applyFlags(cfg *config.Config, f flags) {
	flag.Visit(func(fl *flag.Flag) {
		switch fl.Name {
		case "focus", "f":
			cfg.Focus = f.focus
		case "short-break", "b":
			cfg.ShortBreak = f.shortBreak
		case "long-break":
			cfg.LongBreak = f.longBreak
		case "long-break-every":
			cfg.LongBreakEvery = f.every
		case "theme":
			cfg.Theme = f.themeName
		case "no-sound":
			cfg.Sound = ""
		case "no-notify":
			cfg.Notify = false
		case "no-scanlines":
			cfg.Scanlines = false
		case "no-seconds":
			cfg.ShowSeconds = false
		}
	})

	// A one-minute cycle keeps the verification run short and, unlike
	// forcing a full-length phase to end, logs an honest duration.
	if f.skipToEnd {
		cfg.Focus = time.Minute
		cfg.ShortBreak = time.Minute
		cfg.LongBreak = time.Minute
	}
}

func printStats(cfg config.Config, st *store.Store, hs *habit.Store) error {
	sessions, skipped, err := st.Load()
	if err != nil {
		return err
	}
	list, err := hs.Load()
	if err != nil {
		return err
	}
	pal, _ := theme.ByName(cfg.Theme)
	now := time.Now()
	active := list.Active()
	fmt.Println(ui.StatsReport(pal, store.Compute(sessions, now), active,
		store.Progress(sessions, active, now), st.Path(), terminalWidth()))
	if skipped > 0 {
		fmt.Fprintf(os.Stderr, "\npomo: skipped %d unreadable line(s) in %s\n", skipped, st.Path())
	}
	return nil
}

// logSession appends completed work to the log without running the timer, for
// time spent away from the terminal. Without it the contribution bars would
// misrepresent the day.
//
// The duration is optional: omitting it logs one session at the habit's own
// focus length, which is what a session-count goal needs.
func logSession(out io.Writer, cfg config.Config, st *store.Store, hs *habit.Store, args []string, date string, now time.Time) error {
	if len(args) == 0 {
		return fmt.Errorf(
			"usage: pomo -log <habit> [duration]\n" +
				"  pomo -log work 90m                     90 minutes\n" +
				"  pomo -log reading                      one session at that habit's focus length\n" +
				"  pomo -log -log-date 2026-08-16 work 4h backdated; flags come before the habit")
	}

	list, err := hs.Load()
	if err != nil {
		return err
	}
	name := args[0]
	h, ok := list.ByName(name)
	if !ok {
		known := list.Names()
		if len(known) == 0 {
			return fmt.Errorf("no habit named %q, and none are defined yet — add one with pomo, then h, then a", name)
		}
		return fmt.Errorf("no habit named %q; try one of: %s", name, strings.Join(known, ", "))
	}

	// A habit's own focus length is the natural size of "one session".
	dur := cfg.Focus
	if h.Focus > 0 {
		dur = h.Focus
	}
	if len(args) > 1 {
		dur, err = time.ParseDuration(args[1])
		if err != nil {
			return fmt.Errorf("%q is not a duration; try 90m or 1h30m", args[1])
		}
		if dur <= 0 {
			return fmt.Errorf("duration must be more than zero")
		}
	}
	if len(args) > 2 {
		return fmt.Errorf("unexpected argument %q; -log takes a habit and an optional duration", args[2])
	}

	when := now
	if date != "" {
		// Parsed in the local zone, and anchored at noon so the entry cannot
		// slide into the neighbouring day across a DST change.
		day, err := time.ParseInLocation("2006-01-02", date, when.Location())
		if err != nil {
			return fmt.Errorf("%q is not a date; try 2026-08-17", date)
		}
		when = day.Add(12 * time.Hour)
	}

	mins := int(dur.Round(time.Minute) / time.Minute)
	if mins <= 0 {
		return fmt.Errorf("that rounds to no minutes at all")
	}
	sess := store.Session{
		Start: when,
		Mins:  mins,
		Habit: h.ID,
		Task:  h.Name,
		Phase: store.PhaseFocus,
		Done:  true,
	}
	if err := st.Append(sess); err != nil {
		return err
	}

	sessions, _, err := st.Load()
	if err != nil {
		return err
	}
	pal, _ := theme.ByName(cfg.Theme)
	active := list.Active()
	progress := store.Progress(sessions, active, now)

	fmt.Fprintf(out, "logged %s to %s\n\n", habit.FormatMinutes(mins), h.Name)
	fmt.Fprint(out, ui.HabitRowReport(pal, h, progress[h.ID]))
	return nil
}

// terminalWidth is the width to lay non-interactive output out for. The stats
// bars shrink rather than overhang, so a piped or unknown-width terminal gets
// the standard frame width instead of a guess.
func terminalWidth() int {
	if w, _, err := term.GetSize(os.Stdout.Fd()); err == nil && w > 0 {
		return w
	}
	return 0
}

func printHabits(cfg config.Config, st *store.Store, hs *habit.Store) error {
	list, err := hs.Load()
	if err != nil {
		return err
	}
	sessions, _, err := st.Load()
	if err != nil {
		return err
	}
	pal, _ := theme.ByName(cfg.Theme)
	active := list.Active()
	fmt.Print(ui.HabitsReport(pal, active, store.Progress(sessions, active, time.Now())))
	if len(active) > 0 {
		fmt.Printf("\n  habits  %s\n", hs.Path())
	}
	return nil
}

func runDemo(cfg config.Config, f flags) error {
	if f.svgPath != "" {
		out, err := ui.DemoSVG(cfg, f.demoText, f.svgPixel)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(f.svgPath), 0o755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
		if err := os.WriteFile(f.svgPath, []byte(out), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", f.svgPath, err)
		}
		fmt.Fprintf(os.Stderr, "wrote %s\n", f.svgPath)
		return nil
	}

	out, err := ui.DemoArt(cfg, f.demoText, f.mono)
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}

// runUpdate replaces this binary with the newest published release.
//
// The archive is verified against the release's published SHA-256 before
// anything is written, and the user is asked first unless -y is passed:
// replacing an executable on someone's PATH is not something to do quietly.
func runUpdate(assumeYes bool) error {
	current := resolveVersion()

	confirm := func(from, to string) bool {
		fmt.Printf("update pomo %s -> %s? [y/N] ", from, to)
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			return false
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		return answer == "y" || answer == "yes"
	}
	if assumeYes {
		confirm = nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err := selfupdate.Run(ctx, selfupdate.Options{
		Current: current,
		Out:     os.Stdout,
		Confirm: confirm,
	})
	if errors.Is(err, selfupdate.ErrUpToDate) {
		return nil
	}
	return err
}
