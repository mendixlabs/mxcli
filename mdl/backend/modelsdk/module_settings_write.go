// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/JordtenBulte-OLC/mxcli/mdl/bsonutil"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// GetModuleSettings returns the Projects$ModuleSettings for a module (found by
// the settings unit's container == moduleID). Returns an error when the module
// has no settings unit, matching the legacy read contract.
func (b *Backend) GetModuleSettings(moduleID model.ID) (*types.ModuleSettings, error) {
	all, err := b.ListModuleSettings()
	if err != nil {
		return nil, err
	}
	for _, ms := range all {
		if ms.ContainerID == moduleID {
			return ms, nil
		}
	}
	return nil, fmt.Errorf("module settings not found for module: %s", moduleID)
}

// UpdateModuleSettings rewrites the Projects$ModuleSettings unit in place,
// including all JarDependencies and their Exclusions. Mirrors the legacy
// serializer (pure BSON; JarDependencies/Exclusions use the marker-2 array form).
func (b *Backend) UpdateModuleSettings(ms *types.ModuleSettings) error {
	if ms == nil {
		return fmt.Errorf("UpdateModuleSettings: nil module settings")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateModuleSettings: not connected for writing")
	}

	deps := bson.A{int32(2)} // listType marker
	for _, d := range ms.JarDependencies {
		depDoc := bson.M{
			"$ID":        moduleSettingsID(d.ID),
			"$Type":      "Projects$JarDependency",
			"GroupId":    d.GroupID,
			"ArtifactId": d.ArtifactID,
			"Version":    d.Version,
			"IsIncluded": d.IsIncluded,
		}
		if len(d.Exclusions) > 0 {
			excArr := bson.A{int32(2)}
			for _, e := range d.Exclusions {
				excArr = append(excArr, bson.M{
					"$ID":        moduleSettingsID(e.ID),
					"$Type":      "Projects$JarDependencyExclusion",
					"GroupId":    e.GroupID,
					"ArtifactId": e.ArtifactID,
				})
			}
			depDoc["Exclusions"] = excArr
		}
		deps = append(deps, depDoc)
	}

	doc := bson.M{
		"$ID":                 bsonutil.IDToBsonBinary(string(ms.ID)),
		"$Type":               "Projects$ModuleSettings",
		"BasedOnVersion":      ms.BasedOnVersion,
		"ExportLevel":         orDefault(ms.ExportLevel, "Source"),
		"ExtensionName":       ms.ExtensionName,
		"JarDependencies":     deps,
		"ProtectedModuleType": orDefault(ms.ProtectedModuleType, "AddOn"),
		"SolutionIdentifier":  ms.SolutionIdentifier,
		"Version":             orDefault(ms.Version, "1.0.0"),
	}
	// Mendix 11.12+ requires "$ID" first in every storage object; doc and its
	// nested JarDependency/Exclusion maps are unordered, so order on write.
	contents, err := bson.Marshal(bsonutil.OrderStorageValue(doc))
	if err != nil {
		return fmt.Errorf("UpdateModuleSettings: marshal: %w", err)
	}
	return b.writer.UpdateRawUnit(string(ms.ID), contents)
}

// moduleSettingsID returns a binary-UUID $ID for a jar-dependency / exclusion,
// generating a fresh one when the model carries no ID.
func moduleSettingsID(id model.ID) any {
	if id != "" {
		return bsonutil.IDToBsonBinary(string(id))
	}
	return bsonutil.NewIDBsonBinary()
}
