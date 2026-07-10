// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
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
