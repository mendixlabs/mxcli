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

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/sdk/agenteditor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
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
		outputJavadocIndented(ctx.Output, c.Documentation, "")
	}

	fmt.Fprintf(ctx.Output, "create or modify consumed mcp service %s%s (\n", qualifiedName, describeFolderClause(ctx, c.ContainerID))

	var lines []string
	if c.ProtocolVersion != "" {
		lines = append(lines, fmt.Sprintf("  ProtocolVersion: '%s'", escapeSQLString(c.ProtocolVersion)))
	}
	if c.Endpoint != nil && c.Endpoint.QualifiedName != "" {
		lines = append(lines, "  Endpoint: @"+c.Endpoint.QualifiedName)
	}
	if c.AuthenticationMicroflow != nil && c.AuthenticationMicroflow.QualifiedName != "" {
		lines = append(lines, "  AuthenticationMicroflow: "+c.AuthenticationMicroflow.QualifiedName)
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

// consumedMCPEndpointRef resolves a consumed MCP service's Endpoint (#2060).
// AgentEditorCommons reads the constant's value with GetConstantValue and
// imports the service only when it is not empty, so the reference must be a
// String constant: the document has no other way to hold the URL.
func consumedMCPEndpointRef(ctx *ExecContext, qn ast.QualifiedName) (*agenteditor.DocRef, error) {
	consts, err := ctx.Backend.ListConstants()
	if err != nil {
		return nil, mdlerrors.NewBackend("list constants", err)
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range consts {
		if c.Name != qn.Name || h.GetModuleName(h.FindModuleID(c.ContainerID)) != qn.Module {
			continue
		}
		if c.Type.Kind != "String" {
			return nil, fmt.Errorf("endpoint must be a String constant holding the server URL; %s is %s", qn, c.Type.Kind)
		}
		return &agenteditor.DocRef{DocumentID: string(c.ID), QualifiedName: qn.String()}, nil
	}
	return nil, fmt.Errorf("endpoint: constant not found: %s", qn)
}

// consumedMCPCredentialsProblem is why MCPClient cannot use mf as a consumed
// MCP service's credentials microflow, or "".
func consumedMCPCredentialsProblem(mf *microflows.Microflow) string {
	if len(mf.Parameters) > 0 {
		return fmt.Sprintf("must take no parameters: MCPClient calls it without arguments (it has $%s)", mf.Parameters[0].Name)
	}
	if lt, ok := mf.ReturnType.(*microflows.ListType); !ok || lt.EntityQualifiedName != "System.HttpHeader" {
		return "must return List of System.HttpHeader: MCPClient sends that list as the request headers"
	}
	return ""
}
