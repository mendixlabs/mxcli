// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// A reverse Reference traversal used as a LIST over an owner="both" association
// is the one case that must fall back to a DatabaseRetrieveSource: a reverse
// AssociationRetrieveSource over an owner-both Reference resolves to a single
// object in Mendix (CE0100 when fed to a loop/aggregate), so there is no
// in-memory way to obtain the list. (Owner-default reverse traversals stay
// association retrieves — see the DefaultOwner test — which is what keeps memory
// and database retrieves distinct for issue #726.)
func TestAddRetrieveAction_ReverseReferenceOwnerBothListUsesDatabaseSource(t *testing.T) {
	fb := newRetrieveAssociationFlowBuilder(domainmodel.AssociationOwnerBoth)
	fb.varTypes["Child"] = "Sample.Child"
	fb.listInputVariables = map[string]bool{"Parents": true}

	fb.addRetrieveAction(&ast.RetrieveStmt{
		Variable:      "Parents",
		StartVariable: "Child",
		Source:        ast.QualifiedName{Module: "Sample", Name: "Parent_Child"},
	})

	action := onlyRetrieveAction(t, fb)
	source, ok := action.Source.(*microflows.DatabaseRetrieveSource)
	if !ok {
		t.Fatalf("owner-both reverse list retrieve source = %T, want DatabaseRetrieveSource", action.Source)
	}
	if source.EntityQualifiedName != "Sample.Parent" || source.XPathConstraint != "[Sample.Parent_Child = $Child]" {
		t.Fatalf("database source = %#v", source)
	}
	if got := fb.varTypes["Parents"]; got != "List of Sample.Parent" {
		t.Fatalf("result var type = %q, want List of Sample.Parent", got)
	}
}

func TestAddRetrieveAction_ReverseReferenceOwnerBothObjectUsagePreservesAssociationSource(t *testing.T) {
	fb := newRetrieveAssociationFlowBuilder(domainmodel.AssociationOwnerBoth)
	fb.varTypes["Child"] = "Sample.Child"
	fb.objectInputVariables = map[string]bool{"Parent": true}

	fb.addRetrieveAction(&ast.RetrieveStmt{
		Variable:      "Parent",
		StartVariable: "Child",
		Source:        ast.QualifiedName{Module: "Sample", Name: "Parent_Child"},
	})

	action := onlyRetrieveAction(t, fb)
	source, ok := action.Source.(*microflows.AssociationRetrieveSource)
	if !ok {
		t.Fatalf("owner-both object reverse retrieve source = %T, want AssociationRetrieveSource", action.Source)
	}
	if source.StartVariable != "Child" || source.AssociationQualifiedName != "Sample.Parent_Child" {
		t.Fatalf("association source = %#v", source)
	}
	if got := fb.varTypes["Parent"]; got != "Sample.Parent" {
		t.Fatalf("result var type = %q, want Sample.Parent", got)
	}
}

func TestAddRetrieveAction_ReverseReferenceDefaultOwnerUsesAssociationSource(t *testing.T) {
	fb := newRetrieveAssociationFlowBuilder(domainmodel.AssociationOwnerDefault)
	fb.varTypes["Child"] = "Sample.Child"

	fb.addRetrieveAction(&ast.RetrieveStmt{
		Variable:      "Parents",
		StartVariable: "Child",
		Source:        ast.QualifiedName{Module: "Sample", Name: "Parent_Child"},
	})

	action := onlyRetrieveAction(t, fb)
	source, ok := action.Source.(*microflows.AssociationRetrieveSource)
	if !ok {
		t.Fatalf("default-owner reverse retrieve source = %T, want AssociationRetrieveSource", action.Source)
	}
	if source.StartVariable != "Child" || source.AssociationQualifiedName != "Sample.Parent_Child" {
		t.Fatalf("association source = %#v", source)
	}
	if got := fb.varTypes["Parents"]; got != "List of Sample.Parent" {
		t.Fatalf("result var type = %q, want List of Sample.Parent", got)
	}
}

