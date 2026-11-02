// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/agenteditor"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/security"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// ako/mxcli#707 — describe output that did not re-parse, or re-parsed into
// something other than what was stored.
//
// Every test here asserts the round-trip property rather than a substring: the
// describe output is fed to the real visitor, and the value the resulting
// statement would store is compared with the value the model holds. A substring
// assertion would have to spell the escape it checks, which is the thing under
// test.

// reparse parses describe output with the real visitor and fails the test,
// printing the output, when it does not parse.
func reparse(t *testing.T, out string) *ast.Program {
	t.Helper()
	prog, errs := visitor.Build(out)
	if len(errs) > 0 {
		t.Fatalf("describe output does not re-parse: %v\n--- output ---\n%s", errs, out)
	}
	assertTerminated(t, out) // #744
	return prog
}

// findStmt returns the first statement of type T in prog.
func findStmt[T ast.Statement](t *testing.T, prog *ast.Program, out string) T {
	t.Helper()
	for _, s := range prog.Statements {
		if v, ok := s.(T); ok {
			return v
		}
	}
	var zero T
	t.Fatalf("describe output has no %T statement:\n%s", zero, out)
	return zero
}

// --- apostrophes in described strings -------------------------------------

func TestIssue707_EntityStringDefaultAndErrorMessageReparse(t *testing.T) {
	mod := mkModule("Shop")
	attr := &domainmodel.Attribute{
		BaseElement: model.BaseElement{ID: nextID("attr")},
		Name:        "Label",
		Type:        &domainmodel.StringAttributeType{Length: 50},
		Value:       &domainmodel.AttributeValue{DefaultValue: "it's here"},
	}
	entity := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: nextID("ent")},
		Name:        "Product",
		Persistable: true,
		Attributes:  []*domainmodel.Attribute{attr},
		ValidationRules: []*domainmodel.ValidationRule{{
			BaseElement:  model.BaseElement{ID: nextID("vr")},
			AttributeID:  attr.ID,
			Type:         "Required",
			ErrorMessage: &model.Text{Translations: map[string]string{"en_US": "Label can't be empty"}},
		}},
	}
	dm := mkDomainModel(mod.ID, entity)
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeEntity(ctx, ast.QualifiedName{Module: "Shop", Name: "Product"}))

	out := buf.String()
	stmt := findStmt[*ast.CreateEntityStmt](t, reparse(t, out), out)
	if len(stmt.Attributes) != 1 {
		t.Fatalf("attributes = %d, want 1\n%s", len(stmt.Attributes), out)
	}
	got := stmt.Attributes[0]
	if got.DefaultValue != "it's here" {
		t.Errorf("default: stored %q, re-exec stores %v\n%s", "it's here", got.DefaultValue, out)
	}
	if got.NotNullError != "Label can't be empty" {
		t.Errorf("not null error: stored %q, re-exec stores %q\n%s", "Label can't be empty", got.NotNullError, out)
	}
}

func TestIssue707_ModuleRoleDescriptionReparses(t *testing.T) {
	mod := mkModule("Shop")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModuleSecurityFunc: func() ([]*security.ModuleSecurity, error) {
			return []*security.ModuleSecurity{{
				ContainerID: mod.ID,
				ModuleRoles: []*security.ModuleRole{{Name: "Clerk", Description: "Can't delete orders"}},
			}}, nil
		},
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) { return &security.ProjectSecurity{}, nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	assertNoError(t, describeModuleRole(ctx, ast.QualifiedName{Module: "Shop", Name: "Clerk"}))

	out := buf.String()
	stmt := findStmt[*ast.CreateModuleRoleStmt](t, reparse(t, out), out)
	if stmt.DescriptionText() != "Can't delete orders" {
		t.Errorf("description: stored %q, re-exec stores %q\n%s", "Can't delete orders", stmt.DescriptionText(), out)
	}
}

