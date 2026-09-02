// Command ansible-vault encrypts/decrypts/views files in Ansible's
// Vault 1.1 format.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/go-ansible/vault"
	"golang.org/x/term"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) < 1 {
		usage()
		return 2
	}
	sub, rest := args[0], args[1:]

	pwFile, vaultID, files, err := parseVaultFlags(rest)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ansible-vault:", err)
		return 2
	}
	if len(files) == 0 {
		usage()
		return 2
	}

	password, err := resolvePassword(pwFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ansible-vault:", err)
		return 1
	}

	switch sub {
	case "encrypt":
		return encryptFiles(files, password, vaultID)
	case "decrypt":
		return decryptFiles(files, password)
	case "view":
		return viewFile(files, password)
	default:
		usage()
		return 2
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  ansible-vault encrypt FILE [FILE2 ...] --vault-password-file=PATH [--vault-id=NAME]
  ansible-vault decrypt FILE [FILE2 ...] --vault-password-file=PATH
  ansible-vault view FILE --vault-password-file=PATH`)
}

// parseVaultFlags does minimal hand-rolled flag parsing (--flag=value
// or --flag value) so file arguments can be interleaved naturally,
// matching how ansible-vault itself is invoked.
func parseVaultFlags(args []string) (pwFile, vaultID string, files []string, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case strings.HasPrefix(a, "--vault-password-file="):
			pwFile = strings.TrimPrefix(a, "--vault-password-file=")
		case a == "--vault-password-file":
			i++
			if i >= len(args) {
				return "", "", nil, fmt.Errorf("--vault-password-file requires a value")
			}
			pwFile = args[i]
		case strings.HasPrefix(a, "--vault-id="):
			vaultID = strings.TrimPrefix(a, "--vault-id=")
		case a == "--vault-id":
			i++
			if i >= len(args) {
				return "", "", nil, fmt.Errorf("--vault-id requires a value")
			}
			vaultID = args[i]
		default:
			files = append(files, a)
		}
	}
	return pwFile, vaultID, files, nil
}

func resolvePassword(pwFile string) (string, error) {
	if pwFile != "" {
		data, err := os.ReadFile(pwFile)
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

func encryptFiles(files []string, password, vaultID string) int {
	code := 0
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ansible-vault:", err)
			code = 1
			continue
		}
		if vault.IsVault(data) {
			fmt.Fprintf(os.Stderr, "ansible-vault: %s is already encrypted\n", path)
			code = 1
			continue
		}
		enc, err := vault.Encrypt(data, password, vaultID)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ansible-vault:", err)
			code = 1
			continue
		}
		if err := os.WriteFile(path, []byte(enc), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, "ansible-vault:", err)
			code = 1
			continue
		}
		fmt.Printf("Encryption successful: %s\n", path)
	}
	return code
}

func decryptFiles(files []string, password string) int {
	code := 0
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ansible-vault:", err)
			code = 1
			continue
		}
		plain, err := vault.Decrypt(string(data), password)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ansible-vault:", err)
			code = 1
			continue
		}
		if err := os.WriteFile(path, plain, 0o600); err != nil {
			fmt.Fprintln(os.Stderr, "ansible-vault:", err)
			code = 1
			continue
		}
		fmt.Printf("Decryption successful: %s\n", path)
	}
	return code
}

func viewFile(files []string, password string) int {
	if len(files) != 1 {
		fmt.Fprintln(os.Stderr, "ansible-vault: view takes exactly one file")
		return 2
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "ansible-vault:", err)
		return 1
	}
	plain, err := vault.Decrypt(string(data), password)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ansible-vault:", err)
		return 1
	}
	os.Stdout.Write(plain)
	return 0
}
