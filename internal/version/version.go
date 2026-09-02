// Package version formats a one-line --version string shared by all
// four go-ansible CLI binaries.
package version

import (
	"fmt"
	"runtime/debug"
)

// String returns a one-line version string for tool: the module
// version Go's build info records when the binary was built with `go
// install pkg@version` (or from a tagged module dependency), or "dev"
// for a local `go build` inside a repo checkout, which has no such
// version embedded.
func String(tool string) string {
	v := "dev"
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		v = info.Main.Version
	}
	return fmt.Sprintf("%s %s (go-ansible, a pure-Go port of Ansible)", tool, v)
}
