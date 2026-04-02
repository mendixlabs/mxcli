// SPDX-License-Identifier: Apache-2.0

// Package mpr - Additional types and utility methods for Reader.
package mpr

import (
	"encoding/json"
	"fmt"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
	"github.com/mendixlabs/mxcli/sdk/pages"

	"go.mongodb.org/mongo-driver/bson"
)

// JavaAction represents a Java action.
type JavaAction struct {
	model.BaseElement
	ContainerID   model.ID `json:"containerId"`
	Name          string   `json:"name"`
	Documentation string   `json:"documentation,omitempty"`
}

// GetName returns the Java action's name.
func (ja *JavaAction) GetName() string {
	return ja.Name
}

// GetContainerID returns the container ID.
func (ja *JavaAction) GetContainerID() model.ID {
	return ja.ContainerID
}

// ListJavaActions returns all Java actions in the project.
func (r *Reader) ListJavaActions() ([]*JavaAction, error) {
	units, err := r.listUnitsByType("JavaActions$JavaAction")
	if err != nil {
		return nil, err
	}

	var result []*JavaAction
	for _, u := range units {
		ja, err := r.parseJavaAction(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse java action %s: %w", u.ID, err)
		}
		result = append(result, ja)
	}

	return result, nil
}

// JavaScriptAction represents a JavaScript action.
type JavaScriptAction struct {
	model.BaseElement
	ContainerID             model.ID                           `json:"containerId"`
	Name                    string                             `json:"name"`
	Documentation           string                             `json:"documentation,omitempty"`
	Platform                string                             `json:"platform,omitempty"`
	Excluded                bool                               `json:"excluded"`
	ExportLevel             string                             `json:"exportLevel,omitempty"`
	ActionDefaultReturnName string                             `json:"actionDefaultReturnName,omitempty"`
	ReturnType              javaactions.CodeActionReturnType   `json:"returnType,omitempty"`
	Parameters              []*javaactions.JavaActionParameter `json:"parameters,omitempty"`
	TypeParameters          []*javaactions.TypeParameterDef    `json:"typeParameters,omitempty"`
	MicroflowActionInfo     *javaactions.MicroflowActionInfo   `json:"microflowActionInfo,omitempty"`
}

// GetName returns the JavaScript action's name.
func (jsa *JavaScriptAction) GetName() string {
	return jsa.Name
}

// GetContainerID returns the container ID.
func (jsa *JavaScriptAction) GetContainerID() model.ID {
	return jsa.ContainerID
}

// FindTypeParameterName looks up a type parameter name by its ID.
func (jsa *JavaScriptAction) FindTypeParameterName(id model.ID) string {
	for _, tp := range jsa.TypeParameters {
		if tp.ID == id {
			return tp.Name
		}
	}
	return ""
}

