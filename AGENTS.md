# AGENTS.md

**snug** — the hausfold family's terminal presentation library. One Go package,
one binary, and one bash fallback (`share/ui.sh`), because `bench` and `haus`
are bash and will stay bash.

**This file is the one set of instructions, for every agent** — Claude Code,
Codex, OpenCode, Cursor, Copilot alike. Per-client wiring lives in that client's
own file (`CLAUDE.md` here is the `@AGENTS.md` import and nothing else).

## What belongs here, and what does not

| | |
|---|---|
| ✅ the **standard itself** — roles, marks, layout, the live-region contract, the record protocol | here, in `README.md`, and the rules a *caller* has to meet are below. It binds five repos and there is no second copy: a rule missing here is missing everywhere |
| ✅ how a line, a table or a live region is **drawn** | here |
| ✅ what a role **degrades to** at 256 / 16 / no colour | here |
| ✅ the glyph set and its declared widths | here |
| ✅ the **bash fallback** — `share/ui.sh`, the same spec for a machine with no `snug` on PATH | here, since 2026-08-27 |
| ❌ which colour a role **is** | `hausfold/nebelung` — `palette.go` is generated from it, never hand-edited |
| ❌ **whether** a tool should print something | that tool's repo |
| ❌ notifications, banners, anything off the terminal | `hausfold/trill` |

## The one rule everything else follows from

**Nothing snug draws may reach the terminal's last column.**

Not "should rarely" — may not, at any width, in any tier, including the
fallbacks. A line whose width *equals* the terminal's leaves the cursor past the
edge and the terminal wraps it anyway, which is why `Term.Avail()` is
`Width - 1` and not `Width` — on a terminal. A stream with no window has no last
column to stay inside, and gets `NoFold`, exactly as `Prose()` and
`share/ui.sh`'s `ui_measure` already do: fitting a pipe to 80 cells is the
`tput cols` mistake one layer up.

Every defect this library was written for is a violation of that rule:

- `bench release` repainted a job list by moving the cursor up by the number of
  rows it **printed**. The row was `%-34s`-padded, so it was 48 cells whatever
  the jobs were called, and any terminal ≤48 columns wrapped each row into two
  screen lines while the cursor walked up through the middle of the block.
- `haus`'s phase painter uses `\r`, which returns to column 0 of the current
  **physical** row — so a wrapped phase line orphans its own stub above it.
- `printf '%-9s'` pads by **bytes**. Every column after a multi-byte glyph was
  sheared by a different amount depending on which glyph it was.

`TestRegionNeverReachesTheLastColumn`, `TestTableNeverReachesTheLastColumn`,
`TestBashTableNeverReachesTheLastColumn` and bats's *"a table never reaches the
last column"* sweep widths 2–200 and are the tests to run first when anything
here changes. `TestBashTableMatchesGo` is the other one that matters most: it
diffs the two painters line for line at every width, at `none` **and** at `256`
— the second because a role with the colour off leaves no trace, so layout is
all the first can compare.
The floor is **2 cells**: one glyph. At 1 there is nothing honest left to draw.

## Traps, each of which cost a debugging session

- **`tput cols` is wrong and will pass review.** terminfo carries a *static*
  size — 80 for every `xterm-*` entry — and answers **80 in a 40-column pty**.
  Only `TIOCGWINSZ` tracks a resize, which is what `x/term` asks. In a shell,
  that is `stty size`, read from `/dev/tty` and not `<&1`, because inside `$( )`
  fd 1 is the substitution's pipe.
- **No width library can be trusted with a mark, emoji or not.** Taking the
  emoji-presentation glyphs out of the table did not fix this, because the
  hazard is `East_Asian_Width`, in two shapes. `ⓘ` (U+24D8), `–` (U+2013), `·`
  (U+00B7) — and `…` (U+2026), the Ellipsis rather than a mark — are
  **Ambiguous**: one cell in a Western locale, **two** under an East-Asian one,
  and `x/ansi` and `runewidth` each expose a mode for each answer, so neither
  has a single one to give. `⚠` (U+26A0) is the other: `Emoji = Yes` and one
  cell bare, two as the emoji-presentation sequence `U+26A0 U+FE0F`, which is
  why the table holds bare codepoints and a caller must never append a
  variation selector to a mark. Glyph widths are therefore **declared** in
  `glyph.go` and verified against real terminals; measurement libraries are used
  only on *content*, which is ordinary text and where they are reliable. To
  check a terminal, hold a mark against a ruler and read the DIGITS — another
  mark is no reference, since a locale that doubles one Ambiguous glyph doubles
  them all: `printf '123456789\n\u24D8|\n'` — the `|` sits at column 3 if the
  glyph is two cells.
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
- **A `defer` does not survive SIGINT, and the LIBRARY still must not take it.**
  Go's default disposition for SIGINT terminates without running one, so
  `defer r.Close()` — the pattern every Go consumer writes, and the one the
  README shows — leaves the cursor hidden on the user's terminal for good. The
  fix is split on purpose: `Printer.CloseLive()` is the seam for a handler that
  cannot hold the `*Region`, and **only `cmd/snug` installs the handler**. A
  package that quietly claims SIGINT is a package its importer has to fight, and
  scruff imports this one. The handler calls `signal.Reset` as its FIRST
  statement, before `CloseLive` can block on the printer's mutex, or a second ⌃C
  lands in a channel nobody is reading and the process becomes unkillable.

