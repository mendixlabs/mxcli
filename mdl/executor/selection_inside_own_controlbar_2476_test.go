// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// mendixlabs/mxcli#1911 (ako/mxcli#2476): a data view with `DataSource:
// selection dgItems` inside dgItems' own `controlbar {}` — the selection-driven
// command bar above the columns — passed check, and Studio Pro and mxbuild
// accept it, but exec refused "widget not found: dgItems": the grid's name was
// registered only once its slots, the control bar among them, were built.
const selectionInControlbar2476 = `mdl 1;
create module S2476;
create persistent entity S2476.Thing ( NId: String(20) );
create or replace snippet S2476.SN_Test (
  Params: ( $Entity: S2476.Thing )
) {
  datagrid dgItems (
    DataSource: database from S2476.Thing,
    Selection: Single
  ) {
    controlbar {
      dataview dvGallery (
        DataSource: selection dgItems
      ) {
        container ctnBar (Class: 'command-bar') { }
      }
    }
    column (Attribute: NId, Caption: 'ID') {
      textfilter tf1
    }
  }
};
`

func TestSelectionOfTheGridInsideItsOwnControlbar_2476(t *testing.T) {
	exec, out := openPedAppWithWidgets(t)
	if err := agreeExec(t, exec, selectionInControlbar2476); err != nil {
		t.Fatalf("exec refused the snippet: %v\n%s", err, out.String())
	}
	// Control: a selection naming no widget of the document is still refused.
	exec2, _ := openPedAppWithWidgets(t)
	bad := strings.Replace(selectionInControlbar2476, "DataSource: selection dgItems", "DataSource: selection dgNope", 1)
	if err := agreeExec(t, exec2, bad); err == nil || !strings.Contains(err.Error(), "dgNope") {
		t.Errorf("control: a selection of an unknown widget must be refused, got %v", err)
	}
}
