// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

// TestAttrNameForOData_ReservedWords verifies that Mendix-reserved attribute
// names are prefixed with the entity name so Studio Pro does not reject them.
// Regression test for issue #526.
func TestAttrNameForOData_ReservedWords(t *testing.T) {
	cases := []struct {
		prop   string
		entity string
		want   string
	}{
		// Already-covered names
		{"Id", "Photo", "PhotoId"},
		{"id", "Photo", "Photoid"},
		{"Name", "Airline", "AirlineName"},
		{"name", "Airline", "Airlinename"},
		// Newly-added reserved names (issue #526)
		{"Owner", "Trip", "TripOwner"},
		{"owner", "Trip", "Tripowner"},
		{"Type", "Flight", "FlightType"},
		{"type", "Flight", "Flighttype"},
		{"Context", "Person", "PersonContext"},
		{"context", "Person", "Personcontext"},
		{"ChangedBy", "Event", "EventChangedBy"},
		{"changedby", "Event", "Eventchangedby"},
		{"ChangedDate", "Event", "EventChangedDate"},
		{"changeddate", "Event", "Eventchangeddate"},
		{"CreatedDate", "Event", "EventCreatedDate"},
		{"createddate", "Event", "Eventcreateddate"},
		// Non-reserved names must pass through unchanged
		{"AirlineCode", "Airline", "AirlineCode"},
		{"Concurrency", "Airline", "Concurrency"},
		{"FirstName", "Person", "FirstName"},
	}

	for _, tc := range cases {
		got := attrNameForOData(tc.prop, tc.entity)
		if got != tc.want {
			t.Errorf("attrNameForOData(%q, %q) = %q; want %q", tc.prop, tc.entity, got, tc.want)
		}
	}
}

// TestApplyExternalEntityFields_ConservativeCapabilityDefault guards the OData
// capability defaulting: an entity set with no InsertRestrictions/
// DeleteRestrictions annotation must import as Creatable/Deletable = false,
// matching Mendix's conservative read-only default. Verified against the
// TripPin RESTier service (zero capability annotations, entities Creatable=False
// per mx check). An earlier permissive default caused CE6630 regressions.
func TestApplyExternalEntityFields_ConservativeCapabilityDefault(t *testing.T) {
	ent := &domainmodel.Entity{}
	et := &types.EdmEntityType{Name: "Person"}
	// entitySet with no Insertable/Deletable annotation (nil) — the TripPin case.
	es := &types.EdmEntitySet{Name: "People"}

	applyExternalEntityFields(ent, et, true /*isTopLevel*/, "Svc.TripPin", es, nil, nil)

	if ent.Creatable {
		t.Error("Creatable = true, want false (absent InsertRestrictions ⇒ conservative read-only)")
	}
	if ent.Deletable {
		t.Error("Deletable = true, want false (absent DeleteRestrictions ⇒ conservative read-only)")
	}

	// An explicit annotation must turn the capability on.
	on := true
	es2 := &types.EdmEntitySet{Name: "People", Insertable: &on}
	ent2 := &domainmodel.Entity{}
	applyExternalEntityFields(ent2, et, true, "Svc.TripPin", es2, nil, nil)
	if !ent2.Creatable {
		t.Error("Creatable = false, want true (explicit InsertRestrictions=true)")
	}
}

// TestCreateNavigationAssociations_NoDuplicateOnReimport guards against the
// re-import duplication where each OData nav property spawned a numerically-
// suffixed copy (Friends, Friends2, Friends3 …) on every import. A nav property
// that already has an association must be skipped.
func TestCreateNavigationAssociations_NoDuplicateOnReimport(t *testing.T) {
	person := &types.EdmEntityType{
		Name: "Person",
		NavigationProperties: []*types.EdmNavigationProperty{
			{Name: "Friends", Type: "Collection(NS.Person)"},
			{Name: "BestFriend", Type: "NS.Person"},
		},
	}
	doc := &types.EdmxDocument{
		Schemas:    []*types.EdmSchema{{Namespace: "NS", EntityTypes: []*types.EdmEntityType{person}}},
		EntitySets: []*types.EdmEntitySet{{Name: "People", EntityType: "NS.Person"}},
	}
	people := &domainmodel.Entity{Name: "People", Persistable: true}
	people.ID = model.ID("ent-people")
	dm := &domainmodel.DomainModel{Entities: []*domainmodel.Entity{people}}
	dm.ID = model.ID("dm-1")

	typeByQualified := map[string]*types.EdmEntityType{"NS.Person": person}
	esMap := map[string]string{"NS.Person": "People"}

	var created []*domainmodel.Association
	mb := &mock.MockBackend{
		CreateAssociationFunc: func(_ model.ID, a *domainmodel.Association) error {
			created = append(created, a)
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))

	// First import creates both nav associations.
	if n := createNavigationAssociations(ctx, dm, doc, typeByQualified, esMap, "TripPin.Client"); n != 2 {
		t.Fatalf("first import created %d associations, want 2", n)
	}
	// Persist them into the domain model, as a re-read between imports would.
	dm.Associations = append(dm.Associations, created...)
	countAfterFirst := len(created)

	// Second import must create nothing — no Friends2 / BestFriend2 duplicates.
	if n := createNavigationAssociations(ctx, dm, doc, typeByQualified, esMap, "TripPin.Client"); n != 0 {
		t.Errorf("re-import created %d associations, want 0 (duplicates)", n)
	}
	if extra := len(created) - countAfterFirst; extra != 0 {
		t.Errorf("re-import persisted %d new associations via the backend, want 0", extra)
	}
}

// TestMendixAttrTypeToEdm guards the Mendix→EDM type mapping used to populate a
// published attribute's EdmType. Without it Studio Pro reports CE5016
// ("published as ."). String/Decimal/Boolean/DateTimeOffset are verified against
// Studio Pro's corrected BSON.
func TestMendixAttrTypeToEdm(t *testing.T) {
	cases := []struct {
		typ  domainmodel.AttributeType
		want string
	}{
		{&domainmodel.StringAttributeType{}, "Edm.String"},
		{&domainmodel.HashedStringAttributeType{}, "Edm.String"},
		{&domainmodel.IntegerAttributeType{}, "Edm.Int32"},
		{&domainmodel.LongAttributeType{}, "Edm.Int64"},
		{&domainmodel.AutoNumberAttributeType{}, "Edm.Int64"},
		{&domainmodel.DecimalAttributeType{}, "Edm.Decimal"},
		{&domainmodel.BooleanAttributeType{}, "Edm.Boolean"},
		{&domainmodel.DateTimeAttributeType{}, "Edm.DateTimeOffset"},
		{&domainmodel.BinaryAttributeType{}, "Edm.Binary"},
		{nil, ""},
	}
	for _, c := range cases {
		if got := mendixAttrTypeToEdm(c.typ); got != c.want {
			t.Errorf("mendixAttrTypeToEdm(%T) = %q, want %q", c.typ, got, c.want)
		}
	}
}
