// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ImageSourceRule checks for IMAGE widgets with no source configured.
// Unconfigured image widgets crash the runtime with "Did not expect an argument to be undefined".
type ImageSourceRule struct{}

// NewImageSourceRule creates a new image source rule.
func NewImageSourceRule() *ImageSourceRule {
	return &ImageSourceRule{}
}

func (r *ImageSourceRule) ID() string                       { return "MPR005" }
func (r *ImageSourceRule) Name() string                     { return "UnconfiguredImage" }
func (r *ImageSourceRule) Category() string                 { return "correctness" }
func (r *ImageSourceRule) DefaultSeverity() linter.Severity { return linter.SeverityWarning }

func (r *ImageSourceRule) Description() string {
	return "Checks for IMAGE widgets with no source configured (crashes at runtime)"
}

// imageWidgetInfo holds information about a found image widget.
type imageWidgetInfo struct {
	Name       string
	WidgetType string
	Configured bool
}

// Check iterates over catalog widgets, loads raw BSON for containers with image widgets,
// and checks whether each image widget has a source configured.
func (r *ImageSourceRule) Check(ctx *linter.LintContext) []linter.Violation {
	reader := ctx.Reader()
	if reader == nil {
		return nil
	}

	// Collect containers that have image widgets
	type containerInfo struct {
		ID            string
		QualifiedName string
		Type          string // "PAGE" or "SNIPPET"
		ModuleName    string
	}
	containers := make(map[string]containerInfo)

	for w := range ctx.Widgets() {
		if ctx.IsExcluded(w.ModuleName) {
			continue
		}
		if w.WidgetType != "Forms$StaticImageViewer" && w.WidgetType != "Forms$ImageViewer" {
			continue
		}
		if _, ok := containers[w.ContainerID]; !ok {
			containers[w.ContainerID] = containerInfo{
				ID:            w.ContainerID,
				QualifiedName: w.ContainerQualifiedName,
				Type:          w.ContainerType,
				ModuleName:    w.ModuleName,
			}
		}
	}

	var violations []linter.Violation

	for _, c := range containers {
		rawData, err := reader.GetRawUnit(model.ID(c.ID))
		if err != nil || rawData == nil {
			continue
		}

		widgets := findImageWidgets(rawData)
		for _, w := range widgets {
			if w.Configured {
				continue
			}

			docType := "page"
			if c.Type == "SNIPPET" {
				docType = "snippet"
			}

			violations = append(violations, linter.Violation{
				RuleID:   r.ID(),
				Severity: r.DefaultSeverity(),
				Message: fmt.Sprintf("IMAGE '%s' in %s has no source configured and will crash at runtime",
					w.Name, c.QualifiedName),
				Location: linter.Location{
					Module:       c.ModuleName,
					DocumentType: docType,
					DocumentName: docNameFromQualified(c.QualifiedName),
					DocumentID:   c.ID,
				},
				Suggestion: "Use STATICIMAGE with an image resource, DYNAMICIMAGE with a data source, or remove the widget",
			})
		}
	}

	return violations
}

// docNameFromQualified extracts the document name from a qualified name like "Module.Name".
func docNameFromQualified(qualifiedName string) string {
	for i := len(qualifiedName) - 1; i >= 0; i-- {
		if qualifiedName[i] == '.' {
			return qualifiedName[i+1:]
		}
	}
	return qualifiedName
}

// IsUnconfiguredImage checks whether a widget BSON map represents an unconfigured image widget.
// Returns true if the widget is a static image with no Image field, or a dynamic image
// with no DataSource.EntityRef.
func IsUnconfiguredImage(widgetType string, w map[string]any) bool {
	switch widgetType {
	case "Forms$StaticImageViewer":
		return w["Image"] == nil
	case "Forms$ImageViewer":
		ds, ok := w["DataSource"].(map[string]any)
		if !ok || ds == nil {
			return true
		}
		return ds["EntityRef"] == nil
	default:
		return false
	}
}

