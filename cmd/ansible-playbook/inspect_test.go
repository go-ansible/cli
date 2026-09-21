package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The playbook every case below inspects, and the transcripts real
// ansible-core 2.21.4 printed for it. Tabs before TAGS are real
// Ansible's own, as is the blank line between plays.
const inspectPlaybook = `- name: first play
  hosts: web
  gather_facts: false
  tags: [playtag]
  tasks:
    - {name: alpha, debug: {msg: a}, tags: [t1, shared]}
    - {name: beta,  debug: {msg: b}}
    - name: a block
      block:
        - {name: gamma, debug: {msg: g}, tags: [t2]}
      rescue:
        - {name: in-rescue, debug: {msg: r}}
      always:
        - {name: in-always, debug: {msg: w}}
      tags: [blocktag]
- name: second play
  hosts: h1
  gather_facts: false
  tasks:
    - {name: delta, debug: {msg: d}, tags: [shared]}
`

func TestInspectFlagsMatchRealAnsible(t *testing.T) {
	dir := t.TempDir()
	inv := filepath.Join(dir, "inv.yml")
	writeFile(t, inv, `web:
  hosts:
    h1: {ansible_connection: local}
    h2: {ansible_connection: local}
`)
	pb := filepath.Join(dir, "site.yml")
	writeFile(t, pb, inspectPlaybook)

	tests := []struct {
		name string
		args []string
		want string
	}{{
		name: "--list-tasks",
		args: []string{"--list-tasks"},
		want: "\nplaybook: " + pb + "\n" +
			"\n  play #1 (web): first play\tTAGS: [playtag]\n" +
			"    tasks:\n" +
			"      alpha\tTAGS: [playtag, shared, t1]\n" +
			"      beta\tTAGS: [playtag]\n" +
			// gamma is inside a block; its rescue and always tasks are
			// NOT listed, which is real ansible-core's own asymmetry.
			"      gamma\tTAGS: [blocktag, playtag, t2]\n" +
			"\n  play #2 (h1): second play\tTAGS: []\n" +
			"    tasks:\n" +
			"      delta\tTAGS: [shared]\n",
	}, {
		name: "--list-tags",
		args: []string{"--list-tags"},
		want: "\nplaybook: " + pb + "\n" +
			"\n  play #1 (web): first play\tTAGS: [playtag]\n" +
			"      TASK TAGS: [blocktag, playtag, shared, t1, t2]\n" +
			"\n  play #2 (h1): second play\tTAGS: []\n" +
			"      TASK TAGS: [shared]\n",
	}, {
		name: "--syntax-check prints only the header",
		args: []string{"--syntax-check"},
		want: "\nplaybook: " + pb + "\n",
	}, {
		// The listing shares the run's own tag rule, so a filter shows
		// exactly the tasks a run would execute.
		name: "--list-tasks --tags t1",
		args: []string{"--list-tasks", "--tags", "t1"},
		want: "\nplaybook: " + pb + "\n" +
			"\n  play #1 (web): first play\tTAGS: [playtag]\n" +
			"    tasks:\n" +
			"      alpha\tTAGS: [playtag, shared, t1]\n" +
			"\n  play #2 (h1): second play\tTAGS: []\n" +
			"    tasks:\n",
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var code int
			out := captureStdout(t, func() {
				code = run(append([]string{"-i", inv, "--no-color"}, append(tt.args, pb)...))
			})
			if code != 0 {
				t.Errorf("exit = %d, want 0", code)
			}
			if out != tt.want {
				t.Errorf("got:\n%q\nwant:\n%q", out, tt.want)
			}
		})
	}
}

// --list-hosts resolves each play's pattern and honours --limit. Host
// ORDER is deliberately not asserted: real ansible-core's own order is
// non-deterministic — three runs of the same playbook gave three
// different orders — so only the set is meaningful.
func TestListHostsMatchesRealAnsible(t *testing.T) {
	dir := t.TempDir()
	inv := filepath.Join(dir, "inv.yml")
	writeFile(t, inv, `web:
  hosts:
    h1: {ansible_connection: local}
    h2: {ansible_connection: local}
    h3: {ansible_connection: local}
`)
	pb := filepath.Join(dir, "site.yml")
	writeFile(t, pb, `- {name: p, hosts: "web:!h1", gather_facts: false, tasks: [{name: t, debug: {msg: x}}]}
`)

	out := captureStdout(t, func() {
		if code := run([]string{"-i", inv, "--no-color", "--list-hosts", pb}); code != 0 {
			t.Fatalf("exit = %d", code)
		}
	})
	// The raw pattern text, not a split of it.
	if !strings.Contains(out, "    pattern: ['web:!h1']\n") {
		t.Errorf("pattern line missing or reformatted:\n%s", out)
	}
	if !strings.Contains(out, "    hosts (2):\n") {
		t.Errorf("host count wrong:\n%s", out)
	}
	for _, h := range []string{"h2", "h3"} {
		if !strings.Contains(out, "      "+h+"\n") {
			t.Errorf("missing host %s:\n%s", h, out)
		}
	}
	if strings.Contains(out, "      h1\n") {
		t.Errorf("h1 is excluded by the pattern but was listed:\n%s", out)
	}

	// --limit narrows it further.
	out = captureStdout(t, func() {
		run([]string{"-i", inv, "--no-color", "--list-hosts", "--limit", "h3", pb})
	})
	if !strings.Contains(out, "    hosts (1):\n") || !strings.Contains(out, "      h3\n") {
		t.Errorf("--limit not applied to --list-hosts:\n%s", out)
	}
}

