// SPDX-License-Identifier: Apache-2.0

// Package executor - Commands for agent-editor Consumed MCP Service documents.
//
// Handles `SHOW CONSUMED MCP SERVICES [IN module]` and
// `DESCRIBE CONSUMED MCP SERVICE Module.Name`. The underlying BSON is a
// CustomBlobDocuments$CustomBlobDocument with CustomDocumentType =
// "agenteditor.consumedMCPService".
package executor

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/sdk/agenteditor"
)

// listAgentEditorConsumedMCPServices handles SHOW CONSUMED MCP SERVICES [IN module].
func listAgentEditorConsumedMCPServices(ctx *ExecContext, moduleName string) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	svcs, err := ctx.Backend.ListAgentEditorConsumedMCPServices()
	if err != nil {
		return mdlerrors.NewBackend("list consumed mcp services", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}

	result := &TableResult{
		Columns: []string{"Qualified Name", "Module", "Name", "Protocol", "Version", "Timeout"},
	}

	for _, c := range svcs {
		modID := h.FindModuleID(c.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName != "" && modName != moduleName {
			continue
		}
		result.Rows = append(result.Rows, []any{
			fmt.Sprintf("%s.%s", modName, c.Name),
			modName,
			c.Name,
			c.ProtocolVersion,
			c.Version,
			c.ConnectionTimeoutSeconds,
		})
	}

	result.Summary = fmt.Sprintf("(%d consumed mcp service(s))", len(result.Rows))
	return writeResult(ctx, result)
}

// describeAgentEditorConsumedMCPService handles DESCRIBE CONSUMED MCP SERVICE Module.Name.
func describeAgentEditorConsumedMCPService(ctx *ExecContext, name ast.QualifiedName) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	c := findAgentEditorConsumedMCPService(ctx, name.Module, name.Name)
	if c == nil {
		return mdlerrors.NewNotFound("consumed mcp service", name.String())
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}
	modID := h.FindModuleID(c.ContainerID)
	modName := h.GetModuleName(modID)
	qualifiedName := fmt.Sprintf("%s.%s", modName, c.Name)

	if c.Documentation != "" {
		fmt.Fprintf(ctx.Output, "/**\n * %s\n */\n", c.Documentation)
	}

	fmt.Fprintf(ctx.Output, "create consumed mcp service %s (\n", qualifiedName)

	var lines []string
	if c.ProtocolVersion != "" {
		lines = append(lines, fmt.Sprintf("  ProtocolVersion: '%s'", escapeSQLString(c.ProtocolVersion)))
	}
	if c.Version != "" {
		lines = append(lines, fmt.Sprintf("  Version: '%s'", escapeSQLString(c.Version)))
	}
	if c.ConnectionTimeoutSeconds != 0 {
		lines = append(lines, fmt.Sprintf("  ConnectionTimeoutSeconds: %d", c.ConnectionTimeoutSeconds))
	}
	if c.InnerDocumentation != "" {
		lines = append(lines, fmt.Sprintf("  Documentation: '%s'", escapeSQLString(c.InnerDocumentation)))
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

// findAgentEditorConsumedMCPService looks up an MCP service by module and name.
func findAgentEditorConsumedMCPService(ctx *ExecContext, moduleName, svcName string) *agenteditor.ConsumedMCPService {
	svcs, err := ctx.Backend.ListAgentEditorConsumedMCPServices()
	if err != nil {
		return nil
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil
	}
	for _, c := range svcs {
		modID := h.FindModuleID(c.ContainerID)
		modName := h.GetModuleName(modID)
		if c.Name == svcName && modName == moduleName {
			return c
		}
	}
	return nil
}

// --- Executor method wrappers for backward compatibility ---
