// SPDX-License-Identifier: Apache-2.0

// Package executor - CREATE MICROFLOW command
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// isBuiltinModuleEntity returns true for modules whose entities are defined
// internally by the Mendix runtime and are therefore not present in the MPR's
// domain models. These types are serialized using the qualified name reference
// ("System.Workflow", "System.User", etc.) and resolved at runtime.
func isBuiltinModuleEntity(moduleName string) bool {
	return moduleName == "System"
}

// execCreateMicroflow handles CREATE MICROFLOW statements.
// loadRestServices returns all consumed REST services, or nil if no reader.
func loadRestServices(ctx *ExecContext) ([]*model.ConsumedRestService, error) {
	if !ctx.Connected() {
		return nil, nil
	}
	svcs, err := ctx.Backend.ListConsumedRestServices()
	return svcs, err
}

func execCreateMicroflow(ctx *ExecContext, s *ast.CreateMicroflowStmt) error {
	e := ctx.executor
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	// Find or auto-create module
	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	// Resolve folder if specified
	containerID := module.ID
	if s.Folder != "" {
		folderID, err := resolveFolder(ctx, module.ID, s.Folder)
		if err != nil {
			return mdlerrors.NewBackend("resolve folder "+s.Folder, err)
		}
		containerID = folderID
	}

	// Check if microflow with same name already exists in this module
	var existingID model.ID
	var existingContainerID model.ID
	existingMicroflows, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return mdlerrors.NewBackend("check existing microflows", err)
	}
	for _, existing := range existingMicroflows {
		if existing.Name == s.Name.Name && getModuleID(ctx, existing.ContainerID) == module.ID {
			if !s.CreateOrModify {
				return mdlerrors.NewAlreadyExistsMsg("microflow", s.Name.Module+"."+s.Name.Name, "microflow '"+s.Name.Module+"."+s.Name.Name+"' already exists (use CREATE OR REPLACE to overwrite)")
			}
			existingID = existing.ID
			existingContainerID = existing.ContainerID
			break
		}
	}

	// For CREATE OR REPLACE/MODIFY, reuse the existing ID to preserve references
	microflowID := model.ID(types.GenerateID())
	if existingID != "" {
		microflowID = existingID
		// Keep the original folder unless a new folder is explicitly specified
		if s.Folder == "" {
			containerID = existingContainerID
		}
	}

	// Build the microflow
	mf := &microflows.Microflow{
		BaseElement: model.BaseElement{
			ID: microflowID,
		},
		ContainerID:              containerID,
		Name:                     s.Name.Name,
		Documentation:            s.Documentation,
		AllowConcurrentExecution: true, // Default: allow concurrent execution
		MarkAsUsed:               false,
		Excluded:                 s.Excluded,
	}

	// Build entity resolver function for parameter/return types
	entityResolver := func(qn ast.QualifiedName) model.ID {
		// Get all domain models and build module name map
		dms, err := ctx.Backend.ListDomainModels()
		if err != nil {
			return ""
		}
		modules, _ := ctx.Backend.ListModules()
		moduleNames := make(map[model.ID]string)
		for _, m := range modules {
			moduleNames[m.ID] = m.Name
		}
		// Search for entity in all domain models
		for _, dm := range dms {
			modName := moduleNames[dm.ContainerID]
			if modName != qn.Module {
				continue
			}
			for _, ent := range dm.Entities {
				if ent.Name == qn.Name {
					return ent.ID
				}
			}
		}
		return ""
	}

	// Validate and add parameters
	for _, p := range s.Parameters {
		// Validate entity references for List and Entity types.
		// Built-in modules (e.g. System) are not stored in the MPR domain models;
		// their types are serialized by qualified name and resolved at runtime.
		if p.Type.EntityRef != nil && !isBuiltinModuleEntity(p.Type.EntityRef.Module) {
			entityID := entityResolver(*p.Type.EntityRef)
			if entityID == "" {
				return mdlerrors.NewNotFoundMsg("entity", p.Type.EntityRef.Module+"."+p.Type.EntityRef.Name,
					fmt.Sprintf("entity '%s.%s' not found for parameter '%s'", p.Type.EntityRef.Module, p.Type.EntityRef.Name, p.Name))
			}
		}
		// Validate enumeration references for Enumeration types
		if p.Type.Kind == ast.TypeEnumeration && p.Type.EnumRef != nil {
			if found := findEnumeration(ctx, p.Type.EnumRef.Module, p.Type.EnumRef.Name); found == nil {
				return mdlerrors.NewNotFoundMsg("enumeration", p.Type.EnumRef.Module+"."+p.Type.EnumRef.Name,
					fmt.Sprintf("enumeration '%s.%s' not found for parameter '%s'", p.Type.EnumRef.Module, p.Type.EnumRef.Name, p.Name))
			}
		}
		param := &microflows.MicroflowParameter{
			BaseElement: model.BaseElement{
				ID: model.ID(types.GenerateID()),
			},
			ContainerID: mf.ID,
			Name:        p.Name,
			Type:        convertASTToMicroflowDataType(p.Type, entityResolver),
		}
		mf.Parameters = append(mf.Parameters, param)
	}

	// Validate and set return type
	if s.ReturnType != nil {
		// Validate entity references for return type.
		// Built-in modules (e.g. System) are not stored in the MPR domain models;
		// their types are serialized by qualified name and resolved at runtime.
		if s.ReturnType.Type.EntityRef != nil && !isBuiltinModuleEntity(s.ReturnType.Type.EntityRef.Module) {
			entityID := entityResolver(*s.ReturnType.Type.EntityRef)
			if entityID == "" {
				return mdlerrors.NewNotFoundMsg("entity", s.ReturnType.Type.EntityRef.Module+"."+s.ReturnType.Type.EntityRef.Name,
					fmt.Sprintf("entity '%s.%s' not found for return type", s.ReturnType.Type.EntityRef.Module, s.ReturnType.Type.EntityRef.Name))
			}
		}
		// Validate enumeration references for return type
		if s.ReturnType.Type.Kind == ast.TypeEnumeration && s.ReturnType.Type.EnumRef != nil {
			if found := findEnumeration(ctx, s.ReturnType.Type.EnumRef.Module, s.ReturnType.Type.EnumRef.Name); found == nil {
				return mdlerrors.NewNotFoundMsg("enumeration", s.ReturnType.Type.EnumRef.Module+"."+s.ReturnType.Type.EnumRef.Name,
					fmt.Sprintf("enumeration '%s.%s' not found for return type", s.ReturnType.Type.EnumRef.Module, s.ReturnType.Type.EnumRef.Name))
			}
		}
		mf.ReturnType = convertASTToMicroflowDataType(s.ReturnType.Type, entityResolver)
		// Set return variable name if provided (AS $VarName)
		if s.ReturnType.Variable != "" {
			mf.ReturnVariableName = s.ReturnType.Variable
		}
	} else {
		mf.ReturnType = &microflows.VoidType{}
	}

	// Build flow graph from body statements
	// Initialize variable types from parameters
	varTypes := make(map[string]string)
	declaredVars := make(map[string]string)

	for _, p := range s.Parameters {
		if p.Type.EntityRef != nil {
			entityQN := p.Type.EntityRef.Module + "." + p.Type.EntityRef.Name
			if p.Type.Kind == ast.TypeListOf {
				// Store "List of Module.Entity" for list parameters
				varTypes[p.Name] = "List of " + entityQN
			} else {
				// Store "Module.Entity" for single entity parameters
				varTypes[p.Name] = entityQN
			}
		} else {
			// Primitive type parameters are also considered declared
			declaredVars[p.Name] = p.Type.Kind.String()
		}
	}
	// Get hierarchy for resolving page/microflow references
	hierarchy, _ := getHierarchy(ctx)

	restServices, _ := loadRestServices(ctx)

	builder := &flowBuilder{
		posX:         200,
		posY:         200,
		baseY:        200, // Base Y for happy path
		spacing:      HorizontalSpacing,
		varTypes:     varTypes,
		declaredVars: declaredVars,
		measurer:     &layoutMeasurer{varTypes: varTypes},
		reader:       ctx.Backend,
		hierarchy:    hierarchy,
		restServices: restServices,
	}

	mf.ObjectCollection = builder.buildFlowGraph(s.Body, s.ReturnType)

	// Check for validation errors
	if errors := builder.GetErrors(); len(errors) > 0 {
		// Report all errors to the user
		var errMsg strings.Builder
		errMsg.WriteString(fmt.Sprintf("microflow '%s.%s' has validation errors:\n", s.Name.Module, s.Name.Name))
		for _, err := range errors {
			errMsg.WriteString(fmt.Sprintf("  - %s\n", err))
		}
		return fmt.Errorf("%s", errMsg.String())
	}

	// Create or update the microflow
	if existingID != "" {
		if err := ctx.Backend.UpdateMicroflow(mf); err != nil {
			return mdlerrors.NewBackend("update microflow", err)
		}
		fmt.Fprintf(ctx.Output, "Replaced microflow: %s.%s\n", s.Name.Module, s.Name.Name)
	} else {
		if err := ctx.Backend.CreateMicroflow(mf); err != nil {
			return mdlerrors.NewBackend("create microflow", err)
		}
		fmt.Fprintf(ctx.Output, "Created microflow: %s.%s\n", s.Name.Module, s.Name.Name)
	}

	// Track the created microflow so it can be resolved by subsequent page creations
	returnEntityName := extractEntityFromReturnType(mf.ReturnType)
	e.trackCreatedMicroflow(s.Name.Module, s.Name.Name, mf.ID, containerID, returnEntityName)

	// Invalidate hierarchy cache so the new microflow's container is visible
	invalidateHierarchy(ctx)
	return nil
}
