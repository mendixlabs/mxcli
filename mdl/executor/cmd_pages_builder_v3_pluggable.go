// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/widgets"
)

// =============================================================================
// Custom/Pluggable Widget Builders V3
// =============================================================================

// buildGallerySelectionProperty clones an itemSelection property and updates the Selection value.
func (pb *pageBuilder) buildGallerySelectionProperty(propMap bson.D, selectionMode string) bson.D {
	result := make(bson.D, 0, len(propMap))

	for _, elem := range propMap {
		if elem.Key == "$ID" {
			// Generate new ID
			result = append(result, bson.E{Key: "$ID", Value: mpr.IDToBsonBinary(mpr.GenerateID())})
		} else if elem.Key == "Value" {
			// Clone Value and update Selection
			if valueMap, ok := elem.Value.(bson.D); ok {
				result = append(result, bson.E{Key: "Value", Value: pb.cloneGallerySelectionValue(valueMap, selectionMode)})
			} else {
				result = append(result, elem)
			}
		} else {
			result = append(result, elem)
		}
	}

	return result
}

// cloneGallerySelectionValue clones a WidgetValue and updates the Selection field.
func (pb *pageBuilder) cloneGallerySelectionValue(valueMap bson.D, selectionMode string) bson.D {
	result := make(bson.D, 0, len(valueMap))

	for _, elem := range valueMap {
		if elem.Key == "$ID" {
			result = append(result, bson.E{Key: "$ID", Value: mpr.IDToBsonBinary(mpr.GenerateID())})
		} else if elem.Key == "Selection" {
			// Update selection mode
			result = append(result, bson.E{Key: "Selection", Value: selectionMode})
		} else if elem.Key == "Action" {
			// Clone action with new ID
			if actionMap, ok := elem.Value.(bson.D); ok {
				result = append(result, bson.E{Key: "Action", Value: pb.cloneActionWithNewID(actionMap)})
			} else {
				result = append(result, elem)
			}
		} else {
			result = append(result, elem)
		}
	}

	return result
}

// cloneActionWithNewID clones an action (e.g., NoAction) with a new ID.
func (pb *pageBuilder) cloneActionWithNewID(actionMap bson.D) bson.D {
	result := make(bson.D, 0, len(actionMap))

	for _, elem := range actionMap {
		if elem.Key == "$ID" {
			result = append(result, bson.E{Key: "$ID", Value: mpr.IDToBsonBinary(mpr.GenerateID())})
		} else {
			result = append(result, elem)
		}
	}

	return result
}

// buildWidgetV3ToBSON builds a V3 widget and serializes it directly to BSON.
func (pb *pageBuilder) buildWidgetV3ToBSON(w *ast.WidgetV3) (bson.D, error) {
	widget, err := pb.buildWidgetV3(w)
	if err != nil {
		return nil, err
	}
	if widget == nil {
		return nil, nil
	}
	return mpr.SerializeWidget(widget), nil
}

// buildTextFilterV3 creates a DataGrid Text Filter pluggable widget.
func (pb *pageBuilder) buildTextFilterV3(w *ast.WidgetV3) (*pages.CustomWidget, error) {
	widgetID := model.ID(mpr.GenerateID())

	// Load embedded template with both Type and Object
	embeddedType, embeddedObject, embeddedIDs, embeddedObjectTypeID, err := widgets.GetTemplateFullBSON(pages.WidgetIDDataGridTextFilter, mpr.GenerateID, pb.reader.Path())
	if err != nil {
		return nil, fmt.Errorf("failed to load TextFilter template: %w", err)
	}
	if embeddedType == nil || embeddedObject == nil {
		return nil, fmt.Errorf("TextFilter template not found or incomplete")
	}

	// Apply Attributes and FilterType properties from AST
	attributes := w.GetAttributes()
	filterType := w.GetFilterType()
	if len(attributes) > 0 || filterType != "" {
		embeddedObject, err = pb.applyFilterWidgetProperties(embeddedObject, embeddedType, attributes, filterType)
		if err != nil {
			return nil, fmt.Errorf("failed to apply filter properties: %w", err)
		}
	}

	// Convert widget IDs to pages.PropertyTypeIDEntry format
	propertyTypeIDs := convertPropertyTypeIDs(embeddedIDs)

	// Create the widget with both Type and Object
	cw := &pages.CustomWidget{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       widgetID,
				TypeName: "CustomWidgets$CustomWidget",
			},
			Name: w.Name,
		},
		Editable:          "Always",
		RawType:           embeddedType,
		RawObject:         embeddedObject,
		PropertyTypeIDMap: propertyTypeIDs,
		ObjectTypeID:      embeddedObjectTypeID,
	}

	if err := pb.registerWidgetName(w.Name, cw.ID); err != nil {
		return nil, err
	}

	return cw, nil
}

