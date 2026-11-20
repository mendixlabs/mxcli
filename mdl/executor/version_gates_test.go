// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1434: every version gate an exec handler applies is
// reported by CheckVersionGates — the pass `mxcli check -p` and exec's
// pre-flight run — on a project below the gate's floor, and none on one at or
// above it. The handlers refuse through checkVersionGates(ctx, stmt), so a
// statement in this table is refused by exec exactly when check reports it.
func TestCheckVersionGates_ReportsEveryHandlerGate(t *testing.T) {
	url := "item/{Id}"
	api := "API"
	cases := []struct {
		name  string
		stmt  ast.Statement
		below [3]int // a version under the gate's floor
		want  string // the statement the refusal names
	}{
		{"rest client", &ast.CreateRestClientStmt{}, [3]int{10, 0, 0}, "create rest client"},
		{"data transformer", &ast.CreateDataTransformerStmt{}, [3]int{11, 8, 0}, "create data transformer"},
		{"published rest", &ast.CreatePublishedRestServiceStmt{}, [3]int{9, 24, 0}, "create published rest service"},
		{"alter published rest", &ast.AlterPublishedRestServiceStmt{}, [3]int{9, 24, 0}, "alter published rest service"},
		{"grant published rest", &ast.GrantPublishedRestServiceAccessStmt{}, [3]int{9, 24, 0}, "grant access on published rest service"},
		{"odata graphql", &ast.CreateODataServiceStmt{SupportsGraphQL: true}, [3]int{10, 13, 0}, "SupportsGraphQL"},
		// ako/mxcli#1163: the document mxcli writes is 10.4+; 9.24.30 does not
		// load a project holding one (TypeCacheUnknownTypeException).
		{"published odata", &ast.CreateODataServiceStmt{}, [3]int{9, 24, 30}, "create published odata service"},
		{"published odata 10.3", &ast.CreateODataServiceStmt{}, [3]int{10, 3, 0}, "create published odata service"},
		{"message definition document", &ast.CreateMessageDefinitionStmt{}, [3]int{11, 14, 0}, "create message definition"},
		{"workflow group", &ast.AlterSettingsStmt{Section: "WORKFLOWS", AddGroup: true, GroupName: "G"}, [3]int{11, 1, 0}, "add group"},
		{"view entity", &ast.CreateViewEntityStmt{}, [3]int{10, 17, 0}, "create view entity"},
		// ako/mxcli#1632: an OQL comment is CE0174 below 11.7 (measured 10.24.25,
		// 11.6.6), so now that the query keeps one, a version that rejects it
		// refuses the statement instead of writing an unbuildable model.
		{"view query comment", &ast.CreateViewEntityStmt{Query: ast.OQLQuery{
			RawQuery: "-- totals\nselect sum(s.Amount) as Total\nfrom M.Sale as s"}}, [3]int{11, 6, 6}, "a comment in a view entity's OQL query"},
		// ako/mxcli#1650: STRING_AGG is 11.2+ (and 10.24.22+ on the 10.24 line).
		{"view query string_agg", &ast.CreateViewEntityStmt{Query: ast.OQLQuery{
			RawQuery: "select string_agg(s.Name, ', ') as Names\nfrom M.Sale as s"}}, [3]int{11, 1, 0}, "STRING_AGG in a view entity's OQL query"},
		{"agent model", &ast.CreateModelStmt{}, [3]int{11, 8, 0}, "create model"},
		{"consumed mcp", &ast.CreateConsumedMCPServiceStmt{}, [3]int{11, 8, 0}, "create consumed mcp service"},
		{"knowledge base", &ast.CreateKnowledgeBaseStmt{}, [3]int{11, 8, 0}, "create knowledge base"},
		{"agent", &ast.CreateAgentStmt{}, [3]int{11, 8, 0}, "create agent"},
		{"page parameters", &ast.CreatePageStmtV3{Parameters: []ast.PageParameter{{Name: "P"}}}, [3]int{9, 3, 0}, "create page with parameters"},
		{"send email", &ast.CreateMicroflowStmt{Body: []ast.MicroflowStatement{
			&ast.IfStmt{ThenBody: []ast.MicroflowStatement{&ast.SendEmailStmt{}}}}}, [3]int{11, 12, 0}, "send email"},
		{"deep link", &ast.CreateMicroflowStmt{URL: &url}, [3]int{10, 5, 0}, "a microflow URL"},
		{"synchronize unsynchronized", &ast.CreateNanoflowStmt{Body: []ast.MicroflowStatement{
			&ast.SynchronizeStmt{SyncType: "Unsynchronized"}}}, [3]int{9, 3, 0}, "synchronize unsynchronized"},
		{"workflow event handlers", &ast.CreateWorkflowStmt{EventHandlers: []ast.WorkflowEventHandlerNode{{AnyEvent: true}}}, [3]int{10, 6, 0}, "on workflow events"},
		{"workflow agent task", &ast.CreateWorkflowStmt{Activities: []ast.WorkflowActivityNode{
			&ast.WorkflowCallMicroflowNode{Agent: true}}}, [3]int{11, 8, 0}, "call agent microflow"},
		{"event subprocess", &ast.CreateWorkflowStmt{EventSubProcesses: []ast.WorkflowEventSubProcessNode{{Name: "E"}}}, [3]int{11, 7, 0}, "event subprocess"},
		{"timer event subprocess", &ast.CreateWorkflowStmt{EventSubProcesses: []ast.WorkflowEventSubProcessNode{{Name: "E", Timer: true}}}, [3]int{11, 12, 0}, "on … timer"},
		// ako/mxcli#1716: the 11.13 runtime has no recurrence on the start and
		// fires it once; the 11.15 runtime repeats it.
		{"repeating event subprocess", &ast.CreateWorkflowStmt{EventSubProcesses: []ast.WorkflowEventSubProcessNode{{Name: "E", Timer: true,
			Recurrence: &ast.WorkflowRecurrenceNode{Interval: 1, Unit: "day", MaxExecutions: 3}}}}, [3]int{11, 14, 0}, "repeat every"},
		{"notification", &ast.CreateWorkflowStmt{Activities: []ast.WorkflowActivityNode{&ast.WorkflowNotificationNode{}}}, [3]int{11, 10, 0}, "notification activities"},
		// mendixlabs/mxcli#1498: a domain-model export level is 9.3+.
		{"entity export level", &ast.CreateEntityStmt{ExportLevel: &api}, [3]int{9, 2, 0}, "export level on an entity"},
		{"attribute export level", &ast.CreateEntityStmt{Attributes: []ast.Attribute{{Name: "A", ExportLevel: &api}}}, [3]int{9, 2, 0}, "or attribute"},
		{"alter entity export level", &ast.AlterEntityStmt{Operation: ast.AlterEntitySetExportLevel, ExportLevel: "API"}, [3]int{9, 2, 0}, "export level on an entity"},
		{"modify attribute export level", &ast.AlterEntityStmt{Operation: ast.AlterEntityModifyAttribute, ModifyExportLevel: &api}, [3]int{9, 2, 0}, "or attribute"},
		{"association export level", &ast.CreateAssociationStmt{ExportLevel: &api}, [3]int{9, 2, 0}, "export level on an association"},
		{"alter association export level", &ast.AlterAssociationStmt{Operation: ast.AlterAssociationSetExportLevel}, [3]int{9, 2, 0}, "export level on an association"},
		{"enumeration export level", &ast.CreateEnumerationStmt{ExportLevel: &api}, [3]int{9, 2, 0}, "export level on an enumeration"},
		{"alter enumeration export level", &ast.AlterEnumerationStmt{Operation: ast.AlterEnumSetExportLevel}, [3]int{9, 2, 0}, "export level on an enumeration"},
		{"alter workflow", &ast.AlterWorkflowStmt{}, [3]int{8, 18, 0}, "alter workflow"},
		{"alter workflow agent task", &ast.AlterWorkflowStmt{Operations: []ast.AlterWorkflowOp{
			&ast.InsertAfterOp{NewActivities: []ast.WorkflowActivityNode{&ast.WorkflowCallMicroflowNode{Agent: true}}}}}, [3]int{11, 8, 0}, "call agent microflow"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog := &ast.Program{Statements: []ast.Statement{&ast.CreateModuleStmt{Name: "M"}, c.stmt}}

			vs := versionGateViolations(versionCtx(t, c.below[0], c.below[1], c.below[2]), prog)
			if len(vs) != 1 {
				t.Fatalf("want one MDL-VERSION01 below the floor, got %d: %+v", len(vs), vs)
			}
			if v := vs[0]; v.RuleID != "MDL-VERSION01" || !strings.HasPrefix(v.Message, "statement 2: ") ||
				!strings.Contains(v.Message, c.want) || v.Suggestion == "" {
				t.Errorf("violation = %+v, want statement 2 naming %q with a hint", v, c.want)
			}
			// The handler's own refusal is the same text: exec and check agree.
			if err := checkVersionGates(versionCtx(t, c.below[0], c.below[1], c.below[2]), c.stmt); err == nil ||
				!strings.Contains(vs[0].Message, strings.SplitN(err.Error(), "\n", 2)[0]) {
				t.Errorf("exec's refusal %v differs from check's %q", err, vs[0].Message)
			}

			// Control: a version with every feature passes.
			if vs := versionGateViolations(versionCtx(t, 11, 99, 0), prog); len(vs) != 0 {
				t.Errorf("control: reported at 11.99: %+v", vs)
			}
		})
	}
}

