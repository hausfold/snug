package snug

import (
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
			{"holt", "stale", "/Users/you/code/workshop/holt/internal/ui/ui.go", "3d"},
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
		term := Term{Width: w, Profile: NoColor}
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
	term := Term{Width: 24, Profile: NoColor}
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
	term := Term{Width: 200, Profile: NoColor}
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
	term := Term{Width: 20, Profile: NoColor}
	out := strings.Join(tbl.Render(term, NewTheme(term)), "")
	// `b` is weighted 99:1 but only ever wants 2 cells; `a` must get the rest.
	if !strings.Contains(out, "aaaaaaaaaa") {
		t.Fatalf("greedy column starved its neighbour: %q", out)
	}
}
