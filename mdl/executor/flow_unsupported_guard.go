// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// FlowUnsupportedRule is the refusal of a rebuild that would delete activities
// MDL cannot express.
//
// The reader keeps an action it has no mapping for as a type name only;
// describe prints `-- Unsupported action: <$Type>`, which no statement
// recreates. Rebuilding such a flow from MDL — a drop + create from its
// description (the route MDL-V1-REBUILD recommends), or the whole-flow rebuild
// `create or modify` falls back to under mdl 0 — therefore deletes those
// activities, and nothing reported it: check passed and exec said "Created"
// (mendixlabs/mxcli#1735 for Send external object, #1738 for Generate
// document). Guard, don't drop (ADR-0005): the rebuild is refused before
// anything is written. The in-place splice under mdl 1 keeps them and is not
// affected.
const FlowUnsupportedRule = "MDL-FLOW-UNSUPPORTED"

// unsupportedActions lists, sorted, the storage types of the actions in oc —
// loop bodies included — that MDL cannot express, with "×n" for repeats.
func unsupportedActions(oc *microflows.MicroflowObjectCollection) []string {
	counts := map[string]int{}
	var walk func(oc *microflows.MicroflowObjectCollection)
	walk = func(oc *microflows.MicroflowObjectCollection) {
		if oc == nil {
			return
		}
		for _, obj := range oc.Objects {
			switch o := obj.(type) {
			case *microflows.ActionActivity:
				switch a := o.Action.(type) {
				case *microflows.UnsupportedAction:
					counts[a.StorageType]++
				case *microflows.UnknownAction:
					counts[a.TypeName]++
				}
			case *microflows.LoopedActivity:
				walk(o.ObjectCollection)
			}
		}
	}
	walk(oc)
	out := make([]string, 0, len(counts))
	for typ, n := range counts {
		if n > 1 {
			typ = fmt.Sprintf("%s ×%d", typ, n)
		}
		out = append(out, typ)
	}
	sort.Strings(out)
	return out
}

// storedUnsupportedActions is unsupportedActions of the stored flow, nil when
// there is none (or it cannot be read).
func storedUnsupportedActions(ctx *ExecContext, nanoflow bool, qn ast.QualifiedName) []string {
	a, err := loadAlterFlow(ctx, &ast.AlterFlowStmt{Nanoflow: nanoflow, Name: qn})
	if err != nil || a == nil || a.mf == nil {
		return nil
	}
	return unsupportedActions(a.mf.ObjectCollection)
}

// unsupportedRebuildMessage is the refusal's text, shared by check and exec.
func unsupportedRebuildMessage(kind, name, how string, types []string) string {
	return fmt.Sprintf("%s %s holds activities MDL cannot express (%s); %s rebuilds it from MDL and would "+
		"delete them, and nothing downstream reports it. Nothing was written", kind, name, strings.Join(types, ", "), how)
}

// unsupportedRebuildAdvice says what keeps them.
func unsupportedRebuildAdvice(kind, name string) string {
	return fmt.Sprintf("Change the %s in place instead: `create or modify %s %s` under `mdl 1;` splices the change "+
		"and keeps every activity it does not touch, and `alter %s %s { … }` edits single activities. "+
		"Or change it in Studio Pro", kind, kind, name, kind, name)
}

func unsupportedRebuildViolation(kind string, qn ast.QualifiedName, how string, types []string) linter.Violation {
	return linter.Violation{
		RuleID:     FlowUnsupportedRule,
		Severity:   linter.SeverityError,
		Message:    unsupportedRebuildMessage(kind, qn.String(), how, types),
		Suggestion: unsupportedRebuildAdvice(kind, qn.String()),
		Location:   linter.Location{Module: qn.Module, DocumentType: kind, DocumentName: qn.Name},
	}
}

// checkNoUnsupportedActionsLost refuses exec's whole-flow rewrite of a stored
// flow that holds actions MDL cannot express. It runs before the write; the
// drop + create route is refused earlier, by the verdict pass, because by the
// time its create runs the drop has already deleted the flow.
func checkNoUnsupportedActionsLost(kind, qualifiedName string, oc *microflows.MicroflowObjectCollection) error {
	types := unsupportedActions(oc)
	if len(types) == 0 {
		return nil
	}
	return mdlerrors.NewUnsupported(unsupportedRebuildMessage(kind, qualifiedName, "this statement", types) +
		".\n  " + unsupportedRebuildAdvice(kind, qualifiedName) + ".")
}