// buildNumberFilterV3 creates a DataGrid Number Filter pluggable widget.
func (pb *pageBuilder) buildNumberFilterV3(w *ast.WidgetV3) (*pages.CustomWidget, error) {
	widgetID := model.ID(mpr.GenerateID())

	// Load embedded template with both Type and Object
	embeddedType, embeddedObject, embeddedIDs, embeddedObjectTypeID, err := widgets.GetTemplateFullBSON(pages.WidgetIDDataGridNumberFilter, mpr.GenerateID, pb.reader.Path())
	if err != nil {
		return nil, fmt.Errorf("failed to load NumberFilter template: %w", err)
	}
	if embeddedType == nil || embeddedObject == nil {
		return nil, fmt.Errorf("NumberFilter template not found or incomplete")
	}

	// Apply Attributes and FilterType properties from AST
	attributes := w.GetAttributes()
	filterType := w.GetFilterType()
	if len(attributes) > 0 || filterType != "" {
		embeddedObject, err = pb.applyFilterWidgetProperties(embeddedObject, embeddedType, attributes, filterType)
		if err != nil {
			return nil, fmt.Errorf("failed to apply filter properties: %w", err)
		}
	}

	// Convert widget IDs to pages.PropertyTypeIDEntry format
	propertyTypeIDs := convertPropertyTypeIDs(embeddedIDs)

	// Create the widget with both Type and Object
	cw := &pages.CustomWidget{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       widgetID,
				TypeName: "CustomWidgets$CustomWidget",
			},
			Name: w.Name,
		},
		Editable:          "Always",
		RawType:           embeddedType,
		RawObject:         embeddedObject,
		PropertyTypeIDMap: propertyTypeIDs,
		ObjectTypeID:      embeddedObjectTypeID,
	}

	if err := pb.registerWidgetName(w.Name, cw.ID); err != nil {
		return nil, err
	}

	return cw, nil
}

// buildDropdownFilterV3 creates a Dropdown Filter pluggable widget.
func (pb *pageBuilder) buildDropdownFilterV3(w *ast.WidgetV3) (*pages.CustomWidget, error) {
	widgetID := model.ID(mpr.GenerateID())

	// Load embedded template with both Type and Object
	embeddedType, embeddedObject, embeddedIDs, embeddedObjectTypeID, err := widgets.GetTemplateFullBSON(pages.WidgetIDDataGridDropdownFilter, mpr.GenerateID, pb.reader.Path())
	if err != nil {
		return nil, fmt.Errorf("failed to load DropdownFilter template: %w", err)
	}
	if embeddedType == nil || embeddedObject == nil {
		return nil, fmt.Errorf("DropdownFilter template not found or incomplete")
	}

	// Apply Attributes and FilterType properties from AST
	attributes := w.GetAttributes()
	filterType := w.GetFilterType()
	if len(attributes) > 0 || filterType != "" {
		embeddedObject, err = pb.applyFilterWidgetProperties(embeddedObject, embeddedType, attributes, filterType)
		if err != nil {
			return nil, fmt.Errorf("failed to apply filter properties: %w", err)
		}
	}

	// Convert widget IDs to pages.PropertyTypeIDEntry format
	propertyTypeIDs := convertPropertyTypeIDs(embeddedIDs)

	// Create the widget with both Type and Object
	cw := &pages.CustomWidget{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       widgetID,
				TypeName: "CustomWidgets$CustomWidget",
			},
			Name: w.Name,
		},
		Editable:          "Always",
		RawType:           embeddedType,
		RawObject:         embeddedObject,
		PropertyTypeIDMap: propertyTypeIDs,
		ObjectTypeID:      embeddedObjectTypeID,
	}

	if err := pb.registerWidgetName(w.Name, cw.ID); err != nil {
		return nil, err
	}

	return cw, nil
}

