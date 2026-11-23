// SPDX-License-Identifier: Apache-2.0

package sessionreport

import (
	"path"
	"regexp"
	"strings"
)

// Categories, in the order they are reported. Each rule that assigns one is in
// categorize / segmentCategory, and docs-site/src/tools/session-report.md
// states them in prose.
const (
	CatOrientation = "orientation" // reading skills/docs/source, syntax/help, describe/show/list
	CatWrite       = "write"       // writing or editing .mdl and other source
	CatValidate    = "validate"    // check, lint, docker check, a build
	CatApply       = "apply"       // exec, or a -c statement that changes the model
	CatVerify      = "verify"      // test, run, playwright, screenshot
	CatDiagnosis   = "diagnosis"   // logs, grepping through output
	CatRetry       = "retry"       // a call in an error→retry chain (see chains.go)
	CatDelegate    = "delegate"    // handing work to a subagent
	CatOther       = "other"       // git, setup, anything unmatched
)

// CategoryOrder is the reporting order.
var CategoryOrder = []string{CatOrientation, CatWrite, CatValidate, CatApply, CatVerify, CatDiagnosis, CatRetry, CatDelegate, CatOther}

// Bash buckets: what kind of program a Bash call ran.
const (
	BucketMxcli      = "mxcli"
	BucketPlaywright = "playwright"
	BucketGit        = "git"
	BucketBuild      = "build"
	BucketOther      = "other"
)

// VerbFunc resolves an mxcli argv (argv[0] is "mxcli") to its command path,
// e.g. "exec" or "docker check". It returns "" when the argv names no
// subcommand. The CLI passes one backed by cobra so the verb list maintains
// itself; DefaultVerb is the fallback used in tests.
type VerbFunc func(args []string) string

// valuedRootFlags are mxcli's global flags that take a value; DefaultVerb
// skips their argument when looking for the subcommand.
var valuedRootFlags = map[string]bool{"-p": true, "--project": true, "-c": true, "--command": true, "--mdl": true}

// DefaultVerb takes the first one or two non-flag words, "docker"/"diag"/
// "playwright"/... being the groups that have subcommands.
func DefaultVerb(args []string) string {
	var words []string
	for i := 1; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			if valuedRootFlags[a] {
				i++
			}
			continue
		}
		words = append(words, a)
		if len(words) == 2 {
			break
		}
	}
	if len(words) == 0 {
		return ""
	}
	switch words[0] {
	case "docker", "diag", "playwright", "setup", "eval", "theme", "catalog", "tunnel-hub":
		if len(words) > 1 && !strings.Contains(words[1], ".") && !strings.Contains(words[1], "/") {
			return words[0] + " " + words[1]
		}
	}
	return words[0]
}

// Segment is one simple command inside a Bash call.
type Segment struct {
	Program string   // basename of the program run
	Args    []string // argv, Args[0] is the program as written
	Raw     string
	Bucket  string
	Verb    string // mxcli: the command path, or "-c <statement keyword>"
	Scripts []string
}

