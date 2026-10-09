// SPDX-License-Identifier: Apache-2.0

// Package testrunner implements the MDL test framework for executing and
// validating microflow tests against a running Mendix runtime.
package testrunner

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/srctext"
)

// TestCase represents a single test extracted from a test file.
type TestCase struct {
	ID      string   // Generated test ID (test_1, test_2, ...)
	Name    string   // From @test annotation
	MDL     string   // Raw MDL statements for this test block
	Expects []Expect // @expect assertions
	// AssertionErrors holds one message per annotation that claims to assert
	// something and cannot. A test carrying any of these is reported as an ERROR
	// and never run: an assertion that cannot be evaluated must not be able to
	// report a pass.
	AssertionErrors []string
	Verify          []Verify // @verify OQL post-conditions
	// Setups are the microflows called before the test's own statements, in
	// order: the file header's first, then the test's own. See writeSetupCalls.
	Setups  []string
	Cleanup string // @cleanup strategy ("rollback" or "none")
	// ExpectsThrow records that @throws was written, which Throws cannot: a bare
	// @throws carries no message, and "" is also what an absent annotation
	// leaves behind. Keying the generators on Throws != "" therefore dropped the
	// bare form silently, and the test then failed on the very exception it was
	// written to expect (ako/mxcli#301).
	ExpectsThrow bool
	Throws       string // @throws expected error message, "" when written bare
	SourceFile   string // Original file path
	Line         int    // Line number in source file
	// BodyLine is the 1-based source line the MDL body starts on, which is not
	// Line: that one points at the doc comment. Checking a test file needs the
	// body's own line, so a diagnostic can be reported where the author wrote the
	// statement rather than where the annotation is — see check_source.go.
	BodyLine int
	// Version is the MDL language version the body is written in: the
	// `mdl <n>;` header its file starts with (for a markdown file, its
	// ```mdl-test block), and mdl 0 without one, as for any script
	// (ADR-0011). It decides what the body means, so the generators and
	// check write it at the head of the script they make (ako/mxcli#847).
	Version langver.Version
	// HeaderLine is the 1-based source line of that header, 0 when there is
	// none. check renders the header there, keeping every other line where
	// the author wrote it.
	HeaderLine int
}

// expectsThrow reports whether the test expects its body to raise an error.
//
// Two fields can say so, and either alone is enough: ExpectsThrow is what the
// parser sets, and a non-empty Throws implies it — there is no test that names
// an expected message and does not expect a throw. Reading them through one
// accessor is what keeps a caller that sets only the message (every construction
// site that predates the bare form) from silently becoming a non-throws test.
func (tc TestCase) expectsThrow() bool {
	return tc.ExpectsThrow || tc.Throws != ""
}

// AssertionCount reports how many assertions the test actually makes.
//
// @expect, @verify and @throws each assert something a runner evaluates. An
// annotation that is parsed and never executed must not be counted here — that
// is what made @verify look like an assertion while asserting nothing.
func (tc TestCase) AssertionCount() int {
	n := len(tc.Expects) + len(tc.Verify)
	if tc.expectsThrow() {
		n++
	}
	return n
}

// TestSuite represents a collection of tests from one or more files.
type TestSuite struct {
	Name  string     // Suite name (derived from file name)
	Tests []TestCase // Test cases
	// FileErrors holds one entry per test file that could not be parsed.
	//
	// A malformed file used to abort the whole run, so an unrelated bad file
	// stopped every other test in the directory from being listed or executed
	// (#903). It is carried here instead and reported as an ERROR result, which
	// is the same fail-closed treatment an uncompilable @expect gets: the run
	// stays red and names the file, and the tests that parse still run.
	FileErrors []FileError
	// FilesRead counts the test files actually opened, which is NOT the number of
	// paths on the command line: a directory is one path and contributes as many
	// files as it holds test-named entries — or none. Reporting the path count
	// made `mxcli test <dir>` say "0 test(s) in 1 file(s)" for a directory nothing
	// had been read from (ako/mxcli-maintenance §5).
	FilesRead int
}

// FileError is a test file that could not be parsed, and why.
type FileError struct {
	Path string
	Err  error
}

