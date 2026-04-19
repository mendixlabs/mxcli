// SPDX-License-Identifier: Apache-2.0

package types

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"
)

// iso8601Pattern matches common ISO 8601 datetime strings that Mendix Studio Pro
// recognizes as DateTime primitive types in JSON structures.
var iso8601Pattern = regexp.MustCompile(
	`^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}(:\d{2})?(\.\d+)?(Z|[+-]\d{2}:?\d{2})?$`,
)

// PrettyPrintJSON re-formats a JSON string with standard indentation.
// Returns the original string if it is not valid JSON.
func PrettyPrintJSON(s string) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(s), "", "  "); err != nil {
		return s
	}
	return buf.String()
}

// normalizeDateTimeValue pads fractional seconds to 7 digits to match
// Studio Pro's .NET DateTime format (e.g., "2015-05-22T14:56:29.000Z" → "2015-05-22T14:56:29.0000000Z").
func normalizeDateTimeValue(s string) string {
	// Find the decimal point after seconds
	dotIdx := strings.Index(s, ".")
	if dotIdx == -1 {
		// No fractional part — insert .0000000 before timezone suffix.
		// Search only after the time portion (index 19+ in "2015-05-22T14:56:29Z")
		// to avoid matching '-' in the date portion.
		if len(s) < 19 {
			return s
		}
		if idx := strings.IndexAny(s[19:], "Z+-"); idx >= 0 {
			return s[:19+idx] + ".0000000" + s[19+idx:]
		}
		return s
	}
	// Find where fractional digits end (at Z, +, - or end of string)
	fracEnd := len(s)
	for i := dotIdx + 1; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			fracEnd = i
			break
		}
	}
	frac := s[dotIdx+1 : fracEnd]
	if len(frac) < 7 {
		frac = frac + strings.Repeat("0", 7-len(frac))
	} else {
		frac = frac[:7]
	}
	return s[:dotIdx+1] + frac + s[fracEnd:]
}

// BuildJsonElementsFromSnippet parses a JSON snippet and builds the element tree
// that Mendix Studio Pro would generate. Returns the root element.
// The optional customNameMap maps JSON keys to custom ExposedNames (as set in
// Studio Pro's "Custom name" column). Unmapped keys use auto-generated names.
func BuildJsonElementsFromSnippet(snippet string, customNameMap map[string]string) ([]*JsonElement, error) {
	// Validate JSON
	if !json.Valid([]byte(snippet)) {
		return nil, fmt.Errorf("invalid JSON snippet")
	}

	// Detect root type (object or array)
	dec := json.NewDecoder(strings.NewReader(snippet))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON snippet: %w", err)
	}

	b := &snippetBuilder{customNameMap: customNameMap}
	tracker := &nameTracker{seen: make(map[string]int)}

	switch tok {
	case json.Delim('{'):
		root := b.buildElementFromRawObject("Root", "(Object)", snippet, tracker)
		root.MinOccurs = 0
		root.MaxOccurs = 0
		root.Nillable = true
		return []*JsonElement{root}, nil

	case json.Delim('['):
		root := b.buildElementFromRawRootArray("Root", "(Array)", snippet, tracker)
		root.MinOccurs = 0
		root.MaxOccurs = 0
		root.Nillable = true
		return []*JsonElement{root}, nil

	default:
		return nil, fmt.Errorf("JSON snippet must be an object or array at root level")
	}
}

// snippetBuilder holds state for building the element tree from a JSON snippet.
type snippetBuilder struct {
	customNameMap map[string]string // JSON key → custom ExposedName
}

// reservedExposedNames are element names that Mendix rejects as ExposedName values.
// Studio Pro handles these by prefixing with underscore and keeping original case.
var reservedExposedNames = map[string]bool{
	"Id": true, "Type": true,
}

// resolveExposedName returns the custom name if mapped, otherwise capitalizes the JSON key.
// Reserved names (Id, Type) are prefixed with underscore to match Studio Pro behavior.
func (b *snippetBuilder) resolveExposedName(jsonKey string) string {
	if b.customNameMap != nil {
		if custom, ok := b.customNameMap[jsonKey]; ok {
			return custom
		}
	}
	name := capitalizeFirst(jsonKey)
	if reservedExposedNames[name] {
		return "_" + jsonKey
	}
	return name
}

// nameTracker tracks used ExposedNames at each level to handle duplicates.
type nameTracker struct {
	seen map[string]int
}

