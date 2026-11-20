// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

func deprecationCodes(prog *ast.Program) []string {
	var out []string
	for _, d := range prog.Deprecations {
		out = append(out, d.Code)
	}
	return out
}

func mustBuild(t *testing.T, src string) *ast.Program {
	t.Helper()
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", src, errs)
	}
	return prog
}

// Every registry entry's own example must be detected, and its canonical
// rewrite must be silent AND build the same statements. The last part is what
// makes the rewrite safe to apply mechanically (ADR-0011: a wrong rewrite is a
// silent change of meaning).
func TestRegistryExamplesRecordTheirCode(t *testing.T) {
	for _, e := range deprecation.All() {
		t.Run(e.Code, func(t *testing.T) {
			old := mustBuild(t, e.Example)
			if got := deprecationCodes(old); !reflect.DeepEqual(got, []string{e.Code}) {
				t.Errorf("Example %q recorded %v, want [%s]", e.Example, got, e.Code)
			}
			canon := mustBuild(t, e.CanonicalExample)
			if got := deprecationCodes(canon); len(got) != 0 {
				t.Errorf("CanonicalExample %q recorded %v, want none", e.CanonicalExample, got)
			}
			if !reflect.DeepEqual(old.Statements, canon.Statements) {
				t.Errorf("Example and CanonicalExample build different statements:\n old:   %#v\n canon: %#v",
					old.Statements, canon.Statements)
			}
			// A structural rewrite is proven by the AST comparison above alone.
			if e.Rewrite.Structural != "" {
				return
			}
			// The rewrite is a token swap; the canonical example must be exactly it.
			re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(e.Rewrite.Token) + `\b`)
			if got := re.ReplaceAllString(e.Example, e.Rewrite.Replacement); got != e.CanonicalExample {
				t.Errorf("Rewrite %s -> %s gives %q, CanonicalExample is %q",
					e.Rewrite.Token, e.Rewrite.Replacement, got, e.CanonicalExample)
			}
		})
	}
}

