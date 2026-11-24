// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"database/sql"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ako/mxcli#2265: an Agent Editor document that names a document the project
// no longer has passes mx check and the build; Studio Pro reports it only when
// the project is opened.
func TestAgentDocumentDanglingReferenceRule(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`CREATE TABLE modules (Id TEXT, Name TEXT PRIMARY KEY, Source TEXT, AppStoreVersion TEXT)`,
		`INSERT INTO modules VALUES ('m', 'ServiceDesk', '', ''), ('lib', 'Lib', 'AppStore', '1.0.0')`,
		`CREATE TABLE objects (ObjectType TEXT, QualifiedName TEXT)`,
		`INSERT INTO objects VALUES ('MICROFLOW', 'ServiceDesk.Tool_GetOrderStatus2'), ('AI_MODEL', 'ServiceDesk.GPT'),
			('AGENT', 'ServiceDesk.SupportAgent')`,
		`CREATE TABLE refs (SourceType TEXT, SourceName TEXT, TargetType TEXT, TargetName TEXT, RefKind TEXT, ModuleName TEXT)`,
		// The renamed tool resolves; the stale name does not.
		`INSERT INTO refs VALUES
			('AGENT', 'ServiceDesk.SupportAgent', 'MICROFLOW', 'ServiceDesk.Tool_GetOrderStatus', 'call', 'ServiceDesk'),
			('AGENT', 'ServiceDesk.SupportAgent', 'MICROFLOW', 'ServiceDesk.Tool_GetOrderStatus2', 'call', 'ServiceDesk'),
			('AGENT', 'ServiceDesk.SupportAgent', 'AI_MODEL', 'ServiceDesk.GPT', 'model', 'ServiceDesk'),
			('AGENT', 'ServiceDesk.SupportAgent', 'ENTITY', 'System.User', 'type', 'ServiceDesk'),
			('AGENT', 'Lib.VendorAgent', 'MICROFLOW', 'Lib.Gone', 'call', 'Lib'),
			('MICROFLOW', 'ServiceDesk.Caller', 'MICROFLOW', 'ServiceDesk.Gone', 'call', 'ServiceDesk')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	ctx := linter.NewLintContextFromDB(catalog.WrapSqlDB(db))
	vs := NewAgentDocumentDanglingReferenceRule().Check(ctx)
	if got, want := violationNames(vs), "MPR022 ServiceDesk.SupportAgent"; got != want {
		t.Fatalf("violations = %q, want %q (not System, not a Marketplace module's, not a microflow's call)", got, want)
	}
	if want := "agent ServiceDesk.SupportAgent names microflow ServiceDesk.Tool_GetOrderStatus,"; vs[0].Message[:len(want)] != want {
		t.Errorf("message = %q", vs[0].Message)
	}
}
