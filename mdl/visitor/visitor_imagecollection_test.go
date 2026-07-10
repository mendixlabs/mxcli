// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestCreateImageCollection(t *testing.T) {
	input := `CREATE IMAGE COLLECTION MyModule.Icons EXPORT LEVEL 'Public' (
		IMAGE MyIcon FROM FILE '/images/icon.png'
	);`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.CreateImageCollectionStmt)
	if !ok {
		t.Fatalf("Expected CreateImageCollectionStmt, got %T", prog.Statements[0])
	}
	if stmt.Name.Name != "Icons" {
		t.Errorf("Got name %s", stmt.Name.Name)
	}
	if stmt.ExportLevel != "Public" {
		t.Errorf("Got ExportLevel %q", stmt.ExportLevel)
	}
	if len(stmt.Images) != 1 {
		t.Fatalf("Expected 1 image, got %d", len(stmt.Images))
	}
	if stmt.Images[0].Name != "MyIcon" {
		t.Errorf("Got image name %q", stmt.Images[0].Name)
	}
	if stmt.Images[0].FilePath != "/images/icon.png" {
		t.Errorf("Got FilePath %q", stmt.Images[0].FilePath)
	}
}
