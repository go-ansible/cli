// Command ansible-playbook runs a playbook against an inventory.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
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
	showVersion := fs.Bool("version", false, "print the version and exit")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: ansible-playbook -i INVENTORY [-e KEY=VAL ...] PLAYBOOK.yml [PLAYBOOK2.yml ...]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Println(version.String("ansible-playbook"))
		return 0
	}
	if *inventoryPath == "" || fs.NArg() == 0 {
		fs.Usage()
		return 2
	}

	inv, err := inventory.Load(*inventoryPath)
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
	e.Callbacks = []playbook.Callback{playbook.NewDefaultCallback(os.Stdout, !*noColor)}

	failed := false
	for _, path := range fs.Args() {
		pb, err := playbook.ParseFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ansible-playbook:", err)
			if errors.Is(err, iofs.ErrNotExist) {
				return 1
			}
			return 4
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
