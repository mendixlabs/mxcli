// SPDX-License-Identifier: Apache-2.0

// Package visitor provides an ANTLR parse tree visitor that builds AST nodes.
package visitor

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// errorListener collects ANTLR syntax errors.
type errorListener struct {
	*antlr.DefaultErrorListener
	errors []error
	source []string // input split into lines, for source-aware hints
	// hinted records the (line, hint) pairs already reported, so one mistake
	// that cascades into several ANTLR errors carries its explanation once.
	hinted map[string]bool
}

func newErrorListener() *errorListener {
	return &errorListener{
		DefaultErrorListener: antlr.NewDefaultErrorListener(),
		errors:               make([]error, 0),
		hinted:               make(map[string]bool),
	}
}

// SyntaxError is called by ANTLR when a syntax error is encountered.
func (l *errorListener) SyntaxError(_ antlr.Recognizer, _ any, line, column int, msg string, _ antlr.RecognitionException) {
	offending := ""
	if line >= 1 && line <= len(l.source) {
		offending = l.source[line-1]
	}
	enhancedMsg := l.deduplicateHint(enhanceErrorMessage(msg, offending), line)
	l.errors = append(l.errors, fmt.Errorf("line %d:%d %s", line, column, enhancedMsg))
}

// deduplicateHint strips the explanatory block from an enhanced message when the
// same block was already attached to an error on the same line.
//
// One mistake commonly produces a cascade — an INDEX inside the attribute
// parentheses yields four ANTLR errors on one line — and repeating a six-line
// worked example under each of them buries the errors it is meant to explain.
// Suppression is per line, so the same mistake made twice in a file is still
// explained at each site.
//
// A hint is everything from the first blank line on; that is the shape every
// branch of enhanceErrorMessage builds ("%s\n\n  …"), and a raw ANTLR message
// is a single line, so the separator cannot occur in one.
func (l *errorListener) deduplicateHint(msg string, line int) string {
	i := strings.Index(msg, "\n\n")
	if i < 0 {
		return msg
	}
	key := fmt.Sprintf("%d\x00%s", line, msg[i:])
	if l.hinted[key] {
		return msg[:i]
	}
	l.hinted[key] = true
	return msg
}

// simplifyExpecting collapses ANTLR's raw `expecting {A, B, …}` token-set dumps,
// which are precise but noisy: a failed statement start lists ~30 internal token
// names (`{<EOF>, DOC_COMMENT, CREATE, ALTER, …}`), drowning the actual signal.
// The all-statement-keywords set is rewritten to a human phrase; any other
// oversized set is truncated. A small set (the useful case, e.g. `expecting ':'`)
// is left untouched.
func simplifyExpecting(msg string) string {
	const marker = "expecting {"
	i := strings.Index(msg, marker)
	if i < 0 {
		return msg
	}
	rel := strings.Index(msg[i:], "}")
	if rel < 0 {
		return msg
	}
	inner := msg[i+len(marker) : i+rel]
	toks := strings.Split(inner, ", ")
	set := make(map[string]bool, len(toks))
	for _, t := range toks {
		set[t] = true
	}
	rest := msg[i+rel+1:]
	// The top-level statement-start set — the "this isn't a valid statement"
	// case, usually a misspelled verb or an unterminated previous statement.
	if set["CREATE"] && set["ALTER"] && set["DROP"] {
		return msg[:i] + "expecting the start of a statement (create, alter, drop, show, describe, …)" + rest
	}
	if len(toks) > 6 {
		return msg[:i] + "expecting one of {" + strings.Join(toks[:6], ", ") + ", …}" + rest
	}
	return msg
}

