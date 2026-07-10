// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func TestBuildFlowGraph_DownloadFileCreatesRealAction(t *testing.T) {
	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		varTypes:     map[string]string{"GeneratedReport": "System.FileDocument"},
		declaredVars: map[string]string{"GeneratedReport": "System.FileDocument"},
	}

	fb.buildFlowGraph([]ast.MicroflowStatement{
		&ast.DownloadFileStmt{
			FileDocument:  "GeneratedReport",
			ShowInBrowser: true,
			ErrorHandling: &ast.ErrorHandlingClause{Type: ast.ErrorHandlingContinue},
		},
	}, nil)

	var action *microflows.DownloadFileAction
	for _, obj := range fb.objects {
		activity, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		if dl, ok := activity.Action.(*microflows.DownloadFileAction); ok {
			action = dl
			break
		}
	}
	if action == nil {
		t.Fatal("expected DownloadFileAction")
	}
	if action.FileDocument != "GeneratedReport" {
		t.Fatalf("FileDocument = %q, want GeneratedReport", action.FileDocument)
	}
	if !action.ShowInBrowser {
		t.Fatal("ShowInBrowser = false, want true")
	}
	if action.ErrorHandlingType != microflows.ErrorHandlingTypeContinue {
		t.Fatalf("ErrorHandlingType = %q, want Continue", action.ErrorHandlingType)
	}
}
