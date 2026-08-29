package main

import (
	"os"
	"syscall"
	"testing"
)

// 128+signum is what a shell reports for a process a signal stopped, and 130 is
// the number a person recognises after ⌃C. Getting it wrong is invisible until
// something reads `$?` — `bench release` exits on its watch's status, and CI
// treats any non-zero as a failure but not every non-zero the same.
func TestSignalExit(t *testing.T) {
	for _, c := range []struct {
		sig  os.Signal
		want int
	}{
		{os.Interrupt, 130},
		{syscall.SIGTERM, 143},
		{syscall.SIGHUP, 129},
		{nil, 130}, // not a syscall.Signal: fall back rather than panic
	} {
		if got := signalExit(c.sig); got != c.want {
			t.Errorf("signalExit(%v) = %d, want %d", c.sig, got, c.want)
		}
	}
}
