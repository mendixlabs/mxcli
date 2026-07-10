// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	genDm "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/domainmodels"
	genProj "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/projects"
	genSec "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/security"
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
)

func init() {
	// A freshly-created module's contained units carry empty mandatory collections
	// Studio Pro always serializes (verified against the legacy writer + real BSON).
	codec.RegisterTypeDefaults("DomainModels$DomainModel", codec.TypeDefaults{
		MandatoryLists: []string{"Annotations", "Entities", "Associations", "CrossAssociations"},
	})
	codec.RegisterTypeDefaults("Security$ModuleSecurity", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"ModuleRoles": 1}, // by-name role list
	})
	codec.RegisterTypeDefaults("Projects$ModuleSettings", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"JarDependencies": 2},
	})
}

// CreateModule creates a new module and its mandatory contained units (an empty
// domain model, module security, and module settings), mirroring the legacy
// writer. Modules live under the project root.
func (b *Backend) CreateModule(m *model.Module) error {
	if m == nil {
		return fmt.Errorf("CreateModule: nil module")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateModule: not connected for writing")
	}
	if m.ID == "" {
		m.ID = model.ID(mmpr.GenerateID())
	}
	rootID, err := b.reader.GetProjectRootID()
	if err != nil {
		return fmt.Errorf("CreateModule: project root: %w", err)
	}
	enc := &codec.Encoder{}

	// Append at the end of the module list (NewSortIndex is a display order).
	sortIdx := float64(len(b.mustListModules()))

	// 1. ModuleImpl (the module unit itself).
	gm := genProj.NewModule()
	gm.SetID(element.ID(m.ID))
	gm.SetName(m.Name)
	gm.SetFromAppStore(m.FromAppStore)
	gm.SetAppStoreGuid(m.AppStoreGuid)
	gm.SetAppStoreVersion(m.AppStoreVersion)
	gm.SetAppStoreVersionGuid("")
	gm.SetAppStorePackageIdString("")
	gm.SetIsThemeModule(false)
	gm.SetSortIndex(sortIdx)
	contents, err := enc.Encode(gm)
	if err != nil {
		return fmt.Errorf("CreateModule: encode module: %w", err)
	}
	if err := b.writer.InsertUnit(string(m.ID), rootID, "Modules", "Projects$ModuleImpl", contents); err != nil {
		return fmt.Errorf("CreateModule: insert module: %w", err)
	}

	// 2. Empty domain model.
	if err := b.insertChildUnit(enc, m.ID, "DomainModel", "DomainModels$DomainModel", func() element.Element {
		d := genDm.NewDomainModel()
		d.SetDocumentation("")
		return d
	}); err != nil {
		return err
	}

	// 3. Module security (empty role list).
	if err := b.insertChildUnit(enc, m.ID, "ModuleSecurity", "Security$ModuleSecurity", func() element.Element {
		return genSec.NewModuleSecurity()
	}); err != nil {
		return err
	}

	// 4. Module settings (Studio Pro defaults for a new module).
	if err := b.insertChildUnit(enc, m.ID, "ModuleSettings", "Projects$ModuleSettings", func() element.Element {
		s := genProj.NewModuleSettings()
		s.SetExportLevel("Source")
		s.SetProtectedModuleType("AddOn")
		s.SetVersion("1.0.0")
		s.SetSolutionIdentifier("")
		s.SetExtensionName("")
		s.SetBasedOnVersion("")
		return s
	}); err != nil {
		return err
	}
	return nil
}

// UpdateModule rewrites an existing module unit. The read path (GetModule) only
// surfaces Name+ID, so a rename is the only meaningful change; we decode the gen
// module and set Name alone, preserving FromAppStore / SortIndex / IsThemeModule
// and every other field the semantic model does not carry (ADR-0005: mutate only
// what changed, never round-trip through a lossy model). References to the old
// qualified name are fixed separately via UpdateQualifiedNameInAllUnits.
func (b *Backend) UpdateModule(m *model.Module) error {
	if m == nil {
		return fmt.Errorf("UpdateModule: nil module")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateModule: not connected for writing")
	}
	if m.ID == "" {
		return fmt.Errorf("UpdateModule: module has no ID")
	}
	raw, err := b.reader.GetRawUnitBytes(string(m.ID))
	if err != nil {
		return fmt.Errorf("UpdateModule: read unit %s: %w", m.ID, err)
	}
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		return fmt.Errorf("UpdateModule: decode unit %s: %w", m.ID, err)
	}
	gm, ok := el.(*genProj.Module)
	if !ok {
		return fmt.Errorf("UpdateModule: unit %s is not a Module (%s)", m.ID, el.TypeName())
	}
	gm.SetName(m.Name)
	return b.persistUnit(m.ID, gm)
}

