// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// =============================================================================
// formatListOperation
// =============================================================================

func TestFormatListOperation_Head(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.HeadOperation{ListVariable: "Orders"}, "First")
	if got != "$First = head($Orders);" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_Tail(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.TailOperation{ListVariable: "Orders"}, "Rest")
	if got != "$Rest = tail($Orders);" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_Find(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.FindOperation{ListVariable: "Orders", Expression: "$Order/Status = 'Active'"}, "Found")
	if got != "$Found = find($Orders, $Order/Status = 'Active');" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_Filter(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.FilterOperation{ListVariable: "Orders", Expression: "$Order/Amount > 100"}, "Filtered")
	if got != "$Filtered = filter($Orders, $Order/Amount > 100);" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_Sort(t *testing.T) {
	e := newTestExecutor()
	// "Date" is a reserved keyword, so it must be quoted to re-parse (#619 sortSpec).
	got := e.formatListOperation(&microflows.SortOperation{
		ListVariable: "Orders",
		Sorting: []*microflows.SortItem{
			{AttributeQualifiedName: "MyModule.Order.Date", Direction: microflows.SortDirectionDescending},
		},
	}, "Sorted")
	if got != `$Sorted = sort($Orders, "Date" desc);` {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_SortNonReserved(t *testing.T) {
	e := newTestExecutor()
	// A non-reserved attribute name stays bare (mdlIdent quotes only reserved words).
	got := e.formatListOperation(&microflows.SortOperation{
		ListVariable: "Orders",
		Sorting: []*microflows.SortItem{
			{AttributeQualifiedName: "MyModule.Order.Amount", Direction: microflows.SortDirectionAscending},
		},
	}, "Sorted")
	if got != "$Sorted = sort($Orders, Amount asc);" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_Union(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.UnionOperation{ListVariable1: "A", ListVariable2: "B"}, "Combined")
	if got != "$Combined = union($A, $B);" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_Intersect(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.IntersectOperation{ListVariable1: "A", ListVariable2: "B"}, "Common")
	if got != "$Common = intersect($A, $B);" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_Subtract(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.SubtractOperation{ListVariable1: "A", ListVariable2: "B"}, "Diff")
	if got != "$Diff = subtract($A, $B);" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_Contains(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.ContainsOperation{ListVariable: "Orders", ObjectVariable: "Order"}, "HasIt")
	if got != "$HasIt = contains($Orders, $Order);" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_Equals(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.EqualsOperation{ListVariable1: "A", ListVariable2: "B"}, "Same")
	if got != "$Same = equals($A, $B);" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_FindByAttribute(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.FindByAttributeOperation{
		ListVariable: "Orders",
		Attribute:    "MyModule.Order.Status",
		Expression:   "'Active'",
	}, "Found")
	if got != "$Found = find($Orders, Status = 'Active');" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_FindByAssociation(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.FindByAttributeOperation{
		ListVariable: "Orders",
		Association:  "MyModule.Order_Customer",
		Expression:   "$Customer",
	}, "Found")
	if got != "$Found = find($Orders, Order_Customer = $Customer);" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_FindByAttributeEmpty(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.FindByAttributeOperation{
		ListVariable: "Orders",
	}, "Found")
	if got != "-- $Found = find($Orders) — missing attribute/expression" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_FilterByAttribute(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.FilterByAttributeOperation{
		ListVariable: "Orders",
		Attribute:    "MyModule.Order.IsActive",
		Expression:   "true",
	}, "Filtered")
	if got != "$Filtered = filter($Orders, IsActive = true);" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_Range(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.ListRangeOperation{ListVariable: "Orders", OffsetExpression: "5", LimitExpression: "10"}, "Page")
	if got != "$Page = range($Orders, 5, 10);" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_RangeOffsetOnly(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.ListRangeOperation{ListVariable: "Orders", OffsetExpression: "5"}, "Page")
	if got != "$Page = range($Orders, 5);" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_RangeLimitOnly(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.ListRangeOperation{ListVariable: "Orders", LimitExpression: "10"}, "Page")
	if got != "$Page = range($Orders, 0, 10);" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_Nil(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(nil, "Result")
	if got != "$Result = list operation ...;" {
		t.Errorf("got %q", got)
	}
}
