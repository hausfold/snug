// Command snug draws the hausfold family's terminal output for callers that
// cannot import the library — which means every shell script we ship.
//
// Two shapes, and the difference matters:
//
//	snug say "resolving inputs"        one line, one fork (~4ms)
//	snug run < protocol                one fork for a WHOLE command
//
// Use `run` for anything with a live region or more than a handful of lines. A
// fork costs about four milliseconds; a fork per line would put a third of a
// second of pure overhead into a sixty-line `haus rebuild`. `run` reads records
// off stdin for the life of the command, so the process count is one and the
// spinner keeps turning while log lines scroll above it.
//
// Streams follow the family contract: stdout carries DATA only, everything a
// human reads goes to stderr. That is why `run` works as a bash coproc —
// bash pipes stdin and stdout and leaves stderr on the terminal, which is
// exactly the split this wants.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/hausfold/snug"
)

const usage = `snug — how the hausfold family puts a line on screen

  snug say|ok|warn|fail|info|hint <text>   one line, one fork
  snug run                                 read records on stdin, one fork per command
  snug width                               the window's width in cells, measured
  snug caps                                what snug detected about this terminal
  snug demo                                every mark, role and tier, on this terminal

records for ` + "`run`" + `, tab-separated, one per line:

  say|ok|warn|fail|info|hint <text>        a line
  data <text>                              to stdout, unpainted
  row <state> <name> [detail]              buffer a live row; state is one of
                                           run wait ok warn fail skip
  paint                                    repaint the live region
  clear                                    empty it
  end                                      close it and restore the cursor
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	p := snug.NewPrinter()
	arg := strings.Join(os.Args[2:], " ")

	switch os.Args[1] {
	case "say", "ok", "warn", "fail", "info", "hint":
		emit(p, os.Args[1], arg)
	case "data":
		p.Data("%s\n", arg)
	case "run":
		os.Exit(run(p))
	case "width":
		fmt.Println(p.Term().Width)
	case "caps":
		caps(p)
	case "demo":
		demo(p)
	case "-h", "--help", "help":
		fmt.Fprint(os.Stderr, usage)
	default:
		p.Fail("no such verb: %s", os.Args[1])
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
}

func emit(p *snug.Printer, verb, text string) {
	switch verb {
	case "say":
		p.Say("%s", text)
	case "ok":
		p.OK("%s", text)
	case "warn":
		p.Warn("%s", text)
	case "fail":
		p.Fail("%s", text)
	case "info":
		p.Info("%s", text)
	case "hint":
		p.Hint("%s", text)
	}
}

// run is the coprocess loop.
//
// The frame counter lives here rather than in the caller: a shell driving this
// at ten frames a second should send `paint` and nothing else, and never have
// to know which spinner glyph comes next.
func run(p *snug.Printer) int {
	var (
		region *snug.Region
		rows   []snug.Row
		frame  int
	)
	defer func() {
		if region != nil {
			region.Close()
		}
	}()

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		f := strings.Split(sc.Text(), "\t")
		verb := f[0]
		rest := func(n int) string {
			if len(f) > n {
				return f[n]
			}
			return ""
		}
		switch verb {
		case "":
			continue
		case "say", "ok", "warn", "fail", "info", "hint":
			emit(p, verb, strings.Join(f[1:], "\t"))
		case "data":
			p.Data("%s\n", strings.Join(f[1:], "\t"))
		case "row":
			rows = append(rows, row(rest(1), rest(2), rest(3), frame))
		case "clear":
			rows = nil
		case "paint":
			if region == nil {
				region = p.Live()
			}
			frame++
			for i := range rows {
				rows[i].Frame = frame
			}
			region.Set(rows)
			rows = nil
		case "end":
			if region != nil {
				region.Close()
				region = nil
			}
		case "frame":
			// Explicit frame number, for a caller keeping its own clock.
			if n, err := strconv.Atoi(rest(1)); err == nil {
				frame = n
			}
		default:
			p.Warn("snug: unknown record %q", verb)
		}
	}
	return 0
}

func row(state, name, detail string, frame int) snug.Row {
	r := snug.Row{Name: name, Detail: detail, Frame: frame}
	switch state {
	case "run":
		r.Spin = true
	case "ok":
		r.Mark = snug.MarkOK
	case "warn":
		r.Mark = snug.MarkWarn
	case "fail":
		r.Mark = snug.MarkErr
	case "skip":
		r.Mark = snug.MarkSkip
	default: // wait, and anything we don't know
		r.Mark = snug.MarkBullet
	}
	return r
}

func caps(p *snug.Printer) {
	t := p.Term()
	prof := map[snug.Profile]string{
		snug.NoColor: "none", snug.ANSI16: "16", snug.ANSI256: "256", snug.TrueColor: "truecolor",
	}[t.Profile]
	variant := map[snug.Variant]string{
		snug.Nebelung: "nebelung", snug.NebelungHighContrast: "nebelung-high-contrast",
		snug.NebelungLatte: "nebelung-latte", snug.NebelungLatteHC: "nebelung-latte-high-contrast",
	}[t.Variant]
	p.Print(snug.Table{
		Indent: 3,
		Cols: []snug.Col{
			{Head: "", Min: 7, Weight: 1, Role: snug.Field},
			{Head: "", Min: 6, Weight: 3, Role: snug.Subject},
		},
		Rows: [][]string{
			{"width", fmt.Sprintf("%d cells (prose caps at %d)", t.Width, t.Prose())},
			{"tty", fmt.Sprintf("%v", t.IsTTY)},
			{"colour", prof},
			{"variant", variant},
			{"alphabet", alphabet(t)},
		},
	})
}

func alphabet(t snug.Term) string {
	g, _ := t.Glyph(snug.MarkSay)
	if strings.HasPrefix(g, "~") {
		return "ascii (locale is not UTF-8, or SNUG_ASCII is set)"
	}
	return "utf-8"
}

func demo(p *snug.Printer) {
	p.Say("snug — every mark and role, on this terminal")
	p.OK("ok · something current, healthy, passed")
	p.Warn("warn · something stale, degraded, wanting attention")
	p.Fail("fail · something that failed, was refused, or is missing")
	p.Info("info · a secondary note under the thing it belongs to")
	p.Hint("hint · what to run next")
	p.Say("a long line, to show that folding hangs its continuations at the gutter " +
		"instead of letting the terminal soft-wrap them back underneath the mark, " +
		"which is what every one of our CLIs did before this existed")
	p.Print(snug.Table{
		Indent: 3,
		Cols: []snug.Col{
			{Head: "repo", Min: 6, Weight: 2, Role: snug.Subject},
			{Head: "state", Min: 5, Weight: 1, Role: snug.OK},
			{Head: "path", Min: 10, Weight: 4, Role: snug.Path, Cut: snug.CutLeft},
			{Head: "age", Min: 3, Weight: 1, Role: snug.Muted, Cut: snug.CutNever},
		},
		Rows: [][]string{
			{"haus", "current", "/Users/you/code/workshop/haus/modules/core/haus.sh", "2h"},
			{"holt", "stale", "/Users/you/code/workshop/holt/internal/ui/ui.go", "3d"},
			{"nebelung", "current", "/Users/you/code/workshop/nebelung/palette/nebelung.json", "11m"},
		},
	})
	p.Info("narrow the window and run it again — the table sheds columns, then stacks")
}
