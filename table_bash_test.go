package snug

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The bash half lays a table out ITSELF. `bench status` on a machine whose PATH
// has no `snug` — a workshop clone whose layer was never activated, a launchd
// job off a thin PATH, CI — is running ui.sh's arithmetic, not this package's.
// Two implementations of one spec is the deal the standard makes, and this is
// the only thing that holds them to it.
//
// A fallback that laid a table out DIFFERENTLY from the binary would be worse
// than no fallback at all: it makes "which machine is this?" a question you
// have to ask about your own output. The palette half of that promise is kept
// by generating both sides from one list; the LAYOUT half cannot be generated,
// so it is diffed here instead — same columns, same rows, every width from too
// narrow to draw at all up to wider than any content.

// bashCmd finds a bash that can run ui.sh, or skips.
//
// macOS ships 3.2 as /bin/bash and the library needs 4 (associative arrays,
// `${v,,}`), so this skips rather than fails there: the `bash` CI job is Ubuntu
// for exactly that reason, and a developer on a Mac with no newer bash should
// see "skipped", not a red suite about someone else's shell.
func bashCmd(t *testing.T) string {
	t.Helper()
	cands := []string{}
	if b := os.Getenv("SNUG_TEST_BASH"); b != "" {
		cands = append(cands, b)
	}
	cands = append(cands, "bash", "/opt/homebrew/bin/bash", "/usr/local/bin/bash")
	for _, c := range cands {
		p, err := exec.LookPath(c)
		if err != nil {
			continue
		}
		out, err := exec.Command(p, "-c", "echo ${BASH_VERSINFO[0]}").Output()
		if err != nil {
			continue
		}
		if major, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil && major >= 4 {
			return p
		}
	}
	t.Skip("no bash 4+ on this machine; the bash half is covered by the Ubuntu `bash` job")
	return ""
}

// bashQuote wraps s for a single-quoted bash word.
func bashQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

var bashRole = map[Role]string{
	Body: "body", Accent: "accent", OK: "ok", Warn: "warn", Err: "err",
	Muted: "muted", Subject: "subject", Path: "path", Field: "field",
}

var bashCut = map[Side]string{CutRight: "right", CutLeft: "left", CutNever: "never"}

// renderBash runs ui.sh's table over the same spec at every width in widths,
// in ONE bash process, and returns its lines per width.
//
// One process for two hundred widths is the same economy the sweeps in
// test/ui.bats keep: the arithmetic under test does not care which fork it runs
// in, and two hundred forks would put this test in the "run it later" bucket
// where it stops being run at all.
func renderBash(t *testing.T, tbl Table, widths []int) map[int][]string {
	t.Helper()
	sh := bashCmd(t)
	ui, err := filepath.Abs("share/ui.sh")
	if err != nil {
		t.Fatal(err)
	}

	var spec strings.Builder
	for _, c := range tbl.Cols {
		fmt.Fprintf(&spec, "ui_col %s %d %d %s %s\n",
			bashQuote(c.Head), c.Min, c.Weight, bashRole[c.Role], bashCut[c.Cut])
	}
	for _, row := range tbl.Rows {
		spec.WriteString("ui_trow")
		for _, cell := range row {
			spec.WriteString(" " + bashQuote(cell))
		}
		spec.WriteString("\n")
	}
	header := 0
	if tbl.Header {
		header = 1
	}
	var ws []string
	for _, w := range widths {
		ws = append(ws, strconv.Itoa(w))
	}

	// UI_TTY / UI_COLS / UI_AVAIL by hand rather than by a pty, the same way the
	// Go tests construct a Term and test/ui.bats drives its own sweeps. The
	// palette is resolved to `none` on both prefixes so this compares LAYOUT;
	// the colours have their own test, on both sides.
	script := fmt.Sprintf(`
set -euo pipefail
source %s
UI_TTY=1; UI_OUT_TTY=1
UI_PROFILE=none; UI_OUT_PROFILE=none
ui__resolve_palette none UI_
ui__resolve_palette none UI_OUT_
for w in %s; do
  UI_COLS=$w; UI_AVAIL=$(( w - 1 )); UI_OUT_AVAIL=$(( w - 1 )); UI_PROSE=$w
  ui_table_clear
%s
  printf '=== %%s\n' "$w"
  ui_table_data %d %d
done
`, bashQuote(ui), strings.Join(ws, " "), spec.String(), tbl.Indent, header)

	cmd := exec.Command(sh, "-c", script)
	cmd.Env = append(os.Environ(), "SNUG_ASCII=1", "NO_COLOR=", "LC_ALL=C")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ui.sh: %v\n%s", err, out)
	}

	got := map[int][]string{}
	cur := 0
	for _, l := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if strings.HasPrefix(l, "=== ") {
			cur, _ = strconv.Atoi(l[4:])
			got[cur] = []string{}
			continue
		}
		got[cur] = append(got[cur], l)
	}
	return got
}

