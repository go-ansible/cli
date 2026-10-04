package verbosity

import (
	"flag"
	"io"
	"testing"
)

// TestRegisterMatchesRealsAcceptedForms pins the forms real ansible
// accepts. All of these exit 0 against ansible-core 2.21.4, and a bogus
// -w exits 2 -- so the acceptance is the flag being recognised rather
// than silently ignored.
//
// The level each form reaches is what the table asserts, not merely that
// it parsed: a flag accepted and dropped on the floor is the failure
// this is guarding against.
func TestRegisterMatchesRealsAcceptedForms(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want int
	}{
		{nil, 0},
		{[]string{"-v"}, 1},
		{[]string{"--verbose"}, 1},
		{[]string{"-v", "-v"}, 2},
		{[]string{"-v", "-v", "-v"}, 3},
		{[]string{"-vv"}, 2},
		{[]string{"-vvv"}, 3},
		{[]string{"-vvvv"}, 4},
		{[]string{"-vvvvvv"}, 6},
		// ADDITIVE, because real's -v is argparse action="count" and
		// its short-option bundling makes -vvv three occurrences. My
		// first version raised to a maximum instead -- a nicer rule
		// real does not have.
		{[]string{"-vvv", "-v"}, 4},
		{[]string{"-vvvvvvv"}, 7},
	} {
		t.Run(join(tc.args), func(t *testing.T) {
			fs := flag.NewFlagSet("t", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			n := Register(fs)
			if err := fs.Parse(tc.args); err != nil {
				t.Fatalf("real accepts %v; we refused it: %v", tc.args, err)
			}
			if *n != tc.want {
				t.Errorf("level = %d, want %d", *n, tc.want)
			}
		})
	}
}

// A bogus flag must still be refused -- real exits 2 for -w. Registering
// a family of names must not turn into accepting anything.
func TestABogusFlagIsStillRefused(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	Register(fs)
	if err := fs.Parse([]string{"-w"}); err == nil {
		t.Error("-w was accepted; real exits 2 for it")
	}
	// A run past the registered ceiling of ten is refused where real
	// accepts it. Recorded here rather than left for someone to find:
	// real's own help says its plugins evaluate up to -vvvvvv, so ten
	// is well past anything that changes output.
	if err := fs.Parse([]string{"-vvvvvvvvvvv"}); err == nil {
		t.Error("an 11-long run parsed; the registration goes to ten, so this would be a surprise")
	}
}

func join(args []string) string {
	if len(args) == 0 {
		return "no flags"
	}
	s := ""
	for i, a := range args {
		if i > 0 {
			s += " "
		}
		s += a
	}
	return s
}
