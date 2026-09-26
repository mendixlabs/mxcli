// SPDX-License-Identifier: Apache-2.0

package widgets

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

// AugmentTemplate modifies a template's Type and Object in-place to match an .mpk definition.
// It adds PropertyTypes (in Type) and Properties (in Object) for keys present in .mpk but
// missing from the template, and removes those present in the template but missing from .mpk.
// Only regular properties are compared (not system properties like Label, Visibility, Editability).
func AugmentTemplate(tmpl *WidgetTemplate, def *mpk.WidgetDefinition) error {
	if tmpl == nil || def == nil {
		return nil
	}

	// Get PropertyTypes array from Type.ObjectType.PropertyTypes
	objType, ok := getMapField(tmpl.Type, "ObjectType")
	if !ok {
		return nil
	}
	propTypes, ok := getArrayField(objType, "PropertyTypes")
	if !ok {
		return nil
	}

	// Get Properties array from Object.Properties
	objProps, ok := getArrayField(tmpl.Object, "Properties")
	if !ok {
		return nil
	}

	// Build set of existing template property keys (non-system only)
	templateKeys := make(map[string]bool)
	// Also build a map of XML type -> exemplar index for cloning
	typeExemplars := make(map[string]int) // ValueType.Type -> index in propTypes
	systemKeys := def.SystemPropertyKeys()

	for i, pt := range propTypes {
		ptMap, ok := pt.(map[string]any)
		if !ok {
			continue
		}
		key, _ := ptMap["PropertyKey"].(string)
		if key == "" {
			continue
		}
		// Skip system properties
		if systemKeys[key] {
			continue
		}
		templateKeys[key] = true

		// Record exemplar for this value type
		vt, ok := getMapField(ptMap, "ValueType")
		if ok {
			vtType, _ := vt["Type"].(string)
			if vtType != "" {
				if _, exists := typeExemplars[vtType]; !exists {
					typeExemplars[vtType] = i
				}
			}
		}
	}

	// Determine mpk property keys (regular only)
	mpkKeys := def.PropertyKeys()

	// Find missing keys (in mpk but not in template)
	var missing []mpk.PropertyDef
	for _, p := range def.Properties {
		if !templateKeys[p.Key] {
			missing = append(missing, p)
		}
	}

	// Find stale keys (in template but not in mpk, excluding system props)
	var stale []string
	for key := range templateKeys {
		if !mpkKeys[key] && !systemKeys[key] {
			stale = append(stale, key)
		}
	}

	// Check if nested augmentation is needed (skip early return if so)
	hasNestedChildren := false
	for _, p := range def.Properties {
		if len(p.Children) > 0 {
			hasNestedChildren = true
			break
		}
	}

	// The add/remove work below is only needed when the property SET differs.
	// The value-level reconciliation that FOLLOWS this block is not: a template
	// whose keys already match the installed package can still carry stale values
	// (Required, AllowUpload, enum options, defaults, order). Returning early here
	// skipped all six of those passes and was the #716 dropdown-filter bug — its
	// property set is byte-for-byte in sync with Data Widgets 3.10, so augmentation
	// silently did nothing at all.
	if len(missing) > 0 || len(stale) > 0 || hasNestedChildren {
		// Remove stale properties
		if len(stale) > 0 {
			staleSet := make(map[string]bool, len(stale))
			for _, key := range stale {
				staleSet[key] = true
			}
			propTypes, objProps = removeProperties(propTypes, objProps, staleSet)
		}

		// Create a cloner for property pair deep-cloning
		cloner := defaultCloner()

		// Add missing properties
		for _, p := range missing {
			bsonType := xmlTypeToBSONType(p.Type)
			if bsonType == "" {
				continue // Unknown type, skip
			}

			// Find an exemplar of the same type to clone
			exemplarIdx, hasExemplar := typeExemplars[bsonType]
			var newPropType, newProp map[string]any
			if hasExemplar {
				var err error
				newPropType, newProp, err = cloner.ClonePair(propTypes, objProps, exemplarIdx, p)
				if err != nil {
					return fmt.Errorf("augment %s: %w", tmpl.WidgetID, err)
				}
			}
			// Fall back to createPropertyPair if cloning failed (no exemplar or no matching property)
			if newPropType == nil || newProp == nil {
				newPropType, newProp = createPropertyPair(p, bsonType)
			}

			if newPropType != nil {
				propTypes = append(propTypes, newPropType)
			}
			if newProp != nil {
				objProps = append(objProps, newProp)
			}
		}

		// Write back top-level
		setArrayField(objType, "PropertyTypes", propTypes)
		setArrayField(tmpl.Object, "Properties", objProps)

		// Augment nested ObjectType properties (e.g., DataGrid2 column properties).
		// Top-level augmentation syncs the property list, but nested ObjectTypes inside
		// IsList Object properties also need syncing when the .mpk version differs
		// from the template version.
		for _, mpkProp := range def.Properties {
			if len(mpkProp.Children) == 0 {
				continue
			}
			if err := augmentNestedObjectType(propTypes, objProps, mpkProp); err != nil {
				return fmt.Errorf("augment nested %s: %w", mpkProp.Key, err)
			}
		}
	}

	// Reconcile enumeration values of existing properties against the .mpk. The
	// augmentation above syncs property KEYS, but an enum property's option SET can
	// change between the embedded-template's widget version and the installed one
	// (e.g. Gallery pagingPosition {top,bottom,both} → {above,below} between 3.4.0
	// and 3.0.1). A stale option in the embedded Type that the installed widget
	// doesn't define triggers CE0463 ("definition has changed"). The .mpk is
	// authoritative, so overwrite each enum PropertyType's option set from it.
	reconcileEnumValues(tmpl.Type, mpkEnumValuesByKey(def))

	// Reconcile per-property metadata (Category, Caption) and the DefaultValue of
	// existing PropertyTypes against the .mpk. reconcileEnumValues above rebuilds an
	// enum's OPTION SET, but leaves a stale DefaultValue: e.g. Gallery pagingPosition's
	// options reconcile to {below,above} while its default stays "bottom" — a value
	// the installed widget no longer defines, so the PropertyType is inconsistent →
	// CE0463. Category likewise drifts across widget versions (e.g.
	// "General::Pagination" → "General::Items" on Data Widgets 3.x). The .mpk is
	// authoritative for a freshly-created instance, so overwrite these from it. This is
	// the within-key definition drift behind the marketplace-updated-widget CE0463
	// (issue #600 / Gallery@10.24) — augment previously reconciled key presence and
	// enum options but not the rest of each matched PropertyType's definition.
	reconcilePropertyMetadata(tmpl.Type, mpkPropDefsByKey(def))

	// Reconcile the schema-derived scalar fields of each matched PropertyType's ValueType
	// against the .mpk — Type, Required, DefaultValue, AllowedTypes, IsList,
	// DataSourceProperty — and, where the Type changes, reset the corresponding Object
	// WidgetValue so it stays consistent with the schema. This closes the large-version
	// -jump within-key drift behind issue #600 (DataGrid2 11.6-era template → Data Widgets
	// 3.10.0). update-widgets is fully generic (it has no widget-specific knowledge), so
	// everything it produces is derivable from the widget package + the generic metamodel
	// defaults — nothing here is hardcoded per widget. Confirmed empirically against the
	// #600 stack: after this reconciliation the Type/DefaultValue drift reconciles to zero
	// and Required matches the spec default (absent→true, fixed in the mpk parser).
	reconcileValueTypesFromMPK(tmpl, mpkPropDefsByKey(def))

	// Emit the generic WidgetValueType envelope fields Studio Pro's current metamodel
	// always serializes but that an older extracted template predates — today AllowUpload
	// (default false, present on every ValueType in mxbuild output). These are generic
	// metamodel defaults, not widget-specific, so they are added to every ValueType that
	// lacks them regardless of key. Missing envelope fields are a within-key definition
	// mismatch → CE0463 on large version jumps (issue #600: DW 3.10.0 emits AllowUpload
	// on all 105 DataGrid2 ValueTypes; the 11.6-era template has none).
	completeValueTypeEnvelope(tmpl.Type)

	// Reorder the top-level PropertyTypes to match the installed .mpk's declaration
	// order. augment above adds/removes/reconciles by KEY but leaves the template's
	// original order; when the installed widget reordered its properties across
	// versions (e.g. Gallery 3.x moved pagingPosition ahead of showTotalCount), the
	// emitted Type's PropertyType order ≠ the installed widget → CE0463. Unlike the
	// WidgetObject's Properties order (which Studio Pro tolerates — see the spike),
	// the WidgetType's PropertyType order is checked. Object↔Type references are by
	// $ID, so reordering the Type list is safe. The .mpk is authoritative.
	reorderPropertyTypes(tmpl.Type, def)

	// A surviving property's own definition attributes are version-specific too;
	// reconciling only the property SET leaves CE0463 unexplained (#716).
	syncDefinitionAttrs(propTypes, def.Properties)

	return nil
}

