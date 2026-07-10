// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestActionFromGen_DownloadFile guards DOWNLOAD FILE rendering. Without the
// DownloadFileAction case it renders "-- Empty action". An empty error-handling
// type defaults to Rollback, matching legacy parseDownloadFileAction.
func TestActionFromGen_DownloadFile(t *testing.T) {
	act := decodeAction(t, bson.D{
		{Key: "$ID", Value: "df-1"},
		{Key: "$Type", Value: "Microflows$DownloadFileAction"},
		{Key: "FileDocumentVariableName", Value: "nodePermission"},
		// Storage key is ShowFileInBrowser (legacy's parseDownloadFileAction reads
		// the wrong "ShowInBrowser" key — a latent legacy bug; gen reads it right).
		{Key: "ShowFileInBrowser", Value: true},
	})
	df, ok := act.(*microflows.DownloadFileAction)
	if !ok {
		t.Fatalf("actionFromGen → %T, want *microflows.DownloadFileAction", act)
	}
	if df.FileDocument != "nodePermission" {
		t.Errorf("FileDocument = %q, want nodePermission", df.FileDocument)
	}
	if !df.ShowInBrowser {
		t.Error("ShowInBrowser = false, want true")
	}
	if df.ErrorHandlingType != microflows.ErrorHandlingTypeRollback {
		t.Errorf("ErrorHandlingType = %q, want Rollback default", df.ErrorHandlingType)
	}
}
