package snug

import (
	"strings"
	"testing"
)

func TestNoColourMeansNoEscapes(t *testing.T) {
	th := NewTheme(Term{Profile: NoColor})
	for r := Body; r <= Field; r++ {
		if s := th.SGR(r); s != "" {
			t.Fatalf("role %d emitted %q with colour off", r, s)
		}
		if got := th.Paint(r, "x"); got != "x" {
			t.Fatalf("role %d painted %q with colour off", r, got)
		}
	}
	if th.Reset() != "" {
		t.Fatal("Reset emitted an escape with colour off")
	}
}

// Body is unset on purpose. Painting ordinary prose fights the reader's own
// background and is the fastest way to look cheap.
func TestBodyIsNeverPainted(t *testing.T) {
	for _, p := range []Profile{ANSI16, ANSI256, TrueColor} {
		if s := NewTheme(Term{Profile: p}).SGR(Body); s != "" {
			t.Fatalf("profile %d painted Body: %q", p, s)
		}
	}
}

func TestTrueColorIsTheNebelungHexExactly(t *testing.T) {
	th := NewTheme(Term{Profile: TrueColor, Variant: Nebelung})
	// mauve #c9a8f1 → 201, 168, 241
	if got, want := th.SGR(Accent), "\x1b[38;2;201;168;241m"; got != want {
		t.Fatalf("Accent = %q, want %q", got, want)
	}
}

// nearest256 has to search the grey RAMP as well as the cube. The cube's grey
// diagonal only samples six values; the ramp has twenty-four, and for a neutral
// that falls between cube levels it is the only honest answer.
func TestNearest256SearchesBothCubeAndRamp(t *testing.T) {
	if got := nearest256(hexRGB("#767676")); got != 243 {
		t.Fatalf("#767676 → %d; ramp entry 243 is exact", got) // 8+11*10 = 118 = 0x76
	}
	// …and still uses the cube when the cube is closer: #878787 is exactly the
	// cube's third grey level, which no ramp entry hits.
	if got := nearest256(hexRGB("#878787")); got < 16 || got > 231 {
		t.Fatalf("#878787 → %d; the cube has it exactly", got)
	}
	th := NewTheme(Term{Profile: ANSI256, Variant: Nebelung})
	if !strings.HasPrefix(th.SGR(Muted), "\x1b[38;5;") {
		t.Fatalf("Muted = %q", th.SGR(Muted))
	}
}

// Sixteen colours cannot carry nine roles, so the collapse is declared rather
// than left to two roles landing on the same base colour by accident.
func TestSixteenColoursCollapseDeclaredRoles(t *testing.T) {
	th := NewTheme(Term{Profile: ANSI16, Variant: Nebelung})
	if th.SGR(Subject) != th.SGR(Accent) {
		t.Fatal("Subject should collapse onto Accent at 16 colours")
	}
	if th.SGR(Path) != th.SGR(Muted) {
		t.Fatal("Path should collapse onto Muted at 16 colours")
	}
	// …but the ones that carry meaning stay distinct. This is what nearest-RGB
	// got wrong: on a pastel palette, ok and warn both resolved to white.
	seen := map[string]Role{}
	for _, r := range []Role{OK, Warn, Err} {
		s := th.SGR(r)
		if other, dup := seen[s]; dup {
			t.Fatalf("roles %d and %d both resolve to %q at 16 colours", other, r, s)
		}
		seen[s] = r
	}
}

// A light machine gets the light answer without asking. This is the entire
// reason roles resolve through a variant instead of a constant.
func TestLatteIsADifferentPalette(t *testing.T) {
	dark := NewTheme(Term{Profile: TrueColor, Variant: Nebelung})
	light := NewTheme(Term{Profile: TrueColor, Variant: NebelungLatte})
	for r := Accent; r <= Field; r++ {
		if dark.SGR(r) == light.SGR(r) {
			t.Fatalf("role %d is the same in nebelung and nebelung-latte", r)
		}
	}
}

// Every role a caller can name has to resolve to something under every variant,
// or a palette regeneration that dropped a token would show up as invisible
// text rather than as a build failure.
func TestEveryRoleResolvesUnderEveryVariant(t *testing.T) {
	for v := Nebelung; v <= NebelungLatteHC; v++ {
		for r, tok := range roleToken {
			if palette[v][tok] == "" {
				t.Fatalf("variant %d has no %q for role %d", v, tok, r)
			}
		}
	}
}
