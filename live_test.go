package snug

import (
	"bytes"
	"strings"
	"testing"
)

func regionAt(width int, rows []Row) *Region {
	t := Term{Width: width, Height: 24, Profile: NoColor, IsTTY: true, Variant: Nebelung}
	p := &Printer{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}, term: t, theme: NewTheme(t)}
	return &Region{p: p, rows: rows}
}

var jobs = []Row{
	{Spin: true, Name: "publish", Detail: ""},
	{Mark: MarkOK, Name: "bump-tap", Detail: "12s"},
	{Mark: MarkBullet, Name: "bump-flake-and-notarize-the-zip", Detail: "queued"},
}

// The contract the whole type exists for: the repaint moves the cursor up by
// the number of rows it PRINTED, so no row may reach the terminal's last
// column — otherwise the terminal wraps one row into two screen lines and the
// cursor walks up through the middle of the block.
func TestRegionNeverReachesTheLastColumn(t *testing.T) {
	for w := 2; w <= 200; w++ {
		lines := regionAt(w, jobs).layout()
		if len(lines) != len(jobs) {
			t.Fatalf("width %d: %d lines for %d rows", w, len(lines), len(jobs))
		}
		for _, l := range lines {
			if Width(l) > w-1 {
				t.Fatalf("width %d: %q is %d cells, limit %d", w, l, Width(l), w-1)
			}
		}
	}
}

// The regression that shipped for an afternoon in bash: the tier was chosen
// from the name column AFTER it had been clamped down to the longest job name,
// so a run of build / test / lint read as a narrow window and dropped its
// durations on a 200-column terminal.
func TestShortNamesKeepTheirDetail(t *testing.T) {
	short := []Row{
		{Mark: MarkOK, Name: "build", Detail: "12s"},
		{Mark: MarkOK, Name: "test", Detail: "5s"},
		{Mark: MarkOK, Name: "lint", Detail: "3s"},
	}
	for _, w := range []int{200, 120, 80, 40, 30} {
		joined := strings.Join(regionAt(w, short).layout(), "\n")
		if !strings.Contains(joined, "12s") {
			t.Fatalf("width %d dropped the duration column:\n%s", w, joined)
		}
	}
}

// The other one: the detail column was budgeted at a hardcoded seven cells,
// because "12m 34s" is surely the longest duration. GitHub allows six-hour
// jobs, and "100m 05s" is eight — straight into the last column.
func TestAWideDetailStillFits(t *testing.T) {
	rows := []Row{
		{Spin: true, Name: "notarize", Detail: "100m 05s"},
		{Mark: MarkOK, Name: "build", Detail: "12s"},
	}
	for w := 2; w <= 200; w++ {
		for _, l := range regionAt(w, rows).layout() {
			if Width(l) > w-1 {
				t.Fatalf("width %d: %q is %d cells", w, l, Width(l))
			}
		}
	}
}

// Tiers shed the least useful thing first: the name is load-bearing, the
// duration is not.
func TestTiersShedTheDetailBeforeTheName(t *testing.T) {
	rows := []Row{{Mark: MarkOK, Name: "bump-tap", Detail: "12s"}}
	wide := strings.Join(regionAt(100, rows).layout(), "\n")
	if !strings.Contains(wide, "bump-tap") || !strings.Contains(wide, "12s") {
		t.Fatalf("wide tier lost a column: %q", wide)
	}
	narrow := strings.Join(regionAt(18, rows).layout(), "\n")
	if !strings.Contains(narrow, "bump-ta") {
		t.Fatalf("narrow tier lost the name: %q", narrow)
	}
	if strings.Contains(narrow, "12s") {
		t.Fatalf("narrow tier kept the detail: %q", narrow)
	}
}

// A region on something that is not a terminal emits no cursor escape at all —
// a piped log, a CI transcript and a bats capture all have to stay readable.
func TestNotATerminalEmitsNoEscapes(t *testing.T) {
	var buf bytes.Buffer
	term := Term{Width: 80, Height: 24, Profile: NoColor, IsTTY: false}
	p := &Printer{Out: &bytes.Buffer{}, Err: &buf, term: term, theme: NewTheme(term)}
	r := p.Live()
	r.Set(jobs)
	r.Set(jobs) // a second identical frame prints nothing: state-change only
	r.Close()
	if strings.Contains(buf.String(), "\x1b") {
		t.Fatalf("escape reached a non-terminal stream: %q", buf.String())
	}
	if n := strings.Count(buf.String(), "\n"); n != len(jobs) {
		t.Fatalf("want one line per state change (%d), got %d:\n%s", len(jobs), n, buf.String())
	}
}