// enhanceErrorMessage turns a raw ANTLR syntax error into something that points
// at the fix: it collapses noisy token-set dumps, recognises a handful of common
// mistakes (adding a correct-vs-wrong example), and otherwise appends a pointer
// to `mxcli syntax`. offendingLine is the source line the error occurred on (""
// when unavailable), used for source-aware hints.
func enhanceErrorMessage(msg, offendingLine string) string {
	// Collapse ANTLR's oversized `expecting {…}` token dumps first, so every
	// branch below (and the fall-through) shows the tamer form.
	msg = simplifyExpecting(msg)

	// A bare `not $x` — Mendix requires `not(expr)`. The parse error surfaces
	// downstream (e.g. "missing THEN at '$x'"), so key off the source line, which
	// is unambiguous for `not $…`. (sudoku findings #3)
	if bareNotRe.MatchString(offendingLine) {
		return fmt.Sprintf("%s\n\n  Mendix requires parentheses around a negated expression — a bare\n"+
			"  'not <expr>' does not parse:\n"+
			"    if not($Cell/IsInvalid) then …   (correct)\n"+
			"    if not $Cell/IsInvalid then …    (wrong — causes parse error)", msg)
	}
	// Check for quoted attribute names after READ/WRITE in a GRANT clause.
	// Users often write `READ "Attr1", "Attr2"` instead of the correct
	// `READ (Attr1, Attr2)` — the grammar expects unquoted identifiers in parens.
	if looksLikeQuotedGrantAttribute(msg) {
		return fmt.Sprintf("%s\n\n  Attribute-level GRANT uses unquoted identifiers inside parentheses,\n"+
			"  not quoted strings. Comma-separate multiple attributes:\n"+
			"    GRANT Mod.Role ON Mod.Entity (READ (Attr1, Attr2), WRITE (Attr1));  (correct)\n"+
			"    GRANT Mod.Role ON Mod.Entity (READ \"Attr1\", \"Attr2\");            (wrong — causes parse error)", msg)
	}

	// Check for an `=` between an enumeration value name and its caption. Users
	// coming from SQL (or the "always quote identifiers" habit) write
	// `Value = 'Caption'`, but MDL enumeration values are `Value 'Caption'`
	// (optionally `Value CAPTION 'Caption'`) — no equals sign. The value name may
	// itself be quoted; the `=` is the actual problem, not the quotes.
	// A `comment 'text'` option on a CREATE that no longer accepts one. It used
	// to parse and do nothing — the visitor stored it and the executor never read
	// it — so scripts written against the old grammar reported success and set no
	// documentation. Removing it turns that silence into an error, and this turns
	// the error into the fix.
	if looksLikeRemovedCreateComment(offendingLine) {
		return fmt.Sprintf("%s\n\n  A `comment '…'` option on CREATE set no documentation — it was parsed and\n"+
			"  discarded — so it has been removed. Use the doc comment, which does work\n"+
			"  and carries more:\n"+
			"    /** What this does. */\n"+
			"    create microflow Mod.MF () begin … end;        (correct)\n"+
			"    create microflow Mod.MF () comment 'What this does' begin … end;  (removed)\n"+
			"  On an existing entity, ALTER ENTITY Mod.E SET COMMENT '…' also works.", msg)
	}

	// `Height: n` on an annotation. Mendix stores no annotation height — the box
	// auto-sizes to its caption — so the bare "expecting {POSITION, CAPTION,
	// WIDTH}" reads like a missing mxcli feature when it is a missing Mendix
	// property. Say which it is, and name the two levers that do exist. (#1014)
	if looksLikeAnnotationProperty(msg) && annotationHeightRe.MatchString(offendingLine) {
		return fmt.Sprintf("%s\n\n  Mendix does not store an annotation height. DomainModels$Annotation has\n"+
			"  exactly four properties — Caption, ExportLevel, Location and Width — so the\n"+
			"  note auto-sizes to its caption and there is nothing to write a height into.\n"+
			"  Height follows from the wrapping, so WIDTH is the lever:\n"+
			"    Width: 300   -- narrower wraps to more lines, so the note is TALLER\n"+
			"    Width: 600   -- wider wraps to fewer lines, so the note is SHORTER\n"+
			"  To stop a note overlapping what sits below it, move those down:\n"+
			"    ALTER ENTITY Mod.Entity SET POSITION (x, y);", msg)
	}

	// Any other unknown property in an annotation's list — the same four-property
	// limit, without the height-specific advice.
	if looksLikeAnnotationProperty(msg) {
		return fmt.Sprintf("%s\n\n  An annotation stores only Caption, Position and Width (Mendix keeps\n"+
			"  Caption, ExportLevel, Location and Width, and nothing else — there is no\n"+
			"  height, no colour and no name).", msg)
	}

	if looksLikeEnumEquals(msg) {
		return fmt.Sprintf("%s\n\n  Enumeration values do not use '='. Write the value name (quoted or not)\n"+
			"  followed by an optional quoted caption:\n"+
			"    create enumeration Mod.E (Value1 'Caption 1', \"Value2\" 'Caption 2');  (correct)\n"+
			"    create enumeration Mod.E (Value1 = 'Caption 1');                       (wrong — causes parse error)", msg)
	}

	// Arithmetic inside an XPath constraint. Mendix XPath can't compute values
	// (no +, -, *, div, mod on the value side) — compute into a variable first,
	// then compare against it. (sudoku findings #8)
	if looksLikeXPathArithmetic(msg) {
		return fmt.Sprintf("%s\n\n  Mendix XPath constraints cannot compute values (no +, -, *, div, mod).\n"+
			"  Compute the value into a variable first, then compare against it:\n"+
			"    $Next = $Game/MoveSeq + 1;\n"+
			"    retrieve $M from Mod.Move where [Seq = $Next] limit 1;   (correct)\n"+
			"    retrieve $M from Mod.Move where [Seq = $Game/MoveSeq + 1] limit 1;  (wrong — XPath can't compute)", msg)
	}

	// An INDEX written INSIDE the attribute parentheses. The index syntax exists
	// — it just belongs after the closing paren — but ANTLR reports the index
	// name as an attribute missing its type ("missing ':' at '\"IdxRowCol\"'"),
	// which reads as "indexes are not supported here" and sends the author off to
	// find a different spelling. Keyed off the source line, because the token
	// error lands on the name rather than on `index`. (sudoku findings #4)
	if misplacedIndexRe.MatchString(offendingLine) {
		return fmt.Sprintf("%s\n\n  An INDEX belongs AFTER the attribute parentheses, not inside them:\n"+
			"    create entity Mod.Cell (Row: Integer, Col: Integer)\n"+
			"      index \"IdxRowCol\" on (Row, Col);                     (correct)\n"+
			"    create entity Mod.Cell (Row: Integer, Col: Integer,\n"+
			"      index \"IdxRowCol\" on (Row, Col));                    (wrong — causes parse error)\n"+
			"  On an existing entity: alter entity Mod.Cell add index \"IdxRowCol\" on (Row, Col);", msg)
	}

	// Check for a misplaced EXTENDS / GENERALIZATION clause. It must precede the
	// attribute list — `create entity Mod.Child extends Mod.Parent ( ... )` — but
	// users often append it after the closing parenthesis, where ANTLR reports a
	// generic "extraneous/mismatched input 'extends'".
	if looksLikeMisplacedExtends(msg) {
		return fmt.Sprintf("%s\n\n  An EXTENDS (or GENERALIZATION) clause must come BEFORE the attribute\n"+
			"  parentheses, not after them:\n"+
			"    create entity Mod.Child extends Mod.Parent (Name: String);  (correct)\n"+
			"    create entity Mod.Child (Name: String) extends Mod.Parent;  (wrong — causes parse error)", msg)
	}

	// Check for a string literal that runs off the end of its line BEFORE the
	// apostrophe heuristic — a newline inside the offending token means the lexer
	// hit end-of-line inside an unterminated literal, which the "double your
	// apostrophes" hint would only make worse. (findings #6)
	if looksLikeMultilineStringLiteral(msg) {
		return fmt.Sprintf("%s\n\n  A string literal is not terminated before the end of the line —\n"+
			"  MDL string literals cannot span multiple lines. Keep the whole string on\n"+
			"  one line (concatenate across lines with + if it is long):\n"+
			"    dynamicclasses: 'if $x then ''a'' else ''b'''   (correct — one line)\n"+
			"    dynamicclasses: 'if $x then ''a''\n                     else ''b'''   (wrong — spans lines)", msg)
	}

	// Check for unescaped apostrophe in string literals first.
	// When 'it's here' is parsed, ANTLR sees 'it' as a complete string, then
	// the leftover characters (like "s", "ll", "t") appear as unexpected tokens.
	// Detect this by looking for very short mismatched/extraneous tokens that are
	// likely word fragments from a broken string.
	if looksLikeUnescapedApostrophe(msg) {
		return fmt.Sprintf("%s\n\n  This may be caused by an unescaped apostrophe in a string literal.\n"+
			"  In MDL strings, use '' (two single quotes) to escape apostrophes:\n"+
			"    'it''s here'  (correct)\n"+
			"    'it's here'   (wrong — causes parse error)", msg)
	}

	// Check for pattern: mismatched input 'Word' or extraneous input 'Word'
	for keyword := range hintedKeywordSet {
		patterns := []string{
			fmt.Sprintf("mismatched input '%s'", keyword),
			fmt.Sprintf("extraneous input '%s'", keyword),
			fmt.Sprintf("mismatched input '%s'", strings.ToLower(keyword)),
			fmt.Sprintf("extraneous input '%s'", strings.ToLower(keyword)),
			fmt.Sprintf("mismatched input '%s'", strings.ToUpper(keyword)),
			fmt.Sprintf("extraneous input '%s'", strings.ToUpper(keyword)),
		}
		for _, pattern := range patterns {
			if strings.Contains(msg, pattern) {
				return fmt.Sprintf("%s\n\n%s", msg, reservedKeywordHint(keyword))
			}
		}
	}

	// `alter entity … add <name>: <type>` without the `attribute` keyword. The raw
	// error is an unhelpful "no viable alternative at input 'add<Name>'"; key off
	// the source line, which unambiguously shows `add <name>:` with a non-clause
	// word. (traceops #16)
	if m := addMissingAttributeRe.FindStringSubmatch(offendingLine); m != nil && strings.Contains(msg, "no viable alternative") {
		switch strings.ToLower(m[1]) {
		case "attribute", "column", "index", "event", "association", "value", "role", "handler":
			// a real clause keyword — not the missing-keyword mistake
		default:
			return fmt.Sprintf("%s\n\n  ALTER ENTITY needs the `attribute` keyword before a new attribute:\n"+
				"    alter entity Module.Entity add attribute %s: <type>;   (correct)\n"+
				"    alter entity Module.Entity add %s: <type>;             (wrong)", msg, m[1], m[1])
		}
	}

	// Nothing matched a specific pattern — the location is precise but the fix
	// isn't spelled out. Point at the syntax reference so the correct form is one
	// command away. (Kept to one line since it can repeat across cascading errors.)
	return msg + "  [see: mxcli syntax <topic>, e.g. entity | microflow | page]"
}

