// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"log"
	"strings"

	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/widgets"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// unquoteIdentifier strips surrounding double-quotes or backticks from a quoted identifier.
func unquoteIdentifier(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '`' && s[len(s)-1] == '`') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// unquoteQualifiedName strips quotes from each segment of a dotted qualified name.
func unquoteQualifiedName(s string) string {
	parts := strings.Split(s, ".")
	for i, p := range parts {
		parts[i] = unquoteIdentifier(p)
	}
	return strings.Join(parts, ".")
}

// resolveAttributePath resolves a short attribute name to a fully qualified name
// using the current entity context. If the attribute already has dots or no entity
// context is available, the attribute is returned as-is.
func (pb *pageBuilder) resolveAttributePath(attr string) string {
	if attr == "" {
		return ""
	}
	// If the attribute already contains a dot, it's already qualified
	if strings.Contains(attr, ".") {
		return attr
	}
	// If we have an entity context, prefix the attribute with it
	if pb.entityContext != "" {
		return pb.entityContext + "." + attr
	}
	return attr
}

// resolveAssociationPath resolves a short association name to a fully qualified name.
// Associations are module-level objects, so the path is Module.AssociationName (2-part).
// If the name already contains a dot, it's returned as-is.
func (pb *pageBuilder) resolveAssociationPath(assocName string) string {
	if assocName == "" {
		return ""
	}
	// If already qualified (contains a dot), return as-is
	if strings.Contains(assocName, ".") {
		return assocName
	}
	// Extract module name from entity context (e.g., "PgTest.Order" → "PgTest")
	if pb.entityContext != "" {
		parts := strings.SplitN(pb.entityContext, ".", 2)
		if len(parts) >= 1 {
			return parts[0] + "." + assocName
		}
	}
	return assocName
}

// updateWidgetPropertyValue finds and updates a specific property value in a WidgetObject.
// The updateFn is called with the WidgetValue and should return the modified value.
func updateWidgetPropertyValue(obj bson.D, propTypeIDs map[string]pages.PropertyTypeIDEntry, propertyKey string, updateFn func(bson.D) bson.D) bson.D {
	// Find the PropertyTypeID for this key
	propEntry, ok := propTypeIDs[propertyKey]
	if !ok {
		return obj
	}

	result := make(bson.D, 0, len(obj))
	for _, elem := range obj {
		if elem.Key == "Properties" {
			if arr, ok := elem.Value.(bson.A); ok {
				result = append(result, bson.E{Key: "Properties", Value: updatePropertyInArray(arr, propEntry.PropertyTypeID, updateFn)})
				continue
			}
		}
		result = append(result, elem)
	}
	return result
}

// updatePropertyInArray finds a property by TypePointer and updates its value.
func updatePropertyInArray(arr bson.A, propertyTypeID string, updateFn func(bson.D) bson.D) bson.A {
	result := make(bson.A, len(arr))
	matched := false
	for i, item := range arr {
		if prop, ok := item.(bson.D); ok {
			if matchesTypePointer(prop, propertyTypeID) {
				result[i] = updatePropertyValue(prop, updateFn)
				matched = true
			} else {
				result[i] = item
			}
		} else {
			result[i] = item
		}
	}
	if !matched {
		log.Printf("WARNING: updatePropertyInArray: no match for TypePointer %s in %d properties", propertyTypeID, len(arr)-1)
	}
	return result
}

// matchesTypePointer checks if a WidgetProperty has the given TypePointer.
func matchesTypePointer(prop bson.D, propertyTypeID string) bool {
	// Normalize: strip dashes for comparison (BlobToUUID returns dashed format,
	// but propertyTypeIDs from template loader use undashed 32-char hex).
	normalizedTarget := strings.ReplaceAll(propertyTypeID, "-", "")
	for _, elem := range prop {
		if elem.Key == "TypePointer" {
			// Handle both primitive.Binary (from MPR) and []byte (from JSON templates)
			switch v := elem.Value.(type) {
			case primitive.Binary:
				propID := strings.ReplaceAll(types.BlobToUUID(v.Data), "-", "")
				return propID == normalizedTarget
			case []byte:
				propID := strings.ReplaceAll(types.BlobToUUID(v), "-", "")
				if propID == normalizedTarget {
					return true
				}
				// Also try raw hex encoding (no GUID swap) for templates
				rawHex := fmt.Sprintf("%x", v)
				return rawHex == normalizedTarget
			}
		}
	}
	return false
}

