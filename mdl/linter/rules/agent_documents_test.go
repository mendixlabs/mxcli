// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// agentDocsDB is a catalog with module M (the user's), a Marketplace module
// Lib, and an AgentEditorCommons of the given version ("" for none).
func agentDocsDB(t *testing.T, commonsVersion string) catalog.CatalogDB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`CREATE TABLE modules (Id TEXT, Name TEXT PRIMARY KEY, Source TEXT, AppStoreVersion TEXT)`,
		`INSERT INTO modules VALUES ('m', 'M', '', ''), ('lib', 'Lib', 'AppStore', '1.0.0')`,
		`CREATE TABLE agents (QualifiedName TEXT, ModuleName TEXT, SelectedModel TEXT)`,
		`INSERT INTO agents VALUES ('M.WithModel', 'M', 'gpt-4o'), ('M.NoModel', 'M', NULL),
			('M.EmptyModel', 'M', ''), ('Lib.NoModel', 'Lib', NULL)`,
		`CREATE TABLE consumed_mcp_services (QualifiedName TEXT, ModuleName TEXT, Endpoint TEXT)`,
		`INSERT INTO consumed_mcp_services VALUES ('M.WithEndpoint', 'M', 'M.Url'), ('M.NoEndpoint', 'M', ''),
			('Lib.NoEndpoint', 'Lib', '')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if commonsVersion != "" {
		if _, err := db.Exec(`INSERT INTO modules VALUES ('aec', 'AgentEditorCommons', 'AppStore', ?)`, commonsVersion); err != nil {
			t.Fatal(err)
		}
	}
	return catalog.WrapSqlDB(db)
}

func violationNames(vs []linter.Violation) string {
	var out []string
	for _, v := range vs {
		out = append(out, v.RuleID+" "+v.Location.DocumentName)
	}
	return strings.Join(out, "|")
}

// ako/mxcli#2089: nothing flagged a stored agent without a selected model or a
// consumed MCP service without an endpoint, whoever wrote it. A Marketplace
// module's documents are its vendor's, as for every other rule.
func TestAgentWithoutSelectedModelRule(t *testing.T) {
	ctx := linter.NewLintContextFromDB(agentDocsDB(t, "2.6.0"))
	if got, want := violationNames(NewAgentWithoutSelectedModelRule().Check(ctx)), "MPR014 M.EmptyModel|MPR014 M.NoModel"; got != want {
		t.Errorf("violations = %q, want %q", got, want)
	}
}

// Agent Editor 1.x does not read selectedModel (AgentEditorCommons 1.1.0), so
// an agent there is fine without one. Without the module the version is
// unknown and the rule applies, as every current release reads it.
func TestAgentWithoutSelectedModelRule_Version(t *testing.T) {
	if got := violationNames(NewAgentWithoutSelectedModelRule().Check(linter.NewLintContextFromDB(agentDocsDB(t, "1.1.0")))); got != "" {
		t.Errorf("AgentEditorCommons 1.1.0: violations = %q, want none", got)
	}
	if got := violationNames(NewAgentWithoutSelectedModelRule().Check(linter.NewLintContextFromDB(agentDocsDB(t, "")))); got == "" {
		t.Error("no AgentEditorCommons version: want the rule to apply")
	}
}

func TestConsumedMCPServiceWithoutEndpointRule(t *testing.T) {
	// Every measured Agent Editor reads the endpoint, 1.1.0 included.
	ctx := linter.NewLintContextFromDB(agentDocsDB(t, "1.1.0"))
	if got, want := violationNames(NewConsumedMCPServiceWithoutEndpointRule().Check(ctx)), "MPR015 M.NoEndpoint"; got != want {
		t.Errorf("violations = %q, want %q", got, want)
	}
}
