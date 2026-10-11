package main

import (
	"os"
	"regexp"
	"testing"
)

// flagsWithValue is kept by hand, and extractPattern is wrong the moment
// it falls behind: a value-taking flag that is missing from it has its
// VALUE mistaken for the host pattern. That is not a hypothetical --
// adding --vault-password-file broke
//
//	ansible -i inv all -m debug --vault-password-file pw
//
// with `unexpected extra argument "pw" (pattern already given as "all")`,
// because flag.FlagSet stops at the positional pattern and extractPattern
// is what puts the arguments back in an order Parse can read.
//
// So the list is checked against the flag DEFINITIONS rather than trusted.
// Reading the source keeps the two from drifting: the FlagSet is built
// inside run(), where a test cannot reach it.
func TestEveryValueTakingFlagIsInFlagsWithValue(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}

	// fs.String("name", ...) and fs.StringVar(ptr, "name", ...)
	decl := regexp.MustCompile(`fs\.String(?:Var)?\((?:[A-Za-z][A-Za-z0-9]*, )?"([a-z][a-z0-9-]*)"`)
	found := decl.FindAllStringSubmatch(string(src), -1)
	if len(found) < 5 {
		t.Fatalf("the flag-definition scan found only %d string flags, which cannot be right: "+
			"the regexp has drifted from the source and this guard is blind", len(found))
	}

	for _, m := range found {
		name := m[1]
		// A single-character flag has only the one spelling that real
		// accepts: measured, `ansible --i inv ...` is a usage error
		// there, so requiring "--i" here would be requiring something
		// real rejects. Long flags are spelled both ways.
		spellings := []string{"-" + name}
		if len(name) > 1 {
			spellings = append(spellings, "--"+name)
		}
		for _, spelling := range spellings {
			if !flagsWithValue[spelling] {
				t.Errorf("flag %q takes a value but %q is missing from flagsWithValue: "+
					"extractPattern will mistake its value for the host pattern", name, spelling)
			}
		}
	}
}
