// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#2381. In Mendix 11 a *consumed* business event service is the same
// BusinessEvents$BusinessEventService document as a published one: what makes it
// a consumer is `Definition: null` plus the producer's AsyncAPI contract in
// `Document`, with a handler microflow per ServiceOperation.
//
// The rebuild writes neither, so `create or modify` over a consumer dropped the
// contract and the handler and invented a Definition — with `check --references`
// clean and `exec` reporting success. The contract is the only place the channel
// id lives, and the channel id is what binds the subscriber to the producer's
// topic at deployment, so losing it breaks the subscription.
//
// The posture is guard-don't-drop (ADR-0005): refuse the statement rather than
// quietly produce a smaller document.

const storedContract = `asyncapi: 2.2.0
info:
  title: "OrdersEventService"
  version: "1.0.0"
channels:
  f2477dc90f714d69bb12df0df1ee9cce:
    subscribe:
      operationId: receiveOrderPlaceEventEvents
`

// storedConsumer is the fixture shape: Definition nil, Document set, one
// subscribe operation bound to an entity and a handler microflow.
func storedConsumer(containerID model.ID) *model.BusinessEventService {
	return &model.BusinessEventService{
		BaseElement: model.BaseElement{ID: "sp-consumer"},
		ContainerID: containerID,
		Name:        "OrdersEventClient",
		ExportLevel: "Hidden",
		Definition:  nil,
		Document:    storedContract,
		OperationImplementations: []*model.ServiceOperation{{
			BaseElement: model.BaseElement{ID: "sp-op"},
			MessageName: "OrderPlaceEvent",
			Operation:   "subscribe",
			Entity:      "M.CBE_OrderPlaceEvent",
			Microflow:   "M.HandleBE_OrderPlaceEvent",
		}},
	}
}

func TestCreateOrModifyBusinessEventService_RefusesClobberingStoredContract(t *testing.T) {
	mod := mkModule("M")
	stored := storedConsumer(mod.ID)
	h := mkHierarchy(mod)
	withContainer(h, stored.ContainerID, mod.ID)

	var written *model.BusinessEventService
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListBusinessEventServicesFunc: func() ([]*model.BusinessEventService, error) {
			return []*model.BusinessEventService{stored}, nil
		},
		UpdateBusinessEventServiceFunc: func(svc *model.BusinessEventService) error {
			written = svc
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	err := createBusinessEventService(ctx, &ast.CreateBusinessEventServiceStmt{
		Name:           ast.QualifiedName{Module: "M", Name: "OrdersEventClient"},
		ServiceName:    "OrdersEventService",
		CreateOrModify: true,
		Messages: []*ast.BusinessEventMessageDef{{
			MessageName: "OrderPlaceEvent",
			Operation:   "subscribe",
			Entity:      "M.CBE_OrderPlaceEvent",
			Attributes: []*ast.BusinessEventAttributeDef{
				{Name: "OrderId", TypeName: "Long"},
			},
		}},
	})

	if err == nil {
		t.Fatalf("rewrite of a consumer was accepted; it must refuse.\nwritten = %+v", written)
	}
	if written != nil {
		t.Errorf("the write was attempted anyway: Document=%q Definition=%v", written.Document, written.Definition != nil)
	}
	// The message has to say what is at stake and how to proceed, or the refusal
	// is just a wall: the contract is unrecoverable from the model once gone.
	for _, want := range []string{"contract", "M.OrdersEventClient"} {
		if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(want)) {
			t.Errorf("refusal does not mention %q: %v", want, err)
		}
	}
}

