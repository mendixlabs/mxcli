// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"go.mongodb.org/mongo-driver/v2/bson"
)

// List markers (ako/mxcli#1640).
//
// Mendix stores every list as a BSON array whose first entry is an int32 typed-
// array marker, then the items. Studio Pro writes 2 for some lists and 3 for
// others, and which one is a property of the list, not of anything a script
// says: it is measured per list and registered here, and the encoder writes the
// registered marker for every list it builds.
//
// A list the encoder does NOT build keeps the marker it had. That happens on
// every rewrite of a stored document: an unchanged pluggable widget's Type and
// Object pass through byte for byte, an existing element's clean properties are
// copied as raw bytes, a mutator edits the stored document in place. So a marker
// an older mxcli wrote before the list was measured survived every later
// rewrite, while a fresh document got Studio Pro's. NormalizeListMarkers is the
// write-side half of the same rule: every write gives a registered list its
// registered marker, whoever built the list.
//
// Only a marker REGISTERED for the owner $Type and key is applied. The marker
// the encoder writes for an unregistered list (3) is a default, not a
// measurement, so a stored marker there is left as it is; it is either Studio
// Pro's or unknown. The child-$Type registrations are not applied either: they
// stand in for the list's real rule only on the lists the encoder builds, which
// all start with one of the registered children, and are not a statement about
// every list that does.

// childListMarkers maps a child element $Type to the marker the encoder writes
// for a list whose first item is of that type.
var childListMarkers = map[string]int32{}

// propertyListMarkers maps an owning $Type and list key to its marker.
var propertyListMarkers = map[string]map[string]int32{}

// RegisterChildListMarker records the marker of a list whose first item is of
// childType. It is the encoder's rule for a list it builds; NormalizeListMarkers
// does not apply it.
func RegisterChildListMarker(childType string, marker int32) {
	childListMarkers[childType] = marker
}

// ChildListMarker returns the marker registered for a list whose first item is
// of childType.
func ChildListMarker(childType string) (int32, bool) {
	m, ok := childListMarkers[childType]
	return m, ok
}

// RegisterPropertyListMarker records the marker Studio Pro writes for the list
// stored under key on elements of ownerType, empty or not.
func RegisterPropertyListMarker(ownerType, key string, marker int32) {
	if propertyListMarkers[ownerType] == nil {
		propertyListMarkers[ownerType] = map[string]int32{}
	}
	propertyListMarkers[ownerType][key] = marker
}

// PropertyListMarker returns the marker registered for the list under key on
// elements of ownerType.
func PropertyListMarker(ownerType, key string) (int32, bool) {
	m, ok := propertyListMarkers[ownerType][key]
	return m, ok
}

// NormalizeListMarkers gives every list with a registered owner/key marker that
// marker, wherever it sits in the unit. It changes nothing else, and returns
// contents unchanged (the same slice) when no marker differs or contents is not
// a document.
func NormalizeListMarkers(contents []byte) []byte {
	if len(propertyListMarkers) == 0 {
		return contents
	}
	var doc bson.D
	if err := parseForStorageForm(contents, &doc); err != nil {
		return contents
	}
	if !normalizeListMarkersIn(doc) {
		return contents
	}
	out, err := bson.Marshal(doc)
	if err != nil {
		return contents
	}
	return out
}

func normalizeListMarkersIn(v any) bool {
	changed := false
	switch x := v.(type) {
	case bson.D:
		owner, _ := lookupString(x, "$Type")
		keys := propertyListMarkers[owner]
		for i := range x {
			if m, ok := keys[x[i].Key]; ok {
				if a, isList := x[i].Value.(bson.A); isList && len(a) > 0 {
					if cur, isMarker := a[0].(int32); isMarker && cur != m {
						a[0] = m
						changed = true
					}
				}
			}
			if normalizeListMarkersIn(x[i].Value) {
				changed = true
			}
		}
	case bson.A:
		for _, e := range x {
			if normalizeListMarkersIn(e) {
				changed = true
			}
		}
	}
	return changed
}
