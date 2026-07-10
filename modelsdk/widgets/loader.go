// SPDX-License-Identifier: Apache-2.0

// Package widgets provides embedded widget templates for pluggable widgets.
package widgets

import (
	"bytes"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"log"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

// placeholderBinaryPrefix is the GUID-swapped byte pattern for placeholder IDs.
// Placeholder IDs are "aa000000000000000000000000XXXXXX". After hex decode and GUID
// byte-swap in hexToIDBlob, the first 13 bytes become \x00\x00\x00\xaa followed by
// 9 zero bytes. The last 3 bytes are the counter and vary.
var placeholderBinaryPrefix = []byte{0x00, 0x00, 0x00, 0xaa, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

// placeholderStringPrefix is the ASCII prefix of a placeholder ID that leaked as a string.
const placeholderStringPrefix = "aa000000000000000000000000"

// containsPlaceholderID recursively walks a bson.D checking for leaked placeholder IDs.
// Returns true if any placeholder ID is found as a binary blob or string value.
func containsPlaceholderID(doc bson.D) bool {
	for _, elem := range doc {
		if containsPlaceholderValue(elem.Value) {
			return true
		}
	}
	return false
}

// containsPlaceholderValue checks a single BSON value for placeholder IDs.
func containsPlaceholderValue(val any) bool {
	switch v := val.(type) {
	case []byte:
		if len(v) == 16 && bytes.HasPrefix(v, placeholderBinaryPrefix) {
			return true
		}
	case bson.D:
		return containsPlaceholderID(v)
	case bson.A:
		for _, item := range v {
			if containsPlaceholderValue(item) {
				return true
			}
		}
	case string:
		if len(v) == 32 && strings.HasPrefix(v, placeholderStringPrefix) {
			return true
		}
	}
	return false
}

// sortedMapKeys returns map keys in a deterministic order matching Mendix BSON conventions:
// $ID first, $Type second, then remaining keys alphabetically.
func sortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ki, kj := keys[i], keys[j]
		// $ID always first
		if ki == "$ID" {
			return true
		}
		if kj == "$ID" {
			return false
		}
		// $Type always second
		if ki == "$Type" {
			return true
		}
		if kj == "$Type" {
			return false
		}
		// $ keys before non-$ keys
		iDollar := strings.HasPrefix(ki, "$")
		jDollar := strings.HasPrefix(kj, "$")
		if iDollar != jDollar {
			return iDollar
		}
		return strings.ToLower(ki) < strings.ToLower(kj)
	})
	return keys
}

//go:embed templates/mendix-11.6/*.json
var templateFS embed.FS

// WidgetTemplate represents a loaded widget template.
type WidgetTemplate struct {
	WidgetID      string         `json:"widgetId"`
	Name          string         `json:"name"`
	Version       string         `json:"version"`
	ExtractedFrom string         `json:"extractedFrom"`
	Generated     bool           `json:"-"`         // true if derived from MPK, not from embedded template
	StableIds     bool           `json:"stableIds"` // true = use identity mapping for Type $IDs (CE0463 fix)
	Type          map[string]any `json:"type"`
	Object        map[string]any `json:"object"` // WidgetObject with all property values
}

// templateCache caches loaded templates.
var (
	templateCache     = make(map[string]*WidgetTemplate)
	templateCacheLock sync.RWMutex
)

// generatedCache stores MPK-derived templates for the session lifetime.
// Key: widgetID string. Value: *WidgetTemplate (placeholder IDs, not yet remapped).
var generatedCache sync.Map

// widgetTemplateIndex maps widget IDs to template filenames.
// Built lazily by scanning embedded template JSON files.
var (
	widgetTemplateIndex     map[string]string
	widgetTemplateIndexOnce sync.Once
)

// getWidgetTemplateIndex returns the widget ID → filename mapping,
// built by scanning all embedded JSON templates for their "widgetId" field.
func getWidgetTemplateIndex() map[string]string {
	widgetTemplateIndexOnce.Do(func() {
		widgetTemplateIndex = make(map[string]string)
		entries, err := templateFS.ReadDir("templates/mendix-11.6")
		if err != nil {
			return
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			// Read just enough to extract widgetId
			data, err := templateFS.ReadFile("templates/mendix-11.6/" + entry.Name())
			if err != nil {
				continue
			}
			var header struct {
				WidgetID string         `json:"widgetId"`
				Type     map[string]any `json:"type"`
			}
			if err := json.Unmarshal(data, &header); err != nil {
				continue
			}
			wid := header.WidgetID
			// Fallback: extract WidgetId from type.WidgetId for older templates
			if wid == "" && header.Type != nil {
				if v, ok := header.Type["WidgetId"].(string); ok {
					wid = v
				}
			}
			if wid == "" {
				continue
			}
			widgetTemplateIndex[wid] = entry.Name()
		}
	})
	return widgetTemplateIndex
}

