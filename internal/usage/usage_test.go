package usage

import "testing"

func TestWanted(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want bool
	}{
		{"long", []string{"--help"}, true},
		{"short", []string{"-h"}, true},
		{"among others", []string{"-i", "inv", "--help", "play.yml"}, true},
		{"none", []string{"-i", "inv", "play.yml"}, false},
		{"empty", nil, false},
		// Anything after a bare -- is an operand: a playbook really
		// named --help is not a request for help.
		{"after a double dash", []string{"--", "--help"}, false},
		{"before a double dash", []string{"--help", "--", "x"}, true},
		// Not help: a different flag that merely starts the same way.
		{"help-ish flag", []string{"--help-me"}, false},
		{"h-ish flag", []string{"-hosts"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Wanted(tc.args); got != tc.want {
				t.Errorf("Wanted(%q) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}