// addMissingAttributeRe matches `add <name>:` on a source line — the shape of an
// ALTER ENTITY add-attribute clause missing its `attribute` keyword. The captured
// word is checked against the real clause keywords in enhanceErrorMessage so a
// valid `add index`/`add event handler`/etc. is not mis-hinted. (traceops #16)
var addMissingAttributeRe = regexp.MustCompile(`(?i)\badd\s+([A-Za-z_]\w*)\s*:`)

// bareNotRe matches a bare `not $…` (not followed by `(`) on a source line — the
// exact shape of the unparenthesized-negation mistake. Scoped to `not $var` to
// stay false-positive-free (it won't fire on `not(...)`, `is not null`, etc.).
var bareNotRe = regexp.MustCompile(`(?i)\bnot\s+\$`)

// removedCreateCommentRe matches a `comment '…'` option on one of the CREATE
// statements that no longer takes one. Anchored on the statement keyword so it
// does not fire on the many places COMMENT is still valid — constants, JSON
// structures, image collections, database connections, workflow activities and
// ALTER … SET COMMENT.
// Two shapes, because a CREATE is usually written across several lines and the
// offending line is then the option on its own:
//
//	create microflow M.MF () comment 'x' begin …    -> the whole statement
//	  comment 'x'                                   -> just the option
//
// The second alternative is safe on the statements where COMMENT is still valid
// — a valid option does not produce a parse error for the hint to attach to.
var removedCreateCommentRe = regexp.MustCompile(
	`(?i)(\bcreate\b.*\b(entity|enumeration|module|microflow|nanoflow|rule)\b.*\bcomment\s*'` +
		`|^\s*comment\s*'[^']*'\s*$)`)

