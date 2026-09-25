package invload

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// collect returns a Warner that records what it was told, and the
// slice it records into.
func collect() (func(string), *[]string) {
	var got []string
	return func(msg string) { got = append(got, msg) }, &got
}

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// Every expectation here is what ansible-core 2.21.4 printed on stderr
// for the same input, captured before the code was written.
func TestLoadWarnings(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope.ini")
	bad := write(t, dir, "bad.ini", "[unclosed\nh1\n")
	empty := write(t, dir, "empty.ini", "")
	good := write(t, dir, "good.ini", "h1\n")

	cases := []struct {
		name    string
		path    string
		pattern string
		want    []string
	}{{
		// Measured: two warnings, and NO parse-failure line -- real's
		// plugins only report one once they have tried to parse.
		name: "missing source", path: missing, pattern: "all",
		want: []string{
			"Unable to parse " + missing + " as an inventory source",
			"No inventory was parsed, only implicit localhost is available",
			"provided hosts list is empty, only localhost is available. Note that the implicit localhost does not match 'all'",
		},
	}, {
		// Measured: a source that EXISTS and fails to parse is also
		// reported by cause. Real says more (a structured block with
		// the plugin and a source position); the first line is the
		// part this port can produce.
		name: "unparseable source", path: bad, pattern: "all",
		want: []string{
			`Failed to parse inventory: line 1: unterminated section header "[unclosed"`,
			"Unable to parse " + bad + " as an inventory source",
			"No inventory was parsed, only implicit localhost is available",
			"provided hosts list is empty, only localhost is available. Note that the implicit localhost does not match 'all'",
		},
	}, {
		// Measured: an EMPTY file parses. It gets the empty-hosts
		// warning only -- not "No inventory was parsed".
		name: "empty but valid source", path: empty, pattern: "all",
		want: []string{
			"provided hosts list is empty, only localhost is available. Note that the implicit localhost does not match 'all'",
		},
	}, {
		// Measured: no -i at all still warns that nothing was parsed.
		name: "no source", path: "", pattern: "all",
		want: []string{
			"No inventory was parsed, only implicit localhost is available",
			"provided hosts list is empty, only localhost is available. Note that the implicit localhost does not match 'all'",
		},
	}, {
		// Measured: `ansible -i /nope.ini localhost -m ping` prints
		// TWO warnings where the same command with pattern h1 prints
		// three -- real suppresses the empty-hosts one when the
		// pattern is one the implicit localhost answers to.
		name: "localhost pattern suppresses the empty warning", path: missing, pattern: "localhost",
		want: []string{
			"Unable to parse " + missing + " as an inventory source",
			"No inventory was parsed, only implicit localhost is available",
		},
	}, {
		name: "usable source warns nothing", path: good, pattern: "all",
		want: nil,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			warn, got := collect()
			inv := Load(tc.path, "", tc.pattern, warn)
			if inv == nil {
				t.Fatal("Load returned nil inventory; it must never do that")
			}
			if strings.Join(*got, "\n") != strings.Join(tc.want, "\n") {
				t.Errorf("warnings =\n  %s\nwant\n  %s",
					strings.Join(*got, "\n  "), strings.Join(tc.want, "\n  "))
			}
		})
	}
}

// The contract that makes the caller simpler: Load never fails, so an
// unusable inventory cannot end a run. The returned inventory still
// answers to the implicit localhost.
func TestLoadNeverFailsAndKeepsImplicitLocalhost(t *testing.T) {
	warn, _ := collect()
	inv := Load(filepath.Join(t.TempDir(), "absent.ini"), "", "all", warn)
	hosts, err := inv.Match("localhost")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if len(hosts) != 1 || hosts[0].Name != "localhost" {
		t.Fatalf("Match(localhost) = %v, want the implicit localhost", hosts)
	}
}

// Real reports the ABSOLUTE path even when -i was given a relative
// one. Measured: `-i nope.ini` warned about /full/path/nope.ini.
func TestLoadReportsAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	warn, got := collect()
	Load("nope.ini", "", "all", warn)
	if len(*got) == 0 || !filepath.IsAbs(strings.TrimSuffix(strings.TrimPrefix((*got)[0], "Unable to parse "), " as an inventory source")) {
		t.Fatalf("first warning = %q, want an absolute path in it", (*got))
	}
}
