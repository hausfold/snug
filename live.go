package snug

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

// Row is one line of a live region. Detail is the right-hand column — a
// duration, a count, a state — and is the first thing given up when the window
// gets narrow, because the name is the load-bearing half.
type Row struct {
	Mark   Mark
	Frame  int  // spinner frame, used when Mark is MarkNone and Spin is true
	Spin   bool // draw a spinner instead of a fixed mark
	Name   string
	Detail string
}

// Region is a block of lines rewritten in place — a job list, a phase list, a
// counter. Exactly one may be open per Printer.
//
// The contract, in one sentence: the repaint may only move the cursor up by the
// number of SCREEN lines it used, so every row is folded to fit and printed
// rows equal screen lines by construction. Everything else here follows from
// that.
type Region struct {
	p       *Printer
	painted int
	dirty   bool // a resize happened; drop everything and repaint fresh
	rows    []Row
	winch   chan os.Signal
	closed  bool
	seen    map[string]string // not-a-terminal path: last state printed per row
}

// Live opens a live region on the printer's Err stream.
//
// On a stream that is not a terminal it returns a region that degrades to one
// plain line per STATE CHANGE: no cursor escape ever reaches a file, a pipe or a
// CI log. Motion, unlike colour, is not gated on NO_COLOR — a spinner on a
// colourless terminal is still the thing you want to see.
func (p *Printer) Live() *Region {
	p.mu.Lock()
	defer p.mu.Unlock()
	r := &Region{p: p}
	if p.term.IsTTY {
		r.winch = make(chan os.Signal, 1)
		signal.Notify(r.winch, syscall.SIGWINCH)
		go r.watch()
		fmt.Fprint(p.Err, "\x1b[?25l") // hide the cursor while we repaint
	}
	p.live = r
	return r
}

func (r *Region) watch() {
	for range r.winch {
		r.p.Resize()
		r.p.mu.Lock()
		r.dirty = true
		r.p.mu.Unlock()
	}
}

// Set replaces the region's contents and repaints.
func (r *Region) Set(rows []Row) {
	r.p.mu.Lock()
	defer r.p.mu.Unlock()
	// A closed region is done: the cursor is back and the final frame is in
	// scrollback, so a repaint now would write cursor-up and rows AFTER the
	// `?25h` that ended it. Only reachable since the binary grew a signal
	// handler — that handler closes from another goroutine while this one may
	// be between frames — but it is the caller's contract either way.
	if r.closed {
		return
	}
	r.rows = rows
	r.paint()
}

// Close restores the cursor and leaves the final frame in scrollback.
//
// Safe to call twice, and safe to call from a deferred function on any exit
// path — a terminal left with no cursor is the worst thing a live region can do
// to you, so this must run even when the process is dying.
func (r *Region) Close() {
	r.p.mu.Lock()
	defer r.p.mu.Unlock()
	if r.closed {
		return
	}
	r.closed = true
	if r.winch != nil {
		signal.Stop(r.winch)
		close(r.winch)
		fmt.Fprint(r.p.Err, "\x1b[?25h")
	}
	r.p.live = nil
}

// CloseLive closes whatever region is open on this printer, if any.
//
// It exists for the one caller that cannot hold the *Region: a signal handler.
// A process killed mid-frame has hidden the cursor and not put it back, which
// the standard (the workshop's docs/cli-presentation.md, "Live regions") makes
// a hard requirement precisely because a terminal left with no cursor is the
// worst thing a spinner can do to you — and Go's default SIGINT disposition
// runs no defer at all.
//
// The library deliberately does NOT install that handler itself: a Go program
// importing snug owns its own signal policy, and a package that quietly takes
// SIGINT is a package you have to fight. `snug run` — a process whose whole job
// is drawing — installs it, and this is what it calls.
func (p *Printer) CloseLive() {
	p.mu.Lock()
	r := p.live
	p.mu.Unlock()
	if r != nil {
		r.Close()
	}
}

// above prints scrolling log lines over a region that is still turning. The
// region is erased, the lines are written, and the region repaints underneath —
// so output and animation share the screen instead of fighting for it.
func (r *Region) above(lines []string) {
	if !r.p.term.IsTTY {
		for _, l := range lines {
			fmt.Fprintln(r.p.Err, l)
		}
		return
	}
	r.rewind()
	for _, l := range lines {
		fmt.Fprintln(r.p.Err, l)
	}
	r.paint()
}

// rewind puts the cursor back at the top of the block and clears it.
func (r *Region) rewind() {
	if r.painted > 0 {
		fmt.Fprintf(r.p.Err, "\x1b[%dA", r.painted)
	}
	fmt.Fprint(r.p.Err, "\r\x1b[J")
	r.painted = 0
}

