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
	"io"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-ansible/cli/internal/extravars"
	"github.com/go-ansible/cli/internal/termcolor"
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
		fmt.Fprintln(os.Stderr, "[ERROR]:", err)
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

	announce(os.Stdout, time.Now(), os.Args)

	res, err := syncRepo(dest, opts.url, opts.checkout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]:", err)
		return 1
	}
	reportSync(os.Stdout, res)
	if opts.onlyIfChanged && !res.changed {
		fmt.Println("ansible-pull: no change since last run, skipping (--only-if-changed)")
		return 0
	}

	pbPath := filepath.Join(dest, playbookName)

	extra, err := extravars.Parse(opts.extraVars)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]:", err)
		return 2
	}

	// A relative --inventory names a file IN THE CHECKOUT, not in the
	// directory ansible-pull was run from: real runs the playbook with
	// the checkout as its working directory, so `-i hosts` finds the
	// inventory the repository carries. That is the ordinary
	// ansible-pull idiom, and resolving it here against the process's
	// own cwd made it fail outright.
	inv, err := loadOrDefaultInventory(resolveAgainst(dest, opts.inventoryPath))
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]:", err)
		return 1
	}

	pb, err := playbook.ParseFile(pbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]:", err)
		if opts.purge {
			os.RemoveAll(dest)
		}
		if errors.Is(err, iofs.ErrNotExist) {
			return 1
		}
		return 4
	}

	e := playbook.New(inv)
	// ansible-pull applies the repository to THIS machine and no
	// other, however wide the playbook's own hosts: is. Real does it
	// by limiting the run to the local hostname or localhost --
	// measured with a three-host inventory and a play targeting all:
	// real ran on localhost and on this machine's name, and NOT on
	// the third host. Without the limit a pull-mode agent would
	// configure every host the repository's inventory happens to
	// name, which is the opposite of what pull mode is for.
	e.Limit = localTargets()
	// A pull's warnings go to STDOUT, not stderr. Real runs the work
	// as subprocesses and forwards everything they write to its own
	// stdout -- measured: its stderr is empty and the host-pattern
	// warnings appear among the stdout lines. A reader of a pull log
	// therefore has one stream, in order, which is what makes an
	// unattended run readable afterwards.
	e.Warn = playbook.NewWarner(os.Stdout)
	e.ExtraVars = extra
	e.BaseDir = dest
	e.Callbacks = []playbook.Callback{playbook.NewDefaultCallback(os.Stdout, termcolor.Enabled(os.Stdout, opts.noColor))}

	rr, runErr := e.RunPlaybook(context.Background(), pb)

	code := 0
	if runErr != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]:", runErr)
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
// syncResult is what the clone or pull did, in the shape real's own
// git-module task reports: the revision before and after, and whether
// they differ. before is empty on a first clone, which real prints as
// a JSON null.
type syncResult struct {
	before  string
	after   string
	changed bool
	// cloned distinguishes a first clone from a pull. Real reports
	// remote_url_changed on a pull and not on a clone.
	cloned bool
}

func syncRepo(dest, url, checkoutRef string) (res syncResult, err error) {
	repo, err := git.PlainOpen(dest)
	if err != nil {
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return res, fmt.Errorf("creating %s: %w", filepath.Dir(dest), err)
		}
		cloned, err := git.PlainClone(dest, false, &git.CloneOptions{URL: url})
		if err != nil {
			return res, fmt.Errorf("cloning %s: %w", url, err)
		}
		if err := checkoutIfNeeded(dest, checkoutRef); err != nil {
			return res, err
		}
		res = syncResult{changed: true, cloned: true}
		if head, err := cloned.Head(); err == nil {
			res.after = head.Hash().String()
		}
		return res, nil
	}

	head, err := repo.Head()
	beforeHash := plumbing.ZeroHash
	if err == nil {
		beforeHash = head.Hash()
	}

	wt, err := repo.Worktree()
	if err != nil {
		return res, fmt.Errorf("opening worktree: %w", err)
	}
	if err := wt.Pull(&git.PullOptions{}); err != nil && err != git.NoErrAlreadyUpToDate {
		return res, fmt.Errorf("pulling %s: %w", url, err)
	}

	if err := checkoutIfNeeded(dest, checkoutRef); err != nil {
		return res, err
	}

	head, err = repo.Head()
	afterHash := plumbing.ZeroHash
	if err == nil {
		afterHash = head.Hash()
	}
	return syncResult{
		before:  beforeHash.String(),
		after:   afterHash.String(),
		changed: beforeHash != afterHash,
	}, nil
}

// reportSync prints what the clone or pull did, in the shape real's
// git-module task reports it -- the first thing an ansible-pull run
// prints, before the playbook it then runs.
func reportSync(w io.Writer, res syncResult) {
	status := "SUCCESS"
	if res.changed {
		status = "CHANGED"
	}
	fmt.Fprintf(w, "localhost | %s => {\n", status)
	if res.before == "" {
		fmt.Fprint(w, "    \"after\": "+strconv.Quote(res.after)+",\n    \"before\": null,\n")
	} else {
		fmt.Fprint(w, "    \"after\": "+strconv.Quote(res.after)+",\n    \"before\": "+strconv.Quote(res.before)+",\n")
	}
	fmt.Fprintf(w, "    \"changed\": %t", res.changed)
	if !res.cloned {
		// Real reports this on a pull and not on a first clone.
		fmt.Fprint(w, ",\n    \"remote_url_changed\": false")
	}
	fmt.Fprint(w, "\n}\n")
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

// resolveAgainst makes a relative path relative to base, and leaves an
// absolute one alone -- real accepts an --inventory outside the
// checkout that way.
func resolveAgainst(base, path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, path)
}

// localTargets is the pattern ansible-pull limits every run to. Real
// builds it from four names and two constants (ansible/cli/pull.py):
//
//	set(fqdn, node, fqdn-before-the-first-dot, node-before-the-dot)
//	wrapped as  localhost,<those>,127.0.0.1
//
// so an inventory may name this machine any of the ways a machine is
// usually named and still be reached. Real warns about whichever
// terms it cannot match rather than failing, which is what makes the
// limit safe to apply to an inventory that knows nothing about this
// host.
//
// Two differences from real, both stated rather than papered over:
//
//   - real's fqdn comes from socket.getfqdn(), which performs a DNS
//     lookup; this uses os.Hostname(), which does not. On a machine
//     whose resolver returns a longer name than its hostname, real
//     limits to one more term than this does.
//   - real joins a Python SET, whose iteration order for strings
//     varies between processes. Its own term order is therefore not
//     reproducible -- not by this port, and not by real itself from
//     one run to the next. This emits them in a fixed order, which is
//     the only stable choice available.
func localTargets() string {
	names := []string{}
	seen := map[string]bool{}
	add := func(n string) {
		if n != "" && !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	if h, err := os.Hostname(); err == nil {
		add(h)
		if short, _, found := strings.Cut(h, "."); found {
			add(short)
		}
	}
	out := append([]string{"localhost"}, names...)
	return strings.Join(append(out, "127.0.0.1"), ",")
}

// announce prints what real prints before it does anything else: when
// the run started, and the command line that started it. A pull runs
// unattended and its output is usually read later out of a log, where
// those two lines are the only record of which invocation produced
// what follows.
func announce(w io.Writer, now time.Time, argv []string) {
	fmt.Fprintf(w, "Starting Ansible Pull at %s\n", now.Format("2006-01-02 15:04:05"))
	fmt.Fprintln(w, strings.Join(argv, " "))
}
