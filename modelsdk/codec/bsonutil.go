package codec

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ---------------------------------------------------------------------------
// Bridge helpers — raw BSON operations exposed to engine/ so that engine/
// never imports go.mongodb.org/mongo-driver/bson directly.
//
// DO NOT add type aliases (type D = bson.D) or marshal wrappers here.
// engine/ must use modelsdk/gen/* SDK constructors, not raw BSON types.
// ---------------------------------------------------------------------------

// SetBSONDocField sets or replaces a key in a parsed bson.D document.
// This is the in-memory equivalent of PatchBSONField (which works on []byte).
// Use this when the caller already has a bson.D from tree traversal.
func SetBSONDocField(doc bson.D, key string, val any) bson.D {
	for i, e := range doc {
		if e.Key == key {
			doc[i].Value = val
			return doc
		}
	}
	return append(doc, bson.E{Key: key, Value: val})
}

// PatchBSONField sets or replaces a top-level field in a BSON document.
func PatchBSONField(data []byte, key string, value any) ([]byte, error) {
	var doc bson.D
	if err := bson.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	found := false
	for i, e := range doc {
		if e.Key == key {
			doc[i].Value = value
			found = true
			break
		}
	}
	if !found {
		doc = append(doc, bson.E{Key: key, Value: value})
	}
	return bson.Marshal(doc)
}

// PatchBSONArrayAppend appends a value to a versioned BSON array field.
func PatchBSONArrayAppend(data []byte, key string, value any) ([]byte, error) {
	var doc bson.D
	if err := bson.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	for i, e := range doc {
		if e.Key == key {
			if arr, ok := e.Value.(bson.A); ok {
				doc[i].Value = append(arr, value)
			}
			break
		}
	}
	return bson.Marshal(doc)
}

// PatchBSONArrayRemove removes a string value from a versioned BSON array field.
// Preserves the int32(3) version marker.
func PatchBSONArrayRemove(data []byte, key string, value string) ([]byte, error) {
	var doc bson.D
	if err := bson.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	for i, e := range doc {
		if e.Key != key {
			continue
		}
		arr, ok := e.Value.(bson.A)
		if !ok {
			break
		}
		newArr := bson.A{}
		for _, item := range arr {
			if s, ok := item.(string); ok && s == value {
				continue
			}
			newArr = append(newArr, item)
		}
		doc[i].Value = newArr
		break
	}
	return bson.Marshal(doc)
}

// ReadBSONFieldString reads a string field from raw BSON bytes.
func ReadBSONFieldString(data []byte, key string) (string, error) {
	raw := bson.Raw(data)
	val, err := raw.LookupErr(key)
	if err != nil {
		return "", fmt.Errorf("field %q not found", key)
	}
	s, ok := val.StringValueOK()
	if !ok {
		return "", fmt.Errorf("field %q is not a string", key)
	}
	return s, nil
}

// PatchNestedRefList patches a ByNameRefList field on a nested document
// identified by a name field within a versioned array. Used for patterns like:
//
//	doc.UserRoles[Name=="Admin"].ModuleRoles = ["Module.Role1", "Module.Role2"]
func PatchNestedRefList(data []byte, arrayKey, nameKey, nameValue, refListKey string, refs []string) ([]byte, error) {
	var doc bson.D
	if err := bson.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	for i, e := range doc {
		if e.Key != arrayKey {
			continue
		}
		arr, ok := e.Value.(bson.A)
		if !ok {
			continue
		}
		for j, item := range arr {
			d, ok := item.(bson.D)
			if !ok {
				continue
			}
			matched := false
			for _, f := range d {
				if f.Key == nameKey {
					if s, ok := f.Value.(string); ok && s == nameValue {
						matched = true
					}
				}
			}
			if !matched {
				continue
			}
			rolesArr := bson.A{int32(3)}
			for _, r := range refs {
				rolesArr = append(rolesArr, r)
			}
			found := false
			for k, f := range d {
				if f.Key == refListKey {
					d[k].Value = rolesArr
					found = true
					break
				}
			}
			if !found {
				d = append(d, bson.E{Key: refListKey, Value: rolesArr})
			}
			arr[j] = d
		}
		doc[i].Value = arr
	}
	return bson.Marshal(doc)
}