func TestAddRetrieveAction_ReverseReferenceNonPersistableParentUsesAssociationSource(t *testing.T) {
	fb := newRetrieveAssociationFlowBuilderWithPersistability(domainmodel.AssociationOwnerDefault, false, true)
	fb.varTypes["Child"] = "Sample.Child"

	fb.addRetrieveAction(&ast.RetrieveStmt{
		Variable:      "Parents",
		StartVariable: "Child",
		Source:        ast.QualifiedName{Module: "Sample", Name: "Parent_Child"},
	})

	action := onlyRetrieveAction(t, fb)
	source, ok := action.Source.(*microflows.AssociationRetrieveSource)
	if !ok {
		t.Fatalf("non-persistable reverse retrieve source = %T, want AssociationRetrieveSource", action.Source)
	}
	if source.StartVariable != "Child" || source.AssociationQualifiedName != "Sample.Parent_Child" {
		t.Fatalf("association source = %#v", source)
	}
	if got := fb.varTypes["Parents"]; got != "List of Sample.Parent" {
		t.Fatalf("result var type = %q, want List of Sample.Parent", got)
	}
}

func TestAddRetrieveAction_ReverseReferenceUnknownOwnerPreservesAssociationSource(t *testing.T) {
	fb := newRetrieveAssociationFlowBuilder("")
	fb.varTypes["Child"] = "Sample.Child"

	fb.addRetrieveAction(&ast.RetrieveStmt{
		Variable:      "Parents",
		StartVariable: "Child",
		Source:        ast.QualifiedName{Module: "Sample", Name: "Parent_Child"},
	})

	action := onlyRetrieveAction(t, fb)
	source, ok := action.Source.(*microflows.AssociationRetrieveSource)
	if !ok {
		t.Fatalf("unknown-owner reverse retrieve source = %T, want AssociationRetrieveSource", action.Source)
	}
	if source.StartVariable != "Child" || source.AssociationQualifiedName != "Sample.Parent_Child" {
		t.Fatalf("association source = %#v", source)
	}
	if got := fb.varTypes["Parents"]; got != "List of Sample.Parent" {
		t.Fatalf("result var type = %q, want List of Sample.Parent", got)
	}
}

func TestAddRetrieveAction_ReferenceSetRegistersOtherEntityListType(t *testing.T) {
	fb := newRetrieveAssociationFlowBuilderWithType(domainmodel.AssociationTypeReferenceSet, domainmodel.AssociationOwnerBoth, true, true)
	fb.varTypes["Parent"] = "Sample.Parent"

	fb.addRetrieveAction(&ast.RetrieveStmt{
		Variable:      "Children",
		StartVariable: "Parent",
		Source:        ast.QualifiedName{Module: "Sample", Name: "Parent_Child"},
	})

	action := onlyRetrieveAction(t, fb)
	source, ok := action.Source.(*microflows.AssociationRetrieveSource)
	if !ok {
		t.Fatalf("reference-set retrieve source = %T, want AssociationRetrieveSource", action.Source)
	}
	if source.StartVariable != "Parent" || source.AssociationQualifiedName != "Sample.Parent_Child" {
		t.Fatalf("association source = %#v", source)
	}
	if got := fb.varTypes["Children"]; got != "List of Sample.Child" {
		t.Fatalf("result var type = %q, want List of Sample.Child", got)
	}
}

func TestBuildFlowGraph_ReverseReferenceOwnerBothAttributeUsagePreservesAssociationSource(t *testing.T) {
	fb := newRetrieveAssociationFlowBuilder(domainmodel.AssociationOwnerBoth)
	fb.posX = 200
	fb.posY = 200
	fb.spacing = HorizontalSpacing
	fb.varTypes["Child"] = "Sample.Child"
	stmts := []ast.MicroflowStatement{
		&ast.RetrieveStmt{
			Variable:      "Parent",
			StartVariable: "Child",
			Source:        ast.QualifiedName{Module: "Sample", Name: "Parent_Child"},
		},
		&ast.CallMicroflowStmt{
			MicroflowName: ast.QualifiedName{Module: "Sample", Name: "UseParent"},
			Arguments: []ast.CallArgument{
				{
					Name:  "parentName",
					Value: &ast.AttributePathExpr{Variable: "Parent", Path: []string{"Name"}},
				},
			},
		},
	}

	fb.buildFlowGraph(stmts, nil)

	action := firstRetrieveAction(t, fb)
	source, ok := action.Source.(*microflows.AssociationRetrieveSource)
	if !ok {
		t.Fatalf("owner-both attribute usage source = %T, want AssociationRetrieveSource", action.Source)
	}
	if source.StartVariable != "Child" || source.AssociationQualifiedName != "Sample.Parent_Child" {
		t.Fatalf("association source = %#v", source)
	}
	if got := fb.varTypes["Parent"]; got != "Sample.Parent" {
		t.Fatalf("result var type = %q, want Sample.Parent", got)
	}
}

