package snug

import (
	"fmt"
	"strconv"
)

// Role is what a piece of text MEANS. Callers name a role; they never name a
// colour. That indirection is the whole point: seven hand-picked 256-colour
// indices, copy-pasted into four files, is how the family ended up ΔE 2–27 from
// the flavour every other tool on the machine was wearing — with two different
// greys for one role, and a primary accent that resolved to blue, the one hue
// nebelung exists to strip out.
type Role int

const (
	Body    Role = iota // ordinary text
	Accent              // the tool speaking — say(), section heads
	OK                  // current, healthy, passed
	Warn                // stale, wants attention, degraded
	Err                 // failed, refused, missing
	Muted               // secondary detail — durations, counts
	Subject             // the thing under discussion — repo, host, lane
	Path                // a filesystem path or a store path
	Field               // a key in a key/value grid
)

// roleToken maps a role onto the nebelung token it wears.
//
// Body is deliberately absent rather than mapped to `text`: painting ordinary
// prose fights the reader's own background and is the fastest way to look cheap.
var roleToken = map[Role]string{
	Accent:  "mauve",
	OK:      "green",
	Warn:    "peach",
	Err:     "red",
	Muted:   "overlay1",
	Subject: "sapphire",
	Path:    "teal",
	Field:   "subtext0",
}

// collapse16 is what a role becomes when the terminal has only sixteen colours.
// Distinctions the palette cannot carry are given up on purpose, rather than
// letting two roles land on the same base colour by accident and read as a bug.
var collapse16 = map[Role]Role{
	Subject: Accent,
	Path:    Muted,
	Field:   Muted,
}

// Theme resolves a role to an escape sequence for one terminal.
type Theme struct {
	profile Profile
	variant Variant
	cache   map[Role]string
}

// NewTheme builds the theme for a terminal snapshot.
func NewTheme(t Term) *Theme {
	return &Theme{profile: t.Profile, variant: t.Variant, cache: map[Role]string{}}
}

// Reset ends a run of colour. Empty when the terminal has none, so the same
// format string works either way.
func (th *Theme) Reset() string {
	if th.profile == NoColor {
		return ""
	}
	return "\x1b[0m"
}

// SGR is the escape that starts a run of this role, or "" when the terminal has
// no colour. Cached: a live region asks for these ten times a second.
func (th *Theme) SGR(r Role) string {
	if s, ok := th.cache[r]; ok {
		return s
	}
	s := th.sgr(r)
	th.cache[r] = s
	return s
}

func (th *Theme) sgr(r Role) string {
	if th.profile == NoColor || r == Body {
		return ""
	}
	if th.profile == ANSI16 {
		if c, ok := collapse16[r]; ok {
			r = c
		}
	}
	tok, ok := roleToken[r]
	if !ok {
		return ""
	}
	rr, gg, bb := hexRGB(palette[th.variant][tok])
	switch th.profile {
	case TrueColor:
		return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", rr, gg, bb)
	case ANSI256:
		return "\x1b[38;5;" + strconv.Itoa(nearest256(rr, gg, bb)) + "m"
	default:
		if n, ok := role16[r]; ok {
			return "\x1b[" + strconv.Itoa(n) + "m"
		}
		return ""
	}
}

// Paint wraps s in a role, and is a no-op on a colourless terminal.
//
// It never pads: colour must live OUTSIDE a width, because an escape counted as
// width shears every column after it.
func (th *Theme) Paint(r Role, s string) string {
	sgr := th.SGR(r)
	if sgr == "" {
		return s
	}
	return sgr + s + "\x1b[0m"
}

func hexRGB(h string) (int, int, int) {
	if len(h) == 7 && h[0] == '#' {
		v, err := strconv.ParseUint(h[1:], 16, 32)
		if err == nil {
			return int(v >> 16 & 0xff), int(v >> 8 & 0xff), int(v & 0xff)
		}
	}
	return 0, 0, 0
}

// cubeLevels are the six values xterm's 6×6×6 colour cube samples each channel
// at. They are not evenly spaced, which is why the nearest cube colour has to be
// searched for rather than divided out.
var cubeLevels = [6]int{0, 95, 135, 175, 215, 255}

// nearest256 picks the closest of xterm's 216 cube colours and 24 greys.
//
// Both are searched, because the grey ramp is much finer than the cube's grey
// diagonal — nebelung's `overlay1` (#858585) lands within ΔE 2 of grey 245 and
// nowhere near any cube entry.
func nearest256(r, g, b int) int {
	best, bestD := 0, 1<<30
	for ri, rv := range cubeLevels {
		for gi, gv := range cubeLevels {
			for bi, bv := range cubeLevels {
				if d := dist(r, g, b, rv, gv, bv); d < bestD {
					best, bestD = 16+36*ri+6*gi+bi, d
				}
			}
		}
	}
	for i := range 24 {
		v := 8 + i*10
		if d := dist(r, g, b, v, v, v); d < bestD {
			best, bestD = 232+i, d
		}
	}
	return best
}

// role16 is what each role becomes on a sixteen-colour terminal, DECLARED
// rather than computed.
//
// Nearest-RGB is the wrong algorithm here, and measurably so: nebelung is a
// pastel palette, so `green` (#abe1a6) and `peach` (#f5b58e) both land nearer
// mid-grey than any named colour, and ok and warn come out identical — the two
// roles it matters most to tell apart. Sixteen colours are names, so the map is
// by INTENT: ok is green because ok means green, not because the arithmetic
// said so.
var role16 = map[Role]int{
	Accent: 95, // bright magenta — mauve's nearest name
	OK:     92, // bright green
	Warn:   93, // bright yellow
	Err:    91, // bright red
	Muted:  90, // bright black, which every theme renders as a grey
	Field:  37, // white
}

// dist weights the channels the way the eye does — green hardest, blue least.
// Plain Euclidean RGB picks visibly wrong neighbours for the muted, low-chroma
// colours this palette is made of.
func dist(r1, g1, b1, r2, g2, b2 int) int {
	dr, dg, db := r1-r2, g1-g2, b1-b2
	return 3*dr*dr + 6*dg*dg + db*db
}