// PatchDocumentAllowedRoles adds or removes role strings from a versioned array
// field (AllowedModuleRoles, AllowedRoles, etc.) on a document.
// action must be "add" or "remove".
func PatchDocumentAllowedRoles(data []byte, fieldKey, action string, roles []string) ([]byte, error) {
	var doc bson.D
	if err := bson.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	fieldIdx := -1
	for i, e := range doc {
		if e.Key == fieldKey {
			fieldIdx = i
			break
		}
	}

	if fieldIdx < 0 && action == "add" {
		// Field doesn't exist yet — create it with the version marker.
		arr := bson.A{int32(3)}
		for _, r := range roles {
			arr = append(arr, r)
		}
		doc = append(doc, bson.E{Key: fieldKey, Value: arr})
		return bson.Marshal(doc)
	}
	if fieldIdx < 0 {
		// Nothing to remove from a non-existent field.
		return bson.Marshal(doc)
	}

	arr, ok := doc[fieldIdx].Value.(bson.A)
	if !ok {
		return bson.Marshal(doc)
	}

	if action == "add" {
		existing := make(map[string]bool)
		for _, item := range arr {
			if s, ok := item.(string); ok {
				existing[s] = true
			}
		}
		for _, r := range roles {
			if !existing[r] {
				arr = append(arr, r)
			}
		}
	} else { // remove
		removeSet := make(map[string]bool, len(roles))
		for _, r := range roles {
			removeSet[r] = true
		}
		newArr := bson.A{}
		for _, item := range arr {
			if s, ok := item.(string); ok && removeSet[s] {
				continue
			}
			newArr = append(newArr, item)
		}
		arr = newArr
	}
	doc[fieldIdx].Value = arr
	return bson.Marshal(doc)
}

// PatchEntityAccessRuleRoles patches ModuleRoles on an access rule within a
// nested entity inside a DomainModel document. If isNew is true, the last access
// rule is patched (the newly added one). Otherwise, it finds the access rule
// whose existing ModuleRoles match roleNames.
func PatchEntityAccessRuleRoles(data []byte, entityName string, roleNames []string, isNew bool) ([]byte, error) {
	var doc bson.D
	if err := bson.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	entities := getArray(doc, "Entities")
	for ei, eItem := range entities {
		ed, ok := eItem.(bson.D)
		if !ok {
			continue
		}
		if getString(ed, "Name") != entityName {
			continue
		}

		accessRules := getArray(ed, "AccessRules")

		rolesArr := bson.A{int32(3)}
		for _, rn := range roleNames {
			rolesArr = append(rolesArr, rn)
		}

		if isNew {
			if len(accessRules) > 0 {
				lastIdx := len(accessRules) - 1
				if ruleDoc, ok := accessRules[lastIdx].(bson.D); ok {
					ruleDoc = SetBSONDocField(ruleDoc, "ModuleRoles", rolesArr)
					accessRules[lastIdx] = ruleDoc
				}
			}
		} else {
			for ri, rItem := range accessRules {
				ruleDoc, ok := rItem.(bson.D)
				if !ok {
					continue
				}
				existingRoles := getArray(ruleDoc, "ModuleRoles")
				if matchRoleNames(existingRoles, roleNames) {
					ruleDoc = SetBSONDocField(ruleDoc, "ModuleRoles", rolesArr)
					accessRules[ri] = ruleDoc
					break
				}
			}
		}

		ed = SetBSONDocField(ed, "AccessRules", accessRules)
		entities[ei] = ed
		break
	}

	doc = SetBSONDocField(doc, "Entities", entities)
	return bson.Marshal(doc)
}

// getString returns the string value for the given key, or "".
func getString(doc bson.D, key string) string {
	for _, e := range doc {
		if e.Key == key {
			if s, ok := e.Value.(string); ok {
				return s
			}
		}
	}
	return ""
}

