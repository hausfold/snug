# snug

**How the hausfold family puts a line on screen.**

A cat's whiskers are how it knows whether it fits through the gap. Every CLI
defect this library exists for is a tool that never measured — a job list
repainted 48 cells wide in a 40-column window, a `%-38s` column that wrapped,
a glyph padded by bytes when the terminal counts cells.

`snug` measures, budgets and folds. Nothing it draws ever reaches the
terminal's last column, at any width, so a live region's printed rows and the
screen's lines are equal by construction — which is the property every
in-place repaint quietly depends on and none of ours held.

```
≋   snug — every mark and role, on this terminal
✓   ok · something current, healthy, passed
⚠   warn · something stale, degraded, wanting attention
✗   fail · something that failed, was refused, or is missing
    haus     current …/modules/core/haus.sh 2h
    scruff   stale   …/internal/ui/ui.go    3d
    nebelung current …/nebelung.json        11m
```

Glyph widths are **declared**, not measured. No mark defaults to emoji
presentation, and that is not the same guarantee as safety: `ⓘ`, `–`, `·` and
the truncation `…` are `East_Asian_Width = Ambiguous`, so they are one cell in
a Western locale and two under an East-Asian one — and every width library
ships a mode for each, which means a library has no single answer to give. `⚠`
is one cell bare and two as the emoji-presentation sequence `U+26A0 U+FE0F`, so
the table holds bare codepoints. None of that is a terminal, and the table
records what a terminal actually draws.

Narrow the window and nothing wraps: a live region sheds its detail column and
padding together, then its gutter; a table budgets down to its minimums, then
stacks into label/value pairs.

## For Go

```go
p := snug.NewPrinter()
p.Say("resolving %d inputs", n)
p.OK("nebelung is current")

p.PrintData(snug.Table{ // → stdout, budgeted for stdout
    Indent: 3,
    Cols: []snug.Col{
        {Head: "repo", Min: 6, Weight: 2, Role: snug.Subject},
        {Head: "path", Min: 10, Weight: 4, Role: snug.Path, Cut: snug.CutLeft},
    },
    Rows: rows,
})

r := p.Live()
defer r.Close()
for {
    r.Set(rows)     // repaints in place; SIGWINCH is handled
}
```

A `defer` does not survive a ⌃C: Go's default SIGINT disposition terminates the
process without running one, so `r.Close()` never fires and the cursor stays
hidden on the terminal for good. snug will not install a signal handler for you
— your program owns that policy — so if you open a live region, close it from
your own handler with `p.CloseLive()`, which is safe with no region open and
safe to call twice:

```go
sig := make(chan os.Signal, 1)
signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
go func() {
    s := <-sig
    signal.Reset(os.Interrupt, syscall.SIGTERM) // a second ⌃C must still kill
    p.CloseLive()
    os.Exit(128 + int(s.(syscall.Signal)))
}()
```

`snug run` does exactly this for shell callers, so a `coproc` needs nothing.

## For shells

```sh
snug say "resolving inputs"          # one line, one fork (~4.5 ms)

coproc SNUG { snug run; }            # one fork for a WHOLE command
printf 'row\trun\tpublish\t\npaint\n' >&${SNUG[1]}
```

Use `run` for anything with a live region or more than a handful of lines. A
fork is ~4.5 ms, so **fork per command, never per line**: sixty `snug say`s in a
`haus rebuild` is 270 ms of pure overhead, and one `snug run` is one fork. `run`
also lets log lines scroll *above* a spinner that is still turning — which a
shell painter cannot do at all.

`snug caps` reports what it detected; `snug demo` draws the whole vocabulary on
your terminal, which is the fastest way to see what a resize does to it.
[`AGENTS.md`](./AGENTS.md) has the rules a caller has to meet — the
coprocess's lifetime, a background job's own write end, `sudo` on `/dev/tty`.

### When there is no binary

A script cannot always assume `snug` is on PATH — an older generation still on
someone's disk, a launchd job or `ssh mac …` off a thin PATH, a checkout on a
machine that never installed the layer, CI. `share/ui.sh` ships **inside the
same derivation**, beside `bin/snug`, and is the whole painter on those:

