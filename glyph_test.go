package snug

import "testing"

// Every mark occupies exactly Gutter cells, so a ✓ line and a ≋ line start
// their text in the same column and a folded continuation has a fixed indent to
// hang from. This is the whole reason widths are DECLARED rather than measured:
// four of the marks are East_Asian_Width = Ambiguous, which is one cell here and
// two under an East-Asian locale, and every width library has a mode for each.
// A library has no single answer to give. The table does.
func TestEveryMarkFillsTheGutterExactly(t *testing.T) {
	term := Term{Width: 80}
	t.Setenv("LANG", "en_US.UTF-8")
	for m := MarkNone; m <= MarkHint; m++ {
		g, w := term.Glyph(m)
		if w != Gutter {
			t.Fatalf("mark %d reported width %d, want %d", m, w, Gutter)
		}
		if got := len([]rune(g)); got == 0 {
			t.Fatalf("mark %d rendered empty", m)
		}
		// The DECLARED width plus its padding: the check that catches a glyph
		// swapped for a wider one without the padding being re-thought.
		if declared := glyphs[m].width; declared+countTrailingSpaces(g) != Gutter {
			t.Fatalf("mark %d: declared %d + %d pad != %d (%q)",
				m, declared, countTrailingSpaces(g), Gutter, g)
		}
	}
}

func countTrailingSpaces(s string) int {
	n := 0
	for i := len(s) - 1; i >= 0 && s[i] == ' '; i-- {
		n++
	}
	return n
}

// The ASCII alphabet has to hold the same gutter, or a C-locale terminal gets
// every column sheared instead of merely plain.
func TestASCIIAlphabetHoldsTheGutter(t *testing.T) {
	t.Setenv("SNUG_ASCII", "1")
	term := Term{Width: 80}
	for m := MarkSay; m <= MarkHint; m++ {
		g, w := term.Glyph(m)
		if w != Gutter || Width(g) != Gutter {
			t.Fatalf("ascii mark %d = %q (%d cells), want %d", m, g, Width(g), Gutter)
		}
	}
	if got := term.Spin(0); Width(got) != 1 {
		t.Fatalf("ascii spinner frame %q is %d cells", got, Width(got))
	}
	if got := term.Ellipsis(); Width(got) != 1 {
		t.Fatalf("ascii ellipsis %q is %d cells", got, Width(got))
	}
}
