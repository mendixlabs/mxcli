// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// alterPageActionCtx builds the mock plumbing shared by the tests below and
// returns the statement runner plus a pointer to the action the mutator saw.
func alterPageActionCtx(t *testing.T, mutErr error) (func(*ast.ActionV3) error, *pages.ClientAction) {
	t.Helper()
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "TestPage")
	// The CREATE PAGE builder resolves a microflow action against the backend,
	// so the mock has to hold one — that resolution is the delegation working.
	mf := mkMicroflow(mod.ID, "ACT_Other")
	var got pages.ClientAction

	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc:    func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:      func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return &mock.MockPageMutator{
				SetWidgetActionFunc: func(widgetRef string, action pages.ClientAction) error {
					if widgetRef != "btnSave" {
						t.Errorf("widgetRef = %q, want btnSave", widgetRef)
					}
					got = action
					return mutErr
				},
				SaveFunc: func() error { return nil },
			}, nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)
	withContainer(h, mf.ContainerID, mod.ID)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	return func(action *ast.ActionV3) error {
		return execAlterPage(ctx, &ast.AlterPageStmt{
			PageName: ast.QualifiedName{Module: "MyModule", Name: "TestPage"},
			Operations: []ast.AlterPageOperation{
				&ast.SetPropertyOp{
					Target:     ast.WidgetRef{Widget: "btnSave"},
					Properties: map[string]any{"Action": action},
				},
			},
		})
	}, &got
}

// TestAlterPage_SetAction_RoutesToMutator pins that `SET Action` reaches
// SetWidgetAction rather than being written through SetWidgetProperty as a
// scalar. An action is a polymorphic node; storing it as a value would produce a
// document Studio Pro cannot open.
func TestAlterPage_SetAction_RoutesToMutator(t *testing.T) {
	run, got := alterPageActionCtx(t, nil)
	assertNoError(t, run(&ast.ActionV3{Type: "microflow", Target: "MyModule.ACT_Other"}))

	if *got == nil {
		t.Fatal("SetWidgetAction was not called")
	}
	mf, ok := (*got).(*pages.MicroflowClientAction)
	if !ok {
		t.Fatalf("action type = %T, want *pages.MicroflowClientAction", *got)
	}
	if mf.MicroflowName != "MyModule.ACT_Other" {
		t.Errorf("Microflow = %q, want MyModule.ACT_Other", mf.MicroflowName)
	}
}

