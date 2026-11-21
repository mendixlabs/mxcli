// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#2449 (mendixlabs/mxcli#1903): adding one operation to a stored
// published REST service. `add resource` with the stored name passed check and
// exec refused it; `create or modify` naming only the new operation deleted
// the others, and exec said only "Modified published rest service".

func bookingService2449() *model.PublishedRestService {
	return &model.PublishedRestService{Name: "BookingApi", Resources: []*model.PublishedRestResource{{
		Name: "bookings",
		Operations: []*model.PublishedRestOperation{
			{HTTPMethod: "GET", Path: ""},
			{HTTPMethod: "GET", Path: "{id}"},
		},
	}}}
}

func TestCreateOrModifyNamesTheOperationsItRemoves_2449(t *testing.T) {
	stmt := &ast.CreatePublishedRestServiceStmt{CreateOrModify: true, Resources: []*ast.PublishedRestResourceDef{{
		Name: "bookings", Operations: []*ast.PublishedRestOperationDef{{HTTPMethod: "DELETE", Path: "{id}"}},
	}}}
	got := droppedPublishedRestOperations(bookingService2449(), stmt)
	if strings.Join(got, ", ") != "GET bookings, GET bookings/{id}" {
		t.Errorf("dropped = %v, want both GET operations", got)
	}
	// Control: the stored operations listed — describe's spelling, `/` and
	// `''` for the resource's own path — remove nothing.
	full := &ast.CreatePublishedRestServiceStmt{CreateOrModify: true, Resources: []*ast.PublishedRestResourceDef{{
		Name: "bookings", Operations: []*ast.PublishedRestOperationDef{
			{HTTPMethod: "get", Path: "/"}, {HTTPMethod: "GET", Path: "/{id}"}, {HTTPMethod: "DELETE", Path: "{id}"},
		},
	}}}
	if got := droppedPublishedRestOperations(bookingService2449(), full); len(got) != 0 {
		t.Errorf("a statement listing every stored operation removes %v", got)
	}
}

func TestAlterResourceProblemsMatchExec_2449(t *testing.T) {
	alter := func(actions ...ast.PublishedRestAlterAction) *ast.AlterPublishedRestServiceStmt {
		return &ast.AlterPublishedRestServiceStmt{Name: ast.QualifiedName{Module: "M", Name: "BookingApi"}, Actions: actions}
	}
	add := func(n string) ast.PublishedRestAlterAction {
		return &ast.PublishedRestAddResourceAction{Resource: &ast.PublishedRestResourceDef{Name: n}}
	}
	drop := func(n string) ast.PublishedRestAlterAction { return &ast.PublishedRestDropResourceAction{Name: n} }

	if p := publishedRestAlterResourceProblems(bookingService2449(), alter(add("bookings"))); len(p) != 1 || !strings.Contains(p[0], "already exists") {
		t.Errorf("add of a stored resource: %v", p)
	}
	if p := publishedRestAlterResourceProblems(bookingService2449(), alter(drop("rooms"))); len(p) != 1 || !strings.Contains(p[0], "not found") {
		t.Errorf("drop of a missing resource: %v", p)
	}
	// Controls: drop then add back, and a new resource, are what exec applies.
	for _, a := range [][]ast.PublishedRestAlterAction{{drop("bookings"), add("bookings")}, {add("rooms")}} {
		if p := publishedRestAlterResourceProblems(bookingService2449(), alter(a...)); len(p) != 0 {
			t.Errorf("valid actions refused: %v", p)
		}
	}
}
