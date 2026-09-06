package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStringList(t *testing.T) {
	var s stringList
	if err := s.Set("a"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("b"); err != nil {
		t.Fatal(err)
	}
	if s.String() != "[a b]" {
		t.Fatalf("String() = %q", s.String())
	}
}

func TestRunSuccess(t *testing.T) {
	dir := t.TempDir()
	inv := filepath.Join(dir, "inv.yml")
	pb := filepath.Join(dir, "site.yml")
	writeFile(t, inv, "all:\n  hosts:\n    localhost:\n      ansible_connection: local\n")
	writeFile(t, pb, "- hosts: all\n  gather_facts: false\n  tasks:\n    - debug: {}\n")

	code := run([]string{"-i", inv, pb})
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
}

func TestRunFailure(t *testing.T) {
	dir := t.TempDir()
	inv := filepath.Join(dir, "inv.yml")
	pb := filepath.Join(dir, "site.yml")
	writeFile(t, inv, "all:\n  hosts:\n    localhost:\n      ansible_connection: local\n")
	writeFile(t, pb, "- hosts: all\n  gather_facts: false\n  tasks:\n    - fail:\n        msg: boom\n")

	code := run([]string{"-i", inv, "--no-color", pb})
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestRunMissingInventoryOrPlaybook(t *testing.T) {
	if code := run([]string{}); code != 2 {
		t.Fatalf("exit = %d", code)
	}
	if code := run([]string{"-i", "inv.yml"}); code != 2 {
		t.Fatalf("exit = %d, want 2 for missing playbook arg", code)
	}
}

func TestRunBadFlags(t *testing.T) {
	if code := run([]string{"-bogus-flag"}); code != 2 {
		t.Fatalf("exit = %d", code)
	}
}

func TestRunInventoryLoadError(t *testing.T) {
	dir := t.TempDir()
	pb := filepath.Join(dir, "site.yml")
	writeFile(t, pb, "- hosts: all\n  tasks: []\n")
	code := run([]string{"-i", filepath.Join(dir, "absent.yml"), pb})
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
}

func TestRunPlaybookFileMissing(t *testing.T) {
	dir := t.TempDir()
	inv := filepath.Join(dir, "inv.yml")
	writeFile(t, inv, "all:\n  hosts:\n    localhost: {}\n")
	code := run([]string{"-i", inv, filepath.Join(dir, "absent.yml")})
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
}

func TestRunPlaybookParseError(t *testing.T) {
	dir := t.TempDir()
	inv := filepath.Join(dir, "inv.yml")
	pb := filepath.Join(dir, "site.yml")
	writeFile(t, inv, "all:\n  hosts:\n    localhost: {}\n")
	writeFile(t, pb, "not: [valid playbook")
	code := run([]string{"-i", inv, pb})
	if code != 4 {
		t.Fatalf("exit = %d, want 4", code)
	}
}

func TestRunExtraVarsError(t *testing.T) {
	dir := t.TempDir()
	inv := filepath.Join(dir, "inv.yml")
	pb := filepath.Join(dir, "site.yml")
	writeFile(t, inv, "all:\n  hosts:\n    localhost: {}\n")
	writeFile(t, pb, "- hosts: all\n  tasks: []\n")
	code := run([]string{"-i", inv, "-e", "not-key-value", pb})
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestRunExtraVarsApplied(t *testing.T) {
	dir := t.TempDir()
	inv := filepath.Join(dir, "inv.yml")
	pb := filepath.Join(dir, "site.yml")
	writeFile(t, inv, "all:\n  hosts:\n    localhost:\n      ansible_connection: local\n")
	writeFile(t, pb, `- hosts: all
  gather_facts: false
  tasks:
    - name: check
      fail:
        msg: "got {{ x }}"
      when: x != "expected"
`)
	code := run([]string{"-i", inv, "-e", "x=expected", pb})
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (extra var should have satisfied the when)", code)
	}
}

func TestRunTagsSkipsUntaggedTask(t *testing.T) {
	dir := t.TempDir()
	inv := filepath.Join(dir, "inv.yml")
	pb := filepath.Join(dir, "site.yml")
	writeFile(t, inv, "all:\n  hosts:\n    localhost:\n      ansible_connection: local\n")
	writeFile(t, pb, `- hosts: all
  gather_facts: false
  tasks:
    - name: should not run
      tags: [never-selected]
      fail:
        msg: boom
    - name: should run
      tags: [selected]
      debug: {}
`)
	code := run([]string{"-i", inv, "--tags", "selected", pb})
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (the failing task should have been filtered out by --tags)", code)
	}
}

func TestRunSkipTagsExcludesFailingTask(t *testing.T) {
	dir := t.TempDir()
	inv := filepath.Join(dir, "inv.yml")
	pb := filepath.Join(dir, "site.yml")
	writeFile(t, inv, "all:\n  hosts:\n    localhost:\n      ansible_connection: local\n")
	writeFile(t, pb, `- hosts: all
  gather_facts: false
  tasks:
    - name: should be skipped
      tags: [broken]
      fail:
        msg: boom
`)
	code := run([]string{"-i", inv, "--skip-tags", "broken", pb})
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (the failing task should have been excluded by --skip-tags)", code)
	}
}

