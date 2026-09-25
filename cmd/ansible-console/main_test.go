package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
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
	// Real's shape, measured: user@pattern (count)[f:forks]$ -- this
	// asserted "all (1)> ", which was this port's own invention.
	if got := s.prompt(); !strings.HasSuffix(got, "@all (1)[f:5]$ ") {
		t.Fatalf("prompt = %q, want it to end in \"@all (1)[f:5]$ \"", got)
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

// Measured: bare "cd" goes back to everything, which real spells "*"
// -- the prompt then shows "*". This asserted "all", our own spelling.
func TestHandleLineCdNoArgSelectsEverything(t *testing.T) {
	var out bytes.Buffer
	s, _ := newSession(localInventory(t), "localhost", &out)
	s.handleLine("cd")
	if s.pattern != "*" {
		t.Fatalf("pattern = %q, want \"*\"", s.pattern)
	}
}

func TestHandleLineCdBadPatternReportsErrorKeepsOldPattern(t *testing.T) {
	var out bytes.Buffer
	s, _ := newSession(localInventory(t), "all", &out)
	s.handleLine("cd [bad")
	if s.pattern != "all" {
		t.Fatalf("pattern = %q, want unchanged \"all\" after a bad cd", s.pattern)
	}
	if !strings.Contains(out.String(), "[ERROR]:") {
		t.Fatalf("output = %q, want an error message", out.String())
	}
}

// Measured: real does NOT toggle on a bare "become" -- it asks for a
// value. This asserted the toggle, which was this port's behaviour;
// the refusal is covered by TestBareBecomeIsRefused.
func TestHandleLineBecomeAndNobecome(t *testing.T) {
	var out bytes.Buffer
	s, _ := newSession(localInventory(t), "all", &out)
	if s.become {
		t.Fatal("become should start disabled")
	}
	s.handleLine("become yes")
	if !s.become {
		t.Fatal("become yes should enable it")
	}
	s.handleLine("nobecome")
	if s.become {
		t.Fatal("nobecome should disable it")
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
	// A shell/command result reads as real's ad-hoc callback writes
	// it — "host | CHANGED | rc=0 >>" and then the output itself —
	// not as a JSON dump. This asserted SUCCESS, which was this
	// port's own wording for a changed command.
	if want := "localhost | CHANGED | rc=0 >>\nfrom the console\n"; out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
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

// The prompt is real's, measured: the user the console would connect
// as, the pattern, how many hosts it matches, and the fork count --
// ending in "#" rather than "$" when become is on.
func TestPromptShape(t *testing.T) {
	var out bytes.Buffer
	s, err := newSession(localInventory(t), "all", &out)
	if err != nil {
		t.Fatal(err)
	}
	s.remoteUser = "someone"
	want := "someone@all (" + itoa(len(s.hosts)) + ")[f:5]$ "
	if got := s.prompt(); got != want {
		t.Errorf("prompt = %q, want %q", got, want)
	}
	s.become = true
	if got := s.prompt(); !strings.HasSuffix(got, "# ") {
		t.Errorf("with become, prompt = %q, want it to end in \"# \"", got)
	}
	s.become = false
	s.handleLine("forks 2")
	if got := s.prompt(); !strings.Contains(got, "[f:2]") {
		t.Errorf("after forks 2, prompt = %q", got)
	}
	s.handleLine("remote_user root")
	if got := s.prompt(); !strings.HasPrefix(got, "root@") {
		t.Errorf("after remote_user root, prompt = %q", got)
	}
}

// Measured: real refuses a bare `become` rather than toggling, because
// the prompt already shows the state and a silent toggle would be
// ambiguous.
func TestBareBecomeIsRefused(t *testing.T) {
	var out bytes.Buffer
	s, _ := newSession(localInventory(t), "all", &out)
	s.handleLine("become")
	if !strings.Contains(out.String(), "Please specify become value, e.g. `become yes`") {
		t.Errorf("output = %q", out.String())
	}
	if s.become {
		t.Error("a bare become turned it on")
	}
	// And anything not plainly affirmative leaves it off, so a typo
	// cannot leave a session silently escalated.
	for _, v := range []string{"yes", "true", "on", "1"} {
		s.become = false
		s.handleLine("become " + v)
		if !s.become {
			t.Errorf("become %s did not enable it", v)
		}
	}
	for _, v := range []string{"no", "false", "bogus", "0"} {
		s.become = true
		s.handleLine("become " + v)
		if s.become {
			t.Errorf("become %s left it on", v)
		}
	}
}

func TestVerbosityReportsTheNewLevel(t *testing.T) {
	var out bytes.Buffer
	s, _ := newSession(localInventory(t), "all", &out)
	s.handleLine("verbosity 2")
	if !strings.Contains(out.String(), "verbosity level set to 2") {
		t.Errorf("output = %q", out.String())
	}
	if s.verbosity != 2 {
		t.Errorf("verbosity = %d", s.verbosity)
	}
	// A value that is not a number changes nothing and says so.
	out.Reset()
	s.handleLine("verbosity notanumber")
	if s.verbosity != 2 {
		t.Errorf("a bad value changed the level to %d", s.verbosity)
	}
}

// Bare `cd` goes back to everything, which real spells "*" -- and the
// prompt then shows "*", not "all".
func TestBareCdSelectsEverything(t *testing.T) {
	var out bytes.Buffer
	s, _ := newSession(localInventory(t), "all", &out)
	s.handleLine("cd")
	if s.pattern != "*" {
		t.Errorf("pattern = %q, want \"*\"", s.pattern)
	}
}

// The banner is printed once at startup and the farewell however the
// session ends -- including when the input simply runs out, which is
// what a piped script does.
func TestBannerAndFarewell(t *testing.T) {
	var out bytes.Buffer
	s, _ := newSession(localInventory(t), "all", &out)
	runREPL(s, strings.NewReader("list\n"), &out)
	got := out.String()
	if !strings.HasPrefix(got, consoleBanner+"\n\n") {
		t.Errorf("output does not start with the banner: %q", got[:min(80, len(got))])
	}
	if !strings.HasSuffix(got, consoleFarewell+"\n") {
		t.Errorf("output does not end with the farewell: %q", got)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