// TestAlterPage_SetAction_DelegatesToCreatePageBuilder covers the point of the
// change: SET builds through the CREATE PAGE builder, so every action form works
// here without a second switch to keep in sync. `SAVE_CHANGES CLOSE_PAGE` is the
// interesting one — the close is a flag on the action, not a separate action.
func TestAlterPage_SetAction_DelegatesToCreatePageBuilder(t *testing.T) {
	tests := []struct {
		name   string
		action *ast.ActionV3
		verify func(t *testing.T, a pages.ClientAction)
	}{
		{
			name:   "save and close",
			action: &ast.ActionV3{Type: "save", ClosePage: true},
			verify: func(t *testing.T, a pages.ClientAction) {
				s, ok := a.(*pages.SaveChangesClientAction)
				if !ok {
					t.Fatalf("type = %T", a)
				}
				if !s.ClosePage {
					t.Error("ClosePage = false, want true")
				}
			},
		},
		{
			// The bare form closes, as in Studio Pro (ako/mxcli#2536).
			name:   "bare save closes",
			action: &ast.ActionV3{Type: "save"},
			verify: func(t *testing.T, a pages.ClientAction) {
				s, ok := a.(*pages.SaveChangesClientAction)
				if !ok {
					t.Fatalf("type = %T", a)
				}
				if !s.ClosePage {
					t.Error("ClosePage = false, want true")
				}
			},
		},
		{
			name:   "save without close",
			action: &ast.ActionV3{Type: "save", Settings: &ast.ActionSettingsV3{ClosePage: boolPtr2536(false)}},
			verify: func(t *testing.T, a pages.ClientAction) {
				s, ok := a.(*pages.SaveChangesClientAction)
				if !ok {
					t.Fatalf("type = %T", a)
				}
				if s.ClosePage {
					t.Error("ClosePage = true, want false")
				}
			},
		},
		{
			name:   "close page",
			action: &ast.ActionV3{Type: "close"},
			verify: func(t *testing.T, a pages.ClientAction) {
				if _, ok := a.(*pages.ClosePageClientAction); !ok {
					t.Fatalf("type = %T, want *pages.ClosePageClientAction", a)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run, got := alterPageActionCtx(t, nil)
			assertNoError(t, run(tt.action))
			if *got == nil {
				t.Fatal("SetWidgetAction was not called")
			}
			tt.verify(t, *got)
		})
	}
}

// TestAlterPage_SetAction_RejectsNonAction guards the type assertion: a scalar
// where an action expression belongs must be a clear error, not a panic.
func TestAlterPage_SetAction_RejectsNonAction(t *testing.T) {
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "TestPage")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return &mock.MockPageMutator{SaveFunc: func() error { return nil }}, nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	err := execAlterPage(ctx, &ast.AlterPageStmt{
		PageName: ast.QualifiedName{Module: "MyModule", Name: "TestPage"},
		Operations: []ast.AlterPageOperation{
			&ast.SetPropertyOp{
				Target:     ast.WidgetRef{Widget: "btnSave"},
				Properties: map[string]any{"Action": "not-an-action"},
			},
		},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "must be an action expression")
}

// mendixlabs/mxcli#1317 — `alter page … set ('onClickAction': call microflow
// M.F(P = $P)) on w` built the action with an empty parameter scope, so a page
// parameter argument fell through classifyFlowArgValue and was written as the
// Expression "$P" — the form #1140 showed Studio Pro does not bind (CE1571 on
// opening the page). CREATE PAGE and ALTER PAGE INSERT/REPLACE already seed the
// scope from the stored page; SET must too.
func TestAlterPage_SetNamedAction_PageParamBindsThroughVariable(t *testing.T) {
	for _, tc := range []struct {
		name      string
		container backend.ContainerKind
		wantKind  string
	}{
		{"page", backend.ContainerPage, "parameter"},
		{"snippet", backend.ContainerSnippet, "snippet"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mod := mkModule("LearningJourney")
			pg := mkPage(mod.ID, "LearningJourney_Details")
			mf := mkMicroflow(mod.ID, "ACT_ExcludeItemStep")
			var got pages.ClientAction

			mb := &mock.MockBackend{
				IsConnectedFunc:    func() bool { return true },
				ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
				ListFoldersFunc:    func() ([]*types.FolderInfo, error) { return nil, nil },
				ListPagesFunc:      func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
				ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
				OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
					return &mock.MockPageMutator{
						ContainerTypeFunc: func() backend.ContainerKind { return tc.container },
						ParamScopeFunc: func() (map[string]model.ID, map[string]string) {
							return map[string]model.ID{"LearningJourney": "p1"},
								map[string]string{"LearningJourney": "LearningJourney.LearningJourney"}
						},
						SetWidgetNamedActionFunc: func(widgetRef, key string, action pages.ClientAction) error {
							got = action
							return nil
						},
						SaveFunc: func() error { return nil },
					}, nil
				},
			}
			h := mkHierarchy(mod)
			withContainer(h, pg.ContainerID, mod.ID)
			withContainer(h, mf.ContainerID, mod.ID)
			ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

			assertNoError(t, execAlterPage(ctx, &ast.AlterPageStmt{
				PageName: ast.QualifiedName{Module: "LearningJourney", Name: "LearningJourney_Details"},
				Operations: []ast.AlterPageOperation{
					&ast.SetPropertyOp{
						Target: ast.WidgetRef{Widget: "pDSLink_ButtonAndTracking8"},
						Properties: map[string]any{"onClickAction": &ast.ActionV3{
							Type:   "microflow",
							Target: "LearningJourney.ACT_ExcludeItemStep",
							Args:   []ast.FlowArgV3{{Name: "LearningJourney", Value: "$LearningJourney"}},
						}},
					},
				},
			}))

			mfa, ok := got.(*pages.MicroflowClientAction)
			if !ok {
				t.Fatalf("action = %T, want *pages.MicroflowClientAction", got)
			}
			if len(mfa.ParameterMappings) != 1 {
				t.Fatalf("mappings = %d, want 1", len(mfa.ParameterMappings))
			}
			pm := mfa.ParameterMappings[0]
			if pm.VariableKind != tc.wantKind || pm.Variable != "$LearningJourney" {
				t.Errorf("mapping = Variable %q kind %q, want $LearningJourney kind %q — "+
					"an unclassified $-argument is written as an Expression Studio Pro does not bind",
					pm.Variable, pm.VariableKind, tc.wantKind)
			}
		})
	}
}

