package snug

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Width is the number of terminal CELLS s occupies, ignoring any escapes in it.
//
// Not bytes: `printf '%-9s'` pads by bytes under most locales, and every column
// after a multi-byte glyph is sheared by a different amount depending on which
// glyph it was. Not runes either: a grapheme cluster can be several.
func Width(s string) int { return ansi.StringWidth(s) }

// Truncate cuts s to at most w cells, marking the cut with tail.
//
// The mark is INSIDE the budget, so the result is never wider than asked — the
// caller has already spent those cells on something else.
func Truncate(s string, w int, tail string) string {
	if w <= 0 {
		return ""
	}
	if Width(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, tail)
}

// TruncateLeft cuts from the FRONT, which is what a path wants: `…/internal/ui`
// keeps the part that identifies the file, where cutting the other end would
// leave every path in a repo looking identical.
//
// It cuts at a separator when it can. `…/orkshop/haus` is worse than useless —
// it reads as a directory that does not exist, and the eye stops on it. A
// slightly shorter `…/haus` is the honest answer.
func TruncateLeft(s string, w int, head string) string {
	if w <= 0 {
		return ""
	}
	if Width(s) <= w {
		return s
	}
	hw := Width(head)
	if hw >= w {
		return Truncate(head, w, "")
	}
	keep := w - hw
	r := []rune(s)

	// Walk left while the suffix still fits, keeping the leftmost position and
	// the leftmost SEPARATOR position — both are therefore the longest of their
	// kind that fits. Prefer the separator whenever it keeps at least half of
	// what the raw cut would; below that the boundary costs more than it earns.
	best, bestSep := len(r), -1
	for i := len(r); i >= 0; i-- {
		if Width(string(r[i:])) > keep {
			break
		}
		best = i
		// The separator itself is INCLUDED (`…/haus`, not `…haus`), so it has to
		// be re-measured with it: `r[i:]` fitting says nothing about `r[i-1:]`,
		// and taking that on trust returns a string one cell wider than asked
		// for — the exact off-by-one this library exists to stop.
		if i > 0 && r[i-1] == '/' && Width(string(r[i-1:])) <= keep {
			bestSep = i - 1
		}
	}
	if bestSep >= 0 && Width(string(r[bestSep:]))*2 >= Width(string(r[best:])) {
		return head + string(r[bestSep:])
	}
	return head + string(r[best:])
}

// Fold breaks s into lines of at most w cells, at word boundaries.
//
// The family never soft-wraps. A tool that lets the terminal wrap has given up
// its own indentation — and, in a live region, has desynced the repaint, because
// the cursor moves up by the number of lines PRINTED and the terminal counted
// the ones it drew.
//
// A word longer than w (a store path, a URL) is broken mid-word rather than
// allowed past the edge: overflowing is the one thing that is never allowed.
func Fold(s string, w int) []string {
	if w < 1 {
		w = 1
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		out = append(out, foldOne(para, w)...)
	}
	return out
}

func foldOne(s string, w int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	var out []string
	line := ""
	for _, word := range words {
		for Width(word) > w {
			// Longer than a whole line on its own — a store path, a URL. Take a
			// full line of it and carry the rest; overflowing is the one thing
			// never allowed.
			if line != "" {
				out = append(out, line)
				line = ""
			}
			head := ansi.Truncate(word, w, "")
			if head == "" {
				// Not even one grapheme fits: a two-cell character in a one-cell
				// window. Consume it and mark the loss, because the only outcome
				// worse than a dropped character is a loop that never ends.
				r := []rune(word)
				n := 1
				for n < len(r) && Width(string(r[:n])) == 0 {
					n++ // carry combining marks with their base
				}
				word = string(r[n:])
				out = append(out, Truncate("…", w, ""))
				continue
			}
			out = append(out, head)
			word = word[len(head):]
		}
		switch {
		case line == "":
			line = word
		case Width(line)+1+Width(word) <= w:
			line += " " + word
		default:
			out = append(out, line)
			line = word
		}
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}

// Pad extends s to w cells with spaces, measured in cells rather than bytes.
// Shorter than w only if s is already wider, which the caller is expected to
// have prevented by truncating first.
func Pad(s string, w int) string {
	if d := w - Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}
