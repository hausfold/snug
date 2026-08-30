package snug

import (
	"fmt"
	"strings"
)

// Side is which end of a value a column gives up when it has to.
type Side int

const (
	CutRight Side = iota // a name: keep the front — `bump-flake-and-…`
	CutLeft              // a path: keep the tail — `…/internal/ui`
	CutNever             // a duration or a count: it is wrong or it is absent
)

// Col describes one column's appetite.
//
// Min is the width below which the column stops being worth showing; when the
// minimums no longer fit, the whole table drops to stacked key/value rather
// than emitting a row it knows will wrap. Weight shares out whatever is left
// over once every column has its minimum.
type Col struct {
	Head   string
	Min    int
	Weight int
	Role   Role
	Cut    Side
}

// Table is a set of columns budgeted against the window, not declared in a
// format string. `%-38s` is a width the terminal never agreed to.
type Table struct {
	Cols   []Col
	Rows   [][]string
	Indent int  // cells before the first column; 3 matches the family gutter
	Header bool // draw the column names as a first row
}

// Head doubles as the column's LABEL: it is what the stacked fallback prints
// beside each value when the window is too narrow for any table at all. So give
// every column a Head even when Header is false, which is the family default —
// our tables are read by shape, and a header row on three rows of data is
// furniture.

const colGap = 1 // one space between columns; two reads as a gutter, not a gap

// Render lays the table out for a terminal and returns finished lines.
func (t Table) Render(term Term, th *Theme) []string {
	if len(t.Cols) == 0 || len(t.Rows) == 0 {
		return nil
	}
	indent := t.Indent
	avail := term.Avail() - indent
	gaps := colGap * (len(t.Cols) - 1)

	natural := make([]int, len(t.Cols))
	for i, c := range t.Cols {
		natural[i] = Width(c.Head)
	}
	for _, row := range t.Rows {
		for i := range t.Cols {
			if i < len(row) {
				if w := Width(row[i]); w > natural[i] {
					natural[i] = w
				}
			}
		}
	}

	widths, ok := t.budget(natural, avail-gaps)
	if !ok {
		return t.stack(term, th)
	}
	pad := strings.Repeat(" ", indent)
	rows := t.Rows
	if t.Header {
		head := make([]string, len(t.Cols))
		for i, c := range t.Cols {
			head[i] = c.Head
		}
		rows = append([][]string{head}, rows...)
	}
	out := make([]string, 0, len(rows))
	for n, row := range rows {
		cells := make([]string, 0, len(t.Cols))
		for i, c := range t.Cols {
			v := ""
			if i < len(row) {
				v = row[i]
			}
			v = cut(v, widths[i], c.Cut, term.Ellipsis())
			if i < len(t.Cols)-1 {
				v = Pad(v, widths[i]) // never pad the last column: trailing
			} //                        spaces are what wrap a row that just fit
			role := c.Role
			if t.Header && n == 0 {
				role = Field
			}
			cells = append(cells, th.Paint(role, v))
		}
		out = append(out, strings.TrimRight(pad+strings.Join(cells, strings.Repeat(" ", colGap)), " "))
	}
	return out
}

// budget hands out `avail` cells. It reports false when even the minimums do
// not fit, which is the caller's signal to stop drawing a table at all.
func (t Table) budget(natural []int, avail int) ([]int, bool) {
	sum := 0
	for _, n := range natural {
		sum += n
	}
	if sum <= avail {
		return natural, true // everything fits at its natural width
	}

	widths := make([]int, len(t.Cols))
	floor := 0
	for i, c := range t.Cols {
		m := c.Min
		if m > natural[i] {
			m = natural[i] // never reserve more than the column will ever use
		}
		if c.Cut == CutNever {
			m = natural[i] // a duration is never abbreviated; it is dropped whole
		}
		widths[i], floor = m, floor+m
	}
	if floor > avail {
		return nil, false
	}

	// Share the surplus by weight, but never past what a column can use. Repeat
	// until nothing moves: a column hitting its natural width releases its share
	// back to the others, which is the difference between one wide column
	// hogging the window and every column being readable.
	surplus := avail - floor
	for surplus > 0 {
		totalWeight := 0
		for i, c := range t.Cols {
			if widths[i] < natural[i] {
				w := c.Weight
				if w < 1 {
					w = 1
				}
				totalWeight += w
			}
		}
		if totalWeight == 0 {
			break
		}
		moved := 0
		for i, c := range t.Cols {
			if widths[i] >= natural[i] {
				continue
			}
			w := c.Weight
			if w < 1 {
				w = 1
			}
			give := surplus * w / totalWeight
			if give < 1 {
				give = 1
			}
			if widths[i]+give > natural[i] {
				give = natural[i] - widths[i]
			}
			if give > surplus-moved {
				give = surplus - moved
			}
			widths[i] += give
			moved += give
			if moved == surplus {
				break
			}
		}
		if moved == 0 {
			break
		}
		surplus -= moved
	}
	return widths, true
}

