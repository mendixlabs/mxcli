// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#2239: `add jar dependency` had no IF NOT EXISTS, so a script that
// declares a module's Java dependency could not be executed twice. The guard
// reuses the grammar's shared `ifNotExists` rule, as ADD ATTRIBUTE / ADD VALUE do.
func TestAddJarDependencyIfNotExistsParses(t *testing.T) {
	prog, errs := Build(`mdl 1;
alter module "ServiceDesk"
  add jar dependency if not exists (
    group = 'org.duckdb',
    artifact = 'duckdb_jdbc',
    version = '2.0.0.1',
    included = false,
  )
  add jar dependency ( group = 'org.example', artifact = 'plain', version = '1.0' );
`)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %s", errsText(errs))
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("got %d statements, want 1", len(prog.Statements))
	}
	stmt, ok := prog.Statements[0].(*ast.AlterModuleJarDepStmt)
	if !ok || len(stmt.Actions) != 2 {
		t.Fatalf("want one alter module with two actions, got %#v", prog.Statements[0])
	}
	guarded, ok := stmt.Actions[0].(*ast.AddJarDepAction)
	if !ok || !guarded.IfNotExists {
		t.Fatalf("guarded add: IfNotExists not carried (%#v)", stmt.Actions[0])
	}
	if guarded.Group != "org.duckdb" || guarded.Artifact != "duckdb_jdbc" || guarded.Version != "2.0.0.1" || guarded.Included {
		t.Errorf("the guard swallowed a property: %+v", guarded)
	}
	plain, ok := stmt.Actions[1].(*ast.AddJarDepAction)
	if !ok || plain.IfNotExists {
		t.Errorf("control: the bare form must not carry the guard (%#v)", stmt.Actions[1])
	}
}
