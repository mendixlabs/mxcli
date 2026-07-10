// SPDX-License-Identifier: Apache-2.0

// Package executor - Commands for agent-editor Knowledge Base documents.
//
// Handles `SHOW KNOWLEDGE BASES [IN module]` and
// `DESCRIBE KNOWLEDGE BASE Module.Name`. The underlying BSON is a
// CustomBlobDocuments$CustomBlobDocument with CustomDocumentType =
// "agenteditor.knowledgebase".
package executor

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/sdk/agenteditor"
)

// listAgentEditorKnowledgeBases handles SHOW KNOWLEDGE BASES [IN module].
func listAgentEditorKnowledgeBases(ctx *ExecContext, moduleName string) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	kbs, err := ctx.Backend.ListAgentEditorKnowledgeBases()
	if err != nil {
		return mdlerrors.NewBackend("list knowledge bases", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}

	result := &TableResult{
		Columns: []string{"Qualified Name", "Module", "Name", "Provider", "Key Constant", "Embedding Model"},
	}

	for _, k := range kbs {
		modID := h.FindModuleID(k.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName != "" && modName != moduleName {
			continue
		}
		keyConstant := ""
		if k.Key != nil {
			keyConstant = k.Key.QualifiedName
		}
		result.Rows = append(result.Rows, []any{
			fmt.Sprintf("%s.%s", modName, k.Name),
			modName,
			k.Name,
			k.Provider,
			keyConstant,
			k.ModelDisplayName,
		})
	}

	result.Summary = fmt.Sprintf("(%d knowledge base(s))", len(result.Rows))
	return writeResult(ctx, result)
}

// describeAgentEditorKnowledgeBase handles DESCRIBE KNOWLEDGE BASE Module.Name.
func describeAgentEditorKnowledgeBase(ctx *ExecContext, name ast.QualifiedName) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	k := findAgentEditorKnowledgeBase(ctx, name.Module, name.Name)
	if k == nil {
		return mdlerrors.NewNotFound("knowledge base", name.String())
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}
	modID := h.FindModuleID(k.ContainerID)
	modName := h.GetModuleName(modID)
	qualifiedName := fmt.Sprintf("%s.%s", modName, k.Name)

	if k.Documentation != "" {
		fmt.Fprintf(ctx.Output, "/**\n * %s\n */\n", k.Documentation)
	}

	fmt.Fprintf(ctx.Output, "create knowledge base %s (\n", qualifiedName)

	var lines []string
	if k.Provider != "" {
		lines = append(lines, fmt.Sprintf("  Provider: %s", k.Provider))
	}
	if k.Key != nil && k.Key.QualifiedName != "" {
		lines = append(lines, fmt.Sprintf("  Key: %s", k.Key.QualifiedName))
	}
	if k.ModelDisplayName != "" {
		lines = append(lines, fmt.Sprintf("  ModelDisplayName: '%s'", escapeSQLString(k.ModelDisplayName)))
	}
	if k.ModelName != "" {
		lines = append(lines, fmt.Sprintf("  ModelName: '%s'", escapeSQLString(k.ModelName)))
	}
	if k.KeyName != "" {
		lines = append(lines, fmt.Sprintf("  KeyName: '%s'", escapeSQLString(k.KeyName)))
	}
	if k.KeyID != "" {
		lines = append(lines, fmt.Sprintf("  KeyId: '%s'", escapeSQLString(k.KeyID)))
	}
	if k.Environment != "" {
		lines = append(lines, fmt.Sprintf("  Environment: '%s'", escapeSQLString(k.Environment)))
	}
	if k.DeepLinkURL != "" {
		lines = append(lines, fmt.Sprintf("  DeepLinkURL: '%s'", escapeSQLString(k.DeepLinkURL)))
	}

	for i, line := range lines {
		if i < len(lines)-1 {
			fmt.Fprintln(ctx.Output, line+",")
		} else {
			fmt.Fprintln(ctx.Output, line)
		}
	}

	fmt.Fprintln(ctx.Output, ");")
	fmt.Fprintln(ctx.Output, "/")
	return nil
}

// findAgentEditorKnowledgeBase looks up a KB by module and name.
func findAgentEditorKnowledgeBase(ctx *ExecContext, moduleName, kbName string) *agenteditor.KnowledgeBase {
	kbs, err := ctx.Backend.ListAgentEditorKnowledgeBases()
	if err != nil {
		return nil
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil
	}
	for _, k := range kbs {
		modID := h.FindModuleID(k.ContainerID)
		modName := h.GetModuleName(modID)
		if k.Name == kbName && modName == moduleName {
			return k
		}
	}
	return nil
}

// --- Executor method wrappers for backward compatibility ---
