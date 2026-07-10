// SPDX-License-Identifier: Apache-2.0

// Package executor - Shared helper functions for module/folder resolution,
// reference validation, and data type conversion.
package executor

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

// ----------------------------------------------------------------------------
// Module and Folder Resolution
// ----------------------------------------------------------------------------

// getModulesFromCache returns cached modules or loads them.
func getModulesFromCache(ctx *ExecContext) ([]*model.Module, error) {
	if ctx.Cache != nil && ctx.Cache.modules != nil {
		return ctx.Cache.modules, nil
	}
	modules, err := ctx.Backend.ListModules()
	if err != nil {
		return nil, err
	}
	if ctx.Cache != nil {
		ctx.Cache.modules = modules
	}
	return modules, nil
}

// invalidateModuleCache clears the module cache so next lookup gets fresh data.
// Also invalidates the hierarchy cache since new modules affect hierarchy.
func invalidateModuleCache(ctx *ExecContext) {
	if ctx.Cache != nil {
		ctx.Cache.modules = nil
		ctx.Cache.hierarchy = nil
	}
}

func findModule(ctx *ExecContext, name string) (*model.Module, error) {
	// Module name is required - objects must always belong to a module
	if name == "" {
		return nil, mdlerrors.NewValidation("module name is required: objects must be created within a module (use ModuleName.ObjectName syntax)")
	}

	modules, err := getModulesFromCache(ctx)
	if err != nil {
		return nil, mdlerrors.NewBackend("list modules", err)
	}

	for _, m := range modules {
		if m.Name == name {
			return m, nil
		}
	}

	return nil, mdlerrors.NewNotFound("module", name)
}

// findOrCreateModule looks up a module by name, auto-creating it if it doesn't exist
// and the executor has write access. Used by CREATE operations to avoid requiring
// manual module creation.
func findOrCreateModule(ctx *ExecContext, name string) (*model.Module, error) {
	m, err := findModule(ctx, name)
	if err == nil {
		return m, nil
	}
	if !ctx.ConnectedForWrite() || name == "" {
		return nil, err
	}
	// Auto-create the module
	if createErr := execCreateModule(ctx, &ast.CreateModuleStmt{Name: name}); createErr != nil {
		return nil, mdlerrors.NewBackend("auto-create module "+name, createErr)
	}
	return findModule(ctx, name)
}

func findModuleByID(ctx *ExecContext, id model.ID) (*model.Module, error) {
	modules, err := getModulesFromCache(ctx)
	if err != nil {
		return nil, mdlerrors.NewBackend("list modules", err)
	}

	for _, m := range modules {
		if m.ID == id {
			return m, nil
		}
	}

	return nil, mdlerrors.NewNotFoundMsg("module", string(id), "module not found with ID: "+string(id))
}

// resolveFolder resolves a folder path (e.g., "Resources/Images") to a folder ID.
// The path is relative to the given module. If the folder doesn't exist, it creates it.
func resolveFolder(ctx *ExecContext, moduleID model.ID, folderPath string) (model.ID, error) {
	if folderPath == "" {
		return moduleID, nil
	}

	folders, err := ctx.Backend.ListFolders()
	if err != nil {
		return "", mdlerrors.NewBackend("list folders", err)
	}

	// Split path into parts
	parts := strings.Split(folderPath, "/")
	currentContainerID := moduleID

	for _, part := range parts {
		if part == "" {
			continue
		}

		// Find folder with this name under current container
		var foundFolder *types.FolderInfo
		for _, f := range folders {
			if f.ContainerID == currentContainerID && f.Name == part {
				foundFolder = f
				break
			}
		}

		if foundFolder != nil {
			currentContainerID = foundFolder.ID
		} else {
			// Create the folder
			parentID := currentContainerID
			newFolderID, err := createFolder(ctx, part, parentID)
			if err != nil {
				return "", mdlerrors.NewBackend("create folder "+part, err)
			}
			currentContainerID = newFolderID

			// Add to the list so subsequent lookups find it
			folders = append(folders, &types.FolderInfo{
				ID:          newFolderID,
				ContainerID: parentID,
				Name:        part,
			})
		}
	}

	return currentContainerID, nil
}

// createFolder creates a new folder in the project.
func createFolder(ctx *ExecContext, name string, containerID model.ID) (model.ID, error) {
	folder := &model.Folder{
		BaseElement: model.BaseElement{
			ID:       model.ID(types.GenerateID()),
			TypeName: "Projects$Folder",
		},
		ContainerID: containerID,
		Name:        name,
	}

	if err := ctx.Backend.CreateFolder(folder); err != nil {
		return "", err
	}

	return folder.ID, nil
}

