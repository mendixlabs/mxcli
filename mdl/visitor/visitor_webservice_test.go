// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestCallWebServiceStatement(t *testing.T) {
	stmt := firstStatement(t, `$Root = call web service SampleSOAP.OrderService
operation FetchSampleItems
send mapping SampleSOAP.OrderRequest
receive mapping SampleSOAP.OrderResponse
timeout 30
on error rollback;`)

	call, ok := stmt.(*ast.CallWebServiceStmt)
	if !ok {
		t.Fatalf("expected CallWebServiceStmt, got %T", stmt)
	}
	if call.OutputVariable != "Root" {
		t.Errorf("OutputVariable = %q, want Root", call.OutputVariable)
	}
	if call.ServiceID != "SampleSOAP.OrderService" {
		t.Errorf("ServiceID = %q", call.ServiceID)
	}
	if call.OperationName != "FetchSampleItems" {
		t.Errorf("OperationName = %q", call.OperationName)
	}
	if call.SendMappingID != "SampleSOAP.OrderRequest" {
		t.Errorf("SendMappingID = %q", call.SendMappingID)
	}
	if call.ReceiveMappingID != "SampleSOAP.OrderResponse" {
		t.Errorf("ReceiveMappingID = %q", call.ReceiveMappingID)
	}
	if call.Timeout == nil {
		t.Fatal("expected Timeout expression")
	}
	if call.ErrorHandling == nil || call.ErrorHandling.Type != ast.ErrorHandlingRollback {
		t.Fatalf("ErrorHandling = %#v, want rollback", call.ErrorHandling)
	}
}

func TestCallWebServiceStatementQuotedFallbackRefs(t *testing.T) {
	stmt := firstStatement(t, `$Root = call web service 'sample-service-id'
operation 'FetchSampleItems'
send mapping 'sample-send-mapping-id'
receive mapping 'sample-receive-mapping-id';`)

	call, ok := stmt.(*ast.CallWebServiceStmt)
	if !ok {
		t.Fatalf("expected CallWebServiceStmt, got %T", stmt)
	}
	if call.ServiceID != "sample-service-id" {
		t.Errorf("ServiceID = %q", call.ServiceID)
	}
	if call.OperationName != "FetchSampleItems" {
		t.Errorf("OperationName = %q", call.OperationName)
	}
	if call.SendMappingID != "sample-send-mapping-id" {
		t.Errorf("SendMappingID = %q", call.SendMappingID)
	}
	if call.ReceiveMappingID != "sample-receive-mapping-id" {
		t.Errorf("ReceiveMappingID = %q", call.ReceiveMappingID)
	}
}

func TestCallWebServiceRawStatement(t *testing.T) {
	stmt := firstStatement(t, `$Root = call web service raw 'AQID';`)

	call, ok := stmt.(*ast.CallWebServiceStmt)
	if !ok {
		t.Fatalf("expected CallWebServiceStmt, got %T", stmt)
	}
	if call.OutputVariable != "Root" {
		t.Errorf("OutputVariable = %q, want Root", call.OutputVariable)
	}
	if call.RawBSONBase64 != "AQID" {
		t.Errorf("RawBSONBase64 = %q, want AQID", call.RawBSONBase64)
	}
	if call.ServiceID != "" || call.OperationName != "" {
		t.Errorf("raw statement should not set structured refs: %#v", call)
	}
}
