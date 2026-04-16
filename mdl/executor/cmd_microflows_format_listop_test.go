// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// =============================================================================
// formatListOperation
// =============================================================================

func TestFormatListOperation_Head(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.HeadOperation{ListVariable: "Orders"}, "First")
	if got != "$First = HEAD($Orders);" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_Tail(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.TailOperation{ListVariable: "Orders"}, "Rest")
	if got != "$Rest = TAIL($Orders);" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_Find(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.FindOperation{ListVariable: "Orders", Expression: "$Order/Status = 'Active'"}, "Found")
	if got != "$Found = FIND($Orders, $Order/Status = 'Active');" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_Filter(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.FilterOperation{ListVariable: "Orders", Expression: "$Order/Amount > 100"}, "Filtered")
	if got != "$Filtered = FILTER($Orders, $Order/Amount > 100);" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_Sort(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.SortOperation{
		ListVariable: "Orders",
		Sorting: []*microflows.SortItem{
			{AttributeQualifiedName: "MyModule.Order.Date", Direction: microflows.SortDirectionDescending},
		},
	}, "Sorted")
	if got != "$Sorted = SORT($Orders, Date DESC);" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_Union(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.UnionOperation{ListVariable1: "A", ListVariable2: "B"}, "Combined")
	if got != "$Combined = UNION($A, $B);" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_Intersect(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.IntersectOperation{ListVariable1: "A", ListVariable2: "B"}, "Common")
	if got != "$Common = INTERSECT($A, $B);" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_Subtract(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.SubtractOperation{ListVariable1: "A", ListVariable2: "B"}, "Diff")
	if got != "$Diff = SUBTRACT($A, $B);" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_Contains(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.ContainsOperation{ListVariable: "Orders", ObjectVariable: "Order"}, "HasIt")
	if got != "$HasIt = CONTAINS($Orders, $Order);" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_Equals(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.EqualsOperation{ListVariable1: "A", ListVariable2: "B"}, "Same")
	if got != "$Same = EQUALS($A, $B);" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_Nil(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(nil, "Result")
	if got != "$Result = LIST OPERATION ...;" {
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
	if got != "$Found = FIND($Orders, $currentObject/Status = 'Active');" {
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
	if got != "$Found = FIND($Orders, $currentObject/Order_Customer = $Customer);" {
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
	if got != "$Filtered = FILTER($Orders, $currentObject/IsActive = true);" {
		t.Errorf("got %q", got)
	}
}

func TestFormatListOperation_Range(t *testing.T) {
	e := newTestExecutor()
	got := e.formatListOperation(&microflows.RangeOperation{
		ListVariable:     "Orders",
		OffsetExpression: "0",
		LimitExpression:  "10",
	}, "Page")
	if got != "$Page = RANGE($Orders, 0, 10);" {
		t.Errorf("got %q", got)
	}
}