```sh
# from Nix, with no vendored copy and nothing to keep in step
wrapProgram $out/bin/yours --set MY_UI_SH ${snug}/share/ui.sh

# and in the script, degrading rather than assuming
[ -r "${MY_UI_SH:-}" ] && source "$MY_UI_SH"

ui_say "resolving inputs"          # → stderr, folded, hung at the gutter
ui_data "$path"                    # → stdout, untouched
ui_row run build "12s"; ui_paint   # → a live region, repainted in place

ui_col repo  6 1 subject right     # head, min, weight, role, cut side
ui_col where 10 2 path   left
ui_trow bench "$dir"
ui_table_data 3 1                  # → stdout, budgeted for stdout

ui_cell c warn "3 files"           # a role for ONE cell, where the column's
ui_trow haus "$c"                  # meaning changes row by row
```

Same roles, same glyphs, same tiers, same palette — its colour tables are
generated from the same `TOKENS` list as `palette.go`, and its *table* layout is
diffed against the binary's at every width by `TestBashTableMatchesGo`. The
region tiers are held by each half's own width sweeps, not across them. Lower
fidelity in one place only: it measures characters, not cells, so it is honest
about ordinary text and hands emoji to the binary. It is deliberately *not* a
wrapper around `snug` when snug is present — one fork per command is the whole
economy, and only the caller can see where a command begins.

## Streams

**Stdout carries data only**, because a caller does `cd "$(scruff child …)"`.
`Say`, `Warn` and `Fail` write to `Err`; `Data` and `PrintData` are the only
writers of `Out` — stderr and stdout as `NewPrinter` sets them.

A **report** is the thing the user ran the command for — `bench status`, the
`scruff` listing — so it is data, and `PrintData` puts it on stdout where
`| less` carries it whole. `Print` is the other half, on stderr with `Say` and
`Warn`, for a table that is part of what the tool is *saying*. Each measures the
stream it lands on, never the other: budget a report from stderr and a TTY
stdout beside a redirected stderr draws plain, while a piped stdout beside a
live stderr draws escapes into the pipe. `ui_table_data` and `ui_table` are the
same two halves.

## Colour

Callers name a **role**, never a colour. Nine of them:

| role | means | nebelung token |
| --- | --- | --- |
| `accent` | the tool speaking — `say`, section heads | `mauve` |
| `ok` | current, healthy, passed | `green` |
| `warn` | stale, wants attention, degraded | `peach` |
| `err` | failed, refused, missing | `red` |
| `muted` | secondary detail, durations, counts | `overlay1` |
| `subject` | the thing under discussion — repo, host, lane | `sapphire` |
| `path` | a filesystem path or a store path | `teal` |
| `field` | a key in a key/value grid | `subtext0` |
| `body` | ordinary text | *terminal default* |

`body` is deliberately unset rather than a token: painting ordinary prose fights
the user's own background.

A role belongs to a **column**, said once. The exception is a column whose
meaning changes row by row — a dirty count that is amber only when it is not
zero — so one cell may carry a role of its own (`snug.Cell`, `ui_cell`). It is
a role and never an escape: a caller that builds the row with the colour already
in it has put something in the cell that the padding then counts.