// DeleteModule deletes a module and every unit transitively contained in it
// (DomainModel, ModuleSecurity, ModuleSettings, Folders, Documents). Orphaned
// child units would crash Studio Pro (KeyNotFoundException in LoadChildUnits), so
// descendants are removed first, then the module unit itself.
func (b *Backend) DeleteModule(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteModule: not connected for writing")
	}
	descendants, err := b.collectDescendantUnitIDs(string(id))
	if err != nil {
		return err
	}
	for _, child := range descendants {
		if err := b.writer.DeleteUnit(child); err != nil {
			return fmt.Errorf("DeleteModule: delete child %s: %w", child, err)
		}
	}
	if err := b.writer.DeleteUnit(string(id)); err != nil {
		return fmt.Errorf("DeleteModule: delete module %s: %w", id, err)
	}
	return nil
}

// DeleteModuleWithCleanup deletes the module (and child units) and also removes
// the module's generated source directories (themesource, javasource,
// javascriptsource), whose names derive from the module name rather than its ID.
// Without this, dropping a module strands its proxies and java/javascript action
// source for a module that no longer exists in the model.
func (b *Backend) DeleteModuleWithCleanup(id model.ID, moduleName string) error {
	if err := b.DeleteModule(id); err != nil {
		return err
	}
	if b.path == "" {
		return nil
	}
	projectDir := filepath.Dir(b.path)
	lower := strings.ToLower(moduleName)
	// themesource and javasource use the lowercased module name; javascriptsource
	// uses the original casing (with a lowercase fallback).
	for _, dir := range []string{
		filepath.Join(projectDir, "themesource", lower),
		filepath.Join(projectDir, "javasource", lower),
		filepath.Join(projectDir, "javascriptsource", moduleName),
		filepath.Join(projectDir, "javascriptsource", lower),
	} {
		if stat, err := os.Stat(dir); err == nil && stat.IsDir() {
			_ = os.RemoveAll(dir)
		}
	}
	return nil
}

// collectDescendantUnitIDs returns every unit ID transitively contained under
// parentID (excluding parentID itself), walking the ContainerID hierarchy.
func (b *Backend) collectDescendantUnitIDs(parentID string) ([]string, error) {
	units, err := b.reader.ListUnits()
	if err != nil {
		return nil, fmt.Errorf("DeleteModule: list units: %w", err)
	}
	childrenOf := make(map[string][]string, len(units))
	for _, u := range units {
		if u.ID == u.ContainerID {
			continue // a unit that contains itself (project root) — skip self-loop
		}
		childrenOf[u.ContainerID] = append(childrenOf[u.ContainerID], u.ID)
	}
	var out []string
	queue := append([]string(nil), childrenOf[parentID]...)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		out = append(out, id)
		queue = append(queue, childrenOf[id]...)
	}
	return out, nil
}

// insertChildUnit builds, encodes, and inserts a fresh-ID'd unit contained in the
// module under the given containment name.
func (b *Backend) insertChildUnit(enc *codec.Encoder, moduleID model.ID, containment, unitType string, build func() element.Element) error {
	id := mmpr.GenerateID()
	el := build()
	if ider, ok := el.(interface{ SetID(element.ID) }); ok {
		ider.SetID(element.ID(id))
	}
	contents, err := enc.Encode(el)
	if err != nil {
		return fmt.Errorf("CreateModule: encode %s: %w", unitType, err)
	}
	if err := b.writer.InsertUnit(id, string(moduleID), containment, unitType, contents); err != nil {
		return fmt.Errorf("CreateModule: insert %s: %w", unitType, err)
	}
	return nil
}

func (b *Backend) mustListModules() []*model.Module {
	mods, err := b.ListModules()
	if err != nil {
		return nil
	}
	return mods
}
