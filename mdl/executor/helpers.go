// SPDX-License-Identifier: Apache-2.0

// Package executor - Shared helper functions for module/folder resolution,
// reference validation, and data type conversion.
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/folderpath"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ----------------------------------------------------------------------------
// Module and Folder Resolution
// ----------------------------------------------------------------------------

// getModulesFromCache returns cached modules or loads them.
func getModulesFromCache(ctx *ExecContext) ([]*model.Module, error) {
	if ctx.Cache == nil {
		return ctx.Backend.ListModules()
	}
	return ctx.Cache.cachedModules(ctx.Backend.ListModules)
}

// cachedModules returns the cached module list, filling it with list on first
// use.
//
// The list is filled lazily, and `refresh catalog full source` reaches this
// from a pool of goroutines sharing one executorCache (every describeEntity
// calls findModule). The fill is therefore made under modulesMu: unguarded,
// one worker published its freshly built slice while another read the field,
// which the race detector reported twice per refresh (ako/mxcli#765). The
// slice is never mutated once published, so returning it after the lock is
// released is safe.
func (c *executorCache) cachedModules(list func() ([]*model.Module, error)) ([]*model.Module, error) {
	c.modulesMu.Lock()
	defer c.modulesMu.Unlock()
	if c.modules != nil {
		return c.modules, nil
	}
	modules, err := list()
	if err != nil {
		return nil, err
	}
	c.modules = modules
	return modules, nil
}

