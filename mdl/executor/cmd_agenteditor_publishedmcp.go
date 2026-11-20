// SPDX-License-Identifier: Apache-2.0

// Package executor - Commands for agent-editor Published MCP Service documents
// (ako/mxcli#2020): Agent Editor 2.6+ with MCP Server 5.x stores an MCP server
// the app publishes as a CustomBlobDocument with CustomDocumentType
// "agenteditor.publishedMCPService". At startup AgentEditorCommons reads each
// one and calls MCPServer.CreateMCPServer and MCPServer.AddTool with its
// values, so the rules below are the ones those actions enforce.
package executor

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/agenteditor"
)

// publishedMCPServiceRule is the check rule for a published MCP service the
// runtime would refuse to start or would start differently than written.
const publishedMCPServiceRule = "MDL-MCPSERVER01"

// listAgentEditorPublishedMCPServices handles LIST PUBLISHED MCP SERVICES [IN module].
func listAgentEditorPublishedMCPServices(ctx *ExecContext, moduleName string) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	svcs, err := ctx.Backend.ListAgentEditorPublishedMCPServices()
	if err != nil {
		return mdlerrors.NewBackend("list published mcp services", err)
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}
	result := &TableResult{
		Columns: []string{"Qualified Name", "Module", "Name", "Endpoint", "Protocol", "Version", "Authentication", "Tools"},
	}
	for _, p := range svcs {
		modName := h.GetModuleName(h.FindModuleID(p.ContainerID))
		if moduleName != "" && modName != moduleName {
			continue
		}
		result.Rows = append(result.Rows, []any{
			fmt.Sprintf("%s.%s", modName, p.Name), modName, p.Name,
			p.EndpointPath, p.ProtocolVersion, p.Version, p.AuthenticationEnabled, len(p.Tools),
		})
	}
	result.Summary = fmt.Sprintf("(%d published mcp service(s))", len(result.Rows))
	return writeResult(ctx, result)
}

// describeAgentEditorPublishedMCPService handles DESCRIBE PUBLISHED MCP SERVICE Module.Name.
func describeAgentEditorPublishedMCPService(ctx *ExecContext, name ast.QualifiedName) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	p := findAgentEditorPublishedMCPService(ctx, name.Module, name.Name)
	if p == nil {
		return mdlerrors.NewNotFound("published mcp service", name.String())
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}
	qualifiedName := fmt.Sprintf("%s.%s", h.GetModuleName(h.FindModuleID(p.ContainerID)), p.Name)

	if p.Documentation != "" {
		outputJavadocIndented(ctx.Output, p.Documentation, "")
	}
	fmt.Fprintf(ctx.Output, "create or modify published mcp service %s%s (\n", qualifiedName, describeFolderClause(ctx, p.ContainerID))

	var lines []string
	add := func(key, val string) {
		if val != "" {
			lines = append(lines, fmt.Sprintf("  %s: %s", key, val))
		}
	}
	if p.ServerName != p.Name {
		add("ServerName", quotedOrEmpty(ctx, p.ServerName))
	}
	add("EndpointPath", quotedOrEmpty(ctx, p.EndpointPath))
	add("Version", quotedOrEmpty(ctx, p.Version))
	if bareMCPToolName.MatchString(p.ProtocolVersion) {
		add("ProtocolVersion", p.ProtocolVersion) // an enumeration value: v2025_03_26
	} else {
		add("ProtocolVersion", quotedOrEmpty(ctx, p.ProtocolVersion))
	}
	if p.AuthenticationEnabled {
		add("AuthenticationEnabled", "true")
	}
	if p.AuthenticationMicroflow != nil {
		add("AuthenticationMicroflow", p.AuthenticationMicroflow.QualifiedName)
	}
	add("AuthorizationServerUrl", quotedOrEmpty(ctx, p.AuthorizationServerURL))
	add("Scopes", quotedOrEmpty(ctx, p.Scopes))
	add("Documentation", quotedOrEmpty(ctx, p.InnerDocumentation))
	fmt.Fprintln(ctx.Output, strings.Join(lines, ",\n"))

	if len(p.Tools) == 0 {
		fmt.Fprintln(ctx.Output, ");")
		return nil
	}
	fmt.Fprintln(ctx.Output, ")")
	fmt.Fprintln(ctx.Output, "{")
	for i, t := range p.Tools {
		if i > 0 {
			fmt.Fprintln(ctx.Output)
		}
		fmt.Fprintf(ctx.Output, "  tool %s (\n", mcpToolName(t.Name))
		var tl []string
		if t.SourceMicroflow != nil && t.SourceMicroflow.QualifiedName != "" {
			tl = append(tl, "    Microflow: "+t.SourceMicroflow.QualifiedName)
		}
		if t.Description != "" {
			tl = append(tl, "    Description: "+formatAgentString(ctx, t.Description))
		}
		if t.Title != "" {
			tl = append(tl, "    Title: "+mdlQuote(ctx, t.Title))
		}
		if t.Schema != "" {
			tl = append(tl, "    Schema: "+formatAgentString(ctx, t.Schema))
		}
		fmt.Fprintln(ctx.Output, strings.Join(tl, ",\n"))
		fmt.Fprintln(ctx.Output, "  )")
	}
	fmt.Fprintln(ctx.Output, "};")
	return nil
}

