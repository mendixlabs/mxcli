// SPDX-License-Identifier: Apache-2.0

package adapters

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/exprcheck"
	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
)

func TestCheckAdapter_ConvertsHintsToViolations(t *testing.T) {
	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "M", Name: "F"},
		Body: []ast.MicroflowStatement{},
	}
	v := NewCheckAdapter(nil)
	out := v.CheckMicroflow(stmt)
	var got []linter.Violation = out.AsViolations()
	if len(got) != 0 {
		t.Errorf("empty microflow should produce 0 violations, got %d", len(got))
	}
}

// catalogStub records every AttributeKind call so the test can assert
// the adapter passed (entity, attr) precisely, not the placeholder ("","").
type catalogStub struct {
	calls []string
}

func (c *catalogStub) AttributeKind(entity, attr string) (exprcheck.TypeKind, bool) {
	c.calls = append(c.calls, entity+"|"+attr)
	return exprcheck.KindUnknown, false
}
func (c *catalogStub) AttributeEnumQN(string, string) (string, bool) { return "", false }
func (c *catalogStub) EnumCases(string) ([]string, bool)             { return nil, false }
func (c *catalogStub) MicroflowReturn(string) (exprcheck.TypeKind, bool) {
	return exprcheck.KindUnknown, false
}
func (c *catalogStub) MicroflowParam(string, string) (exprcheck.TypeKind, bool) {
	return exprcheck.KindUnknown, false
}

func TestCheckAdapter_CreateItemEmbedsEntityAttrInSlotPath(t *testing.T) {
	stub := &catalogStub{}
	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "M", Name: "F"},
		Body: []ast.MicroflowStatement{
			&ast.CreateObjectStmt{
				Variable:   "C",
				EntityType: ast.QualifiedName{Module: "Sales", Name: "Customer"},
				Changes: []ast.ChangeItem{
					{Attribute: "Status", Value: &ast.SourceExpr{Source: "'Active'"}},
				},
			},
		},
	}
	v := NewCheckAdapter(stub)
	v.CheckMicroflow(stmt)
	if len(stub.calls) == 0 {
		t.Fatalf("catalog never queried; adapter did not embed entity.attr in SlotPath")
	}
	want := "Sales.Customer|Status"
	var saw bool
	for _, c := range stub.calls {
		if c == want {
			saw = true
			break
		}
	}
	if !saw {
		t.Errorf("expected catalog query %q, got %+v", want, stub.calls)
	}
}
