// SPDX-License-Identifier: Apache-2.0

// Package mpr - Raw BSON access methods for Reader.
//
// These methods expose unit contents as raw BSON or typed wrappers from
// mdl/types. They power the BSON-debug surface that mdl/backend/mpr needs
// to delegate at to modelsdk/mpr.Reader instead of sdk/mpr.Reader.
package mpr

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// GetRawUnit retrieves raw BSON data for a unit by ID as a map.
func (r *Reader) GetRawUnit(id model.ID) (map[string]any, error) {
	contents, err := r.GetRawUnitBytes(string(id))
	if err != nil {
		return nil, err
	}
	contents, err = r.resolveContents(string(id), contents)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}
	// bson v2 decodes nested documents/arrays in interface{} slots as bson.D /
	// bson.A, whereas the legacy reader (bson v1) yields nested map[string]any /
	// []any. The raw-BSON consumers (DESCRIBE PAGE's widget-tree walk, snippet
	// variables, etc.) type-assert map[string]any and []any, so normalise the
	// whole tree to the legacy shape — otherwise every nested assertion fails and
	// pages render as empty shells.
	for k, v := range raw {
		raw[k] = normalizeBSONValue(v)
	}
	return raw, nil
}

// normalizeBSONValue recursively converts bson.D → map[string]any and bson.A →
// []any so raw-BSON consumers written against the legacy (bson v1) shape work
// unchanged against the modelsdk (bson v2) reader.
func normalizeBSONValue(v any) any {
	switch t := v.(type) {
	case bson.D:
		m := make(map[string]any, len(t))
		for _, e := range t {
			m[e.Key] = normalizeBSONValue(e.Value)
		}
		return m
	case bson.A:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = normalizeBSONValue(e)
		}
		return out
	case map[string]any:
		for k, e := range t {
			t[k] = normalizeBSONValue(e)
		}
		return t
	case []any:
		for i, e := range t {
			t[i] = normalizeBSONValue(e)
		}
		return t
	case bson.Binary:
		// Reference IDs (entity/attribute/widget refs) are stored as binary. The
		// raw-BSON consumers (extractBinaryID, blobToUUID, …) type-assert the v1
		// primitive.Binary the legacy reader produces, so convert to match — else
		// reference properties (Attribute / DataSource / CaptionAttribute) read empty.
		return primitive.Binary{Subtype: t.Subtype, Data: t.Data}
	default:
		return v
	}
}

// GetUnitTypes returns a count of units by type.
func (r *Reader) GetUnitTypes() (map[string]int, error) {
	units, err := r.listUnitsByType("")
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int)
	for _, u := range units {
		counts[u.Type]++
	}
	return counts, nil
}

// ListRawUnitsByType returns all raw units matching the given BSON type prefix.
// Returns types.RawUnit values keyed off model.ID for cross-package consumers.
func (r *Reader) ListRawUnitsByType(typePrefix string) ([]*types.RawUnit, error) {
	units, err := r.listUnitsByType(typePrefix)
	if err != nil {
		return nil, err
	}
	result := make([]*types.RawUnit, 0, len(units))
	for _, u := range units {
		contents, err := r.resolveContents(u.ID, u.Contents)
		if err != nil {
			continue
		}
		result = append(result, &types.RawUnit{
			ID:          model.ID(u.ID),
			ContainerID: model.ID(u.ContainerID),
			Type:        u.Type,
			Contents:    contents,
		})
	}
	return result, nil
}

// rawUnitBSONType maps a user-friendly object type to a BSON $Type prefix.
// Empty string passes through (list-all). Unsupported types return "" and
// callers should treat that as an error.
func rawUnitBSONType(objectType string) string {
	switch strings.ToLower(objectType) {
	case "page":
		return "Forms$Page"
	case "microflow":
		return "Microflows$Microflow"
	case "nanoflow":
		return "Microflows$Nanoflow"
	case "enumeration":
		return "Enumerations$Enumeration"
	case "snippet":
		return "Forms$Snippet"
	case "layout":
		return "Forms$Layout"
	case "constant":
		return "Constants$Constant"
	case "workflow":
		return "Workflows$Workflow"
	case "imagecollection":
		return "Images$ImageCollection"
	case "javaaction":
		return "JavaActions$JavaAction"
	case "javascriptaction":
		return "JavaScriptActions$JavaScriptAction"
	case "":
		return ""
	default:
		return ""
	}
}