// The guard must not fire on an ordinary publisher: it keys on a stored
// contract, not on the statement. Without this control the guard could refuse
// every rewrite and still pass the test above.
func TestCreateOrModifyBusinessEventService_PublisherRewriteStillAllowed(t *testing.T) {
	mod := mkModule("M")
	stored := &model.BusinessEventService{
		BaseElement: model.BaseElement{ID: "sp-publisher"},
		ContainerID: mod.ID,
		Name:        "OrdersEventService",
		ExportLevel: "Hidden",
		Document:    "", // a publisher stores no contract
		Definition: &model.BusinessEventDefinition{
			BaseElement: model.BaseElement{ID: "sp-def"},
			ServiceName: "OrdersEventService",
			Channels: []*model.BusinessEventChannel{{
				BaseElement: model.BaseElement{ID: "sp-ch"},
				ChannelName: "f2477dc90f714d69bb12df0df1ee9cce",
				Messages: []*model.BusinessEventMessage{{
					BaseElement: model.BaseElement{ID: "sp-msg"},
					MessageName: "OrderPlaceEvent",
					Attributes: []*model.BusinessEventAttribute{
						{BaseElement: model.BaseElement{ID: "sp-attr"}, AttributeName: "OrderId", AttributeType: "Long"},
					},
				}},
			}},
		},
		OperationImplementations: []*model.ServiceOperation{{
			BaseElement: model.BaseElement{ID: "sp-op"},
			MessageName: "OrderPlaceEvent",
			Operation:   "publish",
			Entity:      "M.PBE_OrderPlaceEvent",
		}},
	}
	h := mkHierarchy(mod)
	withContainer(h, stored.ContainerID, mod.ID)

	var written *model.BusinessEventService
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListBusinessEventServicesFunc: func() ([]*model.BusinessEventService, error) {
			return []*model.BusinessEventService{stored}, nil
		},
		UpdateBusinessEventServiceFunc: func(svc *model.BusinessEventService) error {
			written = svc
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	err := createBusinessEventService(ctx, &ast.CreateBusinessEventServiceStmt{
		Name:           ast.QualifiedName{Module: "M", Name: "OrdersEventService"},
		ServiceName:    "OrdersEventService",
		CreateOrModify: true,
		Messages: []*ast.BusinessEventMessageDef{{
			MessageName: "OrderPlaceEvent",
			Operation:   "publish",
			Entity:      "M.PBE_OrderPlaceEvent",
			Attributes: []*ast.BusinessEventAttributeDef{
				{Name: "OrderId", TypeName: "Long"},
			},
		}},
	})
	assertNoError(t, err)
	if written == nil {
		t.Fatal("a publisher rewrite was refused; the guard keys on the wrong thing")
	}
}

// A statement silent about the handler keeps the stored one, the way a silent
// doc comment does (#1018). MDL has had `MICROFLOW qualifiedName` on the message
// rule all along, so this is a carry gap, not a missing spelling.
func TestCreateOrModifyBusinessEventService_CarriesStoredHandlerMicroflow(t *testing.T) {
	mod := mkModule("M")
	stored := &model.BusinessEventService{
		BaseElement: model.BaseElement{ID: "sp-svc"},
		ContainerID: mod.ID,
		Name:        "OrderEvents",
		ExportLevel: "Hidden",
		Definition: &model.BusinessEventDefinition{
			BaseElement: model.BaseElement{ID: "sp-def"},
			ServiceName: "OrderEvents",
			Channels: []*model.BusinessEventChannel{{
				BaseElement: model.BaseElement{ID: "sp-ch"},
				ChannelName: "aaaabbbbccccddddeeeeffff00001111",
				Messages: []*model.BusinessEventMessage{{
					BaseElement: model.BaseElement{ID: "sp-msg"},
					MessageName: "OrderPlaced",
					Attributes: []*model.BusinessEventAttribute{
						{BaseElement: model.BaseElement{ID: "sp-attr"}, AttributeName: "OrderId", AttributeType: "Long"},
					},
				}},
			}},
		},
		OperationImplementations: []*model.ServiceOperation{{
			BaseElement: model.BaseElement{ID: "sp-op"},
			MessageName: "OrderPlaced",
			Operation:   "subscribe",
			Entity:      "M.CBE_OrderPlaced",
			Microflow:   "M.HandleBE_OrderPlaced",
		}},
	}
	h := mkHierarchy(mod)
	withContainer(h, stored.ContainerID, mod.ID)

	var written *model.BusinessEventService
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListBusinessEventServicesFunc: func() ([]*model.BusinessEventService, error) {
			return []*model.BusinessEventService{stored}, nil
		},
		UpdateBusinessEventServiceFunc: func(svc *model.BusinessEventService) error {
			written = svc
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	stmtFor := func(microflow string) *ast.CreateBusinessEventServiceStmt {
		return &ast.CreateBusinessEventServiceStmt{
			Name:           ast.QualifiedName{Module: "M", Name: "OrderEvents"},
			ServiceName:    "OrderEvents",
			CreateOrModify: true,
			Messages: []*ast.BusinessEventMessageDef{{
				MessageName: "OrderPlaced",
				Operation:   "subscribe",
				Entity:      "M.CBE_OrderPlaced",
				Microflow:   microflow,
				Attributes: []*ast.BusinessEventAttributeDef{
					{Name: "OrderId", TypeName: "Long"},
				},
			}},
		}
	}

	// Silent statement: the stored handler survives.
	assertNoError(t, createBusinessEventService(ctx, stmtFor("")))
	if written == nil || len(written.OperationImplementations) != 1 {
		t.Fatalf("no operation written: %+v", written)
	}
	if got := written.OperationImplementations[0].Microflow; got != "M.HandleBE_OrderPlaced" {
		t.Errorf("handler microflow = %q, want the stored M.HandleBE_OrderPlaced", got)
	}

	// Control: a statement that names one wins, so the carry is not pinning the
	// stored value against an authored change.
	written = nil
	assertNoError(t, createBusinessEventService(ctx, stmtFor("M.OtherHandler")))
	if got := written.OperationImplementations[0].Microflow; got != "M.OtherHandler" {
		t.Errorf("authored handler microflow = %q, want M.OtherHandler", got)
	}
}

// describe printed only `create or modify business event service M.X` for a
// consumer — no property list, no body, no `;` — because the whole statement is
// written inside `if found.Definition != nil`. `mxcli exec` of that output is a
// parse error, so the round trip was broken.
//
// Part B gave the shape a grammar, so describe now writes the statement. What
// this test pins is the property the comment form was standing in for: the
// output parses, and re-executing it restates the same service rather than
// destroying its contract.
func TestDescribeBusinessEventService_ConsumerDescribesAsParseableStatement(t *testing.T) {
	for _, lang := range []langver.Version{langver.V0, langver.V1} {
		mod := mkModule("M")
		stored := storedConsumer(mod.ID)
		h := mkHierarchy(mod)
		withContainer(h, stored.ContainerID, mod.ID)
		mb := &mock.MockBackend{
			IsConnectedFunc: func() bool { return true },
			ListBusinessEventServicesFunc: func() ([]*model.BusinessEventService, error) {
				return []*model.BusinessEventService{stored}, nil
			},
		}
		ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
		ctx.describeLang = &lang
		assertNoError(t, describeBusinessEventService(ctx, ast.QualifiedName{Module: "M", Name: "OrdersEventClient"}))

		src := buf.String()
		if lang == langver.V1 {
			src = langver.HeaderFor(langver.V1) + "\n" + src
		}
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("mdl %d: consumer describe output does not parse: %v\n%s", lang, errs, src)
		}
		var stmt *ast.CreateBusinessEventServiceStmt
		for _, s := range prog.Statements {
			if c, ok := s.(*ast.CreateBusinessEventServiceStmt); ok {
				stmt = c
			}
		}
		if stmt == nil {
			t.Fatalf("mdl %d: no statement in consumer describe:\n%s", lang, src)
		}
		// The contract must come back byte-identical. A newline added inside the
		// $$…$$ block for looks is a character the re-executed contract has and
		// the stored one does not, and describe → exec then reports Modified on
		// an unchanged service.
		if stmt.Contract != storedContract {
			t.Errorf("mdl %d: contract not byte-identical through the round trip\n got %q\nwant %q",
				lang, stmt.Contract, storedContract)
		}
		if len(stmt.Consumes) != 1 {
			t.Fatalf("mdl %d: re-parsed %d consume items, want 1\n%s", lang, len(stmt.Consumes), src)
		}
		got := stmt.Consumes[0]
		if got.MessageName != "OrderPlaceEvent" ||
			got.Entity != "M.CBE_OrderPlaceEvent" ||
			got.Microflow != "M.HandleBE_OrderPlaceEvent" {
			t.Errorf("mdl %d: subscription lost through the round trip: %+v", lang, got)
		}
	}
}

// Two consumers have no statement to write, and both have to fall back to
// comments rather than emit something that cannot be re-executed.
func TestDescribeBusinessEventService_ConsumerWithoutAStatementFallsBackToComments(t *testing.T) {
	for _, tc := range []struct {
		name, document, want string
	}{
		// DOLLAR_STRING is `'$$' .*? '$$'`, so an embedded $$ ends the block
		// early and there is no escape for it.
		{"contract containing $$", "asyncapi: 2.2.0\nsummary: a $$ b\n", "ContractFile"},
		{"no contract at all", "", "no AsyncAPI contract"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mod := mkModule("M")
			stored := storedConsumer(mod.ID)
			stored.Document = tc.document
			h := mkHierarchy(mod)
			withContainer(h, stored.ContainerID, mod.ID)
			mb := &mock.MockBackend{
				IsConnectedFunc: func() bool { return true },
				ListBusinessEventServicesFunc: func() ([]*model.BusinessEventService, error) {
					return []*model.BusinessEventService{stored}, nil
				},
			}
			ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
			assertNoError(t, describeBusinessEventService(ctx, ast.QualifiedName{Module: "M", Name: "OrdersEventClient"}))

			src := langver.HeaderFor(langver.V1) + "\n" + buf.String()
			prog, errs := visitor.Build(src)
			if len(errs) > 0 {
				t.Fatalf("fallback output does not parse: %v\n%s", errs, src)
			}
			for _, s := range prog.Statements {
				if _, ok := s.(*ast.CreateBusinessEventServiceStmt); ok {
					t.Fatalf("emitted a statement that cannot round-trip:\n%s", src)
				}
			}
			if !strings.Contains(buf.String(), tc.want) {
				t.Errorf("fallback does not say why there is no statement (want %q):\n%s", tc.want, buf.String())
			}
			// The subscriptions still have to be stated, whichever form is used.
			if !strings.Contains(buf.String(), "M.HandleBE_OrderPlaceEvent") {
				t.Errorf("fallback omits the handler:\n%s", buf.String())
			}
		})
	}
}

