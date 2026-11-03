// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

// mendixlabs/mxcli#1733 (ako/mxcli#1935): after `alter entity … rename attribute
// Reference to CustomerReference`, the stored member points at
// MyFirstModule.Booking.CustomerReference but keeps OriginalName/ExposedName
// "Reference" until Studio Pro refreshes it. Describe printed `Reference,` —
// the stored element name as if it were the attribute — and exec of that
// output failed: `MyFirstModule.Booking has no attribute "Reference"`.
//
// The member must be printed by the attribute it references, with the stored
// exposed name kept: `CustomerReference as 'Reference'`.
func TestDescribeMessageDefinitionNamesTheReferencedAttribute(t *testing.T) {
	mod := mkModule("MyFirstModule")
	c := &model.MessageDefinitionCollection{
		BaseElement: model.BaseElement{ID: nextID("mdc")},
		ContainerID: mod.ID,
		Name:        "MD_Booking",
		Definitions: []*model.MessageDefinition{
			{Name: "BookingMessage", Root: &model.MessageDefinitionElement{Kind: "Entity", Entity: "MyFirstModule.Booking", ExposedName: "Booking",
				Children: []*model.MessageDefinitionElement{
					// as stored after the rename
					{Kind: "Attribute", Attribute: "MyFirstModule.Booking.CustomerReference", OriginalName: "Reference", ExposedName: "Reference"},
					// control: an unrenamed member still prints bare
					{Kind: "Attribute", Attribute: "MyFirstModule.Booking.Code", OriginalName: "Code", ExposedName: "Code"},
				}}},
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, c.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListMessageDefinitionCollectionsFunc: func() ([]*model.MessageDefinitionCollection, error) {
			return []*model.MessageDefinitionCollection{c}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execDescribeMessageDefinitionCollection(ctx, ast.QualifiedName{Module: "MyFirstModule", Name: "MD_Booking"}))
	out := buf.String()

	if !strings.Contains(out, "CustomerReference as 'Reference',") {
		t.Errorf("describe should name the referenced attribute and keep the exposed name:\n%s", out)
	}
	if !strings.Contains(out, "    Code\n") {
		t.Errorf("an unrenamed member should print bare:\n%s", out)
	}

	s := findStmt[*ast.CreateMessageDefinitionCollectionStmt](t, reparse(t, out), out)
	m := s.Definitions[0].Members[0]
	if m.Attribute != "CustomerReference" || m.ExposedName != "Reference" {
		t.Errorf("reparsed member = attribute %q exposed %q, want CustomerReference / Reference", m.Attribute, m.ExposedName)
	}
}