func (t *nameTracker) uniqueName(base string) string {
	t.seen[base]++
	count := t.seen[base]
	if count == 1 {
		return base
	}
	return fmt.Sprintf("%s_%d", base, count)
}

func (t *nameTracker) child() *nameTracker {
	return &nameTracker{seen: make(map[string]int)}
}

// capitalizeFirst capitalizes the first letter of a string for ExposedName.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// buildElementFromRawObject builds an Object element by decoding a raw JSON object string,
// preserving the original key order (Go's map[string]any loses order).
func (b *snippetBuilder) buildElementFromRawObject(exposedName, path, rawJSON string, tracker *nameTracker) *JsonElement {
	elem := &JsonElement{
		ExposedName:    exposedName,
		Path:           path,
		ElementType:    "Object",
		PrimitiveType:  "Unknown",
		MinOccurs:      0,
		MaxOccurs:      0,
		Nillable:       true,
		MaxLength:      -1,
		FractionDigits: -1,
		TotalDigits:    -1,
	}

	childTracker := tracker.child()

	// Decode with key order preserved
	dec := json.NewDecoder(strings.NewReader(rawJSON))
	if _, err := dec.Token(); err != nil { // opening {
		return elem
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		key, ok := tok.(string)
		if !ok {
			continue
		}
		// Capture the raw value to pass down for nested objects/arrays
		var rawVal json.RawMessage
		if err := dec.Decode(&rawVal); err != nil {
			break
		}

		childName := childTracker.uniqueName(b.resolveExposedName(key))
		childPath := path + "|" + key
		child := b.buildElementFromRawValue(childName, childPath, key, rawVal, childTracker)
		elem.Children = append(elem.Children, child)
	}

	return elem
}

// buildElementFromRawValue inspects a json.RawMessage to determine its type and build the element.
func (b *snippetBuilder) buildElementFromRawValue(exposedName, path, jsonKey string, raw json.RawMessage, tracker *nameTracker) *JsonElement {
	trimmed := strings.TrimSpace(string(raw))

	// Object — recurse with raw JSON to preserve key order
	if len(trimmed) > 0 && trimmed[0] == '{' {
		return b.buildElementFromRawObject(exposedName, path, trimmed, tracker)
	}

	// Array
	if len(trimmed) > 0 && trimmed[0] == '[' {
		return b.buildElementFromRawArray(exposedName, path, jsonKey, trimmed, tracker)
	}

	// Primitive — unmarshal to determine type
	var val any
	if err := json.Unmarshal(raw, &val); err != nil {
		return buildValueElement(exposedName, path, "Unknown", string(raw))
	}

	switch v := val.(type) {
	case string:
		primitiveType := "String"
		if iso8601Pattern.MatchString(v) {
			primitiveType = "DateTime"
			v = normalizeDateTimeValue(v)
		}
		return buildValueElement(exposedName, path, primitiveType, fmt.Sprintf("%q", v))
	case float64:
		// Check the raw JSON text for a decimal point — Go's %v drops ".0" from 41850.0
		if v == math.Trunc(v) && !strings.Contains(trimmed, ".") && v >= -(1<<53) && v <= (1<<53) {
			return buildValueElement(exposedName, path, "Integer", fmt.Sprintf("%v", int64(v)))
		}
		return buildValueElement(exposedName, path, "Decimal", fmt.Sprintf("%v", v))
	case bool:
		return buildValueElement(exposedName, path, "Boolean", fmt.Sprintf("%v", v))
	case nil:
		// JSON null → Unknown primitive type (matches Studio Pro)
		return buildValueElement(exposedName, path, "Unknown", "")
	default:
		return buildValueElement(exposedName, path, "String", "")
	}
}