// Statements with no gate, or a gated statement without the gated clause, are
// not reported — `synchronize all` on 9.0, a page without parameters, OData
// without GraphQL from 10.4.
func TestCheckVersionGates_UngatedFormsPass(t *testing.T) {
	prog := &ast.Program{Statements: []ast.Statement{
		&ast.CreateModuleStmt{Name: "M"},
		&ast.CreateMicroflowStmt{},
		&ast.CreateNanoflowStmt{Body: []ast.MicroflowStatement{&ast.SynchronizeStmt{SyncType: "All"}}},
		&ast.CreatePageStmtV3{},
		&ast.AlterSettingsStmt{Section: "MODEL"},
		// No EXPORT LEVEL clause: the stored level is carried (#1498).
		&ast.CreateEntityStmt{Attributes: []ast.Attribute{{Name: "A"}}},
		&ast.CreateAssociationStmt{},
		&ast.CreateEnumerationStmt{},
		&ast.AlterEntityStmt{Operation: ast.AlterEntitySetDocumentation},
	}}
	if vs := versionGateViolations(versionCtx(t, 9, 0, 0), prog); len(vs) != 0 {
		t.Errorf("ungated forms reported on 9.0: %+v", vs)
	}
	odata := &ast.Program{Statements: []ast.Statement{&ast.CreateModuleStmt{Name: "M"}, &ast.CreateODataServiceStmt{}}}
	if vs := versionGateViolations(versionCtx(t, 10, 4, 0), odata); len(vs) != 0 {
		t.Errorf("a published OData service without GraphQL reported on 10.4: %+v", vs)
	}
}

