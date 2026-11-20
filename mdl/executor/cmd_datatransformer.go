// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
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

	if len(rows) == 0 && ctx.Format != FormatJSON {
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

		// Emit re-executable MDL, the documentation first (mendixlabs/mxcli#1510).
		writeDocumentDocComment(w, dt.Documentation)
		fmt.Fprintf(w, "create or modify data transformer %s.%s%s\n", modName, dt.Name, describeFolderClause(ctx, dt.ContainerID))

		// Source — collapse newlines into spaces for single-line string
		sourceContent := strings.ReplaceAll(dt.SourceJSON, "\n", " ")
		sourceContent = strings.ReplaceAll(sourceContent, "'", "''")
		fmt.Fprintf(w, "source %s '%s'\n", dt.SourceType, sourceContent)
		fmt.Fprintln(w, "{")

		for _, step := range dt.Steps {
			if strings.Contains(step.Expression, "\n") && !strings.Contains(step.Expression, "$$") {
				// Multi-line: use $$ quoting
				fmt.Fprintf(w, "  %s $$\n%s\n  $$;\n", step.Technology, step.Expression)
			} else {
				// Single-line, or holding `$$`, which ends a dollar-quoted
				// literal early (ako/mxcli#1003): a regular string
				fmt.Fprintf(w, "  %s %s;\n", step.Technology, mdlQuote(ctx, step.Expression))
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

	if err := checkVersionGates(ctx, s); err != nil {
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

	var existingContainer model.ID
	if existing != nil {
		existingContainer = existing.ContainerID
	}
	containerID, err := containerForDocument(ctx, module.ID, s.Folder, existingContainer)
	if err != nil {
		return err
	}

	dt := &model.DataTransformer{
		ContainerID: containerID,
		Name:        s.Name.Name,
		SourceType:  s.SourceType,
		SourceJSON:  s.SourceJSON,
		// The doc comment (mendixlabs/mxcli#1510).
		Documentation: s.Documentation,
	}
	if existing != nil {
		// Excluded is model state, not script state (#914).
		dt.Excluded = existing.Excluded
		// A rewrite that carried no doc comment keeps the stored one (#1018).
		dt.Documentation = carriedDocumentation(s.DocumentationSet, s.Documentation, existing.Documentation)
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
		if _, err := applyDocumentFolder(ctx, dt.ID, existingContainer, containerID); err != nil {
			return err
		}
		if !ctx.Quiet {
			ctx.ReportMutation("Modified", "data transformer: %s.%s (%d steps)",
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
	// Prefer the live transformer over an excluded twin of the same name (#914).
	if dt, ok := pickLive(transformers,
		func(dt *model.DataTransformer) bool {
			return strings.EqualFold(h.GetModuleName(h.FindModuleID(dt.ContainerID)), moduleName) &&
				strings.EqualFold(dt.Name, name)
		},
		func(dt *model.DataTransformer) bool { return dt.Excluded },
	); ok {
		return dt, dt.ID
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
