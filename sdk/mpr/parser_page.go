// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"

	"go.mongodb.org/mongo-driver/bson"
)

func (r *Reader) parsePage(unitID, containerID string, contents []byte) (*pages.Page, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	page := &pages.Page{}
	page.ID = model.ID(unitID)
	page.TypeName = "Pages$Page"
	page.ContainerID = model.ID(containerID)

	if name, ok := raw["Name"].(string); ok {
		page.Name = name
	}
	if doc, ok := raw["Documentation"].(string); ok {
		page.Documentation = doc
	}
	// URL field is stored as "Url" (not "URL")
	if url, ok := raw["Url"].(string); ok {
		page.URL = url
	} else if url, ok := raw["URL"].(string); ok {
		// Fallback for legacy format
		page.URL = url
	}
	if layoutID, ok := raw["Layout"].(string); ok {
		page.LayoutID = model.ID(layoutID)
	}
	if markAsUsed, ok := raw["MarkAsUsed"].(bool); ok {
		page.MarkAsUsed = markAsUsed
	}
	if excluded, ok := raw["Excluded"].(bool); ok {
		page.Excluded = excluded
	}

	// Parse allowed module roles (BY_NAME references)
	allowedRoles := extractBsonArray(raw["AllowedModuleRoles"])
	for _, r := range allowedRoles {
		if name, ok := r.(string); ok {
			page.AllowedRoles = append(page.AllowedRoles, model.ID(name))
		}
	}

	// Parse title
	if title, ok := raw["Title"].(map[string]any); ok {
		page.Title = parseText(title)
	}

	// Parse parameters
	// Format: [3] for empty, [3, {param1}, {param2}...] for non-empty
	// Each parameter is a bson.D document directly in the array
	if params, ok := raw["Parameters"].(bson.A); ok {
		// Skip version marker (first element), iterate through rest
		for i := 1; i < len(params); i++ {
			// Primary format: direct bson.D document
			if paramDoc, ok := params[i].(bson.D); ok {
				paramMapInterface := make(map[string]any)
				for _, elem := range paramDoc {
					paramMapInterface[elem.Key] = elem.Value
				}
				param := parsePageParameter(paramMapInterface)
				page.Parameters = append(page.Parameters, param)
			} else if paramMap, ok := params[i].(map[string]any); ok {
				// Alternative: direct map
				param := parsePageParameter(paramMap)
				page.Parameters = append(page.Parameters, param)
			}
		}
	}

	return page, nil
}

func parseText(raw map[string]any) *model.Text {
	text := &model.Text{}

	text.ID = model.ID(extractBsonID(raw["$ID"]))

	text.Translations = make(map[string]string)

	// Handle Microflows$StringTemplate format (direct "Text" field)
	if textVal, ok := raw["Text"].(string); ok {
		text.Translations["en_US"] = textVal
		return text
	}

	// Try "Translations" format - could be a map or an array
	if translations, ok := raw["Translations"].(map[string]any); ok {
		for lang, val := range translations {
			if str, ok := val.(string); ok {
				text.Translations[lang] = str
			}
		}
	}

	// Also try "Translations" as an array of Translation objects (BSON format: [2, {$Type: "Texts$Translation", ...}])
	if transArray := extractBsonArray(raw["Translations"]); len(transArray) > 0 {
		for _, item := range transArray {
			if transMap, ok := item.(map[string]any); ok {
				langCode := extractString(transMap["LanguageCode"])
				textVal := extractString(transMap["Text"])
				if langCode != "" {
					text.Translations[langCode] = textVal
				}
			}
		}
	}

	// Also try "Items" format (array of Translation objects)
	if items := extractBsonArray(raw["Items"]); len(items) > 0 {
		for _, item := range items {
			if transMap, ok := item.(map[string]any); ok {
				langCode := extractString(transMap["LanguageCode"])
				textVal := extractString(transMap["Text"])
				if langCode != "" {
					text.Translations[langCode] = textVal
				}
			}
		}
	}

	return text
}

func parsePageParameter(raw map[string]any) *pages.PageParameter {
	param := &pages.PageParameter{}

	if id, ok := raw["$ID"].(string); ok {
		param.ID = model.ID(id)
	}
	if name, ok := raw["Name"].(string); ok {
		param.Name = name
	}
	if defaultValue, ok := raw["DefaultValue"].(string); ok {
		param.DefaultValue = defaultValue
	}
	if isRequired, ok := raw["IsRequired"].(bool); ok {
		param.IsRequired = isRequired
	}

	// Entity can be in two places:
	// 1. Old format: directly as "Entity" field (string ID)
	// 2. New format: nested in ParameterType[0].Entity (qualified name)
	if entityID, ok := raw["Entity"].(string); ok {
		param.EntityID = model.ID(entityID)
	}

	// Parse ParameterType to get entity name and/or primitive type
	// ParameterType can be a map/bson.D (single object) or array (with version marker)
	parseParamTypeDoc := func(doc bson.D) {
		for _, elem := range doc {
			switch elem.Key {
			case "$Type":
				if typeName, ok := elem.Value.(string); ok && typeName != "DataTypes$ObjectType" {
					param.TypeName = typeName
				}
			case "Entity":
				if entity, ok := elem.Value.(string); ok {
					param.EntityName = entity
				}
			}
		}
	}
	parseParamTypeMap := func(m map[string]any) {
		if typeName, ok := m["$Type"].(string); ok && typeName != "DataTypes$ObjectType" {
			param.TypeName = typeName
		}
		if entity, ok := m["Entity"].(string); ok {
			param.EntityName = entity
		}
	}

	if paramType, ok := raw["ParameterType"].(bson.D); ok {
		parseParamTypeDoc(paramType)
	} else if paramType, ok := raw["ParameterType"].(map[string]any); ok {
		parseParamTypeMap(paramType)
	} else if paramTypeArr, ok := raw["ParameterType"].(bson.A); ok {
		for _, item := range paramTypeArr {
			if typeDoc, ok := item.(bson.D); ok {
				parseParamTypeDoc(typeDoc)
			} else if typeMap, ok := item.(map[string]any); ok {
				parseParamTypeMap(typeMap)
			}
		}
	}

	return param
}

// parseLayout parses layout contents from BSON.
func (r *Reader) parseLayout(unitID, containerID string, contents []byte) (*pages.Layout, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	layout := &pages.Layout{}
	layout.ID = model.ID(unitID)
	layout.TypeName = "Pages$Layout"
	layout.ContainerID = model.ID(containerID)

	if name, ok := raw["Name"].(string); ok {
		layout.Name = name
	}
	if doc, ok := raw["Documentation"].(string); ok {
		layout.Documentation = doc
	}
	if layoutType, ok := raw["LayoutType"].(string); ok {
		layout.LayoutType = pages.LayoutType(layoutType)
	}

	return layout, nil
}

// parseEnumeration parses enumeration contents from BSON.
