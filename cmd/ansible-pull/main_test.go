package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initPullTestRepo creates a real local git repository containing a
// local.yml playbook that writes a marker file, so a real run can be
// verified end to end without a network round trip.
func initPullTestRepo(t *testing.T, markerPath string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
	dir := t.TempDir()
	gitCmd := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	gitCmd("init", "--quiet", "--initial-branch=main")
	playbook := `- hosts: all
  gather_facts: false
  tasks:
    - name: write marker
      copy:
        content: "pulled"
        dest: ` + markerPath + `
`
	if err := os.WriteFile(filepath.Join(dir, "local.yml"), []byte(playbook), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd("add", "local.yml")
	gitCmd("commit", "--quiet", "-m", "initial")
	gitCmd("tag", "v1.0.0")
	return dir
}

func TestParsePullFlagsRequiresURL(t *testing.T) {
	opts, _, err := parsePullFlags([]string{})
	if err != nil {
		t.Fatal(err)
	}
	if opts.url != "" {
		t.Fatalf("url = %q, want empty", opts.url)
	}
}

func TestParsePullFlagsAll(t *testing.T) {
	opts, pb, err := parsePullFlags([]string{
		"-U", "https://example.com/repo.git",
		"-C", "v1.0.0",
		"-d", "/tmp/dest",
		"-i", "inv.yml",
		"-e", "a=1",
		"--only-if-changed",
		"--purge",
		"--no-color",
		"site.yml",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.url != "https://example.com/repo.git" || opts.checkout != "v1.0.0" || opts.directory != "/tmp/dest" ||
		opts.inventoryPath != "inv.yml" || len(opts.extraVars) != 1 || !opts.onlyIfChanged || !opts.purge || !opts.noColor {
		t.Fatalf("opts = %+v", opts)
	}
	if pb != "site.yml" {
		t.Fatalf("playbook = %q", pb)
	}
}

func TestParsePullFlagsDefaultPlaybookName(t *testing.T) {
	_, pb, err := parsePullFlags([]string{"-U", "x"})
	if err != nil {
		t.Fatal(err)
	}
	if pb != "local.yml" {
		t.Fatalf("playbook = %q, want local.yml", pb)
	}
}

func TestParsePullFlagsMissingValue(t *testing.T) {
	if _, _, err := parsePullFlags([]string{"-U"}); err == nil {
		t.Fatal("want error for -U with no value")
	}
}

func TestParsePullFlagsUnknownFlag(t *testing.T) {
	if _, _, err := parsePullFlags([]string{"--bogus"}); err == nil {
		t.Fatal("want error for an unrecognized flag")
	}
}

func TestParsePullFlagsTwoPositionals(t *testing.T) {
	if _, _, err := parsePullFlags([]string{"a.yml", "b.yml"}); err == nil {
		t.Fatal("want error for two positional playbook names")
	}
}

func TestDefaultCheckoutDirStable(t *testing.T) {
	a := defaultCheckoutDir("https://example.com/repo.git")
	b := defaultCheckoutDir("https://example.com/repo.git")
	if a != b {
		t.Fatalf("defaultCheckoutDir not stable: %q != %q", a, b)
	}
	c := defaultCheckoutDir("https://example.com/other.git")
	if a == c {
		t.Fatal("defaultCheckoutDir should differ for a different URL")
	}
}

func TestLoadOrDefaultInventoryDefaultsToLocalhost(t *testing.T) {
	inv, err := loadOrDefaultInventory("")
	if err != nil {
		t.Fatal(err)
	}
	hosts, err := inv.Match("all")
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0].Name != "localhost" {
		t.Fatalf("hosts = %v", hosts)
	}
	if inv.HostVars("localhost")["ansible_connection"] != "local" {
		t.Fatal("default inventory host should force a local connection")
	}
}

func TestLoadOrDefaultInventoryLoadsGivenPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inv.yml")
	if err := os.WriteFile(path, []byte("all:\n  hosts:\n    custom: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inv, err := loadOrDefaultInventory(path)
	if err != nil {
		t.Fatal(err)
	}
	hosts, err := inv.Match("all")
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0].Name != "custom" {
		t.Fatalf("hosts = %v", hosts)
	}
}

func TestRunEndToEndClonesAndRuns(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker.txt")
	origin := initPullTestRepo(t, marker)
	dest := filepath.Join(dir, "checkout")

	code := run([]string{"-U", origin, "-d", dest, "--no-color"})
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "pulled" {
		t.Fatalf("marker content = %q", data)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("checkout dir should still exist without --purge: %v", err)
	}
}

func TestRunSecondCallPullsExistingCheckout(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker.txt")
	origin := initPullTestRepo(t, marker)
	dest := filepath.Join(dir, "checkout")

	if code := run([]string{"-U", origin, "-d", dest, "--no-color"}); code != 0 {
		t.Fatalf("first run exit = %d", code)
	}
	if code := run([]string{"-U", origin, "-d", dest, "--no-color"}); code != 0 {
		t.Fatalf("second run exit = %d", code)
	}
}

func TestRunOnlyIfChangedSkipsWhenUnchanged(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker.txt")
	origin := initPullTestRepo(t, marker)
	dest := filepath.Join(dir, "checkout")

	if code := run([]string{"-U", origin, "-d", dest, "--no-color"}); code != 0 {
		t.Fatalf("first run exit = %d", code)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"-U", origin, "-d", dest, "--no-color", "--only-if-changed"}); code != 0 {
		t.Fatalf("second (unchanged) run exit = %d", code)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("--only-if-changed should have skipped re-running the playbook")
	}
}

func TestRunCheckoutSpecificTag(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker.txt")
	origin := initPullTestRepo(t, marker)
	dest := filepath.Join(dir, "checkout")

	code := run([]string{"-U", origin, "-C", "v1.0.0", "-d", dest, "--no-color"})
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("expected the playbook at the checked-out tag to have run")
	}
}

