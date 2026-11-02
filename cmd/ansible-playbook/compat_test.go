package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const retiredPlaybook = `- hosts: all
  gather_facts: no
  sudo: yes
  tasks:
    - debug:
        msg: ok
      always_run: yes
`

const currentPlaybook = `- hosts: all
  gather_facts: no
  become: true
  tasks:
    - debug:
        msg: ok
`

// TestCompatReportRunFindsRetiredConstructs checks both halves of the
// verdict. A report that only ever printed findings would pass a test
// that only fed it a bad playbook, and a user whose playbook IS fine
// needs to be told so plainly rather than shown an empty page.
func TestCompatReportRunFindsRetiredConstructs(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "old.yml")
	good := filepath.Join(dir, "new.yml")
	writeFile(t, bad, retiredPlaybook)
	writeFile(t, good, currentPlaybook)

	var out, errb bytes.Buffer
	if code := compatReportRun(&out, &errb, []string{bad}, ""); code != 4 {
		t.Errorf("a playbook with retired keywords exited %d, want 4", code)
	}
	got := out.String()
	for _, want := range []string{"sudo", "become", "always_run", "check_mode"} {
		if !strings.Contains(got, want) {
			t.Errorf("report does not mention %q:\n%s", want, got)
		}
	}

	out.Reset()
	errb.Reset()
	if code := compatReportRun(&out, &errb, []string{good}, ""); code != 0 {
		t.Errorf("a playbook with nothing retired exited %d, want 0:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "nothing here was retired") {
		t.Errorf("a clean playbook should say so, got:\n%s", out.String())
	}
}

// TestCompatReportRunDistinguishesCannotLookFromNothingWrong is the
// point of a separate exit status for an unreadable playbook. "I could
// not look" reading as "there is nothing wrong" is the mistake this
// whole flag exists to stop people making, and it would be an easy one
// to make here: a missing file could just as plausibly have returned 0
// with an empty report.
func TestCompatReportRunDistinguishesCannotLookFromNothingWrong(t *testing.T) {
	dir := t.TempDir()
	var out, errb bytes.Buffer
	code := compatReportRun(&out, &errb, []string{filepath.Join(dir, "absent.yml")}, "")
	if code == 0 || code == 4 {
		t.Fatalf("an unreadable playbook exited %d; 0 would read as clean and 4 as findings", code)
	}
	if errb.Len() == 0 {
		t.Error("nothing was written to stderr about a playbook that could not be read")
	}
}

// TestCompatReportFlagIsWiredUp goes through run(), because a flag
// declared and never read would let every test above pass while the
// binary ignored it.
//
// It uses a CLEAN playbook on purpose. A retired one exits 4 either
// way — with the flag from the report, without it from the parse error
// — so the exit status alone could not tell the flag from its absence.
// A clean playbook exits 0 either way too, but only the report path
// prints its verdict and runs nothing, so the output is what
// distinguishes them.
func TestCompatReportFlagIsWiredUp(t *testing.T) {
	dir := t.TempDir()
	pb := filepath.Join(dir, "new.yml")
	writeFile(t, pb, currentPlaybook)

	var code int
	out := captureStdout(t, func() { code = run([]string{"--compat-report", pb}) })
	if code != 0 {
		t.Fatalf("run --compat-report on a clean playbook exited %d, want 0:\n%s", code, out)
	}
	if !strings.Contains(out, "nothing here was retired") {
		t.Fatalf("--compat-report did not reach the report; output was:\n%s", out)
	}
	if strings.Contains(out, "PLAY ") || strings.Contains(out, "TASK ") {
		t.Errorf("--compat-report RAN the playbook instead of reporting on it:\n%s", out)
	}

	// The other direction, through run() as well: a retired playbook
	// must still come back 4.
	bad := filepath.Join(dir, "old.yml")
	writeFile(t, bad, retiredPlaybook)
	out = captureStdout(t, func() { code = run([]string{"--compat-report", bad}) })
	if code != 4 {
		t.Fatalf("run --compat-report on a retired playbook exited %d, want 4:\n%s", code, out)
	}
	if !strings.Contains(out, "become") {
		t.Errorf("the report did not name the replacement:\n%s", out)
	}
}

// TestCompatReportDoesNotLoadTheInventory pins where the flag sits in
// run(). It reaches no host and resolves no pattern, so loading an
// inventory is pure noise: before the branch was moved above
// invload.Load, every report printed two warnings about an implicit
// localhost that nothing was ever going to connect to, which reads like
// a problem with the playbook being reported on.
func TestCompatReportDoesNotLoadTheInventory(t *testing.T) {
	dir := t.TempDir()
	pb := filepath.Join(dir, "new.yml")
	writeFile(t, pb, currentPlaybook)

	var code int
	out, errOut := captureBoth(t, func() { code = run([]string{"--compat-report", pb}) })
	if code != 0 {
		t.Fatalf("exit = %d:\n%s%s", code, out, errOut)
	}
	// The control: the capture must be able to see a warning at all,
	// or "no warnings" would be the only thing it could ever report.
	var control string
	_, control = captureBoth(t, func() { run([]string{pb}) })
	if !strings.Contains(control, "WARNING") {
		t.Fatalf("control failed: a normal run printed no inventory warning, "+
			"so this test cannot tell a silenced one from an uncaptured one:\n%s", control)
	}
	if strings.Contains(errOut, "WARNING") {
		t.Errorf("--compat-report loaded the inventory:\n%s", errOut)
	}
}

// captureBoth runs fn with os.Stdout and os.Stderr each redirected to a
// pipe, and returns what it wrote to each.
func captureBoth(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()
	or, ow, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	er, ew, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	savedOut, savedErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = ow, ew

	outc := make(chan string, 1)
	errc := make(chan string, 1)
	go func() { b, _ := io.ReadAll(or); outc <- string(b) }()
	go func() { b, _ := io.ReadAll(er); errc <- string(b) }()

	fn()

	ow.Close()
	ew.Close()
	os.Stdout, os.Stderr = savedOut, savedErr
	return <-outc, <-errc
}