// ParseTestFile parses a test file (.test.mdl or .test.md) and extracts test cases.
func ParseTestFile(path string) (*TestSuite, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading test file: %w", err)
	}
	// A BOM or UTF-16 is decoded as for every other script (mendixlabs/mxcli#1253).
	content, err := srctext.Decode(raw)
	if err != nil {
		return nil, fmt.Errorf("reading test file: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	base := filepath.Base(path)

	// Derive suite name from filename
	suiteName := strings.TrimSuffix(base, filepath.Ext(base))
	if strings.HasSuffix(suiteName, ".test") {
		suiteName = strings.TrimSuffix(suiteName, ".test")
	}

	var tests []TestCase
	switch ext {
	case ".md":
		tests, err = parseMarkdownTests(string(content), path)
	default:
		// .mdl or .test.mdl
		tests, err = parseMDLTests(string(content), path)
	}
	if err != nil {
		return nil, err
	}

	return &TestSuite{
		Name:  suiteName,
		Tests: tests,
	}, nil
}

// ParseTestDir parses all test files under a directory, at any depth, in
// lexical path order. A suite per module (tests/Sales/*.test.mdl) is the usual
// layout, so `mxcli test tests` must find what `mxcli test tests/Sales` finds
// (mendixlabs/mxcli#1363). Hidden folders (.git, .mxcli, ...) are not entered.
func ParseTestDir(dir string) (*TestSuite, error) {
	suite := &TestSuite{
		Name: filepath.Base(dir),
	}

	err := walkTestTree(dir, func(path string, d fs.DirEntry) {
		if !isTestFile(d.Name()) {
			return
		}
		suite.FilesRead++
		sub, err := ParseTestFile(path)
		if err != nil {
			suite.FileErrors = append(suite.FileErrors, FileError{Path: path, Err: err})
			return
		}
		suite.Tests = append(suite.Tests, sub.Tests...)
	})
	if err != nil {
		return nil, fmt.Errorf("reading test directory: %w", err)
	}

	return suite, nil
}

// walkTestTree calls fn for every non-directory entry under root, in lexical
// order, without entering hidden (dot-prefixed) folders below root. The root
// itself is always walked, whatever it is called: `mxcli test .` has a root
// named ".". A root that is a symlink is followed (as os.ReadDir did);
// symlinked folders below it are not, which keeps a link cycle from looping.
func walkTestTree(root string, fn func(path string, d fs.DirEntry)) error {
	if info, err := os.Lstat(root); err == nil && info.Mode()&os.ModeSymlink != 0 {
		if resolved, err := filepath.EvalSymlinks(root); err == nil {
			root = resolved
		}
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if path == root {
			return fmt.Errorf("%s is not a directory", root)
		}
		fn(path, d)
		return nil
	})
}

// isTestFile returns true if the filename matches a test file pattern.
func isTestFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".test.mdl") ||
		strings.HasSuffix(lower, ".test.md")
}

// parseMDLTests parses test blocks from a .test.mdl file.
// Each test block is a javadoc comment followed by MDL statements, separated by '/'.
func parseMDLTests(content string, sourcePath string) ([]TestCase, error) {
	version, headerLine, content, err := takeLanguageHeader(content, 1)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", sourcePath, err)
	}
	blocks := splitTestBlocks(content)
	var tests []TestCase

	fileSetups, err := headerSetups(content)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", sourcePath, err)
	}

	for _, block := range blocks {
		// Extract javadoc comment and MDL body
		doc, body, line, bodyLine, err := extractDocAndBody(block)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", sourcePath, err)
		}
		if doc == "" {
			// No javadoc — skip this block (it's not a test)
			continue
		}

		annotations := parseAnnotations(doc)
		if annotations.Test == "" {
			// Has javadoc but no @test — not a test block
			continue
		}

		if err := validateCleanup(annotations.Cleanup); err != nil {
			return nil, fmt.Errorf("%s: test %q: %w", sourcePath, annotations.Test, err)
		}

		// Number the tests, not the chunks: a chunk that holds only a file
		// header is skipped, and numbering by chunk index left a gap where it
		// had been — so the first test in a file could report as test_2.
		testID := fmt.Sprintf("test_%d", len(tests)+1)
		tests = append(tests, TestCase{
			ID:              testID,
			Name:            annotations.Test,
			MDL:             strings.TrimSpace(body),
			Expects:         annotations.Expects,
			AssertionErrors: annotations.AssertionErrors,
			Verify:          annotations.Verify,
			// The file's fixtures run before the test's own: they are the
			// broader precondition, and a test's own setup may build on them.
			Setups:       append(append([]string{}, fileSetups...), annotations.Setups...),
			Cleanup:      annotations.Cleanup,
			ExpectsThrow: annotations.throwsSet,
			Throws:       annotations.Throws,
			SourceFile:   sourcePath,
			Line:         line,
			BodyLine:     bodyLine,
			Version:      version,
			HeaderLine:   headerLine,
		})
	}

	return tests, nil
}

