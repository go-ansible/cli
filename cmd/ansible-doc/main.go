// Command ansible-doc prints documentation for go-ansible's modules.
//
// Scope note: real ansible-doc reads a structured DOCUMENTATION YAML
// block every module carries (synopsis, per-argument type/required/
// default/choices, examples, return values) and can also document
// plugins beyond modules (lookups, filters, callbacks — none of which
// this port has a registry for). This port's modules were never
// written with that structured metadata; what they do have, on every
// one of them, is a Go doc comment documenting arguments and every
// deviation from real Ansible's behavior — this project's convention
// throughout. ansible-doc here prints that comment verbatim rather
// than a synthesized DOCUMENTATION-shaped block: real content, just
// not real Ansible's exact rendering.
package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/go-ansible/cli/internal/version"
	"github.com/go-ansible/modules"

	"github.com/go-ansible/cli/internal/usage"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	// -h/--help is a SUCCESSFUL request: real answers it on stdout with
	// status 0, where a usage ERROR goes to stderr with status 2.
	if usage.Wanted(args) {
		usage.Answer(usageText)
		return 0
	}
	if len(args) > 0 && (args[0] == "--version" || args[0] == "-version") {
		fmt.Println(version.String("ansible-doc"))
		return 0
	}

	var list bool
	var names []string
	for _, a := range args {
		switch a {
		case "-l", "--list":
			list = true
		default:
			if len(a) > 0 && a[0] == '-' {
				fmt.Fprintln(os.Stderr, "ansible-doc: unrecognized argument:", a)
				usageText(os.Stderr)
				return 2
			}
			names = append(names, a)
		}
	}

	r := modules.Default()

	if list {
		if len(names) > 0 {
			// -l reads a positional as a COLLECTION to list, not a
			// module, and refuses one that is not namespace.name --
			// measured: `ansible-doc -l debug` gives this sentence and
			// exit 1, where this port gave its own message and exit 2.
			for _, n := range names {
				if !strings.Contains(n, ".") {
					fmt.Fprintf(os.Stderr, "[ERROR]: Invalid collection name (must be of the form namespace.collection): %s\n", n)
					return 1
				}
			}
			// A well-formed collection name this port has nothing for
			// lists nothing, which is what real does for a collection
			// it cannot find.
			return 0
		}
		printList(r)
		return 0
	}

	if len(names) == 0 {
		usageText(os.Stderr)
		return 2
	}

	// A name it cannot find is a WARNING, not an error, and the docs of
	// the names it DID find are still printed -- measured:
	//
	//	$ ansible-doc debug no_such_module_at_all
	//	[WARNING]: no_such_module_at_all was not found
	//	> MODULE ansible.builtin.debug (...)
	//	[rc=0]
	//
	// This port printed its own sentence and exited 1, so someone
	// asking for three modules and mistyping one got a failure exit
	// for a command that had answered two thirds of the question --
	// and real does not fail there.
	//
	// Real emits the warning ahead of the docs and this collects the
	// misses first, which matches. That ORDERING is not asserted
	// anywhere, and cannot be: the warning is on stderr and the docs
	// on stdout, the tests capture the two separately, and the
	// binaries harness refuses to compare them merged (merging would
	// compare the reference's buffering). A neuter deferring the
	// warnings to the end PASSES. Said here rather than left to look
	// covered.
	var found []string
	for _, name := range names {
		if _, ok := r.Get(name); !ok {
			fmt.Fprintf(os.Stderr, "[WARNING]: %s was not found\n", name)
			continue
		}
		found = append(found, name)
	}
	for i, name := range found {
		if i > 0 {
			fmt.Println()
		}
		printModule(name)
	}
	return 0
}

func usageText(w io.Writer) {
	fmt.Fprintln(w, "usage: ansible-doc [-l | --list] [MODULE ...]")
}

// Real's own column layout, measured from ansible-doc -l: the name
// field is 59 characters wide and the whole line is 78, so the summary
// gets 19 and is truncated with an ellipsis when longer.
const (
	listNameWidth = 59
	listLineWidth = 78
	// The description block under the header wraps at 70 including its
	// two-space indent -- also measured, across six modules.
	bodyWidth  = 70
	bodyIndent = "  "
)

// fqcn qualifies a module name the way real prints it. Every module this
// port implements is one of ansible-core's own, so the namespace is
// always ansible.builtin.
func fqcn(name string) string {
	if strings.Contains(name, ".") {
		return name
	}
	return "ansible.builtin." + name
}

func printList(r *modules.Registry) {
	names := r.Names()
	sort.Strings(names)
	for _, n := range names {
		fmt.Printf("%-*s%s\n", listNameWidth, fqcn(n), truncate(listSynopsis(modules.Docs[n]), listLineWidth-listNameWidth))
	}
}

// truncate cuts to width, ending in an ellipsis when it had to cut --
// which is what real does rather than letting the line run on.
func truncate(s string, width int) string {
	if len(s) <= width {
		return s
	}
	if width <= 3 {
		return s[:width]
	}
	return s[:width-3] + "..."
}

