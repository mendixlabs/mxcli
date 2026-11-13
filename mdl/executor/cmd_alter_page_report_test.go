// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// alterPageReportRun executes one ALTER PAGE/SNIPPET against a backend that
// keeps write statistics; landed says whether the mutator's Save reaches disk
// (true) or is elided as a no-op by storage (false), as ADR-0008 does when the
// alter has already been applied.
func alterPageReportRun(t *testing.T, containerType string, landed bool) (string, bool) {
	t.Helper()
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "TestPage")
	snp := mkSnippet(mod.ID, "TestSnippet")
	saved := false
	cb := &countingBackend{}
	cb.MockBackend = &mock.MockBackend{
		IsConnectedFunc:  func() bool { return true },
		ListModulesFunc:  func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc:  func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:    func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		ListSnippetsFunc: func() ([]*pages.Snippet, error) { return []*pages.Snippet{snp}, nil },
		OpenPageForMutationFunc: func(model.ID) (backend.PageMutator, error) {
			return &mock.MockPageMutator{
				SetWidgetPropertyFunc: func(string, string, any) error { return nil },
				SaveFunc: func() error {
					saved = true
					if landed {
						cb.offer(1, 1)
					} else {
						cb.offer(1, 0)
					}
					return nil
				},
			}, nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)
	withContainer(h, snp.ContainerID, mod.ID)
	var buf bytes.Buffer
	ctx := &ExecContext{
		Context: context.Background(),
		Backend: cb,
		Output:  &buf,
		Format:  FormatTable,
		Cache:   &executorCache{hierarchy: h},
	}
	name := "TestPage"
	if containerType == "snippet" {
		name = "TestSnippet"
	}
	assertNoError(t, execAlterPage(ctx, &ast.AlterPageStmt{
		ContainerType: containerType,
		PageName:      ast.QualifiedName{Module: "MyModule", Name: name},
		Operations: []ast.AlterPageOperation{
			&ast.SetPropertyOp{
				Target:     ast.WidgetRef{Widget: "myWidget"},
				Properties: map[string]any{"Caption": "Hello"},
			},
		},
	}))
	return buf.String(), saved
}

// TestAlterPageReportsUnchangedWhenTheWriteWasElided is ako/mxcli#2156:
// re-running an already-applied `alter snippet … { replace … }` printed
// "Altered snippet" while storage elided the write — every unit byte-identical,
// no mtime moved — so the output could not prove the "second exec writes
// nothing" gate the rest of the script passed with "Unchanged …".
func TestAlterPageReportsUnchangedWhenTheWriteWasElided(t *testing.T) {
	for _, tc := range []struct{ kind, want string }{
		{"snippet", "Unchanged snippet MyModule.TestSnippet\n"},
		{"page", "Unchanged page MyModule.TestPage\n"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			got, saved := alterPageReportRun(t, tc.kind, false)
			if !saved {
				t.Fatal("expected Save to be offered — the elision is storage's call, not the handler's")
			}
			if got != tc.want {
				t.Errorf("reported %q, want %q", got, tc.want)
			}
		})
	}
}

// TestAlterPageReportsAlteredWhenTheWriteLanded is the control (and the guard
// against #1446's direction): a write that reached disk still says "Altered".
func TestAlterPageReportsAlteredWhenTheWriteLanded(t *testing.T) {
	for _, tc := range []struct{ kind, want string }{
		{"snippet", "Altered snippet MyModule.TestSnippet\n"},
		{"page", "Altered page MyModule.TestPage\n"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			got, saved := alterPageReportRun(t, tc.kind, true)
			if !saved {
				t.Fatal("expected Save to be called")
			}
			if got != tc.want {
				t.Errorf("reported %q, want %q", got, tc.want)
			}
		})
	}
}
