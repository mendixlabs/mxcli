// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// GallerySelectionListenerRule catches Studio Pro error CE3637:
// "A data view cannot listen to the selection of Gallery 'X', because it
// is not available here." This happens whenever a DataView is bound via
// `DataSource: selection X` but the referenced gallery has its pluggable
// `itemSelectionMode` left at the default `"clear"`. Mendix only exposes
// the gallery's selection to a listener when `itemSelectionMode` is
// `"toggle"` ("Item click toggles selection: Yes" in Studio Pro).
//
// The rule walks each page's BSON, collects DataViews whose DataSource is
// a Forms$ListenTargetSource, and verifies that the named target is a
// gallery widget (`com.mendix.widget.web.gallery.Gallery`) with
// `itemSelectionMode == "toggle"`.
type GallerySelectionListenerRule struct{}

// NewGallerySelectionListenerRule creates a new gallery selection listener rule.
func NewGallerySelectionListenerRule() *GallerySelectionListenerRule {
	return &GallerySelectionListenerRule{}
}

func (r *GallerySelectionListenerRule) ID() string                       { return "MPR009" }
func (r *GallerySelectionListenerRule) Name() string                     { return "GallerySelectionListener" }
func (r *GallerySelectionListenerRule) Category() string                 { return "correctness" }
func (r *GallerySelectionListenerRule) DefaultSeverity() linter.Severity { return linter.SeverityError }

func (r *GallerySelectionListenerRule) Description() string {
	return "Gallery referenced by DataView's `DataSource: selection X` must set ItemSelectionMode: toggle (Studio Pro CE3637)"
}

// Check runs the rule across all non-excluded pages and snippets.
// Uses ctx.Pages()/ctx.Snippets() (fast-mode catalog tables) so the rule
// works without `REFRESH CATALOG FULL` — only the raw BSON is read to walk
// the widget tree.
func (r *GallerySelectionListenerRule) Check(ctx *linter.LintContext) []linter.Violation {
	reader := ctx.Reader()
	if reader == nil {
		return nil
	}

	var violations []linter.Violation

	check := func(id, qualifiedName, moduleName, docType string) {
		raw, err := reader.GetRawUnit(model.ID(id))
		if err != nil || raw == nil {
			return
		}
		byName := collectWidgetsByName(raw)
		for _, dv := range byName {
			target := dataViewListenTarget(dv)
			if target == "" {
				continue
			}
			gallery, ok := byName[target]
			if !ok {
				continue
			}
			if !isGalleryWidget(gallery) {
				continue
			}
			mode := customWidgetPropertyString(gallery, "itemSelectionMode")
			if mode == "toggle" {
				continue
			}
			violations = append(violations, linter.Violation{
				RuleID:   r.ID(),
				Severity: r.DefaultSeverity(),
				Message: fmt.Sprintf("DataView '%s' listens to gallery '%s' but the gallery's ItemSelectionMode is %q (need \"toggle\") — Studio Pro CE3637",
					widgetName(dv), target, fallback(mode, "clear")),
				Location: linter.Location{
					Module:       moduleName,
					DocumentType: docType,
					DocumentName: docNameFromQualified(qualifiedName),
					DocumentID:   id,
				},
				Suggestion: fmt.Sprintf("Add `ItemSelectionMode: toggle` to gallery '%s'", target),
			})
		}
	}

	for p := range ctx.Pages() {
		if ctx.IsExcluded(p.ModuleName) {
			continue
		}
		check(p.ID, p.QualifiedName, p.ModuleName, "page")
	}
	for s := range ctx.Snippets() {
		if ctx.IsExcluded(s.ModuleName) {
			continue
		}
		check(s.ID, s.QualifiedName, s.ModuleName, "snippet")
	}
	return violations
}

func widgetName(w map[string]any) string { return extractStr(w["Name"]) }

func fallback(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// dataViewListenTarget returns the ListenTarget widget name if w is a
// DataView whose DataSource is a Forms$ListenTargetSource; "" otherwise.
func dataViewListenTarget(w map[string]any) string {
	t := extractStr(w["$Type"])
	if t != "Forms$DataView" && t != "Pages$DataView" {
		return ""
	}
	ds, ok := w["DataSource"].(map[string]any)
	if !ok {
		return ""
	}
	if extractStr(ds["$Type"]) != "Forms$ListenTargetSource" {
		return ""
	}
	return extractStr(ds["ListenTarget"])
}

// isGalleryWidget reports whether w is the pluggable gallery widget.
func isGalleryWidget(w map[string]any) bool {
	if extractStr(w["$Type"]) != "CustomWidgets$CustomWidget" {
		return false
	}
	typeObj, ok := w["Type"].(map[string]any)
	if !ok {
		return false
	}
	return extractStr(typeObj["WidgetId"]) == "com.mendix.widget.web.gallery.Gallery"
}

// customWidgetPropertyString reads a primitive string property by key
// from a CustomWidgets$CustomWidget, resolving Object.Properties[].TypePointer
// against Type.ObjectType.PropertyTypes[].PropertyKey.
func customWidgetPropertyString(w map[string]any, propertyKey string) string {
	typeObj, ok := w["Type"].(map[string]any)
	if !ok {
		return ""
	}
	objectType, ok := typeObj["ObjectType"].(map[string]any)
	if !ok {
		return ""
	}
	idForKey := make(map[string]string)
	for _, pt := range getBsonArray(objectType["PropertyTypes"]) {
		ptMap, ok := pt.(map[string]any)
		if !ok {
			continue
		}
		key := extractStr(ptMap["PropertyKey"])
		id := bsonBinaryID(ptMap["$ID"])
		if key != "" && id != "" {
			idForKey[id] = key
		}
	}

	obj, ok := w["Object"].(map[string]any)
	if !ok {
		return ""
	}
	for _, prop := range getBsonArray(obj["Properties"]) {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		tpID := bsonBinaryID(propMap["TypePointer"])
		if idForKey[tpID] != propertyKey {
			continue
		}
		value, ok := propMap["Value"].(map[string]any)
		if !ok {
			continue
		}
		if pv, ok := value["PrimitiveValue"].(string); ok {
			return pv
		}
	}
	return ""
}

// bsonBinaryID returns the GUID string for a BSON binary $ID/TypePointer value.
// Mirrors mdl/executor extractBinaryID without taking a dependency on that package.
func bsonBinaryID(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return guidFromBytes(val)
	case primitive.Binary:
		return guidFromBytes(val.Data)
	}
	return ""
}