// shellOps split a command line into simple commands. Quoted text is kept
// intact, and a heredoc body is skipped, so `cat > x.mdl <<'EOF' … EOF` is
// one segment and its body is never mistaken for commands.
func splitShell(cmd string) []string {
	var segs []string
	var cur strings.Builder
	var quote rune
	lines := strings.Split(cmd, "\n")
	var heredoc []string
	for li := 0; li < len(lines); li++ {
		line := lines[li]
		if len(heredoc) > 0 {
			if strings.TrimSpace(line) == heredoc[0] {
				heredoc = heredoc[1:]
			}
			continue
		}
		rs := []rune(line)
		for i := 0; i < len(rs); i++ {
			r := rs[i]
			if quote != 0 {
				cur.WriteRune(r)
				if r == '\\' && quote == '"' && i+1 < len(rs) {
					i++
					cur.WriteRune(rs[i])
				} else if r == quote {
					quote = 0
				}
				continue
			}
			switch r {
			case '\'', '"':
				quote = r
				cur.WriteRune(r)
			case '\\':
				cur.WriteRune(r)
				if i+1 < len(rs) {
					i++
					cur.WriteRune(rs[i])
				}
			case '#':
				if cur.Len() == 0 || strings.HasSuffix(cur.String(), " ") {
					i = len(rs) // comment to end of line
				} else {
					cur.WriteRune(r)
				}
			case ';', '|', '&':
				// "2>&1" and ">&" are redirections, not separators.
				if r == '&' && i > 0 && rs[i-1] == '>' {
					cur.WriteRune(r)
					continue
				}
				if r == '&' && i+1 < len(rs) && rs[i+1] == '>' {
					cur.WriteRune(r)
					continue
				}
				segs = append(segs, cur.String())
				cur.Reset()
				if i+1 < len(rs) && (rs[i+1] == r) {
					i++
				}
			case '<':
				if i+1 < len(rs) && rs[i+1] == '<' && (i+2 >= len(rs) || rs[i+2] != '<') {
					// heredoc marker: <<WORD, <<-WORD, <<'WORD', <<"WORD"
					j := i + 2
					if j < len(rs) && rs[j] == '-' {
						j++
					}
					for j < len(rs) && rs[j] == ' ' {
						j++
					}
					k := j
					for k < len(rs) && !strings.ContainsRune(" ;|&)<>", rs[k]) {
						k++
					}
					word := strings.Trim(string(rs[j:k]), `'"`)
					if word != "" {
						heredoc = append(heredoc, word)
					}
					cur.WriteString(string(rs[i:k]))
					i = k - 1
					continue
				}
				cur.WriteRune(r)
			default:
				cur.WriteRune(r)
			}
		}
		if quote == 0 {
			segs = append(segs, cur.String())
			cur.Reset()
		} else {
			cur.WriteRune('\n')
		}
	}
	if cur.Len() > 0 {
		segs = append(segs, cur.String())
	}
	var out []string
	for _, s := range segs {
		s = strings.TrimSpace(s)
		s = strings.TrimLeft(s, "({ ")
		s = strings.TrimRight(s, ")} ")
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// shellFields splits a simple command into words, removing quotes.
func shellFields(s string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	inWord := false
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			inWord = true
		case r == ' ' || r == '\t' || r == '\n':
			if inWord {
				out = append(out, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if inWord {
		out = append(out, cur.String())
	}
	return out
}

var envAssign = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// wrappers run the command that follows them.
var wrappers = map[string]bool{"env": true, "time": true, "nohup": true, "sudo": true, "exec": true, "command": true, "xargs": true}

// ParseBash splits a Bash command into its simple commands and classifies each.
func ParseBash(cmd string, verb VerbFunc) []Segment {
	if verb == nil {
		verb = DefaultVerb
	}
	var out []Segment
	for _, raw := range splitShell(cmd) {
		f := shellFields(raw)
		for len(f) > 0 {
			p := path.Base(f[0])
			switch {
			case envAssign.MatchString(f[0]):
				f = f[1:]
				continue
			case wrappers[p]:
				f = f[1:]
				continue
			case p == "timeout" && len(f) > 1:
				f = f[2:]
				continue
			}
			break
		}
		if len(f) == 0 {
			continue
		}
		seg := Segment{Program: path.Base(f[0]), Args: f, Raw: raw}
		classifySegment(&seg, verb)
		out = append(out, seg)
	}
	return out
}

func classifySegment(s *Segment, verb VerbFunc) {
	p := s.Program
	switch {
	case p == "mxcli":
		s.Bucket = BucketMxcli
		args := append([]string{"mxcli"}, s.Args[1:]...)
		s.Verb = verb(args)
		if s.Verb == "" {
			s.Verb = oneShotVerb(args)
		}
		for _, a := range args[1:] {
			if strings.HasSuffix(a, ".mdl") || strings.HasSuffix(a, ".test.md") {
				s.Scripts = append(s.Scripts, a)
			}
		}
	case strings.Contains(p, "playwright") || strings.Contains(p, "screenshot") ||
		((p == "npx" || p == "bunx" || p == "pnpx") && len(s.Args) > 1 && strings.Contains(s.Args[1], "playwright")):
		s.Bucket = BucketPlaywright
	case p == "git" || p == "gh":
		s.Bucket = BucketGit
	case p == "make" || p == "go" || p == "npm" || p == "npx" || p == "bun" || p == "yarn" ||
		p == "mvn" || p == "gradle" || p == "mx" || p == "mxbuild" ||
		(p == "docker" && len(s.Args) > 1 && s.Args[1] == "build"):
		s.Bucket = BucketBuild
	default:
		s.Bucket = BucketOther
	}
}

// oneShotVerb names a `mxcli -c "<statement>"` (or a REPL) by the
// statement's first keyword, so `-c describe` and `-c create` are told apart.
func oneShotVerb(args []string) string {
	for i := 1; i < len(args); i++ {
		if (args[i] == "-c" || args[i] == "--command") && i+1 < len(args) {
			w := strings.Fields(strings.ToLower(args[i+1]))
			if len(w) == 0 {
				return "-c"
			}
			kw := strings.TrimSuffix(w[0], ";")
			if kw == "mdl" && len(w) > 2 { // "mdl 1; describe …"
				kw = strings.TrimSuffix(w[2], ";")
			}
			return "-c " + kw
		}
	}
	return "REPL"
}

// mxcliVerbCategory maps an mxcli verb to a category.
func mxcliVerbCategory(v string) string {
	first := strings.Fields(v)
	if len(first) == 0 {
		return CatOther
	}
	if first[0] == "-c" {
		if len(first) < 2 {
			return CatOrientation
		}
		switch first[1] {
		case "create", "alter", "drop", "grant", "revoke", "move", "rename", "update", "insert", "delete", "set", "import", "generate":
			return CatApply
		default: // describe, show, list, select, …
			return CatOrientation
		}
	}
	switch v {
	case "exec":
		return CatApply
	case "fmt":
		return CatWrite
	case "check", "lint", "docker check", "docker build", "build", "report", "diff":
		return CatValidate
	case "test", "run", "docker run", "screenshot", "oql", "eval check":
		return CatVerify
	case "diag", "logs", "diag loop-report", "diag session-report":
		return CatDiagnosis
	case "new", "init", "setup", "setup mxbuild", "setup mxcli", "version", "REPL":
		return CatOther
	}
	switch first[0] {
	case "playwright":
		return CatVerify
	case "docker":
		return CatVerify
	case "syntax", "help", "describe", "show", "list", "search", "context", "refs", "callers", "callees", "impact", "catalog", "select", "structure", "tree":
		return CatOrientation
	case "diag":
		return CatDiagnosis
	}
	return CatOrientation
}

var redirectTarget = regexp.MustCompile(`(?:^|[^0-9&>])>>?\s*([^\s&|;>]+)`)

var logArg = regexp.MustCompile(`\.log\b|/logs?/|\.output\b|journalctl`)

var quietPrograms = map[string]bool{"cd": true, "echo": true, "export": true, "true": true, "sleep": true, "pwd": true, "set": true, "source": true, ".": true, "printf": true, "mkdir": true, "date": true, "wait": true}

var filterPrograms = map[string]bool{"grep": true, "rg": true, "egrep": true, "tail": true, "head": true, "awk": true, "sed": true, "jq": true, "wc": true, "sort": true, "uniq": true, "cut": true, "less": true, "cat": true, "tr": true, "column": true}

// segmentCategory classifies one simple command. ok is false for commands that
// carry no intent of their own (cd, echo, export, …).
func segmentCategory(s Segment) (cat string, ok bool) {
	if s.Bucket == BucketMxcli {
		return mxcliVerbCategory(s.Verb), true
	}
	// A redirect into a file, tee, or sed -i writes source.
	if m := redirectTarget.FindStringSubmatch(s.Raw); m != nil && m[1] != "/dev/null" && !strings.HasPrefix(m[1], "&") {
		return CatWrite, true
	}
	if s.Program == "tee" || (s.Program == "sed" && hasArg(s.Args, "-i")) || s.Program == "cp" || s.Program == "mv" {
		return CatWrite, true
	}
	if quietPrograms[s.Program] {
		return "", false
	}
	switch s.Bucket {
	case BucketPlaywright:
		return CatVerify, true
	case BucketBuild:
		if hasArg(s.Args, "test") {
			return CatVerify, true
		}
		return CatValidate, true
	case BucketGit:
		return CatOther, true
	}
	if s.Program == "docker" && len(s.Args) > 1 && s.Args[1] == "logs" {
		return CatDiagnosis, true
	}
	if s.Program == "curl" || s.Program == "wget" {
		return CatVerify, true
	}
	if filterPrograms[s.Program] || s.Program == "ls" || s.Program == "find" || s.Program == "tree" || s.Program == "fd" {
		if logArg.MatchString(s.Raw) {
			return CatDiagnosis, true
		}
		return CatOrientation, true
	}
	return CatOther, true
}

func hasArg(args []string, a string) bool {
	for _, x := range args[1:] {
		if x == a || strings.HasPrefix(x, a+"=") {
			return true
		}
	}
	return false
}

// categoryRank decides which segment names a chained Bash call:
// `exec x.mdl && docker check` is an apply, `cat log | grep` a diagnosis.
var categoryRank = map[string]int{CatApply: 7, CatVerify: 6, CatValidate: 5, CatWrite: 4, CatDiagnosis: 3, CatOrientation: 2, CatOther: 1}

// bucketRank decides which program names a chained Bash call in the
// breakdown. An mxcli segment always wins: the breakdown exists to show them.
var bucketRank = map[string]int{BucketMxcli: 5, BucketPlaywright: 4, BucketBuild: 3, BucketGit: 2, BucketOther: 1}

// categorize assigns a call its category (before retry chains, which
// override it). It also fills in the Bash segments for the breakdown.
func categorize(c *Call, verb VerbFunc) (string, []Segment) {
	tool := c.Tool
	lt := strings.ToLower(tool)
	switch {
	case tool == "Bash":
		segs := ParseBash(c.Command, verb)
		best, bestRank := CatOther, 0
		for _, s := range segs {
			cat, ok := segmentCategory(s)
			if !ok {
				continue
			}
			if r := categoryRank[cat]; r > bestRank {
				best, bestRank = cat, r
			}
		}
		return best, segs
	case tool == "Edit" || tool == "Write" || tool == "MultiEdit" || tool == "NotebookEdit":
		return CatWrite, nil
	case tool == "Read":
		if logArg.MatchString(c.Path) {
			return CatDiagnosis, nil
		}
		return CatOrientation, nil
	case tool == "Grep" || tool == "Glob" || tool == "LS":
		if logArg.MatchString(c.Path) {
			return CatDiagnosis, nil
		}
		return CatOrientation, nil
	case tool == "Skill" || tool == "ToolSearch" || tool == "WebFetch" || tool == "WebSearch":
		return CatOrientation, nil
	case tool == "Agent" || tool == "Task":
		return CatDelegate, nil
	case strings.Contains(lt, "playwright") || strings.Contains(lt, "browser") ||
		strings.Contains(lt, "screenshot") || strings.Contains(lt, "chrome"):
		return CatVerify, nil
	}
	return CatOther, nil
}

// --- failure detection -----------------------------------------------------

// errorLine matches a line that reports a failure on its own, for the case the
// exit code is masked (`mxcli check x.mdl; echo done`, `| tail`). Anchored at
// the start of a line so a grep hit ("file.go:12: Error:") does not count.
var errorLine = regexp.MustCompile(`^\s*(?:Error|ERROR|Parse error|Reference error|Validation error|FAILED|FAIL)\b[: ]|^\s*- line \d+:\d+ |^\s*\[error\]|^panic: |^Exit code [1-9]`)

var exitCodeLine = regexp.MustCompile(`^Exit code \d+$`)

// detectFailure decides whether a result is a failure, and picks the line that
// says why. A result is a failure when the harness marked it is_error (for
// Bash: a non-zero exit), or — for Bash only — when a line of the output
// starts like an mxcli or test-runner error. The line rule is for a command
// whose exit code a pipe hides; a call that only reads (every segment is
// orientation: cat, sed -n, grep, mxcli syntax/help, …) hides nothing, and
// what it prints is the file's text — a skill quoting an error is not one
// (ako/mxcli#2525).
func detectFailure(tool, command string, isError bool, text string) (bool, string) {
	failed := isError
	if !failed && tool == "Bash" && !onlyReads(command) {
		for _, l := range strings.Split(text, "\n") {
			if errorLine.MatchString(l) {
				failed = true
				break
			}
		}
	}
	if !failed {
		return false, ""
	}
	return true, firstErrorLine(text)
}

func firstErrorLine(text string) string {
	text = strings.ReplaceAll(text, "<tool_use_error>", "")
	text = strings.ReplaceAll(text, "</tool_use_error>", "")
	var first, exit string
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(stripANSI(l))
		if l == "" || strings.HasPrefix(l, "WARNING: This is a vibe-coded") {
			continue
		}
		if exitCodeLine.MatchString(l) {
			exit = l
			continue
		}
		if errorLine.MatchString(l) {
			return truncate(scrub(l), 100)
		}
		if first == "" {
			first = l
		}
	}
	if first == "" {
		first = exit
	}
	return truncate(scrub(first), 100)
}

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func stripANSI(s string) string { return ansi.ReplaceAllString(s, "") }

// --- snippets and normalisation --------------------------------------------

var (
	absPath   = regexp.MustCompile(`(?:/[^\s/'"]+){3,}`)
	secretish = regexp.MustCompile(`[A-Za-z0-9_\-+/=]{32,}`)
	digits    = regexp.MustCompile(`\d+`)
	hexRun    = regexp.MustCompile(`[0-9a-fA-F]{8,}`)
	spaces    = regexp.MustCompile(`\s+`)
)

// shortPath keeps the last two path elements: enough to recognise a file,
// and it keeps home directories and project roots out of a shared report.
func shortPath(p string) string {
	p = strings.TrimRight(p, "/")
	parts := strings.Split(p, "/")
	if len(parts) <= 2 {
		return p
	}
	return "…/" + strings.Join(parts[len(parts)-2:], "/")
}

// scrub shortens absolute paths and drops anything token-shaped.
func scrub(s string) string {
	s = absPath.ReplaceAllStringFunc(s, shortPath)
	s = secretish.ReplaceAllString(s, "…")
	return s
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// Snippet is the short, shareable description of a call.
func Snippet(c *Call) string {
	switch c.Tool {
	case "Bash":
		line := strings.TrimSpace(c.Command)
		segs := splitShell(line)
		// The mxcli command is what the report is about, when there is one.
		for _, s := range segs {
			if f := shellFields(s); len(f) > 0 && path.Base(f[0]) == "mxcli" {
				segs = []string{s}
				break
			}
		}
		// Skip a leading `cd dir &&` / `export X=…;` — it says nothing.
		for _, s := range segs {
			f := shellFields(s)
			if len(f) > 0 && (quietPrograms[path.Base(f[0])] || (len(f) == 1 && envAssign.MatchString(f[0]))) {
				continue
			}
			line = s
			break
		}
		if i := strings.IndexByte(line, '\n'); i >= 0 {
			line = line[:i] + " …"
		}
		return truncate(scrub(spaces.ReplaceAllString(line, " ")), 64)
	case "Read", "Edit", "Write", "MultiEdit", "NotebookEdit":
		return shortPath(c.Path)
	case "Grep", "Glob":
		s := truncate(c.Pattern, 30)
		if c.Path != "" {
			s += " in " + shortPath(c.Path)
		}
		return scrub(s)
	case "Skill":
		return c.Name
	case "Agent", "Task":
		return truncate(c.Name, 48)
	}
	return ""
}

// NormalizeError folds an error line into a family: numbers become N, paths
// their last element, quoted names stay (they are usually the token at fault).
func NormalizeError(s string) string {
	s = absPath.ReplaceAllStringFunc(s, func(p string) string { return path.Base(p) })
	s = hexRun.ReplaceAllString(s, "#")
	s = digits.ReplaceAllString(s, "N")
	s = spaces.ReplaceAllString(s, " ")
	return truncate(strings.TrimSpace(s), 90)
}

// onlyReads reports whether every segment of a Bash command is orientation.
// An empty command is not known to read.
func onlyReads(command string) bool {
	segs := ParseBash(command, nil)
	if len(segs) == 0 {
		return false
	}
	for _, s := range segs {
		if cat, ok := segmentCategory(s); !ok || cat != CatOrientation {
			return false
		}
	}
	return true
}
