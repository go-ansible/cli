// Command ansible-pull clones (or updates) a git repository and runs a
// playbook from it against this machine — the pull-mode counterpart to
// ansible-playbook's push mode, for a target that fetches and applies
// its own configuration instead of being reached over SSH.
//
// Scope note: unlike real ansible-pull, the checkout destination isn't
// hashed exactly the way ansible-core hashes it (this port uses a
// simpler deterministic name derived from the URL) — a fresh run
// always resolves to the same directory for the same URL, which is
// the property that actually matters, but the two tools won't agree
// on a byte-identical path. Default playbook lookup only tries
// local.yml (real ansible-pull also tries <hostname>.yml and
// main.yml) — pass the playbook explicitly to use another name.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"

	"github.com/go-ansible/cli/internal/extravars"
	"github.com/go-ansible/cli/internal/report"
	"github.com/go-ansible/cli/internal/version"
	"github.com/go-ansible/inventory"
	"github.com/go-ansible/playbook"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) > 0 && (args[0] == "--version" || args[0] == "-version") {
		fmt.Println(version.String("ansible-pull"))
		return 0
	}

	opts, playbookName, err := parsePullFlags(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ansible-pull:", err)
		usage()
		return 2
	}
	if opts.url == "" {
		usage()
		return 2
	}

	dest := opts.directory
	if dest == "" {
		dest = defaultCheckoutDir(opts.url)
	}

	changed, err := syncRepo(dest, opts.url, opts.checkout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ansible-pull:", err)
		return 1
	}
	if opts.onlyIfChanged && !changed {
		fmt.Println("ansible-pull: no change since last run, skipping (--only-if-changed)")
		return 0
	}

	pbPath := filepath.Join(dest, playbookName)

	extra, err := extravars.Parse(opts.extraVars)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ansible-pull:", err)
		return 2
	}

	inv, err := loadOrDefaultInventory(opts.inventoryPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ansible-pull:", err)
		return 1
	}

	pb, err := playbook.ParseFile(pbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ansible-pull:", err)
		if opts.purge {
			os.RemoveAll(dest)
		}
		if errors.Is(err, iofs.ErrNotExist) {
			return 1
		}
		return 4
	}

	e := playbook.New(inv)
	e.ExtraVars = extra
	e.BaseDir = dest
	printer := report.NewPrinter(os.Stdout, !opts.noColor)
	e.OnResult = printer.OnResult

	rr, runErr := e.RunPlaybook(context.Background(), pb)
	if rr != nil {
		report.Recap(os.Stdout, rr, !opts.noColor)
	}

	code := 0
	if runErr != nil {
		fmt.Fprintln(os.Stderr, "ansible-pull:", runErr)
		code = 1
	} else if rr != nil && rr.Failed() {
		code = 2
	}

	if opts.purge {
		os.RemoveAll(dest)
	}
	return code
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: ansible-pull -U REPO_URL [-C CHECKOUT] [-d DIR] [-i INVENTORY] [-e KEY=VAL ...] [--only-if-changed] [--purge] [--no-color] [PLAYBOOK.yml]")
}

type pullOptions struct {
	url           string
	checkout      string
	directory     string
	inventoryPath string
	extraVars     []string
	onlyIfChanged bool
	purge         bool
	noColor       bool
}