// A handler microflow is authorable (`MICROFLOW qualifiedName` on the message
// rule) but describe never printed one, so describe → exec dropped it on a
// publisher-shaped service too — a loss independent of the consumer case.
func TestDescribeBusinessEventService_EmitsHandlerMicroflow(t *testing.T) {
	mod := mkModule("M")
	svc := &model.BusinessEventService{
		BaseElement: model.BaseElement{ID: nextID("bes")},
		ContainerID: mod.ID,
		Name:        "OrderEvents",
		Definition: &model.BusinessEventDefinition{
			ServiceName: "OrderEvents",
			Channels: []*model.BusinessEventChannel{{
				ChannelName: "ch1",
				Messages: []*model.BusinessEventMessage{{
					MessageName: "OrderPlaced",
					Attributes:  []*model.BusinessEventAttribute{{AttributeName: "OrderId", AttributeType: "Long"}},
				}},
			}},
		},
		OperationImplementations: []*model.ServiceOperation{{
			MessageName: "OrderPlaced",
			Operation:   "subscribe",
			Entity:      "M.CBE_OrderPlaced",
			Microflow:   "M.HandleBE_OrderPlaced",
		}},
	}
	h := mkHierarchy(mod)
	withContainer(h, svc.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListBusinessEventServicesFunc: func() ([]*model.BusinessEventService, error) {
			return []*model.BusinessEventService{svc}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeBusinessEventService(ctx, ast.QualifiedName{Module: "M", Name: "OrderEvents"}))

	src := langver.HeaderFor(langver.V1) + "\n" + buf.String()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs, src)
	}
	var stmt *ast.CreateBusinessEventServiceStmt
	for _, s := range prog.Statements {
		if c, ok := s.(*ast.CreateBusinessEventServiceStmt); ok {
			stmt = c
		}
	}
	if stmt == nil {
		t.Fatalf("no business event service in:\n%s", src)
	}
	if len(stmt.Messages) != 1 {
		t.Fatalf("re-parsed %d messages, want 1\n%s", len(stmt.Messages), src)
	}
	if got := stmt.Messages[0].Microflow; got != "M.HandleBE_OrderPlaced" {
		t.Errorf("re-parsed handler microflow = %q, want M.HandleBE_OrderPlaced\n%s", got, src)
	}
}
