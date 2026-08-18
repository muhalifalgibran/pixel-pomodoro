# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```sh
go build -o pomo ./cmd/pomo     # build
go test ./...                   # all tests
go test -race ./...             # what CI runs
go vet ./...
gofmt -l .                      # must print nothing; CI fails on any output

go test ./internal/ui/ -run TestHabitRowsFitTheFrame -v   # one test
go test ./internal/store/ -run 'Undo|RemoveLast'          # one pattern
```

CI (`.github/workflows/ci.yml`) runs gofmt / vet / `test -race`, then
cross-compiles `./cmd/pomo` for darwin arm64+amd64, linux amd64+arm64 and
windows amd64 with `CGO_ENABLED=0`. The art is `go:embed`ed, so a broken sprite
is a build-time failure — run the cross-compile check before touching `assets/`.

### Driving the app without waiting 25 minutes

These flags ship in the binary on purpose; they are the manual test harness.

```sh
pomo -tick-scale 60      # multiply elapsed time — a full cycle in seconds
pomo -skip-to-end        # start 1s from the end of a 1m phase
pomo -demo               # render the art in every palette and exit
pomo -demo -svg out.svg  # regenerate docs/hud.svg
pomo -log-date 2026-08-16 -log work 4h   # backdate, to build a history to look at
```

Never point these at your own data. Use a scratch dir:

```sh
export XDG_DATA_HOME=$(mktemp -d) XDG_CONFIG_HOME=$(mktemp -d)
```

## Architecture

Go + Bubble Tea TUI. A **habit tracker whose input device is a pomodoro timer** —
with zero habits defined it degrades to a plain pomodoro.

### Everything is derived from one append-only log

`sessions.jsonl` is the only source of truth. XP, level, streaks, per-habit
progress and the contribution bars are **recomputed by replaying it** —
`store.Compute` and `store.Progress`. Nothing caches them, so they cannot drift.

Consequences worth internalising before changing anything in `internal/store`:

- **Never add a derived field to disk.** If you find yourself wanting to persist
  a total, recompute it instead.
- `Session.IsWork()` (`Done && Mins > 0 && (phase == focus || zen)`) gates every
  figure. Breaks and skipped phases are filtered out everywhere.
- **`Session.Done` does not mean "habit complete."** It means the phase ran to
  its end rather than being skipped. Habit completion is derived:
  `p.Met = p.Value >= h.Goal.Target` (`internal/store/progress.go`).
- The log is append-only with **one** exception: `Store.Remove` drops a line
  that is byte-identical to the session handed to it, wherever it now sits, via
  a raw-line rewrite and an atomic rename. Byte matching is the safety property
  — a caller can only take back its own append. **Do not rebuild the file from
  `Load()`** — `Load` skips unparseable lines on purpose, so writing back what
  it returned would silently delete damaged-but-present history. `Remove`
  therefore works on raw lines, and a test pins that.

### Time is always injected, never read

`timer.State.Advance(elapsed)`, `store.Compute(sessions, now)`,
`store.Progress(sessions, habits, now)` — every one takes time as a parameter.
`internal/timer` touches neither the clock nor the filesystem. This is why no
test sleeps, and why a suspended laptop handing back one enormous delta resolves
into several phase transitions correctly. Keep new code on the same rule.

Day boundaries are **noon-anchored** (`civil()`, `weekStart()`) so `AddDate`
arithmetic survives DST. Weeks start Monday.

### Three files, two roots

| File | Where | Why there |
|---|---|---|
| `habits.json` | config dir | habit definitions are **intent** |
| `sessions.jsonl` | data dir | sessions are **history** |
| `state.json` | data dir | your place in an unfinished phase, 12h expiry |

Clearing your history must not cost you your habit list — hence the split.
`habits.json` and `state.json` are written temp-file-plus-rename; `config.toml`
is read-only to pomo and never rewritten.

### Adding a screen to the TUI

`internal/ui/model.go` holds one `mode` enum. Note `zen` is an **orthogonal
bool**, not a mode — it changes what `modeNormal` renders. A new screen is five
mechanical parts:

1. A `mode` constant (append to the enum).
2. A `*Keys []string` legend slice in `text.go`, and its entry in
   `TestEveryLegendFitsUnderTheFrame`'s map.
3. A `case` in `handleKey`'s mode switch.
4. A per-mode key handler (`habitsKey` / `checkKey` are the models to copy).
5. A `case` in `View()`, wrapped in `withLegend`.

**Every full-screen view has a print-and-exit twin** that calls the same render
function — `-stats`/`StatsReport`, `-habits`/`HabitsReport`,
`-today`/`TodayReport`. Keep that pattern: it is what makes the views testable
as pure functions.

### Layout is measured to the cell

The HUD frame is `geom.BandW + 2` — derived from the sprite and clock widths,
currently 60 cells. Row column widths are declared as constants that **sum to
it** (`view_habits.go`, `view_check.go`), so widening one column means narrowing
another. Tests assert both the row widths and that every legend fits under the
frame; changing the art changes the budget. `helpBlock` reflows a key set to one line or a
downward-filling grid based on measured width, never on a hint count.

The pixel band uses half-blocks — each cell is two vertical pixels — so sprite
and clock blits must land on **even rows** or every cell re-pairs and the art
mangles. `canvas` emits raw SGR with run-length coalescing rather than styling
per pixel, which is what holds 20fps.

## Conventions

- **Comments explain *why*, usually naming the rejected alternative.** Match
  this; it is the dominant style and the reason the codebase is navigable.
- Tests: stdlib `testing` only. No testify, no golden files, no `testdata/`.
  Table tests are anonymous struct slices plus `t.Run`. `t.TempDir()` for every
  store. UI tests drive the real `tea.Model` and assert `strings.Contains` on
  `m.View()` plus width invariants — helpers `testModel`, `habitModel`, `press`,
  `tick`, `sizeTo` in `ui_test.go`.
- A test that pins behaviour to a live key set or column constant will be
  rewritten every time a screen gains a key. Prefer a fixture that states the
  rule (see `TestHelpBlockIsAGridOfColumns`).
- Commits: lowercase Conventional Commits, subject describes the effect
  (`feat: -log records work done away from the timer`).
- Functions that write user-visible output take an `io.Writer` so they are
  testable (`logSession`, `printToday`).

## Other agent configs

An OpenAI Codex config exists at `~/.codex/config.toml`. To bring its MCP
servers, commands, subagents or instructions across, reply `/import` to scan and
list what is importable, then `/import --yes=<digest>` to apply the user-level
items. (If `/import` is unavailable here, run `claude import` from a terminal.)
