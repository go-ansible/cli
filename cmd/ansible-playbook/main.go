// Command ansible-playbook runs a playbook against an inventory.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/go-ansible/cli/internal/vaultpw"
	"io"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-ansible/cli/internal/extravars"
	"github.com/go-ansible/cli/internal/version"
	"github.com/go-ansible/inventory"
	"github.com/go-ansible/playbook"
	"golang.org/x/term"
)

type stringList []string

func (s *stringList) String() string     { return fmt.Sprint([]string(*s)) }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("ansible-playbook", flag.ContinueOnError)
	inventoryPath := fs.String("i", "", "inventory file or directory (also --inventory)")
	fs.StringVar(inventoryPath, "inventory", "", "inventory file or directory")
	var extra stringList
	fs.Var(&extra, "e", "extra variables: key=value, @file.yml, or a JSON object (repeatable, also --extra-vars)")
	fs.Var(&extra, "extra-vars", "extra variables: key=value, @file.yml, or a JSON object (repeatable)")
	var runTags, skipTags stringList
	fs.Var(&runTags, "t", "only run tasks tagged with one of these tags (repeatable, also --tags)")
	fs.Var(&runTags, "tags", "only run tasks tagged with one of these tags (repeatable)")
	fs.Var(&skipTags, "skip-tags", "skip tasks tagged with one of these tags (repeatable)")
	// -1 (not a valid real fork count) means "not given" — playbook.New
	// already applies the real default (5, or ANSIBLE_FORKS) before
	// this flag is even parsed, and inv isn't available yet to
	// construct that Engine here to read its own default back out.
	forks := fs.Int("f", -1, "number of parallel processes to use (also --forks)")
	fs.IntVar(forks, "forks", -1, "number of parallel processes to use")
	noColor := fs.Bool("no-color", false, "disable colored output")
	checkMode := fs.Bool("check", false, "don't make any changes; instead try to predict some of the changes that may occur (also -C)")
	fs.BoolVar(checkMode, "C", false, "don't make any changes (also --check)")
	var inspectFlags inspectMode
	fs.BoolVar(&inspectFlags.listTasks, "list-tasks", false, "list all tasks that would be executed")
	fs.BoolVar(&inspectFlags.listTags, "list-tags", false, "list all available tags")
	fs.BoolVar(&inspectFlags.listHosts, "list-hosts", false, "outputs a list of matching hosts")
	fs.BoolVar(&inspectFlags.syntaxCheck, "syntax-check", false, "perform a syntax check on the playbook, but do not execute it")
	limit := fs.String("limit", "", "further limit selected hosts to an additional pattern (also -l)")
	fs.StringVar(limit, "l", "", "further limit selected hosts to an additional pattern")
	diffMode := fs.Bool("diff", false, "when changing any file, show the differences (also -D)")
	fs.BoolVar(diffMode, "D", false, "when changing any file, show the differences (also --diff)")
	vaultPasswordFile := fs.String("vault-password-file", "", "read the vault password from this file (also --vault-pass-file)")
	fs.StringVar(vaultPasswordFile, "vault-pass-file", "", "read the vault password from this file")
	askVaultPass := fs.Bool("ask-vault-password", false, "prompt for the vault password (also --ask-vault-pass)")
	fs.BoolVar(askVaultPass, "ask-vault-pass", false, "prompt for the vault password")
	showVersion := fs.Bool("version", false, "print the version and exit")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: ansible-playbook -i INVENTORY [-e KEY=VAL ...] PLAYBOOK.yml [PLAYBOOK2.yml ...]")
		fs.PrintDefaults()
	}
	playbooks, err := parseInterspersed(fs, args)
	if err != nil {
		return 2
	}
	if *showVersion {
		fmt.Println(version.String("ansible-playbook"))
		return 0
	}
	if *inventoryPath == "" || len(playbooks) == 0 {
		fs.Usage()
		return 2
	}

	// The password is resolved before anything is read, because the
	// inventory itself may be encrypted. It is only asked for when the
	// caller said one exists — otherwise a plaintext run would stop to
	// prompt for a password nothing needs.
	var vaultPassword string
	if *vaultPasswordFile != "" || *askVaultPass {
		vaultPassword, err = vaultpw.Resolve(*vaultPasswordFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ansible-playbook:", err)
			return 1
		}
	}

	inv, err := inventory.LoadWithVault(*inventoryPath, vaultPassword)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ansible-playbook:", err)
		return 1
	}

	vars, err := extravars.Parse(extra)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ansible-playbook:", err)
		return 2
	}

	e := playbook.New(inv)
	if *forks >= 0 {
		e.Forks = *forks
	}
	e.ExtraVars = vars
	e.RunTags = splitTagList(runTags)
	e.SkipTags = splitTagList(skipTags)
	e.Prompt = terminalPrompt
	e.VaultPassword = vaultPassword
	e.CheckMode = *checkMode
	e.DiffMode = *diffMode
	e.Limit = *limit

	// Real Ansible makes this check ONCE, before any play runs, and
	// against "all" rather than any play's own pattern
	// (ansible/cli/__init__.py get_host_list) — which is why a play
	// whose hosts: matches nothing merely reports "skipping: no hosts
	// matched" while a --limit matching nothing is a hard error.
	if *limit != "" {
		all, aerr := inv.Match("all")
		if aerr != nil {
			fmt.Fprintln(os.Stderr, "ansible-playbook:", aerr)
			return 1
		}
		targeted, terr := inv.Match(*limit)
		if terr != nil {
			fmt.Fprintln(os.Stderr, "ansible-playbook:", terr)
			return 1
		}
		if len(all) > 0 && len(targeted) == 0 {
			fmt.Fprintln(os.Stderr, "[WARNING]: Could not match supplied host pattern, ignoring:", *limit)
			fmt.Fprintln(os.Stderr, "[ERROR]: Specified inventory, host pattern and/or --limit leaves us with no hosts to target.")
			return 1
		}
	}
	e.Callbacks = []playbook.Callback{playbook.NewDefaultCallback(os.Stdout, !*noColor)}

	failed := false
	for _, path := range playbooks {
		pb, err := playbook.ParseFileWithVault(path, vaultPassword)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ansible-playbook:", err)
			if errors.Is(err, iofs.ErrNotExist) {
				return 1
			}
			return 4
		}
		if inspectFlags.any() {
			// These flags print what the playbook WOULD do and run
			// nothing at all, so they come after parsing (which is what
			// --syntax-check is checking) and before any connection.
			inspect(os.Stdout, inspectFlags, path, pb, inv, *limit,
				splitTagList(runTags), splitTagList(skipTags))
			continue
		}

		e.BaseDir = filepath.Dir(path)
		rr, err := e.RunPlaybook(context.Background(), pb)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ansible-playbook:", err)
			return 1
		}
		if rr != nil && rr.Failed() {
			failed = true
		}
	}
	if failed {
		return 2
	}
	return 0
}

