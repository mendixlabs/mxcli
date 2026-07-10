// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

// mapPageWidget maps one executor page widget onto its pg_patch_page (Pages$*)
// form, then attaches any conditional-visibility setting (VISIBLE IF) uniformly.
func (b *Backend) mapPageWidget(w pages.Widget) (map[string]any, error) {
	m, err := b.mapPageWidgetBody(w)
	if err != nil {
		return nil, err
	}
	// Overlay the widget's design properties in one place (the body builders set an
	// empty designProperties via pageAppearance). Previously these were silently
	// dropped on every widget; injecting here covers all widget types uniformly.
	if bwg, ok := w.(interface{ GetBaseWidget() *pages.BaseWidget }); ok {
		if dps := bwg.GetBaseWidget().DesignProperties; len(dps) > 0 {
			if ap, ok := m["appearance"].(map[string]any); ok {
				ap["designProperties"] = designPropertiesMap(dps)
			}
		}
	}
	if cv := conditionalVisibility(w); cv != nil {
		m["conditionalVisibilitySettings"] = cv
	}
	return m, nil
}

// conditionalVisibility builds a Pages$ConditionalVisibilitySettings from a
// widget's VISIBLE IF expression. The MDL `visible:` property only ever produces
// an expression (no module-role / attribute / source-variable conditions), so
// only the expression form is mapped; the rest stay at their pg defaults.
func conditionalVisibility(w pages.Widget) map[string]any {
	type baseWidgetGetter interface {
		GetBaseWidget() *pages.BaseWidget
	}
	bwg, ok := w.(baseWidgetGetter)
	if !ok {
		return nil
	}
	cv := bwg.GetBaseWidget().ConditionalVisibility
	if cv == nil || cv.Expression == "" {
		return nil
	}
	return map[string]any{
		"$Type":          "Pages$ConditionalVisibilitySettings",
		"expression":     cv.Expression,
		"conditions":     []any{},
		"moduleRoles":    []any{},
		"ignoreSecurity": false,
	}
}