// misplacedIndexRe matches an INDEX definition written as if it were an entry in
// the attribute list: `index "Name" on (A, B)`, `index "Name" (A)` or the
// anonymous `index (A)`. The discriminator against the standalone `create index`
// statement — which is valid and has the same leading token — is that the
// parenthesis follows the optional ON directly, where the standalone form names
// the entity in between (`index Idx on Mod.Cell (Row)`).
var misplacedIndexRe = regexp.MustCompile(`(?i)^\s*index\s+("[^"]*"|[A-Za-z_]\w*)?\s*(on\s*)?\(`)

// xpathArithmeticRe matches an arithmetic operator that ANTLR rejected inside a
// bracketed XPath constraint. `expecting ']'` only occurs inside `[…]`, so a
// stray arithmetic operator there is a compute-in-constraint attempt. The op set
// is quoted-literal to avoid matching, say, a `-` inside a normal expression.
var xpathArithmeticRe = regexp.MustCompile(`mismatched input '(\+|\*|div|mod)' expecting '\]'`)

// looksLikeXPathArithmetic detects an arithmetic operator used on the value side
// of an XPath constraint (e.g. `[Seq = $Game/MoveSeq + 1]`). Mendix XPath cannot
// compute values, so this must be pre-computed into a variable. (findings #8)
func looksLikeXPathArithmetic(msg string) bool {
	return xpathArithmeticRe.MatchString(msg)
}

