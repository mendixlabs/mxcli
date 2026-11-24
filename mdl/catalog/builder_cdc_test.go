// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

// A change data capture service is an EXIT from the model: nothing in the app
// calls it, and the entity whose changes it publishes is load-bearing for a
// consumer mxcli cannot see. Before these tables and edges, `refs`, `impact`
// and `show callers` read a CDC-only entity as unused — so a rename, a drop or
// a major bump silently moved or ended a Kafka topic (ako/mxcli#2384 A2).

const cdcModuleID = model.ID("mod-cdc")

// cdcFixture is the service the TestApp measurements were taken from: one
// document, two tracked entities, attributes and one association.
func cdcFixture() *model.ChangeDataCaptureService {
	svc := &model.ChangeDataCaptureService{
		ContainerID: cdcModuleID,
		Name:        "OrdersCDCService",
		ServiceName: "OrdersCDCService",
		AppName:     "TestApp",
	}
	svc.ID = "svc-1"

	orderId := &model.ChangeProperty{ExposedName: "OrderId", Attribute: "Sales.Order.OrderId"}
	orderId.ID = "p-1"
	orderCustomer := &model.ChangeProperty{ExposedName: "Order_Customer", Association: "Sales.Order_Customer"}
	orderCustomer.ID = "p-2"
	order := &model.ChangeSource{Entity: "Sales.Order", ExposedName: "Order",
		VersionedSourceID: "vs-1", Properties: []*model.ChangeProperty{orderId, orderCustomer}}
	order.ID = "src-1"

	name := &model.ChangeProperty{ExposedName: "Name", Attribute: "Sales.Customer.Name"}
	name.ID = "p-3"
	customer := &model.ChangeSource{Entity: "Sales.Customer", ExposedName: "Customer",
		VersionedSourceID: "vs-2", Properties: []*model.ChangeProperty{name}}
	customer.ID = "src-2"

	svc.Sources = []*model.ChangeSource{order, customer}

	vsOrder := &model.VersionedChangeSource{EntityGUID: "guid-order", ExposedName: "Order",
		Version: &model.ChangeVersion{Major: 1, Minor: 0}}
	vsOrder.ID = "vs-1"
	vsCustomer := &model.VersionedChangeSource{EntityGUID: "guid-customer", ExposedName: "Customer",
		Version: &model.ChangeVersion{Major: 2, Minor: 3}}
	vsCustomer.ID = "vs-2"
	svc.Baseline = &model.ChangeBaseline{
		Version: &model.ChangeVersion{Major: 1, Minor: 4},
		Sources: []*model.VersionedChangeSource{vsOrder, vsCustomer},
	}
	return svc
}