// reorderPropertyTypes reorders the top-level ObjectType.PropertyTypes to match the
// installed .mpk's property declaration order. Leading array markers are preserved;
// keys the .mpk does not declare (system/accessibility props absent from
// def.Properties) keep their relative order after the declared ones (stable sort).
func reorderPropertyTypes(tmplType map[string]any, def *mpk.WidgetDefinition) {
	objType, ok := getMapField(tmplType, "ObjectType")
	if !ok {
		return
	}
	// Rank by the full declared order (regular + system properties interleaved) so
	// system properties (Label/Visibility/Editability) land at their .mpk-declared
	// position rather than being pushed to the end. Widgets that declare no inline
	// system properties (DataGrid2, Gallery) have AllTopLevel == the regular list, so
	// this is a no-op for them; ComboBox declares them mid-list, so it matters there.
	order := def.AllTopLevel
	if len(order) == 0 {
		order = def.Properties
	}
	reorderObjectTypePropertyTypes(objType, order)
}

// reorderObjectTypePropertyTypes reorders one ObjectType's PropertyTypes to the given
// .mpk property order, then recurses into each object-list property's nested ObjectType
// using that property's children order. Leading array markers keep their position; keys
// the .mpk does not declare are kept after the declared ones (stable sort). Nested order
// matters as much as top-level: an object-list column whose child PropertyTypes are in the
// template's old order (not the installed widget's) is a within-key definition mismatch →
// CE0463 (issue #600: DataGrid2 3.10.0 reordered/added the column export* properties).
func reorderObjectTypePropertyTypes(objType map[string]any, props []mpk.PropertyDef) {
	propTypes, ok := getArrayField(objType, "PropertyTypes")
	if !ok {
		return
	}
	order := make(map[string]int, len(props))
	childrenByKey := make(map[string][]mpk.PropertyDef)
	for i, p := range props {
		order[p.Key] = i
		if len(p.Children) > 0 {
			childrenByKey[p.Key] = p.Children
		}
	}
	rank := func(pt any) int {
		m, ok := pt.(map[string]any)
		if !ok {
			return -1 // markers sort first, keeping their leading position
		}
		key, _ := m["PropertyKey"].(string)
		if pos, ok := order[key]; ok {
			return pos
		}
		return 1 << 30 // not declared by the .mpk (system/accessibility): keep after declared
	}
	sort.SliceStable(propTypes, func(i, j int) bool {
		return rank(propTypes[i]) < rank(propTypes[j])
	})
	setArrayField(objType, "PropertyTypes", propTypes)

	// Recurse into nested ObjectTypes of object-list properties.
	for _, pt := range propTypes {
		m, ok := pt.(map[string]any)
		if !ok {
			continue
		}
		key, _ := m["PropertyKey"].(string)
		kids, ok := childrenByKey[key]
		if !ok {
			continue
		}
		vt, ok := getMapField(m, "ValueType")
		if !ok {
			continue
		}
		nestedObjType, ok := getMapField(vt, "ObjectType")
		if !ok {
			continue
		}
		reorderObjectTypePropertyTypes(nestedObjType, kids)
	}
}

// mpkPropDefsByKey indexes a widget's PropertyDefs by key, across both top-level and
// nested (object-list) properties.
func mpkPropDefsByKey(def *mpk.WidgetDefinition) map[string]mpk.PropertyDef {
	out := map[string]mpk.PropertyDef{}
	var add func([]mpk.PropertyDef)
	add = func(props []mpk.PropertyDef) {
		for _, p := range props {
			out[p.Key] = p
			if len(p.Children) > 0 {
				add(p.Children)
			}
		}
	}
	add(def.Properties)
	return out
}

