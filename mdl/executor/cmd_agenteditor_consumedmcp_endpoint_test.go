// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/agenteditor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// consumedMCPFixture is a module M holding a String constant Url, an Integer
// constant Port, a credentials microflow GetCredentials, and optionally a
// stored consumed MCP service Svc.
type consumedMCPFixture struct {
	mod   *model.Module
	url   *model.Constant
	port  *model.Constant
	creds *microflows.Microflow
	// badReturn and withParam break the credentials contract.
	badReturn, withParam *microflows.Microflow
	stored               *agenteditor.ConsumedMCPService
	written              *agenteditor.ConsumedMCPService
	mb                   *mock.MockBackend
	h                    *ContainerHierarchy
}

func newConsumedMCPFixture(stored bool) *consumedMCPFixture {
	f := &consumedMCPFixture{mod: mkModule("M")}
	f.url = mkConstant(f.mod.ID, "Url", "String", "http://127.0.0.1:8765/mcp")
	f.port = mkConstant(f.mod.ID, "Port", "Integer", "8765")
	f.creds = mkMicroflow(f.mod.ID, "GetCredentials")
	f.creds.ReturnType = &microflows.ListType{EntityQualifiedName: "System.HttpHeader"}
	f.badReturn = mkMicroflow(f.mod.ID, "ReturnsString")
	f.badReturn.ReturnType = &microflows.StringType{}
	f.withParam = mkMicroflow(f.mod.ID, "TakesParam")
	f.withParam.ReturnType = &microflows.ListType{EntityQualifiedName: "System.HttpHeader"}
	f.withParam.Parameters = []*microflows.MicroflowParameter{{Name: "Token", Type: &microflows.StringType{}}}
	f.h = mkHierarchy(f.mod)
	withContainer(f.h, f.mod.ID, f.mod.ID)
	if stored {
		f.stored = &agenteditor.ConsumedMCPService{BaseElement: model.BaseElement{ID: "svc-1"}, ContainerID: f.mod.ID, Name: "Svc",
			ProtocolVersion:         "v2025_03_26",
			Endpoint:                &agenteditor.DocRef{DocumentID: string(f.url.ID), QualifiedName: "M.Url"},
			AuthenticationMicroflow: &agenteditor.DocRef{DocumentID: string(f.creds.ID), QualifiedName: "M.GetCredentials"}}
	}
	record := func(c *agenteditor.ConsumedMCPService) error { f.written = c; return nil }
	f.mb = &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListModulesFunc:   func() ([]*model.Module, error) { return []*model.Module{f.mod}, nil },
		ListConstantsFunc: func() ([]*model.Constant, error) { return []*model.Constant{f.url, f.port}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return []*microflows.Microflow{f.creds, f.badReturn, f.withParam}, nil
		},
		ListAgentEditorConsumedMCPServicesFunc: func() ([]*agenteditor.ConsumedMCPService, error) {
			if f.stored == nil {
				return nil, nil
			}
			return []*agenteditor.ConsumedMCPService{f.stored}, nil
		},
		CreateAgentEditorConsumedMCPServiceFunc: record,
		UpdateAgentEditorConsumedMCPServiceFunc: record,
	}
	return f
}

func (f *consumedMCPFixture) exec(t *testing.T, src string) error {
	t.Helper()
	ctx, _ := newMockCtx(t, withBackend(f.mb), withHierarchy(f.h))
	switch s := parseMDL(t, src).Statements[0].(type) {
	case *ast.CreateConsumedMCPServiceStmt:
		return execCreateConsumedMCPService(ctx, s)
	case *ast.AlterConsumedMCPServiceStmt:
		return execAlterConsumedMCPService(ctx, s)
	default:
		t.Fatalf("unexpected statement %T", s)
		return nil
	}
}

