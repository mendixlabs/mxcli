// SPDX-License-Identifier: Apache-2.0

// Command gen-completions parses MDLLexer.g4 and generates Go source with
// LSP completion items derived from the grammar's keyword tokens. A token the
// parser grammar beside it accepts only as a deprecated alias is left out
// (aliases.go).
//
// Usage:
//
//	go run ./cmd/gen-completions -lexer mdl/grammar/MDLLexer.g4 -output cmd/mxcli/lsp_completions_gen.go
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"log"
	"os"
	"regexp"
	"strings"
	"unicode"
)

// tokenEntry represents a keyword token extracted from the lexer grammar.
type tokenEntry struct {
	Name     string // Token name, e.g. "CREATE"
	Text     string // User-facing keyword text, e.g. "CREATE"
	Category string // Section category, e.g. "DDL keyword"
}

func main() {
	lexerPath := flag.String("lexer", "mdl/grammar/MDLLexer.g4", "Path to MDLLexer.g4")
	outputPath := flag.String("output", "cmd/mxcli/lsp_completions_gen.go", "Output Go file")
	flag.Parse()

	entries, err := parseLexerGrammar(*lexerPath)
	if err != nil {
		log.Fatalf("parsing lexer grammar: %v", err)
	}
	sources, err := parserGrammarSources(*lexerPath)
	if err != nil {
		log.Fatalf("reading parser grammar: %v", err)
	}
	entries = withoutTokens(entries, aliasOnlyTokens(sources))

	src, err := generateSource(entries)
	if err != nil {
		log.Fatalf("generating source: %v", err)
	}

	if err := os.WriteFile(*outputPath, src, 0644); err != nil {
		log.Fatalf("writing output: %v", err)
	}

	fmt.Fprintf(os.Stderr, "Generated %s with %d keyword entries\n", *outputPath, len(entries))
}

// withoutTokens drops the entries for the named tokens.
func withoutTokens(entries []tokenEntry, drop map[string]bool) []tokenEntry {
	out := entries[:0:0]
	for _, e := range entries {
		if !drop[e.Name] {
			out = append(out, e)
		}
	}
	return out
}

// Tokens to exclude from completions — not keywords the user would type.
var excludeTokens = map[string]bool{
	// Whitespace and comments
	"WS": true, "DOC_COMMENT": true, "BLOCK_COMMENT": true, "LINE_COMMENT": true,
	// Punctuation
	"SEMICOLON": true, "COMMA": true, "DOT": true, "LPAREN": true, "RPAREN": true,
	"LBRACE": true, "RBRACE": true, "LBRACKET": true, "RBRACKET": true,
	"COLON": true, "AT": true, "PIPE": true, "DOUBLE_COLON": true,
	"ARROW": true, "QUESTION": true, "HASH": true,
	// Operators
	"NOT_EQUALS": true, "LESS_THAN_OR_EQUAL": true, "GREATER_THAN_OR_EQUAL": true,
	"EQUALS": true, "LESS_THAN": true, "GREATER_THAN": true,
	"PLUS": true, "MINUS": true, "STAR": true, "SLASH": true, "PERCENT": true,
	// Literals and identifiers
	"STRING_LITERAL": true, "NUMBER_LITERAL": true, "DOLLAR_STRING": true,
	"MENDIX_TOKEN": true, "IDENTIFIER": true, "VARIABLE": true,
	"HYPHENATED_ID": true, "QUOTED_IDENTIFIER": true,
	// `{1} = <unbound>`: a value placeholder describe prints, not a keyword
	"UNBOUND_PARAM": true,
}

// sectionHeaderRE matches section headers like "// ============" followed by "// SECTION NAME"
var sectionHeaderRE = regexp.MustCompile(`^//\s*=+\s*$`)
var sectionNameRE = regexp.MustCompile(`^//\s*(.+?)\s*$`)

// tokenRuleRE matches token definitions like: TOKEN_NAME: ... ;  // optional comment
var tokenRuleRE = regexp.MustCompile(`^([A-Z][A-Z0-9_]*)\s*:\s*(.*?)\s*;\s*(?://.*)?$`)

// fragmentLetterRE matches single fragment letter references like A, B, C
var fragmentLetterRE = regexp.MustCompile(`^[A-Z]$`)

