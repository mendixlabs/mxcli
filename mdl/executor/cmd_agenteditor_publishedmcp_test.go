// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/agenteditor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#2020: describe -> create or modify of a published MCP service keeps
// each stored tool's uuid, resolves a tool's microflow to its unit ID, and
// gives a new tool a uuid of its own.
func TestCreateOrModifyPublishedMCPService_KeepsToolUUIDs(t *testing.T) {
	mod := mkModule("M")
	mf := mkMicroflow(mod.ID, "mcptool1")
	h := mkHierarchy(mod)
	withContainer(h, mf.ContainerID, mod.ID)
	stored := &agenteditor.PublishedMCPService{
		BaseElement: model.BaseElement{ID: "svc-1"}, ContainerID: mod.ID, Name: "Svc",
		ServerName: "SvcBeforeRename", EndpointPath: "svc", Version: "1.0.0", ProtocolVersion: "v2025_03_26",
		Tools: []agenteditor.PublishedMCPTool{{UUID: "06e34e7d-8103-43f3-aa12-4243d3858956", Name: "tool1",
			SourceMicroflow: &agenteditor.DocRef{QualifiedName: "M.mcptool1", DocumentID: string(mf.ID)}}},
	}
	var written *agenteditor.PublishedMCPService
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
		ListAgentEditorPublishedMCPServicesFunc: func() ([]*agenteditor.PublishedMCPService, error) {
			return []*agenteditor.PublishedMCPService{stored}, nil
		},
		UpdateAgentEditorPublishedMCPServiceFunc: func(p *agenteditor.PublishedMCPService) error {
			written = p
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	prog := parseMDL(t, `create or modify published mcp service M.Svc (
  EndpointPath: 'svc', Version: '1.0.0', ProtocolVersion: 'v2025_03_26'
) {
  tool tool1 ( Microflow: M.mcptool1, Description: 'd' )
  tool tool2 ( Microflow: M.mcptool1, Description: 'new' )
};`)
	assertNoError(t, execCreatePublishedMCPService(ctx, prog.Statements[0].(*ast.CreatePublishedMCPServiceStmt)))
	if written == nil || len(written.Tools) != 2 {
		t.Fatalf("written %+v", written)
	}
	// ServerName omitted: the stored one is kept (a Studio Pro duplicate keeps
	// the original's).
	if written.ServerName != "SvcBeforeRename" {
		t.Errorf("serverName %q, want the stored %q", written.ServerName, "SvcBeforeRename")
	}
	if got := written.Tools[0].UUID; got != stored.Tools[0].UUID {
		t.Errorf("tool1 uuid %q, want the stored %q", got, stored.Tools[0].UUID)
	}
	if ref := written.Tools[0].SourceMicroflow; ref == nil || ref.DocumentID != string(mf.ID) || ref.QualifiedName != "M.mcptool1" {
		t.Errorf("tool1 microflow %+v, want %s", ref, mf.ID)
	}
	// Control: a tool the stored service lacks gets a fresh uuid.
	if got := written.Tools[1].UUID; got == "" || got == stored.Tools[0].UUID {
		t.Errorf("tool2 uuid %q, want a fresh one", got)
	}
}

// The runtime (MCPServer.CreateMCPServer 5.4) refuses these services; check
// says so before the app starts without them.
func TestValidatePublishedMCPService(t *testing.T) {
	cases := []struct {
		name, props, tools string
		want               string // "" = no error
		severity           linter.Severity
	}{
		{"valid", `EndpointPath: 'orders', Version: '1', ProtocolVersion: v2025_03_26`, `tool t ( Microflow: M.T )`, "", 0},
		{"reserved path", `EndpointPath: 'mcp', Version: '1', ProtocolVersion: v2025_03_26`, "", "is reserved", linter.SeverityError},
		{"reserved suffix", `EndpointPath: 'x/MCP', Version: '1', ProtocolVersion: v2025_03_26`, "", "is reserved", linter.SeverityError},
		{"sub-path", `EndpointPath: 'api/orders', Version: '1', ProtocolVersion: v2025_03_26`, "", "has a '/'", linter.SeverityError},
		{"no version", `EndpointPath: 'o', ProtocolVersion: v2025_03_26`, "", "Version is required", linter.SeverityError},
		{"scopes without url", `EndpointPath: 'o', Version: '1', ProtocolVersion: v2025_03_26, AuthenticationEnabled: true, AuthenticationMicroflow: M.Auth, Scopes: 'a'`, "", "Scopes can only be set", linter.SeverityError},
		{"url without microflow", `EndpointPath: 'o', Version: '1', ProtocolVersion: v2025_03_26, AuthenticationEnabled: true, AuthorizationServerUrl: 'https://x.example'`, "", "needs an AuthenticationMicroflow", linter.SeverityError},
		{"url not http", `EndpointPath: 'o', Version: '1', ProtocolVersion: v2025_03_26, AuthenticationEnabled: true, AuthenticationMicroflow: M.Auth, AuthorizationServerUrl: 'login.example'`, "", "absolute http(s) URL", linter.SeverityError},
		{"auth keys ignored", `EndpointPath: 'o', Version: '1', ProtocolVersion: v2025_03_26, AuthenticationMicroflow: M.Auth`, "", "ignored while AuthenticationEnabled is false", linter.SeverityWarning},
		{"enabled, no microflow", `EndpointPath: 'o', Version: '1', ProtocolVersion: v2025_03_26, AuthenticationEnabled: true`, "", "every request is accepted", linter.SeverityWarning},
		{"tool without microflow", `EndpointPath: 'o', Version: '1', ProtocolVersion: v2025_03_26`, `tool t ( Description: 'd' )`, "has no Microflow", linter.SeverityError},
		{"duplicate tool", `EndpointPath: 'o', Version: '1', ProtocolVersion: v2025_03_26`, `tool t ( Microflow: M.T ) tool t ( Microflow: M.U )`, "declared twice", linter.SeverityError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := "create published mcp service M.Svc (" + c.props + ")"
			if c.tools != "" {
				src += " { " + c.tools + " }"
			}
			prog := parseMDL(t, src+";")
			vs := ValidatePublishedMCPService(prog.Statements[0].(*ast.CreatePublishedMCPServiceStmt))
			if c.want == "" {
				if len(vs) != 0 {
					t.Errorf("want no finding, got %v", vs)
				}
				return
			}
			for _, v := range vs {
				if strings.Contains(v.Message, c.want) {
					if v.Severity != c.severity || v.RuleID != publishedMCPServiceRule {
						t.Errorf("%s: severity %v rule %s, want %v %s", c.want, v.Severity, v.RuleID, c.severity, publishedMCPServiceRule)
					}
					return
				}
			}
			t.Errorf("want a finding containing %q, got %v", c.want, vs)
		})
	}
}