func printModule(name string) {
	// Real's header names the module by its FQCN, and then its source
	// file in parentheses. There is no source file here -- the modules
	// are compiled in -- so the parenthetical is left off rather than
	// filled with something that is not a path.
	fmt.Printf("> MODULE %s\n\n", fqcn(name))
	for _, line := range wrap(synopsis(modules.Docs[name]), bodyWidth-len(bodyIndent)) {
		if line == "" {
			// A paragraph break is an EMPTY line, not an indented one:
			// indenting it left trailing whitespace on every gap.
			fmt.Println()
			continue
		}
		fmt.Println(bodyIndent + line)
	}
}

// wrap breaks text into lines of at most width, keeping the blank lines
// that separate paragraphs.
func wrap(text string, width int) []string {
	var out []string
	for _, para := range strings.Split(text, "\n\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			continue
		}
		if len(out) > 0 {
			out = append(out, "")
		}
		line := words[0]
		for _, w := range words[1:] {
			if len(line)+1+len(w) > width {
				out = append(out, line)
				line = w
				continue
			}
			line += " " + w
		}
		out = append(out, line)
	}
	return out
}

// synopsis is the documentation with this port's own Go identifier
// removed from the front.
//
// Every one of these doc comments opens "module<Name> implements ...",
// because that is Go's convention for a doc comment: it names the
// thing it documents. Printing it verbatim put a Go function name in
// front of every entry of `ansible-doc -l` and at the top of every
// module's page -- 565 of the 566 -- so a person reading the
// documentation read this port's internals.
//
// ONLY that prefix is removed. The rest of the sentence usually
// continues "(a subset of) Ansible's `x` module: ...", which is
// accurate and worth keeping. Trying to strip that too needs a guess
// about where the description starts, and the guess is wrong in a way
// that matters: a regex reaching for the colon after "module" crossed
// a paragraph break on apt and returned its ARGUMENT LIST as the
// description.
func synopsis(doc string) string {
	doc = strings.TrimSpace(doc)
	if rest, ok := stripGoIdentifier(doc); ok {
		doc = rest
	}
	return doc
}

// stripGoIdentifier removes a leading "moduleXxx implements " and
// reports whether it found one.
func stripGoIdentifier(doc string) (string, bool) {
	const verb = " implements "
	if !strings.HasPrefix(doc, "module") {
		return doc, false
	}
	i := strings.Index(doc, verb)
	if i < 0 {
		return doc, false
	}
	// Everything before the verb must be one identifier, or this is a
	// sentence that merely happens to start with the word "module".
	for _, r := range doc[:i] {
		if !(r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return doc, false
		}
	}
	return strings.TrimSpace(doc[i+len(verb):]), true
}

// listSynopsis is the one-line summary for the -l table.
//
// It strips more than synopsis does, and has to: the table gives a
// summary 19 columns, and what follows the Go identifier is usually the
// boilerplate "(a subset of) Ansible's `x` module: ". Truncated to 19
// that reads "(a subset of) An..." for every module in the list, which
// is less use than nothing.
//
// The stripping is confined to the FIRST PARAGRAPH, which is why it is
// safe here where a whole-document regex was not: reaching for the colon
// after "module" across the whole text crossed a paragraph break on apt
// and returned its argument list as the description. A paragraph cannot
// be crossed if it is all there is.
func listSynopsis(doc string) string {
	para, _, _ := strings.Cut(strings.TrimSpace(doc), "\n\n")
	line := strings.Join(strings.Fields(synopsis(para)), " ")

	// "Ansible's `x` module[ (collection)][ qualifier]: description"
	if rest, ok := afterBoilerplate(line); ok {
		return rest
	}
	return line
}

// afterBoilerplate removes the "[hedge ]Ansible's `x`[ (collection)]
// module[ qualifier][:]" opening and returns what follows.
//
// The pieces are skipped one at a time rather than matched as a whole,
// because every one of them varies: the hedge is "(a subset of)" or "(a
// best-effort approximation of)" or absent, the collection parenthetical
// may sit between the name and the word module, and the description may
// be introduced by a colon or simply continue ("module for Debian/Ubuntu
// package management"). A single pattern missed 279 of the 566.
//
// If what is left is empty or mere punctuation, the opening was the
// WHOLE sentence -- some modules say no more than which module they
// implement -- and the caller keeps the line as it was. Returning "."
// for those, which an earlier version did, replaced a weak summary with
// no summary.
func afterBoilerplate(line string) (string, bool) {
	i := strings.Index(line, "Ansible's `")
	if i < 0 {
		return line, false
	}
	rest := line[i+len("Ansible's `"):]

	// The quoted module name.
	j := strings.Index(rest, "`")
	if j < 0 {
		return line, false
	}
	rest = strings.TrimSpace(rest[j+1:])

	// An optional parenthetical naming the collection.
	if strings.HasPrefix(rest, "(") {
		k := strings.Index(rest, ")")
		if k < 0 {
			return line, false
		}
		rest = strings.TrimSpace(rest[k+1:])
	}

	// Then the word itself.
	if !strings.HasPrefix(rest, "module") {
		return line, false
	}
	rest = strings.TrimSpace(strings.TrimPrefix(rest, "module"))

	// A colon introduces the description when there is one; otherwise
	// whatever follows IS the description.
	if idx := strings.Index(rest, ":"); idx >= 0 && idx < 40 {
		rest = strings.TrimSpace(rest[idx+1:])
	}
	if strings.Trim(rest, " .,;:-") == "" {
		return line, false
	}
	return rest, true
}
