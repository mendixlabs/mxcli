// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/catalog"
)

// TestAssembleEntityContext_Issue396_DefinitionNotEmpty verifies that the
// Entity Definition section is populated, not empty. The bug was a SELECT on
// a non-existent column (IndexCount) that caused the query to fail silently,
// leaving the section body blank.
func TestAssembleEntityContext_Issue396_DefinitionNotEmpty(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatalf("failed to create catalog: %v", err)
	}
	defer cat.Close()

	db := cat.CatalogDB()
	_, err = db.Exec(`INSERT INTO entities_data
		(Id, Name, QualifiedName, ModuleName, EntityType, Generalization, AttributeCount)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"uuid-1", "Account", "Administration.Account", "Administration",
		"Persistable", "System.User", 3)
	if err != nil {
		t.Fatalf("failed to seed entities: %v", err)
	}

	ctx, buf := newMockCtx(t)
	ctx.Catalog = cat

	var out strings.Builder
	assembleEntityContext(ctx, &out, "Administration.Account", 2)

	got := out.String()
	if strings.Contains(got, "### Entity Definition\n\n\n") {
		t.Errorf("Entity Definition section is empty (bug #396); got:\n%s", got)
	}
	if !strings.Contains(got, "Account") {
		t.Errorf("expected entity name 'Account' in output; got:\n%s", got)
	}
	if !strings.Contains(got, "Persistable") {
		t.Errorf("expected EntityType 'Persistable' in output; got:\n%s", got)
	}
	if !strings.Contains(got, "System.User") {
		t.Errorf("expected Generalization 'System.User' in output; got:\n%s", got)
	}
	_ = buf
}
