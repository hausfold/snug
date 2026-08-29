package snug

import (
	"bytes"
	"strings"
	"testing"
)

func regionAt(width int, rows []Row) *Region { return regionOn(width, NoColor, rows) }

func regionOn(width int, prof Profile, rows []Row) *Region {
	t := Term{Width: width, Height: 24, Profile: prof, IsTTY: true, Variant: Nebelung}
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
// It sweeps every colour depth, and that is not thoroughness for its own sake.
// Running it colourless only is what let the `bare` tier ship two cells over the
// edge: it trimmed the gutter's padding back off with strings.TrimRight(mark,
// " "), which does exactly nothing once the mark ends in a reset escape. The bug
// was invisible to this test until the day it had a painted string to look at.
func TestRegionNeverReachesTheLastColumn(t *testing.T) {
	for _, prof := range []Profile{NoColor, ANSI16, ANSI256, TrueColor} {
		for w := 2; w <= 200; w++ {
			lines := regionOn(w, prof, jobs).layout()
			if len(lines) != len(jobs) {
				t.Fatalf("profile %d, width %d: %d lines for %d rows", prof, w, len(lines), len(jobs))
			}
			for _, l := range lines {
				if Width(l) > w-1 {
					t.Fatalf("profile %d, width %d: %q is %d cells, limit %d", prof, w, l, Width(l), w-1)
				}
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

// The cursor comes back on EVERY exit path, and a signal handler is the path
// that has no *Region to close — it has only the printer. `snug run` installs
// one because Go's default SIGINT disposition runs no defer at all: without
// this, a ⌃C through `haus rebuild` or `bench release` left the terminal with
// no cursor, permanently, and nothing the calling shell can trap fixes it (the
// coprocess is already dead by the time bash's own INT trap runs).
func TestCloseLiveRestoresTheCursorWithoutTheRegion(t *testing.T) {
	term := Term{Width: 80, Height: 24, Profile: NoColor, IsTTY: true, Variant: Nebelung}
	p := &Printer{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}, term: term, theme: NewTheme(term)}

	// Nothing open: a handler that fires before the first frame must not panic
	// and must not write a stray escape at somebody's shell prompt.
	p.CloseLive()
	if got := p.Err.(*bytes.Buffer).String(); got != "" {
		t.Fatalf("wrote %q with no region open", got)
	}

	r := p.Live()
	buf := p.Err.(*bytes.Buffer)
	if !strings.Contains(buf.String(), "\x1b[?25l") {
		t.Fatalf("Live did not hide the cursor: %q", buf.String())
	}
	buf.Reset()

	p.CloseLive()
	if !strings.Contains(buf.String(), "\x1b[?25h") {
		t.Fatalf("CloseLive did not restore the cursor: %q", buf.String())
	}
	if !r.closed {
		t.Fatal("CloseLive left the region open")
	}

	// Idempotent, because the handler and the deferred Close both run on the
	// paths where the process is dying slowly enough for both to happen.
	buf.Reset()
	p.CloseLive()
	r.Close()
	if got := buf.String(); got != "" {
		t.Fatalf("a second close wrote %q", got)
	}
}

// Not a terminal: no cursor escape may reach a file, a pipe or a CI log — on
// this path as on every other.
func TestCloseLiveWritesNothingOffATerminal(t *testing.T) {
	term := Term{Width: 80, Height: 24, Profile: NoColor, IsTTY: false, Variant: Nebelung}
	p := &Printer{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}, term: term, theme: NewTheme(term)}
	p.Live()
	p.CloseLive()
	if got := p.Err.(*bytes.Buffer).String(); got != "" {
		t.Fatalf("wrote %q with no terminal", got)
	}
}