// buildDateFilterV3 creates a Date Filter pluggable widget.
func (pb *pageBuilder) buildDateFilterV3(w *ast.WidgetV3) (*pages.CustomWidget, error) {
	widgetID := model.ID(mpr.GenerateID())

	// Load embedded template with both Type and Object
	embeddedType, embeddedObject, embeddedIDs, embeddedObjectTypeID, err := widgets.GetTemplateFullBSON(pages.WidgetIDDataGridDateFilter, mpr.GenerateID, pb.reader.Path())
	if err != nil {
		return nil, fmt.Errorf("failed to load DateFilter template: %w", err)
	}
	if embeddedType == nil || embeddedObject == nil {
		return nil, fmt.Errorf("DateFilter template not found or incomplete")
	}

	// Apply Attributes and FilterType properties from AST
	attributes := w.GetAttributes()
	filterType := w.GetFilterType()
	if len(attributes) > 0 || filterType != "" {
		embeddedObject, err = pb.applyFilterWidgetProperties(embeddedObject, embeddedType, attributes, filterType)
		if err != nil {
			return nil, fmt.Errorf("failed to apply filter properties: %w", err)
		}
	}

	// Convert widget IDs to pages.PropertyTypeIDEntry format
	propertyTypeIDs := convertPropertyTypeIDs(embeddedIDs)

	// Create the widget with both Type and Object
	cw := &pages.CustomWidget{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       widgetID,
				TypeName: "CustomWidgets$CustomWidget",
			},
			Name: w.Name,
		},
		Editable:          "Always",
		RawType:           embeddedType,
		RawObject:         embeddedObject,
		PropertyTypeIDMap: propertyTypeIDs,
		ObjectTypeID:      embeddedObjectTypeID,
	}

	if err := pb.registerWidgetName(w.Name, cw.ID); err != nil {
		return nil, err
	}

	return cw, nil
}

// =============================================================================
// Filter Widget Property Helpers
// =============================================================================

// applyFilterWidgetProperties applies Attributes and FilterType to a filter widget's RawObject.
// This works for TEXTFILTER, NUMBERFILTER, DROPDOWNFILTER, and DATEFILTER widgets.
func (pb *pageBuilder) applyFilterWidgetProperties(rawObject bson.D, rawType bson.D, attributes []string, filterType string) (bson.D, error) {
	if len(attributes) == 0 && filterType == "" {
		return rawObject, nil
	}

	// Build property key map from RawType.ObjectType.PropertyTypes
	propKeyMap := make(map[string]string) // TypePointer ID -> PropertyKey
	objType := getBsonField(rawType, "ObjectType")
	if objType != nil {
		propTypes := getBsonArray(objType, "PropertyTypes")
		for _, pt := range propTypes {
			ptMap, ok := pt.(bson.D)
			if !ok {
				continue
			}
			id := getBsonBinaryID(ptMap, "$ID")
			key := getBsonString(ptMap, "PropertyKey")
			if id != "" && key != "" {
				propKeyMap[id] = key
			}
		}
	}

	// Reverse map: PropertyKey -> TypePointer ID
	keyToIDMap := make(map[string]string)
	for id, key := range propKeyMap {
		keyToIDMap[key] = id
	}

	// Get the ObjectType for the "attributes" property (for nested objects)
	var attributeObjectTypeID string
	var attributePropertyTypeID string
	var attributeValueTypeID string
	if attrTypePointerID, ok := keyToIDMap["attributes"]; ok {
		// Find the PropertyType for "attributes" in ObjectType.PropertyTypes
		for _, pt := range getBsonArray(objType, "PropertyTypes") {
			ptMap, ok := pt.(bson.D)
			if !ok {
				continue
			}
			if getBsonBinaryID(ptMap, "$ID") == attrTypePointerID {
				// Get the ObjectType inside ValueType
				valueType := getBsonField(ptMap, "ValueType")
				if valueType != nil {
					innerObjType := getBsonField(valueType, "ObjectType")
					if innerObjType != nil {
						attributeObjectTypeID = getBsonBinaryID(innerObjType, "$ID")
						// Get the PropertyType for "attribute" inside
						for _, innerPt := range getBsonArray(innerObjType, "PropertyTypes") {
							innerPtMap, ok := innerPt.(bson.D)
							if !ok {
								continue
							}
							if getBsonString(innerPtMap, "PropertyKey") == "attribute" {
								attributePropertyTypeID = getBsonBinaryID(innerPtMap, "$ID")
								innerValueType := getBsonField(innerPtMap, "ValueType")
								if innerValueType != nil {
									attributeValueTypeID = getBsonBinaryID(innerValueType, "$ID")
								}
							}
						}
					}
				}
			}
		}
	}

	// Validate that we have all required IDs for attribute objects
	canCreateAttributes := len(attributes) > 0 &&
		attributeObjectTypeID != "" &&
		attributePropertyTypeID != "" &&
		attributeValueTypeID != ""

	// Modify Properties array in rawObject
	propsArray := getBsonArray(rawObject, "Properties")
	newPropsArray := make([]any, 0, len(propsArray))

	for _, prop := range propsArray {
		propMap, ok := prop.(bson.D)
		if !ok {
			newPropsArray = append(newPropsArray, prop)
			continue
		}

		typePointer := getBsonBinaryID(propMap, "TypePointer")
		propKey := propKeyMap[typePointer]

		switch propKey {
		case "attrChoice":
			// Set to "linked" (custom) if attributes are specified and we can create them
			if canCreateAttributes {
				propMap = setBsonPrimitiveValue(propMap, "linked")
			}
		case "attributes":
			// Add attribute objects only if we have all required IDs
			if canCreateAttributes {
				propMap = pb.buildAttributeObjects(propMap, attributes, attributeObjectTypeID, attributePropertyTypeID, attributeValueTypeID)
			}
		case "defaultFilter":
			// Set filter type
			if filterType != "" {
				propMap = setBsonPrimitiveValue(propMap, filterType)
			}
		}

		newPropsArray = append(newPropsArray, propMap)
	}

	// Update Properties in rawObject
	return setBsonArrayField(rawObject, "Properties", newPropsArray), nil
}

