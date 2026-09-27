package main

import (
	"bytes"
	"io"
	"os"
	"regexp"
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

	// Drained WHILE f runs. A pipe holds about 64 KB, and reading only
	// after f returned deadlocked the moment the output exceeded that:
	// `ansible-doc -l` already writes ~44 KB, so this was one growth
	// spurt away from hanging the suite instead of failing it. It was
	// found by a neuter that removed the line truncation -- the test did
	// not report a wrong width, it stopped for ten minutes.
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	f()
	w.Close()
	os.Stdout = old
	return <-done
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
	// Real's banner names the module by its FQCN.
	if !strings.Contains(out, "> MODULE ansible.builtin.debug") {
		t.Fatalf("output = %q, want the module banner", out)
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
	if !strings.Contains(out, "> MODULE ansible.builtin.debug") ||
		!strings.Contains(out, "> MODULE ansible.builtin.ping") {
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

// firstLine has become listSynopsis, which does what firstLine did and
// then removes the Go identifier and the boilerplate after it.
func TestListSynopsisFlattensTheFirstParagraph(t *testing.T) {
	if got := listSynopsis("\n\n  hello world  \nmore text\n"); got != "hello world more text" {
		t.Fatalf("listSynopsis = %q", got)
	}
	if got := listSynopsis(""); got != "" {
		t.Fatalf("listSynopsis(\"\") = %q, want empty", got)
	}
}

// Real's layout, measured from ansible-doc -l: the name field is 59
// characters wide and the whole line is 78, the name is fully qualified,
// and a summary too long for the remaining 19 is cut with an ellipsis.
func TestListMatchesRealsColumnLayout(t *testing.T) {
	out := captureStdout(t, func() { run([]string{"-l"}) })
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 100 {
		t.Fatalf("only %d modules listed", len(lines))
	}
	for _, ln := range lines {
		if len(ln) != 78 {
			t.Fatalf("line is %d characters, want 78: %q", len(ln), ln)
		}
		if !strings.HasPrefix(ln, "ansible.builtin.") {
			t.Fatalf("line does not start with the FQCN: %q", ln)
		}
		// The summary begins at 59 and the name field is blank-padded.
		if ln[58] != ' ' {
			t.Fatalf("name field is not padded to 59: %q", ln)
		}
	}
}

// Go's doc convention makes every comment open with the identifier it
// documents, and printing it verbatim put a Go function name in front of
// all 566 entries -- a person reading the documentation read this port's
// internals.
func TestNoGoIdentifierReachesTheReader(t *testing.T) {
	out := captureStdout(t, func() { run([]string{"-l"}) })
	if strings.Contains(out, "module") && regexp.MustCompile(`\bmodule[A-Z]`).MatchString(out) {
		t.Errorf("a Go identifier reached the listing:\n%s", firstMatchingLine(out, `module[A-Z]`))
	}
	page := captureStdout(t, func() { printModule("ping") })
	if regexp.MustCompile(`\bmodule[A-Z]`).MatchString(page) {
		t.Errorf("a Go identifier reached the module page:\n%s", page)
	}
	// And the header is real's: "> MODULE <fqcn>".
	if !strings.HasPrefix(page, "> MODULE ansible.builtin.ping\n") {
		t.Errorf("header = %q", strings.SplitN(page, "\n", 2)[0])
	}
}

func TestModulePageWrapsLikeReal(t *testing.T) {
	// Measured across six modules: the description block is indented two
	// spaces and wraps at 70 including the indent.
	page := captureStdout(t, func() { printModule("copy") })
	for _, ln := range strings.Split(page, "\n")[2:] {
		if ln == "" {
			continue // a paragraph break, which must NOT be indented
		}
		if !strings.HasPrefix(ln, "  ") {
			t.Errorf("body line is not indented: %q", ln)
		}
		if len(ln) > 70 {
			t.Errorf("body line is %d characters, want at most 70: %q", len(ln), ln)
		}
		if strings.TrimRight(ln, " ") != ln {
			t.Errorf("body line has trailing whitespace: %q", ln)
		}
	}
}

func TestListSynopsisStripsTheBoilerplateButNotTheDescription(t *testing.T) {
	for _, tc := range []struct{ name, doc, want string }{
		{"colon form", "moduleAcl implements (a subset of) Ansible's `acl` module: sets an ACL.", "sets an ACL."},
		{"qualifier form", "moduleApt implements (a subset of) Ansible's `apt` module for Debian.", "for Debian."},
		{"collection between", "moduleX implements Ansible's `x`\n(community.general) module: does a thing.", "does a thing."},
		{"long hedge", "moduleExpect implements (a best-effort approximation of) Ansible's `expect` module: runs a command.", "runs a command."},
		// Nothing beyond the boilerplate: keep it rather than return "."
		{"nothing more", "modulePip implements (a subset of) Ansible's `pip` module.",
			"(a subset of) Ansible's `pip` module."},
		// The paragraph break is never crossed: reaching past it returned
		// apt's ARGUMENT LIST as its description.
		{"args in the next paragraph",
			"moduleApt implements (a subset of) Ansible's `apt` module for Debian.\n\nArgs: name (string, required); state (present|absent).",
			"for Debian."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := listSynopsis(tc.doc); got != tc.want {
				t.Errorf("listSynopsis =\n %q\nwant %q", got, tc.want)
			}
		})
	}
}

func firstMatchingLine(s, pat string) string {
	re := regexp.MustCompile(pat)
	for _, ln := range strings.Split(s, "\n") {
		if re.MatchString(ln) {
			return ln
		}
	}
	return ""
}
