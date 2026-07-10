// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
)

// violationsFor parses MDL and runs the layout-grid check, returning the messages.
func layoutGridMessages(t *testing.T, src string) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	var msgs []string
	for _, v := range ValidatePageLayoutGrid(prog) {
		msgs = append(msgs, v.Message)
	}
	return msgs
}

// An edit page with the DataView directly under the page (no layout grid) warns.
func TestValidatePageLayoutGrid_BareFormWarns(t *testing.T) {
	src := `CREATE PAGE M.Customer_NewEdit (
  params: { $Customer: M.Customer },
  title: 'Edit', layout: Atlas_Core.PopupLayout
) {
  dataview dvCustomer (datasource: $Customer) {
    textbox tb (label: 'Name', attribute: Name)
  }
};`
	msgs := layoutGridMessages(t, src)
	if len(msgs) != 1 || !strings.Contains(msgs[0], "dvCustomer") || !strings.Contains(msgs[0], "layout grid") {
		t.Fatalf("expected one layout-grid warning mentioning dvCustomer, got %v", msgs)
	}
}

// The prescribed NewEdit pattern (grid → row → column → dataview) is clean.
func TestValidatePageLayoutGrid_GridWrappedClean(t *testing.T) {
	src := `CREATE PAGE M.Customer_NewEdit (
  params: { $Customer: M.Customer },
  title: 'Edit', layout: Atlas_Core.PopupLayout
) {
  layoutgrid g {
    row r {
      column c (desktopwidth: autofill) {
        dataview dvCustomer (datasource: $Customer) {
          textbox tb (label: 'Name', attribute: Name)
        }
      }
    }
  }
};`
	if msgs := layoutGridMessages(t, src); len(msgs) != 0 {
		t.Fatalf("grid-wrapped form should be clean, got %v", msgs)
	}
}

// The data source is irrelevant: a database-bound form (has inputs) outside a grid
// warns just like a parameter-bound one.
func TestValidatePageLayoutGrid_DatabaseFormWarns(t *testing.T) {
	src := `CREATE PAGE M.Edit (
  title: 'Edit', layout: Atlas_Core.PopupLayout
) {
  dataview dvForm (datasource: database M.Customer) {
    textbox tb (label: 'Name', attribute: Name)
  }
};`
	msgs := layoutGridMessages(t, src)
	if len(msgs) != 1 || !strings.Contains(msgs[0], "dvForm") {
		t.Fatalf("expected one warning for the database form, got %v", msgs)
	}
}

// A display-only DataView (no input widgets) has no label/input-width concern and
// is not flagged, even outside a grid.
func TestValidatePageLayoutGrid_DisplayOnlyClean(t *testing.T) {
	src := `CREATE PAGE M.Show (
  params: { $Customer: M.Customer },
  title: 'Show', layout: Atlas_Core.PopupLayout
) {
  dataview dvShow (datasource: $Customer) {
    dynamictext t (content: 'hello')
  }
};`
	if msgs := layoutGridMessages(t, src); len(msgs) != 0 {
		t.Fatalf("display-only DataView should not warn, got %v", msgs)
	}
}
