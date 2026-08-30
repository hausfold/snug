package snug

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// Printer is one tool's voice. Take one at startup and keep it.
//
// The stream rule is a family contract, not a style choice: STDOUT CARRIES DATA
// ONLY. Every diagnostic, prompt and progress line goes to stderr, because
// callers do `cd "$(scruff child …)"` and hooks read paths off stdout. Say/Warn/
// Fail therefore write to Err; Data and PrintData are the only things that
// write to Out.
//
// So a printer measures BOTH streams, separately, and that is not symmetry for
// its own sake. A report drawn on stdout has to be budgeted, gated and painted
// for stdout: ask stderr how wide the window is and a TTY stdout beside a
// redirected stderr draws plain, while a piped stdout beside a live stderr
// draws escapes into the pipe. One stream, one question, one answer.
type Printer struct {
	Out io.Writer // data — a path, JSON, the report a caller captures
	Err io.Writer // everything a human reads

	mu       sync.Mutex
	term     Term // measured from Err — the narration stream
	theme    *Theme
	outTerm  Term // measured from Out — the stream a REPORT lands on
	outTheme *Theme
	live     *Region
}

// NewPrinter measures the terminal from Err — the stream the human is reading —
// and builds a printer for it.
func NewPrinter() *Printer { return NewPrinterOn(os.Stdout, os.Stderr) }

// NewPrinterOn is NewPrinter with the streams given, for tests and for callers
// that have already redirected.
func NewPrinterOn(out, errw io.Writer) *Printer {
	et, ot := measure(errw), measure(out)
	return &Printer{
		Out: out, Err: errw,
		term: et, theme: NewTheme(et),
		outTerm: ot, outTheme: NewTheme(ot),
	}
}

// measure reads one stream's window, or answers for a stream that has none.
//
// The zero IsTTY is the load-bearing half: everything downstream that asks
// "should this be folded, truncated or painted?" keys on it, and a writer that
// is not a file has no window by construction.
func measure(w io.Writer) Term {
	if f, ok := w.(*os.File); ok {
		return DetectTerm(f)
	}
	return Term{Width: 80, Height: 24, Profile: NoColor}
}

// Term is the current snapshot of the NARRATION stream. Cheap; it is not
// re-measured per call.
func (p *Printer) Term() Term { return p.term }

// OutTerm is the same snapshot for the DATA stream.
//
// It is exported because Term answers about Err, so a caller composing a report
// on stdout that asked Term how wide it was would be told about the other
// stream — correct exactly as often as the two happen to be the same terminal,
// and silently wrong the rest of the time.
func (p *Printer) OutTerm() Term { return p.outTerm }

// Theme is the resolved palette of the NARRATION stream, for callers composing
// their own lines.
func (p *Printer) Theme() *Theme { return p.theme }

// OutTheme is the same palette for the DATA stream, and it is exported for the
// same reason OutTerm is — more urgently, because getting it wrong is visible in
// the output rather than only in the layout. A caller composing a report by hand
// from OutTerm's geometry and Theme's colour writes escapes into a pipe the
// moment stderr is a terminal and stdout is not, which is the exact failure
// PrintData exists to make unrepresentable. Geometry and palette come from the
// same stream or from neither.
func (p *Printer) OutTheme() *Theme { return p.outTheme }

// Resize re-measures the window. Call it from a SIGWINCH handler; Live does it
// for you while a region is open.
func (p *Printer) Resize() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if f, ok := p.Err.(*os.File); ok {
		p.term = DetectTerm(f)
		p.theme = NewTheme(p.term)
	}
	// Both, because SIGWINCH is about the WINDOW and a report on stdout is
	// drawn in it too. Re-measuring only the stream that happens to be narrating
	// is how a resized table keeps the width it had at startup.
	if f, ok := p.Out.(*os.File); ok {
		p.outTerm = DetectTerm(f)
		p.outTheme = NewTheme(p.outTerm)
	}
}

// Say is the tool speaking.
func (p *Printer) Say(format string, a ...any) { p.line(MarkSay, Accent, format, a...) }

// OK marks something current, healthy or passed.
func (p *Printer) OK(format string, a ...any) { p.line(MarkOK, OK, format, a...) }

// Warn marks something stale or degraded.
func (p *Printer) Warn(format string, a ...any) { p.line(MarkWarn, Warn, format, a...) }

// Fail marks a failure. It does not exit: main owns that, so every path can
// return an error carrying its own exit code.
func (p *Printer) Fail(format string, a ...any) { p.line(MarkErr, Err, format, a...) }

// Info is a secondary note under something else.
func (p *Printer) Info(format string, a ...any) { p.line(MarkInfo, Muted, format, a...) }

// Hint is what to run next, printed under the thing that wants it.
func (p *Printer) Hint(format string, a ...any) { p.line(MarkHint, Muted, format, a...) }

// Data writes to stdout, unfolded and unpainted. Reserve it for what a caller
// captures — a path, a JSON document, a rev. For a report with columns, reach
// for PrintData instead: it lands on the same stream and gets a budget.
func (p *Printer) Data(format string, a ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintf(p.Out, format, a...)
}

// line is the one place a human-facing line is built.
//
// Folding, not wrapping: the text is broken at the prose cap and every
// continuation hangs at the gutter, so a long sentence reads as one indented
// block instead of running back under the mark.
func (p *Printer) line(m Mark, r Role, format string, a ...any) {
	msg := format
	if len(a) > 0 {
		msg = fmt.Sprintf(format, a...)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.writeLines(p.render(m, r, msg))
}

func (p *Printer) render(m Mark, r Role, msg string) []string {
	// The gutter is three cells whatever the mark is, so a ✓ line and a 🌫 line
	// start their text in the same column.
	//
	// Under four columns it cannot be: three cells of gutter plus one of text is
	// already past the edge. So it collapses to the mark and a space, and then
	// to the mark alone — the same floor the live region drops to, for the same
	// reason. Clamping the text budget up to 1 while the gutter stayed at 3 is
	// how a two-column window got a four-cell line.
	prose := p.term.Prose()
	mark, gw := p.term.Glyph(m)
	switch {
	case prose >= Gutter+1:
	case prose >= 3:
		gw, mark = 2, p.term.GlyphBare(m)+" "
	default:
		gw, mark = 1, p.term.GlyphBare(m)
	}
	painted := p.theme.Paint(r, mark)
	budget := prose - gw
	if budget < 1 {
		// One cell, and a mark to put in it. The mark is what survives; below
		// one cell there is nothing honest left to draw.
		return []string{painted}
	}
	folded := Fold(msg, budget)
	out := make([]string, 0, len(folded))
	pad := strings.Repeat(" ", gw)
	for i, l := range folded {
		head := pad
		if i == 0 {
			head = painted
		}
		out = append(out, head+l)
	}
	return out
}

// writeLines sends finished lines out, cooperating with an open live region so
// a log line scrolls ABOVE a spinner that is still turning — the thing a shell
// painter cannot do at all.
func (p *Printer) writeLines(lines []string) {
	if p.live != nil {
		p.live.above(lines)
		return
	}
	for _, l := range lines {
		fmt.Fprintln(p.Err, l)
	}
}