// PERF NOTE (from the misc branch's catalog work — issue #651): this resolves
// by re-reading and re-bson.Unmarshalling every unit of the type on EVERY call.
// `refresh catalog source` calls it once per document (thousands of times), so
// it is O(N²) — it took ~6 hours on a large app. sdk/mpr fixed this by building
// a one-time index keyed by "$Type\x00QualifiedName" → unit, decoding only the
// `Name` field (a small struct, not map[string]any). This engine inherits the
// same O(N²); it should adopt the same name index (the contentCache here helps
// the constant factor but not the algorithm).
//
// GetRawUnitByName returns the raw BSON contents for a unit by qualified
// name. Supported object types match sdk/mpr.Reader.GetRawUnitByName.
// Entity and association names dispatch to the domain-model walker.
func (r *Reader) GetRawUnitByName(objectType, qualifiedName string) (*types.RawUnitInfo, error) {
	switch strings.ToLower(objectType) {
	case "entity":
		return r.getRawEntityByName(qualifiedName)
	case "association":
		return r.getRawAssociationByName(qualifiedName)
	}

	typePrefix := rawUnitBSONType(objectType)
	if typePrefix == "" && objectType != "" {
		return nil, fmt.Errorf("unsupported object type: %s", objectType)
	}

	units, err := r.listUnitsByType(typePrefix)
	if err != nil {
		return nil, err
	}

	moduleMap, containerParent, err := r.buildModuleResolution()
	if err != nil {
		return nil, err
	}

	for _, u := range units {
		contents, err := r.resolveContents(u.ID, u.Contents)
		if err != nil {
			continue
		}
		var raw map[string]any
		if err := bson.Unmarshal(contents, &raw); err != nil {
			continue
		}
		name, _ := raw["Name"].(string)
		moduleName := ResolveModuleName(u.ContainerID, moduleMap, containerParent)

		fullName := name
		if moduleName != "" {
			fullName = moduleName + "." + name
		}
		if fullName == qualifiedName {
			return &types.RawUnitInfo{
				ID:            u.ID,
				QualifiedName: fullName,
				Type:          u.Type,
				ModuleName:    moduleName,
				Contents:      contents,
			}, nil
		}
	}
	return nil, fmt.Errorf("%s not found: %s", objectType, qualifiedName)
}

// ListRawUnits returns all units of the given object type with metadata.
func (r *Reader) ListRawUnits(objectType string) ([]*types.RawUnitInfo, error) {
	typePrefix := rawUnitBSONType(objectType)
	if typePrefix == "" && objectType != "" {
		return nil, fmt.Errorf("unsupported object type: %s", objectType)
	}

	units, err := r.listUnitsByType(typePrefix)
	if err != nil {
		return nil, err
	}

	moduleMap, containerParent, err := r.buildModuleResolution()
	if err != nil {
		return nil, err
	}

	result := make([]*types.RawUnitInfo, 0, len(units))
	for _, u := range units {
		contents, err := r.resolveContents(u.ID, u.Contents)
		if err != nil {
			continue
		}
		var raw map[string]any
		if err := bson.Unmarshal(contents, &raw); err != nil {
			continue
		}
		name, _ := raw["Name"].(string)
		moduleName := ResolveModuleName(u.ContainerID, moduleMap, containerParent)
		fullName := name
		if moduleName != "" {
			fullName = moduleName + "." + name
		}
		result = append(result, &types.RawUnitInfo{
			ID:            u.ID,
			QualifiedName: fullName,
			Type:          u.Type,
			ModuleName:    moduleName,
			Contents:      contents,
		})
	}
	return result, nil
}

// GetRawMicroflowByName returns the raw BSON contents for a microflow by
// qualified name. Convenience wrapper over GetRawUnitByName.
func (r *Reader) GetRawMicroflowByName(qualifiedName string) ([]byte, error) {
	unit, err := r.GetRawUnitByName("microflow", qualifiedName)
	if err != nil {
		return nil, err
	}
	return unit.Contents, nil
}

// buildModuleResolution builds the module-name and container-parent maps used
// by name-lookup helpers, including MPR v2 folder hierarchies.
func (r *Reader) buildModuleResolution() (map[string]string, map[string]string, error) {
	modules, err := r.ListModules()
	if err != nil {
		return nil, nil, err
	}
	moduleMap := make(map[string]string, len(modules))
	for _, m := range modules {
		moduleMap[m.ID] = m.Name
	}
	containerParent, err := r.BuildContainerParent()
	if err != nil {
		return nil, nil, err
	}
	return moduleMap, containerParent, nil
}

