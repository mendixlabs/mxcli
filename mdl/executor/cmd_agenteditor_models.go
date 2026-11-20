// SPDX-License-Identifier: Apache-2.0

// Package executor - Commands for agent-editor Model documents.
//
// Handles `SHOW MODELS [IN module]` and `DESCRIBE MODEL Module.Name`.
// The underlying BSON is a CustomBlobDocuments$CustomBlobDocument with
// CustomDocumentType = "agenteditor.model". See
// docs/11-proposals/PROPOSAL_agent_document_support.md for schema.
package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/agenteditor"
)

// listAgentEditorModels handles SHOW MODELS [IN module].
func listAgentEditorModels(ctx *ExecContext, moduleName string) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	models, err := ctx.Backend.ListAgentEditorModels()
	if err != nil {
		return mdlerrors.NewBackend("list models", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}

	result := &TableResult{
		Columns: []string{"Qualified Name", "Module", "Name", "Provider", "Key Constant", "Display Name"},
	}

	for _, m := range models {
		modID := h.FindModuleID(m.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName != "" && modName != moduleName {
			continue
		}

		keyConstant := ""
		if m.Key != nil {
			keyConstant = m.Key.QualifiedName
		}

		result.Rows = append(result.Rows, []any{
			fmt.Sprintf("%s.%s", modName, m.Name),
			modName,
			m.Name,
			m.Provider,
			keyConstant,
			m.DisplayName,
		})
	}

	result.Summary = fmt.Sprintf("(%d model(s))", len(result.Rows))
	return writeResult(ctx, result)
}

// describeAgentEditorModel handles DESCRIBE MODEL Module.Name.
// Emits a round-trippable CREATE MODEL statement.
func describeAgentEditorModel(ctx *ExecContext, name ast.QualifiedName) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	m := findAgentEditorModel(ctx, name.Module, name.Name)
	if m == nil {
		return mdlerrors.NewNotFound("model", name.String())
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}
	modID := h.FindModuleID(m.ContainerID)
	modName := h.GetModuleName(modID)
	qualifiedName := fmt.Sprintf("%s.%s", modName, m.Name)

	if m.Documentation != "" {
		outputJavadocIndented(ctx.Output, m.Documentation, "")
	}

	fmt.Fprintf(ctx.Output, "create or modify ai model %s%s (\n", qualifiedName, describeFolderClause(ctx, m.ContainerID))

	// Emit properties in stable order. User-set properties (Provider, Key)
	// come first; Portal-populated metadata comes last and only if non-empty.
	var lines []string
	if m.Provider != "" {
		lines = append(lines, fmt.Sprintf("  Provider: %s", m.Provider))
	}
	if m.Key != nil && m.Key.QualifiedName != "" {
		lines = append(lines, fmt.Sprintf("  Key: @%s", m.Key.QualifiedName))
	}
	// Portal-populated fields — round-tripped but flagged read-only in MDL.
	if m.DisplayName != "" {
		lines = append(lines, fmt.Sprintf("  DisplayName: '%s'", escapeSQLString(m.DisplayName)))
	}
	if m.KeyName != "" {
		lines = append(lines, fmt.Sprintf("  KeyName: '%s'", escapeSQLString(m.KeyName)))
	}
	if m.KeyID != "" {
		lines = append(lines, fmt.Sprintf("  KeyId: '%s'", escapeSQLString(m.KeyID)))
	}
	if m.Environment != "" {
		lines = append(lines, fmt.Sprintf("  Environment: '%s'", escapeSQLString(m.Environment)))
	}
	if m.ResourceName != "" {
		lines = append(lines, fmt.Sprintf("  ResourceName: '%s'", escapeSQLString(m.ResourceName)))
	}
	if m.DeepLinkURL != "" {
		lines = append(lines, fmt.Sprintf("  DeepLinkURL: '%s'", escapeSQLString(m.DeepLinkURL)))
	}

	for i, line := range lines {
		if i < len(lines)-1 {
			fmt.Fprintln(ctx.Output, line+",")
		} else {
			fmt.Fprintln(ctx.Output, line)
		}
	}

	fmt.Fprintln(ctx.Output, ");")
	return nil
}