// createOrReplaceCases holds one statement body per document kind that
// createStatement accepts. TestCreateOrReplaceMatchesModifyExceptExemptKinds
// builds each with `create or replace` and with `create or modify`.
var createOrReplaceCases = map[string]string{
	"entity":                      "persistent entity M.Customer (Name: String(200));",
	"association":                 "association M.Order_Customer from M.Order to M.Customer type Reference;",
	"module":                      "module M;",
	"modulerole":                  "module role M.Admin description 'Full access';",
	"microflow":                   "microflow M.ACT_Recalculate () begin return; end;",
	"javaaction":                  "java action M.FormatCurrency(Amount: Decimal not null) returns String as $$return \"\";$$;",
	"javascriptaction":            "javascript action M.IsStrictMode() returns Boolean platform Web as $$return true;$$;",
	"page":                        "page M.OrderList (Title: 'Orders', Layout: Atlas_Core.Atlas_Default) { dynamictext txtHeading (Content: 'Orders') };",
	"layout":                      "layout M.App_Default (layouttype: 'Responsive') { placeholder Main }",
	"snippet":                     "snippet M.CustomerInfo { dynamictext t (Content: 'x') }",
	"enumeration":                 "enumeration M.Color (Red 'Red');",
	"validationrule":              "validation rule for M.Customer.Email regex M.EmailPattern error message 'Invalid';",
	"databaseconnection":          "database connection M.Erp (Type: 'PostgreSQL', ConnectionString: @M.DbUrl, Username: @M.DbUser, Password: @M.DbPass);",
	"constant":                    "constant M.ApiBaseUrl ( Type: String, DefaultValue: 'https://api.example.com' );",
	"restclient":                  "consumed rest service M.PetStore (BaseUrl: 'https://petstore.example.com', Authentication: NONE) { };",
	"index":                       "index idx_name on M.Customer (Name);",
	"odataclient":                 "consumed odata service M.Api (Version: '1.0', ODataVersion: OData4, MetadataUrl: 'https://api.example.com/$metadata');",
	"odataservice":                "published odata service M.CustomerAPI (path: 'odata/customers/', version: '1.0.0', ODataVersion: OData4, namespace: 'M.Customers', Authentication: (basic)) { };",
	"externalentity":              "external entity M.Remote from consumed odata service M.Api (EntitySet: 'Remotes', RemoteName: 'Remote');",
	"externalentities":            "external entities from M.Api into Integration;",
	"navigation":                  "navigation Responsive home page M.Home_Web;",
	"businesseventservice":        "business event service M.CustomerEventsApi (ServiceName: 'CustomerEventsApi', EventNamePrefix: '') { message CustomerChangedEvent (CustomerId: Long) publish entity M.PBE_CustomerChangedEvent; };",
	"workflow":                    "workflow M.LeaveApproval parameter $Context: M.LeaveRequest begin end workflow;",
	"userrole":                    "user role Clerk ( ModuleRoles: (M.User) );",
	"demouser":                    "demo user 'demo' ( Password: 'Password1!', UserRoles: (Clerk) );",
	"imagecollection":             "image collection M.AppIcons;",
	"annotation":                  "annotation in M (Caption: 'Orders', Position: (60, 40));",
	"queue":                       "task queue M.Q_Orders (Parallelism: 3);",
	"scheduledevent":              "scheduled event M.NightlyCleanup (Microflow: M.SE_Cleanup, Repeat: Daily, HourOfDay: 4, MinuteOfHour: 0, TimeZone: Server, Enabled: true);",
	"regularexpression":           "regular expression M.Email (Expression: '.+@.+');",
	"jsonstructure":               "json structure M.JSON_Pet sample '{\"id\": 1}';",
	"messagedefinitioncollection": "message definition collection M.MD_Order {definition OrderMessage for M.Order as 'Orders' {OrderId}};",
	"messagedefinition":           "message definition M.OrderMessage for M.Order as 'Orders' {OrderId};",
	"importmapping":               "import mapping M.IMM_Order with json structure M.JSON_Order { create M.Order { Id = id } };",
	"exportmapping":               "export mapping M.EMM_Order with json structure M.JSON_Order { M.Order { orderId = OrderId } };",
	"configuration":               "configuration 'Default';",
	"publishedrestservice":        "published rest service M.OrderAPI (Path: 'rest/orders/v1', Version: '1.0.0', ServiceName: 'Order API') { };",
	"datatransformer":             "data transformer M.Flatten source json '{\"id\": 1}' { jslt '{\"id\": .id}'; };",
	"model":                       "ai model M.GPT4 (Provider: MxCloudGenAI, Key: @M.ModelApiKey);",
	"consumedmcpservice":          "consumed mcp service M.WebSearch (ProtocolVersion: v2025_03_26, Version: '1.0');",
	"publishedmcpservice":         "published mcp service M.Svc (EndpointPath: 'svc', Version: '1.0', ProtocolVersion: v2025_03_26);",
	"knowledgebase":               "knowledge base M.Docs (Provider: MxCloudGenAI, Key: @M.KBApiKey);",
	"agent":                       "agent M.Summarizer (UsageType: Task, Model: M.GPT4, SystemPrompt: 'Summarize.', UserPrompt: 'Text.');",
	"nanoflow":                    "nanoflow M.NF_Validate () begin return; end;",
	"rule":                        "rule M.Rule_IsSolvent ($c: M.Customer) returns Boolean begin return true; end;",
	"menu":                        "menu M.Main_Menu { menu item 'Plain' };",
	"translations":                "translations in Administration for nl_NL ('Save' as 'Opslaan');",
}

// foldPageModeFlags erases the AST differences between `or replace` and `or
// modify` that mean nothing: pages, snippets and layouts keep both flags, and
// every reader in the executor tests `IsModify || IsReplace`; microflows and
// nanoflows keep the spelling, which only messages read.
func foldPageModeFlags(stmts []ast.Statement) {
	for _, s := range stmts {
		switch n := s.(type) {
		case *ast.CreatePageStmtV3:
			n.IsModify, n.IsReplace = n.IsModify || n.IsReplace, false
		case *ast.CreateSnippetStmtV3:
			n.IsModify, n.IsReplace = n.IsModify || n.IsReplace, false
		case *ast.CreateLayoutStmt:
			n.IsModify, n.IsReplace = n.IsModify || n.IsReplace, false
		case *ast.CreateMicroflowStmt:
			// The spelling only, kept so a refusal names the statement as
			// written (ako/mxcli#1681).
			n.CreateOrReplace = false
		case *ast.CreateNanoflowStmt:
			n.CreateOrReplace = false
		}
	}
}