func guidFromBytes(data []byte) string {
	if len(data) != 16 {
		return string(data)
	}
	const hex = "0123456789abcdef"
	// Mendix GUIDs are stored with the first three groups byte-reversed.
	order := [16]int{3, 2, 1, 0, 5, 4, 7, 6, 8, 9, 10, 11, 12, 13, 14, 15}
	out := make([]byte, 0, 36)
	for i, idx := range order {
		out = append(out, hex[data[idx]>>4], hex[data[idx]&0x0f])
		if i == 3 || i == 5 || i == 7 || i == 9 {
			out = append(out, '-')
		}
	}
	return string(out)
}

// collectWidgetsByName walks a raw page/snippet document and returns every
// named widget keyed by Name. Names are not unique across containers in
// general, but Studio Pro requires uniqueness within a page for selection
// listeners, so a flat name map is sufficient for this rule.
func collectWidgetsByName(rawData map[string]any) map[string]map[string]any {
	out := make(map[string]map[string]any)
	// Pages: FormCall.Arguments[].Widgets
	if formCall, ok := rawData["FormCall"].(map[string]any); ok {
		for _, arg := range getBsonArray(formCall["Arguments"]) {
			if argMap, ok := arg.(map[string]any); ok {
				for _, w := range getBsonArray(argMap["Widgets"]) {
					if wMap, ok := w.(map[string]any); ok {
						collectWidgetsRecursive(wMap, out)
					}
				}
			}
		}
	}
	// Snippets: top-level Widgets
	for _, w := range getBsonArray(rawData["Widgets"]) {
		if wMap, ok := w.(map[string]any); ok {
			collectWidgetsRecursive(wMap, out)
		}
	}
	return out
}

func collectWidgetsRecursive(w map[string]any, out map[string]map[string]any) {
	if name := extractStr(w["Name"]); name != "" {
		out[name] = w
	}
	for _, child := range getBsonArray(w["Widgets"]) {
		if cm, ok := child.(map[string]any); ok {
			collectWidgetsRecursive(cm, out)
		}
	}
	for _, fw := range getBsonArray(w["FooterWidgets"]) {
		if fwMap, ok := fw.(map[string]any); ok {
			collectWidgetsRecursive(fwMap, out)
		}
	}
	for _, row := range getBsonArray(w["Rows"]) {
		if rowMap, ok := row.(map[string]any); ok {
			for _, col := range getBsonArray(rowMap["Columns"]) {
				if colMap, ok := col.(map[string]any); ok {
					for _, cw := range getBsonArray(colMap["Widgets"]) {
						if cwMap, ok := cw.(map[string]any); ok {
							collectWidgetsRecursive(cwMap, out)
						}
					}
				}
			}
		}
	}
	for _, tp := range getBsonArray(w["TabPages"]) {
		if tpMap, ok := tp.(map[string]any); ok {
			for _, tw := range getBsonArray(tpMap["Widgets"]) {
				if twMap, ok := tw.(map[string]any); ok {
					collectWidgetsRecursive(twMap, out)
				}
			}
		}
	}
	if cr, ok := w["CenterRegion"].(map[string]any); ok {
		for _, cw := range getBsonArray(cr["Widgets"]) {
			if cwMap, ok := cw.(map[string]any); ok {
				collectWidgetsRecursive(cwMap, out)
			}
		}
	}
	// Pluggable widget child slots: Object.Properties[].Value.Widgets
	if obj, ok := w["Object"].(map[string]any); ok {
		for _, prop := range getBsonArray(obj["Properties"]) {
			if propMap, ok := prop.(map[string]any); ok {
				if value, ok := propMap["Value"].(map[string]any); ok {
					for _, pw := range getBsonArray(value["Widgets"]) {
						if pwMap, ok := pw.(map[string]any); ok {
							collectWidgetsRecursive(pwMap, out)
						}
					}
				}
			}
		}
	}
}