func quotedOrEmpty(ctx *ExecContext, s string) string {
	if s == "" {
		return ""
	}
	return formatAgentString(ctx, s)
}

var bareMCPToolName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// mcpToolName spells a tool name as MDL reads it back: bare when it is an
// identifier, quoted otherwise (MCP tool names may hold '-' or '.').
func mcpToolName(name string) string {
	if bareMCPToolName.MatchString(name) {
		return name
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// findAgentEditorPublishedMCPService looks up a published MCP service by module and name.
func findAgentEditorPublishedMCPService(ctx *ExecContext, moduleName, svcName string) *agenteditor.PublishedMCPService {
	svcs, err := ctx.Backend.ListAgentEditorPublishedMCPServices()
	if err != nil {
		return nil
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil
	}
	for _, p := range svcs {
		if p.Name == svcName && h.GetModuleName(h.FindModuleID(p.ContainerID)) == moduleName {
			return p
		}
	}
	return nil
}

func execCreatePublishedMCPService(ctx *ExecContext, s *ast.CreatePublishedMCPServiceStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	// The document needs Agent Editor 2.6+; nothing downstream validates a
	// custom blob, so an older project only fails when Studio Pro opens it.
	if err := checkVersionGates(ctx, s); err != nil {
		return err
	}

	existing := findAgentEditorPublishedMCPService(ctx, s.Name.Module, s.Name.Name)
	if existing != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExists("published mcp service", s.Name.String())
	}
	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}
	var existingContainer model.ID
	if existing != nil {
		existingContainer = existing.ContainerID
	}
	containerID, err := containerForDocument(ctx, module.ID, s.Folder, existingContainer)
	if err != nil {
		return err
	}

	p := &agenteditor.PublishedMCPService{
		ContainerID:            containerID,
		Name:                   s.Name.Name,
		Documentation:          s.OuterDocumentation,
		ServerName:             s.ServerName,
		EndpointPath:           s.EndpointPath,
		Version:                s.Version,
		ProtocolVersion:        s.ProtocolVersion,
		AuthenticationEnabled:  s.AuthenticationEnabled,
		AuthorizationServerURL: s.AuthorizationServerURL,
		Scopes:                 s.Scopes,
		InnerDocumentation:     s.InnerDocumentation,
	}
	// Studio Pro's editor does not show the server name: a new document gets
	// its own name, a duplicate keeps the original's. Omitted, the stored one
	// is kept.
	if p.ServerName == "" && existing != nil {
		p.ServerName = existing.ServerName
	}
	if p.ServerName == "" {
		p.ServerName = s.Name.Name
	}
	if s.AuthenticationMicroflow != nil {
		ref, err := publishedMCPMicroflowRef(ctx, *s.AuthenticationMicroflow)
		if err != nil {
			return fmt.Errorf("create published mcp service %s: authentication microflow: %w", s.Name, err)
		}
		p.AuthenticationMicroflow = ref
	}
	for _, td := range s.Tools {
		tool, err := buildPublishedMCPTool(ctx, "create published mcp service", s.Name, td)
		if err != nil {
			return err
		}
		p.Tools = append(p.Tools, *tool)
	}
	if existing != nil {
		p.Documentation = carriedDocumentation(s.DocumentationSet, s.OuterDocumentation, existing.Documentation)
		carryPublishedMCPToolIDs(p, existing)
	}
	if err := publishedMCPServiceError(s.Name.String(), p); err != nil {
		return err
	}

	if existing != nil {
		p.ID = existing.ID
		p.Excluded = existing.Excluded // model state, not script state (#914)
		if err := ctx.Backend.UpdateAgentEditorPublishedMCPService(p); err != nil {
			return mdlerrors.NewBackend("update published mcp service", err)
		}
		invalidateHierarchy(ctx)
		if _, err := applyDocumentFolder(ctx, p.ID, existingContainer, containerID); err != nil {
			return err
		}
		ctx.ReportMutation("Modified", "published mcp service: %s", s.Name)
		return nil
	}
	if err := ctx.Backend.CreateAgentEditorPublishedMCPService(p); err != nil {
		return mdlerrors.NewBackend("create published mcp service", err)
	}
	invalidateHierarchy(ctx)
	fmt.Fprintf(ctx.Output, "Created published mcp service: %s\n", s.Name)
	return nil
}

