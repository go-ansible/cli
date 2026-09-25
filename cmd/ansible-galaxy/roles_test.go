package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeRole(t *testing.T, dir, name string, installInfo string) {
	t.Helper()
	meta := filepath.Join(dir, name, "meta")
	if err := os.MkdirAll(meta, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(meta, "main.yml"), []byte("galaxy_info:\n  author: someone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if installInfo != "" {
		if err := os.WriteFile(filepath.Join(meta, ".galaxy_install_info"), []byte(installInfo), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// Measured against ansible-core 2.21.4: a directory is a role only if
// it holds meta/main.yml. A plain directory is not listed, and neither
// is one carrying only a meta/.galaxy_install_info.
func TestRolesInNeedsMetaMainYML(t *testing.T) {
	dir := t.TempDir()
	makeRole(t, dir, "real_role", "")
	if err := os.MkdirAll(filepath.Join(dir, "plaindir"), 0o755); err != nil {
		t.Fatal(err)
	}
	onlyInfo := filepath.Join(dir, "onlyinfo", "meta")
	if err := os.MkdirAll(onlyInfo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(onlyInfo, ".galaxy_install_info"), []byte("version: 1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notadir"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got := rolesIn(dir)
	if len(got) != 1 || got[0].name != "real_role" {
		names := make([]string, len(got))
		for i, r := range got {
			names[i] = r.name
		}
		t.Fatalf("rolesIn = %v, want just real_role", names)
	}
}

// The version comes from meta/.galaxy_install_info, which the
// installer writes -- NOT from galaxy_info.version in meta/main.yml.
// Measured: a role declaring galaxy_info.version 9.9.9 still lists as
// "(unknown version)".
func TestRoleVersionComesFromInstallInfo(t *testing.T) {
	dir := t.TempDir()
	makeRole(t, dir, "versioned", "install_date: Thu Sep 25 2026\nversion: 1.2.3\n")
	makeRole(t, dir, "unversioned", "")
	// A version declared the other way, which real ignores.
	if err := os.WriteFile(filepath.Join(dir, "unversioned", "meta", "main.yml"),
		[]byte("galaxy_info:\n  author: x\n  version: 9.9.9\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	byName := map[string]string{}
	for _, r := range rolesIn(dir) {
		byName[r.name] = r.String()
	}
	if !strings.HasSuffix(byName["versioned"], ", 1.2.3") {
		t.Errorf("versioned: %q", byName["versioned"])
	}
	if !strings.HasSuffix(byName["unversioned"], ", (unknown version)") {
		t.Errorf("unversioned: %q -- galaxy_info.version must not be used", byName["unversioned"])
	}
}

// The defaults are searched even when --roles-path is given, which is
// why `list -p somewhere` still warns about the three that do not
// exist.
func TestRolesSearchPathKeepsTheDefaults(t *testing.T) {
	got := rolesSearchPath("/somewhere")
	if len(got) != len(defaultRolesPaths)+1 || got[0] != "/somewhere" {
		t.Fatalf("got %v", got)
	}
	if len(rolesSearchPath("")) != len(defaultRolesPaths) {
		t.Fatalf("with no -p, got %v", rolesSearchPath(""))
	}
}

func TestParseListFlags(t *testing.T) {
	name, path, err := parseListFlags([]string{"-p", "roles", "myrole"})
	if err != nil || name != "myrole" || path != "roles" {
		t.Fatalf("name=%q path=%q err=%v", name, path, err)
	}
	if _, _, err := parseListFlags([]string{"--bogus"}); err == nil {
		t.Error("an unknown flag must be refused")
	}
	if _, _, err := parseListFlags([]string{"-p"}); err == nil {
		t.Error("-p without a value must be refused")
	}
}

// remove deletes the role and says so; a role that is not there is not
// an error. Real exits 0 either way.
func TestRunRemove(t *testing.T) {
	dir := t.TempDir()
	makeRole(t, dir, "doomed", "")
	if code := runRemove([]string{"-p", dir, "doomed"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "doomed")); !os.IsNotExist(err) {
		t.Error("the role is still on disk")
	}
	if code := runRemove([]string{"-p", dir, "doomed"}); code != 0 {
		t.Fatalf("removing an absent role: exit = %d, want 0", code)
	}
	if code := runRemove([]string{"-p", dir}); code != 2 {
		t.Fatalf("remove with no role named: exit = %d, want 2", code)
	}
}

// Whatever it finds, list exits 0: real reports absence through
// warnings, not through the exit status.
func TestRunListAlwaysExitsZero(t *testing.T) {
	dir := t.TempDir()
	makeRole(t, dir, "r", "")
	for _, args := range [][]string{
		{"-p", dir},
		{"-p", dir, "r"},
		{"-p", dir, "nosuch"},
		{"-p", filepath.Join(dir, "nonexistent")},
	} {
		if code := runList(args); code != 0 {
			t.Errorf("list %v: exit = %d, want 0", args, code)
		}
	}
}
