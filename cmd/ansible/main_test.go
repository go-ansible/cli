package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractPatternFirst(t *testing.T) {
	pattern, flags, err := extractPattern([]string{"all", "-i", "inv.yml", "-m", "command", "-a", "echo hi"})
	if err != nil {
		t.Fatal(err)
	}
	if pattern != "all" {
		t.Fatalf("pattern = %q", pattern)
	}
	want := []string{"-i", "inv.yml", "-m", "command", "-a", "echo hi"}
	if len(flags) != len(want) {
		t.Fatalf("flags = %v", flags)
	}
	for i := range want {
		if flags[i] != want[i] {
			t.Fatalf("flags[%d] = %q, want %q", i, flags[i], want[i])
		}
	}
}

func TestExtractPatternLast(t *testing.T) {
	pattern, _, err := extractPattern([]string{"-i", "inv.yml", "all"})
	if err != nil {
		t.Fatal(err)
	}
	if pattern != "all" {
		t.Fatalf("pattern = %q", pattern)
	}
}

func TestExtractPatternBooleanFlagNotConsumed(t *testing.T) {
	pattern, flags, err := extractPattern([]string{"all", "-b", "-i", "inv.yml"})
	if err != nil {
		t.Fatal(err)
	}
	if pattern != "all" {
		t.Fatalf("pattern = %q", pattern)
	}
	if len(flags) != 3 || flags[0] != "-b" {
		t.Fatalf("flags = %v, -b should not consume the next token", flags)
	}
}

func TestExtractPatternEqualsForm(t *testing.T) {
	pattern, flags, err := extractPattern([]string{"all", "--inventory=inv.yml"})
	if err != nil {
		t.Fatal(err)
	}
	if pattern != "all" || len(flags) != 1 {
		t.Fatalf("pattern=%q flags=%v", pattern, flags)
	}
}

func TestExtractPatternMissingFlagValue(t *testing.T) {
	if _, _, err := extractPattern([]string{"all", "-i"}); err == nil {
		t.Fatal("want error when a value-flag has no following token")
	}
}

func TestExtractPatternTwoPositionals(t *testing.T) {
	if _, _, err := extractPattern([]string{"all", "web"}); err == nil {
		t.Fatal("want error for two non-flag tokens")
	}
}

func TestExtractPatternNone(t *testing.T) {
	pattern, _, err := extractPattern([]string{"-i", "inv.yml"})
	if err != nil {
		t.Fatal(err)
	}
	if pattern != "" {
		t.Fatalf("pattern = %q, want empty", pattern)
	}
}

func TestRunEndToEnd(t *testing.T) {
	dir := t.TempDir()
	inv := filepath.Join(dir, "inv.yml")
	if err := os.WriteFile(inv, []byte("all:\n  hosts:\n    localhost:\n      ansible_connection: local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code := run([]string{"all", "-i", inv, "-m", "command", "-a", "echo hi"})
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
}

func TestRunNoHostsMatched(t *testing.T) {
	dir := t.TempDir()
	inv := filepath.Join(dir, "inv.yml")
	if err := os.WriteFile(inv, []byte("all:\n  hosts:\n    localhost: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A pattern that matched nothing is NOT an error. Measured against
	// ansible-core 2.21.4: `ansible -i inv nomatch -m ping` exits 0,
	// having warned "Could not match supplied host pattern" on stderr
	// and run the module on no hosts. This asserted 1 -- our exit
	// status, not real's.
	code := run([]string{"nomatch", "-i", inv, "-m", "command", "-a", "echo hi"})
	if code != 0 {
		t.Fatalf("exit = %d, want 0: an unmatched pattern warns, it does not fail", code)
	}
}

func TestRunModuleFails(t *testing.T) {
	dir := t.TempDir()
	inv := filepath.Join(dir, "inv.yml")
	if err := os.WriteFile(inv, []byte("all:\n  hosts:\n    localhost:\n      ansible_connection: local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code := run([]string{"all", "-i", inv, "-m", "fail", "-a", "msg=boom"})
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestRunMissingRequiredFlags(t *testing.T) {
	if code := run([]string{}); code != 2 {
		t.Fatalf("exit = %d", code)
	}
	if code := run([]string{"-i", "inv.yml"}); code != 2 {
		t.Fatalf("exit = %d, want 2 for missing pattern", code)
	}
}

// An inventory that cannot be read is NOT fatal either: real warns and
// carries on with the implicit localhost.
func TestRunInventoryErrorIsNotFatal(t *testing.T) {
	code := run([]string{"all", "-i", filepath.Join(t.TempDir(), "absent.yml"), "-m", "command"})
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
}

// And the invocation real users type most often -- no -i at all --
// must work, because the implicit localhost always exists.
func TestRunWithoutInventoryTargetsImplicitLocalhost(t *testing.T) {
	if code := run([]string{"localhost", "-m", "ping"}); code != 0 {
		t.Fatalf("exit = %d, want 0 for `ansible localhost -m ping`", code)
	}
}

func TestRunVersion(t *testing.T) {
	if code := run([]string{"--version"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
}
