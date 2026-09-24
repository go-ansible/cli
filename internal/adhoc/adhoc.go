// Package adhoc is the ad-hoc single-module-against-a-host-pattern
// execution logic shared by cmd/ansible and cmd/ansible-console — the
// same "connect, run one module on every matched host, print a
// host | STATUS => json line as each finishes" flow both binaries
// need, factored out once instead of copied.
package adhoc

import (
	"context"
	"fmt"
	"github.com/go-ansible/template"
	"io"
	"strings"
	"sync"

	"github.com/go-ansible/inventory"
	"github.com/go-ansible/modules"
	"github.com/go-ansible/playbook"
	remoteexec "github.com/go-remoteexec/transport"
)

// ParseModuleArgs builds a module's argument map from an ad-hoc -a
// string. command/shell (and any module taking a single free-form
// string) get the whole string as _raw_params; every other module
// parses it as space-separated key=value pairs, Ansible's ad-hoc
// convention.
func ParseModuleArgs(module, argsStr string) map[string]any {
	switch module {
	case "command", "shell":
		return map[string]any{"_raw_params": argsStr}
	}
	out := map[string]any{}
	for _, pair := range strings.Fields(argsStr) {
		key, val, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		out[key] = val
	}
	return out
}

// Run executes moduleName/args against every named host (connecting
// via inv's own vars), optionally under become, writing one
// "host | STATUS => json" line to w as each host finishes — in
// whatever order they complete, matching real ansible's own ad-hoc
// output. Reports whether any host failed or was unreachable.
func Run(ctx context.Context, inv *inventory.Inventory, hostNames []string, moduleName string, args map[string]any, become bool, w io.Writer) bool {
	registry := modules.Default()

	var wg sync.WaitGroup
	var mu sync.Mutex
	anyFailed := false

	for _, h := range hostNames {
		wg.Add(1)
		go func(hostName string) {
			defer wg.Done()
			hostVars := inv.HostVars(hostName)

			conn, err := playbook.DefaultConnect(ctx, hostName, hostVars)
			if err != nil {
				mu.Lock()
				fmt.Fprintf(w, "%s | UNREACHABLE => %s\n", hostName, err)
				anyFailed = true
				mu.Unlock()
				return
			}
			defer conn.Close()

			var target remoteexec.Connection = conn
			if become {
				target = remoteexec.Become(conn, remoteexec.BecomeConfig{})
			}

			res, err := registry.Run(ctx, moduleName, target, args)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				fmt.Fprintf(w, "%s | UNREACHABLE => %s\n", hostName, err)
				anyFailed = true
				return
			}
			status := "SUCCESS"
			if res.Failed {
				status = "FAILED"
				anyFailed = true
			} else if res.Changed {
				// Real says CHANGED, not "SUCCESS (changed)".
				status = "CHANGED"
			}
			fmt.Fprint(w, formatResult(hostName, status, moduleName, res))
		}(h)
	}
	wg.Wait()

	return anyFailed
}

// noJSONModules are the ones real's ad-hoc callback reports as a line
// of text rather than a JSON dump — ansible's own MODULE_NO_JSON. A
// command's output is what the caller ran the command FOR; wrapping it
// in JSON with escaped newlines makes it unreadable.
var noJSONModules = map[string]bool{
	"command": true, "shell": true, "raw": true,
	"win_command": true, "win_shell": true,
}

// debugAllowedKeys is what real's _clean_results leaves on a debug
// result once it has a msg: everything else is dropped so the message
// stands alone, which is the whole point of debug.
var debugAllowedKeys = map[string]bool{"msg": true, "failed": true, "changed": true, "skipped": true}

// formatResult renders one host's result the way real's "minimal"
// callback does — the ad-hoc default, and a different shape from the
// playbook one.
func formatResult(host, status, module string, res modules.Result) string {
	name := modules.NormalizeName(module)

	// command/shell/raw: "host | CHANGED | rc=0 >>" then the output
	// itself. Real concatenates stdout, stderr and msg in that order.
	if noJSONModules[name] {
		rc := -1
		if v, ok := res.Extra["rc"].(int); ok {
			rc = v
		}
		body := stringField(res.Extra, "stdout") + stringField(res.Extra, "stderr") + res.Msg
		return fmt.Sprintf("%s | %s | rc=%d >>\n%s\n", host, status, rc, body)
	}

	payload := map[string]any{"changed": res.Changed}
	if res.Msg != "" {
		payload["msg"] = res.Msg
	}
	for k, v := range res.Extra {
		// Internal bookkeeping, never part of a result a caller sees.
		if strings.HasPrefix(k, "_ansible_") {
			continue
		}
		payload[k] = v
	}
	if len(res.Facts) > 0 {
		payload["ansible_facts"] = setupFactsDict(res.Facts)
		delete(payload, "msg")
	}
	if name == "debug" && res.Msg != "" {
		for k := range payload {
			if !debugAllowedKeys[k] {
				delete(payload, k)
			}
		}
		// A debug result carries no "changed" either — its action
		// plugin never sets one.
		delete(payload, "changed")
	}
	enc, err := template.ToJSON(payload, 4)
	if err != nil {
		return fmt.Sprintf("%s | %s => %v\n", host, status, payload)
	}
	return fmt.Sprintf("%s | %s => %s\n", host, status, enc)
}

func stringField(extra map[string]any, key string) string {
	s, _ := extra[key].(string)
	return s
}

// setupFactsDict is the shape `ansible -m setup` dumps: every fact
// under its ansible_-prefixed name, with two exceptions real keeps
// bare — gather_subset and module_setup — and the internal ones
// (leading underscore) left out entirely.
//
// This is NOT vars.InjectFacts' shape. That one carries both the bare
// map under "ansible_facts" AND the aliases, because a playbook reads
// facts both ways; nesting it here produced an ansible_facts inside
// ansible_facts and a doubled "ansible__ansible_facts_gathered".
func setupFactsDict(facts map[string]any) map[string]any {
	out := make(map[string]any, len(facts))
	for k, v := range facts {
		switch {
		case strings.HasPrefix(k, "_"):
			continue
		case k == "gather_subset" || k == "module_setup":
			out[k] = v
		default:
			out["ansible_"+k] = v
		}
	}
	return out
}