// GetTemplate loads a widget template by widget ID.
// Returns nil if the template is not found.
func GetTemplate(widgetID string) (*WidgetTemplate, error) {
	// Check cache first
	templateCacheLock.RLock()
	if tmpl, ok := templateCache[widgetID]; ok {
		templateCacheLock.RUnlock()
		return tmpl, nil
	}
	templateCacheLock.RUnlock()

	// Find template file from auto-scanned index
	index := getWidgetTemplateIndex()
	filename, ok := index[widgetID]
	if !ok {
		return nil, nil // Not found, not an error
	}

	// Load template
	data, err := templateFS.ReadFile("templates/mendix-11.6/" + filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read template %s: %w", filename, err)
	}

	var tmpl WidgetTemplate
	if err := json.Unmarshal(data, &tmpl); err != nil {
		return nil, fmt.Errorf("failed to parse template %s: %w", filename, err)
	}

	// Cache the template
	templateCacheLock.Lock()
	templateCache[widgetID] = &tmpl
	templateCacheLock.Unlock()

	return &tmpl, nil
}

// getOrGenerateTemplate returns a WidgetTemplate for widgetID. It checks the embedded
// template cache first, then falls back to deriving a template from the project's .mpk
// widget file. Returns nil, nil when the widget is unknown and no MPK is available.
func getOrGenerateTemplate(widgetID, projectPath string) (*WidgetTemplate, error) {
	// 1. Embedded templates (existing path)
	if tmpl, err := GetTemplate(widgetID); err != nil || tmpl != nil {
		return tmpl, err
	}

	// 2. Session cache of previously generated templates
	if cached, ok := generatedCache.Load(widgetID); ok {
		return cached.(*WidgetTemplate), nil
	}

	// 3. Derive from MPK in project/widgets/
	if projectPath == "" {
		return nil, nil
	}
	projectDir := filepath.Dir(projectPath)
	mpkPath, err := mpk.FindMPK(projectDir, widgetID)
	if err != nil {
		return nil, fmt.Errorf("widget %q: scan MPK directory: %w", widgetID, err)
	}
	if mpkPath == "" {
		return nil, nil // no MPK found — caller treats nil as "widget unknown"
	}
	def, err := mpk.ParseMPKForWidget(mpkPath, widgetID)
	if err != nil {
		return nil, fmt.Errorf("widget %q: parse MPK: %w", widgetID, err)
	}
	if def == nil {
		return nil, nil
	}
	tmpl := GenerateFromMPK(def)
	generatedCache.Store(widgetID, tmpl)
	return tmpl, nil
}

// ResetGeneratedCache clears the MPK-derived template cache (for testing).
func ResetGeneratedCache() {
	generatedCache.Range(func(k, _ any) bool {
		generatedCache.Delete(k)
		return true
	})
}

// GetTemplateBSON loads a widget template and converts its type definition to BSON.
// The returned bson.D can be used directly in widget creation.
// IDs in the template are regenerated with new UUIDs while preserving internal references.
// If projectPath is non-empty, the template is augmented from the project's .mpk widget file.
func GetTemplateBSON(widgetID string, idGenerator func() string, projectPath string) (bson.D, map[string]PropertyTypeIDEntry, error) {
	tmpl, err := getOrGenerateTemplate(widgetID, projectPath)
	if err != nil {
		return nil, nil, err
	}
	if tmpl == nil {
		return nil, nil, nil
	}

	// Deep-clone and augment from .mpk (skip for generated templates — already complete)
	if !tmpl.Generated {
		tmpl = augmentFromMPK(tmpl, widgetID, projectPath)
	}

	// Phase 1: Collect all $ID values and create old->new ID mappings
	idMapping := make(map[string]string)
	collectIDs(tmpl.Type, idGenerator, idMapping)

	// Phase 2: Convert JSON to BSON, replacing IDs using the mapping
	propertyTypeIDs := make(map[string]PropertyTypeIDEntry)
	bsonType := jsonToBSONWithMapping(tmpl.Type, idMapping, propertyTypeIDs)

	if containsPlaceholderID(bsonType) {
		return nil, nil, fmt.Errorf("placeholder ID leak detected in widget template type for %s: aa000000-prefix ID was not remapped", widgetID)
	}

	return bsonType, propertyTypeIDs, nil
}

