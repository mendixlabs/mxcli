// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

func nativeNav() *mock.MockBackend {
	return &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetNavigationFunc: func() (*types.NavigationDocument, error) {
			return &types.NavigationDocument{Profiles: []*types.NavigationProfile{
				{Name: "NativePhone", IsNative: true,
					HomePage:  &types.NavHomePage{Microflow: "M.Home_NF"},
					MenuItems: []*types.NavMenuItem{{Caption: "Reports", Page: "M.Reports", ActionType: "PageAction"}}},
				{Name: "Responsive", Kind: "Responsive"},
			}}, nil
		},
	}
}

func runNav(t *testing.T, mb *mock.MockBackend, script string) (string, error) {
	t.Helper()
	ctx, buf := newMockCtx(t, withBackend(mb))
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatal(errs[0])
	}
	err := execCreateNavigation(ctx, prog.Statements[0].(*ast.CreateNavigationStmt))
	return buf.String(), err
}

// ako/mxcli#980: the native writer applies home pages, sync and the bottom bar
// only, so a login / not-found page or on-sync-error on a native profile was
// dropped while exec reported "updated". It is refused now.
func TestNativeProfile_RefusesWhatItsWriterCannotApply(t *testing.T) {
	for _, clause := range []string{"login page M.Login", "not found page M.NF", "on sync error continue"} {
		wrote := false
		mb := nativeNav()
		mb.UpdateNavigationProfileFunc = func(model.ID, string, types.NavigationProfileSpec) error { wrote = true; return nil }
		_, err := runNav(t, mb, "create or modify navigation NativePhone home nanoflow M.Home_NF "+clause+";")
		if err == nil || !strings.Contains(err.Error(), "native profile") || wrote {
			t.Errorf("%q: err=%v wrote=%v, want a refusal and no write", clause, err, wrote)
		}
	}
	// CONTROL: home, sync and the bottom bar are what the native writer applies.
	var got types.NavigationProfileSpec
	mb := nativeNav()
	mb.UpdateNavigationProfileFunc = func(_ model.ID, _ string, spec types.NavigationProfileSpec) error { got = spec; return nil }
	if _, err := runNav(t, mb, "create or modify navigation NativePhone home nanoflow M.Home_NF sync ( sync M.Order all; ) { menu item 'X' ( OnClick: show page M.P ) };"); err != nil {
		t.Fatal(err)
	}
	if !got.HasSync || len(got.HomePages) != 1 || got.HomePages[0].IsPage {
		t.Errorf("spec = %+v", got)
	}
	if !got.HasMenu || len(got.MenuItems) != 1 || got.MenuItems[0].Caption != "X" {
		t.Errorf("bottom bar not passed to the writer: %+v", got.MenuItems)
	}
}

// `home nanoflow` is a native profile's; on a web profile it is refused, and
// `home microflow` on a native profile — what describe used to print — is
// read as the nanoflow with a warning.
func TestNavigationHomeNanoflow(t *testing.T) {
	if _, err := runNav(t, nativeNav(), "create or modify navigation Responsive home nanoflow M.N;"); err == nil {
		t.Error("home nanoflow on a web profile was accepted")
	}
	out, err := runNav(t, nativeNav(), "create or modify navigation NativePhone home microflow M.Home_NF;")
	if err != nil || !strings.Contains(out, "write home nanoflow") {
		t.Errorf("home microflow on a native profile: err=%v out=%q", err, out)
	}
}

// Describe of a native profile names its nanoflow home `home nanoflow` and
// emits the bottom bar as the profile's { } block, so its own output re-runs.
func TestDescribeNativeProfile_ReRuns(t *testing.T) {
	mb := nativeNav()
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeNavigation(ctx, ast.QualifiedName{Name: "NativePhone"}))
	out := buf.String()
	if !strings.Contains(out, "home nanoflow M.Home_NF") || strings.Contains(out, "home microflow") {
		t.Errorf("native home:\n%s", out)
	}
	if !strings.Contains(out, "{\n  menu item 'Reports' ( OnClick: show page M.Reports )\n};") {
		t.Errorf("bottom bar not emitted as a { } block:\n%s", out)
	}
	mb.UpdateNavigationProfileFunc = func(model.ID, string, types.NavigationProfileSpec) error { return nil }
	if _, err := runNav(t, mb, out); err != nil {
		t.Errorf("describe output does not re-run: %v\n%s", err, out)
	}
}
