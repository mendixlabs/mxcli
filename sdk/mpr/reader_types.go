// SPDX-License-Identifier: Apache-2.0

// Package mpr - Reader methods for listing and querying model units.
package mpr

import (
	"encoding/json"
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"

	"go.mongodb.org/mongo-driver/bson"
)

// ListJavaActions returns all Java actions in the project.
func (r *Reader) ListJavaActions() ([]*types.JavaAction, error) {
	units, err := r.listUnitsByType("JavaActions$JavaAction")
	if err != nil {
		return nil, err
	}

	var result []*types.JavaAction
	for _, u := range units {
		ja, err := r.parseJavaAction(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse java action %s: %w", u.ID, err)
		}
		result = append(result, ja)
	}

	return result, nil
}

// ListJavaScriptActions returns all JavaScript actions in the project.
func (r *Reader) ListJavaScriptActions() ([]*types.JavaScriptAction, error) {
	units, err := r.listUnitsByType("JavaScriptActions$JavaScriptAction")
	if err != nil {
		return nil, err
	}

	var result []*types.JavaScriptAction
	for _, u := range units {
		jsa, err := r.parseJavaScriptAction(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse javascript action %s: %w", u.ID, err)
		}
		result = append(result, jsa)
	}

	return result, nil
}

// ListBuildingBlocks returns all building blocks in the project.
func (r *Reader) ListBuildingBlocks() ([]*pages.BuildingBlock, error) {
	units, err := r.listUnitsByType("Forms$BuildingBlock")
	if err != nil {
		return nil, err
	}

	var result []*pages.BuildingBlock
	for _, u := range units {
		bb, err := r.parseBuildingBlock(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse building block %s: %w", u.ID, err)
		}
		result = append(result, bb)
	}

	return result, nil
}

// ListPageTemplates returns all page templates in the project.
func (r *Reader) ListPageTemplates() ([]*pages.PageTemplate, error) {
	units, err := r.listUnitsByType("Forms$PageTemplate")
	if err != nil {
		return nil, err
	}

	var result []*pages.PageTemplate
	for _, u := range units {
		pt, err := r.parsePageTemplate(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse page template %s: %w", u.ID, err)
		}
		result = append(result, pt)
	}

	return result, nil
}

// ListNavigationDocuments returns all navigation documents in the project.
func (r *Reader) ListNavigationDocuments() ([]*types.NavigationDocument, error) {
	units, err := r.listUnitsByType("Navigation$NavigationDocument")
	if err != nil {
		return nil, err
	}

	var result []*types.NavigationDocument
	for _, u := range units {
		nav, err := r.parseNavigationDocument(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse navigation document %s: %w", u.ID, err)
		}
		result = append(result, nav)
	}

	return result, nil
}

// GetNavigation returns the project's navigation document (singleton).
func (r *Reader) GetNavigation() (*types.NavigationDocument, error) {
	docs, err := r.ListNavigationDocuments()
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("no navigation document found")
	}
	return docs[0], nil
}

// ListImageCollections returns all image collections in the project.
func (r *Reader) ListImageCollections() ([]*types.ImageCollection, error) {
	units, err := r.listUnitsByType("Images$ImageCollection")
	if err != nil {
		return nil, err
	}

	var result []*types.ImageCollection
	for _, u := range units {
		ic, err := r.parseImageCollection(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse image collection %s: %w", u.ID, err)
		}
		result = append(result, ic)
	}

	return result, nil
}

// ListJsonStructures returns all JSON structures in the project.
func (r *Reader) ListJsonStructures() ([]*types.JsonStructure, error) {
	units, err := r.listUnitsByType("JsonStructures$JsonStructure")
	if err != nil {
		return nil, err
	}

	var result []*types.JsonStructure
	for _, u := range units {
		js, err := r.parseJsonStructure(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse JSON structure %s: %w", u.ID, err)
		}
		result = append(result, js)
	}

	return result, nil
}

// GetJsonStructureByQualifiedName retrieves a JSON structure by its qualified name (Module.Name).
func (r *Reader) GetJsonStructureByQualifiedName(moduleName, name string) (*types.JsonStructure, error) {
	all, err := r.ListJsonStructures()
	if err != nil {
		return nil, err
	}

	modules, err := r.ListModules()
	if err != nil {
		return nil, err
	}

	moduleID := ""
	for _, m := range modules {
		if m.Name == moduleName {
			moduleID = string(m.ID)
			break
		}
	}

	for _, js := range all {
		if js.Name == name && (moduleID == "" || string(js.ContainerID) == moduleID) {
			return js, nil
		}
	}
	return nil, fmt.Errorf("JSON structure %s.%s not found", moduleName, name)
}

// ListRawUnitsByType returns all raw units matching the given type prefix,
// including their BSON contents. This is useful for scanning BSON directly
// without full parsing.
func (r *Reader) ListRawUnitsByType(typePrefix string) ([]*types.RawUnit, error) {
	units, err := r.listUnitsByType(typePrefix)
	if err != nil {
		return nil, err
	}

	var result []*types.RawUnit
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

// ListUnits returns all units with their IDs and types.
func (r *Reader) ListUnits() ([]*types.UnitInfo, error) {
	units, err := r.listUnitsByType("")
	if err != nil {
		return nil, err
	}

	var result []*types.UnitInfo
	for _, u := range units {
		result = append(result, &types.UnitInfo{
			ID:              model.ID(u.ID),
			ContainerID:     model.ID(u.ContainerID),
			ContainmentName: u.ContainmentName,
			Type:            u.Type,
		})
	}

	return result, nil
}

// ListFolders returns all project folders with their names.
func (r *Reader) ListFolders() ([]*types.FolderInfo, error) {
	units, err := r.listUnitsByType("Projects$Folder")
	if err != nil {
		return nil, err
	}

	var result []*types.FolderInfo
	for _, u := range units {
		name := ""
		if len(u.Contents) > 0 {
			var raw map[string]any
			if err := bson.Unmarshal(u.Contents, &raw); err == nil {
				if n, ok := raw["Name"].(string); ok {
					name = n
				}
			}
		}
		result = append(result, &types.FolderInfo{
			ID:          model.ID(u.ID),
			ContainerID: model.ID(u.ContainerID),
			Name:        name,
		})
	}

	return result, nil
}

// ExportJSON exports the entire model as JSON.
func (r *Reader) ExportJSON() ([]byte, error) {
	modules, err := r.ListModules()
	if err != nil {
		modules = nil // Continue even if modules fail
	}

	domainModels, err := r.ListDomainModels()
	if err != nil {
		domainModels = nil
	}

	microflowsList, err := r.ListMicroflows()
	if err != nil {
		microflowsList = nil
	}

	nanoflows, err := r.ListNanoflows()
	if err != nil {
		nanoflows = nil
	}

	pagesList, err := r.ListPages()
	if err != nil {
		pagesList = nil
	}

	layouts, err := r.ListLayouts()
	if err != nil {
		layouts = nil
	}

	enumerations, err := r.ListEnumerations()
	if err != nil {
		enumerations = nil
	}

	constants, err := r.ListConstants()
	if err != nil {
		constants = nil
	}

	export := map[string]any{
		"modules":      modules,
		"domainModels": domainModels,
		"microflows":   microflowsList,
		"nanoflows":    nanoflows,
		"pages":        pagesList,
		"layouts":      layouts,
		"enumerations": enumerations,
		"constants":    constants,
	}

	return json.MarshalIndent(export, "", "  ")
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
