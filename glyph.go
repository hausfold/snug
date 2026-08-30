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
// somewhere to hang from. Three, because every mark is one cell and two spaces
// is what the family's CLIs have always put after theirs.
const Gutter = 3

// glyph carries its own DECLARED width, and that is the important part.
//
// No mark DEFAULTS to emoji presentation — none has Emoji_Presentation = Yes,
// which is the property an emoji's disputed width comes from. That is a
// narrower claim than "no emoji": `⚠` (U+26A0) carries Emoji = Yes and is here
// anyway, and it is one of the two reasons the widths are still declared.
//
// Because dropping the emoji did NOT make measurement safe. The hazard is
// East_Asian_Width, and it survives in two shapes:
//
//   - `ⓘ` (U+24D8), `–` (U+2013) and `·` (U+00B7) — and `…` (U+2026), which is
//     not a mark but is the Ellipsis every truncation ends in — are
//     East_Asian_Width = Ambiguous. That is one cell in a Western locale and
//     TWO under an East-Asian one, and both x/ansi and runewidth expose a mode
//     for each answer, so neither has a single one to give.
//   - `⚠` (U+26A0) is the other. Bare it is one cell, but the
//     emoji-presentation sequence U+26A0 U+FE0F is two. The table stores bare
//     codepoints and never a sequence, and a caller that appends a variation
//     selector to a mark has silently doubled it.
//
// So: glyph widths are declared and verified against a real terminal, and
// measurement libraries are used only on CONTENT, which is ordinary text and
// where they agree. To re-check a terminal, hold a mark against the ruler —
// the digits are the answer, not the other mark, since a locale that doubles
// one Ambiguous glyph doubles them all and leaves any two of them aligned:
//
//	printf '123456789\nⓘ|\n'
//
// The `|` sits at column 2 iff the mark is one cell. For a number rather than
// an eyeball, ask the terminal where its cursor ended up:
//
//	stty -echo; printf '\r\u24D8\033[6n'; IFS=';' read -rd R _ col; stty echo
//	echo "$col"   # 2 → one cell, 3 → two
type glyph struct {
	utf8  string
	ascii string
	width int
}

var glyphs = map[Mark]glyph{
	MarkNone:   {"", "", 0},
	MarkSay:    {"\u224B", "~", 1}, // the family's signature: the ascii `~`, tripled
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
// Padding here rather than at the call site is what keeps a ✓ line and a ≋ line
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

// GlyphBare is the mark ALONE — one cell, with no padding to the Gutter.
//
// The narrow tiers of a live region need it, and the padded form is a bug
// there: at `bare` the gutter has collapsed to a single space, so a mark still
// carrying two cells of padding puts the row two cells past the window's edge.
//
// Trimming that padding off afterwards looks like it covers this and does not.
// A painted mark ends in a reset escape, so `strings.TrimRight(mark, " ")`
// silently removes nothing the moment colour is on — which is exactly how it
// shipped, with a width sweep that only ever ran colourless to say it was fine.
func (t Term) GlyphBare(m Mark) string {
	g, ok := glyphs[m]
	if !ok || m == MarkNone {
		return ""
	}
	if !t.unicode() {
		return g.ascii
	}
	return g.utf8
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