func parsePullFlags(args []string) (pullOptions, string, error) {
	var opts pullOptions
	playbookName := "local.yml"
	havePositional := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		next := func() (string, error) {
			i++
			if i >= len(args) {
				return "", fmt.Errorf("%s requires a value", a)
			}
			return args[i], nil
		}
		switch a {
		case "-U", "--url":
			v, err := next()
			if err != nil {
				return opts, "", err
			}
			opts.url = v
		case "-C", "--checkout":
			v, err := next()
			if err != nil {
				return opts, "", err
			}
			opts.checkout = v
		case "-d", "--directory":
			v, err := next()
			if err != nil {
				return opts, "", err
			}
			opts.directory = v
		case "-i", "--inventory":
			v, err := next()
			if err != nil {
				return opts, "", err
			}
			opts.inventoryPath = v
		case "-e", "--extra-vars":
			v, err := next()
			if err != nil {
				return opts, "", err
			}
			opts.extraVars = append(opts.extraVars, v)
		case "--only-if-changed":
			opts.onlyIfChanged = true
		case "--purge":
			opts.purge = true
		case "--no-color":
			opts.noColor = true
		default:
			if len(a) > 0 && a[0] == '-' {
				return opts, "", fmt.Errorf("unrecognized argument: %s", a)
			}
			if havePositional {
				return opts, "", fmt.Errorf("unexpected extra argument %q (playbook already given as %q)", a, playbookName)
			}
			playbookName = a
			havePositional = true
		}
	}
	return opts, playbookName, nil
}

// defaultCheckoutDir derives a stable local directory for url, under
// the user's home directory — the same URL always resolves to the
// same path, so repeated runs update an existing checkout instead of
// re-cloning fresh each time. Not the same hash real ansible-pull
// uses, but the same "one URL, one durable directory" property.
func defaultCheckoutDir(url string) string {
	sum := sha256.Sum256([]byte(url))
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".ansible", "pull", hex.EncodeToString(sum[:])[:16])
}

// syncRepo clones url into dest if it doesn't exist yet, or fetches
// and fast-forwards an existing checkout otherwise; if checkout names
// a branch/tag/commit, it's resolved and checked out after the
// clone/pull. changed reports whether HEAD moved (or this was a fresh
// clone) — the signal --only-if-changed needs.
func syncRepo(dest, url, checkoutRef string) (changed bool, err error) {
	repo, err := git.PlainOpen(dest)
	if err != nil {
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return false, fmt.Errorf("creating %s: %w", filepath.Dir(dest), err)
		}
		if _, err := git.PlainClone(dest, false, &git.CloneOptions{URL: url}); err != nil {
			return false, fmt.Errorf("cloning %s: %w", url, err)
		}
		return true, checkoutIfNeeded(dest, checkoutRef)
	}

	head, err := repo.Head()
	beforeHash := plumbing.ZeroHash
	if err == nil {
		beforeHash = head.Hash()
	}

	wt, err := repo.Worktree()
	if err != nil {
		return false, fmt.Errorf("opening worktree: %w", err)
	}
	if err := wt.Pull(&git.PullOptions{}); err != nil && err != git.NoErrAlreadyUpToDate {
		return false, fmt.Errorf("pulling %s: %w", url, err)
	}

	if err := checkoutIfNeeded(dest, checkoutRef); err != nil {
		return false, err
	}

	head, err = repo.Head()
	afterHash := plumbing.ZeroHash
	if err == nil {
		afterHash = head.Hash()
	}
	return beforeHash != afterHash, nil
}

func checkoutIfNeeded(dest, ref string) error {
	if ref == "" {
		return nil
	}
	repo, err := git.PlainOpen(dest)
	if err != nil {
		return err
	}
	hash, err := repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return fmt.Errorf("resolving checkout %q: %w", ref, err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}
	if err := wt.Checkout(&git.CheckoutOptions{Hash: *hash}); err != nil {
		return fmt.Errorf("checking out %q: %w", ref, err)
	}
	return nil
}

// loadOrDefaultInventory loads path if given, otherwise synthesizes
// the implicit "just this machine, over a local connection" inventory
// real ansible-pull defaults to — it exists to configure the host it
// runs ON, not a remote fleet.
func loadOrDefaultInventory(path string) (*inventory.Inventory, error) {
	if path != "" {
		return inventory.Load(path)
	}
	inv := inventory.New()
	inv.AddHost("localhost", map[string]any{"ansible_connection": "local"})
	return inv, nil
}