func TestIssue707_PublishedODataServiceStringsReparse(t *testing.T) {
	svc := &model.PublishedODataService{
		Name:         "Orders",
		Path:         "odata/o'reilly/v1",
		Version:      "1.0'beta",
		ODataVersion: "OData4",
		Namespace:    "O'Reilly",
		ServiceName:  "Reilly's orders",
		Summary:      "It's the orders API",
	}
	var out bytes.Buffer
	ctx := &ExecContext{Output: &out}
	assertNoError(t, outputPublishedODataServiceMDL(ctx, svc, "Shop", "Bob's APIs"))

	stmt := findStmt[*ast.CreateODataServiceStmt](t, reparse(t, out.String()), out.String())
	for _, c := range []struct{ field, stored, got string }{
		{"Folder", "Bob's APIs", stmt.Folder},
		{"Path", svc.Path, stmt.Path},
		{"Version", svc.Version, stmt.Version},
		{"Namespace", svc.Namespace, stmt.Namespace},
		{"ServiceName", svc.ServiceName, stmt.ServiceName},
		{"Summary", svc.Summary, stmt.Summary},
	} {
		if c.got != c.stored {
			t.Errorf("%s: stored %q, re-exec stores %q\n%s", c.field, c.stored, c.got, out.String())
		}
	}
}

