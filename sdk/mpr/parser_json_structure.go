// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"

	"go.mongodb.org/mongo-driver/bson"
)

// parseJsonStructure parses a JsonStructures$JsonStructure unit from BSON.
func (r *Reader) parseJsonStructure(unitID, containerID string, contents []byte) (*model.JsonStructure, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	js := &model.JsonStructure{}
	js.ID = model.ID(unitID)
	js.TypeName = "JsonStructures$JsonStructure"
	js.ContainerID = model.ID(containerID)

	if name, ok := raw["Name"].(string); ok {
		js.Name = name
	}
	if doc, ok := raw["Documentation"].(string); ok {
		js.Documentation = doc
	}
	if excluded, ok := raw["Excluded"].(bool); ok {
		js.Excluded = excluded
	}
	if exportLevel, ok := raw["ExportLevel"].(string); ok {
		js.ExportLevel = exportLevel
	}
	if snippet, ok := raw["JsonSnippet"].(string); ok {
		js.JsonSnippet = snippet
	}

	// Parse elements array (may start with int32 version prefix)
	if elements, ok := raw["Elements"].(bson.A); ok {
		for _, e := range elements {
			if elemMap, ok := e.(map[string]any); ok {
				elem := parseJsonElement(elemMap)
				js.Elements = append(js.Elements, elem)
			}
		}
	}

	return js, nil
}

// parseJsonElement recursively parses a JsonStructures$JsonElement from a BSON map.
func parseJsonElement(raw map[string]any) *model.JsonElement {
	elem := &model.JsonElement{}

	if id, ok := extractBsonIDString(raw["$ID"]); ok {
		elem.ID = model.ID(id)
	}
	elem.TypeName = "JsonStructures$JsonElement"

	if v, ok := raw["ExposedName"].(string); ok {
		elem.ExposedName = v
	}
	if v, ok := raw["ExposedItemName"].(string); ok {
		elem.ExposedItemName = v
	}
	if v, ok := raw["ElementType"].(string); ok {
		elem.ElementType = v
	}
	if v, ok := raw["PrimitiveType"].(string); ok {
		elem.PrimitiveType = v
	}
	if v, ok := raw["Path"].(string); ok {
		elem.Path = v
	}
	if v, ok := raw["MinOccurs"].(int32); ok {
		elem.MinOccurs = int(v)
	}
	if v, ok := raw["MaxOccurs"].(int32); ok {
		elem.MaxOccurs = int(v)
	}
	if v, ok := raw["Nillable"].(bool); ok {
		elem.Nillable = v
	}
	if v, ok := raw["IsDefaultType"].(bool); ok {
		elem.IsDefaultType = v
	}

	// Parse children recursively (may start with int32 version prefix)
	if children, ok := raw["Children"].(bson.A); ok {
		for _, c := range children {
			if childMap, ok := c.(map[string]any); ok {
				child := parseJsonElement(childMap)
				elem.Children = append(elem.Children, child)
			}
		}
	}

	return elem
}

// extractBsonIDString extracts a UUID string from a BSON binary ID field.
func extractBsonIDString(val any) (string, bool) {
	if val == nil {
		return "", false
	}
	switch v := val.(type) {
	case string:
		return v, true
	default:
		// Try to extract from primitive.Binary or similar
		s := extractBsonID(val)
		if s != "" {
			return s, true
		}
		return "", false
	}
}
