// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Key order (ako/mxcli#2467).
//
// Studio Pro writes every element as $ID, $Type, then the rest of its keys in
// case-insensitive order: no exception in 34 596 objects of a Mendix 10.24
// project, 43 998 of an 11.12.6 one and 44 003 of an 11.14 one. Byte order is
// not it: five types have keys out of byte order there (`_Key` before
// `Caption`, `AllowedTypes` before `AllowNonPersistableEntities`).
//
// mxcli wrote a new element's keys in metamodel order and appended a property
// it added to a stored element at the end. Studio Pro reads either, but on
// first open it rewrites every such unit in its own order, and the next mxcli
// edit flipped it back, so every diff after mxcli work carried hundreds of
// reordered units. The write path puts every unit it stores in Studio Pro's
// order instead. Content is untouched, and the write elision compares content
// without order, so a stored unit is reordered only when it is really written.

// keyLess is Studio Pro's key order: $ID, then $Type, then case-insensitive,
// with byte order breaking a tie between keys that differ only in case.
func keyLess(a, b string) bool {
	if ra, rb := keyRank(a), keyRank(b); ra != rb {
		return ra < rb
	}
	la, lb := strings.ToLower(a), strings.ToLower(b)
	if la != lb {
		return la < lb
	}
	return a < b
}

func keyRank(k string) int {
	switch k {
	case "$ID":
		return 0
	case "$Type":
		return 1
	}
	return 2
}

// SortKeys returns contents with every element's keys in Studio Pro's order,
// and contents itself when they already are or it is not a document.
func SortKeys(contents []byte) []byte {
	var doc bson.D
	if err := parseForStorageForm(contents, &doc); err != nil {
		return contents
	}
	if !sortKeysIn(doc) {
		return contents
	}
	out, err := bson.Marshal(doc)
	if err != nil {
		return contents
	}
	return out
}

// sortKeysIn sorts every document in v in place and reports whether any key
// moved.
func sortKeysIn(v any) bool {
	changed := false
	switch x := v.(type) {
	case bson.D:
		if !sort.SliceIsSorted(x, func(i, j int) bool { return keyLess(x[i].Key, x[j].Key) }) {
			sort.SliceStable(x, func(i, j int) bool { return keyLess(x[i].Key, x[j].Key) })
			changed = true
		}
		for i := range x {
			if sortKeysIn(x[i].Value) {
				changed = true
			}
		}
	case bson.A:
		for _, e := range x {
			if sortKeysIn(e) {
				changed = true
			}
		}
	}
	return changed
}
