// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/internal/testutil"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/canon"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// Part C2 of PROPOSAL_kafka_event_integration.md: authoring a change data
// capture service.
//
// The subject is the 11.14 fixture, whose domain model Studio Pro authored, so
// GUID != $ID and a writer that used the $ID fails. Each test asserts that
// property of the fixture before relying on it — on an mxcli-created entity the
// two are equal from birth and the whole GUID check is vacuous (CLAUDE.md).

func cdcWriteProject(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	if err := testutil.CopyFixture(dst, "../../../testdata/testapp-views"); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	return filepath.Join(dst, "TestApp.mpr")
}

// The CDC type of every Mendix attribute type, from Studio Pro's own editor
// (see cdcTypeForAttribute). A missing case is a document mxbuild refuses, and
// guessing was what §9.3 wanted a Studio Pro export for.
func TestCdcTypeForAttribute_EveryMendixType(t *testing.T) {
	for _, tc := range []struct {
		at   domainmodel.AttributeType
		want string
	}{
		{&domainmodel.BinaryAttributeType{}, "Binary"},
		{&domainmodel.BooleanAttributeType{}, "Boolean"},
		{&domainmodel.DateTimeAttributeType{}, "DateTime"},
		// mxcli's own spelling; stores as DateTimeAttributeType because Mendix
		// has no date-only type.
		{&domainmodel.DateAttributeType{}, "DateTime"},
		{&domainmodel.DecimalAttributeType{}, "Decimal"},
		{&domainmodel.EnumerationAttributeType{}, "Enumeration"},
		{&domainmodel.IntegerAttributeType{}, "Int32"},
		{&domainmodel.LongAttributeType{}, "Int64"},
		{&domainmodel.AutoNumberAttributeType{}, "Int64"},
		{&domainmodel.StringAttributeType{}, "String"},
		{&domainmodel.HashedStringAttributeType{}, "String"},
	} {
		got, err := cdcTypeForAttribute(&domainmodel.Attribute{Type: tc.at})
		if err != nil {
			t.Errorf("%s: %v", tc.at.GetTypeName(), err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s -> %q, want %q", tc.at.GetTypeName(), got, tc.want)
		}
	}
	if _, err := cdcTypeForAttribute(&domainmodel.Attribute{}); err == nil {
		t.Error("an attribute with no type must be refused, not defaulted")
	}
}

// The GUIDs the baseline stores must be the domain model's own. This is the
// assertion the whole design turns on: the runtime keys the database on them.
func TestCreateChangeDataCaptureService_CarriesTheDomainModelGUIDs(t *testing.T) {
	proj := cdcWriteProject(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}

	// What the domain model actually stores, read independently of the writer.
	dms, err := b.ListDomainModels()
	if err != nil {
		t.Fatal(err)
	}
	var car *domainmodel.Entity
	var dmID model.ID
	for _, dm := range dms {
		for _, e := range dm.Entities {
			if e.Name == "Car" {
				car, dmID = e, dm.ID
			}
		}
	}
	if car == nil {
		t.Fatal("fixture has no MyFirstModule.Car")
	}
	raw, err := b.GetRawUnitBytes(dmID)
	if err != nil {
		t.Fatal(err)
	}
	guids := canon.ElementGUIDs(raw)

	// CONTROL: the fixture must be Studio Pro-authored, or this test cannot
	// fail for a writer that uses the $ID.
	if guids[string(car.ID)] == string(car.ID) {
		t.Fatal("Car's GUID equals its $ID: this fixture cannot detect a writer that stores the $ID")
	}
	for _, a := range car.Attributes {
		if guids[string(a.ID)] == "" {
			t.Fatalf("attribute %s has no stored GUID in the fixture", a.Name)
		}
	}

	svc := &model.ChangeDataCaptureService{
		ContainerID: mod.ID,
		Name:        "CarsCDCService",
		Sources: []*model.ChangeSource{{
			Entity: "MyFirstModule.Car",
			Properties: []*model.ChangeProperty{
				{Attribute: "ModelId"}, {Attribute: "Brand"}, {Attribute: "Model"},
			},
		}},
	}
	if err := b.CreateChangeDataCaptureService(svc); err != nil {
		t.Fatalf("CreateChangeDataCaptureService: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatal(err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })
	got, err := b2.ListChangeDataCaptureServices()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("%d services after create, want 1", len(got))
	}
	w := got[0]

	if w.AppName != "TestApp" {
		t.Errorf("AppName = %q, want the project name TestApp", w.AppName)
	}
	if w.ServiceName != "CarsCDCService" {
		t.Errorf("ServiceName = %q, want it defaulted to the document name", w.ServiceName)
	}
	if w.Baseline == nil {
		t.Fatal("no baseline written: mxbuild reports CE1585")
	}
	if len(w.Baseline.Sources) != 1 || len(w.Sources) != 1 {
		t.Fatalf("sides disagree: %d live, %d baseline", len(w.Sources), len(w.Baseline.Sources))
	}
	bs := w.Baseline.Sources[0]

	if want := guids[string(car.ID)]; bs.EntityGUID != want {
		t.Errorf("EntityGUID = %q, want the domain model's %q", bs.EntityGUID, want)
	}
	if bs.EntityGUID == string(car.ID) {
		t.Error("EntityGUID is the entity's $ID, which would detach the entity from its table")
	}
	attrGUID := map[string]string{}
	attrType := map[string]string{}
	for _, a := range car.Attributes {
		attrGUID[a.Name] = guids[string(a.ID)]
		ct, _ := cdcTypeForAttribute(a)
		attrType[a.Name] = ct
	}
	for _, p := range bs.Properties {
		if want := attrGUID[p.ExposedName]; p.PropertyGUID != want {
			t.Errorf("%s PropertyGUID = %q, want %q", p.ExposedName, p.PropertyGUID, want)
		}
		if want := attrType[p.ExposedName]; p.Type != want {
			t.Errorf("%s Type = %q, want %q", p.ExposedName, p.Type, want)
		}
		// An attribute carries the all-zero associated-entity GUID, never empty.
		if p.AssociatedEntityGUID != zeroGUID {
			t.Errorf("%s AssociatedEntityGUID = %q, want the all-zero GUID", p.ExposedName, p.AssociatedEntityGUID)
		}
	}

	// Every pointer must land on a baseline element in the same unit: mxbuild
	// does not validate them, so nothing downstream would report their loss.
	baselineIDs := map[model.ID]bool{}
	for _, s := range w.Baseline.Sources {
		baselineIDs[s.ID] = true
		for _, p := range s.Properties {
			baselineIDs[p.ID] = true
		}
	}
	if !baselineIDs[w.Sources[0].VersionedSourceID] {
		t.Error("VersionedSourcePointer does not resolve to a baseline source")
	}
	for _, p := range w.Sources[0].Properties {
		if !baselineIDs[p.VersionedPropertyID] {
			t.Errorf("%s VersionedPropertyPointer does not resolve to a baseline property", p.ExposedName)
		}
	}
	// A fresh service is 1.0 throughout.
	if got := w.Baseline.Version.String(); got != "1.0" {
		t.Errorf("service version = %q, want 1.0 on create", got)
	}
	if got := bs.Version.String(); got != "1.0" {
		t.Errorf("source version = %q, want 1.0 on create", got)
	}
}

// The written document's shape must be Studio Pro's: the same property set per
// type and the same list markers. The two sides use DIFFERENT markers (live 3,
// baseline 2) and nothing about the shapes suggests that, so it is compared
// against the real document rather than asserted from memory.
func TestCreateChangeDataCaptureService_ShapeMatchesStudioPro(t *testing.T) {
	proj := cdcWriteProject(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	mod, _ := b.GetModuleByName("MyFirstModule")
	svc := &model.ChangeDataCaptureService{
		ContainerID: mod.ID,
		Name:        "CarsCDCService",
		Sources: []*model.ChangeSource{{
			Entity:     "MyFirstModule.Car",
			Properties: []*model.ChangeProperty{{Attribute: "Brand"}},
		}},
	}
	if err := b.CreateChangeDataCaptureService(svc); err != nil {
		t.Fatalf("create: %v", err)
	}
	written, err := b.GetRawUnit(svc.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	refBytes, err := os.ReadFile("testdata/cdc-OrdersCDCService.json")
	if err != nil {
		t.Fatal(err)
	}
	var ref map[string]any
	if err := json.Unmarshal(refBytes, &ref); err != nil {
		t.Fatal(err)
	}

	refKeys, refMarkers := cdcShape(ref)
	gotKeys, gotMarkers := cdcShape(written)

	for typ, want := range refKeys {
		got, ok := gotKeys[typ]
		if !ok {
			continue // a type this small service does not use (e.g. AssociationProperty)
		}
		if !sameStrings(got, want) {
			t.Errorf("%s property set:\n got  %v\n want %v", typ, got, want)
		}
	}
	for key, want := range refMarkers {
		if got, ok := gotMarkers[key]; ok && got != want {
			t.Errorf("%s list marker = %d, want %d", key, got, want)
		}
	}
	// The markers that matter most, named so a failure says which side broke.
	for key, want := range map[string]int{
		"ChangeDataCapture$EntityChangeDataCaptureService.Sources":          3,
		"ChangeDataCapture$EntityChangeSource.Properties":                   3,
		"ChangeDataCapture$VersionedEntityChangeDataCaptureService.Sources": 2,
		"ChangeDataCapture$VersionedEntityChangeSource.Properties":          2,
	} {
		if got, ok := gotMarkers[key]; !ok || got != want {
			t.Errorf("%s marker = %d (present %v), want %d", key, got, ok, want)
		}
	}
}

// cdcShape collects, per $Type, the sorted property names and the list marker
// of each list property.
func cdcShape(doc map[string]any) (map[string][]string, map[string]int) {
	keys := map[string][]string{}
	markers := map[string]int{}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			typ, _ := x["$Type"].(string)
			if typ != "" {
				if _, seen := keys[typ]; !seen {
					var ks []string
					for k := range x {
						ks = append(ks, k)
					}
					keys[typ] = sortedCopy(ks)
				}
			}
			for k, vv := range x {
				if lst, ok := vv.([]any); ok && len(lst) > 0 {
					if m, ok := asInt(lst[0]); ok && typ != "" {
						markers[typ+"."+k] = m
					}
				}
				walk(vv)
			}
		case []any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(doc)
	return keys, markers
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func asInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int32:
		return int(x), true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	}
	return 0, false
}

// The version policy, as a unit over bumpCdcVersions: §5.2's rule is INFERRED
// from one Studio Pro document (§9.2), and the major is a topic segment, so a
// wrong bump silently starts a new topic rather than failing a build.
func TestBumpCdcVersions_Policy(t *testing.T) {
	v := func(ma, mi int) *model.ChangeVersion { return &model.ChangeVersion{Major: ma, Minor: mi} }
	stored := &model.ChangeBaseline{
		Version: v(1, 1),
		Sources: []*model.VersionedChangeSource{
			{ExposedName: "Order", Version: v(1, 1), Properties: []*model.VersionedChangeProperty{
				{ExposedName: "OrderId", Type: "Int64"}, {ExposedName: "Price", Type: "Decimal"},
			}},
			{ExposedName: "Customer", Version: v(1, 0), Properties: []*model.VersionedChangeProperty{
				{ExposedName: "CustomerId", Type: "Int64"},
			}},
		},
	}
	src := func(name string, members ...[2]string) resolvedSource {
		rs := resolvedSource{exposedName: name}
		for _, m := range members {
			rs.members = append(rs.members, resolvedMember{exposedName: m[0], cdcType: m[1]})
		}
		return rs
	}
	order := func(ms ...[2]string) resolvedSource { return src("Order", ms...) }
	customer := src("Customer", [2]string{"CustomerId", "Int64"})

	for _, tc := range []struct {
		name        string
		sources     []resolvedSource
		wantOrder   string
		wantService string
	}{
		{"unchanged keeps its version",
			[]resolvedSource{order([2]string{"OrderId", "Int64"}, [2]string{"Price", "Decimal"}), customer},
			"1.1", "1.1"},
		{"an addition is a minor bump",
			[]resolvedSource{order([2]string{"OrderId", "Int64"}, [2]string{"Price", "Decimal"}, [2]string{"Note", "String"}), customer},
			"1.2", "1.2"},
		{"a removal is a major bump, and a new topic",
			[]resolvedSource{order([2]string{"OrderId", "Int64"}), customer},
			"2.0", "2.0"},
		{"a type change is a major bump: a consumer cannot absorb it",
			[]resolvedSource{order([2]string{"OrderId", "Int64"}, [2]string{"Price", "String"}), customer},
			"2.0", "2.0"},
		{"a renamed exposed name reads as a removal plus an addition, so major",
			[]resolvedSource{order([2]string{"OrderId", "Int64"}, [2]string{"Amount", "Decimal"}), customer},
			"2.0", "2.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := bumpCdcVersions(stored, tc.sources, nil)
			ov := got.source["Order"]
			if g := ov.String(); g != tc.wantOrder {
				t.Errorf("Order version = %s, want %s", g, tc.wantOrder)
			}
			sv := got.service
			if g := sv.String(); g != tc.wantService {
				t.Errorf("service version = %s, want %s", g, tc.wantService)
			}
			// An untouched source must not be dragged along by another's bump:
			// its topic would change for no reason.
			cv := got.source["Customer"]
			if g := cv.String(); g != "1.0" {
				t.Errorf("Customer version = %s, want it left at 1.0", g)
			}
		})
	}
}

// A source the stored baseline does not have starts at 1.0 and does not bump
// the service: it publishes to a topic of its own, so no existing consumer is
// affected.
func TestBumpCdcVersions_NewSourceDoesNotBumpTheService(t *testing.T) {
	stored := &model.ChangeBaseline{
		Version: &model.ChangeVersion{Major: 3, Minor: 2},
		Sources: []*model.VersionedChangeSource{{
			ExposedName: "Order",
			Version:     &model.ChangeVersion{Major: 3, Minor: 2},
			Properties:  []*model.VersionedChangeProperty{{ExposedName: "OrderId", Type: "Int64"}},
		}},
	}
	got := bumpCdcVersions(stored, []resolvedSource{
		{exposedName: "Order", members: []resolvedMember{{exposedName: "OrderId", cdcType: "Int64"}}},
		{exposedName: "Invoice", members: []resolvedMember{{exposedName: "InvoiceId", cdcType: "Int64"}}},
	}, nil)
	iv := got.source["Invoice"]
	if g := iv.String(); g != "1.0" {
		t.Errorf("new source = %s, want 1.0", g)
	}
	sv := got.service
	if g := sv.String(); g != "3.2" {
		t.Errorf("service = %s, want it unmoved at 3.2", g)
	}
}

// A member that is neither an attribute nor an association is refused, and so
// is a non-persistable entity: CDC publishes committed database changes, so
// there is nothing for it to observe on a non-persistent entity.
func TestCreateChangeDataCaptureService_RefusesWhatCannotBeTracked(t *testing.T) {
	proj := cdcWriteProject(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	mod, _ := b.GetModuleByName("MyFirstModule")

	for _, tc := range []struct {
		name, entity, member, wantIn string
	}{
		{"unknown member", "MyFirstModule.Car", "Nope", "not an attribute or an association"},
		{"unknown entity", "MyFirstModule.Nope", "Brand", "not found"},
		// VCar is a view entity. It reports Persistable = true, so the flag
		// alone would let it through -- Source is what tells it apart.
		{"view entity", "MyFirstModule.VCar", "Brand", "no table of its own"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := b.CreateChangeDataCaptureService(&model.ChangeDataCaptureService{
				ContainerID: mod.ID, Name: "Zz" + tc.name,
				Sources: []*model.ChangeSource{{
					Entity:     tc.entity,
					Properties: []*model.ChangeProperty{{Attribute: tc.member}},
				}},
			})
			if err == nil {
				t.Fatalf("%s was accepted", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantIn) {
				t.Errorf("error %q does not mention %q", err, tc.wantIn)
			}
		})
	}
}
