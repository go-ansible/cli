package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every expected string below was measured against ansible-core 2.21.4
// before it was written here.

func TestVaultFailureBlock(t *testing.T) {
	var b bytes.Buffer
	vaultFailure(&b, "view", "vt/t1.yml", "vt/t1.yml", reasonNotVault)
	got := b.String()

	abs, err := filepath.Abs("vt/t1.yml")
	if err != nil {
		t.Fatal(err)
	}
	want := "[ERROR]: Failed to view 'vt/t1.yml': Input is not vault encrypted data.\n" +
		"\nFailed to view 'vt/t1.yml'.\n" +
		"\n<<< caused by >>>\n\n" +
		"Input is not vault encrypted data.\n" +
		"Origin: " + abs + "\n\n"
	if got != want {
		t.Errorf("block:\n got %q\nwant %q", got, want)
	}
}

// The path asymmetry is real's own: view names the file AS GIVEN,
// decrypt and rekey name it absolute. Measured both ways round.
func TestVaultFailurePathAsymmetry(t *testing.T) {
	abs, err := filepath.Abs("vt/t1.yml")
	if err != nil {
		t.Fatal(err)
	}
	var v, d bytes.Buffer
	vaultFailure(&v, "view", "vt/t1.yml", "vt/t1.yml", reasonNotVault)
	vaultFailure(&d, "decrypt", abs, "vt/t1.yml", reasonNotVault)

	if !strings.Contains(v.String(), "Failed to view 'vt/t1.yml':") {
		t.Errorf("view must name the path as given:\n%s", v.String())
	}
	if strings.Contains(v.String(), "Failed to view '"+abs+"':") {
		t.Errorf("view must NOT print the absolute path in its message lines:\n%s", v.String())
	}
	if !strings.Contains(d.String(), "Failed to decrypt '"+abs+"':") {
		t.Errorf("decrypt must name the path absolute:\n%s", d.String())
	}
	// Both carry the SAME Origin, which is always resolved.
	for _, out := range []string{v.String(), d.String()} {
		if !strings.Contains(out, "Origin: "+abs+"\n") {
			t.Errorf("Origin must be the absolute path:\n%s", out)
		}
	}
}

func TestDecryptReasonNamesWhichFailure(t *testing.T) {
	// Real distinguishes the two, and they are the only two a caller
	// can tell apart from outside.
	if got := decryptReason([]byte("top: secret\n")); got != reasonNotVault {
		t.Errorf("plaintext input = %q, real gives %q", got, reasonNotVault)
	}
	enc := []byte("$ANSIBLE_VAULT;1.1;AES256\n3061...\n")
	if got := decryptReason(enc); got != reasonNoSecrets {
		t.Errorf("vault payload with a bad password = %q, real gives %q", got, reasonNoSecrets)
	}
}

func TestReadSourceFailureCarriesPythonsErrno(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nosuch.yml")
	_, err := os.ReadFile(missing)
	if err == nil {
		t.Fatal("expected a read error")
	}
	var b bytes.Buffer
	readSourceFailure(&b, missing, err)
	want := "[ERROR]: Unable to read source file (" + missing + "): [Errno 2] No such file or directory: '" + missing + "'\n"
	if b.String() != want {
		t.Errorf("\n got %q\nwant %q", b.String(), want)
	}
}

func TestPasswordFileMissingPrintsBothLines(t *testing.T) {
	var b bytes.Buffer
	passwordFileMissing(&b, "vt/nopw.txt")
	abs, err := filepath.Abs("vt/nopw.txt")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, real prints two (a WARNING then the ERROR):\n%s", len(lines), b.String())
	}
	if want := "[WARNING]: Error getting vault password file (default): The vault password file " + abs + " was not found"; lines[0] != want {
		t.Errorf("line 1:\n got %q\nwant %q", lines[0], want)
	}
	if want := "[ERROR]: The vault password file " + abs + " was not found"; lines[1] != want {
		t.Errorf("line 2:\n got %q\nwant %q", lines[1], want)
	}
}
