package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/go-ansible/vault"
)

// Real's vault errors, measured against ansible-core 2.21.4.
//
// A failure to read an encrypted file is not one line but a BLOCK:
//
//	[ERROR]: Failed to view 'vt/t1.yml': Input is not vault encrypted data.
//
//	Failed to view 'vt/t1.yml'.
//
//	<<< caused by >>>
//
//	Input is not vault encrypted data.
//	Origin: /abs/path/vt/t1.yml
//
// on stderr, exit 1, with a trailing blank line. This port printed a
// single line of its own wording ("vault: not an ansible-vault
// payload"), so a script matching real's text -- or a person reading
// it -- found neither the verb, the file, nor the reason real names.
//
// The Origin is the ABSOLUTE path in every case. The path in the two
// message lines is NOT: `view` prints it as the caller gave it, while
// `decrypt` and `rekey` print it absolute. That asymmetry is real's
// own -- measured, both ways round -- and is reproduced rather than
// tidied, because a caller grepping for the name it passed finds it
// under view and not under decrypt either way.
const (
	reasonNotVault   = "Input is not vault encrypted data."
	reasonNoSecrets  = "Decryption failed (no vault secrets were found that could decrypt)."
	reasonAlreadyEnc = "input is already encrypted"
)

// vaultFailure prints real's block for a verb that could not read a
// file. pathShown is what the two message lines name; the Origin is
// always resolved.
func vaultFailure(w io.Writer, verb, pathShown, path, reason string) {
	fmt.Fprintf(w, "[ERROR]: Failed to %s '%s': %s\n", verb, pathShown, reason)
	fmt.Fprintf(w, "\nFailed to %s '%s'.\n", verb, pathShown)
	fmt.Fprintf(w, "\n<<< caused by >>>\n\n")
	fmt.Fprintf(w, "%s\n", reason)
	fmt.Fprintf(w, "Origin: %s\n\n", absOrSelf(path))
}

// decryptReason maps this port's own decrypt error onto the sentence
// real prints for it. Only two outcomes are distinguishable from the
// outside, and real names them: the input was not a vault payload at
// all, or it was and no secret opened it.
func decryptReason(data []byte) string {
	if !vault.IsVault(data) {
		return reasonNotVault
	}
	return reasonNoSecrets
}

// readSourceFailure is real's wording for a file it could not read at
// all, which carries Python's own errno text:
//
//	[ERROR]: Unable to read source file (/abs/f.yml): [Errno 2] No such
//	file or directory: '/abs/f.yml'
//
// The errno number and sentence are CPython's; the two this port can
// produce from a plain open are mapped by hand rather than guessed,
// and anything else falls back to the Go error so a reader still gets
// something true.
func readSourceFailure(w io.Writer, path string, err error) {
	abs := absOrSelf(path)
	if detail := pythonErrno(err, abs); detail != "" {
		fmt.Fprintf(w, "[ERROR]: Unable to read source file (%s): %s\n", abs, detail)
		return
	}
	fmt.Fprintf(w, "[ERROR]: Unable to read source file (%s): %v\n", abs, err)
}

func pythonErrno(err error, abs string) string {
	switch {
	case os.IsNotExist(err):
		return fmt.Sprintf("[Errno 2] No such file or directory: '%s'", abs)
	case os.IsPermission(err):
		return fmt.Sprintf("[Errno 13] Permission denied: '%s'", abs)
	}
	return ""
}

// passwordFileMissing is real's pair of lines for a --vault-password-file
// that is not there: a WARNING naming the id it was trying, then the
// ERROR. Measured; this port printed one line carrying a Go open error.
func passwordFileMissing(w io.Writer, path string) {
	abs := absOrSelf(path)
	fmt.Fprintf(w, "[WARNING]: Error getting vault password file (default): The vault password file %s was not found\n", abs)
	fmt.Fprintf(w, "[ERROR]: The vault password file %s was not found\n", abs)
}

func absOrSelf(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}
