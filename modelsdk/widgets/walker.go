// SPDX-License-Identifier: Apache-2.0

package widgets

import "github.com/JordtenBulte-OLC/mxcli/modelsdk/widgets/mpk"

// findPropertyType finds a PropertyType map by PropertyKey in a PropertyTypes array.
func findPropertyType(propTypes []any, key string) (ptMap map[string]any, ptID string) {
	for _, pt := range propTypes {
		m, ok := pt.(map[string]any)
		if !ok {
			continue
		}
		k, _ := m["PropertyKey"].(string)
		if k == key {
			id, _ := m["$ID"].(string)
			return m, id
		}
	}
	return nil, ""
}

// nestedObjectContext holds the navigation results for a nested ObjectType property:
// the ObjectType's PropertyTypes and the corresponding Object Properties from each
// WidgetObject instance.
type nestedObjectContext struct {
	nestedObjType     map[string]any
	nestedPropTypes   []any
	nestedObjProps    [][]any          // Properties array per WidgetObject
	objPropContainers []map[string]any // WidgetObject containers (for write-back)
}

// resolveNestedObjectContext navigates from a matched PropertyType into its
// ValueType.ObjectType.PropertyTypes and finds the corresponding nested
// Object Properties via TypePointer matching.
func resolveNestedObjectContext(matchedPT map[string]any, matchedPTID string, objProps []any) *nestedObjectContext {
	vt, ok := getMapField(matchedPT, "ValueType")
	if !ok {
		return nil
	}
	nestedObjType, ok := getMapField(vt, "ObjectType")
	if !ok || nestedObjType == nil {
		return nil
	}
	nestedPropTypes, ok := getArrayField(nestedObjType, "PropertyTypes")
	if !ok {
		return nil
	}

	ctx := &nestedObjectContext{
		nestedObjType:   nestedObjType,
		nestedPropTypes: nestedPropTypes,
	}

	// Find the corresponding Object WidgetProperty and its nested WidgetObjects
	for _, prop := range objProps {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		tp, _ := propMap["TypePointer"].(string)
		if tp != matchedPTID {
			continue
		}
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
			props, ok := getArrayField(objMap, "Properties")
			if ok {
				ctx.nestedObjProps = append(ctx.nestedObjProps, props)
				ctx.objPropContainers = append(ctx.objPropContainers, objMap)
			}
		}
		break
	}

	return ctx
}

// buildExemplarIndex scans PropertyTypes and returns:
// - existingKeys: set of PropertyKeys present
// - exemplars: map from ValueType.Type to the first index having that type
func buildExemplarIndex(propTypes []any) (existingKeys map[string]bool, exemplars map[string]int) {
	existingKeys = make(map[string]bool)
	exemplars = make(map[string]int)
	for i, npt := range propTypes {
		nptMap, ok := npt.(map[string]any)
		if !ok {
			continue
		}
		key, _ := nptMap["PropertyKey"].(string)
		if key == "" {
			continue
		}
		existingKeys[key] = true

		nvt, ok := getMapField(nptMap, "ValueType")
		if ok {
			vtType, _ := nvt["Type"].(string)
			if vtType != "" {
				if _, exists := exemplars[vtType]; !exists {
					exemplars[vtType] = i
				}
			}
		}
	}
	return
}

// diffPropertyKeys computes the missing and stale property keys between an mpk
// definition's children and the existing PropertyType keys.
func diffPropertyKeys(mpkChildren []mpk.PropertyDef, existingKeys map[string]bool) (missing []mpk.PropertyDef, staleKeys []string) {
	mpkChildKeys := make(map[string]bool, len(mpkChildren))
	for _, child := range mpkChildren {
		mpkChildKeys[child.Key] = true
		if !existingKeys[child.Key] {
			missing = append(missing, child)
		}
	}
	for key := range existingKeys {
		if !mpkChildKeys[key] {
			staleKeys = append(staleKeys, key)
		}
	}
	return
}

// removeNestedProperties removes stale PropertyTypes and their corresponding
// Properties from nested ObjectType structures. Returns updated slices.
func removeNestedProperties(nestedPropTypes []any, nestedObjProps [][]any, staleKeys []string) ([]any, [][]any) {
	staleSet := make(map[string]bool, len(staleKeys))
	for _, key := range staleKeys {
		staleSet[key] = true
	}

	// Collect IDs of stale PropertyTypes
	staleIDs := make(map[string]bool)
	var filteredPropTypes []any
	for _, npt := range nestedPropTypes {
		nptMap, ok := npt.(map[string]any)
		if !ok {
			filteredPropTypes = append(filteredPropTypes, npt)
			continue
		}
		key, _ := nptMap["PropertyKey"].(string)
		if staleSet[key] {
			id, _ := nptMap["$ID"].(string)
			if id != "" {
				staleIDs[id] = true
			}
			continue
		}
		filteredPropTypes = append(filteredPropTypes, npt)
	}

	// Remove matching Properties from each WidgetObject
	for i, nop := range nestedObjProps {
		var filtered []any
		for _, prop := range nop {
			propMap, ok := prop.(map[string]any)
			if !ok {
				filtered = append(filtered, prop)
				continue
			}
			tp, _ := propMap["TypePointer"].(string)
			if staleIDs[tp] {
				continue
			}
			filtered = append(filtered, prop)
		}
		nestedObjProps[i] = filtered
	}

	return filteredPropTypes, nestedObjProps
}
