// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
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
// including all JarDependencies and their Exclusions (marker-2 arrays). It
// writes onto the stored unit, so the keys mxcli does not model keep their
// values (#1920).
func (b *Backend) UpdateModuleSettings(ms *types.ModuleSettings) error {
	if ms == nil {
		return fmt.Errorf("UpdateModuleSettings: nil module settings")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateModuleSettings: not connected for writing")
	}
	stored, err := b.reader.GetRawUnitBytes(string(ms.ID))
	if err != nil {
		return fmt.Errorf("UpdateModuleSettings: read stored settings: %w", err)
	}
	contents, err := moduleSettingsContents(stored, ms)
	if err != nil {
		return err
	}
	return b.writer.UpdateRawUnit(string(ms.ID), contents)
}

// moduleSettingsContents is the stored settings unit with the keys ms models
// written into it.
//
// It starts from the stored keys, not from an empty document: the unit also
// holds PackageId (Studio Pro's Module ID), a Marketplace module's Checksum,
// EnableDetailedTroubleshooting and more that mxcli does not model. A rebuild
// from the modelled keys alone had the write path fill those with defaults, so
// a jar dependency change wiped the Module ID and the Checksum and switched
// detailed troubleshooting on (#1920).
func moduleSettingsContents(stored []byte, ms *types.ModuleSettings) ([]byte, error) {
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

	doc := bson.M{}
	if len(stored) > 0 {
		if err := bson.Unmarshal(stored, &doc); err != nil {
			return nil, fmt.Errorf("UpdateModuleSettings: decode stored settings: %w", err)
		}
	}
	for k, v := range map[string]any{
		"$ID":                 bsonutil.IDToBsonBinary(string(ms.ID)),
		"$Type":               "Projects$ModuleSettings",
		"BasedOnVersion":      ms.BasedOnVersion,
		"ExportLevel":         orDefault(ms.ExportLevel, "Source"),
		"ExtensionName":       ms.ExtensionName,
		"JarDependencies":     deps,
		"ProtectedModuleType": orDefault(ms.ProtectedModuleType, "AddOn"),
		"SolutionIdentifier":  ms.SolutionIdentifier,
		"Version":             orDefault(ms.Version, "1.0.0"),
	} {
		doc[k] = v
	}
	// Mendix 11.12+ requires "$ID" first in every storage object; doc and its
	// nested JarDependency/Exclusion maps are unordered, so order on write.
	contents, err := bson.Marshal(bsonutil.OrderStorageValue(doc))
	if err != nil {
		return nil, fmt.Errorf("UpdateModuleSettings: marshal: %w", err)
	}
	return contents, nil
}

// moduleSettingsID returns a binary-UUID $ID for a jar-dependency / exclusion,
// generating a fresh one when the model carries no ID.
func moduleSettingsID(id model.ID) any {
	if id != "" {
		return bsonutil.IDToBsonBinary(string(id))
	}
	return bsonutil.NewIDBsonBinary()
}
