// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/backend/widgetobj"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
)

// ALTER … SET on a pluggable widget's attribute property (ako/mxcli#1622).
//
// Which entity a bare attribute name belongs to is not a fact of the BSON: it
// is the widget's binding rule (the executor's resolveBindingScope) applied to
// the property's datasource link and the scopes around the widget. So the
// mutator does not bind; it reports what the rule needs — PluggablePropertyScope
// — and writes what the executor resolved — a backend.AttributeBinding.

// PluggablePropertyScope describes propKey of the pluggable widget widgetRef:
// its kind, the widget's own datasource it is linked to, and the scope of that
// datasource and of the nearest enclosing one. The key is matched
// case-insensitively, as the setter matches it (#1069).
func (m *Mutator) PluggablePropertyScope(widgetRef, propKey string) (backend.PluggablePropertyScope, bool) {
	result := m.widgetFinder(m.rawData, widgetRef)
	if result == nil || len(result.colPropKeys) > 0 ||
		bsonnav.DGetString(result.widget, "$Type") != "CustomWidgets$CustomWidget" {
		return backend.PluggablePropertyScope{}, false
	}
	objType := bsonnav.DGetDoc(bsonnav.DGetDoc(result.widget, "Type"), "ObjectType")
	for _, pt := range bsonnav.DGetArrayElements(bsonnav.DGet(objType, "PropertyTypes")) {
		ptDoc, ok := pt.(bson.D)
		if !ok {
			continue
		}
		key := bsonnav.DGetString(ptDoc, "PropertyKey")
		if key == "" || !strings.EqualFold(key, propKey) {
			continue
		}
		vt := bsonnav.DGetDoc(ptDoc, "ValueType")
		scope := backend.PluggablePropertyScope{
			Key:        key,
			Kind:       bsonnav.DGetString(vt, "Type"),
			DataSource: bsonnav.DGetString(vt, "DataSourceProperty"),
		}
		if scope.Kind == "Enumeration" {
			scope.EnumKeys = pluggableEnumerationKeys(result.widget, bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(ptDoc, "$ID")))
		}
		if scope.DataSource != "" {
			scope.Linked = toSourceScope(resolveSourceScope(m.rawData, pluggablePropertyDataSource(result.widget, scope.DataSource)))
		}
		if ds, ok := findNearestDataSourceDoc(m.rawData, widgetRef); ok {
			scope.Enclosing = toSourceScope(resolveSourceScope(m.rawData, ds))
		}
		return scope, true
	}
	return backend.PluggablePropertyScope{}, false
}

func toSourceScope(s sourceScope) backend.SourceScope {
	return backend.SourceScope{Entity: s.Entity, Microflow: s.Microflow, Nanoflow: s.Nanoflow}
}

// pluggablePropertyDataSource is the DataSource document stored under the
// widget's property dsKey, or nil.
func pluggablePropertyDataSource(widget bson.D, dsKey string) bson.D {
	keys := buildPropKeyMap(widget)
	for _, p := range bsonnav.DGetArrayElements(bsonnav.DGet(bsonnav.DGetDoc(widget, "Object"), "Properties")) {
		pd, ok := p.(bson.D)
		if !ok || keys[bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(pd, "TypePointer"))] != dsKey {
			continue
		}
		return bsonnav.DGetDoc(bsonnav.DGetDoc(pd, "Value"), "DataSource")
	}
	return nil
}

// setAttributeBindingValue writes a resolved attribute binding into an
// Attribute-kind property's WidgetValue: the AttributeRef (with its association
// hops, if any), and the
// SourceVariable naming the parameter it is read from — or null, so a value
// bound back to the context object is no longer read from the parameter.
func setAttributeBindingValue(valDoc bson.D, propName, kind string, b backend.AttributeBinding) error {
	if kind != "Attribute" {
		return fmt.Errorf("pluggable property %q holds a value of kind %s, not an attribute", propName, kind)
	}
	if b.Path == "" {
		return fmt.Errorf("pluggable property %q: no attribute to bind", propName)
	}
	// Both slots are part of every WidgetValue Studio Pro writes. A value
	// without them cannot be completed in place, and a SourceVariable left
	// unwritten would keep reading the old parameter.
	for _, key := range []string{"AttributeRef", "SourceVariable"} {
		if !hasKey(valDoc, key) {
			return fmt.Errorf("pluggable property %q stores no %s to set — rewrite the widget with "+
				"CREATE OR REPLACE PAGE (or ALTER PAGE REPLACE) instead", propName, key)
		}
	}
	bsonnav.DSet(valDoc, "AttributeRef", bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "DomainModels$AttributeRef"},
		{Key: "Attribute", Value: b.Path},
		// The association hops of `Assoc/Attr`, as CREATE writes them
		// (SetAttributeWithSteps, ako/mxcli#2283); nil for a direct attribute.
		{Key: "EntityRef", Value: widgetobj.AttributeEntityRefBSON(b.Steps)},
	})
	var sv any
	if b.SourceVariable != nil && (b.SourceVariable.Widget != "" || b.SourceVariable.Variable != "") {
		sv = widgetobj.PageVariableBSON(b.SourceVariable.Widget, b.SourceVariable.Variable, b.SourceVariable.Kind)
	}
	bsonnav.DSet(valDoc, "SourceVariable", sv)
	return nil
}
