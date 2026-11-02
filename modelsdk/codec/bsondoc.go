package codec

import (
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ---------------------------------------------------------------------------
// BSONDocument — opaque wrapper around bson.D for engine/ callers.
//
// This type lets engine/ read, modify, and construct BSON documents without
// importing go.mongodb.org/mongo-driver/bson. All bson.D manipulation is
// encapsulated here.
// ---------------------------------------------------------------------------

// BSONDocument wraps a parsed BSON document for in-place manipulation.
type BSONDocument struct {
	doc bson.D
}

// BSONArray wraps a parsed BSON array for in-place manipulation.
type BSONArray struct {
	arr bson.A
}

// ParseBSON deserializes raw BSON bytes into a BSONDocument.
func ParseBSON(data []byte) (*BSONDocument, error) {
	var doc bson.D
	if err := bson.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return &BSONDocument{doc: doc}, nil
}

// Marshal serializes the document back to raw BSON bytes.
func (d *BSONDocument) Marshal() ([]byte, error) {
	return bson.Marshal(d.doc)
}

// ToBuilder converts a BSONDocument to a BSONDocBuilder, sharing the
// underlying bson.D. This allows passing parsed documents to APIs that
// expect *BSONDocBuilder (e.g. InsertWidgetsNear, ReplaceWidgetByName).
func (d *BSONDocument) ToBuilder() *BSONDocBuilder {
	return &BSONDocBuilder{doc: d.doc}
}

// ---------------------------------------------------------------------------
// Field getters
// ---------------------------------------------------------------------------

// GetString returns a string field value, or "" if not found.
func (d *BSONDocument) GetString(key string) string {
	return getString(d.doc, key)
}

// GetInt32 returns an integer field value, or 0 if not found. The stored width
// is not assumed: Studio Pro — and the codec, since ako/mxcli#1264 — write
// Integer properties as int64, so an int32-only assertion would read 0.
func (d *BSONDocument) GetInt32(key string) int32 {
	for _, e := range d.doc {
		if e.Key == key {
			switch v := e.Value.(type) {
			case int32:
				return v
			case int64:
				return int32(v)
			case float64:
				return int32(v)
			}
		}
	}
	return 0
}

// GetBool returns a bool field value, or false if not found.
func (d *BSONDocument) GetBool(key string) bool {
	for _, e := range d.doc {
		if e.Key == key {
			if v, ok := e.Value.(bool); ok {
				return v
			}
		}
	}
	return false
}

// GetDoc returns a nested document, or nil if not found.
func (d *BSONDocument) GetDoc(key string) *BSONDocument {
	for _, e := range d.doc {
		if e.Key == key {
			if v, ok := e.Value.(bson.D); ok {
				return &BSONDocument{doc: v}
			}
		}
	}
	return nil
}

// GetArray returns a nested array, or an empty versioned array if not found.
func (d *BSONDocument) GetArray(key string) *BSONArray {
	for _, e := range d.doc {
		if e.Key == key {
			if v, ok := e.Value.(bson.A); ok {
				return &BSONArray{arr: v}
			}
		}
	}
	return &BSONArray{arr: bson.A{int32(3)}}
}

// Has checks whether a field exists.
func (d *BSONDocument) Has(key string) bool {
	for _, e := range d.doc {
		if e.Key == key {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Field setters
// ---------------------------------------------------------------------------

// Set sets or replaces a field with any value.
func (d *BSONDocument) Set(key string, value any) *BSONDocument {
	d.doc = SetBSONDocField(d.doc, key, value)
	return d
}

// SetDoc sets or replaces a field with a nested document.
func (d *BSONDocument) SetDoc(key string, child *BSONDocument) *BSONDocument {
	d.doc = SetBSONDocField(d.doc, key, child.doc)
	return d
}

// SetArray sets or replaces a field with an array.
func (d *BSONDocument) SetArray(key string, arr *BSONArray) *BSONDocument {
	d.doc = SetBSONDocField(d.doc, key, arr.arr)
	return d
}

// SetBuilder sets or replaces a field with a BSONDocBuilder document.
func (d *BSONDocument) SetBuilder(key string, b *BSONDocBuilder) *BSONDocument {
	d.doc = SetBSONDocField(d.doc, key, b.doc)
	return d
}

// Remove removes a field by key.
func (d *BSONDocument) Remove(key string) *BSONDocument {
	newDoc := make(bson.D, 0, len(d.doc))
	for _, e := range d.doc {
		if e.Key != key {
			newDoc = append(newDoc, e)
		}
	}
	d.doc = newDoc
	return d
}

// ---------------------------------------------------------------------------
// BSONArray methods
// ---------------------------------------------------------------------------

// Len returns the number of elements (excluding version marker).
func (a *BSONArray) Len() int {
	count := 0
	for _, item := range a.arr {
		if _, ok := item.(bson.D); ok {
			count++
		}
	}
	return count
}

// Doc returns the document at index i (skipping version markers).
func (a *BSONArray) Doc(i int) *BSONDocument {
	idx := 0
	for _, item := range a.arr {
		if d, ok := item.(bson.D); ok {
			if idx == i {
				return &BSONDocument{doc: d}
			}
			idx++
		}
	}
	return nil
}

// FindByName finds a document in the array where field "Name" matches.
// Returns the document and its raw index in the array, or (nil, -1).
func (a *BSONArray) FindByName(name string) (*BSONDocument, int) {
	return a.FindByField("Name", name)
}

// FindByField finds a document in the array where the given field matches.
// Returns the document and its raw index in the array, or (nil, -1).
func (a *BSONArray) FindByField(field, value string) (*BSONDocument, int) {
	for i, item := range a.arr {
		if d, ok := item.(bson.D); ok {
			for _, e := range d {
				if e.Key == field {
					if s, ok := e.Value.(string); ok && s == value {
						return &BSONDocument{doc: d}, i
					}
				}
			}
		}
	}
	return nil, -1
}

// Append adds a document to the array.
func (a *BSONArray) Append(doc *BSONDocument) {
	a.arr = append(a.arr, doc.doc)
}

// AppendBuilder adds a BSONDocBuilder document to the array.
func (a *BSONArray) AppendBuilder(b *BSONDocBuilder) {
	a.arr = append(a.arr, b.doc)
}

// InsertAt inserts a document at the given raw array index, shifting
// subsequent elements to the right.
func (a *BSONArray) InsertAt(rawIdx int, doc *BSONDocument) {
	if rawIdx < 0 || rawIdx > len(a.arr) {
		a.arr = append(a.arr, doc.doc)
		return
	}
	a.arr = append(a.arr, nil)
	copy(a.arr[rawIdx+1:], a.arr[rawIdx:])
	a.arr[rawIdx] = doc.doc
}

// RemoveAt removes the element at the given raw array index.
func (a *BSONArray) RemoveAt(rawIdx int) {
	if rawIdx >= 0 && rawIdx < len(a.arr) {
		a.arr = append(a.arr[:rawIdx], a.arr[rawIdx+1:]...)
	}
}

// Update replaces the document at the given raw array index.
func (a *BSONArray) Update(rawIdx int, doc *BSONDocument) {
	if rawIdx >= 0 && rawIdx < len(a.arr) {
		a.arr[rawIdx] = doc.doc
	}
}

// Each iterates over document elements in the array, calling fn for each.
// fn receives the document and its raw array index. Return false to stop.
func (a *BSONArray) Each(fn func(doc *BSONDocument, rawIdx int) bool) {
	for i, item := range a.arr {
		if d, ok := item.(bson.D); ok {
			if !fn(&BSONDocument{doc: d}, i) {
				return
			}
		}
	}
}

// NewVersionedArray creates a new empty array with the Mendix version marker.
func NewVersionedArray() *BSONArray {
	return &BSONArray{arr: bson.A{int32(3)}}
}

// NewEmptyDoc creates a new empty BSONDocument.
func NewEmptyDoc() *BSONDocument {
	return &BSONDocument{doc: bson.D{}}
}

// ---------------------------------------------------------------------------
// Recursive widget-tree helpers for ALTER PAGE operations.
// These let engine/ traverse and mutate BSON widget trees without
// importing bson types.
// ---------------------------------------------------------------------------

// WalkWidgets recursively walks the widget tree in this document,
// calling fn for each document that has a "$Type" field. The walk
// descends into "Widgets" and "Arguments" array fields.
// fn receives each widget doc; return true from fn to stop early.
// Returns true if early-stopped.
func (d *BSONDocument) WalkWidgets(fn func(widget *BSONDocument) bool) bool {
	return walkWidgetDoc(d, fn)
}

func walkWidgetDoc(d *BSONDocument, fn func(*BSONDocument) bool) bool {
	if d.Has("$Type") {
		if fn(d) {
			return true
		}
	}
	for _, key := range []string{"Widgets", "Arguments", "LayoutCall"} {
		for i, e := range d.doc {
			if e.Key != key {
				continue
			}
			switch v := e.Value.(type) {
			case bson.D:
				child := &BSONDocument{doc: v}
				if walkWidgetDoc(child, fn) {
					d.doc[i].Value = child.doc
					return true
				}
				d.doc[i].Value = child.doc
			case bson.A:
				arr := &BSONArray{arr: v}
				if walkWidgetArr(arr, fn) {
					d.doc[i].Value = arr.arr
					return true
				}
				d.doc[i].Value = arr.arr
			}
		}
	}
	return false
}

func walkWidgetArr(a *BSONArray, fn func(*BSONDocument) bool) bool {
	for i, item := range a.arr {
		if raw, ok := item.(bson.D); ok {
			child := &BSONDocument{doc: raw}
			if walkWidgetDoc(child, fn) {
				a.arr[i] = child.doc
				return true
			}
			a.arr[i] = child.doc
		}
	}
	return false
}

// ModifyWidgetByName finds a widget with the given Name field in the
// document tree and calls fn to modify it. Returns true if found.
func (d *BSONDocument) ModifyWidgetByName(name string, fn func(widget *BSONDocument)) bool {
	return modifyByName(d, name, fn)
}

func modifyByName(d *BSONDocument, name string, fn func(*BSONDocument)) bool {
	if d.GetString("Name") == name {
		fn(d)
		return true
	}
	for _, key := range []string{"Widgets", "Arguments", "LayoutCall"} {
		for i, e := range d.doc {
			if e.Key != key {
				continue
			}
			switch v := e.Value.(type) {
			case bson.D:
				child := &BSONDocument{doc: v}
				if modifyByName(child, name, fn) {
					d.doc[i].Value = child.doc
					return true
				}
			case bson.A:
				for j, item := range v {
					if raw, ok := item.(bson.D); ok {
						child := &BSONDocument{doc: raw}
						if modifyByName(child, name, fn) {
							v[j] = child.doc
							d.doc[i].Value = v
							return true
						}
					}
				}
			}
		}
	}
	return false
}

// RemoveWidgetByName removes a widget with the given Name from the
// document's widget tree. Returns true if the widget was found and removed.
func (d *BSONDocument) RemoveWidgetByName(name string) bool {
	return removeByName(d, name)
}

func removeByName(d *BSONDocument, name string) bool {
	for i, e := range d.doc {
		if e.Key != "Widgets" && e.Key != "Arguments" {
			continue
		}
		arr, ok := e.Value.(bson.A)
		if !ok {
			continue
		}
		// Check direct children first
		for j, item := range arr {
			raw, ok := item.(bson.D)
			if !ok {
				continue
			}
			if getString(raw, "Name") == name {
				// Remove this element
				newArr := make(bson.A, 0, len(arr)-1)
				newArr = append(newArr, arr[:j]...)
				newArr = append(newArr, arr[j+1:]...)
				d.doc[i].Value = newArr
				return true
			}
		}
		// Recurse into children
		for j, item := range arr {
			raw, ok := item.(bson.D)
			if !ok {
				continue
			}
			child := &BSONDocument{doc: raw}
			if removeByName(child, name) {
				arr[j] = child.doc
				d.doc[i].Value = arr
				return true
			}
		}
	}
	// Also check nested docs (like LayoutCall)
	for i, e := range d.doc {
		if raw, ok := e.Value.(bson.D); ok {
			child := &BSONDocument{doc: raw}
			if removeByName(child, name) {
				d.doc[i].Value = child.doc
				return true
			}
		}
	}
	return false
}

// InsertWidgetsNear inserts builder documents before or after a named
// widget in the document tree. position is "BEFORE" or "AFTER".
// Returns true if the target widget was found.
func (d *BSONDocument) InsertWidgetsNear(targetName, position string, widgets []*BSONDocBuilder) bool {
	return insertNear(d, targetName, position, widgets)
}

func insertNear(d *BSONDocument, targetName, position string, widgets []*BSONDocBuilder) bool {
	for i, e := range d.doc {
		if e.Key != "Widgets" && e.Key != "Arguments" {
			continue
		}
		arr, ok := e.Value.(bson.A)
		if !ok {
			continue
		}
		for j, item := range arr {
			raw, ok := item.(bson.D)
			if !ok {
				continue
			}
			if getString(raw, "Name") == targetName {
				idx := j
				if position == "AFTER" {
					idx = j + 1
				}
				newArr := make(bson.A, 0, len(arr)+len(widgets))
				newArr = append(newArr, arr[:idx]...)
				for _, w := range widgets {
					newArr = append(newArr, w.doc)
				}
				newArr = append(newArr, arr[idx:]...)
				d.doc[i].Value = newArr
				return true
			}
			// Recurse
			child := &BSONDocument{doc: raw}
			if insertNear(child, targetName, position, widgets) {
				arr[j] = child.doc
				d.doc[i].Value = arr
				return true
			}
		}
	}
	// Also check nested docs (like LayoutCall)
	for i, e := range d.doc {
		if raw, ok := e.Value.(bson.D); ok {
			child := &BSONDocument{doc: raw}
			if insertNear(child, targetName, position, widgets) {
				d.doc[i].Value = child.doc
				return true
			}
		}
	}
	return false
}

// ReplaceWidgetByName replaces a named widget with one or more new
// widget builders. Returns true if the target was found and replaced.
func (d *BSONDocument) ReplaceWidgetByName(targetName string, widgets []*BSONDocBuilder) bool {
	return replaceByName(d, targetName, widgets)
}

func replaceByName(d *BSONDocument, targetName string, widgets []*BSONDocBuilder) bool {
	for i, e := range d.doc {
		if e.Key != "Widgets" && e.Key != "Arguments" {
			continue
		}
		arr, ok := e.Value.(bson.A)
		if !ok {
			continue
		}
		for j, item := range arr {
			raw, ok := item.(bson.D)
			if !ok {
				continue
			}
			if getString(raw, "Name") == targetName {
				newArr := make(bson.A, 0, len(arr)-1+len(widgets))
				newArr = append(newArr, arr[:j]...)
				for _, w := range widgets {
					newArr = append(newArr, w.doc)
				}
				newArr = append(newArr, arr[j+1:]...)
				d.doc[i].Value = newArr
				return true
			}
			// Recurse
			child := &BSONDocument{doc: raw}
			if replaceByName(child, targetName, widgets) {
				arr[j] = child.doc
				d.doc[i].Value = arr
				return true
			}
		}
	}
	// Also check nested docs (like LayoutCall)
	for i, e := range d.doc {
		if raw, ok := e.Value.(bson.D); ok {
			child := &BSONDocument{doc: raw}
			if replaceByName(child, targetName, widgets) {
				d.doc[i].Value = child.doc
				return true
			}
		}
	}
	return false
}

// AppendToVersionedArray appends a BSONDocBuilder document to the named
// versioned array field, creating the array with version marker if needed.
func (d *BSONDocument) AppendToVersionedArray(key string, item *BSONDocBuilder) {
	for i, e := range d.doc {
		if e.Key == key {
			if arr, ok := e.Value.(bson.A); ok {
				d.doc[i].Value = append(arr, item.doc)
				return
			}
		}
	}
	d.doc = append(d.doc, bson.E{Key: key, Value: bson.A{int32(3), item.doc}})
}

// RemoveFromVersionedArrayByField removes elements from a versioned array
// where the given field matches one of the provided values.
func (d *BSONDocument) RemoveFromVersionedArrayByField(arrayKey, fieldKey string, matchValues ...string) {
	matchSet := make(map[string]bool, len(matchValues))
	for _, v := range matchValues {
		matchSet[v] = true
	}
	for i, e := range d.doc {
		if e.Key != arrayKey {
			continue
		}
		arr, ok := e.Value.(bson.A)
		if !ok {
			return
		}
		newArr := bson.A{}
		for _, item := range arr {
			if raw, ok := item.(bson.D); ok {
				if matchSet[getString(raw, fieldKey)] {
					continue
				}
			}
			newArr = append(newArr, item)
		}
		d.doc[i].Value = newArr
		return
	}
}

// GetValue returns the raw value for a field, or nil if not found.
// This is used when the caller needs to pass through opaque values
// (e.g., for SET operations on arbitrary properties).
func (d *BSONDocument) GetValue(key string) any {
	for _, e := range d.doc {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}
