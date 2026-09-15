// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// Two member positions inside a widget that reference checking walked past:
// an XPath constraint's members (mendixlabs/mxcli#1049) and a template
// parameter written as a `$variable/` path (mendixlabs/mxcli#1046). Both passed
// check, passed exec, and failed the build with CE1613.

func paramWidget(name string, prop string, value any) *ast.WidgetV3 {
	return &ast.WidgetV3{
		Type: "dynamictext", Name: name,
		Properties: map[string]any{
			prop: []ast.ParamAssignmentV3{{Index: 1, Value: value}},
		},
	}
}

func pageWith(widgets ...*ast.WidgetV3) *ast.Program {
	return &ast.Program{Statements: []ast.Statement{
		&ast.CreatePageStmtV3{
			Name:    ast.QualifiedName{Module: "Shop", Name: "P"},
			Widgets: widgets,
		},
	}}
}

func varPath(v string, path ...string) *ast.AttributePathExpr {
	return &ast.AttributePathExpr{Variable: v, Path: path}
}

// ---------------------------------------------------------------------------
// #1046 — ContentParams
// ---------------------------------------------------------------------------

// A template parameter is evaluated against the widget's own context object.
// The writer strips ONE prefix and only on one branch, so the rule is narrower
// than "a variable root is wrong". Measured on 11.13.0, each shape executed and
// built:
//
//	OrderNo                                    clean
//	Bench.Order_Customer/Name                  clean  (association hop)
//	$currentObject/Bench.Order_Customer/Name   clean  (prefix stripped)
//	$currentObject/OrderNo                     CE1613
//	$Customer/Name                             CE1613
func TestValidateWidgetParamPaths_ReportsAVariableRootedPath(t *testing.T) {
	for _, tc := range []struct {
		name     string
		variable string
		path     []string
		want     string
	}{
		{"a page parameter with a bare attribute", "Customer", []string{"Name"}, "Name"},
		{"$currentObject with a bare attribute", "currentObject", []string{"OrderNo"}, "OrderNo"},
		// A non-currentObject root is kept verbatim whatever follows it, so an
		// association path under one is broken too.
		{"a page parameter with an association path", "Customer",
			[]string{"Shop.Order_Customer", "Name"}, "Shop.Order_Customer/Name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := ValidateWidgetParamPaths(pageWith(
				paramWidget("dt", "ContentParams", varPath(tc.variable, tc.path...))))
			if len(v) != 1 {
				t.Fatalf("got %d violations, want 1: %v", len(v), v)
			}
			if v[0].RuleID != "MDL-WIDGET24" {
				t.Errorf("rule = %s, want MDL-WIDGET24", v[0].RuleID)
			}
			// The message has to say what to write instead — the fix is one
			// token, and an error that only says "wrong" makes the author guess.
			if !strings.Contains(v[0].Message, `Write "`+tc.want+`"`) {
				t.Errorf("message should name the replacement %q: %s", tc.want, v[0].Message)
			}
			if !strings.Contains(v[0].Message, "CE1613") {
				t.Errorf("message should name the build error it prevents: %s", v[0].Message)
			}
		})
	}
}

// THE CONTROL for the half of the rule that is easy to get wrong. The writer
// DOES strip `$currentObject/` when an association path follows, so flagging
// every variable root reports a page that builds cleanly. Measured, not assumed:
// `$currentObject/Bench.Order_Customer/Name` executed and built at 0 errors.
func TestValidateWidgetParamPaths_AcceptsCurrentObjectOnAnAssociationPath(t *testing.T) {
	v := ValidateWidgetParamPaths(pageWith(paramWidget("dt", "ContentParams",
		varPath("currentObject", "Shop.Order_Customer", "Name"))))
	if len(v) != 0 {
		t.Errorf("reported a shape the writer resolves: %v", v)
	}
}