// One source of truth: a handler that called checkFeature with its own feature
// key would be a gate check -p cannot see. Every gate goes through
// statementVersionGates (or, for a gate that depends on stored state, a helper
// in version_gates.go).
func TestCheckFeatureIsOnlyCalledFromTheGateTable(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	call := regexp.MustCompile(`\bcheckFeature\(`)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "version_gates.go" || f == "cmd_features.go" {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if call.MatchString(line) && !strings.HasPrefix(strings.TrimSpace(line), "//") {
				t.Errorf("%s:%d calls checkFeature directly; add the gate to statementVersionGates so `check -p` applies it too", f, i+1)
			}
		}
	}
}

// The comment gate fires on a COMMENT, not on the characters: a query with
// none, or with `--` and `/*` inside a string literal, passes on 10.24.25 — the
// view-entity floor is 10.21, so nothing else gates these.
func TestCheckVersionGates_ViewQueryWithoutCommentPasses(t *testing.T) {
	prog := &ast.Program{Statements: []ast.Statement{
		&ast.CreateModuleStmt{Name: "M"},
		&ast.CreateViewEntityStmt{Query: ast.OQLQuery{RawQuery: "select s.Code as Code\nfrom M.Sale as s"}},
		&ast.CreateViewEntityStmt{Query: ast.OQLQuery{RawQuery: "select s.Code as Code\nfrom M.Sale as s\nwhere s.Code != '--' and s.Code != '/* x */'"}},
	}}
	if vs := versionGateViolations(versionCtx(t, 10, 24, 25), prog); len(vs) != 0 {
		t.Errorf("a view query without a comment was gated on 10.24.25: %+v", vs)
	}
}