// getArray returns the bson.A for the given key, or an empty bson.A with version marker.
func getArray(doc bson.D, key string) bson.A {
	for _, e := range doc {
		if e.Key == key {
			if a, ok := e.Value.(bson.A); ok {
				return a
			}
		}
	}
	return bson.A{int32(3)}
}

// matchRoleNames checks if a BSON roles array contains the same set of role names.
func matchRoleNames(rolesArr bson.A, roleNames []string) bool {
	var existing []string
	for _, item := range rolesArr {
		if s, ok := item.(string); ok {
			existing = append(existing, s)
		}
	}
	if len(existing) != len(roleNames) {
		return false
	}
	set := make(map[string]bool, len(existing))
	for _, r := range existing {
		set[r] = true
	}
	for _, r := range roleNames {
		if !set[r] {
			return false
		}
	}
	return true
}

// AppendEncodedToVersionedArray encodes an element.Element via the codec
// Encoder and appends the resulting bson.D to a versioned array field in data.
// This replaces the pattern of NewBSONDoc().Set(...)  + AppendToVersionedArray
// with SDK constructors + encode + append.
func AppendEncodedToVersionedArray(data []byte, arrayKey string, elem element.Element) ([]byte, error) {
	enc := &Encoder{}
	encoded, err := enc.Encode(elem)
	if err != nil {
		return nil, fmt.Errorf("encode element: %w", err)
	}
	var elemDoc bson.D
	if err := bson.Unmarshal(encoded, &elemDoc); err != nil {
		return nil, fmt.Errorf("unmarshal encoded element: %w", err)
	}

	var doc bson.D
	if err := bson.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal data: %w", err)
	}
	found := false
	for i, e := range doc {
		if e.Key == arrayKey {
			if arr, ok := e.Value.(bson.A); ok {
				arr = append(arr, elemDoc)
				doc[i].Value = arr
				found = true
			}
			break
		}
	}
	if !found {
		doc = append(doc, bson.E{Key: arrayKey, Value: bson.A{int32(3), elemDoc}})
	}
	return bson.Marshal(doc)
}

// PatchNestedBSONField sets or replaces a field inside a sub-document.
// parentKey identifies the top-level field that contains the sub-document,
// childKey is the field within that sub-document to set.
func PatchNestedBSONField(data []byte, parentKey, childKey string, value any) ([]byte, error) {
	var doc bson.D
	if err := bson.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	for i, e := range doc {
		if e.Key != parentKey {
			continue
		}
		child, ok := e.Value.(bson.D)
		if !ok {
			continue
		}
		doc[i].Value = SetBSONDocField(child, childKey, value)
		return bson.Marshal(doc)
	}
	return bson.Marshal(doc)
}

// PatchVersionedArrayElements patches fields on each element of a versioned
// BSON array by index. patches maps element index (0-based, skipping the
// version marker) to a map of field-name -> value. Used to add non-SDK fields
// to elements created by SDK constructors.
func PatchVersionedArrayElements(data []byte, arrayKey string, patches map[int]map[string]any) ([]byte, error) {
	var doc bson.D
	if err := bson.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	for i, e := range doc {
		if e.Key != arrayKey {
			continue
		}
		arr, ok := e.Value.(bson.A)
		if !ok {
			continue
		}
		// Versioned arrays start with int32(3) marker at index 0.
		startIdx := 0
		if len(arr) > 0 {
			if _, isInt := arr[0].(int32); isInt {
				startIdx = 1
			}
		}
		for elemIdx, fields := range patches {
			rawIdx := startIdx + elemIdx
			if rawIdx >= len(arr) {
				continue
			}
			d, ok := arr[rawIdx].(bson.D)
			if !ok {
				continue
			}
			for k, v := range fields {
				d = SetBSONDocField(d, k, v)
			}
			arr[rawIdx] = d
		}
		doc[i].Value = arr
	}
	return bson.Marshal(doc)
}

// RenameBSONField renames a top-level field key in a BSON document.
func RenameBSONField(data []byte, oldKey, newKey string) ([]byte, error) {
	var doc bson.D
	if err := bson.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	for i, e := range doc {
		if e.Key == oldKey {
			doc[i].Key = newKey
			break
		}
	}
	return bson.Marshal(doc)
}

