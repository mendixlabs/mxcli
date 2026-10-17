// SPDX-License-Identifier: Apache-2.0

// Package executor - CREATE RULE command
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

// execCreateRule handles CREATE RULE statements.

// Mirrors execCreateNanoflow — a rule shares a microflow's parameters, return
// type and body — with three differences that follow from the document shape
// (measured on ako/TestApp, Mendix 11.13.0):
//
//   - No AllowedModuleRoles. A rule stores none, because it is not
//     independently callable, so there is nothing to preserve across a rewrite
//     and no default grant to apply.
//   - The return type is mandatory and must be Boolean or an enumeration.
//   - The flow builder runs with neither isNanoflow nor a rule flag: a rule's
//     body is a server-side microflow body, and what it may NOT contain is
//     refused by validateRule before the graph is built.
func execCreateRule(ctx *ExecContext, s *ast.CreateRuleStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	// Validate name is not empty
	if strings.TrimSpace(s.Name.Name) == "" {
		return mdlerrors.NewValidation("rule name must not be empty")
	}

	if err := refuseExposeOnFlavour(s.Expose, "rule", s.Name.Module+"."+s.Name.Name); err != nil {
		return err
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

	// Check whether a rule of this name already exists in the module.
	var existingID model.ID
	var existingDocumentation string
	haveExisting := false
	var existingContainerID model.ID
	// Excluded is model state, not script state: an absent @excluded must not
	// clear a stored exclusion (#914).
	existingExcluded := false
	// Same as the microflow path: "apply entity access" is a SECURITY setting
	// and model state, so an absent annotation preserves the stored value
	// rather than widening what the rule may read.
	existingApplyEntityAccess := false
	// Studio Pro's rule editor writes "Variable" here on both reference rules, so
	// a rule mxcli creates matches rather than storing an empty name that Studio
	// Pro would fill in on first edit.
	existingReturnVariableName := "Variable"
	existingRules, err := ctx.Backend.ListRules()
	if err != nil {
		return mdlerrors.NewBackend("check existing rules", err)
	}
	// A module may hold several rules with this name as long as all but one are
	// excluded, so target the live one rather than whichever comes first.
	if existing, ok := pickLive(existingRules,
		func(r *microflows.Rule) bool {
			return r.Name == s.Name.Name && getModuleID(ctx, r.ContainerID) == module.ID
		},
		func(r *microflows.Rule) bool { return r.Excluded },
	); ok {
		if !s.CreateOrModify {
			return mdlerrors.NewAlreadyExistsMsg("rule", s.Name.Module+"."+s.Name.Name, "rule '"+s.Name.Module+"."+s.Name.Name+"' already exists (use create or modify to overwrite)")
		}
		existingID = existing.ID
		existingContainerID = existing.ContainerID
		existingExcluded = existing.Excluded
		existingApplyEntityAccess = existing.ApplyEntityAccess
		existingDocumentation = existing.Documentation
		haveExisting = true
		// MDL has no surface for ReturnVariableName, and Studio Pro writes one
		// ("Variable" on both reference rules), so carry the stored value rather
		// than blanking it on every rewrite.
		existingReturnVariableName = existing.ReturnVariableName
	}

	// For CREATE OR REPLACE/MODIFY, reuse the existing ID to preserve references
	qualifiedName := s.Name.Module + "." + s.Name.Name
	ruleID := model.ID(types.GenerateID())
	if existingID != "" {
		ruleID = existingID
		if s.Folder == "" {
			containerID = existingContainerID
		}
	}

	// Build the rule. No AllowedModuleRoles: a rule document stores none.
	rule := &microflows.Rule{
		BaseElement: model.BaseElement{
			ID: ruleID,
		},
		ContainerID:        containerID,
		Name:               s.Name.Name,
		Documentation:      s.Documentation,
		MarkAsUsed:         false,
		Excluded:           s.Excluded || existingExcluded,
		ApplyEntityAccess:  carriedApplyEntityAccess(s.ApplyEntityAccess, existingApplyEntityAccess),
		ReturnVariableName: existingReturnVariableName,
	}

	// A rewrite that carried no doc comment keeps the stored one (#1018).
	if haveExisting {
		rule.Documentation = carriedDocumentation(s.DocumentationSet, s.Documentation, existingDocumentation)
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
				if retyped, ok := bareEnumerationType(ctx, p.Type); ok {
					s.Parameters[i].Type = retyped
					p = s.Parameters[i]
				} else {
					return mdlerrors.NewNotFoundMsg("entity", p.Type.EntityRef.Module+"."+p.Type.EntityRef.Name,
						fmt.Sprintf("entity or enumeration '%s.%s' not found for parameter '%s'", p.Type.EntityRef.Module, p.Type.EntityRef.Name, p.Name))
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
			ContainerID: rule.ID,
			Name:        p.Name,
			Type:        convertASTToMicroflowDataType(p.Type, entityResolver),
			Position:    positionFromAST(p.Position),
		}
		rule.Parameters = append(rule.Parameters, param)
	}

	// Validate and set return type
	if s.ReturnType != nil {
		if s.ReturnType.Type.EntityRef != nil && !isBuiltinModuleEntity(s.ReturnType.Type.EntityRef.Module) {
			entityID := entityResolver(*s.ReturnType.Type.EntityRef)
			if entityID == "" {
				// A bare qualified name may be an enumeration, as for parameters above.
				if retyped, ok := bareEnumerationType(ctx, s.ReturnType.Type); ok && s.ReturnType.Type.Kind == ast.TypeEntity {
					s.ReturnType.Type = retyped
				} else {
					return mdlerrors.NewNotFoundMsg("entity", s.ReturnType.Type.EntityRef.Module+"."+s.ReturnType.Type.EntityRef.Name,
						fmt.Sprintf("entity or enumeration '%s.%s' not found for return type", s.ReturnType.Type.EntityRef.Module, s.ReturnType.Type.EntityRef.Name))
				}
			}
		}
		if s.ReturnType.Type.Kind == ast.TypeEnumeration && s.ReturnType.Type.EnumRef != nil {
			if found := findEnumeration(ctx, s.ReturnType.Type.EnumRef.Module, s.ReturnType.Type.EnumRef.Name); found == nil {
				return mdlerrors.NewNotFoundMsg("enumeration", s.ReturnType.Type.EnumRef.Module+"."+s.ReturnType.Type.EnumRef.Name,
					fmt.Sprintf("enumeration '%s.%s' not found for return type", s.ReturnType.Type.EnumRef.Module, s.ReturnType.Type.EnumRef.Name))
			}
		}
		rule.ReturnType = convertASTToMicroflowDataType(s.ReturnType.Type, entityResolver)
	}

	// Validate rule-specific constraints before building the flow graph. This
	// covers the mandatory Boolean/enumeration return type, so rule.ReturnType is
	// never left nil past this point.
	if errMsg := validateRule(qualifiedName, s.Body, s.ReturnType); errMsg != "" {
		return fmt.Errorf("%s", errMsg)
	}
	if err := validateRuleSetTargets(qualifiedName, s.Parameters, s.Body); err != nil {
		return err
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
	}

	rule.ObjectCollection = builder.buildFlowGraph(s.Body, s.ReturnType)

	// Check for validation errors
	if errors := builder.GetErrors(); len(errors) > 0 {
		var errMsg strings.Builder
		errMsg.WriteString(fmt.Sprintf("rule '%s.%s' has validation errors:\n", s.Name.Module, s.Name.Name))
		for _, err := range errors {
			errMsg.WriteString(fmt.Sprintf("  - %s\n", err))
		}
		return fmt.Errorf("%s", errMsg.String())
	}

	// Create or update the rule
	if existingID != "" {
		if err := ctx.Backend.UpdateRule(rule); err != nil {
			return mdlerrors.NewBackend("update rule", err)
		}
		if _, err := applyDocumentFolder(ctx, rule.ID, existingContainerID, containerID); err != nil {
			return err
		}
		ctx.ReportMutation("Replaced", "rule: %s.%s", s.Name.Module, s.Name.Name)
	} else {
		if err := ctx.Backend.CreateRule(rule); err != nil {
			return mdlerrors.NewBackend("create rule", err)
		}
		fmt.Fprintf(ctx.Output, "Created rule: %s.%s\n", s.Name.Module, s.Name.Name)
	}

	invalidateHierarchy(ctx)
	return nil
}
