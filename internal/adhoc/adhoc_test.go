package adhoc

import (
	"bytes"
	"context"
	"github.com/go-ansible/modules"
	"reflect"
	"strings"
	"testing"

	"github.com/go-ansible/inventory"
)

func TestParseModuleArgsCommand(t *testing.T) {
	args, err := ParseModuleArgs("command", "echo hi there")
	if err != nil {
		t.Fatal(err)
	}
	if args["_raw_params"] != "echo hi there" {
		t.Fatalf("args = %v", args)
	}
}

func TestParseModuleArgsShell(t *testing.T) {
	args, err := ParseModuleArgs("shell", "echo a | b")
	if err != nil {
		t.Fatal(err)
	}
	if args["_raw_params"] != "echo a | b" {
		t.Fatalf("args = %v", args)
	}
}

func TestParseModuleArgsKeyValue(t *testing.T) {
	args, err := ParseModuleArgs("copy", "src=/a dest=/b")
	if err != nil {
		t.Fatal(err)
	}
	if args["src"] != "/a" || args["dest"] != "/b" {
		t.Fatalf("args = %v", args)
	}
}

// This test used to be called "SkipsBadPair" and asserted len(args)==2
// -- it PINNED the defect, requiring the stray token to be dropped.
// Real refuses it: measured, `ansible -m copy -a 'src=/a badtoken
// dest=/b'` prints
//
//	[ERROR]: Task failed: Action 'ansible.builtin.copy' does not support raw params.
//
// A stray token is a raw param, and copy cannot take one. Silently
// dropping it ran a DIFFERENT task than the one asked for.
func TestParseModuleArgsRefusesAStrayToken(t *testing.T) {
	args, err := ParseModuleArgs("copy", "src=/a badtoken dest=/b")
	if err == nil {
		t.Fatalf("a stray token was accepted: args = %v, want a refusal", args)
	}
	const want = "Action 'ansible.builtin.copy' does not support raw params."
	if err.Error() != want {
		t.Errorf("error:\n got %q\nwant %q", err.Error(), want)
	}
}

