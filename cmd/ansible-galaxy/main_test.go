package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParseInstallFlags(t *testing.T) {
	reqFile, rolesDir, err := parseInstallFlags([]string{"-r", "reqs.yml"})
	if err != nil {
		t.Fatal(err)
	}
	if reqFile != "reqs.yml" || rolesDir != "roles" {
		t.Fatalf("reqFile=%q rolesDir=%q", reqFile, rolesDir)
	}
}

func TestParseInstallFlagsCustomRolesPath(t *testing.T) {
	_, rolesDir, err := parseInstallFlags([]string{"--role-file", "reqs.yml", "-p", "custom/"})
	if err != nil {
		t.Fatal(err)
	}
	if rolesDir != "custom/" {
		t.Fatalf("rolesDir = %q", rolesDir)
	}
}

func TestParseInstallFlagsMissingValue(t *testing.T) {
	if _, _, err := parseInstallFlags([]string{"-r"}); err == nil {
		t.Fatal("want error")
	}
}

func TestParseInstallFlagsUnknown(t *testing.T) {
	if _, _, err := parseInstallFlags([]string{"--bogus"}); err == nil {
		t.Fatal("want error for an unrecognized flag")
	}
}

func TestRoleNameDefaultsToSrcBase(t *testing.T) {
	r := RoleRequirement{Src: "https://example.com/org/myrole.git"}
	if got := roleName(r); got != "myrole.git" {
		t.Fatalf("roleName = %q", got)
	}
}

func TestRoleNameExplicit(t *testing.T) {
	r := RoleRequirement{Name: "explicit", Src: "https://example.com/x"}
	if got := roleName(r); got != "explicit" {
		t.Fatalf("roleName = %q", got)
	}
}

func TestLoadRequirements(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "reqs.yml")
	if err := os.WriteFile(p, []byte("roles:\n  - name: r1\n    src: /x\n    version: main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reqs, err := loadRequirements(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs.Roles) != 1 || reqs.Roles[0].Name != "r1" || reqs.Roles[0].Version != "main" {
		t.Fatalf("reqs = %+v", reqs)
	}
}

func TestLoadRequirementsMissing(t *testing.T) {
	if _, err := loadRequirements(filepath.Join(t.TempDir(), "absent.yml")); err == nil {
		t.Fatal("want error")
	}
}

func TestLoadRequirementsBadYAML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "reqs.yml")
	if err := os.WriteFile(p, []byte("not: [valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadRequirements(p); err == nil {
		t.Fatal("want error")
	}
}

// initTestRepo creates a real local git repository with one commit and
// one tag, for use as a role source.
func initTestRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
	dir := t.TempDir()
	run := func(args ...string) {
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
	run("init", "--quiet", "--initial-branch=main")
	if err := os.WriteFile(filepath.Join(dir, "tasks.yml"), []byte("- debug: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "tasks.yml")
	run("commit", "--quiet", "-m", "initial")
	run("tag", "v1.0.0")
	return dir
}

func TestInstallRole(t *testing.T) {
	origin := initTestRepo(t)
	rolesDir := t.TempDir()

	if err := installRole(RoleRequirement{Name: "myrole", Src: origin}, rolesDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(rolesDir, "myrole", "tasks.yml")); err != nil {
		t.Fatal(err)
	}
}

func TestInstallRoleWithVersion(t *testing.T) {
	origin := initTestRepo(t)
	rolesDir := t.TempDir()

	if err := installRole(RoleRequirement{Name: "myrole", Src: origin, Version: "v1.0.0"}, rolesDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(rolesDir, "myrole", "tasks.yml")); err != nil {
		t.Fatal(err)
	}
}

func TestInstallRoleNoSrc(t *testing.T) {
	if err := installRole(RoleRequirement{Name: "x"}, t.TempDir()); err == nil {
		t.Fatal("want error for missing src")
	}
}

func TestInstallRoleAlreadyExists(t *testing.T) {
	origin := initTestRepo(t)
	rolesDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(rolesDir, "myrole"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := installRole(RoleRequirement{Name: "myrole", Src: origin}, rolesDir); err == nil {
		t.Fatal("want error when the destination already exists")
	}
}

func TestInstallRoleBadVersion(t *testing.T) {
	origin := initTestRepo(t)
	rolesDir := t.TempDir()
	err := installRole(RoleRequirement{Name: "myrole", Src: origin, Version: "no-such-ref"}, rolesDir)
	if err == nil {
		t.Fatal("want error for an unresolvable version")
	}
}

func TestInstallRoleBadSrc(t *testing.T) {
	rolesDir := t.TempDir()
	err := installRole(RoleRequirement{Name: "x", Src: filepath.Join(t.TempDir(), "does-not-exist")}, rolesDir)
	if err == nil {
		t.Fatal("want error for an unreachable src")
	}
}

func TestRunVersion(t *testing.T) {
	if code := run([]string{"--version"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
}