// carryPublishedMCPToolIDs gives each tool the uuid of the stored tool with the
// same name: Agent Editor keys its tools by uuid, and the rewrite pairs on it
// to keep what a tool holds beyond mxcli's fields.
func carryPublishedMCPToolIDs(p, stored *agenteditor.PublishedMCPService) {
	byName := map[string]string{}
	for _, t := range stored.Tools {
		byName[t.Name] = t.UUID
	}
	for i := range p.Tools {
		if id, ok := byName[p.Tools[i].Name]; ok && id != "" {
			p.Tools[i].UUID = id
		}
	}
}

func buildPublishedMCPTool(ctx *ExecContext, verb string, svc ast.QualifiedName, td ast.PublishedMCPToolDef) (*agenteditor.PublishedMCPTool, error) {
	tool := &agenteditor.PublishedMCPTool{
		UUID:        types.GenerateID(),
		Name:        td.Name,
		Description: td.Description,
		Title:       td.Title,
		Schema:      td.Schema,
	}
	if td.Microflow == nil {
		return nil, fmt.Errorf("%s %s: tool %s needs a Microflow", verb, svc, td.Name)
	}
	ref, err := publishedMCPMicroflowRef(ctx, *td.Microflow)
	if err != nil {
		return nil, fmt.Errorf("%s %s: tool %s: %w", verb, svc, td.Name, err)
	}
	tool.SourceMicroflow = ref
	return tool, nil
}

// publishedMCPMicroflowRef resolves a microflow to the reference Agent Editor
// stores: its unit ID as documentId, its qualified name.
func publishedMCPMicroflowRef(ctx *ExecContext, qn ast.QualifiedName) (*agenteditor.DocRef, error) {
	id, err := resolveMicroflowByName(ctx, qn.String())
	if err != nil {
		return nil, err
	}
	return &agenteditor.DocRef{QualifiedName: qn.String(), DocumentID: string(id)}, nil
}

func execDropPublishedMCPService(ctx *ExecContext, s *ast.DropPublishedMCPServiceStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	p := findAgentEditorPublishedMCPService(ctx, s.Name.Module, s.Name.Name)
	if p == nil {
		return mdlerrors.NewNotFound("published mcp service", s.Name.String())
	}
	if err := ctx.Backend.DeleteAgentEditorPublishedMCPService(string(p.ID)); err != nil {
		return mdlerrors.NewBackend("delete published mcp service", err)
	}
	fmt.Fprintf(ctx.Output, "Dropped published mcp service: %s\n", s.Name)
	return nil
}

