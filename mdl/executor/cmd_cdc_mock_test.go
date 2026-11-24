// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

// Part A2 of docs/11-proposals/PROPOSAL_kafka_event_integration.md: the read
// side of entity change data capture.
//
// The shape here is the fixture's, so the expectations are measured rather than
// invented: OrdersCDCService in TestApp, Order at 1.1 with four attributes and
// an association, Customer at 1.0 with three attributes.

func cdcMockService(modID model.ID) *model.ChangeDataCaptureService {
	order := &model.ChangeSource{
		BaseElement: model.BaseElement{ID: "src-order"},
		Entity:      "CdcPublish.Order", ExposedName: "Order",
		VersionedSourceID: "b-order",
		Properties: []*model.ChangeProperty{
			{BaseElement: model.BaseElement{ID: "p1"}, ExposedName: "OrderId", Attribute: "CdcPublish.Order.OrderId", VersionedPropertyID: "bp1"},
			{BaseElement: model.BaseElement{ID: "p2"}, ExposedName: "PriceAmount", Attribute: "CdcPublish.Order.Price", VersionedPropertyID: "bp2"},
			{BaseElement: model.BaseElement{ID: "p3"}, ExposedName: "Order_Customer", Association: "CdcPublish.Order_Customer", VersionedPropertyID: "bp3"},
		},
	}
	customer := &model.ChangeSource{
		BaseElement: model.BaseElement{ID: "src-cust"},
		Entity:      "CdcPublish.Customer", ExposedName: "Client",
		VersionedSourceID: "b-cust",
		Properties: []*model.ChangeProperty{
			{BaseElement: model.BaseElement{ID: "p4"}, ExposedName: "CustomerId", Attribute: "CdcPublish.Customer.CustomerId", VersionedPropertyID: "bp4"},
		},
	}
	svc := &model.ChangeDataCaptureService{
		BaseElement: model.BaseElement{ID: "cdc1"},
		ContainerID: modID,
		Name:        "OrdersCDCService",
		ServiceName: "OrdersCDCService",
		AppName:     "TestApp",
		Sources:     []*model.ChangeSource{order, customer},
		Baseline: &model.ChangeBaseline{
			Version: &model.ChangeVersion{Major: 1, Minor: 1},
			Sources: []*model.VersionedChangeSource{
				{
					BaseElement: model.BaseElement{ID: "b-order"},
					EntityGUID:  "057964e1-fd78-4e74-bd66-c74a06131511", ExposedName: "Order",
					Version: &model.ChangeVersion{Major: 1, Minor: 1},
					Properties: []*model.VersionedChangeProperty{
						{BaseElement: model.BaseElement{ID: "bp1"}, ExposedName: "OrderId", Type: "Int64"},
						{BaseElement: model.BaseElement{ID: "bp2"}, ExposedName: "PriceAmount", Type: "Decimal"},
						{BaseElement: model.BaseElement{ID: "bp3"}, ExposedName: "Order_Customer", Type: "ArrayOfInt64"},
					},
				},
				{
					BaseElement: model.BaseElement{ID: "b-cust"},
					EntityGUID:  "bae3f172-aebb-49a6-ad92-783f53cf5014", ExposedName: "Client",
					Version: &model.ChangeVersion{Major: 2, Minor: 0},
					Properties: []*model.VersionedChangeProperty{
						{BaseElement: model.BaseElement{ID: "bp4"}, ExposedName: "CustomerId", Type: "Int64"},
					},
				},
			},
		},
	}
	return svc
}

