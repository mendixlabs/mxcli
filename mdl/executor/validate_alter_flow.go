// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ValidateAlterFlow runs the annotation rules (MDL059, MDL060, MDL079, MDL092)
// over the fragments an `alter microflow|nanoflow` inserts or replaces.
//
// A fragment is built by the same flowBuilder as a create body, which drops an
// annotation it cannot read — the visitor records why, and only the validator
// says so. The rules were run over create bodies alone, so
// `@annotation('text')` in an `insert` passed check and exec ("Altered
// microflow") and no note was written, while the same line in `create or
// modify` was refused (mendixlabs/mxcli#1597).
//
// Only the annotation rules: the rest of the microflow rule set reads the whole
// flow — its parameters, its declares, what a later statement uses — and a
// fragment is a piece of a stored flow that this pass cannot see.
//
// The note labels are checked per fragment, as each fragment's builder keeps
// them: an `@annotation(id: n1)` no earlier mention in the fragment declares
// names nothing — the labels describe prints are not stored — and was left to
// exec to refuse, check passing it as "not predicted" (ako/mxcli#1695).
func ValidateAlterFlow(s *ast.AlterFlowStmt) []linter.Violation {
	v := &microflowValidator{mfName: s.Name.String(), docType: s.Kind(), fragment: true}
	for _, op := range s.Operations {
		v.walkAnnotations(op.Body)
		v.checkAnnotationLabels(op.Body)
		// One statement at a time, so a fragment is judged as a whole flow
		// is (MDL-TEMPLATE01, ako/mxcli#1683).
		v.checkMessageTemplates(op.Body)
		v.checkTemplateGrammar(op.Body) // MDL-TEMPLATE02, ako/mxcli#1397
	}
	return v.violations
}

// validateAlterFlowRules is validateMicroflowRules for an alter: exec refuses
// what ValidateAlterFlow reports, restricted to the same verified allowlist, so
// `exec -c` and the REPL — which skip the check preflight — refuse what a
// create body would have.
func validateAlterFlowRules(s *ast.AlterFlowStmt) error {
	var msgs []string
	for _, v := range ValidateAlterFlow(s) {
		if v.Severity != linter.SeverityError || !execEnforcedMicroflowRules[v.RuleID] {
			continue
		}
		msg := fmt.Sprintf("[%s] %s", v.RuleID, v.Message)
		if v.Suggestion != "" {
			msg += "\n    " + v.Suggestion
		}
		msgs = append(msgs, msg)
	}
	if len(msgs) == 0 {
		return nil
	}
	return mdlerrors.NewValidationf("alter %s '%s' has validation errors:\n  - %s",
		s.Kind(), s.Name.String(), strings.Join(msgs, "\n  - "))
}
