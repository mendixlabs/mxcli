// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#2384 part B: a consumed business event service is authorable. It is
// the contract plus one subscription per event, and no Definition — that
// absence is what tells the two roles apart in storage.

// consumerCtx returns a context over a project holding `stored` (nil for an
// empty project), and a pointer to whatever gets written.
func consumerCtx(t *testing.T, stored *model.BusinessEventService) (*ExecContext, **model.BusinessEventService) {
	t.Helper()
	mod := mkModule("M")
	h := mkHierarchy(mod)
	services := []*model.BusinessEventService{}
	if stored != nil {
		stored.ContainerID = mod.ID
		withContainer(h, stored.ContainerID, mod.ID)
		services = append(services, stored)
	}
	written := new(*model.BusinessEventService)
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListBusinessEventServicesFunc: func() ([]*model.BusinessEventService, error) {
			return services, nil
		},
		CreateBusinessEventServiceFunc: func(svc *model.BusinessEventService) error { *written = svc; return nil },
		UpdateBusinessEventServiceFunc: func(svc *model.BusinessEventService) error { *written = svc; return nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, written
}

func consumerStmt(contract, contractFile string) *ast.CreateBusinessEventServiceStmt {
	return &ast.CreateBusinessEventServiceStmt{
		Name:         ast.QualifiedName{Module: "M", Name: "OrdersEventClient"},
		Contract:     contract,
		ContractFile: contractFile,
		ContractSet:  true,
		Consumes: []*ast.BusinessEventConsumeDef{{
			MessageName: "OrderPlaceEvent",
			Entity:      "M.CBE_OrderPlaceEvent",
			Microflow:   "M.HandleBE_OrderPlaceEvent",
		}},
	}
}

func TestCreateBusinessEventService_FromInlineContract(t *testing.T) {
	ctx, written := consumerCtx(t, nil)
	assertNoError(t, createBusinessEventService(ctx, consumerStmt(storedContract, "")))

	svc := *written
	if svc == nil {
		t.Fatal("nothing was written")
	}
	if svc.Document != storedContract {
		t.Errorf("contract not stored verbatim:\n got %q\nwant %q", svc.Document, storedContract)
	}
	// The discriminator. A consumer with a Definition is a shape Studio Pro
	// never writes, and it is what the pre-part-B rebuild produced.
	if svc.Definition != nil {
		t.Errorf("a consumed service must have no Definition, got %+v", svc.Definition)
	}
	if len(svc.OperationImplementations) != 1 {
		t.Fatalf("wrote %d operations, want 1", len(svc.OperationImplementations))
	}
	op := svc.OperationImplementations[0]
	// `consume` is the MDL keyword; the stored Operation stays "subscribe",
	// which is what Studio Pro writes and what the runtime reads.
	if op.Operation != "subscribe" {
		t.Errorf("Operation = %q, want subscribe", op.Operation)
	}
	if op.MessageName != "OrderPlaceEvent" || op.Entity != "M.CBE_OrderPlaceEvent" || op.Microflow != "M.HandleBE_OrderPlaceEvent" {
		t.Errorf("subscription not written: %+v", op)
	}
}

func TestCreateBusinessEventService_FromContractFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "contract.yaml"), []byte(storedContract), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, written := consumerCtx(t, nil)
	ctx.ScriptDir = dir

	assertNoError(t, createBusinessEventService(ctx, consumerStmt("", "contract.yaml")))
	svc := *written
	if svc == nil {
		t.Fatal("nothing was written")
	}
	// The text is what gets stored, so nothing afterwards depends on the file:
	// the same service described and re-run elsewhere carries its own contract.
	if svc.Document != storedContract {
		t.Errorf("ContractFile not stored verbatim:\n got %q\nwant %q", svc.Document, storedContract)
	}
}

