// Package termcolor decides whether output should carry ANSI colour,
// by real Ansible's rules rather than by a flag alone.
package termcolor

import (
	"os"

	"golang.org/x/term"
)

// Enabled reports whether to colour output written to f.
//
// Real's rule, measured against ansible-core 2.21.4 rather than read
// off its source:
//
//	redirected                        no colour
//	a terminal                        colour
//	ANSIBLE_FORCE_COLOR=1, redirected colour
//	ANSIBLE_NOCOLOR=1                 no colour
//	NO_COLOR=1                        no colour
//	NO_COLOR=1 and FORCE_COLOR=1      no colour -- refusing wins
//
// noColorFlag is the binary's own --no-color, which forces it off
// whatever the environment says.
//
// This matters more than it looks. Before it, every run of this port
// that was REDIRECTED -- to a log, a file, a CI transcript -- carried
// escape sequences that real would not have written. The differential
// corpus could not see it, because the corpus passed --no-color to
// this side and nothing to real's.
func Enabled(f *os.File, noColorFlag bool) bool {
	if noColorFlag {
		return false
	}
	// A refusal beats a demand: NO_COLOR is a cross-tool convention
	// and real honours it over its own ANSIBLE_FORCE_COLOR.
	if os.Getenv("NO_COLOR") != "" || os.Getenv("ANSIBLE_NOCOLOR") != "" {
		return false
	}
	if os.Getenv("ANSIBLE_FORCE_COLOR") != "" {
		return true
	}
	return term.IsTerminal(int(f.Fd()))
}