// reconcilePropertyMetadata walks a widget Type and, for every PropertyType whose key
// the installed .mpk defines, overwrites its Category, Caption, and ValueType
// DefaultValue from the .mpk so the emitted definition matches the installed widget.
// Only non-empty .mpk values are applied (the .mpk always carries a category/caption;
// DefaultValue is present for enumeration/boolean/integer types).
func reconcilePropertyMetadata(node any, byKey map[string]mpk.PropertyDef) {
	switch v := node.(type) {
	case map[string]any:
		if v["$Type"] == "CustomWidgets$WidgetPropertyType" {
			if key, _ := v["PropertyKey"].(string); key != "" {
				if pd, ok := byKey[key]; ok {
					if pd.Category != "" {
						v["Category"] = pd.Category
					}
					if pd.Caption != "" {
						v["Caption"] = pd.Caption
					}
					if pd.Description != "" {
						v["Description"] = pd.Description
					}
					if pd.DefaultValue != "" {
						if vt, ok := v["ValueType"].(map[string]any); ok {
							vt["DefaultValue"] = pd.DefaultValue
						}
					}
				}
			}
		}
		for _, val := range v {
			reconcilePropertyMetadata(val, byKey)
		}
	case []any:
		for _, item := range v {
			reconcilePropertyMetadata(item, byKey)
		}
	}
}

// reconcileValueTypesFromMPK overwrites the schema-derived scalar fields of every
// matched PropertyType's ValueType from the .mpk (Type, Required, DefaultValue,
// AllowedTypes, IsList, DataSourceProperty), and — because an in-place Type change would
// otherwise leave the Object's WidgetValue shaped for the old type (the Object↔schema
// inconsistency that triggers CE0463) — resets each affected Object WidgetValue.
//
// The .mpk is the authoritative schema for a freshly-created instance; mxbuild's generic
// update-widgets derives exactly these values from the same package. DefaultValue is set
// unconditionally (including to "" when the .mpk omits it) so a stale template default —
// e.g. a value the installed widget no longer defines — is cleared, not preserved.
func reconcileValueTypesFromMPK(tmpl *WidgetTemplate, byKey map[string]mpk.PropertyDef) {
	// PropertyType $IDs whose Type changed → their Object WidgetValue must be reset.
	changedTypeIDs := map[string]mpk.PropertyDef{}

	var walk func(any)
	walk = func(node any) {
		switch v := node.(type) {
		case map[string]any:
			if v["$Type"] == "CustomWidgets$WidgetPropertyType" {
				if key, _ := v["PropertyKey"].(string); key != "" {
					if pd, ok := byKey[key]; ok {
						if vt, ok := v["ValueType"].(map[string]any); ok {
							bsonType := xmlTypeToBSONType(pd.Type)
							if bsonType != "" {
								if old, _ := vt["Type"].(string); old != bsonType {
									if id, _ := v["$ID"].(string); id != "" {
										changedTypeIDs[id] = pd
									}
								}
								vt["Type"] = bsonType
							}
							vt["Required"] = pd.Required
							vt["IsList"] = pd.IsList
							vt["Multiline"] = pd.Multiline
							vt["DefaultValue"] = pd.DefaultValue
							vt["AllowedTypes"] = buildAllowedTypesArray(pd.AllowedTypes)
							vt["SelectionTypes"] = buildSelectionTypesArray(pd.SelectionTypes)
							vt["DataSourceProperty"] = pd.DataSource
							// Normalize the mutually-exclusive type-specific fields to the
							// authoritative .mpk type. A ValueType cloned from a wrong-typed
							// exemplar (or whose Type changed across widget versions) otherwise
							// keeps stale fields that don't apply to its current type — e.g. a
							// TextTemplate carrying EnumerationValues from an Enumeration
							// exemplar, or a Widgets property carrying a cloned ReturnType.
							// mxbuild emits these empty for the non-matching type, so a
							// leftover is a within-key definition mismatch → CE0463.
							switch bsonType {
							case "Enumeration":
								vt["EnumerationValues"] = buildEnumValuesArray(pd.EnumValues)
								vt["ReturnType"] = nil
							case "Expression":
								vt["EnumerationValues"] = []any{float64(2)}
								if pd.ReturnType != "" || pd.ReturnTypeAssignableTo != "" {
									vt["ReturnType"] = buildReturnType(pd.ReturnType, pd.ReturnTypeAssignableTo)
								} else {
									vt["ReturnType"] = nil
								}
							default:
								vt["EnumerationValues"] = []any{float64(2)}
								vt["ReturnType"] = nil
							}
							// Widget-shipped caption/template translations (from the .mpk
							// <translations>), emitted into the definition as a
							// WidgetTranslation list. Absent here → CE0463 on widgets that
							// ship localized captions (DataGrid2 nl_NL).
							if len(pd.Translations) > 0 {
								vt["Translations"] = buildTranslationsArray(pd.Translations)
							}
							// The package's action variables. A template from an older
							// widget version (or one cloned without them) otherwise keeps
							// a list that disagrees with the installed definition — CE0463
							// (#1200). Rewritten only when it differs, so a template that
							// already agrees keeps its entries as they are.
							if !actionVariablesMatch(vt["ActionVariables"], pd.ActionVariables) {
								vt["ActionVariables"] = buildActionVariablesArray(pd.ActionVariables)
							}
						}
					}
				}
			}
			for _, val := range v {
				walk(val)
			}
		case []any:
			for _, item := range v {
				walk(item)
			}
		}
	}
	walk(tmpl.Type)

	if len(changedTypeIDs) == 0 {
		return
	}
	// Reset each Object WidgetValue whose PropertyType's Type changed, so the Object
	// matches the reconciled schema. The WidgetProperty's TypePointer references the
	// PropertyType $ID (IDs are remapped later, so they still match at this stage).
	var resetWalk func(any)
	resetWalk = func(node any) {
		switch v := node.(type) {
		case map[string]any:
			if v["$Type"] == "CustomWidgets$WidgetProperty" {
				if tp, _ := v["TypePointer"].(string); tp != "" {
					if pd, ok := changedTypeIDs[tp]; ok {
						if val, ok := v["Value"].(map[string]any); ok {
							resetPropertyValue(val, pd)
						}
					}
				}
			}
			for _, child := range v {
				resetWalk(child)
			}
		case []any:
			for _, item := range v {
				resetWalk(item)
			}
		}
	}
	resetWalk(tmpl.Object)
}