// getRawEntityByName finds an entity within domain models and returns its
// inline BSON document (re-marshalled to bytes).
func (r *Reader) getRawEntityByName(qualifiedName string) (*types.RawUnitInfo, error) {
	parts := strings.Split(qualifiedName, ".")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid entity name: %s (expected Module.Entity)", qualifiedName)
	}
	targetModule := parts[0]
	targetEntity := parts[1]

	units, err := r.listUnitsByType("DomainModels$DomainModel")
	if err != nil {
		return nil, err
	}
	moduleMap, _, err := r.buildModuleResolution()
	if err != nil {
		return nil, err
	}

	for _, u := range units {
		moduleName := moduleMap[u.ContainerID]
		if moduleName != targetModule {
			continue
		}
		contents, err := r.resolveContents(u.ID, u.Contents)
		if err != nil {
			continue
		}
		var rawD bson.D
		if err := bson.Unmarshal(contents, &rawD); err != nil {
			continue
		}
		entities := lookupBsonArray(rawD, "Entities")
		// Skip versioned-array marker at index 0.
		for i := 1; i < len(entities); i++ {
			entity, ok := entities[i].(bson.D)
			if !ok {
				continue
			}
			if name, ok := lookupBsonString(entity, "Name"); ok && name == targetEntity {
				entityBytes, err := bson.Marshal(entity)
				if err != nil {
					return nil, err
				}
				return &types.RawUnitInfo{
					ID:            u.ID,
					QualifiedName: qualifiedName,
					Type:          "DomainModels$Entity",
					ModuleName:    moduleName,
					Contents:      entityBytes,
				}, nil
			}
		}
	}
	return nil, fmt.Errorf("entity not found: %s", qualifiedName)
}

// getRawAssociationByName finds an association within domain models.
func (r *Reader) getRawAssociationByName(qualifiedName string) (*types.RawUnitInfo, error) {
	parts := strings.Split(qualifiedName, ".")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid association name: %s (expected Module.AssociationName)", qualifiedName)
	}
	targetModule := parts[0]
	targetAssoc := parts[1]

	units, err := r.listUnitsByType("DomainModels$DomainModel")
	if err != nil {
		return nil, err
	}
	moduleMap, _, err := r.buildModuleResolution()
	if err != nil {
		return nil, err
	}

	for _, u := range units {
		moduleName := moduleMap[u.ContainerID]
		if moduleName != targetModule {
			continue
		}
		contents, err := r.resolveContents(u.ID, u.Contents)
		if err != nil {
			continue
		}
		var rawD bson.D
		if err := bson.Unmarshal(contents, &rawD); err != nil {
			continue
		}
		assocs := lookupBsonArray(rawD, "Associations")
		for i := 1; i < len(assocs); i++ {
			assoc, ok := assocs[i].(bson.D)
			if !ok {
				continue
			}
			if name, ok := lookupBsonString(assoc, "Name"); ok && name == targetAssoc {
				assocBytes, err := bson.Marshal(assoc)
				if err != nil {
					return nil, err
				}
				return &types.RawUnitInfo{
					ID:            u.ID,
					QualifiedName: qualifiedName,
					Type:          "DomainModels$Association",
					ModuleName:    moduleName,
					Contents:      assocBytes,
				}, nil
			}
		}
	}
	return nil, fmt.Errorf("association not found: %s", qualifiedName)
}

// FindViewEntitySourceDocumentID returns the first ViewEntitySourceDocument
// matching the module + document name. Empty ID when not found, mirroring
// sdk/mpr.Reader behavior.
func (r *Reader) FindViewEntitySourceDocumentID(moduleName, docName string) (model.ID, error) {
	units, err := r.listUnitsByType("DomainModels$ViewEntitySourceDocument")
	if err != nil {
		return "", err
	}
	moduleMap, _, err := r.buildModuleResolution()
	if err != nil {
		return "", err
	}
	for _, u := range units {
		contents, err := r.resolveContents(u.ID, u.Contents)
		if err != nil {
			continue
		}
		var raw map[string]any
		if err := bson.Unmarshal(contents, &raw); err != nil {
			continue
		}
		name, _ := raw["Name"].(string)
		if moduleMap[u.ContainerID] == moduleName && name == docName {
			return model.ID(u.ID), nil
		}
	}
	return "", nil
}

