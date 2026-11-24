// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/mendixlabs/mxcli/internal/testutil"
)

// Part A2 of docs/11-proposals/PROPOSAL_kafka_event_integration.md: reading an
// entity change data capture service.
//
// The fixture is the real unit Studio Pro wrote (TestApp 619864b, Mendix
// 11.16.1), inserted verbatim with AddRawUnit — so the read is exercised against
// Studio Pro's own BSON rather than against something mxcli encoded. That
// distinction is the point: an element mxcli created has GUID == $ID from birth,
// so a GUID bug is undetectable on it (CLAUDE.md). Verified on this fixture:
// every entity's GUID differs from its $ID as raw bytes, and the baseline
// references the GUID.
const (
	fixtureOrderEntityGUID    = "057964e1-fd78-4e74-bd66-c74a06131511"
	fixtureCustomerEntityGUID = "bae3f172-aebb-49a6-ad92-783f53cf5014"
	fixtureOrderIdAttrGUID    = "f3bcaa9f-4ed8-46de-9b7d-dad81425bc33"
	fixtureAssocGUID          = "4057dfad-680c-44cb-bfd2-97706b919d3e"
	zeroGUID                  = "00000000-0000-0000-0000-000000000000"
)

// cdcFixtureProject copies a project and inserts the Studio Pro-authored CDC
// unit into MyFirstModule.
//
// The subject is the 11.14 fixture, not the 11.6 one: the CDC document needs
// Baseline (11.10+), ServiceName (11.8+) and EntityChangeSource.ExposedName /
// .Properties (11.9+), and on 11.6 the storage layer refuses it outright — which
// is asserted as a control below, and is §5.2's "Gate: Mendix >= 11.10" measured
// rather than assumed.
func cdcFixtureProject(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	if err := testutil.CopyFixture(dst, "../../../testdata/testapp-views"); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	proj := filepath.Join(dst, "TestApp.mpr")
	contents, err := os.ReadFile("testdata/cdc-OrdersCDCService.mxunit")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v (%v)", err, mod)
	}
	// The unit keeps the $ID the document carries, so the unit row and the
	// document agree, as they do in a Studio Pro project.
	raw, err := base64.StdEncoding.DecodeString("XJcmnizTcUGevz8cZk2vnQ==")
	if err != nil {
		t.Fatal(err)
	}
	unitID, err := uuid.FromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.AddRawUnit(unitID.String(), string(mod.ID), "Documents",
		"ChangeDataCapture$EntityChangeDataCaptureService", contents); err != nil {
		t.Fatalf("AddRawUnit: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	return proj
}

func TestListChangeDataCaptureServices_ReadsStudioProUnit(t *testing.T) {
	proj := cdcFixtureProject(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	svcs, err := b.ListChangeDataCaptureServices()
	if err != nil {
		t.Fatalf("ListChangeDataCaptureServices: %v", err)
	}
	if len(svcs) != 1 {
		t.Fatalf("%d services, want 1", len(svcs))
	}
	svc := svcs[0]

	if svc.Name != "OrdersCDCService" || svc.ServiceName != "OrdersCDCService" {
		t.Errorf("Name/ServiceName = %q/%q", svc.Name, svc.ServiceName)
	}
	// AppName is the topic's second segment, and mxbuild reports CE1577 on an
	// empty one — so losing it is both a build error and a silent mis-topic.
	if svc.AppName != "TestApp" {
		t.Errorf("AppName = %q, want TestApp", svc.AppName)
	}

	// ── the live side ────────────────────────────────────────────────────
	if len(svc.Sources) != 2 {
		t.Fatalf("%d live sources, want 2", len(svc.Sources))
	}
	order := svc.Sources[0]
	if order.Entity != "CdcPublish.Order" || order.ExposedName != "Order" {
		t.Errorf("source 0 = %q / %q, want CdcPublish.Order / Order", order.Entity, order.ExposedName)
	}
	if len(order.Properties) != 5 {
		t.Errorf("Order has %d properties, want 5 — and never an `id`, which Studio Pro shows but does not store", len(order.Properties))
	}
	var attrs, assocs int
	for _, p := range order.Properties {
		if p.IsAssociation() {
			assocs++
			if p.Association != "CdcPublish.Order_Customer" {
				t.Errorf("association property = %q", p.Association)
			}
		} else {
			attrs++
			if p.Attribute == "" {
				t.Errorf("attribute property %q has no qualified name", p.ExposedName)
			}
		}
	}
	if attrs != 4 || assocs != 1 {
		t.Errorf("Order: %d attributes + %d associations, want 4 + 1", attrs, assocs)
	}
	// The live side points at the baseline. mxbuild does not validate these, so
	// nothing downstream would report their loss.
	if order.VersionedSourceID == "" {
		t.Error("live source has no VersionedSourceID — the pointer into the baseline was dropped")
	}

	// ── the frozen side ──────────────────────────────────────────────────
	if svc.Baseline == nil {
		t.Fatal("Baseline is nil: the half mxbuild raises CE1585 about")
	}
	if got := svc.Baseline.Version.String(); got != "1.1" {
		t.Errorf("service baseline version = %q, want 1.1", got)
	}
	if len(svc.Baseline.Sources) != 2 {
		t.Fatalf("%d baseline sources, want 2", len(svc.Baseline.Sources))
	}
	bOrder, bCustomer := svc.Baseline.Sources[0], svc.Baseline.Sources[1]
	if bOrder.EntityGUID != fixtureOrderEntityGUID {
		t.Errorf("Order EntityGUID = %q, want %q — the runtime keys the database on this", bOrder.EntityGUID, fixtureOrderEntityGUID)
	}
	if bCustomer.EntityGUID != fixtureCustomerEntityGUID {
		t.Errorf("Customer EntityGUID = %q, want %q", bCustomer.EntityGUID, fixtureCustomerEntityGUID)
	}
	if got, want := bOrder.Version.String(), "1.1"; got != want {
		t.Errorf("Order baseline version = %q, want %q", got, want)
	}
	if got, want := bCustomer.Version.String(), "1.0"; got != want {
		t.Errorf("Customer baseline version = %q, want %q", got, want)
	}

	byName := map[string]string{}    // exposed name -> property GUID
	assocGUID := map[string]string{} // exposed name -> associated entity GUID
	for _, p := range bOrder.Properties {
		byName[p.ExposedName] = p.PropertyGUID
		assocGUID[p.ExposedName] = p.AssociatedEntityGUID
	}
	if got := byName["OrderId"]; got != fixtureOrderIdAttrGUID {
		t.Errorf("OrderId PropertyGUID = %q, want %q", got, fixtureOrderIdAttrGUID)
	}
	if got := byName["Order_Customer"]; got != fixtureAssocGUID {
		t.Errorf("Order_Customer PropertyGUID = %q, want the association's GUID %q", got, fixtureAssocGUID)
	}
	// An attribute carries the all-zero associated-entity GUID; an association
	// carries the TO entity's. Reading them the same way would hide the one that
	// matters.
	if got := assocGUID["OrderId"]; got != zeroGUID && got != "" {
		t.Errorf("OrderId AssociatedEntityGUID = %q, want the all-zero GUID", got)
	}
	if got := assocGUID["Order_Customer"]; got == zeroGUID || got == "" {
		t.Errorf("Order_Customer AssociatedEntityGUID = %q, want the TO entity's GUID", got)
	}

	// The measured CDC Type values. Only these four appear in any fixture, which
	// is why a writer must refuse the rest rather than guess (§9.3).
	types := map[string]int{}
	for _, p := range bOrder.Properties {
		types[p.Type]++
	}
	for _, want := range []string{"Int64", "String", "Decimal", "ArrayOfInt64"} {
		if types[want] == 0 {
			t.Errorf("no property of CDC type %s was read; got %v", want, types)
		}
	}
}

// A project with no CDC service reads as an empty list, not an error — the
// listing is a query, and every other list behaves that way.
func TestListChangeDataCaptureServices_NoneIsEmptyNotError(t *testing.T) {
	b := New()
	if err := b.Connect(copyFixture(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	svcs, err := b.ListChangeDataCaptureServices()
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if len(svcs) != 0 {
		t.Errorf("%d services in a project that has none", len(svcs))
	}
}

// CONTROL for the version gate: the same unit, the same call, on a project below
// 11.10 is refused — and the refusal names the properties and their versions
// rather than silently storing a document Studio Pro could not open.
func TestChangeDataCaptureUnit_RefusedBelowItsVersion(t *testing.T) {
	proj := copyFixture(t) // the 11.6.6 fixture
	contents, err := os.ReadFile("testdata/cdc-OrdersCDCService.mxunit")
	if err != nil {
		t.Fatal(err)
	}
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	err = b.AddRawUnit(uuid.NewString(), string(mod.ID), "Documents",
		"ChangeDataCapture$EntityChangeDataCaptureService", contents)
	if err == nil {
		t.Fatal("an 11.6 project stored a document that needs 11.10")
	}
	for _, want := range []string{"Baseline", "11.10.0"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal does not mention %q: %v", want, err)
		}
	}
}
