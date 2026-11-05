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

// ako/mxcli#1756 (mendixlabs/mxcli#1664): a profile's application title —
// "Mendix" in a new project, the prefix of every browser tab — and its
// application icon had no clause, and describe did not print them.

// appTitleCtx is a project with a web profile, a native one, and one image
// collection MyFirstModule.Images holding Logo. got receives the spec written.
func appTitleCtx(t *testing.T, got *types.NavigationProfileSpec, wrote *bool) *ExecContext {
	t.Helper()
	mod := mkModule("MyFirstModule")
	ic := &types.ImageCollection{
		BaseElement: model.BaseElement{ID: nextID("ic")},
		ContainerID: mod.ID,
		Name:        "Images",
		Images:      []types.Image{{Name: "Logo"}},
	}
	h := mkHierarchy(mod)
	withContainer(h, ic.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetNavigationFunc: func() (*types.NavigationDocument, error) {
			return &types.NavigationDocument{Profiles: []*types.NavigationProfile{
				{Name: "Responsive", Kind: "Responsive", AppTitle: "Mendix", AppIcon: "Atlas_Core.Content.Mendix"},
				{Name: "NativePhone", IsNative: true},
			}}, nil
		},
		ListImageCollectionsFunc: func() ([]*types.ImageCollection, error) { return []*types.ImageCollection{ic}, nil },
		UpdateNavigationProfileFunc: func(_ model.ID, _ string, spec types.NavigationProfileSpec) error {
			if got != nil {
				*got = spec
			}
			if wrote != nil {
				*wrote = true
			}
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx
}

func execNavScript(t *testing.T, ctx *ExecContext, script string) error {
	t.Helper()
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatalf("%q: %v", script, errs[0])
	}
	return execCreateNavigation(ctx, prog.Statements[0].(*ast.CreateNavigationStmt))
}

// The clauses reach the writer as the spec; a statement that does not mention
// them leaves the stored title and icon alone (nil), as every omitted clause
// does (mendixlabs/mxcli#1522).
func TestAppTitleClauseReachesTheSpec(t *testing.T) {
	for _, c := range []struct {
		clause      string
		title, icon *string
	}{
		{"", nil, nil},
		{"app title 'EWO'", strP("EWO"), nil},
		{"app title 'It''s mine'", strP("It's mine"), nil},
		{"app icon MyFirstModule.Images.Logo", nil, strP("MyFirstModule.Images.Logo")},
		{"app icon none", nil, strP("")},
		{"app icon MyFirstModule.Images.Logo app title 'EWO'", strP("EWO"), strP("MyFirstModule.Images.Logo")},
	} {
		var got types.NavigationProfileSpec
		if err := execNavScript(t, appTitleCtx(t, &got, nil), "create or modify navigation Responsive home page M.Home "+c.clause+";"); err != nil {
			t.Fatalf("%q: %v", c.clause, err)
		}
		if !sameStrP(got.AppTitle, c.title) || !sameStrP(got.AppIcon, c.icon) {
			t.Errorf("%q: spec title %s icon %s, want %s and %s", c.clause, showStrP(got.AppTitle), showStrP(got.AppIcon), showStrP(c.title), showStrP(c.icon))
		}
	}
}

// A native profile has no AppTitle or AppIcon (Navigation$NativeNavigationProfile
// declares neither), so the clauses are refused there rather than dropped.
func TestAppTitleClauseIsRefusedOnANativeProfile(t *testing.T) {
	for _, clause := range []string{"app title 'EWO'", "app icon MyFirstModule.Images.Logo"} {
		wrote := false
		err := execNavScript(t, appTitleCtx(t, nil, &wrote), "create or modify navigation NativePhone home nanoflow M.N "+clause+";")
		if err == nil || !strings.Contains(err.Error(), "native profile") || wrote {
			t.Errorf("%q: err=%v wrote=%v, want a refusal and no write", clause, err, wrote)
		}
	}
}

// The icon is resolved like every other image reference, and exec refuses an
// unresolved one before writing — the name is stored as written, so a typo
// would otherwise surface only at the next build.
func TestAppIconMustResolveAtExec(t *testing.T) {
	for _, c := range []struct {
		icon string
		want string // "" = written
	}{
		{"MyFirstModule.Images.Logo", ""},
		// Mendix resolves an image reference case-sensitively: an app icon
		// spelled `Atlas_Core.Content.mendix` for the stored `Mendix` is CE1613
		// at the navigation profile (measured on 11.13.0).
		{"myfirstmodule.images.logo", "app icon: image collection not found: myfirstmodule.images"},
		{"none", ""},
		{"MyFirstModule.Images.Nope", "app icon: image not found: Nope in image collection MyFirstModule.Images"},
		{"Shop.Images.Logo", "app icon: image collection not found: Shop.Images"},
		{"MyFirstModule.Logo", "app icon: invalid image reference: MyFirstModule.Logo"},
	} {
		wrote := false
		err := execNavScript(t, appTitleCtx(t, nil, &wrote), "create or modify navigation Responsive app icon "+c.icon+";")
		if c.want == "" {
			if err != nil || !wrote {
				t.Errorf("%s: err=%v wrote=%v, want it written", c.icon, err, wrote)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), c.want) || wrote {
			t.Errorf("%s: err=%v wrote=%v, want a refusal containing %q and no write", c.icon, err, wrote, c.want)
		}
	}
}

// `check --references` reports the same unresolved icon, and accepts an image
// the script itself creates, as a page's image reference does (#1419).
func TestAppIconIsCheckedByReferences(t *testing.T) {
	for _, c := range []struct {
		script string
		want   string // "" = clean
	}{
		{"create or modify navigation Responsive app icon MyFirstModule.Images.Logo;", ""},
		{"create or modify navigation Responsive app icon MyFirstModule.Images.Nope;", "app icon: image not found: Nope"},
		{"create image collection Shop.Brand (image Logo from file 'logo.png');\n" +
			"create or modify navigation Responsive app icon Shop.Brand.Logo;", ""},
		{"create image collection Shop.Brand (image Logo from file 'logo.png');\n" +
			"create or modify navigation Responsive app icon Shop.Brand.Other;", "app icon: image not found: Other"},
	} {
		ctx := appTitleCtx(t, nil, nil)
		prog, errs := visitor.Build(c.script)
		if len(errs) > 0 {
			t.Fatalf("%q: %v", c.script, errs)
		}
		sc := newScriptContext()
		sc.collectDefinitions(prog)
		got := validateNavigationTargets(ctx, prog, sc)
		if c.want == "" {
			if len(got) != 0 {
				t.Errorf("%q: %v, want clean", c.script, got)
			}
			continue
		}
		if len(got) != 1 || !strings.Contains(got[0].Error(), c.want) {
			t.Errorf("%q: %v, want one error containing %q", c.script, got, c.want)
		}
	}
}

// Describe prints what is stored, right after the profile's name, and its
// output parses back to the same values. An empty title and no icon are not
// printed: re-running the description then keeps them as they are.
func TestDescribeAppTitleAndIcon(t *testing.T) {
	for _, c := range []struct {
		title, icon string
		want        []string
		absent      []string
	}{
		{"EWO", "MyFirstModule.Images.Logo", []string{"create or modify navigation Responsive\n  app title 'EWO'\n  app icon MyFirstModule.Images.Logo\n"}, nil},
		// Studio Pro's blank-project values are printed too: they are what is
		// stored, and a profile mxcli adds stores neither.
		{"Mendix", "Atlas_Core.Content.Mendix", []string{"  app title 'Mendix'\n", "  app icon Atlas_Core.Content.Mendix\n"}, nil},
		{"It's mine", "", []string{"  app title 'It''s mine'\n"}, []string{"app icon"}},
		{"", "", nil, []string{"app title", "app icon"}},
		// A segment the lexer owns as a keyword (and `none`, which the clause
		// reads as "no icon" when it stands alone) still re-reads as the name.
		{"", "MyFirstModule.Images.Title", []string{"  app icon MyFirstModule.Images."}, []string{"app title"}},
		{"", "MyFirstModule.Images.none", []string{"  app icon MyFirstModule.Images."}, []string{"app title"}},
	} {
		ctx, buf := newMockCtx(t)
		outputNavigationProfile(ctx, &types.NavigationProfile{Name: "Responsive", Kind: "Responsive", AppTitle: c.title, AppIcon: c.icon})
		out := buf.String()
		for _, w := range c.want {
			if !strings.Contains(out, w) {
				t.Errorf("title %q icon %q: missing %q in\n%s", c.title, c.icon, w, out)
			}
		}
		for _, a := range c.absent {
			if strings.Contains(out, a) {
				t.Errorf("title %q icon %q: %q printed in\n%s", c.title, c.icon, a, out)
			}
		}

		// Round trip: the description parses, and states the stored values.
		prog, errs := visitor.Build(strings.TrimSpace(out))
		if len(errs) > 0 {
			t.Fatalf("title %q icon %q: describe output does not parse: %v\n%s", c.title, c.icon, errs, out)
		}
		stmt := prog.Statements[0].(*ast.CreateNavigationStmt)
		if c.title != "" && (stmt.AppTitle == nil || *stmt.AppTitle != c.title) {
			t.Errorf("title %q re-read as %s", c.title, showStrP(stmt.AppTitle))
		}
		if c.title == "" && stmt.AppTitle != nil {
			t.Errorf("an empty title re-read as %s", showStrP(stmt.AppTitle))
		}
		if c.icon != "" && (stmt.AppIcon == nil || *stmt.AppIcon != c.icon) {
			t.Errorf("icon %q re-read as %s", c.icon, showStrP(stmt.AppIcon))
		}
		if c.icon == "" && stmt.AppIcon != nil {
			t.Errorf("no icon re-read as %s", showStrP(stmt.AppIcon))
		}
	}

	// A native profile has neither, whatever the struct carries.
	ctx, buf := newMockCtx(t)
	outputNavigationProfile(ctx, &types.NavigationProfile{Name: "NativePhone", IsNative: true, AppTitle: "x", AppIcon: "M.I.L"})
	if out := buf.String(); strings.Contains(out, "app title") || strings.Contains(out, "app icon") {
		t.Errorf("a native profile described with an app title or icon:\n%s", out)
	}
}

func strP(s string) *string { return &s }

func sameStrP(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func showStrP(p *string) string {
	if p == nil {
		return "nil"
	}
	return "'" + *p + "'"
}
