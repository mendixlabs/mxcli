// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// FlowUnsupportedRule is check's and exec's preflight refusal of a `create or
// modify` that exec would carry out as a whole-flow rebuild (mdl 0, the
// MDL-V1-REBUILD warning) of a flow holding activities MDL cannot express.
//
// The rebuild itself is refused by the guard in buildMicroflowFromStmt
// (validate_unsupported_actions.go), and a drop + create by
// validateDropRecreateKeepsActivities. But the rebuild guard only fires when
// exec reaches the statement, after every statement before it has been
// written — a half-applied script. Predicting it in the verdict pass refuses
// the script before anything runs, as the drop + create guard already does.
const FlowUnsupportedRule = "MDL-FLOW-UNSUPPORTED"

// unsupportedRebuildViolation is the verdict pass's report of that refusal,
// worded as exec's (unsupportedActionLossError).
func unsupportedRebuildViolation(kind string, qn ast.QualifiedName, types []string) linter.Violation {
	return linter.Violation{
		RuleID:   FlowUnsupportedRule,
		Severity: linter.SeverityError,
		Message: "exec would refuse this statement: " + unsupportedActionLossError(kind, qn.String(),
			"the whole-flow rebuild exec runs for this statement", types).Error(),
		Location: linter.Location{Module: qn.Module, DocumentType: kind, DocumentName: qn.Name},
	}
}