// Loop usage detected via the flow graph makes an owner-both reverse Reference a
// list consumer, so it falls back to a DatabaseRetrieveSource (see the
// OwnerBothList test for why).
func TestBuildFlowGraph_ReverseReferenceOwnerBothLoopUsageUsesDatabaseSource(t *testing.T) {
	fb := newRetrieveAssociationFlowBuilder(domainmodel.AssociationOwnerBoth)
	fb.posX = 200
	fb.posY = 200
	fb.spacing = HorizontalSpacing
	fb.varTypes["Child"] = "Sample.Child"
	stmts := []ast.MicroflowStatement{
		&ast.RetrieveStmt{
			Variable:      "Parents",
			StartVariable: "Child",
			Source:        ast.QualifiedName{Module: "Sample", Name: "Parent_Child"},
		},
		&ast.LoopStmt{
			LoopVariable: "Parent",
			ListVariable: "Parents",
			Body:         []ast.MicroflowStatement{},
		},
	}

	fb.buildFlowGraph(stmts, nil)

	action := firstRetrieveAction(t, fb)
	source, ok := action.Source.(*microflows.DatabaseRetrieveSource)
	if !ok {
		t.Fatalf("owner-both loop usage source = %T, want DatabaseRetrieveSource", action.Source)
	}
	if source.EntityQualifiedName != "Sample.Parent" || source.XPathConstraint != "[Sample.Parent_Child = $Child]" {
		t.Fatalf("database source = %#v", source)
	}
	if got := fb.varTypes["Parents"]; got != "List of Sample.Parent" {
		t.Fatalf("result var type = %q, want List of Sample.Parent", got)
	}
}

func newRetrieveAssociationFlowBuilder(owner domainmodel.AssociationOwner) *flowBuilder {
	return newRetrieveAssociationFlowBuilderWithPersistability(owner, true, true)
}

func newRetrieveAssociationFlowBuilderWithPersistability(owner domainmodel.AssociationOwner, parentPersistable, childPersistable bool) *flowBuilder {
	return newRetrieveAssociationFlowBuilderWithType(domainmodel.AssociationTypeReference, owner, parentPersistable, childPersistable)
}

func newRetrieveAssociationFlowBuilderWithType(associationType domainmodel.AssociationType, owner domainmodel.AssociationOwner, parentPersistable, childPersistable bool) *flowBuilder {
	moduleID := model.ID("sample-module")
	parentID := model.ID("parent-entity")
	childID := model.ID("child-entity")
	return &flowBuilder{
		varTypes: map[string]string{},
		backend: &mock.MockBackend{
			GetModuleByNameFunc: func(name string) (*model.Module, error) {
				if name != "Sample" {
					return nil, nil
				}
				return &model.Module{BaseElement: model.BaseElement{ID: moduleID}, Name: name}, nil
			},
			GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
				if id != moduleID {
					return nil, nil
				}
				return &domainmodel.DomainModel{
					ContainerID: moduleID,
					Entities: []*domainmodel.Entity{
						{BaseElement: model.BaseElement{ID: parentID}, Name: "Parent", Persistable: parentPersistable},
						{BaseElement: model.BaseElement{ID: childID}, Name: "Child", Persistable: childPersistable},
					},
					Associations: []*domainmodel.Association{
						{
							Name:     "Parent_Child",
							ParentID: parentID,
							ChildID:  childID,
							Type:     associationType,
							Owner:    owner,
						},
					},
				}, nil
			},
		},
	}
}

func onlyRetrieveAction(t *testing.T, fb *flowBuilder) *microflows.RetrieveAction {
	t.Helper()
	if len(fb.objects) != 1 {
		t.Fatalf("got %d objects, want 1", len(fb.objects))
	}
	activity, ok := fb.objects[0].(*microflows.ActionActivity)
	if !ok {
		t.Fatalf("got object %T, want *microflows.ActionActivity", fb.objects[0])
	}
	action, ok := activity.Action.(*microflows.RetrieveAction)
	if !ok {
		t.Fatalf("got action %T, want *microflows.RetrieveAction", activity.Action)
	}
	return action
}

func firstRetrieveAction(t *testing.T, fb *flowBuilder) *microflows.RetrieveAction {
	t.Helper()
	for _, object := range fb.objects {
		activity, ok := object.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		action, ok := activity.Action.(*microflows.RetrieveAction)
		if ok {
			return action
		}
	}
	t.Fatalf("retrieve action not found in %d objects", len(fb.objects))
	return nil
}
