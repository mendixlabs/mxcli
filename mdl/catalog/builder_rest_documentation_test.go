// SPDX-License-Identifier: Apache-2.0

package catalog

import "testing"

// mendixlabs/mxcli#1510: the catalog wrote an empty Documentation for every
// published REST service ("not stored on published REST"), so QUAL002 reported
// each one as undocumented whatever Studio Pro or MDL had written — a finding
// nobody could close. The column carries the service's own documentation.
func TestPublishedRestServiceDocumentationReachesCatalog_Issue1510(t *testing.T) {
	svc := oneService()
	svc.Documentation = "The public orders API."
	cat := restFixture(t, svc)

	rows := queryRows(t, cat, `SELECT Documentation FROM published_rest_services WHERE QualifiedName = 'Sales.OrdersApi'`)
	if len(rows) != 1 {
		t.Fatalf("want 1 service row, got %d", len(rows))
	}
	if got := rows[0][0]; got != "The public orders API." {
		t.Errorf("Documentation = %v, want the service's documentation", got)
	}

	// Control: an undocumented service stays empty, so the rule still fires.
	cat = restFixture(t, oneService())
	rows = queryRows(t, cat, `SELECT Documentation FROM published_rest_services`)
	if len(rows) != 1 || rows[0][0] != "" {
		t.Errorf("undocumented service: got %v, want one empty Documentation", rows)
	}
}

// A published REST service's authentication reaches the catalog, as a
// published OData service's does, so `report configuration` can say which
// services need callers set up (ako/mxcli#2426).
func TestPublishedRestServiceAuthenticationReachesCatalog(t *testing.T) {
	svc := oneService()
	svc.AuthenticationTypes = []string{"Basic", "Session"}
	cat := restFixture(t, svc)
	rows := queryRows(t, cat, `SELECT AuthenticationTypes FROM published_rest_services WHERE QualifiedName = 'Sales.OrdersApi'`)
	if len(rows) != 1 || rows[0][0] != "Basic, Session" {
		t.Errorf("AuthenticationTypes = %v, want \"Basic, Session\"", rows)
	}
	// Control: a service without authentication stays empty.
	rows = queryRows(t, restFixture(t, oneService()), `SELECT AuthenticationTypes FROM published_rest_services`)
	if len(rows) != 1 || rows[0][0] != "" {
		t.Errorf("no authentication: got %v, want one empty value", rows)
	}
}
