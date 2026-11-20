// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/dmlayout"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ent builds an entity at a position with n attributes, so a test can state a
// box's height through the thing that actually drives it.
func ent(name string, x, y, attrs int) *domainmodel.Entity {
	e := &domainmodel.Entity{Name: name, Location: model.Point{X: x, Y: y}}
	for i := 0; i < attrs; i++ {
		e.Attributes = append(e.Attributes, &domainmodel.Attribute{Name: "A"})
	}
	return e
}

// names flattens clusters to compare them without depending on pointer order.
func names(clusters [][]*domainmodel.Entity) []string {
	var out []string
	for _, c := range clusters {
		var n []string
		for _, e := range c {
			n = append(n, e.Name)
		}
		out = append(out, strings.Join(n, "+"))
	}
	return out
}

func TestOverlappingEntitiesRule_Metadata(t *testing.T) {
	r := NewOverlappingEntitiesRule()
	if r.ID() != "MPR016" {
		t.Errorf("ID = %q, want MPR016", r.ID())
	}
	if r.Name() != "OverlappingEntities" {
		t.Errorf("Name = %q, want OverlappingEntities", r.Name())
	}
	// Same category and severity as MPR008, its microflow-side sibling.
	if r.Category() != "correctness" {
		t.Errorf("Category = %q, want correctness", r.Category())
	}
	if r.DefaultSeverity() != linter.SeverityWarning {
		t.Errorf("Severity = %v, want warning", r.DefaultSeverity())
	}
}

// Two boxes that genuinely cover each other: the report's "the payment attempt
// covers the bottom of the cart".
func TestOverlappingEntityClusters_ReportsARealOverlap(t *testing.T) {
	got := overlappingEntityClusters([]*domainmodel.Entity{
		ent("Cart", 0, 0, 0),
		ent("PaymentAttempt", 100, 30, 0),
	})
	if want := []string{"Cart+PaymentAttempt"}; len(got) != 1 || names(got)[0] != want[0] {
		t.Fatalf("clusters = %v, want %v", names(got), want)
	}
}

// The discriminating case for this rule's whole design.
//
// dmlayout.Rect.overlaps keeps boxes `clearance` (20px) apart, because that is
// what PLACING needs: boxes 10px apart are unreadable and the placer should
// separate them. A CHECK may not use that predicate. Two entities 10px apart
// are badly laid out, but they do not overlap, and a finding that said they did
// would be false — so the rule uses Intersects, which has no margin.
//
// If this test fails because the rule started reporting, the rule has been
// switched to the placer's predicate and every tight-but-legal diagram in every
// project now produces findings.
func TestOverlappingEntityClusters_IgnoresBoxesThatMerelySitClose(t *testing.T) {
	a, b := ent("Cart", 0, 0, 0), ent("Coupon", 170, 0, 0)

	// The control: the placer's predicate DOES fire on this pair, which is why
	// the rule needs its own.
	if !dmlayout.EntityBox(a).TooClose(dmlayout.EntityBox(b)) {
		t.Fatal("precondition: the placer's predicate should consider a 10px gap too close")
	}
	if dmlayout.EntityBox(a).Intersects(dmlayout.EntityBox(b)) {
		t.Fatal("precondition: a 10px gap is not an intersection")
	}

	if got := overlappingEntityClusters([]*domainmodel.Entity{a, b}); len(got) != 0 {
		t.Fatalf("clusters = %v, want none for boxes that only sit close", names(got))
	}
}

// The report's worst case: "All 26 entities sit at the same position on the
// diagram, one on top of another." Reporting that as 26*25/2 = 325 pairwise
// findings would bury the one fact worth knowing, so a connected cluster is one
// finding however large it is.
func TestOverlappingEntityClusters_CollapsesAPileIntoOneCluster(t *testing.T) {
	var ents []*domainmodel.Entity
	for i := 0; i < 26; i++ {
		ents = append(ents, ent(string(rune('A'+i)), 0, 0, 0))
	}
	got := overlappingEntityClusters(ents)
	if len(got) != 1 {
		t.Fatalf("got %d clusters, want 1 for a pile of 26", len(got))
	}
	if len(got[0]) != 26 {
		t.Fatalf("cluster holds %d entities, want 26", len(got[0]))
	}
}

