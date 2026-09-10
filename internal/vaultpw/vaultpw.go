// Package vaultpw resolves the vault password the way real Ansible's
// CLI does, so every command that accepts --vault-password-file gets the
// same behavior from one place rather than each growing its own copy.
package vaultpw

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// Resolve returns the vault password: from the file named by
// --vault-password-file if given, else from ANSIBLE_VAULT_PASSWORD, else
// by prompting. The trailing newline of a password FILE is stripped —
// real Ansible does the same, and it is the difference between a working
// password file and a baffling "decryption failed".
//
// Prompting hides the input on a terminal and reads a plain line when
// stdin is a pipe, so a scripted caller still works.
func Resolve(passwordFile string) (string, error) {
	if passwordFile != "" {
		data, err := os.ReadFile(passwordFile)
		if err != nil {
			return "", fmt.Errorf("reading vault password file: %w", err)
		}
		return strings.TrimRight(string(data), "\n"), nil
	}
	if env := os.Getenv("ANSIBLE_VAULT_PASSWORD"); env != "" {
		return env, nil
	}
	fmt.Fprint(os.Stderr, "Vault password: ")
	if term.IsTerminal(int(os.Stdin.Fd())) {
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", fmt.Errorf("reading password: %w", err)
		}
		return string(pw), nil
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading password: %w", err)
	}
	return strings.TrimRight(line, "\n"), nil
}