// RenameFieldInVersionedArrayElement renames a field key inside a specific
// element (by 0-based index, skipping version marker) of a versioned array.
func RenameFieldInVersionedArrayElement(data []byte, arrayKey string, elemIdx int, oldField, newField string) ([]byte, error) {
	var doc bson.D
	if err := bson.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	for i, e := range doc {
		if e.Key != arrayKey {
			continue
		}
		arr, ok := e.Value.(bson.A)
		if !ok {
			continue
		}
		startIdx := 0
		if len(arr) > 0 {
			if _, isInt := arr[0].(int32); isInt {
				startIdx = 1
			}
		}
		rawIdx := startIdx + elemIdx
		if rawIdx >= len(arr) {
			continue
		}
		d, ok := arr[rawIdx].(bson.D)
		if !ok {
			continue
		}
		for j, f := range d {
			if f.Key == oldField {
				d[j].Key = newField
				break
			}
		}
		arr[rawIdx] = d
		doc[i].Value = arr
	}
	return bson.Marshal(doc)
}

// PatchNestedVersionedArrayElements patches fields on elements of a versioned
// array that is nested inside a specific element of another versioned array.
// parentArrayKey and parentIdx identify the parent element; childArrayKey
// identifies the nested array within that element.
func PatchNestedVersionedArrayElements(data []byte, parentArrayKey string, parentIdx int, childArrayKey string, patches map[int]map[string]any) ([]byte, error) {
	var doc bson.D
	if err := bson.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	for i, e := range doc {
		if e.Key != parentArrayKey {
			continue
		}
		arr, ok := e.Value.(bson.A)
		if !ok {
			continue
		}
		startIdx := 0
		if len(arr) > 0 {
			if _, isInt := arr[0].(int32); isInt {
				startIdx = 1
			}
		}
		rawIdx := startIdx + parentIdx
		if rawIdx >= len(arr) {
			continue
		}
		parentDoc, ok := arr[rawIdx].(bson.D)
		if !ok {
			continue
		}
		for pi, pe := range parentDoc {
			if pe.Key != childArrayKey {
				continue
			}
			childArr, ok := pe.Value.(bson.A)
			if !ok {
				continue
			}
			childStart := 0
			if len(childArr) > 0 {
				if _, isInt := childArr[0].(int32); isInt {
					childStart = 1
				}
			}
			for childIdx, fields := range patches {
				childRawIdx := childStart + childIdx
				if childRawIdx >= len(childArr) {
					continue
				}
				d, ok := childArr[childRawIdx].(bson.D)
				if !ok {
					continue
				}
				for k, v := range fields {
					d = SetBSONDocField(d, k, v)
				}
				childArr[childRawIdx] = d
			}
			parentDoc[pi].Value = childArr
		}
		arr[rawIdx] = parentDoc
		doc[i].Value = arr
	}
	return bson.Marshal(doc)
}

// ScanBSONStrings recursively scans a BSON document and calls fn for each
// string value found. fn receives (fieldPath, value). Return false from fn
// to stop scanning early.
func ScanBSONStrings(data []byte, fn func(fieldPath, value string) bool) {
	scanBSONRaw(bson.Raw(data), "", fn)
}

func scanBSONRaw(raw bson.Raw, prefix string, fn func(string, string) bool) {
	elems, err := raw.Elements()
	if err != nil {
		return
	}
	for _, elem := range elems {
		key := elem.Key()
		fieldPath := key
		if prefix != "" {
			fieldPath = prefix + "." + key
		}
		val := elem.Value()
		switch val.Type {
		case bson.TypeString:
			if !fn(fieldPath, val.StringValue()) {
				return
			}
		case bson.TypeEmbeddedDocument:
			if doc, ok := val.DocumentOK(); ok {
				scanBSONRaw(doc, fieldPath, fn)
			}
		case bson.TypeArray:
			if arr, ok := val.ArrayOK(); ok {
				scanBSONRaw(bson.Raw(arr), fieldPath, fn)
			}
		}
	}
}
