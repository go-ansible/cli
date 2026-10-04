// Package verbosity registers real ansible's -v family on a flag.FlagSet.
//
// Go's flag package has no counting flag, and real accepts the count
// three ways -- measured against ansible-core 2.21.4, all of which exit
// 0: `-v`, `-vv` (and longer runs, up to at least `-vvvvvv`), repeated
// `-v -v`, and `--verbose`. A bogus `-w` exits 2, so the acceptance
// above is the flag being recognised rather than ignored.
//
// (One measurement here was wrong the first time: a loop passing "$f"
// unquoted made zsh hand `-v -v` over as ONE argument, which real
// rejected -- and I nearly recorded "real refuses a repeated -v".
// Passing the arguments as "$@" showed it counts to 2.)
package verbosity

import (
	"flag"
	"strconv"
)

// Register adds the -v family to fs and returns the counter they write
// to. Read it after fs.Parse.
func Register(fs *flag.FlagSet) *int {
	n := new(int)
	// -v counts: passed twice it reaches 2, like real's own count
	// action. The longer runs are separate flag NAMES because Go parses
	// `-vv` as the flag "vv", not as two "v"s.
	fs.Var(&counter{n: n, weight: 1}, "v",
		"verbose mode: an ok/changed line carries the whole result (repeatable; also -vv.., --verbose)")
	// Up to ten, where real's own help says "the builtin plugins
	// currently evaluate up to -vvvvvv". Real accepts an arbitrarily
	// long run; stopping at six would refuse a `-vvvvvvv` that real
	// takes, and the extra registrations cost nothing.
	for run := 2; run <= 10; run++ {
		name := ""
		for i := 0; i < run; i++ {
			name += "v"
		}
		// ⚠ Higher levels are ACCEPTED but add nothing beyond -v, and
		// the help text says so rather than letting a reader infer that
		// the output is complete: real's -vv prefixes
		// "task path: <file>:<line>", which needs source positions
		// go-ansible/playbook's parser does not keep, and -vvv adds
		// connection tracing. Rejecting the flags outright would break
		// every habitual `ansible-playbook -vv`, which is worse.
		fs.Var(&counter{n: n, weight: run}, name,
			"as -v; the extra levels are accepted but add nothing more (no task paths, no connection tracing)")
	}
	fs.Var(&counter{n: n, weight: 1}, "verbose", "as -v")
	return n
}

// counter is a boolean-style flag.Value that ADDS its weight to the
// shared count.
//
// Additive rather than "raise to at least", because that is what real
// does and the difference is observable: real declares
// `-v/--verbose … action="count"`, and argparse's short-option bundling
// makes `-vvv` three occurrences of -v, so `-vvv -v` is FOUR there. My
// first version raised to a maximum instead, on the reasoning that a
// run states a level -- a nicer rule that real does not have, and a
// nicer rule is still a divergence. (The probe could not settle it: the
// version banner real prints is the only visible marker and it
// saturates at level 2, so the authority here is real's own argparse
// declaration, not a measurement.)
type counter struct {
	n      *int
	weight int
}

func (c *counter) String() string   { return strconv.Itoa(c.weight) }
func (c *counter) IsBoolFlag() bool { return true }

func (c *counter) Set(s string) error {
	// A bool-style flag is Set("true") when given alone, and
	// Set("<value>") for the -v=false form the flag package allows.
	if s == "false" {
		return nil
	}
	*c.n += c.weight
	return nil
}