func parseLexerGrammar(path string) ([]tokenEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []tokenEntry
	currentCategory := ""
	prevLineWasSeparator := false

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Track section headers: a line of "// ===..." followed by "// SECTION NAME"
		if sectionHeaderRE.MatchString(line) {
			prevLineWasSeparator = true
			continue
		}
		if prevLineWasSeparator {
			prevLineWasSeparator = false
			if m := sectionNameRE.FindStringSubmatch(line); m != nil {
				currentCategory = normalizeCategoryName(m[1])
			}
			continue
		}

		// Skip fragment rules
		if strings.HasPrefix(line, "fragment ") {
			continue
		}

		// Skip blank lines, comments, grammar declarations
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "lexer grammar") {
			continue
		}

		// Handle multi-line token rules (rules that end with | on next lines)
		// Accumulate continuation lines for multi-alternative rules
		if tokenRuleRE.MatchString(line) {
			m := tokenRuleRE.FindStringSubmatch(line)
			tokenName := m[1]
			tokenBody := m[2]

			if excludeTokens[tokenName] {
				continue
			}

			text := reconstructKeyword(tokenName, tokenBody)
			if text == "" {
				continue
			}

			category := currentCategory
			if tokenName == "RECEIVE" {
				// RECEIVE is shared by REST and legacy SOAP call-web-service statements.
				category = "Service keyword"
			}

			entries = append(entries, tokenEntry{
				Name:     tokenName,
				Text:     text,
				Category: category,
			})
			continue
		}

		// Handle multi-line token definitions — match just the start: NAME: body
		// and skip them (we only care about single-line rules for keywords)
		// Multi-line rules are typically complex alternatives for compound tokens
		// like DELETE_AND_REFERENCES. Check if we can extract from the first alt.
		startRE := regexp.MustCompile(`^([A-Z][A-Z0-9_]*)\s*:\s*(.*)$`)
		if m := startRE.FindStringSubmatch(line); m != nil {
			tokenName := m[1]
			if excludeTokens[tokenName] {
				// Skip to end of rule
				for scanner.Scan() {
					if strings.HasSuffix(strings.TrimSpace(scanner.Text()), ";") {
						break
					}
				}
				continue
			}

			body := m[2]
			// If body ends with ; it's actually a complete rule
			if before, ok := strings.CutSuffix(strings.TrimSpace(body), ";"); ok {
				body = before
				text := reconstructKeyword(tokenName, body)
				if text != "" {
					entries = append(entries, tokenEntry{
						Name:     tokenName,
						Text:     text,
						Category: currentCategory,
					})
				}
				continue
			}

			// Multi-line rule — try to reconstruct from first alternative (body)
			// Consume remaining lines
			for scanner.Scan() {
				l := strings.TrimSpace(scanner.Text())
				if strings.HasSuffix(l, ";") {
					break
				}
			}

			// Try to reconstruct from the first alternative
			firstAlt := firstAlternative(body)
			text := reconstructKeyword(tokenName, firstAlt)
			if text != "" {
				entries = append(entries, tokenEntry{
					Name:     tokenName,
					Text:     text,
					Category: currentCategory,
				})
			}
		}
	}

	return entries, scanner.Err()
}

// groupOpenRE matches the paren that opens a grouped rule body, as in
// `ERROR_MESSAGE: ( E R R O R WS+ M E S S A G E` whose group carries a
// right-edge predicate.
var groupOpenRE = regexp.MustCompile(`^\(\s+`)

// firstAlternative returns the keyword spelling on the first line of a
// multi-line lexer rule: the body with a trailing `|` and an opening group
// paren removed. Without the paren strip a predicated keyword silently dropped
// out of the LSP completions (mendixlabs/mxcli#1720).
func firstAlternative(body string) string {
	alt := strings.TrimSpace(body)
	alt = groupOpenRE.ReplaceAllString(alt, "")
	alt = strings.TrimSuffix(alt, "|")
	return strings.TrimSpace(alt)
}

// reconstructKeyword converts a token body (the RHS of the lexer rule) to user-facing text.
//
// Examples:
//
//	"C R E A T E"              -> "CREATE"
//	"S T R I N G"              -> "String"     (for _TYPE suffix tokens — handled by caller)
//	"N O N '-' P E R S I S T E N T" -> "NON-PERSISTENT"
//	"S A V E '_' C H A N G E S"     -> "SAVE_CHANGES"
//	"H '1'"                    -> "H1"
//	"N O T WS+ N U L L"       -> "NOT NULL"
//	"V '3'"                    -> "V3"
func reconstructKeyword(tokenName, body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}

	// Split by spaces
	parts := strings.Fields(body)
	if len(parts) == 0 {
		return ""
	}

	var result []rune
	for _, part := range parts {
		switch {
		case fragmentLetterRE.MatchString(part):
			// Single uppercase letter = case-insensitive fragment reference -> uppercase letter
			result = append(result, rune(part[0]))
		case part == "WS+", part == "WS*", part == "WS":
			// Whitespace in multi-word keywords -> space
			result = append(result, ' ')
		case len(part) == 3 && part[0] == '\'' && part[2] == '\'':
			// Literal character like '1', '-', '_'
			result = append(result, rune(part[1]))
		case part == "'_'":
			result = append(result, '_')
		case part == "'_'?":
			// Optional underscore — include it for the completion text
			result = append(result, '_')
		case part == "'-'":
			result = append(result, '-')
		default:
			// Unknown part — not a simple keyword token
			return ""
		}
	}

	text := string(result)
	if text == "" {
		return ""
	}

	// Apply naming conventions based on token suffix
	switch {
	case strings.HasSuffix(tokenName, "_TYPE"):
		// Data types: STRING_TYPE -> "String", BOOLEAN_TYPE -> "Boolean"
		text = titleCase(text)
	case strings.HasSuffix(tokenName, "_STYLE"):
		// Button styles: WARNING_STYLE -> "Warning"
		text = titleCase(text)
	default:
		// Keep uppercase for regular keywords
		text = strings.ToUpper(text)
	}

	return text
}

