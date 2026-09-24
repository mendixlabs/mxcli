// SPDX-License-Identifier: Apache-2.0

// A navigation tree or menu bar draws its items from a navigation profile or
// from a menu document. Only the profile was read, written and described: a
// tree on a menu document (Atlas_Core.Tablet_Sidebar, Phone_Sidebar) described
// as a bare `navigationtree`, exec wrote it back on the Responsive profile, and
// `Menu:` in a script was ignored without a word (mendixlabs/mxcli#1189).
package executor

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func describeWidget(t *testing.T, raw map[string]any) string {
	t.Helper()
	ctx := (&Executor{}).newExecContext(context.Background())
	var out bytes.Buffer
	ctx.Output = &out
	for _, w := range parseRawWidget(ctx, raw) {
		outputWidgetMDLV3(ctx, w, 0)
	}
	return out.String()
}

func TestDescribeMenuWidgets_PrintTheMenuDocument(t *testing.T) {
	for _, typ := range []string{"Forms$NavigationTree", "Forms$MenuBar"} {
		got := describeWidget(t, map[string]any{
			"$Type": typ,
			"Name":  "nav",
			"MenuSource": map[string]any{
				"$Type": "Forms$MenuDocumentSource",
				"Menu":  "Atlas_Core.Tablet_Menu",
			},
		})
		if !strings.Contains(got, "(Menu: Atlas_Core.Tablet_Menu)") {
			t.Errorf("%s: describe = %q, want the menu document", typ, got)
		}
		if strings.Contains(got, "Profile") {
			t.Errorf("%s: describe = %q, a menu-document tree has no profile", typ, got)
		}
	}
}

func TestDescribeNavigationTree_StillPrintsTheProfile(t *testing.T) {
	got := describeWidget(t, map[string]any{
		"$Type": "Forms$NavigationTree",
		"Name":  "nav",
		"MenuSource": map[string]any{
			"$Type":             "Forms$NavigationSource",
			"NavigationProfile": "Responsive",
		},
	})
	if !strings.Contains(got, "(Profile: 'Responsive')") {
		t.Errorf("describe = %q, want the profile", got)
	}
}

func menuSourceBuilder(known ...string) *pageBuilder {
	mb := &mock.MockBackend{
		GetMenuDocumentByQualifiedNameFunc: func(module, name string) (*types.MenuDocument, error) {
			for _, k := range known {
				if k == module+"."+name {
					return &types.MenuDocument{Name: name}, nil
				}
			}
			return nil, fmt.Errorf("menu not found")
		},
	}
	return &pageBuilder{backend: mb}
}

func TestBuildMenuWidgets_KeepTheMenuDocument(t *testing.T) {
	pb := menuSourceBuilder("MyFirstModule.Side_Menu")
	w := &ast.WidgetV3{Type: "navigationtree", Name: "nav", Properties: map[string]any{"Menu": "MyFirstModule.Side_Menu"}}
	tree, err := pb.buildNavigationTreeV3(w)
	if err != nil {
		t.Fatal(err)
	}
	if got := tree.(*pages.NavigationTree).MenuDocument; got != "MyFirstModule.Side_Menu" {
		t.Errorf("tree MenuDocument = %q", got)
	}
	w.Type = "menubar"
	bar, err := pb.buildMenuBarV3(w)
	if err != nil {
		t.Fatal(err)
	}
	if got := bar.(*pages.MenuBar).MenuDocument; got != "MyFirstModule.Side_Menu" {
		t.Errorf("bar MenuDocument = %q", got)
	}
}

func TestBuildNavigationTree_RejectsAMissingMenuOrTwoSources(t *testing.T) {
	pb := menuSourceBuilder("MyFirstModule.Side_Menu")
	for _, tc := range []struct {
		props map[string]any
		want  string
	}{
		{map[string]any{"Menu": "MyFirstModule.Nope"}, "menu not found: MyFirstModule.Nope"},
		{map[string]any{"Menu": "MyFirstModule.Side_Menu", "Profile": "Responsive"}, "give one"},
		{map[string]any{"Menu": "Side_Menu"}, "qualified name"},
	} {
		_, err := pb.buildNavigationTreeV3(&ast.WidgetV3{Type: "navigationtree", Name: "nav", Properties: tc.props})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: err = %v, want %q", tc.props, err, tc.want)
		}
	}
}