// The quoting and Jinja cases the old parser could not see at all.
func TestParseModuleArgsSeesQuotesAndJinja(t *testing.T) {
	for _, c := range []struct{ name, args, key, want string }{
		{"a quoted value keeps its spaces", `msg="hello world"`, "msg", "hello world"},
		{"a jinja block is one token", "msg={{ v }}", "msg", "{{ v }}"},
	} {
		t.Run(c.name, func(t *testing.T) {
			args, err := ParseModuleArgs("debug", c.args)
			if err != nil {
				t.Fatal(err)
			}
			if got := args[c.key]; got != c.want {
				t.Errorf("%s = %#v, want %#v", c.key, got, c.want)
			}
		})
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
	// Real says CHANGED, and reports a command as a line of text
	// rather than a JSON dump — "SUCCESS (changed)" was this port's
	// own invention.
	if want := "localhost | CHANGED | rc=0 >>\n\n"; buf.String() != want {
		t.Fatalf("output = %q, want %q", buf.String(), want)
	}
}

// TestFormatResultMatchesRealsMinimalCallback pins the ad-hoc output
// shapes MEASURED from real ansible-core 2.21.4. The ad-hoc default
// callback is "minimal", a different shape from the playbook one, and
// this port printed a JSON dump for everything.
func TestFormatResultMatchesRealsMinimalCallback(t *testing.T) {
	for _, tc := range []struct {
		name, module, status string
		res                  modules.Result
		want                 string
	}{{
		// A command's output is what the caller ran it FOR; real
		// prints it as text, with rc on the header line.
		name: "a command is text, not JSON", module: "command", status: "CHANGED",
		res:  modules.Result{Changed: true}.WithExtra("rc", 0).WithExtra("stdout", "inline\n"),
		want: "h1 | CHANGED | rc=0 >>\ninline\n\n",
	}, {
		// stdout, then stderr, then msg — real's own order.
		name: "stderr follows stdout", module: "shell", status: "CHANGED",
		res: modules.Result{Changed: true}.WithExtra("rc", 0).
			WithExtra("stdout", "out").WithExtra("stderr", "err"),
		want: "h1 | CHANGED | rc=0 >>\nouterr\n",
	}, {
		// debug's message stands alone: no changed, and none of the
		// bookkeeping keys.
		name: "debug shows only its message", module: "debug", status: "SUCCESS",
		res:  modules.Result{Msg: "hi"}.WithExtra("_ansible_verbose_always", true),
		want: "h1 | SUCCESS => {\n    \"msg\": \"hi\"\n}\n",
	}, {
		name: "an ordinary module dumps its result", module: "ping", status: "SUCCESS",
		res:  modules.Result{}.WithExtra("ping", "pong"),
		want: "h1 | SUCCESS => {\n    \"changed\": false,\n    \"ping\": \"pong\"\n}\n",
	}, {
		// _ansible_* keys are internal bookkeeping and never part of
		// a result a caller sees.
		name: "internal keys are stripped", module: "copy", status: "CHANGED",
		res:  modules.Result{Changed: true}.WithExtra("_ansible_verbose_always", true),
		want: "h1 | CHANGED => {\n    \"changed\": true\n}\n",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatResult("h1", tc.status, tc.module, tc.res); got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

// TestSetupFactsDict: `ansible -m setup` dumps every fact under its
// ansible_-prefixed name, keeping gather_subset and module_setup bare
// and leaving the internal ones out. It is NOT vars.InjectFacts'
// shape — nesting that produced an ansible_facts inside ansible_facts
// and a doubled "ansible__ansible_facts_gathered".
func TestSetupFactsDict(t *testing.T) {
	got := setupFactsDict(map[string]any{
		"system":                  "Darwin",
		"gather_subset":           []any{"all"},
		"module_setup":            true,
		"_ansible_facts_gathered": true,
	})
	want := map[string]any{
		"ansible_system": "Darwin",
		"gather_subset":  []any{"all"},
		"module_setup":   true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %#v\nwant %#v", got, want)
	}
}

// Ad-hoc arguments are TEMPLATED per host, against the same variable
// set a play would give that host (playbook.AdHocHostVars). Measured
// against real ansible-core 2.21.4: `ansible -m debug -a 'msg={{
// inventory_hostname }}'` prints the host's name, and a group_vars
// value resolves the same way.
//
// Before this, the whole matrix of eight -a shapes had one case left
// over: the string parsed correctly as a single token and was then
// passed through LITERALLY, so the module saw "{{ inventory_hostname }}".
func TestRunTemplatesItsArguments(t *testing.T) {
	// The expected text is matched as the RENDERED msg field, not
	// anywhere in the output: "localhost" alone also appears in the
	// "localhost | SUCCESS" prefix, so a bare Contains made the magic
	// variable case pass even with the templating removed. The neuter
	// is what exposed that.
	for _, c := range []struct{ name, expr, want string }{
		{"a magic variable", "{{ inventory_hostname }}", `"msg": "localhost"`},
		// Unquoted on purpose: a whole-expression template keeps its
		// NATIVE type, so this is the integer 2, not "2". Real prints
		// it unquoted too.
		{"an expression keeps its native type", "{{ 1 + 1 }}", `"msg": 2`},
		{"text around a block", "host-{{ inventory_hostname }}-end", `"msg": "host-localhost-end"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			failed := Run(context.Background(), localInventory(t), []string{"localhost"},
				"debug", map[string]any{"msg": c.expr}, false, &buf)
			if failed {
				t.Fatalf("Run reported failure: %s", buf.String())
			}
			if out := buf.String(); !strings.Contains(out, c.want) {
				t.Errorf("%q was not resolved: output = %q, want it to contain %q", c.expr, out, c.want)
			}
		})
	}
}

// The CONTROL: a literal with no template in it must come through
// untouched, so "it resolved" cannot be satisfied by mangling.
func TestRunLeavesAPlainStringAlone(t *testing.T) {
	var buf bytes.Buffer
	if failed := Run(context.Background(), localInventory(t), []string{"localhost"},
		"debug", map[string]any{"msg": "{ not a template }"}, false, &buf); failed {
		t.Fatalf("Run reported failure: %s", buf.String())
	}
	if out := buf.String(); !strings.Contains(out, `"msg": "{ not a template }"`) {
		t.Errorf("a plain string was altered: output = %q", out)
	}
}
