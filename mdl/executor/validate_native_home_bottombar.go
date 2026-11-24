// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// # A native home page on a bottom-bar layout must be in the bar (CE6108)
//
// A NativePhone home page — the profile's own or a role's — whose layout has
// "Bottom bar" enabled must be the page of one of the profile's bottom bar
// items: mxbuild reports CE6108 otherwise ("If the bottom bar is enabled on a
// home page, then that page must be included in the bottom bar menu of the
// native navigation profile"). check and exec accepted such a profile and the
// build refused it (ako/mxcli#2560); `syntax navigation create` documents the
// rule, and nothing enforced it.
//
// It needs the project: the layout is usually Atlas_Core's, and whether a page
// uses it is stored on the page. A page or layout the script creates counts as
// the script states it (nativeBarScript), so the check passes or fails the way
// the build will once the script has run. A statement that leaves the home
// pages or the bottom bar out keeps the stored ones, and they are judged.

// nativeBarScript is what a script states that the check reads: the layout
// each page it creates uses, and whether each layout it creates shows the
// bottom bar. nil at exec, where the script's earlier statements are stored.
type nativeBarScript struct {
	pageLayout map[string]string
	layoutBar  map[string]bool
}

func newNativeBarScript(prog *ast.Program) *nativeBarScript {
	s := &nativeBarScript{pageLayout: map[string]string{}, layoutBar: map[string]bool{}}
	for _, stmt := range prog.Statements {
		switch st := stmt.(type) {
		case *ast.CreatePageStmtV3:
			if st.Layout != "" {
				s.pageLayout[strings.ToLower(st.Name.String())] = st.Layout
			}
		case *ast.CreateLayoutStmt:
			on := false
			for k, v := range st.Properties {
				if strings.EqualFold(k, "ShowBottomBar") {
					on, _ = v.(bool)
				}
			}
			s.layoutBar[strings.ToLower(st.Name.String())] = on
		}
	}
	return s
}

// nativeHomeBottomBarErrors is one line per home page of a native profile that
// is drawn on a bottom-bar layout and is not a bottom bar item.
func nativeHomeBottomBarErrors(ctx *ExecContext, stmt ast.Statement, script *nativeBarScript) []string {
	s, ok := stmt.(*ast.CreateNavigationStmt)
	if !ok || !isNativeProfileName(s.ProfileName) || !ctx.Connected() {
		return nil
	}
	type home struct{ page, role string }
	var homes []home
	for _, hp := range s.HomePages {
		if hp.IsPage {
			h := home{page: hp.Target.String()}
			if hp.ForRole != nil {
				h.role = hp.ForRole.String()
			}
			homes = append(homes, h)
		}
	}
	bar := map[string]bool{}
	for _, it := range s.MenuItems {
		if p := barItemPage(it); p != "" {
			bar[strings.ToLower(p)] = true
		}
	}
	// What the statement leaves out is kept from the stored profile.
	if len(s.HomePages) == 0 || !s.HasMenuBlock {
		if nav, err := ctx.Backend.GetNavigation(); err == nil && nav != nil {
			for _, p := range nav.Profiles {
				if !p.IsNative || !strings.EqualFold(p.Name, s.ProfileName) {
					continue
				}
				if len(s.HomePages) == 0 {
					if p.HomePage != nil && p.HomePage.Page != "" {
						homes = append(homes, home{page: p.HomePage.Page})
					}
					for _, rb := range p.RoleBasedHomePages {
						if rb.Page != "" {
							homes = append(homes, home{page: rb.Page, role: rb.UserRole})
						}
					}
				}
				if !s.HasMenuBlock {
					for _, mi := range p.MenuItems {
						if mi.Page != "" {
							bar[strings.ToLower(mi.Page)] = true
						}
					}
				}
			}
		}
	}

	layouts := projectLayoutBars(ctx)
	var out []string
	for _, h := range homes {
		if bar[strings.ToLower(h.page)] {
			continue
		}
		layout := pageLayoutFor(ctx, h.page, script)
		if layout == "" {
			continue
		}
		on, known := false, false
		if script != nil {
			on, known = script.layoutBar[strings.ToLower(layout)]
		}
		if !known {
			on, known = layouts[strings.ToLower(layout)]
		}
		if !known || !on {
			continue
		}
		which := "home page " + h.page
		if h.role != "" {
			which = "home page " + h.page + " for " + h.role
		}
		out = append(out, fmt.Sprintf("%s uses layout %s, which shows the bottom bar, and is not one of the bottom bar's items, "+
			"which mx check refuses (CE6108); add `menu item '<caption>' ( OnClick: show page %s, Icon: glyph <code> )` "+
			"to the profile's { } block, or give the page a layout without the bottom bar", which, layout, h.page))
	}
	return out
}

// barItemPage is the page a bottom bar item opens, "" when it opens none.
func barItemPage(it ast.NavMenuItemDef) string {
	if it.Action != nil && it.Action.Type == "showPage" {
		return it.Action.Target
	}
	if it.Page != nil {
		return it.Page.String()
	}
	return ""
}

// pageLayoutFor is the layout a page uses: as the script creates it, else as
// the project stores it; "" when neither has the page.
func pageLayoutFor(ctx *ExecContext, page string, script *nativeBarScript) string {
	if script != nil {
		if l, ok := script.pageLayout[strings.ToLower(page)]; ok {
			return l
		}
	}
	mod, name, ok := strings.Cut(page, ".")
	if !ok {
		return ""
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return ""
	}
	p, err := findPageByName(ctx, ast.QualifiedName{Module: mod, Name: name}, h)
	if err != nil || p == nil {
		return ""
	}
	l, err := ctx.Backend.PageLayoutName(p.ID)
	if err != nil {
		return ""
	}
	return l
}

// projectLayoutBars is whether each stored layout shows the bottom bar, by
// lower-cased qualified name.
func projectLayoutBars(ctx *ExecContext) map[string]bool {
	out := map[string]bool{}
	layouts, err := ctx.Backend.ListLayouts()
	if err != nil {
		return out
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return out
	}
	for _, l := range layouts {
		out[strings.ToLower(h.GetModuleName(h.FindModuleID(l.ContainerID))+"."+l.Name)] = l.ShowBottomBar
	}
	return out
}
