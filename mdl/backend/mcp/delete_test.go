// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDeleteDocument_NativeWhenAdvertised(t *testing.T) {
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		if name == "ped_delete_document" {
			return "SUCCESS: Deleting documents (1)", false
		}
		return "unexpected", true
	})
	f.tools = map[string][]string{"ped_delete_document": {"documents"}}
	b := &Backend{client: f.connectClient(t)}

	if err := b.deleteDocument(constantDocType, "MyFirstModule", "Gone"); err != nil {
		t.Fatalf("deleteDocument: %v", err)
	}
	call, ok := f.callByName("ped_delete_document")
	if !ok {
		t.Fatal("ped_delete_document was not called")
	}
	raw, _ := json.Marshal(call.Args["documents"])
	want := `[{"documentName":"Gone","documentType":"Constants$Constant","moduleName":"MyFirstModule"}]`
	if string(raw) != want {
		t.Errorf("documents = %s, want %s", raw, want)
	}
}

// The success text of ped_delete_document has not been observed live (the tool
// failed on every 11.16.0 attempt), so an unrecognised reply is settled by
// looking: the delete succeeded iff the document is gone.
func TestDeleteDocument_UnrecognisedReplyVerifiedByLookup(t *testing.T) {
	for _, tc := range []struct {
		name    string
		found   string
		wantErr bool
	}{
		{"gone", `{"foundDocuments":[]}`, false},
		{"still there", `{"foundDocuments":[{"qualifiedName":"MyFirstModule.Gone"}]}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
				if name == "ped_find_document" {
					return tc.found, false
				}
				return "Deleted 1 document.", false
			})
			f.tools = map[string][]string{"ped_delete_document": {"documents"}}
			b := &Backend{client: f.connectClient(t)}
			err := b.deleteDocument(constantDocType, "MyFirstModule", "Gone")
			if (err != nil) != tc.wantErr {
				t.Fatalf("deleteDocument err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestDeleteDocument_NativeFailureSurfaces(t *testing.T) {
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		if name == "ped_find_document" {
			return `{"foundDocuments":[{"qualifiedName":"MyFirstModule.Gone"}]}`, false
		}
		return handlerSessionError, false
	})
	f.tools = map[string][]string{"ped_delete_document": {"documents"}}
	b := &Backend{client: f.connectClient(t)}

	err := b.deleteDocument(constantDocType, "MyFirstModule", "Gone")
	if err == nil {
		t.Fatal("a failed native delete must not report success")
	}
	if !strings.Contains(err.Error(), "Session 'mcp-server' not found") {
		t.Errorf("error should carry Studio Pro's message, got: %v", err)
	}
}

func TestDeleteDocument_NoNativeToolNeeds1116(t *testing.T) {
	f := newFakePED(t, nil)
	f.tools = map[string][]string{"ped_create_document": {"documents"}} // pre-11.16 surface
	b := &Backend{client: f.connectClient(t)}

	err := b.deleteDocument(constantDocType, "MyFirstModule", "Gone")
	if err == nil || !strings.Contains(err.Error(), "needs Studio Pro 11.16+ (ped_delete_document)") {
		t.Fatalf("without ped_delete_document the error should name the version it needs, got: %v", err)
	}
	if _, ok := f.callByName("ped_delete_document"); ok {
		t.Error("ped_delete_document must not be called when the server does not advertise it")
	}
}