// CaptionParams is the same position under another name, and a check that
// covered only one of them would leave the other silent.
func TestValidateWidgetParamPaths_CoversCaptionParams(t *testing.T) {
	v := ValidateWidgetParamPaths(pageWith(
		paramWidget("btn", "CaptionParams", varPath("Order", "OrderNo"))))
	if len(v) != 1 {
		t.Fatalf("got %d violations, want 1: %v", len(v), v)
	}
}

// CONTROL: the spellings that are correct must stay silent. An association PATH
// is legal here — it is an attribute reached over an association, which the
// writer stores as AttributeRef plus steps — so only the VARIABLE root is wrong.
func TestValidateWidgetParamPaths_AcceptsWhatIsLegal(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"a bare attribute", &ast.IdentifierExpr{Name: "Name"}},
		{"an association hop then an attribute", &ast.IdentifierExpr{Name: "Shop.Order_Customer/Name"}},
		{"a bare variable with no path", &ast.VariableExpr{Name: "Customer"}},
		{"nothing at all", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if v := ValidateWidgetParamPaths(pageWith(
				paramWidget("dt", "ContentParams", tc.value))); len(v) != 0 {
				t.Errorf("reported a legal parameter: %v", v)
			}
		})
	}
}

// It needs no project, which is the point of putting it in the unconditional
// pass: `mxcli check page.mdl` would otherwise stay silent on a mistake it can
// see from the statement alone.
func TestValidateWidgetParamPaths_NeedsNoProject(t *testing.T) {
	v := ValidateProgram(pageWith(
		paramWidget("dt", "ContentParams", varPath("Customer", "Name"))), "")
	var found bool
	for _, x := range v {
		if x.RuleID == "MDL-WIDGET24" {
			found = true
		}
	}
	if !found {
		t.Errorf("MDL-WIDGET24 did not fire in the no-project pass: %v", v)
	}
}

// ---------------------------------------------------------------------------
// #1049 — XPath constraint members
// ---------------------------------------------------------------------------

func dbGrid(entityQN, where string) *ast.WidgetV3 {
	return &ast.WidgetV3{
		Type: "datagrid", Name: "g",
		Properties: map[string]any{
			"DataSource": &ast.DataSourceV3{
				Type: "database", Reference: entityQN, Where: where,
			},
		},
	}
}

func TestValidateXPathMembers_ReportsAMemberThatIsNeither(t *testing.T) {
	ctx := memberFixture(t)
	errs := validateXPathMembers(ctx, pageWith(
		dbGrid("Shop.Order", "[Shop.Order_Status = 'Open']")))

	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
	}
	for _, want := range []string{"Shop.Order_Status", "Shop.Order", "CE1613"} {
		if !strings.Contains(errs[0].Error(), want) {
			t.Errorf("error should mention %q: %v", want, errs[0])
		}
	}
}

func TestValidateXPathMembers_ReportsABareAttributeThatIsMissing(t *testing.T) {
	ctx := memberFixture(t)
	errs := validateXPathMembers(ctx, pageWith(
		dbGrid("Shop.Order", "[Nonsense = 'x']")))
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "Nonsense") {
		t.Fatalf("a bare attribute that does not exist went unreported: %v", errs)
	}
}

// CONTROL: every constraint that resolves must stay silent. Each of these is a
// shape a real page uses.
func TestValidateXPathMembers_AcceptsWhatResolves(t *testing.T) {
	ctx := memberFixture(t)
	for _, tc := range []struct{ name, entity, where string }{
		{"an attribute the entity declares", "Shop.Order", "[Status = 'Open']"},
		{"an attribute INHERITED from the generalization", "Shop.Order", "[Code = 'x']"},
		{"an association hop, then the target's attribute",
			"Shop.Order", "[Shop.Order_Customer/Shop.Customer/Name = 'x']"},
		{"no constraint at all", "Shop.Order", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if errs := validateXPathMembers(ctx, pageWith(
				dbGrid(tc.entity, tc.where))); len(errs) != 0 {
				t.Errorf("reported a constraint that resolves: %v", errs)
			}
		})
	}
}

