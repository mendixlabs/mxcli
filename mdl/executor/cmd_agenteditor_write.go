// SPDX-License-Identifier: Apache-2.0

// Package executor - CREATE/DROP handlers for Consumed MCP Service,
// Knowledge Base, and Agent agent-editor documents.
package executor

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/sdk/agenteditor"
)

// ---------------------------------------------------------------------------
// CONSUMED MCP SERVICE
// ---------------------------------------------------------------------------

func execCreateConsumedMCPService(ctx *ExecContext, s *ast.CreateConsumedMCPServiceStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	existing := findAgentEditorConsumedMCPService(ctx, s.Name.Module, s.Name.Name)
	if existing != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExists("consumed mcp service", s.Name.String())
	}

	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	c := &agenteditor.ConsumedMCPService{
		ContainerID:              module.ID,
		Name:                     s.Name.Name,
		Documentation:            s.OuterDocumentation,
		ProtocolVersion:          s.ProtocolVersion,
		Version:                  s.Version,
		InnerDocumentation:       s.InnerDocumentation,
		ConnectionTimeoutSeconds: s.ConnectionTimeoutSeconds,
	}

	if existing != nil {
		c.ID = existing.ID
		if err := ctx.Backend.UpdateAgentEditorConsumedMCPService(c); err != nil {
			return mdlerrors.NewBackend("update consumed mcp service", err)
		}
		invalidateHierarchy(ctx)
		fmt.Fprintf(ctx.Output, "Modified consumed mcp service: %s\n", s.Name)
		return nil
	}

	if err := ctx.Backend.CreateAgentEditorConsumedMCPService(c); err != nil {
		return mdlerrors.NewBackend("create consumed mcp service", err)
	}
	invalidateHierarchy(ctx)
	fmt.Fprintf(ctx.Output, "Created consumed mcp service: %s\n", s.Name)
	return nil
}

func execDropConsumedMCPService(ctx *ExecContext, s *ast.DropConsumedMCPServiceStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	c := findAgentEditorConsumedMCPService(ctx, s.Name.Module, s.Name.Name)
	if c == nil {
		return mdlerrors.NewNotFound("consumed mcp service", s.Name.String())
	}
	if err := ctx.Backend.DeleteAgentEditorConsumedMCPService(string(c.ID)); err != nil {
		return mdlerrors.NewBackend("delete consumed mcp service", err)
	}
	fmt.Fprintf(ctx.Output, "Dropped consumed mcp service: %s\n", s.Name)
	return nil
}

// ---------------------------------------------------------------------------
// KNOWLEDGE BASE
// ---------------------------------------------------------------------------

func execCreateKnowledgeBase(ctx *ExecContext, s *ast.CreateKnowledgeBaseStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	existing := findAgentEditorKnowledgeBase(ctx, s.Name.Module, s.Name.Name)
	if existing != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExists("knowledge base", s.Name.String())
	}

	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	var keyRef *agenteditor.ConstantRef
	if s.Key != nil {
		keyRef, err = resolveConstantRef(ctx, *s.Key)
		if err != nil {
			return fmt.Errorf("create knowledge base %s: %w", s.Name, err)
		}
	}

	provider := s.Provider
	if provider == "" {
		provider = "MxCloudGenAI"
	}

	k := &agenteditor.KnowledgeBase{
		ContainerID:      module.ID,
		Name:             s.Name.Name,
		Documentation:    s.Documentation,
		Provider:         provider,
		Key:              keyRef,
		ModelDisplayName: s.ModelDisplayName,
		ModelName:        s.ModelName,
		KeyName:          s.KeyName,
		KeyID:            s.KeyID,
		Environment:      s.Environment,
		DeepLinkURL:      s.DeepLinkURL,
	}

	if existing != nil {
		k.ID = existing.ID
		if err := ctx.Backend.UpdateAgentEditorKnowledgeBase(k); err != nil {
			return mdlerrors.NewBackend("update knowledge base", err)
		}
		invalidateHierarchy(ctx)
		fmt.Fprintf(ctx.Output, "Modified knowledge base: %s\n", s.Name)
		return nil
	}

	if err := ctx.Backend.CreateAgentEditorKnowledgeBase(k); err != nil {
		return mdlerrors.NewBackend("create knowledge base", err)
	}
	invalidateHierarchy(ctx)
	fmt.Fprintf(ctx.Output, "Created knowledge base: %s\n", s.Name)
	return nil
}

func execDropKnowledgeBase(ctx *ExecContext, s *ast.DropKnowledgeBaseStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	k := findAgentEditorKnowledgeBase(ctx, s.Name.Module, s.Name.Name)
	if k == nil {
		return mdlerrors.NewNotFound("knowledge base", s.Name.String())
	}
	if err := ctx.Backend.DeleteAgentEditorKnowledgeBase(string(k.ID)); err != nil {
		return mdlerrors.NewBackend("delete knowledge base", err)
	}
	fmt.Fprintf(ctx.Output, "Dropped knowledge base: %s\n", s.Name)
	return nil
}

