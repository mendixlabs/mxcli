// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/internal/testutil"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
)

// ako/mxcli#1805: on a Mendix 11.14.0 project mxcli wrote, `mxcli fix
// property-sets` reported
//
//	MIXED  ODataPublish$EntitySet.ReadMode: on 42 of 60 element(s); without it: … [ODataPublish$PublishedODataService2 F1AdminApi]
//	MIXED  ODataPublish$PublishedMicroflow.Parameters: on 2 of 5 element(s); only on: … [ODataPublish$PublishedODataService2 F1OpsApi]
//
// The published-service writer wrote an entity set's ReadMode, InsertMode,
// UpdateMode and DeleteMode only when the statement stated them, and an
// action's Parameters only when the microflow had any. Studio Pro writes all
// five on every element (the 11.14.0 service in testdata/testapp-views), and
// `mx convert -p -s` on 10.24.24, 11.13.0 and 11.15.0 fills an absent one with
// ReadSource, ChangeNotSupported and an empty marker-3 list — so the writer now
// writes those when the statement leaves them out, and the stored meaning of a
// service mxcli wrote before does not change.

// studioProEntitySetKeys is the property-name set Studio Pro 11.14.0 wrote on
// the entity set of MyFirstModule.CarOdataApi in testdata/testapp-views.
func studioProEntitySetKeys(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "testapp-views", "mprcontents",
		"6e", "16", "6e16f1f7-1207-4d70-8847-c369a7dd360a.mxunit"))
	if err != nil {
		t.Skipf("testdata/testapp-views not available: %v", err)
	}
	sets := bson.Raw(b).Lookup("EntitySets").Array()
	vals, err := sets.Values()
	if err != nil || len(vals) != 2 {
		t.Fatalf("subject: the Studio Pro service should hold one entity set, got %d (%v)", len(vals)-1, err)
	}
	return rawKeyNames(t, vals[1].Document())
}

func rawKeyNames(t *testing.T, d bson.Raw) []string {
	t.Helper()
	elems, err := d.Elements()
	if err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(elems))
	for _, e := range elems {
		keys = append(keys, e.Key())
	}
	sort.Strings(keys)
	return keys
}

// listItems returns the documents of a stored Mendix list (marker first).
func listItems(t *testing.T, doc bson.Raw, key string) (marker bson.RawValue, items []bson.Raw) {
	t.Helper()
	v, err := doc.LookupErr(key)
	if err != nil {
		t.Fatalf("%s is absent", key)
	}
	vals, err := v.Array().Values()
	if err != nil || len(vals) == 0 {
		t.Fatalf("%s is not a marked list: %v", key, err)
	}
	for _, x := range vals[1:] {
		items = append(items, x.Document())
	}
	return vals[0], items
}

func encodedPublishedService(t *testing.T, svc *model.PublishedODataService) bson.Raw {
	t.Helper()
	raw, err := (&codec.Encoder{}).Encode(publishedODataServiceToGen(svc, true))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return raw
}

func TestIssue1805_EntitySetWritesStudioProPropertySet(t *testing.T) {
	want := studioProEntitySetKeys(t)
	raw := encodedPublishedService(t, &model.PublishedODataService{
		Name: "ZzApi",
		EntityTypes: []*model.PublishedEntityType{
			{Entity: "MyFirstModule.Stated", ExposedName: "Stated"},
			{Entity: "MyFirstModule.Unstated", ExposedName: "Unstated"},
		},
		EntitySets: []*model.PublishedEntitySet{
			{ExposedName: "Stated", EntityTypeName: "MyFirstModule.Stated",
				ReadMode: "source", InsertMode: "source", UpdateMode: "source", DeleteMode: "source"},
			// `publish entity M.E as 'Unstated'` with no mode stated.
			{ExposedName: "Unstated", EntityTypeName: "MyFirstModule.Unstated"},
		},
	})
	_, sets := listItems(t, raw, "EntitySets")
	if len(sets) != 2 {
		t.Fatalf("got %d entity sets, want 2", len(sets))
	}
	for _, es := range sets {
		name := es.Lookup("ExposedName").StringValue()
		if got := rawKeyNames(t, es); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("entity set %s has property names\n  %v\nStudio Pro writes\n  %v\n"+
				"— `fix property-sets` reports ODataPublish$EntitySet as MIXED", name, got, want)
		}
	}
	unstated := sets[1]
	for key, typ := range map[string]string{
		"ReadMode":   "ODataPublish$ReadSource",
		"InsertMode": "ODataPublish$ChangeNotSupported",
		"UpdateMode": "ODataPublish$ChangeNotSupported",
		"DeleteMode": "ODataPublish$ChangeNotSupported",
	} {
		v, err := unstated.LookupErr(key)
		if err != nil {
			t.Errorf("unstated entity set has no %s", key)
			continue
		}
		if got := v.Document().Lookup("$Type").StringValue(); got != typ {
			t.Errorf("unstated %s = %s, want %s — what Mendix gives the absent property", key, got, typ)
		}
	}
}