Roles resolve against [nebelung](https://github.com/hausfold/nebelung) and
degrade by what the terminal can carry: the exact hex on truecolor, the nearest
cube or ramp entry at 256, *declared names* at 16 (nearest-RGB on a pastel
palette lands `ok` and `warn` both on ANSI 7, the white slot), and at none the
glyph carries the meaning alone.

`NO_COLOR` is honoured, `CLICOLOR_FORCE` overrides it, `TERM=dumb` overrides
both, and a non-terminal is colourless unless forced. The glyph carries the
meaning; the colour is the courtesy.

## The marks

One glyph per role, an ASCII fallback when the locale isn't UTF-8, and **every
one of them one cell**:

| role | glyph | ascii |
| --- | --- | --- |
| `say` | `≋` | `~` |
| `ok` | `✓` | `+` |
| `warn` | `⚠` | `!` |
| `err` | `✗` | `x` |
| `info` | `ⓘ` | `i` |
| `skip` | `–` | `-` |
| `bullet` | `·` | `.` |
| `hint` | `↳` | `>` |
| `spin` | `⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏` | `\|/-\` |

Because they are all one cell, the gutter is **3 cells wide, always** — glyph
plus padding to 3 — so lines with different glyphs align and a folded line has a
fixed indent to hang from.

## Layout

- **Prose folds at `min(terminal width, 100)` cells.** Tables and live regions
  are exempt: they are bounded by their own content, and a job list that stopped
  at 100 in a 200-column window would be hiding the room it had.
- **Every line fits or is folded — never soft-wrapped.** A tool that lets the
  terminal wrap has given up its own indentation. Folds land on a word boundary
  and hang at the gutter.
- **Columns are budgeted, not declared.** Each gets a *weight* and a *minimum*.
  If the natural widths fit, they are used; otherwise every column drops to its
  minimum and the remainder is shared **by weight**, repeatedly, because a
  column that reaches its natural width releases its share back. Weight decides,
  not width — "the widest gives up first" is the usual outcome, not the rule.
- **Truncation is by priority, with `…` inside the field.** A name cuts from the
  right, a path from the left (`…/scruff/internal/ui`), a duration never.
- **Below the sum of the minimums a table stops budgeting and stacks** — one
  label/value pair per line, the label truncated too, rather than emit a row it
  knows will wrap.
- **The floor is 2 cells**: one glyph. At 1 there is nothing honest left to draw.

## A live region

A block of lines rewritten in place — a job list, a phase list, a counter. It
has three tiers of its own, widest first, each giving up the least useful thing
left:

| tier | keeps |
| --- | --- |
| `table` | padded name column, aligned detail |
| `list` | name only — the detail goes, and so does the padding |
| `bare` | the 3-cell gutter collapses to one space |

The contract, which `Region` and `ui_paint` both hold:

1. **Only on a TTY.** Piped, in CI or under `bats` it degrades to one plain line
   per *state change*. No cursor escape ever reaches a file.
2. **Motion is not gated on `NO_COLOR`.** A spinner on a colourless terminal is
   still the thing you want to see.
3. **Repaint counts screen lines, not logical rows** — equal by construction.
4. **`SIGWINCH` re-measures and repaints from scratch**, clearing to end of
   screen rather than trusting the old height.
5. **The cursor is restored on every exit path**, `SIGINT` and a `set -e` abort
   included.
6. **Frame rate and poll rate stay unrelated.** Never fetch-paint-sleep.
7. **Scrollback is append-only.** A finished region leaves its last frame there
   and moves on.

### The record protocol

What a shell writes to `snug run`: tab-separated, one per line, verb first —
`say<TAB>text` and its siblings, `data<TAB>text`,
`row<TAB>state<TAB>name<TAB>detail`, then `paint`, `clear`, `frame<TAB>n` (the
counter the next `paint` advances from) and `end`. `snug --help` is the list,
with the six row states.

A space after the verb does not parse; `run` splits on tabs and answers `unknown
record`. A row never carries an empty field between two non-empty ones, because
`read` collapses consecutive delimiters — only the trailing field may be empty.
Multi-line text is one record per line; the emitter folds the newlines.

## What it is not

Not a TUI framework. There is no alt-screen, no event loop, no widget tree —
`snug` is a *filter the caller drives*, because `bench` and `haus` are bash and
will stay bash. The family's look is quiet: aligned text and a fog palette, no
borders and no boxes.

**This repo is the standard**, not one implementation of one kept elsewhere.
Everything above is the contract every hausfold CLI is held to, and
[`AGENTS.md`](./AGENTS.md) is the other half of it: what a caller must do to
meet the contract, and what breaks when it doesn't. A rule missing from the two
of them is missing everywhere.

## Taking it

As a **Go package**, nothing here is involved — `go get github.com/hausfold/snug`
and import it.

As a **binary**, this repo is a flake. Add it as an input and take
`overlays.default`, which puts `snug` in `pkgs`:

```nix
inputs.snug = {
  url = "github:hausfold/snug";
  # Not optional in practice: the overlay hands back a package built from
  # snug's OWN nixpkgs, so without this you realise a second nixpkgs and a
  # second Go toolchain for one small binary.
  inputs.nixpkgs.follows = "nixpkgs";
};
```

`nix run github:hausfold/snug -- demo` needs none of that, and is the fastest
way to see what it draws.

## License

MIT.