// parseMarkdownTests extracts test blocks from ```mdl-test fenced code blocks.
func parseMarkdownTests(content string, sourcePath string) ([]TestCase, error) {
	var tests []TestCase
	testNum := 0

	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNum := 0
	inCodeBlock := false
	var blockLines []string
	blockStart := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if !inCodeBlock {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```mdl-test") {
				inCodeBlock = true
				blockLines = nil
				blockStart = lineNum
			}
		} else {
			trimmed := strings.TrimSpace(line)
			if trimmed == "```" {
				// End of code block — parse it
				inCodeBlock = false
				blockContent := strings.Join(blockLines, "\n")
				// Each block is a script of its own, so it carries its own
				// header; check refuses a file whose blocks disagree.
				version, headerLine, blockContent, err := takeLanguageHeader(blockContent, blockStart+1)
				if err != nil {
					return nil, fmt.Errorf("%s: test at line %d: %w", sourcePath, blockStart, err)
				}

				// Parse the block as a single test. Its text starts on the
				// line after the fence, which is the line its lines count from.
				doc, body, _, bodyLine, err := extractDocAndBody(testBlock{Text: blockContent, Line: blockStart + 1})
				if err != nil {
					return nil, fmt.Errorf("%s: %w", sourcePath, err)
				}
				annotations := parseAnnotations(doc)

				if err := validateCleanup(annotations.Cleanup); err != nil {
					return nil, fmt.Errorf("%s: test at line %d: %w", sourcePath, blockStart, err)
				}

				testNum++
				testID := fmt.Sprintf("test_%d", testNum)

				name := annotations.Test
				if name == "" {
					name = fmt.Sprintf("test at line %d", blockStart)
				}

				tests = append(tests, TestCase{
					ID:              testID,
					Name:            name,
					MDL:             strings.TrimSpace(body),
					Expects:         annotations.Expects,
					AssertionErrors: annotations.AssertionErrors,
					Verify:          annotations.Verify,
					Setups:          annotations.Setups,
					Cleanup:         annotations.Cleanup,
					ExpectsThrow:    annotations.throwsSet,
					Throws:          annotations.Throws,
					SourceFile:      sourcePath,
					Line:            blockStart,
					BodyLine:        bodyLine,
					Version:         version,
					HeaderLine:      headerLine,
				})
			} else {
				blockLines = append(blockLines, line)
			}
		}
	}

	return tests, nil
}

// takeLanguageHeader reads the `mdl <n>;` header a test file starts with and
// returns the text with the header blanked out — every byte but a newline
// becomes a space — so each line and column after it stays where it was.
// firstLine is the source line text starts on; headerLine is 0 when there is
// no header.
//
// The header is read exactly where a script's is (langver.HeaderSpan): the
// first token after whitespace and `--` / `/* */` comments, so before the first
// `/** … */`. Left in the text, it was the first chunk's body, which made that
// chunk's doc comment no longer leading — and the first test was skipped with
// no message (ako/mxcli#847).
//
// A version this mxcli does not know is refused: running the bodies under
// older rules than they were written for is the silent change of meaning the
// header exists to prevent.
func takeLanguageHeader(text string, firstLine int) (v langver.Version, headerLine int, rest string, err error) {
	written, start, end, ok := langver.HeaderSpan(text)
	if !ok {
		return langver.V0, 0, text, nil
	}
	n, convErr := strconv.Atoi(written)
	if v = langver.Version(n); convErr != nil || !v.Known() {
		return langver.V0, 0, text, errors.New(langver.UnknownVersionError(written))
	}
	blank := []byte(text[start:end])
	for i, c := range blank {
		if c != '\n' && c != '\r' {
			blank[i] = ' '
		}
	}
	headerLine = firstLine + strings.Count(text[:start], "\n")
	return v, headerLine, text[:start] + string(blank) + text[end:], nil
}