// A role's tasks are listed, prefixed with the role they came from.
func TestListTasksNamesTheRole(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "inv.yml"), "all:\n  hosts:\n    h1: {ansible_connection: local}\n")
	roleTasks := filepath.Join(dir, "roles", "r1", "tasks")
	if err := os.MkdirAll(roleTasks, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(roleTasks, "main.yml"),
		"- {name: role-task, debug: {msg: r}, tags: [rt]}\n")
	pb := filepath.Join(dir, "site.yml")
	writeFile(t, pb, "- {name: p, hosts: all, gather_facts: false, roles: [r1], tasks: [{name: plain, debug: {msg: x}}]}\n")

	out := captureStdout(t, func() {
		run([]string{"-i", filepath.Join(dir, "inv.yml"), "--no-color", "--list-tasks", pb})
	})
	if !strings.Contains(out, "      r1 : role-task\tTAGS: [rt]\n") {
		t.Errorf("role task must be listed as `r1 : role-task`:\n%s", out)
	}
	if !strings.Contains(out, "      plain\tTAGS: []\n") {
		t.Errorf("the play's own task is missing:\n%s", out)
	}
}

// A broken playbook fails a syntax check with real ansible-playbook's
// own exit code.
func TestSyntaxCheckRejectsABrokenPlaybook(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "inv.yml"), "all:\n  hosts:\n    h1: {ansible_connection: local}\n")
	pb := filepath.Join(dir, "broken.yml")
	writeFile(t, pb, "- name: bad\n  hosts: all\n  tasks:\n    - {name: x, debug: {msg: y}\n")

	if code := run([]string{"-i", filepath.Join(dir, "inv.yml"), "--syntax-check", pb}); code != 4 {
		t.Errorf("exit = %d, real ansible-playbook exits 4 on a parse error", code)
	}
}

// TestStartAtTaskFlag covers the CLI wiring for --start-at-task,
// including the glob matching and case sensitivity measured from real
// ansible-core 2.21.4.
func TestStartAtTaskFlag(t *testing.T) {
	dir := t.TempDir()
	inv := filepath.Join(dir, "inv.yml")
	writeFile(t, inv, "all:\n  hosts:\n    h1: {ansible_connection: local}\n")
	pb := filepath.Join(dir, "site.yml")
	writeFile(t, pb, `- name: p1
  hosts: all
  gather_facts: false
  tasks:
    - {name: one,   debug: {msg: x}}
    - {name: two,   debug: {msg: x}}
- name: p2
  hosts: all
  gather_facts: false
  tasks:
    - {name: three, debug: {msg: x}}
`)

	ran := func(args ...string) string {
		out := captureStdout(t, func() {
			if code := run(append([]string{"-i", inv, "--no-color"}, append(args, pb)...)); code != 0 {
				t.Fatalf("exit = %d", code)
			}
		})
		var tasks []string
		for _, name := range []string{"one", "two", "three"} {
			if strings.Contains(out, "TASK ["+name+"]") {
				tasks = append(tasks, name)
			}
		}
		return strings.Join(tasks, ",")
	}

	for _, tt := range []struct{ start, want string }{
		{"one", "one,two,three"},
		{"two", "two,three"},
		// The started state carries across plays.
		{"three", "three"},
		{"tw*", "two,three"},
		{"*wo", "two,three"},
		// A bare prefix does not match, and the match is case-sensitive.
		{"tw", ""},
		{"TWO", ""},
		{"nosuch", ""},
	} {
		if got := ran("--start-at-task", tt.start); got != tt.want {
			t.Errorf("--start-at-task %q ran %q, real ansible-playbook runs %q", tt.start, got, tt.want)
		}
	}
}

// --flush-cache is accepted and changes nothing: this port keeps no
// fact cache, so there is nothing to flush.
func TestFlushCacheIsAcceptedAndHarmless(t *testing.T) {
	dir := t.TempDir()
	inv := filepath.Join(dir, "inv.yml")
	writeFile(t, inv, "all:\n  hosts:\n    h1: {ansible_connection: local}\n")
	pb := filepath.Join(dir, "site.yml")
	writeFile(t, pb, "- {name: p, hosts: all, gather_facts: false, tasks: [{name: t, debug: {msg: x}}]}\n")

	withFlag := captureStdout(t, func() {
		if code := run([]string{"-i", inv, "--no-color", "--flush-cache", pb}); code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
	})
	without := captureStdout(t, func() {
		run([]string{"-i", inv, "--no-color", pb})
	})
	if !strings.Contains(withFlag, "TASK [t]") {
		t.Errorf("--flush-cache changed what ran:\n%s", withFlag)
	}
	if strings.Count(withFlag, "TASK [t]") != strings.Count(without, "TASK [t]") {
		t.Error("--flush-cache must be a no-op here")
	}
}
