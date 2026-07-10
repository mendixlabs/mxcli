// SPDX-License-Identifier: Apache-2.0

// Package executor - Commands for agent-editor Agent documents.
//
// Handles `SHOW AGENTS [IN module]` and `DESCRIBE AGENT Module.Name`.
// The underlying BSON is a CustomBlobDocuments$CustomBlobDocument with
// CustomDocumentType = "agenteditor.agent".
package executor

import (
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/sdk/agenteditor"
)

// listAgentEditorAgents handles SHOW AGENTS [IN module].
func listAgentEditorAgents(ctx *ExecContext, moduleName string) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	agents, err := ctx.Backend.ListAgentEditorAgents()
	if err != nil {
		return mdlerrors.NewBackend("list agents", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}

	result := &TableResult{
		Columns: []string{"Qualified Name", "Module", "Name", "Usage", "Model", "Tools", "KBs"},
	}

	for _, a := range agents {
		modID := h.FindModuleID(a.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName != "" && modName != moduleName {
			continue
		}
		modelName := ""
		if a.Model != nil {
			modelName = a.Model.QualifiedName
		}
		result.Rows = append(result.Rows, []any{
			fmt.Sprintf("%s.%s", modName, a.Name),
			modName,
			a.Name,
			a.UsageType,
			modelName,
			len(a.Tools),
			len(a.KBTools),
		})
	}

	result.Summary = fmt.Sprintf("(%d agent(s))", len(result.Rows))
	return writeResult(ctx, result)
}

// describeAgentEditorAgent handles DESCRIBE AGENT Module.Name. Emits a
// round-trippable CREATE AGENT statement reflecting the Contents JSON.
func describeAgentEditorAgent(ctx *ExecContext, name ast.QualifiedName) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	a := findAgentEditorAgent(ctx, name.Module, name.Name)
	if a == nil {
		return mdlerrors.NewNotFound("agent", name.String())
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}
	modID := h.FindModuleID(a.ContainerID)
	modName := h.GetModuleName(modID)
	qualifiedName := fmt.Sprintf("%s.%s", modName, a.Name)

	if a.Documentation != "" {
		fmt.Fprintf(ctx.Output, "/**\n * %s\n */\n", a.Documentation)
	}

	fmt.Fprintf(ctx.Output, "create agent %s (\n", qualifiedName)

	// Build property lines. User-set properties are emitted in a stable
	// order; empty values are omitted.
	var lines []string
	if a.UsageType != "" {
		lines = append(lines, fmt.Sprintf("  UsageType: %s", a.UsageType))
	}
	if a.Description != "" {
		lines = append(lines, fmt.Sprintf("  Description: '%s'", escapeSQLString(a.Description)))
	}
	if a.Model != nil && a.Model.QualifiedName != "" {
		lines = append(lines, fmt.Sprintf("  Model: %s", a.Model.QualifiedName))
	}
	if a.Entity != nil && a.Entity.QualifiedName != "" {
		lines = append(lines, fmt.Sprintf("  Entity: %s", a.Entity.QualifiedName))
	}
	if len(a.Variables) > 0 {
		var parts []string
		for _, v := range a.Variables {
			kind := "String"
			if v.IsAttributeInEntity {
				kind = "EntityAttribute"
			}
			parts = append(parts, fmt.Sprintf("\"%s\": %s", v.Key, kind))
		}
		lines = append(lines, fmt.Sprintf("  Variables: (%s)", strings.Join(parts, ", ")))
	}
	if a.MaxTokens != nil {
		lines = append(lines, fmt.Sprintf("  MaxTokens: %d", *a.MaxTokens))
	}
	if a.ToolChoice != "" {
		lines = append(lines, fmt.Sprintf("  ToolChoice: %s", a.ToolChoice))
	}
	if a.Temperature != nil {
		lines = append(lines, fmt.Sprintf("  Temperature: %g", *a.Temperature))
	}
	if a.TopP != nil {
		lines = append(lines, fmt.Sprintf("  TopP: %g", *a.TopP))
	}
	if a.SystemPrompt != "" {
		lines = append(lines, fmt.Sprintf("  SystemPrompt: %s", formatAgentString(a.SystemPrompt)))
	}
	if a.UserPrompt != "" {
		lines = append(lines, fmt.Sprintf("  UserPrompt: %s", formatAgentString(a.UserPrompt)))
	}

	for i, line := range lines {
		if i < len(lines)-1 {
			fmt.Fprintln(ctx.Output, line+",")
		} else {
			fmt.Fprintln(ctx.Output, line)
		}
	}

	// Body with TOOL / MCP SERVICE / KNOWLEDGE BASE blocks if present.
	hasBody := len(a.Tools) > 0 || len(a.KBTools) > 0
	if hasBody {
		fmt.Fprintln(ctx.Output, ")")
		fmt.Fprintln(ctx.Output, "{")

		for i, t := range a.Tools {
			emitToolBlock(ctx, t)
			if i < len(a.Tools)-1 || len(a.KBTools) > 0 {
				fmt.Fprintln(ctx.Output)
			}
		}
		for i, kb := range a.KBTools {
			emitKBBlock(ctx, kb)
			if i < len(a.KBTools)-1 {
				fmt.Fprintln(ctx.Output)
			}
		}

		fmt.Fprintln(ctx.Output, "};")
	} else {
		fmt.Fprintln(ctx.Output, ");")
	}
	fmt.Fprintln(ctx.Output, "/")
	return nil
}

