// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestRetrieveSourceFromGen_SortBy guards the retrieve "sort by …" clause. The
// sort columns live in the DatabaseRetrieveSource's NewSortings child (a
// SortingsList); without reading it the clause is silently dropped.
func TestRetrieveSourceFromGen_SortBy(t *testing.T) {
	raw := mustMarshalFlow(bson.D{
		{Key: "$ID", Value: "rs-1"},
		{Key: "$Type", Value: "Microflows$DatabaseRetrieveSource"},
		{Key: "Entity", Value: "LoftManagement.Application"},
		{Key: "NewSortings", Value: bson.D{
			{Key: "$ID", Value: "sl-1"},
			{Key: "$Type", Value: "Microflows$SortingsList"},
			{Key: "Sortings", Value: bson.A{
				int32(1), // typed-array marker
				bson.D{
					{Key: "$ID", Value: "rsort-1"},
					{Key: "$Type", Value: "Microflows$RetrieveSorting"},
					{Key: "SortOrder", Value: "asc"},
					{Key: "AttributeRef", Value: bson.D{
						{Key: "$ID", Value: "ar-1"},
						{Key: "$Type", Value: "DomainModels$AttributeRef"},
						{Key: "Attribute", Value: "LoftManagement.Application.Name"},
					}},
				},
			}},
		}},
	})
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	src, ok := retrieveSourceFromGen(el).(*microflows.DatabaseRetrieveSource)
	if !ok {
		t.Fatalf("retrieveSourceFromGen → not a DatabaseRetrieveSource")
	}
	if len(src.Sorting) != 1 {
		t.Fatalf("Sorting = %d items, want 1 (NewSortings dropped)", len(src.Sorting))
	}
	if got := src.Sorting[0]; got.AttributeQualifiedName != "LoftManagement.Application.Name" || string(got.Direction) != "asc" {
		t.Errorf("sort item = {%q, %q}, want {LoftManagement.Application.Name, asc}", got.AttributeQualifiedName, got.Direction)
	}
}

// TestRetrieveSourceToGen_SortByRoundTrip guards the write path: a database
// retrieve's "sort by …" columns must be serialized into the NewSortings
// envelope. Previously retrieveSourceToGen wrote an empty SortItemList and
// dropped every sort column, so DESCRIBE emitted a retrieve with no sort
// (issue #727).
func TestRetrieveSourceToGen_SortByRoundTrip(t *testing.T) {
	in := &microflows.DatabaseRetrieveSource{
		EntityQualifiedName: "SortBug.Ticket",
		Sorting: []*microflows.SortItem{
			{AttributeQualifiedName: "SortBug.Ticket.Priority", Direction: microflows.SortDirectionDescending},
			{AttributeQualifiedName: "SortBug.Ticket.Name", Direction: microflows.SortDirectionAscending},
		},
	}

	el := retrieveSourceToGen(in)
	if el == nil {
		t.Fatal("retrieveSourceToGen returned nil")
	}
	raw, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	out, ok := retrieveSourceFromGen(decoded).(*microflows.DatabaseRetrieveSource)
	if !ok {
		t.Fatal("round-trip did not yield a DatabaseRetrieveSource")
	}
	if len(out.Sorting) != 2 {
		t.Fatalf("Sorting = %d items after round-trip, want 2 (sort columns dropped on write)", len(out.Sorting))
	}
	if out.Sorting[0].AttributeQualifiedName != "SortBug.Ticket.Priority" || out.Sorting[0].Direction != microflows.SortDirectionDescending {
		t.Errorf("sort[0] = {%q, %q}, want {SortBug.Ticket.Priority, Descending}", out.Sorting[0].AttributeQualifiedName, out.Sorting[0].Direction)
	}
	if out.Sorting[1].AttributeQualifiedName != "SortBug.Ticket.Name" || out.Sorting[1].Direction != microflows.SortDirectionAscending {
		t.Errorf("sort[1] = {%q, %q}, want {SortBug.Ticket.Name, Ascending}", out.Sorting[1].AttributeQualifiedName, out.Sorting[1].Direction)
	}
}