func execAlterPublishedMCPService(ctx *ExecContext, s *ast.AlterPublishedMCPServiceStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	p := findAgentEditorPublishedMCPService(ctx, s.Name.Module, s.Name.Name)
	if p == nil {
		return mdlerrors.NewNotFound("published mcp service", s.Name.String())
	}
	for key, val := range s.Sets {
		if err := applyPublishedMCPServiceChange(ctx, p, key, val); err != nil {
			return err
		}
	}
	for _, name := range s.DropTools {
		kept := p.Tools[:0]
		found := false
		for _, t := range p.Tools {
			if t.Name == name {
				found = true
				continue
			}
			kept = append(kept, t)
		}
		if !found {
			return mdlerrors.NewNotFound("tool", fmt.Sprintf("%s on %s", name, s.Name))
		}
		p.Tools = kept
	}
	for _, td := range s.AddTools {
		tool, err := buildPublishedMCPTool(ctx, "alter published mcp service", s.Name, td)
		if err != nil {
			return err
		}
		p.Tools = append(p.Tools, *tool)
	}
	if err := publishedMCPServiceError(s.Name.String(), p); err != nil {
		return err
	}
	if err := ctx.Backend.UpdateAgentEditorPublishedMCPService(p); err != nil {
		return mdlerrors.NewBackend("alter published mcp service", err)
	}
	invalidateHierarchy(ctx)
	fmt.Fprintf(ctx.Output, "Altered published mcp service: %s\n", s.Name)
	return nil
}

func applyPublishedMCPServiceChange(ctx *ExecContext, p *agenteditor.PublishedMCPService, key, val string) error {
	switch strings.ToLower(key) {
	case "servername":
		p.ServerName = val
	case "endpointpath":
		p.EndpointPath = val
	case "version":
		p.Version = val
	case "protocolversion":
		p.ProtocolVersion = val
	case "authenticationenabled":
		p.AuthenticationEnabled = strings.EqualFold(val, "true")
	case "authenticationmicroflow":
		if val == "" {
			p.AuthenticationMicroflow = nil
			return nil
		}
		ref, err := publishedMCPMicroflowRef(ctx, parseQualifiedNameStr(val))
		if err != nil {
			return fmt.Errorf("alter published mcp service %s: authentication microflow: %w", p.Name, err)
		}
		p.AuthenticationMicroflow = ref
	case "authorizationserverurl":
		p.AuthorizationServerURL = val
	case "scopes":
		p.Scopes = val
	case "documentation":
		p.InnerDocumentation = val
	default:
		return mdlerrors.NewUnsupported(fmt.Sprintf("unknown published mcp service property: %s", key))
	}
	return nil
}

// publishedMCPServiceIssue is one finding of checkPublishedMCPService.
type publishedMCPServiceIssue struct {
	err     bool
	message string
}

