// Command ansible-playbook runs a playbook against an inventory.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-ansible/cli/internal/extravars"
	"github.com/go-ansible/cli/internal/report"
	"github.com/go-ansible/inventory"
	"github.com/go-ansible/playbook"
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
	noColor := fs.Bool("no-color", false, "disable colored output")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: ansible-playbook -i INVENTORY [-e KEY=VAL ...] PLAYBOOK.yml [PLAYBOOK2.yml ...]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
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
	e.ExtraVars = vars
	e.RunTags = splitTagList(runTags)
	e.SkipTags = splitTagList(skipTags)
	printer := report.NewPrinter(os.Stdout, !*noColor)
	e.OnResult = printer.OnResult

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
		if rr != nil {
			report.Recap(os.Stdout, rr, !*noColor)
		}
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
