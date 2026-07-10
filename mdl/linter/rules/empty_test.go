// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"database/sql"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/catalog"
	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"

	_ "modernc.org/sqlite"
)

// setupMicroflowsDB creates an in-memory SQLite database with the microflows and modules tables.
// Each row is [Id, Name, QualifiedName, ModuleName, Folder, MicroflowType, Description, ReturnType, ParameterCount, ActivityCount, Complexity].
func setupMicroflowsDB(t *testing.T, rows [][]any) catalog.CatalogDB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE modules (Name TEXT PRIMARY KEY, Source TEXT)`)
	if err != nil {
		t.Fatalf("failed to create modules table: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE microflows (
		Id TEXT, Name TEXT, QualifiedName TEXT, ModuleName TEXT, Folder TEXT,
		MicroflowType TEXT, Description TEXT, ReturnType TEXT,
		ParameterCount INTEGER, ActivityCount INTEGER, Complexity INTEGER
	)`)
	if err != nil {
		t.Fatalf("failed to create microflows table: %v", err)
	}

	for _, row := range rows {
		_, err := db.Exec(`INSERT INTO microflows VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row...)
		if err != nil {
			t.Fatalf("failed to insert row: %v", err)
		}
		// Ensure module exists
		moduleName := row[3].(string)
		if _, err := db.Exec(`INSERT OR IGNORE INTO modules (Name, Source) VALUES (?, '')`, moduleName); err != nil {
			t.Fatalf("failed to insert module: %v", err)
		}
	}

	return catalog.WrapSqlDB(db)
}

func TestEmptyMicroflowRule_NoViolations(t *testing.T) {
	db := setupMicroflowsDB(t, [][]any{
		{"id1", "ACT_Process", "MyModule.ACT_Process", "MyModule", "", "Microflow", "", "Void", 0, 3, 1},
	})
	defer db.Close()

	ctx := linter.NewLintContextFromDB(db)
	rule := NewEmptyMicroflowRule()
	violations := rule.Check(ctx)

	if len(violations) != 0 {
		t.Errorf("expected 0 violations, got %d", len(violations))
	}
}

func TestEmptyMicroflowRule_DetectsEmpty(t *testing.T) {
	db := setupMicroflowsDB(t, [][]any{
		{"id1", "ACT_Process", "MyModule.ACT_Process", "MyModule", "", "Microflow", "", "Void", 0, 0, 0},
		{"id2", "ACT_Other", "MyModule.ACT_Other", "MyModule", "", "Microflow", "", "Void", 0, 5, 2},
	})
	defer db.Close()

	ctx := linter.NewLintContextFromDB(db)
	rule := NewEmptyMicroflowRule()
	violations := rule.Check(ctx)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].RuleID != "MPR002" {
		t.Errorf("expected rule ID MPR002, got %s", violations[0].RuleID)
	}
	if violations[0].Location.DocumentName != "ACT_Process" {
		t.Errorf("expected document ACT_Process, got %s", violations[0].Location.DocumentName)
	}
}

func TestEmptyMicroflowRule_Metadata(t *testing.T) {
	r := NewEmptyMicroflowRule()
	if r.ID() != "MPR002" {
		t.Errorf("ID = %q, want MPR002", r.ID())
	}
	if r.Category() != "quality" {
		t.Errorf("Category = %q, want quality", r.Category())
	}
}
