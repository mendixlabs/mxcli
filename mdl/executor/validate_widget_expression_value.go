// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// validateWidgetExpressionValues is MDL-WIDGET42: a generic property written as
// an expression whose schema kind does not take one.
//
// The visitor keeps every expression a generic key is given — as an
// *ast.WidgetExpressionV3, or, for the bare `$var/Attr` spelling that parses as a
// data source, as that data source plus its source text — because only the
// widget's schema says whether the key is Expression-typed (widget_expression_value.go).
// This is the check-time half of that decision, so that `caption: $a + 'x'` on a
// text-template property is refused before exec rather than written empty.
//
// item is the object-list mapping when w is an item (a chart series, an HTML
// element attribute), nil for a widget; def is w's own definition, nil for a
// built-in widget or an item.
func validateWidgetExpressionValues(w *ast.WidgetV3, def *WidgetDefinition, item *ObjectListMapping,
	isObjectListItem bool, locationPrefix string) []linter.Violation {
	if w == nil || len(w.Properties) == 0 {
		return nil
	}
	keys := make([]string, 0, len(w.Properties))
	for k := range w.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys) // deterministic report order

	var out []linter.Violation
	for _, key := range keys {
		var text string
		isExpr := false
		switch v := w.Properties[key].(type) {
		case *ast.WidgetExpressionV3:
			text, isExpr = v.Text, true
		case *ast.DataSourceV3:
			src, ok := w.ExpressionSource(key)
			if !ok {
				continue
			}
			text = src
		default:
			continue
		}
		op, known := expressionValueOperation(key, def, item)
		switch {
		case op == "expression":
			continue
		case isExpr && !known && (def != nil || item != nil):
			// A key the definition does not map: MDL-WIDGET01 (unknown) or
			// MDL-WIDGET06 (known, unmapped) already speaks for it, and the
			// builder resolves its kind from the template.
			continue
		case isExpr && !known && isObjectListItem:
			continue // an item of a list whose mapping is out of sight
		case !isExpr && !scalarOperations[op]:
			// `$var/Assoc` in a datasource or action slot is exactly that.
			continue
		case !isExpr && op == "image" && isImageContextRef(w.Properties[key].(*ast.DataSourceV3)):
			// `Image: $currentObject` binds the image to the object in context
			// (mendixlabs/mxcli#1476); MDL-WIDGET22 judges where it may appear.
			continue
		}
		kind := "a plain-value property"
		if op != "" {
			kind = "a " + operationNoun(op) + " property"
		}
		out = append(out, linter.Violation{
			RuleID:   "MDL-WIDGET42",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s: widget `%s` property `%s` is %s and cannot hold the expression %s — "+
					"only an Expression-typed property of a pluggable widget takes one",
				locationPrefix, widgetLabelForError(w), key, kind, text),
			Suggestion: "Write a plain value here, or set the expression on an Expression property " +
				"(`mxcli widget describe <widget> -p app.mpr` lists each property's type).",
		})
	}
	return out
}

// expressionValueOperation is the engine operation the definition gives key —
// the object-list item's mapping when item is set, the widget's own (and its
// modes') mappings otherwise. known is false when nothing maps the key.
func expressionValueOperation(key string, def *WidgetDefinition, item *ObjectListMapping) (string, bool) {
	matches := func(propertyKey, source string, aliases []string) bool {
		if strings.EqualFold(propertyKey, key) || (source != "" && strings.EqualFold(source, key)) {
			return true
		}
		for _, a := range aliases {
			if strings.EqualFold(a, key) {
				return true
			}
		}
		return false
	}
	if item != nil {
		for _, ip := range item.ItemProperties {
			if matches(ip.PropertyKey, ip.Source, ip.MdlAliases) {
				return ip.Operation, true
			}
		}
		return "", false
	}
	if def == nil {
		return "", false
	}
	mappings := append([]PropertyMapping(nil), def.PropertyMappings...)
	for _, m := range def.Modes {
		mappings = append(mappings, m.PropertyMappings...)
	}
	for _, m := range mappings {
		if matches(m.PropertyKey, m.Source, m.MdlAliases) {
			return m.Operation, true
		}
	}
	return "", false
}
