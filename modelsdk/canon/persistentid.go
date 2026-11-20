// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"bytes"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore"
)

// persistentIDKey is the sole nested identity property carried here.
//
// Every Workflows$* element — activities, outcomes, boundary events, and the
// workflow document itself — stores one, and both engines mint a fresh GUID for
// it on every write (modelsdk's addFreshPersistentID, legacy's
// idToBsonBinary(generateUUID())). Neither reads the stored value back, so a
// workflow document never equals itself: no-op elision could not fire, and an
// ALTER that changed nothing still produced a version-control diff (issue #949).
const persistentIDKey = "PersistentId"

// CarryPersistentIDs returns contents with each element's PersistentId replaced
// by the value the corresponding stored element already holds.
//
// This is the nested-element counterpart to CarryIdentity, which only reaches
// top-level properties of the document root — PersistentId lives on elements
// arbitrarily deep in the flow tree, so the correspondence has to be established
// structurally. It reuses the pairing TransplantIDs is built on: two elements
// correspond when they agree on $Type and align within their list, and only
// paired elements exchange anything.
//
// The same three properties that make the $ID transplant safe hold here:
//
//   - The patch is in place over fixed-width 16-byte binaries, so no length
//     prefix can be disturbed and nothing is re-marshalled.
//   - The mapping is injective — a stored PersistentId is handed out at most
//     once — so two elements cannot end up sharing one.
//   - It moves the written document *towards* what is already on disk, which is
//     what eliding the write would have done implicitly.
//
// Unlike an $ID, a PersistentId is not a pointer target: nothing in the document
// references it, so substituting one by value touches exactly its own element.
// That is what lets this run as a plain value substitution rather than needing
// the reference-rewriting argument TransplantIDs makes.
//
// A document that cannot be read on either side passes through untouched.
func CarryPersistentIDs(contents, stored []byte) []byte {
	// Only a workflow's elements have the property. A pair needs it on the new
	// side as a 16-byte binary, stored as this byte sequence: without it there
	// is nothing to carry, and the two documents need not be parsed to find
	// that out — as they were for every page, microflow and domain model
	// written (ako/mxcli#2160).
	if !bytes.Contains(contents, persistentIDElement) {
		return contents
	}
	return carryPersistentIDsUnfiltered(contents, stored)
}

// persistentIDElement is how a binary PersistentId property starts in a stored
// document: the binary type byte, the key, and the key's terminator.
var persistentIDElement = []byte("\x05" + persistentIDKey + "\x00")

// carryPersistentIDsUnfiltered is CarryPersistentIDs without the check that
// the document has the property at all.
func carryPersistentIDsUnfiltered(contents, stored []byte) []byte {
	m := persistentIDMapping(contents, stored)
	if len(m) == 0 {
		return contents
	}
	out := append([]byte(nil), contents...)
	if !patchDocument(bsoncore.Document(out), m) {
		return contents
	}
	// A duplicate is the one real failure: two elements claiming one identity is
	// worse than two fresh ones. Verify rather than trust the mapping.
	if hasDuplicatePersistentID(out) {
		return contents
	}
	return out
}

// persistentIDMapping pairs the two documents structurally and returns
// new PersistentId → stored PersistentId for every element that corresponds.
func persistentIDMapping(contents, stored []byte) map[string][]byte {
	var newDoc, oldDoc bson.D
	if err := bson.Unmarshal(contents, &newDoc); err != nil {
		return nil
	}
	if err := bson.Unmarshal(stored, &oldDoc); err != nil {
		return nil
	}
	p := &persistPairer{pairs: map[string][]byte{}, claimed: map[string]bool{}}
	p.pairValue(newDoc, oldDoc)
	return p.pairs
}

// persistPairer mirrors pairer, but records the PersistentId property instead of
// the element $ID. The traversal is deliberately identical: a divergence between
// the two would mean the $ID and the PersistentId of one element could be taken
// from two *different* stored elements.
type persistPairer struct {
	pairs   map[string][]byte
	claimed map[string]bool
}

func (p *persistPairer) record(newID, oldID string, oldData []byte) {
	if _, seen := p.pairs[newID]; seen {
		return
	}
	if p.claimed[oldID] {
		return
	}
	p.claimed[oldID] = true
	p.pairs[newID] = oldData
}

func (p *persistPairer) pairValue(newV, oldV any) {
	if nd, ok := asDoc(newV); ok {
		od, ok := asDoc(oldV)
		if !ok {
			return
		}
		p.pairDoc(nd, od)
		return
	}
	if news, ok := asSlice(newV); ok {
		olds, ok := asSlice(oldV)
		if !ok {
			return
		}
		for _, pair := range alignSlices(news, olds) {
			p.pairValue(news[pair[0]], olds[pair[1]])
		}
	}
}

// pairDoc corresponds two elements. As in pairer, a differing $Type means these
// are not the same element, so nothing is carried from it or from anything under
// it — inheriting an identity there would claim a replacement was an edit.
func (p *persistPairer) pairDoc(nd, od map[string]any) {
	if typeName(nd) != typeName(od) {
		return
	}
	if nb, ok := binary16(nd, persistentIDKey); ok {
		if ob, ok := binary16(od, persistentIDKey); ok {
			p.record(blobToUUID(nb), blobToUUID(ob), ob)
		}
	}
	for _, k := range sortedKeys(nd) {
		if ov, ok := od[k]; ok {
			p.pairValue(nd[k], ov)
		}
	}
}

// binary16 reads a fixed-width 16-byte binary property. Any other width or type
// is left alone: the patch is in place, so only an equal-length value is safe.
func binary16(d map[string]any, key string) ([]byte, bool) {
	b, ok := d[key].(bson.Binary)
	if !ok || len(b.Data) != 16 {
		return nil, false
	}
	return b.Data, true
}

// hasDuplicatePersistentID reports whether any two elements share a PersistentId.
func hasDuplicatePersistentID(raw []byte) bool {
	seen := map[string]bool{}
	dup := false
	var walk func(doc bsoncore.Document)
	walk = func(doc bsoncore.Document) {
		elems, err := doc.Elements()
		if err != nil {
			return
		}
		for _, e := range elems {
			v := e.Value()
			switch v.Type {
			case bsoncore.TypeBinary:
				if e.Key() != persistentIDKey {
					continue
				}
				_, data, ok := v.BinaryOK()
				if !ok || len(data) != 16 {
					continue
				}
				id := blobToUUID(data)
				if seen[id] {
					dup = true
				}
				seen[id] = true
			case bsoncore.TypeEmbeddedDocument:
				if sub, ok := v.DocumentOK(); ok {
					walk(sub)
				}
			case bsoncore.TypeArray:
				if arr, ok := v.ArrayOK(); ok {
					walk(bsoncore.Document(arr))
				}
			}
		}
	}
	walk(bsoncore.Document(raw))
	return dup
}
