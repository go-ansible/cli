// Command ansible runs a single module against a pattern of inventory
// hosts (Ansible's "ad-hoc" mode).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/go-ansible/cli/internal/adhoc"
	"github.com/go-ansible/cli/internal/invload"
	"github.com/go-ansible/cli/internal/version"
	"github.com/go-ansible/playbook"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("ansible", flag.ContinueOnError)
	inventoryPath := fs.String("i", "", "inventory file or directory (also --inventory)")
	fs.StringVar(inventoryPath, "inventory", "", "inventory file or directory")
	moduleName := fs.String("m", "command", "module to run (also --module-name)")
	fs.StringVar(moduleName, "module-name", "command", "module to run")
	moduleArgs := fs.String("a", "", "module arguments (also --args)")
	fs.StringVar(moduleArgs, "args", "", "module arguments")
	become := fs.Bool("b", false, "run with privilege escalation (also --become)")
	fs.BoolVar(become, "become", false, "run with privilege escalation")
	showVersion := fs.Bool("version", false, "print the version and exit")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: ansible PATTERN -i INVENTORY -m MODULE [-a ARGS] [-b]")
		fs.PrintDefaults()
	}
	// Ansible's real invocation puts the pattern first (`ansible all -i
	// ... -m ...`), but Go's flag package stops parsing at the first
	// non-flag token, treating everything after it as positional. Pull
	// the pattern out wherever it appears so flags can surround it
	// either way.
	pattern, flagArgs, err := extractPattern(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]:", err)
		fs.Usage()
		return 2
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Println(version.String("ansible"))
		return 0
	}
	// -i is NOT required: `ansible localhost -m ping` with no
	// inventory at all is one of real's most ordinary invocations, and
	// it works there because the implicit localhost always exists.
	if pattern == "" {
		fs.Usage()
		return 2
	}

	// One Warner for the process, as real has one Display.
	warn := playbook.NewWarner(os.Stderr)

	// An unusable inventory is a warning, not a failure — see invload.
	// Ad-hoc resolves against the USER's pattern, not "all", which is
	// what suppresses the empty-inventory warning for `localhost`.
	inv := invload.Load(*inventoryPath, "", pattern, warn)

	hosts, unmatched, err := inv.MatchReport(pattern)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]:", err)
		return 1
	}
	for _, term := range unmatched {
		warn("Could not match supplied host pattern, ignoring: " + term)
	}
	// A pattern that matched nothing is not an error: real warns and
	// exits 0, having run the module on no hosts. Measured —
	// `ansible -i /nope.ini h1 -m ping` exits 0 there and 1 here.
	if len(hosts) == 0 {
		return 0
	}

	moduleArgsMap := adhoc.ParseModuleArgs(*moduleName, *moduleArgs)
	hostNames := make([]string, len(hosts))
	for i, h := range hosts {
		hostNames[i] = h.Name
	}

	anyFailed := adhoc.Run(context.Background(), inv, hostNames, *moduleName, moduleArgsMap, *become, os.Stdout)
	if anyFailed {
		return 2
	}
	return 0
}

// flagsWithValue names every flag (short and long form) that consumes
// the following token as its value, so extractPattern can skip over
// `-i inventory.yml`-style pairs without mistaking the value for the
// pattern.
var flagsWithValue = map[string]bool{
	"-i": true, "-inventory": true, "--inventory": true,
	"-m": true, "-module-name": true, "--module-name": true,
	"-a": true, "-args": true, "--args": true,
}

// extractPattern pulls the single non-flag token (Ansible's host
// pattern) out of args, wherever it appears, returning it separately
// from the remaining flag tokens (in their original relative order, so
// flag.FlagSet.Parse still sees valid `-flag value` pairs).
func extractPattern(args []string) (pattern string, flagArgs []string, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flagArgs = append(flagArgs, a)
			if flagsWithValue[a] && !strings.Contains(a, "=") {
				i++
				if i >= len(args) {
					return "", nil, fmt.Errorf("%s requires a value", a)
				}
				flagArgs = append(flagArgs, args[i])
			}
			continue
		}
		if pattern != "" {
			return "", nil, fmt.Errorf("unexpected extra argument %q (pattern already given as %q)", a, pattern)
		}
		pattern = a
	}
	return pattern, flagArgs, nil
}
