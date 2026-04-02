// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/mendixlabs/mxcli/model"

	"go.mongodb.org/mongo-driver/bson"
)

// dateTimeRegex matches ISO 8601 datetime strings (must have date + T + time).
var dateTimeRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`)

// parseDateTime attempts to parse an ISO 8601 datetime string and reformat it
// to Studio Pro's canonical form: 7 decimal places (100-nanosecond precision), local timezone offset.
// Returns (formatted, true) if recognized, ("", false) otherwise.
func parseDateTime(s string) (string, bool) {
	if !dateTimeRegex.MatchString(s) {
		return "", false
	}
	var t time.Time
	var err error
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		t, err = time.Parse(layout, s)
		if err == nil {
			break
		}
	}
	if err != nil {
		return "", false
	}
	t = t.In(time.Local)
	// Format with exactly 7 decimal places (Studio Pro uses .NET 100-nanosecond ticks)
	// and the local timezone offset (e.g. +02:00), matching Studio Pro's canonical form.
	hundredNanos := t.Nanosecond() / 100
	_, offsetSec := t.Zone()
	sign := "+"
	if offsetSec < 0 {
		sign = "-"
		offsetSec = -offsetSec
	}
	offsetHours := offsetSec / 3600
	offsetMins := (offsetSec % 3600) / 60
	formatted := t.Format("2006-01-02T15:04:05") +
		fmt.Sprintf(".%07d", hundredNanos) +
		fmt.Sprintf("%s%02d:%02d", sign, offsetHours, offsetMins)
	return `"` + formatted + `"`, true
}

// CreateJsonStructure creates a new JSON structure document.
func (w *Writer) CreateJsonStructure(js *model.JsonStructure) error {
	if js.ID == "" {
		js.ID = model.ID(generateUUID())
	}
	js.TypeName = "JsonStructures$JsonStructure"

	contents, err := w.serializeJsonStructure(js)
	if err != nil {
		return fmt.Errorf("failed to serialize JSON structure: %w", err)
	}

	return w.insertUnit(string(js.ID), string(js.ContainerID), "Documents", "JsonStructures$JsonStructure", contents)
}

// UpdateJsonStructure updates an existing JSON structure document.
func (w *Writer) UpdateJsonStructure(js *model.JsonStructure) error {
	contents, err := w.serializeJsonStructure(js)
	if err != nil {
		return fmt.Errorf("failed to serialize JSON structure: %w", err)
	}
	return w.updateUnit(string(js.ID), contents)
}

// DeleteJsonStructure deletes a JSON structure document.
func (w *Writer) DeleteJsonStructure(id model.ID) error {
	return w.deleteUnit(string(id))
}

// MoveJsonStructure moves a JSON structure to a new container.
func (w *Writer) MoveJsonStructure(js *model.JsonStructure) error {
	return w.moveUnitByID(string(js.ID), string(js.ContainerID))
}

func (w *Writer) serializeJsonStructure(js *model.JsonStructure) ([]byte, error) {
	elements := bson.A{int32(2)}
	for _, elem := range js.Elements {
		elements = append(elements, serializeJsonElement(elem))
	}

	exportLevel := js.ExportLevel
	if exportLevel == "" {
		exportLevel = "Hidden"
	}

	doc := bson.M{
		"$ID":           idToBsonBinary(string(js.ID)),
		"$Type":         "JsonStructures$JsonStructure",
		"Name":          js.Name,
		"Documentation": js.Documentation,
		"Excluded":      js.Excluded,
		"ExportLevel":   exportLevel,
		"JsonSnippet":   js.JsonSnippet,
		"Elements":      elements,
	}
	return bson.Marshal(doc)
}

func serializeJsonElement(elem *model.JsonElement) bson.M {
	id := string(elem.ID)
	if id == "" {
		id = generateUUID()
	}

	children := bson.A{int32(2)}
	for _, child := range elem.Children {
		children = append(children, serializeJsonElement(child))
	}

	maxOccurs := elem.MaxOccurs
	if maxOccurs == 0 {
		maxOccurs = 1
	}

	primitiveType := elem.PrimitiveType
	if primitiveType == "" {
		primitiveType = "Unknown"
	}

	fractionDigits := elem.FractionDigits
	if fractionDigits == 0 {
		fractionDigits = -1
	}
	totalDigits := elem.TotalDigits
	if totalDigits == 0 {
		totalDigits = -1
	}
	// MaxLength is -1 for Object/Array (set in deriveElement), 0 for Value
	maxLength := elem.MaxLength

	return bson.M{
		"$ID":             idToBsonBinary(id),
		"$Type":           "JsonStructures$JsonElement",
		"ExposedName":     elem.ExposedName,
		"ExposedItemName": elem.ExposedItemName,
		"ElementType":     elem.ElementType,
		"PrimitiveType":   primitiveType,
		"Path":            elem.Path,
		"OriginalValue":   elem.OriginalValue,
		"MinOccurs":       int32(elem.MinOccurs),
		"MaxOccurs":       int32(maxOccurs),
		"FractionDigits":  int32(fractionDigits),
		"TotalDigits":     int32(totalDigits),
		"MaxLength":       int32(maxLength),
		"Nillable":        elem.Nillable,
		"IsDefaultType":   elem.IsDefaultType,
		"ErrorMessage":    "",
		"WarningMessage":  "",
		"Children":        children,
	}
}

// FormatJsonSnippet pretty-prints a JSON string with 2-space indentation,
// matching the output of Studio Pro's "Format" button.
func FormatJsonSnippet(snippet string) (string, error) {
	var raw any
	if err := json.Unmarshal([]byte(snippet), &raw); err != nil {
		return "", fmt.Errorf("invalid JSON: %w", err)
	}
	formatted, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return "", err
	}
	return string(formatted), nil
}

// DeriveJsonElementsFromSnippet parses a raw JSON string and builds a JsonElement tree.
// The root element represents the top-level JSON object with ExposedName "Root".
func DeriveJsonElementsFromSnippet(snippet string) ([]*model.JsonElement, error) {
	var raw any
	if err := json.Unmarshal([]byte(snippet), &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON snippet: %w", err)
	}

	root := deriveElement("Root", "", "(Object)", raw)
	root.MinOccurs = 1 // root element is required (MinOccurs=1), all children are optional (0)
	return []*model.JsonElement{root}, nil
}

// deriveElement recursively derives a JsonElement from a JSON value.
// exposedName is the display name (PascalCase), jsonKey is the original JSON key used in paths,
// parentPath is the accumulated path of the parent element.
func deriveElement(exposedName, jsonKey, parentPath string, val any) *model.JsonElement {
	// Compute this element's path: for the root (jsonKey="") use parentPath directly;
	// otherwise append |jsonKey to parentPath.
	path := parentPath
	if jsonKey != "" {
		path = parentPath + "|" + jsonKey
	}

	elem := &model.JsonElement{
		BaseElement: model.BaseElement{
			ID:       model.ID(generateUUID()),
			TypeName: "JsonStructures$JsonElement",
		},
		ExposedName: exposedName,
		Path:        path,
		MaxOccurs:   1,
		Nillable:    true,
	}

	switch v := val.(type) {
	case map[string]any:
		elem.ElementType = "Object"
		elem.PrimitiveType = "Unknown"
		elem.MaxLength = -1
		// Sort keys to match the alphabetical order produced by FormatJsonSnippet
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			elem.Children = append(elem.Children, deriveElement(capitalizeFirst(key), key, path, v[key]))
		}
	case []any:
		elem.ElementType = "Array"
		elem.PrimitiveType = "Unknown"
		elem.MaxLength = -1
		itemName := singularize(exposedName)
		// ExposedItemName on the Array container must be "" — Studio Pro expects this.
		// The singularized name is only used as ExposedName of the child item Object element.
		// Derive schema from first element if available; item path appends |(Object)
		if len(v) > 0 {
			itemElem := deriveElement(itemName, "", path+"|(Object)", v[0])
			itemElem.MaxOccurs = -1
			elem.Children = append(elem.Children, itemElem)
		}
	case string:
		elem.ElementType = "Value"
		if formatted, ok := parseDateTime(v); ok {
			// Studio Pro detects ISO 8601 datetime strings as DateTime, not String.
			// MaxLength for DateTime is -1 (same as Integer/Boolean), not 0.
			elem.PrimitiveType = "DateTime"
			elem.MaxLength = -1
			elem.OriginalValue = formatted
		} else {
			elem.PrimitiveType = "String"
			elem.OriginalValue = `"` + v + `"`
		}
	case float64:
		// Integer/Decimal values use MaxLength=-1 (not applicable), unlike String which uses MaxLength=0.
		elem.ElementType = "Value"
		elem.MaxLength = -1
		if v == float64(int64(v)) {
			elem.PrimitiveType = "Integer"
			elem.OriginalValue = fmt.Sprintf("%d", int64(v))
		} else {
			elem.PrimitiveType = "Decimal"
			elem.OriginalValue = fmt.Sprintf("%g", v)
		}
	case bool:
		// Boolean values use MaxLength=-1 (not applicable), unlike String which uses MaxLength=0.
		elem.ElementType = "Value"
		elem.MaxLength = -1
		elem.PrimitiveType = "Boolean"
		if v {
			elem.OriginalValue = "true"
		} else {
			elem.OriginalValue = "false"
		}
	case nil:
		elem.ElementType = "Value"
		elem.PrimitiveType = "String"
		elem.Nillable = true
	default:
		elem.ElementType = "Value"
		elem.PrimitiveType = "String"
	}

	return elem
}

// capitalizeFirst uppercases the first rune of s, leaving the rest unchanged.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// singularize returns a simple singular form of a plural word for array item names.
func singularize(name string) string {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, "data") {
		return name[:len(name)-1] + "um" // data → datum
	}
	if strings.HasSuffix(lower, "ies") {
		return name[:len(name)-3] + "y" // Countries → Country, LegalEntities → LegalEntity
	}
	if len(name) > 1 && name[len(name)-1] == 's' {
		return name[:len(name)-1]
	}
	return name + "Item"
}
