package snug

import (
	"os"
	"strings"
)

// Mark is the small symbol at the head of a line. It is load-bearing and the
// colour is not: every role has to survive NO_COLOR, so the glyph is what
// carries the meaning and the colour is the courtesy.
type Mark int

const (
	MarkNone Mark = iota
	MarkSay
	MarkOK
	MarkWarn
	MarkErr
	MarkInfo
	MarkSkip
	MarkBullet
	MarkHint
)

// Gutter is how many cells a line gives to its mark, padding included.
//
// Fixed, so that lines with different marks align and a folded continuation has
// somewhere to hang from. Three, because the widest mark is two cells and one
// space is the least that reads as separation.
const Gutter = 3

// glyph carries its own DECLARED width, and that is the important part.
//
// No width library can be trusted with 🌫 (U+1F32B). It has Emoji_Presentation
// = No, so `x/ansi` and `runewidth` both answer 1 — and every terminal the
// family runs on draws it in two cells. They also disagree with each other on
// the variation-selector form (2 against 1), which is the tell that this is not
// a question a table lookup settles. So the width is declared here, verified
// against real terminals, and measurement libraries are used only on CONTENT,
// which is ordinary text and where they are reliable.
//
// If a terminal ever proves otherwise, this is one number:
//
//	printf '123456789\n\U0001F32B|\n'   # the | sits at column 3 if it is 2 cells
type glyph struct {
	utf8  string
	ascii string
	width int
}

var glyphs = map[Mark]glyph{
	MarkNone:   {"", "", 0},
	MarkSay:    {"\U0001F32B", "~", 2}, // fog — the family's signature
	MarkOK:     {"✓", "+", 1},
	MarkWarn:   {"⚠", "!", 1},
	MarkErr:    {"✗", "x", 1},
	MarkInfo:   {"ⓘ", "i", 1},
	MarkSkip:   {"–", "-", 1},
	MarkBullet: {"·", ".", 1},
	MarkHint:   {"↳", ">", 1},
}

// Spinner frames. One cell each, and the painter budgets on that — a frame
// wider than one cell needs Gutter changed with it.
var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

var spinnerASCII = []string{"|", "/", "-", "\\"}

// Ellipsis marks a truncation. One cell in both alphabets.
func (t Term) Ellipsis() string {
	if t.unicode() {
		return "…"
	}
	return "~"
}

// Spin returns frame n of the spinner.
func (t Term) Spin(n int) string {
	f := spinner
	if !t.unicode() {
		f = spinnerASCII
	}
	if n < 0 {
		n = -n
	}
	return f[n%len(f)]
}

// Glyph returns the mark and its declared width, padded to Gutter.
//
// Padding here rather than at the call site is what keeps a ✓ line and a 🌫 line
// starting their text in the same column.
func (t Term) Glyph(m Mark) (string, int) {
	g, ok := glyphs[m]
	if !ok || m == MarkNone {
		return strings.Repeat(" ", Gutter), Gutter
	}
	s, w := g.utf8, g.width
	if !t.unicode() {
		s, w = g.ascii, len(g.ascii)
	}
	if pad := Gutter - w; pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s, Gutter
}

// unicode reports whether the far end can be trusted with the UTF-8 alphabet.
//
// The locale is the only signal there is. It is a weak one — a UTF-8 locale
// says nothing about which glyphs the FONT has — but the failure it prevents is
// the loud one: mojibake in a C-locale terminal, where every mark becomes three
// question marks and every column shears by two.
func (t Term) unicode() bool {
	if v, ok := os.LookupEnv("SNUG_ASCII"); ok && v != "" && v != "0" {
		return false
	}
	for _, k := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := os.Getenv(k); v != "" {
			return strings.Contains(strings.ToUpper(v), "UTF-8") ||
				strings.Contains(strings.ToUpper(v), "UTF8")
		}
	}
	return false
}
