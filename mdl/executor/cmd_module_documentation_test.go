// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1314: since #1269 lint reads
// modules().domain_model_documentation and project_security().admin_user_name,
// and MDL could write neither — every MDL-built module had empty documentation
// and the admin was always MxAdmin, so the rules over them had no negative case.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/security"
)

type dmDocCapture struct {
	calls []string
	dmID  model.ID
}

// moduleDocBackend serves one module, existing or not, whose domain model has
// the given documentation, and records SetDomainModelDocumentation calls.
func moduleDocBackend(existing bool, stored string) (*mock.MockBackend, *dmDocCapture) {
	cap := &dmDocCapture{}
	mod := mkModule("Sales")
	dm := &domainmodel.DomainModel{BaseElement: model.BaseElement{ID: "dm-sales"},
		ContainerID: mod.ID, Documentation: stored}
	mods := []*model.Module{}
	if existing {
		mods = append(mods, mod)
	}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return mods, nil },
		CreateModuleFunc: func(m *model.Module) error {
			m.ID = mod.ID
			mods = append(mods, m)
			return nil
		},
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			if id != mod.ID {
				return nil, nil
			}
			return dm, nil
		},
		SetDomainModelDocumentationFunc: func(id model.ID, doc string) error {
			cap.dmID = id
			cap.calls = append(cap.calls, doc)
			dm.Documentation = doc
			return nil
		},
		GetModuleSecurityFunc: func(model.ID) (*security.ModuleSecurity, error) { return nil, nil },
	}
	return mb, cap
}

func TestCreateModule_DocCommentSetsDomainModelDocumentation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		existing bool
		stmt     *ast.CreateModuleStmt
		want     []string
	}{
		{"new module", false,
			&ast.CreateModuleStmt{Name: "Sales", Documentation: "Sales data", DocumentationSet: true},
			[]string{"Sales data"}},
		{"existing module, or modify", true,
			&ast.CreateModuleStmt{Name: "Sales", Documentation: "Sales data", DocumentationSet: true, CreateOrModify: true},
			[]string{"Sales data"}},
		{"empty comment clears", true,
			&ast.CreateModuleStmt{Name: "Sales", DocumentationSet: true, CreateOrModify: true},
			[]string{""}},
		// Controls: a statement that says nothing about documentation, and a
		// plain create of a module that exists (a no-op, as it always was),
		// must not write it.
		{"control: no comment", true,
			&ast.CreateModuleStmt{Name: "Sales", CreateOrModify: true}, nil},
		{"control: plain create of an existing module", true,
			&ast.CreateModuleStmt{Name: "Sales", Documentation: "Sales data", DocumentationSet: true}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mb, cap := moduleDocBackend(tc.existing, "stored by Studio Pro")
			ctx, _ := newMockCtx(t, withBackend(mb))
			assertNoError(t, execCreateModule(ctx, tc.stmt))
			if strings.Join(cap.calls, "|") != strings.Join(tc.want, "|") || len(cap.calls) != len(tc.want) {
				t.Fatalf("SetDomainModelDocumentation calls = %q, want %q", cap.calls, tc.want)
			}
			if len(tc.want) > 0 && cap.dmID != "dm-sales" {
				t.Errorf("wrote documentation to %q, want the module's domain model", cap.dmID)
			}
		})
	}
}

// describe module prints the documentation as the doc comment that sets it, so
// its output re-run stores the same text.
func TestDescribeModule_EmitsDomainModelDocumentation(t *testing.T) {
	mb, _ := moduleDocBackend(true, "Sales data,\nper region")
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeModule(ctx, "Sales", false))

	prog, errs := visitor.Build(buf.String())
	if len(errs) > 0 {
		t.Fatalf("describe module emitted MDL the parser rejects: %v\n%s", errs, buf.String())
	}
	var got *ast.CreateModuleStmt
	for _, s := range prog.Statements {
		if m, ok := s.(*ast.CreateModuleStmt); ok {
			got = m
		}
	}
	if got == nil || !got.DocumentationSet || got.Documentation != "Sales data,\nper region" {
		t.Fatalf("describe module does not round-trip the documentation: %+v\n%s", got, buf.String())
	}

	// Control: no documentation, no comment.
	mb, _ = moduleDocBackend(true, "")
	ctx, buf = newMockCtx(t, withBackend(mb))
	assertNoError(t, describeModule(ctx, "Sales", false))
	if strings.Contains(buf.String(), "/**") {
		t.Errorf("describe module printed a doc comment for an undocumented domain model:\n%s", buf.String())
	}
}

