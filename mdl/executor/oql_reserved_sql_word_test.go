// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ako/mxcli#2208: the SQL keywords that are plausible model names. Each word
// below was measured on mxbuild 11.16.0 as an unquoted entity name
// (`from Module.Word as x`) and as an unquoted attribute (`x.Word`): every one
// fails with CE0174 "The 'Word' part is incomplete or incorrect". In the same
// run Status, Value, Date, Position, Index, Coalesce and Top built clean, so
// they are the negative cases.
func TestMDL032_SQLKeywordsUsedAsModelNames(t *testing.T) {
	has := func(oql, word string) bool {
		for _, v := range ValidateOQLSyntax(oql) {
			if v.RuleID == "MDL032" && strings.Contains(v.Message, `"`+word+`"`) {
				return true
			}
		}
		return false
	}
	reserved := []string{"Order", "Group", "Limit", "Offset", "End", "Key", "Count", "Sum",
		"Distinct", "Select", "Like", "Asc", "Desc", "Having", "Where", "Join", "Exists", "Cast"}
	for _, w := range reserved {
		t.Run("entity "+w, func(t *testing.T) {
			oql := "select c.Email as Email from M.Shipment as s inner join s/M.Shipment_" + w + "/M." + w + " as o inner join o/M.X_C/M.Customer as c"
			if !has(oql, w) {
				t.Errorf("no MDL032 for unquoted entity %s in %q", w, oql)
			}
		})
		t.Run("attribute "+w, func(t *testing.T) {
			oql := "select x." + w + " as V from M.Thing as x"
			if !has(oql, w) {
				t.Errorf("no MDL032 for unquoted attribute %s in %q", w, oql)
			}
		})
		t.Run("quoted "+w, func(t *testing.T) {
			oql := `select x."` + w + `" as V from M."` + w + `" as x`
			if has(oql, w) {
				t.Errorf("MDL032 fired on the quoted form %q, which builds", oql)
			}
		})
	}
	for _, w := range []string{"Status", "Value", "Date", "Position", "Index", "Coalesce", "Top"} {
		oql := "select x." + w + " as V from M." + w + " as x"
		for _, v := range ValidateOQLSyntax(oql) {
			if v.RuleID == "MDL032" {
				t.Errorf("MDL032 on %s, which builds unquoted: %s", w, v.Message)
			}
		}
	}
}

// The keywords in their keyword positions must not trip the rule: the regex
// looks only after a '.' or after 'as'.
func TestMDL032_SQLKeywordsInKeywordPosition(t *testing.T) {
	oql := `select s.Carrier as Carrier, count(distinct s.ID) as Parcels, sum(s.Days) as Total,
  cast(s.Days as DECIMAL) as D, case when s.Days > 1 then 'a' else 'b' end as Band
from M.Shipment as s
inner join s/M.Shipment_Order/M."Order" as o
where s.Delivered = true and s.Note like '%x%' and exists (select 1 as One from M.X as x)
group by s.Carrier, s.Days, s.ID, s.Note
having count(s.ID) > 1
order by s.Carrier asc limit 10 offset 0`
	for _, v := range ValidateOQLSyntax(oql) {
		if v.RuleID == "MDL032" {
			t.Errorf("MDL032 on a keyword in keyword position: %s", v.Message)
		}
	}
}

// ako/mxcli#2208: `empty` is the microflow/XPath literal. OQL has none, reads
// the word as a path, and mxbuild 11.16.0 rejects `e.Leg != empty` with CE0174
// "Cannot resolve path part 'empty'".
func TestValidateOQLSyntax_EmptyIsNotOQL(t *testing.T) {
	rule := func(oql string) string {
		for _, v := range ValidateOQLSyntax(oql) {
			if v.RuleID == "MDL030" && strings.Contains(v.Message, "empty") {
				return v.Suggestion
			}
		}
		return ""
	}
	for _, c := range []struct{ oql, want string }{
		{"select e.Leg as Leg from M.E as e where e.Leg != empty", "is not null"},
		{"select e.Leg as Leg from M.E as e where e.Leg<>EMPTY", "is not null"},
		{"select e.Leg as Leg from M.E as e where e.Leg = empty", "is null"},
		{"select e.Leg as Leg from M.E as e where empty = e.Leg", "is null"},
	} {
		if got := rule(c.oql); !strings.Contains(got, c.want) {
			t.Errorf("%q: suggestion %q, want it to mention %q", c.oql, got, c.want)
		}
	}
	for _, oql := range []string{
		"select e.Leg as Leg from M.E as e where e.Leg is not null",
		"select e.Leg as Leg from M.E as e where e.Note = 'empty'",
		"select e.Leg as Leg from M.E as e where e.Note != ' = empty'",
		"select e.IsEmpty as IsEmpty from M.E as e where e.IsEmpty = true",
		"select e.Leg as Leg from M.E as e where e.Leg = e.EmptyLeg",
	} {
		if got := rule(oql); got != "" {
			t.Errorf("false positive on %q", oql)
		}
	}
}
