// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// idxEntity builds a persistable entity with named attributes and indexes
// given as the attribute names each index covers, in order.
func idxEntity(name string, attrs []string, indexes ...[]string) *domainmodel.Entity {
	e := &domainmodel.Entity{Name: name, Persistable: true}
	byName := map[string]model.ID{}
	for i, a := range attrs {
		id := model.ID(name + "-attr-" + a)
		at := &domainmodel.Attribute{Name: a}
		at.ID = id
		byName[a] = id
		e.Attributes = append(e.Attributes, at)
		_ = i
	}
	for n, cols := range indexes {
		ix := &domainmodel.Index{Name: "IX" + string(rune('A'+n))}
		for _, c := range cols {
			ix.Attributes = append(ix.Attributes, &domainmodel.IndexAttribute{AttributeID: byName[c]})
		}
		e.Indexes = append(e.Indexes, ix)
	}
	return e
}

func TestUnindexedConstraintRule_Metadata(t *testing.T) {
	r := NewUnindexedConstraintRule()
	if r.ID() != "MPR019" {
		t.Errorf("ID = %q, want MPR019", r.ID())
	}
	if r.Category() != "performance" {
		t.Errorf("Category = %q, want performance", r.Category())
	}
	// Info, not warning: whether an index is worth having depends on table size
	// and write rate, which the model does not know.
	if r.DefaultSeverity() != linter.SeverityInfo {
		t.Errorf("Severity = %v, want info", r.DefaultSeverity())
	}
}

// RequiredCatalogMode must be declared: the rule reads xpath_expressions, which
// only REFRESH CATALOG FULL writes. Without it lint builds a fast catalog, the
// table is empty and the rule silently reports nothing — the MPR017 trap
// (ako/mxcli#2403), which linter.CatalogRequirer's own doc warns about.
func TestUnindexedConstraintRule_RequiresFullCatalog(t *testing.T) {
	req, ok := any(NewUnindexedConstraintRule()).(linter.CatalogRequirer)
	if !ok {
		t.Fatal("rule does not implement linter.CatalogRequirer, so lint builds a fast catalog and the rule finds nothing")
	}
	if got := req.RequiredCatalogMode(); got != linter.CatalogFull {
		t.Errorf("RequiredCatalogMode = %v, want CatalogFull (it reads xpath_expressions)", got)
	}
}

// An index covers a query on its FIRST attribute only. Getting this wrong is
// the mistake that would make the rule wrong rather than merely noisy: a
// composite index on (Status, Date) does nothing for a query on Date alone.
func TestLeadingIndexAttributes(t *testing.T) {
	e := idxEntity("Order",
		[]string{"Number", "Status", "Date", "Total"},
		[]string{"Number"},         // single-column
		[]string{"Status", "Date"}, // composite: Status leads
	)
	got := leadingIndexAttributes(e)

	for _, want := range []string{"number", "status"} {
		if !got[want] {
			t.Errorf("leadingIndexAttributes missing %q", want)
		}
	}
	if got["date"] {
		t.Error("Date is the SECOND attribute of a composite index and is not covered by it")
	}
	if got["total"] {
		t.Error("Total is in no index")
	}
}

// A system-member index segment ("CreatedDate" / "ChangedDate") has nothing
// behind its AttributeID (mendixlabs/mxcli#1401), so it must not resolve to an
// empty name and mark "" as covered.
func TestLeadingIndexAttributes_SystemMemberSegment(t *testing.T) {
	e := idxEntity("Order", []string{"Number"})
	ix := &domainmodel.Index{Name: "IXAudit"}
	ix.Attributes = append(ix.Attributes, &domainmodel.IndexAttribute{Type: "CreatedDate"})
	e.Indexes = append(e.Indexes, ix)

	got := leadingIndexAttributes(e)
	if got[""] {
		t.Error(`a system-member segment resolved to the empty name and marked "" as indexed`)
	}
	if len(got) != 0 {
		t.Errorf("leadingIndexAttributes = %v, want none (the only index leads with a system member)", got)
	}
}

// Which attributes an XPath constrains.
func TestConstraintAttributeNames(t *testing.T) {
	attrs := map[string]string{"status": "Status", "total": "Total", "isconfidential": "IsConfidential"}
	for _, tc := range []struct {
		name       string
		constraint string
		want       []string
	}{
		{"simple comparison", `[Status = 'Open']`, []string{"status"}},
		{"two attributes", `[Status = 'Open' and Total > 100]`, []string{"status", "total"}},
		{"a function call around it", `[IsConfidential = false()]`, []string{"isconfidential"}},
		{"no predicate at all", ``, nil},
		{
			// An attribute reached ACROSS an association belongs to the other
			// entity; indexing it is that entity's business, not the target's.
			"across an association",
			`[Shop.Order_Customer/Shop.Customer/Name = 'x']`, nil,
		},
		{"a name that is not an attribute of this entity", `[Nonsense = 1]`, nil},
		{"unparseable is skipped, not guessed", `[ this is not xpath (((`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := constraintAttributeNames(tc.constraint, attrs)
			if len(got) != len(tc.want) {
				t.Fatalf("constraintAttributeNames(%q) = %v, want %v", tc.constraint, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("constraintAttributeNames(%q) = %v, want %v", tc.constraint, got, tc.want)
				}
			}
		})
	}
}