func TestRunCheckoutBadRefErrors(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker.txt")
	origin := initPullTestRepo(t, marker)
	dest := filepath.Join(dir, "checkout")

	code := run([]string{"-U", origin, "-C", "no-such-ref", "-d", dest, "--no-color"})
	if code != 1 {
		t.Fatalf("exit = %d, want 1 for an unresolvable checkout ref", code)
	}
}

func TestRunPurgeRemovesCheckout(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker.txt")
	origin := initPullTestRepo(t, marker)
	dest := filepath.Join(dir, "checkout")

	code := run([]string{"-U", origin, "-d", dest, "--no-color", "--purge"})
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("--purge should have removed the checkout directory")
	}
}

func TestRunMissingURL(t *testing.T) {
	if code := run([]string{}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestRunBadFlags(t *testing.T) {
	if code := run([]string{"--bogus"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestRunCloneFailureErrors(t *testing.T) {
	dir := t.TempDir()
	code := run([]string{"-U", filepath.Join(dir, "does-not-exist"), "-d", filepath.Join(dir, "dest")})
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
}

func TestRunMissingPlaybookInRepoErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
	origin := t.TempDir()
	gitCmd := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = origin
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	gitCmd("init", "--quiet", "--initial-branch=main")
	if err := os.WriteFile(filepath.Join(origin, "readme.md"), []byte("no playbook here"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd("add", "readme.md")
	gitCmd("commit", "--quiet", "-m", "no playbook")

	code := run([]string{"-U", origin, "-d", filepath.Join(dir, "dest"), "--no-color"})
	if code != 1 {
		t.Fatalf("exit = %d, want 1 for a missing local.yml in the repo", code)
	}
}

// A relative --inventory names a file IN THE CHECKOUT: real runs the
// playbook with the checkout as its working directory, so `-i hosts`
// finds the inventory the repository carries. Resolving it against
// the process's own cwd made the ordinary ansible-pull idiom fail
// outright with "stat hosts: no such file or directory".
func TestRelativeInventoryResolvesAgainstTheCheckout(t *testing.T) {
	if got, want := resolveAgainst("/checkout", "hosts"), filepath.Join("/checkout", "hosts"); got != want {
		t.Errorf("resolveAgainst = %q, want %q", got, want)
	}
	// An absolute path is left alone -- real accepts an inventory
	// outside the checkout.
	if got := resolveAgainst("/checkout", "/elsewhere/inv"); got != "/elsewhere/inv" {
		t.Errorf("absolute path was rewritten to %q", got)
	}
	// No inventory at all stays empty, so the caller still falls back
	// to its implicit localhost.
	if got := resolveAgainst("/checkout", ""); got != "" {
		t.Errorf("empty path became %q", got)
	}
}

// ansible-pull applies the repository to THIS machine and no other,
// however wide the playbook's own hosts: is. Measured with a
// three-host inventory and a play targeting all: real ran on
// localhost and on this machine's name, and NOT on the third host.
func TestLocalTargetsNamesThisMachineAndLocalhost(t *testing.T) {
	got := localTargets()
	if !strings.HasSuffix(got, ",localhost") && got != "localhost" {
		t.Fatalf("localTargets = %q, want it to include localhost", got)
	}
	if h, err := os.Hostname(); err == nil && h != "" {
		if !strings.HasPrefix(got, h+",") {
			t.Errorf("localTargets = %q, want it to start with this host's name %q", got, h)
		}
	}
}

// The clone result is the first thing a pull prints, in the shape
// real's own git-module task reports it. Captured from
// ansible-core 2.21.4: a first clone has a null "before" and no
// remote_url_changed; a later run has both revisions and reports
// remote_url_changed false.
func TestReportSyncShapes(t *testing.T) {
	var out strings.Builder
	reportSync(&out, syncResult{after: "abc123", changed: true, cloned: true})
	want := "localhost | CHANGED => {\n    \"after\": \"abc123\",\n    \"before\": null,\n    \"changed\": true\n}\n"
	if out.String() != want {
		t.Errorf("first clone:\n%q\nwant\n%q", out.String(), want)
	}

	out.Reset()
	reportSync(&out, syncResult{before: "abc123", after: "abc123", changed: false})
	want = "localhost | SUCCESS => {\n    \"after\": \"abc123\",\n    \"before\": \"abc123\",\n    \"changed\": false,\n    \"remote_url_changed\": false\n}\n"
	if out.String() != want {
		t.Errorf("unchanged run:\n%q\nwant\n%q", out.String(), want)
	}

	// A pull that moved the revision is CHANGED but still carries the
	// previous one.
	out.Reset()
	reportSync(&out, syncResult{before: "old", after: "new", changed: true})
	got := out.String()
	if !strings.HasPrefix(got, "localhost | CHANGED => {") || !strings.Contains(got, "\"before\": \"old\"") {
		t.Errorf("moved revision:\n%q", got)
	}
}

// End to end, through run() rather than through the helpers: a
// repository that carries its own inventory is the ordinary
// ansible-pull idiom, and `-i hosts` must find the one in the
// CHECKOUT. Testing resolveAgainst alone does not cover this -- the
// helper was correct while the caller still passed the raw path, and
// only a test taking the path the binary takes can tell.
func TestRunFindsAnInventoryCarriedByTheRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
	dir := t.TempDir()
	origin := t.TempDir()
	gitCmd := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = origin
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	gitCmd("init", "--quiet", "--initial-branch=main")
	marker := filepath.Join(dir, "marker.txt")
	if err := os.WriteFile(filepath.Join(origin, "local.yml"), []byte(
		"- hosts: all\n  gather_facts: false\n  tasks:\n    - copy: {content: \"pulled\", dest: "+marker+"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The inventory lives in the REPOSITORY, not where the test runs.
	if err := os.WriteFile(filepath.Join(origin, "hosts"), []byte("localhost ansible_connection=local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd("add", "local.yml", "hosts")
	gitCmd("commit", "--quiet", "-m", "initial")

	dest := filepath.Join(dir, "checkout")
	if code := run([]string{"-U", origin, "-d", dest, "--no-color", "-i", "hosts"}); code != 0 {
		t.Fatalf("exit = %d, want 0 -- `-i hosts` must resolve inside the checkout", code)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "pulled" {
		t.Fatalf("marker = %q, err = %v", data, err)
	}
}

// And the limit is applied through run() too: a repository whose
// inventory names OTHER machines must not have them configured by a
// pull on this one.
func TestRunDoesNotTouchOtherHostsInTheInventory(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
	dir := t.TempDir()
	origin := t.TempDir()
	gitCmd := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = origin
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	gitCmd("init", "--quiet", "--initial-branch=main")
	// otherbox would be reached by `hosts: all` without the limit, and
	// writes a file naming itself if it ever runs.
	stamp := filepath.Join(dir, "{{ inventory_hostname }}.txt")
	if err := os.WriteFile(filepath.Join(origin, "local.yml"), []byte(
		"- hosts: all\n  gather_facts: false\n  tasks:\n    - copy: {content: \"ran\", dest: \""+stamp+"\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(origin, "hosts"), []byte(
		"localhost ansible_connection=local\notherbox ansible_connection=local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd("add", "local.yml", "hosts")
	gitCmd("commit", "--quiet", "-m", "initial")

	if code := run([]string{"-U", origin, "-d", filepath.Join(dir, "checkout"), "--no-color", "-i", "hosts"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "localhost.txt")); err != nil {
		t.Errorf("localhost was not configured: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "otherbox.txt")); err == nil {
		t.Error("otherbox was configured: a pull must apply to THIS machine only")
	}
}
