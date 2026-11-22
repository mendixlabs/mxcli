// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
)

// mendixlabs/mxcli#1018 — a rewrite that does not restate a `/** … */` doc
// comment must not delete the stored one.
//
// Measured before the fix: `create or replace microflow` and `create or modify
// entity` both wrote an empty Documentation over the stored value, with the run
// reporting success and mx check clean (a document with no documentation is
// valid). `ALTER ENTITY … ADD ATTRIBUTE` preserved it, which is what localises
// the defect to the rewrite paths.
//
// Table-driven over every doctype that accepts a doc comment, because the
// carry-forward has to be added per rewrite path and fixing only the two
// doctypes in the report would leave the class open — the shape the wiki calls
// duplicate-resolver-drift.

type docPreserveCase struct {
	name string
	// storedOnly marks a doctype whose documentation DESCRIBE does not render,
	// so the assertion reads the stored units instead.
	storedOnly bool
	// modelsdk marks a doctype the legacy engine refuses to author (rules, and
	// anything else modelsdk-only). The harness defaults to legacy, so without
	// this the case fails at its own precondition and says nothing about #1018.
	modelsdk bool
	// minMajor/minMinor gate a doctype that does not exist in every supported
	// Mendix version. Without this the case fails at CREATE on an older
	// project, which says nothing about #1018 — the doctype is simply absent.
	// Kept as a version rather than a boolean so the reason is legible at the
	// case, and mirrors sdk/versions/mendix-11.yaml.
	minMajor, minMinor int
	// create carries a doc comment; rewrite deliberately does not.
	create   string
	rewrite  string
	describe string
}

const docMarker = "DOC-MARKER-PRESERVE-ME"

