// Command ansible-console is an interactive REPL for running ad-hoc
// modules against a live-changeable host pattern — the interactive
// counterpart to ansible's one-shot ad-hoc mode.
//
// Scope note: real ansible-console supports tab completion (readline,
// completing module names and host patterns), a `forks`/`serial`
// command, and several more meta-commands (`list_hosts`, `remote_user`,
// `become_user`, `!<shell command>` verbatim escape). This is a
// deliberately smaller REPL over the same session state: cd/list/
// become/nobecome/help/exit, plus running any registered module by
// name or a bare shell command — no line-editing beyond what the
// terminal itself provides (no readline library), no tab completion.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"strconv"
	"strings"

	"github.com/go-ansible/cli/internal/adhoc"
	"github.com/go-ansible/cli/internal/version"
	"github.com/go-ansible/inventory"
	"github.com/go-ansible/modules"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout))
}

func run(args []string, in io.Reader, out io.Writer) int {
	if len(args) > 0 && (args[0] == "--version" || args[0] == "-version") {
		fmt.Fprintln(out, version.String("ansible-console"))
		return 0
	}

	fs := flag.NewFlagSet("ansible-console", flag.ContinueOnError)
	fs.SetOutput(out)
	inventoryPath := fs.String("i", "", "inventory file or directory (also --inventory)")
	fs.StringVar(inventoryPath, "inventory", "", "inventory file or directory")
	fs.Usage = func() {
		fmt.Fprintln(out, "usage: ansible-console -i INVENTORY [PATTERN]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *inventoryPath == "" {
		fs.Usage()
		return 2
	}
	pattern := "all"
	if fs.NArg() > 0 {
		pattern = fs.Arg(0)
	}

	inv, err := inventory.Load(*inventoryPath)
	if err != nil {
		fmt.Fprintln(out, "[ERROR]:", err)
		return 1
	}

	s, err := newSession(inv, pattern, out)
	if err != nil {
		fmt.Fprintln(out, "[ERROR]:", err)
		return 1
	}

	runREPL(s, in, out)
	return 0
}

// session holds the REPL's mutable state across lines: which hosts
// the current pattern matches, and whether become is on.
type session struct {
	inv     *inventory.Inventory
	pattern string
	hosts   []string
	become  bool
	out     io.Writer

	// remoteUser, forks and verbosity are shown in the prompt and
	// settable from it, as real's console allows.
	remoteUser string
	forks      int
	verbosity  int
}

func newSession(inv *inventory.Inventory, pattern string, out io.Writer) (*session, error) {
	s := &session{inv: inv, out: out, remoteUser: currentUser(), forks: defaultForks}
	if err := s.setPattern(pattern); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *session) setPattern(pattern string) error {
	matched, err := s.inv.Match(pattern)
	if err != nil {
		return err
	}
	names := make([]string, len(matched))
	for i, h := range matched {
		names[i] = h.Name
	}
	s.pattern = pattern
	s.hosts = names
	return nil
}

// prompt is real's: the user it would connect as, the current
// pattern, how many hosts that matches, and the fork count -- ending
// in "#" rather than "$" when become is on, the same way a root shell
// prompt does.
//
//	david@all (3)[f:5]$
//	david@all (3)[f:5]#      (become)
func (s *session) prompt() string {
	end := "$"
	if s.become {
		end = "#"
	}
	return fmt.Sprintf("%s@%s (%d)[f:%d]%s ", s.remoteUser, s.pattern, len(s.hosts), s.forks, end)
}

// defaultForks is real's own default, and what the prompt shows until
// the forks command changes it.
const defaultForks = 5

// currentUser is who the console would connect as when nothing says
// otherwise -- real's own default remote_user.
func currentUser() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	if n := os.Getenv("USER"); n != "" {
		return n
	}
	return "root"
}

// consoleBanner is printed once at startup, verbatim as real prints
// it, followed by a blank line.
const consoleBanner = "Welcome to the ansible console. Type help or ? to list commands."

// consoleFarewell is printed when the session ends, however it ends.
const consoleFarewell = "Ansible-console was exited."

