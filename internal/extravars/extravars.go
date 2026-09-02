// Package extravars parses Ansible's `-e`/`--extra-vars` command-line
// argument: a `key=value` pair, a raw JSON/YAML object, or `@file` to
// load one from disk (JSON or YAML, by extension).
package extravars

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Parse merges every -e argument (in order — later ones win on key
// conflict, matching Ansible) into one map.
func Parse(args []string) (map[string]any, error) {
	out := map[string]any{}
	for _, a := range args {
		vars, err := parseOne(a)
		if err != nil {
			return nil, fmt.Errorf("--extra-vars %q: %w", a, err)
		}
		for k, v := range vars {
			out[k] = v
		}
	}
	return out, nil
}

func parseOne(a string) (map[string]any, error) {
	a = strings.TrimSpace(a)
	if strings.HasPrefix(a, "@") {
		return parseFile(a[1:])
	}
	trimmed := strings.TrimSpace(a)
	if strings.HasPrefix(trimmed, "{") {
		var m map[string]any
		if err := json.Unmarshal([]byte(trimmed), &m); err != nil {
			return nil, fmt.Errorf("parsing JSON: %w", err)
		}
		return m, nil
	}
	// key=value [key2=value2 ...], Ansible's simple form.
	out := map[string]any{}
	for _, pair := range strings.Fields(a) {
		key, val, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, fmt.Errorf("expected key=value, got %q", pair)
		}
		out[key] = coerce(val)
	}
	return out, nil
}

func parseFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var m map[string]any
	if strings.HasSuffix(path, ".json") {
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
		return m, nil
	}
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return m, nil
}

// coerce interprets a bare key=value's value as a bool/int/float where
// unambiguous, else leaves it as a string — matching Ansible's own
// extra-vars typing for the simple key=value form.
func coerce(s string) any {
	switch s {
	case "true", "True", "TRUE":
		return true
	case "false", "False", "FALSE":
		return false
	}
	if i, err := strconv.Atoi(s); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return s
}
