// SPDX-License-Identifier: Apache-2.0

// Package mpr - Widget template functionality for Reader.
package mpr

import (
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// RawCustomWidgetType holds the raw BSON data for a CustomWidgetType
// extracted from an existing widget in the project.
type RawCustomWidgetType struct {
	WidgetID   string // e.g., "com.mendix.widget.web.combobox.Combobox"
	RawType    bson.D // The full Type field as bson.D
	RawObject  bson.D // The full Object field as bson.D (WidgetObject with all properties)
	UnitID     string // ID of the unit where this was found
	UnitName   string // Name of the page/snippet (for identification)
	WidgetName string // Name of the widget (from Name field)
}

// FindCustomWidgetType searches for an existing CustomWidget with the given
// widgetID and returns its full Type definition as raw BSON. This can be used
// as a template for creating new widgets of the same type.
func (r *Reader) FindCustomWidgetType(widgetID string) (*RawCustomWidgetType, error) {
	// Search through all pages for a CustomWidget with the matching widgetID
	units, err := r.listUnitsByType("Forms$Page")
	if err != nil {
		return nil, err
	}

	// Also check snippets
	snippetUnits, err := r.listUnitsByType("Forms$Snippet")
	if err == nil {
		units = append(units, snippetUnits...)
	}

	for _, u := range units {
		contents, err := r.resolveContents(u.ID, u.Contents)
		if err != nil {
			continue
		}

		// Quick check if this unit might contain the widget
		if !containsWidgetID(contents, widgetID) {
			continue
		}

		// Parse and extract the widget type and object
		rawType, rawObject := extractWidgetTypeAndObject(contents, widgetID)
		if rawType != nil {
			return &RawCustomWidgetType{
				WidgetID:  widgetID,
				RawType:   rawType,
				RawObject: rawObject,
				UnitID:    u.ID,
			}, nil
		}
	}

	return nil, nil // Not found
}

// FindAllCustomWidgetTypes searches for ALL CustomWidgets with the given
// widgetID and returns their full Type/Object definitions as raw BSON.
// This allows identification of different configurations of the same widget type.
func (r *Reader) FindAllCustomWidgetTypes(widgetID string) ([]*RawCustomWidgetType, error) {
	var results []*RawCustomWidgetType

	// Search through all pages
	units, err := r.listUnitsByType("Forms$Page")
	if err != nil {
		return nil, err
	}

	// Also check snippets
	snippetUnits, err := r.listUnitsByType("Forms$Snippet")
	if err == nil {
		units = append(units, snippetUnits...)
	}

	for _, u := range units {
		contents, err := r.resolveContents(u.ID, u.Contents)
		if err != nil {
			continue
		}

		// Quick check if this unit might contain the widget
		if !containsWidgetID(contents, widgetID) {
			continue
		}

		// Get the unit name for identification
		unitName := extractUnitName(contents)

		// Parse and extract ALL widgets of this type from this unit
		var doc bson.D
		if err := bson.Unmarshal(contents, &doc); err != nil {
			continue
		}

		widgets := findAllCustomWidgets(doc, widgetID)
		for _, w := range widgets {
			results = append(results, &RawCustomWidgetType{
				WidgetID:   widgetID,
				RawType:    w.rawType,
				RawObject:  w.rawObject,
				UnitID:     u.ID,
				UnitName:   unitName,
				WidgetName: w.name,
			})
		}
	}

	return results, nil
}

// extractUnitName extracts the Name field from a BSON document.
func extractUnitName(contents []byte) string {
	var doc bson.D
	if err := bson.Unmarshal(contents, &doc); err != nil {
		return ""
	}
	for _, elem := range doc {
		if elem.Key == "Name" {
			if name, ok := elem.Value.(string); ok {
				return name
			}
		}
	}
	return ""
}

// widgetInfo holds extracted widget data.
type widgetInfo struct {
	rawType   bson.D
	rawObject bson.D
	name      string
}

// findAllCustomWidgets recursively searches for ALL CustomWidgets with the given widgetID.
func findAllCustomWidgets(doc bson.D, widgetID string) []widgetInfo {
	var results []widgetInfo

	// Check if this document is a CustomWidget with matching widgetID
	isCustomWidget := false
	var typeDoc, objectDoc bson.D
	var widgetName string

	for _, elem := range doc {
		if elem.Key == "$Type" && elem.Value == "CustomWidgets$CustomWidget" {
			isCustomWidget = true
		}
		if elem.Key == "Type" {
			if t, ok := elem.Value.(bson.D); ok {
				typeDoc = t
			}
		}
		if elem.Key == "Object" {
			if o, ok := elem.Value.(bson.D); ok {
				objectDoc = o
			}
		}
		if elem.Key == "Name" {
			if n, ok := elem.Value.(string); ok {
				widgetName = n
			}
		}
	}

	// If this is a CustomWidget with matching widgetID, add to results
	if isCustomWidget && typeDoc != nil && matchesWidgetID(typeDoc, widgetID) {
		results = append(results, widgetInfo{
			rawType:   typeDoc,
			rawObject: objectDoc,
			name:      widgetName,
		})
	}

	// Recursively search nested documents
	for _, elem := range doc {
		switch v := elem.Value.(type) {
		case bson.D:
			results = append(results, findAllCustomWidgets(v, widgetID)...)
		case bson.A:
			results = append(results, findAllCustomWidgetsInArray(v, widgetID)...)
		}
	}

	return results
}

// findAllCustomWidgetsInArray searches an array for CustomWidgets.
func findAllCustomWidgetsInArray(arr bson.A, widgetID string) []widgetInfo {
	var results []widgetInfo
	for _, item := range arr {
		switch v := item.(type) {
		case bson.D:
			results = append(results, findAllCustomWidgets(v, widgetID)...)
		case bson.A:
			results = append(results, findAllCustomWidgetsInArray(v, widgetID)...)
		}
	}
	return results
}

// GetPropertyValue extracts a property value from a RawObject by property key.
func (r *RawCustomWidgetType) GetPropertyValue(propertyKey string) string {
	if r.RawObject == nil {
		return ""
	}
	for _, elem := range r.RawObject {
		if elem.Key == "Properties" {
			if arr, ok := elem.Value.(bson.A); ok {
				for _, item := range arr {
					if prop, ok := item.(bson.D); ok {
						propKey := getPropertyKey(prop)
						if propKey == propertyKey {
							return getPrimitiveValue(prop)
						}
					}
				}
			}
		}
	}
	return ""
}

// getPropertyKey extracts the property key from a WidgetProperty.
func getPropertyKey(prop bson.D) string {
	for _, elem := range prop {
		if elem.Key == "TypePointer" {
			// We can't easily map TypePointer to PropertyKey without the Type
			// So let's look for it differently
		}
	}
	// Check Value for the property type info
	for _, elem := range prop {
		if elem.Key == "Value" {
			if val, ok := elem.Value.(bson.D); ok {
				for _, ve := range val {
					if ve.Key == "$Type" {
						// The type hints at what property this is
						return ve.Value.(string)
					}
				}
			}
		}
	}
	return ""
}

// getPrimitiveValue extracts the PrimitiveValue from a WidgetProperty.
func getPrimitiveValue(prop bson.D) string {
	for _, elem := range prop {
		if elem.Key == "Value" {
			if val, ok := elem.Value.(bson.D); ok {
				for _, ve := range val {
					if ve.Key == "PrimitiveValue" {
						if pv, ok := ve.Value.(string); ok {
							return pv
						}
					}
				}
			}
		}
	}
	return ""
}

// GetAllPrimitiveValues returns all non-empty PrimitiveValue fields from the RawObject.
func (r *RawCustomWidgetType) GetAllPrimitiveValues() []string {
	if r.RawObject == nil {
		return nil
	}
	var values []string
	for _, elem := range r.RawObject {
		if elem.Key == "Properties" {
			if arr, ok := elem.Value.(bson.A); ok {
				for _, item := range arr {
					if prop, ok := item.(bson.D); ok {
						if pv := getPrimitiveValue(prop); pv != "" {
							values = append(values, pv)
						}
					}
				}
			}
		}
	}
	return values
}

// containsWidgetID does a quick string check to see if the BSON might contain the widget.
func containsWidgetID(contents []byte, widgetID string) bool {
	return strings.Contains(string(contents), widgetID)
}

// extractWidgetTypeAndObject parses BSON and extracts both the CustomWidgetType and WidgetObject
// for the given widgetID. This allows cloning the complete widget with all its property values.
func extractWidgetTypeAndObject(contents []byte, widgetID string) (bson.D, bson.D) {
	var doc bson.D
	if err := bson.Unmarshal(contents, &doc); err != nil {
		return nil, nil
	}

	// Recursively search for CustomWidget with matching widgetID
	return findCustomWidget(doc, widgetID)
}

// findCustomWidget recursively searches for a CustomWidget with the given widgetID
// and returns both its Type and Object fields.
func findCustomWidget(doc bson.D, widgetID string) (bson.D, bson.D) {
	// Check if this document is a CustomWidget with matching widgetID
	isCustomWidget := false
	var typeDoc, objectDoc bson.D

	for _, elem := range doc {
		if elem.Key == "$Type" && elem.Value == "CustomWidgets$CustomWidget" {
			isCustomWidget = true
		}
		if elem.Key == "Type" {
			if t, ok := elem.Value.(bson.D); ok {
				typeDoc = t
			}
		}
		if elem.Key == "Object" {
			if o, ok := elem.Value.(bson.D); ok {
				objectDoc = o
			}
		}
	}

	// If this is a CustomWidget with matching widgetID, return its Type and Object
	if isCustomWidget && typeDoc != nil && matchesWidgetID(typeDoc, widgetID) {
		return typeDoc, objectDoc
	}

	// Recursively search nested documents
	for _, elem := range doc {
		switch v := elem.Value.(type) {
		case bson.D:
			if t, o := findCustomWidget(v, widgetID); t != nil {
				return t, o
			}
		case bson.A:
			if t, o := findCustomWidgetInArray(v, widgetID); t != nil {
				return t, o
			}
		}
	}

	return nil, nil
}

// findCustomWidgetInArray searches an array for a CustomWidget.
func findCustomWidgetInArray(arr bson.A, widgetID string) (bson.D, bson.D) {
	for _, item := range arr {
		switch v := item.(type) {
		case bson.D:
			if t, o := findCustomWidget(v, widgetID); t != nil {
				return t, o
			}
		case bson.A:
			if t, o := findCustomWidgetInArray(v, widgetID); t != nil {
				return t, o
			}
		}
	}
	return nil, nil
}

// matchesWidgetID checks if a BSON document is a CustomWidgetType with the given widgetID.
func matchesWidgetID(doc bson.D, widgetID string) bool {
	hasCorrectType := false
	hasCorrectWidgetID := false

	for _, elem := range doc {
		if elem.Key == "$Type" && elem.Value == "CustomWidgets$CustomWidgetType" {
			hasCorrectType = true
		}
		if elem.Key == "WidgetId" && elem.Value == widgetID {
			hasCorrectWidgetID = true
		}
	}

	return hasCorrectType && hasCorrectWidgetID
}

// IDMapping tracks the mapping from old IDs to new IDs during cloning.
type IDMapping struct {
	OldToNewID      map[string]string                    // Maps old ID -> new ID for all elements
	PropertyTypeIDs map[string]pages.PropertyTypeIDEntry // Maps PropertyKey -> PropertyTypeID/ValueTypeID
	ObjectTypeID    string                               // The cloned ObjectType ID
}

// CloneWidgetType creates a deep copy of the widget type with all IDs regenerated.
// It returns a mapping from old PropertyType keys to new PropertyType IDs and ValueType IDs,
// as well as the ObjectType ID which is needed for the WidgetObject's TypePointer.
func CloneWidgetType(rawType bson.D) (cloned bson.D, propertyTypeIDs map[string]pages.PropertyTypeIDEntry, objectTypeID string) {
	mapping := &IDMapping{
		OldToNewID:      make(map[string]string),
		PropertyTypeIDs: make(map[string]pages.PropertyTypeIDEntry),
	}
	cloned = cloneDocWithNewIDs(rawType, mapping)
	return cloned, mapping.PropertyTypeIDs, mapping.ObjectTypeID
}

// CloneCustomWidgetType is an alias for CloneWidgetType for clarity.
func CloneCustomWidgetType(rawType bson.D) (cloned bson.D, propertyTypeIDs map[string]pages.PropertyTypeIDEntry, objectTypeID string) {
	return CloneWidgetType(rawType)
}

// CloneWidgetObject creates a deep copy of a WidgetObject with all IDs regenerated.
// The idMapping is used to update TypePointers to reference the new IDs from the cloned Type.
func CloneWidgetObject(rawObject bson.D, idMapping map[string]string) bson.D {
	if rawObject == nil {
		return nil
	}
	return cloneObjectWithNewIDs(rawObject, idMapping)
}

// CloneCustomWidget clones both the Type and Object of a CustomWidget.
// Returns the cloned Type, cloned Object, PropertyType IDs map, and ObjectType ID.
func CloneCustomWidget(rawType, rawObject bson.D) (clonedType, clonedObject bson.D, propertyTypeIDs map[string]pages.PropertyTypeIDEntry, objectTypeID string) {
	mapping := &IDMapping{
		OldToNewID:      make(map[string]string),
		PropertyTypeIDs: make(map[string]pages.PropertyTypeIDEntry),
	}

	// Clone the Type first to build the ID mapping
	clonedType = cloneDocWithNewIDs(rawType, mapping)

	// Clone the Object using the ID mapping to update TypePointers
	if rawObject != nil {
		clonedObject = cloneObjectWithNewIDs(rawObject, mapping.OldToNewID)
	}

	return clonedType, clonedObject, mapping.PropertyTypeIDs, mapping.ObjectTypeID
}

// ExtractPropertyTypeIDs extracts PropertyType IDs from a widget type WITHOUT regenerating IDs.
// This is used when creating new widget instances that reference an EXISTING widget type in the project.
// The TypePointers in the new instance must use the ORIGINAL IDs from the project's widget type.
func ExtractPropertyTypeIDs(rawType bson.D) (propertyTypeIDs map[string]pages.PropertyTypeIDEntry, objectTypeID string) {
	propertyTypeIDs = make(map[string]pages.PropertyTypeIDEntry)
	extractPropertyTypeIDsFromDoc(rawType, propertyTypeIDs, &objectTypeID)
	return propertyTypeIDs, objectTypeID
}

// extractPropertyTypeIDsFromDoc recursively extracts PropertyType/ValueType IDs without regenerating them.
func extractPropertyTypeIDsFromDoc(doc bson.D, propertyTypeIDs map[string]pages.PropertyTypeIDEntry, objectTypeID *string) {
	var currentPropertyKey string
	var currentID string
	var currentValueTypeID string
	var currentDefaultValue string
	var currentValueType string
	var currentObjectTypeID string
	var currentNestedPropertyIDs map[string]pages.PropertyTypeIDEntry
	var docType string

	// First pass: collect all values from this document
	for _, elem := range doc {
		switch elem.Key {
		case "$Type":
			if t, ok := elem.Value.(string); ok {
				docType = t
			}
		case "$ID":
			if binID, ok := elem.Value.(primitive.Binary); ok {
				currentID = blobToUUID(binID.Data)
			}
		case "PropertyKey":
			if key, ok := elem.Value.(string); ok {
				currentPropertyKey = key
			}
		case "ValueType":
			if nested, ok := elem.Value.(bson.D); ok {
				currentNestedPropertyIDs = make(map[string]pages.PropertyTypeIDEntry)
				extractValueTypeInfo(nested, &currentValueTypeID, &currentDefaultValue, &currentValueType, &currentObjectTypeID, currentNestedPropertyIDs)
			}
		}
	}

	// After collecting values, determine what type this is and record IDs
	isPropertyType := docType == "CustomWidgets$WidgetPropertyType"
	isObjectType := docType == "CustomWidgets$WidgetObjectType"

	if isObjectType && currentID != "" {
		*objectTypeID = currentID
	}

	// Record PropertyType entry
	if isPropertyType && currentPropertyKey != "" {
		propertyTypeIDs[currentPropertyKey] = pages.PropertyTypeIDEntry{
			PropertyTypeID:    currentID, // Use the ID we collected
			ValueTypeID:       currentValueTypeID,
			DefaultValue:      currentDefaultValue,
			ValueType:         currentValueType,
			ObjectTypeID:      currentObjectTypeID,
			NestedPropertyIDs: currentNestedPropertyIDs,
		}
	}

	// Second pass: recurse into nested documents and arrays
	for _, elem := range doc {
		if elem.Key == "ValueType" {
			continue // Already processed
		}
		if nested, ok := elem.Value.(bson.D); ok {
			extractPropertyTypeIDsFromDoc(nested, propertyTypeIDs, objectTypeID)
		}
		if arr, ok := elem.Value.(bson.A); ok {
			for _, item := range arr {
				if nested, ok := item.(bson.D); ok {
					extractPropertyTypeIDsFromDoc(nested, propertyTypeIDs, objectTypeID)
				}
			}
		}
	}
}

// extractValueTypeInfo extracts ValueType ID, default value, value type, and nested ObjectType info.
func extractValueTypeInfo(doc bson.D, valueTypeID, defaultValue, valueType *string, objectTypeID *string, nestedPropertyIDs map[string]pages.PropertyTypeIDEntry) {
	for _, elem := range doc {
		if elem.Key == "$ID" {
			if binID, ok := elem.Value.(primitive.Binary); ok {
				*valueTypeID = blobToUUID(binID.Data)
			}
		}
		if elem.Key == "DefaultValue" {
			if dv, ok := elem.Value.(string); ok {
				*defaultValue = dv
			}
		}
		if elem.Key == "Type" {
			if vt, ok := elem.Value.(string); ok {
				*valueType = vt
			}
		}
		if elem.Key == "ObjectType" {
			if nested, ok := elem.Value.(bson.D); ok {
				extractObjectTypeInfo(nested, objectTypeID, nestedPropertyIDs)
			}
		}
	}
}

// extractObjectTypeInfo extracts ObjectType ID and its nested PropertyType IDs.
func extractObjectTypeInfo(doc bson.D, objectTypeID *string, nestedPropertyIDs map[string]pages.PropertyTypeIDEntry) {
	var dummyObjectTypeID string
	for _, elem := range doc {
		if elem.Key == "$ID" {
			if binID, ok := elem.Value.(primitive.Binary); ok {
				*objectTypeID = blobToUUID(binID.Data)
			}
		}
		if elem.Key == "PropertyTypes" {
			if arr, ok := elem.Value.(bson.A); ok {
				for _, item := range arr {
					if propType, ok := item.(bson.D); ok {
						extractPropertyTypeIDsFromDoc(propType, nestedPropertyIDs, &dummyObjectTypeID)
					}
				}
			}
		}
	}
}

// cloneDocWithNewIDs recursively clones a BSON document with regenerated IDs.
// It builds an ID mapping and tracks PropertyType/ValueType IDs.
func cloneDocWithNewIDs(doc bson.D, mapping *IDMapping) bson.D {
	result := make(bson.D, 0, len(doc))

	// First pass: check if this is a PropertyType or ObjectType and extract its key and old ID
	var currentPropertyKey string
	var currentPropertyTypeID string
	var currentValueTypeID string
	var oldID string
	isPropertyType := false
	isObjectType := false
	isValueType := false

	for _, elem := range doc {
		if elem.Key == "$Type" {
			switch elem.Value {
			case "CustomWidgets$WidgetPropertyType":
				isPropertyType = true
			case "CustomWidgets$WidgetObjectType":
				isObjectType = true
			case "CustomWidgets$WidgetValueType":
				isValueType = true
			}
		}
		if elem.Key == "PropertyKey" {
			if key, ok := elem.Value.(string); ok {
				currentPropertyKey = key
			}
		}
		if elem.Key == "$ID" {
			if binID, ok := elem.Value.(primitive.Binary); ok {
				oldID = blobToUUID(binID.Data)
			}
		}
	}

	// Clone each element
	for _, elem := range doc {
		newElem := bson.E{Key: elem.Key}

		if elem.Key == "$ID" {
			// Generate new ID and record the mapping
			newID := generateUUID()
			newElem.Value = idToBsonBinary(newID)

			// Store the old -> new ID mapping
			if oldID != "" {
				mapping.OldToNewID[oldID] = newID
			}

			// Track PropertyType and ValueType IDs
			if isPropertyType {
				currentPropertyTypeID = newID
			}
			if isValueType {
				currentValueTypeID = newID
			}
			// Track ObjectType ID for WidgetObject reference
			if isObjectType {
				mapping.ObjectTypeID = newID
			}
		} else {
			// Clone the value
			switch v := elem.Value.(type) {
			case bson.D:
				// Recursively clone nested document
				clonedNested := cloneDocWithNewIDs(v, mapping)
				newElem.Value = clonedNested

				// If this is a ValueType, extract its new ID
				if elem.Key == "ValueType" {
					for _, e := range clonedNested {
						if e.Key == "$ID" {
							if binID, ok := e.Value.(primitive.Binary); ok {
								currentValueTypeID = blobToUUID(binID.Data)
							}
							break
						}
					}
				}
			case bson.A:
				newElem.Value = cloneArrayWithNewIDs(v, mapping)
			default:
				newElem.Value = v
			}
		}

		result = append(result, newElem)
	}

	// Record PropertyType IDs
	if isPropertyType && currentPropertyKey != "" {
		mapping.PropertyTypeIDs[currentPropertyKey] = pages.PropertyTypeIDEntry{
			PropertyTypeID: currentPropertyTypeID,
			ValueTypeID:    currentValueTypeID,
		}
	}

	return result
}

// cloneArrayWithNewIDs recursively clones a BSON array with regenerated IDs.
func cloneArrayWithNewIDs(arr bson.A, mapping *IDMapping) bson.A {
	result := make(bson.A, len(arr))
	for i, item := range arr {
		switch v := item.(type) {
		case bson.D:
			result[i] = cloneDocWithNewIDs(v, mapping)
		case bson.A:
			result[i] = cloneArrayWithNewIDs(v, mapping)
		default:
			result[i] = v
		}
	}
	return result
}

// cloneObjectWithNewIDs clones a WidgetObject with new IDs, updating TypePointers
// to reference the new IDs from the cloned Type.
func cloneObjectWithNewIDs(doc bson.D, idMapping map[string]string) bson.D {
	result := make(bson.D, 0, len(doc))

	for _, elem := range doc {
		newElem := bson.E{Key: elem.Key}

		if elem.Key == "$ID" {
			// Generate new ID for the object itself
			newID := generateUUID()
			newElem.Value = idToBsonBinary(newID)
		} else if elem.Key == "TypePointer" {
			// Update TypePointer to reference the new ID from the cloned Type
			if binID, ok := elem.Value.(primitive.Binary); ok {
				oldID := blobToUUID(binID.Data)
				if newID, found := idMapping[oldID]; found {
					newElem.Value = idToBsonBinary(newID)
				} else {
					// Keep the original if not found in mapping
					newElem.Value = elem.Value
				}
			} else {
				newElem.Value = elem.Value
			}
		} else {
			// Clone the value
			switch v := elem.Value.(type) {
			case bson.D:
				newElem.Value = cloneObjectWithNewIDs(v, idMapping)
			case bson.A:
				newElem.Value = cloneObjectArrayWithNewIDs(v, idMapping)
			default:
				newElem.Value = v
			}
		}

		result = append(result, newElem)
	}

	return result
}

// cloneObjectArrayWithNewIDs clones an array within a WidgetObject.
func cloneObjectArrayWithNewIDs(arr bson.A, idMapping map[string]string) bson.A {
	result := make(bson.A, len(arr))
	for i, item := range arr {
		switch v := item.(type) {
		case bson.D:
			result[i] = cloneObjectWithNewIDs(v, idMapping)
		case bson.A:
			result[i] = cloneObjectArrayWithNewIDs(v, idMapping)
		default:
			result[i] = v
		}
	}
	return result
}