// headerSetups returns the @setup microflows declared in the file's header
// comment, which apply to every test in the file.
//
// The header is the file's first javadoc comment when it carries no @test. That
// is the one shape a header can have: it may sit in its own '/'-terminated
// chunk, or share a chunk with the first test (there is no '/' between them),
// and this reads the same thing either way.
//
// Declaring a fixture once for a file is what the annotation is for — otherwise
// it says no more than `call microflow X;` at the top of each body. But a header
// can only carry what a file-wide default can honour: @cleanup, @expect, @verify
// and @throws each describe one test's execution, and silently ignoring them
// here would be the same absent-annotation bug @setup is being fixed for.
func headerSetups(content string) ([]string, error) {
	docs := scanDocComments(content, 1)
	if len(docs) == 0 {
		return nil, nil
	}
	a := parseAnnotations(docs[0].Text)
	if a.Test != "" {
		// The first comment is a test's own, so the file has no header.
		return nil, nil
	}

	var offending []string
	if len(a.Expects) > 0 || len(a.AssertionErrors) > 0 {
		offending = append(offending, "@expect")
	}
	if len(a.Verify) > 0 {
		offending = append(offending, "@verify")
	}
	if a.throwsSet {
		offending = append(offending, "@throws")
	}
	if a.cleanupSet {
		offending = append(offending, "@cleanup")
	}
	if len(offending) > 0 {
		return nil, fmt.Errorf(
			"the file header comment carries %s, which describes one test's execution and "+
				"cannot be a file-wide default — move it into that test's own doc comment "+
				"(a header may carry @setup)", strings.Join(offending, ", "))
	}
	return a.Setups, nil
}

// testBlock is one '/'-separated chunk of a test file, with the line its first
// character sits on. The line travels with the chunk because it cannot be
// recovered from the text afterwards: the previous implementation searched the
// whole file for the chunk's first 20 characters, which found the wrong chunk
// whenever two started alike and panicked on a chunk shorter than that.
type testBlock struct {
	Text string
	Line int
}

// splitTestBlocks splits MDL content on '/' delimiters (the microflow block terminator).
func splitTestBlocks(content string) []testBlock {
	// Split on lines that are just '/' (the MDL block separator)
	var blocks []testBlock
	var current []string
	start := 1
	lineNum := 0
	scanner := bufio.NewScanner(strings.NewReader(content))

	flush := func() {
		if len(current) > 0 {
			blocks = append(blocks, testBlock{Text: strings.Join(current, "\n"), Line: start})
			current = nil
		}
	}

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		switch {
		case strings.TrimSpace(line) == "/":
			flush()
			start = lineNum + 1
		case len(current) == 0 && strings.TrimSpace(line) == "":
			// A blank line before the chunk's first content is not part of it;
			// keeping it would report every test one line above where it is.
			start = lineNum + 1
		default:
			current = append(current, line)
		}
	}

	// Don't forget the last block (after the last '/' or if no '/' found)
	flush()

	return blocks
}

