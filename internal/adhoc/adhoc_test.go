package adhoc

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/go-ansible/inventory"
)

func TestParseModuleArgsCommand(t *testing.T) {
	args := ParseModuleArgs("command", "echo hi there")
	if args["_raw_params"] != "echo hi there" {
		t.Fatalf("args = %v", args)
	}
}

func TestParseModuleArgsShell(t *testing.T) {
	args := ParseModuleArgs("shell", "echo a | b")
	if args["_raw_params"] != "echo a | b" {
		t.Fatalf("args = %v", args)
	}
}

func TestParseModuleArgsKeyValue(t *testing.T) {
	args := ParseModuleArgs("copy", "src=/a dest=/b")
	if args["src"] != "/a" || args["dest"] != "/b" {
		t.Fatalf("args = %v", args)
	}
}

func TestParseModuleArgsSkipsBadPair(t *testing.T) {
	args := ParseModuleArgs("copy", "src=/a badtoken dest=/b")
	if len(args) != 2 || args["src"] != "/a" || args["dest"] != "/b" {
		t.Fatalf("args = %v", args)
	}
}

func localInventory(t *testing.T) *inventory.Inventory {
	t.Helper()
	inv, err := inventory.ParseYAML([]byte("all:\n  hosts:\n    localhost:\n      ansible_connection: local\n"))
	if err != nil {
		t.Fatal(err)
	}
	return inv
}

func TestRunSuccess(t *testing.T) {
	var buf bytes.Buffer
	failed := Run(context.Background(), localInventory(t), []string{"localhost"}, "debug", map[string]any{"msg": "hi"}, false, &buf)
	if failed {
		t.Fatalf("Run reported failure: %s", buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "localhost | SUCCESS") || !strings.Contains(out, "\"hi\"") {
		t.Fatalf("output = %q", out)
	}
}

func TestRunModuleFailure(t *testing.T) {
	var buf bytes.Buffer
	failed := Run(context.Background(), localInventory(t), []string{"localhost"}, "fail", map[string]any{"msg": "boom"}, false, &buf)
	if !failed {
		t.Fatal("want Run to report failure")
	}
	if !strings.Contains(buf.String(), "localhost | FAILED") {
		t.Fatalf("output = %q", buf.String())
	}
}

func TestRunUnreachableHost(t *testing.T) {
	inv := localInventory(t)
	var buf bytes.Buffer
	failed := Run(context.Background(), inv, []string{"no-such-host"}, "command", map[string]any{"_raw_params": "echo hi"}, false, &buf)
	if !failed {
		t.Fatal("want Run to report failure for an unreachable host")
	}
	if !strings.Contains(buf.String(), "no-such-host | UNREACHABLE") {
		t.Fatalf("output = %q", buf.String())
	}
}

func TestRunUnknownModuleReportsFailed(t *testing.T) {
	var buf bytes.Buffer
	failed := Run(context.Background(), localInventory(t), []string{"localhost"}, "no_such_module", nil, false, &buf)
	if !failed {
		t.Fatal("want Run to report failure for an unregistered module")
	}
}

func TestRunChangedStatus(t *testing.T) {
	var buf bytes.Buffer
	failed := Run(context.Background(), localInventory(t), []string{"localhost"}, "command", map[string]any{"_raw_params": "true"}, false, &buf)
	if failed {
		t.Fatalf("Run reported failure: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "SUCCESS (changed)") {
		t.Fatalf("output = %q, want a changed command to report SUCCESS (changed)", buf.String())
	}
}