// ----------------------------------------------------------------------------
// Reference Existence Checks
// ----------------------------------------------------------------------------

// enumerationExists checks if an enumeration exists in the project.
func enumerationExists(ctx *ExecContext, qualifiedName string) bool {
	if !ctx.Connected() {
		return false
	}

	// Parse the qualified name to get module and enum name
	parts := strings.Split(qualifiedName, ".")
	if len(parts) != 2 {
		return false
	}
	moduleName, enumName := parts[0], parts[1]

	// Find the module to get its ID
	module, err := findModule(ctx, moduleName)
	if err != nil {
		return false
	}

	// Get all enumerations and check if one matches
	enums, err := ctx.Backend.ListEnumerations()
	if err != nil {
		return false
	}

	for _, enum := range enums {
		if enum.ContainerID == module.ID && enum.Name == enumName {
			return true
		}
	}
	return false
}

// ----------------------------------------------------------------------------
// Widget Reference Validation
// ----------------------------------------------------------------------------

// validateWidgetReferences validates all qualified name references in a widget tree.
// It checks DataSource (microflow/nanoflow/entity), Action (page/microflow/nanoflow),
// and Snippet references.
func validateWidgetReferences(ctx *ExecContext, widgets []*ast.WidgetV3, sc *scriptContext) []string {
	if !ctx.Connected() || len(widgets) == 0 {
		return nil
	}

	// Collect all references from the widget tree
	refs := &widgetRefCollector{}
	refs.collectFromWidgets(widgets)
	refs.dedupe()

	if refs.empty() {
		return nil
	}

	// Build lookup maps lazily (only for reference types that are actually used)
	var errors []string

	if len(refs.microflows) > 0 {
		known := buildMicroflowQualifiedNames(ctx)
		for _, ref := range refs.microflows {
			if !known[ref] && !sc.microflows[ref] {
				errors = append(errors, fmt.Sprintf("microflow not found: %s", ref))
			}
		}
	}

	if len(refs.nanoflows) > 0 {
		known := buildNanoflowQualifiedNames(ctx)
		for _, ref := range refs.nanoflows {
			if !known[ref] {
				errors = append(errors, fmt.Sprintf("nanoflow not found: %s", ref))
			}
		}
	}

	if len(refs.pages) > 0 {
		known := buildPageQualifiedNames(ctx)
		for _, ref := range refs.pages {
			if !known[ref] && !sc.pages[ref] {
				errors = append(errors, fmt.Sprintf("page not found: %s", ref))
			}
		}
	}

	if len(refs.snippets) > 0 {
		known := buildSnippetQualifiedNames(ctx)
		for _, ref := range refs.snippets {
			if !known[ref] && !sc.snippets[ref] {
				errors = append(errors, fmt.Sprintf("snippet not found: %s", ref))
			}
		}
	}

	if len(refs.entities) > 0 {
		known := buildEntityQualifiedNames(ctx)
		for _, ref := range refs.entities {
			if !known[ref] && !sc.entities[ref] {
				errors = append(errors, fmt.Sprintf("entity not found: %s", ref))
			}
		}
	}

	return errors
}

// widgetRefCollector collects qualified name references from a widget tree.
type widgetRefCollector struct {
	microflows []string
	nanoflows  []string
	pages      []string
	snippets   []string
	entities   []string
}

// dedupe collapses repeated references within each category, preserving first
// occurrence order. A single widget tree often points at the same target more
// than once (e.g. an overview page with both a New button and a per-row Edit
// button referencing the same edit page), which would otherwise produce
// duplicate identical diagnostics.
func (c *widgetRefCollector) dedupe() {
	c.microflows = uniqueStrings(c.microflows)
	c.nanoflows = uniqueStrings(c.nanoflows)
	c.pages = uniqueStrings(c.pages)
	c.snippets = uniqueStrings(c.snippets)
	c.entities = uniqueStrings(c.entities)
}