// looksLikeMisplacedExtends detects ANTLR errors caused by an EXTENDS /
// GENERALIZATION clause placed after the entity's attribute parentheses instead
// of before them. The offending token surfaces as extraneous/mismatched input.
func looksLikeMisplacedExtends(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "input 'extends'") || strings.Contains(lower, "input 'generalization'")
}

// looksLikeEnumEquals detects `Value = 'Caption'` inside an enumeration value
// list. ANTLR reports the `=` as `mismatched input '=' expecting ')'`; an
// attribute default with `=` produces a different ("no viable alternative")
// message, so this pattern is specific to the enum-value case.
// looksLikeRemovedCreateComment detects a `comment '…'` option on a CREATE
// statement that no longer accepts one (entity, enumeration, module, microflow,
// nanoflow, rule).
//
// It keys off the source line rather than the ANTLR message because the error
// lands wherever the parser gave up — on the quoted string, on the following
// token, or on `begin` — and none of those name the real problem.
func looksLikeRemovedCreateComment(offendingLine string) bool {
	return removedCreateCommentRe.MatchString(offendingLine)
}

func looksLikeEnumEquals(msg string) bool {
	return strings.Contains(msg, "mismatched input '=' expecting ')'")
}

// looksLikeQuotedGrantAttribute detects ANTLR errors from `READ "Attr"` /
// `WRITE "Attr"` — a common mistake where users quote attribute names instead
// of using the correct `READ (Attr1, Attr2)` identifier list.
//
// Typical ANTLR shapes:
//   - no viable alternative at input 'READ"Attr1"'
//   - no viable alternative at input 'WRITE"Attr1"'
//   - mismatched input '"Attr"' expecting {CREATE, DELETE, READ, WRITE}
func looksLikeQuotedGrantAttribute(msg string) bool {
	if strings.Contains(msg, `input 'READ"`) || strings.Contains(msg, `input 'WRITE"`) {
		return true
	}
	// Quoted string appearing where a GRANT access right is expected.
	if strings.Contains(msg, `expecting {CREATE, DELETE, READ, WRITE}`) &&
		(strings.Contains(msg, `input '"`) || strings.Contains(msg, `input "`)) {
		return true
	}
	return false
}

// contractionSuffixes are the token fragments ANTLR is left holding after an
// unescaped apostrophe splits an English contraction: 'don't' → string 'don' +
// leftover t; 'it's' → s; 'you'll' → ll; 'you're' → re; 'we've' → ve; 'he'd' →
// d; 'I'm' → m. Matching this fixed set (rather than "any short lowercase word")
// is what keeps the apostrophe hint from firing on real MDL keywords/identifiers
// that happen to be short and lowercase — e.g. a misplaced `on`, `in`, `as`,
// `to`, `by` produced the wrong "unescaped apostrophe" advice before (findings #4).
var contractionSuffixes = map[string]bool{
	"s": true, "t": true, "d": true, "m": true,
	"re": true, "ve": true, "ll": true,
}

// looksLikeMultilineStringLiteral detects an unterminated string literal that ran
// off the end of its line. ANTLR reports "token recognition error at: ”" with the
// offending text spilling onto the next line (a newline appears in the payload).
// This must be distinguished from a plain unescaped apostrophe — the apostrophe
// hint ("double your apostrophes") would make a line-spanning string worse.
func looksLikeMultilineStringLiteral(msg string) bool {
	idx := strings.Index(msg, "token recognition error at:")
	if idx < 0 {
		return false
	}
	return strings.ContainsAny(msg[idx:], "\n\r")
}

