// SPDX-License-Identifier: Apache-2.0

package exprcatalog

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/exprcheck"
)

// seeded returns a Reader over a real catalog database holding a small Shop
// model. A real database rather than a stub: the point of this package is that
// the SQL matches the schema, which a stub cannot check.
func seeded(t *testing.T) *Reader {
	t.Helper()
	cat, err := catalog.New()
	if err != nil {
		t.Fatalf("catalog.New: %v", err)
	}
	t.Cleanup(func() { cat.Close() })

	db := cat.CatalogDB()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}

	exec(`INSERT INTO attributes_data (Id, Name, EntityQualifiedName, DataType, EnumerationQualifiedName)
	      VALUES ('a1', 'Name', 'Shop.Order', 'String', ''),
	             ('a2', 'Quantity', 'Shop.Order', 'Integer', ''),
	             ('a3', 'Status', 'Shop.Order', 'Enumeration', 'Shop.OrderStatus'),
	             ('a4', 'Code', 'Shop.Order', 'AutoNumber', ''),
	             ('a5', 'Secret', 'Shop.Order', 'HashedString', ''),
	             ('a6', 'Name', 'Shop.Customer', 'String', '')`)

	exec(`INSERT INTO enumeration_values_data (Id, EnumerationQualifiedName, Name, Ordinal)
	      VALUES ('v1', 'Shop.OrderStatus', 'Open', 0),
	             ('v2', 'Shop.OrderStatus', 'Shipped', 1),
	             ('v3', 'Shop.OrderStatus', 'Closed', 2)`)

	exec(`INSERT INTO microflows_data (Id, QualifiedName, MicroflowType, ReturnType)
	      VALUES ('m1', 'Shop.ACT_Total', 'MICROFLOW', 'Decimal'),
	             ('m2', 'Shop.ACT_Find', 'MICROFLOW', 'Object:Shop.Order'),
	             ('m3', 'Shop.ACT_Log', 'MICROFLOW', 'Void'),
	             ('m4', 'Shop.NF_Refresh', 'NANOFLOW', 'Boolean')`)

	exec(`INSERT INTO microflow_parameters_data (Id, MicroflowQualifiedName, Name, ParameterType, Ordinal)
	      VALUES ('p1', 'Shop.ACT_Total', 'Order', 'Object:Shop.Order', 0),
	             ('p2', 'Shop.ACT_Total', 'Discount', 'Decimal', 1),
	             ('p3', 'Shop.ACT_Total', 'Status', 'Enumeration:Shop.OrderStatus', 2)`)

	exec(`INSERT INTO associations_data (Id, QualifiedName, FromEntity, ToEntity)
	      VALUES ('as1', 'Shop.Order_Customer', 'Shop.Order', 'Shop.Customer'),
	             ('as2', 'Shop.Broken', 'Shop.Order', '')`)

	r, err := Load(db)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return r
}

// TestReaderSatisfiesTheSeam is the point of the package: before it, nothing
// implemented CatalogReader, so exprcheck ran with a nil Catalog and every
// semantic rule was skipped.
func TestReaderSatisfiesTheSeam(t *testing.T) {
	var _ exprcheck.CatalogReader = seeded(t)
}

func TestAttributeKind(t *testing.T) {
	r := seeded(t)
	tests := []struct {
		entity, attr string
		want         exprcheck.TypeKind
		found        bool
	}{
		{"Shop.Order", "Name", exprcheck.KindString, true},
		{"Shop.Order", "Quantity", exprcheck.KindInteger, true},
		{"Shop.Order", "Status", exprcheck.KindEnumeration, true},
		// AutoNumber is a runtime-assigned Long, HashedString a String with
		// different storage — both are ordinary types to an expression.
		{"Shop.Order", "Code", exprcheck.KindLong, true},
		{"Shop.Order", "Secret", exprcheck.KindString, true},
		// Same attribute name on another entity must not answer for this one.
		{"Shop.Customer", "Quantity", exprcheck.KindUnknown, false},
		{"Shop.Missing", "Name", exprcheck.KindUnknown, false},
	}
	for _, tc := range tests {
		got, ok := r.AttributeKind(tc.entity, tc.attr)
		if got != tc.want || ok != tc.found {
			t.Errorf("AttributeKind(%q, %q) = (%v, %v), want (%v, %v)",
				tc.entity, tc.attr, got, ok, tc.want, tc.found)
		}
	}
}