// GetTemplateFullBSON loads a widget template and converts both Type and Object to BSON.
// The returned bson.D values can be used directly in widget creation.
// IDs in the template are regenerated with new UUIDs while preserving internal references.
// If projectPath is non-empty, the template is augmented from the project's .mpk widget file.
// Returns: (clonedType, clonedObject, propertyTypeIDs, objectTypeID, stableIds, error)
func GetTemplateFullBSON(widgetID string, idGenerator func() string, projectPath string) (bson.D, bson.D, map[string]PropertyTypeIDEntry, string, bool, error) {
	tmpl, err := getOrGenerateTemplate(widgetID, projectPath)
	if err != nil {
		return nil, nil, nil, "", false, err
	}
	if tmpl == nil {
		return nil, nil, nil, "", false, nil
	}

	// Deep-clone and augment from .mpk (skip for generated templates — already complete)
	if !tmpl.Generated {
		tmpl = augmentFromMPK(tmpl, widgetID, projectPath)
	}
	stableIds := false

	// Phase 1: Collect all $ID values from Type and create old->new ID mappings
	idMapping := make(map[string]string)
	collectIDs(tmpl.Type, idGenerator, idMapping)

	// Also collect IDs from Object
	if tmpl.Object != nil {
		collectIDs(tmpl.Object, idGenerator, idMapping)
	}

	// Phase 2: Convert Type JSON to BSON, replacing IDs using the mapping
	propertyTypeIDs := make(map[string]PropertyTypeIDEntry)
	var objectTypeID string
	bsonType := jsonToBSONWithMappingAndObjectType(tmpl.Type, idMapping, propertyTypeIDs, &objectTypeID)

	// Phase 3: Convert Object JSON to BSON, replacing IDs using the mapping
	var bsonObject bson.D
	if tmpl.Object != nil {
		bsonObject = jsonToBSONObjectWithMapping(tmpl.Object, idMapping)
	}

	if containsPlaceholderID(bsonType) {
		return nil, nil, nil, "", false, fmt.Errorf("placeholder ID leak detected in widget template type for %s: aa000000-prefix ID was not remapped", widgetID)
	}
	if bsonObject != nil && containsPlaceholderID(bsonObject) {
		return nil, nil, nil, "", false, fmt.Errorf("placeholder ID leak detected in widget template object for %s: aa000000-prefix ID was not remapped", widgetID)
	}

	return bsonType, bsonObject, propertyTypeIDs, objectTypeID, stableIds, nil
}

