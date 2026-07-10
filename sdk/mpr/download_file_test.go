// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/bson"
)

func TestDownloadFileAction_Roundtrip(t *testing.T) {
	action := &microflows.DownloadFileAction{
		BaseElement:       model.BaseElement{ID: "download-action-id"},
		ErrorHandlingType: microflows.ErrorHandlingTypeContinue,
		FileDocument:      "GeneratedReport",
		ShowInBrowser:     true,
	}

	doc := serializeMicroflowAction(action)
	data, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("failed to marshal BSON: %v", err)
	}

	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal BSON: %v", err)
	}

	parsed := parseDownloadFileAction(raw)
	if parsed.ErrorHandlingType != microflows.ErrorHandlingTypeContinue {
		t.Fatalf("ErrorHandlingType = %q, want Continue", parsed.ErrorHandlingType)
	}
	if parsed.FileDocument != "GeneratedReport" {
		t.Fatalf("FileDocument = %q, want GeneratedReport", parsed.FileDocument)
	}
	if !parsed.ShowInBrowser {
		t.Fatal("ShowInBrowser = false, want true")
	}
}

func TestParseDownloadFileAction_DefaultsErrorHandlingToRollback(t *testing.T) {
	action := parseDownloadFileAction(map[string]any{
		"$ID":                      "download-action-id",
		"FileDocumentVariableName": "GeneratedReport",
		"ShowInBrowser":            false,
	})

	if action.ErrorHandlingType != microflows.ErrorHandlingTypeRollback {
		t.Fatalf("ErrorHandlingType = %q, want Rollback", action.ErrorHandlingType)
	}
}
