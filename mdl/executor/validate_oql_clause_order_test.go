// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// Mendix OQL has two clause orders and the MDL grammar accepts both
// (`oqlQueryTerm`, MDLCatalog.g4). Studio Pro stores the from-first one, so it
// is what `DESCRIBE ENTITY` emits — and extractSelectClause could only read the
// select-first one, so feeding a describe straight back to `check` reported
// `could not parse select clause from OQL query` on a query Mendix itself had
// written.
//
// Every test here pairs the from-first query with its select-first twin as the
// control: the two must be read and judged identically, or the fix is only
// moving the blind spot.

// fromFirst and selectFirst are the same query in the two clause orders.
// `Order` is quoted because the unquoted form is CE0174 (ako/mxcli#2208): a
// well-formed fixture has to be one mxbuild accepts.
const (
	fromFirstOQL   = `from Sales."Order" as o group by o.Number select o.Number as Number, count(o.ID) as Lines`
	selectFirstOQL = `select o.Number as Number, count(o.ID) as Lines from Sales."Order" as o group by o.Number`
)

func TestExtractSelectClause_ReadsBothClauseOrders(t *testing.T) {
	const want = "o.Number as Number, count(o.ID) as Lines"

	if got := extractSelectClause(fromFirstOQL); got != want {
		t.Errorf("from-first: extractSelectClause = %q, want %q", got, want)
	}
	// Control: the order that always worked must be unchanged.
	if got := extractSelectClause(selectFirstOQL); got != want {
		t.Errorf("select-first (control): extractSelectClause = %q, want %q", got, want)
	}
}

func TestExtractSelectClause_FromFirstTerminators(t *testing.T) {
	const cols = "o.Number as Number"
	cases := []struct {
		name string
		oql  string
		want string
	}{
		{
			// The ordinary shape: nothing follows the select list at all.
			name: "runs to the end of the query",
			oql:  `from Sales.Order as o select o.Number as Number`,
			want: cols,
		},
		{
			name: "order by ends the list",
			oql:  `from Sales.Order as o select o.Number as Number order by o.Number limit 10`,
			want: cols,
		},
		{
			name: "limit ends the list",
			oql:  `from Sales.Order as o select o.Number as Number limit 10`,
			want: cols,
		},
		{
			name: "union ends the first query term",
			oql:  `from Sales.Order as o select o.Number as Number union from Sales.Quote as q select q.Number as Number`,
			want: cols,
		},
		{
			// A subquery's own SELECT must not be mistaken for the outer one,
			// which in this order comes AFTER it.
			name: "subquery select is not the outer select",
			oql:  `from (from Sales.Line as l select l.OrderID as OrderID) as t select t.OrderID as Number`,
			want: "t.OrderID as Number",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractSelectClause(tc.oql); got != tc.want {
				t.Errorf("extractSelectClause(%q) = %q, want %q", tc.oql, got, tc.want)
			}
		})
	}
}

// TestExtractSelectClause_OrderAndLimitAreNamesToo is the control for widening
// the terminator set. ORDER/LIMIT/OFFSET end a from-first select list, and they
// are also perfectly ordinary attribute and alias names — so admitting them in
// the SELECT-FIRST order, where they cannot legally appear before the FROM,
// would cut a real query's list in half and then report every remaining column
// against the wrong declared attribute.
func TestExtractSelectClause_OrderAndLimitAreNamesToo(t *testing.T) {
	cases := []struct {
		name, oql, want string
	}{
		{
			name: "select-first: a column named Limit is not a LIMIT clause",
			oql:  `select o.Limit as Limit, o.Number as Number from Sales.Order as o`,
			want: "o.Limit as Limit, o.Number as Number",
		},
		{
			name: "select-first: a column named Order is not an ORDER BY",
			oql:  `select o.Order as Order, o.Number as Number from Sales.Order as o`,
			want: "o.Order as Order, o.Number as Number",
		},
		{
			// Even in from-first, a bare ORDER is a name: only the PHRASE
			// "order by" ends the list.
			name: "from-first: a bare Order alias is not an ORDER BY",
			oql:  `from Sales.Order as o select o.Number as Order`,
			want: "o.Number as Order",
		},
		{
			// A quoted identifier is how a reserved word survives MxBuild, and
			// mxcli passes the quotes through — so the scanner has to skip
			// quoted runs or it matches the keyword inside one.
			name: "from-first: a quoted reserved word in a source position",
			oql:  `from Sales.Order as o select o."Order" as OrderValue, o.Number as Number`,
			want: `o."Order" as OrderValue, o.Number as Number`,
		},
		{
			name: "from-first: a string literal containing a keyword",
			oql:  `from Sales.Order as o select 'order by' as Label, o.Number as Number`,
			want: `'order by' as Label, o.Number as Number`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractSelectClause(tc.oql); got != tc.want {
				t.Errorf("extractSelectClause(%q) = %q, want %q", tc.oql, got, tc.want)
			}
		})
	}
}

