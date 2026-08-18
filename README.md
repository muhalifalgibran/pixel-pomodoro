# pixel-pomodoro

[![ci](https://github.com/muhalifalgibran/pixel-pomodoro/actions/workflows/ci.yml/badge.svg)](https://github.com/muhalifalgibran/pixel-pomodoro/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/muhalifalgibran/pixel-pomodoro)](https://github.com/muhalifalgibran/pixel-pomodoro/releases/latest)
[![license](https://img.shields.io/github/license/muhalifalgibran/pixel-pomodoro)](LICENSE)

A habit tracker for the terminal, driven by a pixel-art pomodoro timer. Keep a
list of habits with daily or weekly targets, pick one whenever you sit down, and
watch a contribution bar fill in for every day you hit the goal.

<p align="center">
  <img src="docs/hud.svg" width="620" alt="The pomo mascot and clock in the focus, short-break and long-break palettes">
</p>

Each terminal cell holds **two vertical pixels** — the top pixel is the
foreground of `▀`, the bottom is its background. That doubles the vertical
resolution, which is the difference between pixel art and ASCII art.

```
▛▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▜
▌ LV.4  ▮▮▮▮▮▮▯▯ 1240xp                         STREAK 7d  ▐
▌            ██                                            ▐
▌       ▄    ██    ▄                                       ▐
▌   ▄▄████▄▄████▄▄████▄▄                                   ▐
▌     ▀██████████████▀                                     ▐
▌   ▄██████████████████▄      ▄▀▀▀▄ ▀▀▀▀▄   ▄ █   █  ▄█    ▐
▌ ▄██████████████████████▄      ▄▄▀  ▄▄▄▀   ▀ █▄▄▄█   █    ▐
▌ ████████████████████████    ▄▀        █   ▄     █   █    ▐
▌ ████████████████████████    ▀▀▀▀▀ ▀▀▀▀    ▀     ▀  ▀▀▀   ▐
▌ ▀██████████████████████▀                                 ▐
▌  ▀████████████████████▀                                  ▐
▌    ▀████████████████▀                                    ▐
▌       ▀▀▀██████▀▀▀                                       ▐
▌  ▰▰▰▰▰▰▰▰▰▰▰▱▱▱▱▱▱▱▱▱  FOCUS  ● ● ● ○  3/4               ▐
▌  ▶ work        2h 5m / 4h  ▰▰▰▰▰▱▱▱▱▱                    ▐
▙▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▟
 space pause    h habits    e note
 s skip         t stats     q quit
 r reset        z zen       / hide
```

The line under the bar is the active habit and how far into today's goal it is.

That block above is only the silhouette — colour is what the art is actually
made of. Run `pomo -demo` to see it properly.

## Features

- **Habits with goals.** "reading — 1 session a day", "work — 4h a day",
  "gym — 3 times a week". Add and edit them in the TUI; `h` picks one to work on.
- **A contribution bar per habit.** One cell per day for the last 30, shaded by
  how much of that day's goal you reached.
- **Streaks that mean something.** A habit's streak counts consecutive days its
  own goal was *met*, so a five-minute session does not keep a four-hour streak
  alive.
- **Zen mode.** `z` starts an open-ended stopwatch with no target and no habit,
  for work that should not be boxed.
- **Faceless pixel-art mascot.** Mood comes from palette and motion, not a
  cartoon face. The tomato breathes, squashes onto its base, and gives off
  steam while you're focused.
- **Time you can see.** The mascot drains as the phase runs down, and the clock
  digits roll over like an odometer.
- **A countdown that escalates.** The last ten seconds shift to amber, glow
  harder, and shake on the beat.
- **Tick a habit off without the timer.** `l` opens today's checklist; `space`
  credits one session, `u` takes it back. Some habits you want the clock to make
  you do; some you already did.
- **Log work you did elsewhere.** `pomo -log work 90m`, so the bars do not lie
  about your day.
- **Picks up where you left off.** Quit mid-session and the next launch resumes
  the same phase, clock and habit.
- **Per-habit rhythm and colour.** A habit can carry its own focus length and
  accent colour, so picking it picks how it feels.

## Requirements

A truecolor terminal (`COLORTERM=truecolor`) at least 60x20. Below that it
falls back to a compact text layout rather than rendering broken art.

Notifications and the completion sound are macOS-only; elsewhere the timer runs
quietly.

## Install

**No Go required.** `pomo` is a single static binary with the art embedded in
it, so a download is all you need.

Grab the archive for your machine from the
[latest release](https://github.com/muhalifalgibran/pixel-pomodoro/releases/latest):

```sh
tar -xzf pomo_*.tar.gz
sudo mv pomo_*/pomo /usr/local/bin/pomo
pomo
```

On macOS, Gatekeeper blocks unsigned downloads the first time. Clear it with:

```sh
xattr -d com.apple.quarantine /usr/local/bin/pomo
```

Verify a download against the published checksums:

```sh
sha256sum -c checksums.txt --ignore-missing
```

<details>
<summary>If you do have Go</summary>

```sh
go install github.com/muhalifalgibran/pixel-pomodoro/cmd/pomo@latest
```

This lands in `$(go env GOPATH)/bin`, so make sure that is on your `PATH`.

Or from a clone:

```sh
git clone https://github.com/muhalifalgibran/pixel-pomodoro
cd pixel-pomodoro
go build -o pomo ./cmd/pomo
```

</details>

## Usage

```sh
pomo                                  # resume, or a 25 minute focus session
pomo --task "write the parser"        # free-text label, with no habit
pomo -f 50m -short-break 10m          # your own rhythm
pomo -theme indigo -no-sound          # quieter
pomo -paused                          # set things up before starting
pomo -fresh                           # ignore the saved position, start over
pomo -habit work                      # start straight on a habit
pomo -zen                             # open-ended stopwatch, no goal
pomo -log work 90m                    # log work you did away from the terminal
pomo -log reading                     # log one session at that habit's length
pomo -today                           # print today's checklist and progress
pomo -habits                          # print the habit list and progress
pomo -stats                           # print stats and contribution bars
pomo --update                         # install the latest release
```

| Key | Action |
| --- | --- |
| `space` | Pause / resume |
| `s` | Skip the current phase |
| `r` | Restart the current phase |
| `h` | Habits: pick, add, edit, remove |
| `l` | Today's checklist: tick a habit off without the timer |
| `t` | Stats and contribution bars |
| `z` | Zen mode on or off |
| `e` | Free-text note (when no habit is active) |
| `/` | Show or hide the key legend |
| `q` | Quit |

In the habit list: `j`/`k` move, `enter` starts the one under the cursor, `a`
adds, `E` edits, `d` removes.

In the checklist: `j`/`k` move, `space` ticks the one under the cursor off,
`u` takes the last tick back, `enter` starts the timer on it instead.

## Updating

```sh
pomo --update
```

Fetches the latest release, checks the download against the SHA-256 published
alongside it, and replaces the running binary in place. It asks first; `-y`
skips the prompt.

The checksum is not optional. An unverified archive is never extracted, never
made executable and never written anywhere it could be run — a self-updater
that skips that step is a remote code execution vector wearing a convenience
feature's clothes. If a release ever ships without `checksums.txt`, the update
refuses rather than installing blind.

If pomo lives somewhere you cannot write, such as `/usr/local/bin`, use
`sudo pomo --update`.

## Habits

A habit is a name and a target. Press `h` then `a` to add one; there is nothing
to configure before pomo will run, and with no habits at all it behaves exactly
like a plain pomodoro timer.

Goals are written the way you would say them, and the form shows you what it
read as you type — `4h` becomes `→ 4h a day`, nonsense becomes
`→ not a goal yet`:

| Type this | Means |
| --- | --- |
| `1 session` | one completed focus session a day |
| `3 sessions` | three a day |
| `4h` | four hours of focused time a day |
| `90m` or `1h 30m` | ninety minutes a day |
| `3 sessions / week` | three in a calendar week, Monday to Sunday |
| `10h / week` | ten hours across the week |

A bare number counts sessions, so `3` is the same as `3 sessions`.

A habit can also carry its own **focus length**, so `work` runs 50/10 while
`reading` runs 25/5.

**Colours are assigned for you.** Leave the colour field empty and pomo picks
one, preferring a colour nothing else is using, so your first eight habits are
all visually distinct. It is written into `habits.json`, so you can change it
whenever you like — and clearing the field asks for a new one rather than for no
colour.

They come from a curated set rather than being genuinely random: random RGB
lands on muddy browns and near-blacks that vanish against the dark panel, and a
colour that changed between launches would defeat the point of having one.

Habits live in `habits.json` in the config directory, written atomically. They
sit with your config rather than beside your session log on purpose: habit
definitions are intent, sessions are history, and clearing your history should
not cost you your habit list.

### Two ways to move a goal

The timer is one way to feed a habit, not the only one. Press `l` for today's
checklist:

```
TODAY  1 of 2 done

  [x] reading              done              ▰▰▰▰▰▰▰▰▰▰   1d
▸ [ ] work                 0m / 4h           ▱▱▱▱▱▱▱▱▱▱    –
```

`space` credits one session — the habit's own focus length, exactly what
`pomo -log reading` records — so a "1 session a day" goal is done in one press,
and a "4h a day" goal fills a session at a time rather than in one lie. The last
tick of a goal fills only what is left, so 25-minute sessions against a 90
minute goal go 25, 50, 75, 90 rather than overshooting to 100, and a goal
already met says so instead of quietly stacking another session on top.

The key hint names the amount that press would credit — `space skip 25m`, then
`space skip 15m` once only a quarter hour is left — because calling it "done"
on a goal one press will not finish is what makes it read as a checkbox you
should press again.

`u` takes back the ticks you made this run, one session per press, all the way
down. `enter` starts the timer on that habit instead, for the ones you want to
be made to do.

A tick is an ordinary session in the log. Nothing downstream can tell it apart
from timed work, which is why the streaks and bars keep adding up.

### Streaks and the contribution bar

```
LAST 30 DAYS

  reading time  ██▒█·███▒██·█████▒███████·███   24/30  7d
  vibe antarta  ▒█··▒▒█▒·██▒·███▒·██▒█··▒██▒█   17/30  2d
  work          ███▒██·████▒██·▒████·███▒███    25/30  9d

  · none   ░ under half   ▒ close   █ goal met
```

Shade is carried by the glyph as well as the colour, so the bar still reads in a
terminal without truecolor.

A **streak** counts consecutive periods where the goal was actually met, not
merely days something was logged. A period that has not finished yet does not
break it — it should not report a broken streak because it is only 9am.

For a **weekly** goal, the streak and the met-count are weekly, but the bar is
still daily: a day is shaded against the target spread over seven days, so a
normal day's work does not look like a failure.

## Zen mode

```sh
pomo -zen      # or press z
```

A stopwatch with no target, no phases and no habit. `space` pauses it, `z` stops
and logs it.

Zen time earns XP and keeps your global streak alive — the work was real — but
it can never move a habit's goal, streak or bar, because it belongs to none.
The stats screen totals it separately so the time does not look lost.

The clock counts `MM:SS` for the first hour and `HH:MM` after that. Both are five
glyphs, which is what stops an unbounded stopwatch from reflowing the HUD; the
line beneath always spells the elapsed time out in full, since five glyphs
cannot say which unit is which.

## Configuration

`$XDG_CONFIG_HOME/pomo/config.toml`, falling back to
`~/.config/pomo/config.toml`. Every key is optional; flags override the file.

```toml
focus             = "25m"
short_break       = "5m"
long_break        = "15m"
long_break_every  = 4        # focus sessions per long break
auto_start_breaks = true
auto_start_focus  = false
show_seconds      = true     # false shows MM until the final minute
resume            = true     # pick up where you left off after a quit
scanlines         = true     # the CRT dim on alternate pixel rows
theme             = "ember"  # ember, mint, indigo or zen
notify            = true
sound             = "/System/Library/Sounds/Glass.aiff"
```

## Progress

Finished sessions are appended to `$XDG_DATA_HOME/pomo/sessions.jsonl`
(`~/.local/share/pomo/sessions.jsonl` by default), one JSON object per line:

```json
{"start":"2026-08-16T09:00:00Z","mins":25,"habit":"work","task":"work","phase":"focus","done":true}
```

`habit` is a stable ID assigned when the habit is created. Renaming a habit
therefore keeps every session it earned. Lines written before habits existed
carry only `task`, and are matched against habit names when read — nothing on
disk is ever rewritten.

The log is append-only with exactly one exception: `u` on the checklist removes
a tick you made in this run of pomo. It matches the line byte for byte, so it
can only ever take back its own entry — a finished phase, a break, a `pomo -log`
from another terminal are all left exactly where they are, and a tick from a
previous run is history rather than a stack. Every other line is copied through
untouched, including any pomo could not parse, and the replacement lands by
rename, so a crash leaves either the old file or the new one.

XP, level and streak are **derived by replaying that log** at launch. Nothing
caches them, so the numbers cannot drift out of sync with the sessions that
earned them. (The separate `state.json` described under
[Resuming](#resuming) holds only your place in an unfinished phase — it never
feeds these totals.)

- **XP** — total completed focus minutes. Skipped sessions and breaks earn none.
- **Level** — `floor(sqrt(XP / 25)) + 1`, so levels come quickly at first and
  then stretch out.
- **Streak** — consecutive days with at least one completed focus session. A day
  you haven't started yet doesn't break it.

Editing or deleting lines in the log changes your stats accordingly. That is the
intended way to correct history.

## Resuming

Quitting mid-phase writes your position to `state.json` next to the session
log. The next launch restores the phase, the remaining time, the cycle count
and the task label, and marks the session `↻ resumed`.

A saved position expires after 12 hours. Coming back the next week and landing
mid-phase in a session you have forgotten about is worse than starting clean.
`-fresh` ignores it on demand, and `resume = false` turns it off entirely.

This file is deliberately separate from `sessions.jsonl`. That log is the
append-only record of finished work and the only source of XP, level and
streak; `state.json` is throwaway state for one unfinished phase, and deleting
it costs nothing but your place.

## Editing the art

Sprites are plain text under `assets/sprites/`, one character per pixel plus a
palette. No recompile-and-guess loop: change a pixel, rebuild, run `pomo -demo`.

```
size: 24 24
palette:
  . = transparent
  G = #7cc44a    leaf light
  R = #ef5350    mid bright
  d = #a01818    dark
pixels:
  ...........ss...........
  ......G....ss....G......
  ...
```

A malformed sprite fails at startup with a line number rather than rendering
garbage.

The clock font is a 5x7 bitmap per glyph in `internal/pixfont/glyphs.go`. Four
layers composite on top of it: a dim all-`8`s **ghost** mask behind (the unlit
segments of an LCD), a hue-matched **outline**, a vertical gradient **face**,
and a **bevel** that lights the top edge of every stroke.

> **On vertical offsets:** each cell packs two pixels into one `▀`, so shifting
> a sprite by one pixel re-pairs every cell. In colour this is harmless and is
> exactly how the mascot bobs by half a cell. It only matters for
> `Canvas.Silhouette()`, the mono debug view, which collapses a two-colour cell
> into a single `█` and so looks broken at an odd offset. Static layout still
> lands on even rows to keep those debug renders comparable.

## Development

```sh
go test ./...
```

These flags ship in the binary on purpose. They are what makes a 25-minute timer
debuggable.

| Flag | What it does |
| --- | --- |
| `-demo` | Render the mascot and clock in all three palettes, then exit |
| `-demo -mono` | Opacity silhouettes only, for checking bitmaps |
| `-demo -text 08:05` | Render a specific clock string |
| `-demo -svg out.svg` | Write the art to an SVG (this is how the image above is made) |
| `-tick-scale 60` | Multiply elapsed time; fast-forwards a full cycle into seconds |
| `-skip-to-end` | Start one second from the end of a 1m phase, to verify the completion path |
| `-log-date` | With `-log`, backdate an entry, for building a history to look at |
| `-version` | Print the version and exit |

### Layout

```
cmd/pomo/            flags, config resolution, program start
internal/timer/      the state machine. no clock, no I/O, fully testable
internal/canvas/     pixel buffer, half-block ANSI emitter
internal/pixfont/    5x7 clock glyphs and their four polish layers
internal/sprite/     .pix parser
internal/theme/      per-phase palettes and cross-fading
internal/anim/       fixed-capacity particle pool, easing
internal/store/      session log and the stats derived from it
internal/habit/      habit definitions, goals, and their atomic JSON store
internal/notify/     macOS notification and sound, stubbed elsewhere
internal/paths/      where config and data live
internal/selfupdate/ --update: fetch, verify and swap the binary
internal/ui/         Bubble Tea model, HUD layout, stats and checklist screens
assets/sprites/      the art
```

Two decisions worth knowing about:

**The timer never reads the wall clock.** The UI measures real elapsed time and
feeds it in. Counting ticks would drift, and a suspended laptop hands back one
enormous delta that has to resolve into several phase transitions at once — both
are covered by tests.

**The canvas emits raw SGR with run-length coalescing** rather than going
through a styling library per pixel. Per-cell styling allocates and will not
hold 20fps on a 58x24 pixel band.

## Built with

[Bubble Tea](https://github.com/charmbracelet/bubbletea) ·
[Lip Gloss](https://github.com/charmbracelet/lipgloss) ·
[Harmonica](https://github.com/charmbracelet/harmonica) ·
[BurntSushi/toml](https://github.com/BurntSushi/toml)

## License

MIT — see [LICENSE](LICENSE).