// buildAttributeObjects creates the Objects array for the "attributes" property.
func (pb *pageBuilder) buildAttributeObjects(propMap bson.D, attributes []string, objectTypeID, propertyTypeID, valueTypeID string) bson.D {
	// Get existing Value field
	value := getBsonField(propMap, "Value")
	if value == nil {
		return propMap
	}

	// Create Objects array with attribute entries
	objects := make([]any, 0, len(attributes)+1)
	objects = append(objects, int32(2)) // BSON array prefix

	for _, attr := range attributes {
		// Resolve short attribute names using entity context (e.g., "Name" → "Module.Entity.Name")
		resolvedAttr := pb.resolveAttributePath(attr)
		attrObj := pb.createAttributeObject(resolvedAttr, objectTypeID, propertyTypeID, valueTypeID)
		objects = append(objects, attrObj)
	}

	// Update Value.Objects
	value = setBsonArrayField(value, "Objects", objects)
	return setBsonField(propMap, "Value", value)
}

// createAttributeObject creates a single attribute object entry for filter widget Attributes.
// The structure follows CustomWidgets$WidgetObject with a nested WidgetProperty for "attribute".
// TypePointers reference the Type's PropertyType IDs (not regenerated).
func (pb *pageBuilder) createAttributeObject(attributePath string, objectTypeID, propertyTypeID, valueTypeID string) bson.D {
	return bson.D{
		{Key: "$ID", Value: hexToBytes(mpr.GenerateID())},
		{Key: "$Type", Value: "CustomWidgets$WidgetObject"},
		{Key: "Properties", Value: []any{
			int32(2),
			bson.D{
				{Key: "$ID", Value: hexToBytes(mpr.GenerateID())},
				{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
				{Key: "TypePointer", Value: hexToBytes(propertyTypeID)},
				{Key: "Value", Value: bson.D{
					{Key: "$ID", Value: hexToBytes(mpr.GenerateID())},
					{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
					{Key: "Action", Value: bson.D{
						{Key: "$ID", Value: hexToBytes(mpr.GenerateID())},
						{Key: "$Type", Value: "Forms$NoAction"},
						{Key: "DisabledDuringExecution", Value: true},
					}},
					{Key: "AttributeRef", Value: func() any {
						if strings.Count(attributePath, ".") < 2 {
							return nil
						}
						return bson.D{
							{Key: "$ID", Value: hexToBytes(mpr.GenerateID())},
							{Key: "$Type", Value: "DomainModels$AttributeRef"},
							{Key: "Attribute", Value: attributePath},
							{Key: "EntityRef", Value: nil},
						}
					}()},
					{Key: "DataSource", Value: nil},
					{Key: "EntityRef", Value: nil},
					{Key: "Expression", Value: ""},
					{Key: "Form", Value: ""},
					{Key: "Icon", Value: nil},
					{Key: "Image", Value: ""},
					{Key: "Microflow", Value: ""},
					{Key: "Nanoflow", Value: ""},
					{Key: "Objects", Value: []any{int32(2)}},
					{Key: "PrimitiveValue", Value: ""},
					{Key: "Selection", Value: "None"},
					{Key: "SourceVariable", Value: nil},
					{Key: "TextTemplate", Value: nil},
					{Key: "TranslatableValue", Value: nil},
					{Key: "TypePointer", Value: hexToBytes(valueTypeID)},
					{Key: "Widgets", Value: []any{int32(2)}},
					{Key: "XPathConstraint", Value: ""},
				}},
			},
		}},
		{Key: "TypePointer", Value: hexToBytes(objectTypeID)},
	}
}

// BSON helper functions for filter properties

func getBsonField(d bson.D, key string) bson.D {
	for _, elem := range d {
		if elem.Key == key {
			if nested, ok := elem.Value.(bson.D); ok {
				return nested
			}
		}
	}
	return nil
}

func getBsonArray(d bson.D, key string) []any {
	for _, elem := range d {
		if elem.Key == key {
			switch v := elem.Value.(type) {
			case []any:
				return v
			case bson.A:
				return []any(v)
			}
		}
	}
	return nil
}

func getBsonString(d bson.D, key string) string {
	for _, elem := range d {
		if elem.Key == key {
			if s, ok := elem.Value.(string); ok {
				return s
			}
		}
	}
	return ""
}

func getBsonBinaryID(d bson.D, key string) string {
	for _, elem := range d {
		if elem.Key == key {
			if b, ok := elem.Value.([]byte); ok {
				return bytesToHex(b)
			}
		}
	}
	return ""
}

func setBsonPrimitiveValue(propMap bson.D, value string) bson.D {
	for i, elem := range propMap {
		if elem.Key == "Value" {
			if valueMap, ok := elem.Value.(bson.D); ok {
				for j, vElem := range valueMap {
					if vElem.Key == "PrimitiveValue" {
						valueMap[j].Value = value
						break
					}
				}
				propMap[i].Value = valueMap
			}
			break
		}
	}
	return propMap
}

func setBsonArrayField(d bson.D, key string, value []any) bson.D {
	for i, elem := range d {
		if elem.Key == key {
			d[i].Value = value
			return d
		}
	}
	return d
}

func setBsonField(d bson.D, key string, value bson.D) bson.D {
	for i, elem := range d {
		if elem.Key == key {
			d[i].Value = value
			return d
		}
	}
	return d
}

// hexToBytes converts a hex string (with or without dashes) to a 16-byte blob
// in Microsoft GUID format (little-endian for first 3 segments).
// This matches the format used by Mendix and uuidToBlob in sdk/mpr/writer_core.go.
func hexToBytes(hexStr string) []byte {
	// Remove dashes if present (UUID format)
	clean := strings.ReplaceAll(hexStr, "-", "")
	if len(clean) != 32 {
		return nil
	}

	// Decode hex to bytes
	decoded := make([]byte, 16)
	for i := range 16 {
		decoded[i] = hexByte(clean[i*2])<<4 | hexByte(clean[i*2+1])
	}

	// Swap bytes to Microsoft GUID format (little-endian for first 3 segments)
	blob := make([]byte, 16)
	// First 4 bytes: reversed
	blob[0] = decoded[3]
	blob[1] = decoded[2]
	blob[2] = decoded[1]
	blob[3] = decoded[0]
	// Next 2 bytes: reversed
	blob[4] = decoded[5]
	blob[5] = decoded[4]
	// Next 2 bytes: reversed
	blob[6] = decoded[7]
	blob[7] = decoded[6]
	// Last 8 bytes: unchanged
	copy(blob[8:], decoded[8:])

	return blob
}

func hexByte(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

// bytesToHex converts a 16-byte blob from Microsoft GUID format to a hex string.
// This reverses the byte swapping done by hexToBytes, matching blobToUUID in sdk/mpr/reader.go.
func bytesToHex(b []byte) string {
	if len(b) != 16 {
		// Fallback for non-standard lengths
		if len(b) > 1024 {
			return "" // reject unreasonably large inputs
		}
		const hexChars = "0123456789abcdef"
		result := make([]byte, len(b)*2)
		for i, v := range b {
			result[i*2] = hexChars[v>>4]
			result[i*2+1] = hexChars[v&0x0f]
		}
		return string(result)
	}

	// Reverse Microsoft GUID byte swapping to get canonical hex
	return fmt.Sprintf("%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x",
		b[3], b[2], b[1], b[0], // First 4 bytes: reversed
		b[5], b[4], // Next 2 bytes: reversed
		b[7], b[6], // Next 2 bytes: reversed
		b[8], b[9], b[10], b[11], b[12], b[13], b[14], b[15]) // Last 8 bytes: unchanged
}