// ako/mxcli#2060: a consumed MCP service had no endpoint and no credentials
// microflow, so AgentEditorCommons skipped it at import ("No value could be
// found for the local configuration for the constant") and the agent's MCP
// tools never appeared.
func TestCreateConsumedMCPService_EndpointAndCredentials(t *testing.T) {
	f := newConsumedMCPFixture(false)
	assertNoError(t, f.exec(t, `create consumed mcp service M.Svc (ProtocolVersion: v2025_03_26, Endpoint: @M.Url, AuthenticationMicroflow: M.GetCredentials)`))
	if f.written == nil {
		t.Fatal("not written")
	}
	if ep := f.written.Endpoint; ep == nil || ep.QualifiedName != "M.Url" || ep.DocumentID != string(f.url.ID) {
		t.Errorf("endpoint %+v, want M.Url with the constant's unit id %s", ep, f.url.ID)
	}
	if am := f.written.AuthenticationMicroflow; am == nil || am.QualifiedName != "M.GetCredentials" || am.DocumentID != string(f.creds.ID) {
		t.Errorf("credentials microflow %+v, want M.GetCredentials with id %s", am, f.creds.ID)
	}
}

func TestCreateConsumedMCPService_EndpointRefusals(t *testing.T) {
	for src, want := range map[string]string{
		`create consumed mcp service M.Svc (ProtocolVersion: v2025_03_26, Endpoint: @M.Port)`:                                 "String constant",
		`create consumed mcp service M.Svc (ProtocolVersion: v2025_03_26, Endpoint: @M.Missing)`:                              "constant not found",
		`create consumed mcp service M.Svc (ProtocolVersion: v2025_03_26, Endpoint: @M.Url, AuthenticationMicroflow: M.Nope)`: "M.Nope",
	} {
		f := newConsumedMCPFixture(false)
		err := f.exec(t, src)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s:\n  error %v, want it to mention %q", src, err, want)
		}
		if f.written != nil {
			t.Errorf("%s: written despite the error", src)
		}
	}
}

func TestCreateOrModifyConsumedMCPService_KeepsEndpoint(t *testing.T) {
	f := newConsumedMCPFixture(true)
	assertNoError(t, f.exec(t, `create or modify consumed mcp service M.Svc (ProtocolVersion: v2025_03_26, Version: '2.0')`))
	if f.written == nil || f.written.Endpoint == nil || f.written.Endpoint.QualifiedName != "M.Url" ||
		f.written.AuthenticationMicroflow == nil || f.written.AuthenticationMicroflow.QualifiedName != "M.GetCredentials" {
		t.Errorf("left out, the stored endpoint and credentials microflow are lost: %+v", f.written)
	}
}

func TestAlterConsumedMCPService_EndpointAndCredentials(t *testing.T) {
	f := newConsumedMCPFixture(true)
	f.stored.Endpoint = nil
	assertNoError(t, f.exec(t, `alter consumed mcp service M.Svc set Endpoint = @M.Url, AuthenticationMicroflow = ''`))
	if f.written == nil || f.written.Endpoint == nil || f.written.Endpoint.DocumentID != string(f.url.ID) {
		t.Errorf("alter set Endpoint: %+v", f.written)
	}
	if f.written.AuthenticationMicroflow != nil {
		t.Errorf("AuthenticationMicroflow = '' left %+v", f.written.AuthenticationMicroflow)
	}
	f2 := newConsumedMCPFixture(true)
	if err := f2.exec(t, `alter consumed mcp service M.Svc set Endpoint = @M.Port`); err == nil || !strings.Contains(err.Error(), "String constant") {
		t.Errorf("alter to a non-String constant: %v", err)
	}
}

// describe prints both, and its output reads back as them.
func TestDescribeConsumedMCPService_EndpointAndCredentials(t *testing.T) {
	f := newConsumedMCPFixture(true)
	ctx, buf := newMockCtx(t, withBackend(f.mb), withHierarchy(f.h))
	assertNoError(t, describeAgentEditorConsumedMCPService(ctx, ast.QualifiedName{Module: "M", Name: "Svc"}))
	out := buf.String()
	assertContainsStr(t, out, "Endpoint: @M.Url")
	assertContainsStr(t, out, "AuthenticationMicroflow: M.GetCredentials")
	s := parseMDL(t, out).Statements[0].(*ast.CreateConsumedMCPServiceStmt)
	if s.Endpoint == nil || s.Endpoint.String() != "M.Url" || s.AuthenticationMicroflow == nil || s.AuthenticationMicroflow.String() != "M.GetCredentials" {
		t.Errorf("describe output does not read back:\n%s", out)
	}
	// Control: a service without them prints neither.
	f.stored.Endpoint, f.stored.AuthenticationMicroflow = nil, nil
	buf.Reset()
	assertNoError(t, describeAgentEditorConsumedMCPService(ctx, ast.QualifiedName{Module: "M", Name: "Svc"}))
	if strings.Contains(buf.String(), "Endpoint") || strings.Contains(buf.String(), "AuthenticationMicroflow") {
		t.Errorf("a service without them prints them:\n%s", buf.String())
	}
}

