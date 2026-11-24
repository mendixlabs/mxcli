// SPDX-License-Identifier: Apache-2.0

package types

import "testing"

// ako/mxcli#2265: an Agent Editor document names what it uses as
// {"documentId", "qualifiedName"} pairs inside its Contents JSON.
func TestRewriteBlobQualifiedNames(t *testing.T) {
	const indented = `{
  "tools": [
    {
      "document": {
        "documentId": "dca6f75b",
        "qualifiedName": "ServiceDesk.Tool_GetOrderStatus"
      }
    },
    {
      "document": {"documentId": "aa", "qualifiedName": "ServiceDesk.Tool_GetOrderStatusV2"}
    }
  ],
  "systemPrompt": "Call ServiceDesk.Tool_GetOrderStatus first",
  "entity": {"documentId":"e1","qualifiedName":"ServiceDesk.Tool_GetOrderStatus.Inner"}
}`
	got, n := RewriteBlobQualifiedNames(indented, "ServiceDesk.Tool_GetOrderStatus", "ServiceDesk.Tool_GetOrderStatus2")
	if n != 2 {
		t.Fatalf("rewrote %d references, want 2 (the exact name and the prefixed one):\n%s", n, got)
	}
	want := `{
  "tools": [
    {
      "document": {
        "documentId": "dca6f75b",
        "qualifiedName": "ServiceDesk.Tool_GetOrderStatus2"
      }
    },
    {
      "document": {"documentId": "aa", "qualifiedName": "ServiceDesk.Tool_GetOrderStatusV2"}
    }
  ],
  "systemPrompt": "Call ServiceDesk.Tool_GetOrderStatus first",
  "entity": {"documentId":"e1","qualifiedName":"ServiceDesk.Tool_GetOrderStatus2.Inner"}
}`
	if got != want {
		t.Errorf("rewrite changed more than the qualified names, or missed one:\n got %s\nwant %s", got, want)
	}

	// A module rename passes the bare module name.
	got, n = RewriteBlobQualifiedNames(`{"model":{"documentId":"m","qualifiedName":"ServiceDesk.GPT"}}`, "ServiceDesk", "Support")
	if n != 1 || got != `{"model":{"documentId":"m","qualifiedName":"Support.GPT"}}` {
		t.Errorf("module rename: got %d %s", n, got)
	}

	// Nothing to rewrite: the input comes back unchanged.
	in := `{"model":{"documentId":"m","qualifiedName":"Other.GPT"}}`
	if got, n := RewriteBlobQualifiedNames(in, "ServiceDesk", "Support"); n != 0 || got != in {
		t.Errorf("unrelated name rewritten: %d %s", n, got)
	}
}
