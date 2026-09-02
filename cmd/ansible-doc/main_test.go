package main

import (
	"bytes"
	"io"
	"os"
	"strings"
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

func TestRunVersion(t *testing.T) {
	if code := run([]string{"--version"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
}

func TestRunNoArgs(t *testing.T) {
	if code := run(nil); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestRunUnknownFlag(t *testing.T) {
	if code := run([]string{"--bogus"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestRunUnknownModule(t *testing.T) {
	if code := run([]string{"no_such_module"}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
}

func TestRunKnownModulePrintsDoc(t *testing.T) {
	var code int
	out := captureStdout(t, func() { code = run([]string{"debug"}) })
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out, "DEBUG") {
		t.Fatalf("output = %q, want the module name banner", out)
	}
	if !strings.Contains(out, "Ansible's `debug` module") {
		t.Fatalf("output = %q, want the actual doc comment content", out)
	}
}

func TestRunMultipleModules(t *testing.T) {
	var code int
	out := captureStdout(t, func() { code = run([]string{"debug", "ping"}) })
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out, "DEBUG") || !strings.Contains(out, "PING") {
		t.Fatalf("output = %q, want both module banners", out)
	}
}

func TestRunMixedKnownAndUnknownReturnsError(t *testing.T) {
	if code := run([]string{"debug", "no_such_module"}); code != 1 {
		t.Fatalf("exit = %d, want 1 (partial failure)", code)
	}
}

func TestRunListPrintsAllModules(t *testing.T) {
	var code int
	out := captureStdout(t, func() { code = run([]string{"-l"}) })
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out, "debug") || !strings.Contains(out, "setup") {
		t.Fatalf("output missing expected module names: %q", out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 60 {
		t.Fatalf("got %d lines, want at least 60 (one per registered module)", len(lines))
	}
}

func TestRunListRejectsModuleNames(t *testing.T) {
	if code := run([]string{"-l", "debug"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestFirstLine(t *testing.T) {
	if got := firstLine("\n\n  hello world  \nmore text\n"); got != "hello world" {
		t.Fatalf("firstLine = %q", got)
	}
	if got := firstLine(""); got != "" {
		t.Fatalf("firstLine(\"\") = %q, want empty", got)
	}
}
