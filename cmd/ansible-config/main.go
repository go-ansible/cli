// Command ansible-config shows go-ansible's configuration.
//
// Scope note: go-ansible reads no ansible.cfg file at all — there is
// no config-file layer anywhere in this port, only inventory/host
// vars and a small, fixed set of ANSIBLE_* environment variables (see
// playbook.ConfigDefaults, the data source for both subcommands
// below). Real ansible-config's `list`/`dump` cover several hundred
// settings across many plugin types; this one covers exactly the
// three go-ansible actually reads: remote_user, host_key_checking,
// timeout. `view` always fails, honestly: there is no config file to
// show, unlike real ansible-config, which shows an empty/default
// config when none exists.
package main

import (
	"fmt"
	"os"

	"github.com/go-ansible/cli/internal/version"
	"github.com/go-ansible/playbook"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) > 0 && (args[0] == "--version" || args[0] == "-version") {
		fmt.Println(version.String("ansible-config"))
		return 0
	}
	if len(args) < 1 {
		usage()
		return 2
	}

	switch args[0] {
	case "list":
		printList()
		return 0
	case "dump":
		printDump()
		return 0
	case "view":
		fmt.Fprintln(os.Stderr, "ansible-config: no config file — go-ansible reads no ansible.cfg at all, only inventory vars and the ANSIBLE_* variables listed by \"ansible-config list\"")
		return 1
	default:
		fmt.Fprintf(os.Stderr, "ansible-config: unknown subcommand: %s\n", args[0])
		usage()
		return 2
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: ansible-config {list|dump|view}")
}

func printList() {
	for _, s := range playbook.ConfigDefaults() {
		fmt.Printf("%s:\n", s.Name)
		fmt.Printf("  env: %s\n", s.EnvVar)
		fmt.Printf("  default: %s\n\n", s.Default)
	}
}

func printDump() {
	for _, s := range playbook.ConfigDefaults() {
		origin := "default"
		if s.Current != s.Default {
			origin = "env: " + s.EnvVar
		}
		fmt.Printf("%s(%s) = %s\n", s.Name, origin, s.Current)
	}
}
