// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"errors"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// protectedModuleFixture is a project with one Protected (add-on) module,
// MyAddOn, and one ordinary module, Open, holding the same kinds of element —
// the control: every reference that is refused into MyAddOn is accepted into
// Open, because only an add-on module hides anything.
func protectedModuleFixture(t *testing.T, settingsErr error) *ExecContext {
	t.Helper()
	app := mkModule("MyFirstModule")
	addOn := mkModule("MyAddOn")
	open := mkModule("Open")

	mf := func(mod *model.Module, name, level string) *microflows.Microflow {
		m := mkMicroflow(mod.ID, name)
		m.ExportLevel = level
		return m
	}
	ent := func(mod *model.Module, name, level string) *domainmodel.Entity {
		e := mkEntity(mod.ID, name)
		e.ExportLevel = level
		return e
	}
	flows := []*microflows.Microflow{
		mf(addOn, "ACT_Home", "API"),
		mf(addOn, "ACT_HomeHidden", "Hidden"),
		mf(addOn, "Hidden_MF", ""), // unset is Hidden: mxcli and mxbuild both read it so
		mf(addOn, "API_MF", "API"),
		mf(open, "MF", "Hidden"),
	}
	// A Usable microflow returning the Usable entity Pub, for the members of a
	// call result (#1654).
	getPub := mf(addOn, "API_GetPub", "API")
	getPub.ReturnType = &microflows.ObjectType{EntityQualifiedName: "MyAddOn.Pub"}
	flows = append(flows, getPub)
	// Members, for the CE1406 member cases: Pub and Pub2 are Usable entities
	// with one Usable and one Hidden attribute each, joined by a Hidden and a
	// Usable association. Open.E holds a Hidden attribute too — the control.
	member := func(name, level string) *domainmodel.Attribute {
		return &domainmodel.Attribute{BaseElement: model.BaseElement{ID: nextID("attr")}, Name: name,
			Type: &domainmodel.StringAttributeType{Length: 10}, ExportLevel: level}
	}
	pub, pub2 := ent(addOn, "Pub", "API"), ent(addOn, "Pub2", "API")
	pub.Attributes = []*domainmodel.Attribute{member("Shown", "API"), member("Inner", "Hidden")}
	pub2.Attributes = []*domainmodel.Attribute{member("Visible", "API"), member("Hid", "Hidden")}
	openE := ent(open, "E", "Hidden")
	openE.Attributes = []*domainmodel.Attribute{member("Inner", "Hidden")}
	addOnDM := mkDomainModel(addOn.ID, ent(addOn, "Request", "Hidden"), ent(addOn, "Secret", "Hidden"), pub, pub2)
	hiddenAssoc := mkAssociation(addOn.ID, "Pub_Pub2", pub.ID, pub2.ID)
	hiddenAssoc.ExportLevel = "Hidden"
	apiAssoc := mkAssociation(addOn.ID, "Pub_Pub2Api", pub.ID, pub2.ID)
	apiAssoc.ExportLevel = "API"
	addOnDM.Associations = []*domainmodel.Association{hiddenAssoc, apiAssoc}
	dms := map[model.ID]*domainmodel.DomainModel{
		addOn.ID: addOnDM,
		open.ID:  mkDomainModel(open.ID, openE),
		app.ID:   mkDomainModel(app.ID),
	}
	pgs := []*pages.Page{mkPage(addOn.ID, "Home_Page"), mkPage(open.ID, "P"), mkPage(app.ID, "Home_Web")}
	enum := func(mod *model.Module, name, level string) *model.Enumeration {
		e := mkEnumeration(mod.ID, name, "A")
		e.ExportLevel = level
		return e
	}
	enums := []*model.Enumeration{enum(addOn, "Kind", "Hidden"), enum(addOn, "PubKind", "API"), enum(open, "Kind", "Hidden")}

	mods := []*model.Module{app, addOn, open}
	ctx, _ := newMockCtx(t, withBackend(&mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return mods, nil },
		ListModuleSettingsFunc: func() ([]*types.ModuleSettings, error) {
			if settingsErr != nil {
				return nil, settingsErr
			}
			return []*types.ModuleSettings{
				{ContainerID: app.ID, ExportLevel: "Source"},
				{ContainerID: addOn.ID, ExportLevel: "Protected", ProtectedModuleType: "AddOn"},
				{ContainerID: open.ID, ExportLevel: "Source"},
			}, nil
		},
		ListMicroflowsFunc:   func() ([]*microflows.Microflow, error) { return flows, nil },
		ListPagesFunc:        func() ([]*pages.Page, error) { return pgs, nil },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return enums, nil },
		GetDomainModelFunc:   func(id model.ID) (*domainmodel.DomainModel, error) { return dms[id], nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{dms[app.ID], dms[addOn.ID], dms[open.ID]}, nil
		},
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			for _, m := range mods {
				if m.Name == name {
					return m, nil
				}
			}
			return nil, nil
		},
	}), withHierarchy(mkHierarchy(mods...)))
	return ctx
}

