// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// MDL-WF23 (ako/mxcli#2242): mx check reports CE6696 on an activity with more
// than five boundary events; check and exec let six through.
func TestWorkflowBoundaryEventCount(t *testing.T) {
	task := func(n int) string {
		var b strings.Builder
		b.WriteString(wfTemplateHead + "begin user task R 'R' outcomes 'Done' { }")
		for i := 0; i < n; i++ {
			b.WriteString(" boundary event non interrupting timer addDays([%CurrentDateTime%], 1) { }")
		}
		b.WriteString("; end workflow;")
		return b.String()
	}
	if vs := wfRuleViolations(ValidateWorkflow(parseWorkflowStmt(t, task(6))), wfBoundaryCountRule); len(vs) != 1 || !strings.Contains(vs[0].Message, "CE6696") {
		t.Errorf("six boundary events: want one CE6696 violation, got %v", vs)
	}
	// CONTROL: five is the limit, and builds.
	if vs := wfRuleViolations(ValidateWorkflow(parseWorkflowStmt(t, task(5))), wfBoundaryCountRule); len(vs) != 0 {
		t.Errorf("five boundary events were reported: %v", vs)
	}

	alter := func(n int) *ast.AlterWorkflowStmt {
		src := "alter workflow M.W { insert into Review {" + strings.Repeat(" boundary event interrupting timer addDays([%CurrentDateTime%], 7) { }", n) + " } };"
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatal(errs)
		}
		return prog.Statements[0].(*ast.AlterWorkflowStmt)
	}
	if vs := wfRuleViolations(ValidateAlterWorkflow(alter(6)), wfBoundaryCountRule); len(vs) != 1 {
		t.Errorf("six inserts onto one activity: want one violation, got %v", vs)
	}
	if vs := wfRuleViolations(ValidateAlterWorkflow(alter(5)), wfBoundaryCountRule); len(vs) != 0 {
		t.Errorf("five inserts were reported: %v", vs)
	}
}
