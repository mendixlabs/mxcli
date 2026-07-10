// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"github.com/JordtenBulte-OLC/mxcli/mdl/bsonutil"

	"go.mongodb.org/mongo-driver/bson"
)

// marshalUnitIDFirst normalizes a unit document so every nested storage object
// leads with "$ID" (and "$Type" second), then marshals it — the 11.12-safe
// replacement for a bare bson.Marshal(doc) at a unit-serialization boundary.
//
// Mendix 11.12+ rejects any storage object whose first BSON property is not
// "$ID". Several legacy writers preserve round-trip fidelity by carrying parsed
// subtrees as Go maps and marshalling them back, but bson.Marshal emits map keys
// in random order — so "$ID" only lands first by luck. bsonutil.HoistStorageID
// lifts "$ID"/"$Type" to the front while preserving the original order of every
// other key (a blind sort corrupts template-derived pluggable-widget trees).
func marshalUnitIDFirst(doc any) ([]byte, error) {
	return bson.Marshal(bsonutil.HoistStorageID(doc))
}
