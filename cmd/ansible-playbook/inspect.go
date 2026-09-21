package main

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-ansible/inventory"
	"github.com/go-ansible/playbook"
)

// The inspection flags print what a playbook WOULD do and exit without
// running anything: --list-tasks, --list-tags, --list-hosts and
// --syntax-check. Every format below was measured from real
// ansible-core 2.21.4, down to the tab before TAGS and the blank line
// between plays.
type inspectMode struct {
	listTasks   bool
	listTags    bool
	listHosts   bool
	syntaxCheck bool
}

func (m inspectMode) any() bool {
	return m.listTasks || m.listTags || m.listHosts || m.syntaxCheck
}

// inspect prints one playbook's listing. A --syntax-check does nothing
// beyond the header, because reaching here means the playbook parsed.
func inspect(w io.Writer, m inspectMode, path string, pb playbook.Playbook,
	inv *inventory.Inventory, limit string, runTags, skipTags []string) {

	fmt.Fprintf(w, "\nplaybook: %s\n", path)
	if m.syntaxCheck {
		return
	}

	for i, play := range pb {
		name := play.Name
		if name == "" {
			// An unnamed play is named after its hosts pattern, the
			// same fallback the run-time banner uses.
			name = play.Hosts
		}
		fmt.Fprintf(w, "\n  play #%d (%s): %s\tTAGS: [%s]\n",
			i+1, play.Hosts, name, strings.Join(sortedTags(play.Tags), ", "))

		switch {
		case m.listHosts:
			// The pattern is printed as a Python one-element list of the
			// RAW pattern text — real Ansible does not split it here,
			// so "web:!h1" prints as ['web:!h1'].
			fmt.Fprintf(w, "    pattern: ['%s']\n", play.Hosts)
			hosts := matchedHosts(inv, play.Hosts, limit)
			fmt.Fprintf(w, "    hosts (%d):\n", len(hosts))
			for _, h := range hosts {
				fmt.Fprintf(w, "      %s\n", h)
			}
		case m.listTasks:
			fmt.Fprintln(w, "    tasks:")
			for _, t := range play.ListedTasks() {
				if !playbook.TagsSelect(t.Tags, runTags, skipTags) {
					continue
				}
				fmt.Fprintf(w, "      %s\tTAGS: [%s]\n",
					taskDisplayName(t), strings.Join(sortedTags(t.Tags), ", "))
			}
		case m.listTags:
			seen := map[string]bool{}
			for _, t := range play.ListedTasks() {
				if !playbook.TagsSelect(t.Tags, runTags, skipTags) {
					continue
				}
				for _, tag := range t.Tags {
					seen[tag] = true
				}
			}
			all := make([]string, 0, len(seen))
			for tag := range seen {
				all = append(all, tag)
			}
			fmt.Fprintf(w, "      TASK TAGS: [%s]\n", strings.Join(sortedTags(all), ", "))
		}
	}
}

// taskDisplayName is the name a listing shows: the task's own name, or
// its module when it has none, prefixed with the role it came from —
// real Ansible prints "myrole : the task", spaces around the colon.
func taskDisplayName(t playbook.Task) string {
	name := t.Name
	if name == "" {
		name = t.Module
	}
	if t.RoleDir != "" {
		return filepath.Base(t.RoleDir) + " : " + name
	}
	return name
}

func sortedTags(tags []string) []string {
	out := append([]string(nil), tags...)
	sort.Strings(out)
	return out
}

// matchedHosts resolves a play's pattern and applies --limit, returning
// host names. A pattern that matches nothing is not an error here: the
// listing simply shows none, as real Ansible's does.
func matchedHosts(inv *inventory.Inventory, pattern, limit string) []string {
	hosts, err := inv.Match(pattern)
	if err != nil {
		return nil
	}
	if limit != "" {
		allowed, lerr := inv.Match(limit)
		if lerr != nil {
			return nil
		}
		keep := make(map[string]bool, len(allowed))
		for _, h := range allowed {
			keep[h.Name] = true
		}
		var kept []*inventory.Host
		for _, h := range hosts {
			if keep[h.Name] {
				kept = append(kept, h)
			}
		}
		hosts = kept
	}
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		out = append(out, h.Name)
	}
	return out
}
