// Package vaultpw resolves the vault password the way real Ansible's
// CLI does, so every command that accepts --vault-password-file gets the
// same behavior from one place rather than each growing its own copy.
package vaultpw

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/go-ansible/playbook"
	"golang.org/x/term"
)

// envPasswordFile is real Ansible's own environment variable for this,
// declared in its config/base.yml as
// DEFAULT_VAULT_PASSWORD_FILE: {env: [ANSIBLE_VAULT_PASSWORD_FILE],
// ini: [{key: vault_password_file, section: defaults}], type: path}.
// It names a FILE; real has no variable carrying the password itself.
const envPasswordFile = "ANSIBLE_VAULT_PASSWORD_FILE"

// envPasswordValue is a variable this port used to honour and real does
// not. See Resolve: it is refused now rather than ignored.
const envPasswordValue = "ANSIBLE_VAULT_PASSWORD"

// Resolve returns the vault password, in real Ansible's own order of
// precedence: the file named by --vault-password-file, else the one
// named by $ANSIBLE_VAULT_PASSWORD_FILE, else the one named by
// [defaults] vault_password_file in ansible.cfg, else by prompting.
// Measured against ansible-core 2.21.4, which reads a vaulted
// vars_files successfully from the environment variable alone.
//
// The trailing newline of a password FILE is stripped — real Ansible
// does the same, and it is the difference between a working password
// file and a baffling "decryption failed".
//
// Prompting hides the input on a terminal and reads a plain line when
// stdin is a pipe, so a scripted caller still works. (Real does not
// prompt unless asked with --ask-vault-pass; it fails with "Attempting
// to decrypt but no vault secrets found". This port's extra prompt is a
// convenience, and the callers decide whether to reach it.)
//
// ⛔ SECURITY. This used to accept $ANSIBLE_VAULT_PASSWORD, carrying the
// password ITSELF. Real Ansible has no such variable — only the _FILE
// one, deliberately — and an environment variable is inherited by every
// child process, which for this port means every local command a module
// runs. A password file is not: it has an owner and a mode. The
// variable is therefore refused with an error naming its replacement,
// rather than silently ignored, so a caller that was relying on it
// learns why its playbook stopped decrypting instead of meeting a
// password prompt in a CI job with no terminal.
func Resolve(passwordFile string) (string, error) {
	if passwordFile == "" {
		passwordFile = passwordFileFromConfig()
	}
	if passwordFile != "" {
		data, err := os.ReadFile(passwordFile)
		if err != nil {
			return "", fmt.Errorf("reading vault password file: %w", err)
		}
		return strings.TrimRight(string(data), "\n"), nil
	}
	if os.Getenv(envPasswordValue) != "" {
		return "", fmt.Errorf("%s carries the password itself and is not honoured: "+
			"real Ansible has no such variable, and an environment variable is inherited by "+
			"every child process. Write the password to a file and name it with %s "+
			"(or --vault-password-file) instead",
			envPasswordValue, envPasswordFile)
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

// PasswordFileConfigured reports whether a vault password file is named
// anywhere other than the command line — the environment variable or
// ansible.cfg. ansible-playbook needs it because it only resolves a
// password when one was ASKED for, and "asked for" has to include these
// two: real reads a vaulted vars_files from the environment variable
// with no flag at all.
func PasswordFileConfigured() bool { return passwordFileFromConfig() != "" }

func passwordFileFromConfig() string {
	if v := os.Getenv(envPasswordFile); v != "" {
		return v
	}
	// [defaults] vault_password_file in the ansible.cfg real Ansible
	// would read, located by the same search order it uses.
	if v, ok := playbook.ConfigFileValueForEnv(envPasswordFile); ok {
		return v
	}
	return ""
}