// extractDocAndBody separates the test's javadoc comment from the MDL body.
// Returns (docComment, body, lineNumber, error).
//
// A chunk may open with several comments — a file-level header, a `--` note —
// before the test's own doc comment, because a header is not separated from the
// test below it by a '/'. The test's doc is the LAST comment in that leading
// run; everything after it is the body.
//
// Taking the FIRST `/**` instead is #927 bug 1: the header became the doc, it
// carried no @test, and the whole chunk — the real test with it — was dropped
// with no message. Scanning for the delimiters by raw substring search is bug
// 1b: a `--` line whose prose spelled them out was read as a doc comment, so
// describing the bug in a comment re-triggered it.
func extractDocAndBody(block testBlock) (string, string, int, int, error) {
	docs := scanDocComments(block.Text, block.Line)

	// More than one @test in a chunk means a '/' separator is missing. Silently
	// keeping one of them is what bug 1 did, and an author cannot tell a dropped
	// test from a passing one, so this is refused rather than resolved.
	var named []string
	for _, d := range docs {
		if name := parseAnnotations(d.Text).Test; name != "" {
			named = append(named, name)
		}
	}
	if len(named) > 1 {
		return "", "", 0, 0, fmt.Errorf(
			"test %q is followed by another @test doc comment (%q) with no '/' separator "+
				"between them, so only one of the two could run: add a line containing "+
				"just '/' after the first test's statements", named[0], named[1])
	}

	// The test's doc is the last comment of the leading run: a file-level header
	// and the test's own doc live in the same chunk, in that order.
	var doc *docComment
	for i := range docs {
		if docs[i].Leading {
			doc = &docs[i]
		}
	}
	if doc == nil {
		return "", strings.TrimSpace(block.Text), block.Line, bodyStartLine(block.Text, 0, block.Line), nil
	}
	return doc.Text, strings.TrimSpace(block.Text[doc.End:]), doc.Line,
		bodyStartLine(block.Text, doc.End, block.Line), nil
}

// bodyStartLine is the 1-based file line the body's first non-blank character
// sits on, counting from the chunk's own first line.
//
// Computed here rather than derived from the doc comment's length, because the
// body is TrimSpace'd: leading blank lines belong to neither, and a body that
// starts on the same line as the comment's closing delimiter has no leading line
// of its own at all.
func bodyStartLine(chunk string, from, chunkLine int) int {
	line := chunkLine
	for i := 0; i < from && i < len(chunk); i++ {
		if chunk[i] == '\n' {
			line++
		}
	}
	for i := from; i < len(chunk); i++ {
		switch chunk[i] {
		case '\n':
			line++
		case ' ', '\t', '\r':
		default:
			return line
		}
	}
	return line
}

// docComment is one `/** … */` comment found in a chunk.
type docComment struct {
	Text string
	Line int
	End  int // index just past the closing delimiter
	// Leading is true when nothing but whitespace and other comments preceded
	// it, which is where a test's doc comment sits.
	Leading bool
}

// scanDocComments finds the javadoc comments in a chunk.
//
// It is a scanner rather than a substring search because the delimiters mean
// nothing outside a comment: `/**` written in the prose of a `--` line comment
// (#927 bug 1b) or inside a string literal is text, not a doc comment.
func scanDocComments(text string, startLine int) []docComment {
	var (
		out      []docComment
		i        int
		line     = startLine
		seenBody bool
	)
	for i < len(text) {
		switch {
		case text[i] == '\n':
			line++
			i++
		case text[i] == ' ' || text[i] == '\t' || text[i] == '\r':
			i++
		case strings.HasPrefix(text[i:], "--"):
			// A line comment runs to the end of the line, delimiters and all.
			if nl := strings.IndexByte(text[i:], '\n'); nl >= 0 {
				i += nl
			} else {
				i = len(text)
			}
		case strings.HasPrefix(text[i:], "/*"):
			end := strings.Index(text[i:], "*/")
			if end == -1 {
				// Unterminated. Whatever follows is not MDL either; leave it to
				// the MDL parser to report against the real source.
				return out
			}
			comment := text[i : i+end+2]
			if strings.HasPrefix(comment, "/**") {
				out = append(out, docComment{
					Text:    comment,
					Line:    line,
					End:     i + len(comment),
					Leading: !seenBody,
				})
			}
			line += strings.Count(comment, "\n")
			i += len(comment)
		case text[i] == '\'':
			seenBody = true
			i, line = skipMDLString(text, i, line)
		default:
			seenBody = true
			i++
		}
	}
	return out
}

// skipMDLString advances past a single-quoted MDL string literal, which escapes
// a quote by doubling it and may span lines.
func skipMDLString(text string, i, line int) (int, int) {
	for j := i + 1; j < len(text); j++ {
		switch text[j] {
		case '\'':
			if j+1 < len(text) && text[j+1] == '\'' {
				j++
				continue
			}
			return j + 1, line
		case '\n':
			line++
		}
	}
	return len(text), line
}