// execCreateAgentEditorModel handles CREATE MODEL Module.Name (...).
func execCreateAgentEditorModel(ctx *ExecContext, s *ast.CreateModelStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Agent Editor documents need Studio Pro 11.9+ and the AgentEditorCommons
	// module. Nothing downstream catches an older project: the documents are
	// custom blobs, so mxbuild does not validate them and the build stays green
	// while Studio Pro cannot open the result.
	if err := checkVersionGates(ctx, s); err != nil {
		return err
	}

	existing := findAgentEditorModel(ctx, s.Name.Module, s.Name.Name)
	if existing != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExists("model", s.Name.String())
	}

	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	var keyRef *agenteditor.ConstantRef
	if s.Key != nil {
		keyRef, err = resolveConstantRef(ctx, *s.Key)
		if err != nil {
			return fmt.Errorf("create model %s: %w", s.Name, err)
		}
	}

	provider := s.Provider
	if provider == "" {
		provider = "MxCloudGenAI"
	}

	var existingContainerM model.ID
	if existing != nil {
		existingContainerM = existing.ContainerID
	}
	containerIDM, err := containerForDocument(ctx, module.ID, s.Folder, existingContainerM)
	if err != nil {
		return err
	}

	m := &agenteditor.Model{
		ContainerID:   containerIDM,
		Name:          s.Name.Name,
		Documentation: s.Documentation,
		Provider:      provider,
		Key:           keyRef,
		DisplayName:   s.DisplayName,
		KeyName:       s.KeyName,
		KeyID:         s.KeyID,
		Environment:   s.Environment,
		ResourceName:  s.ResourceName,
		DeepLinkURL:   s.DeepLinkURL,
	}
	// A rewrite that carried no doc comment keeps the stored one (#1018).
	if existing != nil {
		m.Documentation = carriedDocumentation(s.DocumentationSet, s.Documentation, existing.Documentation)
	}

	if existing != nil {
		// The model Agent Editor selected from the Portal resource: MDL has no
		// spelling for it, and a restatement wrote both empty.
		m.Type, m.InnerName = existing.Type, existing.InnerName
		m.ID = existing.ID
		// Excluded is model state, not script state (#914).
		m.Excluded = existing.Excluded
		if err := ctx.Backend.UpdateAgentEditorModel(m); err != nil {
			return mdlerrors.NewBackend("update model", err)
		}
		invalidateHierarchy(ctx)
		if _, err := applyDocumentFolder(ctx, m.ID, existingContainerM, containerIDM); err != nil {
			return err
		}
		ctx.ReportMutation("Modified", "model: %s", s.Name)
		return nil
	}

	if err := ctx.Backend.CreateAgentEditorModel(m); err != nil {
		return mdlerrors.NewBackend("create model", err)
	}
	invalidateHierarchy(ctx)
	fmt.Fprintf(ctx.Output, "Created model: %s\n", s.Name)
	return nil
}

// execDropAgentEditorModel handles DROP MODEL Module.Name.
func execDropAgentEditorModel(ctx *ExecContext, s *ast.DropModelStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	m := findAgentEditorModel(ctx, s.Name.Module, s.Name.Name)
	if m == nil {
		return mdlerrors.NewNotFound("model", s.Name.String())
	}

	if err := ctx.Backend.DeleteAgentEditorModel(string(m.ID)); err != nil {
		return mdlerrors.NewBackend("delete model", err)
	}
	fmt.Fprintf(ctx.Output, "Dropped model: %s\n", s.Name)
	return nil
}

// resolveConstantRef looks up a String constant by qualified name and
// returns a ConstantRef ready to embed in a Model/KnowledgeBase
// document's providerFields.key field.
func resolveConstantRef(ctx *ExecContext, name ast.QualifiedName) (*agenteditor.ConstantRef, error) {
	consts, err := ctx.Backend.ListConstants()
	if err != nil {
		return nil, fmt.Errorf("failed to list constants: %w", err)
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range consts {
		modID := h.FindModuleID(c.ContainerID)
		modName := h.GetModuleName(modID)
		if c.Name == name.Name && modName == name.Module {
			return &agenteditor.ConstantRef{
				DocumentID:    string(c.ID),
				QualifiedName: name.String(),
			}, nil
		}
	}
	return nil, fmt.Errorf("constant not found: %s", name)
}

// findAgentEditorModel looks up a model by module and name.
func findAgentEditorModel(ctx *ExecContext, moduleName, modelName string) *agenteditor.Model {
	models, err := ctx.Backend.ListAgentEditorModels()
	if err != nil {
		return nil
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil
	}
	for _, m := range models {
		modID := h.FindModuleID(m.ContainerID)
		modName := h.GetModuleName(modID)
		if m.Name == modelName && modName == moduleName {
			return m
		}
	}
	return nil
}

// --- Executor method wrappers for backward compatibility ---
