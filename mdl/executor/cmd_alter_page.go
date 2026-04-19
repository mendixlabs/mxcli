// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// execAlterPage handles ALTER PAGE/SNIPPET Module.Name { operations }.
func execAlterPage(ctx *ExecContext, s *ast.AlterPageStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	var unitID model.ID
	var containerID model.ID
	containerType := s.ContainerType
	if containerType == "" {
		containerType = "PAGE"
	}

	if containerType == "SNIPPET" {
		snippet, modID, err := findSnippetByName(ctx, s.PageName, h)
		if err != nil {
			return err
		}
		unitID = snippet.ID
		containerID = modID
	} else {
		page, err := findPageByName(ctx, s.PageName, h)
		if err != nil {
			return err
		}
		unitID = page.ID
		containerID = h.FindModuleID(page.ContainerID)
	}

	// Open the page for mutation via the backend
	mutator, err := ctx.Backend.OpenPageForMutation(unitID)
	if err != nil {
		return mdlerrors.NewBackend("open "+strings.ToLower(containerType)+" for mutation", err)
	}

	// Resolve module name for building new widgets
	modName := h.GetModuleName(containerID)

	for _, op := range s.Operations {
		switch o := op.(type) {
		case *ast.SetPropertyOp:
			if err := applySetPropertyMutator(mutator, o); err != nil {
				return mdlerrors.NewBackend("SET", err)
			}
		case *ast.InsertWidgetOp:
			if err := applyInsertWidgetMutator(ctx, mutator, o, modName, containerID); err != nil {
				return mdlerrors.NewBackend("INSERT", err)
			}
		case *ast.DropWidgetOp:
			if err := applyDropWidgetMutator(mutator, o); err != nil {
				return mdlerrors.NewBackend("DROP", err)
			}
		case *ast.ReplaceWidgetOp:
			if err := applyReplaceWidgetMutator(ctx, mutator, o, modName, containerID); err != nil {
				return mdlerrors.NewBackend("REPLACE", err)
			}
		case *ast.AddVariableOp:
			if err := mutator.AddVariable(o.Variable.Name, o.Variable.DataType, o.Variable.DefaultValue); err != nil {
				return mdlerrors.NewBackend("ADD VARIABLE", err)
			}
		case *ast.DropVariableOp:
			if err := mutator.DropVariable(o.VariableName); err != nil {
				return mdlerrors.NewBackend("DROP VARIABLE", err)
			}
		case *ast.SetLayoutOp:
			if containerType == "SNIPPET" {
				return mdlerrors.NewUnsupported("SET Layout is not supported for snippets")
			}
			newLayoutQN := o.NewLayout.Module + "." + o.NewLayout.Name
			if err := mutator.SetLayout(newLayoutQN, o.Mappings); err != nil {
				return mdlerrors.NewBackend("SET Layout", err)
			}
		default:
			return mdlerrors.NewUnsupported(fmt.Sprintf("unknown ALTER %s operation type: %T", containerType, op))
		}
	}

	// Persist
	if err := mutator.Save(); err != nil {
		return mdlerrors.NewBackend("save modified "+strings.ToLower(containerType), err)
	}

	fmt.Fprintf(ctx.Output, "Altered %s %s\n", strings.ToLower(containerType), s.PageName.String())
	return nil
}

// ============================================================================
// SET property via mutator
// ============================================================================

func applySetPropertyMutator(mutator backend.PageMutator, op *ast.SetPropertyOp) error {
	for propName, value := range op.Properties {
		if op.Target.IsColumn() {
			if err := mutator.SetColumnProperty(op.Target.Widget, op.Target.Column, propName, value); err != nil {
				return mdlerrors.NewBackend("set "+propName+" on "+op.Target.Name(), err)
			}
		} else if propName == "DataSource" {
			// DataSource requires special handling via SetWidgetDataSource
			ds, err := convertASTDataSource(value)
			if err != nil {
				return err
			}
			if err := mutator.SetWidgetDataSource(op.Target.Widget, ds); err != nil {
				return mdlerrors.NewBackend("set DataSource on "+op.Target.Name(), err)
			}
		} else {
			if err := mutator.SetWidgetProperty(op.Target.Widget, propName, value); err != nil {
				return mdlerrors.NewBackend("set "+propName+" on "+op.Target.Name(), err)
			}
		}
	}
	return nil
}

