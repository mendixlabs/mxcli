//go:build roundtrip

// SPDX-License-Identifier: Apache-2.0

// External test package: avoids exprcheck → executor → adapters → exprcheck
// import cycle that an internal _test.go in package exprcheck would create
// (executor depends on exprcheck/adapters via the flowBuilder ExecAdapter
// wired in P1.13).
package exprcheck_test

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mprbackend "github.com/JordtenBulte-OLC/mxcli/mdl/backend/mpr"
	"github.com/JordtenBulte-OLC/mxcli/mdl/executor"
	"github.com/JordtenBulte-OLC/mxcli/mdl/exprcheck"
	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
)

// TestRoundTrip_DescribeProducesZeroHints walks every microflow in the
// fixture MPR, regenerates its MDL via DescribeMicroflowToString, parses
// each expression with the robust parser, and asserts no hints fire on
// known-good source. Failures reveal grammar gaps (missing parsePrimary
// branch, missing funcTable entry, missing slot constraint).
//
// Build tag `roundtrip` keeps it out of the default suite — the fixture
// requires a real .mpr on disk.
func TestRoundTrip_DescribeProducesZeroHints(t *testing.T) {
	mprPath, err := filepath.Abs("../../testdata/expr-checker/minimal.mpr")
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	be := mprbackend.New()
	if err := be.Connect(mprPath); err != nil {
		t.Skipf("fixture MPR not available at %s: %v", mprPath, err)
	}
	defer be.Disconnect()

	ctx := &executor.ExecContext{
		Context: context.Background(),
		Backend: be,
		ExecIO: executor.ExecIO{
			Output: io.Discard,
			Quiet:  true,
		},
	}
	h, err := executor.GetHierarchyForMining(ctx)
	if err != nil {
		t.Fatalf("hierarchy: %v", err)
	}
	mfs, err := be.ListMicroflows()
	if err != nil {
		t.Fatalf("list microflows: %v", err)
	}
	parser := exprcheck.NewParser()
	resolver := exprcheck.DefaultSlotResolver()

	var totalSlots, totalHints int
	for _, mf := range mfs {
		modID := h.FindModuleID(mf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == "" || mf.Name == "" {
			continue
		}
		qn := ast.QualifiedName{Module: modName, Name: mf.Name}
		mdl, err := executor.DescribeMicroflowToString(ctx, qn)
		if err != nil {
			continue
		}
		prog, errs := visitor.Build(mdl)
		if len(errs) > 0 {
			t.Errorf("parse failed for %s: %v", qn.String(), errs)
			continue
		}
		for _, st := range prog.Statements {
			mfStmt, ok := st.(*ast.CreateMicroflowStmt)
			if !ok {
				continue
			}
			visitExpressions(mfStmt, func(slotPath string, expr ast.Expression) {
				src := exprSource(expr)
				if src == "" {
					return
				}
				totalSlots++
				_, hs := parser.Parse(src, exprcheck.Context{
					SlotPath:  slotPath,
					Microflow: qn.String(),
					Slots:     resolver,
				})
				if len(hs) > 0 {
					t.Errorf("non-zero hints for %s [%s] %q: code=%s problem=%q",
						qn.String(), slotPath, src, hs[0].Code, hs[0].Problem)
					totalHints += len(hs)
				}
			})
		}
	}
	t.Logf("round-trip: %d microflow(s), %d expression slot(s), %d hint(s)", len(mfs), totalSlots, totalHints)
}

func exprSource(e ast.Expression) string {
	if se, ok := e.(*ast.SourceExpr); ok {
		return se.Source
	}
	return ""
}

// visitExpressions mirrors adapters.walkBody but stays self-contained
// so the test does not depend on adapter internals.
func visitExpressions(mf *ast.CreateMicroflowStmt, fn func(string, ast.Expression)) {
	var walk func([]ast.MicroflowStatement)
	walk = func(body []ast.MicroflowStatement) {
		for _, s := range body {
			switch x := s.(type) {
			case *ast.IfStmt:
				if x.Condition != nil {
					fn("IfStmt.Condition", x.Condition)
				}
				walk(x.ThenBody)
				walk(x.ElseBody)
			case *ast.WhileStmt:
				if x.Condition != nil {
					fn("WhileStmt.Condition", x.Condition)
				}
				walk(x.Body)
			case *ast.LoopStmt:
				walk(x.Body)
			case *ast.ReturnStmt:
				if x.Value != nil {
					fn("ReturnStmt.Value", x.Value)
				}
			case *ast.DeclareStmt:
				if x.InitialValue != nil {
					fn("DeclareStmt.InitialValue", x.InitialValue)
				}
			case *ast.MfSetStmt:
				if x.Value != nil {
					fn("MfSetStmt.Value", x.Value)
				}
			case *ast.LogStmt:
				if x.Message != nil {
					fn("LogStmt.Message", x.Message)
				}
			case *ast.CreateObjectStmt:
				entityQN := x.EntityType.String()
				for _, ci := range x.Changes {
					if ci.Value != nil {
						fn("CreateItem.Value:"+entityQN+"."+ci.Attribute, ci.Value)
					}
				}
			case *ast.ChangeObjectStmt:
				for _, ci := range x.Changes {
					if ci.Value != nil {
						fn("ChangeItem.Value", ci.Value)
					}
				}
			case *ast.CallMicroflowStmt:
				for _, a := range x.Arguments {
					if a.Value != nil {
						fn("CallArgument.Value", a.Value)
					}
				}
			case *ast.CallNanoflowStmt:
				for _, a := range x.Arguments {
					if a.Value != nil {
						fn("CallArgument.Value", a.Value)
					}
				}
			}
		}
	}
	walk(mf.Body)
}
