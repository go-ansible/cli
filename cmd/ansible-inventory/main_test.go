package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	f()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// The inventory every expectation below was measured against, with
// real ansible-inventory, before any of this was written.
const fixtureINI = `[web]
h1 ansible_connection=local http_port=80
h2 ansible_connection=local

[db]
h3 ansible_connection=local

[prod:children]
web
db

[prod:vars]
env=production

[all:vars]
site=paris
`

func writeFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "inv.ini")
	if err := os.WriteFile(path, []byte(fixtureINI), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// Captured verbatim from ansible-core 2.21.4. Orders matter and are
// NOT alphabetical: prod's children are web then db as the section
// wrote them, all's are ungrouped then prod.
const wantList = `{
    "_meta": {
        "hostvars": {
            "h1": {
                "ansible_connection": "local",
                "env": "production",
                "http_port": 80,
                "site": "paris"
            },
            "h2": {
                "ansible_connection": "local",
                "env": "production",
                "site": "paris"
            },
            "h3": {
                "ansible_connection": "local",
                "env": "production",
                "site": "paris"
            }
        },
        "profile": "inventory_legacy"
    },
    "all": {
        "children": [
            "ungrouped",
            "prod"
        ]
    },
    "db": {
        "hosts": [
            "h3"
        ]
    },
    "prod": {
        "children": [
            "web",
            "db"
        ]
    },
    "web": {
        "hosts": [
            "h1",
            "h2"
        ]
    }
}
`

func TestList(t *testing.T) {
	path := writeFixture(t)
	var code int
	got := captureStdout(t, func() { code = run([]string{"-i", path, "--list"}) })
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got != wantList {
		t.Errorf("--list output:\n%s\nwant:\n%s", got, wantList)
	}
}

// --export reports what the inventory SAYS: groups keep their vars and
// a host carries only its own.
const wantExport = `{
    "_meta": {
        "hostvars": {
            "h1": {
                "ansible_connection": "local",
                "http_port": 80
            },
            "h2": {
                "ansible_connection": "local"
            },
            "h3": {
                "ansible_connection": "local"
            }
        },
        "profile": "inventory_legacy"
    },
    "all": {
        "children": [
            "ungrouped",
            "prod"
        ],
        "vars": {
            "site": "paris"
        }
    },
    "db": {
        "hosts": [
            "h3"
        ]
    },
    "prod": {
        "children": [
            "web",
            "db"
        ],
        "vars": {
            "env": "production"
        }
    },
    "web": {
        "hosts": [
            "h1",
            "h2"
        ]
    }
}
`

func TestListExport(t *testing.T) {
	path := writeFixture(t)
	got := captureStdout(t, func() { run([]string{"-i", path, "--export", "--list"}) })
	if got != wantExport {
		t.Errorf("--export --list output:\n%s\nwant:\n%s", got, wantExport)
	}
}

const wantGraph = `@all:
  |--@ungrouped:
  |--@prod:
  |  |--@web:
  |  |  |--h1
  |  |  |--h2
  |  |--@db:
  |  |  |--h3
`

func TestGraph(t *testing.T) {
	path := writeFixture(t)
	got := captureStdout(t, func() { run([]string{"-i", path, "--graph"}) })
	if got != wantGraph {
		t.Errorf("--graph output:\n%s\nwant:\n%s", got, wantGraph)
	}
}

// Group vars come after the group's children and hosts, at the same
// depth as them; a host's vars come one level deeper than the host.
const wantGraphVars = `@all:
  |--@ungrouped:
  |--@prod:
  |  |--@web:
  |  |  |--h1
  |  |  |  |--{ansible_connection = local}
  |  |  |  |--{env = production}
  |  |  |  |--{http_port = 80}
  |  |  |  |--{site = paris}
  |  |  |--h2
  |  |  |  |--{ansible_connection = local}
  |  |  |  |--{env = production}
  |  |  |  |--{site = paris}
  |  |--@db:
  |  |  |--h3
  |  |  |  |--{ansible_connection = local}
  |  |  |  |--{env = production}
  |  |  |  |--{site = paris}
  |  |--{env = production}
  |--{site = paris}
`

func TestGraphVars(t *testing.T) {
	path := writeFixture(t)
	got := captureStdout(t, func() { run([]string{"-i", path, "--graph", "--vars"}) })
	if got != wantGraphVars {
		t.Errorf("--graph --vars output:\n%s\nwant:\n%s", got, wantGraphVars)
	}
}

func TestHost(t *testing.T) {
	path := writeFixture(t)
	want := `{
    "ansible_connection": "local",
    "env": "production",
    "http_port": 80,
    "site": "paris"
}
`
	got := captureStdout(t, func() { run([]string{"-i", path, "--host", "h1"}) })
	if got != want {
		t.Errorf("--host h1 output:\n%s\nwant:\n%s", got, want)
	}
}

// Measured: exit 5, not 1 or 2.
func TestNoActionExits5(t *testing.T) {
	path := writeFixture(t)
	if code := run([]string{"-i", path}); code != noActionExit {
		t.Fatalf("exit = %d, want %d", code, noActionExit)
	}
}

func TestHostMatchingNothingExits5(t *testing.T) {
	path := writeFixture(t)
	if code := run([]string{"-i", path, "--host", "nosuchhost"}); code != noActionExit {
		t.Fatalf("exit = %d, want %d", code, noActionExit)
	}
}

// --limit filters the hosts, and a group left with none loses its
// entry -- but the CHILD lists are structural and keep every name.
func TestLimitFiltersHostsNotStructure(t *testing.T) {
	path := writeFixture(t)
	got := captureStdout(t, func() { run([]string{"-i", path, "--list", "--limit", "web"}) })
	for _, want := range []string{`"h1"`, `"h2"`, `"web"`, `"db"`} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Errorf("--limit web output is missing %s:\n%s", want, got)
		}
	}
	// h3 was limited out, so db has no hosts left and no entry.
	if bytes.Contains([]byte(got), []byte(`"h3"`)) {
		t.Errorf("--limit web still lists h3:\n%s", got)
	}
	if bytes.Contains([]byte(got), []byte("\"db\": {")) {
		t.Errorf("--limit web still gives db an entry:\n%s", got)
	}
}

func TestOutputToFile(t *testing.T) {
	path := writeFixture(t)
	out := filepath.Join(t.TempDir(), "dump.json")
	if code := run([]string{"-i", path, "--list", "--output", out}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != wantList {
		t.Errorf("file contents differ from stdout output")
	}
}

// An unusable inventory is not fatal here either, and still has the
// all/ungrouped pair -- measured.
func TestUnreadableInventoryStillDumps(t *testing.T) {
	want := `{
    "_meta": {
        "hostvars": {},
        "profile": "inventory_legacy"
    },
    "all": {
        "children": [
            "ungrouped"
        ]
    }
}
`
	var code int
	got := captureStdout(t, func() { code = run([]string{"-i", "/nope.ini", "--list"}) })
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got != want {
		t.Errorf("output:\n%s\nwant:\n%s", got, want)
	}
}

// Refused explicitly rather than silently giving JSON.
func TestUnimplementedFormatsAreRefused(t *testing.T) {
	path := writeFixture(t)
	for _, f := range []string{"--yaml", "--toml"} {
		if code := run([]string{"-i", path, "--list", f}); code != 1 {
			t.Errorf("%s: exit = %d, want 1", f, code)
		}
	}
}

func TestVersion(t *testing.T) {
	if code := run([]string{"--version"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
}