// findImageWidgets walks a raw BSON page/snippet document and returns all image widgets found.
func findImageWidgets(rawData map[string]any) []imageWidgetInfo {
	// Pages have FormCall → Arguments → Widgets
	if formCall, ok := rawData["FormCall"].(map[string]any); ok {
		args := getBsonArray(formCall["Arguments"])
		var result []imageWidgetInfo
		for _, arg := range args {
			if argMap, ok := arg.(map[string]any); ok {
				widgets := getBsonArray(argMap["Widgets"])
				for _, w := range widgets {
					if wMap, ok := w.(map[string]any); ok {
						result = append(result, findImageWidgetsRecursive(wMap)...)
					}
				}
			}
		}
		return result
	}

	// Snippets have Widgets directly
	widgets := getBsonArray(rawData["Widgets"])
	var result []imageWidgetInfo
	for _, w := range widgets {
		if wMap, ok := w.(map[string]any); ok {
			result = append(result, findImageWidgetsRecursive(wMap)...)
		}
	}
	return result
}

// findImageWidgetsRecursive walks a widget tree and collects image widget info.
func findImageWidgetsRecursive(w map[string]any) []imageWidgetInfo {
	var result []imageWidgetInfo

	widgetType := extractStr(w["$Type"])

	// Check if this is an image widget
	if widgetType == "Forms$StaticImageViewer" || widgetType == "Forms$ImageViewer" {
		name := extractStr(w["Name"])
		result = append(result, imageWidgetInfo{
			Name:       name,
			WidgetType: widgetType,
			Configured: !IsUnconfiguredImage(widgetType, w),
		})
	}

	// Recurse into child widgets
	for _, child := range getBsonArray(w["Widgets"]) {
		if childMap, ok := child.(map[string]any); ok {
			result = append(result, findImageWidgetsRecursive(childMap)...)
		}
	}

	// LayoutGrid rows → columns → widgets
	for _, row := range getBsonArray(w["Rows"]) {
		if rowMap, ok := row.(map[string]any); ok {
			for _, col := range getBsonArray(rowMap["Columns"]) {
				if colMap, ok := col.(map[string]any); ok {
					for _, cw := range getBsonArray(colMap["Widgets"]) {
						if cwMap, ok := cw.(map[string]any); ok {
							result = append(result, findImageWidgetsRecursive(cwMap)...)
						}
					}
				}
			}
		}
	}

	// Footer widgets
	for _, fw := range getBsonArray(w["FooterWidgets"]) {
		if fwMap, ok := fw.(map[string]any); ok {
			result = append(result, findImageWidgetsRecursive(fwMap)...)
		}
	}

	// TabContainer tab pages
	for _, tp := range getBsonArray(w["TabPages"]) {
		if tpMap, ok := tp.(map[string]any); ok {
			for _, tw := range getBsonArray(tpMap["Widgets"]) {
				if twMap, ok := tw.(map[string]any); ok {
					result = append(result, findImageWidgetsRecursive(twMap)...)
				}
			}
		}
	}

	// CustomWidget nested widgets in properties
	if obj, ok := w["Object"].(map[string]any); ok {
		for _, prop := range getBsonArray(obj["Properties"]) {
			if propMap, ok := prop.(map[string]any); ok {
				if value, ok := propMap["Value"].(map[string]any); ok {
					for _, pw := range getBsonArray(value["Widgets"]) {
						if pwMap, ok := pw.(map[string]any); ok {
							result = append(result, findImageWidgetsRecursive(pwMap)...)
						}
					}
				}
			}
		}
	}

	// NavigationList items
	for _, item := range getBsonArray(w["Items"]) {
		if itemMap, ok := item.(map[string]any); ok {
			for _, iw := range getBsonArray(itemMap["Widgets"]) {
				if iwMap, ok := iw.(map[string]any); ok {
					result = append(result, findImageWidgetsRecursive(iwMap)...)
				}
			}
		}
	}

	return result
}

// extractStr safely extracts a string from an interface value.
func extractStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// getBsonArray extracts elements from a BSON array value.
// Handles primitive.A and []any types, and strips the leading type indicator
// integer that Mendix BSON arrays use.
func getBsonArray(v any) []any {
	if v == nil {
		return nil
	}
	var arr []any
	switch a := v.(type) {
	case []any:
		arr = a
	case primitive.A:
		arr = []any(a)
	default:
		return nil
	}
	if len(arr) == 0 {
		return nil
	}
	// Strip leading type indicator integer (Mendix BSON convention)
	if _, ok := arr[0].(int32); ok {
		return arr[1:]
	}
	if _, ok := arr[0].(int); ok {
		return arr[1:]
	}
	return arr
}
