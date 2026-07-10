// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"

	"go.mongodb.org/mongo-driver/bson"
)

// CreateModule creates a new module in the project.
// This also creates the associated domain model for the module.
func (w *Writer) CreateModule(module *model.Module) error {
	if module.ID == "" {
		module.ID = model.ID(generateUUID())
	}
	module.TypeName = "Projects$ModuleImpl"

	// Get project root ID - modules are contained in the project root
	projectRootID, err := w.reader.GetProjectRootID()
	if err != nil {
		return fmt.Errorf("failed to get project root: %w", err)
	}

	// Serialize and insert module
	contents, err := w.serializeModule(module)
	if err != nil {
		return fmt.Errorf("failed to serialize module: %w", err)
	}

	if err := w.insertUnit(string(module.ID), projectRootID, "Modules", "Projects$ModuleImpl", contents); err != nil {
		return fmt.Errorf("failed to insert module unit: %w", err)
	}

	// Create empty domain model for the module
	dmID := generateUUID()
	dm := &domainmodel.DomainModel{
		ContainerID: module.ID,
	}
	dm.ID = model.ID(dmID)
	dm.TypeName = "DomainModels$DomainModel"

	dmContents, err := w.serializeDomainModel(dm)
	if err != nil {
		return fmt.Errorf("failed to serialize domain model: %w", err)
	}

	if err := w.insertUnit(dmID, string(module.ID), "DomainModel", "DomainModels$DomainModel", dmContents); err != nil {
		return fmt.Errorf("failed to insert domain model unit: %w", err)
	}

	// Create empty module security for the module
	msID := generateUUID()
	msContents, err := w.serializeModuleSecurity(msID)
	if err != nil {
		return fmt.Errorf("failed to serialize module security: %w", err)
	}

	if err := w.insertUnit(msID, string(module.ID), "ModuleSecurity", "Security$ModuleSecurity", msContents); err != nil {
		return fmt.Errorf("failed to insert module security unit: %w", err)
	}

	// Create module settings for the module
	settingsID := generateUUID()
	settingsContents, err := w.serializeModuleSettings(settingsID)
	if err != nil {
		return fmt.Errorf("failed to serialize module settings: %w", err)
	}

	if err := w.insertUnit(settingsID, string(module.ID), "ModuleSettings", "Projects$ModuleSettings", settingsContents); err != nil {
		return fmt.Errorf("failed to insert module settings unit: %w", err)
	}

	return nil
}

// UpdateModule updates an existing module.
func (w *Writer) UpdateModule(module *model.Module) error {
	contents, err := w.serializeModule(module)
	if err != nil {
		return fmt.Errorf("failed to serialize module: %w", err)
	}

	return w.updateUnit(string(module.ID), contents)
}

// DeleteModule deletes a module and all its child units (DomainModel, ModuleSecurity,
// ModuleSettings, Folders, Documents). This prevents orphaned units which cause
// Studio Pro to crash with KeyNotFoundException in UnitLoader.LoadChildUnits.
func (w *Writer) DeleteModule(id model.ID) error {
	if err := w.deleteChildUnits(string(id)); err != nil {
		return fmt.Errorf("failed to delete child units: %w", err)
	}
	return w.deleteUnit(string(id))
}

// DeleteModuleWithCleanup deletes a module and also removes its themesource directory.
// The moduleName is needed because the themesource directory name is derived from
// the module name (lowercased), not the module ID.
func (w *Writer) DeleteModuleWithCleanup(id model.ID, moduleName string) error {
	if err := w.DeleteModule(id); err != nil {
		return err
	}

	// Remove the module's generated source directories. themesource and
	// javasource use the lowercased module name; javascriptsource uses the
	// original casing (with a lowercase fallback). Studio Pro deletes these when a
	// module is removed; leaving them strands proxies/actions for a module that no
	// longer exists in the model.
	projectDir := filepath.Dir(w.reader.path)
	removeModuleSourceDirs(projectDir, moduleName)

	return nil
}

