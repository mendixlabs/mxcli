// SPDX-License-Identifier: Apache-2.0

// Package visitor provides an ANTLR parse tree visitor that builds AST nodes.
package visitor

import (
	"fmt"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

// errorListener collects ANTLR syntax errors.
type errorListener struct {
	*antlr.DefaultErrorListener
	errors []error
}

func newErrorListener() *errorListener {
	return &errorListener{
		DefaultErrorListener: antlr.NewDefaultErrorListener(),
		errors:               make([]error, 0),
	}
}

// SyntaxError is called by ANTLR when a syntax error is encountered.
func (l *errorListener) SyntaxError(_ antlr.Recognizer, _ any, line, column int, msg string, _ antlr.RecognitionException) {
	// Check if the error is about a reserved keyword being used as an identifier
	enhancedMsg := enhanceErrorMessage(msg)
	l.errors = append(l.errors, fmt.Errorf("line %d:%d %s", line, column, enhancedMsg))
}

// enhanceErrorMessage checks if an error message indicates a reserved keyword
// was used as an identifier and provides a more helpful message.
func enhanceErrorMessage(msg string) string {
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
	if looksLikeEnumEquals(msg) {
		return fmt.Sprintf("%s\n\n  Enumeration values do not use '='. Write the value name (quoted or not)\n"+
			"  followed by an optional quoted caption:\n"+
			"    create enumeration Mod.E (Value1 'Caption 1', \"Value2\" 'Caption 2');  (correct)\n"+
			"    create enumeration Mod.E (Value1 = 'Caption 1');                       (wrong — causes parse error)", msg)
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

	// Common reserved keywords that users try to use as identifiers
	reservedKeywords := map[string]bool{
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

	// Check for pattern: mismatched input 'Word' or extraneous input 'Word'
	for keyword := range reservedKeywords {
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
				return fmt.Sprintf("%s\n\n  '%s' is a reserved keyword in MDL. Use a different name like:\n"+
					"    - %s_  (add underscore suffix)\n"+
					"    - _%s  (add underscore prefix)\n"+
					"    - My%s (add prefix)\n\n"+
					"  Run 'mxcli syntax keywords' to see all reserved keywords.",
					msg, keyword, keyword, keyword, keyword)
			}
		}
	}

	return msg
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

// looksLikeUnescapedApostrophe detects ANTLR errors that are likely caused by
// unescaped apostrophes in string literals. When 'don't' is parsed, ANTLR sees
// 'don' as a complete string, then 't' as an unexpected token, producing errors
// like: missing END at 's', mismatched input 't', or token recognition error at: ”;
// We detect short (1-4 char) lowercase word fragments and unbalanced quote errors.
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

		// Short lowercase word fragments are likely apostrophe artifacts
		// e.g., "s" from "it's", "ll" from "you'll", "t" from "don't",
		// "re" from "you're", "ve" from "we've", "d" from "he'd"
		if len(token) >= 1 && len(token) <= 4 && isLowerAlpha(token) {
			return true
		}
	}
	return false
}

// isLowerAlpha returns true if s consists entirely of lowercase ASCII letters.
func isLowerAlpha(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 'a' || s[i] > 'z' {
			return false
		}
	}
	return true
}

// Builder walks the ANTLR parse tree and builds AST nodes.
type Builder struct {
	*parser.BaseMDLParserListener
	program    *ast.Program
	statements []ast.Statement
	errors     []error
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
	return &ast.Program{Statements: builder.statements}, allErrors
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
