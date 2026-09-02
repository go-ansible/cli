// Package report formats playbook.Result / ad-hoc module results the
// way ansible-playbook/ansible print them to a terminal: TASK headers,
// per-host ok/changed/failed/skipped lines, and a PLAY RECAP.
package report

import (
	"fmt"
	"io"
	"sort"

	"github.com/go-ansible/playbook"
)

const (
	colorGreen  = "\033[0;32m"
	colorYellow = "\033[0;33m"
	colorRed    = "\033[0;31m"
	colorCyan   = "\033[0;36m"
	colorReset  = "\033[0m"
)

// Printer prints playbook.Result values as they arrive, grouping
// consecutive results under a TASK header when the task name changes.
type Printer struct {
	w        io.Writer
	Color    bool
	lastTask string
}

func NewPrinter(w io.Writer, color bool) *Printer {
	return &Printer{w: w, Color: color}
}

func (p *Printer) color(code, s string) string {
	if !p.Color {
		return s
	}
	return code + s + colorReset
}

// OnResult is a playbook.Engine.OnResult callback.
func (p *Printer) OnResult(r playbook.Result) {
	if r.Task != p.lastTask {
		fmt.Fprintf(p.w, "\n%s\n", p.color(colorCyan, "TASK ["+r.Task+"]"))
		p.lastTask = r.Task
	}
	switch {
	case r.Failed:
		line := fmt.Sprintf("failed: [%s]", r.Host)
		if r.Msg != "" {
			line += " => " + r.Msg
		}
		fmt.Fprintln(p.w, p.color(colorRed, line))
	case r.Skipped:
		fmt.Fprintln(p.w, p.color(colorCyan, fmt.Sprintf("skipping: [%s]", r.Host)))
	case r.Changed:
		fmt.Fprintln(p.w, p.color(colorYellow, fmt.Sprintf("changed: [%s]", r.Host)))
	default:
		fmt.Fprintln(p.w, p.color(colorGreen, fmt.Sprintf("ok: [%s]", r.Host)))
	}
}

// Recap prints Ansible's PLAY RECAP block.
func Recap(w io.Writer, rr *playbook.RunResult, color bool) {
	fmt.Fprintln(w)
	printer := &Printer{w: w, Color: color}
	fmt.Fprintln(w, printer.color(colorCyan, "PLAY RECAP"))
	summary := rr.Summary()
	hosts := make([]string, 0, len(summary))
	for h := range summary {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	for _, h := range hosts {
		s := summary[h]
		line := fmt.Sprintf("%-24s : ok=%-4d changed=%-4d failed=%-4d skipped=%-4d",
			h, s.Ok+s.Changed, s.Changed, s.Failed, s.Skipped)
		code := colorGreen
		if s.Failed > 0 {
			code = colorRed
		} else if s.Changed > 0 {
			code = colorYellow
		}
		fmt.Fprintln(w, printer.color(code, line))
	}
}
