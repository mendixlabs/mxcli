// SPDX-License-Identifier: Apache-2.0

package executor

import (
	antlr "github.com/antlr4-go/antlr/v4"
	"github.com/JordtenBulte-OLC/mxcli/mdl/grammar/parser"
)

// mdlIdent renders name as an MDL identifier suitable for DESCRIBE output,
// double-quoting it when it would not lex as a bare IDENTIFIER — e.g. when it
// collides with a reserved keyword ("List", "Column", "Template", …). This keeps
// DESCRIBE output re-parseable by `mxcli check`. See issue #619.
//
// The reserved set is not hardcoded: name is run through the actual MDL lexer,
// so the check stays correct as the grammar's keyword set evolves and never
// produces false positives (a widget named "Dot" lexes as IDENTIFIER, not the
// DOT punctuation token, so it is left unquoted).
func mdlIdent(name string) string {
	if name == "" || lexesAsBareIdentifier(name) {
		return name
	}
	// QUOTED_IDENTIFIER is '"' ~["\r\n]* '"' — no escape sequence, and Mendix
	// element names never contain a double quote, so plain wrapping is safe.
	return `"` + name + `"`
}

// lexesAsBareIdentifier reports whether name lexes as a single IDENTIFIER token
// spanning the whole string — i.e. it is safe to emit unquoted. Unlike
// isBareIdentifier (which only checks the character shape), this also rejects
// reserved keywords such as "List", because they lex to a keyword token.
func lexesAsBareIdentifier(name string) bool {
	lexer := parser.NewMDLLexer(antlr.NewInputStream(name))
	lexer.RemoveErrorListeners()
	tokens := lexer.GetAllTokens()
	if len(tokens) != 1 {
		return false
	}
	t := tokens[0]
	return t.GetTokenType() == parser.MDLLexerIDENTIFIER &&
		t.GetStart() == 0 && t.GetStop() == len(name)-1
}
