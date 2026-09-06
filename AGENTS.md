# AGENTS.md

**snug** — how the hausfold family puts a line on screen: one Go package, one
binary (`cmd/snug`) and `share/ui.sh`, the same spec in bash for `bench`, `haus`
and any machine with no `snug` on PATH. One file for every agent; `CLAUDE.md`
only imports it.

## What lives here

| | |
|---|---|
| the standard — roles, marks, layout, live-region contract, record protocol | `README.md`; the rules a *caller* must meet are below. No second copy exists: a rule missing here is missing everywhere |
| how a line, table or region is drawn; what a role degrades to at 256 / 16 / none; glyph widths; the bash fallback `share/ui.sh` | here |
| which colour a role **is** | `hausfold/nebelung` — `palette.go` is generated from it |
| **whether** a tool prints something | that tool's repo |
| anything off the terminal | `hausfold/trill` |

## The one rule

**Nothing snug draws may reach the terminal's last column** — at any width, in
any tier, fallbacks included. A line as wide as the terminal wraps, so
`Term.Avail()` is `Width - 1`; a stream with no window gets `NoFold`, as
`Prose()` and `ui_measure` do. The floor is 2 cells: one glyph.

Run first on any change: `TestRegionNeverReachesTheLastColumn`,
`TestTableNeverReachesTheLastColumn`, `TestBashTableNeverReachesTheLastColumn`
and bats's *"a table never reaches the last column"*, all sweeping widths 2–200.
`TestBashTableMatchesGo` diffs the two painters at every width, at `none` and at
`256` — colour off leaves no trace, so `none` compares layout only.

## Traps

- **`tput cols` answers 80 in a 40-column pty** — terminfo is static; only
  `TIOCGWINSZ` (`x/term`) tracks a resize. In a shell that is `stty size` read
  from `/dev/tty`, not `<&1`: inside `$( )` fd 1 is the pipe.
- **Glyph widths are declared in `glyph.go`, never measured** (why: README's
  preamble). Width libraries measure content only; never append a variation
  selector to a mark. Check a terminal against digits, never another mark:
  `printf '123456789\n\u24D8|\n'` — `|` at column 3 means two cells.
- **Choose the tier from the WINDOW, then clamp the column to the CONTENT.**
  The other order drops durations on a 200-column terminal:
  `TestShortNamesKeepTheirDetail`.
- **A column that never truncates is measured, not assumed.** `12m 34s` is
  seven cells; GitHub allows six-hour jobs, and `100m 05s` is eight.
- **Nearest-RGB is wrong at 16 colours** — pastel `green` and `peach` both land
  on grey, so `ok` and `warn` merge. `role16` maps by intent;
  `TestSixteenColoursCollapseDeclaredRoles`.
- **Re-measure with the separator in.** `TruncateLeft` cuts at a `/` and keeps it.
- **A `defer` does not survive SIGINT, and the library must not take it.**
  `Printer.CloseLive()` is the seam; only `cmd/snug` installs a handler, since
  scruff imports this package. It calls `signal.Reset` FIRST, before
  `CloseLive` can block on the printer's mutex, or a second ⌃C is unkillable.

## Rules a bash caller has to meet

Each has broken `bench` or `haus` from the outside.

- **The live region opens the coprocess and closes with it** — one fork per
  command, and only the caller sees where a command begins, so dispatch is the
  caller's, never `ui.sh`'s. Open only with a terminal on fd 2 and ui.sh
  loaded. A `snug` that dies stays dead for the command; never re-fork per frame.
- **Outside a region the coprocess is the wrong writer, and a message verb
  never opens one** — a record is drawn by another process on its own schedule
  and lands above the title a `printf`ed table follows. Close it before the
  command's own tables or a dumped build log, never with the command.
- **A background job that draws needs its own copy of the write end.** Bash
  closes coprocess fds in every child, so writes to `${SNUG[1]}` silently
  vanish: `exec {FD}>&"${SNUG[1]}"`, then close bash's copy or EOF never comes.
  Converse: a background job that draws NOTHING must drop that fd, or the close
  waits forever. Write a test that counts frames.
- **Nothing repaints while `sudo` might be prompting** on `/dev/tty`. Probe
  `sudo -n true`; on failure draw a still row.
- **Never name your ui.sh path variable `UI_SH`** — it is ui.sh's source-twice
  sentinel (`[ -n "${UI_SH:-}" ] && return 0`), so the file returns before
  defining anything and every role is legitimately empty.