// completeValueTypeEnvelope walks a widget Type and ensures every WidgetValueType carries
// the generic metamodel envelope fields Studio Pro's current version always serializes but
// an older extracted template may predate. These are widget-agnostic defaults (mxbuild
// emits them on every ValueType regardless of widget), so they are added wherever absent
// and never overwrite an explicit value.
func completeValueTypeEnvelope(node any) {
	switch v := node.(type) {
	case map[string]any:
		if v["$Type"] == "CustomWidgets$WidgetValueType" {
			if _, ok := v["AllowUpload"]; !ok {
				v["AllowUpload"] = false
			}
		}
		for _, val := range v {
			completeValueTypeEnvelope(val)
		}
	case []any:
		for _, item := range v {
			completeValueTypeEnvelope(item)
		}
	}
}

// buildReturnType builds a CustomWidgets$WidgetReturnType from a .mpk expression
// property's declared <returnType type="..."/> or <returnType assignableTo="..."/>.
// When only assignableTo is declared (no concrete type), mxbuild emits Type "None"
// with AssignableTo set. The placeholder $ID is remapped to a fresh UUID by the
// loader's ID phase.
func buildReturnType(typ, assignableTo string) map[string]any {
	if typ == "" {
		typ = "None"
	}
	return map[string]any{
		"$ID":            placeholderID(),
		"$Type":          "CustomWidgets$WidgetReturnType",
		"AssignableTo":   assignableTo,
		"EntityProperty": "",
		"IsList":         false,
		"Type":           typ,
	}
}

// buildTranslationsArray builds a ValueType.Translations list (leading Mendix array
// marker 2 followed by CustomWidgets$WidgetTranslation entries) from a .mpk property's
// declared <translations>. Placeholder $IDs are remapped by the loader's ID phase.
func buildTranslationsArray(trans []mpk.Translation) []any {
	arr := []any{float64(2)}
	for _, t := range trans {
		arr = append(arr, map[string]any{
			"$ID":          placeholderID(),
			"$Type":        "CustomWidgets$WidgetTranslation",
			"LanguageCode": t.Lang,
			"Text":         t.Text,
		})
	}
	return arr
}

// buildActionVariablesArray builds a ValueType.ActionVariables list (leading
// Mendix array marker 2 followed by CustomWidgets$WidgetActionVariable entries)
// from a .mpk action property's declared <actionVariables> — the shape Studio
// Pro stores (templates/mendix-11.6/combobox.json, onChangeFilterInputEvent).
// Caption is a plain string, not a Texts$Text. Placeholder $IDs are remapped by
// the loader's ID phase.
func buildActionVariablesArray(vars []mpk.ActionVariable) []any {
	arr := []any{float64(2)}
	for _, v := range vars {
		arr = append(arr, map[string]any{
			"$ID":     placeholderID(),
			"$Type":   "CustomWidgets$WidgetActionVariable",
			"Caption": v.Caption,
			"Key":     v.Key,
			"Type":    v.Type,
		})
	}
	return arr
}

// actionVariablesMatch reports whether a stored ActionVariables list already
// declares exactly vars, in order — so reconcile leaves a template that agrees
// with its package byte-for-byte instead of re-minting its entries' $IDs.
func actionVariablesMatch(stored any, vars []mpk.ActionVariable) bool {
	arr, ok := stored.([]any)
	if !ok || len(arr) == 0 || len(arr)-1 != len(vars) {
		return false
	}
	for i, v := range vars {
		e, ok := arr[i+1].(map[string]any)
		if !ok || e["Key"] != v.Key || e["Type"] != v.Type || e["Caption"] != v.Caption {
			return false
		}
	}
	return true
}

// buildAllowedTypesArray builds a ValueType.AllowedTypes list (leading Mendix array
// marker 1 followed by the allowed Mendix type names) from a .mpk property's declared
// attributeTypes. Mirrors the construction in createDefaultValueType.
func buildAllowedTypesArray(types []string) []any {
	arr := []any{float64(1)}
	for _, t := range types {
		arr = append(arr, t)
	}
	return arr
}

// buildSelectionTypesArray builds a ValueType.SelectionTypes list (leading Mendix
// array marker 1 followed by the selection type names, e.g. "None"/"Single") from a
// .mpk selection property's declared <selectionType> elements. Empty (marker only)
// when the property declares none.
func buildSelectionTypesArray(names []string) []any {
	arr := []any{float64(1)}
	for _, n := range names {
		arr = append(arr, n)
	}
	return arr
}

// mpkEnumValuesByKey indexes a widget's enumeration option sets by property key,
// across both top-level and nested (object-list) properties.
func mpkEnumValuesByKey(def *mpk.WidgetDefinition) map[string][]mpk.EnumValue {
	out := map[string][]mpk.EnumValue{}
	var add func([]mpk.PropertyDef)
	add = func(props []mpk.PropertyDef) {
		for _, p := range props {
			if len(p.EnumValues) > 0 {
				out[p.Key] = p.EnumValues
			}
			if len(p.Children) > 0 {
				add(p.Children)
			}
		}
	}
	add(def.Properties)
	return out
}

// reconcileEnumValues walks a widget Type and, for every enumeration PropertyType
// whose key has a .mpk option set, rebuilds its ValueType.EnumerationValues from
// the .mpk so the embedded Type's enum members exactly match the installed widget.
func reconcileEnumValues(node any, byKey map[string][]mpk.EnumValue) {
	switch v := node.(type) {
	case map[string]any:
		if v["$Type"] == "CustomWidgets$WidgetPropertyType" {
			if vt, ok := v["ValueType"].(map[string]any); ok && vt["Type"] == "Enumeration" {
				if key, _ := v["PropertyKey"].(string); key != "" {
					if opts, ok := byKey[key]; ok {
						vt["EnumerationValues"] = buildEnumValuesArray(opts)
					}
				}
			}
		}
		for _, val := range v {
			reconcileEnumValues(val, byKey)
		}
	case []any:
		for _, item := range v {
			reconcileEnumValues(item, byKey)
		}
	}
}

// buildEnumValuesArray builds a CustomWidgets$WidgetEnumerationValue list (with the
// leading Mendix array marker) from .mpk enumeration options. Placeholder $IDs are
// remapped to fresh UUIDs by the loader's ID phase.
func buildEnumValuesArray(opts []mpk.EnumValue) []any {
	arr := []any{float64(2)}
	for _, o := range opts {
		arr = append(arr, map[string]any{
			"$ID":     placeholderID(),
			"$Type":   "CustomWidgets$WidgetEnumerationValue",
			"_Key":    o.Key,
			"Caption": o.Caption,
		})
	}
	return arr
}

