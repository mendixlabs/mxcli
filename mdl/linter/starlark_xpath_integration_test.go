// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/catalog"
	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
)

const xpathTestRule = `
RULE_ID = "PERF001"
RULE_NAME = "xpath builtins"
DESCRIPTION = "exercises xpath_expressions and parse_xpath builtins"
CATEGORY = "performance"
SEVERITY = "info"

def check():
    out = []
    for e in xpath_expressions():
        ast = parse_xpath(e.xpath_expression)
        out.append(violation(
            message = "entry %s kind %s" % (e.document_qualified_name, ast.kind),
        ))
    return out
`

func TestStarlarkXPathBuiltins(t *testing.T) {
	// Use a file-based catalog: an in-memory one pools separate connections, so
	// inserts on one aren't visible to queries on another.
	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatalf("catalog.NewFromFile: %v", err)
	}
	defer cat.Close()
	db := cat.CatalogDB()

	// One retrieve action in module MyApp with a not(…) XPath.
	if _, err := db.Exec(
		`INSERT INTO xpath_expressions_data
		 (Id, DocumentType, DocumentId, DocumentQualifiedName,
		  ComponentType, ComponentId, XPathExpression,
		  IsParameterized, UsageType, ModuleName, ProjectId, SnapshotId)
		 VALUES ('x1','MICROFLOW','mf-1','MyApp.GetActiveItems',
		         'RETRIEVE_ACTION','ra-1','[not(Status = ''MyApp.Status.Active'')]',
		         0,'RETRIEVE','MyApp','default','s1')`); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "xpath_test.star")
	if err := os.WriteFile(path, []byte(xpathTestRule), 0644); err != nil {
		t.Fatal(err)
	}

	rule, err := linter.LoadStarlarkRule(path)
	if err != nil {
		t.Fatalf("LoadStarlarkRule: %v", err)
	}

	ctx := linter.NewLintContext(cat, nil)
	violations := rule.Check(ctx)

	var msgs []string
	for _, v := range violations {
		msgs = append(msgs, v.Message)
	}
	joined := strings.Join(msgs, "\n")

	for _, want := range []string{
		"entry MyApp.GetActiveItems", // xpath_expressions() returned the row
		"kind unary",                 // parse_xpath() parsed not(…) as a unary node
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in violations:\n%s", want, joined)
		}
	}
}
