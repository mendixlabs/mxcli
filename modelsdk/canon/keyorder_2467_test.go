// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"bytes"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/version"
)

// Studio Pro writes every element as $ID, $Type, then the rest of its keys in
// case-insensitive order: 0 exceptions in 34 596 objects (10.24), 43 998
// (11.12.6) and 44 003 (11.14) of Mendix-written projects, while a byte-order
// sort has 5 types out of place (`_Key` before `Caption`, `AllowedTypes` before
// `AllowNonPersistableEntities`). mxcli wrote metamodel order, so Studio Pro
// rewrote every unit it had touched on first open (ako/mxcli#2467).

func keys2467(d bson.D) []string {
	out := make([]string, len(d))
	for i, e := range d {
		out[i] = e.Key
	}
	return out
}

func TestSortKeysStudioProOrder_2467(t *testing.T) {
	raw, _ := bson.Marshal(bson.D{
		{Key: "$ID", Value: "a"},
		{Key: "$Type", Value: "Microflows$ActionActivity"},
		{Key: "Size", Value: "1;1"},
		{Key: "Action", Value: bson.D{
			{Key: "$ID", Value: "b"},
			{Key: "$Type", Value: "Microflows$ChangeAction"},
			{Key: "Items", Value: bson.A{int32(2), bson.D{
				{Key: "$ID", Value: "c"},
				{Key: "$Type", Value: "Microflows$ChangeActionItem"},
				{Key: "Attribute", Value: "M.E.A"},
				{Key: "Association", Value: ""},
			}}},
			{Key: "ErrorHandlingType", Value: "Rollback"},
		}},
		{Key: "AllowNonPersistableEntities", Value: false},
		{Key: "_Key", Value: "k"},
		{Key: "AllowedTypes", Value: bson.A{int32(1)}},
		{Key: "Caption", Value: "c"},
	})
	out := SortKeys(raw)
	var doc bson.D
	if err := bson.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	want := []string{"$ID", "$Type", "_Key", "Action", "AllowedTypes", "AllowNonPersistableEntities", "Caption", "Size"}
	if got := keys2467(doc); !same2467(got, want) {
		t.Errorf("top-level keys = %v, want %v", got, want)
	}
	action := doc[3].Value.(bson.D)
	if got, want := keys2467(action), []string{"$ID", "$Type", "ErrorHandlingType", "Items"}; !same2467(got, want) {
		t.Errorf("nested keys = %v, want %v", got, want)
	}
	item := action[3].Value.(bson.A)[1].(bson.D)
	if got, want := keys2467(item), []string{"$ID", "$Type", "Association", "Attribute"}; !same2467(got, want) {
		t.Errorf("list item keys = %v, want %v", got, want)
	}
	// Order only: the content is the same document.
	if eq, err := Equal(raw, out); err != nil || !eq {
		t.Errorf("sorting changed the document's content (err %v)", err)
	}
}

// Control: a document already in Studio Pro's order comes back as the same
// bytes, so a Studio Pro-written unit is never re-encoded for it.
func TestSortKeysLeavesSortedDocument_2467(t *testing.T) {
	raw, _ := bson.Marshal(bson.D{
		{Key: "$ID", Value: "a"},
		{Key: "$Type", Value: "Forms$Appearance"},
		{Key: "Class", Value: ""},
		{Key: "DesignProperties", Value: bson.A{int32(3)}},
		{Key: "DynamicClasses", Value: ""},
		{Key: "Style", Value: ""},
	})
	if out := SortKeys(raw); !bytes.Equal(out, raw) {
		t.Error("a document in Studio Pro's order was rewritten")
	}
}

// The write path sorts (listMarkers: the bytes that land); the stored-unit
// comparison path does not, so it never reports a reorder as a change.
func TestCompleteForStorageSortsOnWrite_2467(t *testing.T) {
	raw, _ := bson.Marshal(bson.D{
		{Key: "$ID", Value: "a"},
		{Key: "$Type", Value: "Microflows$MicroflowCallParameterMapping"},
		{Key: "Parameter", Value: "M.F.p"},
		{Key: "Argument", Value: "1"},
	})
	pv := version.Version{Major: 11, Minor: 12, Patch: 6}
	keys := func(listMarkers bool) []string {
		out, err := CompleteForStorage(raw, &pv, listMarkers)
		if err != nil {
			t.Fatal(err)
		}
		var doc bson.D
		if err := bson.Unmarshal(out, &doc); err != nil {
			t.Fatal(err)
		}
		return keys2467(doc)
	}
	if got := keys(true); !same2467(got, []string{"$ID", "$Type", "Argument", "Parameter"}) {
		t.Errorf("write path keys = %v, want Studio Pro's order", got)
	}
	if got := keys(false); !same2467(got, []string{"$ID", "$Type", "Parameter", "Argument"}) {
		t.Errorf("comparison path keys = %v, want them as stored", got)
	}
}

func same2467(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