// storedContains searches the project's units for text. Some doctypes —
// associations, view entities — store documentation that DESCRIBE does not
// render, so the describe-based assertion cannot see them. Reading the stored
// bytes is what the original #1018 measurement did and is the more faithful
// check anyway: it asks what is in the model, not what the reader reports.
func storedContains(t *testing.T, projectPath, needle string) bool {
	t.Helper()
	dir := filepath.Join(filepath.Dir(projectPath), "mprcontents")
	found := false
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || found {
			return nil //nolint:nilerr // a partial walk is reported as not-found
		}
		b, readErr := os.ReadFile(p)
		if readErr == nil && bytes.Contains(b, []byte(needle)) {
			found = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return found
}

func docPreserveCases() []docPreserveCase {
	doc := "/** " + docMarker + " */\n"
	return []docPreserveCase{
		{
			name:     "entity",
			create:   doc + "create entity TestModule.DocEnt ( Label: String );",
			rewrite:  "create or modify entity TestModule.DocEnt ( Label: String, Extra: Integer );",
			describe: "describe entity TestModule.DocEnt",
		},
		{
			name:     "microflow",
			create:   doc + "create microflow TestModule.DocMf ()\nbegin\nend;",
			rewrite:  "create or replace microflow TestModule.DocMf ()\nbegin\nend;",
			describe: "describe microflow TestModule.DocMf",
		},
		{
			name:     "queue",
			create:   doc + "create queue TestModule.DocQ ( Parallelism: 2 );",
			rewrite:  "create or modify queue TestModule.DocQ ( Parallelism: 3 );",
			describe: "describe queue TestModule.DocQ",
		},
		{
			name:     "regular expression",
			create:   doc + "create regular expression TestModule.DocRe ( Expression: '[0-9]+' );",
			rewrite:  "create or modify regular expression TestModule.DocRe ( Expression: '[0-9]{2}' );",
			describe: "describe regular expression TestModule.DocRe",
		},
		{
			name: "scheduled event",
			create: "create microflow TestModule.DocSeFlow ()\nbegin\nend;\n" + doc +
				"create scheduled event TestModule.DocSe ( Microflow: TestModule.DocSeFlow, Repeat: Daily, HourOfDay: 4 );",
			rewrite:  "create or modify scheduled event TestModule.DocSe ( Microflow: TestModule.DocSeFlow, Repeat: Daily, HourOfDay: 5 );",
			describe: "describe scheduled event TestModule.DocSe",
		},
		{
			name:     "nanoflow",
			create:   doc + "create nanoflow TestModule.DocNf ()\nbegin\nend;",
			rewrite:  "create or replace nanoflow TestModule.DocNf ()\nbegin\nend;",
			describe: "describe nanoflow TestModule.DocNf",
		},
		{
			name:     "rule",
			modelsdk: true,
			create:   doc + "create rule TestModule.DocRule ()\nreturns Boolean\nbegin\n  return true;\nend;",
			rewrite:  "create or modify rule TestModule.DocRule ()\nreturns Boolean\nbegin\n  return false;\nend;",
			describe: "describe rule TestModule.DocRule",
		},
		{
			name:     "json structure",
			create:   doc + "create json structure TestModule.DocJs\n  snippet $${\"a\": 1}$$;",
			rewrite:  "create or modify json structure TestModule.DocJs\n  snippet $${\"a\": 1, \"b\": 2}$$;",
			describe: "describe json structure TestModule.DocJs",
		},
		{
			name:     "image collection",
			create:   doc + "create image collection TestModule.DocIc;",
			rewrite:  "create or modify image collection TestModule.DocIc;",
			describe: "describe image collection TestModule.DocIc",
		},
		{
			name: "workflow",
			create: "create entity TestModule.DocWfCtx ( Label: String );\n" + doc +
				"create workflow TestModule.DocWf\n  parameter $WorkflowContext: TestModule.DocWfCtx\nbegin\nend workflow;",
			rewrite:  "create or modify workflow TestModule.DocWf\n  parameter $WorkflowContext: TestModule.DocWfCtx\nbegin\nend workflow;",
			describe: "describe workflow TestModule.DocWf",
		},
		{
			name:     "constant",
			create:   doc + "create constant TestModule.DocConst type string default 'a';",
			rewrite:  "create or modify constant TestModule.DocConst type string default 'b';",
			describe: "describe constant TestModule.DocConst",
		},
		{
			name:       "association",
			storedOnly: true,
			create: "create entity TestModule.DocA ( L: String );\ncreate entity TestModule.DocB ( L: String );\n" + doc +
				"create association TestModule.DocA_DocB from TestModule.DocA to TestModule.DocB;",
			rewrite: "create or modify association TestModule.DocA_DocB from TestModule.DocA to TestModule.DocB;",
		},
		{
			name:       "view entity",
			storedOnly: true,
			create: "create entity TestModule.DocSrc ( L: String );\n" + doc +
				"create view entity TestModule.DocView as ( select s.L as L from TestModule.DocSrc as s );",
			rewrite: "create or modify view entity TestModule.DocView as ( select s.L as Label from TestModule.DocSrc as s );",
		},
		{
			name:       "business event service",
			storedOnly: true,
			create: "create entity TestModule.DocBePayload ( L: String );\n" + doc +
				"create business event service TestModule.DocBes\n( ServiceName: 'DocBes', EventNamePrefix: 'com.example' )\n{\n  message DocCreated (OrderId: long) publish\n    entity TestModule.DocBePayload;\n};",
			rewrite: "create or modify business event service TestModule.DocBes\n( ServiceName: 'DocBes2', EventNamePrefix: 'com.example' )\n{\n  message DocCreated (OrderId: long) publish\n    entity TestModule.DocBePayload;\n};",
		},
		{
			name:     "java action",
			create:   doc + "create java action MyFirstModule.DocJa () returns String as $$\npublic String executeAction() { return \"x\"; }\n$$;",
			rewrite:  "create or modify java action MyFirstModule.DocJa () returns String as $$\npublic String executeAction() { return \"y\"; }\n$$;",
			describe: "describe java action MyFirstModule.DocJa",
		},
		{
			name:     "menu",
			modelsdk: true,
			create:   "create microflow TestModule.DocMenuMf ()\nbegin\nend;\n" + doc + "create menu TestModule.DocMenu (\n  menu item 'Home' microflow TestModule.DocMenuMf;\n);",
			rewrite:  "create or modify menu TestModule.DocMenu (\n  menu item 'Home2' microflow TestModule.DocMenuMf;\n);",
			describe: "describe menu TestModule.DocMenu",
		},
		{
			name:       "odata service",
			storedOnly: true,
			create: "create entity TestModule.DocOdEnt ( L: String );\n" + doc +
				"create odata service TestModule.DocOd (\n  path: 'odata/doc/',\n  version: '1.0.0',\n  ODataVersion: OData4,\n  namespace: 'TestModule.Doc'\n)\nauthentication basic\n{\n  publish entity TestModule.DocOdEnt as 'Ents' (\n    ReadMode: source\n  )\n  expose (*);\n};",
			rewrite: "create or modify odata service TestModule.DocOd (\n  path: 'odata/doc2/',\n  version: '1.0.1',\n  ODataVersion: OData4,\n  namespace: 'TestModule.Doc'\n)\nauthentication basic\n{\n  publish entity TestModule.DocOdEnt as 'Ents' (\n    ReadMode: source\n  )\n  expose (*);\n};",
		},
		{
			name:       "page",
			storedOnly: true,
			create:     doc + "create page TestModule.DocPage ( Title: 'T', Layout: Atlas_Core.Atlas_Default ) {\n  container c { }\n};",
			rewrite:    "create or replace page TestModule.DocPage ( Title: 'T2', Layout: Atlas_Core.Atlas_Default ) {\n  container c2 { }\n};",
		},
		{
			name:       "snippet",
			storedOnly: true,
			create:     doc + "create snippet TestModule.DocSnip {\n  container c { }\n};",
			rewrite:    "create or replace snippet TestModule.DocSnip {\n  container c2 { }\n};",
		},
		{
			name:       "layout",
			storedOnly: true,
			modelsdk:   true,
			create:     doc + "create layout TestModule.DocLayout (\n  layouttype: 'Responsive'\n) {\n  scrollcontainer layoutContainer {\n    region center {\n      placeholder Main\n    }\n  }\n};",
			rewrite:    "create or replace layout TestModule.DocLayout (\n  layouttype: 'Responsive'\n) {\n  scrollcontainer layoutContainer {\n    region center (class: 'x') {\n      placeholder Main\n    }\n  }\n};",
		},
		{
			name:       "javascript action",
			storedOnly: true,
			create:     doc + "create javascript action MyFirstModule.DocJs ()\nreturns String\nas $$\nreturn Promise.resolve('a');\n$$;",
			rewrite:    "create or modify javascript action MyFirstModule.DocJs ()\nreturns String\nas $$\nreturn Promise.resolve('b');\n$$;",
		},
		{
			name:       "odata client",
			storedOnly: true,
			create:     doc + "create odata client MyFirstModule.DocOdc (\n  MetadataUrl: 'http://127.0.0.1:9/nope/$metadata'\n);",
			rewrite:    "create or modify odata client MyFirstModule.DocOdc (\n  MetadataUrl: 'http://127.0.0.1:9/other/$metadata'\n);",
		},
		{
			name:       "external entity",
			storedOnly: true,
			// The entity's attributes come from the client's cached contract
			// (ako/mxcli#2497), so the client has one.
			create: "create odata client MyFirstModule.DocExtOdc (\n  MetadataUrl: '" + thingsContractURL() + "'\n);\n" + doc +
				"create external entity MyFirstModule.DocExt\nfrom odata client MyFirstModule.DocExtOdc\n(\n  EntitySet: 'Things',\n  Countable: Yes\n);",
			rewrite: "create or modify external entity MyFirstModule.DocExt\nfrom odata client MyFirstModule.DocExtOdc\n(\n  EntitySet: 'Things',\n  Countable: No\n);",
		},
		{
			name:       "rest client",
			storedOnly: true,
			create:     doc + "create rest client MyFirstModule.DocRc (\n  BaseUrl: 'http://localhost:3001/api',\n  Authentication: none\n)\n{\n  operation \"Ping\" {\n    Method: get,\n    Path: '/ping'\n  }\n};",
			rewrite:    "create or modify rest client MyFirstModule.DocRc (\n  BaseUrl: 'http://localhost:3001/api2',\n  Authentication: none\n)\n{\n  operation \"Ping\" {\n    Method: get,\n    Path: '/ping'\n  }\n};",
		},
		{
			name:       "ai model",
			minMajor:   11,
			minMinor:   9,
			storedOnly: true,
			create:     doc + "create model TestModule.DocModel ( Provider: MxCloudGenAI );",
			rewrite:    "create or modify model TestModule.DocModel ( Provider: MxCloudGenAI );",
		},
		{
			name:       "knowledge base",
			minMajor:   11,
			minMinor:   9,
			storedOnly: true,
			create:     doc + "create knowledge base TestModule.DocKb ( Provider: MxCloudGenAI );",
			rewrite:    "create or modify knowledge base TestModule.DocKb ( Provider: MxCloudGenAI );",
		},
		{
			name:       "consumed mcp service",
			minMajor:   11,
			minMinor:   9,
			storedOnly: true,
			create:     doc + "create consumed mcp service TestModule.DocMcp ( ProtocolVersion: 'v2025_03_26' );",
			rewrite:    "create or modify consumed mcp service TestModule.DocMcp ( ProtocolVersion: 'v2025_03_26' );",
		},
		{
			name:       "agent",
			minMajor:   11,
			minMinor:   9,
			storedOnly: true,
			create: "create model TestModule.DocAgentModel ( Provider: MxCloudGenAI );\n" + doc +
				"create agent TestModule.DocAgent ( UsageType: Task, Model: TestModule.DocAgentModel, SystemPrompt: 'p' );",
			rewrite: "create or modify agent TestModule.DocAgent ( UsageType: Task, Model: TestModule.DocAgentModel, SystemPrompt: 'p2' );",
		},
		{
			name:     "enumeration",
			create:   doc + "create enumeration TestModule.DocEnum ( A 'A', B 'B' );",
			rewrite:  "create or replace enumeration TestModule.DocEnum ( A 'A', B 'B', C 'C' );",
			describe: "describe enumeration TestModule.DocEnum",
		},
		{
			name: "import mapping",
			create: "create entity TestModule.DocImEnt ( Code: String );\n" +
				"create json structure TestModule.DocImJs sample '{\"code\": \"x\"}';\n" + doc +
				"create import mapping TestModule.DocIm\n  with json structure TestModule.DocImJs\n{\n  create TestModule.DocImEnt {\n    Code = code\n  }\n};",
			rewrite:  "create or modify import mapping TestModule.DocIm\n  with json structure TestModule.DocImJs\n{\n  find TestModule.DocImEnt or create {\n    Code = code key\n  }\n};",
			describe: "describe import mapping TestModule.DocIm",
		},
		{
			name: "export mapping",
			create: "create entity TestModule.DocEmEnt ( Code: String );\n" +
				"create json structure TestModule.DocEmJs sample '{\"code\": \"x\"}';\n" + doc +
				"create export mapping TestModule.DocEm\n  with json structure TestModule.DocEmJs\n{\n  TestModule.DocEmEnt {\n    code = Code\n  }\n};",
			rewrite:  "create or modify export mapping TestModule.DocEm\n  with json structure TestModule.DocEmJs\n  null values SendAsNil\n{\n  TestModule.DocEmEnt {\n    code = Code\n  }\n};",
			describe: "describe export mapping TestModule.DocEm",
		},
		{
			name:     "published rest service",
			create:   doc + "create published rest service MyFirstModule.DocPrs (\n  Path: 'rest/doc/v1',\n  Version: '1.0.0'\n)\n{\n};",
			rewrite:  "create or modify published rest service MyFirstModule.DocPrs (\n  Path: 'rest/doc/v2',\n  Version: '2.0.0'\n)\n{\n};",
			describe: "describe published rest service MyFirstModule.DocPrs",
		},
		{
			name:     "data transformer",
			minMajor: 11,
			minMinor: 9,
			create:   doc + "create data transformer MyFirstModule.DocDt\n  source json '{\"a\": 1}'\n{\n  jslt '{\"b\": .a}';\n};",
			rewrite:  "create or modify data transformer MyFirstModule.DocDt\n  source json '{\"a\": 2}'\n{\n  jslt '{\"c\": .a}';\n};",
			describe: "describe data transformer MyFirstModule.DocDt",
		},
		{
			// The module's documentation is its domain model's
			// (mendixlabs/mxcli#1314).
			name:     "module",
			create:   doc + "create module DocModule1314;",
			rewrite:  "create or modify module DocModule1314;",
			describe: "describe module DocModule1314",
		},
	}
}

// TestDocumentation_SurvivesRewrite is the defect itself.
func TestDocumentation_SurvivesRewrite(t *testing.T) {
	for _, tc := range docPreserveCases() {
		t.Run(tc.name, func(t *testing.T) {
			env := setupTestEnv(t)
			if tc.modelsdk {
				env = setupTestEnvWithBackend(t, func() backend.FullBackend { return modelsdkbackend.New() })
			}
			defer env.teardown()

			if tc.minMajor > 0 {
				env.requireMinVersion(t, tc.minMajor, tc.minMinor)
			}

			if err := env.executeMDL(tc.create); err != nil {
				t.Fatalf("create: %v", err)
			}
			if tc.storedOnly {
				if !storedContains(t, env.projectPath, docMarker) {
					t.Fatalf("precondition failed: the doc comment did not reach the stored model")
				}
				if err := env.executeMDL(tc.rewrite); err != nil {
					t.Fatalf("rewrite: %v", err)
				}
				if !storedContains(t, env.projectPath, docMarker) {
					t.Errorf("the rewrite deleted the documentation it never mentioned (#1018)")
				}
				return
			}
			before, err := env.describeMDL(tc.describe)
			if err != nil {
				t.Fatalf("describe after create: %v", err)
			}
			if !strings.Contains(before, docMarker) {
				t.Fatalf("precondition failed: the doc comment did not survive CREATE, so this test "+
					"cannot say anything about the rewrite:\n%s", before)
			}

			if err := env.executeMDL(tc.rewrite); err != nil {
				t.Fatalf("rewrite: %v", err)
			}
			after, err := env.describeMDL(tc.describe)
			if err != nil {
				t.Fatalf("describe after rewrite: %v", err)
			}
			if !strings.Contains(after, docMarker) {
				t.Errorf("the rewrite deleted the documentation it never mentioned (#1018):\n%s", after)
			}
		})
	}
}

// TestDocumentation_UntouchedObjectKeepsItsOwn is the control that separates
// "rewrites drop documentation" from "writes drop documentation". Without it a
// green run proves only that nothing was written at all.
func TestDocumentation_UntouchedObjectKeepsItsOwn(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(
		"/** " + docMarker + " */\ncreate entity TestModule.DocBystander ( Label: String );",
	); err != nil {
		t.Fatalf("create bystander: %v", err)
	}
	if err := env.executeMDL(
		"/** unrelated */\ncreate microflow TestModule.DocOther ()\nbegin\nend;",
	); err != nil {
		t.Fatalf("create other: %v", err)
	}
	// Rewrite the OTHER document.
	if err := env.executeMDL(
		"create or replace microflow TestModule.DocOther ()\nbegin\nend;",
	); err != nil {
		t.Fatalf("rewrite other: %v", err)
	}

	out, err := env.describeMDL("describe entity TestModule.DocBystander")
	if err != nil {
		t.Fatalf("describe bystander: %v", err)
	}
	if !strings.Contains(out, docMarker) {
		t.Errorf("a bystander lost its documentation when a different document was rewritten:\n%s", out)
	}
}

// TestDocumentation_EmptyCommentClears pins the other half of the decision:
// an ABSENT doc comment preserves, an explicitly EMPTY one clears. Without this
// the fix makes documentation unclearable — and for a microflow there is no
// `ALTER MICROFLOW … SET DOCUMENTATION` to fall back on, so the empty comment is
// the only spelling available.
func TestDocumentation_EmptyCommentClears(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(
		"/** " + docMarker + " */\ncreate microflow TestModule.DocClear ()\nbegin\nend;",
	); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := env.executeMDL(
		"/** */\ncreate or replace microflow TestModule.DocClear ()\nbegin\nend;",
	); err != nil {
		t.Fatalf("rewrite with empty doc comment: %v", err)
	}
	out, err := env.describeMDL("describe microflow TestModule.DocClear")
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if strings.Contains(out, docMarker) {
		t.Errorf("an explicitly empty /** */ did not clear the documentation:\n%s", out)
	}
}

// thingsContractURL writes a $metadata with one entity set, Things, to a
// temporary file and returns its file:// URL.
func thingsContractURL() string {
	dir, err := os.MkdirTemp("", "doc-ext-contract")
	if err != nil {
		panic(err)
	}
	path := filepath.Join(dir, "metadata.xml")
	contract := `<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx Version="4.0" xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx">
  <edmx:DataServices>
    <Schema Namespace="Doc" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <EntityType Name="Thing"><Key><PropertyRef Name="Code"/></Key>
        <Property Name="Code" Type="Edm.String" MaxLength="20" Nullable="false"/></EntityType>
      <EntityContainer Name="C"><EntitySet Name="Things" EntityType="Doc.Thing"/></EntityContainer>
    </Schema>
  </edmx:DataServices>
</edmx:Edmx>`
	if err := os.WriteFile(path, []byte(contract), 0o644); err != nil {
		panic(err)
	}
	return "file://" + path
}