// check warns on a create without an endpoint: the import skips such a
// service, so it can never connect.
func TestValidateConsumedMCPEndpoint(t *testing.T) {
	prog := parseMDL(t, `
create consumed mcp service M.NoEndpoint (ProtocolVersion: v2025_03_26);
create consumed mcp service M.Good (ProtocolVersion: v2025_03_26, Endpoint: @M.Url);
create or modify consumed mcp service M.Kept (ProtocolVersion: v2025_03_26);
`)
	got := map[string]string{}
	for _, v := range ValidateConsumedMCPEndpoint(prog) {
		if v.RuleID != consumedMCPEndpointRule {
			t.Errorf("rule %s", v.RuleID)
		}
		got[v.Location.DocumentName] = v.Message
	}
	if !strings.Contains(got["M.NoEndpoint"], "Endpoint") {
		t.Errorf("M.NoEndpoint: %q, want an Endpoint warning", got["M.NoEndpoint"])
	}
	// Controls: a stated endpoint, and a create or modify (which keeps the
	// stored one), say nothing.
	for _, name := range []string{"M.Good", "M.Kept"} {
		if got[name] != "" {
			t.Errorf("%s warned: %s", name, got[name])
		}
	}
}

// check --references predicts what exec and the runtime refuse: a missing or
// non-String endpoint constant, and a credentials microflow MCPClient cannot
// call. MCPClient runs it with no arguments and reads its result as a list of
// System.HttpHeader (ConsumedMCPService_CreateMCPClient, MCP Client 4.4).
func TestReferences_ConsumedMCPServiceEndpoint(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`create consumed mcp service M.Svc (ProtocolVersion: v2025_03_26, Endpoint: @M.Url, AuthenticationMicroflow: M.GetCredentials);`, ""},
		{`create consumed mcp service M.Svc (ProtocolVersion: v2025_03_26, Endpoint: @M.Port);`, "String constant"},
		{`create consumed mcp service M.Svc (ProtocolVersion: v2025_03_26, Endpoint: @M.Missing);`, "not found"},
		{`create consumed mcp service M.Svc (ProtocolVersion: v2025_03_26, Endpoint: @M.Url, AuthenticationMicroflow: M.Nope);`, "not found"},
		{`create consumed mcp service M.Svc (ProtocolVersion: v2025_03_26, Endpoint: @M.Url, AuthenticationMicroflow: M.ReturnsString);`, "List of System.HttpHeader"},
		{`create consumed mcp service M.Svc (ProtocolVersion: v2025_03_26, Endpoint: @M.Url, AuthenticationMicroflow: M.TakesParam);`, "no parameters"},
		{`create constant M.Later type String default 'http://x/mcp';
create consumed mcp service M.Svc (ProtocolVersion: v2025_03_26, Endpoint: @M.Later);`, ""},
	} {
		t.Run(c.want+"/"+c.src[len(c.src)-40:], func(t *testing.T) {
			f := newConsumedMCPFixture(false)
			ctx, _ := newMockCtx(t, withBackend(f.mb), withHierarchy(f.h))
			errs := enumErrors(validateProgram(ctx, parseMDL(t, c.src)))
			if c.want == "" {
				if errs != "" {
					t.Fatalf("unexpected: %s", errs)
				}
				return
			}
			if !strings.Contains(errs, c.want) {
				t.Fatalf("errors %q, want one mentioning %q", errs, c.want)
			}
		})
	}
}