// checkPublishedMCPService applies the rules MCPServer.CreateMCPServer (5.4)
// and AgentDocumentUtils.initPublishedMCPService apply at startup. An error is
// a service the runtime refuses to start (it logs and skips it, so the app
// still boots); a warning is a value the runtime ignores.
func checkPublishedMCPService(p *agenteditor.PublishedMCPService) []publishedMCPServiceIssue {
	var out []publishedMCPServiceIssue
	fail := func(f string, a ...any) { out = append(out, publishedMCPServiceIssue{true, fmt.Sprintf(f, a...)}) }
	warn := func(f string, a ...any) { out = append(out, publishedMCPServiceIssue{false, fmt.Sprintf(f, a...)}) }

	for _, req := range []struct{ key, val string }{
		{"ServerName", p.ServerName}, {"EndpointPath", p.EndpointPath}, {"Version", p.Version}, {"ProtocolVersion", p.ProtocolVersion},
	} {
		if strings.TrimSpace(req.val) == "" {
			fail("%s is required: the runtime does not start a service without it", req.key)
		}
	}
	path := p.EndpointPath
	if strings.EqualFold(path, "mcp") || strings.HasSuffix(strings.ToLower(path), "/mcp") {
		fail("EndpointPath %q is reserved: it cannot be 'mcp' or end in '/mcp'", path)
	} else if strings.Contains(path, "/") {
		fail("EndpointPath %q has a '/': the runtime matches the first path segment only, so the service is unreachable", path)
	}
	authURL := strings.TrimSpace(p.AuthorizationServerURL)
	scopes := strings.TrimSpace(p.Scopes)
	if !p.AuthenticationEnabled {
		if p.AuthenticationMicroflow != nil {
			warn("AuthenticationMicroflow is ignored while AuthenticationEnabled is false")
		}
		if authURL != "" || scopes != "" {
			warn("AuthorizationServerUrl and Scopes are ignored while AuthenticationEnabled is false")
		}
	} else {
		if p.AuthenticationMicroflow == nil {
			if authURL != "" {
				fail("AuthorizationServerUrl needs an AuthenticationMicroflow, which validates the access token")
			} else {
				warn("AuthenticationEnabled without an AuthenticationMicroflow authenticates nothing: every request is accepted")
			}
		}
		if authURL != "" && !isHTTPURL(authURL) {
			fail("AuthorizationServerUrl %q must be an absolute http(s) URL", authURL)
		}
		if scopes != "" && authURL == "" {
			fail("Scopes can only be set together with an AuthorizationServerUrl")
		}
	}
	seen := map[string]bool{}
	for _, t := range p.Tools {
		if t.Name == "" {
			fail("a tool has no name")
			continue
		}
		if seen[t.Name] {
			fail("tool %s is declared twice", t.Name)
		}
		seen[t.Name] = true
		if t.SourceMicroflow == nil || t.SourceMicroflow.QualifiedName == "" {
			fail("tool %s has no Microflow", t.Name)
		}
	}
	return out
}

func isHTTPURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Host != "" && (strings.EqualFold(u.Scheme, "https") || strings.EqualFold(u.Scheme, "http"))
}

// publishedMCPServiceError is the exec-time form of the check: the first error.
func publishedMCPServiceError(name string, p *agenteditor.PublishedMCPService) error {
	for _, is := range checkPublishedMCPService(p) {
		if is.err {
			return mdlerrors.NewValidationf("published mcp service %s: %s", name, is.message)
		}
	}
	return nil
}

// ValidatePublishedMCPService is the check-time form, run without a project.
// Microflows resolve under --references; here only their presence counts.
func ValidatePublishedMCPService(s *ast.CreatePublishedMCPServiceStmt) []linter.Violation {
	p := &agenteditor.PublishedMCPService{
		ServerName: s.ServerName, EndpointPath: s.EndpointPath, Version: s.Version, ProtocolVersion: s.ProtocolVersion,
		AuthenticationEnabled: s.AuthenticationEnabled, AuthorizationServerURL: s.AuthorizationServerURL, Scopes: s.Scopes,
	}
	if p.ServerName == "" {
		p.ServerName = s.Name.Name
	}
	if s.AuthenticationMicroflow != nil {
		p.AuthenticationMicroflow = &agenteditor.DocRef{QualifiedName: s.AuthenticationMicroflow.String()}
	}
	for _, td := range s.Tools {
		t := agenteditor.PublishedMCPTool{Name: td.Name}
		if td.Microflow != nil {
			t.SourceMicroflow = &agenteditor.DocRef{QualifiedName: td.Microflow.String()}
		}
		p.Tools = append(p.Tools, t)
	}
	var out []linter.Violation
	for _, is := range checkPublishedMCPService(p) {
		sev := linter.SeverityWarning
		if is.err {
			sev = linter.SeverityError
		}
		out = append(out, linter.Violation{
			RuleID:   publishedMCPServiceRule,
			Severity: sev,
			Message:  fmt.Sprintf("published mcp service %s: %s", s.Name, is.message),
			Location: linter.Location{DocumentType: "published mcp service", DocumentName: s.Name.String()},
		})
	}
	return out
}