// TestFromFirstOQL_IsCheckedNotSkipped is the point of the fix. The reported
// symptom was one bogus error; the unreported half was that MDL030 and MDL072
// stopped running entirely, because both sit behind `if selectClause != ""`.
// A checker that goes quiet is worse than one that complains.
func TestFromFirstOQL_IsCheckedNotSkipped(t *testing.T) {
	// A column with no alias at all — MDL030's reason for existing.
	const missingAlias = `from Sales.Order as o group by o.Number select o.Number, count(o.ID) as Lines`

	ids := oqlRuleIDs(missingAlias)
	if !hasOQLRule(ids, "MDL030") {
		t.Errorf("from-first: MDL030 did not fire on a column with no alias, got %v", ids)
	}
	// Control: the same defect in the order that always worked.
	if ids := oqlRuleIDs(`select o.Number, count(o.ID) as Lines from Sales.Order as o`); !hasOQLRule(ids, "MDL030") {
		t.Errorf("select-first (control): MDL030 did not fire, got %v", ids)
	}
	// Control: a well-formed from-first query is not newly complained about.
	if ids := oqlRuleIDs(fromFirstOQL); len(ids) != 0 {
		t.Errorf("from-first well-formed query now reports %v, want none", ids)
	}
}

func TestFromFirstOQL_NoLongerReportsCouldNotParse(t *testing.T) {
	attrs := []ast.ViewAttribute{
		{Name: "Number", Type: ast.DataType{Kind: ast.TypeString, Length: 200}},
		{Name: "Lines", Type: ast.DataType{Kind: ast.TypeInteger}},
	}
	if v := ValidateOQLTypes(fromFirstOQL, attrs); len(v) != 0 {
		t.Errorf("from-first: unexpected type violations %v", v)
	}

	// And the type rules really do run now, rather than passing vacuously:
	// count() is Integer, so declaring it Decimal must be caught in BOTH orders.
	wrong := []ast.ViewAttribute{
		{Name: "Number", Type: ast.DataType{Kind: ast.TypeString, Length: 200}},
		{Name: "Lines", Type: ast.DataType{Kind: ast.TypeDecimal}},
	}
	for _, c := range []struct{ name, oql string }{
		{"from-first", fromFirstOQL},
		{"select-first (control)", selectFirstOQL},
	} {
		v := ValidateOQLTypes(c.oql, wrong)
		if len(v) != 1 || !strings.Contains(v[0].Message, "declared as Decimal") {
			t.Errorf("%s: want one MDL031 about Decimal, got %v", c.name, v)
		}
	}
}

// TestUnreadableSelectClauseIsReported pins the backstop. The FROM-first gap
// was invisible for as long as it was because an unreadable select clause
// turned the column rules off silently — so if a third clause shape ever
// arrives, the checker has to say it could not read the query rather than
// report nothing and look clean.
func TestUnreadableSelectClauseIsReported(t *testing.T) {
	ids := oqlRuleIDs(`select from Sales.Order as o`)
	if !hasOQLRule(ids, "MDL030") {
		t.Fatalf("an empty select list was accepted in silence, got %v", ids)
	}
	found := false
	for _, v := range ValidateOQLSyntax(`select from Sales.Order as o`) {
		if strings.Contains(v.Message, "could not be read") {
			found = true
		}
	}
	if !found {
		t.Error("the diagnostic does not say the columns went unchecked")
	}
	// Control: a query whose columns ARE readable must not collect it.
	for _, oql := range []string{fromFirstOQL, selectFirstOQL} {
		for _, v := range ValidateOQLSyntax(oql) {
			if strings.Contains(v.Message, "could not be read") {
				t.Errorf("readable query %q reported as unreadable", oql)
			}
		}
	}
}

// TestDescribeOutputOrderIsWhatMendixStores documents why the from-first order
// is the one that matters, so nobody later "simplifies" the terminator sets
// back to one. DESCRIBE ENTITY emits the stored OqlQuery verbatim
// (cmd_entities_describe.go), so describe → check is exactly this path.
func TestDescribeOutputOrderIsWhatMendixStores(t *testing.T) {
	if strings.Contains(strings.ToUpper(fromFirstOQL), "SELECT") &&
		strings.Index(strings.ToUpper(fromFirstOQL), "FROM") > strings.Index(strings.ToUpper(fromFirstOQL), "SELECT") {
		t.Fatal("fromFirstOQL is not actually from-first; the test fixture drifted")
	}
}
