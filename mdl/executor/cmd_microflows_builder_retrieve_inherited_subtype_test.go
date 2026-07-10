// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

// Reverse-traversal type registration must follow the entity inheritance
// chain. When a Reference association has Parent=Header / Child=Message
// and a microflow holds a $Request variable typed as a Request entity that
// extends Message, traversing $Request/Module.Headers should be classified
// as reverse traversal — yielding `List of Header` — even though
// `Request != Message` exactly. Failing this check leaves the result
// variable typed as the parent (Message) singleton, which downstream
// list-operation builders read as a non-list and emit FindByExpression
// instead of Find with a qualified attribute, triggering CE0117 in mx
// check on Studio Pro.
func TestAddRetrieveAction_ReverseRefThroughInheritedChild(t *testing.T) {
	moduleID := model.ID("synthetic-module")
	headerID := model.ID("header-entity")
	messageID := model.ID("message-entity")
	requestID := model.ID("request-entity")
	fb := &flowBuilder{
		varTypes: map[string]string{
			// $Request is a Request, which extends Message
			"Request": "Synthetic.Request",
		},
		listInputVariables: map[string]bool{
			// Result is consumed as a list (drives the owner-both branch)
			"HeaderList": true,
		},
		backend: &mock.MockBackend{
			GetModuleByNameFunc: func(name string) (*model.Module, error) {
				if name != "Synthetic" {
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
						{
							BaseElement: model.BaseElement{ID: headerID},
							Name:        "Header",
							Persistable: true,
						},
						{
							BaseElement: model.BaseElement{ID: messageID},
							Name:        "Message",
							Persistable: true,
						},
						{
							BaseElement:       model.BaseElement{ID: requestID},
							Name:              "Request",
							Persistable:       true,
							GeneralizationRef: "Synthetic.Message",
						},
					},
					Associations: []*domainmodel.Association{
						{
							Name:     "Headers",
							ParentID: headerID,
							ChildID:  messageID,
							Type:     domainmodel.AssociationTypeReference,
							Owner:    domainmodel.AssociationOwnerDefault,
						},
					},
				}, nil
			},
		},
	}

	fb.addRetrieveAction(&ast.RetrieveStmt{
		Variable:      "HeaderList",
		StartVariable: "Request",
		Source:        ast.QualifiedName{Module: "Synthetic", Name: "Headers"},
	})

	got := fb.varTypes["HeaderList"]
	want := "List of Synthetic.Header"
	if got != want {
		t.Errorf("varTypes[HeaderList] = %q, want %q", got, want)
	}
}

// Direct exact-match (no inheritance involved) must continue to be
// classified as reverse traversal — guards against the helper accidentally
// regressing the non-inheritance path.
func TestAddRetrieveAction_ReverseRefDirectChild(t *testing.T) {
	moduleID := model.ID("synthetic-module")
	headerID := model.ID("header-entity")
	messageID := model.ID("message-entity")
	fb := &flowBuilder{
		varTypes: map[string]string{
			"Message": "Synthetic.Message",
		},
		listInputVariables: map[string]bool{
			"HeaderList": true,
		},
		backend: &mock.MockBackend{
			GetModuleByNameFunc: func(name string) (*model.Module, error) {
				if name != "Synthetic" {
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
						{BaseElement: model.BaseElement{ID: headerID}, Name: "Header", Persistable: true},
						{BaseElement: model.BaseElement{ID: messageID}, Name: "Message", Persistable: true},
					},
					Associations: []*domainmodel.Association{
						{
							Name:     "Headers",
							ParentID: headerID,
							ChildID:  messageID,
							Type:     domainmodel.AssociationTypeReference,
							Owner:    domainmodel.AssociationOwnerDefault,
						},
					},
				}, nil
			},
		},
	}

	fb.addRetrieveAction(&ast.RetrieveStmt{
		Variable:      "HeaderList",
		StartVariable: "Message",
		Source:        ast.QualifiedName{Module: "Synthetic", Name: "Headers"},
	})

	if got, want := fb.varTypes["HeaderList"], "List of Synthetic.Header"; got != want {
		t.Errorf("varTypes[HeaderList] = %q, want %q", got, want)
	}
}
