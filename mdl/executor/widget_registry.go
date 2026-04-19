// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/sdk/widgets/definitions"
)

// WidgetRegistry holds loaded widget definitions keyed by uppercase MDL name.
type WidgetRegistry struct {
	byMDLName  map[string]*WidgetDefinition // keyed by uppercase MDLName
	byWidgetID map[string]*WidgetDefinition // keyed by widgetId
}

// knownOperations is the set of operation names supported by the widget engine.
var knownOperations = map[string]bool{
	"attribute":        true,
	"association":      true,
	"primitive":        true,
	"selection":        true,
	"expression":       true,
	"datasource":       true,
	"widgets":          true,
	"texttemplate":     true,
	"action":           true,
	"attributeObjects": true,
}

// NewWidgetRegistry creates a registry pre-loaded with embedded definitions.
func NewWidgetRegistry() (*WidgetRegistry, error) {
	reg := &WidgetRegistry{
		byMDLName:  make(map[string]*WidgetDefinition),
		byWidgetID: make(map[string]*WidgetDefinition),
	}

	entries, err := definitions.EmbeddedFS.ReadDir(".")
	if err != nil {
		return nil, mdlerrors.NewBackend("read embedded definitions", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".def.json") {
			continue
		}

		data, err := definitions.EmbeddedFS.ReadFile(entry.Name())
		if err != nil {
			return nil, mdlerrors.NewBackend(fmt.Sprintf("read definition %s", entry.Name()), err)
		}

		var def WidgetDefinition
		if err := json.Unmarshal(data, &def); err != nil {
			return nil, mdlerrors.NewBackend(fmt.Sprintf("parse definition %s", entry.Name()), err)
		}

		if err := validateDefinitionOperations(&def, entry.Name()); err != nil {
			return nil, err
		}

		reg.byMDLName[strings.ToUpper(def.MDLName)] = &def
		reg.byWidgetID[def.WidgetID] = &def
	}

	return reg, nil
}

// Get returns a widget definition by MDL name (case-insensitive).
func (r *WidgetRegistry) Get(mdlName string) (*WidgetDefinition, bool) {
	def, ok := r.byMDLName[strings.ToUpper(mdlName)]
	return def, ok
}

// GetByWidgetID returns a widget definition by its full widget ID.
func (r *WidgetRegistry) GetByWidgetID(widgetID string) (*WidgetDefinition, bool) {
	def, ok := r.byWidgetID[widgetID]
	return def, ok
}

// All returns all registered definitions.
func (r *WidgetRegistry) All() []*WidgetDefinition {
	result := make([]*WidgetDefinition, 0, len(r.byMDLName))
	for _, def := range r.byMDLName {
		result = append(result, def)
	}
	return result
}

// Count returns the number of registered definitions.
func (r *WidgetRegistry) Count() int {
	return len(r.byMDLName)
}

// LoadUserDefinitions scans global and project-level directories for user-provided definitions.
// Project definitions override global ones with the same MDL name.
func (r *WidgetRegistry) LoadUserDefinitions(projectPath string) error {
	// 1. Global: ~/.mxcli/widgets/*.def.json
	homeDir, err := os.UserHomeDir()
	if err == nil {
		globalDir := filepath.Join(homeDir, ".mxcli", "widgets")
		if err := r.loadDefinitionsFromDir(globalDir); err != nil {
			return fmt.Errorf("global widgets: %w", err)
		}
	} else {
		log.Printf("warning: cannot determine home directory for user widget definitions: %v", err)
	}

	// 2. Project: <projectDir>/.mxcli/widgets/*.def.json (overrides global)
	if projectPath != "" {
		projectDir := filepath.Dir(projectPath)
		localDir := filepath.Join(projectDir, ".mxcli", "widgets")
		if err := r.loadDefinitionsFromDir(localDir); err != nil {
			return fmt.Errorf("project widgets: %w", err)
		}
	}

	return nil
}