// FindAllViewEntitySourceDocumentIDs returns every matching document ID.
func (r *Reader) FindAllViewEntitySourceDocumentIDs(moduleName, docName string) ([]model.ID, error) {
	units, err := r.listUnitsByType("DomainModels$ViewEntitySourceDocument")
	if err != nil {
		return nil, err
	}
	moduleMap, _, err := r.buildModuleResolution()
	if err != nil {
		return nil, err
	}
	var ids []model.ID
	for _, u := range units {
		contents, err := r.resolveContents(u.ID, u.Contents)
		if err != nil {
			continue
		}
		var raw map[string]any
		if err := bson.Unmarshal(contents, &raw); err != nil {
			continue
		}
		name, _ := raw["Name"].(string)
		if moduleMap[u.ContainerID] == moduleName && name == docName {
			ids = append(ids, model.ID(u.ID))
		}
	}
	return ids, nil
}

// ---------------------------------------------------------------------------
// Custom widget discovery (delegated to modelsdk-native walkers)
// ---------------------------------------------------------------------------

// FindCustomWidgetType searches pages + snippets for a CustomWidget with the
// given widgetID. Returns the first match. Nil + nil when not found, matching
// sdk/mpr.Reader semantics.
func (r *Reader) FindCustomWidgetType(widgetID string) (*types.RawCustomWidgetType, error) {
	all, err := r.FindAllCustomWidgetTypes(widgetID)
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, nil
	}
	return all[0], nil
}

// FindAllCustomWidgetTypes searches every page and snippet and returns every
// CustomWidget definition matching widgetID.
func (r *Reader) FindAllCustomWidgetTypes(widgetID string) ([]*types.RawCustomWidgetType, error) {
	units, err := r.listUnitsByType("Forms$Page")
	if err != nil {
		return nil, err
	}
	if snippets, err := r.listUnitsByType("Forms$Snippet"); err == nil {
		units = append(units, snippets...)
	}

	var results []*types.RawCustomWidgetType
	for _, u := range units {
		contents, err := r.resolveContents(u.ID, u.Contents)
		if err != nil {
			continue
		}
		if !strings.Contains(string(contents), widgetID) {
			continue
		}
		unitName := lookupRootName(contents)
		var doc bson.D
		if err := bson.Unmarshal(contents, &doc); err != nil {
			continue
		}
		for _, w := range collectCustomWidgets(doc, widgetID) {
			results = append(results, &types.RawCustomWidgetType{
				WidgetID:   widgetID,
				RawType:    w.rawType,
				RawObject:  w.rawObject,
				UnitID:     u.ID,
				UnitName:   unitName,
				WidgetName: w.name,
			})
		}
	}
	return results, nil
}

// ListAllCustomWidgetTypes scans every page and snippet and returns one entry
// per unique widget ID (first occurrence wins). Used by extract-templates.
func (r *Reader) ListAllCustomWidgetTypes() ([]*types.RawCustomWidgetType, error) {
	units, err := r.listUnitsByType("Forms$Page")
	if err != nil {
		return nil, err
	}
	if snippets, err := r.listUnitsByType("Forms$Snippet"); err == nil {
		units = append(units, snippets...)
	}

	seen := make(map[string]bool)
	var results []*types.RawCustomWidgetType
	for _, u := range units {
		contents, err := r.resolveContents(u.ID, u.Contents)
		if err != nil {
			continue
		}
		if !strings.Contains(string(contents), `CustomWidgets$CustomWidget"`) {
			continue
		}
		var doc bson.D
		if err := bson.Unmarshal(contents, &doc); err != nil {
			continue
		}
		for _, wid := range collectCustomWidgetIDs(doc) {
			if seen[wid] {
				continue
			}
			widgets := collectCustomWidgets(doc, wid)
			if len(widgets) == 0 {
				continue
			}
			seen[wid] = true
			results = append(results, &types.RawCustomWidgetType{
				WidgetID:  wid,
				RawType:   widgets[0].rawType,
				RawObject: widgets[0].rawObject,
				UnitID:    u.ID,
			})
		}
	}
	return results, nil
}

// ScanOqlQueryUpdates is not yet ported to modelsdk/mpr. The signature is
// preserved so backend.go can delegate to either reader implementation behind
// a feature flag during the migration window.
func (r *Reader) ScanOqlQueryUpdates(oldQualifiedName, newQualifiedName string) ([]types.UnitPatch, int, error) {
	return nil, 0, fmt.Errorf("modelsdk/mpr.Reader.ScanOqlQueryUpdates not yet implemented; use sdk/mpr.Reader for OQL scans")
}