// removeModuleSourceDirs deletes the themesource/javasource/javascriptsource
// directories belonging to a module. Mirrored by the modelsdk backend's
// DeleteModuleWithCleanup.
func removeModuleSourceDirs(projectDir, moduleName string) {
	lower := strings.ToLower(moduleName)
	dirs := []string{
		filepath.Join(projectDir, "themesource", lower),
		filepath.Join(projectDir, "javasource", lower),
		filepath.Join(projectDir, "javascriptsource", moduleName),
		filepath.Join(projectDir, "javascriptsource", lower),
	}
	for _, dir := range dirs {
		if stat, err := os.Stat(dir); err == nil && stat.IsDir() {
			os.RemoveAll(dir)
		}
	}
}

// deleteChildUnits recursively deletes all units whose ContainerID matches the given parent.
func (w *Writer) deleteChildUnits(parentID string) error {
	parentBlob := uuidToBlob(parentID)
	if parentBlob == nil {
		return fmt.Errorf("invalid parent ID: %s", parentID)
	}

	// Find all child units
	rows, err := w.reader.db.Query("SELECT UnitID FROM Unit WHERE ContainerID = ? AND UnitID != ContainerID", parentBlob)
	if err != nil {
		return err
	}
	defer rows.Close()

	var childIDs []string
	for rows.Next() {
		var childBlob []byte
		if err := rows.Scan(&childBlob); err != nil {
			return err
		}
		childIDs = append(childIDs, blobToUUID(childBlob))
	}

	// Recursively delete children of children first (depth-first)
	for _, childID := range childIDs {
		if err := w.deleteChildUnits(childID); err != nil {
			return err
		}
		if err := w.deleteUnit(childID); err != nil {
			return err
		}
	}

	return nil
}

// CreateFolder creates a new folder in the project.
func (w *Writer) CreateFolder(folder *model.Folder) error {
	if folder.ID == "" {
		folder.ID = model.ID(generateUUID())
	}
	folder.TypeName = "Projects$Folder"

	// Serialize and insert folder
	contents, err := w.serializeFolder(folder)
	if err != nil {
		return fmt.Errorf("failed to serialize folder: %w", err)
	}

	if err := w.insertUnit(string(folder.ID), string(folder.ContainerID), "Folders", "Projects$Folder", contents); err != nil {
		return fmt.Errorf("failed to insert folder unit: %w", err)
	}

	return nil
}

// serializeFolder serializes a folder to BSON.
func (w *Writer) serializeFolder(folder *model.Folder) ([]byte, error) {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(folder.ID))},
		{Key: "$Type", Value: "Projects$Folder"},
		{Key: "Name", Value: folder.Name},
	}

	return marshalUnitIDFirst(doc)
}

// DeleteFolder deletes a folder unit if it is empty.
// Returns an error if the folder contains any child units.
func (w *Writer) DeleteFolder(id model.ID) error {
	idStr := string(id)
	blob := uuidToBlob(idStr)
	if blob == nil {
		return fmt.Errorf("invalid folder ID: %s", idStr)
	}

	var count int
	err := w.reader.db.QueryRow(
		"SELECT COUNT(*) FROM Unit WHERE ContainerID = ? AND UnitID != ContainerID",
		blob,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check folder contents: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("folder is not empty: contains %d child unit(s)", count)
	}

	return w.deleteUnit(idStr)
}

// MoveFolder moves a folder to a new container (folder or module root).
func (w *Writer) MoveFolder(id model.ID, newContainerID model.ID) error {
	return w.moveUnitByID(string(id), string(newContainerID))
}

func (w *Writer) serializeModuleSecurity(id string) ([]byte, error) {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(id)},
		{Key: "$Type", Value: "Security$ModuleSecurity"},
		{Key: "ModuleRoles", Value: bson.A{int32(1)}},
	}
	return marshalUnitIDFirst(doc)
}