// convertASTDataSource converts an AST DataSource value to a pages.DataSource.
func convertASTDataSource(value interface{}) (pages.DataSource, error) {
	ds, ok := value.(*ast.DataSourceV3)
	if !ok {
		return nil, mdlerrors.NewValidation("DataSource value must be a datasource expression")
	}

	switch ds.Type {
	case "selection":
		return &pages.ListenToWidgetSource{WidgetName: ds.Reference}, nil
	case "database":
		return &pages.DatabaseSource{EntityName: ds.Reference}, nil
	case "microflow":
		return &pages.MicroflowSource{Microflow: ds.Reference}, nil
	case "nanoflow":
		return &pages.NanoflowSource{Nanoflow: ds.Reference}, nil
	default:
		return nil, mdlerrors.NewUnsupported("unsupported DataSource type for ALTER PAGE SET: " + ds.Type)
	}
}

// ============================================================================
// INSERT widget via mutator
// ============================================================================

func applyInsertWidgetMutator(ctx *ExecContext, mutator backend.PageMutator, op *ast.InsertWidgetOp, moduleName string, moduleID model.ID) error {
	// Check for duplicate widget names before building
	for _, w := range op.Widgets {
		if w.Name != "" && mutator.FindWidget(w.Name) {
			return mdlerrors.NewAlreadyExistsMsg("widget", w.Name, fmt.Sprintf("duplicate widget name '%s': a widget with this name already exists on the page", w.Name))
		}
	}

	// Find entity context from enclosing DataView/DataGrid/ListView
	entityCtx := mutator.EnclosingEntity(op.Target.Widget)

	// Build new widgets from AST
	widgets, err := buildWidgetsFromAST(ctx, op.Widgets, moduleName, moduleID, entityCtx, mutator)
	if err != nil {
		return mdlerrors.NewBackend("build widgets", err)
	}

	return mutator.InsertWidget(op.Target.Widget, op.Target.Column, op.Position, widgets)
}

// ============================================================================
// DROP widget via mutator
// ============================================================================

func applyDropWidgetMutator(mutator backend.PageMutator, op *ast.DropWidgetOp) error {
	refs := make([]backend.WidgetRef, len(op.Targets))
	for i, t := range op.Targets {
		refs[i] = backend.WidgetRef{Widget: t.Widget, Column: t.Column}
	}
	return mutator.DropWidget(refs)
}

// ============================================================================
// REPLACE widget via mutator
// ============================================================================

func applyReplaceWidgetMutator(ctx *ExecContext, mutator backend.PageMutator, op *ast.ReplaceWidgetOp, moduleName string, moduleID model.ID) error {
	// Check for duplicate widget names (skip the widget being replaced)
	for _, w := range op.NewWidgets {
		if w.Name != "" && w.Name != op.Target.Widget && mutator.FindWidget(w.Name) {
			return mdlerrors.NewAlreadyExistsMsg("widget", w.Name, fmt.Sprintf("duplicate widget name '%s': a widget with this name already exists on the page", w.Name))
		}
	}

	// Find entity context from enclosing DataView/DataGrid/ListView
	entityCtx := mutator.EnclosingEntity(op.Target.Widget)

	// Build new widgets from AST
	widgets, err := buildWidgetsFromAST(ctx, op.NewWidgets, moduleName, moduleID, entityCtx, mutator)
	if err != nil {
		return mdlerrors.NewBackend("build replacement widgets", err)
	}

	return mutator.ReplaceWidget(op.Target.Widget, op.Target.Column, widgets)
}

// ============================================================================
// Widget building from AST (domain logic stays in executor)
// ============================================================================

// buildWidgetsFromAST converts AST widgets to pages.Widget domain objects.
// It uses the mutator for scope resolution (WidgetScope, ParamScope).
func buildWidgetsFromAST(ctx *ExecContext, widgets []*ast.WidgetV3, moduleName string, moduleID model.ID, entityContext string, mutator backend.PageMutator) ([]pages.Widget, error) {
	e := ctx.executor
	paramScope, paramEntityNames := mutator.ParamScope()
	widgetScope := mutator.WidgetScope()

	pb := &pageBuilder{
		writer:           e.writer,
		reader:           e.reader,
		moduleID:         moduleID,
		moduleName:       moduleName,
		entityContext:    entityContext,
		widgetScope:      widgetScope,
		paramScope:       paramScope,
		paramEntityNames: paramEntityNames,
		execCache:        ctx.Cache,
		fragments:        ctx.Fragments,
		themeRegistry:    ctx.GetThemeRegistry(),
		widgetBackend:    ctx.Backend,
	}

	var result []pages.Widget
	for _, w := range widgets {
		widget, err := pb.buildWidgetV3(w)
		if err != nil {
			return nil, mdlerrors.NewBackend("build widget "+w.Name, err)
		}
		if widget == nil {
			continue
		}
		result = append(result, widget)
	}
	return result, nil
}
