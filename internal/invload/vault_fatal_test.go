package invload

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-ansible/playbook"
	"github.com/go-ansible/vault"
)

// A vault-encrypted inventory source that cannot be read is the one
// inventory problem real Ansible treats as FATAL rather than as a
// warning. Measured against ansible-core 2.21.4, identically from
// ansible-inventory, ansible-playbook and ansible:
//
//	-i /nope.ini                 WARNING, exit 0
//	encrypted, no vault secret   [ERROR], exit 4
//	encrypted, wrong password    [ERROR], exit 4
//
// The distinction matters: an unusable source means "carry on with what
// you have", while a source you cannot decrypt means the inventory you
// asked for is not the one you would get. Before this, both were
// warnings and every command exited 0 -- so `ansible-inventory --list >
// hosts.json && deploy` succeeded with a hosts.json holding no hosts.

const invPassword = "mypassword"

// encryptedInventory writes an inventory whose group_vars/all is vault
// encrypted, and returns the path to the inventory file.
func encryptedInventory(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	invPath := filepath.Join(dir, "hosts.ini")
	if err := os.WriteFile(invPath, []byte("h1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gv := filepath.Join(dir, "group_vars", "all")
	if err := os.MkdirAll(gv, 0o700); err != nil {
		t.Fatal(err)
	}
	enc, err := vault.Encrypt([]byte("db_password: s3cr3t\n"), invPassword, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gv, "vault.yml"), []byte(enc), 0o600); err != nil {
		t.Fatal(err)
	}
	return invPath
}

func TestAnUndecryptableSourceIsFatal(t *testing.T) {
	path := encryptedInventory(t)

	for _, c := range []struct {
		name, password, want string
	}{
		{"no password at all", "", "Attempting to decrypt but no vault secrets found."},
		{"the wrong password", "not-the-password", "Decryption failed (no vault secrets were found that could decrypt)."},
	} {
		t.Run(c.name, func(t *testing.T) {
			var warned []string
			warn := func(msg string) { warned = append(warned, msg) }
			inv, err := Load(path, c.password, "all", playbook.Warner(warn))
			if err == nil {
				t.Fatalf("an undecryptable source was not fatal: err=nil, inventory has %d host(s), "+
					"and the caller would carry on and exit 0", len(inv.Hosts))
			}
			if err.Error() != c.want {
				t.Errorf("sentence:\n got %q\nwant %q", err.Error(), c.want)
			}
		})
	}
}

// The CONTROL for the above: the correct password must still load, or
// "it is fatal" could simply mean the fixture never decrypts.
func TestTheCorrectPasswordStillLoads(t *testing.T) {
	path := encryptedInventory(t)
	inv, err := Load(path, invPassword, "all", playbook.NewWarner(os.Stderr))
	if err != nil {
		t.Fatalf("the correct password did not load the inventory: %v", err)
	}
	if _, ok := inv.Hosts["h1"]; !ok {
		t.Fatalf("decrypted, but the host is missing; got %d host(s)", len(inv.Hosts))
	}
	// The decrypted value lands on the GROUP the group_vars file names,
	// not copied onto each host: that is where inventory keeps it.
	g, ok := inv.Groups["all"]
	if !ok {
		t.Fatalf("no \"all\" group; groups: %d", len(inv.Groups))
	}
	if got := g.Vars["db_password"]; got != "s3cr3t" {
		t.Errorf("db_password on group all = %v, want s3cr3t", got)
	}
}

// And an ordinary unusable source stays a WARNING, which is real's
// measured behaviour and the thing this change must not break.
func TestAMissingSourceIsStillOnlyAWarning(t *testing.T) {
	var warned int
	warn := func(string) { warned++ }
	inv, err := Load(filepath.Join(t.TempDir(), "absent.ini"), "", "all", playbook.Warner(warn))
	if err != nil {
		t.Fatalf("a missing source became fatal: %v — real exits 0 here", err)
	}
	if inv == nil {
		t.Fatal("a missing source returned no inventory")
	}
	if warned == 0 {
		t.Error("a missing source warned about nothing")
	}
}