// stack is the fallback below the sum of the minimums: one label/value pair per
// line, which has no alignment to shear.
//
// It is bound by the same rule as everything else — nothing wider than the
// window — which is why the label is truncated too. A fallback that overflows
// is not a fallback.
func (t Table) stack(term Term, th *Theme) []string {
	avail := term.Avail()
	var out []string
	for n, row := range t.Rows {
		if n > 0 {
			out = append(out, "")
		}
		for i, c := range t.Cols {
			if i >= len(row) || row[i] == "" {
				continue
			}
			label := Truncate(c.Head+" ", avail, "")
			budget := avail - Width(label)
			if budget < 1 {
				// No room for a value beside its label. Give the label its own
				// line and let the value fold under it.
				out = append(out, th.Paint(Field, label))
				for _, l := range Fold(row[i], max(avail, 1)) {
					out = append(out, th.Paint(c.Role, l))
				}
				continue
			}
			// A column that declared which END matters keeps that promise here
			// too: folding a path breaks it mid-component (`…nebelun` / `g/…`),
			// which reads as a directory that does not exist. One left-cut line
			// says more than three ragged ones.
			if c.Cut == CutLeft {
				out = append(out, th.Paint(Field, label)+
					th.Paint(c.Role, TruncateLeft(row[i], budget, term.Ellipsis())))
				continue
			}
			pad := strings.Repeat(" ", Width(label))
			for j, l := range Fold(row[i], budget) {
				if j == 0 {
					out = append(out, th.Paint(Field, label)+th.Paint(c.Role, l))
				} else {
					out = append(out, pad+th.Paint(c.Role, l))
				}
			}
		}
	}
	return out
}

func cut(s string, w int, side Side, tail string) string {
	switch side {
	case CutLeft:
		return TruncateLeft(s, w, tail)
	case CutNever:
		if Width(s) > w {
			return "" // it is right or it is absent
		}
		return s
	default:
		return Truncate(s, w, tail)
	}
}

// Print renders the table onto the printer's Err stream — the human one, beside
// Say and Warn. Use it for a table that is part of what the tool is SAYING, and
// it will scroll above an open live region like any other line.
func (p *Printer) Print(t Table) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.writeLines(t.Render(p.term, p.theme))
}

// PrintData renders the table onto the printer's Out stream, budgeted, gated
// and painted for THAT stream.
//
// A report is not a diagnostic. The `scruff` listing and `bench status`'s tables
// are the thing the user ran the command for rather than the tool talking about
// it, and the family keeps them on fd 1 so `bench status | less` carries them
// whole while the narration stays on fd 2. Print would put them on the wrong
// side of that split.
//
// Measuring Out is the whole reason this is a method and not four lines in each
// caller. `bench` had to carry two gates asking about two streams to get this
// right in bash, and wrote down the one edge it still lost: a TTY stdout with a
// redirected stderr drew plain. Here the report asks its own stream.
//
// Close any live region first. Like Data, and unlike Print, this does not
// cooperate with one — and when Out and Err are the same terminal it is
// destructive rather than merely uncooperative: the region's next repaint walks
// up `painted` lines and clears downward from a cursor the report has since
// moved, so it erases the report the user actually ran the command for.
//
// That is not an oversight to fix later. The region's arithmetic counts the
// lines IT wrote to Err, and no arithmetic makes a foreign write to another
// descriptor land where a repaint expects to find its own. A command draws a
// region or a report, not both; `p.CloseLive()` is how it stops drawing one.
func (p *Printer) PrintData(t Table) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, l := range t.Render(p.outTerm, p.outTheme) {
		fmt.Fprintln(p.Out, l)
	}
}
