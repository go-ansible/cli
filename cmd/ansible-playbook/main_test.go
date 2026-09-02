package main

import (
	"os"
	"path/filepath"
	"testing"
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
