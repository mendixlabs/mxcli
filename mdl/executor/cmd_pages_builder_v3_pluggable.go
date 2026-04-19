// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
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
			result = append(result, bson.E{Key: "$ID", Value: bsonutil.NewIDBsonBinary()})
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
			result = append(result, bson.E{Key: "$ID", Value: bsonutil.NewIDBsonBinary()})
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
			result = append(result, bson.E{Key: "$ID", Value: bsonutil.NewIDBsonBinary()})
		} else {
			result = append(result, elem)
		}
	}

	return result
}

// buildWidgetV3ToBSON builds a V3 widget and serializes it to an opaque storage form.
func (pb *pageBuilder) buildWidgetV3ToBSON(w *ast.WidgetV3) (bson.D, error) {
	widget, err := pb.buildWidgetV3(w)
	if err != nil {
		return nil, err
	}
	if widget == nil {
		return nil, nil
	}
	raw := pb.widgetBackend.SerializeWidgetToOpaque(widget)
	if raw == nil {
		return nil, nil
	}
	bsonD, ok := raw.(bson.D)
	if !ok {
		return nil, mdlerrors.NewValidationf("SerializeWidgetToOpaque returned unexpected type %T", raw)
	}
	return bsonD, nil
}

// createAttributeObject creates a single attribute object entry for filter widget Attributes.
// Used by the widget engine's opAttributeObjects operation.
// The structure follows CustomWidgets$WidgetObject with a nested WidgetProperty for "attribute".
// TypePointers reference the Type's PropertyType IDs (not regenerated).
func (pb *pageBuilder) createAttributeObject(attributePath string, objectTypeID, propertyTypeID, valueTypeID string) (bson.D, error) {
	if strings.Count(attributePath, ".") < 2 {
		return nil, mdlerrors.NewValidationf("invalid attribute path %q: expected Module.Entity.Attribute format", attributePath)
	}
	return bson.D{
		{Key: "$ID", Value: hexToBytes(types.GenerateID())},
		{Key: "$Type", Value: "CustomWidgets$WidgetObject"},
		{Key: "Properties", Value: []any{
			int32(2),
			bson.D{
				{Key: "$ID", Value: hexToBytes(types.GenerateID())},
				{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
				{Key: "TypePointer", Value: hexToBytes(propertyTypeID)},
				{Key: "Value", Value: bson.D{
					{Key: "$ID", Value: hexToBytes(types.GenerateID())},
					{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
					{Key: "Action", Value: bson.D{
						{Key: "$ID", Value: hexToBytes(types.GenerateID())},
						{Key: "$Type", Value: "Forms$NoAction"},
						{Key: "DisabledDuringExecution", Value: true},
					}},
					{Key: "AttributeRef", Value: bson.D{
						{Key: "$ID", Value: hexToBytes(types.GenerateID())},
						{Key: "$Type", Value: "DomainModels$AttributeRef"},
						{Key: "Attribute", Value: attributePath},
						{Key: "EntityRef", Value: nil},
					}},
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
	}, nil
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
