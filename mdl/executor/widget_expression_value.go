// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// A pluggable widget property's schema kind decides what its value means, and
// only the executor knows that kind. The grammar reads a bare Mendix expression
// in two ways: `$currentObject/ColorHex` matches the variable-led data source
// form, and anything longer (`if … then … else`, `$a/B + 'x'`) the expression
// form. Both used to reach an Expression-typed property as a non-string, and
// every reader stringified it to "" — so `dynamicBarColor: $currentObject/ColorHex`
// checked clean, executed clean, and wrote an empty expression: the chart drew
// its default colours and DESCRIBE showed nothing.
//
// scalarPropertyText is the one place those values are turned into the text a
// scalar operation stores: an Expression property takes the expression's source
// text, and every other scalar kind refuses it — an error, never an empty value.

// scalarOperations are the engine operations whose value is a single piece of
// text. Datasource, action, widgets and the list operations read their own
// shapes and never pass through here.
var scalarOperations = map[string]bool{
	"expression":   true,
	"texttemplate": true,
	"primitive":    true,
	"attribute":    true,
	"image":        true,
	"selection":    true,
}

// operationForValueType maps a template's ValueType onto the engine operation
// that writes it, for the scalar kinds only ("" for every other kind).
func operationForValueType(valueType string) string {
	switch valueType {
	case "Expression":
		return "expression"
	case "TextTemplate":
		return "texttemplate"
	case "Attribute":
		return "attribute"
	case "String", "Integer", "Decimal", "Boolean", "Enumeration":
		return "primitive"
	}
	return ""
}

// scalarPropertyText returns the text property key of w stores under operation
// op, for the authored value raw. A value that is an expression — written as one
// (*ast.WidgetExpressionV3) or parsed as a variable-led data source whose source
// text the visitor kept — is the expression's text for an Expression property
// and an error for every other scalar kind.
func scalarPropertyText(w *ast.WidgetV3, key string, raw any, op string) (string, error) {
	if !scalarOperations[op] {
		return stringifyAny(raw), nil
	}
	switch v := raw.(type) {
	case *ast.WidgetExpressionV3:
		if op == "expression" {
			return v.Text, nil
		}
		if text, ok := parameterAttributeValue(v); ok && op == "attribute" {
			return text, nil // `$Param.Attr`, not an expression
		}
		return "", expressionInScalarPropertyError(w, key, op, v.Text)
	case *ast.DataSourceV3:
		// `$Name/Assoc/Attr` parses as a data source; on an attribute property
		// it is a rooted attribute path, resolved by the engine (ako/mxcli#2107).
		if root, path, rooted := rootedAttributeValue(v); rooted && op == "attribute" {
			return "$" + root + "/" + path, nil
		}
		src, hasSrc := w.ExpressionSource(key)
		if op == "expression" && hasSrc {
			return src, nil
		}
		if !hasSrc {
			src = formatDataSourceV3(v)
		}
		if op == "expression" {
			return "", fmt.Errorf("widget `%s` property `%s` takes an expression, not the data source %s",
				widgetLabelForError(w), key, src)
		}
		return "", expressionInScalarPropertyError(w, key, op, src)
	}
	return stringifyAny(raw), nil
}

func expressionInScalarPropertyError(w *ast.WidgetV3, key, op, text string) error {
	return fmt.Errorf("widget `%s` property `%s` is a %s property and cannot hold the expression %s — "+
		"only an Expression-typed property takes one (MDL-WIDGET42)",
		widgetLabelForError(w), key, operationNoun(op), text)
}

func operationNoun(op string) string {
	switch op {
	case "texttemplate":
		return "text template"
	case "primitive":
		return "plain-value"
	}
	return op
}

func widgetLabelForError(w *ast.WidgetV3) string {
	if w == nil {
		return ""
	}
	if w.Name != "" {
		return w.Name
	}
	return strings.ToLower(w.Type)
}
