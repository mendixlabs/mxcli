// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#2353: `describe contract operation from openapi` showed
// `Query: ($limit: Integer)` while the imported service described back as
// `Query: ($limit: String)`. Rest$QueryParameter has no DataType — a consumed
// REST query parameter is text in the model — so the import cannot keep the
// spec's type, and the preview must show what the import writes.
const queryAndPathSpec = `{
  "openapi": "3.0.0",
  "info": { "title": "Pets", "version": "1.0.0" },
  "servers": [{ "url": "https://api.example.com" }],
  "paths": {
    "/pets": { "get": { "operationId": "listPets",
      "parameters": [ { "name": "limit", "in": "query", "schema": { "type": "integer" } } ],
      "responses": { "200": { "description": "OK" } } } },
    "/pets/{petId}": { "get": { "operationId": "getPet",
      "parameters": [ { "name": "petId", "in": "path", "required": true, "schema": { "type": "integer" } } ],
      "responses": { "200": { "description": "OK" } } } }
  }
}`

func TestDescribeContractFromOpenAPI_QueryTypeIsWhatTheImportWrites_Issue2353(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(queryAndPathSpec))
	}))
	defer srv.Close()

	ctx, buf := newMockCtx(t)
	assertNoError(t, describeContractFromOpenAPI(ctx, &ast.DescribeContractFromOpenAPIStmt{SpecPath: srv.URL}))
	out := buf.String()
	if !strings.Contains(out, "$limit: String") {
		t.Errorf("preview does not show the query parameter as the import stores it (String):\n%s", out)
	}
	// CONTROL: a path parameter has a stored DataType and keeps the spec's.
	if !strings.Contains(out, "$petId: Integer") {
		t.Errorf("path parameter lost its type:\n%s", out)
	}
}