// paint draws every row, folded to fit. Caller holds the lock.
func (r *Region) paint() {
	if !r.p.term.IsTTY {
		r.paintPlain()
		return
	}
	if r.dirty {
		// A resize reflows whatever is already on screen in ways that cannot be
		// modelled, so don't try: drop to column 0, wipe below, and paint fresh.
		// `[J` clears DOWNWARD, so the pre-resize block stays where it is rather
		// than being taken out of real scrollback.
		fmt.Fprint(r.p.Err, "\r\x1b[J")
		r.painted, r.dirty = 0, false
	} else if r.painted > 0 {
		fmt.Fprintf(r.p.Err, "\x1b[%dA", r.painted)
	}

	for _, l := range r.layout() {
		fmt.Fprint(r.p.Err, "\x1b[2K"+l+"\n")
	}
	r.painted = len(r.rows)
	// Wipe anything a taller previous frame left below: a list can shrink when a
	// snapshot arrives late, or when a narrower window drops a row.
	fmt.Fprint(r.p.Err, "\x1b[J")
}

// paintPlain is the not-a-terminal path: one line per state change, no escapes.
func (r *Region) paintPlain() {
	for _, row := range r.rows {
		key := row.Name
		if r.seen == nil {
			r.seen = map[string]string{}
		}
		state := fmt.Sprintf("%d/%v", row.Mark, row.Spin)
		if r.seen[key] == state {
			continue
		}
		r.seen[key] = state
		mark, _ := r.p.term.Glyph(row.Mark)
		fmt.Fprintf(r.p.Err, "   %s %s (%s)\n", strings.TrimSpace(mark), row.Name, row.Detail)
	}
}

// layout budgets the window across the two columns and renders every row.
//
// Three tiers, widest first, each giving up the least useful thing left:
//
//	table — name column padded, detail aligned beside it
//	list  — name only; the detail goes, and so does the padding
//	bare  — the gutter collapses to a single space
//
// The tier is chosen from the WINDOW, then the column is clamped to the
// CONTENT — never the other way round. Clamping first and testing the clamped
// value asks "is the longest name short?" when it means "is there room?", and a
// CI run of build / test / lint drops its durations on a 200-column terminal.
func (r *Region) layout() []string {
	t := r.p.term
	th := r.p.theme
	avail := t.Avail()

	// The detail column is MEASURED, never assumed. Budgeting seven cells
	// because "12m 34s" is the longest duration puts "100m 05s" into the last
	// column and soft-wraps the row — the bug this whole type exists to stop.
	widest, detailw := 0, 0
	for _, row := range r.rows {
		if w := Width(row.Name); w > widest {
			widest = w
		}
		if w := Width(row.Detail); w > detailw {
			detailw = w
		}
	}
	if widest > 34 {
		widest = 34 // one verbose name can't push every detail off to the right
	}

	gut, detailed := Gutter+3, true // Gutter plus the leading indent
	namew := avail - gut - detailw - 1
	if namew >= 8 && detailw > 0 {
		if namew > widest {
			namew = widest
		}
	} else {
		detailed = false
		if avail < 12 {
			gut = 2 // bare: no indent, one space
		}
		namew = avail - gut
		if namew > widest {
			namew = widest
		}
		if namew < 1 {
			namew = 0 // no room for a name; the glyph alone is still true
		}
	}

	// The two narrow tiers take the mark UNPADDED. Their gutter is a single
	// space, and two carried cells of padding is two cells past the edge — see
	// GlyphBare for why trimming it back off afterwards does not work.
	narrow := namew == 0 || gut == 2

	out := make([]string, 0, len(r.rows))
	for _, row := range r.rows {
		var mark string
		if row.Spin {
			g := t.Spin(row.Frame)
			if !narrow {
				g = Pad(g, Gutter)
			}
			mark = th.Paint(Accent, g)
		} else {
			g := t.GlyphBare(row.Mark)
			if !narrow {
				g, _ = t.Glyph(row.Mark)
			}
			mark = th.Paint(markRole(row.Mark), g)
		}
		name := Truncate(row.Name, namew, t.Ellipsis())

		switch {
		case namew == 0:
			out = append(out, mark)
		case gut == 2:
			out = append(out, mark+" "+th.Paint(Subject, name))
		case detailed:
			out = append(out, "   "+mark+th.Paint(Subject, Pad(name, namew))+
				" "+th.Paint(Muted, row.Detail))
		default:
			out = append(out, "   "+mark+th.Paint(Subject, name))
		}
	}
	return out
}

func markRole(m Mark) Role {
	switch m {
	case MarkOK:
		return OK
	case MarkWarn:
		return Warn
	case MarkErr:
		return Err
	case MarkSay:
		return Accent
	default:
		return Muted
	}
}
