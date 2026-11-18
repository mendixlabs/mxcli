// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func TestCreateBusinessEventService(t *testing.T) {
	input := `CREATE BUSINESS EVENT SERVICE MyModule.OrderEvents (
		ServiceName: 'OrderService',
		EventNamePrefix: 'com.example.orders'
	) {
		MESSAGE OrderCreated (OrderId: Long) PUBLISH ENTITY MyModule.Order;
	};`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.CreateBusinessEventServiceStmt)
	if !ok {
		t.Fatalf("Expected CreateBusinessEventServiceStmt, got %T", prog.Statements[0])
	}
	if stmt.ServiceName != "OrderService" {
		t.Errorf("Got ServiceName %q", stmt.ServiceName)
	}
	if stmt.EventNamePrefix != "com.example.orders" {
		t.Errorf("Got EventNamePrefix %q", stmt.EventNamePrefix)
	}
	if len(stmt.Messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(stmt.Messages))
	}
	if stmt.Messages[0].MessageName != "OrderCreated" {
		t.Errorf("Got MessageName %q", stmt.Messages[0].MessageName)
	}
	if stmt.Messages[0].Operation != "PUBLISH" {
		t.Errorf("Got Operation %q", stmt.Messages[0].Operation)
	}
}

// ako/mxcli#2384 part B: the consumed shape. `Contract:` takes a $$…$$ block
// because the contract is YAML, and `consume` binds one of its events.
func TestCreateBusinessEventService_ConsumedShape(t *testing.T) {
	input := "mdl 1;\n" + `create business event service M.OrdersClient (
		Contract: $$asyncapi: 2.2.0
info:
  title: "OrdersEventService"
$$
	) {
		consume OrderPlaceEvent entity M.CBE_OrderPlaceEvent microflow M.HandleBE_OrderPlaceEvent;
	};`
	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.CreateBusinessEventServiceStmt)
	if !ok {
		t.Fatalf("got %T", prog.Statements[0])
	}
	if !stmt.IsConsumer() {
		t.Error("statement does not read as a consumer")
	}
	// Verbatim: the contract's own quoting and newlines are content.
	const wantContract = "asyncapi: 2.2.0\ninfo:\n  title: \"OrdersEventService\"\n"
	if stmt.Contract != wantContract {
		t.Errorf("contract = %q, want %q", stmt.Contract, wantContract)
	}
	if len(stmt.Consumes) != 1 {
		t.Fatalf("got %d consume items, want 1", len(stmt.Consumes))
	}
	c := stmt.Consumes[0]
	if c.MessageName != "OrderPlaceEvent" || c.Entity != "M.CBE_OrderPlaceEvent" || c.Microflow != "M.HandleBE_OrderPlaceEvent" {
		t.Errorf("consume item = %+v", c)
	}
}

// R11. The two shapes write different documents, so a statement that mixes them
// is refused by name rather than half-applied — guessing which half the author
// meant is how a contract gets dropped.
func TestCreateBusinessEventService_ShapesAreExclusive(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{
			"contract plus ServiceName",
			`( Contract: $$a: b$$, ServiceName: 'X' ) { consume E entity M.E; }`,
			"cannot also set ServiceName",
		},
		{
			"contract plus message",
			`( Contract: $$a: b$$ ) { message E (I: Long) publish entity M.E; }`,
			"use `consume",
		},
		{
			"both contract keys",
			`( Contract: $$a: b$$, ContractFile: 'c.yaml' ) { consume E entity M.E; }`,
			"not both",
		},
		{
			"consume without a contract",
			`( ServiceName: 'X' ) { consume E entity M.E; }`,
			"needs a contract",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := Build("mdl 1;\ncreate business event service M.S " + tc.body + ";")
			if len(errs) == 0 {
				t.Fatalf("mixed shape accepted: %s", tc.body)
			}
			var joined string
			for _, e := range errs {
				joined += e.Error() + "\n"
			}
			if !strings.Contains(joined, tc.want) {
				t.Errorf("error does not say what collided (want %q):\n%s", tc.want, joined)
			}
		})
	}
}