// jsonToBSONWithMappingAndObjectType converts Type JSON to BSON and extracts the ObjectType ID.
func jsonToBSONWithMappingAndObjectType(data map[string]any, idMapping map[string]string, propertyTypeIDs map[string]PropertyTypeIDEntry, objectTypeID *string) bson.D {
	result := make(bson.D, 0, len(data))

	// Track if this is a PropertyType or ObjectType
	var isPropertyType bool
	var isObjectType bool
	var propertyKey string
	var propertyTypeIDVal string
	var valueTypeID string
	var defaultValue string
	var valueType string
	var required bool
	var dataSourceProp string
	var nestedObjectTypeID string
	var nestedPropertyIDs map[string]PropertyTypeIDEntry
	var nestedKeyOrder []string

	// First pass: detect type
	if typeVal, ok := data["$Type"]; ok {
		if typeStr, ok := typeVal.(string); ok {
			if typeStr == "CustomWidgets$WidgetPropertyType" {
				isPropertyType = true
			} else if typeStr == "CustomWidgets$WidgetObjectType" {
				isObjectType = true
			}
		}
	}
	if keyVal, ok := data["PropertyKey"]; ok {
		if keyStr, ok := keyVal.(string); ok {
			propertyKey = keyStr
		}
	}

	// Convert each field in sorted order for deterministic BSON output
	for _, key := range sortedMapKeys(data) {
		val := data[key]
		elem := bson.E{Key: key}

		if key == "$ID" {
			// Convert hex ID to binary, using mapping
			if oldID, ok := val.(string); ok {
				newID := idMapping[oldID]
				if newID == "" {
					newID = oldID
				}
				elem.Value = hexToIDBlob(newID)

				if isPropertyType {
					propertyTypeIDVal = newID
				}
				if isObjectType && objectTypeID != nil {
					*objectTypeID = newID
				}
			}
		} else if key == "ValueType" && isPropertyType {
			// For PropertyTypes, extract ValueType info including nested ObjectType, DefaultValue, Type, Required
			nestedPropertyIDs = make(map[string]PropertyTypeIDEntry)
			elem.Value = jsonValueToBSONWithNestedObjectType(val, idMapping, &valueTypeID, &nestedObjectTypeID, nestedPropertyIDs, &nestedKeyOrder, &defaultValue, &valueType, &required, &dataSourceProp)
		} else {
			elem.Value = jsonValueToBSONWithMappingAndObjectType(val, idMapping, propertyTypeIDs, &valueTypeID, key == "ValueType", objectTypeID)
		}

		result = append(result, elem)
	}

	// Record PropertyType IDs
	if isPropertyType && propertyKey != "" {
		entry := PropertyTypeIDEntry{
			PropertyTypeID:     propertyTypeIDVal,
			ValueTypeID:        valueTypeID,
			DefaultValue:       defaultValue,
			ValueType:          valueType,
			Required:           required,
			DataSourceProperty: dataSourceProp,
		}
		if nestedObjectTypeID != "" {
			entry.ObjectTypeID = nestedObjectTypeID
			entry.NestedPropertyIDs = nestedPropertyIDs
			entry.NestedKeyOrder = nestedKeyOrder
		}
		propertyTypeIDs[propertyKey] = entry
	}

	return result
}

// jsonValueToBSONWithNestedObjectType extracts ValueType info including nested ObjectType, DefaultValue, and Type.
func jsonValueToBSONWithNestedObjectType(val any, idMapping map[string]string, valueTypeID *string, nestedObjectTypeID *string, nestedPropertyIDs map[string]PropertyTypeIDEntry, nestedKeyOrder *[]string, defaultValue *string, valueType *string, required *bool, dataSourceProperty ...*string) any {
	switch v := val.(type) {
	case map[string]any:
		result := make(bson.D, 0, len(v))

		// Extract IDs and metadata from ValueType in sorted order
		for _, key := range sortedMapKeys(v) {
			fieldVal := v[key]
			elem := bson.E{Key: key}

			if key == "$ID" {
				if oldID, ok := fieldVal.(string); ok {
					newID := idMapping[oldID]
					if newID == "" {
						newID = oldID
					}
					elem.Value = hexToIDBlob(newID)
					*valueTypeID = newID
				}
			} else if key == "ObjectType" {
				// Extract nested ObjectType and its PropertyTypes
				elem.Value = extractNestedObjectType(fieldVal, idMapping, nestedObjectTypeID, nestedPropertyIDs, nestedKeyOrder)
			} else if key == "DefaultValue" {
				// Extract default value
				if dv, ok := fieldVal.(string); ok {
					*defaultValue = dv
				}
				elem.Value = jsonValueToBSONSimple(fieldVal, idMapping)
			} else if key == "Type" {
				// Extract value type
				if vt, ok := fieldVal.(string); ok {
					*valueType = vt
				}
				elem.Value = jsonValueToBSONSimple(fieldVal, idMapping)
			} else if key == "Required" {
				// Extract required flag
				if r, ok := fieldVal.(bool); ok {
					*required = r
				}
				elem.Value = jsonValueToBSONSimple(fieldVal, idMapping)
			} else if key == "DataSourceProperty" && len(dataSourceProperty) > 0 && dataSourceProperty[0] != nil {
				// Extract datasource linkage: non-empty when this attribute draws from a sibling datasource property
				if dsp, ok := fieldVal.(string); ok {
					*dataSourceProperty[0] = dsp
				}
				elem.Value = jsonValueToBSONSimple(fieldVal, idMapping)
			} else {
				elem.Value = jsonValueToBSONSimple(fieldVal, idMapping)
			}

			result = append(result, elem)
		}
		return result

	default:
		return jsonValueToBSONSimple(val, idMapping)
	}
}

