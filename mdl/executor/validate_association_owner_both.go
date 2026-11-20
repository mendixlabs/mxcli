// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// CE0002 — an association with owner Both between an entity and itself, or between
// an entity and one of its generalizations. Measured on MxBuild 11.16.1, for
// Reference and ReferenceSet alike: same entity, child -> parent, parent -> child
// and a longer chain are all rejected; an unrelated pair, and every other owner, build.
//
// Only generalizations the script itself declares are followed. An entity that
// extends another in the project is not seen here, so that case still reaches
// `mx check`; refusing less is the safe direction.
const associationOwnerBothRule = "MDL099"

// ValidateAssociationOwnerBoth reports each `owner both` association whose two ends
// are one entity or sit on one generalization chain.
func ValidateAssociationOwnerBoth(prog *ast.Program) []linter.Violation {
	extends := map[string]string{}
	for _, s := range prog.Statements {
		if e, ok := s.(*ast.CreateEntityStmt); ok && e.Generalization != nil {
			extends[strings.ToLower(e.Name.String())] = strings.ToLower(e.Generalization.String())
		}
	}
	// chainOf is the entity followed by each of its generalizations; the visited
	// set ends a cycle, which Mendix rejects separately.
	chainOf := func(name string) map[string]bool {
		seen := map[string]bool{}
		for n := name; n != "" && !seen[n]; n = extends[n] {
			seen[n] = true
		}
		return seen
	}
	var out []linter.Violation
	for _, s := range prog.Statements {
		a, ok := s.(*ast.CreateAssociationStmt)
		if !ok || a.Owner != ast.OwnerBoth {
			continue
		}
		end := func(q ast.QualifiedName) string {
			if q.Module == "" {
				q.Module = a.Name.Module
			}
			return strings.ToLower(q.String())
		}
		from, to := end(a.Parent), end(a.Child)
		if from != to && !chainOf(from)[to] && !chainOf(to)[from] {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   associationOwnerBothRule,
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"association %s has owner both but %s and %s are the same entity or one extends the other — "+
					"Mendix rejects it (CE0002: an association owned by both sides cannot connect an entity to itself or to its generalization)",
				a.Name.String(), a.Parent.String(), a.Child.String()),
			Location:   linter.Location{DocumentType: "association", DocumentName: a.Name.String()},
			Suggestion: "Use the default owner (`owner default`) or `owner parent`, or relate two unrelated entities.",
		})
	}
	return out
}
