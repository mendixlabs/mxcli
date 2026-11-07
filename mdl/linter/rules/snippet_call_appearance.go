// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
)

// SnippetCallAppearanceRule reports a snippet call that carries Class, Style,
// DynamicClasses or design properties (mendixlabs/mxcli#1511).
//
// A snippet call renders no element of its own — the snippet's widgets are
// inlined into the parent — so those settings have nothing to land on.
// Measured on Mendix 11.15.0 (`run --local`, `playwright check`): a snippetcall
// with Class 'probe-x', Style 'border: 3px solid red', DynamicClasses
// 'probe-dyn' and the design properties 'Align self' / 'Hide on' produced no
// element carrying any of them; the snippet's own container was a direct child
// of the enclosing container. A container on the same page with the same four
// settings carried every one (the control). `mx check` reports nothing, so
// without this the setting is silently useless.
//
// `mxcli check` warns the same as MDL-WIDGET48 when a script writes one; this
// rule finds the ones a project already holds, including those authored in
// Studio Pro, which offers the fields on a snippet call.
type SnippetCallAppearanceRule struct{}

// NewSnippetCallAppearanceRule creates a new snippet call appearance rule.
func NewSnippetCallAppearanceRule() *SnippetCallAppearanceRule {
	return &SnippetCallAppearanceRule{}
}

func (r *SnippetCallAppearanceRule) ID() string                       { return "MPR013" }
func (r *SnippetCallAppearanceRule) Name() string                     { return "SnippetCallAppearance" }
func (r *SnippetCallAppearanceRule) Category() string                 { return "correctness" }
func (r *SnippetCallAppearanceRule) DefaultSeverity() linter.Severity { return linter.SeverityWarning }

// RequiredCatalogMode: ctx.Widgets() reads widgets_data, which only a full
// catalog build fills.
func (r *SnippetCallAppearanceRule) RequiredCatalogMode() linter.CatalogMode {
	return linter.CatalogFull
}

func (r *SnippetCallAppearanceRule) Description() string {
	return "Checks for snippet calls with Class, Style, DynamicClasses or design properties, which never reach the page (a snippet call renders no element)"
}

const snippetCallWidgetType = "Forms$SnippetCallWidget"

// Check reports one violation per styled snippet call. Class, Style and
// DynamicClasses come from the catalog; design properties, which the catalog
// does not record, from the stored document when a reader is available.
func (r *SnippetCallAppearanceRule) Check(ctx *linter.LintContext) []linter.Violation {
	type call struct {
		w      linter.Widget
		fields []string
	}
	var calls []*call
	byContainer := map[string]map[string]*call{}

	for w := range ctx.Widgets() {
		if ctx.IsExcluded(w.ModuleName) || w.WidgetType != snippetCallWidgetType {
			continue
		}
		c := &call{w: w}
		if strings.TrimSpace(w.Class) != "" {
			c.fields = append(c.fields, "Class")
		}
		if strings.TrimSpace(w.Style) != "" {
			c.fields = append(c.fields, "Style")
		}
		if strings.TrimSpace(w.DynamicClasses) != "" {
			c.fields = append(c.fields, "DynamicClasses")
		}
		calls = append(calls, c)
		if byContainer[w.ContainerID] == nil {
			byContainer[w.ContainerID] = map[string]*call{}
		}
		byContainer[w.ContainerID][w.Name] = c
	}

	if reader := ctx.Reader(); reader != nil {
		for containerID, named := range byContainer {
			raw, err := reader.GetRawUnit(model.ID(containerID))
			if err != nil || raw == nil {
				continue
			}
			for name, keys := range findSnippetCallDesignProperties(raw) {
				if c := named[name]; c != nil {
					for _, k := range keys {
						c.fields = append(c.fields, fmt.Sprintf("design property '%s'", k))
					}
				}
			}
		}
	}

	var violations []linter.Violation
	for _, c := range calls {
		if len(c.fields) == 0 {
			continue
		}
		docType := widgetDocType(c.w.ContainerType)
		violations = append(violations, linter.Violation{
			RuleID:   r.ID(),
			Severity: r.DefaultSeverity(),
			Message: fmt.Sprintf("snippet call '%s' in %s sets %s, but a snippet call renders no element of its own "+
				"(the snippet's widgets are inlined into the parent), so the setting never reaches the page",
				c.w.Name, c.w.ContainerQualifiedName, strings.Join(c.fields, ", ")),
			Location: linter.Location{
				Module:       c.w.ModuleName,
				DocumentType: docType,
				DocumentName: docNameFromQualified(c.w.ContainerQualifiedName),
				DocumentID:   c.w.ContainerID,
			},
			Suggestion: "Put the class or style on a container inside the snippet, " +
				"or wrap the snippet call in a container and style that container",
		})
	}
	return violations
}

// findSnippetCallDesignProperties walks a raw page or snippet document and
// returns, per snippet call name, the design-property keys it stores.
//
// It visits every nested document rather than following named child slots: a
// snippet call can sit in any widget's children (layout grid columns, tab
// pages, pluggable widget slots, list view templates), and the one thing that
// identifies it is its $Type.
func findSnippetCallDesignProperties(raw map[string]any) map[string][]string {
	out := map[string][]string{}
	var walk func(v any)
	walk = func(v any) {
		switch n := v.(type) {
		case map[string]any:
			if extractStr(n["$Type"]) == snippetCallWidgetType {
				if app, ok := n["Appearance"].(map[string]any); ok {
					for _, dp := range getBsonArray(app["DesignProperties"]) {
						if m, ok := dp.(map[string]any); ok {
							if k := extractStr(m["Key"]); k != "" {
								name := extractStr(n["Name"])
								out[name] = append(out[name], k)
							}
						}
					}
				}
			}
			for _, child := range n {
				walk(child)
			}
		case []any:
			for _, child := range n {
				walk(child)
			}
		}
	}
	walk(raw)
	return out
}
