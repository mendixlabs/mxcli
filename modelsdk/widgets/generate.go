// SPDX-License-Identifier: Apache-2.0

package widgets

import "github.com/JordtenBulte-OLC/mxcli/modelsdk/widgets/mpk"

// GenerateFromMPK builds a complete WidgetTemplate from a parsed MPK WidgetDefinition.
// All $IDs are placeholder IDs (aa000000... prefix). loader.go's collectIDs remaps them
// to real UUIDs before BSON serialisation — matching the lifecycle of embedded templates.
// System properties (Label, Visibility, Editability) are not added; Studio Pro injects them.
func GenerateFromMPK(def *mpk.WidgetDefinition) *WidgetTemplate {
	typeID := placeholderID()
	objTypeID := placeholderID()

	propTypes := []any{float64(2)} // Mendix array version marker
	objProps := []any{float64(2)}

	for _, p := range def.Properties {
		bsonType := xmlTypeToBSONType(p.Type)
		if bsonType == "" {
			continue // unknown XML type — skip silently
		}
		pt, prop := createPropertyPair(p, bsonType)
		if pt != nil {
			propTypes = append(propTypes, pt)
		}
		if prop != nil {
			objProps = append(objProps, prop)
		}
	}

	platform := def.SupportedPlatform
	if platform == "" {
		platform = "Web"
	}

	typeMap := map[string]any{
		"$ID":                      typeID,
		"$Type":                    "CustomWidgets$CustomWidgetType",
		"HelpUrl":                  def.HelpURL,
		"OfflineCapable":           def.OfflineCapable,
		"StudioCategory":           def.StudioCategory,
		"StudioProCategory":        def.StudioProCategory,
		"SupportedPlatform":        platform,
		"WidgetDescription":        def.Description,
		"WidgetId":                 def.ID,
		"WidgetName":               def.Name,
		"WidgetNeedsEntityContext": def.NeedsEntityContext,
		"WidgetPluginWidget":       def.IsPluggable,
		"ObjectType": map[string]any{
			"$ID":           objTypeID,
			"$Type":         "CustomWidgets$WidgetObjectType",
			"PropertyTypes": propTypes,
		},
	}

	objectMap := map[string]any{
		"$ID":         placeholderID(),
		"$Type":       "CustomWidgets$WidgetObject",
		"TypePointer": objTypeID,
		"Properties":  objProps,
	}

	return &WidgetTemplate{
		WidgetID:  def.ID,
		Name:      def.Name,
		Version:   def.Version,
		Generated: true,
		Type:      typeMap,
		Object:    objectMap,
	}
}