// THE CONTROL that a real false positive produced. An entity the project does
// not have makes the whole constraint unanswerable — reporting from it called a
// perfectly good association missing, on a page written against a module the
// script creates or run against the wrong app.
func TestValidateXPathMembers_SilentWhenTheBaseEntityIsUnknown(t *testing.T) {
	ctx := memberFixture(t)
	for _, where := range []string{
		"[Shop.Order_Customer/Shop.Customer/Name = 'x']", // a REAL association
		"[Anything = 'x']",
		"[Made.Up_Thing = 'x']",
	} {
		if errs := validateXPathMembers(ctx, pageWith(
			dbGrid("Nowhere.Missing", where))); len(errs) != 0 {
			t.Errorf("reported against an entity the project does not have: %v", errs)
		}
	}
}

// THE REGRESSION this check shipped with, reported against a real app
// (ako/mxcli-sudoku FINDINGS #57). An association a specialization has only
// through its GENERALIZATION was called missing on a page mxbuild builds at 0
// errors — measured on 11.13.0: `Administration.Account` (extends System.User)
// constrained on `System.UserRoles` (declared from System.User).
//
// It landed because a helper documented as "returns false when it cannot
// establish the answer" gained a second caller that read false as evidence. The
// entity's own attributes were fine throughout, because that path walks the
// chain; only the association path compared two names.
func TestValidateXPathMembers_FollowsGeneralizationForAssociations(t *testing.T) {
	ctx := memberFixture(t)
	for _, tc := range []struct{ name, where string }{
		{"an inherited association as a bare hop", "[not(Shop.Base_Region)]"},
		{"an inherited association, then the target's attribute",
			"[Shop.Base_Region/Shop.Region/RegionName != '']"},
		// Same defect in the other spelling: an association that LEAVES its
		// module is stored in CrossAssociations, which the lookup never read.
		{"a cross-module association", "[Shop.Order_Bin/Warehouse.Bin/BinCode != '']"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if errs := validateXPathMembers(ctx, pageWith(
				dbGrid("Shop.Order", tc.where))); len(errs) != 0 {
				t.Errorf("reported an association the entity really has: %v", errs)
			}
		})
	}
}

// THE CONTROL for the fix above, and the reason it is a chain walk rather than
// "stop reporting associations". Each of these must still be reported, or the
// rule has been disabled rather than corrected.
func TestValidateXPathMembers_StillReportsAnAssociationTheEntityLacks(t *testing.T) {
	ctx := memberFixture(t)
	for _, tc := range []struct{ name, entity, where string }{
		// Real association, wrong entity: Order_Customer runs Order↔Customer and
		// Region is neither, nor a specialization of either.
		{"an association of some other entity", "Shop.Region",
			"[Shop.Order_Customer/Shop.Customer/Name = 'x']"},
		// No association of that name in a module that WAS read.
		{"a name no association has", "Shop.Order", "[Shop.Order_Status = 'Open']"},
		// The generalization direction is one-way: Base does not get Order's.
		{"a SPECIALIZATION's association, from the generalization", "Shop.Base",
			"[Shop.Order_Customer/Shop.Customer/Name = 'x']"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if errs := validateXPathMembers(ctx, pageWith(
				dbGrid(tc.entity, tc.where))); len(errs) != 1 {
				t.Errorf("did not report an association the entity lacks: %v", errs)
			}
		})
	}
}