// extractNestedObjectType extracts ObjectType ID and its PropertyType IDs.
func extractNestedObjectType(val any, idMapping map[string]string, objectTypeID *string, nestedPropertyIDs map[string]PropertyTypeIDEntry, nestedKeyOrder *[]string) any {
	if val == nil {
		return nil
	}

	objType, ok := val.(map[string]any)
	if !ok {
		return jsonValueToBSONSimple(val, idMapping)
	}

	result := make(bson.D, 0, len(objType))

	for _, key := range sortedMapKeys(objType) {
		fieldVal := objType[key]
		elem := bson.E{Key: key}

		if key == "$ID" {
			if oldID, ok := fieldVal.(string); ok {
				newID := idMapping[oldID]
				if newID == "" {
					newID = oldID
				}
				elem.Value = hexToIDBlob(newID)
				*objectTypeID = newID
			}
		} else if key == "PropertyTypes" {
			// Extract PropertyTypes within the nested ObjectType
			elem.Value = extractNestedPropertyTypes(fieldVal, idMapping, nestedPropertyIDs, nestedKeyOrder)
		} else {
			elem.Value = jsonValueToBSONSimple(fieldVal, idMapping)
		}

		result = append(result, elem)
	}

	return result
}

// extractNestedPropertyTypes extracts PropertyType IDs from a nested ObjectType's PropertyTypes array.
func extractNestedPropertyTypes(val any, idMapping map[string]string, nestedPropertyIDs map[string]PropertyTypeIDEntry, nestedKeyOrder *[]string) any {
	arr, ok := val.([]any)
	if !ok {
		return jsonValueToBSONSimple(val, idMapping)
	}

	result := make(bson.A, len(arr))
	for i, item := range arr {
		if propType, ok := item.(map[string]any); ok {
			// Extract property key and IDs
			var propKey, propTypeID, valueTypeID string

			if typeVal, ok := propType["$Type"]; ok {
				if typeStr, ok := typeVal.(string); ok && typeStr == "CustomWidgets$WidgetPropertyType" {
					if keyVal, ok := propType["PropertyKey"]; ok {
						if keyStr, ok := keyVal.(string); ok {
							propKey = keyStr
						}
					}
					if idVal, ok := propType["$ID"]; ok {
						if oldID, ok := idVal.(string); ok {
							newID := idMapping[oldID]
							if newID == "" {
								newID = oldID
							}
							propTypeID = newID
						}
					}
					// Extract ValueType ID
					if vtVal, ok := propType["ValueType"]; ok {
						if vt, ok := vtVal.(map[string]any); ok {
							if vtID, ok := vt["$ID"]; ok {
								if oldID, ok := vtID.(string); ok {
									newID := idMapping[oldID]
									if newID == "" {
										newID = oldID
									}
									valueTypeID = newID
								}
							}
						}
					}

					// Extract DefaultValue, Type, and Required from nested ValueType
					var nestedDefaultValue, nestedValueType string
					var nestedRequired bool
					if vtVal, ok := propType["ValueType"]; ok {
						if vt, ok := vtVal.(map[string]any); ok {
							if dv, ok := vt["DefaultValue"].(string); ok {
								nestedDefaultValue = dv
							}
							if t, ok := vt["Type"].(string); ok {
								nestedValueType = t
							}
							if r, ok := vt["Required"].(bool); ok {
								nestedRequired = r
							}
						}
					}

					// Record the nested property type
					if propKey != "" {
						// Capture template PropertyTypes order (first occurrence) so the
						// object-list item builder emits nested properties in schema order
						// rather than alphabetical — Studio Pro raises CE0463 otherwise.
						if _, exists := nestedPropertyIDs[propKey]; !exists && nestedKeyOrder != nil {
							*nestedKeyOrder = append(*nestedKeyOrder, propKey)
						}
						nestedPropertyIDs[propKey] = PropertyTypeIDEntry{
							PropertyTypeID: propTypeID,
							ValueTypeID:    valueTypeID,
							DefaultValue:   nestedDefaultValue,
							ValueType:      nestedValueType,
							Required:       nestedRequired,
						}
					}
				}
			}

			// Convert the property type to BSON
			result[i] = jsonValueToBSONSimple(item, idMapping)
		} else {
			result[i] = jsonValueToBSONSimple(item, idMapping)
		}
	}

	return result
}

