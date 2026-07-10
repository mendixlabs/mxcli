// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/catalog"
)

func TestListLanguages_NilCatalog(t *testing.T) {
	ctx, _ := newMockCtx(t)
	// Catalog is nil by default in newMockCtx
	err := listLanguages(ctx)
	assertError(t, err)
}

func TestListLanguages_EmptyStringsTable(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatalf("failed to create catalog: %v", err)
	}
	defer cat.Close()

	ctx, buf := newMockCtx(t)
	ctx.Catalog = cat

	assertNoError(t, listLanguages(ctx))
	assertContainsStr(t, buf.String(), "No translatable strings found")
}

func TestListLanguages_WithRows(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatalf("failed to create catalog: %v", err)
	}
	defer cat.Close()

	db := cat.CatalogDB()
	_, err = db.Exec(`INSERT INTO strings (QualifiedName, ObjectType, StringValue, StringContext, Language, ElementId, ModuleName) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"MyModule.HomePage", "Page", "Hello", "Caption", "en_US", "id1", "MyModule")
	if err != nil {
		t.Fatalf("failed to seed strings table: %v", err)
	}
	_, err = db.Exec(`INSERT INTO strings (QualifiedName, ObjectType, StringValue, StringContext, Language, ElementId, ModuleName) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"MyModule.HomePage", "Page", "Bonjour", "Caption", "fr_FR", "id2", "MyModule")
	if err != nil {
		t.Fatalf("failed to seed strings table: %v", err)
	}

	ctx, buf := newMockCtx(t)
	ctx.Catalog = cat

	assertNoError(t, listLanguages(ctx))
	out := buf.String()
	assertContainsStr(t, out, "en_US")
	assertContainsStr(t, out, "fr_FR")
	assertContainsStr(t, out, "Language")
}
