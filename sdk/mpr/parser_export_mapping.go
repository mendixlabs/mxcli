// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"

	"go.mongodb.org/mongo-driver/bson"
)

// parseExportMapping parses an ExportMappings$ExportMapping unit from BSON.
func (r *Reader) parseExportMapping(unitID, containerID string, contents []byte) (*model.ExportMapping, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	em := &model.ExportMapping{}
	em.ID = model.ID(unitID)
	em.TypeName = "ExportMappings$ExportMapping"
	em.ContainerID = model.ID(containerID)

	if name, ok := raw["Name"].(string); ok {
		em.Name = name
	}
	if doc, ok := raw["Documentation"].(string); ok {
		em.Documentation = doc
	}
	if excluded, ok := raw["Excluded"].(bool); ok {
		em.Excluded = excluded
	}
	if exportLevel, ok := raw["ExportLevel"].(string); ok {
		em.ExportLevel = exportLevel
	}
	if v, ok := raw["JsonStructure"].(string); ok {
		em.JsonStructure = v
	}
	if v, ok := raw["XmlSchema"].(string); ok {
		em.XmlSchema = v
	}
	if v, ok := raw["MessageDefinition"].(string); ok {
		em.MessageDefinition = v
	}
	if v, ok := raw["NullValueOption"].(string); ok {
		em.NullValueOption = v
	}

	// Parse top-level mapping elements (array with int32 version prefix)
	if elements, ok := raw["Elements"].(bson.A); ok {
		for _, e := range elements {
			if elemMap, ok := e.(map[string]any); ok {
				elem := parseExportMappingElement(elemMap)
				if elem != nil {
					em.Elements = append(em.Elements, elem)
				}
			}
		}
	}

	return em, nil
}

// parseExportMappingElement dispatches to the correct parser based on $Type.
func parseExportMappingElement(raw map[string]any) *model.ExportMappingElement {
	typeName, _ := raw["$Type"].(string)
	switch typeName {
	case "ExportMappings$ObjectMappingElement":
		return parseExportObjectMappingElement(raw)
	case "ExportMappings$ValueMappingElement":
		return parseExportValueMappingElement(raw)
	default:
		return nil
	}
}

func parseExportObjectMappingElement(raw map[string]any) *model.ExportMappingElement {
	elem := &model.ExportMappingElement{Kind: "Object"}

	if id, ok := extractBsonIDString(raw["$ID"]); ok {
		elem.ID = model.ID(id)
	}
	elem.TypeName = "ExportMappings$ObjectMappingElement"

	if v, ok := raw["Entity"].(string); ok {
		elem.Entity = v
	}
	if v, ok := raw["ExposedName"].(string); ok {
		elem.ExposedName = v
	}
	if v, ok := raw["JsonPath"].(string); ok {
		elem.JsonPath = v
	}
	if v, ok := raw["Association"].(string); ok {
		elem.Association = v
	}

	// Parse children recursively (mix of object and value elements)
	if children, ok := raw["Children"].(bson.A); ok {
		for _, c := range children {
			if childMap, ok := c.(map[string]any); ok {
				child := parseExportMappingElement(childMap)
				if child != nil {
					elem.Children = append(elem.Children, child)
				}
			}
		}
	}

	return elem
}

func parseExportValueMappingElement(raw map[string]any) *model.ExportMappingElement {
	elem := &model.ExportMappingElement{Kind: "Value"}

	if id, ok := extractBsonIDString(raw["$ID"]); ok {
		elem.ID = model.ID(id)
	}
	elem.TypeName = "ExportMappings$ValueMappingElement"

	if v, ok := raw["Attribute"].(string); ok {
		elem.Attribute = v
	}
	if v, ok := raw["ExposedName"].(string); ok {
		elem.ExposedName = v
	}
	if v, ok := raw["JsonPath"].(string); ok {
		elem.JsonPath = v
	}

	// Extract the primitive type from the nested Type object
	if typeObj, ok := raw["Type"].(map[string]any); ok {
		elem.DataType = extractPrimitiveTypeName(typeObj)
	}

	return elem
}