// emitToolBlock writes one TOOL or MCP SERVICE block for the agent body.
func emitToolBlock(ctx *ExecContext, t agenteditor.AgentTool) {
	switch t.ToolType {
	case "mcp":
		if t.Document == nil {
			// malformed — skip
			return
		}
		fmt.Fprintf(ctx.Output, "  mcp service %s {\n", t.Document.QualifiedName)
		fmt.Fprintf(ctx.Output, "    Enabled: %t\n", t.Enabled)
		if t.Description != "" {
			fmt.Fprintf(ctx.Output, "    Description: '%s'\n", escapeSQLString(t.Description))
		}
		fmt.Fprintln(ctx.Output, "  }")
	default:
		// Microflow or unknown tool type — emit generic TOOL block.
		name := t.Name
		if name == "" {
			name = "Tool_" + strings.ReplaceAll(t.ID, "-", "")[:8]
		}
		fmt.Fprintf(ctx.Output, "  tool %s {\n", name)
		if t.ToolType != "" {
			fmt.Fprintf(ctx.Output, "    ToolType: %s,\n", t.ToolType)
		}
		if t.Document != nil && t.Document.QualifiedName != "" {
			fmt.Fprintf(ctx.Output, "    Document: %s,\n", t.Document.QualifiedName)
		}
		fmt.Fprintf(ctx.Output, "    Enabled: %t", t.Enabled)
		if t.Description != "" {
			fmt.Fprintln(ctx.Output, ",")
			fmt.Fprintf(ctx.Output, "    Description: '%s'\n", escapeSQLString(t.Description))
		} else {
			fmt.Fprintln(ctx.Output)
		}
		fmt.Fprintln(ctx.Output, "  }")
	}
}

// emitKBBlock writes one KNOWLEDGE BASE block for the agent body.
func emitKBBlock(ctx *ExecContext, kb agenteditor.AgentKBTool) {
	name := kb.Name
	if name == "" {
		name = "KB_" + strings.ReplaceAll(kb.ID, "-", "")[:8]
	}
	fmt.Fprintf(ctx.Output, "  knowledge base %s {\n", name)
	if kb.Document != nil && kb.Document.QualifiedName != "" {
		fmt.Fprintf(ctx.Output, "    Source: %s,\n", kb.Document.QualifiedName)
	}
	if kb.CollectionIdentifier != "" {
		fmt.Fprintf(ctx.Output, "    Collection: '%s',\n", escapeSQLString(kb.CollectionIdentifier))
	}
	if kb.MaxResults != 0 {
		fmt.Fprintf(ctx.Output, "    MaxResults: %d,\n", kb.MaxResults)
	}
	if kb.Description != "" {
		fmt.Fprintf(ctx.Output, "    Description: '%s',\n", escapeSQLString(kb.Description))
	}
	fmt.Fprintf(ctx.Output, "    Enabled: %t\n", kb.Enabled)
	fmt.Fprintln(ctx.Output, "  }")
}

// findAgentEditorAgent looks up an agent by module and name.
func findAgentEditorAgent(ctx *ExecContext, moduleName, agentName string) *agenteditor.Agent {
	agents, err := ctx.Backend.ListAgentEditorAgents()
	if err != nil {
		return nil
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil
	}
	for _, a := range agents {
		modID := h.FindModuleID(a.ContainerID)
		modName := h.GetModuleName(modID)
		if a.Name == agentName && modName == moduleName {
			return a
		}
	}
	return nil
}

// formatAgentString formats a string value for MDL output. Single-line
// values use single quotes; multi-line values use $$...$$ dollar-quoting
// so they round-trip cleanly through the parser (which doesn't support
// newlines inside single-quoted strings).
func formatAgentString(s string) string {
	if strings.ContainsAny(s, "\r\n") {
		return "$$" + s + "$$"
	}
	return "'" + escapeSQLString(s) + "'"
}