## Rules a bash caller has to meet

These are not about drawing; they are the ways a *consumer* has broken this
library from the outside. Each cost a debugging session in `bench` or `haus`.

- **The live region opens the coprocess and closes with it.** One fork per
  command is the whole economy, and only the caller can see where a command
  begins — so the dispatch belongs to the caller, not to `ui.sh`. Open only
  under a terminal on fd 2 and only when ui.sh loaded, so a machine with the
  binary but no fallback gets one answer rather than two. A `snug` that dies
  once must stay dead for the command: a failed record write that re-forks per
  frame is the regression the coprocess exists to prevent.
- **Outside a region the coprocess is the wrong writer, and a message verb must
  never open one.** A record crosses a pipe and is drawn by another process on
  ITS stderr; a table the caller `printf`s goes straight to the terminal. The
  two are on different schedules, so mixing them prints rows above the title
  that introduces them. Inside a region there is no race, because a caller
  writes nothing directly while one is up. A run of regions may share one
  coprocess, but the close cannot wait for the command — a caller that ends
  inside another command's tables, or dumps a build log on failure, needs the
  coprocess already gone.
- **Anything drawing from a background job needs its own duplicate of the write
  end.** Bash closes a coprocess's descriptors in every child it forks, so a
  background painter writing to `${SNUG[1]}` silently does nothing — the row
  freezes while every assertion stays green. `exec {FD}>&"${SNUG[1]}"` is the
  fix; bash's own copy is then closed, or nothing reaches EOF. **Its converse:
  anything backgrounded that draws NOTHING must drop that fd**, because the
  close waits for EOF and an inherited copy keeps it from arriving — and where
  that job's exit condition is "the parent is gone", the two wait for each other
  with no clock on it. **Write a test that counts frames**: a spinner that never
  turns looks exactly like a phase that is taking a while.
- **Nothing may repaint while `sudo` might be asking for a password.** The
  prompt goes to `/dev/tty` — the terminal the region repaints, and one its line
  count knows nothing about. Probe with `sudo -n true` and draw a still row when
  it fails; the safe direction is the still row.
- **Never name your path-to-ui.sh variable `UI_SH`.** That is ui.sh's own
  source-twice sentinel (`[ -n "${UI_SH:-}" ] && return 0`), so a caller holding
  the path in it makes the file return before defining anything: no error, no
  colour, and a suite that stays green because every role is legitimately empty
  when the painter is absent.
- **ui.sh is bash 4+, and macOS's `/bin/bash` is 3.2**, where it does not
  degrade but half-loads with `bad substitution`. Use `#!/usr/bin/env bash` and
  a `BASH_VERSINFO` guard, and put the guard in the text that actually *sources*
  it: for a script handing a snippet to another shell, the snippet is the
  caller, and a `grep` against the outer file is satisfied by a guard protecting
  nothing.
- **A width probe must not be able to kill its caller.** `sz="$(stty size …)" &&
  COLS=… || COLS="$(tput cols)"` — `set -e` exempts every command in such a list
  *except the last*, and `tput` exits 2 with `TERM` unset, which is any session
  with no pty. The caller then exits 2 with nothing on either stream. `|| true`
  inside that final substitution is the fix, and ui.sh deliberately does not
  inherit the line.
- **Force the gate where colour is correct without a tty**, rather than
  measuring — a statusline rendered with both descriptors captured and then
  printed into a terminal is the case. Force only the TTY answer, leaving
  `NO_COLOR` and `TERM=dumb` still able to win.
- **Ask the precedence, never re-derive it.** ui.sh measures both streams at
  load and resolves a palette for each: narration reads `UI_*`, a report reads
  `UI_OUT_*`. Swapping `UI_TTY` and asking again is how one binary comes to
  answer `NO_COLOR` + `CLICOLOR_FORCE` two ways. Neither answer is wrong in the
  abstract; two answers in one binary is.