func TestIssue707_PublishedRestServiceStringsReparse(t *testing.T) {
	mod := mkModule("Shop")
	svc := &model.PublishedRestService{
		BaseElement: model.BaseElement{ID: nextID("prs")},
		ContainerID: mod.ID,
		Name:        "OrderAPI",
		Path:        "rest/o'reilly/v1",
		Version:     "1.0'beta",
		ServiceName: "Reilly's orders",
		Resources: []*model.PublishedRestResource{{
			Name: "orders",
			Operations: []*model.PublishedRestOperation{
				{HTTPMethod: "get", Path: "{id}/it's", Microflow: "Shop.GetOrder"},
			},
		}},
	}
	h := mkHierarchy(mod)
	withContainer(h, svc.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListPublishedRestServicesFunc: func() ([]*model.PublishedRestService, error) {
			return []*model.PublishedRestService{svc}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describePublishedRestService(ctx, ast.QualifiedName{Module: "Shop", Name: "OrderAPI"}))

	out := buf.String()
	stmt := findStmt[*ast.CreatePublishedRestServiceStmt](t, reparse(t, out), out)
	if stmt.Path != svc.Path || stmt.Version != svc.Version || stmt.ServiceName != svc.ServiceName {
		t.Errorf("service strings: stored (%q, %q, %q), re-exec stores (%q, %q, %q)\n%s",
			svc.Path, svc.Version, svc.ServiceName, stmt.Path, stmt.Version, stmt.ServiceName, out)
	}
	if len(stmt.Resources) != 1 || len(stmt.Resources[0].Operations) != 1 {
		t.Fatalf("resources/operations did not survive:\n%s", out)
	}
	if got := stmt.Resources[0].Operations[0].Path; got != "{id}/it's" {
		t.Errorf("operation path: stored %q, re-exec stores %q\n%s", "{id}/it's", got, out)
	}
}

func TestIssue707_RestClientBaseUrlAndPathReparse(t *testing.T) {
	svc := &model.ConsumedRestService{
		Name:    "Reilly",
		BaseUrl: "https://api.example.com/o'reilly",
		Operations: []*model.RestClientOperation{{
			Name:         "GetBook",
			HttpMethod:   "GET",
			Path:         "/books/it's",
			Headers:      []*model.RestClientHeader{{Name: "X-Note", Value: "it's"}},
			ResponseType: "NONE",
		}},
	}
	ctx, buf := newMockCtx(t)
	assertNoError(t, outputConsumedRestServiceMDL(ctx, svc, "Shop"))

	out := buf.String()
	stmt := findStmt[*ast.CreateRestClientStmt](t, reparse(t, out), out)
	if stmt.BaseUrl != svc.BaseUrl {
		t.Errorf("BaseUrl: stored %q, re-exec stores %q\n%s", svc.BaseUrl, stmt.BaseUrl, out)
	}
	if len(stmt.Operations) != 1 {
		t.Fatalf("operations = %d, want 1\n%s", len(stmt.Operations), out)
	}
	op := stmt.Operations[0]
	if op.Path != "/books/it's" {
		t.Errorf("Path: stored %q, re-exec stores %q\n%s", "/books/it's", op.Path, out)
	}
	if len(op.Headers) != 1 || op.Headers[0].Value != "it's" {
		t.Errorf("header value: stored %q, re-exec stores %+v\n%s", "it's", op.Headers, out)
	}
}

// --- agent MCP block --------------------------------------------------------

// The MCP-service block printed `Enabled: true` and `Description: '…'` with no
// comma between them, so any agent whose MCP tool had a description described
// into a syntax error. The generic tool block right below it had the comma.
func TestIssue707_AgentMCPServiceWithDescriptionReparses(t *testing.T) {
	mod := mkModule("M")
	a := &agenteditor.Agent{
		BaseElement: model.BaseElement{ID: nextID("aea")},
		ContainerID: mod.ID,
		Name:        "Helper",
		UsageType:   "Task",
		Model:       &agenteditor.DocRef{QualifiedName: "M.GPT"},
		Tools: []agenteditor.AgentTool{{
			ID:          "11111111-2222-3333-4444-555555555555",
			ToolType:    "mcp",
			Enabled:     true,
			Description: "Looks up the customer's orders",
			Document:    &agenteditor.DocRef{QualifiedName: "M.OrdersMCP"},
		}},
	}
	h := mkHierarchy(mod)
	withContainer(h, a.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:           func() bool { return true },
		ListAgentEditorAgentsFunc: func() ([]*agenteditor.Agent, error) { return []*agenteditor.Agent{a}, nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeAgentEditorAgent(ctx, ast.QualifiedName{Module: "M", Name: "Helper"}))

	out := buf.String()
	stmt := findStmt[*ast.CreateAgentStmt](t, reparse(t, out), out)
	if len(stmt.Tools) != 1 {
		t.Fatalf("tools = %d, want 1\n%s", len(stmt.Tools), out)
	}
	if got := stmt.Tools[0]; !got.Enabled || got.Description != "Looks up the customer's orders" {
		t.Errorf("mcp tool: stored enabled/%q, re-exec stores %v/%q\n%s",
			"Looks up the customer's orders", got.Enabled, got.Description, out)
	}
}

// --- workflow captions -----------------------------------------------------

// A decision's and a parallel split's caption were printed only as a trailing
// `-- caption` comment, so describe → exec replaced them with "Decision" and
// "Parallel split". The grammar has always had `comment '…'` for both.
func TestIssue707_WorkflowDecisionAndSplitCaptionsSurvive(t *testing.T) {
	dec := &workflows.ExclusiveSplitActivity{Expression: "$WorkflowContext/Total > 1000"}
	dec.Name = "decision1"
	dec.Caption = "Large order?"
	dec.Outcomes = []workflows.ConditionOutcome{
		&workflows.BooleanConditionOutcome{Value: true, Flow: &workflows.Flow{}},
		&workflows.BooleanConditionOutcome{Value: false, Flow: &workflows.Flow{}},
	}
	split := &workflows.ParallelSplitActivity{}
	split.Name = "split1"
	split.Caption = "Check stock and credit"
	split.Outcomes = []*workflows.ParallelSplitOutcome{{Flow: &workflows.Flow{}}, {Flow: &workflows.Flow{}}}

	lines := formatWorkflowActivities(nil, &workflows.Flow{Activities: []workflows.WorkflowActivity{dec, split}}, "  ")
	src := "create workflow M.WF\n  parameter $WorkflowContext: M.E\nbegin\n" +
		strings.Join(lines, "\n") + "\nend workflow;"
	stmt := findStmt[*ast.CreateWorkflowStmt](t, reparse(t, src), src)
	if len(stmt.Activities) != 2 {
		t.Fatalf("activities = %d, want 2\n%s", len(stmt.Activities), src)
	}
	d, ok := stmt.Activities[0].(*ast.WorkflowDecisionNode)
	if !ok || d.Caption != dec.Caption || d.Name != dec.Name || d.Expression != dec.Expression {
		t.Errorf("decision: stored name/caption/expr %q/%q/%q, re-exec stores %+v\n%s",
			dec.Name, dec.Caption, dec.Expression, stmt.Activities[0], src)
	}
	s, ok := stmt.Activities[1].(*ast.WorkflowParallelSplitNode)
	if !ok || s.Caption != split.Caption || s.Name != split.Name {
		t.Errorf("parallel split: stored name/caption %q/%q, re-exec stores %+v\n%s",
			split.Name, split.Caption, stmt.Activities[1], src)
	}
}

// A decision whose caption is the default the writer supplies ("Decision")
// carries nothing: describe must not start emitting it, and the stored name
// must still come back. This is the control for the test above.
func TestIssue707_WorkflowDefaultCaptionStaysImplicit(t *testing.T) {
	dec := &workflows.ExclusiveSplitActivity{}
	dec.Name = "Decision"
	dec.Caption = "Decision"
	dec.Outcomes = []workflows.ConditionOutcome{
		&workflows.BooleanConditionOutcome{Value: true, Flow: &workflows.Flow{}},
	}
	lines := formatWorkflowActivities(nil, &workflows.Flow{Activities: []workflows.WorkflowActivity{dec}}, "  ")
	src := "create workflow M.WF\n  parameter $WorkflowContext: M.E\nbegin\n" +
		strings.Join(lines, "\n") + "\nend workflow;"
	if strings.Contains(src, "comment 'Decision'") {
		t.Errorf("default caption emitted as a clause:\n%s", src)
	}
	stmt := findStmt[*ast.CreateWorkflowStmt](t, reparse(t, src), src)
	if d, ok := stmt.Activities[0].(*ast.WorkflowDecisionNode); !ok || d.Caption != "" || d.Name != "" {
		t.Errorf("decision: want implicit name and caption, got %+v\n%s", stmt.Activities[0], src)
	}
}

// --- secrets ---------------------------------------------------------------

// DESCRIBE printed `DatabasePassword = '<plaintext>'`, so a described project
// committed to a repository leaked the credential into the diff. The password is
// omitted: `create or modify configuration` is a patch on an existing
// configuration, so re-executing the output leaves the stored password as it is.
func TestIssue707_ConfigurationDescribeDoesNotPrintPassword(t *testing.T) {
	const secret = "s3cr3t'pw"
	cfg := &model.ServerConfiguration{
		Name: "Default", DatabaseType: "PostgreSql", DatabaseUrl: "db.example.com:5432",
		DatabaseName: "o'reilly", DatabaseUserName: "mx", DatabasePassword: secret,
		HttpPortNumber: 8080, ServerPortNumber: 8090,
	}
	ps := &model.ProjectSettings{Configuration: &model.ConfigurationSettings{
		Configurations: []*model.ServerConfiguration{cfg},
	}}
	mb := &mock.MockBackend{
		IsConnectedFunc:           func() bool { return true },
		GetProjectSettingsFunc:    func() (*model.ProjectSettings, error) { return ps, nil },
		UpdateProjectSettingsFunc: func(*model.ProjectSettings) error { return nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeSettings(ctx, "Default"))
	out := buf.String()

	if strings.Contains(out, "s3cr3t") {
		t.Fatalf("describe printed the database password:\n%s", out)
	}

	// Round trip: executing the output on the same project changes nothing,
	// the password included.
	stmt := findStmt[*ast.CreateConfigurationStmt](t, reparse(t, out), out)
	before := *cfg
	buf.Reset()
	assertNoError(t, createConfiguration(ctx, stmt))
	if !reflect.DeepEqual(*cfg, before) {
		t.Errorf("re-executing describe output changed the configuration:\nbefore %+v\nafter  %+v\n%s", before, *cfg, out)
	}

	// And describing again gives the same text.
	buf.Reset()
	assertNoError(t, describeSettings(ctx, "Default"))
	if again := buf.String(); again != out {
		t.Errorf("describe is not a fixed point:\n--- first ---\n%s\n--- second ---\n%s", out, again)
	}
}

// DESCRIBE DEMO USER printed `password '***'`, and executing that set the
// password to three asterisks. The placeholder now means "the stored password":
// replayed onto the same project it keeps the stored one; replayed where the
// user does not exist there is no stored password, and the statement refuses
// rather than inventing one.
func demoUserCtx(t *testing.T, users ...*security.DemoUser) (*ExecContext, *bytes.Buffer, *security.ProjectSecurity) {
	t.Helper()
	ps := &security.ProjectSecurity{
		BaseElement:     model.BaseElement{ID: nextID("ps")},
		EnableDemoUsers: true,
		SecurityLevel:   security.SecurityLevelPrototype,
		DemoUsers:       users,
	}
	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) { return ps, nil },
		RemoveDemoUserFunc: func(_ model.ID, name string) error {
			var kept []*security.DemoUser
			for _, du := range ps.DemoUsers {
				if du.UserName != name {
					kept = append(kept, du)
				}
			}
			ps.DemoUsers = kept
			return nil
		},
		// Adds, or updates by user name in place — the backend's contract.
		AddDemoUserFunc: func(_ model.ID, name, pw, entity string, roles []string) error {
			for _, du := range ps.DemoUsers {
				if strings.EqualFold(du.UserName, name) {
					du.Password, du.Entity, du.UserRoles = pw, entity, roles
					return nil
				}
			}
			ps.DemoUsers = append(ps.DemoUsers, &security.DemoUser{UserName: name, Password: pw, Entity: entity, UserRoles: roles})
			return nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	return ctx, buf, ps
}

func TestIssue707_DemoUserReplayKeepsStoredPassword(t *testing.T) {
	ctx, buf, ps := demoUserCtx(t, &security.DemoUser{
		UserName: "demo_admin", Password: "Pa55word!", Entity: "Administration.Account", UserRoles: []string{"Administrator"},
	})
	assertNoError(t, describeDemoUser(ctx, "demo_admin"))
	out := buf.String()
	if strings.Contains(out, "Pa55word!") {
		t.Fatalf("describe printed the demo user's password:\n%s", out)
	}

	stmt := findStmt[*ast.CreateDemoUserStmt](t, reparse(t, out), out)
	assertNoError(t, execCreateDemoUser(ctx, stmt))
	if len(ps.DemoUsers) != 1 || ps.DemoUsers[0].Password != "Pa55word!" {
		t.Fatalf("re-executing describe output changed the password: %+v\n%s", ps.DemoUsers, out)
	}

	buf.Reset()
	assertNoError(t, describeDemoUser(ctx, "demo_admin"))
	if again := buf.String(); again != out {
		t.Errorf("describe is not a fixed point:\n--- first ---\n%s\n--- second ---\n%s", out, again)
	}
}

func TestIssue707_DemoUserPlaceholderRefusedForNewUser(t *testing.T) {
	src, srcBuf, _ := demoUserCtx(t, &security.DemoUser{
		UserName: "demo_admin", Password: "Pa55word!", Entity: "Administration.Account", UserRoles: []string{"Administrator"},
	})
	assertNoError(t, describeDemoUser(src, "demo_admin"))
	out := srcBuf.String()
	stmt := findStmt[*ast.CreateDemoUserStmt](t, reparse(t, out), out)

	// A project without that user: the placeholder has nothing to stand for.
	dst, _, ps := demoUserCtx(t)
	err := execCreateDemoUser(dst, stmt)
	if err == nil {
		t.Fatalf("placeholder password was accepted for a new user; stored %+v", ps.DemoUsers)
	}
	if !strings.Contains(err.Error(), "password") {
		t.Errorf("error does not say what is wrong: %v", err)
	}
	if len(ps.DemoUsers) != 0 {
		t.Errorf("a demo user was created: %+v", ps.DemoUsers)
	}

	// Control: a real password is still accepted.
	stmt.Password = "Pa55word!"
	assertNoError(t, execCreateDemoUser(dst, stmt))
	if len(ps.DemoUsers) != 1 || ps.DemoUsers[0].Password != "Pa55word!" {
		t.Errorf("control: real password not stored: %+v", ps.DemoUsers)
	}
}
