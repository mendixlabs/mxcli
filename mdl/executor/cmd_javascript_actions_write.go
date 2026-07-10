// SPDX-License-Identifier: Apache-2.0

// Package executor - CREATE/DROP JAVASCRIPT ACTION command handlers.
package executor

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// execCreateJavaScriptAction handles CREATE JAVASCRIPT ACTION statements. It
// mirrors execCreateJavaAction, building a *types.JavaScriptAction (with the
// JavaScript $Type names and a Platform) and writing the .js stub.
func execCreateJavaScriptAction(ctx *ExecContext, s *ast.CreateJavaScriptActionStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	modules, err := ctx.Backend.ListModules()
	if err != nil {
		return mdlerrors.NewBackend("get modules", err)
	}
	var containerID model.ID
	var moduleName string
	for _, mod := range modules {
		if mod.Name == s.Name.Module {
			containerID = mod.ID
			moduleName = mod.Name
			break
		}
	}
	if containerID == "" {
		return mdlerrors.NewNotFound("module", s.Name.Module)
	}

	// Detect an existing action of the same name (for CREATE OR MODIFY).
	existing, err := ctx.Backend.ListJavaScriptActions()
	if err != nil {
		return mdlerrors.NewBackend("list javascript actions", err)
	}
	var existingID model.ID
	for _, ex := range existing {
		exModName := h.GetModuleName(h.FindModuleID(ex.ContainerID))
		if exModName == s.Name.Module && ex.Name == s.Name.Name {
			if !s.CreateOrModify {
				return mdlerrors.NewAlreadyExists("javascript action", s.Name.Module+"."+s.Name.Name)
			}
			existingID = ex.ID
			break
		}
	}

	newID := model.ID(types.GenerateID())
	if existingID != "" {
		newID = existingID
	}

	jsa := &types.JavaScriptAction{
		BaseElement:             model.BaseElement{ID: newID, TypeName: "JavaScriptActions$JavaScriptAction"},
		ContainerID:             containerID,
		Name:                    s.Name.Name,
		Documentation:           s.Documentation,
		ExportLevel:             "Public",
		ActionDefaultReturnName: "ReturnValueName",
		Platform:                platformOrDefault(s.Platform),
	}

	// Type parameter definitions (with IDs for BY_ID references).
	typeParamNameToID := make(map[string]model.ID)
	typeParamNames := make(map[string]bool)
	for _, tpName := range s.TypeParameters {
		tpDef := &types.TypeParameterDef{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Name:        tpName,
		}
		jsa.TypeParameters = append(jsa.TypeParameters, tpDef)
		typeParamNameToID[tpName] = tpDef.ID
		typeParamNames[tpName] = true
	}

	for _, param := range s.Parameters {
		p := &types.JavaActionParameter{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "JavaScriptActions$JavaScriptActionParameter",
			},
			Name:       param.Name,
			IsRequired: param.IsRequired,
		}
		switch {
		case param.Type.Kind == ast.TypeEntityTypeParam:
			tpName := param.Type.TypeParamName
			p.ParameterType = &types.EntityTypeParameterType{
				BaseElement:       model.BaseElement{ID: model.ID(types.GenerateID())},
				TypeParameterID:   typeParamNameToID[tpName],
				TypeParameterName: tpName,
			}
		case isTypeParamRef(param.Type, typeParamNames):
			tpName := getTypeParamRefName(param.Type)
			p.ParameterType = &types.TypeParameter{
				BaseElement:     model.BaseElement{ID: model.ID(types.GenerateID())},
				TypeParameterID: typeParamNameToID[tpName],
				TypeParameter:   tpName,
			}
		default:
			p.ParameterType = astDataTypeToJavaActionParamType(param.Type)
		}
		jsa.Parameters = append(jsa.Parameters, p)
	}

	if isTypeParamRef(s.ReturnType, typeParamNames) {
		tpName := getTypeParamRefName(s.ReturnType)
		jsa.ReturnType = &types.TypeParameter{
			BaseElement:     model.BaseElement{ID: model.ID(types.GenerateID())},
			TypeParameterID: typeParamNameToID[tpName],
			TypeParameter:   tpName,
		}
	} else {
		jsa.ReturnType = astDataTypeToJavaActionReturnType(s.ReturnType)
	}

	if s.ExposedCaption != "" {
		jsa.MicroflowActionInfo = &types.MicroflowActionInfo{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Caption:     s.ExposedCaption,
			Category:    s.ExposedCategory,
		}
	}

	if existingID != "" {
		if err := ctx.Backend.UpdateJavaScriptAction(jsa); err != nil {
			return mdlerrors.NewBackend("update javascript action", err)
		}
	} else {
		if err := ctx.Backend.CreateJavaScriptAction(jsa); err != nil {
			return mdlerrors.NewBackend("create javascript action", err)
		}
	}

	if err := ctx.Backend.WriteJavaScriptSourceFile(moduleName, s.Name.Name, s.JavaScriptCode, jsa.Parameters, jsa.ReturnType); err != nil {
		return mdlerrors.NewBackend("write javascript source file", err)
	}

	ctx.InvalidateCache()
	if existingID != "" {
		fmt.Fprintf(ctx.Output, "Modified javascript action: %s.%s\n", s.Name.Module, s.Name.Name)
	} else {
		fmt.Fprintf(ctx.Output, "Created javascript action: %s.%s\n", s.Name.Module, s.Name.Name)
	}
	return nil
}

// execDropJavaScriptAction handles DROP JAVASCRIPT ACTION statements.
func execDropJavaScriptAction(ctx *ExecContext, s *ast.DropJavaScriptActionStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	actions, err := ctx.Backend.ListJavaScriptActions()
	if err != nil {
		return mdlerrors.NewBackend("list javascript actions", err)
	}
	for _, jsa := range actions {
		modName := h.GetModuleName(h.FindModuleID(jsa.ContainerID))
		if modName == s.Name.Module && jsa.Name == s.Name.Name {
			if err := ctx.Backend.DeleteJavaScriptAction(jsa.ID); err != nil {
				return mdlerrors.NewBackend("delete javascript action", err)
			}
			if err := ctx.Backend.DeleteJavaScriptSourceFile(modName, jsa.Name); err != nil {
				return mdlerrors.NewBackend("delete javascript source file", err)
			}
			ctx.InvalidateCache()
			fmt.Fprintf(ctx.Output, "Dropped javascript action: %s.%s\n", s.Name.Module, s.Name.Name)
			return nil
		}
	}
	return mdlerrors.NewNotFound("javascript action", s.Name.Module+"."+s.Name.Name)
}

// platformOrDefault returns the platform value, defaulting to Web.
func platformOrDefault(p string) string {
	if p == "" {
		return "Web"
	}
	return p
}
