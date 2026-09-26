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
//	NO_COLOR=1 and FORCE_COLOR=1      COLOUR -- the demand wins
//	ANSIBLE_NOCOLOR=1 and FORCE=1     COLOUR -- likewise
//
// That last pair is the opposite of what this file said a version
// ago, and the correction is worth recording. The first measurement
// passed both variables as ONE unquoted shell parameter, and zsh does
// not word-split an unquoted parameter: env received a single
// malformed assignment, NO_COLOR was never set, and the run was read
// as "refusing wins". The binary corpus, which passes them as
// separate words, disagreed within the hour.
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
	// The DEMAND beats the refusal, measured both ways round. Real
	// checks ANSIBLE_FORCE_COLOR first and returns on it, so NO_COLOR
	// never gets a say -- whatever one might expect of the cross-tool
	// convention.
	if os.Getenv("ANSIBLE_FORCE_COLOR") != "" {
		return true
	}
	if os.Getenv("NO_COLOR") != "" || os.Getenv("ANSIBLE_NOCOLOR") != "" {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}