// An empty or missing file must be an error, not an empty contract: mxbuild
// throws "business events service should have definitions" on a contractless
// consumer, which is a far worse place to find out.
func TestCreateBusinessEventService_UnusableContractFileRefused(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "empty.yaml"), []byte("   \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, file string }{
		{"empty file", "empty.yaml"},
		{"missing file", "nope.yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, written := consumerCtx(t, nil)
			ctx.ScriptDir = dir
			err := createBusinessEventService(ctx, consumerStmt("", tc.file))
			if err == nil {
				t.Fatalf("accepted an unusable ContractFile; wrote %+v", *written)
			}
			if *written != nil {
				t.Errorf("wrote a service anyway: %+v", *written)
			}
			if !strings.Contains(err.Error(), tc.file) {
				t.Errorf("error does not name the file: %v", err)
			}
		})
	}
}

// The #2381 guard has a lifecycle (docs-wiki/bug-patterns/rewrite-drops-
// unauthored-state.md): while MDL could not express a consumer it refused every
// rewrite of a stored contract. Now that it can, a contract-bearing statement is
// a restatement and must pass — otherwise the feature's own scripts are refused
// by the backstop that exists to protect them.
func TestCreateOrModifyBusinessEventService_ContractStatementIsNotRefused(t *testing.T) {
	stored := storedConsumer("")
	ctx, written := consumerCtx(t, stored)

	stmt := consumerStmt(storedContract, "")
	stmt.CreateOrModify = true
	assertNoError(t, createBusinessEventService(ctx, stmt))

	svc := *written
	if svc == nil {
		t.Fatal("the rewrite was refused; the guard did not change with the feature")
	}
	if svc.Document != storedContract {
		t.Errorf("contract not preserved through the rewrite: %d chars", len(svc.Document))
	}
	// The operation $ID is carried by message name, so an unchanged rewrite
	// equals the stored unit and is elided (ADR-0008).
	if len(svc.OperationImplementations) != 1 || svc.OperationImplementations[0].ID != "sp-op" {
		t.Errorf("operation $ID not carried: %+v", svc.OperationImplementations)
	}
}

// The other arm. A published-shaped statement over a stored contract still
// destroys it, so it must still refuse — a guard that opened up for part B and
// forgot this would reinstate #2381 in silence.
func TestCreateOrModifyBusinessEventService_PublishedShapeStillRefusedOverAContract(t *testing.T) {
	stored := storedConsumer("")
	ctx, written := consumerCtx(t, stored)

	err := createBusinessEventService(ctx, &ast.CreateBusinessEventServiceStmt{
		Name:           ast.QualifiedName{Module: "M", Name: "OrdersEventClient"},
		ServiceName:    "OrdersEventService",
		CreateOrModify: true,
		Messages: []*ast.BusinessEventMessageDef{{
			MessageName: "OrderPlaceEvent", Operation: "subscribe", Entity: "M.CBE_OrderPlaceEvent",
			Attributes: []*ast.BusinessEventAttributeDef{{Name: "OrderId", TypeName: "Long"}},
		}},
	})
	if err == nil {
		t.Fatalf("a published-shaped rewrite over a contract was accepted; #2381 is back.\nwritten = %+v", *written)
	}
	if *written != nil {
		t.Errorf("the write was attempted anyway: %+v", *written)
	}
}

// A statement silent about the entity or the handler keeps the stored ones: the
// same partial-statement rule the published shape follows (#1018, #2381).
func TestCreateOrModifyBusinessEventService_ConsumeCarriesStoredBindings(t *testing.T) {
	stored := storedConsumer("")
	ctx, written := consumerCtx(t, stored)

	stmt := &ast.CreateBusinessEventServiceStmt{
		Name:           ast.QualifiedName{Module: "M", Name: "OrdersEventClient"},
		Contract:       storedContract,
		ContractSet:    true,
		CreateOrModify: true,
		Consumes:       []*ast.BusinessEventConsumeDef{{MessageName: "OrderPlaceEvent"}},
	}
	assertNoError(t, createBusinessEventService(ctx, stmt))

	op := (*written).OperationImplementations[0]
	if op.Entity != "M.CBE_OrderPlaceEvent" {
		t.Errorf("stored entity dropped: %q", op.Entity)
	}
	if op.Microflow != "M.HandleBE_OrderPlaceEvent" {
		t.Errorf("stored handler dropped: %q", op.Microflow)
	}
}