// Two separate collisions are two findings, not one cluster of four.
func TestOverlappingEntityClusters_SeparatesIndependentOverlaps(t *testing.T) {
	got := overlappingEntityClusters([]*domainmodel.Entity{
		ent("Cart", 0, 0, 0),
		ent("Coupon", 50, 10, 0),
		ent("Order", 1000, 1000, 0),
		ent("OrderLine", 1050, 1010, 0),
	})
	if len(got) != 2 {
		t.Fatalf("clusters = %v, want 2", names(got))
	}
}

// A box's height comes from its attribute count, so a tall entity reaches a
// neighbour that a default-sized one would clear. This is the mendixlabs/mxcli#1479
// lesson — a row pitch alone cannot place a 20-attribute entity — and it is the
// test that proves the rule measures boxes through dmlayout's estimator rather
// than assuming one size for every entity.
func TestOverlappingEntityClusters_HeightFollowsTheAttributeCount(t *testing.T) {
	// Control: with no attributes the upper box is 60px tall and clears y=100.
	if got := overlappingEntityClusters([]*domainmodel.Entity{
		ent("Order", 0, 0, 0),
		ent("OrderLine", 0, 100, 0),
	}); len(got) != 0 {
		t.Fatalf("control: clusters = %v, want none", names(got))
	}

	// The same pair, with 20 attributes on the upper entity: 34 + 20*16 = 354px
	// tall, so it now covers the box at y=100.
	if got := overlappingEntityClusters([]*domainmodel.Entity{
		ent("Order", 0, 0, 20),
		ent("OrderLine", 0, 100, 0),
	}); len(got) != 1 {
		t.Fatalf("clusters = %v, want the tall entity to reach its neighbour", names(got))
	}
}

// Unlike MPR008, this rule does NOT skip the origin. An unpositioned microflow
// activity defaults to (0,0), so MPR008 treats that as "no position recorded";
// entities are placed by dmlayout on every create path, so several entities at
// one position is the pile defect itself, and skipping it would miss the worst
// case in the benchmark that motivated the rule.
func TestOverlappingEntityClusters_DoesNotSkipTheOrigin(t *testing.T) {
	got := overlappingEntityClusters([]*domainmodel.Entity{
		ent("Cart", 0, 0, 0),
		ent("Coupon", 0, 0, 0),
	})
	if len(got) != 1 {
		t.Fatalf("clusters = %v, want the pair at the origin reported", names(got))
	}
}

func TestOverlappingEntityClusters_IgnoresNilAndSingletons(t *testing.T) {
	if got := overlappingEntityClusters(nil); len(got) != 0 {
		t.Fatalf("clusters = %v, want none", names(got))
	}
	if got := overlappingEntityClusters([]*domainmodel.Entity{nil, ent("Cart", 0, 0, 0), nil}); len(got) != 0 {
		t.Fatalf("clusters = %v, want none", names(got))
	}
}

// dmReader supplies domain models the way the production backend does. The
// capability is not in LintReader (see LintContext.DomainModels), so the rule
// reaches it by assertion and this reader is what satisfies it.
type dmReader struct {
	rawUnitReader
	dms     []*domainmodel.DomainModel
	modules []*model.Module
}

func (r dmReader) ListDomainModels() ([]*domainmodel.DomainModel, error) { return r.dms, nil }
func (r dmReader) ListModules() ([]*model.Module, error)                 { return r.modules, nil }

func newDMContext(t *testing.T, r linter.LintReader) *linter.LintContext {
	t.Helper()
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cat.Close() })
	return linter.NewLintContext(cat, r)
}

// End to end: the rule reads the domain model through the context, names both
// entities, and points at the command that repairs it.
func TestOverlappingEntitiesRule_ReportsThroughTheContext(t *testing.T) {
	dm := &domainmodel.DomainModel{
		ContainerID: model.ID("m1"),
		Entities:    []*domainmodel.Entity{ent("Cart", 0, 0, 0), ent("PaymentAttempt", 100, 30, 0)},
	}
	ctx := newDMContext(t, dmReader{
		dms:     []*domainmodel.DomainModel{dm},
		modules: []*model.Module{{Name: "Shop", BaseElement: model.BaseElement{ID: model.ID("m1")}}},
	})

	got := NewOverlappingEntitiesRule().Check(ctx)
	if len(got) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(got), got)
	}
	v := got[0]
	if v.RuleID != "MPR016" {
		t.Errorf("RuleID = %q", v.RuleID)
	}
	if v.Location.Module != "Shop" {
		t.Errorf("Location.Module = %q, want Shop", v.Location.Module)
	}
	for _, want := range []string{"Cart", "PaymentAttempt"} {
		if !strings.Contains(v.Message, want) {
			t.Errorf("message %q does not name %q", v.Message, want)
		}
	}
	if !strings.Contains(v.Suggestion, "--only-overlapping") {
		t.Errorf("suggestion %q does not name the repair command", v.Suggestion)
	}
}

