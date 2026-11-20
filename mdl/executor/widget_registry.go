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
	byMDLName       map[string]*WidgetDefinition // keyed by uppercase MDLName
	byWidgetID      map[string]*WidgetDefinition // keyed by widgetId
	knownOperations map[string]bool              // operations accepted during validation
	embeddedIDs     map[string]string            // uppercase MDLName → widgetId of the embedded (built-in) definition
	projectPath     string                       // set by LoadWidgetRegistry; lets validation resolve editorConfig visibility rules for built-in widgets whose .def.json carries none
}

// defaultKnownOperations is the set of operation names supported by the widget engine.
var defaultKnownOperations = map[string]bool{
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
	// An image collection entry on an image-typed property (the Image widget's
	// `imageObject`). Without it the property is unreachable from MDL and the
	// widget's default source has nothing to show — see setImageValue.
	"image": true,
	// An icon element on an icon-typed property (the Image widget's
	// `imageIcon`), written `Icon:` exactly as a button's is. Without it an
	// `ImageType: icon` image could not say which icon, and mxbuild refused it
	// with "No icon selected." (ako/mxcli#2174).
	"icon": true,
}

// knownOperations is the active set used for validation, initialized from
// defaultKnownOperations and now stored per-registry to avoid global mutable state.

func copyOps(src map[string]bool) map[string]bool {
	dst := make(map[string]bool, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// NewWidgetRegistry creates a registry pre-loaded with embedded definitions.
// Uses the default set of known operations for validation.
func NewWidgetRegistry() (*WidgetRegistry, error) {
	return NewWidgetRegistryWithOps(nil)
}

// NewWidgetRegistryWithOps creates a registry pre-loaded with embedded definitions,
// extending the default known operations with extraOps for validation.
// This allows user-defined widgets to declare custom operations that would otherwise
// fail validation. Pass nil for the default set.
func NewWidgetRegistryWithOps(extraOps map[string]bool) (*WidgetRegistry, error) {
	ops := copyOps(defaultKnownOperations)
	for op := range extraOps {
		ops[op] = true
	}

	reg := &WidgetRegistry{
		byMDLName:       make(map[string]*WidgetDefinition),
		byWidgetID:      make(map[string]*WidgetDefinition),
		knownOperations: ops,
		embeddedIDs:     make(map[string]string),
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

		if err := reg.validateDefinitionOperations(&def, entry.Name()); err != nil {
			return nil, err
		}

		def.MDLName = strings.ToLower(def.MDLName)
		lowerSlotContainers(&def)
		fillActionSources(&def)
		reg.byMDLName[strings.ToUpper(def.MDLName)] = &def
		reg.byWidgetID[def.WidgetID] = &def
		reg.embeddedIDs[strings.ToUpper(def.MDLName)] = def.WidgetID
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

		if err := r.validateDefinitionOperations(&def, entry.Name()); err != nil {
			return err
		}

		def.MDLName = strings.ToLower(def.MDLName)
		lowerSlotContainers(&def)
		fillActionSources(&def)
		upperName := strings.ToUpper(def.MDLName)
		// A definition for a DIFFERENT widget must never take a built-in
		// keyword: `widget init` used to name the native Image `image` and the
		// native RadioButtons `radiobuttons`, and those files are still on
		// users' disks (mendixlabs/mxcli#1463). Keep the widget reachable by id
		// and leave the keyword with the widget it documents.
		builtinID, isEmbedded := r.embeddedIDs[upperName]
		if isBuilderOwnedWidgetKeyword(def.MDLName) || (isEmbedded && builtinID != def.WidgetID) {
			builtin := "a built-in Forms widget"
			if isEmbedded {
				builtin = builtinID
			}
			log.Printf("warning: widget definition %q (%s) is not registered as `%s` — that keyword is %s. "+
				"Use `pluggablewidget '%s'`, or give the definition another mdlName (or delete the file and re-run `mxcli widget init`)",
				filePath, def.WidgetID, def.MDLName, builtin, def.WidgetID)
			r.byWidgetID[def.WidgetID] = &def
			continue
		}
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

// lowerSlotContainers lower-cases every child slot's container keyword — the
// definition's own and each mode's — which is how applyChildSlots looks a
// child's keyword up. A mode's slots were left as written, so a mode-scoped
// slot (a combo box's custom content, ako/mxcli#1744) spelled in upper case,
// as every built-in definition spells its containers, matched no child.
func lowerSlotContainers(def *WidgetDefinition) {
	for i := range def.ChildSlots {
		def.ChildSlots[i].MDLContainer = strings.ToLower(def.ChildSlots[i].MDLContainer)
	}
	for m := range def.Modes {
		for i := range def.Modes[m].ChildSlots {
			def.Modes[m].ChildSlots[i].MDLContainer = strings.ToLower(def.Modes[m].ChildSlots[i].MDLContainer)
		}
	}
}

// validateDefinitionOperations checks that all operation names in a definition
// are recognized by the known operations set, and validates source/operation
// compatibility and mapping order dependencies.
func (r *WidgetRegistry) validateDefinitionOperations(def *WidgetDefinition, source string) error {
	if err := r.validateMappings(def.PropertyMappings, source, ""); err != nil {
		return err
	}
	for _, s := range def.ChildSlots {
		if !r.knownOperations[s.Operation] {
			return mdlerrors.NewValidationf("%s: unknown operation %q in childSlots for key %q", source, s.Operation, s.PropertyKey)
		}
	}
	for _, mode := range def.Modes {
		ctx := fmt.Sprintf("mode %q ", mode.Name)
		if err := r.validateMappings(mode.PropertyMappings, source, ctx); err != nil {
			return err
		}
		for _, s := range mode.ChildSlots {
			if !r.knownOperations[s.Operation] {
				return mdlerrors.NewValidationf("%s: unknown operation %q in %schildSlots for key %q", source, s.Operation, ctx, s.PropertyKey)
			}
		}
	}
	return nil
}

// sourceOperationCompatible checks that a mapping's Source and Operation are compatible.
var incompatibleSourceOps = map[string]map[string]bool{
	"Attribute": {"association": true, "datasource": true},
	// Attributes → attribute is the list keyword feeding ONE attribute property
	// (a drop-down filter's `attr`, #1519); resolveMapping refuses a list of more
	// than one rather than keeping the first.
	"Attributes":  {"association": true, "datasource": true},
	"Association": {"attribute": true, "datasource": true},
	"DataSource":  {"attribute": true, "association": true},
}

// validateMappings validates a slice of property mappings for operation existence,
// source/operation compatibility, and mapping order (Association requires prior DataSource).
func (r *WidgetRegistry) validateMappings(mappings []PropertyMapping, source, modeCtx string) error {
	hasDataSource := false
	for _, m := range mappings {
		if !r.knownOperations[m.Operation] {
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

// fillActionSources gives an action mapping with no Source the one its key
// names (actionSourceForKey). Definitions are cached on disk —
// <project>/.mxcli/widgets and ~/.mxcli/widgets — by whichever mxcli extracted
// them, so a slot an older extraction left unrouted stays unrouted for check
// while exec, deriving afresh, writes it: a Switch's `action` slot, refused as
// MDL-WIDGET37 by check and written by exec (ako/mxcli#2114). A mapping that
// names its Source keeps it.
func fillActionSources(def *WidgetDefinition) {
	fill := func(ms []PropertyMapping) {
		for i := range ms {
			if ms[i].Operation == "action" && ms[i].Source == "" {
				ms[i].Source = actionSourceForKey(ms[i].PropertyKey)
			}
		}
	}
	fill(def.PropertyMappings)
	for i := range def.Modes {
		fill(def.Modes[i].PropertyMappings)
	}
}