// A non-persistable entity has no table, so nothing to index.
func TestIndexableAttributes_SkipsNonPersistable(t *testing.T) {
	e := idxEntity("View", []string{"Status"})
	e.Persistable = false
	if got := leadingIndexAttributes(e); len(got) != 0 {
		t.Fatalf("got %v, want none for a non-persistable entity", got)
	}
	if indexableEntity(e) {
		t.Error("indexableEntity should be false for a non-persistable entity")
	}
	e.Persistable = true
	if !indexableEntity(e) {
		t.Error("indexableEntity should be true for a persistable entity")
	}
}

func TestUnindexedConstraintRule_NoReaderReportsNothing(t *testing.T) {
	if got := NewUnindexedConstraintRule().Check(linter.NewLintContextFromDB(nil)); len(got) != 0 {
		t.Fatalf("got %d findings with no reader, want 0", len(got))
	}
}

// A unique attribute is treated as covered.
//
// This records an open question rather than a settled fact: Mendix's "unique"
// is a validation rule, and whether the database schema also carries a unique
// index for it was not established. Without this exclusion the only findings on
// the 900-unit Studio Pro-authored testdata/testapp were two attributes both
// declared `unique`, so the exclusion is what keeps every finding the rule
// makes a true one. See uniqueAttributes for how to settle it; if Mendix does
// not index unique attributes, this test and the exclusion both go.
func TestUniqueAttributesAreTreatedAsCovered(t *testing.T) {
	e := idxEntity("Customer", []string{"CustomerId", "Name"})
	vr := &domainmodel.ValidationRule{
		AttributeID: e.Attributes[0].ID,
		Type:        "Unique",
		Rule:        &domainmodel.UniqueValidationRuleInfo{},
	}
	e.ValidationRules = append(e.ValidationRules, vr)

	got := uniqueAttributes(e)
	if !got["customerid"] {
		t.Error("a Unique validation rule did not mark its attribute unique")
	}
	if got["name"] {
		t.Error("an attribute with no validation rule was marked unique")
	}
}

// An attribute reference arrives in two different forms, so both must resolve.
//
// Measured on testdata/testapp: ValidationRule.AttributeID holds a QUALIFIED
// NAME ("Mappings.Order.OrderId"), while an index segment written by mxcli
// holds the attribute's UUID. Assuming the UUID made the unique-attribute
// exclusion match nothing at all — which looked exactly like the exclusion not
// being needed, since the rule simply kept reporting.
func TestResolveAttributeRef_AcceptsUUIDAndQualifiedName(t *testing.T) {
	e := idxEntity("Order", []string{"OrderId", "OrderDate"})
	byID, names := attributeRefIndex(e)

	if got := resolveAttributeRef(e.Attributes[0].ID, byID, names); got != "orderid" {
		t.Errorf("by UUID = %q, want orderid", got)
	}
	if got := resolveAttributeRef(model.ID("Mappings.Order.OrderId"), byID, names); got != "orderid" {
		t.Errorf("by qualified name = %q, want orderid", got)
	}
	if got := resolveAttributeRef(model.ID("Mappings.Order.NotAnAttribute"), byID, names); got != "" {
		t.Errorf("unknown name = %q, want empty", got)
	}
	if got := resolveAttributeRef("", byID, names); got != "" {
		t.Errorf("empty ref = %q, want empty", got)
	}
}

// ako/mxcli#2502: MPR019 reported 39 findings on a frontend app, all 39 on an
// external entity. An external entity is stored persistable, but its table lives
// in the publishing app, so there is no index here to add. A local persistable
// entity is still indexable (the control).
func TestIndexableEntity_ExternalEntityHasNoTableHere(t *testing.T) {
	e := idxEntity("Calendar", []string{"month"})
	e.Persistable = true
	if !indexableEntity(e) {
		t.Fatal("control: a local persistable entity is indexable")
	}
	e.Source = "Rest$ODataRemoteEntitySource"
	if indexableEntity(e) {
		t.Error("an external entity has no table in this app, so it is not indexable")
	}
}