// cdcCatalog runs the real builder over the given services.
func cdcCatalog(t *testing.T, services ...*model.ChangeDataCaptureService) *Catalog {
	t.Helper()
	cat, err := New()
	if err != nil {
		t.Fatalf("new catalog: %v", err)
	}
	t.Cleanup(func() { cat.Close() })

	tx, err := cat.db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	b := &Builder{
		catalog:  cat,
		reader:   &mock.MockBackend{ListChangeDataCaptureServicesFunc: func() ([]*model.ChangeDataCaptureService, error) { return services, nil }},
		snapshot: &Snapshot{ID: "snap-1"},
		hierarchy: &hierarchy{
			moduleIDs:       map[model.ID]bool{cdcModuleID: true},
			moduleNames:     map[model.ID]string{cdcModuleID: "Sales"},
			containerParent: map[model.ID]model.ID{},
			folderNames:     map[model.ID]string{},
		},
		tx: wrapTx(tx),
	}
	if err := b.buildChangeDataCaptureServices(); err != nil {
		t.Fatalf("buildChangeDataCaptureServices: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return cat
}

func TestCatalogRecordsChangeDataCaptureService(t *testing.T) {
	cat := cdcCatalog(t, cdcFixture())

	rows := queryRows(t, cat, `SELECT QualifiedName, ModuleName, ServiceName, AppName, Revision,
		SourceCount, PropertyCount FROM change_data_capture_services`)
	if len(rows) != 1 {
		t.Fatalf("want one service row, got %d", len(rows))
	}
	got := rows[0]
	for i, want := range []any{"Sales.OrdersCDCService", "Sales", "OrdersCDCService", "TestApp", "1.4"} {
		if got[i] != want {
			t.Errorf("column %d: got %v, want %v", i, got[i], want)
		}
	}
	// Revision is the BASELINE's version — what mxbuild compares against —
	// not the highest source version (2.3 here).
	if got[4] == "2.3" {
		t.Errorf("Revision is the baseline's version, not a source's")
	}
	if got[5] != int64(2) || got[6] != int64(3) {
		t.Errorf("got %v sources and %v properties, want 2 and 3", got[5], got[6])
	}
}

// The topic is the row's reason for existing: it is assembled from the service's
// AppName, the source's ExposedName and the BASELINE's major for that source —
// three properties in two halves of the document. A per-source major means two
// sources of one service have different topics, which is exactly the case that
// a single service-level version would have got wrong.
func TestCatalogComputesTheCdcTopicPerSource(t *testing.T) {
	cat := cdcCatalog(t, cdcFixture())
	var got []string
	for _, row := range queryRows(t, cat,
		`SELECT Entity, Version, Topic, AttributeCount, AssociationCount FROM change_data_capture_sources`) {
		got = append(got, row[0].(string)+" "+row[1].(string)+" "+row[2].(string))
	}
	sort.Strings(got)
	want := []string{
		"Sales.Customer 2.3 cdc.TestApp.Customer.2.{space}",
		"Sales.Order 1.0 cdc.TestApp.Order.1.{space}",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %v, want %v", got, want)
	}
}

// CONTROL: a service with no baseline has no accepted version for any source,
// so there is no topic to print. A topic missing a segment is one no producer
// writes to, and storing it would be worse than storing none.
func TestCatalogLeavesTheTopicEmptyWithoutABaseline(t *testing.T) {
	svc := cdcFixture()
	svc.Baseline = nil
	cat := cdcCatalog(t, svc)
	for _, row := range queryRows(t, cat, `SELECT Topic, Version FROM change_data_capture_sources`) {
		if row[0] != "" || row[1] != "" {
			t.Errorf("want no topic and no version without a baseline, got %v", row)
		}
	}
	rows := queryRows(t, cat, `SELECT Revision FROM change_data_capture_services`)
	if len(rows) != 1 || rows[0][0] != "" {
		t.Errorf("want an empty revision without a baseline, got %v", rows)
	}
}

// The edges are what make a CDC-only entity visible to `impact` and
// `show callers`: one per exposed entity, attribute and association.
func TestCatalogRecordsExposeEdges(t *testing.T) {
	cat := cdcCatalog(t, cdcFixture())
	got := edgeRows(t, cat, RefObjectChangeDataCaptureService)
	want := []string{
		"Sales.OrdersCDCService -> ASSOCIATION Sales.Order_Customer expose [Sales]",
		"Sales.OrdersCDCService -> ATTRIBUTE Sales.Customer.Name expose [Sales]",
		"Sales.OrdersCDCService -> ATTRIBUTE Sales.Order.OrderId expose [Sales]",
		"Sales.OrdersCDCService -> ENTITY Sales.Customer expose [Sales]",
		"Sales.OrdersCDCService -> ENTITY Sales.Order expose [Sales]",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// CONTROL: a project with no CDC service leaves both tables empty and adds no
// edge — the tables exist, which is what a reader needs to tell "none" from
// "cannot answer".
func TestCatalogCdcTablesAreEmptyWithoutAService(t *testing.T) {
	cat := cdcCatalog(t)
	for _, q := range []string{
		`SELECT Id FROM change_data_capture_services`,
		`SELECT Id FROM change_data_capture_sources`,
	} {
		if rows := queryRows(t, cat, q); len(rows) != 0 {
			t.Errorf("%s: want no rows, got %d", q, len(rows))
		}
	}
	if got := edgeRows(t, cat, RefObjectChangeDataCaptureService); len(got) != 0 {
		t.Errorf("want no edges, got %v", got)
	}
}