// ako/mxcli#2139: Mendix 11.15 removed message definition collections, and exec
// refused every collection statement on an 11.15+ project — but only inside the
// handler, so `check -p` passed the script ("Check passed!") that exec then
// failed with "Mendix 11.15 removed message definition collections". The
// refusal is a CEILING rather than a floor, and it is reported by the same pass
// as the floors, with exec's own text and hint.
func TestCheckVersionGates_MessageCollectionsRemovedFrom1115(t *testing.T) {
	name := ast.QualifiedName{Module: "Shop", Name: "ApiMessages"}
	cases := []struct {
		stmt ast.Statement
		want string
	}{
		{&ast.CreateMessageDefinitionCollectionStmt{Name: name}, "create message definition collection"},
		{&ast.CreateMessageDefinitionCollectionStmt{Name: name, CreateOrModify: true}, "create message definition collection"},
		{&ast.AlterMessageDefinitionCollectionStmt{Name: name, Op: "DROP", Target: "TicketIn"}, "alter message definition collection"},
		{&ast.DropMessageDefinitionCollectionStmt{Name: name}, "drop message definition collection"},
		{&ast.DescribeStmt{ObjectType: ast.DescribeMessageDefinitionCollection, Name: name}, "describe message definition collection"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			prog := &ast.Program{Statements: []ast.Statement{&ast.CreateModuleStmt{Name: "Shop"}, c.stmt}}
			vs := versionGateViolations(versionCtx(t, 11, 16, 0), prog)
			if len(vs) != 1 {
				t.Fatalf("want one MDL-VERSION01 on 11.16.0, got %d: %+v", len(vs), vs)
			}
			v := vs[0]
			if v.RuleID != "MDL-VERSION01" || !strings.HasPrefix(v.Message, "statement 2: ") ||
				!strings.Contains(v.Message, c.want) ||
				!strings.Contains(v.Message, "Mendix 11.15 removed message definition collections") ||
				!strings.Contains(v.Suggestion, "create message definition Module.Name for Module.Entity") {
				t.Errorf("violation = %+v, want statement 2 refusing %q with the document-form hint", v, c.want)
			}
			// exec's refusal is the same text.
			if err := checkVersionGates(versionCtx(t, 11, 16, 0), c.stmt); err == nil ||
				!strings.Contains(v.Message, strings.SplitN(err.Error(), "\n", 2)[0]) {
				t.Errorf("exec's refusal %v differs from check's %q", err, v.Message)
			}
			// Control: 11.14 still stores collections.
			if vs := versionGateViolations(versionCtx(t, 11, 14, 0), prog); len(vs) != 0 {
				t.Errorf("control: a collection statement was refused on 11.14: %+v", vs)
			}
		})
	}
}
