// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"
	"testing"
)

// ako/mxcli#1686: IMPORT … LINK through a table-stored self-association named
// both junction columns after the one entity (`myfirstmodule$personid`), so
// the insert named one column twice. Measured on Mendix 11.12.6 against
// PostgreSQL, `MyFirstModule.Person_Friend` from Person to Person, storage
// table:
//
//	mendixsystem$association: table_name myfirstmodule$person_friend,
//	  parent_column_name myfirstmodule$personid1,
//	  child_column_name  myfirstmodule$personid2
//	information_schema: the same two columns.
//
// The row's names are used; the convention, used only when there is no row,
// numbers the two the same way.

func TestJunctionColumnsFromTheSystemRow_1686(t *testing.T) {
	conn := openSQLite(t, "self1686",
		`CREATE TABLE mendixsystem$association (id TEXT, association_name TEXT, table_name TEXT,
			parent_column_name TEXT, child_column_name TEXT, storage_format)`,
		`INSERT INTO mendixsystem$association VALUES ('a', 'MyFirstModule.Person_Friend', 'myfirstmodule$person_friend',
			'myfirstmodule$personid1', 'myfirstmodule$personid2', 2)`,
	)
	sys, err := LookupAssociationInfo(context.Background(), conn, "MyFirstModule.Person_Friend")
	if err != nil || sys == nil {
		t.Fatalf("lookup: %v %v", sys, err)
	}
	p, c := JunctionColumns(sys, "MyFirstModule.Person", "MyFirstModule.Person")
	if p != "myfirstmodule$personid1" || c != "myfirstmodule$personid2" {
		t.Errorf("columns = %q, %q; want the row's personid1, personid2", p, c)
	}
}

func TestJunctionColumnsByConvention_1686(t *testing.T) {
	// No row (the app has not run since the association was made).
	p, c := JunctionColumns(nil, "MyFirstModule.Person", "MyFirstModule.Person")
	if p != "myfirstmodule$personid1" || c != "myfirstmodule$personid2" || p == c {
		t.Errorf("self-association = %q, %q; want personid1, personid2", p, c)
	}
	// Control: two entities keep their unnumbered names.
	p, c = JunctionColumns(nil, "MyFirstModule.EmployeeTbl", "MyFirstModule.Department")
	if p != "myfirstmodule$employeetblid" || c != "myfirstmodule$departmentid" {
		t.Errorf("two entities = %q, %q", p, c)
	}
}
