// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// listDataTransformers handles LIST DATA TRANSFORMERS [IN module].
func listDataTransformers(ctx *ExecContext, moduleName string) error {
	transformers, err := ctx.Backend.ListDataTransformers()
	if err != nil {
		return mdlerrors.NewBackend("list data transformers", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	var rows [][]any
	for _, dt := range transformers {
		modID := h.FindModuleID(dt.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName != "" && !strings.EqualFold(modName, moduleName) {
			continue
		}
		qn := modName + "." + dt.Name
		steps := ""
		for _, s := range dt.Steps {
			if steps != "" {
				steps += " → "
			}
			steps += s.Technology
		}
		rows = append(rows, []any{qn, modName, dt.Name, dt.SourceType, steps})
	}

	if len(rows) == 0 {
		fmt.Fprintln(ctx.Output, "No data transformers found.")
		return nil
	}

	result := &TableResult{
		Columns: []string{"Qualified Name", "Module", "Name", "Source", "Steps"},
		Rows:    rows,
		Summary: fmt.Sprintf("(%d data transformers)", len(rows)),
	}
	return writeResult(ctx, result)
}

// describeDataTransformer handles DESCRIBE DATA TRANSFORMER Module.Name.
func describeDataTransformer(ctx *ExecContext, name ast.QualifiedName) error {
	transformers, err := ctx.Backend.ListDataTransformers()
	if err != nil {
		return mdlerrors.NewBackend("list data transformers", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, dt := range transformers {
		modID := h.FindModuleID(dt.ContainerID)
		modName := h.GetModuleName(modID)
		if !strings.EqualFold(modName, name.Module) || !strings.EqualFold(dt.Name, name.Name) {
			continue
		}

		w := ctx.Output

		// Emit re-executable MDL
		fmt.Fprintf(w, "create data transformer %s.%s\n", modName, dt.Name)

		// Source — collapse newlines into spaces for single-line string
		sourceContent := strings.ReplaceAll(dt.SourceJSON, "\n", " ")
		sourceContent = strings.ReplaceAll(sourceContent, "'", "''")
		fmt.Fprintf(w, "source %s '%s'\n", dt.SourceType, sourceContent)
		fmt.Fprintln(w, "{")

		for _, step := range dt.Steps {
			if strings.Contains(step.Expression, "\n") {
				// Multi-line: use $$ quoting
				fmt.Fprintf(w, "  %s $$\n%s\n  $$;\n", step.Technology, step.Expression)
			} else {
				// Single-line: use regular string
				expr := strings.ReplaceAll(step.Expression, "'", "''")
				fmt.Fprintf(w, "  %s '%s';\n", step.Technology, expr)
			}
		}

		fmt.Fprintln(w, "};")
		return nil
	}

	return mdlerrors.NewNotFound("data transformer", name.Module+"."+name.Name)
}

// execCreateDataTransformer creates a new data transformer.
func execCreateDataTransformer(ctx *ExecContext, s *ast.CreateDataTransformerStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	if err := checkFeature(ctx, "integration", "data_transformer",
		"create data transformer",
		"upgrade your project to 11.9+"); err != nil {
		return err
	}

	// Check for existing transformer with the same name.
	existing, existingID := findDataTransformer(ctx, s.Name.Module, s.Name.Name)
	if existing != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExists("data transformer", s.Name.String())
	}

	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return mdlerrors.NewNotFound("module", s.Name.Module)
	}

	dt := &model.DataTransformer{
		ContainerID: module.ID,
		Name:        s.Name.Name,
		SourceType:  s.SourceType,
		SourceJSON:  s.SourceJSON,
	}

	for _, step := range s.Steps {
		dt.Steps = append(dt.Steps, &model.DataTransformerStep{
			Technology: step.Technology,
			Expression: step.Expression,
		})
	}

	if existingID != "" {
		dt.ID = existingID
		if err := ctx.Backend.UpdateDataTransformer(dt); err != nil {
			return mdlerrors.NewBackend("update data transformer", err)
		}
		if !ctx.Quiet {
			fmt.Fprintf(ctx.Output, "Modified data transformer: %s.%s (%d steps)\n",
				s.Name.Module, s.Name.Name, len(dt.Steps))
		}
		return nil
	}

	if err := ctx.Backend.CreateDataTransformer(dt); err != nil {
		return mdlerrors.NewBackend("create data transformer", err)
	}

	if !ctx.Quiet {
		fmt.Fprintf(ctx.Output, "Created data transformer: %s.%s (%d steps)\n",
			s.Name.Module, s.Name.Name, len(dt.Steps))
	}
	return nil
}

// findDataTransformer looks up a data transformer by module and name, returning the struct and its ID.
func findDataTransformer(ctx *ExecContext, moduleName, name string) (*model.DataTransformer, model.ID) {
	transformers, err := ctx.Backend.ListDataTransformers()
	if err != nil {
		return nil, ""
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil, ""
	}
	for _, dt := range transformers {
		modID := h.FindModuleID(dt.ContainerID)
		modName := h.GetModuleName(modID)
		if strings.EqualFold(modName, moduleName) && strings.EqualFold(dt.Name, name) {
			return dt, dt.ID
		}
	}
	return nil, ""
}

// execDropDataTransformer deletes a data transformer.
func execDropDataTransformer(ctx *ExecContext, s *ast.DropDataTransformerStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	transformers, err := ctx.Backend.ListDataTransformers()
	if err != nil {
		return mdlerrors.NewBackend("list data transformers", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}

	for _, dt := range transformers {
		modID := h.FindModuleID(dt.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == s.Name.Module && dt.Name == s.Name.Name {
			if err := ctx.Backend.DeleteDataTransformer(dt.ID); err != nil {
				return mdlerrors.NewBackend("drop data transformer", err)
			}
			if !ctx.Quiet {
				fmt.Fprintf(ctx.Output, "Dropped data transformer: %s.%s\n", s.Name.Module, s.Name.Name)
			}
			return nil
		}
	}

	return mdlerrors.NewNotFound("data transformer", s.Name.Module+"."+s.Name.Name)
}

// Executor wrappers for unmigrated callers.
