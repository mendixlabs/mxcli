// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#2020: a published MCP service and its tools.
func TestCreatePublishedMCPService(t *testing.T) {
	prog, errs := Build(`create or modify published mcp service Shop.Orders (
  ServerName: 'Orders', EndpointPath: 'orders', Version: '1.0.0', ProtocolVersion: v2025_03_26,
  AuthenticationEnabled: true, AuthenticationMicroflow: Shop.MCP_Authenticate,
  AuthorizationServerUrl: 'https://login.example.com/v2.0', Scopes: 'mcp.read mcp.write',
  Documentation: 'Order tools'
) {
  tool get_order ( Microflow: Shop.GetOrder, Description: 'Fetch one order', Title: 'Get order', Schema: '{"type": "object"}' )
  tool list_orders ( Microflow: Shop.ListOrders, Description: 'List orders' )
};`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	s, ok := prog.Statements[0].(*ast.CreatePublishedMCPServiceStmt)
	if !ok {
		t.Fatalf("got %T", prog.Statements[0])
	}
	if !s.CreateOrModify || s.ServerName != "Orders" || s.EndpointPath != "orders" || s.Version != "1.0.0" ||
		s.ProtocolVersion != "v2025_03_26" || !s.AuthenticationEnabled || s.AuthenticationMicroflow == nil ||
		s.AuthenticationMicroflow.String() != "Shop.MCP_Authenticate" || s.AuthorizationServerURL != "https://login.example.com/v2.0" ||
		s.Scopes != "mcp.read mcp.write" || s.InnerDocumentation != "Order tools" {
		t.Errorf("service: %+v", s)
	}
	if len(s.Tools) != 2 {
		t.Fatalf("tools: %+v", s.Tools)
	}
	if t0 := s.Tools[0]; t0.Name != "get_order" || t0.Microflow == nil || t0.Microflow.String() != "Shop.GetOrder" ||
		t0.Description != "Fetch one order" || t0.Title != "Get order" || t0.Schema != `{"type": "object"}` {
		t.Errorf("tool 0: %+v", t0)
	}
}

func TestAlterPublishedMCPService(t *testing.T) {
	prog, errs := Build(`alter published mcp service Shop.Orders
  set Version = '1.1.0', Scopes = 'mcp.read'
  add tool cancel_order ( Microflow: Shop.CancelOrder, Description: 'Cancel an order' )
  drop tool list_orders;
list published mcp services in Shop;
describe published mcp service Shop.Orders;
drop published mcp service Shop.Orders;`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	a, ok := prog.Statements[0].(*ast.AlterPublishedMCPServiceStmt)
	if !ok {
		t.Fatalf("got %T", prog.Statements[0])
	}
	if a.Sets["version"] != "1.1.0" || a.Sets["scopes"] != "mcp.read" || len(a.AddTools) != 1 ||
		a.AddTools[0].Name != "cancel_order" || a.AddTools[0].Microflow.String() != "Shop.CancelOrder" ||
		len(a.DropTools) != 1 || a.DropTools[0] != "list_orders" {
		t.Errorf("alter: %+v", a)
	}
	if s, ok := prog.Statements[1].(*ast.ShowStmt); !ok || s.ObjectType != ast.ShowPublishedMCPServices || s.InModule != "Shop" {
		t.Errorf("list: %#v", prog.Statements[1])
	}
	if d, ok := prog.Statements[2].(*ast.DescribeStmt); !ok || d.ObjectType != ast.DescribePublishedMCPService {
		t.Errorf("describe: %#v", prog.Statements[2])
	}
	if _, ok := prog.Statements[3].(*ast.DropPublishedMCPServiceStmt); !ok {
		t.Errorf("drop: %T", prog.Statements[3])
	}
}

// A key the document does not have is refused, not dropped; ServerName is one
// it has.
func TestCreatePublishedMCPService_UnknownKey(t *testing.T) {
	_, errs := Build(`mdl 1;
create published mcp service Shop.Orders (ServerName: 'O', EndPoint: 'o') {
  tool t ( Microflow: Shop.T, Document: Shop.T )
};`)
	all := ""
	for _, e := range errs {
		all += e.Error() + "\n"
	}
	if strings.Contains(all, "unknown property 'ServerName'") || !strings.Contains(all, "unknown property 'EndPoint'") || !strings.Contains(all, "unknown property 'Document'") {
		t.Errorf("want both unknown keys reported, got:\n%s", all)
	}
}