// jsonValueToBSONSimple converts a JSON value to BSON without special tracking.
func jsonValueToBSONSimple(val any, idMapping map[string]string) any {
	switch v := val.(type) {
	case map[string]any:
		result := make(bson.D, 0, len(v))
		for _, key := range sortedMapKeys(v) {
			fieldVal := v[key]
			elem := bson.E{Key: key}
			if key == "$ID" {
				if oldID, ok := fieldVal.(string); ok {
					newID := idMapping[oldID]
					if newID == "" {
						newID = oldID
					}
					elem.Value = hexToIDBlob(newID)
				}
			} else {
				elem.Value = jsonValueToBSONSimple(fieldVal, idMapping)
			}
			result = append(result, elem)
		}
		return result

	case []any:
		arr := make(bson.A, len(v))
		for i, item := range v {
			arr[i] = jsonValueToBSONSimple(item, idMapping)
		}
		return arr

	case string:
		if len(v) == 32 && isHexString(v) {
			if newID, ok := idMapping[v]; ok {
				return hexToIDBlob(newID)
			}
		}
		return v

	case float64:
		if v == float64(int64(v)) {
			return int32(v)
		}
		return v

	default:
		return v
	}
}

// jsonValueToBSONWithMappingAndObjectType converts a JSON value to BSON with ObjectType tracking.
func jsonValueToBSONWithMappingAndObjectType(val any, idMapping map[string]string, propertyTypeIDs map[string]PropertyTypeIDEntry, valueTypeID *string, isValueType bool, objectTypeID *string) any {
	switch v := val.(type) {
	case map[string]any:
		result := jsonToBSONWithMappingAndObjectType(v, idMapping, propertyTypeIDs, objectTypeID)
		if isValueType {
			for _, elem := range result {
				if elem.Key == "$ID" {
					if blob, ok := elem.Value.([]byte); ok {
						*valueTypeID = blobToHex(blob)
					}
				}
			}
		}
		return result

	case []any:
		arr := make(bson.A, len(v))
		for i, item := range v {
			arr[i] = jsonValueToBSONWithMappingAndObjectType(item, idMapping, propertyTypeIDs, valueTypeID, false, objectTypeID)
		}
		return arr

	case string:
		if len(v) == 32 && isHexString(v) {
			if newID, ok := idMapping[v]; ok {
				return hexToIDBlob(newID)
			}
		}
		return v

	case float64:
		if v == float64(int64(v)) {
			return int32(v)
		}
		return v

	default:
		return v
	}
}

// jsonToBSONObjectWithMapping converts Object JSON to BSON with ID mapping for TypePointers.
func jsonToBSONObjectWithMapping(data map[string]any, idMapping map[string]string) bson.D {
	result := make(bson.D, 0, len(data))

	for _, key := range sortedMapKeys(data) {
		val := data[key]
		elem := bson.E{Key: key}

		if key == "$ID" {
			if oldID, ok := val.(string); ok {
				newID := idMapping[oldID]
				if newID == "" {
					newID = oldID
				}
				elem.Value = hexToIDBlob(newID)
			}
		} else if key == "TypePointer" {
			// TypePointer references IDs in the Type - use mapping
			if oldID, ok := val.(string); ok {
				newID := idMapping[oldID]
				if newID == "" {
					newID = oldID
				}
				elem.Value = hexToIDBlob(newID)
			} else {
				elem.Value = val
			}
		} else {
			elem.Value = jsonValueToBSONObjectWithMapping(val, idMapping)
		}

		result = append(result, elem)
	}

	return result
}

// jsonValueToBSONObjectWithMapping converts Object JSON values to BSON.
func jsonValueToBSONObjectWithMapping(val any, idMapping map[string]string) any {
	switch v := val.(type) {
	case map[string]any:
		return jsonToBSONObjectWithMapping(v, idMapping)

	case []any:
		arr := make(bson.A, len(v))
		for i, item := range v {
			arr[i] = jsonValueToBSONObjectWithMapping(item, idMapping)
		}
		return arr

	case string:
		// Check if this looks like an ID reference
		if len(v) == 32 && isHexString(v) {
			if newID, ok := idMapping[v]; ok {
				return hexToIDBlob(newID)
			}
		}
		return v

	case float64:
		if v == float64(int64(v)) {
			return int32(v)
		}
		return v

	default:
		return v
	}
}

// PropertyTypeIDEntry is now defined in mdl/types; re-exported here for compatibility.
type PropertyTypeIDEntry = types.PropertyTypeIDEntry