// loadDefinitionsFromDir loads all .def.json files from a directory.
// Returns nil if the directory doesn't exist; returns errors for malformed files.
func (r *WidgetRegistry) loadDefinitionsFromDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		log.Printf("warning: cannot read widget definitions from %s: %v", dir, err)
		return nil
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".def.json") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			return mdlerrors.NewBackend(fmt.Sprintf("read %s", filePath), err)
		}

		var def WidgetDefinition
		if err := json.Unmarshal(data, &def); err != nil {
			return mdlerrors.NewBackend(fmt.Sprintf("parse %s", filePath), err)
		}

		if def.WidgetID == "" || def.MDLName == "" {
			return mdlerrors.NewValidationf("invalid definition %s: widgetId and mdlName are required", entry.Name())
		}

		if err := validateDefinitionOperations(&def, entry.Name()); err != nil {
			return err
		}

		upperName := strings.ToUpper(def.MDLName)
		if existing, ok := r.byMDLName[upperName]; ok {
			// Skip user skeleton definitions (no mappings/modes) when built-in has mappings
			if len(def.PropertyMappings) == 0 && len(def.Modes) == 0 &&
				(len(existing.PropertyMappings) > 0 || len(existing.Modes) > 0) {
				log.Printf("info: skipping user skeleton %q — built-in %s has mappings", entry.Name(), def.MDLName)
				continue
			}
			log.Printf("info: user definition %q overrides built-in %s (widgetId: %s → %s)",
				entry.Name(), def.MDLName, existing.WidgetID, def.WidgetID)
		}
		r.byMDLName[upperName] = &def
		r.byWidgetID[def.WidgetID] = &def
	}
	return nil
}

// validateDefinitionOperations checks that all operation names in a definition
// are recognized by the known operations set, and validates source/operation
// compatibility and mapping order dependencies.
func validateDefinitionOperations(def *WidgetDefinition, source string) error {
	if err := validateMappings(def.PropertyMappings, source, ""); err != nil {
		return err
	}
	for _, s := range def.ChildSlots {
		if !knownOperations[s.Operation] {
			return mdlerrors.NewValidationf("%s: unknown operation %q in childSlots for key %q", source, s.Operation, s.PropertyKey)
		}
	}
	for _, mode := range def.Modes {
		ctx := fmt.Sprintf("mode %q ", mode.Name)
		if err := validateMappings(mode.PropertyMappings, source, ctx); err != nil {
			return err
		}
		for _, s := range mode.ChildSlots {
			if !knownOperations[s.Operation] {
				return mdlerrors.NewValidationf("%s: unknown operation %q in %schildSlots for key %q", source, s.Operation, ctx, s.PropertyKey)
			}
		}
	}
	return nil
}

// sourceOperationCompatible checks that a mapping's Source and Operation are compatible.
var incompatibleSourceOps = map[string]map[string]bool{
	"Attribute":   {"association": true, "datasource": true},
	"Attributes":  {"association": true, "datasource": true, "attribute": true},
	"Association": {"attribute": true, "datasource": true},
	"DataSource":  {"attribute": true, "association": true},
}

// validateMappings validates a slice of property mappings for operation existence,
// source/operation compatibility, and mapping order (Association requires prior DataSource).
func validateMappings(mappings []PropertyMapping, source, modeCtx string) error {
	hasDataSource := false
	for _, m := range mappings {
		if !knownOperations[m.Operation] {
			return mdlerrors.NewValidationf("%s: unknown operation %q in %spropertyMappings for key %q", source, m.Operation, modeCtx, m.PropertyKey)
		}
		// Check source/operation compatibility
		if incompatible, ok := incompatibleSourceOps[m.Source]; ok {
			if incompatible[m.Operation] {
				return mdlerrors.NewValidationf("%s: incompatible source %q with operation %q in %spropertyMappings for key %q",
					source, m.Source, m.Operation, modeCtx, m.PropertyKey)
			}
		}
		// Track DataSource ordering
		if m.Source == "DataSource" {
			hasDataSource = true
		}
		// Association depends on entityContext set by a prior DataSource mapping
		if m.Source == "Association" && !hasDataSource {
			return mdlerrors.NewValidationf("%s: %spropertyMappings key %q uses source 'Association' before any 'DataSource' mapping — entityContext will be stale",
				source, modeCtx, m.PropertyKey)
		}
	}
	return nil
}
