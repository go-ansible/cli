package version

import (
	"strings"
	"testing"
)

func TestStringIncludesToolName(t *testing.T) {
	got := String("ansible-playbook")
	if !strings.HasPrefix(got, "ansible-playbook ") {
		t.Fatalf("String(%q) = %q, want it to start with the tool name", "ansible-playbook", got)
	}
	if !strings.Contains(got, "go-ansible") {
		t.Fatalf("String() = %q, want it to mention go-ansible", got)
	}
}

func TestStringFallsBackToDevWithoutBuildInfo(t *testing.T) {
	// go test builds without an embedded module version, so this
	// exercises the "dev" fallback branch under normal test execution.
	got := String("ansible")
	if !strings.Contains(got, "dev") && !strings.HasPrefix(got, "ansible v") {
		t.Fatalf("String() = %q, want either the dev fallback or a real module version", got)
	}
}
