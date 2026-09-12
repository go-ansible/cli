package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-ansible/vault"
)

// fakeEditor writes a script that replaces the file it is given, and
// records what it found there. It stands in for $EDITOR, which is the
// only way to exercise create/edit without a person at a terminal.
func fakeEditor(t *testing.T, dir, writes string) (script, log string) {
	t.Helper()
	log = filepath.Join(dir, "editor.log")
	script = filepath.Join(dir, "editor.sh")
	body := "#!/bin/sh\ncat \"$1\" > " + log + "\nprintf '%s' '" + writes + "' > \"$1\"\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return script, log
}

func TestCreateAndEdit(t *testing.T) {
	dir := t.TempDir()
	pw := filepath.Join(dir, "pw.txt")
	writeFile(t, pw, "pw1\n")
	target := filepath.Join(dir, "s.yml")

	script, log := fakeEditor(t, dir, "secret: created\n")
	t.Setenv("EDITOR", script)

	// create: the editor opens an EMPTY file, and what it leaves is
	// encrypted to the target.
	if code := run([]string{"create", target, "--vault-password-file=" + pw, "--skip-tty-check"}); code != 0 {
		t.Fatalf("create exit = %d, want 0", code)
	}
	if seen, _ := os.ReadFile(log); len(seen) != 0 {
		t.Errorf("create handed the editor %q, want an empty file", seen)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !vault.IsVault(data) {
		t.Fatal("create did not produce an encrypted file")
	}
	plain, err := vault.Decrypt(string(data), "pw1")
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != "secret: created\n" {
		t.Errorf("content = %q", plain)
	}

	// create refuses an existing file rather than destroying it.
	before := string(data)
	if code := run([]string{"create", target, "--vault-password-file=" + pw, "--skip-tty-check"}); code == 0 {
		t.Error("create on an existing file: exit 0, want non-zero")
	}
	if now, _ := os.ReadFile(target); string(now) != before {
		t.Error("create overwrote an existing file")
	}

	// edit: the editor is handed the DECRYPTED content, and what it
	// leaves is re-encrypted.
	script2, log2 := fakeEditor(t, dir, "secret: edited\n")
	t.Setenv("EDITOR", script2)
	if code := run([]string{"edit", target, "--vault-password-file=" + pw}); code != 0 {
		t.Fatalf("edit exit = %d, want 0", code)
	}
	if seen, _ := os.ReadFile(log2); string(seen) != "secret: created\n" {
		t.Errorf("edit handed the editor %q, want the decrypted content", seen)
	}
	data, _ = os.ReadFile(target)
	plain, err = vault.Decrypt(string(data), "pw1")
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != "secret: edited\n" {
		t.Errorf("edited content = %q", plain)
	}
}

// TestEditUnchangedLeavesFileAlone covers the no-op real Ansible is
// careful about: re-encrypting identical content would rewrite the file
// with a fresh salt and show up as a spurious change in version control.
func TestEditUnchangedLeavesFileAlone(t *testing.T) {
	dir := t.TempDir()
	pw := filepath.Join(dir, "pw.txt")
	writeFile(t, pw, "pw1\n")
	target := filepath.Join(dir, "s.yml")
	enc, err := vault.Encrypt([]byte("a: 1\n"), "pw1", "")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, target, enc)

	noop := filepath.Join(dir, "noop.sh")
	if err := os.WriteFile(noop, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", noop)

	if code := run([]string{"edit", target, "--vault-password-file=" + pw}); code != 0 {
		t.Fatalf("edit exit = %d, want 0", code)
	}
	after, _ := os.ReadFile(target)
	if string(after) != enc {
		t.Error("an unchanged edit rewrote the file, changing its salt for nothing")
	}
}

// TestEditorFailureLeavesNoPlaintextBehind is the one that matters for
// safety: the temporary file holds the secret in the clear, so it must be
// gone even when the editor dies.
func TestEditorFailureLeavesNoPlaintextBehind(t *testing.T) {
	dir := t.TempDir()
	pw := filepath.Join(dir, "pw.txt")
	writeFile(t, pw, "pw1\n")
	target := filepath.Join(dir, "s.yml")
	enc, err := vault.Encrypt([]byte("top: secret\n"), "pw1", "")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, target, enc)

	boom := filepath.Join(dir, "boom.sh")
	if err := os.WriteFile(boom, []byte("#!/bin/sh\nexit 7\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", boom)

	leftovers := func() []string {
		m, _ := filepath.Glob(filepath.Join(os.TempDir(), "ansible-vault-*"))
		return m
	}
	before := len(leftovers())
	if code := run([]string{"edit", target, "--vault-password-file=" + pw}); code == 0 {
		t.Error("edit with a failing editor: exit 0, want non-zero")
	}
	if after := len(leftovers()); after != before {
		t.Errorf("a decrypted temporary file was left behind (%d -> %d)", before, after)
	}
	// And the encrypted file is untouched.
	if now, _ := os.ReadFile(target); string(now) != enc {
		t.Error("a failed edit modified the target file")
	}
}

func TestEditorCommand(t *testing.T) {
	t.Setenv("EDITOR", "")
	if got := editorCommand("/tmp/x"); len(got) != 2 || got[0] != "vi" {
		t.Errorf("default editor = %#v, want vi (real Ansible's default)", got)
	}
	t.Setenv("EDITOR", "code -w")
	got := editorCommand("/tmp/x")
	want := []string{"code", "-w", "/tmp/x"}
	if len(got) != len(want) {
		t.Fatalf("EDITOR with arguments = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("argv[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestEditRefusesPlaintext(t *testing.T) {
	dir := t.TempDir()
	pw := filepath.Join(dir, "pw.txt")
	writeFile(t, pw, "pw1\n")
	plain := filepath.Join(dir, "p.yml")
	writeFile(t, plain, "x: 1\n")
	if code := run([]string{"edit", plain, "--vault-password-file=" + pw}); code == 0 {
		t.Error("edit on a plaintext file: exit 0, want non-zero")
	}
	// The file is left exactly as it was, not encrypted behind the
	// caller's back.
	if now, _ := os.ReadFile(plain); string(now) != "x: 1\n" {
		t.Errorf("plaintext file was modified: %q", now)
	}
}