// augmentNestedObjectType syncs nested ObjectType PropertyTypes for an Object-type property.
// When a .mpk defines children for a property (e.g., DataGrid2 "columns" has showContentAs,
// attribute, content, header, etc.), this function ensures the template's nested ObjectType
// has the same PropertyTypes as the .mpk, adding missing ones and removing stale ones.
func augmentNestedObjectType(propTypes []any, objProps []any, mpkProp mpk.PropertyDef) error {
	matchedPT, matchedPTID := findPropertyType(propTypes, mpkProp.Key)
	if matchedPT == nil {
		return nil
	}

	ctx := resolveNestedObjectContext(matchedPT, matchedPTID, objProps)
	if ctx == nil {
		return nil
	}

	existingKeys, nestedExemplars := buildExemplarIndex(ctx.nestedPropTypes)
	missing, staleKeys := diffPropertyKeys(mpkProp.Children, existingKeys)

	if len(missing) == 0 && len(staleKeys) == 0 {
		return nil
	}

	nestedPropTypes := ctx.nestedPropTypes
	nestedObjProps := ctx.nestedObjProps

	if len(staleKeys) > 0 {
		nestedPropTypes, nestedObjProps = removeNestedProperties(nestedPropTypes, nestedObjProps, staleKeys)
	}

	nestedPropTypes, nestedObjProps, err := addMissingProperties(nestedPropTypes, nestedObjProps, nestedExemplars, missing)
	if err != nil {
		return err
	}

	// Write back
	setArrayField(ctx.nestedObjType, "PropertyTypes", nestedPropTypes)
	for i, container := range ctx.objPropContainers {
		if i < len(nestedObjProps) {
			setArrayField(container, "Properties", nestedObjProps[i])
		}
	}
	return nil
}

// addMissingProperties clones or creates PropertyTypes and Properties for missing keys.
func addMissingProperties(nestedPropTypes []any, nestedObjProps [][]any, exemplars map[string]int, missing []mpk.PropertyDef) ([]any, [][]any, error) {
	cloner := defaultCloner()
	for _, child := range missing {
		bsonType := xmlTypeToBSONType(child.Type)
		if bsonType == "" {
			continue
		}

		exemplarIdx, hasExemplar := exemplars[bsonType]
		var newPropType, newProp map[string]any
		if hasExemplar && len(nestedObjProps) > 0 {
			var err error
			newPropType, newProp, err = cloner.ClonePair(nestedPropTypes, nestedObjProps[0], exemplarIdx, child)
			if err != nil {
				return nil, nil, fmt.Errorf("clone nested property %q: %w", child.Key, err)
			}
		}
		if newPropType == nil || newProp == nil {
			newPropType, newProp = createPropertyPair(child, bsonType)
		}

		if newPropType != nil {
			nestedPropTypes = append(nestedPropTypes, newPropType)
		}
		if newProp != nil {
			for i := range nestedObjProps {
				nestedObjProps[i] = append(nestedObjProps[i], newProp)
			}
		}
	}
	return nestedPropTypes, nestedObjProps, nil
}

// removeProperties removes PropertyTypes and their corresponding Properties by PropertyKey.
func removeProperties(propTypes []any, objProps []any, staleKeys map[string]bool) ([]any, []any) {
	// Collect IDs of PropertyTypes to remove
	removeIDs := make(map[string]bool)
	var newPropTypes []any
	for _, pt := range propTypes {
		ptMap, ok := pt.(map[string]any)
		if !ok {
			newPropTypes = append(newPropTypes, pt) // Keep markers (e.g., float64(2))
			continue
		}
		key, _ := ptMap["PropertyKey"].(string)
		if staleKeys[key] {
			id, _ := ptMap["$ID"].(string)
			if id != "" {
				removeIDs[id] = true
			}
			continue // Skip this PropertyType
		}
		newPropTypes = append(newPropTypes, pt)
	}

	// Remove corresponding Properties whose TypePointer matches a removed PropertyType
	var newObjProps []any
	for _, prop := range objProps {
		propMap, ok := prop.(map[string]any)
		if !ok {
			newObjProps = append(newObjProps, prop) // Keep markers
			continue
		}
		tp, _ := propMap["TypePointer"].(string)
		if removeIDs[tp] {
			continue // Remove this property
		}
		newObjProps = append(newObjProps, prop)
	}

	return newPropTypes, newObjProps
}

// defaultCloner returns a PropertyCloner using the package-level placeholderID generator.
func defaultCloner() *PropertyCloner {
	return NewPropertyCloner(placeholderID)
}

// createPropertyPair creates a new PropertyType/Property pair from scratch.
func createPropertyPair(p mpk.PropertyDef, bsonType string) (map[string]any, map[string]any) {
	ptID := placeholderID()
	vtID := placeholderID()

	// Create PropertyType
	pt := map[string]any{
		"$ID":         ptID,
		"$Type":       "CustomWidgets$WidgetPropertyType",
		"Caption":     p.Caption,
		"Category":    p.Category,
		"Description": p.Description,
		"IsDefault":   false,
		"PropertyKey": p.Key,
		"ValueType":   createDefaultValueType(vtID, bsonType, p),
	}

	// Create Property (WidgetProperty with WidgetValue)
	prop := map[string]any{
		"$ID":         placeholderID(),
		"$Type":       "CustomWidgets$WidgetProperty",
		"TypePointer": ptID,
		"Value":       createDefaultWidgetValue(vtID, bsonType, p),
	}

	return pt, prop
}

// createDefaultValueType creates a default ValueType structure for a given BSON type.
func createDefaultValueType(vtID string, bsonType string, p mpk.PropertyDef) map[string]any {
	// Build AllowedTypes: version marker 1 followed by allowed Mendix type names.
	allowedTypes := []any{float64(1)}
	for _, t := range p.AllowedTypes {
		allowedTypes = append(allowedTypes, t)
	}

	vt := map[string]any{
		"$ID":                         vtID,
		"$Type":                       "CustomWidgets$WidgetValueType",
		"ActionVariables":             buildActionVariablesArray(p.ActionVariables),
		"AllowNonPersistableEntities": false,
		"AllowedTypes":                allowedTypes,
		"AssociationTypes":            []any{float64(1)},
		"DataSourceProperty":          "",
		"DefaultType":                 defaultTypeOf(p),
		"DefaultValue":                p.DefaultValue,
		"EntityProperty":              "",
		"EnumerationValues":           []any{float64(2)},
		"IsLinked":                    false,
		"IsList":                      p.IsList,
		"IsMetaData":                  false,
		"IsPath":                      "No",
		"Multiline":                   false,
		"ObjectType":                  nil,
		"OnChangeProperty":            "",
		"ParameterIsList":             false,
		"PathType":                    "None",
		"Required":                    p.Required,
		"ReturnType":                  nil,
		"SelectableObjectsProperty":   "",
		"SelectionTypes":              []any{float64(1)},
		"SetLabel":                    false,
		"Translations":                []any{float64(2)},
		"Type":                        bsonType,
	}

	if p.DataSource != "" {
		vt["DataSourceProperty"] = p.DataSource
	}

	// Build nested ObjectType for object-type properties with children
	if bsonType == "Object" && len(p.Children) > 0 {
		vt["ObjectType"] = buildNestedObjectType(p.Children)
	}

	return vt
}