// An excluded module produces nothing, like every other rule.
func TestOverlappingEntitiesRule_HonoursModuleExclusion(t *testing.T) {
	dm := &domainmodel.DomainModel{
		ContainerID: model.ID("m1"),
		Entities:    []*domainmodel.Entity{ent("Cart", 0, 0, 0), ent("PaymentAttempt", 100, 30, 0)},
	}
	ctx := newDMContext(t, dmReader{
		dms:     []*domainmodel.DomainModel{dm},
		modules: []*model.Module{{Name: "Shop", BaseElement: model.BaseElement{ID: model.ID("m1")}}},
	})
	ctx.SetExcludedModules([]string{"Shop"})

	if got := NewOverlappingEntitiesRule().Check(ctx); len(got) != 0 {
		t.Fatalf("got %d violations for an excluded module, want 0", len(got))
	}
}

// A reader that cannot supply domain models yields no findings rather than a
// crash or a rule failure — the same way MPR008 returns nil on a nil reader.
func TestOverlappingEntitiesRule_ReaderWithoutDomainModels(t *testing.T) {
	ctx := newDMContext(t, rawUnitReader{})
	if got := NewOverlappingEntitiesRule().Check(ctx); len(got) != 0 {
		t.Fatalf("got %d violations, want 0", len(got))
	}
	if got := NewOverlappingEntitiesRule().Check(linter.NewLintContextFromDB(nil)); len(got) != 0 {
		t.Fatalf("got %d violations with a nil reader, want 0", len(got))
	}
}

// The System module is skipped, and so is anything from the Marketplace.
//
// This is a regression guard, not a hypothetical. MPR016's first run against
// testdata/pedapp reported "41 entities overlap on the System domain model
// diagram: ConsumedODataConfiguration, Error, FileDocument, HttpHeader,
// HttpMessage, HttpRequest and 35 more" — Mendix's own model, which no user can
// lay out (`mxcli layout` refuses System outright). A rule that reads the MODEL
// does not inherit the notPlatformModule WHERE clause that keeps catalog-based
// rules off platform modules, so it sees them all until it filters.
//
// Note System is NOT identifiable by its Source, which is empty for it exactly
// as for the user's own modules; it has a fixed ID. QUAL002 shipped this same
// bug once, on the same entities.
func TestOverlappingEntitiesRule_SkipsPlatformModules(t *testing.T) {
	const systemModuleID = "00000000-0000-0000-0000-000000000001"

	overlapping := func() []*domainmodel.Entity {
		return []*domainmodel.Entity{ent("Cart", 0, 0, 0), ent("PaymentAttempt", 100, 30, 0)}
	}
	for _, tc := range []struct {
		name   string
		module *model.Module
	}{
		{"System", &model.Module{Name: "System", BaseElement: model.BaseElement{ID: model.ID(systemModuleID)}}},
		{"Marketplace", &model.Module{Name: "Administration", FromAppStore: true, AppStoreVersion: "4.3.2",
			BaseElement: model.BaseElement{ID: model.ID("m1")}}},
		{"MarketplaceByGuidAlone", &model.Module{Name: "Atlas_Core", AppStoreGuid: "abc",
			BaseElement: model.BaseElement{ID: model.ID("m1")}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dm := &domainmodel.DomainModel{ContainerID: tc.module.ID, Entities: overlapping()}
			ctx := newDMContext(t, dmReader{
				dms:     []*domainmodel.DomainModel{dm},
				modules: []*model.Module{tc.module},
			})
			if got := NewOverlappingEntitiesRule().Check(ctx); len(got) != 0 {
				t.Fatalf("got %d violations for %s, want 0: %+v", len(got), tc.name, got)
			}
		})
	}

	// The control: the identical model in a module the user owns IS reported,
	// so the filter above is selective and not just switching the rule off.
	dm := &domainmodel.DomainModel{ContainerID: model.ID("m1"), Entities: overlapping()}
	ctx := newDMContext(t, dmReader{
		dms:     []*domainmodel.DomainModel{dm},
		modules: []*model.Module{{Name: "Shop", BaseElement: model.BaseElement{ID: model.ID("m1")}}},
	})
	if got := NewOverlappingEntitiesRule().Check(ctx); len(got) != 1 {
		t.Fatalf("control: got %d violations for an owned module, want 1", len(got))
	}
}
