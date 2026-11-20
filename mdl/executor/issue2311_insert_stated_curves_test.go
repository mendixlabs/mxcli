// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#2311: a `create or modify` that inserted statements drew the
// connectors at their ends with the splice's own vectors, and the next run of
// the same script refused the flow as "redrawn". The insert now carries the
// curves the script states: the one on the statement before the run, for the
// connector into it, and the one on the run's last statement.
func TestStatedCurves_InsertCarriesTheScriptsCurves_Issue2311(t *testing.T) {
	before := &ast.LogStmt{Annotations: &ast.ActivityAnnotations{
		Curve: &ast.FlowCurve{From: &ast.Position{X: 30}, To: &ast.Position{X: -30}}}}
	last := &ast.LogStmt{}
	pd := &patchDiff{before: before}
	op := &ast.AlterFlowOperation{Op: ast.AlterFlowInsertAfter}
	pd.statedCurves(op, []ast.MicroflowStatement{&ast.LogStmt{}, last})
	if !op.StatesCurves || op.InCurve != before.Annotations.Curve || op.OutCurve != nil {
		t.Fatalf("op = %+v, want the predecessor's curve in and none (straight) out", op)
	}
	in, out := flowVectors(op.InCurve), flowVectors(op.OutCurve)
	if in.Origin != "30;0" || in.Destination != "-30;0" || out.Origin != "0;0" || out.Destination != "0;0" {
		t.Errorf("vectors in %+v out %+v", in, out)
	}
}

// CONTROL: a run at the start of a list (no statement before it) or after a
// decision states nothing about the connector into it, and keeps the splice's.
func TestStatedCurves_NotAfterADecision_Issue2311(t *testing.T) {
	for _, before := range []ast.MicroflowStatement{nil, &ast.IfStmt{}} {
		pd := &patchDiff{before: before}
		op := &ast.AlterFlowOperation{Op: ast.AlterFlowInsertBefore}
		pd.statedCurves(op, []ast.MicroflowStatement{&ast.LogStmt{}})
		if op.StatesCurves {
			t.Errorf("before %T: curves stated, want the splice's own", before)
		}
	}
}