// The INSERT/REPLACE half of #1317's scope fix. buildWidgetsFromAST and
// buildColumnSpecsFromAST seeded the parameter scope but never isSnippet, so a
// button inserted into a snippet bound its `$Order` argument as a PageParameter.
// mxbuild 11.12.2 reports it as CE0115 ("arguments … do not match the expected
// parameters") on the inserted button only; CREATE SNIPPET and ALTER SET bind
// the same argument as SnippetParameter.
func TestAlterSnippet_InsertedFlowArgBindsSnippetParameter(t *testing.T) {
	mod := mkModule("SN")
	mf := mkMicroflow(mod.ID, "ACT_Use")
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
	}
	h := mkHierarchy(mod)
	withContainer(h, mf.ContainerID, mod.ID)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	for _, tc := range []struct {
		container backend.ContainerKind
		wantKind  string
	}{
		{backend.ContainerSnippet, "snippet"},
		{backend.ContainerPage, "parameter"}, // control: a page keeps PageParameter
	} {
		t.Run(string(tc.container), func(t *testing.T) {
			mut := &mock.MockPageMutator{
				ContainerTypeFunc: func() backend.ContainerKind { return tc.container },
				ParamScopeFunc: func() (map[string]model.ID, map[string]string) {
					return map[string]model.ID{"Order": "p1"}, map[string]string{"Order": "SN.Order"}
				},
				WidgetScopeFunc: func() map[string]model.ID { return map[string]model.ID{} },
			}
			btn := &ast.WidgetV3{Type: "actionbutton", Name: "btnInsert", Properties: map[string]any{
				"Caption": "Insert",
				"Action": &ast.ActionV3{Type: "microflow", Target: "SN.ACT_Use",
					Args: []ast.FlowArgV3{{Name: "Order", Value: "$Order"}}},
			}}
			ws, err := buildWidgetsFromAST(ctx, []*ast.WidgetV3{btn}, "SN", mod.ID, "", mut)
			assertNoError(t, err)
			if len(ws) != 1 {
				t.Fatalf("widgets = %d, want 1", len(ws))
			}
			b, ok := ws[0].(*pages.ActionButton)
			if !ok {
				t.Fatalf("widget = %T, want *pages.ActionButton", ws[0])
			}
			mfa, ok := b.Action.(*pages.MicroflowClientAction)
			if !ok || len(mfa.ParameterMappings) != 1 {
				t.Fatalf("action = %#v, want one microflow mapping", b.Action)
			}
			if got := mfa.ParameterMappings[0].VariableKind; got != tc.wantKind {
				t.Errorf("VariableKind = %q, want %q — mxbuild reports CE0115 on the button", got, tc.wantKind)
			}
		})
	}
}

func boolPtr2536(b bool) *bool { return &b }
