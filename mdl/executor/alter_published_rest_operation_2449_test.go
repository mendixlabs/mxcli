// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#2449 part 1: an operation of a stored resource, added or dropped
// on its own. The operation is written as `create` writes it, inside
// `add operation in resource '<name>' { … }`; the resource's other
// operations, and the service's other resources, are written back as stored.

func alterOps2449(t *testing.T, src string) (*model.PublishedRestService, error) {
	t.Helper()
	mod := mkModule("M")
	h := mkHierarchy(mod)
	svc := bookingService2449()
	svc.ID = nextID("prs")
	svc.ContainerID = mod.ID
	svc.Resources[0].Operations[0].Microflow = "M.PRS_ListBookings"
	svc.Resources[0].Operations[1].Microflow = "M.PRS_GetBooking"
	svc.Resources[0].Operations[1].Summary = "set in Studio Pro"
	withContainer(h, svc.ContainerID, mod.ID)
	var written *model.PublishedRestService
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListPublishedRestServicesFunc: func() ([]*model.PublishedRestService, error) {
			return []*model.PublishedRestService{svc}, nil
		},
		UpdatePublishedRestServiceFunc: func(s *model.PublishedRestService) error { written = s; return nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.AlterPublishedRestServiceStmt)
	if err := execAlterPublishedRestService(ctx, stmt); err != nil {
		return written, err
	}
	return written, nil
}

func operationKeys2449(svc *model.PublishedRestService) []string {
	var out []string
	for _, r := range svc.Resources {
		for _, op := range r.Operations {
			out = append(out, publishedRestOperationKey(r.Name, op.HTTPMethod, op.Path)+" -> "+op.Microflow)
		}
	}
	return out
}

func TestAlterAddOperationInResource_2449(t *testing.T) {
	got, err := alterOps2449(t, `alter published rest service M.BookingApi
  add operation in resource 'bookings' {
    delete '{id}' microflow M.PRS_DeleteBooking;
    post microflow M.PRS_CreateBooking commit Yes;
  };`)
	if err != nil {
		t.Fatal(err)
	}
	want := "GET bookings -> M.PRS_ListBookings, GET bookings/{id} -> M.PRS_GetBooking, " +
		"DELETE bookings/{id} -> M.PRS_DeleteBooking, POST bookings -> M.PRS_CreateBooking"
	if strings.Join(operationKeys2449(got), ", ") != want {
		t.Errorf("operations = %v\nwant %s", operationKeys2449(got), want)
	}
	// What MDL cannot spell on a stored operation stays.
	if got.Resources[0].Operations[1].Summary != "set in Studio Pro" {
		t.Error("the stored operation lost its summary")
	}
}

func TestAlterDropOperationInResource_2449(t *testing.T) {
	got, err := alterOps2449(t, `alter published rest service M.BookingApi drop operation get '{id}' in resource 'bookings';`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(operationKeys2449(got), ", ") != "GET bookings -> M.PRS_ListBookings" {
		t.Errorf("operations = %v", operationKeys2449(got))
	}
}

func TestAlterOperationRefusals_2449(t *testing.T) {
	for src, want := range map[string]string{
		`alter published rest service M.BookingApi add operation in resource 'bookings' { get '{id}' microflow M.X; };`: "already exists",
		`alter published rest service M.BookingApi add operation in resource 'rooms' { get microflow M.X; };`:           "add resource 'rooms'",
		`alter published rest service M.BookingApi drop operation put '{id}' in resource 'bookings';`:                   "not found",
	} {
		written, err := alterOps2449(t, src)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s\n  err = %v, want one mentioning %q", src, err, want)
		}
		if written != nil {
			t.Errorf("%s\n  written despite the refusal", src)
		}
		// check says the same before exec.
		prog, _ := visitor.Build(src)
		probs := publishedRestAlterResourceProblems(bookingService2449(), prog.Statements[0].(*ast.AlterPublishedRestServiceStmt))
		if len(probs) == 0 {
			t.Errorf("%s\n  check found nothing", src)
		}
	}
}