func TestAttributeEnumQN(t *testing.T) {
	r := seeded(t)
	if qn, ok := r.AttributeEnumQN("Shop.Order", "Status"); !ok || qn != "Shop.OrderStatus" {
		t.Errorf("got (%q, %v), want Shop.OrderStatus", qn, ok)
	}
	// A non-enumeration attribute has no enum, and must report that rather than
	// an empty string that reads as one.
	if qn, ok := r.AttributeEnumQN("Shop.Order", "Name"); ok {
		t.Errorf("a String attribute reported enum %q", qn)
	}
}

func TestEnumCases(t *testing.T) {
	r := seeded(t)
	got, ok := r.EnumCases("Shop.OrderStatus")
	if !ok {
		t.Fatal("the enumeration's cases were not found")
	}
	want := []string{"Open", "Shipped", "Closed"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v — cases must come back in model order", got, want)
		}
	}
	if _, ok := r.EnumCases("Shop.Nope"); ok {
		t.Error("an unknown enumeration reported cases")
	}
}

// TestEnumCasesReturnsACopy pins that a caller cannot corrupt the index. The
// reader is loaded once and serves every expression in the run, so a slice
// handed out by reference and then sorted in place would change the answer for
// everything after it.
func TestEnumCasesReturnsACopy(t *testing.T) {
	r := seeded(t)
	first, _ := r.EnumCases("Shop.OrderStatus")
	first[0] = "MUTATED"
	second, _ := r.EnumCases("Shop.OrderStatus")
	if second[0] != "Open" {
		t.Errorf("the index was mutated through a returned slice: got %q", second[0])
	}
}

func TestMicroflowReturn(t *testing.T) {
	r := seeded(t)
	tests := []struct {
		qn    string
		want  exprcheck.TypeKind
		found bool
	}{
		{"Shop.ACT_Total", exprcheck.KindDecimal, true},
		{"Shop.ACT_Find", exprcheck.KindObject, true},
		// Nanoflows share the microflows view and must resolve the same way.
		{"Shop.NF_Refresh", exprcheck.KindBoolean, true},
		// Void has no kind that says "no value", so it reports not-found rather
		// than inventing one.
		{"Shop.ACT_Log", exprcheck.KindUnknown, false},
		{"Shop.Absent", exprcheck.KindUnknown, false},
	}
	for _, tc := range tests {
		got, ok := r.MicroflowReturn(tc.qn)
		if got != tc.want || ok != tc.found {
			t.Errorf("MicroflowReturn(%q) = (%v, %v), want (%v, %v)", tc.qn, got, ok, tc.want, tc.found)
		}
	}
}

func TestMicroflowParam(t *testing.T) {
	r := seeded(t)
	if got, ok := r.MicroflowParam("Shop.ACT_Total", "Order"); !ok || got != exprcheck.KindObject {
		t.Errorf("got (%v, %v), want an Object parameter", got, ok)
	}
	// Callers hold variable names with the sigil; accept either spelling.
	if got, ok := r.MicroflowParam("Shop.ACT_Total", "$Discount"); !ok || got != exprcheck.KindDecimal {
		t.Errorf("got (%v, %v), want a Decimal parameter for the $-prefixed name", got, ok)
	}
	if _, ok := r.MicroflowParam("Shop.ACT_Total", "Nope"); ok {
		t.Error("an unknown parameter resolved")
	}
	// A parameter of one flow must not answer for another.
	if _, ok := r.MicroflowParam("Shop.ACT_Find", "Order"); ok {
		t.Error("a parameter leaked across microflows")
	}
}

// TestLoadOnEmptyCatalogIsNotAnError pins the degradation the package promises:
// an empty or partial catalog yields a reader that answers "unknown", which
// exprcheck reads as catch-less. It must not fail the caller.
func TestLoadOnEmptyCatalogIsNotAnError(t *testing.T) {
	cat, err := catalog.New()
	if err != nil {
		t.Fatalf("catalog.New: %v", err)
	}
	defer cat.Close()

	r, err := Load(cat.CatalogDB())
	if err != nil {
		t.Fatalf("Load on an empty catalog: %v", err)
	}
	if _, ok := r.AttributeKind("Shop.Order", "Name"); ok {
		t.Error("an empty catalog answered a lookup")
	}
}

func TestLoadRejectsANilCatalog(t *testing.T) {
	if _, err := Load(nil); err == nil {
		t.Error("Load(nil) succeeded; a nil catalog would silently disable every check")
	}
}