// mapPageWidgetBody maps the widget's own Pages$* form (without conditional
// settings). Coverage grows one widget type at a time; unmapped widgets are
// rejected.
func (b *Backend) mapPageWidgetBody(w pages.Widget) (map[string]any, error) {
	switch wd := w.(type) {
	case *pages.Container:
		children, err := b.mapPageWidgets(wd.Widgets)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"$Type":      "Pages$DivContainer",
			"name":       wd.Name,
			"appearance": pageAppearance(wd.Class, wd.Style),
			"widgets":    children,
		}, nil
	case *pages.ActionButton:
		action, err := mapClientAction(wd.Action)
		if err != nil {
			return nil, fmt.Errorf("button %q: %w", wd.Name, err)
		}
		style := buttonStyle(string(wd.ButtonStyle))
		// The executor stores a button's caption in CaptionTemplate (a client
		// template, so "{1}" params work); Caption is the legacy plain-text field.
		caption := clientTemplateValue(wd.CaptionTemplate)
		if s, ok := caption.(string); ok && s == "" {
			caption = textValue(wd.Caption)
		}
		return map[string]any{
			"$Type":                         "Pages$ActionButton",
			"name":                          wd.Name,
			"appearance":                    pageAppearance(wd.Class, wd.Style),
			"conditionalVisibilitySettings": nil,
			"ct:caption":                    caption,
			"t:tooltip":                     textValue(wd.Tooltip),
			"icon":                          nil,
			"action":                        action,
			"tabIndex":                      wd.TabIndex,
			"renderType":                    "Button",
			"buttonStyle":                   style,
			"ariaRole":                      "Button",
		}, nil
	case *pages.DynamicText:
		content := clientTemplateValue(wd.Content)
		if s, ok := content.(string); ok && s == "" && wd.AttributePath != "" {
			content = wd.AttributePath
		}
		renderMode := string(wd.RenderMode)
		if renderMode == "" {
			renderMode = "Text"
		}
		return map[string]any{
			"$Type":           "Pages$DynamicText",
			"name":            wd.Name,
			"appearance":      pageAppearance(wd.Class, wd.Style),
			"ct:content":      content,
			"tabIndex":        wd.TabIndex,
			"renderMode":      renderMode,
			"nativeTextStyle": "Text",
		}, nil
	case *pages.DataView:
		src, err := mapDataViewSource(wd.DataSource)
		if err != nil {
			return nil, fmt.Errorf("data view %q: %w", wd.Name, err)
		}
		children, err := b.mapPageWidgets(wd.Widgets)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"$Type":      "Pages$DataView",
			"name":       wd.Name,
			"appearance": pageAppearance(wd.Class, wd.Style),
			"dataSource": src,
			"editable":   wd.Editable,
			"widgets":    children,
		}, nil
	case *pages.LayoutGrid:
		rows := make([]any, 0, len(wd.Rows))
		for _, r := range wd.Rows {
			cols := make([]any, 0, len(r.Columns))
			for _, c := range r.Columns {
				kids, err := b.mapPageWidgets(c.Widgets)
				if err != nil {
					return nil, err
				}
				weight := c.Weight
				if weight <= 0 {
					weight = 12
				}
				tablet := c.TabletWeight
				if tablet <= 0 {
					tablet = weight
				}
				phone := c.PhoneWeight
				if phone <= 0 {
					phone = 12
				}
				cols = append(cols, map[string]any{
					"$Type":             "Pages$LayoutGridColumn",
					"appearance":        pageAppearance("", ""),
					"weight":            weight,
					"tabletWeight":      tablet,
					"phoneWeight":       phone,
					"previewWidth":      -1,
					"verticalAlignment": "None",
					"widgets":           kids,
				})
			}
			rows = append(rows, map[string]any{
				"$Type":                 "Pages$LayoutGridRow",
				"appearance":            pageAppearance("", ""),
				"verticalAlignment":     "None",
				"horizontalAlignment":   "None",
				"spacingBetweenColumns": true,
				"columns":               cols,
			})
		}
		return map[string]any{
			"$Type":      "Pages$LayoutGrid",
			"name":       wd.Name,
			"appearance": pageAppearance(wd.Class, wd.Style),
			"tabIndex":   wd.TabIndex,
			"width":      "FullWidth",
			"rows":       rows,
		}, nil
	case *pages.ListView:
		src, err := mapListViewSource(wd.DataSource)
		if err != nil {
			return nil, fmt.Errorf("list view %q: %w", wd.Name, err)
		}
		rowWidgets := wd.Widgets
		if len(rowWidgets) == 0 && len(wd.Templates) > 0 {
			rowWidgets = wd.Templates[0].Widgets
		}
		kids, err := b.mapPageWidgets(rowWidgets)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"$Type":      "Pages$ListView",
			"name":       wd.Name,
			"appearance": pageAppearance(wd.Class, wd.Style),
			"dataSource": src,
			"editable":   wd.Editable,
			"widgets":    kids,
		}, nil
	case *pages.TabContainer:
		tabs := make([]any, 0, len(wd.TabPages))
		for _, tp := range wd.TabPages {
			kids, err := b.mapPageWidgets(tp.Widgets)
			if err != nil {
				return nil, fmt.Errorf("tab page %q: %w", tp.Name, err)
			}
			caption := textValue(tp.Caption)
			if caption == "" {
				caption = tp.Name
			}
			tabs = append(tabs, map[string]any{
				"$Type":         "Pages$TabPage",
				"name":          tp.Name,
				"t:caption":     caption,
				"refreshOnShow": tp.RefreshOnShow,
				"widgets":       kids,
			})
		}
		return map[string]any{
			"$Type":      "Pages$TabContainer",
			"name":       wd.Name,
			"appearance": pageAppearance(wd.Class, wd.Style),
			"tabIndex":   wd.TabIndex,
			"tabPages":   tabs,
		}, nil
	case *pages.CustomWidget:
		return b.mapCustomWidget(wd)
	case *pages.DataGrid:
		return nil, fmt.Errorf("legacy DataGrid is not supported by the MCP backend — pg_patch_page has no Pages$DataGrid type (use a ListView, or DataGrid 2 which is a pluggable widget)")
	case *pages.TextBox:
		return inputWidget("Pages$TextBox", wd.Name, wd.Label, wd.AttributePath, wd.Class, wd.Style), nil
	case *pages.CheckBox:
		return inputWidget("Pages$CheckBox", wd.Name, wd.Label, wd.AttributePath, wd.Class, wd.Style), nil
	case *pages.DatePicker:
		return inputWidget("Pages$DatePicker", wd.Name, wd.Label, wd.AttributePath, wd.Class, wd.Style), nil
	case *pages.TextArea:
		return inputWidget("Pages$TextArea", wd.Name, wd.Label, wd.AttributePath, wd.Class, wd.Style), nil
	case *pages.RadioButtons:
		return inputWidget("Pages$RadioButtonGroup", wd.Name, wd.Label, wd.AttributePath, wd.Class, wd.Style), nil
	default:
		return nil, fmt.Errorf("page widget type %s is not yet supported by the MCP backend", w.GetTypeName())
	}
}

