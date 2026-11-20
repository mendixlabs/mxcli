// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// CheckCarriedIncludedAssociations is the check -p half of the omitted-clause
// warning (MDL-ODATA10, ako/mxcli#1914): a create or modify of a stored
// microflow whose call external action argument holds Associations to include,
// restated without `include associations ( … )`. exec keeps the stored tree
// (the #1922 carry) and says so; check says it before the write, so an author
// who deleted the clause to remove the associations learns that it did not.
//
// The pairing is the carry's — service, action and parameter name — so the
// two agree on which arguments are carried. A warning, never an error: keeping
// what Studio Pro ticked is the safe outcome, and the script may rely on it.
func (e *Executor) CheckCarriedIncludedAssociations(prog *ast.Program) []linter.Violation {
	if e == nil {
		return nil
	}
	return CheckCarriedIncludedAssociations(e.newExecContext(context.Background()), prog)
}

// CheckCarriedIncludedAssociations is the ExecContext-level entry point.
func CheckCarriedIncludedAssociations(ctx *ExecContext, prog *ast.Program) []linter.Violation {
	if ctx == nil || prog == nil || !ctx.Connected() {
		return nil
	}
	dropped := map[string]bool{}
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.DropMicroflowStmt:
			// drop + create starts from nothing: there is no stored tree to carry.
			dropped[s.Name.String()] = true
		case *ast.CreateMicroflowStmt:
			qn := s.Name.String()
			if !s.CreateOrModify || dropped[qn] {
				continue
			}
			stored := storedMicroflowObjects(ctx, s.Name)
			if stored == nil {
				continue
			}
			trees := map[string][]*microflows.IncludedAssociation{}
			for _, call := range externalActionCalls(stored) {
				for _, pm := range call.ParameterMappings {
					if len(pm.IncludedAssociations) > 0 {
						trees[externalActionKey(call)+"|"+strings.ToLower(pm.ParameterName)] = pm.IncludedAssociations
					}
				}
			}
			if len(trees) == 0 {
				continue
			}
			warned := map[string]bool{}
			forEachMicroflowStatement(s.Body, func(st ast.MicroflowStatement) {
				call, ok := st.(*ast.CallExternalActionStmt)
				if !ok {
					return
				}
				callQN := call.ServiceName.String() + "." + call.ActionName
				for _, arg := range call.Arguments {
					if arg.IncludeAssociations != nil {
						continue
					}
					key := strings.ToLower(callQN) + "|" + strings.ToLower(arg.Name)
					tree, ok := trees[key]
					if !ok || warned[key] {
						continue
					}
					warned[key] = true
					c := carriedIncludes{call: callQN, param: arg.Name, tree: formatIncludedAssociations(tree)}
					out = append(out, linter.Violation{
						RuleID:   carriedIncludesRule,
						Severity: linter.SeverityWarning,
						Message:  strings.TrimPrefix(carriedIncludesWarning(qn, c), "Warning: "),
					})
				}
			})
		}
	}
	return out
}

// storedMicroflowObjects is the live stored microflow's object collection, nil
// when the project has none of that name.
func storedMicroflowObjects(ctx *ExecContext, name ast.QualifiedName) *microflows.MicroflowObjectCollection {
	mfs, err := microflowsNamed(ctx, name.Name)
	if err != nil {
		return nil
	}
	h, err := getHierarchy(ctx)
	if err != nil || h == nil {
		return nil
	}
	mf, ok := pickLive(mfs,
		func(m *microflows.Microflow) bool {
			return m.Name == name.Name && h.GetModuleName(h.FindModuleID(m.ContainerID)) == name.Module
		},
		microflowExcluded)
	if !ok || mf == nil {
		return nil
	}
	return mf.ObjectCollection
}
