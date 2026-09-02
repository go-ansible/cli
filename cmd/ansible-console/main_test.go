package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-ansible/inventory"
)

func localInventory(t *testing.T) *inventory.Inventory {
	t.Helper()
	inv, err := inventory.ParseYAML([]byte(`
all:
  hosts:
    localhost:
      ansible_connection: local
`))
	if err != nil {
		t.Fatal(err)
	}
	return inv
}

func TestRunVersion(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"--version"}, strings.NewReader(""), &out); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "ansible-console") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRunMissingInventory(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{}, strings.NewReader(""), &out); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestRunBadInventoryPath(t *testing.T) {
	var out bytes.Buffer
	code := run([]string{"-i", "/no/such/inventory.yml"}, strings.NewReader(""), &out)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
}

func TestRunBadPattern(t *testing.T) {
	dir := t.TempDir()
	inv := filepath.Join(dir, "inv.yml")
	if err := os.WriteFile(inv, []byte("all:\n  hosts:\n    localhost: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := run([]string{"-i", inv, "[bad"}, strings.NewReader(""), &out)
	if code != 1 {
		t.Fatalf("exit = %d, want 1 for an unparsable pattern", code)
	}
}

func TestSessionPromptShowsPatternAndCount(t *testing.T) {
	var out bytes.Buffer
	s, err := newSession(localInventory(t), "all", &out)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.prompt(); got != "all (1)> " {
		t.Fatalf("prompt = %q", got)
	}
}

func TestHandleLineExitQuit(t *testing.T) {
	var out bytes.Buffer
	s, err := newSession(localInventory(t), "all", &out)
	if err != nil {
		t.Fatal(err)
	}
	if !s.handleLine("exit") {
		t.Fatal("exit should end the session")
	}
	if !s.handleLine("quit") {
		t.Fatal("quit should end the session")
	}
}

func TestHandleLineBlankIsNoop(t *testing.T) {
	var out bytes.Buffer
	s, _ := newSession(localInventory(t), "all", &out)
	if s.handleLine("   ") {
		t.Fatal("a blank line should not end the session")
	}
	if out.Len() != 0 {
		t.Fatalf("blank line produced output: %q", out.String())
	}
}

func TestHandleLineHelp(t *testing.T) {
	var out bytes.Buffer
	s, _ := newSession(localInventory(t), "all", &out)
	s.handleLine("help")
	if !strings.Contains(out.String(), "Commands:") {
		t.Fatalf("output = %q", out.String())
	}
	out.Reset()
	s.handleLine("?")
	if !strings.Contains(out.String(), "Commands:") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestHandleLineList(t *testing.T) {
	var out bytes.Buffer
	s, _ := newSession(localInventory(t), "all", &out)
	s.handleLine("list")
	if strings.TrimSpace(out.String()) != "localhost" {
		t.Fatalf("output = %q", out.String())
	}
}

func TestHandleLineCdChangesPattern(t *testing.T) {
	var out bytes.Buffer
	s, _ := newSession(localInventory(t), "all", &out)
	s.handleLine("cd localhost")
	if s.pattern != "localhost" || len(s.hosts) != 1 {
		t.Fatalf("pattern = %q hosts = %v", s.pattern, s.hosts)
	}
}

func TestHandleLineCdNoArgDefaultsToAll(t *testing.T) {
	var out bytes.Buffer
	s, _ := newSession(localInventory(t), "localhost", &out)
	s.handleLine("cd")
	if s.pattern != "all" {
		t.Fatalf("pattern = %q, want all", s.pattern)
	}
}

func TestHandleLineCdBadPatternReportsErrorKeepsOldPattern(t *testing.T) {
	var out bytes.Buffer
	s, _ := newSession(localInventory(t), "all", &out)
	s.handleLine("cd [bad")
	if s.pattern != "all" {
		t.Fatalf("pattern = %q, want unchanged \"all\" after a bad cd", s.pattern)
	}
	if !strings.Contains(out.String(), "ansible-console:") {
		t.Fatalf("output = %q, want an error message", out.String())
	}
}

func TestHandleLineBecomeToggle(t *testing.T) {
	var out bytes.Buffer
	s, _ := newSession(localInventory(t), "all", &out)
	if s.become {
		t.Fatal("become should start disabled")
	}
	s.handleLine("become")
	if !s.become {
		t.Fatal("become should be enabled")
	}
	s.handleLine("nobecome")
	if s.become {
		t.Fatal("become should be disabled again")
	}
}

func TestHandleLineNoHostsMatched(t *testing.T) {
	var out bytes.Buffer
	s, _ := newSession(localInventory(t), "all", &out)
	s.handleLine("cd nomatch")
	out.Reset()
	s.handleLine("echo hi")
	if !strings.Contains(out.String(), "matches no hosts") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestHandleLineRunsRegisteredModuleByName(t *testing.T) {
	var out bytes.Buffer
	s, _ := newSession(localInventory(t), "all", &out)
	s.handleLine("debug msg=hello")
	if !strings.Contains(out.String(), "localhost | SUCCESS") || !strings.Contains(out.String(), "hello") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestHandleLineRunsBareLineAsShell(t *testing.T) {
	var out bytes.Buffer
	s, _ := newSession(localInventory(t), "all", &out)
	s.handleLine("echo from the console")
	if !strings.Contains(out.String(), "localhost | SUCCESS") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRunREPLEndToEnd(t *testing.T) {
	var out bytes.Buffer
	s, err := newSession(localInventory(t), "all", &out)
	if err != nil {
		t.Fatal(err)
	}
	in := strings.NewReader("list\ndebug msg=hi\nexit\n")
	runREPL(s, in, &out)
	got := out.String()
	if !strings.Contains(got, "localhost\n") {
		t.Fatalf("output missing list result: %q", got)
	}
	if !strings.Contains(got, "SUCCESS") {
		t.Fatalf("output missing module result: %q", got)
	}
}

func TestRunREPLStopsOnEOFWithoutExitCommand(t *testing.T) {
	var out bytes.Buffer
	s, err := newSession(localInventory(t), "all", &out)
	if err != nil {
		t.Fatal(err)
	}
	// No trailing "exit" — EOF alone must end the loop (it would hang
	// forever otherwise).
	in := strings.NewReader("list\n")
	done := make(chan struct{})
	go func() {
		runREPL(s, in, &out)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runREPL did not return on EOF")
	}
}