// titleCase converts "STRING" to "String", "DATETIME" to "DateTime", etc.
func titleCase(s string) string {
	s = strings.ToUpper(s)
	// Special cases for compound words
	replacements := map[string]string{
		"DATETIME":       "DateTime",
		"AUTONUMBER":     "AutoNumber",
		"HASHEDSTRING":   "HashedString",
		"STRINGTEMPLATE": "StringTemplate",
	}
	if r, ok := replacements[s]; ok {
		return r
	}
	// Default: capitalize first letter, lowercase rest
	runes := []rune(strings.ToLower(s))
	if len(runes) > 0 {
		runes[0] = unicode.ToUpper(runes[0])
	}
	return string(runes)
}

// normalizeCategoryName converts section headers like "DDL KEYWORDS" to "DDL keyword".
func normalizeCategoryName(raw string) string {
	raw = strings.TrimSpace(raw)
	// Map known section headers to detail strings
	switch {
	case strings.Contains(raw, "WHITESPACE"):
		return "" // Skip
	case strings.Contains(raw, "MULTI-WORD"):
		return "Multi-word keyword"
	case strings.Contains(raw, "DDL"):
		return "DDL keyword"
	case strings.Contains(raw, "CONNECTION"):
		return "Connection keyword"
	case strings.Contains(raw, "OQL"), strings.Contains(raw, "QUERY"):
		return "Query keyword"
	case strings.Contains(raw, "ANCHOR"):
		return "Flow annotation keyword"
	case strings.Contains(raw, "MICROFLOW"):
		return "Microflow keyword"
	case strings.Contains(raw, "PAGE"), strings.Contains(raw, "WIDGET"):
		return "Widget keyword"
	case strings.Contains(raw, "DATA TYPE"):
		return "Data type"
	case strings.Contains(raw, "AGGREGATE"):
		return "Aggregate function"
	case strings.Contains(raw, "LOGICAL"), strings.Contains(raw, "COMPARISON"):
		return "Logical keyword"
	case strings.Contains(raw, "VALIDATION"), strings.Contains(raw, "CONSTRAINT"):
		return "Validation keyword"
	case strings.Contains(raw, "REST"):
		return "REST keyword"
	case strings.Contains(raw, "UTILITY"):
		return "Utility keyword"
	case strings.Contains(raw, "ARITHMETIC"):
		return "Operator"
	case strings.Contains(raw, "PUNCTUATION"):
		return "" // Skip
	case strings.Contains(raw, "LITERAL"):
		return "" // Skip
	case strings.Contains(raw, "IDENTIFIER"):
		return "" // Skip
	case strings.Contains(raw, "FRAGMENT"):
		return "" // Skip
	default:
		return "Keyword"
	}
}

func generateSource(entries []tokenEntry) ([]byte, error) {
	var buf bytes.Buffer

	buf.WriteString(`// Code generated by cmd/gen-completions; DO NOT EDIT.

package main

import "go.lsp.dev/protocol"

// mdlGeneratedKeywords contains all keyword completion items derived from MDLLexer.g4.
var mdlGeneratedKeywords = []protocol.CompletionItem{
`)

	// Group by category for readability
	lastCategory := ""
	for _, e := range entries {
		if e.Category != lastCategory {
			if lastCategory != "" {
				buf.WriteString("\n")
			}
			fmt.Fprintf(&buf, "\t// %s\n", e.Category)
			lastCategory = e.Category
		}
		fmt.Fprintf(&buf, "\t{Label: %q, Kind: protocol.CompletionItemKindKeyword, Detail: %q},\n",
			e.Text, e.Category)
	}

	buf.WriteString("}\n")

	return format.Source(buf.Bytes())
}