// looksLikeUnescapedApostrophe detects ANTLR errors that are likely caused by
// unescaped apostrophes in string literals. When 'don't' is parsed, ANTLR sees
// 'don' as a complete string, then 't' as an unexpected token, producing errors
// like: missing END at 's', mismatched input 't', or token recognition error at: ”;
// We detect the specific contraction-suffix fragments and unbalanced quote errors.
func looksLikeUnescapedApostrophe(msg string) bool {
	// Pattern 1: "token recognition error at: ''" — unbalanced trailing quote
	if strings.Contains(msg, "token recognition error at: ''") {
		return true
	}

	// Pattern 2: Various ANTLR error shapes with short lowercase tokens
	// e.g., "missing END at 's'", "mismatched input 'll'", "extraneous input 't'"
	for _, prefix := range []string{
		"mismatched input '", "extraneous input '", "missing ",
	} {
		idx := strings.Index(msg, prefix)
		if idx < 0 {
			continue
		}

		// For "missing X at 'token'" pattern, find the "at '" part
		searchFrom := idx + len(prefix)
		if prefix == "missing " {
			atIdx := strings.Index(msg[searchFrom:], " at '")
			if atIdx < 0 {
				continue
			}
			searchFrom = searchFrom + atIdx + len(" at '")
		}

		// Extract the token
		var token string
		if prefix == "missing " {
			tokenEnd := strings.Index(msg[searchFrom:], "'")
			if tokenEnd < 0 {
				continue
			}
			token = msg[searchFrom : searchFrom+tokenEnd]
		} else {
			tokenEnd := strings.Index(msg[searchFrom:], "'")
			if tokenEnd < 0 {
				continue
			}
			token = msg[searchFrom : searchFrom+tokenEnd]
		}

		// Only the specific contraction-suffix fragments are apostrophe artifacts.
		// A real MDL keyword/identifier (on, in, as, to, by, …) is short and
		// lowercase too, but is NOT a contraction leftover, so it must not match.
		if contractionSuffixes[token] {
			return true
		}
	}
	return false
}

// Builder walks the ANTLR parse tree and builds AST nodes.
type Builder struct {
	*parser.BaseMDLParserListener
	program    *ast.Program
	statements []ast.Statement
	errors     []error
	// documentAnnotations collects every `@name` written before a CREATE, with
	// the kind of document it was on — see ExitCreateStatement.
	documentAnnotations []ast.DocumentAnnotation

	// inLayout is set while a CREATE LAYOUT body is being built. The page body
	// builder serves both documents and cannot otherwise tell which it is in,
	// and a `placeholder X { … }` means opposite things in the two: in a page it
	// FILLS a layout's slot, in a layout it is a mistake for the bodiless
	// declaration. See buildLayoutV3 (mendixlabs/mxcli#1063).
	inLayout bool
	// layoutBracedPlaceholders collects the braced placeholder names seen while
	// inLayout, at any depth, for the checker to report.
	layoutBracedPlaceholders []string
}

// NewBuilder creates a new AST builder.
func NewBuilder() *Builder {
	return &Builder{
		BaseMDLParserListener: &parser.BaseMDLParserListener{},
		statements:            make([]ast.Statement, 0),
		errors:                make([]error, 0),
	}
}

// getSpacedText reconstructs text from a parse tree node with spaces between
// leaf tokens. This is needed because ANTLR's GetText() concatenates without
// whitespace (since WS tokens are skipped), which breaks keyword operators
// like MATCH, LIKE, BETWEEN in SQL pass-through queries.
func getSpacedText(tree antlr.Tree) string {
	var tokens []string
	collectLeafTokens(tree, &tokens)
	return strings.Join(tokens, " ")
}

// collectLeafTokens recursively collects terminal node texts from a parse tree.
func collectLeafTokens(tree antlr.Tree, tokens *[]string) {
	if leaf, ok := tree.(antlr.TerminalNode); ok {
		*tokens = append(*tokens, leaf.GetText())
		return
	}
	for i := 0; i < tree.GetChildCount(); i++ {
		collectLeafTokens(tree.GetChild(i), tokens)
	}
}

// Build parses the input and returns the AST program.
func Build(input string) (*ast.Program, []error) {
	// Create custom error listener to capture syntax errors
	errListener := newErrorListener()
	errListener.source = strings.Split(input, "\n")

	// Create lexer with custom error listener
	is := antlr.NewInputStream(input)
	lexer := parser.NewMDLLexer(is)
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errListener)

	// Create parser with custom error listener
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	p := parser.NewMDLParser(stream)
	p.RemoveErrorListeners()
	p.AddErrorListener(errListener)

	// Create builder and walk the tree
	builder := NewBuilder()
	tree := p.Program()
	antlr.ParseTreeWalkerDefault.Walk(builder, tree)

	// Combine syntax errors and builder errors
	allErrors := append(errListener.errors, builder.errors...)
	return &ast.Program{
		Statements:          builder.statements,
		DocumentAnnotations: builder.documentAnnotations,
	}, allErrors
}

