// SPDX-License-Identifier: Apache-2.0

// Package executor - CREATE/DROP handlers for Consumed MCP Service,
// Knowledge Base, and Agent agent-editor documents.
package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/agenteditor"
)

// ---------------------------------------------------------------------------
// CONSUMED MCP SERVICE
// ---------------------------------------------------------------------------

func execCreateConsumedMCPService(ctx *ExecContext, s *ast.CreateConsumedMCPServiceStmt) error {
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

	existing := findAgentEditorConsumedMCPService(ctx, s.Name.Module, s.Name.Name)
	if existing != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExists("consumed mcp service", s.Name.String())
	}

	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	var existingContainerC model.ID
	if existing != nil {
		existingContainerC = existing.ContainerID
	}
	containerIDC, err := containerForDocument(ctx, module.ID, s.Folder, existingContainerC)
	if err != nil {
		return err
	}

	c := &agenteditor.ConsumedMCPService{
		ContainerID:              containerIDC,
		Name:                     s.Name.Name,
		Documentation:            s.OuterDocumentation,
		ProtocolVersion:          s.ProtocolVersion,
		Version:                  s.Version,
		InnerDocumentation:       s.InnerDocumentation,
		ConnectionTimeoutSeconds: s.ConnectionTimeoutSeconds,
	}
	// A rewrite that carried no doc comment keeps the stored one (#1018).
	if existing != nil {
		c.Documentation = carriedDocumentation(s.DocumentationSet, s.OuterDocumentation, existing.Documentation)
	}

	if existing != nil {
		c.ID = existing.ID
		// Excluded is model state, not script state (#914).
		c.Excluded = existing.Excluded
		if err := ctx.Backend.UpdateAgentEditorConsumedMCPService(c); err != nil {
			return mdlerrors.NewBackend("update consumed mcp service", err)
		}
		invalidateHierarchy(ctx)
		if _, err := applyDocumentFolder(ctx, c.ID, existingContainerC, containerIDC); err != nil {
			return err
		}
		ctx.ReportMutation("Modified", "consumed mcp service: %s", s.Name)
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

	// Agent Editor documents need Studio Pro 11.9+ and the AgentEditorCommons
	// module. Nothing downstream catches an older project: the documents are
	// custom blobs, so mxbuild does not validate them and the build stays green
	// while Studio Pro cannot open the result.
	if err := checkVersionGates(ctx, s); err != nil {
		return err
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

	var existingContainerK model.ID
	if existing != nil {
		existingContainerK = existing.ContainerID
	}
	containerIDK, err := containerForDocument(ctx, module.ID, s.Folder, existingContainerK)
	if err != nil {
		return err
	}

	k := &agenteditor.KnowledgeBase{
		ContainerID:      containerIDK,
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
	// A rewrite that carried no doc comment keeps the stored one (#1018).
	if existing != nil {
		k.Documentation = carriedDocumentation(s.DocumentationSet, s.Documentation, existing.Documentation)
	}

	if existing != nil {
		// The Portal knowledge base name: MDL has no spelling for it.
		k.InnerName = existing.InnerName
		k.ID = existing.ID
		// Excluded is model state, not script state (#914).
		k.Excluded = existing.Excluded
		if err := ctx.Backend.UpdateAgentEditorKnowledgeBase(k); err != nil {
			return mdlerrors.NewBackend("update knowledge base", err)
		}
		invalidateHierarchy(ctx)
		if _, err := applyDocumentFolder(ctx, k.ID, existingContainerK, containerIDK); err != nil {
			return err
		}
		ctx.ReportMutation("Modified", "knowledge base: %s", s.Name)
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

	// Agent Editor documents need Studio Pro 11.9+ and the AgentEditorCommons
	// module. Nothing downstream catches an older project: the documents are
	// custom blobs, so mxbuild does not validate them and the build stays green
	// while Studio Pro cannot open the result.
	if err := checkVersionGates(ctx, s); err != nil {
		return err
	}

	existingAgent := findAgentEditorAgent(ctx, s.Name.Module, s.Name.Name)
	if existingAgent != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExists("agent", s.Name.String())
	}

	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	var existingContainerA model.ID
	if existingAgent != nil {
		existingContainerA = existingAgent.ContainerID
	}
	containerIDA, err := containerForDocument(ctx, module.ID, s.Folder, existingContainerA)
	if err != nil {
		return err
	}

	a := &agenteditor.Agent{
		ContainerID:   containerIDA,
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

		EnableBackendStreaming: s.EnableBackendStreaming,
		EnableUIStreaming:      s.EnableUIStreaming,
	}
	// A rewrite that carried no doc comment keeps the stored one (#1018).
	if existingAgent != nil {
		a.Documentation = carriedDocumentation(s.DocumentationSet, s.Documentation, existingAgent.Documentation)
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
		at, err := buildAgentToolFromDef(ctx, "create agent", &s.Name, td)
		if err != nil {
			return err
		}
		a.Tools = append(a.Tools, *at)
	}

	// Knowledge base tools
	for _, kbd := range s.KBTools {
		akt, err := buildAgentKBToolFromDef(ctx, "create agent", &s.Name, kbd)
		if err != nil {
			return err
		}
		a.KBTools = append(a.KBTools, *akt)
	}

	var storedModel *agenteditor.SelectedModel
	if existingAgent != nil {
		carryAgentIdentity(a, existingAgent)
		storedModel = existingAgent.SelectedModel
	}
	a.SelectedModel = selectedModelFrom(s.SelectedModel, s.SelectedModelName, storedModel)

	if existingAgent != nil {
		a.ID = existingAgent.ID
		// Excluded is model state, not script state (#914).
		a.Excluded = existingAgent.Excluded
		if err := ctx.Backend.UpdateAgentEditorAgent(a); err != nil {
			return mdlerrors.NewBackend("update agent", err)
		}
		invalidateHierarchy(ctx)
		if _, err := applyDocumentFolder(ctx, a.ID, existingContainerA, containerIDA); err != nil {
			return err
		}
		ctx.ReportMutation("Modified", "agent: %s", s.Name)
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
