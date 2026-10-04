package vaultpw

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolvePrefersTheFlagOverTheEnvironment pins real Ansible's own
// order: --vault-password-file, then $ANSIBLE_VAULT_PASSWORD_FILE, then
// [defaults] vault_password_file. Each case uses a DIFFERENT password so
// a precedence inversion shows up as the wrong value rather than as a
// pass -- two files holding the same password would make every order
// look correct.
func TestResolvePrefersTheFlagOverTheEnvironment(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	flagFile := write("flag", "FROM-FLAG\n")
	envFile := write("env", "FROM-ENV\n")

	t.Setenv(envPasswordFile, envFile)
	got, err := Resolve(flagFile)
	if err != nil {
		t.Fatal(err)
	}
	if got != "FROM-FLAG" {
		t.Errorf("the flag must win over %s: got %q", envPasswordFile, got)
	}

	// With no flag, the environment variable is used -- the whole point
	// of this change, since real reads a vaulted vars_files from it with
	// no flag at all.
	got, err = Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	if got != "FROM-ENV" {
		t.Errorf("%s was not honoured: got %q", envPasswordFile, got)
	}
}

// TestResolveReadsTheConfigFile covers the third source, which only
// applies when neither the flag nor the environment names one.
func TestResolveReadsTheConfigFile(t *testing.T) {
	dir := t.TempDir()
	pwFile := filepath.Join(dir, "cfgpw")
	if err := os.WriteFile(pwFile, []byte("FROM-CFG\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, "ansible.cfg")
	if err := os.WriteFile(cfg, []byte("[defaults]\nvault_password_file = "+pwFile+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANSIBLE_CONFIG", cfg)
	t.Setenv(envPasswordFile, "")

	if !PasswordFileConfigured() {
		t.Fatal("PasswordFileConfigured did not see the config file; ansible-playbook would never resolve a password")
	}
	got, err := Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	if got != "FROM-CFG" {
		t.Errorf("[defaults] vault_password_file was not honoured: got %q", got)
	}
}

// TestResolveRefusesThePasswordInTheEnvironment pins the ⛔ SECURITY
// decision: $ANSIBLE_VAULT_PASSWORD carried the password ITSELF, real
// Ansible has no such variable, and an environment variable is
// inherited by every child process -- which for this port means every
// local command a module runs.
//
// It must REFUSE rather than ignore. Ignoring it would send a caller
// who relied on it to the password prompt, which in a CI job with no
// terminal reads EOF and reports something unrelated.
func TestResolveRefusesThePasswordInTheEnvironment(t *testing.T) {
	t.Setenv(envPasswordFile, "")
	t.Setenv(envPasswordValue, "SOME-SECRET")

	_, err := Resolve("")
	if err == nil {
		t.Fatal("ANSIBLE_VAULT_PASSWORD was accepted; it must be refused")
	}
	for _, want := range []string{envPasswordValue, envPasswordFile} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error must name %q so the caller knows what to do instead: %v", want, err)
		}
	}
	// And it must not repeat the secret back.
	if strings.Contains(err.Error(), "SOME-SECRET") {
		t.Errorf("the error printed the password: %v", err)
	}
}

// A file's trailing newline is stripped; real Ansible does the same, and
// it is the difference between a working password file and a baffling
// "decryption failed".
func TestResolveStripsTheTrailingNewline(t *testing.T) {
	p := filepath.Join(t.TempDir(), "pw")
	if err := os.WriteFile(p, []byte("pw\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != "pw" {
		t.Errorf("got %q, want %q", got, "pw")
	}
}
