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
	for _, want := range []string{"remote_user", "host_key_checking", "timeout", "ANSIBLE_REMOTE_USER", "ANSIBLE_HOST_KEY_CHECKING", "ANSIBLE_TIMEOUT"} {
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
	if !strings.Contains(out, "host_key_checking(default) = true") {
		t.Fatalf("dump output = %q, want host_key_checking at its default", out)
	}
}

func TestRunDumpShowsEnvOverrideOrigin(t *testing.T) {
	t.Setenv("ANSIBLE_TIMEOUT", "99")
	var code int
	out := captureStdout(t, func() { code = run([]string{"dump"}) })
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out, "timeout(env: ANSIBLE_TIMEOUT) = 99") {
		t.Fatalf("dump output = %q, want timeout attributed to its env var", out)
	}
}