// annotations holds parsed javadoc annotations for a test block.
type annotations struct {
	Test            string
	Expects         []Expect
	AssertionErrors []string
	Verify          []Verify
	Setups          []string
	Cleanup         string
	// cleanupSet records whether @cleanup was written, which the default value
	// of Cleanup cannot express. Only the file-header check needs it.
	cleanupSet bool
	// throwsSet mirrors TestCase.ExpectsThrow: @throws was written, whether or
	// not it carried a message.
	throwsSet bool
	Throws    string
}

var (
	// expectPattern captures the whole annotation body rather than a fixed
	// operand/operator/operand shape. Matching a shape is what made this silent:
	// a line the pattern did not fit produced no assertion at all, and a test
	// with no assertions passes. Everything after @expect is now handed to
	// ParseExpect, which either compiles it or reports why it could not.
	//
	// Every pattern is anchored to the start of the line, because an annotation
	// is a javadoc tag and a tag opens its line. Matching one anywhere turned
	// prose that quotes a tag into a real annotation: writing "`@expect $x = 1`
	// in a sentence" gave the test an assertion nobody wrote, and "@cleanup none
	// would apply here" changed the cleanup strategy. The leading `*` and its
	// indentation are stripped before these run.
	expectPattern  = regexp.MustCompile(`^@expect\s+(.+)`)
	verifyPattern  = regexp.MustCompile(`^@verify\s+(.+)`)
	testPattern    = regexp.MustCompile(`^@test\s+(.+)`)
	setupPattern   = regexp.MustCompile(`^@setup\s+(\S+)`)
	cleanupPattern = regexp.MustCompile(`^@cleanup\s+(\S+)`)
	// throwsPattern matches the tag and hands the rest to parseThrows. The
	// message is optional — the old pattern required it, so a bare @throws
	// matched nothing at all and was dropped without a word.
	throwsPattern = regexp.MustCompile(`^@throws\b(.*)$`)
)

// parseAnnotations extracts test annotations from a javadoc comment.
func parseAnnotations(doc string) annotations {
	var a annotations
	a.Cleanup = "rollback" // default

	// Strip /** and */
	doc = strings.TrimPrefix(doc, "/**")
	doc = strings.TrimSuffix(doc, "*/")

	// Process line by line
	scanner := bufio.NewScanner(strings.NewReader(doc))
	for scanner.Scan() {
		line := scanner.Text()
		// Strip leading * and whitespace
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)

		if m := testPattern.FindStringSubmatch(line); m != nil {
			a.Test = strings.TrimSpace(m[1])
		}
		if m := expectPattern.FindStringSubmatch(line); m != nil {
			exp, err := ParseExpect(m[1])
			if err != nil {
				a.AssertionErrors = append(a.AssertionErrors, err.Error())
			} else {
				a.Expects = append(a.Expects, exp)
			}
		}
		if m := verifyPattern.FindStringSubmatch(line); m != nil {
			v, err := ParseVerify(m[1])
			if err != nil {
				a.AssertionErrors = append(a.AssertionErrors, err.Error())
			} else {
				a.Verify = append(a.Verify, v)
			}
		}
		if m := setupPattern.FindStringSubmatch(line); m != nil {
			a.Setups = append(a.Setups, strings.TrimSpace(m[1]))
		}
		if m := cleanupPattern.FindStringSubmatch(line); m != nil {
			a.Cleanup = strings.TrimSpace(m[1])
			a.cleanupSet = true
		}
		if m := throwsPattern.FindStringSubmatch(line); m != nil {
			parseThrows(&a, m[1])
		}
	}

	checkVerifyCleanup(&a)
	checkThrowsExpect(&a)
	return a
}