- **ui.sh is bash 4+; macOS's `/bin/bash` is 3.2** and half-loads with
  `bad substitution`. `#!/usr/bin/env bash` plus a `BASH_VERSINFO` guard, in
  the text that *sources* it — a snippet handed to another shell is the caller.
- **A width probe must not kill its caller.** In
  `sz="$(stty size …)" && COLS=… || COLS="$(tput cols)"` the last command is
  not `set -e`-exempt, and `tput` exits 2 with `TERM` unset: `|| true` inside
  that substitution.
- **Force the colour gate where colour is correct without a tty** (a statusline
  with both descriptors captured); force only the TTY answer, so `NO_COLOR` and
  `TERM=dumb` still win.
- **Ask the precedence, never re-derive it.** Narration reads `UI_*`, a report
  reads `UI_OUT_*`. Swapping `UI_TTY` is how one binary answers `NO_COLOR` +
  `CLICOLOR_FORCE` two ways.

## Streams

**Stdout carries data only** — callers do `cd "$(scruff child …)"`. `Say`,
`Warn` and `Fail` write to `Err`; `Data` and `PrintData` are the only writers
of `Out`. A report is data: `PrintData` measures `Out`; `Print` is the stderr
half. Geometry and palette come from the stream a line lands on, never the
other. `snug run` works as a `coproc` because bash pipes stdin/stdout and
leaves stderr on the terminal.

## Cost

| | binary | modules | cold start |
|---|---|---|---|
| `charm.land/x/ansi` + `x/term` | 2.3 MB | 9 | 4.5 ms |
| `charm.land/lipgloss/v2` + `x/term` | 3.0 MB | 22 | 4.4 ms |

lipgloss is borders and boxes; what we would use of it `x/ansi` already is. Not
bubbletea: it owns the event loop, and `bench` needs a filter it drives. **A
fork is ~4.5 ms, so fork per COMMAND, never per line** — sixty `snug say`s in a
`haus rebuild` is 270 ms; one `snug run` is one fork.

## The palette is generated — both copies, one run

`script/gen-palette.sh` writes from one nebelung checkout and one `TOKENS` list:
`palette.go` whole (`gofmt`ed, `DO NOT EDIT`), and `share/ui.sh`'s `UI__HEX` and
`UI__X256` blocks between the `▼▼▼`/`▲▲▲` markers — the rest of ui.sh is
hand-written. Never hand-edit either. `UI__X256` is `theme.go`'s `nearest256`
ported into `gen-palette.py` digit for digit: `dist` is weighted 3/6/1, not
Euclidean, and `test/ui.bats` re-derives all thirty-two indices with its own
copy of the arithmetic.

Adding a role is seven places plus a regeneration: `roleToken`, `role16`,
`roleNames`, `UI__TOKEN`, `UI__ANSI16`, `UI__ROLE_LIST`, and `TOKENS` in
`script/gen-palette.py`. Each omission is silent: no `role16` row vanishes on a
16-colour terminal, no `roleNames` row makes `Role.String()` answer `body` so
every `snug.Cell` draws unpainted, no `UI__ROLE_LIST` word makes `ui_cell`
print and measure its raw `\037` tag.

## Working here

- `go test ./...` first; the width sweeps are the point. CI also runs
  `gofmt -w .`, `go vet ./...` and `go test -race`.
- `bats test/ui.bats` and `shellcheck share/ui.sh` are the other half; the Go
  suite never sees them. bats runs `"$BASH"`, never bare `bash` — macOS's 3.2
  fails on the associative array first.
- Feel-test: `go build -o snug ./cmd/snug && ./snug demo`, then resize.
- Ship by default, sized to the change; anything a caller can see waits for
  the user.
- **Not releasable** — consumers pin it by rev, so `bench release snug`
  refuses. `VERSION` only names the derivation (`snug-0.1.0`).
- `vendorHash` in `flake.nix` is pinned, never `null` — `null` fetches at build
  time and fails sandboxed. After a `go.mod`/`go.sum` change,
  `nix build .#default` prints the mismatch; take the `got:` line.
- `share/ui.sh` ships in the derivation (`postInstall`); `haus` reads
  `${snug}/share/ui.sh` off the store path, so moving it breaks a consumer this
  CI cannot see. Check: `nix build .#default && ls result/share`.
- `overlays.default` is how `pkgs.snug` reaches `haus`; a new output is
  invisible downstream until `bench ship` bumps haus's lock.
