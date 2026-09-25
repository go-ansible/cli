// Command ansible-inventory shows the inventory as this port parsed it,
// in real ansible-inventory's own shapes: the --list JSON dump, the
// --host variable dump, and the --graph tree.
//
// Scope note: --yaml and --toml are not implemented. They are real
// output formats and they are refused explicitly rather than ignored,
// so a command line asking for one fails instead of silently getting
// JSON it did not ask for.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/go-ansible/cli/internal/invload"
	"github.com/go-ansible/cli/internal/version"
	"github.com/go-ansible/inventory"
	"github.com/go-ansible/playbook"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// noActionExit is real's exit status when no action flag is given. It
// is 5, not 1 or 2 — measured, and different from every other binary
// here, which is why it is named rather than written inline.
const noActionExit = 5

func run(args []string) int {
	fs := flag.NewFlagSet("ansible-inventory", flag.ContinueOnError)
	inventoryPath := fs.String("i", "", "inventory file or directory (also --inventory)")
	fs.StringVar(inventoryPath, "inventory", "", "inventory file or directory")
	list := fs.Bool("list", false, "output all hosts info, works as inventory script")
	host := fs.String("host", "", "output specific host info, works as inventory script")
	graph := fs.Bool("graph", false, "create inventory graph, if supplying pattern it must be a valid group name")
	showVars := fs.Bool("vars", false, "add vars to graph display, ignored unless used with --graph")
	export := fs.Bool("export", false, "when doing an --list, represent in a way that is optimized for export, not as an accurate representation of how Ansible has processed it")
	output := fs.String("output", "", "when doing --list, send the inventory to a file instead of to the screen")
	limit := fs.String("limit", "", "further limit selected hosts to an additional pattern (also -l)")
	fs.StringVar(limit, "l", "", "further limit selected hosts to an additional pattern")
	yamlOut := fs.Bool("yaml", false, "use YAML format instead of default JSON (not implemented)")
	tomlOut := fs.Bool("toml", false, "use TOML format instead of default JSON (not implemented)")
	showVersion := fs.Bool("version", false, "print the version and exit")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: ansible-inventory [-i INVENTORY] --list | --host HOST | --graph")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Println(version.String("ansible-inventory"))
		return 0
	}
	for flagName, on := range map[string]bool{"--yaml": *yamlOut, "--toml": *tomlOut} {
		if on {
			fmt.Fprintf(os.Stderr, "[ERROR]: %s is not implemented by this port; only the default JSON output is available\n", flagName)
			return 1
		}
	}
	// Real checks this BEFORE reading the inventory, so a bad -i with
	// no action still reports the missing action. Measured: exit 5.
	if !*list && !*graph && *host == "" {
		fmt.Fprintln(os.Stderr, "[ERROR]: No action selected, at least one of --host, --graph or --list needs to be specified.")
		return noActionExit
	}

	warn := playbook.NewWarner(os.Stderr)
	// The empty pattern: ansible-inventory resolves no host list of
	// its own, so the "provided hosts list is empty" warning that
	// ansible-playbook emits does not belong here. Measured.
	inv := invload.Load(*inventoryPath, "", "", warn)

	switch {
	case *host != "":
		return dumpHost(inv, *host, warn)
	case *graph:
		return writeOut(*output, graphText(inv, *showVars))
	default:
		return dumpList(inv, *limit, *export, *output, warn)
	}
}

// selected is the set of host names the dump covers: every host, or
// those matching --limit. A limit term matching nothing warns exactly
// as it does everywhere else.
func selected(inv *inventory.Inventory, limit string, warn playbook.Warner) (map[string]bool, error) {
	if limit == "" {
		return nil, nil // nil means "no filter", distinct from "matched nothing"
	}
	hosts, unmatched, err := inv.MatchReport(limit)
	if err != nil {
		return nil, err
	}
	for _, term := range unmatched {
		warn("Could not match supplied host pattern, ignoring: " + term)
	}
	keep := make(map[string]bool, len(hosts))
	for _, h := range hosts {
		keep[h.Name] = true
	}
	return keep, nil
}

