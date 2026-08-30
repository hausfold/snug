package snug

import (
	"bytes"
	"strings"
	"testing"
)

func demoTable() Table {
	return Table{
		Indent: 3,
		Cols: []Col{
			{Head: "repo", Min: 6, Weight: 2, Role: Subject},
			{Head: "state", Min: 5, Weight: 1, Role: OK},
			{Head: "path", Min: 10, Weight: 4, Role: Path, Cut: CutLeft},
			{Head: "age", Min: 3, Weight: 1, Role: Muted, Cut: CutNever},
		},
		Rows: [][]string{
			{"haus", "current", "/Users/you/code/workshop/haus/modules/core/haus.sh", "2h"},
			{"scruff", "stale", "/Users/you/code/workshop/scruff/internal/ui/ui.go", "3d"},
			{"nebelung", "current", "/Users/you/code/workshop/nebelung/palette/nebelung.json", "11m"},
		},
	}
}

// `%-38s` is a width the terminal never agreed to. A budgeted table has to hold
// the same bound as everything else, at every width, including the ones where
// it gives up and stacks.
func TestTableNeverReachesTheLastColumn(t *testing.T) {
	tbl := demoTable()
	for w := 2; w <= 200; w++ {
		term := Term{Width: w, Profile: NoColor, IsTTY: true}
		for _, l := range tbl.Render(term, NewTheme(term)) {
			if Width(l) > w-1 {
				t.Fatalf("width %d: %q is %d cells, limit %d", w, l, Width(l), w-1)
			}
		}
	}
}

// Below the sum of the minimums the table stops being a table rather than
// emitting a row it knows will wrap. The values still have to be there.
func TestNarrowWindowsStackInsteadOfWrapping(t *testing.T) {
	term := Term{Width: 24, Profile: NoColor, IsTTY: true}
	out := strings.Join(demoTable().Render(term, NewTheme(term)), "\n")
	for _, want := range []string{"haus", "current", "2h", "nebelung"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stacked output lost %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "repo ") {
		t.Fatalf("stacked output should label each value with its column head:\n%s", out)
	}
}

// A column that never truncates is right or it is absent — an abbreviated
// duration is a wrong duration.
func TestCutNeverIsDroppedRatherThanAbbreviated(t *testing.T) {
	tbl := Table{
		Cols: []Col{
			{Head: "name", Min: 4, Weight: 1, Role: Subject},
			{Head: "took", Min: 3, Weight: 1, Role: Muted, Cut: CutNever},
		},
		Rows: [][]string{{"build", "100m 05s"}},
	}
	term := Term{Width: 200, Profile: NoColor, IsTTY: true}
	if out := strings.Join(tbl.Render(term, NewTheme(term)), ""); !strings.Contains(out, "100m 05s") {
		t.Fatalf("wide window dropped the duration: %q", out)
	}
}

// Surplus released by a column that hit its natural width has to reach the
// others, or one greedy column takes the whole window and the rest stay at
// their minimums with room to spare.
func TestSurplusReachesEveryColumn(t *testing.T) {
	tbl := Table{
		Cols: []Col{
			{Head: "a", Min: 2, Weight: 1, Role: Body},
			{Head: "b", Min: 2, Weight: 99, Role: Body},
		},
		Rows: [][]string{{"aaaaaaaaaa", "bb"}},
	}
	term := Term{Width: 20, Profile: NoColor, IsTTY: true}
	out := strings.Join(tbl.Render(term, NewTheme(term)), "")
	// `b` is weighted 99:1 but only ever wants 2 cells; `a` must get the rest.
	if !strings.Contains(out, "aaaaaaaaaa") {
		t.Fatalf("greedy column starved its neighbour: %q", out)
	}
}