// updatePropertyValue updates the Value field in a WidgetProperty.
func updatePropertyValue(prop bson.D, updateFn func(bson.D) bson.D) bson.D {
	result := make(bson.D, 0, len(prop))
	for _, elem := range prop {
		if elem.Key == "Value" {
			if val, ok := elem.Value.(bson.D); ok {
				result = append(result, bson.E{Key: "Value", Value: updateFn(val)})
			} else {
				result = append(result, elem)
			}
		} else {
			result = append(result, elem)
		}
	}
	return result
}

// setPrimitiveValue sets the PrimitiveValue field in a WidgetValue.
func setPrimitiveValue(val bson.D, value string) bson.D {
	result := make(bson.D, 0, len(val))
	for _, elem := range val {
		if elem.Key == "PrimitiveValue" {
			result = append(result, bson.E{Key: "PrimitiveValue", Value: value})
		} else {
			result = append(result, elem)
		}
	}
	return result
}

// setAssociationRef sets the EntityRef field in a WidgetValue for an association binding
// on a pluggable widget. Uses DomainModels$IndirectEntityRef with a Steps array containing
// a DomainModels$EntityRefStep that specifies the association and destination entity.
// MxBuild requires the EntityRef to resolve the association target (CE0642).
func setAssociationRef(val bson.D, assocPath string, entityName string) bson.D {
	result := make(bson.D, 0, len(val))
	for _, elem := range val {
		if elem.Key == "EntityRef" && entityName != "" {
			result = append(result, bson.E{Key: "EntityRef", Value: bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "DomainModels$IndirectEntityRef"},
				{Key: "Steps", Value: bson.A{
					int32(2), // version marker
					bson.D{
						{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
						{Key: "$Type", Value: "DomainModels$EntityRefStep"},
						{Key: "Association", Value: assocPath},
						{Key: "DestinationEntity", Value: entityName},
					},
				}},
			}})
		} else {
			result = append(result, elem)
		}
	}
	return result
}

