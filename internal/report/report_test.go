package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/go-ansible/playbook"
)

func TestPrinterOkChangedFailedSkipped(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinter(&buf, false)
	p.OnResult(playbook.Result{Host: "h", Task: "t1"})
	p.OnResult(playbook.Result{Host: "h", Task: "t1", Changed: true})
	p.OnResult(playbook.Result{Host: "h", Task: "t2", Failed: true, Msg: "boom"})
	p.OnResult(playbook.Result{Host: "h", Task: "t3", Skipped: true})

	out := buf.String()
	if strings.Count(out, "TASK [t1]") != 1 {
		t.Errorf("want exactly one TASK header for t1 (grouped consecutive results), got:\n%s", out)
	}
	if !strings.Contains(out, "ok: [h]") {
		t.Error("missing ok line")
	}
	if !strings.Contains(out, "changed: [h]") {
		t.Error("missing changed line")
	}
	if !strings.Contains(out, "failed: [h] => boom") {
		t.Error("missing failed line with message")
	}
	if !strings.Contains(out, "skipping: [h]") {
		t.Error("missing skipping line")
	}
}

func TestPrinterColor(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinter(&buf, true)
	p.OnResult(playbook.Result{Host: "h", Task: "t"})
	if !strings.Contains(buf.String(), "\033[") {
		t.Error("want ANSI color codes when Color is enabled")
	}
}

func TestRecapCountsAndOrdering(t *testing.T) {
	rr := &playbook.RunResult{Plays: []playbook.PlayResult{{
		Results: []playbook.Result{
			{Host: "b", Changed: true},
			{Host: "a", Failed: true},
			{Host: "a", Skipped: true},
		},
	}}}
	var buf bytes.Buffer
	Recap(&buf, rr, false)
	out := buf.String()
	if !strings.Contains(out, "PLAY RECAP") {
		t.Error("missing PLAY RECAP header")
	}
	aIdx := strings.Index(out, "a ")
	bIdx := strings.Index(out, "b ")
	if aIdx == -1 || bIdx == -1 || aIdx > bIdx {
		t.Errorf("hosts should be sorted alphabetically, got:\n%s", out)
	}
	if !strings.Contains(out, "failed=1") {
		t.Error("missing failed count")
	}
	if !strings.Contains(out, "changed=1") {
		t.Error("missing changed count")
	}
}

func TestRecapColorByOutcome(t *testing.T) {
	cases := []struct {
		name string
		r    playbook.Result
	}{
		{"failed", playbook.Result{Host: "h", Failed: true}},
		{"changed", playbook.Result{Host: "h", Changed: true}},
		{"ok", playbook.Result{Host: "h"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rr := &playbook.RunResult{Plays: []playbook.PlayResult{{Results: []playbook.Result{c.r}}}}
			var buf bytes.Buffer
			Recap(&buf, rr, true)
			if !strings.Contains(buf.String(), "\033[") {
				t.Error("want ANSI color codes when Color is enabled")
			}
		})
	}
}
