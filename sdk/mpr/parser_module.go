// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"

	"go.mongodb.org/mongo-driver/bson"
)

func (r *Reader) parseModule(unitID string, contents []byte) (*model.Module, error) {
	// For MPR v2, contents might be a reference to an external file
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	// Parse BSON contents
	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	module := &model.Module{}
	module.ID = model.ID(unitID)
	module.TypeName = "Projects$Module"

	if name, ok := raw["Name"].(string); ok {
		module.Name = name
	}
	if doc, ok := raw["Documentation"].(string); ok {
		module.Documentation = doc
	}
	if excluded, ok := raw["Excluded"].(bool); ok {
		module.Excluded = excluded
	}
	if fromAppStore, ok := raw["FromAppStore"].(bool); ok {
		module.FromAppStore = fromAppStore
	}
	if appStoreVersion, ok := raw["AppStoreVersion"].(string); ok {
		module.AppStoreVersion = appStoreVersion
	}
	if appStoreGuid, ok := raw["AppStoreGuid"].(string); ok {
		module.AppStoreGuid = appStoreGuid
	}
	if isReusable, ok := raw["IsReusableComponent"].(bool); ok {
		module.IsReusableComponent = isReusable
	}

	return module, nil
}

// parseDomainModel parses domain model contents from BSON.
