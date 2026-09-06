// Command ansible-config shows go-ansible's configuration.
//
// Scope note: go-ansible reads exactly four settings, each from an
// inventory/host var (where applicable) then an ANSIBLE_* environment
// variable then ansible.cfg's own [defaults] section then a compiled-
// in default — see playbook.ConfigDefaults, the data source for both
// subcommands below: remote_user, host_key_checking, timeout, forks.
// Real ansible-config's `list`/`dump` cover several hundred settings
// across many plugin types; no other section or key of a real
// ansible.cfg is read at all. `view` prints the discovered file
// verbatim when go-ansible would use one (same discovery order as
// real Ansible: $ANSIBLE_CONFIG, ./ansible.cfg, ~/.ansible.cfg,
// /etc/ansible/ansible.cfg), and fails honestly, not silently, when
// none of those exists.
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
		return runView()
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
	cfgPath := playbook.ConfigFilePath()
	for _, s := range playbook.ConfigDefaults() {
		// The real origin, not guessed from Current != Default (which
		// would mislabel a config-file-sourced value as coming from
		// the env var, now that both can make Current differ from
		// Default): check the one this port actually gives priority to
		// first, matching envStr/envBool/envInt's own precedence. A
		// found config file that doesn't happen to set THIS key must
		// not be reported as this setting's origin either.
		origin := "default"
		switch {
		case os.Getenv(s.EnvVar) != "":
			origin = "env: " + s.EnvVar
		default:
			if _, ok := playbook.ConfigFileValueForEnv(s.EnvVar); ok {
				origin = "cfg: " + cfgPath
			}
		}
		fmt.Printf("%s(%s) = %s\n", s.Name, origin, s.Current)
	}
}

func runView() int {
	path := playbook.ConfigFilePath()
	if path == "" {
		fmt.Fprintln(os.Stderr, "ansible-config: no config file found ($ANSIBLE_CONFIG, ./ansible.cfg, ~/.ansible.cfg, /etc/ansible/ansible.cfg) — only inventory vars and the ANSIBLE_* variables listed by \"ansible-config list\" apply")
		return 1
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ansible-config:", err)
		return 1
	}
	os.Stdout.Write(data)
	return 0
}