func dumpList(inv *inventory.Inventory, limit string, export bool, output string, warn playbook.Warner) int {
	keep, err := selected(inv, limit, warn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]:", err)
		return 1
	}

	// A host with no variables at all is left out of hostvars
	// entirely rather than carried as an empty object — measured
	// against an inventory whose only host had none.
	hostvars := map[string]any{}
	for name := range inv.Hosts {
		if keep != nil && !keep[name] {
			continue
		}
		vars := inv.HostVars(name)
		if export {
			// --export reports what the inventory SAYS rather than
			// what a play would see, so a host carries only its own
			// vars and the groups keep theirs.
			vars = inv.Hosts[name].Vars
		}
		if len(vars) > 0 {
			hostvars[name] = vars
		}
	}

	out := map[string]any{
		"_meta": map[string]any{
			"hostvars": hostvars,
			// Real 2.21 stamps the dump with the variable-precedence
			// profile it used. This port implements the legacy one,
			// which is also real's default.
			"profile": "inventory_legacy",
		},
	}
	for name, g := range inv.Groups {
		entry := map[string]any{}
		if children := g.ChildNames(); len(children) > 0 {
			// Structural, so NOT filtered by --limit: real keeps a
			// child listed even when the limit emptied it.
			entry["children"] = children
		}
		if hosts := groupHostNames(inv, g, keep); len(hosts) > 0 {
			entry["hosts"] = hosts
		}
		if export && len(g.Vars) > 0 {
			entry["vars"] = g.Vars
		}
		// A group with nothing to say is omitted, which is why an
		// empty "ungrouped" never appears even though it always
		// exists.
		if len(entry) > 0 {
			out[name] = entry
		}
	}
	return writeOut(output, encodeJSON(out))
}

// groupHostNames lists a group's direct hosts in INVENTORY order —
// the order the inventory introduced them, which is the order real
// dumps them in, not alphabetical.
func groupHostNames(inv *inventory.Inventory, g *inventory.Group, keep map[string]bool) []string {
	var names []string
	for name := range g.Hosts {
		if keep != nil && !keep[name] {
			continue
		}
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return inv.HostIndex(names[i]) < inv.HostIndex(names[j])
	})
	return names
}

func dumpHost(inv *inventory.Inventory, name string, warn playbook.Warner) int {
	hosts, unmatched, err := inv.MatchReport(name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]:", err)
		return 1
	}
	for _, term := range unmatched {
		warn("Could not match supplied host pattern, ignoring: " + term)
	}
	if len(hosts) != 1 {
		// Real says "a single valid host", and means it: a pattern
		// matching several is refused the same way one matching none
		// is.
		fmt.Fprintln(os.Stderr, "[ERROR]: You must pass a single valid host to --host parameter")
		return noActionExit
	}
	vars := inv.HostVars(hosts[0].Name)
	if vars == nil {
		vars = map[string]any{}
	}
	fmt.Print(encodeJSON(vars))
	return 0
}

// graphText renders the tree real's --graph prints: every group as
// "@name:", its children first, then its own hosts, then (with --vars)
// its variables, each level two spaces further in behind "|  " rails.
func graphText(inv *inventory.Inventory, showVars bool) string {
	var b strings.Builder
	var walk func(g *inventory.Group, depth int)
	walk = func(g *inventory.Group, depth int) {
		fmt.Fprintf(&b, "%s@%s:\n", graphPrefix(depth), g.Name)
		for _, childName := range g.ChildNames() {
			if child := inv.Groups[childName]; child != nil {
				walk(child, depth+1)
			}
		}
		for _, name := range groupHostNames(inv, g, nil) {
			fmt.Fprintf(&b, "%s%s\n", graphPrefix(depth+1), name)
			if showVars {
				writeGraphVars(&b, inv.HostVars(name), depth+2)
			}
		}
		if showVars {
			writeGraphVars(&b, g.Vars, depth+1)
		}
	}
	if all := inv.Groups["all"]; all != nil {
		walk(all, 0)
	}
	return b.String()
}

func writeGraphVars(b *strings.Builder, vars map[string]any, depth int) {
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(b, "%s{%s = %v}\n", graphPrefix(depth), k, vars[k])
	}
}

// graphPrefix is the indentation for one depth of the graph. Depth 0
// (the "all" group) has none; every deeper level is two spaces, then
// one "|  " rail per level above it, then "|--".
func graphPrefix(depth int) string {
	if depth == 0 {
		return ""
	}
	return "  " + strings.Repeat("|  ", depth-1) + "|--"
}

// encodeJSON matches Python's json.dumps(..., indent=4, sort_keys=True)
// as real calls it. Go sorts map keys already; what it does NOT do by
// default is leave HTML alone, and real never escapes < > & — so a
// variable holding one would diverge without SetEscapeHTML(false).
func encodeJSON(v any) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "    ")
	if err := enc.Encode(v); err != nil {
		return ""
	}
	return b.String()
}

func writeOut(path, text string) int {
	if path == "" {
		fmt.Print(text)
		return 0
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]:", err)
		return 1
	}
	return 0
}
