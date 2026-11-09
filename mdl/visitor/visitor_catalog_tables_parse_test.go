// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/catalog"
)

// Every table SHOW CATALOG TABLES names must parse as CATALOG.<name>. A table
// whose name is also a lexer keyword (COMMUNITIES, QUEUES, and FOLDERS for
// ako/mxcli#1436) parsed to no statement at all unless catalogTableName listed
// it: no error, no output, exit 0. Each was found by hand after it shipped; this
// enumerates the catalog's own list, so the next one fails here instead.
func TestEveryCatalogTableParses(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	tables := cat.Tables()
	if len(tables) < 30 {
		t.Fatalf("control: the catalog lists %d tables", len(tables))
	}
	for _, name := range tables {
		for _, q := range []string{
			"SELECT * FROM " + name + ";",
			"select * from " + strings.ToLower(name) + ";",
		} {
			prog, errs := Build(q)
			if len(errs) > 0 {
				t.Errorf("%s: parse error: %v", q, errs[0])
				continue
			}
			if len(prog.Statements) != 1 {
				t.Errorf("%s: %d statements, want 1 -- add the keyword to catalogTableName", q, len(prog.Statements))
				continue
			}
			if _, ok := prog.Statements[0].(*ast.SelectStmt); !ok {
				t.Errorf("%s: parsed as %T, want a SelectStmt", q, prog.Statements[0])
			}
		}
	}
}