func (w *Writer) serializeModuleSettings(id string) ([]byte, error) {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(id)},
		{Key: "$Type", Value: "Projects$ModuleSettings"},
		{Key: "BasedOnVersion", Value: ""},
		{Key: "ExportLevel", Value: "Source"},
		{Key: "ExtensionName", Value: ""},
		{Key: "JarDependencies", Value: bson.A{int32(2)}},
		{Key: "ProtectedModuleType", Value: "AddOn"},
		{Key: "SolutionIdentifier", Value: ""},
		{Key: "Version", Value: "1.0.0"},
	}
	return marshalUnitIDFirst(doc)
}

// UpdateModuleSettings persists the full Projects$ModuleSettings document,
// including all JarDependencies and their Exclusions.
func (w *Writer) UpdateModuleSettings(ms *types.ModuleSettings) error {
	contents, err := w.serializeModuleSettingsFull(ms)
	if err != nil {
		return fmt.Errorf("failed to serialize module settings: %w", err)
	}
	return w.updateUnit(string(ms.ID), contents)
}

// serializeModuleSettingsFull serializes a ModuleSettings with actual dependencies.
func (w *Writer) serializeModuleSettingsFull(ms *types.ModuleSettings) ([]byte, error) {
	exportLevel := ms.ExportLevel
	if exportLevel == "" {
		exportLevel = "Source"
	}
	protectedType := ms.ProtectedModuleType
	if protectedType == "" {
		protectedType = "AddOn"
	}
	ver := ms.Version
	if ver == "" {
		ver = "1.0.0"
	}

	deps := bson.A{int32(2)} // listType marker
	for _, d := range ms.JarDependencies {
		depID := string(d.ID)
		if depID == "" {
			depID = generateUUID()
		}
		depDoc := bson.D{
			{Key: "$ID", Value: idToBsonBinary(depID)},
			{Key: "$Type", Value: "Projects$JarDependency"},
			{Key: "GroupId", Value: d.GroupID},
			{Key: "ArtifactId", Value: d.ArtifactID},
			{Key: "Version", Value: d.Version},
			{Key: "IsIncluded", Value: d.IsIncluded},
		}
		if len(d.Exclusions) > 0 {
			excArr := bson.A{int32(2)}
			for _, e := range d.Exclusions {
				excID := string(e.ID)
				if excID == "" {
					excID = generateUUID()
				}
				excArr = append(excArr, bson.D{
					{Key: "$ID", Value: idToBsonBinary(excID)},
					{Key: "$Type", Value: "Projects$JarDependencyExclusion"},
					{Key: "GroupId", Value: e.GroupID},
					{Key: "ArtifactId", Value: e.ArtifactID},
				})
			}
			depDoc = append(depDoc, bson.E{Key: "Exclusions", Value: excArr})
		}
		deps = append(deps, depDoc)
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(ms.ID))},
		{Key: "$Type", Value: "Projects$ModuleSettings"},
		{Key: "BasedOnVersion", Value: ms.BasedOnVersion},
		{Key: "ExportLevel", Value: exportLevel},
		{Key: "ExtensionName", Value: ms.ExtensionName},
		{Key: "JarDependencies", Value: deps},
		{Key: "ProtectedModuleType", Value: protectedType},
		{Key: "SolutionIdentifier", Value: ms.SolutionIdentifier},
		{Key: "Version", Value: ver},
	}
	return marshalUnitIDFirst(doc)
}

func (w *Writer) serializeModule(module *model.Module) ([]byte, error) {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(module.ID))},
		{Key: "$Type", Value: "Projects$ModuleImpl"},
		{Key: "Name", Value: module.Name},
		{Key: "FromAppStore", Value: module.FromAppStore},
		{Key: "AppStoreGuid", Value: module.AppStoreGuid},
		{Key: "AppStorePackageIdString", Value: ""},
		{Key: "AppStoreVersion", Value: module.AppStoreVersion},
		{Key: "AppStoreVersionGuid", Value: ""},
		{Key: "IsThemeModule", Value: false},
		{Key: "NewSortIndex", Value: int64(0)},
	}
	return marshalUnitIDFirst(doc)
}
