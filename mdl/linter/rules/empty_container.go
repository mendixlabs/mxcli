// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// EmptyContainerRule checks for CONTAINER widgets with no children.
// Empty containers crash the runtime with "Did not expect an argument to be undefined".
type EmptyContainerRule struct{}

// NewEmptyContainerRule creates a new empty container rule.
func NewEmptyContainerRule() *EmptyContainerRule {
	return &EmptyContainerRule{}
}

func (r *EmptyContainerRule) ID() string                       { return "MPR006" }
func (r *EmptyContainerRule) Name() string                     { return "EmptyContainer" }
func (r *EmptyContainerRule) Category() string                 { return "correctness" }
func (r *EmptyContainerRule) DefaultSeverity() linter.Severity { return linter.SeverityWarning }

func (r *EmptyContainerRule) Description() string {
	return "Checks for CONTAINER widgets with no children (crashes at runtime)"
}

// emptyContainerInfo holds information about a found empty container widget.
type emptyContainerInfo struct {
	Name string
}

// Check iterates over catalog widgets, loads raw BSON for containers with DivContainer widgets,
// and checks whether each DivContainer has at least one child widget.
func (r *EmptyContainerRule) Check(ctx *linter.LintContext) []linter.Violation {
	reader := ctx.Reader()
	if reader == nil {
		return nil
	}

	// Collect page/snippet containers that have DivContainer widgets
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
		if w.WidgetType != "Forms$DivContainer" {
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

		empties := findEmptyContainers(rawData)
		for _, e := range empties {
			docType := "page"
			if c.Type == "SNIPPET" {
				docType = "snippet"
			}

			violations = append(violations, linter.Violation{
				RuleID:   r.ID(),
				Severity: r.DefaultSeverity(),
				Message: fmt.Sprintf("CONTAINER '%s' in %s has no children and will crash at runtime",
					e.Name, c.QualifiedName),
				Location: linter.Location{
					Module:       c.ModuleName,
					DocumentType: docType,
					DocumentName: docNameFromQualified(c.QualifiedName),
					DocumentID:   c.ID,
				},
				Suggestion: "Add a child widget (e.g., DYNAMICTEXT with Content: ' ') or remove the empty container",
			})
		}
	}

	return violations
}

// findEmptyContainers walks a raw BSON page/snippet document and returns all empty DivContainer widgets.
func findEmptyContainers(rawData map[string]any) []emptyContainerInfo {
	// Pages have FormCall → Arguments → Widgets
	if formCall, ok := rawData["FormCall"].(map[string]any); ok {
		args := getBsonArray(formCall["Arguments"])
		var result []emptyContainerInfo
		for _, arg := range args {
			if argMap, ok := arg.(map[string]any); ok {
				widgets := getBsonArray(argMap["Widgets"])
				for _, w := range widgets {
					if wMap, ok := w.(map[string]any); ok {
						result = append(result, findEmptyContainersRecursive(wMap)...)
					}
				}
			}
		}
		return result
	}

	// Snippets have Widgets directly
	widgets := getBsonArray(rawData["Widgets"])
	var result []emptyContainerInfo
	for _, w := range widgets {
		if wMap, ok := w.(map[string]any); ok {
			result = append(result, findEmptyContainersRecursive(wMap)...)
		}
	}
	return result
}

// findEmptyContainersRecursive walks a widget tree and collects empty DivContainer info.
func findEmptyContainersRecursive(w map[string]any) []emptyContainerInfo {
	var result []emptyContainerInfo

	widgetType := extractStr(w["$Type"])

	// Check if this is an empty DivContainer
	if widgetType == "Forms$DivContainer" {
		children := getBsonArray(w["Widgets"])
		if len(children) == 0 {
			name := extractStr(w["Name"])
			result = append(result, emptyContainerInfo{Name: name})
		}
	}

	// Recurse into child widgets
	for _, child := range getBsonArray(w["Widgets"]) {
		if childMap, ok := child.(map[string]any); ok {
			result = append(result, findEmptyContainersRecursive(childMap)...)
		}
	}

	// LayoutGrid rows → columns → widgets
	for _, row := range getBsonArray(w["Rows"]) {
		if rowMap, ok := row.(map[string]any); ok {
			for _, col := range getBsonArray(rowMap["Columns"]) {
				if colMap, ok := col.(map[string]any); ok {
					for _, cw := range getBsonArray(colMap["Widgets"]) {
						if cwMap, ok := cw.(map[string]any); ok {
							result = append(result, findEmptyContainersRecursive(cwMap)...)
						}
					}
				}
			}
		}
	}

	// Footer widgets
	for _, fw := range getBsonArray(w["FooterWidgets"]) {
		if fwMap, ok := fw.(map[string]any); ok {
			result = append(result, findEmptyContainersRecursive(fwMap)...)
		}
	}

	// TabContainer tab pages
	for _, tp := range getBsonArray(w["TabPages"]) {
		if tpMap, ok := tp.(map[string]any); ok {
			for _, tw := range getBsonArray(tpMap["Widgets"]) {
				if twMap, ok := tw.(map[string]any); ok {
					result = append(result, findEmptyContainersRecursive(twMap)...)
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
							result = append(result, findEmptyContainersRecursive(pwMap)...)
						}
					}
				}
			}
		}
	}

	return result
}