// terminalPrompt is playbook.Engine's real, interactive Prompt
// implementation — the library's own default (playbook.defaultPrompt)
// deliberately has no terminal awareness at all. Matches real
// ansible-playbook's own do_var_prompt: not a TTY at all means don't
// even try to read (an empty result, so vars_prompt falls back to
// Default), and a private prompt hides the typed characters via
// term.ReadPassword rather than echoing them.
func terminalPrompt(msg string, private bool) (string, error) {
	fmt.Fprint(os.Stderr, msg)
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "[WARNING]: Not prompting as we are not in interactive mode")
		return "", nil
	}
	if private {
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		return string(pw), nil
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// parseInterspersed parses args allowing flags to appear after positional
// arguments, and returns the positionals. Real ansible-playbook is a
// Python argparse program, where `ansible-playbook site.yml -e k=v` is an
// ordinary invocation; Go's flag package stops at the first non-flag, so
// it took `-e` for a second playbook path and failed on it. Parsing is
// resumed after each positional instead, which is exactly what argparse
// does and keeps repeatable flags (-e, --tags) accumulating across the
// whole command line.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return positional, nil
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

// splitTagList expands a repeatable --tags/--skip-tags flag into a flat
// tag list, splitting each occurrence on commas too — real
// ansible-playbook accepts both `--tags a --tags b` and `--tags a,b`.
func splitTagList(raw []string) []string {
	var out []string
	for _, entry := range raw {
		for _, tag := range strings.Split(entry, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				out = append(out, tag)
			}
		}
	}
	return out
}
