// SPDX-License-Identifier: Apache-2.0

// Package executor - CREATE NANOFLOW command
package executor

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// execCreateNanoflow handles CREATE NANOFLOW statements.
func execCreateNanoflow(ctx *ExecContext, s *ast.CreateNanoflowStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	// Validate name is not empty
	if strings.TrimSpace(s.Name.Name) == "" {
		return mdlerrors.NewValidation("nanoflow name must not be empty")
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

	// Check if nanoflow with same name already exists in this module.
	// NOTE: O(n) scan over all nanoflows — consistent with microflow handler pattern.
	// Consider catalog-based lookup if this becomes a bottleneck for large projects.
	var existingID model.ID
	var existingContainerID model.ID
	var existingAllowedRoles []model.ID
	preserveAllowedRoles := false
	existingNanoflows, err := ctx.Backend.ListNanoflows()
	if err != nil {
		return mdlerrors.NewBackend("check existing nanoflows", err)
	}
	for _, existing := range existingNanoflows {
		if existing.Name == s.Name.Name && getModuleID(ctx, existing.ContainerID) == module.ID {
			if !s.CreateOrModify {
				return mdlerrors.NewAlreadyExistsMsg("nanoflow", s.Name.Module+"."+s.Name.Name, "nanoflow '"+s.Name.Module+"."+s.Name.Name+"' already exists (use create or modify to overwrite)")
			}
			existingID = existing.ID
			existingContainerID = existing.ContainerID
			existingAllowedRoles = cloneRoleIDs(existing.AllowedModuleRoles)
			preserveAllowedRoles = true
			break
		}
	}

	// For CREATE OR REPLACE/MODIFY, reuse the existing ID to preserve references
	qualifiedName := s.Name.Module + "." + s.Name.Name
	nanoflowID := model.ID(types.GenerateID())
	if existingID != "" {
		nanoflowID = existingID
		if s.Folder == "" {
			containerID = existingContainerID
		}
	} else if dropped := consumeDroppedNanoflow(ctx, qualifiedName); dropped != nil {
		nanoflowID = dropped.ID
		if s.Folder == "" && dropped.ContainerID != "" {
			containerID = dropped.ContainerID
		}
		if len(dropped.AllowedRoles) > 0 {
			existingAllowedRoles = dropped.AllowedRoles
			preserveAllowedRoles = true
		}
	}

	// Build the nanoflow
	nf := &microflows.Nanoflow{
		BaseElement: model.BaseElement{
			ID: nanoflowID,
		},
		ContainerID:   containerID,
		Name:          s.Name.Name,
		Documentation: s.Documentation,
		MarkAsUsed:    false,
		Excluded:      s.Excluded,
	}
	if preserveAllowedRoles {
		nf.AllowedModuleRoles = existingAllowedRoles
	} else {
		nf.AllowedModuleRoles = defaultDocumentAccessRoles(ctx, module)
	}

	// Load metadata needed by the entity resolver up front so backend read
	// failures are returned as actionable errors instead of being treated as
	// "entity not found".
	dms, err := ctx.Backend.ListDomainModels()
	if err != nil {
		return mdlerrors.NewBackend("list domain models", err)
	}
	modules, err := ctx.Backend.ListModules()
	if err != nil {
		return mdlerrors.NewBackend("list modules", err)
	}
	moduleNames := make(map[model.ID]string)
	for _, m := range modules {
		moduleNames[m.ID] = m.Name
	}

	// Build entity resolver function for parameter/return types
	entityResolver := func(qn ast.QualifiedName) model.ID {
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
	for i, p := range s.Parameters {
		if p.Type.EntityRef != nil && !isBuiltinModuleEntity(p.Type.EntityRef.Module) {
			entityID := entityResolver(*p.Type.EntityRef)
			if entityID == "" {
				// Bare qualified name in microflow context is treated as TypeEntity by the
				// visitor, but it may actually be an enumeration. Try enum lookup before failing.
				if found := findEnumeration(ctx, p.Type.EntityRef.Module, p.Type.EntityRef.Name); found != nil {
					s.Parameters[i].Type = ast.DataType{Kind: ast.TypeEnumeration, EnumRef: p.Type.EntityRef}
					p = s.Parameters[i]
				} else {
					return mdlerrors.NewNotFoundMsg("entity", p.Type.EntityRef.Module+"."+p.Type.EntityRef.Name,
						fmt.Sprintf("entity '%s.%s' not found for parameter '%s'", p.Type.EntityRef.Module, p.Type.EntityRef.Name, p.Name))
				}
			}
		}
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
			ContainerID: nf.ID,
			Name:        p.Name,
			Type:        convertASTToMicroflowDataType(p.Type, entityResolver),
		}
		nf.Parameters = append(nf.Parameters, param)
	}

	// Validate and set return type
	if s.ReturnType != nil {
		if s.ReturnType.Type.EntityRef != nil && !isBuiltinModuleEntity(s.ReturnType.Type.EntityRef.Module) {
			entityID := entityResolver(*s.ReturnType.Type.EntityRef)
			if entityID == "" {
				return mdlerrors.NewNotFoundMsg("entity", s.ReturnType.Type.EntityRef.Module+"."+s.ReturnType.Type.EntityRef.Name,
					fmt.Sprintf("entity '%s.%s' not found for return type", s.ReturnType.Type.EntityRef.Module, s.ReturnType.Type.EntityRef.Name))
			}
		}
		if s.ReturnType.Type.Kind == ast.TypeEnumeration && s.ReturnType.Type.EnumRef != nil {
			if found := findEnumeration(ctx, s.ReturnType.Type.EnumRef.Module, s.ReturnType.Type.EnumRef.Name); found == nil {
				return mdlerrors.NewNotFoundMsg("enumeration", s.ReturnType.Type.EnumRef.Module+"."+s.ReturnType.Type.EnumRef.Name,
					fmt.Sprintf("enumeration '%s.%s' not found for return type", s.ReturnType.Type.EnumRef.Module, s.ReturnType.Type.EnumRef.Name))
			}
		}
		nf.ReturnType = convertASTToMicroflowDataType(s.ReturnType.Type, entityResolver)
	} else {
		nf.ReturnType = &microflows.VoidType{}
	}

	// Validate nanoflow-specific constraints before building the flow graph
	if errMsg := validateNanoflow(qualifiedName, s.Body, s.ReturnType); errMsg != "" {
		return fmt.Errorf("%s", errMsg)
	}

	// Build flow graph from body statements
	varTypes := make(map[string]string)
	declaredVars := make(map[string]string)

	for _, p := range s.Parameters {
		if p.Type.EntityRef != nil {
			entityQN := p.Type.EntityRef.Module + "." + p.Type.EntityRef.Name
			if p.Type.Kind == ast.TypeListOf {
				varTypes[p.Name] = "List of " + entityQN
			} else {
				varTypes[p.Name] = entityQN
			}
		} else {
			declaredVars[p.Name] = p.Type.Kind.String()
		}
	}

	hierarchy, _ := getHierarchy(ctx)        // best-effort: builder works without hierarchy
	restServices, _ := loadRestServices(ctx) // best-effort: builder works without REST services

	builder := &flowBuilder{
		posX:         200,
		posY:         200,
		baseY:        200,
		spacing:      HorizontalSpacing,
		varTypes:     varTypes,
		declaredVars: declaredVars,
		measurer:     &layoutMeasurer{varTypes: varTypes},
		backend:      ctx.Backend,
		hierarchy:    hierarchy,
		restServices: restServices,
		isNanoflow:   true,
	}

	nf.ObjectCollection = builder.buildFlowGraph(s.Body, s.ReturnType)

	// Check for validation errors
	if errors := builder.GetErrors(); len(errors) > 0 {
		var errMsg strings.Builder
		errMsg.WriteString(fmt.Sprintf("nanoflow '%s.%s' has validation errors:\n", s.Name.Module, s.Name.Name))
		for _, err := range errors {
			errMsg.WriteString(fmt.Sprintf("  - %s\n", err))
		}
		return fmt.Errorf("%s", errMsg.String())
	}

	// Create or update the nanoflow
	if existingID != "" {
		if err := ctx.Backend.UpdateNanoflow(nf); err != nil {
			return mdlerrors.NewBackend("update nanoflow", err)
		}
		fmt.Fprintf(ctx.Output, "Replaced nanoflow: %s.%s\n", s.Name.Module, s.Name.Name)
	} else {
		if err := ctx.Backend.CreateNanoflow(nf); err != nil {
			return mdlerrors.NewBackend("create nanoflow", err)
		}
		fmt.Fprintf(ctx.Output, "Created nanoflow: %s.%s\n", s.Name.Module, s.Name.Name)
	}

	// Track the created nanoflow
	returnEntityName := extractEntityFromReturnType(nf.ReturnType)
	ctx.trackCreatedNanoflow(s.Name.Module, s.Name.Name, nf.ID, containerID, returnEntityName)

	invalidateHierarchy(ctx)
	return nil
}
