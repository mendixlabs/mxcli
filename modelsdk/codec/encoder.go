package codec

import (
	"bytes"
	"fmt"
	"sort"
	"unsafe"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore"
)

// Encoder serializes Element trees back to BSON bytes.
type Encoder struct{}

// Encode serializes an element to []byte.
// Clean elements passthrough raw bytes unchanged.
// Dirty elements rebuild using buildDoc.
func (e *Encoder) Encode(elem element.Element) ([]byte, error) {
	raw := elem.Raw()
	if raw != nil && !elem.IsDirty() {
		return []byte(raw), nil
	}

	doc, err := e.buildDoc(elem)
	if err != nil {
		return nil, err
	}
	return bson.Marshal(doc)
}

// rebuildEntry records which properties of an element need re-encoding.
type rebuildEntry struct {
	name string // property key; kept as string for stable ordering
	wp   element.WritableProperty
	cp   element.ChildProperty
	clp  element.ChildListProperty
}

// bytesOf returns a []byte view of s without copying.
// Safe only for read-only use within the lifetime of s.
func bytesOf(s string) []byte {
	if s == "" {
		return nil
	}
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// rawKeyOf returns the key bytes of a BSON element without allocating.
// bsoncore.Element is a []byte; KeyBytes() slices into it.
func rawKeyOf(re bsoncore.Element) []byte { return re.KeyBytes() }

// stringOf converts a []byte to string without copying.
// Safe only while the underlying []byte is alive.
func stringOf(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// buildDoc constructs a bson.D for a dirty element.
//
// Key optimization over the previous implementation: instead of calling
// bson.Unmarshal(raw, &bson.D) — which recursively decodes the entire BSON
// tree into Go objects (3 591 allocs for a 25 KB microflow) — we iterate the
// raw bytes with bson.Raw.Elements() and pass clean fields through as
// bson.RawValue (zero-alloc). Only fields that are actually dirty are decoded
// and re-encoded.
func (e *Encoder) buildDoc(elem element.Element) (bson.D, error) {
	// Determine cheaply whether any child element is dirty.
	// element.Base propagates child-dirty state up the container chain, so
	// a single IsChildDirty() call on the element avoids O(N) PartList scans
	// when no children are dirty (the common case for scalar-only edits).
	type childDirtyChecker interface{ IsChildDirty() bool }
	childMightBeDirty := true
	if cd, ok := elem.(childDirtyChecker); ok {
		childMightBeDirty = cd.IsChildDirty()
	}

	// Build a compact slice of properties that need rebuilding. Using a slice
	// (not a map) lets us do zero-alloc byte comparison with re.KeyBytes()
	// in the main loop below, avoiding the 21 string allocations from re.Key().
	var rebuild []rebuildEntry // typically 1–5 entries; stays on stack for small N
	for _, prop := range elem.Properties() {
		wp, ok := prop.(element.WritableProperty)
		if !ok {
			continue
		}
		cp, _ := prop.(element.ChildProperty)
		clp, _ := prop.(element.ChildListProperty)

		needsRebuild := wp.Dirty()
		if !needsRebuild && childMightBeDirty {
			// Only pay the O(N) scan cost when the parent reports a dirty child.
			if cp != nil {
				ch := cp.ChildElement()
				needsRebuild = ch != nil && ch.IsDirty()
			} else if clp != nil {
				needsRebuild = anyChildDirty(clp)
			}
		}
		if needsRebuild {
			rebuild = append(rebuild, rebuildEntry{prop.Name(), wp, cp, clp})
		}
	}

	// findRebuild returns the index into rebuild for a BSON key, or -1.
	// Uses zero-alloc byte comparison instead of map[string] lookup.
	// O(M) where M = len(rebuild) ≤ dirty properties (1–5 typical).
	findRebuild := func(keyB []byte) int {
		for i := range rebuild {
			if bytes.Equal(keyB, bytesOf(rebuild[i].name)) {
				return i
			}
		}
		return -1
	}
	// seen tracks which rebuild entries were found in the raw bytes.
	seen := make([]bool, len(rebuild))

	raw := elem.Raw()

	// === New element (no raw bytes) ===
	// Iterate elem.Properties() (a slice) for stable field ordering — not the
	// rebuild slice, whose order varies with property registration.
	if raw == nil {
		doc := bson.D{
			{Key: "$ID", Value: idToBinarySubtype0(elem.ID())},
			{Key: "$Type", Value: elem.TypeName()},
		}
		emitted := make(map[string]bool)
		for _, prop := range elem.Properties() {
			idx := findRebuild(bytesOf(prop.Name()))
			if idx < 0 {
				continue
			}
			val, err := e.encodeEntry(rebuild[idx])
			if err != nil {
				return nil, err
			}
			if val != nil {
				doc = append(doc, bson.E{Key: prop.Name(), Value: val})
				emitted[prop.Name()] = true
			}
		}
		// Apply Studio Pro serialization defaults the gen constructors don't set
		// (the applyDefaults gap): a GUID (= $ID) and mandatory member collections
		// emitted empty as the typed-array marker. Registered per $Type, so this
		// affects only element types explicitly declared via RegisterTypeDefaults.
		if d, ok := lookupTypeDefaults(elem.TypeName()); ok {
			if d.EmitGUID && !emitted["GUID"] {
				doc = append(doc, bson.E{Key: "GUID", Value: idToBinarySubtype0(elem.ID())})
			}
			for _, name := range d.MandatoryLists {
				if !emitted[name] {
					doc = append(doc, bson.E{Key: name, Value: bson.A{int32(3)}})
				}
			}
			if len(d.MandatoryListMarkers) > 0 {
				names := make([]string, 0, len(d.MandatoryListMarkers))
				for name := range d.MandatoryListMarkers {
					names = append(names, name)
				}
				sort.Strings(names)
				for _, name := range names {
					if !emitted[name] {
						doc = append(doc, bson.E{Key: name, Value: bson.A{d.MandatoryListMarkers[name]}})
					}
				}
			}
			for _, name := range d.NullFields {
				if !emitted[name] {
					doc = append(doc, bson.E{Key: name, Value: nil})
				}
			}
			for _, name := range d.EmptyStringFields {
				if !emitted[name] {
					doc = append(doc, bson.E{Key: name, Value: ""})
				}
			}
			for _, name := range d.ZeroGUIDFields {
				if !emitted[name] {
					doc = append(doc, bson.E{Key: name, Value: zeroGUIDBinary()})
				}
			}
			for _, name := range d.FreshGUIDFields {
				if !emitted[name] {
					doc = append(doc, bson.E{Key: name, Value: idToBinarySubtype0("")})
				}
			}
		}
		return doc, nil
	}

	// === Existing element — iterate raw bytes, pass clean fields through ===
	// Uses bsoncore.Document.Elements() for access to KeyBytes() (zero-alloc
	// key reads). bsoncore.Value must be converted to bson.RawValue before
	// storing in bson.E, because bson.Marshal only has a codec for bson.RawValue.
	rawElems, err := bsoncore.Document(raw).Elements()
	if err != nil {
		return nil, fmt.Errorf("read raw elements for %s: %w", elem.TypeName(), err)
	}

	doc := make(bson.D, 0, len(rawElems))
	for _, re := range rawElems {
		keyB := rawKeyOf(re)
		idx := findRebuild(keyB)
		if idx < 0 {
			// Clean field: zero-alloc key + raw value passthrough.
			// Convert bsoncore.Value → bson.RawValue so bson.Marshal uses
			// the registered RawValueEncodeValue codec (verbatim byte copy).
			cv := re.Value()
			doc = append(doc, bson.E{
				Key:   stringOf(keyB),
				Value: bson.RawValue{Type: bson.Type(cv.Type), Value: cv.Data},
			})
			continue
		}
		// Dirty field: encode new value.
		val, err := e.encodeEntry(rebuild[idx])
		if err != nil {
			return nil, err
		}
		if val != nil {
			doc = append(doc, bson.E{Key: rebuild[idx].name, Value: val})
		}
		seen[idx] = true
	}

	// Append dirty fields that didn't exist in the raw bytes (new properties).
	// Iterate Properties() for stable ordering.
	for _, prop := range elem.Properties() {
		idx := findRebuild(bytesOf(prop.Name()))
		if idx < 0 || seen[idx] {
			continue
		}
		val, err := e.encodeEntry(rebuild[idx])
		if err != nil {
			return nil, err
		}
		if val != nil {
			doc = append(doc, bson.E{Key: prop.Name(), Value: val})
		}
	}

	return doc, nil
}

// encodeEntry produces the BSON value for a single dirty property.
func (e *Encoder) encodeEntry(rb rebuildEntry) (any, error) {
	wp := rb.wp

	// Child (Part) property.
	if rb.cp != nil {
		child := rb.cp.ChildElement()
		if wp.Dirty() {
			if child == nil {
				return nil, nil // deleted
			}
			return e.buildDoc(child)
		}
		// Child itself is dirty (parent pointer unchanged).
		if child != nil && child.IsDirty() {
			return e.buildDoc(child)
		}
		return nil, nil
	}

	// Child list (PartList) property.
	if rb.clp != nil {
		children := rb.clp.ChildElements()
		if wp.Dirty() {
			// Full rebuild: all children re-encoded.
			arr := make(bson.A, 0, 1+len(children))
			arr = append(arr, partListMarker(children))
			for _, child := range children {
				childDoc, err := e.buildDoc(child)
				if err != nil {
					return nil, err
				}
				arr = append(arr, childDoc)
			}
			return arr, nil
		}
		// Selective rebuild: dirty children re-encoded, clean ones pass through raw bytes.
		arr := make(bson.A, 0, 1+len(children))
		arr = append(arr, int32(3))
		for _, child := range children {
			if child.IsDirty() {
				childDoc, err := e.buildDoc(child)
				if err != nil {
					return nil, err
				}
				arr = append(arr, childDoc)
			} else {
				arr = append(arr, bson.Raw(child.Raw()))
			}
		}
		return arr, nil
	}

	// Scalar / Enum / Ref property.
	val := wp.BSONValue()
	if id, ok := val.(element.ID); ok && id != "" {
		return idToBinary(id), nil
	}
	return val, nil
}

// anyChildDirty reports whether any element in the ChildListProperty is dirty.
func anyChildDirty(clp element.ChildListProperty) bool {
	for _, child := range clp.ChildElements() {
		if child.IsDirty() {
			return true
		}
	}
	return false
}

// idToBinarySubtype0 converts a UUID string to BSON Binary subtype 0.
// When id is empty a fresh UUID is generated.
func idToBinarySubtype0(id element.ID) any {
	if id == "" {
		return mpr.IDToBsonBinary(mpr.GenerateID())
	}
	return mpr.IDToBsonBinary(string(id))
}

// idToBinary converts a UUID string to Mendix BSON Binary format.
func idToBinary(id element.ID) any {
	return idToBinarySubtype0(id)
}

// zeroGUID is the all-zero UUID Studio Pro uses for an unset GUID reference.
const zeroGUID = "00000000-0000-0000-0000-000000000000"

// zeroGUIDBinary returns the BSON Binary (subtype 0, 16 zero bytes) Studio Pro
// writes for an unset reference field (e.g. AssociationPointer on an
// attribute-based index segment).
func zeroGUIDBinary() any {
	return mpr.IDToBsonBinary(zeroGUID)
}

// partListMarker returns the leading typed-array marker for a PartList, derived
// from the child element $Type (defaulting to 3). An empty list keeps the
// default — empty mandatory lists are entity member collections, all marker 3.
func partListMarker(children []element.Element) int32 {
	if len(children) == 0 {
		return 3
	}
	return lookupListMarker(children[0].TypeName())
}