// parseThrows records an @throws annotation and its optional message.
//
// Three forms, and the third is why this is a function rather than a regexp
// capture: `@throws` alone expects any exception, `@throws 'text'` expects one
// whose message contains that text, and anything else is a mistake that must be
// reported. An unquoted or half-quoted message used to match nothing, which left
// the test with no @throws at all — the same silent-drop shape the bare form
// suffered from (ako/mxcli#301).
func parseThrows(a *annotations, rest string) {
	a.throwsSet = true
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return
	}
	if len(rest) >= 2 && strings.HasPrefix(rest, "'") && strings.HasSuffix(rest, "'") {
		a.Throws = rest[1 : len(rest)-1]
		return
	}
	a.AssertionErrors = append(a.AssertionErrors, fmt.Sprintf(
		"@throws %s: the expected message must be a single-quoted string, as in "+
			"@throws 'not found'. Write @throws on its own to expect any exception", rest))
}

// checkThrowsExpect refuses an @expect on a test that expects an exception.
//
// @throws replaces the body's normal outcome: both generators emit the
// throws-shaped microflow, in which the verdict starts as a failure and only the
// error handler can clear it, and neither emits the @expect checks at all. The
// assertion was therefore never evaluated — while AssertionCount still counted
// it, so the test reported making two assertions and made one. Asserting on a
// return value the body was expected not to produce cannot be made to work, so
// it is refused rather than quietly ignored.
func checkThrowsExpect(a *annotations) {
	if !a.throwsSet || len(a.Expects) == 0 {
		return
	}
	for _, exp := range a.Expects {
		a.AssertionErrors = append(a.AssertionErrors, fmt.Sprintf(
			"@expect %s: this test also has @throws, so the body is expected to fail and "+
				"this assertion is never evaluated. Drop one of the two — assert on the "+
				"error with @throws, or on the result with @expect", exp.Raw))
	}
	a.Expects = nil
}

// checkVerifyCleanup refuses a @verify on a test whose writes are rolled back.
//
// @verify asserts on rows the microflow wrote, and @cleanup rollback — the
// default — undoes them when the call returns, before anything can look. The
// query would run against the pre-test state and report a confident wrong
// answer, which is the failure mode this annotation is being fixed for. So it is
// refused, with the one-line change that makes it work.
func checkVerifyCleanup(a *annotations) {
	if len(a.Verify) == 0 || a.Cleanup != CleanupRollback {
		return
	}
	for _, v := range a.Verify {
		a.AssertionErrors = append(a.AssertionErrors, fmt.Sprintf(
			"@verify %s: this test uses @cleanup rollback (the default), so its writes are "+
				"undone before the query runs and it would assert against the pre-test "+
				"state. Add @cleanup none to the test", v.Raw))
	}
	a.Verify = nil
}

// suiteLanguageVersion is the one language version every test in tests is
// written in, and mdl 0 for none.
//
// A suite becomes ONE script — the after-startup runner, or the batch of test
// flows the endpoint calls — and a script has one header, so the version a
// test is run under is the suite's, not its own. Running an mdl 0 test inside
// an mdl 1 script (or the reverse) would change what its body means without a
// word, so a suite that mixes them is refused, naming the files on each side
// (ako/mxcli#847). Grouping them into one script per version was the other
// option; it doubles the injection and the cleanup for a state that `mxcli fmt
// --upgrade` removes in one command.
func suiteLanguageVersion(tests []TestCase) (langver.Version, error) {
	if len(tests) == 0 {
		return langver.V0, nil
	}
	files := map[langver.Version][]string{}
	seen := map[string]bool{}
	for _, tc := range tests {
		key := fmt.Sprintf("%d\x00%s", tc.Version, tc.SourceFile)
		if !seen[key] {
			seen[key] = true
			files[tc.Version] = append(files[tc.Version], filepath.Base(tc.SourceFile))
		}
	}
	if len(files) == 1 {
		return tests[0].Version, nil
	}
	versions := make([]int, 0, len(files))
	for v := range files {
		versions = append(versions, int(v))
	}
	sort.Ints(versions)
	parts := make([]string, len(versions))
	for i, v := range versions {
		parts[i] = fmt.Sprintf("%s: %s", langver.Version(v), strings.Join(files[langver.Version(v)], ", "))
	}
	newest := langver.Version(versions[len(versions)-1])
	return langver.V0, fmt.Errorf("the test files are written in different MDL language versions (%s), "+
		"and a suite runs as one script with one version. Give every file the same header — "+
		"`mxcli fmt --upgrade -w <file>` adds `%s;` — or run each version's files separately",
		strings.Join(parts, "; "), newest)
}