// createStatementKinds reads the document kinds createStatement accepts from
// the grammar source, so a kind added there fails this test until it has a case.
func createStatementKinds(t *testing.T) []string {
	t.Helper()
	src, err := os.ReadFile("../grammar/MDLParser.g4")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	start := strings.Index(s, "\ncreateStatement\n")
	if start < 0 {
		t.Fatal("createStatement rule not found in MDLParser.g4")
	}
	end := strings.Index(s[start:], "\n    ;")
	body := s[start : start+end]
	var kinds []string
	for _, m := range regexp.MustCompile(`\bcreate(\w+)Statement\b`).FindAllStringSubmatch(body, -1) {
		kinds = append(kinds, strings.ToLower(m[1]))
	}
	sort.Strings(kinds)
	return kinds
}

// `create or replace` is reported as MDL-DEPR001 exactly where it means
// `create or modify`: the two build the same statements. Where they do not,
// it is not an alias, and reporting it would tell the user to make a change
// that alters their script. Both directions are asserted, so the exemption list
// in the visitor cannot drift from what the visitors actually do.
//
// Which kinds are exempt depends on the language version (#731, ADR-0011):
// under mdl 1, `or replace` on a view entity, user role or demo user means
// `or modify` like everywhere else, so only translations stay exempt. Under
// mdl 0 those three keep their alpha meaning and report the language change
// that would alter it instead of the alias.
func TestCreateOrReplaceMatchesModifyExceptExemptKinds(t *testing.T) {
	for _, kind := range createStatementKinds(t) {
		if _, ok := createOrReplaceCases[kind]; !ok {
			t.Errorf("create kind %q has no case in createOrReplaceCases", kind)
		}
	}

	cases := make(map[string]string, len(createOrReplaceCases)+1)
	for k, v := range createOrReplaceCases {
		cases[k] = v
	}
	// A view entity is kind "entity" too.
	cases["entity (view)"] = "view entity M.V (Name: String(100)) as (select c.Name as Name from M.Customer as c);"

	// gatedChange names the language change a kind's `or replace` goes through;
	// a kind absent here means the same thing under every version.
	gatedChange := map[string]string{
		"entity (view)": viewEntityReplaceIsModify.Code,
		"userrole":      roleReplaceIsModify.Code,
		"demouser":      roleReplaceIsModify.Code,
	}

	for _, version := range []struct {
		name, header string
		v1           bool
	}{{"mdl 0", "", false}, {"mdl 1", "mdl 1;\n", true}} {
		for name, body := range cases {
			t.Run(version.name+"/"+name, func(t *testing.T) {
				rep := mustBuild(t, version.header+"create or replace "+strings.TrimSuffix(body, ";")+";") // every statement ends with ; (valid under mdl 0 and mdl 1)
				mod := mustBuild(t, version.header+"create or modify "+strings.TrimSuffix(body, ";")+";")
				if len(rep.Statements) != 1 || len(mod.Statements) != 1 {
					t.Fatalf("want one statement each, got %d and %d — the case does not exercise the visitor",
						len(rep.Statements), len(mod.Statements))
				}
				foldPageModeFlags(rep.Statements)
				foldPageModeFlags(mod.Statements)
				same := reflect.DeepEqual(rep.Statements, mod.Statements)
				gate, gated := gatedChange[name]
				exempt := createOrReplaceIsNotAnAlias[name] || (gated && !version.v1)

				switch {
				case exempt && same:
					t.Errorf("exempt, but `or replace` and `or modify` build the same statements: " +
						"it is an alias after all — remove the exemption")
				case !exempt && !same:
					t.Errorf("`or replace` and `or modify` build different statements, so rewriting one "+
						"to the other changes the script — exempt this kind or fix the visitor:\n"+
						" replace: %#v\n modify:  %#v", rep.Statements, mod.Statements)
				}

				var notes []string
				for _, n := range rep.LanguageNotes {
					notes = append(notes, n.Code)
				}
				wantNotes := []string(nil)
				if gated && !version.v1 {
					wantNotes = []string{gate}
				}
				if !reflect.DeepEqual(notes, wantNotes) {
					t.Errorf("language notes %v, want %v", notes, wantNotes)
				}

				got := deprecationCodes(rep)
				if exempt {
					if len(got) != 0 {
						t.Errorf("exempt kind recorded %v", got)
					}
					return
				}
				if !reflect.DeepEqual(got, []string{deprecation.CreateOrReplace}) {
					t.Errorf("recorded %v, want [%s]", got, deprecation.CreateOrReplace)
				} else if want := strings.TrimSuffix(name, " (view)"); rep.Deprecations[0].Subject != want {
					t.Errorf("subject = %q, want %q", rep.Deprecations[0].Subject, want)
				}
				if got := deprecationCodes(mod); len(got) != 0 {
					t.Errorf("`create or modify` recorded %v", got)
				}
			})
		}
	}
}

