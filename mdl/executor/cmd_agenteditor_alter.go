// SPDX-License-Identifier: Apache-2.0

// Package executor — ALTER handlers for agent-editor documents (Model,
// Knowledge Base, Consumed MCP Service, Agent). See ast/ast_agenteditor.go
// for the Alter*Stmt definitions.
package executor

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/agenteditor"
)

// ---------------------------------------------------------------------------
// ALTER MODEL
// ---------------------------------------------------------------------------

func execAlterModel(ctx *ExecContext, s *ast.AlterModelStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	m := findAgentEditorModel(ctx, s.Name.Module, s.Name.Name)
	if m == nil {
		return mdlerrors.NewNotFound("model", s.Name.String())
	}
	for key, val := range s.Changes {
		if err := applyModelChange(ctx, m, key, val); err != nil {
			return err
		}
	}
	if err := ctx.Backend.UpdateAgentEditorModel(m); err != nil {
		return mdlerrors.NewBackend("alter model", err)
	}
	invalidateHierarchy(ctx)
	fmt.Fprintf(ctx.Output, "Altered model: %s\n", s.Name)
	return nil
}

func applyModelChange(ctx *ExecContext, m *agenteditor.Model, key, val string) error {
	switch strings.ToLower(key) {
	case "provider":
		m.Provider = val
	case "documentation":
		m.Documentation = val
	case "displayname":
		m.DisplayName = val
	case "keyname":
		m.KeyName = val
	case "keyid":
		m.KeyID = val
	case "environment":
		m.Environment = val
	case "resourcename":
		m.ResourceName = val
	case "deeplinkurl":
		m.DeepLinkURL = val
	case "key":
		qn, err := parseAndResolveConstantRef(ctx, val)
		if err != nil {
			return fmt.Errorf("alter model %s: %w", m.Name, err)
		}
		m.Key = qn
	default:
		return mdlerrors.NewUnsupported(fmt.Sprintf("unknown model property: %s", key))
	}
	return nil
}

// ---------------------------------------------------------------------------
// ALTER KNOWLEDGE BASE
// ---------------------------------------------------------------------------

func execAlterKnowledgeBase(ctx *ExecContext, s *ast.AlterKnowledgeBaseStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	k := findAgentEditorKnowledgeBase(ctx, s.Name.Module, s.Name.Name)
	if k == nil {
		return mdlerrors.NewNotFound("knowledge base", s.Name.String())
	}
	for key, val := range s.Changes {
		if err := applyKnowledgeBaseChange(ctx, k, key, val); err != nil {
			return err
		}
	}
	if err := ctx.Backend.UpdateAgentEditorKnowledgeBase(k); err != nil {
		return mdlerrors.NewBackend("alter knowledge base", err)
	}
	invalidateHierarchy(ctx)
	fmt.Fprintf(ctx.Output, "Altered knowledge base: %s\n", s.Name)
	return nil
}

func applyKnowledgeBaseChange(ctx *ExecContext, k *agenteditor.KnowledgeBase, key, val string) error {
	switch strings.ToLower(key) {
	case "provider":
		k.Provider = val
	case "documentation":
		k.Documentation = val
	case "modeldisplayname":
		k.ModelDisplayName = val
	case "modelname":
		k.ModelName = val
	case "keyname":
		k.KeyName = val
	case "keyid":
		k.KeyID = val
	case "environment":
		k.Environment = val
	case "deeplinkurl":
		k.DeepLinkURL = val
	case "key":
		qn, err := parseAndResolveConstantRef(ctx, val)
		if err != nil {
			return fmt.Errorf("alter knowledge base %s: %w", k.Name, err)
		}
		k.Key = qn
	default:
		return mdlerrors.NewUnsupported(fmt.Sprintf("unknown knowledge base property: %s", key))
	}
	return nil
}

// ---------------------------------------------------------------------------
// ALTER CONSUMED MCP SERVICE
// ---------------------------------------------------------------------------

func execAlterConsumedMCPService(ctx *ExecContext, s *ast.AlterConsumedMCPServiceStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	c := findAgentEditorConsumedMCPService(ctx, s.Name.Module, s.Name.Name)
	if c == nil {
		return mdlerrors.NewNotFound("consumed mcp service", s.Name.String())
	}
	for key, val := range s.Changes {
		if err := applyConsumedMCPServiceChange(c, key, val); err != nil {
			return err
		}
	}
	if err := ctx.Backend.UpdateAgentEditorConsumedMCPService(c); err != nil {
		return mdlerrors.NewBackend("alter consumed mcp service", err)
	}
	invalidateHierarchy(ctx)
	fmt.Fprintf(ctx.Output, "Altered consumed mcp service: %s\n", s.Name)
	return nil
}

