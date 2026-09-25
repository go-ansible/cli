package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// defaultRolesPaths are the directories real searches when --roles-path
// is not given, in this order.
var defaultRolesPaths = []string{
	filepath.Join(os.Getenv("HOME"), ".ansible", "roles"),
	"/usr/share/ansible/roles",
	"/etc/ansible/roles",
}

// rolesSearchPath is what real searches: the explicit --roles-path
// first when one was given, then the defaults. The defaults are
// searched EVEN WITH -p, which is why `list -p somewhere` still warns
// about the three that do not exist.
func rolesSearchPath(explicit string) []string {
	if explicit == "" {
		return defaultRolesPaths
	}
	return append([]string{explicit}, defaultRolesPaths...)
}

// installedRole is one role found on disk.
type installedRole struct {
	name    string
	version string
}

// rolesIn lists the roles installed in dir. A subdirectory counts as a
// role only when it holds meta/main.yml — measured: a plain directory
// is not listed, and neither is one that has only a
// meta/.galaxy_install_info.
func rolesIn(dir string) []installedRole {
	f, err := os.Open(dir)
	if err != nil {
		return nil
	}
	defer f.Close()
	// Readdirnames, NOT os.ReadDir, and no sort afterwards: real lists
	// roles in the order the filesystem hands them over (os.listdir),
	// which is neither alphabetical nor stable across filesystems.
	// Measured on this one, real printed gversion, withversion,
	// myrole, other.role where sorting gives gversion, myrole,
	// other.role, withversion.
	//
	// It reads like a missing sort. It is not: os.ReadDir sorts by
	// name, which is exactly what has to be avoided here for the
	// output to match.
	names, err := f.Readdirnames(-1)
	if err != nil {
		return nil
	}
	var out []installedRole
	for _, name := range names {
		fi, err := os.Stat(filepath.Join(dir, name))
		if err != nil || !fi.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, name, "meta", "main.yml")); err != nil {
			continue
		}
		out = append(out, installedRole{name: name, version: roleVersion(dir, name)})
	}
	return out
}

// roleVersion reads meta/.galaxy_install_info, which is where the
// installer records what it fetched. Measured: a version declared
// under galaxy_info in meta/main.yml is NOT used — a role carrying
// `galaxy_info: {version: 9.9.9}` still lists as "(unknown version)".
func roleVersion(dir, name string) string {
	data, err := os.ReadFile(filepath.Join(dir, name, "meta", ".galaxy_install_info"))
	if err != nil {
		return ""
	}
	var info struct {
		Version string `yaml:"version"`
	}
	if err := yaml.Unmarshal(data, &info); err != nil {
		return ""
	}
	return info.Version
}

func (r installedRole) String() string {
	v := r.version
	if v == "" {
		v = "(unknown version)"
	}
	return "- " + r.name + ", " + v
}

// runList is `ansible-galaxy list [role]`. It always exits 0, even
// when it found nothing at all: real reports absence through warnings
// rather than through the exit status.
func runList(args []string) int {
	name, explicit, err := parseListFlags(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]:", err)
		return 2
	}
	paths := rolesSearchPath(explicit)

	// A named role short-circuits: real prints the first path that
	// holds it and warns about nothing, because it never reaches the
	// paths after it.
	if name != "" {
		for _, p := range paths {
			if _, err := os.Stat(filepath.Join(p, name, "meta", "main.yml")); err == nil {
				fmt.Println("# " + abs(p))
				fmt.Println(installedRole{name: name, version: roleVersion(p, name)}.String())
				return 0
			}
		}
		warn("- the role " + name + " was not found")
		warnMissing(paths)
		return 0
	}

	usable := 0
	for _, p := range paths {
		if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
			warn("- the configured path " + p + " does not exist.")
			continue
		}
		usable++
		// The header is printed for a usable path even when it holds
		// no roles at all.
		fmt.Println("# " + abs(p))
		for _, r := range rolesIn(p) {
			fmt.Println(r.String())
		}
	}
	if usable == 0 {
		warn("None of the provided paths were usable. Please specify a valid path with --roles-path.")
	}
	return 0
}

// runRemove is `ansible-galaxy remove <role>...`: it deletes the
// installed role and says so, or says it was not there. Exit 0 either
// way, as real does.
func runRemove(args []string) int {
	names, explicit, err := parseRemoveFlags(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]:", err)
		return 2
	}
	if len(names) == 0 {
		usage()
		return 2
	}
	paths := rolesSearchPath(explicit)
	for _, name := range names {
		removed := false
		for _, p := range paths {
			dir := filepath.Join(p, name)
			if _, err := os.Stat(filepath.Join(dir, "meta", "main.yml")); err != nil {
				continue
			}
			if err := os.RemoveAll(dir); err != nil {
				fmt.Fprintln(os.Stderr, "[ERROR]:", err)
				return 1
			}
			fmt.Println("- successfully removed " + name)
			removed = true
			break
		}
		if !removed {
			fmt.Println("- " + name + " is not installed, skipping.")
		}
	}
	return 0
}

func parseListFlags(args []string) (name, rolesPath string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-p", "--roles-path":
			i++
			if i >= len(args) {
				return "", "", fmt.Errorf("%s requires a value", args[i-1])
			}
			rolesPath = args[i]
		default:
			if strings.HasPrefix(args[i], "-") {
				return "", "", fmt.Errorf("unrecognized argument: %s", args[i])
			}
			if name != "" {
				return "", "", fmt.Errorf("only one role may be named")
			}
			name = args[i]
		}
	}
	return name, rolesPath, nil
}

func parseRemoveFlags(args []string) (names []string, rolesPath string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-p", "--roles-path":
			i++
			if i >= len(args) {
				return nil, "", fmt.Errorf("%s requires a value", args[i-1])
			}
			rolesPath = args[i]
		default:
			if strings.HasPrefix(args[i], "-") {
				return nil, "", fmt.Errorf("unrecognized argument: %s", args[i])
			}
			names = append(names, args[i])
		}
	}
	return names, rolesPath, nil
}

func warnMissing(paths []string) {
	for _, p := range paths {
		if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
			warn("- the configured path " + p + " does not exist.")
		}
	}
}

// warn writes real's warning line to stderr. ansible-galaxy does not
// share the playbook engine's Warner: it emits the same line for the
// same path twice in a run if it looks twice, and deduplicating would
// change what it prints.
func warn(msg string) { fmt.Fprintln(os.Stderr, "[WARNING]: "+msg) }

// abs reports the absolute path real prints in the "# <path>" header,
// falling back to what it was given if that cannot be resolved.
func abs(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}
