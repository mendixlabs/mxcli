// SPDX-License-Identifier: Apache-2.0

package adapters

import "github.com/JordtenBulte-OLC/mxcli/mdl/ast"

// buildVarEntityScope walks a microflow body and records every variable
// known to hold an entity instance, mapping varName → entity QN.
//
// Sources covered:
//   - CreateObjectStmt (Variable ← EntityType)
//   - RetrieveStmt with $var = retrieve … from <Entity> (Variable ← EntityType)
//
// The map is best-effort. An empty entry means "unknown" and the caller
// should fall back to a slot path without entity.attr enrichment.
func buildVarEntityScope(body []ast.MicroflowStatement) map[string]string {
	scope := map[string]string{}
	var walk func([]ast.MicroflowStatement)
	walk = func(stmts []ast.MicroflowStatement) {
		for _, s := range stmts {
			switch n := s.(type) {
			case *ast.CreateObjectStmt:
				if n.Variable != "" {
					scope[n.Variable] = n.EntityType.String()
				}
			case *ast.RetrieveStmt:
				if n.Variable != "" && n.StartVariable == "" && n.Source.Name != "" {
					scope[n.Variable] = n.Source.String()
				}
			case *ast.IfStmt:
				walk(n.ThenBody)
				walk(n.ElseBody)
			case *ast.WhileStmt:
				walk(n.Body)
			case *ast.LoopStmt:
				walk(n.Body)
			}
		}
	}
	walk(body)
	return scope
}