// TestAssociationTarget pins the hop resolution an expression needs. Unlike
// XPath, `$Order/Shop.Order_Customer/Name` does not name the intermediate
// entity, so the association is the only thing that says where the path lands.
func TestAssociationTarget(t *testing.T) {
	r := seeded(t)
	tests := []struct {
		assoc, from, want string
		ok                bool
	}{
		{"Shop.Order_Customer", "Shop.Order", "Shop.Customer", true},
		// Either end: a Mendix expression follows an association both ways and
		// the path looks identical.
		{"Shop.Order_Customer", "Shop.Customer", "Shop.Order", true},
		// Starting somewhere the association does not touch is not a hop.
		{"Shop.Order_Customer", "Shop.Elsewhere", "", false},
		{"Shop.Order_Customer", "", "", false},
		{"Shop.Nope", "Shop.Order", "", false},
		// An association with an unrecorded end is left out entirely rather
		// than resolving to half an answer.
		{"Shop.Broken", "Shop.Order", "", false},
	}
	for _, tc := range tests {
		got, ok := r.AssociationTarget(tc.assoc, tc.from)
		if got != tc.want || ok != tc.ok {
			t.Errorf("AssociationTarget(%q, %q) = (%q, %v), want (%q, %v)",
				tc.assoc, tc.from, got, ok, tc.want, tc.ok)
		}
	}
}

// TestMicroflowParamEnumQN pins which enumeration an enumeration parameter
// takes: the kind alone cannot tell a value of Shop.OrderStatus from one of
// another enumeration, and passing the wrong one is CE0117 at the call.
func TestMicroflowParamEnumQN(t *testing.T) {
	r := seeded(t)
	if got, ok := r.MicroflowParamEnumQN("Shop.ACT_Total", "$Status"); !ok || got != "Shop.OrderStatus" {
		t.Errorf("got (%q, %v), want Shop.OrderStatus", got, ok)
	}
	if k, ok := r.MicroflowParam("Shop.ACT_Total", "Status"); !ok || k != exprcheck.KindEnumeration {
		t.Errorf("got (%v, %v), want an Enumeration parameter", k, ok)
	}
	// An object parameter carries an entity after the colon, not an enumeration.
	if got, ok := r.MicroflowParamEnumQN("Shop.ACT_Total", "Order"); ok {
		t.Errorf("an object parameter named enumeration %q", got)
	}
}

// TestDeclareFlowReplacesTheSignature: a flow the script creates or replaces
// is checked against the parameters the script gives it. A parameter the new
// signature drops must stop resolving, and one of unknown kind is left out
// rather than kept at its stored type.
func TestDeclareFlowReplacesTheSignature(t *testing.T) {
	r := seeded(t)
	r.DeclareFlow("Shop.ACT_Total", []FlowParam{
		{Name: "Discount", Kind: exprcheck.KindInteger},
		{Name: "Code", Kind: exprcheck.KindEnumeration, EnumQN: "Shop.OrderStatus"},
		{Name: "Order", Kind: exprcheck.KindUnknown},
	})
	if k, ok := r.MicroflowParam("Shop.ACT_Total", "Discount"); !ok || k != exprcheck.KindInteger {
		t.Errorf("Discount = (%v, %v), want the redeclared Integer", k, ok)
	}
	if q, ok := r.MicroflowParamEnumQN("Shop.ACT_Total", "Code"); !ok || q != "Shop.OrderStatus" {
		t.Errorf("Code enumeration = (%q, %v)", q, ok)
	}
	for _, gone := range []string{"Status", "Order"} {
		if _, ok := r.MicroflowParam("Shop.ACT_Total", gone); ok {
			t.Errorf("%s still resolves after the signature was replaced", gone)
		}
	}
	if _, ok := r.MicroflowParamEnumQN("Shop.ACT_Total", "Status"); ok {
		t.Error("the dropped enumeration parameter kept its enumeration")
	}
	// A brand-new flow gets its parameters too.
	r.DeclareFlow("Shop.ACT_New", []FlowParam{{Name: "$Qty", Kind: exprcheck.KindInteger}})
	if k, ok := r.MicroflowParam("Shop.ACT_New", "Qty"); !ok || k != exprcheck.KindInteger {
		t.Errorf("new flow's Qty = (%v, %v)", k, ok)
	}
}
