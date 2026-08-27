package snug

import (
	"strings"
	"testing"
)

// The one invariant everything else rests on: nothing this package returns is
// ever wider than the width it was handed. Every live-region desync and every
// sheared column in the family started as a violation of exactly this.
func TestNeverWiderThanAsked(t *testing.T) {
	subjects := []string{
		"publish",
		"bump-flake-and-notarize-the-zip",
		"/Users/you/code/workshop/nebelung/palette/nebelung.json",
		"/Users/you/code/workshop/haus/modules/core/haus.sh",
		"a", "", "//", "/a/b/c/d/e/f/g",
		"名前が長いジョブ", // wide cells
		"🌫 fog",
	}
	for _, s := range subjects {
		for w := range 60 {
			if got := Truncate(s, w, "…"); Width(got) > w {
				t.Fatalf("Truncate(%q, %d) = %q (%d cells)", s, w, got, Width(got))
			}
			if got := TruncateLeft(s, w, "…"); Width(got) > w {
				t.Fatalf("TruncateLeft(%q, %d) = %q (%d cells)", s, w, got, Width(got))
			}
			for _, l := range Fold(s, max(w, 1)) {
				if Width(l) > max(w, 1) {
					t.Fatalf("Fold(%q, %d) line %q is %d cells", s, w, l, Width(l))
				}
			}
		}
	}
}

func TestTruncateLeftCutsAtSeparators(t *testing.T) {
	const p = "/Users/you/code/workshop/haus/modules/core/haus.sh"
	// Wide enough for several components: the cut lands on a `/`, because
	// `…orkshop/haus` reads as a directory that does not exist and the eye
	// stops on it.
	got := TruncateLeft(p, 30, "…")
	if !strings.HasPrefix(got, "…/") {
		t.Fatalf("TruncateLeft(%q, 30) = %q — want a separator after the mark", p, got)
	}
	if Width(got) > 30 {
		t.Fatalf("%q is %d cells", got, Width(got))
	}
	// A path with no separator to find still has to fit.
	if got := TruncateLeft("oneverylongcomponentwithnoslashes", 10, "…"); Width(got) > 10 {
		t.Fatalf("%q is %d cells", got, Width(got))
	}
}

func TestFoldBreaksAWordTooLongForALine(t *testing.T) {
	// A store path in a sentence. Overflowing is the one thing never allowed,
	// so an unbreakable word is broken rather than let past the edge.
	s := "see /nix/store/qf2kha7ygr6lpr6bpdl782lf3fydfj5x-pounce-2026.08.09-2 for it"
	for _, l := range Fold(s, 20) {
		if Width(l) > 20 {
			t.Fatalf("line %q is %d cells", l, Width(l))
		}
	}
}

func TestPadCountsCellsNotBytes(t *testing.T) {
	// `printf '%-9s'` pads by BYTES, which is how every column after a glyph
	// ended up sheared by a different amount depending on which glyph it was.
	if got := Pad("✓", 3); Width(got) != 3 {
		t.Fatalf("Pad(✓, 3) = %q, %d cells", got, Width(got))
	}
	if got := Pad("ab", 5); got != "ab   " {
		t.Fatalf("Pad(ab, 5) = %q", got)
	}
}
