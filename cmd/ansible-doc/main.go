// Command ansible-doc prints documentation for go-ansible's modules.
//
// Scope note: real ansible-doc reads a structured DOCUMENTATION YAML
// block every module carries (synopsis, per-argument type/required/
// default/choices, examples, return values) and can also document
// plugins beyond modules (lookups, filters, callbacks — none of which
// this port has a registry for). This port's modules were never
// written with that structured metadata; what they do have, on every
// one of them, is a Go doc comment documenting arguments and every
// deviation from real Ansible's behavior — this project's convention
// throughout. ansible-doc here prints that comment verbatim rather
// than a synthesized DOCUMENTATION-shaped block: real content, just
// not real Ansible's exact rendering.
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/go-ansible/cli/internal/version"
	"github.com/go-ansible/modules"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) > 0 && (args[0] == "--version" || args[0] == "-version") {
		fmt.Println(version.String("ansible-doc"))
		return 0
	}

	var list bool
	var names []string
	for _, a := range args {
		switch a {
		case "-l", "--list":
			list = true
		default:
			if len(a) > 0 && a[0] == '-' {
				fmt.Fprintln(os.Stderr, "ansible-doc: unrecognized argument:", a)
				usage()
				return 2
			}
			names = append(names, a)
		}
	}

	r := modules.Default()

	if list {
		if len(names) > 0 {
			fmt.Fprintln(os.Stderr, "ansible-doc: -l/--list takes no module names")
			return 2
		}
		printList(r)
		return 0
	}

	if len(names) == 0 {
		usage()
		return 2
	}

	code := 0
	for i, name := range names {
		if i > 0 {
			fmt.Println()
		}
		if _, ok := r.Get(name); !ok {
			fmt.Fprintf(os.Stderr, "ansible-doc: %s: no such module\n", name)
			code = 1
			continue
		}
		printModule(name)
	}
	return code
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: ansible-doc [-l | --list] [MODULE ...]")
}

func printList(r *modules.Registry) {
	names := r.Names()
	sort.Strings(names)
	width := 0
	for _, n := range names {
		if len(n) > width {
			width = len(n)
		}
	}
	for _, n := range names {
		fmt.Printf("%-*s  %s\n", width, n, firstLine(modules.Docs[n]))
	}
}

func printModule(name string) {
	fmt.Printf("> %s\n\n%s", strings.ToUpper(name), modules.Docs[name])
}

// firstLine returns doc's first non-empty line, trimmed — a short
// synopsis for the -l/--list table, since these doc comments open
// with a one-line "module<Name> implements ..." sentence.
func firstLine(doc string) string {
	for _, line := range strings.Split(doc, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}
