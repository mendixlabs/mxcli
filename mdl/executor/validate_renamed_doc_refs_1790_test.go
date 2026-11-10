// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#2070 (mendixlabs/mxcli#1790): `rename page M.Old to New;` followed
// in the same script by `show page M.New` was refused by check --references
// ("page not found: M.New"), while exec ran it: the rename was recorded only
// where the alter-target check looks, not in the script's page set every
// reference check consults.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func TestValidate_ReferenceToADocumentRenamedEarlierInTheScript(t *testing.T) {
	mod := &model.Module{Name: "M"}
	mod.ID = "mod1"
	other := &model.Module{Name: "N"}
	other.ID = "mod2"
	stored := &pages.Page{Name: "Old_Page"}
	stored.ContainerID = "mod1"
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod, other}, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{stored}, nil },
	}
	showPage := func(module, page string) ast.MicroflowStatement {
		return &ast.ShowPageStmt{PageName: qn(module, page)}
	}
	cases := []struct {
		name    string
		before  []ast.Statement
		call    ast.MicroflowStatement
		wantErr string
	}{
		{"the reported script: rename page, then show it by its new name",
			[]ast.Statement{&ast.RenameStmt{ObjectType: "page", Name: qn("M", "Old_Page"), NewName: "New_Page"}},
			showPage("M", "New_Page"), ""},
		{"a page moved to another module, then shown there",
			[]ast.Statement{&ast.MoveStmt{DocumentType: ast.DocumentTypePage, Name: qn("M", "Old_Page"), TargetModule: "N"}},
			showPage("N", "Old_Page"), ""},
		{"control: without the rename the new name is not found",
			nil, showPage("M", "New_Page"), "page not found: M.New_Page"},
		{"control: a dry-run rename provides nothing",
			[]ast.Statement{&ast.RenameStmt{ObjectType: "page", Name: qn("M", "Old_Page"), NewName: "New_Page", DryRun: true}},
			showPage("M", "New_Page"), "page not found: M.New_Page"},
		{"control: a renamed microflow does not provide a page",
			[]ast.Statement{&ast.RenameStmt{ObjectType: "microflow", Name: qn("M", "Old_Page"), NewName: "New_Page"}},
			showPage("M", "New_Page"), "page not found: M.New_Page"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := newMockCtx(t, withBackend(mb))
			body := []ast.MicroflowStatement{tc.call}
			stmts := append(append([]ast.Statement{}, tc.before...), &ast.CreateMicroflowStmt{Name: qn("M", "Caller"), Body: body})
			sc := newScriptContext()
			sc.collectDefinitions(&ast.Program{Statements: stmts})
			errs := validateFlowBodyReferences(ctx, body, sc)
			if tc.wantErr == "" {
				if len(errs) != 0 {
					t.Fatalf("refused: %v", errs)
				}
				return
			}
			if len(errs) != 1 || !strings.Contains(errs[0], tc.wantErr) {
				t.Fatalf("errs = %v, want one containing %q", errs, tc.wantErr)
			}
		})
	}
}

// Every kind markIndirectDoc names must have its set allocated, or a rename of
// that kind panics the check.
func TestMarkIndirectDocCoversEveryKindWithoutPanicking(t *testing.T) {
	sc := newScriptContext()
	for _, k := range []string{"page", "snippet", "layout", "microflow", "nanoflow", "enumeration", "constant", "workflow", "javaaction", ""} {
		sc.markIndirectDoc(k, "M.X")
	}
	if !sc.pages["M.X"] || !sc.workflows["M.X"] || !sc.indirectDocs["javaaction:M.X"] {
		t.Fatal("kinds not recorded")
	}
}
