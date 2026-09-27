// Package usage answers a request for help the way real Ansible does.
package usage

import (
	"io"
	"os"
)

// Wanted reports whether these arguments ask for help.
//
// Measured across all nine of real's binaries: -h and --help are a
// SUCCESSFUL request, answered on STDOUT with exit status 0 and nothing
// on stderr. A usage ERROR -- an unknown flag, or no arguments where
// some are required -- is the opposite: usage on STDERR with status 2
// and nothing on stdout.
//
// This port conflated the two, because Go's flag package treats an
// undefined -h as a parse error: every binary answered --help with
// status 2 and an empty stdout, which is what a script checking `cmd
// --help` sees as a broken command.
//
// Anything after a bare "--" is an operand rather than a flag, so a
// file genuinely named --help is not a request for help.
func Wanted(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == "-h" || a == "--help" {
			return true
		}
	}
	return false
}

// Answer writes the command's usage to stdout, for the caller to return
// 0 after. The writer is passed in rather than assumed so the same
// usage function serves both paths: stdout here, stderr for an error.
func Answer(print func(io.Writer)) {
	print(os.Stdout)
}