// ServerName is stored but not in Studio Pro's editor. A new service is named
// after its document; describe prints ServerName only where they differ, as
// they do on a Studio Pro duplicate (Published_MCP_service_2 keeps
// "Published_MCP_service").
func TestPublishedMCPService_ServerName(t *testing.T) {
	mod := mkModule("M")
	h := mkHierarchy(mod)
	var created *agenteditor.PublishedMCPService
	dup := &agenteditor.PublishedMCPService{
		BaseElement: model.BaseElement{ID: "svc-2"}, ContainerID: mod.ID, Name: "Svc_2",
		ServerName: "Svc", EndpointPath: "svc2", Version: "1.0.0", ProtocolVersion: "v2025_03_26",
	}
	same := &agenteditor.PublishedMCPService{
		BaseElement: model.BaseElement{ID: "svc-3"}, ContainerID: mod.ID, Name: "Other",
		ServerName: "Other", EndpointPath: "other", Version: "1.0.0", ProtocolVersion: "v2025_03_26",
	}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListAgentEditorPublishedMCPServicesFunc: func() ([]*agenteditor.PublishedMCPService, error) {
			return []*agenteditor.PublishedMCPService{dup, same}, nil
		},
		CreateAgentEditorPublishedMCPServiceFunc: func(p *agenteditor.PublishedMCPService) error {
			created = p
			return nil
		},
	}
	ctx, out := newMockCtx(t, withBackend(mb), withHierarchy(h))
	prog := parseMDL(t, `create published mcp service M.Fresh (EndpointPath: 'fresh', Version: '1', ProtocolVersion: v2025_03_26);`)
	assertNoError(t, execCreatePublishedMCPService(ctx, prog.Statements[0].(*ast.CreatePublishedMCPServiceStmt)))
	if created == nil || created.ServerName != "Fresh" {
		t.Errorf("new service serverName %+v, want the document name", created)
	}

	assertNoError(t, describeAgentEditorPublishedMCPService(ctx, ast.QualifiedName{Module: "M", Name: "Svc_2"}))
	assertNoError(t, describeAgentEditorPublishedMCPService(ctx, ast.QualifiedName{Module: "M", Name: "Other"}))
	got := out.String()
	if !strings.Contains(got, "ServerName: 'Svc'") {
		t.Errorf("the duplicate's own server name is not described:\n%s", got)
	}
	// Control: a server named after its document prints no ServerName.
	if strings.Count(got, "ServerName:") != 1 {
		t.Errorf("want ServerName only on the duplicate:\n%s", got)
	}
}
