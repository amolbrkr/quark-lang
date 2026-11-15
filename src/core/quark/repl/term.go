package repl

import (
	"os"

	"golang.org/x/term"
)

// IsTerminal reports whether f is a terminal, where line editing applies.
func IsTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}