## Streams

**Stdout carries DATA only.** Every diagnostic, prompt and progress line goes to
stderr, because callers do `cd "$(scruff child …)"` and hooks read paths off
stdout. `Say`/`Warn`/`Fail` write to `Err`; `Data` and `PrintData` are the only
things that write to `Out`.

A **report** — the table a user ran the command for, rather than the tool
talking about it — is data, so it goes to `Out` through `PrintData`, which
measures `Out` to do it. `Print` is the other half, for a table that is part of
what the tool is *saying*. Geometry and palette always come from the stream a
line lands on: ask the other one and a TTY stdout beside a redirected stderr
draws plain, while a piped stdout beside a live stderr draws escapes into the
pipe.

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

## The palette is generated — both copies of it, in one run

`script/gen-palette.sh` writes **two** files from one nebelung checkout and one
`TOKENS` list:

| | |
|---|---|
| `palette.go` | rewritten whole, then `gofmt`ed. `DO NOT EDIT` header. |
| `share/ui.sh` | the `UI__HEX` and `UI__X256` blocks between the `▼▼▼`/`▲▲▲` markers, spliced in place — the rest of that file is hand-written. |

Hand-editing either puts the family back where it started: seven hand-picked
256-colour indices, ΔE 2–27 from the flavour every other tool on the machine was
wearing, with two different greys for one role and a primary accent that
resolved to **blue** — the one hue nebelung exists to strip out.

**One generator for both, on purpose.** `UI__X256` is `theme.go`'s `nearest256`,
ported into `gen-palette.py` digit for digit, weights and all — the bash half
answering a *different* index from the binary would be worse than having no
fallback, because it makes "which machine is this?" a question you have to ask
about your own output. Two generators kept in step by hand is that bug with
extra steps.

⚠️ The weights matter and are easy to drop. `dist` is 3/6/1, not plain
Euclidean; plain Euclidean picks a visibly different neighbour for colours this
low in chroma. `test/ui.bats` re-derives all thirty-two indices with its own
copy of the arithmetic — written out again rather than imported, because a test
that asks the generator to check its own maths checks nothing.

Adding a role means a row in `roleToken`, a row in `role16`, a row in
`roleNames`, a row in `share/ui.sh`'s `UI__TOKEN` **and** `UI__ANSI16`, a word in
its `UI__ROLE_LIST`, a row in the `TOKENS` list in `script/gen-palette.py`, and
a regeneration. Seven places on purpose, and each omission has its own silent
failure: a role with no 16-colour answer vanishes on somebody's terminal, one
the fallback has never heard of vanishes on somebody's *machine*, one missing
from `roleNames` makes `Role.String()` answer `body` so every `snug.Cell` with
it draws unpainted, and one missing from `UI__ROLE_LIST` makes `ui_cell` emit
its raw `\037` tag into the terminal **and** measure it, shearing the column.

## Working here

- `go test ./...` before anything. The width sweeps are fast and they are the
  point.
- **`bats test/ui.bats` and `shellcheck share/ui.sh` are the other half**, and
  the Go suite never looks at them. Both run in CI's `bash` job. The bats tests
  invoke `"$BASH"`, never a bare `bash`: the library needs bash 4 and macOS
  ships 3.2 as `/bin/bash`, so a bare name fails on the associative array rather
  than on the thing under test.
- `go build -o snug ./cmd/snug && ./snug demo`, then resize the window. That is
  the feel-test; the unit tests prove the bound, the demo shows the taste.
- `gofmt -w .` — CI checks it.
- **Ship by default, sized to the change.** Small things commit, verify and
  ship. Anything that changes what a caller sees waits for the user.
- **Releases are gated** and go through the workshop's `bench release`.
- `vendorHash` in `flake.nix` is **pinned, and must never go back to `null`** —
  a `null` hash lets the build fetch the module graph at run time, which works on
  a laptop and fails in a sandboxed build. Change `go.mod`/`go.sum` and the pin
  is stale: `nix build .#default` prints the mismatch, and the `got:` line is the
  new value.
- **`share/ui.sh` is part of the derivation** (`postInstall`), not just of the
  repo — `haus` reads `${snug}/share/ui.sh` off the store path, on machines with
  no checkout of anything. Moving or renaming it breaks a consumer that CI here
  cannot see; `nix build .#default && ls result/share` is the check.
- The flake also ships `overlays.default`, which is how `pkgs.snug` reaches a
  consumer — `haus` takes this flake as an input and puts the binary on PATH from
  that overlay. Adding an output that consumers read means bumping `haus`'s lock
  (`bench ship` from the workshop) before anything downstream can see it.