func applyConsumedMCPServiceChange(c *agenteditor.ConsumedMCPService, key, val string) error {
	switch strings.ToLower(key) {
	case "protocolversion":
		c.ProtocolVersion = val
	case "version":
		c.Version = val
	case "documentation":
		// Outer documentation; the CREATE statement maps Documentation to
		// the BSON-wrapper Documentation field, matching here for symmetry.
		c.Documentation = val
	case "innerdocumentation":
		c.InnerDocumentation = val
	case "connectiontimeoutseconds":
		n, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("alter consumed mcp service %s: ConnectionTimeoutSeconds must be an integer (got %q)", c.Name, val)
		}
		c.ConnectionTimeoutSeconds = n
	default:
		return mdlerrors.NewUnsupported(fmt.Sprintf("unknown consumed mcp service property: %s", key))
	}
	return nil
}

// ---------------------------------------------------------------------------
// ALTER AGENT
// ---------------------------------------------------------------------------

func execAlterAgent(ctx *ExecContext, s *ast.AlterAgentStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	a := findAgentEditorAgent(ctx, s.Name.Module, s.Name.Name)
	if a == nil {
		return mdlerrors.NewNotFound("agent", s.Name.String())
	}

	// In key order, so SelectedModel is applied before SelectedModelName.
	keys := make([]string, 0, len(s.Sets))
	for key := range s.Sets {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return strings.ToLower(keys[i]) < strings.ToLower(keys[j]) })
	for _, key := range keys {
		if err := applyAgentChange(ctx, a, key, s.Sets[key]); err != nil {
			return err
		}
	}

	for _, td := range s.AddTools {
		tool, err := buildAgentToolFromDef(ctx, "alter agent", &s.Name, td)
		if err != nil {
			return err
		}
		a.Tools = append(a.Tools, *tool)
	}

	for _, kbd := range s.AddKBs {
		kbTool, err := buildAgentKBToolFromDef(ctx, "alter agent", &s.Name, kbd)
		if err != nil {
			return err
		}
		a.KBTools = append(a.KBTools, *kbTool)
	}

	for _, name := range s.DropTools {
		a.Tools = removeAgentTool(a.Tools, func(t agenteditor.AgentTool) bool {
			return t.ToolType == "Microflow" && t.Name == name
		})
	}
	for _, qn := range s.DropMCPServices {
		qns := qn.String()
		a.Tools = removeAgentTool(a.Tools, func(t agenteditor.AgentTool) bool {
			return t.ToolType == "MCP" && t.Document != nil && t.Document.QualifiedName == qns
		})
	}
	for _, name := range s.DropKBs {
		a.KBTools = removeAgentKBTool(a.KBTools, func(t agenteditor.AgentKBTool) bool {
			return t.Name == name
		})
	}

	if err := ctx.Backend.UpdateAgentEditorAgent(a); err != nil {
		return mdlerrors.NewBackend("alter agent", err)
	}
	invalidateHierarchy(ctx)
	fmt.Fprintf(ctx.Output, "Altered agent: %s\n", s.Name)
	return nil
}

func applyAgentChange(ctx *ExecContext, a *agenteditor.Agent, key, val string) error {
	switch strings.ToLower(key) {
	case "usagetype":
		a.UsageType = val
	case "description":
		a.Description = val
	case "documentation":
		a.Documentation = val
	case "systemprompt":
		a.SystemPrompt = val
	case "userprompt":
		a.UserPrompt = val
	case "toolchoice":
		a.ToolChoice = val
	case "maxtokens":
		n, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("alter agent %s: MaxTokens must be an integer (got %q)", a.Name, val)
		}
		a.MaxTokens = &n
	case "temperature":
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return fmt.Errorf("alter agent %s: Temperature must be numeric (got %q)", a.Name, val)
		}
		a.Temperature = &f
	case "topp":
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return fmt.Errorf("alter agent %s: TopP must be numeric (got %q)", a.Name, val)
		}
		a.TopP = &f
	case "model":
		qn := parseQualifiedNameStr(val)
		m := findAgentEditorModel(ctx, qn.Module, qn.Name)
		if m == nil {
			return fmt.Errorf("alter agent %s: model not found: %s", a.Name, val)
		}
		a.Model = &agenteditor.DocRef{DocumentID: string(m.ID), QualifiedName: qn.String()}
	case "enablebackendstreaming", "enableuistreaming":
		b, err := strconv.ParseBool(val)
		if err != nil {
			return fmt.Errorf("alter agent %s: %s takes true or false (got %q)", a.Name, key, val)
		}
		if strings.EqualFold(key, "enablebackendstreaming") {
			a.EnableBackendStreaming = &b
		} else {
			a.EnableUIStreaming = &b
		}
	case "selectedmodel":
		a.SelectedModel = selectedModelFrom(&val, nil, a.SelectedModel)
	case "selectedmodelname":
		if a.SelectedModel == nil {
			return fmt.Errorf("alter agent %s: SelectedModelName needs a SelectedModel; set both", a.Name)
		}
		a.SelectedModel.Name = val
	case "entity":
		qn := parseQualifiedNameStr(val)
		a.Entity = carryEntityRef(&agenteditor.DocRef{QualifiedName: qn.String()}, a.Entity)
	default:
		return mdlerrors.NewUnsupported(fmt.Sprintf("unknown agent property: %s", key))
	}
	return nil
}