// ---------------------------------------------------------------------------
// BSON walker helpers (kept local to avoid leaking into sibling files)
// ---------------------------------------------------------------------------

// customWidgetMatch is the in-package twin of sdk/mpr.widgetInfo.
type customWidgetMatch struct {
	rawType   bson.D
	rawObject bson.D
	name      string
}

// collectCustomWidgets recursively walks a BSON document and returns every
// CustomWidget whose Type doc has the matching WidgetId.
func collectCustomWidgets(doc bson.D, widgetID string) []customWidgetMatch {
	var results []customWidgetMatch
	walkCustomWidgets(doc, widgetID, &results)
	return results
}

func walkCustomWidgets(doc bson.D, widgetID string, out *[]customWidgetMatch) {
	var (
		isCustomWidget bool
		typeDoc        bson.D
		objectDoc      bson.D
		widgetName     string
	)
	for _, elem := range doc {
		switch elem.Key {
		case "$Type":
			if s, ok := elem.Value.(string); ok && s == "CustomWidgets$CustomWidget" {
				isCustomWidget = true
			}
		case "Type":
			if t, ok := elem.Value.(bson.D); ok {
				typeDoc = t
			}
		case "Object":
			if o, ok := elem.Value.(bson.D); ok {
				objectDoc = o
			}
		case "Name":
			if n, ok := elem.Value.(string); ok {
				widgetName = n
			}
		}
	}
	if isCustomWidget && typeDoc != nil && bsonWidgetIDMatches(typeDoc, widgetID) {
		*out = append(*out, customWidgetMatch{rawType: typeDoc, rawObject: objectDoc, name: widgetName})
	}
	for _, elem := range doc {
		switch v := elem.Value.(type) {
		case bson.D:
			walkCustomWidgets(v, widgetID, out)
		case bson.A:
			walkCustomWidgetsArray(v, widgetID, out)
		}
	}
}

func walkCustomWidgetsArray(arr bson.A, widgetID string, out *[]customWidgetMatch) {
	for _, item := range arr {
		switch v := item.(type) {
		case bson.D:
			walkCustomWidgets(v, widgetID, out)
		case bson.A:
			walkCustomWidgetsArray(v, widgetID, out)
		}
	}
}

// collectCustomWidgetIDs gathers every WidgetId present in a CustomWidgetType
// element so the caller can iterate over unique widget IDs.
func collectCustomWidgetIDs(doc bson.D) []string {
	var ids []string
	var walk func(v any)
	walk = func(v any) {
		switch val := v.(type) {
		case bson.D:
			var isType bool
			var widgetID string
			for _, e := range val {
				if e.Key == "$Type" && e.Value == "CustomWidgets$CustomWidgetType" {
					isType = true
				}
				if e.Key == "WidgetId" {
					if s, ok := e.Value.(string); ok {
						widgetID = s
					}
				}
			}
			if isType && widgetID != "" {
				ids = append(ids, widgetID)
			}
			for _, e := range val {
				walk(e.Value)
			}
		case bson.A:
			for _, item := range val {
				walk(item)
			}
		}
	}
	walk(doc)
	return ids
}

func bsonWidgetIDMatches(typeDoc bson.D, widgetID string) bool {
	hasType := false
	hasID := false
	for _, elem := range typeDoc {
		if elem.Key == "$Type" && elem.Value == "CustomWidgets$CustomWidgetType" {
			hasType = true
		}
		if elem.Key == "WidgetId" && elem.Value == widgetID {
			hasID = true
		}
	}
	return hasType && hasID
}

func lookupBsonArray(doc bson.D, key string) bson.A {
	for _, field := range doc {
		if field.Key == key {
			if arr, ok := field.Value.(bson.A); ok {
				return arr
			}
		}
	}
	return nil
}

func lookupBsonString(doc bson.D, key string) (string, bool) {
	for _, field := range doc {
		if field.Key == key {
			if s, ok := field.Value.(string); ok {
				return s, true
			}
		}
	}
	return "", false
}

// lookupRootName extracts the Name field at the root of a BSON document.
func lookupRootName(contents []byte) string {
	var doc bson.D
	if err := bson.Unmarshal(contents, &doc); err != nil {
		return ""
	}
	if s, ok := lookupBsonString(doc, "Name"); ok {
		return s
	}
	return ""
}
