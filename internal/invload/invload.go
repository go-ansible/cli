// Package invload loads the inventory the way real Ansible's CLI does:
// a source it cannot use is a WARNING and the run continues with only
// the implicit localhost, rather than a fatal error.
//
// It exists so that rule lives in ONE place. Every binary that takes
// -i must apply it identically — real applies it in CLI.get_host_list,
// which ansible, ansible-playbook and the rest all funnel through —
// and a rule restated per binary drifts.
package invload

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/go-ansible/inventory"
	"github.com/go-ansible/playbook"
)

// These are real Ansible's own sentences, character for character.
// They were captured from ansible-core 2.21.4's stderr rather than
// copied out of its source, because what it PRINTS is the thing being
// matched.
const (
	unableToParse  = "Unable to parse %s as an inventory source"
	failedToParse  = "Failed to parse inventory: %s"
	noneParsed     = "No inventory was parsed, only implicit localhost is available"
	emptyHostsList = "provided hosts list is empty, only localhost is available. Note that the implicit localhost does not match 'all'"
)

// Load returns the inventory at path, warning through warn about
// anything real Ansible would warn about and NEVER failing: an absent,
// unreadable or unparseable source yields an empty inventory, which
// still answers to the implicit localhost, and the caller carries on.
//
// That is not leniency, it is real's behaviour, and the difference is
// observable: `ansible-playbook -i /nope.ini play.yml` and `ansible
// localhost -m ping` with no -i at all both exit 0 there. A drop-in
// replacement that exits 1 instead breaks the script around it.
//
// pattern is the host pattern the caller will select with — "all" for
// ansible-playbook, the user's own pattern for ad-hoc ansible. It is
// needed because real suppresses the empty-inventory warning when the
// pattern is one the implicit localhost answers to (C.LOCALHOST):
// measured, `ansible -i /nope.ini localhost -m ping` prints two
// warnings where `... h1 -m ping` prints three. Rather than restate
// the list of localhost spellings here, this asks the inventory
// itself — on an empty inventory, only such a pattern matches
// anything — so the two cannot drift apart.
func Load(path, vaultPassword, pattern string, warn playbook.Warner) *inventory.Inventory {
	inv, parsed := loadOrEmpty(path, vaultPassword, warn)
	if !parsed {
		warn(noneParsed)
	}
	if len(inv.Hosts) == 0 {
		if hosts, err := inv.Match(pattern); err != nil || len(hosts) == 0 {
			warn(emptyHostsList)
		}
	}
	return inv
}

// loadOrEmpty reports whether a source was actually parsed, which is
// not the same as whether the inventory has hosts: real warns "No
// inventory was parsed" for a missing file but not for a file that
// parsed to nothing. Measured — an empty .ini produces only the
// empty-hosts warning, while a missing one produces both.
func loadOrEmpty(path, vaultPassword string, warn playbook.Warner) (*inventory.Inventory, bool) {
	if path == "" {
		return inventory.New(), false
	}
	inv, err := inventory.LoadWithVault(path, vaultPassword)
	if err != nil {
		// Real reports the ABSOLUTE path, even when -i was given a
		// relative one: measured, `-i nope.ini` warns about
		// /full/path/nope.ini.
		shown := path
		if abs, aerr := filepath.Abs(path); aerr == nil {
			shown = abs
		}
		// The CAUSE first, then the source — real's order, and only
		// when there was something to parse. A source that does not
		// EXIST gets the "Unable to parse" line alone: real's plugins
		// only report a parse failure once they have actually tried to
		// parse, so an absent -i produces one warning there and this
		// produced two until the ad-hoc comparison caught it. A
		// directory holding no inventory source at all is the same
		// case for the same reason: real names it, having never
		// reached a parser for it. Real says
		// more here than this can: it prints a structured block with
		// the plugin it tried, the source position and an excerpt,
		// which needs per-node positions this port's parsers do not
		// record. The sentence itself is real's own, from the
		// "<<< caused by >>>" half of that block; what differs is the
		// wording of the detail after the colon, which is Python's.
		// Dropping it altogether would leave the user knowing WHICH
		// file is unusable and not why, which real never does.
		// The loader prefixes its errors "inventory: "; inside this
		// sentence that reads "Failed to parse inventory: inventory:
		// ...", so it comes off here — the one place that composes
		// the two.
		if !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, inventory.ErrNoSources) {
			warn(fmt.Sprintf(failedToParse, strings.TrimPrefix(err.Error(), "inventory: ")))
		}
		warn(fmt.Sprintf(unableToParse, shown))
		return inventory.New(), false
	}
	return inv, true
}