func cdcMockCtx(t *testing.T) (*ExecContext, func() string) {
	t.Helper()
	mod := mkModule("MyModule")
	svc := cdcMockService(mod.ID)
	h := mkHierarchy(mod)
	withContainer(h, svc.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListChangeDataCaptureServicesFunc: func() ([]*model.ChangeDataCaptureService, error) {
			return []*model.ChangeDataCaptureService{svc}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, buf.String
}

func TestListChangeDataCaptureServices_Mock(t *testing.T) {
	ctx, out := cdcMockCtx(t)
	assertNoError(t, listChangeDataCaptureServices(ctx, ""))

	s := out()
	for _, want := range []string{"MyModule.OrdersCDCService", "TestApp", "1.1"} {
		assertContainsStr(t, s, want)
	}
	// Sources and properties are counted across the whole service: 2 and 4.
	if !strings.Contains(s, "| 2 ") || !strings.Contains(s, "| 4 ") {
		t.Errorf("source/property counts missing from:\n%s", s)
	}
	// AppName is a column, not a detail: it is the topic's second segment, so
	// two services of the same name under different apps publish nowhere near
	// each other.
	assertContainsStr(t, s, "App")
}

func TestListChangeDataCaptureServices_FilterByModule(t *testing.T) {
	ctx, out := cdcMockCtx(t)
	assertNoError(t, listChangeDataCaptureServices(ctx, "NoSuchModule"))
	assertContainsStr(t, out(), "No change data capture services found in module NoSuchModule")
}

func TestDescribeChangeDataCaptureService_Mock(t *testing.T) {
	ctx, out := cdcMockCtx(t)
	assertNoError(t, describeChangeDataCaptureService(ctx, ast.QualifiedName{Module: "MyModule", Name: "OrdersCDCService"}))

	s := out()
	for _, want := range []string{
		"-- change data capture service MyModule.OrdersCDCService",
		"-- ServiceName: OrdersCDCService",
		"-- AppName: TestApp",
		"-- entity CdcPublish.Order",
		"-- entity CdcPublish.Customer as 'Client'", // exposed name shown only when it differs
		"attribute   CdcPublish.Order.OrderId",
		"attribute   CdcPublish.Order.Price as 'PriceAmount'",
		"association CdcPublish.Order_Customer",
		"[Int64]", "[Decimal]", "[ArrayOfInt64]",
	} {
		assertContainsStr(t, s, want)
	}

	// The topic is what a lake or a subscriber needs, and every segment but the
	// last is a model value — including the MAJOR, so Customer at 2.0 publishes
	// to a different topic from Customer at 1.x. Printing the revision without
	// the topic would leave the reader to know that rule.
	assertContainsStr(t, s, "revision 1.1   topic cdc.TestApp.Order.1.{space}")
	assertContainsStr(t, s, "revision 2.0   topic cdc.TestApp.Client.2.{space}")

	// It must not pretend to be executable: there is no CREATE for this
	// document yet, so an LLM reading the output has to be told.
	assertContainsStr(t, s, "no CREATE statement for this document yet")
}

// A name that is not there names the ones that are: the likeliest slip is the
// module prefix, and a bare "not found" sends the reader after a missing
// document instead of a missing prefix.
func TestDescribeChangeDataCaptureService_NotFoundNamesTheKnownOnes(t *testing.T) {
	ctx, _ := cdcMockCtx(t)
	err := describeChangeDataCaptureService(ctx, ast.QualifiedName{Module: "MyModule", Name: "Nope"})
	if err == nil {
		t.Fatal("want an error for a service that does not exist")
	}
	if !strings.Contains(err.Error(), "MyModule.OrdersCDCService") {
		t.Errorf("error does not name the known service: %v", err)
	}
}

// A service with no baseline still describes: mxbuild reports CE1585 for it, so
// a reader has to be able to see that state rather than crash on it.
func TestDescribeChangeDataCaptureService_NoBaseline(t *testing.T) {
	mod := mkModule("MyModule")
	svc := cdcMockService(mod.ID)
	svc.Baseline = nil
	h := mkHierarchy(mod)
	withContainer(h, svc.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListChangeDataCaptureServicesFunc: func() ([]*model.ChangeDataCaptureService, error) {
			return []*model.ChangeDataCaptureService{svc}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeChangeDataCaptureService(ctx, ast.QualifiedName{Module: "MyModule", Name: "OrdersCDCService"}))
	assertContainsStr(t, buf.String(), "CE1585")
}
