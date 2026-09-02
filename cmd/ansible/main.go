// Command ansible runs a single module against a pattern of inventory
// hosts (Ansible's "ad-hoc" mode).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/go-ansible/inventory"
	"github.com/go-ansible/modules"
	"github.com/go-ansible/playbook"
	remoteexec "github.com/go-remoteexec/transport"
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

	moduleArgsMap := parseModuleArgs(*moduleName, *moduleArgs)
	registry := modules.Default()

	var wg sync.WaitGroup
	var mu sync.Mutex
	anyFailed := false
	ctx := context.Background()

	for _, h := range hosts {
		wg.Add(1)
		go func(hostName string) {
			defer wg.Done()
			hostVars := inv.HostVars(hostName)

			conn, err := playbook.DefaultConnect(ctx, hostName, hostVars)
			if err != nil {
				mu.Lock()
				fmt.Printf("%s | UNREACHABLE => %s\n", hostName, err)
				anyFailed = true
				mu.Unlock()
				return
			}
			defer conn.Close()

			var target remoteexec.Connection = conn
			if *become {
				target = remoteexec.Become(conn, remoteexec.BecomeConfig{})
			}

			res, err := registry.Run(ctx, *moduleName, target, moduleArgsMap)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				fmt.Printf("%s | UNREACHABLE => %s\n", hostName, err)
				anyFailed = true
				return
			}
			status := "SUCCESS"
			if res.Failed {
				status = "FAILED"
				anyFailed = true
			} else if res.Changed {
				status = "SUCCESS (changed)"
			}
			payload := map[string]any{"changed": res.Changed, "msg": res.Msg}
			for k, v := range res.Extra {
				payload[k] = v
			}
			enc, _ := json.MarshalIndent(payload, "", "    ")
			fmt.Printf("%s | %s => %s\n", hostName, status, enc)
		}(h.Name)
	}
	wg.Wait()

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

// parseModuleArgs builds a module's argument map from the -a string.
// command/shell (and any module taking a single free-form string) get
// the whole string as _raw_params; every other module parses it as
// space-separated key=value pairs, Ansible's ad-hoc convention.
func parseModuleArgs(module, argsStr string) map[string]any {
	switch module {
	case "command", "shell":
		return map[string]any{"_raw_params": argsStr}
	}
	out := map[string]any{}
	for _, pair := range strings.Fields(argsStr) {
		key, val, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		out[key] = val
	}
	return out
}
