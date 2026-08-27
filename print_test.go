package snug

import (
	"bytes"
	"strings"
	"testing"
)

func printerAt(width int, isTTY bool) (*Printer, *bytes.Buffer) {
	errBuf := &bytes.Buffer{}
	t := Term{Width: width, Height: 24, Profile: NoColor, IsTTY: isTTY, Variant: Nebelung}
	return &Printer{Out: &bytes.Buffer{}, Err: errBuf, term: t, theme: NewTheme(t)}, errBuf
}

// A stream with no window is not folded.
//
// This is the bug holt found: its acceptance suite greps stderr for whole
// messages, and every assertion long enough to cross column 80 broke the day it
// moved onto snug — split mid-path, by a width that came from nothing but a
// struct's zero-value neighbour. A pipe has no geometry; inventing one for it is
// the `tput cols` mistake wearing different clothes.
func TestAStreamWithNoWindowIsNeverFolded(t *testing.T) {
	long := "still live at /var/folders/9k/xxxxxxxxxxxxxxxxxxxxxxxxxxxx/T/holt-test.AbCdEf/repo — nothing was rebuilt"
	p, err := printerAt(80, false)
	p.Say("%s", long)
	got := strings.TrimRight(err.String(), "\n")
	if strings.Contains(got, "\n") {
		t.Fatalf("a redirected stream was folded:\n%s", got)
	}
	if !strings.Contains(got, long) {
		t.Fatalf("the message did not survive whole:\n%s", got)
	}
}

// The terminal keeps folding, because that is what folding is for.
func TestATerminalStillFoldsAndHangsAtTheGutter(t *testing.T) {
	p, err := printerAt(40, true)
	p.Say("%s", strings.Repeat("word ", 40))
	lines := strings.Split(strings.TrimRight(err.String(), "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("a 40-column terminal did not fold 200 cells of prose: %q", err.String())
	}
	for i, l := range lines {
		if w := Width(l); w > 40 {
			t.Fatalf("line %d overflowed the window at %d cells: %q", i, w, l)
		}
		if i > 0 && !strings.HasPrefix(l, strings.Repeat(" ", Gutter)) {
			t.Fatalf("continuation %d did not hang at the gutter: %q", i, l)
		}
	}
}

// Data is stdout, and was never folded or painted either way.
func TestDataIsNeverFoldedOnATerminal(t *testing.T) {
	out := &bytes.Buffer{}
	tm := Term{Width: 40, Height: 24, Profile: TrueColor, IsTTY: true, Variant: Nebelung}
	p := &Printer{Out: out, Err: &bytes.Buffer{}, term: tm, theme: NewTheme(tm)}
	path := "/var/folders/9k/xxxxxxxxxxxx/T/holt-test.AbCdEf/repo/deep/enough/to/pass/forty"
	p.Data("%s\n", path)
	if got := out.String(); got != path+"\n" {
		t.Fatalf("stdout was not the bytes handed to it:\n%q", got)
	}
}
