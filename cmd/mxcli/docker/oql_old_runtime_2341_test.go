// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ako/mxcli#2341 (mendixlabs/mxcli#1879): a Mendix 9.24 runtime has no
// preview_execute_oql admin action, so `mxcli oql` answered "Action not found
// -- … needs the live-preview dev flags at boot …", sending the user to
// restart with flags that cannot help: the action does not exist in 9.x. When
// both routes are missing, the runtime is asked its version, and a runtime
// before Mendix 10 is named as the reason.

func oqlServerWithVersion(t *testing.T, version string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Action string `json:"action"`
		}
		_ = json.Unmarshal(body, &req)
		if req.Action == "about" {
			_ = json.NewEncoder(w).Encode(map[string]any{"result": 0,
				"feedback": map[string]any{"name": "Mendix Business Server", "version": version}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": -5, "message": "Action not found."})
	}))
}

func TestExecuteOQL_Mendix9RuntimeSaysSo_Issue2341(t *testing.T) {
	server := oqlServerWithVersion(t, "9.24.11.19193")
	defer server.Close()
	host, port := parseTestServerAddr(t, server.URL)
	_, err := ExecuteOQL(OQLOptions{Host: host, Port: port, Token: "x", Direct: true}, "SELECT COUNT(*) AS n FROM System.Session")
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	for _, want := range []string{"9.24.11", "Mendix 10"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error lacks %q:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "dev flags") || strings.Contains(msg, "upgrade mxcli") {
		t.Errorf("error still sends the user to restart with flags:\n%s", msg)
	}
}

// CONTROL: on a Mendix 10+ runtime the action missing does mean the
// live-preview flags are off, and the message keeps saying so.
func TestExecuteOQL_Mendix10WithoutFlagsKeepsTheHint_Issue2341(t *testing.T) {
	server := oqlServerWithVersion(t, "10.24.28.129343")
	defer server.Close()
	host, port := parseTestServerAddr(t, server.URL)
	_, err := ExecuteOQL(OQLOptions{Host: host, Port: port, Token: "x", Direct: true}, "SELECT 1")
	if err == nil || !strings.Contains(err.Error(), "live-preview dev flags") {
		t.Errorf("error = %v, want the dev-flags hint", err)
	}
}