// printerOn builds a printer whose two streams are measured SEPARATELY. Every
// test below is about the case where they disagree, which is the case the
// report path exists for.
func printerOn(outT, errT Term) (*Printer, *bytes.Buffer, *bytes.Buffer) {
	out, errw := &bytes.Buffer{}, &bytes.Buffer{}
	return &Printer{
		Out: out, Err: errw,
		term: errT, theme: NewTheme(errT),
		outTerm: outT, outTheme: NewTheme(outT),
	}, out, errw
}

func tty(w int) Term {
	return Term{Width: w, Height: 24, Profile: NoColor, IsTTY: true, Variant: Nebelung}
}
func pipe() Term {
	return Term{Width: 80, Height: 24, Profile: NoColor, IsTTY: false, Variant: Nebelung}
}

// The split is the point: a report lands on fd 1 so `bench status | less`
// carries it whole, and the narration stays on fd 2.
func TestAReportGoesToStdoutAndNarrationToStderr(t *testing.T) {
	p, out, errw := printerOn(tty(120), tty(120))
	p.PrintData(demoTable())
	p.Print(demoTable())
	if !strings.Contains(out.String(), "nebelung") {
		t.Fatalf("PrintData wrote nothing to stdout:\n%q", out.String())
	}
	if !strings.Contains(errw.String(), "nebelung") {
		t.Fatalf("Print wrote nothing to stderr:\n%q", errw.String())
	}
}

// The bug this method exists to make unrepresentable: a report budgeted from
// the OTHER stream is a report fitted to a window it will never land in.
func TestAReportIsBudgetedForItsOwnStream(t *testing.T) {
	// Narrow stdout, wide stderr. Every line has to fit STDOUT.
	p, out, _ := printerOn(tty(40), tty(200))
	p.PrintData(demoTable())
	for _, l := range strings.Split(strings.TrimRight(out.String(), "\n"), "\n") {
		if w := Width(l); w > 39 {
			t.Fatalf("%d cells in a 40-column report — budgeted from the other stream: %q", w, l)
		}
	}

	// And the mirror, which is the half a one-sided test would pass while still
	// asking the wrong stream: nothing may be given up for stderr's sake either.
	q, wide, _ := printerOn(tty(200), tty(40))
	q.PrintData(demoTable())
	if !strings.Contains(wide.String(), "/Users/you/code/workshop/nebelung/palette/nebelung.json") {
		t.Fatalf("a 200-column report was cut down to the narrow stream:\n%s", wide.String())
	}
}

// A pipe has no last column, so nothing is cut to fit one. Same principle Prose
// already held, arriving at Avail: assuming 80 for a stream with no window is
// `tput cols` with the number hardcoded one layer up.
//
// The path is deliberately longer than the demo's own. At 80 cells the demo
// table happens to fit in 79, so a test built on it passes whether or not the
// truncation is there — a vacuous test for the one behaviour this is about.
func TestAPipedReportIsNeverTruncated(t *testing.T) {
	long := "/Users/you/code/workshop/haus/modules/terminal/lanes/lane-open.sh"
	tbl := demoTable()
	tbl.Rows = append(tbl.Rows, []string{"haus", "current", long, "9s"})
	p, out, _ := printerOn(pipe(), tty(40))
	p.PrintData(tbl)
	got := out.String()
	if !strings.Contains(got, long) {
		t.Fatalf("a redirected report cut a path to a width nobody chose:\n%s", got)
	}
	if strings.Contains(got, "…") {
		t.Fatalf("a redirected report was abbreviated:\n%s", got)
	}
}

// The colour gate asks the report's own stream too. A live truecolor terminal on
// stderr must not put escapes into a stdout somebody is parsing.
func TestAPipedReportCarriesNoEscapes(t *testing.T) {
	errT := tty(120)
	errT.Profile = TrueColor
	p, out, _ := printerOn(pipe(), errT)
	p.PrintData(demoTable())
	if strings.Contains(out.String(), "\x1b") {
		t.Fatalf("escapes reached a redirected report:\n%q", out.String())
	}
}
