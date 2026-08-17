// Command pomo is a pixel-art pomodoro timer for the terminal.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/muhalifalgibran/pixel-pomodoro/internal/config"
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

	noSound     bool
	noNotify    bool
	noScanlines bool
	noSeconds   bool
	paused      bool
	fresh       bool

	// Development and verification affordances. They ship deliberately: they
	// are what makes the timer debuggable without waiting 25 minutes.
	demo      bool
	mono      bool
	demoText  string
	svgPath   string
	svgPixel  int
	statsOnly bool
	tickScale float64
	skipToEnd bool
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
	flag.StringVar(&f.task, "task", "", "label for this session")

	flag.BoolVar(&f.noSound, "no-sound", false, "disable the completion sound")
	flag.BoolVar(&f.noNotify, "no-notify", false, "disable desktop notifications")
	flag.BoolVar(&f.noScanlines, "no-scanlines", false, "disable the CRT scanline dim")
	flag.BoolVar(&f.noSeconds, "no-seconds", false, "show minutes only until the final minute")
	flag.BoolVar(&f.paused, "paused", false, "start paused instead of counting down immediately")
	flag.BoolVar(&f.fresh, "fresh", false, "ignore the saved position and start a new focus phase")

	flag.BoolVar(&f.demo, "demo", false, "render the art once and exit, for iterating on sprites")
	flag.BoolVar(&f.mono, "mono", false, "with -demo, print opacity silhouettes instead of color")
	flag.StringVar(&f.demoText, "text", "23:41", "with -demo, the clock string to render")
	flag.StringVar(&f.svgPath, "svg", "", "with -demo, write the art to this SVG file instead of the terminal")
	flag.IntVar(&f.svgPixel, "svg-pixel", 6, "with -demo -svg, the size in SVG units of one art pixel")
	flag.BoolVar(&f.statsOnly, "stats", false, "print stats and exit")
	flag.Float64Var(&f.tickScale, "tick-scale", 1, "multiply elapsed time; 60 fast-forwards a cycle into seconds")
	flag.BoolVar(&f.skipToEnd, "skip-to-end", false, "start one second from the end of a 1m phase, to verify completion")

	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("pomo %s\n", resolveVersion())
		return nil
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

	if f.statsOnly {
		return printStats(cfg, st)
	}

	// Launching a timer means you want it running; -paused is for when you
	// want to set the task up first.
	model, err := ui.New(ui.Options{
		Config:       cfg,
		Store:        st,
		Task:         f.task,
		TickScale:    f.tickScale,
		StartRunning: !f.paused,
		SkipToEnd:    f.skipToEnd,
		Fresh:        f.fresh,
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

func printStats(cfg config.Config, st *store.Store) error {
	sessions, skipped, err := st.Load()
	if err != nil {
		return err
	}
	pal, _ := theme.ByName(cfg.Theme)
	fmt.Println(ui.StatsReport(pal, store.Compute(sessions, time.Now()), st.Path()))
	if skipped > 0 {
		fmt.Fprintf(os.Stderr, "\npomo: skipped %d unreadable line(s) in %s\n", skipped, st.Path())
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
