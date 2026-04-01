// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"

	"go.mongodb.org/mongo-driver/bson"
)

// parseImportMapping parses an ImportMappings$ImportMapping unit from BSON.
func (r *Reader) parseImportMapping(unitID, containerID string, contents []byte) (*model.ImportMapping, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	im := &model.ImportMapping{}
	im.ID = model.ID(unitID)
	im.TypeName = "ImportMappings$ImportMapping"
	im.ContainerID = model.ID(containerID)

	if name, ok := raw["Name"].(string); ok {
		im.Name = name
	}
	if doc, ok := raw["Documentation"].(string); ok {
		im.Documentation = doc
	}
	if excluded, ok := raw["Excluded"].(bool); ok {
		im.Excluded = excluded
	}
	if exportLevel, ok := raw["ExportLevel"].(string); ok {
		im.ExportLevel = exportLevel
	}
	if v, ok := raw["JsonStructure"].(string); ok {
		im.JsonStructure = v
	}
	if v, ok := raw["XmlSchema"].(string); ok {
		im.XmlSchema = v
	}
	if v, ok := raw["MessageDefinition"].(string); ok {
		im.MessageDefinition = v
	}

	// Parse top-level mapping elements (may start with int32 version prefix)
	if elements, ok := raw["Elements"].(bson.A); ok {
		for _, e := range elements {
			if elemMap, ok := e.(map[string]any); ok {
				elem := parseImportMappingElement(elemMap)
				if elem != nil {
					im.Elements = append(im.Elements, elem)
				}
			}
		}
	}

	return im, nil
}

// parseImportMappingElement dispatches to the correct parser based on $Type.
func parseImportMappingElement(raw map[string]any) *model.ImportMappingElement {
	typeName, _ := raw["$Type"].(string)
	switch typeName {
	case "ImportMappings$ObjectMappingElement":
		return parseImportObjectMappingElement(raw)
	case "ImportMappings$ValueMappingElement":
		return parseImportValueMappingElement(raw)
	default:
		return nil
	}
}

func parseImportObjectMappingElement(raw map[string]any) *model.ImportMappingElement {
	elem := &model.ImportMappingElement{Kind: "Object"}

	if id, ok := extractBsonIDString(raw["$ID"]); ok {
		elem.ID = model.ID(id)
	}
	elem.TypeName = "ImportMappings$ObjectMappingElement"

	if v, ok := raw["Entity"].(string); ok {
		elem.Entity = v
	}
	if v, ok := raw["ExposedName"].(string); ok {
		elem.ExposedName = v
	}
	if v, ok := raw["JsonPath"].(string); ok {
		elem.JsonPath = v
	}
	if v, ok := raw["ObjectHandling"].(string); ok {
		elem.ObjectHandling = v
	}
	if v, ok := raw["Association"].(string); ok {
		elem.Association = v
	}

	// Parse children recursively (mix of object and value elements)
	if children, ok := raw["Children"].(bson.A); ok {
		for _, c := range children {
			if childMap, ok := c.(map[string]any); ok {
				child := parseImportMappingElement(childMap)
				if child != nil {
					elem.Children = append(elem.Children, child)
				}
			}
		}
	}

	return elem
}

func parseImportValueMappingElement(raw map[string]any) *model.ImportMappingElement {
	elem := &model.ImportMappingElement{Kind: "Value"}

	if id, ok := extractBsonIDString(raw["$ID"]); ok {
		elem.ID = model.ID(id)
	}
	elem.TypeName = "ImportMappings$ValueMappingElement"

	if v, ok := raw["Attribute"].(string); ok {
		elem.Attribute = v
	}
	if v, ok := raw["ExposedName"].(string); ok {
		elem.ExposedName = v
	}
	if v, ok := raw["JsonPath"].(string); ok {
		elem.JsonPath = v
	}
	if v, ok := raw["IsKey"].(bool); ok {
		elem.IsKey = v
	}

	// Extract the primitive type from the nested Type object
	if typeObj, ok := raw["Type"].(map[string]any); ok {
		elem.DataType = extractPrimitiveTypeName(typeObj)
	}

	return elem
}

// extractPrimitiveTypeName converts a DataTypes$* BSON type object to a simple type string.
func extractPrimitiveTypeName(typeObj map[string]any) string {
	typeName, _ := typeObj["$Type"].(string)
	switch typeName {
	case "DataTypes$StringType":
		return "String"
	case "DataTypes$IntegerType":
		return "Integer"
	case "DataTypes$LongType":
		return "Long"
	case "DataTypes$DecimalType":
		return "Decimal"
	case "DataTypes$BooleanType":
		return "Boolean"
	case "DataTypes$DateTimeType":
		return "DateTime"
	case "DataTypes$BinaryType":
		return "Binary"
	default:
		return "String"
	}
}
