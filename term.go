package snug

import (
	"os"
	"strings"

	"golang.org/x/term"
)

// Profile is how much colour a terminal can be trusted with.
type Profile int

const (
	NoColor   Profile = iota // no escapes at all; the glyph carries the meaning
	ANSI16                   // the eight base colours and their bright halves
	ANSI256                  // xterm's 6×6×6 cube plus the 24-step grey ramp
	TrueColor                // 24-bit; the nebelung hex, exactly
)

// Variant is which nebelung a machine is wearing.
type Variant int

const (
	Nebelung Variant = iota
	NebelungHighContrast
	NebelungLatte
	NebelungLatteHC
)

var variantNames = map[string]Variant{
	"nebelung":                     Nebelung,
	"nebelung-high-contrast":       NebelungHighContrast,
	"nebelung-latte":               NebelungLatte,
	"nebelung-latte-high-contrast": NebelungLatteHC,
	// The two names a person types when they mean the flavour, not the variant.
	"mocha": Nebelung,
	"latte": NebelungLatte,
}

// Term is one snapshot of what the far end can do. Take it once at startup and
// again on SIGWINCH — never per line, because Size() is a syscall.
type Term struct {
	Width   int
	Height  int
	Profile Profile
	IsTTY   bool
	Variant Variant
}

// DetectTerm reads the environment and the file's window size.
//
// The precedence is the one every well-behaved CLI already agrees on, and it is
// worth stating because it is easy to get subtly wrong: NO_COLOR wins over
// everything except CLICOLOR_FORCE, and a non-TTY is colourless unless forced.
// https://no-color.org, https://bixense.com/clicolors
func DetectTerm(f *os.File) Term {
	t := Term{Width: 80, Height: 24, Variant: detectVariant()}
	t.IsTTY = term.IsTerminal(int(f.Fd()))
	if w, h, err := term.GetSize(int(f.Fd())); err == nil && w > 0 {
		t.Width, t.Height = w, h
	}
	t.Profile = detectProfile(t.IsTTY)
	return t
}

func detectProfile(isTTY bool) Profile {
	forced := os.Getenv("CLICOLOR_FORCE") != "" && os.Getenv("CLICOLOR_FORCE") != "0"
	if _, no := os.LookupEnv("NO_COLOR"); no && !forced {
		return NoColor
	}
	if !isTTY && !forced {
		return NoColor
	}
	t := os.Getenv("TERM")
	if t == "dumb" {
		// "dumb" means it, and it means it even under CLICOLOR_FORCE: there is
		// no escape sequence a dumb terminal will not print literally.
		return NoColor
	}
	switch strings.ToLower(os.Getenv("COLORTERM")) {
	case "truecolor", "24bit":
		return TrueColor
	}
	switch {
	case strings.Contains(t, "truecolor"), strings.Contains(t, "direct"):
		return TrueColor
	case strings.Contains(t, "256"):
		return ANSI256
	case t == "":
		// Forced, with nothing to go on — a CI log renderer, usually. 256 is the
		// safe middle: universally understood, and the degradation from the
		// nebelung hex is small.
		return ANSI256
	}
	return ANSI16
}

// detectVariant asks, in order: an explicit override, then the file haus writes
// when it resolves haus.theme.flavor. Neither present means the default dark —
// which is also the honest answer on a machine with no haus at all.
func detectVariant() Variant {
	if v, ok := variantNames[strings.ToLower(os.Getenv("SNUG_VARIANT"))]; ok {
		return v
	}
	if b, err := os.ReadFile(configHome() + "/snug/variant"); err == nil {
		if v, ok := variantNames[strings.ToLower(strings.TrimSpace(string(b)))]; ok {
			return v
		}
	}
	return Nebelung
}

func configHome() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return d
	}
	return os.Getenv("HOME") + "/.config"
}

// Cap is the widest a line of prose may be, whatever the window does.
//
// Past this a line of text stops being readable, and a maximised terminal is
// not an invitation to fill it. Tables and live regions are exempt: they are
// bounded by their own content already, and a job list that stopped at 100
// while the window was 200 would be hiding the room it had.
const Cap = 100

// NoFold is the width of a stream that has no window: wide enough that Fold
// never breaks a line, and finite so the arithmetic around it stays ordinary.
const NoFold = 1 << 20

// Prose is the width a folded paragraph gets.
//
// A stream that is NOT a terminal is never folded, and that is the same
// principle this library was built on rather than a special case. `tput cols`
// is wrong because it answers a STATIC 80 for a window it never measured;
// assuming 80 for a pipe is that identical mistake with the number hardcoded
// one layer up. A redirected stream has no width to fit, so folding it breaks
// paths and sentences at a column nobody chose — and the thing on the far end
// of that pipe is usually grepping for a whole line.
//
// A forced-colour CI log renderer lands here too, and wants exactly this: it
// has colour but no geometry.
func (t Term) Prose() int {
	if !t.IsTTY {
		return NoFold
	}
	if t.Width < Cap {
		return t.Width
	}
	return Cap
}

// Avail is the last column a line may write into, exclusive.
//
// Never the full width: a line whose width EQUALS the terminal's leaves the
// cursor past the edge and the terminal wraps it anyway. Every live region in
// the family broke on exactly that off-by-one before this existed.
func (t Term) Avail() int {
	if t.Width < 1 {
		return 1
	}
	return t.Width - 1
}