func TestRunTagsCommaSeparatedWithinOneFlag(t *testing.T) {
	dir := t.TempDir()
	inv := filepath.Join(dir, "inv.yml")
	pb := filepath.Join(dir, "site.yml")
	writeFile(t, inv, "all:\n  hosts:\n    localhost:\n      ansible_connection: local\n")
	writeFile(t, pb, `- hosts: all
  gather_facts: false
  tasks:
    - name: a
      tags: [a]
      debug: {}
    - name: b
      tags: [b]
      debug: {}
    - name: c
      tags: [c]
      fail:
        msg: boom
`)
	code := run([]string{"-i", inv, "--tags", "a,b", pb})
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (comma-separated --tags a,b should select a and b, excluding failing c)", code)
	}
}

func TestRunRoleResolvesRelativeToPlaybookFile(t *testing.T) {
	dir := t.TempDir()
	inv := filepath.Join(dir, "inv.yml")
	pb := filepath.Join(dir, "site.yml")
	writeFile(t, inv, "all:\n  hosts:\n    localhost:\n      ansible_connection: local\n")
	if err := os.MkdirAll(filepath.Join(dir, "roles", "myrole", "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "roles", "myrole", "tasks", "main.yml"), "- debug: {}\n")
	writeFile(t, pb, `- hosts: all
  gather_facts: false
  roles:
    - myrole
`)
	code := run([]string{"-i", inv, pb})
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (roles/ should resolve relative to the playbook file, not the cwd)", code)
	}
}

func TestRunVersion(t *testing.T) {
	if code := run([]string{"--version"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
}

// TestRunForksFlagLimitsConcurrency proves --forks actually reaches
// Engine.Forks through this real CLI entrypoint (not just the library
// API playbook's own tests exercise), using real wall-clock timing
// like those tests: 4 hosts each sleep 200ms; --forks 2 can only
// overlap two at a time, so the whole run takes at least two
// sequential rounds (~400ms).
func TestRunForksFlagLimitsConcurrency(t *testing.T) {
	dir := t.TempDir()
	inv := filepath.Join(dir, "inv.yml")
	pb := filepath.Join(dir, "site.yml")
	writeFile(t, inv, "all:\n  hosts:\n"+
		"    h1:\n      ansible_connection: local\n"+
		"    h2:\n      ansible_connection: local\n"+
		"    h3:\n      ansible_connection: local\n"+
		"    h4:\n      ansible_connection: local\n")
	writeFile(t, pb, "- hosts: all\n  gather_facts: false\n  tasks:\n    - command: sleep 0.2\n")

	start := time.Now()
	code := run([]string{"-i", inv, "--forks", "2", pb})
	elapsed := time.Since(start)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if elapsed < 350*time.Millisecond {
		t.Fatalf("elapsed = %v, want >= ~400ms (2 sequential rounds of 4 hosts at --forks 2) — the flag doesn't appear to reach Engine.Forks", elapsed)
	}
}