// The whole promise, at every width the window can be: the two painters draw
// the same table. Every tier is in this range — natural widths at the top,
// squeezed columns through the middle, and the stacked fallback at the bottom
// where not even the minimums fit.
func TestBashTableMatchesGo(t *testing.T) {
	t.Setenv("SNUG_ASCII", "1") // one byte, one character, one cell, both sides

	cases := map[string]Table{
		"the demo table": demoTable(),
		// `bench status`'s lane table, which is what this was built for: a
		// repo, a branch nothing can hold whole, an empty cell on a clean
		// tree, and a path that wants its tail.
		"bench's lanes": {
			Indent: 3,
			Header: true,
			Cols: []Col{
				{Head: "repo", Min: 6, Weight: 1, Role: Subject},
				{Head: "branch", Min: 8, Weight: 3, Role: Body},
				{Head: "dirty", Min: 5, Weight: 1, Role: Muted},
				{Head: "where", Min: 10, Weight: 2, Role: Path, Cut: CutLeft},
			},
			Rows: [][]string{
				{"workshop", "worktree-cli-beautify-snug", "", "/Users/you/.cache/scruff/workshop/cli-beautify-snug"},
				{"haus", "worktree-claude-code-banners", "dirty", "/Users/you/.cache/scruff/haus/claude-code-banners"},
				{"nebelung", "main", "", "/Users/you/code/workshop/nebelung"},
			},
		},
		// A `never` column beside one that folds: the duration is dropped
		// whole rather than abbreviated, and the two halves have to agree on
		// exactly which width that happens at.
		"a duration that is never abbreviated": {
			Indent: 3,
			Cols: []Col{
				{Head: "job", Min: 4, Weight: 3, Role: Subject},
				{Head: "took", Min: 3, Weight: 1, Role: Muted, Cut: CutNever},
			},
			Rows: [][]string{
				{"build", "100m 05s"},
				{"a-job-name-far-longer-than-any-window", "12m 34s"},
				{"lint", "3s"},
			},
		},
	}

	widths := []int{}
	for w := 2; w <= 200; w++ {
		widths = append(widths, w)
	}

	for name, tbl := range cases {
		t.Run(name, func(t *testing.T) {
			bash := renderBash(t, tbl, widths)
			for _, w := range widths {
				term := Term{Width: w, Profile: NoColor, IsTTY: true}
				want := tbl.Render(term, NewTheme(term))
				got := bash[w]
				// Render returns nil for an empty table; bash returns no lines.
				if len(want) != len(got) {
					t.Fatalf("width %d: go drew %d lines, ui.sh drew %d\ngo:\n%s\nui.sh:\n%s",
						w, len(want), len(got), strings.Join(want, "\n"), strings.Join(got, "\n"))
				}
				for i := range want {
					if want[i] != got[i] {
						t.Fatalf("width %d, line %d:\n go:    %q\n ui.sh: %q", w, i, want[i], got[i])
					}
				}
			}
		})
	}
}

// The bound every painter in this library is held to, now for the bash half of
// the table too. A Go-only sweep proves nothing about the file `bench` sources
// on a machine with no binary.
func TestBashTableNeverReachesTheLastColumn(t *testing.T) {
	t.Setenv("SNUG_ASCII", "1")
	widths := []int{}
	for w := 2; w <= 200; w++ {
		widths = append(widths, w)
	}
	for w, lines := range renderBash(t, demoTable(), widths) {
		for _, l := range lines {
			if Width(l) > w-1 {
				t.Fatalf("width %d: ui.sh drew %q, %d cells, limit %d", w, l, Width(l), w-1)
			}
		}
	}
}