// createDefaultWidgetValue creates a default WidgetValue for a given BSON type.
func createDefaultWidgetValue(vtID string, bsonType string, p mpk.PropertyDef) map[string]any {
	val := map[string]any{
		"$ID":               placeholderID(),
		"$Type":             "CustomWidgets$WidgetValue",
		"Action":            createDefaultNoAction(),
		"AttributeRef":      nil,
		"DataSource":        nil,
		"EntityRef":         nil,
		"Expression":        "",
		"Form":              "",
		"Icon":              nil,
		"Image":             "",
		"Microflow":         "",
		"Nanoflow":          "",
		"Objects":           []any{float64(2)},
		"PrimitiveValue":    "",
		"Selection":         "None",
		"SourceVariable":    nil,
		"TextTemplate":      nil,
		"TranslatableValue": nil,
		"TypePointer":       vtID,
		"Widgets":           []any{float64(2)},
		"XPathConstraint":   "",
	}

	// Set type-specific defaults
	switch bsonType {
	case "Boolean":
		if p.DefaultValue != "" {
			val["PrimitiveValue"] = p.DefaultValue
		} else {
			val["PrimitiveValue"] = "false"
		}
	case "Integer":
		if p.DefaultValue != "" {
			val["PrimitiveValue"] = p.DefaultValue
		} else {
			val["PrimitiveValue"] = "0"
		}
	case "Enumeration":
		if p.DefaultValue != "" {
			val["PrimitiveValue"] = p.DefaultValue
		}
	case "TextTemplate":
		// Populate the default template with the widget's shipped caption
		// translations (empty when the .mpk declares none). A required textTemplate
		// left empty fails CE4899 ("property is required"). Templates that the
		// widget's editorConfig.js hides under the current config are nulled
		// afterward by the builder's ApplyPropertyVisibility (#574), so populating
		// unconditionally here is correct: visible→default caption, hidden→null.
		val["TextTemplate"] = createDefaultClientTemplate(p.Translations)
	}

	return val
}

// createDefaultNoAction creates a default Forms$NoAction structure.
func createDefaultNoAction() map[string]any {
	return map[string]any{
		"$ID":                     placeholderID(),
		"$Type":                   "Forms$NoAction",
		"DisabledDuringExecution": true,
	}
}

// createDefaultClientTemplate creates a default Forms$ClientTemplate structure,
// populating the Template's Texts$Text with the property's shipped caption
// translations (Texts$Translation items) — the default caption a freshly-dropped
// widget carries. With no translations the template is present but empty.
func createDefaultClientTemplate(translations []mpk.Translation) map[string]any {
	return map[string]any{
		"$ID":   placeholderID(),
		"$Type": "Forms$ClientTemplate",
		"Fallback": map[string]any{
			"$ID":   placeholderID(),
			"$Type": "Texts$Text",
			"Items": []any{float64(3)},
		},
		"Parameters": []any{float64(2)},
		"Template": map[string]any{
			"$ID":   placeholderID(),
			"$Type": "Texts$Text",
			"Items": buildTextItems(translations),
		},
	}
}

// buildTextItems builds a Texts$Text Items list (leading marker 3 followed by
// Texts$Translation entries) from a property's default caption translations.
func buildTextItems(translations []mpk.Translation) []any {
	arr := []any{float64(3)}
	for _, t := range translations {
		arr = append(arr, map[string]any{
			"$ID":          placeholderID(),
			"$Type":        "Texts$Translation",
			"LanguageCode": t.Lang,
			"Text":         t.Text,
		})
	}
	return arr
}

// resetPropertyValue resets a WidgetValue to defaults for the given property type.
func resetPropertyValue(val map[string]any, p mpk.PropertyDef) {
	bsonType := xmlTypeToBSONType(p.Type)

	// Reset all value fields to defaults
	val["AttributeRef"] = nil
	val["DataSource"] = nil
	val["EntityRef"] = nil
	val["Expression"] = ""
	val["Form"] = ""
	val["Icon"] = nil
	val["Image"] = ""
	val["Microflow"] = ""
	val["Nanoflow"] = ""
	val["Objects"] = []any{float64(2)}
	val["PrimitiveValue"] = ""
	val["Selection"] = "None"
	val["SourceVariable"] = nil
	val["TextTemplate"] = nil
	val["TranslatableValue"] = nil
	val["Widgets"] = []any{float64(2)}
	val["XPathConstraint"] = ""

	// Set type-specific defaults
	switch bsonType {
	case "Boolean":
		if p.DefaultValue != "" {
			val["PrimitiveValue"] = p.DefaultValue
		} else {
			val["PrimitiveValue"] = "false"
		}
	case "Integer":
		if p.DefaultValue != "" {
			val["PrimitiveValue"] = p.DefaultValue
		} else {
			val["PrimitiveValue"] = "0"
		}
	case "Enumeration":
		if p.DefaultValue != "" {
			val["PrimitiveValue"] = p.DefaultValue
		}
	case "TextTemplate":
		val["TextTemplate"] = createDefaultClientTemplate(p.Translations)
	}
}

// xmlTypeToBSONType maps XML property type to BSON ValueType.Type.
func xmlTypeToBSONType(xmlType string) string {
	switch mpk.NormalizeType(xmlType) {
	case "attribute":
		return "Attribute"
	case "expression":
		return "Expression"
	case "textTemplate":
		return "TextTemplate"
	case "widgets":
		return "Widgets"
	case "enumeration":
		return "Enumeration"
	case "boolean":
		return "Boolean"
	case "integer":
		return "Integer"
	case "datasource":
		return "DataSource"
	case "action":
		return "Action"
	case "selection":
		return "Selection"
	case "association":
		return "Association"
	case "object":
		return "Object"
	case "string":
		return "String"
	case "decimal":
		return "Decimal"
	case "icon":
		return "Icon"
	case "image":
		return "Image"
	case "file":
		return "File"
	default:
		return ""
	}
}