// The other axis of could-not-establish, which the base-entity guard does not
// cover. Shop.Imported's generalization lives in a module the fixture refuses to
// read, so an association it might inherit is UNKNOWABLE — and the entity itself
// is known, so the existing guard lets it through to be reported.
//
// Collapse assocUnknown into a reportable state and this fires.
func TestValidateXPathMembers_SilentWhenTheChainCannotBeWalked(t *testing.T) {
	ctx := memberFixture(t)
	if errs := validateXPathMembers(ctx, pageWith(
		dbGrid("Shop.Imported", "[Shop.Order_Customer/Shop.Customer/Name = 'x']"))); len(errs) != 0 {
		t.Errorf("reported past a generalization the backend could not read: %v", errs)
	}

	// CONTROL: the silence is about the CHAIN, not about the entity. A name no
	// association has needs no chain walk to rule out, and is still reported.
	if errs := validateXPathMembers(ctx, pageWith(
		dbGrid("Shop.Imported", "[Shop.Nothing_Here = 'x']"))); len(errs) != 1 {
		t.Errorf("a name that exists nowhere went unreported: %v", errs)
	}
}

// Resolving the hop is not only about what is reported: it types the far end, so
// the steps AFTER an inherited association are checked at all. Before the chain
// walk the hop yielded nothing and everything past it was silent.
func TestResolveAssociationFrom_TypesTheFarEndThroughTheChain(t *testing.T) {
	ctx := memberFixture(t)
	for _, tc := range []struct{ assoc, from, want string }{
		{"Shop.Base_Region", "Shop.Order", "Shop.Region"}, // inherited, forward
		{"Shop.Base_Region", "Shop.Region", "Shop.Base"},  // and from the far end
		{"Shop.Order_Bin", "Shop.Order", "Warehouse.Bin"}, // cross-module
		{"Shop.Order_Customer", "Shop.Order", "Shop.Customer"},
	} {
		got, res := resolveAssociationFrom(ctx, tc.assoc, tc.from)
		if res != assocResolved || got != tc.want {
			t.Errorf("%s from %s = (%q, %v), want (%q, assocResolved)",
				tc.assoc, tc.from, got, res, tc.want)
		}
	}

	// And the three ways it does not resolve stay distinguishable — the whole
	// point of the type. A boolean here is what produced the false positive.
	for _, tc := range []struct {
		assoc, from string
		want        assocResolution
	}{
		{"Shop.Order_Status", "Shop.Order", assocMissing},
		{"Nowhere.Thing", "Shop.Order", assocMissing},
		{"Shop.Order_Customer", "Shop.Region", assocNotAnEnd},
		{"Shop.Order_Customer", "Shop.Imported", assocUnknown},
	} {
		if _, res := resolveAssociationFrom(ctx, tc.assoc, tc.from); res != tc.want {
			t.Errorf("%s from %s = %v, want %v", tc.assoc, tc.from, res, tc.want)
		}
	}
}

// Only a DATABASE source names its entity in the statement. Anything else
// carries it elsewhere, so the constraint is left unchecked rather than checked
// against the wrong entity.
func TestValidateXPathMembers_OnlyChecksADatabaseSource(t *testing.T) {
	ctx := memberFixture(t)
	w := &ast.WidgetV3{
		Type: "datagrid", Name: "g",
		Properties: map[string]any{
			"DataSource": &ast.DataSourceV3{
				Type: "association", Reference: "Shop.Order", Where: "[Nonsense = 'x']",
			},
		},
	}
	if errs := validateXPathMembers(ctx, pageWith(w)); len(errs) != 0 {
		t.Errorf("checked a non-database source: %v", errs)
	}
}

// A constraint mxcli cannot parse is skipped, not reported. The XPath parse runs
// with ANTLR's error listeners removed and can hand back a tree that quietly
// omits part of its input, so reporting from a partial tree means an error about
// text nobody wrote.
func TestValidateXPathMembers_SkipsAnUnparseableConstraint(t *testing.T) {
	ctx := memberFixture(t)
	for _, where := range []string{"[", "[[[", "[ = = ]"} {
		if errs := validateXPathMembers(ctx, pageWith(
			dbGrid("Shop.Order", where))); len(errs) != 0 {
			t.Errorf("reported from a constraint it could not read (%q): %v", where, errs)
		}
	}
}
