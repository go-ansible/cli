package main

import (
	"bytes"
	"github.com/go-ansible/vault"
	"io"
	"os"
	"path/filepath"
	"strings"
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

// TestRunVarsPromptFallsBackToDefaultWhenNotATerminal locks in
// terminalPrompt's real-Ansible-verified non-TTY behavior: a `go test`
// run's stdin is never a real terminal, so this proves the CLI falls
// back to vars_prompt's default rather than hanging waiting for input
// that will never come — matching a real ansible-playbook run's own
// piped-input behavior exactly (verified separately against the real
// binary: "Not prompting as we are not in interactive mode", falls
// back to default).
func TestRunVarsPromptFallsBackToDefaultWhenNotATerminal(t *testing.T) {
	dir := t.TempDir()
	inv := filepath.Join(dir, "inv.yml")
	pb := filepath.Join(dir, "site.yml")
	writeFile(t, inv, "all:\n  hosts:\n    localhost:\n      ansible_connection: local\n")
	writeFile(t, pb, "- hosts: all\n  gather_facts: false\n"+
		"  vars_prompt:\n    - name: username\n      default: anon\n"+
		"  tasks:\n    - debug:\n        msg: \"user={{ username }}\"\n")

	code := run([]string{"-i", inv, pb})
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
}

// TestRunFlagsAfterPlaybook covers the invocation form real
// ansible-playbook accepts and this command used to reject: flags placed
// after the playbook path. Real ansible-playbook is an argparse program,
// so `ansible-playbook site.yml -e k=v` is ordinary; Go's flag package
// stops at the first non-flag, and `-e` was taken for a second playbook.
func TestRunFlagsAfterPlaybook(t *testing.T) {
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
	if code := run([]string{"-i", inv, pb, "-e", "x=expected"}); code != 0 {
		t.Errorf("flags after the playbook: exit = %d, want 0", code)
	}
	// And the fully-trailing form, with the inventory after it too.
	if code := run([]string{pb, "-i", inv, "-e", "x=expected"}); code != 0 {
		t.Errorf("playbook first: exit = %d, want 0", code)
	}
}

// TestRunMultiplePlaybooksWithInterspersedFlags pins that resuming the
// parse after each positional still collects every playbook.
func TestRunMultiplePlaybooksWithInterspersedFlags(t *testing.T) {
	dir := t.TempDir()
	inv := filepath.Join(dir, "inv.yml")
	writeFile(t, inv, "all:\n  hosts:\n    localhost:\n      ansible_connection: local\n")
	marker := filepath.Join(dir, "second-ran.txt")
	first := filepath.Join(dir, "first.yml")
	second := filepath.Join(dir, "second.yml")
	writeFile(t, first, "- hosts: all\n  gather_facts: false\n  tasks: []\n")
	writeFile(t, second, `- hosts: all
  gather_facts: false
  tasks:
    - copy:
        content: ran
        dest: `+marker+"\n")

	if code := run([]string{"-i", inv, first, "-e", "x=1", second}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("second playbook did not run: %v", err)
	}
}

// TestRunVaultPasswordFile covers an encrypted vars_files target and an
// encrypted group_vars file in one run. ansible-playbook had no vault
// flag at all, so neither could be read.
func TestRunVaultPasswordFile(t *testing.T) {
	dir := t.TempDir()
	pw := filepath.Join(dir, "pw.txt")
	writeFile(t, pw, "correct horse\n")

	enc := func(rel, plaintext string) {
		t.Helper()
		text, err := vault.Encrypt([]byte(plaintext), "correct horse", "")
		if err != nil {
			t.Fatal(err)
		}
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, full, text)
	}

	writeFile(t, filepath.Join(dir, "inv.yml"),
		"all:\n  hosts:\n    localhost:\n      ansible_connection: local\n")
	enc("group_vars/all.yml", "gv: from_group_vars\n")
	enc("secrets.yml", "vf: from_vars_file\n")

	out := filepath.Join(dir, "out.txt")
	writeFile(t, filepath.Join(dir, "site.yml"), `- hosts: all
  gather_facts: false
  vars_files: [secrets.yml]
  tasks:
    - copy: {content: "{{ vf }}/{{ gv }}", dest: `+out+`}
`)

	// The password file is stripped of its trailing newline; without it
	// the run must fail with a vault error rather than a YAML crash.
	if code := run([]string{"-i", filepath.Join(dir, "inv.yml"), filepath.Join(dir, "site.yml")}); code == 0 {
		t.Error("encrypted files with no --vault-password-file: exit 0, want a failure")
	}

	code := run([]string{
		"-i", filepath.Join(dir, "inv.yml"),
		"--vault-password-file", pw,
		filepath.Join(dir, "site.yml"),
	})
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "from_vars_file/from_group_vars" {
		t.Errorf("rendered = %q, want %q", got, "from_vars_file/from_group_vars")
	}
}

