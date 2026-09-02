package extravars

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSimple(t *testing.T) {
	got, err := Parse([]string{"x=1", "y=hello"})
	if err != nil {
		t.Fatal(err)
	}
	if got["x"] != 1 || got["y"] != "hello" {
		t.Fatalf("got = %v", got)
	}
}

func TestParseMultipleKeyValueInOneArg(t *testing.T) {
	got, err := Parse([]string{"x=1 y=2"})
	if err != nil {
		t.Fatal(err)
	}
	if got["x"] != 1 || got["y"] != 2 {
		t.Fatalf("got = %v", got)
	}
}

func TestParseLaterWins(t *testing.T) {
	got, err := Parse([]string{"x=1", "x=2"})
	if err != nil {
		t.Fatal(err)
	}
	if got["x"] != 2 {
		t.Fatalf("got = %v, want later -e to win", got)
	}
}

func TestParseBool(t *testing.T) {
	got, err := Parse([]string{"a=true", "b=False"})
	if err != nil {
		t.Fatal(err)
	}
	if got["a"] != true || got["b"] != false {
		t.Fatalf("got = %v", got)
	}
}

func TestParseFloat(t *testing.T) {
	got, err := Parse([]string{"x=1.5"})
	if err != nil {
		t.Fatal(err)
	}
	if got["x"] != 1.5 {
		t.Fatalf("got = %v", got)
	}
}

func TestParseJSON(t *testing.T) {
	got, err := Parse([]string{`{"x": 1, "y": [1,2,3]}`})
	if err != nil {
		t.Fatal(err)
	}
	if got["x"] != float64(1) {
		t.Fatalf("got = %v", got)
	}
	list, ok := got["y"].([]any)
	if !ok || len(list) != 3 {
		t.Fatalf("y = %v", got["y"])
	}
}

func TestParseFileYAML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "vars.yml")
	if err := os.WriteFile(p, []byte("x: 1\ny: hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Parse([]string{"@" + p})
	if err != nil {
		t.Fatal(err)
	}
	if got["x"] != 1 || got["y"] != "hello" {
		t.Fatalf("got = %v", got)
	}
}

func TestParseFileJSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "vars.json")
	if err := os.WriteFile(p, []byte(`{"x": 1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Parse([]string{"@" + p})
	if err != nil {
		t.Fatal(err)
	}
	if got["x"] != float64(1) {
		t.Fatalf("got = %v", got)
	}
}

func TestParseFileMissing(t *testing.T) {
	if _, err := Parse([]string{"@" + filepath.Join(t.TempDir(), "absent.yml")}); err == nil {
		t.Fatal("want error for a missing file")
	}
}

func TestParseFileBadYAML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.yml")
	if err := os.WriteFile(p, []byte("not: [valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse([]string{"@" + p}); err == nil {
		t.Fatal("want error for malformed YAML")
	}
}

func TestParseBadJSON(t *testing.T) {
	if _, err := Parse([]string{`{"x":`}); err == nil {
		t.Fatal("want error for malformed JSON")
	}
}

func TestParseBadKeyValue(t *testing.T) {
	if _, err := Parse([]string{"noequalsign"}); err == nil {
		t.Fatal("want error for a token with no '='")
	}
}

func TestParseEmpty(t *testing.T) {
	got, err := Parse(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %v", got)
	}
}
