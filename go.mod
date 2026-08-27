// snug — how the hausfold family puts a line on screen.
//
// Two dependencies, both load-bearing, and the choice between them and the
// obvious answer is written down in AGENTS.md: `x/ansi` is charm's ANSI-aware
// string layer WITHOUT lipgloss's styling engine (9 modules in the graph
// against lipgloss v2's 22, 2.1 MB against 3.0), and the family's look is
// quiet — aligned text and a fog palette, no borders and no boxes. The parts of
// lipgloss we would actually use are the parts x/ansi already is.
//
// `x/term` is the only honest way to ask the kernel how wide the window is.
// `tput cols` reads terminfo's STATIC size and answers 80 in a 40-column
// window; that is the bug this whole library exists because of.
module github.com/hausfold/snug

go 1.26

require (
	github.com/charmbracelet/x/ansi v0.11.8
	golang.org/x/term v0.45.0
)

require (
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.0 // indirect
	github.com/mattn/go-runewidth v0.0.24 // indirect
	golang.org/x/sys v0.47.0 // indirect
)