// inputWidget builds a label+attribute input widget (TextBox/CheckBox/DatePicker).
// The executor already resolves AttributePath to a fully-qualified
// "Module.Entity.Attribute", which is exactly what pg's attributeRef wants.
func inputWidget(typ, name, label, attribute, class, style string) map[string]any {
	return map[string]any{
		"$Type":            typ,
		"name":             name,
		"appearance":       pageAppearance(class, style),
		"ct:labelTemplate": label,
		"attributeRef": map[string]any{
			"$Type":     "DomainModels$AttributeRef",
			"attribute": attribute,
		},
	}
}

// mapListViewSource maps a list-view data source. A database source becomes a
// Pages$ListViewXPathSource — the official metamodel's list-view database/XPath
// source type, which (unlike the context-only Pages$DataViewSource) carries an
// xPathConstraint and a sortBar. Non-database sources (microflow, page variable)
// fall through to the shared data-view source mapping.
func mapListViewSource(ds pages.DataSource) (map[string]any, error) {
	if db, ok := ds.(*pages.DatabaseSource); ok {
		if db.EntityName == "" {
			return nil, fmt.Errorf("list view database source has no entity")
		}
		src := map[string]any{
			"$Type":     "Pages$ListViewXPathSource",
			"entityRef": map[string]any{"$Type": "DomainModels$DirectEntityRef", "entity": db.EntityName},
		}
		if db.XPathConstraint != "" {
			src["xPathConstraint"] = db.XPathConstraint
		}
		if len(db.Sorting) > 0 {
			src["sortBar"] = gridSortBar(db.Sorting)
		}
		return src, nil
	}
	return mapDataViewSource(ds)
}

