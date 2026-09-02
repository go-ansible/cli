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
	"github.com/go-ansible/cli/internal/version"
	"github.com/go-ansible/inventory"
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
		fmt.Fprintln(os.Stderr, "ansible:", err)
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
	if *inventoryPath == "" || pattern == "" {
		fs.Usage()
		return 2
	}

	inv, err := inventory.Load(*inventoryPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ansible:", err)
		return 1
	}
	hosts, err := inv.Match(pattern)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ansible:", err)
		return 1
	}
	if len(hosts) == 0 {
		fmt.Fprintf(os.Stderr, "ansible: pattern %q matched no hosts\n", pattern)
		return 1
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
