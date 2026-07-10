// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"go.mongodb.org/mongo-driver/bson"
	_ "modernc.org/sqlite"
)

// JSON structures live inside a module but may be nested in subfolders.
// GetJsonStructureByQualifiedName must resolve container IDs through the
// folder hierarchy up to the owning module; otherwise addRestCallAction
// silently defaults SingleObject=false and produces invalid REST-call
// roundtrips on projects that organise their JSON structures in
// folders.
func TestGetJsonStructureByQualifiedName_ResolvesThroughFolders(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.mpr")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`
		CREATE TABLE Unit (
			UnitID BLOB PRIMARY KEY NOT NULL,
			ContainerID BLOB,
			ContainmentName TEXT,
			TreeConflict LONG,
			ContentsHash TEXT,
			ContentsConflicts TEXT,
			Contents BLOB
		)
	`); err != nil {
		t.Fatalf("create Unit: %v", err)
	}

	reader := &Reader{db: db, version: MPRVersionV1}

	moduleID := "11111111-1111-1111-1111-111111111111"
	folderID := "22222222-2222-2222-2222-222222222222"
	jsID := "33333333-3333-3333-3333-333333333333"
	otherModuleID := "44444444-4444-4444-4444-444444444444"

	// Module: SBOMModule
	modBSON, _ := bson.Marshal(bson.D{
		{Key: "$Type", Value: "Projects$ModuleImpl"},
		{Key: "$ID", Value: idToBsonBinary(moduleID)},
		{Key: "Name", Value: "SBOMModule"},
	})
	if _, err := db.Exec(`INSERT INTO Unit (UnitID, ContainerID, ContainmentName, Contents) VALUES (?, ?, 'Module', ?)`,
		uuidToBlob(moduleID), nil, modBSON); err != nil {
		t.Fatalf("insert module: %v", err)
	}

	// Other module (for the negative case)
	otherModBSON, _ := bson.Marshal(bson.D{
		{Key: "$Type", Value: "Projects$ModuleImpl"},
		{Key: "$ID", Value: idToBsonBinary(otherModuleID)},
		{Key: "Name", Value: "OtherModule"},
	})
	if _, err := db.Exec(`INSERT INTO Unit (UnitID, ContainerID, ContainmentName, Contents) VALUES (?, ?, 'Module', ?)`,
		uuidToBlob(otherModuleID), nil, otherModBSON); err != nil {
		t.Fatalf("insert other module: %v", err)
	}

	// Folder inside SBOMModule
	folderBSON, _ := bson.Marshal(bson.D{
		{Key: "$Type", Value: "Projects$Folder"},
		{Key: "$ID", Value: idToBsonBinary(folderID)},
		{Key: "Name", Value: "Payloads"},
	})
	if _, err := db.Exec(`INSERT INTO Unit (UnitID, ContainerID, ContainmentName, Contents) VALUES (?, ?, 'Folder', ?)`,
		uuidToBlob(folderID), uuidToBlob(moduleID), folderBSON); err != nil {
		t.Fatalf("insert folder: %v", err)
	}

	// JSON structure nested inside the folder (not the module directly).
	jsBSON, _ := bson.Marshal(bson.D{
		{Key: "$Type", Value: "JsonStructures$JsonStructure"},
		{Key: "$ID", Value: idToBsonBinary(jsID)},
		{Key: "Name", Value: "OrderPayload"},
		{Key: "Elements", Value: bson.A{
			int32(2),
			bson.D{
				{Key: "$Type", Value: "JsonStructures$JsonElement"},
				{Key: "ExposedName", Value: "Root"},
				{Key: "ElementType", Value: "Object"},
			},
		}},
	})
	if _, err := db.Exec(`INSERT INTO Unit (UnitID, ContainerID, ContainmentName, Contents) VALUES (?, ?, 'Document', ?)`,
		uuidToBlob(jsID), uuidToBlob(folderID), jsBSON); err != nil {
		t.Fatalf("insert json structure: %v", err)
	}

	// Lookup by owning module name should resolve through the folder.
	js, err := reader.GetJsonStructureByQualifiedName("SBOMModule", "OrderPayload")
	if err != nil {
		t.Fatalf("GetJsonStructureByQualifiedName (folder-nested): %v", err)
	}
	if js == nil {
		t.Fatal("expected non-nil JsonStructure")
	}
	if js.Name != "OrderPayload" {
		t.Errorf("Name = %q, want OrderPayload", js.Name)
	}
	if len(js.Elements) != 1 || js.Elements[0].ElementType != "Object" {
		t.Errorf("Elements = %+v, want one Object element", js.Elements)
	}
	if js.ContainerID != model.ID(folderID) {
		t.Errorf("ContainerID = %q, want folder ID %q", js.ContainerID, folderID)
	}

	// Cross-module lookup must fail (the structure belongs to SBOMModule, not OtherModule).
	if _, err := reader.GetJsonStructureByQualifiedName("OtherModule", "OrderPayload"); err == nil {
		t.Error("expected error for wrong-module lookup, got nil")
	}
}