// mapDataViewSource maps a data-view data source onto a Pages$DataViewSource.
// Supported: a page variable/parameter ($Var). Other source kinds (microflow,
// association, listen-to-widget, multi-step paths) are rejected for now.
func mapDataViewSource(ds pages.DataSource) (map[string]any, error) {
	switch s := ds.(type) {
	case *pages.DataViewSource:
		if s.ParameterName == "" && s.EntityName == "" {
			return nil, fmt.Errorf("data view source has neither a page parameter nor an entity")
		}
		src := map[string]any{"$Type": "Pages$DataViewSource", "forceFullObjects": false}
		// A context (parameter-bound) data view still needs its entity configured
		// ON the data source: Studio Pro writes entityRef AND the sourceVariable
		// binding together (see testdata/pg-page-contact-newedit.json). Setting
		// only sourceVariable leaves the source with no entity → CE0488 "No entity
		// configured for the data source of this data view". The executor resolves
		// the page parameter's entity into EntityName (cmd_pages_builder_v3.go), so
		// it is available here even when a parameter is the source.
		if s.EntityName != "" {
			src["entityRef"] = map[string]any{
				"$Type":  "DomainModels$DirectEntityRef",
				"entity": s.EntityName,
			}
		}
		if s.ParameterName != "" {
			src["sourceVariable"] = map[string]any{
				"$Type":         "Pages$PageVariable",
				"pageParameter": s.ParameterName,
				"useAllPages":   false,
			}
		}
		return src, nil
	case *pages.MicroflowSource:
		if s.Microflow == "" {
			return nil, fmt.Errorf("microflow data source has no microflow")
		}
		return map[string]any{
			"$Type":            "Pages$MicroflowSource",
			"forceFullObjects": false,
			"microflowSettings": map[string]any{
				"$Type":             "Pages$MicroflowSettings",
				"microflow":         s.Microflow,
				"parameterMappings": []any{},
				"outputMappings":    []any{},
				"progressBar":       "None",
				"asynchronous":      false,
				"formValidations":   "All",
			},
		}, nil
	case *pages.DatabaseSource:
		if s.XPathConstraint != "" || len(s.Sorting) > 0 {
			// pg's Pages$DataViewSource has no constraint/sort fields and silently
			// drops them, so reject rather than write a misleading widget. (XPath
			// constraints and sorting ARE supported on the pluggable DataGrid 2 /
			// Gallery, which use CustomWidgets$CustomWidgetXPathSource.)
			return nil, fmt.Errorf("an XPath constraint or sorting on a data-view/list-view database source is not supported by pg (use a DataGrid 2 or Gallery, which support both)")
		}
		if s.EntityName == "" {
			return nil, fmt.Errorf("database data source has no entity")
		}
		return map[string]any{
			"$Type":            "Pages$DataViewSource",
			"entityRef":        map[string]any{"$Type": "DomainModels$DirectEntityRef", "entity": s.EntityName},
			"forceFullObjects": false,
		}, nil
	case *pages.EntityPathSource:
		if strings.HasPrefix(s.EntityPath, "$") && !strings.Contains(s.EntityPath, "/") {
			return map[string]any{
				"$Type": "Pages$DataViewSource",
				"sourceVariable": map[string]any{
					"$Type":         "Pages$PageVariable",
					"pageParameter": strings.TrimPrefix(s.EntityPath, "$"),
					"useAllPages":   false,
				},
				"forceFullObjects": false,
			}, nil
		}
		return nil, fmt.Errorf("data view over path %q is not yet supported by the MCP backend (only a page variable $Var)", s.EntityPath)
	default:
		return nil, fmt.Errorf("data view source %T is not yet supported by the MCP backend", ds)
	}
}