// ---------------------------------------------------------------------------
// AGENT
// ---------------------------------------------------------------------------

func execCreateAgent(ctx *ExecContext, s *ast.CreateAgentStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	existingAgent := findAgentEditorAgent(ctx, s.Name.Module, s.Name.Name)
	if existingAgent != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExists("agent", s.Name.String())
	}

	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	a := &agenteditor.Agent{
		ContainerID:   module.ID,
		Name:          s.Name.Name,
		Documentation: s.Documentation,
		Description:   s.Description,
		SystemPrompt:  s.SystemPrompt,
		UserPrompt:    s.UserPrompt,
		UsageType:     s.UsageType,
		MaxTokens:     s.MaxTokens,
		ToolChoice:    s.ToolChoice,
		Temperature:   s.Temperature,
		TopP:          s.TopP,
	}

	// Resolve Model reference
	if s.Model != nil {
		m := findAgentEditorModel(ctx, s.Model.Module, s.Model.Name)
		if m == nil {
			return fmt.Errorf("create agent %s: model not found: %s", s.Name, s.Model)
		}
		a.Model = &agenteditor.DocRef{
			DocumentID:    string(m.ID),
			QualifiedName: s.Model.String(),
		}
	}

	// Resolve Entity reference. The documentId for entity references is an
	// opaque agent-editor-internal ID (not a unit UUID), so we set only
	// qualifiedName here. ASU_AgentEditor populates documentId at runtime.
	if s.Entity != nil {
		a.Entity = &agenteditor.DocRef{
			QualifiedName: s.Entity.String(),
		}
	}

	// Variables
	for _, v := range s.Variables {
		a.Variables = append(a.Variables, agenteditor.AgentVar{
			Key:                 v.Key,
			IsAttributeInEntity: v.IsAttributeInEntity,
		})
	}

	// Tools (MCP SERVICE and TOOL blocks)
	for _, td := range s.Tools {
		at := agenteditor.AgentTool{
			Name:        td.Name,
			Description: td.Description,
			Enabled:     td.Enabled,
			ToolType:    td.ToolType,
		}
		if td.Document != nil && td.ToolType == "mcp" {
			// Resolve MCP service document reference
			svc := findAgentEditorConsumedMCPService(ctx, td.Document.Module, td.Document.Name)
			if svc == nil {
				return fmt.Errorf("create agent %s: consumed mcp service not found: %s", s.Name, td.Document)
			}
			at.Document = &agenteditor.DocRef{
				DocumentID:    string(svc.ID),
				QualifiedName: td.Document.String(),
			}
		}
		a.Tools = append(a.Tools, at)
	}

	// Knowledge base tools
	for _, kbd := range s.KBTools {
		akt := agenteditor.AgentKBTool{
			Name:                 kbd.Name,
			Description:          kbd.Description,
			Enabled:              kbd.Enabled,
			CollectionIdentifier: kbd.Collection,
			MaxResults:           kbd.MaxResults,
		}
		if kbd.Source != nil {
			kb := findAgentEditorKnowledgeBase(ctx, kbd.Source.Module, kbd.Source.Name)
			if kb == nil {
				return fmt.Errorf("create agent %s: knowledge base not found: %s", s.Name, kbd.Source)
			}
			akt.Document = &agenteditor.DocRef{
				DocumentID:    string(kb.ID),
				QualifiedName: kbd.Source.String(),
			}
		}
		a.KBTools = append(a.KBTools, akt)
	}

	if existingAgent != nil {
		a.ID = existingAgent.ID
		if err := ctx.Backend.UpdateAgentEditorAgent(a); err != nil {
			return mdlerrors.NewBackend("update agent", err)
		}
		invalidateHierarchy(ctx)
		fmt.Fprintf(ctx.Output, "Modified agent: %s\n", s.Name)
		return nil
	}

	if err := ctx.Backend.CreateAgentEditorAgent(a); err != nil {
		return mdlerrors.NewBackend("create agent", err)
	}
	invalidateHierarchy(ctx)
	fmt.Fprintf(ctx.Output, "Created agent: %s\n", s.Name)
	return nil
}

func execDropAgent(ctx *ExecContext, s *ast.DropAgentStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	a := findAgentEditorAgent(ctx, s.Name.Module, s.Name.Name)
	if a == nil {
		return mdlerrors.NewNotFound("agent", s.Name.String())
	}
	if err := ctx.Backend.DeleteAgentEditorAgent(string(a.ID)); err != nil {
		return mdlerrors.NewBackend("delete agent", err)
	}
	fmt.Fprintf(ctx.Output, "Dropped agent: %s\n", s.Name)
	return nil
}
