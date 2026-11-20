// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
)

// ako/mxcli#2305: QUAL002's documentation table listed check_modules but the
// rule never looked at modules, so an undocumented app module — the one place
// a report can read what a module is for — went unreported.
func TestQUAL002_FlagsUndocumentedAppModules(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	for _, m := range [][3]string{
		{"Orders", "", ""},
		{"Billing", "Invoices and dunning.", ""},
		{"Mkt", "", "Marketplace v1.0.0"},
		{"System", "", ""},
	} {
		if _, err := cat.CatalogDB().Exec(`INSERT INTO modules_data (Id, Name, QualifiedName, DomainModelDocumentation, Source, ProjectId, SnapshotId)
			VALUES (?, ?, ?, ?, ?, 'default', 's1')`, moduleID(m[0]), m[0], m[0], m[1], m[2]); err != nil {
			t.Fatal(err)
		}
	}
	var flagged []string
	for _, v := range runMissingDocRule(t, cat, nil) {
		if v.Location.DocumentType == "Module" {
			flagged = append(flagged, v.Location.DocumentName)
		}
	}
	if strings.Join(flagged, ",") != "Orders" {
		t.Errorf("modules flagged = %v, want only Orders (Billing is documented, Mkt and System are not the app's)", flagged)
	}
	for _, v := range runMissingDocRule(t, cat, map[string]any{"check_modules": false}) {
		if v.Location.DocumentType == "Module" {
			t.Errorf("check_modules=false still reported %s", v.Location.DocumentName)
		}
	}
}

// moduleID gives System the platform module's fixed id, which is how the lint
// context recognises it.
func moduleID(name string) string {
	if name == "System" {
		return "00000000-0000-0000-0000-000000000001"
	}
	return "id-" + name
}
