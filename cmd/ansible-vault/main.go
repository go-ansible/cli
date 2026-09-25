// Command ansible-vault encrypts/decrypts/views files in Ansible's
// Vault 1.1 format.
package main

import (
	"fmt"
	"github.com/go-ansible/cli/internal/vaultpw"
	"golang.org/x/term"
	"os"
	"strings"

	"github.com/go-ansible/cli/internal/version"
	"github.com/go-ansible/vault"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) > 0 && (args[0] == "--version" || args[0] == "-version") {
		fmt.Println(version.String("ansible-vault"))
		return 0
	}
	if len(args) < 1 {
		usage()
		return 2
	}
	sub, rest := args[0], args[1:]

	// encrypt_string takes its secrets as positional arguments and a
	// --name, so it parses its own flags rather than treating every
	// positional as a file path.
	if sub == "encrypt_string" {
		return encryptString(rest)
	}

	pwFile, vaultID, newPwFile, files, err := parseVaultFlagsFull(rest)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]:", err)
		return 2
	}
	if len(files) == 0 {
		usage()
		return 2
	}

	password, err := resolvePassword(pwFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]:", err)
		return 1
	}

	switch sub {
	case "encrypt":
		return encryptFiles(files, password, vaultID)
	case "decrypt":
		return decryptFiles(files, password)
	case "view":
		return viewFile(files, password)
	case "create":
		if len(files) != 1 {
			fmt.Fprintln(os.Stderr, "ansible-vault: create takes exactly one filename")
			return 2
		}
		// Real Ansible gates this on stdout being a terminal, with the
		// same wording and the same --skip-tty-check escape: an editor
		// opened on a pipe would hang or silently do nothing.
		if !stdoutIsTTY() && !skipTTY {
			fmt.Fprintln(os.Stderr, "ansible-vault: not a tty, editor cannot be opened")
			return 1
		}
		if err := createFile(files[0], password, vaultID); err != nil {
			fmt.Fprintln(os.Stderr, "[ERROR]:", err)
			return 1
		}
		return 0
	case "edit":
		for _, f := range files {
			if err := editFile(f, password); err != nil {
				fmt.Fprintln(os.Stderr, "[ERROR]:", err)
				return 1
			}
		}
		return 0
	case "rekey":
		if newPwFile == "" {
			fmt.Fprintln(os.Stderr, "ansible-vault: rekey requires --new-vault-password-file")
			return 2
		}
		newPassword, err := vaultpw.Resolve(newPwFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "[ERROR]:", err)
			return 1
		}
		return rekeyFiles(files, password, newPassword, vaultID)
	default:
		usage()
		return 2
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  ansible-vault encrypt FILE [FILE2 ...] --vault-password-file=PATH [--vault-id=NAME]
  ansible-vault decrypt FILE [FILE2 ...] --vault-password-file=PATH
  ansible-vault view FILE --vault-password-file=PATH
  ansible-vault rekey FILE [FILE2 ...] --vault-password-file=PATH --new-vault-password-file=PATH
  ansible-vault encrypt_string SECRET [SECRET2 ...] --vault-password-file=PATH [--name NAME]
  ansible-vault create FILE --vault-password-file=PATH [--vault-id=NAME] [--skip-tty-check]
  ansible-vault edit FILE [FILE2 ...] --vault-password-file=PATH`)
}

// parseVaultFlags does minimal hand-rolled flag parsing (--flag=value
// or --flag value) so file arguments can be interleaved naturally,
// matching how ansible-vault itself is invoked.
func parseVaultFlags(args []string) (pwFile, vaultID string, files []string, err error) {
	pwFile, vaultID, _, files, err = parseVaultFlagsFull(args)
	return pwFile, vaultID, files, err
}

// parseVaultFlagsFull also returns --new-vault-password-file, which only
// rekey uses.
// skipTTY is set by --skip-tty-check, which real ansible-vault offers so
// create can run where stdout is not a terminal.
var skipTTY bool

func parseVaultFlagsFull(args []string) (pwFile, vaultID, newPwFile string, files []string, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case strings.HasPrefix(a, "--vault-password-file="):
			pwFile = strings.TrimPrefix(a, "--vault-password-file=")
		case a == "--vault-password-file":
			i++
			if i >= len(args) {
				return "", "", "", nil, fmt.Errorf("--vault-password-file requires a value")
			}
			pwFile = args[i]
		case a == "--skip-tty-check":
			skipTTY = true
		case strings.HasPrefix(a, "--new-vault-password-file="):
			newPwFile = strings.TrimPrefix(a, "--new-vault-password-file=")
		case a == "--new-vault-password-file":
			i++
			if i >= len(args) {
				return "", "", "", nil, fmt.Errorf("--new-vault-password-file requires a value")
			}
			newPwFile = args[i]
		case strings.HasPrefix(a, "--vault-id="):
			vaultID = strings.TrimPrefix(a, "--vault-id=")
		case a == "--vault-id":
			i++
			if i >= len(args) {
				return "", "", "", nil, fmt.Errorf("--vault-id requires a value")
			}
			vaultID = args[i]
		default:
			files = append(files, a)
		}
	}
	return pwFile, vaultID, newPwFile, files, nil
}

func resolvePassword(pwFile string) (string, error) { return vaultpw.Resolve(pwFile) }

func encryptFiles(files []string, password, vaultID string) int {
	code := 0
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "[ERROR]:", err)
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
			fmt.Fprintln(os.Stderr, "[ERROR]:", err)
			code = 1
			continue
		}
		if err := os.WriteFile(path, []byte(enc), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, "[ERROR]:", err)
			code = 1
			continue
		}
		reportSuccess("Encryption successful")
	}
	return code
}

func decryptFiles(files []string, password string) int {
	code := 0
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "[ERROR]:", err)
			code = 1
			continue
		}
		plain, err := vault.Decrypt(string(data), password)
		if err != nil {
			fmt.Fprintln(os.Stderr, "[ERROR]:", err)
			code = 1
			continue
		}
		if err := os.WriteFile(path, plain, 0o600); err != nil {
			fmt.Fprintln(os.Stderr, "[ERROR]:", err)
			code = 1
			continue
		}
		reportSuccess("Decryption successful")
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
		fmt.Fprintln(os.Stderr, "[ERROR]:", err)
		return 1
	}
	plain, err := vault.Decrypt(string(data), password)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]:", err)
		return 1
	}
	os.Stdout.Write(plain)
	return 0
}

// rekeyFiles re-encrypts each file under a new password: decrypt with the
// old one, encrypt with the new. A file that is not encrypted at all is an
// error rather than a silent encrypt, since rekey means "change the
// password on this", and quietly encrypting a plaintext file is not that.
func rekeyFiles(files []string, oldPassword, newPassword, vaultID string) int {
	code := 0
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "[ERROR]:", err)
			code = 1
			continue
		}
		if !vault.IsVault(data) {
			fmt.Fprintf(os.Stderr, "ansible-vault: %s is not encrypted\n", path)
			code = 1
			continue
		}
		plain, err := vault.Decrypt(string(data), oldPassword)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ansible-vault: %s: %v\n", path, err)
			code = 1
			continue
		}
		text, err := vault.Encrypt(plain, newPassword, vaultID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ansible-vault: %s: %v\n", path, err)
			code = 1
			continue
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, "[ERROR]:", err)
			code = 1
			continue
		}
		reportSuccess("Rekey successful")
	}
	return code
}

// encryptString implements `ansible-vault encrypt_string`, which prints a
// !vault-tagged YAML scalar for pasting into an otherwise-readable vars
// file rather than encrypting a file in place.
//
// The layout is real Ansible's, measured from its own output: the body is
// indented ten spaces under the tag, and --name prefixes it with
// "NAME: ". Reading one back is vault.UnmarshalYAML's job, and that
// landed first on purpose — writing something this ecosystem could not
// read would be worse than not writing it.
func encryptString(args []string) int {
	var pwFile, vaultID, name string
	var secrets []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		next := func(flag string) (string, bool) {
			i++
			if i >= len(args) {
				fmt.Fprintf(os.Stderr, "ansible-vault: %s requires a value\n", flag)
				return "", false
			}
			return args[i], true
		}
		switch {
		case strings.HasPrefix(a, "--vault-password-file="):
			pwFile = strings.TrimPrefix(a, "--vault-password-file=")
		case a == "--vault-password-file":
			v, ok := next(a)
			if !ok {
				return 2
			}
			pwFile = v
		case strings.HasPrefix(a, "--vault-id="):
			vaultID = strings.TrimPrefix(a, "--vault-id=")
		case a == "--vault-id":
			v, ok := next(a)
			if !ok {
				return 2
			}
			vaultID = v
		case strings.HasPrefix(a, "--name="):
			name = strings.TrimPrefix(a, "--name=")
		case a == "--name" || a == "-n":
			v, ok := next(a)
			if !ok {
				return 2
			}
			name = v
		default:
			secrets = append(secrets, a)
		}
	}
	if len(secrets) == 0 {
		fmt.Fprintln(os.Stderr, "ansible-vault: encrypt_string needs a value to encrypt")
		return 2
	}

	password, err := vaultpw.Resolve(pwFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]:", err)
		return 1
	}

	for _, secret := range secrets {
		text, err := vault.Encrypt([]byte(secret), password, vaultID)
		if err != nil {
			fmt.Fprintln(os.Stderr, "[ERROR]:", err)
			return 1
		}
		if name != "" {
			fmt.Printf("%s: !vault |\n", name)
		} else {
			fmt.Println("!vault |")
		}
		for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
			fmt.Printf("          %s\n", line)
		}
	}
	return 0
}

// reportSuccess says a vault operation worked, the way real
// ansible-vault does — which is more particular than it looks, and
// all three parts were wrong here:
//
//   - only when stdout is a TTY. Real guards every one of these with
//     `if sys.stdout.isatty()`, so a piped or redirected run says
//     nothing and a script reading the output sees only the data.
//   - on STDERR, so it never lands in that data.
//   - without the file name. Real prints the bare phrase.
//
// Measuring alone would have said "real prints nothing" — the runs
// that showed that were piped. Reading ansible/cli/vault.py alongside
// showed why, and that the interactive message is real and worth
// keeping.
func reportSuccess(message string) {
	if term.IsTerminal(int(os.Stdout.Fd())) {
		fmt.Fprintln(os.Stderr, message)
	}
}