// ListJavaScriptActions returns all JavaScript actions in the project.
func (r *Reader) ListJavaScriptActions() ([]*JavaScriptAction, error) {
	units, err := r.listUnitsByType("JavaScriptActions$JavaScriptAction")
	if err != nil {
		return nil, err
	}

	var result []*JavaScriptAction
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

// NavigationDocument represents a navigation document.
type NavigationDocument struct {
	model.BaseElement
	ContainerID model.ID             `json:"containerId"`
	Name        string               `json:"name"`
	Profiles    []*NavigationProfile `json:"profiles,omitempty"`
}

// GetName returns the navigation document's name.
func (nd *NavigationDocument) GetName() string {
	return nd.Name
}

// GetContainerID returns the container ID.
func (nd *NavigationDocument) GetContainerID() model.ID {
	return nd.ContainerID
}

// NavigationProfile represents a navigation profile (web or native).
type NavigationProfile struct {
	Name               string              `json:"name"`
	Kind               string              `json:"kind"` // Responsive, Phone, Tablet, etc.
	IsNative           bool                `json:"isNative"`
	HomePage           *NavHomePage        `json:"homePage,omitempty"`
	RoleBasedHomePages []*NavRoleBasedHome `json:"roleBasedHomePages,omitempty"`
	LoginPage          string              `json:"loginPage,omitempty"`    // qualified page name
	NotFoundPage       string              `json:"notFoundPage,omitempty"` // qualified page name
	MenuItems          []*NavMenuItem      `json:"menuItems,omitempty"`
	OfflineEntities    []*NavOfflineEntity `json:"offlineEntities,omitempty"`
}

// NavHomePage represents a default home page (page or microflow).
type NavHomePage struct {
	Page      string `json:"page,omitempty"`      // qualified page name
	Microflow string `json:"microflow,omitempty"` // qualified microflow name
}

// NavRoleBasedHome represents a role-specific home page override.
type NavRoleBasedHome struct {
	UserRole  string `json:"userRole"`            // qualified user role name
	Page      string `json:"page,omitempty"`      // qualified page name
	Microflow string `json:"microflow,omitempty"` // qualified microflow name
}

// NavMenuItem represents a menu item (recursive for sub-menus).
type NavMenuItem struct {
	Caption    string         `json:"caption"`
	Page       string         `json:"page,omitempty"`       // target page qualified name
	Microflow  string         `json:"microflow,omitempty"`  // target microflow qualified name
	ActionType string         `json:"actionType,omitempty"` // PageAction, MicroflowAction, NoAction, OpenLinkAction
	Items      []*NavMenuItem `json:"items,omitempty"`
}

// NavOfflineEntity represents an offline entity sync configuration.
type NavOfflineEntity struct {
	Entity     string `json:"entity"`               // qualified entity name
	SyncMode   string `json:"syncMode"`             // All, Constrained, Never, etc.
	Constraint string `json:"constraint,omitempty"` // XPath
}

// ListNavigationDocuments returns all navigation documents in the project.
func (r *Reader) ListNavigationDocuments() ([]*NavigationDocument, error) {
	units, err := r.listUnitsByType("Navigation$NavigationDocument")
	if err != nil {
		return nil, err
	}

	var result []*NavigationDocument
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
func (r *Reader) GetNavigation() (*NavigationDocument, error) {
	docs, err := r.ListNavigationDocuments()
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("no navigation document found")
	}
	return docs[0], nil
}

// ImageCollection represents an image collection.
type ImageCollection struct {
	model.BaseElement
	ContainerID   model.ID `json:"containerId"`
	Name          string   `json:"name"`
	ExportLevel   string   `json:"exportLevel,omitempty"`
	Documentation string   `json:"documentation,omitempty"`
	Images        []Image  `json:"images,omitempty"`
}

// Image represents an image in a collection.
type Image struct {
	ID     model.ID `json:"id"`
	Name   string   `json:"name"`
	Data   []byte   `json:"data,omitempty"`   // raw image bytes
	Format string   `json:"format,omitempty"` // "Png", "Svg", "Gif", "Jpeg", "Bmp"
}

// GetName returns the image collection's name.
func (ic *ImageCollection) GetName() string {
	return ic.Name
}

// GetContainerID returns the container ID.
func (ic *ImageCollection) GetContainerID() model.ID {
	return ic.ContainerID
}

// ListImageCollections returns all image collections in the project.
func (r *Reader) ListImageCollections() ([]*ImageCollection, error) {
	units, err := r.listUnitsByType("Images$ImageCollection")
	if err != nil {
		return nil, err
	}

	var result []*ImageCollection
	for _, u := range units {
		ic, err := r.parseImageCollection(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse image collection %s: %w", u.ID, err)
		}
		result = append(result, ic)
	}

	return result, nil
}

// JsonStructure represents a JSON structure document.
type JsonStructure struct {
	model.BaseElement
	ContainerID   model.ID       `json:"containerId"`
	Name          string         `json:"name"`
	Documentation string         `json:"documentation,omitempty"`
	JsonSnippet   string         `json:"jsonSnippet,omitempty"`
	Elements      []*JsonElement `json:"elements,omitempty"`
	Excluded      bool           `json:"excluded,omitempty"`
	ExportLevel   string         `json:"exportLevel,omitempty"`
}

// JsonElement represents an element in a JSON structure's element tree.
type JsonElement struct {
	ExposedName     string         `json:"exposedName"`
	ExposedItemName string         `json:"exposedItemName,omitempty"`
	Path            string         `json:"path"`
	ElementType     string         `json:"elementType"`   // "Object", "Array", "Value", "Choice"
	PrimitiveType   string         `json:"primitiveType"` // "String", "Integer", "Boolean", "Decimal", "Unknown"
	MinOccurs       int            `json:"minOccurs"`
	MaxOccurs       int            `json:"maxOccurs"` // -1 = unbounded
	Nillable        bool           `json:"nillable,omitempty"`
	IsDefaultType   bool           `json:"isDefaultType,omitempty"`
	MaxLength       int            `json:"maxLength"`      // -1 = unset
	FractionDigits  int            `json:"fractionDigits"` // -1 = unset
	TotalDigits     int            `json:"totalDigits"`    // -1 = unset
	OriginalValue   string         `json:"originalValue,omitempty"`
	Children        []*JsonElement `json:"children,omitempty"`
}

// GetName returns the JSON structure's name.
func (js *JsonStructure) GetName() string {
	return js.Name
}

// GetContainerID returns the container ID.
func (js *JsonStructure) GetContainerID() model.ID {
	return js.ContainerID
}

// ListJsonStructures returns all JSON structures in the project.
func (r *Reader) ListJsonStructures() ([]*JsonStructure, error) {
	units, err := r.listUnitsByType("JsonStructures$JsonStructure")
	if err != nil {
		return nil, err
	}

	var result []*JsonStructure
	for _, u := range units {
		js, err := r.parseJsonStructure(u.ID, u.ContainerID, u.Contents)
		if err != nil {
			return nil, fmt.Errorf("failed to parse JSON structure %s: %w", u.ID, err)
		}
		result = append(result, js)
	}

	return result, nil
}

// UnitInfo contains basic information about a unit.
type UnitInfo struct {
	ID              model.ID
	ContainerID     model.ID
	ContainmentName string
	Type            string
}

// RawUnit holds raw unit data with BSON contents.
type RawUnit struct {
	ID          model.ID
	ContainerID model.ID
	Type        string
	Contents    []byte
}

// ListRawUnitsByType returns all raw units matching the given type prefix,
// including their BSON contents. This is useful for scanning BSON directly
// without full parsing.
func (r *Reader) ListRawUnitsByType(typePrefix string) ([]*RawUnit, error) {
	units, err := r.listUnitsByType(typePrefix)
	if err != nil {
		return nil, err
	}

	var result []*RawUnit
	for _, u := range units {
		contents, err := r.resolveContents(u.ID, u.Contents)
		if err != nil {
			continue
		}
		result = append(result, &RawUnit{
			ID:          model.ID(u.ID),
			ContainerID: model.ID(u.ContainerID),
			Type:        u.Type,
			Contents:    contents,
		})
	}
	return result, nil
}

// ListUnits returns all units with their IDs and types.
func (r *Reader) ListUnits() ([]*UnitInfo, error) {
	units, err := r.listUnitsByType("")
	if err != nil {
		return nil, err
	}

	var result []*UnitInfo
	for _, u := range units {
		result = append(result, &UnitInfo{
			ID:              model.ID(u.ID),
			ContainerID:     model.ID(u.ContainerID),
			ContainmentName: u.ContainmentName,
			Type:            u.Type,
		})
	}

	return result, nil
}

// FolderInfo contains information about a project folder.
type FolderInfo struct {
	ID          model.ID
	ContainerID model.ID
	Name        string
}

// ListFolders returns all project folders with their names.
func (r *Reader) ListFolders() ([]*FolderInfo, error) {
	units, err := r.listUnitsByType("Projects$Folder")
	if err != nil {
		return nil, err
	}

	var result []*FolderInfo
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
		result = append(result, &FolderInfo{
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
