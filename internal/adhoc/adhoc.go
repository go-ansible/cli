// Package adhoc is the ad-hoc single-module-against-a-host-pattern
// execution logic shared by cmd/ansible and cmd/ansible-console — the
// same "connect, run one module on every matched host, print a
// host | STATUS => json line as each finishes" flow both binaries
// need, factored out once instead of copied.
package adhoc

import (
	"context"
	"encoding/json"
	"fmt"
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
				status = "SUCCESS (changed)"
			}
			payload := map[string]any{"changed": res.Changed, "msg": res.Msg}
			for k, v := range res.Extra {
				payload[k] = v
			}
			enc, _ := json.MarshalIndent(payload, "", "    ")
			fmt.Fprintf(w, "%s | %s => %s\n", hostName, status, enc)
		}(h)
	}
	wg.Wait()

	return anyFailed
}