func runREPL(s *session, in io.Reader, out io.Writer) {
	fmt.Fprintf(out, "%s\n\n", consoleBanner)
	scanner := bufio.NewScanner(in)
	fmt.Fprint(out, s.prompt())
	for scanner.Scan() {
		if s.handleLine(scanner.Text()) {
			break
		}
		fmt.Fprint(out, s.prompt())
	}
	// Said however the session ended -- an explicit exit, or the
	// input running out, which is what a piped script does.
	fmt.Fprintf(out, "\n%s\n", consoleFarewell)
}

// handleLine processes one line of REPL input and reports whether the
// session should end.
func (s *session) handleLine(line string) (exit bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	fields := strings.Fields(line)
	switch fields[0] {
	case "exit", "quit":
		return true
	case "help", "?":
		s.printHelp()
		return false
	case "list":
		for _, h := range s.hosts {
			fmt.Fprintln(s.out, h)
		}
		return false
	case "cd":
		// Bare "cd" goes back to everything, which real spells "*"
		// rather than "all" -- and the prompt then shows "*".
		target := "*"
		if len(fields) > 1 {
			target = strings.Join(fields[1:], " ")
		}
		if err := s.setPattern(target); err != nil {
			fmt.Fprintln(s.out, "[ERROR]:", err)
		}
		return false
	case "become":
		// Real refuses a bare "become": the prompt already shows the
		// state, so a command that silently toggled it would be
		// ambiguous.
		if len(fields) < 2 {
			fmt.Fprintln(s.out, "Please specify become value, e.g. `become yes`")
			return false
		}
		s.become = truthy(fields[1])
		return false
	case "nobecome":
		s.become = false
		return false
	case "forks":
		if len(fields) < 2 {
			fmt.Fprintln(s.out, "Please specify a fork value, e.g. `forks 5`")
			return false
		}
		n, err := strconv.Atoi(fields[1])
		if err != nil || n < 0 {
			fmt.Fprintln(s.out, "Please specify a fork value, e.g. `forks 5`")
			return false
		}
		s.forks = n
		return false
	case "remote_user":
		if len(fields) < 2 {
			fmt.Fprintln(s.out, "Please specify a remote user, e.g. `remote_user root`")
			return false
		}
		s.remoteUser = fields[1]
		return false
	case "verbosity":
		if len(fields) < 2 {
			fmt.Fprintln(s.out, "Please specify a verbosity level, e.g. `verbosity 3`")
			return false
		}
		n, err := strconv.Atoi(fields[1])
		if err != nil || n < 0 {
			fmt.Fprintln(s.out, "Please specify a verbosity level, e.g. `verbosity 3`")
			return false
		}
		s.verbosity = n
		fmt.Fprintf(s.out, "verbosity level set to %d\n", n)
		return false
	}

	if len(s.hosts) == 0 {
		fmt.Fprintf(s.out, "ansible-console: pattern %q matches no hosts\n", s.pattern)
		return false
	}

	// Real ansible-console: a line whose first word is a known module
	// name runs that module with the rest of the line as its args;
	// anything else is a bare shell command.
	moduleName := "shell"
	argsStr := line
	if _, ok := modules.Default().Get(fields[0]); ok {
		moduleName = fields[0]
		argsStr = strings.TrimSpace(strings.TrimPrefix(line, fields[0]))
	}
	args := adhoc.ParseModuleArgs(moduleName, argsStr)
	adhoc.Run(context.Background(), s.inv, s.hosts, moduleName, args, s.become, s.out)
	return false
}

func (s *session) printHelp() {
	fmt.Fprintln(s.out, `Commands:
  cd PATTERN     change the target host pattern (default: all)
  list           list hosts currently matched by the pattern
  become         enable privilege escalation for subsequent commands
  nobecome       disable it again
  help, ?        this message
  exit, quit     leave

Anything else runs as a module: a line starting with a registered
module name ("copy dest=/tmp/x content=hi") runs that module with the
rest of the line as its arguments; any other line runs as a shell
command against every matched host.`)
}

// truthy reads the values real accepts for become. It is deliberately
// narrow: anything that is not plainly affirmative turns become OFF,
// so a typo cannot leave a session silently escalated.
func truthy(v string) bool {
	switch strings.ToLower(v) {
	case "yes", "true", "on", "1", "y":
		return true
	}
	return false
}