func TestIssue1805_PublishedMicroflowWritesItsParametersWhenThereAreNone(t *testing.T) {
	raw := encodedPublishedService(t, &model.PublishedODataService{
		Name: "ZzApi",
		Microflows: []*model.PublishedMicroflow{
			{Microflow: "MyFirstModule.Act_NoParams", ExposedName: "NoParams", ReturnTypeKind: "Boolean"},
			{Microflow: "MyFirstModule.Act_Params", ExposedName: "WithParams", ReturnTypeKind: "Boolean",
				Parameters: []*model.PublishedMicroflowParameter{{
					MicroflowParameter: "MyFirstModule.Act_Params.Note", ExposedName: "note", DataTypeKind: "String",
				}}},
		},
	})
	_, actions := listItems(t, raw, "Microflows")
	if len(actions) != 2 {
		t.Fatalf("got %d actions, want 2", len(actions))
	}
	none, some := actions[0], actions[1]
	if a, b := rawKeyNames(t, none), rawKeyNames(t, some); strings.Join(a, ",") != strings.Join(b, ",") {
		t.Fatalf("an action without parameters has property names\n  %v\nand one with them\n  %v\n"+
			"— `fix property-sets` reports ODataPublish$PublishedMicroflow.Parameters as MIXED", a, b)
	}
	marker, params := listItems(t, none, "Parameters")
	if m, ok := marker.Int32OK(); !ok || m != 3 || len(params) != 0 {
		t.Errorf("Parameters of a parameterless action = marker %v + %d item(s), want the empty marker-3 list Mendix writes", marker, len(params))
	}
}

// The census over a project written through both forms of each element, the
// way the reporter's services were: a fresh Mendix 11.14.0 project
// (testdata/testapp-views, Studio Pro-authored, every type uniform) gets a
// service whose entity sets state their modes and do not, and whose actions
// take parameters and do not. `fix property-sets` must find no ODataPublish
// name the elements of a type disagree on.
func TestIssue1805_PublishedServiceCensusHasNoMixedNames(t *testing.T) {
	dst := t.TempDir()
	if err := testutil.CopyFixture(dst, "../../../testdata/testapp-views"); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	proj := filepath.Join(dst, "TestApp.mpr")
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	svc := &model.PublishedODataService{
		ContainerID: mod.ID, Name: "Zz1805Api", Path: "odata/zz1805/", Version: "1.0.0",
		ODataVersion: "OData4", Namespace: "MyFirstModule.Zz1805",
		EntityTypes: []*model.PublishedEntityType{
			{Entity: "MyFirstModule.Car", ExposedName: "StatedCars"},
			{Entity: "MyFirstModule.VCar", ExposedName: "UnstatedCars"},
		},
		EntitySets: []*model.PublishedEntitySet{
			{ExposedName: "StatedCars", EntityTypeName: "MyFirstModule.Car",
				ReadMode: "source", InsertMode: "source", UpdateMode: "source", DeleteMode: "source"},
			{ExposedName: "UnstatedCars", EntityTypeName: "MyFirstModule.VCar"},
		},
		Microflows: []*model.PublishedMicroflow{
			{Microflow: "MyFirstModule.Act_NoParams", ExposedName: "NoParams", ReturnTypeKind: "Boolean"},
			{Microflow: "MyFirstModule.Act_Params", ExposedName: "WithParams", ReturnTypeKind: "Boolean",
				Parameters: []*model.PublishedMicroflowParameter{{
					MicroflowParameter: "MyFirstModule.Act_Params.Note", ExposedName: "note", DataTypeKind: "String",
				}}},
		},
	}
	if err := b.CreatePublishedODataService(svc); err != nil {
		t.Fatalf("CreatePublishedODataService: %v", err)
	}
	// The modify path writes the same converter's output over the stored unit.
	svc.Version = "1.0.1"
	if err := b.UpdatePublishedODataService(svc); err != nil {
		t.Fatalf("UpdatePublishedODataService: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	r, err := mmpr.Open(proj)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	ids, err := r.ListAllUnitIDs()
	if err != nil {
		t.Fatal(err)
	}
	census := mmpr.NewPropertySetCensus()
	for _, id := range ids {
		rep, err := r.StoredPropertySetGaps(id)
		if err != nil {
			t.Fatalf("unit %s: %v", id, err)
		}
		census.Count(rep)
	}
	for _, m := range census.Mixed() {
		t.Errorf("MIXED  %s.%s: on %d of %d element(s)", m.Type, m.Key, m.With, m.Elements)
	}
}