// mendixlabs/mxcli#1498: a reference into a Protected (add-on) module passed
// `check -p --references`, exec wrote it, and mxbuild refused the build. Every
// refused case below was measured on a blank Mendix 11.14.0 app (and 11.13.0)
// with MyAddOn's settings set to ExportLevel Protected; every accepted case is
// the measured control next to it.
func TestValidateProtectedModuleRefs(t *testing.T) {
	cases := []struct {
		name, src string
		want      []string // substrings of the one error expected; nil = clean
	}{
		// --- CE1406, from a project-level document (navigation) ---
		{"nav home page into add-on",
			"create or modify navigation Responsive home page MyAddOn.Home_Page;",
			[]string{"CE1406", "navigation profile 'Responsive' is not allowed to use page 'MyAddOn.Home_Page'", "project level documents"}},
		{"nav role home page into add-on (the reported shape)",
			"create or modify navigation Responsive home page MyFirstModule.Home_Web home page MyAddOn.Home_Page for Administrator;",
			[]string{"CE1406", "page 'MyAddOn.Home_Page'"}},
		{"nav home microflow, hidden",
			"create or modify navigation Responsive home microflow MyAddOn.ACT_HomeHidden;",
			[]string{"CE1406", "microflow 'MyAddOn.ACT_HomeHidden'", "export level api"}},
		{"nav home microflow, api", "create or modify navigation Responsive home microflow MyAddOn.ACT_Home;", nil},
		{"nav menu item shows add-on page",
			"create or modify navigation Responsive home page MyFirstModule.Home_Web { menu item 'A' ( OnClick: show page MyAddOn.Home_Page ) };",
			[]string{"CE1406", "menu item 'A' is not allowed to use page 'MyAddOn.Home_Page'"}},
		{"nav menu item calls hidden microflow",
			"create or modify navigation Responsive home page MyFirstModule.Home_Web { menu item 'A' ( OnClick: call microflow MyAddOn.Hidden_MF ) };",
			[]string{"CE1406", "microflow 'MyAddOn.Hidden_MF'"}},

		// --- CE1406, from an app module ---
		{"call hidden microflow",
			"create microflow MyFirstModule.Caller () begin $r = call microflow MyAddOn.Hidden_MF(); end;",
			[]string{"CE1406", "microflow 'MyAddOn.Hidden_MF'", "app modules cannot reference hidden elements"}},
		{"call api microflow", "create microflow MyFirstModule.Caller () begin $r = call microflow MyAddOn.API_MF(); end;", nil},
		{"show add-on page",
			"create microflow MyFirstModule.Opener () begin show page MyAddOn.Home_Page; end;",
			[]string{"CE1406", "page 'MyAddOn.Home_Page'"}},
		{"parameter of hidden entity",
			"create microflow MyFirstModule.TakesReq ($r: MyAddOn.Secret) begin end;",
			[]string{"CE1406", "parameter 'r' is not allowed to use entity 'MyAddOn.Secret'"}},
		{"parameter of usable entity", "create microflow MyFirstModule.TakesReq ($r: MyAddOn.Pub) begin end;", nil},

		// --- CE5400, a Usable flow in the add-on exposing a Hidden entity ---
		{"api microflow, hidden entity parameter (the reported shape)",
			"create microflow MyAddOn.API_Request_Post ($body: MyAddOn.Request) returns Boolean export level api begin return true; end;",
			[]string{"CE5400", "entity 'MyAddOn.Request' should be 'Usable' because parameter 'body' exposes it"}},
		{"api microflow, hidden entity return type",
			"create microflow MyAddOn.API_Get () returns MyAddOn.Request export level api begin return empty; end;",
			[]string{"CE5400", "because microflow 'MyAddOn.API_Get' exposes it"}},
		{"api microflow, list of hidden entity",
			"create microflow MyAddOn.API_List ($l: list of MyAddOn.Secret) returns Boolean export level api begin return true; end;",
			[]string{"CE5400", "parameter 'l'"}},
		{"api microflow, usable entity",
			"create microflow MyAddOn.API_Pub ($p: MyAddOn.Pub) returns Boolean export level api begin return true; end;", nil},
		{"hidden microflow, hidden entity",
			"create microflow MyAddOn.Internal ($body: MyAddOn.Request) returns Boolean begin return true; end;", nil},

		// --- script-defined targets resolve the way exec will write them ---
		{"script microflow without export level is hidden",
			"create microflow MyAddOn.New () begin end;\ncreate microflow MyFirstModule.C () begin call microflow MyAddOn.New(); end;",
			[]string{"statement 2", "CE1406", "microflow 'MyAddOn.New'"}},
		{"script microflow with export level api",
			"create microflow MyAddOn.New () export level api begin end;\ncreate microflow MyFirstModule.C () begin call microflow MyAddOn.New(); end;", nil},
		{"script entity is hidden",
			"create non-persistent entity MyAddOn.Fresh (X: String(10));\ncreate microflow MyAddOn.API_F ($f: MyAddOn.Fresh) returns Boolean export level api begin return true; end;",
			[]string{"statement 2", "CE5400", "entity 'MyAddOn.Fresh'"}},

		// --- mendixlabs/mxcli#1498 follow-up: the level MDL can now state ---
		{"script entity with export level api",
			"create non-persistent entity MyAddOn.Fresh (X: String(10)) export level api;\ncreate microflow MyAddOn.API_F ($f: MyAddOn.Fresh) returns Boolean export level api begin return true; end;", nil},
		{"stored hidden entity rewritten with export level api",
			"create or modify non-persistent entity MyAddOn.Request (Payload: String(200)) export level api;\ncreate microflow MyAddOn.API_R ($r: MyAddOn.Request) returns Boolean export level api begin return true; end;", nil},
		{"stored hidden entity made usable by alter",
			"alter entity MyAddOn.Request set export level api;\ncreate microflow MyAddOn.API_R ($r: MyAddOn.Request) returns Boolean export level api begin return true; end;", nil},
		{"stored usable entity made hidden by alter",
			"alter entity MyAddOn.Pub set export level hidden;\ncreate microflow MyAddOn.API_P ($p: MyAddOn.Pub) returns Boolean export level api begin return true; end;",
			[]string{"statement 2", "CE5400", "entity 'MyAddOn.Pub'"}},
		{"rewrite without the clause keeps the stored usable level",
			"create or modify non-persistent entity MyAddOn.Pub (X: String(10));\ncreate microflow MyAddOn.API_P ($p: MyAddOn.Pub) returns Boolean export level api begin return true; end;", nil},
		{"the hint names the MDL fix",
			"create microflow MyAddOn.API_Request_Post ($body: MyAddOn.Request) returns Boolean export level api begin return true; end;",
			[]string{"CE5400", "alter entity MyAddOn.Request set export level api"}},
		{"api microflow, hidden enumeration parameter (measured 11.14.0)",
			"create microflow MyAddOn.API_Kind ($k: enum MyAddOn.Kind) returns Boolean export level api begin return true; end;",
			[]string{"CE5400", "enumeration 'MyAddOn.Kind' should be 'Usable' because parameter 'k' exposes it", "alter enumeration MyAddOn.Kind set export level api"}},
		{"api microflow, hidden enumeration return type",
			"create microflow MyAddOn.API_GetKind () returns enum MyAddOn.Kind export level api begin return empty; end;",
			[]string{"CE5400", "enumeration 'MyAddOn.Kind'", "microflow 'MyAddOn.API_GetKind'"}},
		{"api microflow, usable enumeration",
			"create microflow MyAddOn.API_Kind ($k: enum MyAddOn.PubKind) returns Boolean export level api begin return true; end;", nil},
		{"script enumeration is hidden",
			"create enumeration MyAddOn.New (A 'A');\ncreate microflow MyAddOn.API_N ($k: enum MyAddOn.New) returns Boolean export level api begin return true; end;",
			[]string{"statement 2", "CE5400", "enumeration 'MyAddOn.New'"}},
		{"script enumeration with export level api",
			"create enumeration MyAddOn.New (A 'A') export level api;\ncreate microflow MyAddOn.API_N ($k: enum MyAddOn.New) returns Boolean export level api begin return true; end;", nil},
		{"stored hidden enumeration made usable by alter",
			"alter enumeration MyAddOn.Kind set export level api;\ncreate microflow MyAddOn.API_K ($k: enum MyAddOn.Kind) returns Boolean export level api begin return true; end;", nil},
		{"open module api microflow, hidden enumeration",
			"create microflow Open.API_K ($k: enum Open.Kind) returns Boolean export level api begin return true; end;", nil},

		// --- CE1406 on a Hidden MEMBER, from an app module (measured 11.14.0,
		// one microflow per shape; the Usable member beside each is the control)
		{"change hidden attribute",
			"create microflow MyFirstModule.C ($p: MyAddOn.Pub) begin change $p (Inner = 'x'); end;",
			[]string{"CE1406", "change $p is not allowed to use attribute 'MyAddOn.Pub.Inner'", "app modules cannot reference hidden elements",
				"alter entity MyAddOn.Pub modify attribute Inner: String(10) export level api"}},
		{"change usable attribute", "create microflow MyFirstModule.C ($p: MyAddOn.Pub) begin change $p (Shown = 'x'); end;", nil},
		{"create with hidden attribute",
			"create microflow MyFirstModule.C () begin $n = create MyAddOn.Pub (Inner = 'x'); end;",
			[]string{"CE1406", "create MyAddOn.Pub is not allowed to use attribute 'MyAddOn.Pub.Inner'"}},
		{"change sets hidden association",
			"create microflow MyFirstModule.C ($p: MyAddOn.Pub, $q: MyAddOn.Pub2) begin change $p (MyAddOn.Pub_Pub2 = $q); end;",
			[]string{"CE1406", "association 'MyAddOn.Pub_Pub2'", "alter association MyAddOn.Pub_Pub2 set export level api"}},
		{"change sets usable association",
			"create microflow MyFirstModule.C ($p: MyAddOn.Pub, $q: MyAddOn.Pub2) begin change $p (MyAddOn.Pub_Pub2Api = $q); end;", nil},
		{"retrieve over hidden association",
			"create microflow MyFirstModule.C ($p: MyAddOn.Pub) begin retrieve $q from $p/MyAddOn.Pub_Pub2; end;",
			[]string{"CE1406", "retrieve $q is not allowed to use association 'MyAddOn.Pub_Pub2'"}},
		{"retrieve over usable association",
			"create microflow MyFirstModule.C ($p: MyAddOn.Pub) begin retrieve $q from $p/MyAddOn.Pub_Pub2Api; end;", nil},
		{"xpath on hidden attribute",
			"create microflow MyFirstModule.C () begin retrieve $l from MyAddOn.Pub where Inner = 'x'; end;",
			[]string{"CE1406", "attribute 'MyAddOn.Pub.Inner'"}},
		{"xpath on usable attribute", "create microflow MyFirstModule.C () begin retrieve $l from MyAddOn.Pub where Shown = 'x'; end;", nil},
		{"xpath on hidden association",
			"create microflow MyFirstModule.C ($p: MyAddOn.Pub) begin retrieve $l from MyAddOn.Pub2 where MyAddOn.Pub_Pub2 = $p; end;",
			[]string{"CE1406", "association 'MyAddOn.Pub_Pub2'"}},
		{"xpath path to hidden attribute over usable association",
			"create microflow MyFirstModule.C () begin retrieve $l from MyAddOn.Pub where MyAddOn.Pub_Pub2Api/MyAddOn.Pub2/Hid = 'x'; end;",
			[]string{"CE1406", "attribute 'MyAddOn.Pub2.Hid'"}},
		{"expression reads hidden attribute",
			"create microflow MyFirstModule.C ($p: MyAddOn.Pub) returns String begin return $p/Inner; end;",
			[]string{"CE1406", "attribute 'MyAddOn.Pub.Inner'"}},
		{"expression reads usable attribute",
			"create microflow MyFirstModule.C ($p: MyAddOn.Pub) returns String begin return $p/Shown; end;", nil},
		{"expression over usable association to hidden attribute",
			"create microflow MyFirstModule.C ($p: MyAddOn.Pub) returns String begin return $p/MyAddOn.Pub_Pub2Api/Hid; end;",
			[]string{"CE1406", "attribute 'MyAddOn.Pub2.Hid'"}},
		{"expression over hidden association",
			"create microflow MyFirstModule.C ($p: MyAddOn.Pub) returns Boolean begin return $p/MyAddOn.Pub_Pub2 != empty; end;",
			[]string{"CE1406", "association 'MyAddOn.Pub_Pub2'"}},
		{"member in a nested block and a loop",
			"create microflow MyFirstModule.C () begin retrieve $all from MyAddOn.Pub; loop $x in $all begin if true then change $x (Inner = 'x'); end if; end loop; end;",
			[]string{"CE1406", "attribute 'MyAddOn.Pub.Inner'"}},
		{"script makes the attribute usable",
			"alter entity MyAddOn.Pub modify attribute Inner: String(10) export level api;\ncreate microflow MyFirstModule.C ($p: MyAddOn.Pub) begin change $p (Inner = 'x'); end;", nil},
		{"script makes the association usable",
			"alter association MyAddOn.Pub_Pub2 set export level api;\ncreate microflow MyFirstModule.C ($p: MyAddOn.Pub) begin retrieve $q from $p/MyAddOn.Pub_Pub2; end;", nil},
		{"script hides a usable attribute",
			"alter entity MyAddOn.Pub modify attribute Shown: String(10) export level hidden;\ncreate microflow MyFirstModule.C ($p: MyAddOn.Pub) begin change $p (Shown = 'x'); end;",
			[]string{"statement 2", "CE1406", "attribute 'MyAddOn.Pub.Shown'"}},
		{"script-created attribute is hidden",
			"create persistent entity MyAddOn.Fresh (X: String(10)) export level api;\ncreate microflow MyFirstModule.C ($f: MyAddOn.Fresh) begin change $f (X = 'x'); end;",
			[]string{"statement 2", "CE1406", "attribute 'MyAddOn.Fresh.X'"}},
		{"script-created attribute with export level api",
			"create persistent entity MyAddOn.Fresh (X: String(10) export level api) export level api;\ncreate microflow MyFirstModule.C ($f: MyAddOn.Fresh) begin change $f (X = 'x'); end;", nil},
		{"script-created association is hidden",
			"create association MyAddOn.Pub_Fresh from MyAddOn.Pub to MyAddOn.Pub2;\ncreate microflow MyFirstModule.C ($p: MyAddOn.Pub, $q: MyAddOn.Pub2) begin change $p (MyAddOn.Pub_Fresh = $q); end;",
			[]string{"statement 2", "CE1406", "association 'MyAddOn.Pub_Fresh'"}},
		{"inside the add-on a hidden member is fine",
			"create microflow MyAddOn.Inner ($p: MyAddOn.Pub) begin change $p (Inner = 'x'); retrieve $q from $p/MyAddOn.Pub_Pub2; end;", nil},
		{"open module hidden attribute", "create microflow MyFirstModule.C ($e: Open.E) begin change $e (Inner = 'x'); end;", nil},
		{"untyped variable is silent", "create microflow MyFirstModule.C () begin change $nobody (Inner = 'x'); end;", nil},
		// --- a member of a call result (#1654) ---
		{"hidden attribute on a call result",
			"create microflow MyFirstModule.G () returns String begin $p = call microflow MyAddOn.API_GetPub(); return $p/Inner; end;",
			[]string{"CE1406", "the end event is not allowed to use attribute 'MyAddOn.Pub.Inner'"}},
		{"hidden attribute on a call result, in a declare",
			"create microflow MyFirstModule.G () begin $p = call microflow MyAddOn.API_GetPub(); declare $s String = $p/Inner; end;",
			[]string{"CE1406", "declare $s is not allowed to use attribute 'MyAddOn.Pub.Inner'"}},
		{"usable attribute on a call result",
			"create microflow MyFirstModule.G () returns String begin $p = call microflow MyAddOn.API_GetPub(); return $p/Shown; end;", nil},

		// --- controls: an ordinary module hides nothing ---
		{"open module page", "create or modify navigation Responsive home page Open.P;", nil},
		{"open module hidden microflow", "create microflow MyFirstModule.C () begin call microflow Open.MF(); end;", nil},
		{"open module hidden entity", "create microflow MyFirstModule.C ($e: Open.E) begin end;", nil},
		{"open module api microflow, hidden entity",
			"create microflow Open.API ($e: Open.E) returns Boolean export level api begin return true; end;", nil},
		// --- silent where it cannot be established ---
		{"unknown target is the not-found check's", "create microflow MyFirstModule.C () begin call microflow MyAddOn.Nope(); end;", nil},
	}
	ctx := protectedModuleFixture(t, nil)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got []string
			for _, err := range validateProtectedModuleRefs(ctx, parseMDL(t, c.src)) {
				got = append(got, err.Error())
			}
			if c.want == nil {
				if len(got) != 0 {
					t.Fatalf("want no error, got %q", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("want 1 error containing %q, got %q", c.want, got)
			}
			for _, w := range c.want {
				if !strings.Contains(got[0], w) {
					t.Errorf("want %q in %q", w, got[0])
				}
			}
		})
	}
}

// Unreadable module settings mean no module can be shown to be Protected, so
// nothing is reported — a false refusal would be worse than no check.
func TestValidateProtectedModuleRefs_SilentWithoutSettings(t *testing.T) {
	ctx := protectedModuleFixture(t, errors.New("boom"))
	prog := parseMDL(t, "create or modify navigation Responsive home page MyAddOn.Home_Page;")
	if errs := validateProtectedModuleRefs(ctx, prog); len(errs) != 0 {
		t.Fatalf("want no errors without module settings, got %v", errs)
	}
}

// The check is wired into the --references pass.
func TestValidateProgram_ReportsProtectedModuleRef(t *testing.T) {
	ctx := protectedModuleFixture(t, nil)
	prog := parseMDL(t, "create microflow MyFirstModule.Caller () begin $r = call microflow MyAddOn.Hidden_MF(); end;")
	var found bool
	for _, err := range validateProgram(ctx, prog) {
		if strings.Contains(err.Error(), "CE1406") {
			found = true
		}
	}
	if !found {
		t.Fatal("validateProgram did not report the CE1406 reference into the add-on module")
	}
}