// buildElementFromRawRootArray builds a root-level Array element.
// Studio Pro names the child object "JsonObject" (not "RootItem") for root arrays.
func (b *snippetBuilder) buildElementFromRawRootArray(exposedName, path, rawJSON string, tracker *nameTracker) *JsonElement {
	arrayElem := &JsonElement{
		ExposedName:    exposedName,
		Path:           path,
		ElementType:    "Array",
		PrimitiveType:  "Unknown",
		MinOccurs:      0,
		MaxOccurs:      0,
		Nillable:       true,
		MaxLength:      -1,
		FractionDigits: -1,
		TotalDigits:    -1,
	}

	dec := json.NewDecoder(strings.NewReader(rawJSON))
	if _, err := dec.Token(); err != nil { // opening [
		return arrayElem
	}
	if dec.More() {
		var firstItem json.RawMessage
		if err := dec.Decode(&firstItem); err != nil {
			return arrayElem
		}

		itemPath := path + "|(Object)"
		trimmed := strings.TrimSpace(string(firstItem))

		if len(trimmed) > 0 && trimmed[0] == '{' {
			itemElem := b.buildElementFromRawObject("JsonObject", itemPath, trimmed, tracker)
			itemElem.MinOccurs = 0
			itemElem.MaxOccurs = 0
			itemElem.Nillable = true
			arrayElem.Children = append(arrayElem.Children, itemElem)
		} else {
			child := b.buildElementFromRawValue("JsonObject", itemPath, "", firstItem, tracker)
			child.MinOccurs = 0
			child.MaxOccurs = 0
			arrayElem.Children = append(arrayElem.Children, child)
		}
	}

	return arrayElem
}

// buildElementFromRawArray builds an Array element, using the first item's raw JSON for ordering.
// For primitive arrays (strings, numbers), Studio Pro creates a Wrapper element with a Value child.
func (b *snippetBuilder) buildElementFromRawArray(exposedName, path, jsonKey, rawJSON string, tracker *nameTracker) *JsonElement {
	arrayElem := &JsonElement{
		ExposedName:    exposedName,
		Path:           path,
		ElementType:    "Array",
		PrimitiveType:  "Unknown",
		MinOccurs:      0,
		MaxOccurs:      0,
		Nillable:       true,
		MaxLength:      -1,
		FractionDigits: -1,
		TotalDigits:    -1,
	}

	// Decode array and get first element as raw JSON
	dec := json.NewDecoder(strings.NewReader(rawJSON))
	if _, err := dec.Token(); err != nil { // opening [
		return arrayElem
	}
	if dec.More() {
		var firstItem json.RawMessage
		if err := dec.Decode(&firstItem); err != nil {
			return arrayElem
		}

		trimmed := strings.TrimSpace(string(firstItem))

		if len(trimmed) > 0 && trimmed[0] == '{' {
			// Object array: child is NameItem object
			itemName := exposedName + "Item"
			itemPath := path + "|(Object)"
			itemElem := b.buildElementFromRawObject(itemName, itemPath, trimmed, tracker)
			itemElem.MinOccurs = 0
			itemElem.MaxOccurs = -1
			itemElem.Nillable = true
			arrayElem.Children = append(arrayElem.Children, itemElem)
		} else {
			// Primitive array: Studio Pro wraps in a Wrapper element with singular name
			// e.g., tags: ["a","b"] → Tag (Wrapper) → Value (String)
			wrapperName := singularize(exposedName)
			wrapperPath := path + "|(Object)"
			wrapper := &JsonElement{
				ExposedName:    wrapperName,
				Path:           wrapperPath,
				ElementType:    "Wrapper",
				PrimitiveType:  "Unknown",
				MinOccurs:      0,
				MaxOccurs:      0,
				Nillable:       true,
				MaxLength:      -1,
				FractionDigits: -1,
				TotalDigits:    -1,
			}
			valueElem := b.buildElementFromRawValue("Value", wrapperPath+"|", jsonKey, firstItem, tracker)
			valueElem.MinOccurs = 0
			valueElem.MaxOccurs = 0
			wrapper.Children = append(wrapper.Children, valueElem)
			arrayElem.Children = append(arrayElem.Children, wrapper)
		}
	}

	return arrayElem
}

// singularize returns a simple singular form by stripping trailing "s".
// Handles common cases: Tags→Tag, Items→Item, Addresses→Addresse.
func singularize(s string) string {
	if len(s) > 1 && strings.HasSuffix(s, "s") {
		return s[:len(s)-1]
	}
	return s
}

func buildValueElement(exposedName, path, primitiveType, originalValue string) *JsonElement {
	maxLength := -1
	if primitiveType == "String" {
		maxLength = 0
	}
	return &JsonElement{
		ExposedName:    exposedName,
		Path:           path,
		ElementType:    "Value",
		PrimitiveType:  primitiveType,
		MinOccurs:      0,
		MaxOccurs:      0,
		Nillable:       true,
		MaxLength:      maxLength,
		FractionDigits: -1,
		TotalDigits:    -1,
		OriginalValue:  originalValue,
	}
}