// TestRunCheckMode covers --check end to end through the CLI: the run
// reports what would happen and writes nothing. Verified against real
// ansible-core 2.21.4, whose per-task outcomes for this playbook are
// identical.
func TestRunCheckMode(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	inv := filepath.Join(dir, "inv.yml")
	writeFile(t, inv, "all:\n  hosts:\n    localhost:\n      ansible_connection: local\n")
	pb := filepath.Join(dir, "site.yml")
	writeFile(t, pb, `- hosts: all
  gather_facts: false
  tasks:
    - copy: {content: "x\n", dest: `+filepath.Join(out, "c.txt")+`}
    - command: touch `+filepath.Join(out, "cmd.txt")+`
`)

	for _, flag := range []string{"--check", "-C"} {
		if code := run([]string{"-i", inv, flag, pb}); code != 0 {
			t.Fatalf("%s: exit = %d, want 0", flag, code)
		}
		entries, err := os.ReadDir(out)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Fatalf("%s wrote to the filesystem: %d entries", flag, len(entries))
		}
	}

	// And without it, the run really does the work — otherwise the check
	// above would pass for the wrong reason.
	if code := run([]string{"-i", inv, pb}); code != 0 {
		t.Fatalf("real run exit = %d, want 0", code)
	}
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("real run produced %d files, want 2", len(entries))
	}
}

// TestRunDiffMode checks both spellings of the flag, that the unified
// diff reaches stdout, and that it stays off by default. The expected
// text was measured from real ansible-core 2.21.4 running the same
// playbook with --diff --check.
func TestRunDiffMode(t *testing.T) {
	dir := t.TempDir()
	seed := filepath.Join(dir, "seed.txt")
	writeFile(t, seed, "alpha\nbeta\n")
	inv := filepath.Join(dir, "inv.yml")
	writeFile(t, inv, "all:\n  hosts:\n    localhost:\n      ansible_connection: local\n")
	pb := filepath.Join(dir, "site.yml")
	writeFile(t, pb, `- hosts: all
  gather_facts: false
  tasks:
    - name: edit
      lineinfile: {path: `+seed+`, line: gamma}
`)

	wantDiff := "--- before: " + seed + " (content)\n" +
		"+++ after: " + seed + " (content)\n" +
		"@@ -1,2 +1,3 @@\n alpha\n beta\n+gamma\n"

	for _, flag := range []string{"--diff", "-D"} {
		// --check keeps the fixture intact across both spellings.
		out := captureStdout(t, func() {
			if code := run([]string{"-i", inv, flag, "--check", "--no-color", pb}); code != 0 {
				t.Fatalf("%s: exit = %d, want 0", flag, code)
			}
		})
		if !strings.Contains(out, wantDiff) {
			t.Errorf("%s: missing the diff.\n--- got ---\n%s\n--- want to contain ---\n%s", flag, out, wantDiff)
		}
	}

	// Without the flag nothing extra is printed — otherwise the checks
	// above would pass for a reason that has nothing to do with --diff.
	out := captureStdout(t, func() {
		if code := run([]string{"-i", inv, "--check", "--no-color", pb}); code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
	})
	if strings.Contains(out, "@@") || strings.Contains(out, "--- before") {
		t.Errorf("a run without --diff printed one:\n%s", out)
	}
}

// captureStdout runs fn with os.Stdout redirected to a pipe and returns
// what it wrote. The callbacks hold os.Stdout only for the duration of a
// run, so swapping it around fn is enough.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()
	os.Stdout = saved
	w.Close()
	out := <-done
	r.Close()
	return out
}

// TestRunLimit covers both spellings of the flag, that it intersects
// with the play's own hosts rather than replacing them, and that a
// limit leaving nothing to target is a hard error — all measured
// against real ansible-core 2.21.4.
func TestRunLimit(t *testing.T) {
	dir := t.TempDir()
	inv := filepath.Join(dir, "inv.yml")
	writeFile(t, inv, `web:
  hosts:
    h1: {ansible_connection: local}
    h2: {ansible_connection: local}
    h3: {ansible_connection: local}
`)
	pb := filepath.Join(dir, "site.yml")
	writeFile(t, pb, `- hosts: web
  gather_facts: false
  tasks:
    - {name: m, debug: {msg: "{{ inventory_hostname }}"}}
`)

	ran := func(args ...string) (string, int) {
		var code int
		out := captureStdout(t, func() {
			code = run(append([]string{"-i", inv, "--no-color"}, append(args, pb)...))
		})
		var hosts []string
		for _, h := range []string{"h1", "h2", "h3"} {
			if strings.Contains(out, `"msg": "`+h+`"`) {
				hosts = append(hosts, h)
			}
		}
		return strings.Join(hosts, ","), code
	}

	for _, flag := range []string{"--limit", "-l"} {
		if got, code := ran(flag, "h1"); got != "h1" || code != 0 {
			t.Errorf("%s h1: ran on %q exit %d, want h1 exit 0", flag, got, code)
		}
	}
	if got, _ := ran("--limit", "h1,h2"); got != "h1,h2" {
		t.Errorf("--limit h1,h2 ran on %q", got)
	}
	// The exclusion form, which depends on real Ansible's own pattern
	// ordering rather than left-to-right evaluation.
	if got, _ := ran("--limit", "!h1"); got != "h2,h3" {
		t.Errorf(`--limit '!h1' ran on %q, want h2,h3`, got)
	}
	// No limit at all is not a filter.
	if got, _ := ran(); got != "h1,h2,h3" {
		t.Errorf("unlimited run went to %q", got)
	}
	// A limit matching nothing is an error, not a quiet empty run.
	if got, code := ran("--limit", "nosuch"); code == 0 {
		t.Errorf("--limit nosuch exited 0 (ran on %q); real ansible-playbook exits non-zero", got)
	}
	// But an unknown host alongside a known one is fine.
	if got, code := ran("--limit", "h1,nosuch"); got != "h1" || code != 0 {
		t.Errorf("--limit h1,nosuch ran on %q exit %d, want h1 exit 0", got, code)
	}
}
