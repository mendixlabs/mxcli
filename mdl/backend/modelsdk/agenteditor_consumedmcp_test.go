// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/agenteditor"
)

// Consumed MCP services saved by Studio Pro 11.16 (mx-test-projects
// FactoryManagement): GraphStudio_MCP has an endpoint constant and a
// credentials microflow, JiraMCP only an endpoint, stored first.
const (
	graphStudioMCPFixture = "testdata/agenteditor/FactoryManagement.GraphStudio_MCP.11_16.json"
	jiraMCPFixture        = "testdata/agenteditor/FactoryManagement.JiraMCP.11_16.json"
)

func typedConsumedMCP(t *testing.T, contents string) *agenteditor.ConsumedMCPService {
	t.Helper()
	var p struct {
		ProtocolVersion          string              `json:"protocolVersion"`
		Documentation            string              `json:"documentation"`
		Version                  string              `json:"version"`
		ConnectionTimeoutSeconds int                 `json:"connectionTimeoutSeconds"`
		Endpoint                 *agenteditor.DocRef `json:"endpoint"`
		AuthenticationMicroflow  *agenteditor.DocRef `json:"authenticationMicroflow"`
	}
	if err := json.Unmarshal([]byte(contents), &p); err != nil {
		t.Fatal(err)
	}
	return &agenteditor.ConsumedMCPService{ProtocolVersion: p.ProtocolVersion, InnerDocumentation: p.Documentation,
		Version: p.Version, ConnectionTimeoutSeconds: p.ConnectionTimeoutSeconds,
		Endpoint: p.Endpoint, AuthenticationMicroflow: p.AuthenticationMicroflow}
}

// ako/mxcli#2060: mxcli wrote no endpoint and no credentials microflow, so an
// mxcli-written service connected nowhere (AgentEditorCommons imports it only
// when the endpoint constant has a value). Both are modelled now; an unchanged
// service rebuilds byte for byte in either stored key order.
func TestEncodeConsumedMCPServiceContents_EndpointAndCredentials(t *testing.T) {
	for _, path := range []string{graphStudioMCPFixture, jiraMCPFixture} {
		stored := readContentsFixture(t, path)
		c := typedConsumedMCP(t, stored)
		rebuilt, err := encodeConsumedMCPServiceContents(c)
		if err != nil {
			t.Fatal(err)
		}
		if m := decodeMap(t, rebuilt); m["endpoint"] == nil {
			t.Errorf("%s: the encoder does not write endpoint: %s", path, rebuilt)
		}
		got, err := carryStoredContents(stored, rebuilt, consumedMCPServiceContentsSpec)
		if err != nil {
			t.Fatal(err)
		}
		if got != stored {
			t.Errorf("%s: an unchanged service is rewritten:\n got %s\nwant %s", path, got, stored)
		}
	}

	// A new service, with no stored document: endpoint first, the credentials
	// microflow last, each reference documentId first, as Agent Editor saves them.
	c := &agenteditor.ConsumedMCPService{ProtocolVersion: "v2025_03_26", Version: "1.0.0",
		Endpoint:                &agenteditor.DocRef{DocumentID: "c1", QualifiedName: "M.Url"},
		AuthenticationMicroflow: &agenteditor.DocRef{DocumentID: "m1", QualifiedName: "M.GetCredentials"}}
	fresh, err := encodeConsumedMCPServiceContents(c)
	if err != nil {
		t.Fatal(err)
	}
	keys := decodeObjectKeys(t, fresh)
	if keys[0] != "endpoint" || keys[len(keys)-1] != "authenticationMicroflow" {
		t.Errorf("new service key order %v", keys)
	}
	ep, _ := decodeObject(json.RawMessage(rawKey(t, fresh, "endpoint")))
	if !reflect.DeepEqual(ep.keys, []string{"documentId", "qualifiedName"}) {
		t.Errorf("endpoint key order %v", ep.keys)
	}

	// Clearing the credentials microflow removes it from a stored service.
	stored := readContentsFixture(t, graphStudioMCPFixture)
	c = typedConsumedMCP(t, stored)
	c.AuthenticationMicroflow = nil
	rebuilt, _ := encodeConsumedMCPServiceContents(c)
	got, err := carryStoredContents(stored, rebuilt, consumedMCPServiceContentsSpec)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := decodeMap(t, got)["authenticationMicroflow"]; ok {
		t.Errorf("a cleared credentials microflow came back from the stored contents: %s", got)
	}
}

func rawKey(t *testing.T, s, key string) string {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatal(err)
	}
	return string(m[key])
}
