// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/agenteditor"
)

// Saved by Studio Pro 11.16 with Agent Editor 2.6.0 and MCP Server 5.4.0
// (mx-test-projects/AgentBuilderStarterApp, AgentTest.Published_MCP_service).
const publishedMCPServiceFixture = "testdata/agenteditor/AgentBuilderStarterApp.Published_MCP_service.11_16.json"

// Its Studio Pro duplicate with authentication on: serverName is still the
// original's, and authenticationMicroflow comes last, documentId first.
const publishedMCPServiceAuthFixture = "testdata/agenteditor/AgentBuilderStarterApp.Published_MCP_service_2.11_16.json"

func typedPublishedMCPService(t *testing.T, contents string) *agenteditor.PublishedMCPService {
	t.Helper()
	p := &agenteditor.PublishedMCPService{}
	if err := decodePublishedMCPServiceContents(contents, p); err != nil {
		t.Fatal(err)
	}
	return p
}

// ako/mxcli#2020: describe -> exec of an unchanged published MCP service writes
// the bytes Studio Pro saved.
func TestPublishedMCPService_RewriteIsByteIdentical(t *testing.T) {
	stored := readContentsFixture(t, publishedMCPServiceFixture)
	p := typedPublishedMCPService(t, stored)
	if p.ServerName != "Published_MCP_service" || len(p.Tools) != 1 || p.Tools[0].SourceMicroflow == nil ||
		p.Tools[0].SourceMicroflow.QualifiedName != "AgentTest.mcptool1" || p.Tools[0].Title != "ewdwe" {
		t.Fatalf("decoded %+v", p)
	}
	rebuilt, err := encodePublishedMCPServiceContents(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := carryStoredContents(stored, rebuilt, publishedMCPServiceContentsSpec)
	if err != nil {
		t.Fatal(err)
	}
	if got != stored {
		t.Errorf("rewrite differs from the stored contents:\n got %s\nwant %s", got, stored)
	}
}

// The authenticated duplicate rebuilds byte for byte too, and a fresh encode of
// it (no stored document to carry from) already has Agent Editor's key order.
func TestPublishedMCPService_AuthenticatedRewriteIsByteIdentical(t *testing.T) {
	stored := readContentsFixture(t, publishedMCPServiceAuthFixture)
	p := typedPublishedMCPService(t, stored)
	if !p.AuthenticationEnabled || p.AuthenticationMicroflow == nil || p.AuthenticationMicroflow.QualifiedName != "AgentTest.auth_mf" ||
		p.AuthenticationMicroflow.DocumentID != "1987e8e9-32bc-403b-b87c-7f95661a3d57" || p.ServerName != "Published_MCP_service" {
		t.Fatalf("decoded %+v", p)
	}
	rebuilt, err := encodePublishedMCPServiceContents(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := carryStoredContents(stored, rebuilt, publishedMCPServiceContentsSpec)
	if err != nil {
		t.Fatal(err)
	}
	if got != stored {
		t.Errorf("rewrite differs from the stored contents:\n got %s\nwant %s", got, stored)
	}
	fresh, err := formatLikeStored([]byte(rebuilt), "{\n")
	if err != nil {
		t.Fatal(err)
	}
	if fresh != stored {
		t.Errorf("a new document is not in Agent Editor's layout:\n got %s\nwant %s", fresh, stored)
	}
}

// A key Agent Editor adds later, on the service or on a tool, survives a
// rewrite; tools pair with the stored entry by uuid (they have no id).
func TestPublishedMCPService_RewriteKeepsUnmodelledKeys(t *testing.T) {
	stored := readContentsFixture(t, publishedMCPServiceFixture)
	stored = strings.Replace(stored, `"title": "ewdwe"`, `"title": "ewdwe",
      "futureToolKey": true`, 1)
	stored = strings.Replace(stored, `"version": "1.0.0",`, `"version": "1.0.0",
  "futureKey": "x",`, 1)
	p := typedPublishedMCPService(t, stored)
	p.Version = "2.0.0"

	rebuilt, err := encodePublishedMCPServiceContents(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := carryStoredContents(stored, rebuilt, publishedMCPServiceContentsSpec)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(stored, `"version": "1.0.0"`, `"version": "2.0.0"`, 1)
	if got != want {
		t.Errorf("rewrite lost or moved a key:\n got %s\nwant %s", got, want)
	}
}

// A new service is written in Agent Editor 2.6's layout, with the
// authentication keys only when they are set.
func TestPublishedMCPService_EncodeNew(t *testing.T) {
	p := &agenteditor.PublishedMCPService{
		ServerName: "Svc", EndpointPath: "svc", Version: "1.0.0", ProtocolVersion: "v2025_03_26",
		Tools: []agenteditor.PublishedMCPTool{{UUID: "u1", Name: "t1", Description: "d",
			SourceMicroflow: &agenteditor.DocRef{DocumentID: "mf", QualifiedName: "M.Tool"}}},
	}
	raw, err := encodePublishedMCPServiceContents(p)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"authenticationMicroflow", "authorizationServerUrl", "scopes"} {
		if _, ok := m[k]; ok {
			t.Errorf("%s written for a service without authentication: %s", k, raw)
		}
	}
	if !strings.Contains(raw, `"sourceMicroflow":{"qualifiedName":"M.Tool","documentId":"mf"}`) {
		t.Errorf("sourceMicroflow not in Agent Editor's key order: %s", raw)
	}
	if m["authenticationEnabled"] != false {
		t.Errorf("authenticationEnabled = %v, want false", m["authenticationEnabled"])
	}
	p.AuthenticationEnabled = true
	p.AuthenticationMicroflow = &agenteditor.DocRef{DocumentID: "a", QualifiedName: "M.Auth"}
	p.AuthorizationServerURL = "https://login.example.com"
	p.Scopes = "mcp.read"
	raw, _ = encodePublishedMCPServiceContents(p)
	for _, k := range []string{`"documentation":"","authenticationMicroflow":{"documentId":"a","qualifiedName":"M.Auth"},"authorizationServerUrl":"https://login.example.com","scopes":"mcp.read"}`, `"authorizationServerUrl":"https://login.example.com"`, `"scopes":"mcp.read"`} {
		if !strings.Contains(raw, k) {
			t.Errorf("missing %s in %s", k, raw)
		}
	}
}
