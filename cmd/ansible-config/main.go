// Command ansible-config shows go-ansible's configuration.
//
// Scope note: go-ansible honours seven settings, each read from an
// inventory/host var (where applicable) then an ANSIBLE_* environment
// variable then ansible.cfg's own [defaults] section then a compiled-in
// default — see playbook.ConfigDefaults, the data source for both
// subcommands below. Real ansible-config's list/dump cover 219 settings
// across many plugin types; the ones absent here are absent because
// this port does not honour them, and listing a setting that changes
// nothing would say less than saying nothing.
//
// `dump` matches real's output line for line for the settings both know:
// real's canonical name, the source in parentheses, and the value in
// PYTHON's spelling. `list` matches real's YAML shape but NOT its
// descriptions: real's are GPL-licensed prose and these repositories are
// BSD-licensed, so each setting carries this port's own wording instead.
//
// `view` prints the discovered file verbatim when go-ansible would use
// one (same discovery order as real Ansible: $ANSIBLE_CONFIG,
// ./ansible.cfg, ~/.ansible.cfg, /etc/ansible/ansible.cfg), and fails
// honestly, not silently, when none of those exists.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/go-ansible/cli/internal/version"
	"github.com/go-ansible/playbook"

	"github.com/go-ansible/cli/internal/usage"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	// -h/--help is a SUCCESSFUL request: real answers it on stdout with
	// status 0, where a usage ERROR goes to stderr with status 2.
	if usage.Wanted(args) {
		usage.Answer(usageText)
		return 0
	}
	if len(args) > 0 && (args[0] == "--version" || args[0] == "-version") {
		fmt.Println(version.String("ansible-config"))
		return 0
	}
	if len(args) < 1 {
		usageText(os.Stderr)
		return 2
	}

	switch args[0] {
	case "list":
		printList()
		return 0
	case "dump":
		// Real filters to the settings whose value did not come from
		// the compiled-in default.
		printDump(hasFlag(args[1:], "--only-changed", "--only_changed"))
		return 0
	case "view":
		return runView()
	default:
		fmt.Fprintf(os.Stderr, "ansible-config: unknown subcommand: %s\n", args[0])
		usageText(os.Stderr)
		return 2
	}
}

func usageText(w io.Writer) {
	fmt.Fprintln(w, "usage: ansible-config {list|dump|view}")
}

func printList() {
	// Real's shape, measured: one top-level YAML key per setting, its
	// fields indented two spaces and in alphabetical order, with env and
	// ini as LISTS because a setting may have several of each.
	for _, st := range playbook.ConfigDefaults() {
		fmt.Printf("%s:\n", st.Name)
		fmt.Printf("  default: %s\n", yamlScalar(st.Default))
		fmt.Printf("  description: %s\n", yamlScalar(st.Description))
		fmt.Printf("  env:\n  - name: %s\n", st.EnvVar)
		fmt.Printf("  ini:\n  - key: %s\n    section: %s\n", st.IniKey, st.IniSection)
		// Real carries a short human title here. Ours is derived from
		// the canonical name rather than copied: real's titles are part
		// of the same GPL-licensed prose as its descriptions, and the
		// point of this field is the SHAPE -- a consumer parsing the
		// YAML needs the key to exist.
		fmt.Printf("  name: %s\n", humanTitle(st.Name))
		fmt.Printf("  type: %s\n", st.Type)
		if st.VersionAdded != "" {
			fmt.Printf("  version_added: '%s'\n", st.VersionAdded)
		}
	}
}

// humanTitle turns DEFAULT_REMOTE_USER into "Default remote user".
func humanTitle(name string) string {
	words := strings.ToLower(strings.ReplaceAll(name, "_", " "))
	if words == "" {
		return words
	}
	return strings.ToUpper(words[:1]) + words[1:]
}

// yamlScalar quotes a value only where YAML needs it. Real's dump of a
// boolean default is the bare word true, not 'true', and its
// descriptions are bare unless they would parse as something else.
func yamlScalar(v string) string {
	switch v {
	case "True":
		return "true"
	case "False":
		return "false"
	case "None":
		return "null"
	case "":
		return "''"
	}
	if strings.ContainsAny(v, ":#{}[]&*!|>'\"%@`") {
		return "'" + strings.ReplaceAll(v, "'", "''") + "'"
	}
	return v
}

// printDump writes real's own dump format: NAME(source) = value, sorted
// by name, with the value spelled the way Python spells it. onlyChanged
// keeps just the settings that did not come from the default.
func printDump(onlyChanged bool) {
	// playbook.ConfigDefaults already decided each value and where it
	// came from, environment over ansible.cfg over the compiled-in
	// default, and spells the origin the way real spells it. This is a
	// printer, which is all its documentation ever claimed.
	for _, st := range playbook.ConfigDefaults() {
		if onlyChanged && st.Source == "default" {
			continue
		}
		fmt.Printf("%s(%s) = %s\n", st.Name, st.Source, st.Current)
	}
}

// hasFlag reports whether any of the given spellings appears in args.
func hasFlag(args []string, names ...string) bool {
	for _, a := range args {
		for _, n := range names {
			if a == n {
				return true
			}
		}
	}
	return false
}

func runView() int {
	path := playbook.ConfigFilePath()
	if path == "" {
		fmt.Fprintln(os.Stderr, "ansible-config: no config file found ($ANSIBLE_CONFIG, ./ansible.cfg, ~/.ansible.cfg, /etc/ansible/ansible.cfg) — only inventory vars and the ANSIBLE_* variables listed by \"ansible-config list\" apply")
		return 1
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]:", err)
		return 1
	}
	os.Stdout.Write(data)
	return 0
}
