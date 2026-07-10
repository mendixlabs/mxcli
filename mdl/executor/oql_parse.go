// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
)

// ParseOQLString re-parses a raw OQL string through the ANTLR grammar
// and returns structured OQL data. This is used for MPR-loaded entities
// where we only have the raw query string (no parse tree from MDL).
//
// The function wraps the OQL in a synthetic CREATE VIEW ENTITY statement
// so the full MDL parser can process it, then extracts the OQLParsed
// from the resulting AST.
func ParseOQLString(oql string) *ast.OQLParsed {
	if oql == "" {
		return nil
	}

	// Wrap in a synthetic CREATE VIEW ENTITY statement that the MDL parser can handle.
	// We need at least one attribute for valid syntax.
	// No wrapping parens — LPAREN? in the grammar is optional and would conflict
	// with subquery parentheses in FROM/JOIN clauses.
	synthetic := fmt.Sprintf("create view entity _Dummy._Dummy (X: String) as %s ;", oql)

	// Parse errors are expected for complex OQL expressions that the grammar
	// doesn't fully handle (e.g., advanced expressions, subqueries). ANTLR still
	// produces a partial parse tree with the structural parts (FROM, JOIN, SELECT)
	// successfully parsed, so we extract whatever we can.
	prog, _ := visitor.Build(synthetic)
	if prog == nil {
		return nil
	}

	// Find the CreateViewEntityStmt and extract its parsed OQL
	for _, stmt := range prog.Statements {
		if viewStmt, ok := stmt.(*ast.CreateViewEntityStmt); ok {
			if viewStmt.Query.Parsed != nil {
				// Clean up artifacts from the synthetic wrapper.
				// The closing } and ; can leak into clause text extracted
				// by extractOriginalText during error recovery.
				cleanSyntheticArtifacts(viewStmt.Query.Parsed)
				return viewStmt.Query.Parsed
			}
		}
	}

	return nil
}

// cleanSyntheticArtifacts removes trailing } and ; characters that can leak
// into parsed clause text when OQL is wrapped in a synthetic CREATE VIEW ENTITY statement.
func cleanSyntheticArtifacts(p *ast.OQLParsed) {
	p.Where = trimSyntheticSuffix(p.Where)
	p.GroupBy = trimSyntheticSuffix(p.GroupBy)
	p.Having = trimSyntheticSuffix(p.Having)
	p.OrderBy = trimSyntheticSuffix(p.OrderBy)
}

func trimSyntheticSuffix(s string) string {
	s = strings.TrimRight(s, " };")
	return s
}
