// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"database/sql"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/catalog"
	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"

	_ "modernc.org/sqlite"
)

// testMicroflow returns a synthetic Microflow for use in walker unit tests.
func testMicroflow() linter.Microflow {
	return linter.Microflow{
		ID:            "mf1",
		Name:          "ACT_Process",
		QualifiedName: "MyModule.ACT_Process",
		ModuleName:    "MyModule",
	}
}

// setupEntitiesDB creates an in-memory SQLite database with entities and modules tables.
func setupEntitiesDB(t *testing.T, entities [][]any) catalog.CatalogDB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE modules (Name TEXT PRIMARY KEY, Source TEXT)`)
	if err != nil {
		t.Fatalf("failed to create modules table: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE entities (
		Id TEXT, Name TEXT, QualifiedName TEXT, ModuleName TEXT, Folder TEXT,
		EntityType TEXT, Description TEXT, Generalization TEXT,
		AttributeCount INTEGER, AccessRuleCount INTEGER, ValidationRuleCount INTEGER,
		HasEventHandlers INTEGER, IsExternal INTEGER
	)`)
	if err != nil {
		t.Fatalf("failed to create entities table: %v", err)
	}

	for _, row := range entities {
		_, err := db.Exec(`INSERT INTO entities VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row...)
		if err != nil {
			t.Fatalf("failed to insert entity: %v", err)
		}
		moduleName := row[3].(string)
		if _, err := db.Exec(`INSERT OR IGNORE INTO modules (Name, Source) VALUES (?, '')`, moduleName); err != nil {
			t.Fatalf("failed to insert module: %v", err)
		}
	}

	return catalog.WrapSqlDB(db)
}
