package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-ansible/vault"
)

func TestParseVaultFlags(t *testing.T) {
	pwFile, vaultID, files, err := parseVaultFlags([]string{"a.yml", "--vault-password-file=pw.txt", "b.yml"})
	if err != nil {
		t.Fatal(err)
	}
	if pwFile != "pw.txt" || vaultID != "" || len(files) != 2 || files[0] != "a.yml" || files[1] != "b.yml" {
		t.Fatalf("pwFile=%q vaultID=%q files=%v", pwFile, vaultID, files)
	}
}

func TestParseVaultFlagsSpaceForm(t *testing.T) {
	pwFile, vaultID, files, err := parseVaultFlags([]string{"--vault-password-file", "pw.txt", "--vault-id", "prod", "a.yml"})
	if err != nil {
		t.Fatal(err)
	}
	if pwFile != "pw.txt" || vaultID != "prod" || len(files) != 1 {
		t.Fatalf("pwFile=%q vaultID=%q files=%v", pwFile, vaultID, files)
	}
}

func TestParseVaultFlagsMissingValue(t *testing.T) {
	if _, _, _, err := parseVaultFlags([]string{"--vault-password-file"}); err == nil {
		t.Fatal("want error")
	}
	if _, _, _, err := parseVaultFlags([]string{"--vault-id"}); err == nil {
		t.Fatal("want error")
	}
}

func TestResolvePasswordFromFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "pw.txt")
	if err := os.WriteFile(p, []byte("hunter2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pw, err := resolvePassword(p)
	if err != nil {
		t.Fatal(err)
	}
	if pw != "hunter2" {
		t.Fatalf("pw = %q", pw)
	}
}

func TestResolvePasswordFromEnv(t *testing.T) {
	t.Setenv("ANSIBLE_VAULT_PASSWORD", "envpw")
	pw, err := resolvePassword("")
	if err != nil {
		t.Fatal(err)
	}
	if pw != "envpw" {
		t.Fatalf("pw = %q", pw)
	}
}

func TestResolvePasswordFileMissing(t *testing.T) {
	if _, err := resolvePassword(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("want error")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "secret.yml")
	if err := os.WriteFile(f, []byte("key: value\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if code := encryptFiles([]string{f}, "pw", ""); code != 0 {
		t.Fatalf("encryptFiles exit = %d", code)
	}
	data, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if !vault.IsVault(data) {
		t.Fatal("file was not encrypted")
	}

	// Encrypting an already-encrypted file must fail cleanly.
	if code := encryptFiles([]string{f}, "pw", ""); code == 0 {
		t.Fatal("want non-zero exit re-encrypting an already-encrypted file")
	}

	if code := decryptFiles([]string{f}, "pw"); code != 0 {
		t.Fatalf("decryptFiles exit = %d", code)
	}
	data, err = os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "key: value\n" {
		t.Fatalf("content = %q", data)
	}
}

func TestEncryptFilesMissingSource(t *testing.T) {
	if code := encryptFiles([]string{filepath.Join(t.TempDir(), "absent")}, "pw", ""); code == 0 {
		t.Fatal("want non-zero exit for a missing source file")
	}
}

func TestDecryptFilesWrongPassword(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "s.yml")
	if err := os.WriteFile(f, []byte("x: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	encryptFiles([]string{f}, "right", "")
	if code := decryptFiles([]string{f}, "wrong"); code == 0 {
		t.Fatal("want non-zero exit for wrong password")
	}
}

func TestViewFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "s.yml")
	if err := os.WriteFile(f, []byte("x: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	encryptFiles([]string{f}, "pw", "")

	// viewFile writes to stdout; redirect it to capture output.
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	code := viewFile([]string{f}, "pw")
	w.Close()
	os.Stdout = old

	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	if code != 0 {
		t.Fatalf("viewFile exit = %d", code)
	}
	if !strings.Contains(string(buf[:n]), "x: 1") {
		t.Fatalf("output = %q", buf[:n])
	}
}

func TestViewFileWrongArgCount(t *testing.T) {
	if code := viewFile([]string{"a", "b"}, "pw"); code == 0 {
		t.Fatal("want error for view with more than one file")
	}
}

func TestRunEncryptDecryptEndToEnd(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "s.yml")
	if err := os.WriteFile(f, []byte("x: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pwFile := filepath.Join(dir, "pw.txt")
	if err := os.WriteFile(pwFile, []byte("pw"), 0o600); err != nil {
		t.Fatal(err)
	}

	if code := run([]string{"encrypt", f, "--vault-password-file=" + pwFile}); code != 0 {
		t.Fatalf("encrypt exit = %d", code)
	}
	if code := run([]string{"decrypt", f, "--vault-password-file=" + pwFile}); code != 0 {
		t.Fatalf("decrypt exit = %d", code)
	}
}

func TestRunUnknownSubcommand(t *testing.T) {
	if code := run([]string{"bogus"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestRunNoArgs(t *testing.T) {
	if code := run(nil); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestRunNoFiles(t *testing.T) {
	if code := run([]string{"encrypt"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestRunBadFlags(t *testing.T) {
	if code := run([]string{"encrypt", "--vault-password-file"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestRunVersion(t *testing.T) {
	if code := run([]string{"--version"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
}