// buildNestedObjectType creates a WidgetObjectType with PropertyTypes for nested children
// of an object-type property. This is needed for properties like filterList and sortList
// that contain sub-properties (e.g., filter, attribute, caption).
func buildNestedObjectType(children []mpk.PropertyDef) map[string]any {
	propTypes := []any{float64(2)} // version marker

	for _, child := range children {
		childBsonType := xmlTypeToBSONType(child.Type)
		if childBsonType == "" {
			continue
		}

		childVTID := placeholderID()
		childPT := map[string]any{
			"$ID":         placeholderID(),
			"$Type":       "CustomWidgets$WidgetPropertyType",
			"Caption":     child.Caption,
			"Category":    "General",
			"Description": child.Description,
			"IsDefault":   false,
			"PropertyKey": child.Key,
			"ValueType":   createDefaultValueType(childVTID, childBsonType, child),
		}

		propTypes = append(propTypes, childPT)
	}

	return map[string]any{
		"$ID":           placeholderID(),
		"$Type":         "CustomWidgets$WidgetObjectType",
		"PropertyTypes": propTypes,
	}
}

// --- Helpers ---

// placeholderCounter generates sequential placeholder IDs (atomic for concurrent safety).
var placeholderCounter atomic.Uint32

// placeholderID generates a placeholder hex ID. These will be remapped by collectIDs
// in GetTemplateFullBSON, so exact values don't matter — they just need to be unique
// 32-char hex strings.
func placeholderID() string {
	n := placeholderCounter.Add(1)
	return fmt.Sprintf("aa000000000000000000000000%06x", n)
}

// ResetPlaceholderCounter resets the counter (for testing).
func ResetPlaceholderCounter() {
	placeholderCounter.Store(0)
}

// getMapField gets a nested map field from a JSON map.
func getMapField(m map[string]any, key string) (map[string]any, bool) {
	val, ok := m[key]
	if !ok {
		return nil, false
	}
	nested, ok := val.(map[string]any)
	return nested, ok
}

// getArrayField gets an array field from a JSON map.
func getArrayField(m map[string]any, key string) ([]any, bool) {
	val, ok := m[key]
	if !ok {
		return nil, false
	}
	arr, ok := val.([]any)
	return arr, ok
}

// setArrayField sets an array field in a JSON map.
func setArrayField(m map[string]any, key string, arr []any) {
	m[key] = arr
}

// deepCloneMap deep-clones a map[string]interface{} via JSON round-trip.
func deepCloneMap(m map[string]any) (map[string]any, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("deep clone marshal: %w", err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("deep clone unmarshal: %w", err)
	}
	return result, nil
}

// deepCloneTemplate deep-clones a WidgetTemplate so augmentation doesn't mutate the cache.
func deepCloneTemplate(tmpl *WidgetTemplate) (*WidgetTemplate, error) {
	clone := &WidgetTemplate{
		WidgetID:      tmpl.WidgetID,
		Name:          tmpl.Name,
		Version:       tmpl.Version,
		ExtractedFrom: tmpl.ExtractedFrom,
		Generated:     tmpl.Generated,
		StableIds:     tmpl.StableIds,
	}

	if tmpl.Type != nil {
		var err error
		clone.Type, err = deepCloneMap(tmpl.Type)
		if err != nil {
			return nil, fmt.Errorf("clone template type %s: %w", tmpl.WidgetID, err)
		}
	}
	if tmpl.Object != nil {
		var err error
		clone.Object, err = deepCloneMap(tmpl.Object)
		if err != nil {
			return nil, fmt.Errorf("clone template object %s: %w", tmpl.WidgetID, err)
		}
	}

	return clone, nil
}

// collectNestedPropertyTypeIDs extracts PropertyKey→$ID mappings from a ValueType's ObjectType.
func collectNestedPropertyTypeIDs(vt map[string]any) map[string]string {
	result := make(map[string]string)
	objType, ok := getMapField(vt, "ObjectType")
	if !ok {
		return result
	}
	propTypes, ok := getArrayField(objType, "PropertyTypes")
	if !ok {
		return result
	}
	for _, pt := range propTypes {
		ptMap, ok := pt.(map[string]any)
		if !ok {
			continue
		}
		key, _ := ptMap["PropertyKey"].(string)
		id, _ := ptMap["$ID"].(string)
		if key != "" && id != "" {
			result[key] = id
		}
	}
	return result
}

// collectNestedPropertyTypeIDsByKey extracts PropertyKey→$ID from a rebuilt ObjectType map.
func collectNestedPropertyTypeIDsByKey(objType map[string]any) map[string]string {
	result := make(map[string]string)
	propTypes, ok := getArrayField(objType, "PropertyTypes")
	if !ok {
		return result
	}
	for _, pt := range propTypes {
		ptMap, ok := pt.(map[string]any)
		if !ok {
			continue
		}
		key, _ := ptMap["PropertyKey"].(string)
		id, _ := ptMap["$ID"].(string)
		if key != "" && id != "" {
			result[key] = id
		}
	}
	return result
}

// remapObjectTypePointers walks the Object Properties array and updates TypePointers
// that reference old PropertyType IDs from a rebuilt ObjectType.
func remapObjectTypePointers(objProps []any, idRemap map[string]string) {
	if len(idRemap) == 0 {
		return
	}
	for _, prop := range objProps {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		// Check Value.Objects for nested WidgetObjects with TypePointers
		val, ok := getMapField(propMap, "Value")
		if !ok {
			continue
		}
		objects, ok := getArrayField(val, "Objects")
		if !ok {
			continue
		}
		for _, obj := range objects {
			objMap, ok := obj.(map[string]any)
			if !ok {
				continue
			}
			// Remap the object's nested properties' TypePointers
			nestedProps, ok := getArrayField(objMap, "Properties")
			if !ok {
				continue
			}
			for _, nestedProp := range nestedProps {
				npMap, ok := nestedProp.(map[string]any)
				if !ok {
					continue
				}
				if tp, ok := npMap["TypePointer"].(string); ok {
					if newTP, exists := idRemap[tp]; exists {
						npMap["TypePointer"] = newTP
					}
				}
				// Also remap Value.TypePointer (references ValueType $ID)
				if nestedVal, ok := getMapField(npMap, "Value"); ok {
					if tp, ok := nestedVal["TypePointer"].(string); ok {
						if newTP, exists := idRemap[tp]; exists {
							nestedVal["TypePointer"] = newTP
						}
					}
				}
			}
		}
	}
}

