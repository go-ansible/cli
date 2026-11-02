package main

import (
	"fmt"
	"io"

	"github.com/go-ansible/playbook"
)

// compatReportRun prints, for each playbook named on the command line,
// the constructs no current Ansible will run and what replaces them.
//
// It exits 4 when it found anything, which is the status both
// implementations already give a playbook that will not run, so a
// pipeline can gate on it. A playbook that is not readable or not YAML
// at all is a different failure and exits 1: "I could not look" must
// not read as "there is nothing wrong", which is the mistake this whole
// flag exists to stop people making.
func compatReportRun(stdout, stderr io.Writer, playbooks []string, vaultPassword string) int {
	found := 0
	for _, path := range playbooks {
		findings, err := playbook.CompatScanFile(path, vaultPassword)
		if err != nil {
			fmt.Fprintln(stderr, "[ERROR]:", err)
			return 1
		}
		fmt.Fprintf(stdout, "\nplaybook: %s\n", path)
		if len(findings) == 0 {
			fmt.Fprintln(stdout, "  nothing here was retired; every keyword and module name is one this port runs")
			continue
		}
		for _, f := range findings {
			fmt.Fprintf(stdout, "  %s\n", f)
		}
		found += len(findings)
	}
	if found > 0 {
		fmt.Fprintf(stdout, "\n%d item(s) will not run as written.\n", found)
		return 4
	}
	return 0
}