// setAttributeRef sets the AttributeRef field in a WidgetValue.
// The attrPath must be fully qualified (Module.Entity.Attribute, 2+ dots).
// If not fully qualified, AttributeRef is set to nil to avoid Studio Pro crash.
func setAttributeRef(val bson.D, attrPath string) bson.D {
	result := make(bson.D, 0, len(val))
	for _, elem := range val {
		if elem.Key == "AttributeRef" {
			if strings.Count(attrPath, ".") >= 2 {
				result = append(result, bson.E{Key: "AttributeRef", Value: bson.D{
					{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
					{Key: "$Type", Value: "DomainModels$AttributeRef"},
					{Key: "Attribute", Value: attrPath},
					{Key: "EntityRef", Value: nil},
				}})
			} else {
				result = append(result, bson.E{Key: "AttributeRef", Value: nil})
			}
		} else {
			result = append(result, elem)
		}
	}
	return result
}

// convertPropertyTypeIDs converts widgets.PropertyTypeIDEntry to pages.PropertyTypeIDEntry.
func convertPropertyTypeIDs(src map[string]widgets.PropertyTypeIDEntry) map[string]pages.PropertyTypeIDEntry {
	result := make(map[string]pages.PropertyTypeIDEntry)
	for k, v := range src {
		entry := pages.PropertyTypeIDEntry{
			PropertyTypeID: v.PropertyTypeID,
			ValueTypeID:    v.ValueTypeID,
			DefaultValue:   v.DefaultValue,
			ValueType:      v.ValueType,
			Required:       v.Required,
			ObjectTypeID:   v.ObjectTypeID,
		}
		// Convert nested property IDs if present
		if len(v.NestedPropertyIDs) > 0 {
			entry.NestedPropertyIDs = convertPropertyTypeIDs(v.NestedPropertyIDs)
		}
		result[k] = entry
	}
	return result
}

// resolveSnippetRef resolves a snippet qualified name to its ID.
func (pb *pageBuilder) resolveSnippetRef(snippetRef string) (model.ID, error) {
	if snippetRef == "" {
		return "", mdlerrors.NewValidation("empty snippet reference")
	}

	snippetRef = unquoteQualifiedName(snippetRef)
	parts := strings.Split(snippetRef, ".")
	var moduleName, snippetName string
	if len(parts) >= 2 {
		moduleName = parts[0]
		snippetName = parts[len(parts)-1]
	} else {
		snippetName = snippetRef
	}

	snippets, err := pb.reader.ListSnippets()
	if err != nil {
		return "", err
	}

	h, err := pb.getHierarchy()
	if err != nil {
		return "", mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, s := range snippets {
		modID := h.FindModuleID(s.ContainerID)
		modName := h.GetModuleName(modID)
		if s.Name == snippetName && (moduleName == "" || modName == moduleName) {
			return s.ID, nil
		}
	}

	return "", mdlerrors.NewNotFound("snippet", snippetRef)
}

func (pb *pageBuilder) resolveMicroflow(qualifiedName string) (model.ID, error) {
	qualifiedName = unquoteQualifiedName(qualifiedName)
	// Parse qualified name
	parts := strings.Split(qualifiedName, ".")
	if len(parts) < 2 {
		return "", mdlerrors.NewValidationf("invalid microflow name: %s", qualifiedName)
	}
	moduleName := parts[0]
	mfName := strings.Join(parts[1:], ".")

	// First, check if the microflow was created during this session
	// (not yet visible via reader)
	if pb.execCache != nil && pb.execCache.createdMicroflows != nil {
		if info, ok := pb.execCache.createdMicroflows[qualifiedName]; ok {
			return info.ID, nil
		}
	}

	// Get microflows from reader cache
	mfs, err := pb.getMicroflows()
	if err != nil {
		return "", mdlerrors.NewBackend("list microflows", err)
	}

	// Use hierarchy to resolve module names (handles microflows in folders)
	h, err := pb.getHierarchy()
	if err != nil {
		return "", mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find matching microflow
	for _, mf := range mfs {
		modID := h.FindModuleID(mf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == moduleName && mf.Name == mfName {
			return mf.ID, nil
		}
	}

	return "", mdlerrors.NewNotFound("microflow", qualifiedName)
}

func (pb *pageBuilder) resolvePageRef(pageRef string) (model.ID, error) {
	if pageRef == "" {
		return "", mdlerrors.NewValidation("empty page reference")
	}

	pageRef = unquoteQualifiedName(pageRef)
	parts := strings.Split(pageRef, ".")
	var moduleName, pageName string
	if len(parts) >= 2 {
		moduleName = parts[0]
		pageName = parts[len(parts)-1]
	} else {
		pageName = pageRef
	}

	// First, check if the page was created during this session
	// (not yet visible via reader)
	if pb.execCache != nil && pb.execCache.createdPages != nil {
		if info, ok := pb.execCache.createdPages[pageRef]; ok {
			return info.ID, nil
		}
		// Also check with module prefix if not found
		if moduleName != "" {
			if info, ok := pb.execCache.createdPages[moduleName+"."+pageName]; ok {
				return info.ID, nil
			}
		}
	}

	pgs, err := pb.getPages()
	if err != nil {
		return "", err
	}

	h, err := pb.getHierarchy()
	if err != nil {
		return "", mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, p := range pgs {
		modID := h.FindModuleID(p.ContainerID)
		modName := h.GetModuleName(modID)
		if p.Name == pageName && (moduleName == "" || modName == moduleName) {
			return p.ID, nil
		}
	}

	return "", mdlerrors.NewNotFound("page", pageRef)
}