func TestAlterProjectSecurity_AdminUserName(t *testing.T) {
	var gotID model.ID
	var gotName string
	called := false
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) {
			return &security.ProjectSecurity{BaseElement: model.BaseElement{ID: "ps-1"}, AdminUserName: "MxAdmin"}, nil
		},
		SetProjectAdminUserNameFunc: func(id model.ID, name string) error {
			called, gotID, gotName = true, id, name
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	assertNoError(t, execAlterProjectSecurity(ctx, &ast.AlterProjectSecurityStmt{AdminUserName: "appadmin"}))
	if !called || gotID != "ps-1" || gotName != "appadmin" {
		t.Fatalf("SetProjectAdminUserName(%q, %q) called=%v, want (ps-1, appadmin)", gotID, gotName, called)
	}

	// Control: a statement about something else leaves the admin name alone.
	called = false
	on := true
	mb.SetProjectStrictModeFunc = func(model.ID, bool) error { return nil }
	assertNoError(t, execAlterProjectSecurity(ctx, &ast.AlterProjectSecurityStmt{StrictModeEnabled: &on}))
	if called {
		t.Error("a StrictMode statement wrote the admin user name")
	}
}

// mendixlabs/mxcli#624 (ako/mxcli#1259): AdminPassword is written through the
// backend, and the write report never echoes the value.
func TestAlterProjectSecurity_AdminPassword(t *testing.T) {
	const pw = "N3w-Admin-Passw0rd"
	var gotID model.ID
	var gotPW string
	called := false
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) {
			return &security.ProjectSecurity{BaseElement: model.BaseElement{ID: "ps-1"}}, nil
		},
		SetProjectAdminPasswordFunc: func(id model.ID, password string) error {
			called, gotID, gotPW = true, id, password
			return nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, execAlterProjectSecurity(ctx, &ast.AlterProjectSecurityStmt{AdminPassword: pw}))
	if !called || gotID != "ps-1" || gotPW != pw {
		t.Fatalf("SetProjectAdminPassword(%q, %q) called=%v, want (ps-1, %s)", gotID, gotPW, called, pw)
	}
	if strings.Contains(buf.String(), pw) {
		t.Errorf("the write report printed the password:\n%s", buf.String())
	}

	// Control: a statement about something else leaves the password alone.
	called = false
	on := true
	mb.SetProjectStrictModeFunc = func(model.ID, bool) error { return nil }
	assertNoError(t, execAlterProjectSecurity(ctx, &ast.AlterProjectSecurityStmt{StrictModeEnabled: &on}))
	if called {
		t.Error("a StrictMode statement wrote the admin password")
	}
}

// describe app security says whether the administrator has a password — the cause
// of CE0129 — and never what it is.
func TestShowProjectSecurity_AdminPasswordSetNotShown(t *testing.T) {
	for pw, want := range map[string]string{"": "Admin Password: not set", "N3w-Admin-Passw0rd": "Admin Password: set"} {
		mb := &mock.MockBackend{
			IsConnectedFunc: func() bool { return true },
			GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) {
				return &security.ProjectSecurity{AdminUserName: "MxAdmin", AdminPassword: pw}, nil
			},
		}
		ctx, buf := newMockCtx(t, withBackend(mb))
		assertNoError(t, listProjectSecurity(ctx))
		if !strings.Contains(buf.String(), want) {
			t.Errorf("password %q: output lacks %q:\n%s", pw, want, buf.String())
		}
		if pw != "" && strings.Contains(buf.String(), pw) {
			t.Errorf("describe app security printed the password:\n%s", buf.String())
		}
	}
}

// ako/mxcli#2305: `alter module … set documentation '…'` writes the same
// property as the doc comment on create, and stating the stored text again
// writes nothing — the re-run of a script must not report a write it skipped.
func TestAlterModuleSetDocumentation(t *testing.T) {
	for _, tc := range []struct {
		name, stored, doc string
		want              []string
		out               string
	}{
		{"sets it", "", "Sales data", []string{"Sales data"}, "Set documentation of module: Sales"},
		{"replaces it", "old text", "Sales data", []string{"Sales data"}, "Set documentation of module: Sales"},
		{"clears it", "old text", "", []string{""}, "Set documentation of module: Sales"},
		{"control: same text writes nothing", "Sales data", "Sales data", nil, "Documentation of module Sales unchanged"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mb, cap := moduleDocBackend(true, tc.stored)
			mb.GetModuleByNameFunc = func(name string) (*model.Module, error) {
				mods, _ := mb.ListModulesFunc()
				for _, m := range mods {
					if m.Name == name {
						return m, nil
					}
				}
				return nil, nil
			}
			ctx, buf := newMockCtx(t, withBackend(mb))
			assertNoError(t, execAlterModuleSet(ctx, &ast.AlterModuleSetStmt{ModuleName: "Sales", DocumentationSet: true, Documentation: tc.doc}))
			if strings.Join(cap.calls, "|") != strings.Join(tc.want, "|") || len(cap.calls) != len(tc.want) {
				t.Fatalf("SetDomainModelDocumentation calls = %q, want %q", cap.calls, tc.want)
			}
			if !strings.Contains(buf.String(), tc.out) {
				t.Errorf("output %q, want %q", buf.String(), tc.out)
			}
		})
	}
}
