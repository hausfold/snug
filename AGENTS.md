# AGENTS.md

**snug** — the hausfold family's terminal presentation library. One Go package
plus one binary, because `bench` and `haus` are bash and will stay bash.

**This file is the one set of instructions, for every agent** — Claude Code,
Codex, OpenCode, Cursor, Copilot alike. Per-client wiring lives in that client's
own file (`CLAUDE.md` here is the `@AGENTS.md` import and nothing else).

## What belongs here, and what does not

| | |
|---|---|
| ✅ how a line, a table or a live region is **drawn** | here |
| ✅ what a role **degrades to** at 256 / 16 / no colour | here |
| ✅ the glyph set and its declared widths | here |
| ❌ which colour a role **is** | `hausfold/nebelung` — `palette.go` is generated from it, never hand-edited |
| ❌ **whether** a tool should print something | that tool's repo |
| ❌ the standard this implements | `hausfold/workshop`'s `docs/cli-presentation.md` — the design lives there because it binds five repos; this repo is one implementation of it |
| ❌ notifications, banners, anything off the terminal | `hausfold/trill` |

## The one rule everything else follows from

**Nothing snug draws may reach the terminal's last column.**

Not "should rarely" — may not, at any width, in any tier, including the
fallbacks. A line whose width *equals* the terminal's leaves the cursor past the
edge and the terminal wraps it anyway, which is why `Term.Avail()` is
`Width - 1` and not `Width`.

Every defect this library was written for is a violation of that rule:

- `bench release` repainted a job list by moving the cursor up by the number of
  rows it **printed**. The row was `%-34s`-padded, so it was 48 cells whatever
  the jobs were called, and any terminal ≤48 columns wrapped each row into two
  screen lines while the cursor walked up through the middle of the block.
- `haus`'s phase painter uses `\r`, which returns to column 0 of the current
  **physical** row — so a wrapped phase line orphans its own stub above it.
- `printf '%-9s'` pads by **bytes**. Every column after a multi-byte glyph was
  sheared by a different amount depending on which glyph it was.

`TestRegionNeverReachesTheLastColumn` and `TestTableNeverReachesTheLastColumn`
sweep widths 2–200 and are the tests to run first when anything here changes.
The floor is **2 cells**: one glyph. At 1 there is nothing honest left to draw.

## Traps, each of which cost a debugging session

- **`tput cols` is wrong and will pass review.** terminfo carries a *static*
  size — 80 for every `xterm-*` entry — and answers **80 in a 40-column pty**.
  Only `TIOCGWINSZ` tracks a resize, which is what `x/term` asks. In a shell,
  that is `stty size`, read from `/dev/tty` and not `<&1`, because inside `$( )`
  fd 1 is the substitution's pipe.
- **No width library can be trusted with 🌫.** U+1F32B has
  `Emoji_Presentation = No`, so `x/ansi` and `runewidth` both answer **1** while
  every terminal draws it in **two** cells — and the two libraries disagree with
  each other on the variation-selector form. Glyph widths are therefore
  **declared** in `glyph.go` and verified against real terminals; measurement
  libraries are used only on *content*, which is ordinary text and where they
  are reliable. To check a terminal: `printf '123456789\n\U0001F32B|\n'` — the
  `|` sits at column 3 if the glyph is two cells.
- **Choose the tier from the WINDOW, then clamp the column to the CONTENT.**
  Never the other way round. Clamping first and testing the clamped value asks
  *"is the longest name short?"* when it means *"is there room?"* — and a CI run
  of `build` / `test` / `lint` drops its durations on a 200-column terminal.
  `TestShortNamesKeepTheirDetail` is that bug.
- **A column that never truncates must be MEASURED, not assumed.** `bench`
  budgeted seven cells for a duration because `12m 34s` is the longest one.
  GitHub allows six-hour jobs; `100m 05s` is eight.
- **Nearest-RGB is the wrong algorithm at 16 colours.** nebelung is pastel, so
  `green` and `peach` both land nearer mid-grey than any named colour and `ok`
  and `warn` come out identical — the two roles it matters most to tell apart.
  `role16` maps by *intent* instead, and `TestSixteenColoursCollapseDeclaredRoles`
  holds it.
- **Anything that includes a separator must be re-measured with it.**
  `TruncateLeft` cuts paths at a `/` and includes it, so "the suffix after the
  slash fits" says nothing about whether the slash does. That off-by-one shipped
  once and made one row of a table exactly one cell wider than its neighbours.

## Streams

**Stdout carries DATA only.** Every diagnostic, prompt and progress line goes to
stderr, because callers do `cd "$(holt child …)"` and hooks read paths off
stdout. `Say`/`Warn`/`Fail` write to `Err`; `Data` is the only thing that writes
to `Out`.

This is also why `snug run` works as a bash `coproc`: bash pipes stdin and
stdout and leaves stderr on the terminal, which is exactly the split this wants.

## Cost, and why the shape is what it is

Measured before the dependency was chosen, on this machine:

| | binary | modules | cold start |
|---|---|---|---|
| `charm.land/x/ansi` + `x/term` | 2.3 MB | 9 | 4.5 ms |
| `charm.land/lipgloss/v2` + `x/term` | 3.0 MB | 22 | 4.4 ms |

lipgloss is the obvious answer and the wrong one: its headline features are
borders, boxes and joins, and the family's look is quiet — aligned text and a
fog palette. The parts of lipgloss we would actually use are the parts `x/ansi`
already is. Not bubbletea either: it wants to own the event loop and the screen,
and `bench` needs a filter *it* drives.

**A fork costs ~4.5 ms, so fork per COMMAND, never per line.** Sixty lines of
`haus rebuild` through `snug say` would be 270 ms of pure overhead; one
`snug run` coprocess for the whole command is one fork. Any change that puts a
process in a loop is a regression, whatever the benchmark says about the loop.

## The palette is generated

`palette.go` is written by `script/gen-palette.sh` from a nebelung checkout and
carries a `DO NOT EDIT` header. Hand-editing it puts the family back where it
started: seven hand-picked 256-colour indices, ΔE 2–27 from the flavour every
other tool on the machine was wearing, with two different greys for one role and
a primary accent that resolved to **blue** — the one hue nebelung exists to
strip out.

Adding a role means a row in `roleToken`, a row in `role16`, a row in the
`TOKENS` list in `script/gen-palette.py`, and a regeneration. Four places on
purpose: a role without a 16-colour answer is a role that vanishes on somebody's
terminal.

## Working here

- `go test ./...` before anything. The width sweeps are fast and they are the
  point.
- `go build -o snug ./cmd/snug && ./snug demo`, then resize the window. That is
  the feel-test; the unit tests prove the bound, the demo shows the taste.
- `gofmt -w .` — CI checks it.
- **Ship by default, sized to the change.** Small things commit, verify and
  ship. Anything that changes what a caller sees waits for the user.
- **Releases are gated** and go through the workshop's `bench release`.
- `vendorHash` in `flake.nix` is `null` until the first Nix build; take the hash
  the failure prints and pin it. A `null` hash means the build has network, which
  works locally and fails in CI.