// collectIDs recursively collects all $ID values and creates old->new mappings.
func collectIDs(data map[string]any, idGenerator func() string, idMapping map[string]string) {
	for key, val := range data {
		if key == "$ID" {
			if oldID, ok := val.(string); ok && len(oldID) == 32 {
				// Generate new ID for this $ID field.
				// Accept any 32-char string (not just valid hex) to handle
				// manually crafted placeholder IDs in templates.
				newID := idGenerator()
				idMapping[oldID] = newID
			}
		}

		// Recurse into nested structures
		switch v := val.(type) {
		case map[string]any:
			collectIDs(v, idGenerator, idMapping)
		case []any:
			collectIDsInArray(v, idGenerator, idMapping)
		}
	}
}

// collectIDsIdentity maps each $ID to itself (identity). Used for non-placeholder
// templates to preserve the original widget type $IDs so Mendix recognises the
// embedded CustomWidgetType as the known installed widget version.
func collectIDsIdentity(data map[string]any, idMapping map[string]string) {
	for key, val := range data {
		if key == "$ID" {
			if id, ok := val.(string); ok && len(id) == 32 {
				idMapping[id] = id // identity: old → same
			}
		}
		switch v := val.(type) {
		case map[string]any:
			collectIDsIdentity(v, idMapping)
		case []any:
			collectIDsIdentityInArray(v, idMapping)
		}
	}
}

func collectIDsIdentityInArray(arr []any, idMapping map[string]string) {
	for _, item := range arr {
		switch v := item.(type) {
		case map[string]any:
			collectIDsIdentity(v, idMapping)
		case []any:
			collectIDsIdentityInArray(v, idMapping)
		}
	}
}

// containsPlaceholderIDInJSON checks whether any $ID in the JSON map is a placeholder.
func containsPlaceholderIDInJSON(data map[string]any) bool {
	for key, val := range data {
		if key == "$ID" {
			if id, ok := val.(string); ok && len(id) == 32 && strings.HasPrefix(id, placeholderStringPrefix) {
				return true
			}
		}
		switch v := val.(type) {
		case map[string]any:
			if containsPlaceholderIDInJSON(v) {
				return true
			}
		case []any:
			if containsPlaceholderIDInJSONArray(v) {
				return true
			}
		}
	}
	return false
}

func containsPlaceholderIDInJSONArray(arr []any) bool {
	for _, item := range arr {
		switch v := item.(type) {
		case map[string]any:
			if containsPlaceholderIDInJSON(v) {
				return true
			}
		case []any:
			if containsPlaceholderIDInJSONArray(v) {
				return true
			}
		}
	}
	return false
}

// collectIDsInArray recursively collects IDs from an array.
func collectIDsInArray(arr []any, idGenerator func() string, idMapping map[string]string) {
	for _, item := range arr {
		switch v := item.(type) {
		case map[string]any:
			collectIDs(v, idGenerator, idMapping)
		case []any:
			collectIDsInArray(v, idGenerator, idMapping)
		}
	}
}

// jsonToBSONWithMapping converts a JSON map to bson.D, replacing IDs using the mapping.
func jsonToBSONWithMapping(data map[string]any, idMapping map[string]string, propertyTypeIDs map[string]PropertyTypeIDEntry) bson.D {
	result := make(bson.D, 0, len(data))

	// Track if this is a PropertyType
	var isPropertyType bool
	var propertyKey string
	var propertyTypeID string
	var valueTypeID string

	// First pass: detect PropertyType and its key
	if typeVal, ok := data["$Type"]; ok {
		if typeStr, ok := typeVal.(string); ok && typeStr == "CustomWidgets$WidgetPropertyType" {
			isPropertyType = true
		}
	}
	if keyVal, ok := data["PropertyKey"]; ok {
		if keyStr, ok := keyVal.(string); ok {
			propertyKey = keyStr
		}
	}

	// Convert each field in sorted order for deterministic BSON output
	for _, key := range sortedMapKeys(data) {
		val := data[key]
		elem := bson.E{Key: key}

		if key == "$ID" {
			// Convert hex ID to binary, using mapping
			if oldID, ok := val.(string); ok {
				newID := idMapping[oldID]
				if newID == "" {
					newID = oldID // Fallback if not in mapping
				}
				elem.Value = hexToIDBlob(newID)

				// Track IDs for PropertyType
				if isPropertyType {
					propertyTypeID = newID
				}
			}
		} else {
			elem.Value = jsonValueToBSONWithMapping(val, idMapping, propertyTypeIDs, &valueTypeID, key == "ValueType")
		}

		result = append(result, elem)
	}

	// Record PropertyType IDs
	if isPropertyType && propertyKey != "" {
		propertyTypeIDs[propertyKey] = PropertyTypeIDEntry{
			PropertyTypeID: propertyTypeID,
			ValueTypeID:    valueTypeID,
		}
	}

	return result
}