// uniqueStrings returns s with duplicate values removed, preserving order.
func uniqueStrings(s []string) []string {
	if len(s) < 2 {
		return s
	}
	seen := make(map[string]bool, len(s))
	out := s[:0]
	for _, v := range s {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func (c *widgetRefCollector) empty() bool {
	return len(c.microflows) == 0 && len(c.nanoflows) == 0 &&
		len(c.pages) == 0 && len(c.snippets) == 0 && len(c.entities) == 0
}

func (c *widgetRefCollector) collectFromWidgets(widgets []*ast.WidgetV3) {
	for _, w := range widgets {
		c.collectFromWidget(w)
	}
}

func (c *widgetRefCollector) collectFromWidget(w *ast.WidgetV3) {
	// Check DataSource
	if ds := w.GetDataSource(); ds != nil {
		switch ds.Type {
		case "microflow":
			if ds.Reference != "" {
				c.microflows = append(c.microflows, ds.Reference)
			}
		case "nanoflow":
			if ds.Reference != "" {
				c.nanoflows = append(c.nanoflows, ds.Reference)
			}
		case "database":
			if ds.Reference != "" {
				c.entities = append(c.entities, ds.Reference)
			}
		}
	}

	// Check Action
	if action := w.GetAction(); action != nil {
		c.collectFromAction(action)
	}

	// Check Snippet reference
	if snippet := w.GetSnippet(); snippet != "" {
		c.snippets = append(c.snippets, snippet)
	}

	// Recurse into children
	c.collectFromWidgets(w.Children)
}

func (c *widgetRefCollector) collectFromAction(action *ast.ActionV3) {
	switch action.Type {
	case "showPage":
		if action.Target != "" {
			c.pages = append(c.pages, action.Target)
		}
	case "microflow":
		if action.Target != "" {
			c.microflows = append(c.microflows, action.Target)
		}
	case "nanoflow":
		if action.Target != "" {
			c.nanoflows = append(c.nanoflows, action.Target)
		}
	case "create":
		if action.Target != "" {
			c.entities = append(c.entities, action.Target)
		}
	}
	// Check chained ThenAction
	if action.ThenAction != nil {
		c.collectFromAction(action.ThenAction)
	}
}

// ----------------------------------------------------------------------------
// Qualified Name Builders (used by validation and autocomplete)
// ----------------------------------------------------------------------------

// buildMicroflowQualifiedNames returns a set of all microflow qualified names in the project.
func buildMicroflowQualifiedNames(ctx *ExecContext) map[string]bool {
	result := make(map[string]bool)
	h, err := getHierarchy(ctx)
	if err != nil {
		return result
	}
	mfs, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return result
	}
	for _, mf := range mfs {
		qn := h.GetQualifiedName(mf.ContainerID, mf.Name)
		result[qn] = true
	}
	return result
}

// buildNanoflowQualifiedNames returns a set of all nanoflow qualified names in the project.
func buildNanoflowQualifiedNames(ctx *ExecContext) map[string]bool {
	result := make(map[string]bool)
	h, err := getHierarchy(ctx)
	if err != nil {
		return result
	}
	nfs, err := ctx.Backend.ListNanoflows()
	if err != nil {
		return result
	}
	for _, nf := range nfs {
		qn := h.GetQualifiedName(nf.ContainerID, nf.Name)
		result[qn] = true
	}
	return result
}

// buildPageQualifiedNames returns a set of all page qualified names in the project.
func buildPageQualifiedNames(ctx *ExecContext) map[string]bool {
	result := make(map[string]bool)
	h, err := getHierarchy(ctx)
	if err != nil {
		return result
	}
	pgs, err := ctx.Backend.ListPages()
	if err != nil {
		return result
	}
	for _, p := range pgs {
		qn := h.GetQualifiedName(p.ContainerID, p.Name)
		result[qn] = true
	}
	return result
}

// buildSnippetQualifiedNames returns a set of all snippet qualified names in the project.
func buildSnippetQualifiedNames(ctx *ExecContext) map[string]bool {
	result := make(map[string]bool)
	h, err := getHierarchy(ctx)
	if err != nil {
		return result
	}
	snippets, err := ctx.Backend.ListSnippets()
	if err != nil {
		return result
	}
	for _, s := range snippets {
		qn := h.GetQualifiedName(s.ContainerID, s.Name)
		result[qn] = true
	}
	return result
}

// buildEntityQualifiedNames returns a set of all entity qualified names in the project.
func buildEntityQualifiedNames(ctx *ExecContext) map[string]bool {
	result := make(map[string]bool)
	modules, err := getModulesFromCache(ctx)
	if err != nil {
		return result
	}
	moduleNames := make(map[model.ID]string)
	for _, m := range modules {
		moduleNames[m.ID] = m.Name
	}
	dms, err := ctx.Backend.ListDomainModels()
	if err != nil {
		return result
	}
	for _, dm := range dms {
		modName := moduleNames[dm.ContainerID]
		if modName == "" {
			continue
		}
		for _, ent := range dm.Entities {
			result[modName+"."+ent.Name] = true
		}
	}
	return result
}

// buildEntityEnumAttrMap returns a map of bare attribute name → enumeration qualified name
// for all enum-typed attributes on the given entity (e.g. "Status" → "Module.OrderStatus").
// Returns an empty map if the entity is not found or has no enum attributes.
func buildEntityEnumAttrMap(ctx *ExecContext, entityQN string) map[string]string {
	result := make(map[string]string)
	if !ctx.Connected() || entityQN == "" {
		return result
	}
	parts := strings.SplitN(entityQN, ".", 2)
	if len(parts) != 2 {
		return result
	}
	mod, err := findModule(ctx, parts[0])
	if err != nil {
		return result
	}
	dms, err := ctx.Backend.ListDomainModels()
	if err != nil {
		return result
	}
	for _, dm := range dms {
		if dm.ContainerID != mod.ID {
			continue
		}
		for _, ent := range dm.Entities {
			if ent.Name != parts[1] {
				continue
			}
			for _, attr := range ent.Attributes {
				if enumType, ok := attr.Type.(*domainmodel.EnumerationAttributeType); ok {
					result[attr.Name] = enumType.EnumerationRef
				}
			}
			return result
		}
	}
	return result
}

// buildJavaActionQualifiedNames returns a set of all java action qualified names in the project.
func buildJavaActionQualifiedNames(ctx *ExecContext) map[string]bool {
	result := make(map[string]bool)
	h, err := getHierarchy(ctx)
	if err != nil {
		return result
	}
	jas, err := ctx.Backend.ListJavaActions()
	if err != nil {
		return result
	}
	for _, ja := range jas {
		qn := h.GetQualifiedName(ja.ContainerID, ja.Name)
		result[qn] = true
	}
	return result
}

func buildJavaScriptActionQualifiedNames(ctx *ExecContext) map[string]bool {
	result := make(map[string]bool)
	h, err := getHierarchy(ctx)
	if err != nil {
		return result
	}
	jsas, err := ctx.Backend.ListJavaScriptActions()
	if err != nil {
		return result
	}
	for _, jsa := range jsas {
		qn := h.GetQualifiedName(jsa.ContainerID, jsa.Name)
		result[qn] = true
	}
	return result
}

// ----------------------------------------------------------------------------
// Executor method wrappers (for callers in unmigrated files)
// ----------------------------------------------------------------------------

// ----------------------------------------------------------------------------
// Data Type Conversion
// ----------------------------------------------------------------------------

func convertDataType(dt ast.DataType) domainmodel.AttributeType {
	switch dt.Kind {
	case ast.TypeString:
		return &domainmodel.StringAttributeType{Length: dt.Length}
	case ast.TypeInteger:
		return &domainmodel.IntegerAttributeType{}
	case ast.TypeLong:
		return &domainmodel.LongAttributeType{}
	case ast.TypeDecimal:
		return &domainmodel.DecimalAttributeType{}
	case ast.TypeBoolean:
		return &domainmodel.BooleanAttributeType{}
	case ast.TypeDateTime:
		return &domainmodel.DateTimeAttributeType{LocalizeDate: true}
	case ast.TypeDate:
		return &domainmodel.DateAttributeType{}
	case ast.TypeAutoNumber:
		return &domainmodel.AutoNumberAttributeType{}
	case ast.TypeBinary:
		return &domainmodel.BinaryAttributeType{}
	case ast.TypeEnumeration:
		enumRef := ""
		if dt.EnumRef != nil {
			enumRef = dt.EnumRef.String()
		}
		return &domainmodel.EnumerationAttributeType{EnumerationRef: enumRef}
	default:
		return &domainmodel.StringAttributeType{Length: 200}
	}
}

func getAttributeTypeName(at domainmodel.AttributeType) string {
	if at == nil {
		return "Unknown"
	}
	switch t := at.(type) {
	case *domainmodel.StringAttributeType:
		if t.Length > 0 {
			return fmt.Sprintf("String(%d)", t.Length)
		}
		return "String(unlimited)"
	case *domainmodel.IntegerAttributeType:
		return "Integer"
	case *domainmodel.LongAttributeType:
		return "Long"
	case *domainmodel.DecimalAttributeType:
		return "Decimal"
	case *domainmodel.BooleanAttributeType:
		return "Boolean"
	case *domainmodel.DateTimeAttributeType:
		return "DateTime"
	case *domainmodel.DateAttributeType:
		return "Date"
	case *domainmodel.AutoNumberAttributeType:
		return "AutoNumber"
	case *domainmodel.BinaryAttributeType:
		return "Binary"
	case *domainmodel.EnumerationAttributeType:
		// Prefer EnumerationRef (qualified name), fall back to EnumerationID
		if t.EnumerationRef != "" {
			return fmt.Sprintf("Enumeration(%s)", t.EnumerationRef)
		}
		if t.EnumerationID != "" {
			return fmt.Sprintf("Enumeration(%s)", t.EnumerationID)
		}
		return "Enumeration"
	default:
		return "Unknown"
	}
}

func formatAttributeType(at domainmodel.AttributeType) string {
	return getAttributeTypeName(at)
}
