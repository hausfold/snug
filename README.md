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
🌫  snug — every mark and role, on this terminal
✓   ok · something current, healthy, passed
⚠   warn · something stale, degraded, wanting attention
✗   fail · something that failed, was refused, or is missing
    haus     current …/modules/core/haus.sh 2h
    holt     stale   …/internal/ui/ui.go    3d
    nebelung current …/nebelung.json        11m
```

Glyph widths are **declared**, not measured. 🌫 (U+1F32B) has
`Emoji_Presentation = No`, so every source disagrees about it — `x/ansi` and
`runewidth` say one cell, they contradict each other on the variation-selector
form, and folklore says two. None of them is a terminal. The table records what
a terminal actually draws.

Narrow the window and the table sheds its detail column, then its padding, then
stacks into label/value pairs. It never emits a row it knows will wrap.

## For Go

```go
p := snug.NewPrinter()
p.Say("resolving %d inputs", n)
p.OK("nebelung is current")

r := p.Live()
defer r.Close()
for {
    r.Set(rows)     // repaints in place; SIGWINCH is handled
}
```

## For shells

```sh
snug say "resolving inputs"          # one line, one fork (~4 ms)

coproc SNUG { snug run; }            # one fork for a WHOLE command
printf 'row\trun\tpublish\t\npaint\n' >&${SNUG[1]}
```

Use `run` for anything with a live region or more than a handful of lines. A
fork per line would put a third of a second of pure overhead into a sixty-line
`haus rebuild`. `run` also lets log lines scroll *above* a spinner that is still
turning — which a shell painter cannot do at all.

`snug caps` reports what it detected; `snug demo` draws the whole vocabulary on
your terminal, which is the fastest way to see what a resize does to it.

## Colour

Callers name a **role** — `accent`, `ok`, `warn`, `err`, `muted`, `subject`,
`path`, `field` — never a colour. Roles resolve against
[nebelung](https://github.com/hausfold/nebelung) and degrade by what the
terminal can carry: the exact hex on truecolor, the nearest cube or ramp entry
at 256, and *declared names* at 16, because nearest-RGB on a pastel palette puts
`ok` and `warn` both on white.

`NO_COLOR` is honoured, `CLICOLOR_FORCE` overrides it, `TERM=dumb` overrides
both, and a non-terminal is colourless unless forced. The glyph carries the
meaning; the colour is the courtesy.

## What it is not

Not a TUI framework. There is no alt-screen, no event loop, no widget tree —
`snug` is a *filter the caller drives*, because `bench` and `haus` are bash and
will stay bash. The family's look is quiet: aligned text and a fog palette, no
borders and no boxes.

The standard it implements is
[`docs/cli-presentation.md`](https://github.com/hausfold/workshop/blob/main/docs/cli-presentation.md)
in the workshop.

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

Apache 2.0.