func (b *Backend) mapPageWidgets(ws []pages.Widget) ([]any, error) {
	out := make([]any, 0, len(ws))
	for _, w := range ws {
		m, err := b.mapPageWidget(w)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// mapClientAction maps a widget client action onto its Pages$*ClientAction form.
func mapClientAction(a pages.ClientAction) (map[string]any, error) {
	switch act := a.(type) {
	case nil, *pages.NoClientAction:
		return map[string]any{"$Type": "Pages$NoClientAction"}, nil
	case *pages.MicroflowClientAction:
		return map[string]any{
			"$Type":     "Pages$MicroflowClientAction",
			"microflow": act.MicroflowName,
		}, nil
	case *pages.PageClientAction:
		return map[string]any{
			"$Type": "Pages$PageClientAction",
			"pageSettings": map[string]any{
				"$Type":             "Pages$PageSettings",
				"page":              act.PageName,
				"parameterMappings": []any{},
			},
		}, nil
	case *pages.CreateObjectClientAction:
		// Common control-bar "New" action: create an object then open a page on it.
		return map[string]any{
			"$Type":     "Pages$CreateObjectClientAction",
			"entityRef": map[string]any{"$Type": "DomainModels$DirectEntityRef", "entity": act.EntityName},
			"pageSettings": map[string]any{
				"$Type":             "Pages$PageSettings",
				"page":              act.PageName,
				"parameterMappings": []any{},
			},
			"disabledDuringExecution": true,
		}, nil
	case *pages.SaveChangesClientAction:
		// Standard edit-page "Save". Shapes verified against a Studio-Pro-generated
		// page (testdata/pg-page-contact-newedit.json).
		return map[string]any{
			"$Type":                   "Pages$SaveChangesClientAction",
			"disabledDuringExecution": true,
			"syncAutomatically":       false,
			"closePage":               act.ClosePage,
		}, nil
	case *pages.CancelChangesClientAction:
		return map[string]any{
			"$Type":                   "Pages$CancelChangesClientAction",
			"disabledDuringExecution": true,
			"closePage":               act.ClosePage,
		}, nil
	case *pages.ClosePageClientAction:
		return map[string]any{
			"$Type":                   "Pages$ClosePageClientAction",
			"disabledDuringExecution": true,
		}, nil
	default:
		return nil, fmt.Errorf("client action %T is not yet supported by the MCP backend", a)
	}
}

// buttonStyle normalizes an MDL button-style token (which the executor passes
// through verbatim, e.g. lowercase "primary") to pg's canonical enum value
// ("Primary"). pg rejects an unknown value, so an unrecognized style falls back
// to "Default".
func buttonStyle(s string) string {
	switch strings.ToLower(s) {
	case "primary":
		return "Primary"
	case "secondary":
		return "Secondary"
	case "success":
		return "Success"
	case "warning":
		return "Warning"
	case "danger":
		return "Danger"
	case "inverse":
		return "Inverse"
	case "link":
		return "Link"
	default:
		return "Default"
	}
}

// pageAppearance builds a Pages$Appearance from a widget's class/style. Design
// properties start empty and are overlaid in mapPageWidget (one place, all
// widget types) via designPropertiesMap.
func pageAppearance(class, style string) map[string]any {
	return map[string]any{
		"$Type":            "Pages$Appearance",
		"class":            class,
		"style":            style,
		"dynamicClasses":   "",
		"designProperties": map[string]any{},
	}
}

// designPropertiesMap renders a widget's design properties into the object shape
// pg_patch_page expects (verified against testdata/pg-page-contact-newedit-
// designprops.json): keys are "<kind>:<DisplayName>" — "toggle:" with a bool
// value, "option:" with a string value, "compound:" with a nested object of the
// same shape (e.g. "compound:Spacing": {"option:margin-top": "L"}). "custom"
// carries a string value like option.
func designPropertiesMap(dps []pages.DesignPropertyValue) map[string]any {
	out := make(map[string]any, len(dps))
	for _, dp := range dps {
		switch dp.ValueType {
		case "toggle":
			out["toggle:"+dp.Key] = true
		case "compound":
			out["compound:"+dp.Key] = designPropertiesMap(dp.Compound)
		default: // "option" and "custom" both carry a string value
			out["option:"+dp.Key] = dp.Option
		}
	}
	return out
}

// textValue returns the en_US translation of a localized text (empty if nil).
func textValue(t *model.Text) string {
	if t == nil {
		return ""
	}
	return t.Translations["en_US"]
}

// clientTemplateValue maps a client template (e.g. a DynamicText's content) onto
// its pg form. With no parameters it returns the plain template string — pg wraps
// it into a Pages$ClientTemplate and the caller's `ct:` key carries the wrapping.
// With parameters (a "{1}"-style binding) it returns the full Pages$ClientTemplate
// so the parameter bindings are preserved (otherwise the literal "{1}" would show).
func clientTemplateValue(ct *pages.ClientTemplate) any {
	if ct == nil {
		return ""
	}
	text := textValue(ct.Template)
	if len(ct.Parameters) == 0 {
		return text
	}
	params := make([]any, 0, len(ct.Parameters))
	for _, p := range ct.Parameters {
		params = append(params, clientTemplateParam(p))
	}
	return map[string]any{
		"$Type":      "Pages$ClientTemplate",
		"t:template": text, // plain string; the server wraps it in Texts$Text
		"parameters": params,
	}
}

// clientTemplateParam maps one template parameter ("{N}" binding) onto a
// Pages$ClientTemplateParameter. Attribute, literal-expression, and page-variable
// parameters are supported (Studio Pro fills formattingInfo defaults).
func clientTemplateParam(p *pages.ClientTemplateParameter) map[string]any {
	param := map[string]any{"$Type": "Pages$ClientTemplateParameter"}
	switch {
	case p.AttributeRef != "":
		param["attributeRef"] = map[string]any{"$Type": "DomainModels$AttributeRef", "attribute": p.AttributeRef}
	case p.Expression != "":
		param["expression"] = p.Expression
	case p.SourceVariable != "":
		param["sourceVariable"] = map[string]any{"$Type": "Pages$PageVariable", "pageParameter": p.SourceVariable, "useAllPages": false}
	}
	return param
}
