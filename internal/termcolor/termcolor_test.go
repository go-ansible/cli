package termcolor

import (
	"os"
	"testing"
)

// Measured against ansible-core 2.21.4, redirected each time except
// the last row, which used a pty.
func TestEnabled(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	cases := []struct {
		name string
		env  map[string]string
		flag bool
		want bool
	}{
		// The default for anything that is not a terminal, and the
		// one this port got wrong: it coloured every redirected run.
		{"redirected", nil, false, false},
		{"ANSIBLE_FORCE_COLOR", map[string]string{"ANSIBLE_FORCE_COLOR": "1"}, false, true},
		{"ANSIBLE_NOCOLOR", map[string]string{"ANSIBLE_NOCOLOR": "1"}, false, false},
		{"NO_COLOR", map[string]string{"NO_COLOR": "1"}, false, false},
		// A refusal beats a demand -- real honours the cross-tool
		// convention over its own variable.
		{"NO_COLOR beats FORCE", map[string]string{"NO_COLOR": "1", "ANSIBLE_FORCE_COLOR": "1"}, false, false},
		// And the binary's own flag beats everything.
		{"--no-color beats FORCE", map[string]string{"ANSIBLE_FORCE_COLOR": "1"}, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range []string{"NO_COLOR", "ANSIBLE_NOCOLOR", "ANSIBLE_FORCE_COLOR"} {
				t.Setenv(k, "")
				os.Unsetenv(k)
			}
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if got := Enabled(f, tc.flag); got != tc.want {
				t.Errorf("Enabled = %v, want %v", got, tc.want)
			}
		})
	}
}
