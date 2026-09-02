// Command ansible-galaxy installs roles from a requirements file.
//
// Scope note: this implements local/git-sourced role installation
// (`ansible-galaxy install -r requirements.yml`), the mechanism that
// matters for a self-hosted or git-based workflow. It does not talk to
// the galaxy.ansible.com HTTP API (named-role/collection search and
// download) — that is real, separate scope, not yet built.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"gopkg.in/yaml.v3"
)

// Requirements is a requirements.yml document's `roles:` list.
type Requirements struct {
	Roles []RoleRequirement `yaml:"roles"`
}

// RoleRequirement is one role to install.
type RoleRequirement struct {
	Name    string `yaml:"name"`
	Src     string `yaml:"src"`
	Version string `yaml:"version"`
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) < 1 || args[0] != "install" {
		usage()
		return 2
	}

	reqFile, rolesDir, err := parseInstallFlags(args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "ansible-galaxy:", err)
		return 2
	}
	if reqFile == "" {
		usage()
		return 2
	}

	reqs, err := loadRequirements(reqFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ansible-galaxy:", err)
		return 1
	}

	code := 0
	for _, r := range reqs.Roles {
		if err := installRole(r, rolesDir); err != nil {
			fmt.Fprintf(os.Stderr, "ansible-galaxy: installing %s: %v\n", roleName(r), err)
			code = 1
			continue
		}
		fmt.Printf("- %s was installed successfully\n", roleName(r))
	}
	return code
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: ansible-galaxy install -r requirements.yml [-p roles/]")
}

func parseInstallFlags(args []string) (reqFile, rolesDir string, err error) {
	rolesDir = "roles"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-r", "--role-file":
			i++
			if i >= len(args) {
				return "", "", fmt.Errorf("%s requires a value", args[i-1])
			}
			reqFile = args[i]
		case "-p", "--roles-path":
			i++
			if i >= len(args) {
				return "", "", fmt.Errorf("%s requires a value", args[i-1])
			}
			rolesDir = args[i]
		default:
			return "", "", fmt.Errorf("unrecognized argument: %s", args[i])
		}
	}
	return reqFile, rolesDir, nil
}

func loadRequirements(path string) (*Requirements, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var reqs Requirements
	if err := yaml.Unmarshal(data, &reqs); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &reqs, nil
}

func roleName(r RoleRequirement) string {
	if r.Name != "" {
		return r.Name
	}
	return filepath.Base(r.Src)
}

func installRole(r RoleRequirement, rolesDir string) error {
	if r.Src == "" {
		return fmt.Errorf("no src given")
	}
	dest := filepath.Join(rolesDir, roleName(r))

	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("%s already exists (remove it first to reinstall)", dest)
	}
	if err := os.MkdirAll(rolesDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", rolesDir, err)
	}

	repo, err := git.PlainClone(dest, false, &git.CloneOptions{URL: r.Src})
	if err != nil {
		return fmt.Errorf("cloning %s: %w", r.Src, err)
	}

	if r.Version == "" {
		return nil
	}
	hash, err := repo.ResolveRevision(plumbing.Revision(r.Version))
	if err != nil {
		return fmt.Errorf("resolving version %q: %w", r.Version, err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("opening worktree: %w", err)
	}
	if err := wt.Checkout(&git.CheckoutOptions{Hash: *hash}); err != nil {
		return fmt.Errorf("checking out %q: %w", r.Version, err)
	}
	return nil
}
