// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/exprcheck"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// validateToStringIntoShortAttribute warns about a create/change member that
// writes `toString(<Decimal>)` into a String attribute with a length limit
// (mendixlabs/mxcli#1515 item 2, ako/mxcli#1397).
//
// A Decimal such as minutesBetween($a, $b) or `$x div 3` is rendered with its
// whole fraction: 43 characters were seen at runtime, which overflows a
// String(10) on commit. It builds — mxbuild does not know the value's length —
// so this warns rather than refuses, and only where both halves are
// established: the value is provably a Decimal, and the attribute is provably
// bounded. Anything unknown stays silent.
//
// It runs in the --references pass because the bound is the attribute's, which
// only the project or the script's own CREATE ENTITY knows.
func validateToStringIntoShortAttribute(ctx *ExecContext, prog *ast.Program, sc *scriptContext) {
	if prog == nil || sc == nil {
		return
	}
	for _, stmt := range prog.Statements {
		var flowQN string
		var params []ast.MicroflowParam
		var body []ast.MicroflowStatement
		switch s := stmt.(type) {
		case *ast.CreateMicroflowStmt:
			flowQN, params, body = s.Name.String(), s.Parameters, s.Body
		case *ast.CreateNanoflowStmt:
			flowQN, params, body = s.Name.String(), s.Parameters, s.Body
		default:
			continue
		}
		kinds := map[string]exprcheck.TypeKind{}
		for _, p := range params {
			if k, ok := astKindToExprKind(p.Type.Kind); ok {
				kinds[p.Name] = k
			}
		}
		walkTypedFlow(ctx, sc, params, body, func(st ast.MicroflowStatement, vars map[string]string) {
			var entity, what string
			var changes []ast.ChangeItem
			switch s := st.(type) {
			case *ast.DeclareStmt:
				if k, ok := astKindToExprKind(s.Type.Kind); ok {
					kinds[s.Variable] = k
				}
				return
			case *ast.CreateObjectStmt:
				entity, what, changes = s.EntityType.String(), "create "+s.EntityType.String(), s.Changes
			case *ast.ChangeObjectStmt:
				entity, what, changes = vars[s.Variable], "change $"+s.Variable, s.Changes
			default:
				return
			}
			if entity == "" {
				return
			}
			for _, ch := range changes {
				if ch.Op != ast.MemberSet {
					continue
				}
				src := microflowExprSource(ch.Value)
				if !exprcheck.ToStringOfDecimal(src, kinds) {
					continue
				}
				length := boundedStringLength(ctx, sc, entity, ch.Attribute)
				if length <= 0 {
					continue
				}
				sc.warnings = append(sc.warnings, fmt.Sprintf(
					"%s: %s sets %s, a String(%d), to %s — a Decimal is rendered with its whole "+
						"fraction (dozens of characters), which overflows the attribute when it is "+
						"committed. Round it first, e.g. toString(round(…)) or formatDecimal(…, '#0.00').",
					flowQN, what, ch.Attribute, length, src))
			}
		})
	}
}

// boundedStringLength returns the length limit of entity's String attribute
// attr, or 0 when it is unlimited, not a String, or cannot be established. The
// script's own declaration wins over the stored entity, as it does on exec.
func boundedStringLength(ctx *ExecContext, sc *scriptContext, entityQN, attr string) int {
	if decl := sc.entityDecls[entityQN]; decl != nil {
		for _, a := range decl.Attributes {
			if a.Name == attr {
				if a.Type.Kind == ast.TypeString && a.Type.Length > 0 {
					return a.Type.Length
				}
				return 0
			}
		}
		if decl.Generalization == nil {
			return 0
		}
		entityQN = decl.Generalization.String()
	}
	b, ok := ctx.Backend.(entityLookupBackend)
	if !ok {
		return 0
	}
	seen := map[string]bool{}
	for qn := entityQN; qn != "" && !seen[qn]; {
		seen[qn] = true
		ent, found := findEntityByQN(b, qn)
		if !found {
			return 0
		}
		for _, a := range ent.Attributes {
			if a == nil || a.Name != attr {
				continue
			}
			if st, isString := a.Type.(*domainmodel.StringAttributeType); isString && st.Length > 0 {
				return st.Length
			}
			return 0
		}
		qn = ent.GeneralizationRef
	}
	return 0
}