// Errors returns any errors encountered during building.
func (b *Builder) Errors() []error {
	return b.errors
}

// Statements returns the built statements.
func (b *Builder) Statements() []ast.Statement {
	return b.statements
}

// addError adds an error to the builder's error list.
func (b *Builder) addError(err error) {
	b.errors = append(b.errors, err)
}

// addErrorWithExample adds an error with example MDL syntax to help LLMs understand the expected format.
func (b *Builder) addErrorWithExample(message, example string) {
	b.errors = append(b.errors, fmt.Errorf("%s\n\nExpected syntax:\n%s", message, example))
}

// reservedKeywordHint explains how to use a name the MDL parser reserves.
//
// The distinction it draws is the whole point, and getting it wrong costs real
// work either way. An MDL parser keyword is escaped by QUOTING — `write
// ("Title")` parses and the attribute is still called Title — but a name the
// Mendix PLATFORM reserves is not, because the check strips the quotes and
// validates the bare name (CE7247 / MDL021).
//
// This used to offer three renames and never mention quoting, for every keyword.
// Measured against the platform's own list, that was the wrong advice for 38 of
// the 41 keywords here and right for three by accident — and it is expensive
// advice, because renaming an attribute that already exists is a schema change
// where quoting is free. It sent ako/mxcli-maintenance's Title attribute to
// RequestTitle for nothing.
func reservedKeywordHint(keyword string) string {
	if types.IsPlatformReserved(keyword) {
		return fmt.Sprintf(
			"  '%s' is reserved by MENDIX itself, not just by MDL, so quoting it does not help —\n"+
				"  the name is rejected with CE7247 even as \"%s\". Rename it:\n"+
				"    - %sValue\n"+
				"    - My%s\n\n"+
				"  Run 'mxcli syntax keywords' to see all reserved keywords.",
			keyword, keyword, keyword, keyword)
	}
	return fmt.Sprintf(
		"  '%s' is a keyword in MDL. Quote it to use it as a name — the model keeps the\n"+
			"  name exactly as written, so nothing has to be renamed:\n"+
			"    \"%s\"\n\n"+
			"  Run 'mxcli syntax keywords' to see all reserved keywords.",
		keyword, keyword)
}

// hintedKeywordSet holds MDL parser keywords people reach for as identifiers. Being on this list says
// nothing about whether the name is USABLE — see the hint below.
var hintedKeywordSet = map[string]bool{
	"Title": true, "Status": true, "Type": true, "Value": true,
	"Reference": true, "Label": true, "Caption": true, "Name": true,
	"Message": true, "Error": true, "Source": true, "Target": true,
	"Action": true, "Service": true, "Header": true, "Footer": true,
	"Content": true, "Body": true, "Response": true, "Request": true,
	"Result": true, "Data": true, "Info": true, "Warning": true,
	"Success": true, "Default": true, "Template": true, "Version": true,
	"Index": true, "Owner": true, "Method": true, "Path": true,
	"Query": true, "Filter": true, "Sort": true, "Order": true,
	"Count": true, "Sum": true, "Min": true, "Max": true, "Avg": true,
}

// hintedKeywords lists the keywords the hint recognises, sorted so the test that
// classifies them reports a stable set.
func hintedKeywords() []string {
	out := make([]string, 0, len(hintedKeywordSet))
	for k := range hintedKeywordSet {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// annotationHeightRe matches a `Height:` property line.
var annotationHeightRe = regexp.MustCompile(`(?i)^\s*height\s*:`)

// looksLikeAnnotationProperty reports whether a parse error came from an
// annotation's property list. The three-token expecting-set is unique to
// `annotationProperty` in the grammar, and survives simplifyExpecting untouched
// (three tokens, none of them statement-start keywords), so keying on it does
// not misfire on a page widget's Height or any other property list.
func looksLikeAnnotationProperty(msg string) bool {
	i := strings.Index(msg, "expecting {")
	if i < 0 {
		return false
	}
	rel := strings.Index(msg[i:], "}")
	if rel < 0 {
		return false
	}
	set := msg[i : i+rel]
	return strings.Contains(set, "CAPTION") && strings.Contains(set, "POSITION") && strings.Contains(set, "WIDTH")
}