// buildAgentToolFromDef turns a tool or mcp service block into a tool entry,
// on create agent and on alter agent … add. verb names the statement in errors.
// A new entry gets a fresh id: Agent Editor keys its tools by it.
func buildAgentToolFromDef(ctx *ExecContext, verb string, agentName *ast.QualifiedName, td ast.AgentToolDef) (*agenteditor.AgentTool, error) {
	tool := &agenteditor.AgentTool{
		ID:          types.GenerateID(),
		Name:        td.Name,
		Description: td.Description,
		Enabled:     td.Enabled,
		ToolType:    td.ToolType,

		UserAccessApproval: td.UserAccessApproval,
		DisplayTitle:       td.DisplayTitle,
		DisplayDescription: td.DisplayDescription,
	}
	if td.Document == nil {
		return tool, nil
	}
	switch {
	case strings.EqualFold(td.ToolType, "MCP"):
		svc := findAgentEditorConsumedMCPService(ctx, td.Document.Module, td.Document.Name)
		if svc == nil {
			return nil, fmt.Errorf("%s %s: consumed mcp service not found: %s", verb, agentName, td.Document)
		}
		tool.Document = &agenteditor.DocRef{DocumentID: string(svc.ID), QualifiedName: td.Document.String()}
	case strings.EqualFold(td.ToolType, "Microflow"):
		mfID, err := resolveMicroflowByName(ctx, td.Document.String())
		if err != nil {
			return nil, fmt.Errorf("%s %s: tool %s: %w", verb, agentName, td.Name, err)
		}
		tool.Document = &agenteditor.DocRef{DocumentID: string(mfID), QualifiedName: td.Document.String()}
	}
	return tool, nil
}

// buildAgentKBToolFromDef turns a knowledge base block into a knowledge base
// tool entry, with a fresh id.
func buildAgentKBToolFromDef(ctx *ExecContext, verb string, agentName *ast.QualifiedName, kbd ast.AgentKBToolDef) (*agenteditor.AgentKBTool, error) {
	akt := &agenteditor.AgentKBTool{
		ID:                   types.GenerateID(),
		Name:                 kbd.Name,
		Description:          kbd.Description,
		Enabled:              kbd.Enabled,
		CollectionIdentifier: kbd.Collection,
		MaxResults:           kbd.MaxResults,
		MinSimilarity:        kbd.MinSimilarity,
		IsVisible:            kbd.IsVisible,
	}
	if kbd.Source != nil {
		kb := findAgentEditorKnowledgeBase(ctx, kbd.Source.Module, kbd.Source.Name)
		if kb == nil {
			return nil, fmt.Errorf("%s %s: knowledge base not found: %s", verb, agentName, kbd.Source)
		}
		akt.Document = &agenteditor.DocRef{
			DocumentID:    string(kb.ID),
			QualifiedName: kbd.Source.String(),
		}
	}
	return akt, nil
}

func removeAgentTool(tools []agenteditor.AgentTool, match func(agenteditor.AgentTool) bool) []agenteditor.AgentTool {
	out := tools[:0]
	for _, t := range tools {
		if !match(t) {
			out = append(out, t)
		}
	}
	return out
}

func removeAgentKBTool(tools []agenteditor.AgentKBTool, match func(agenteditor.AgentKBTool) bool) []agenteditor.AgentKBTool {
	out := tools[:0]
	for _, t := range tools {
		if !match(t) {
			out = append(out, t)
		}
	}
	return out
}

// parseAndResolveConstantRef parses a "Module.ConstantName" string and
// resolves it to a ConstantRef via the existing resolver.
func parseAndResolveConstantRef(ctx *ExecContext, qname string) (*agenteditor.ConstantRef, error) {
	qn := parseQualifiedNameStr(qname)
	return resolveConstantRef(ctx, qn)
}

// parseQualifiedNameStr parses "Module.Name" into ast.QualifiedName. Mirrors
// visitor.parseQualifiedNameString (kept local to avoid an import cycle).
func parseQualifiedNameStr(s string) ast.QualifiedName {
	parts := strings.SplitN(s, ".", 2)
	if len(parts) == 2 {
		return ast.QualifiedName{Module: parts[0], Name: parts[1]}
	}
	return ast.QualifiedName{Name: s}
}
