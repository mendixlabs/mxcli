// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestCreateConsumedODataService_RoundTrip creates a consumed OData service with
// an HTTP configuration and confirms it round-trips through the reader.
func TestCreateConsumedODataService_RoundTrip(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}

	svc := &model.ConsumedODataService{
		ContainerID:  mod.ID,
		Name:         "ZzClient",
		Version:      "1.0",
		ODataVersion: "OData4",
		MetadataUrl:  "https://api.example.com/odata/v4/$metadata",
		// HttpConfiguration carries the ServiceUrl (CustomLocation) and headers —
		// these MUST round-trip or a CREATE OR MODIFY drops the ServiceUrl (#680
		// OData example, CE5111).
		HttpConfiguration: &model.HttpConfiguration{
			HttpMethod:       "Post",
			OverrideLocation: true,
			CustomLocation:   "'https://api.example.com/odata'",
			HeaderEntries: []*model.HttpHeaderEntry{
				{Key: "X-Api-Key", Value: "'abc123'"},
			},
		},
	}
	if err := b.CreateConsumedODataService(svc); err != nil {
		t.Fatalf("CreateConsumedODataService: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	all, err := b2.ListConsumedODataServices()
	if err != nil {
		t.Fatalf("ListConsumedODataServices: %v", err)
	}
	var got *model.ConsumedODataService
	for _, s := range all {
		if s.Name == "ZzClient" {
			got = s
			break
		}
	}
	if got == nil {
		t.Fatalf("ZzClient not found after create")
	}
	if got.MetadataUrl != "https://api.example.com/odata/v4/$metadata" {
		t.Errorf("MetadataUrl not round-tripped: %q", got.MetadataUrl)
	}
	// HttpConfiguration must round-trip (the read previously dropped it).
	if got.HttpConfiguration == nil {
		t.Fatal("HttpConfiguration not round-tripped (nil)")
	}
	if !got.HttpConfiguration.OverrideLocation || got.HttpConfiguration.CustomLocation != "'https://api.example.com/odata'" {
		t.Errorf("ServiceUrl (CustomLocation) not round-tripped: override=%v loc=%q",
			got.HttpConfiguration.OverrideLocation, got.HttpConfiguration.CustomLocation)
	}
	if len(got.HttpConfiguration.HeaderEntries) != 1 || got.HttpConfiguration.HeaderEntries[0].Key != "X-Api-Key" {
		t.Errorf("header entries not round-tripped: %+v", got.HttpConfiguration.HeaderEntries)
	}
}

// TestCreatePublishedODataService_RoundTrip creates a published OData service with
// one entity type, members (a key attribute), and an entity set, then confirms it
// round-trips through ListPublishedODataServices.
func TestCreatePublishedODataService_RoundTrip(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}

	svc := &model.PublishedODataService{
		ContainerID:         mod.ID,
		Name:                "ZzApi",
		Path:                "odata/zz/",
		Version:             "1.0.0",
		ODataVersion:        "OData4",
		Namespace:           "MyFirstModule.Zz",
		AuthenticationTypes: []string{"Basic"},
		EntityTypes: []*model.PublishedEntityType{{
			Entity:      "MyFirstModule.Thing",
			ExposedName: "Things",
			Members: []*model.PublishedMember{
				{Kind: "attribute", Name: "Code", ExposedName: "code", IsPartOfKey: true, Filterable: true, Sortable: true},
				{Kind: "attribute", Name: "Label", ExposedName: "label"},
			},
		}},
		EntitySets: []*model.PublishedEntitySet{{
			ExposedName: "Things", EntityTypeName: "MyFirstModule.Thing",
			ReadMode: "source", UsePaging: true, PageSize: 100,
		}},
	}
	if err := b.CreatePublishedODataService(svc); err != nil {
		t.Fatalf("CreatePublishedODataService: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	all, err := b2.ListPublishedODataServices()
	if err != nil {
		t.Fatalf("ListPublishedODataServices: %v", err)
	}
	var got *model.PublishedODataService
	for _, s := range all {
		if s.Name == "ZzApi" {
			got = s
			break
		}
	}
	if got == nil {
		t.Fatalf("ZzApi not found after create")
	}
	if got.Path != "odata/zz/" {
		t.Errorf("Path not round-tripped: %q", got.Path)
	}
	if got.ODataVersion != "OData4" {
		t.Errorf("ODataVersion not round-tripped: %q", got.ODataVersion)
	}
	// The full EntityTypes/EntitySets tree must round-trip — the read previously
	// surfaced only counts, so an ALTER stripped the entity sets (NullReference
	// crash on load, #680 OData example).
	if len(got.EntityTypes) != 1 {
		t.Fatalf("EntityTypes not round-tripped: got %d, want 1", len(got.EntityTypes))
	}
	et := got.EntityTypes[0]
	if et.Entity != "MyFirstModule.Thing" || et.ExposedName != "Things" {
		t.Errorf("entity type header wrong: %+v", et)
	}
	if len(et.Members) != 2 {
		t.Fatalf("entity-type members not round-tripped: got %d, want 2", len(et.Members))
	}
	// Member names round-trip qualified (Module.Entity.Attr — the stored form);
	// the writer's qualifyMemberName is a no-op on an already-qualified name.
	if !strings.HasSuffix(et.Members[0].Name, ".Code") || !et.Members[0].IsPartOfKey {
		t.Errorf("key member not round-tripped: %+v", et.Members[0])
	}
	if len(got.EntitySets) != 1 {
		t.Fatalf("EntitySets not round-tripped: got %d, want 1", len(got.EntitySets))
	}
	es := got.EntitySets[0]
	if es.ExposedName != "Things" || es.EntityTypeName != "MyFirstModule.Thing" {
		t.Errorf("entity set header wrong: %+v", es)
	}
	if !es.UsePaging || es.PageSize != 100 {
		t.Errorf("entity set paging not round-tripped: %+v", es)
	}
}

// TestConsumedODataServiceToGen_ConfigMicroflowKey guards issue #728: the config
// microflow must be serialized under the version-appropriate BSON key. On
// 11.10+ that is ConfigurationEntityMicroflow (writing the pre-11.10
// ConfigurationMicroflow makes Studio Pro show "Constants only").
func TestConsumedODataServiceToGen_ConfigMicroflowKey(t *testing.T) {
	svc := &model.ConsumedODataService{
		Name:                   "Svc",
		ConfigurationMicroflow: "MyModule.ConfigureMF",
	}
	for _, tc := range []struct {
		key      string
		wantHave string
		wantGone string
	}{
		{"ConfigurationEntityMicroflow", "ConfigurationEntityMicroflow", "ConfigurationMicroflow"},
		{"ConfigurationMicroflow", "ConfigurationMicroflow", "ConfigurationEntityMicroflow"},
	} {
		raw, err := (&codec.Encoder{}).Encode(consumedODataServiceToGen(svc, tc.key, "HeaderListMicroflow"))
		if err != nil {
			t.Fatalf("encode(%s): %v", tc.key, err)
		}
		r := bson.Raw(raw)
		if v, err := r.LookupErr(tc.wantHave); err != nil {
			t.Errorf("key %s: expected field %q present, got err %v", tc.key, tc.wantHave, err)
		} else if got := v.StringValue(); got != "MyModule.ConfigureMF" {
			t.Errorf("key %s: field %q = %q, want MyModule.ConfigureMF", tc.key, tc.wantHave, got)
		}
		if _, err := r.LookupErr(tc.wantGone); err == nil {
			t.Errorf("key %s: field %q must NOT be present when using %q", tc.key, tc.wantGone, tc.key)
		}
	}
}
