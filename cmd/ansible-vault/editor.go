package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-ansible/vault"
	"golang.org/x/term"
)

// editorCommand returns the editor invocation for path, from $EDITOR with
// real Ansible's own default of vi. The value is split on spaces the way
// real Ansible shlex-splits it, so EDITOR="code -w" works.
func editorCommand(path string) []string {
	editor := os.Getenv("EDITOR")
	if strings.TrimSpace(editor) == "" {
		editor = "vi" // real Ansible's documented default
	}
	return append(strings.Fields(editor), path)
}

// editInTempFile is the shape both create and edit share: put plaintext in
// a temporary file, hand it to the editor, and encrypt whatever comes back
// to path.
//
// The temporary file holds the secret in the clear, so it is created 0600
// by os.CreateTemp, lives in the system temp directory rather than beside
// the target (which may be inside a repository), and is removed on every
// path out — including when the editor fails.
//
// Nothing is written when the content comes back unchanged, matching real
// Ansible: re-encrypting would rewrite the file with a fresh salt and show
// up as a spurious change in version control.
func editInTempFile(path string, existing []byte, password, vaultID string) error {
	tmp, err := os.CreateTemp("", "ansible-vault-*"+filepath.Ext(path))
	if err != nil {
		return fmt.Errorf("creating a temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if len(existing) > 0 {
		if _, err := tmp.Write(existing); err != nil {
			tmp.Close()
			return fmt.Errorf("writing the temporary file: %w", err)
		}
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing the temporary file: %w", err)
	}

	argv := editorCommand(tmpPath)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("unable to execute %q: %w", strings.Join(argv, " "), err)
	}

	edited, err := os.ReadFile(tmpPath)
	if err != nil {
		return fmt.Errorf("reading back the temporary file: %w", err)
	}
	if existing != nil && string(edited) == string(existing) {
		return nil // unchanged: leave the file alone
	}

	text, err := vault.Encrypt(edited, password, vaultID)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text), 0o600)
}

// createFile implements `ansible-vault create`: open an editor on an empty
// file and encrypt what comes back. An existing file is an error with real
// Ansible's own wording — create is not edit, and silently overwriting an
// encrypted file would destroy whatever was in it.
func createFile(path, password, vaultID string) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "[WARNING]: %s does not exist, creating...\n", dir)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
	}
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return fmt.Errorf("%s exists, please use 'edit' instead", path)
	}
	return editInTempFile(path, nil, password, vaultID)
}

// editFile implements `ansible-vault edit`: decrypt, edit, re-encrypt. The
// file's own vault id is preserved, so editing does not silently move a
// file between vault identities.
func editFile(path, password string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !vault.IsVault(data) {
		return fmt.Errorf("%s is not encrypted", path)
	}
	plain, err := vault.Decrypt(string(data), password)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	// Keep the file's own vault id, so editing never silently moves a
	// file between vault identities. A file with none reports "", which
	// is exactly what Encrypt wants for an unlabelled vault.
	vaultID, err := vault.VaultID(string(data))
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return editInTempFile(path, plain, password, vaultID)
}

// stdoutIsTTY reports whether an editor can meaningfully be opened. Real
// Ansible gates create on this and refuses with the message reproduced at
// the call site.
func stdoutIsTTY() bool { return term.IsTerminal(int(os.Stdout.Fd())) }
