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

func TestRunUnknownSubcommand(t *testing.T) {
	if code := run([]string{"bogus"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestRunView(t *testing.T) {
	if code := run([]string{"view"}); code != 1 {
		t.Fatalf("exit = %d, want 1 (no config file exists)", code)
	}
}

func TestRunListPrintsAllThreeSettings(t *testing.T) {
	var code int
	out := captureStdout(t, func() { code = run([]string{"list"}) })
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	// Real's canonical names, which is what ansible-config prints.
	for _, want := range []string{
		"DEFAULT_REMOTE_USER", "HOST_KEY_CHECKING", "DEFAULT_TIMEOUT",
		"ANSIBLE_REMOTE_USER", "ANSIBLE_HOST_KEY_CHECKING", "ANSIBLE_TIMEOUT",
		// The listing metadata real's own list carries.
		"section: defaults", "type: boolean", "description:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q:\n%s", want, out)
		}
	}
}

func TestRunDumpShowsDefaultByDefault(t *testing.T) {
	var code int
	out := captureStdout(t, func() { code = run([]string{"dump"}) })
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	// Real's name, and Python's spelling of the boolean.
	if !strings.Contains(out, "HOST_KEY_CHECKING(default) = True") {
		t.Fatalf("dump output = %q, want HOST_KEY_CHECKING at its default", out)
	}
}

func TestRunDumpShowsEnvOverrideOrigin(t *testing.T) {
	t.Setenv("ANSIBLE_TIMEOUT", "99")
	var code int
	out := captureStdout(t, func() { code = run([]string{"dump"}) })
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out, "DEFAULT_TIMEOUT(env: ANSIBLE_TIMEOUT) = 99") {
		t.Fatalf("dump output = %q, want the timeout attributed to its env var", out)
	}
}

// isolateConfigDiscovery points cwd/$HOME at fresh temp dirs and clears
// $ANSIBLE_CONFIG, so a test asserting on a real ansible.cfg it writes
// can't pick up something unrelated on the machine actually running it.
func isolateConfigDiscovery(t *testing.T) {
	t.Helper()
	t.Setenv("ANSIBLE_CONFIG", "")
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
}

func TestRunViewPrintsRealConfigFile(t *testing.T) {
	isolateConfigDiscovery(t)
	if err := os.WriteFile("ansible.cfg", []byte("[defaults]\nremote_user = cfguser\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var code int
	out := captureStdout(t, func() { code = run([]string{"view"}) })
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out, "remote_user = cfguser") {
		t.Fatalf("view output = %q, want the real file's content", out)
	}
}

func TestRunDumpShowsConfigFileOrigin(t *testing.T) {
	isolateConfigDiscovery(t)
	if err := os.WriteFile("ansible.cfg", []byte("[defaults]\nremote_user = cfguser\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var code int
	out := captureStdout(t, func() { code = run([]string{"dump"}) })
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	// Real spells an ini origin as the PATH ALONE, not "cfg: <path>".
	if !strings.Contains(out, "DEFAULT_REMOTE_USER(") || !strings.Contains(out, "ansible.cfg) = cfguser") {
		t.Fatalf("dump output = %q, want DEFAULT_REMOTE_USER attributed to the config file by path", out)
	}
	// A setting the file doesn't set must NOT be attributed to the file
	// just because one exists — real regression this test locks in.
	if !strings.Contains(out, "DEFAULT_TIMEOUT(default) = 10") {
		t.Fatalf("dump output = %q, want timeout still at its default (the file doesn't set it)", out)
	}
}