func TestShowRecordsDeprecation(t *testing.T) {
	cases := []struct {
		src  string
		want []string
	}{
		{"show entities;", []string{deprecation.Show}},
		{"SHOW MICROFLOWS IN M;", []string{deprecation.Show}},
		{"show callers of M.MF transitive;", []string{deprecation.Show}},
		{"show catalog tables;", []string{deprecation.Show}},
		{"show access on M.E;", []string{deprecation.Show}},
		{"show design properties;", []string{deprecation.Show}},
		{"show widgets;", []string{deprecation.Show}},
		// A single thing is described, not listed (PROPOSAL_mdl_beta_syntax_freeze.md
		// §3, R6): MDL-DEPR090, never MDL-DEPR002, so `fmt --upgrade` does
		// not rewrite them to a `list` that is not canonical either. `list`
		// on these forms is reported too.
		// `show entity|association X` alias nothing: gated instead (MDL-V1-SHOWSUMMARY).
		{"show entity M.E;", nil},
		{"show association M.A;", nil},
		{"show page M.P;", []string{deprecation.ShowSingleThing}},
		{"list page M.P;", []string{deprecation.ShowSingleThing}},
		// The navigation and settings tables are listings (ako/mxcli#755).
		{"show navigation;", []string{deprecation.Show}},
		{"list navigation;", nil},
		{"show navigation homes;", []string{deprecation.Show}},
		{"list navigation homes;", nil},
		{"show navigation menu M.Nav;", []string{deprecation.Show}},
		{"show structure depth 2 in M;", []string{deprecation.ShowSingleThing}},
		{"show context of M.MF;", []string{deprecation.ShowSingleThing}},
		{"show project security;", []string{deprecation.ShowSingleThing}},
		{"show security matrix in M;", []string{deprecation.ShowSingleThing}},
		{"show settings;", []string{deprecation.Show}},
		{"list settings;", nil},
		// Session state is a session command (R7): gated as MDL-V1-SESSION,
		// not a deprecated spelling.
		{"show version;", nil},
		{"show status;", nil},
		{"show connections;", nil},
		{"show catalog status;", nil},
		{"list entities;", nil},
		{"describe entity M.E;", nil},
		// Not the statement keyword: microflow activities spelled `show`.
		{"create or modify microflow M.MF () begin show page M.P(); show message 'hi' type Information; end;", nil},
		// `show lint rules` has no `list` form yet, so it is not an alias.
		{"show lint rules;", nil},
	}
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			if got := deprecationCodes(mustBuild(t, c.src)); !reflect.DeepEqual(got, c.want) {
				t.Errorf("recorded %v, want %v", got, c.want)
			}
		})
	}
}

// The record points at the deprecated token, so a warning in a long script
// names the right line.
func TestDeprecationRecordsPosition(t *testing.T) {
	prog := mustBuild(t, "list modules;\n\n  create or replace enumeration M.C (A 'A');\nshow modules;")
	want := []ast.DeprecatedSpelling{
		{Code: deprecation.CreateOrReplace, Line: 3, Column: 12, Subject: "enumeration"},
		{Code: deprecation.Show, Line: 4, Column: 0},
	}
	if !reflect.DeepEqual(prog.Deprecations, want) {
		t.Errorf("got %+v\nwant %+v", prog.Deprecations, want)
	}
}