// syncDefinitionAttrs copies the scalar attributes that belong to the widget's
// DEFINITION from the installed .mpk onto the template's PropertyTypes.
//
// AugmentTemplate reconciles which properties exist (adds new ones, removes
// stale ones) but, before mendixlabs/mxcli#716, left a surviving property's own
// attributes at whatever the embedded 11.6-era template captured. Those
// attributes are part of what Mendix compares when deciding whether a widget's
// definition has changed, so a widget package that merely flipped one of them
// produced CE0463 on every instance — with nothing in the property SET to
// explain it.
//
// Observed on Data Widgets 3.10 against the 11.6 templates:
//
//	Gallery                 OnChangeProperty "onConfigurationChange" -> "" (x4)
//	DatagridDropdownFilter  Required         false -> true            (x2)
//
// Only attributes the .mpk actually declares are synced. Anything Mendix derives
// but the XML does not carry is left alone — overwriting it with a zero value
// would trade one definition mismatch for another.
func syncDefinitionAttrs(propTypes []any, props []mpk.PropertyDef) {
	byKey := make(map[string]*mpk.PropertyDef, len(props))
	var index func([]mpk.PropertyDef)
	index = func(ps []mpk.PropertyDef) {
		for i := range ps {
			byKey[ps[i].Key] = &ps[i]
			if len(ps[i].Children) > 0 {
				index(ps[i].Children)
			}
		}
	}
	index(props)

	var walk func([]any)
	walk = func(pts []any) {
		for _, pt := range pts {
			ptMap, ok := pt.(map[string]any)
			if !ok {
				continue
			}
			if key, _ := ptMap["PropertyKey"].(string); key != "" {
				if p := byKey[key]; p != nil {
					// These live on the PropertyType's ValueType, not on the
					// PropertyType itself — targeting the wrong node made this a
					// silent no-op. Verified against `mx update-widgets` output:
					// the differing paths are
					// PropertyTypes[N]/ValueType/{Required,OnChangeProperty}.
					//
					// Update in place only. Adding a key the node does not already
					// carry invents a property this Mendix version may not define —
					// the mendixlabs/mxcli#759 failure shape.
					target := ptMap
					if vt, ok := getMapField(ptMap, "ValueType"); ok {
						target = vt
					}
					if _, ok := target["Required"]; ok {
						target["Required"] = p.Required
					}
					if _, ok := target["OnChangeProperty"]; ok {
						target["OnChangeProperty"] = p.OnChange
					}
				}
			}
			// Object-typed properties nest their own PropertyTypes.
			if vt, ok := getMapField(ptMap, "ValueType"); ok {
				if ot, ok := getMapField(vt, "ObjectType"); ok {
					if nested, ok := getArrayField(ot, "PropertyTypes"); ok {
						walk(nested)
					}
				}
			}
			if ot, ok := getMapField(ptMap, "ObjectType"); ok {
				if nested, ok := getArrayField(ot, "PropertyTypes"); ok {
					walk(nested)
				}
			}
		}
	}
	walk(propTypes)
}

// NewPropertyPair builds the (WidgetPropertyType, WidgetProperty) pair for a property
// an .mpk declares but a widget does not carry, with concrete IDs rather than the
// placeholders the template pipeline remaps later.
//
// It exists so `mxcli widget sync` can add a property to a widget instance ALREADY
// STORED in the model using the same construction the authoring path uses, instead of
// a second implementation that could drift from it. Returns ok=false for an XML type
// with no BSON mapping — the caller must skip rather than invent a shape.
//
// The two halves are bound by TypePointer and must be inserted together; a half-move
// yields a project Mendix cannot load.
func NewPropertyPair(p mpk.PropertyDef, newID func() string) (pt, prop map[string]any, ok bool) {
	bsonType := xmlTypeToBSONType(p.Type)
	if bsonType == "" {
		return nil, nil, false
	}
	pt, prop = createPropertyPair(p, bsonType)
	if pt == nil || prop == nil {
		return nil, nil, false
	}
	// createPropertyPair cross-references the PropertyType and its ValueType from the
	// WidgetProperty, so the placeholders must be remapped consistently across BOTH
	// maps — rewriting them independently would break the pairing.
	remap := map[string]string{}
	collectPlaceholders(pt, remap)
	collectPlaceholders(prop, remap)
	for k := range remap {
		remap[k] = newID()
	}
	return rewriteIDs(pt, remap).(map[string]any), rewriteIDs(prop, remap).(map[string]any), true
}

// collectPlaceholders records every placeholder ID appearing anywhere in the value.
func collectPlaceholders(v any, out map[string]string) {
	switch t := v.(type) {
	case map[string]any:
		for _, val := range t {
			collectPlaceholders(val, out)
		}
	case []any:
		for _, item := range t {
			collectPlaceholders(item, out)
		}
	case string:
		if isPlaceholderID(t) {
			out[t] = ""
		}
	}
}

// rewriteIDs returns a copy with every placeholder replaced by its mapped ID.
func rewriteIDs(v any, remap map[string]string) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = rewriteIDs(val, remap)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = rewriteIDs(item, remap)
		}
		return out
	case string:
		if id, ok := remap[t]; ok && id != "" {
			return id
		}
		return t
	}
	return v
}

// isPlaceholderID matches the "aa" prefix placeholderID mints.
func isPlaceholderID(s string) bool {
	return len(s) == 32 && strings.HasPrefix(s, "aa0000000000000000000000")
}

// defaultTypeOf reports the CustomWidgets$WidgetValueType DefaultType for a
// property: the `defaultType` its widget XML declares, or "None" when it declares
// none. It is the KIND of the default (a nanoflow call, an association) as opposed
// to DefaultValue's identity, and Mendix stores it as part of the widget
// DEFINITION — so writing "None" where the package declares "CallNanoflow" is
// CE0463 on every page carrying the widget, with no other symptom.
//
// Only File Uploader declares any in a stock 11.13.0 app (6 properties; every other
// bundled widget declares 0), which is why the whole widget was unwritable while
// the rest were fine.
func defaultTypeOf(p mpk.PropertyDef) string {
	if p.DefaultType == "" {
		return "None"
	}
	return p.DefaultType
}