// invalidateModuleCache clears the module cache so next lookup gets fresh data.
// Also invalidates the hierarchy cache since new modules affect hierarchy.
func invalidateModuleCache(ctx *ExecContext) {
	if c := ctx.Cache; c != nil {
		c.modulesMu.Lock()
		c.modules = nil
		c.modulesMu.Unlock()
		c.hierarchy = nil
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

	currentContainerID := moduleID

	for _, part := range folderpath.Split(folderPath) {

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
//
// It defers to findEnumeration rather than matching containers itself. The
// second implementation this used to carry compared `enum.ContainerID ==
// module.ID`, which only ever matches an enumeration sitting directly in the
// module root: one inside a FOLDER has the folder as its container, so
// `mxcli check --references` reported it missing while DESCRIBE, SHOW, ALTER
// and mxbuild all resolved it — a false negative on a script that executes
// cleanly (mendixlabs/mxcli#1071).
//
// That is the same defect upstream #976 fixed in DROP, which patched the one
// reported command and left the other caller of this question behind. Deleting
// the duplicate is the point of this change: two functions answering "does this
// enumeration exist" is what let them drift, and only one of them was exercised
// by the commands people run interactively. findEnumeration also prefers a live
// enumeration over an excluded twin of the same name (#914), which the copy did
// not do.
func enumerationExists(ctx *ExecContext, qualifiedName string) bool {
	if !ctx.Connected() {
		return false
	}

	// Parse the qualified name to get module and enum name
	parts := strings.Split(qualifiedName, ".")
	if len(parts) != 2 {
		return false
	}

	return findEnumeration(ctx, parts[0], parts[1]) != nil
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
			// sc.nanoflows is the same-script exemption every branch here
			// applies. It was the one branch that did not, so a snippet button
			// bound to a nanoflow created three statements earlier was reported
			// as missing — under the line that says such references are skipped
			// (ledger #138).
			if !known[ref] && !sc.nanoflows[ref] {
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

	if len(refs.menus) > 0 {
		known := buildMenuDocumentQualifiedNames(ctx)
		for _, ref := range refs.menus {
			if known != nil && !known[ref] && !sc.menus[ref] {
				errors = append(errors, menuNotFoundMessage(ref, known))
			}
		}
	}

	if len(refs.images) > 0 {
		// The project's images plus the ones this script creates
		// (mendixlabs/mxcli#1419). A nil set means the project could not be read,
		// and stays nil: imageRefErrors then reports nothing rather than calling
		// every other collection missing.
		known, collections := buildImageQualifiedNames(ctx)
		if known != nil && collections != nil {
			for qn := range sc.imageCollections {
				collections[qn] = true
			}
			for qn := range sc.images {
				known[qn] = true
			}
		}
		errors = append(errors, imageRefErrors(refs.images, known, collections)...)
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
	images     []string
	menus      []string
	// dataSources holds the subset of the references above that a widget's
	// DATA SOURCE names (flow or entity). An excluded document may keep a
	// dangling action target, but not a dangling data source — see
	// splitExcludedWidgetRefs.
	dataSources map[string]bool
}

func (c *widgetRefCollector) addDataSource(ref string) {
	if c.dataSources == nil {
		c.dataSources = map[string]bool{}
	}
	c.dataSources[ref] = true
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
	c.images = uniqueStrings(c.images)
	c.menus = uniqueStrings(c.menus)
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
		len(c.pages) == 0 && len(c.snippets) == 0 && len(c.entities) == 0 &&
		len(c.images) == 0 && len(c.menus) == 0
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
				c.addDataSource(ds.Reference)
			}
		case "nanoflow":
			if ds.Reference != "" {
				c.nanoflows = append(c.nanoflows, ds.Reference)
				c.addDataSource(ds.Reference)
			}
		case "database":
			if ds.Reference != "" {
				c.entities = append(c.entities, ds.Reference)
				c.addDataSource(ds.Reference)
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

	// An image-collection entry, Module.Collection.Image. Which property holds
	// one is a property of the WIDGET, so the table decides — `Image` on a
	// widget that is not in it is not a collection reference and must not be
	// resolved as one.
	for _, key := range imageRefProps[strings.ToLower(w.Type)] {
		if img := strings.TrimSpace(w.GetStringProp(key)); img != "" {
			c.images = append(c.images, img)
		}
	}

	// A menu widget's menu document (`Menu: Module.Menu`). Only on the three
	// menu widgets: `Menu` means nothing on any other.
	switch strings.ToLower(w.Type) {
	case "navigationtree", "menubar", "simplemenubar":
		if m := strings.TrimSpace(w.GetStringProp("Menu")); m != "" {
			c.menus = append(c.menus, m)
		}
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

// buildMenuDocumentQualifiedNames returns the project's menu documents by
// qualified name, or nil when they could not be listed — then nothing is
// reported, rather than every reference being called missing.
func buildMenuDocumentQualifiedNames(ctx *ExecContext) map[string]bool {
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil
	}
	mds, err := ctx.Backend.ListMenuDocuments()
	if err != nil {
		return nil
	}
	result := make(map[string]bool, len(mds))
	for _, md := range mds {
		result[h.GetQualifiedName(md.ContainerID, md.Name)] = true
	}
	return result
}

// menuNotFoundMessage names the menu documents that do exist, since MDL has no
// statement that lists them.
func menuNotFoundMessage(ref string, known map[string]bool) string {
	names := make([]string, 0, len(known))
	for qn := range known {
		names = append(names, qn)
	}
	sort.Strings(names)
	const max = 10
	if len(names) > max {
		names = append(names[:max:max], "…")
	}
	if len(names) == 0 {
		return fmt.Sprintf("menu document not found: %s — the project has none; create one with `create menu`", ref)
	}
	return fmt.Sprintf("menu document not found: %s — the project has: %s", ref, strings.Join(names, ", "))
}

// buildMicroflowQualifiedNames returns a set of all microflow qualified names in the project.
func buildMicroflowQualifiedNames(ctx *ExecContext) map[string]bool {
	result := make(map[string]bool)
	h, err := getHierarchy(ctx)
	if err != nil {
		return result
	}
	mfs, err := microflowHeaders(ctx)
	if err != nil {
		return result
	}
	for _, mf := range mfs {
		qn := h.GetQualifiedName(mf.ContainerID, mf.Name)
		result[qn] = true
	}
	return result
}

// microflowLookup is a backend that can find microflows without decoding
// every one of them (the modelsdk backend).
type microflowLookup interface {
	ListMicroflowsNamed(name string) ([]*microflows.Microflow, error)
	ListMicroflowHeaders() ([]types.DocumentHeader, error)
}

// microflowsNamed is the project's microflows whose local name is name, in
// every module. A caller that looks one microflow up needs no other: the
// modelsdk backend decodes only these, where ListMicroflows decoded the whole
// flow of every microflow in the project — for a lookup check makes several
// times per statement (mendixlabs/mxcli#1272).
func microflowsNamed(ctx *ExecContext, name string) ([]*microflows.Microflow, error) {
	if l, ok := ctx.Backend.(microflowLookup); ok {
		return l.ListMicroflowsNamed(name)
	}
	all, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return nil, err
	}
	var out []*microflows.Microflow
	for _, mf := range all {
		if mf.Name == name {
			out = append(out, mf)
		}
	}
	return out, nil
}

// microflowHeaders is every microflow's ID, container, name and Excluded flag,
// for a name set or an ID-to-name map: no flow is decoded where the backend
// can avoid it.
func microflowHeaders(ctx *ExecContext) ([]types.DocumentHeader, error) {
	if l, ok := ctx.Backend.(microflowLookup); ok {
		return l.ListMicroflowHeaders()
	}
	all, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return nil, err
	}
	out := make([]types.DocumentHeader, len(all))
	for i, mf := range all {
		out[i] = types.DocumentHeader{ID: mf.ID, ContainerID: mf.ContainerID, Name: mf.Name, Excluded: mf.Excluded}
	}
	return out, nil
}

// buildMicroflowReturnTypes maps each stored microflow's qualified name to the
// name of its return type ("Boolean", "String", …), or "" for a microflow that
// returns nothing.
//
// Separate from buildMicroflowQualifiedNames rather than folded into it: that
// set answers "does this name resolve", which several callers want and which
// must keep working when the return type is unavailable. A microflow missing
// here is a microflow whose type nothing may conclude anything from.
func buildMicroflowReturnTypes(ctx *ExecContext) map[string]string {
	result := make(map[string]string)
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
		if mf.ReturnType == nil {
			result[qn] = ""
			continue
		}
		result[qn] = mf.ReturnType.GetTypeName()
	}
	return result
}

// buildQueueQualifiedNames returns the set of task queue qualified names in the
// project, lower-cased — Mendix name resolution is case-insensitive and the
// caller compares an author-written name against it.
func buildQueueQualifiedNames(ctx *ExecContext) map[string]bool {
	result := make(map[string]bool)
	h, err := getHierarchy(ctx)
	if err != nil {
		return result
	}
	queues, err := ctx.Backend.ListQueues()
	if err != nil {
		return result
	}
	for _, q := range queues {
		result[strings.ToLower(h.GetQualifiedName(q.ContainerID, q.Name))] = true
	}
	return result
}

// buildImportMappingQualifiedNames returns the set of import mapping qualified
// names in the project, matched exactly as GetImportMappingByQualifiedName does.
func buildImportMappingQualifiedNames(ctx *ExecContext) map[string]bool {
	result := make(map[string]bool)
	h, err := getHierarchy(ctx)
	if err != nil {
		return result
	}
	ims, err := ctx.Backend.ListImportMappings()
	if err != nil {
		return result
	}
	for _, im := range ims {
		result[h.GetQualifiedName(im.ContainerID, im.Name)] = true
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

// buildAssociationQualifiedNames returns a set of all association qualified names
// in the project, covering both intra-module associations and cross-module ones
// (which live on the FROM entity's domain model, so both come off the same walk).
func buildAssociationQualifiedNames(ctx *ExecContext) map[string]bool {
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
		for _, assoc := range dm.Associations {
			result[modName+"."+assoc.Name] = true
		}
		for _, ca := range dm.CrossAssociations {
			result[modName+"."+ca.Name] = true
		}
	}
	return result
}

// buildModuleRoleQualifiedNames returns the module roles a plain
// CREATE MODULE ROLE would collide with, as Module.Role.
//
// Roles mxcli auto-provisioned are deliberately EXCLUDED. `execCreateModuleRole`
// treats one of those as a hit rather than a conflict — it adopts the caller's
// casing, rewrites the references, and returns nil — so listing it here would
// make `check` refuse a script that runs fine, and refuse it over a role the
// user never asked mxcli to create (`defaultDocumentAccessRoles`). A check that
// reports a statement exec accepts is worse than the under-report it replaces.
//
// Known limit: exec matches role names case-insensitively (Mendix does, CE0123),
// while this set is keyed exactly, like every other doc type's. So
// `create module role M.admin` against a stored `M.Admin` is still reported only
// by exec. That is an under-report of a rare spelling, not a false positive, and
// closing it would mean case-folding the shared name registry for one type.
func buildModuleRoleQualifiedNames(ctx *ExecContext) map[string]bool {
	result := make(map[string]bool)
	modules, err := getModulesFromCache(ctx)
	if err != nil {
		return result
	}
	for _, m := range modules {
		ms, err := ctx.Backend.GetModuleSecurity(m.ID)
		if err != nil || ms == nil {
			continue
		}
		for _, mr := range ms.ModuleRoles {
			if mr == nil || mr.Description == autoDocumentRoleDescription {
				continue
			}
			result[m.Name+"."+mr.Name] = true
		}
	}
	return result
}

// buildRuleQualifiedNames returns a set of all rule qualified names in the project.
func buildRuleQualifiedNames(ctx *ExecContext) map[string]bool {
	result := make(map[string]bool)
	h, err := getHierarchy(ctx)
	if err != nil {
		return result
	}
	rules, err := ctx.Backend.ListRules()
	if err != nil {
		return result
	}
	for _, r := range rules {
		result[h.GetQualifiedName(r.ContainerID, r.Name)] = true
	}
	return result
}

// buildEnumerationQualifiedNames returns a set of all enumeration qualified names in the project.
func buildEnumerationQualifiedNames(ctx *ExecContext) map[string]bool {
	result := make(map[string]bool)
	h, err := getHierarchy(ctx)
	if err != nil || h == nil {
		return result
	}
	enums, err := ctx.Backend.ListEnumerations()
	if err != nil {
		return result
	}
	for _, e := range enums {
		if e == nil {
			continue
		}
		result[h.GetQualifiedName(e.ContainerID, e.Name)] = true
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
		// `not localized` is the one spelling of false; unstated is Mendix's
		// default, true. A rewrite that leaves it unstated carries the stored
		// value afterwards (carryStoredAttributeState, MODIFY ATTRIBUTE).
		return &domainmodel.DateTimeAttributeType{LocalizeDate: dt.Localize.LocalizeDate()}
	case ast.TypeDate:
		return &domainmodel.DateAttributeType{}
	case ast.TypeAutoNumber:
		return &domainmodel.AutoNumberAttributeType{}
	case ast.TypeBinary:
		return &domainmodel.BinaryAttributeType{}
	case ast.TypeHashedString:
		return &domainmodel.HashedStringAttributeType{}
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
	case *domainmodel.HashedStringAttributeType:
		return "HashedString"
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

// localizeClause renders a DateTime attribute's LocalizeDate as the constraint
// that follows its type: " not localized" when false, and nothing for the
// default, so describe only says what differs from a fresh `DateTime` (#1373).
func localizeClause(at domainmodel.AttributeType) string {
	if dt, ok := at.(*domainmodel.DateTimeAttributeType); ok && !dt.LocalizeDate {
		return " not localized"
	}
	return ""
}

// buildWorkflowQualifiedNames returns a set of all workflow qualified names in
// the project. Mirrors buildPageQualifiedNames; needed so a workflow's
// `call workflow` target can be resolved (issue #943).
func buildWorkflowQualifiedNames(ctx *ExecContext) map[string]bool {
	result := make(map[string]bool)
	h, err := getHierarchy(ctx)
	if err != nil {
		return result
	}
	wfs, err := ctx.Backend.ListWorkflows()
	if err != nil {
		return result
	}
	for _, w := range wfs {
		result[h.GetQualifiedName(w.ContainerID, w.Name)] = true
	}
	return result
}

// buildConstantQualifiedNames indexes the project's constants by qualified name,
// for the reference checks that name one (a settings override, for instance).
//
// The second return says whether the index is TRUSTWORTHY. A backend that cannot
// list constants yields an empty map, which is indistinguishable from a project
// that has none — and the difference matters: "no constants exist" makes every
// override dangling, while "cannot tell" must not reject anything. Callers that
// resolve a name check it.
func buildConstantQualifiedNames(ctx *ExecContext) (map[string]bool, bool) {
	result := make(map[string]bool)
	h, err := getHierarchy(ctx)
	if err != nil {
		return result, false
	}
	consts, err := ctx.Backend.ListConstants()
	if err != nil {
		return result, false
	}
	for _, c := range consts {
		if c == nil {
			continue
		}
		result[h.GetQualifiedName(c.ContainerID, c.Name)] = true
	}
	// A backend that has no constant listing at all reports success with nothing
	// in it; treat that as "cannot tell" rather than "the project has none".
	return result, len(consts) > 0
}

// imageRefProps maps a widget type to the properties that hold an
// image-collection entry (Module.Collection.Image).
//
// It is a table rather than a condition on the widget type because the set grew
// and the condition did not. `Image:` was wired for the pluggable `image`
// widget, and mendixlabs/mxcli#1057 then gave the SAME reference to
// `staticimage` — the widget Studio Pro puts in a Selection helper's custom
// states, which is the whole reason #1057 exists — and to `dynamicimage`'s
// fallback. Neither was collected, so a name that does not resolve passed
// `mxcli check --references` and failed the build (mendixlabs/mxcli#1149):
//
//	[error] [CE1613] "The selected image 'Atlas_UI_Resources.Atlas_Icons.
//	        checkbox_checked' no longer exists."   at Static image 'imgAll'
//
// Adding a widget that names an image means adding a row here. The keys are the
// ones `validate_widgets.go` already accepts for these widgets and DESCRIBE
// already emits, so the two lists say the same thing about the same property.
var imageRefProps = map[string][]string{
	"image":        {"Image", "DefaultImage"},
	"staticimage":  {"Image"},
	"dynamicimage": {"DefaultImage"},
}