// jsonValueToBSONWithMapping converts a JSON value to a BSON value, replacing IDs using mapping.
func jsonValueToBSONWithMapping(val any, idMapping map[string]string, propertyTypeIDs map[string]PropertyTypeIDEntry, valueTypeID *string, isValueType bool) any {
	switch v := val.(type) {
	case map[string]any:
		result := jsonToBSONWithMapping(v, idMapping, propertyTypeIDs)
		// If this is a ValueType, extract its new ID
		if isValueType {
			for _, elem := range result {
				if elem.Key == "$ID" {
					if blob, ok := elem.Value.([]byte); ok {
						*valueTypeID = blobToHex(blob)
					}
				}
			}
		}
		return result

	case []any:
		arr := make(bson.A, len(v))
		for i, item := range v {
			arr[i] = jsonValueToBSONWithMapping(item, idMapping, propertyTypeIDs, valueTypeID, false)
		}
		return arr

	case string:
		// Check if this is a reference to an ID (32 hex chars)
		if len(v) == 32 && isHexString(v) {
			// Look up in mapping - if found, it's an ID reference
			if newID, ok := idMapping[v]; ok {
				return hexToIDBlob(newID)
			}
		}
		return v

	case float64:
		// JSON numbers are float64, convert to int if it's a whole number
		if v == float64(int64(v)) {
			return int32(v)
		}
		return v

	default:
		return v
	}
}

// hexToIDBlob converts a hex string (UUID format) to a binary blob for BSON ID.
// The first 3 segments of the UUID are stored in little-endian format (Microsoft GUID),
// matching the format expected by blobToUUID in sdk/mpr/reader.go.
func hexToIDBlob(hexStr string) []byte {
	// Remove any dashes from UUID format
	hexStr = strings.ReplaceAll(hexStr, "-", "")
	data, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil
	}
	if len(data) != 16 {
		return data
	}

	// Swap bytes to match Microsoft GUID format (little-endian for first 3 segments)
	// This ensures round-trip compatibility: hexToIDBlob -> blobToUUID returns original hex
	// Segment 1 (4 bytes): swap [0,1,2,3] -> [3,2,1,0]
	data[0], data[1], data[2], data[3] = data[3], data[2], data[1], data[0]
	// Segment 2 (2 bytes): swap [4,5] -> [5,4]
	data[4], data[5] = data[5], data[4]
	// Segment 3 (2 bytes): swap [6,7] -> [7,6]
	data[6], data[7] = data[7], data[6]
	// Segments 4 and 5 (2+6 bytes): no swap needed

	return data
}

// blobToHex converts a binary blob to a hex string.
func blobToHex(data []byte) string {
	return hex.EncodeToString(data)
}

// isHexString checks if a string is a valid hex string.
func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// augmentFromMPK deep-clones a template and augments it from the project's .mpk file.
// Returns the original template if augmentation is not applicable or fails.
func augmentFromMPK(tmpl *WidgetTemplate, widgetID string, projectPath string) *WidgetTemplate {
	if projectPath == "" {
		return tmpl
	}

	projectDir := filepath.Dir(projectPath)
	mpkPath, err := mpk.FindMPK(projectDir, widgetID)
	if err != nil || mpkPath == "" {
		return tmpl
	}

	def, err := mpk.ParseMPKForWidget(mpkPath, widgetID)
	if err != nil {
		return tmpl
	}
	if def == nil {
		return tmpl
	}

	// Deep-clone so we don't mutate the cached template
	clone, err := deepCloneTemplate(tmpl)
	if err != nil {
		log.Printf("warning: failed to clone template for %s: %v", widgetID, err)
		return tmpl
	}
	if err := AugmentTemplate(clone, def); err != nil {
		log.Printf("warning: failed to augment template for %s from MPK: %v", widgetID, err)
		return tmpl
	}

	return clone
}

// ListAvailableTemplates returns a list of available widget template IDs.
func ListAvailableTemplates() []string {
	index := getWidgetTemplateIndex()
	result := make([]string, 0, len(index))
	for widgetID := range index {
		result = append(result, widgetID)
	}
	return result
}
