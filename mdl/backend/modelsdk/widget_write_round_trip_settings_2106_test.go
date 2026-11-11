// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#2106: the writer stored Studio Pro's defaults over a check box's
// label position and a show page action's close count.
func TestWriteCheckBox_LabelPosition(t *testing.T) {
	d := encodeWidget(t, &pages.CheckBox{
		BaseWidget:    pages.BaseWidget{BaseElement: model.BaseElement{ID: "cb"}, Name: "cb"},
		LabelPosition: "AfterControl",
	})
	if got := bsonnav.DGetString(d, "LabelPosition"); got != "AfterControl" {
		t.Errorf("LabelPosition = %q, want AfterControl", got)
	}
	// Control: unset writes what it always did, nothing.
	d = encodeWidget(t, &pages.CheckBox{BaseWidget: pages.BaseWidget{BaseElement: model.BaseElement{ID: "cb"}, Name: "cb"}})
	if got := bsonnav.DGetString(d, "LabelPosition"); got != "" {
		t.Errorf("unset LabelPosition = %q, want it left unwritten as before", got)
	}
}

func TestWriteShowPageAction_NumberOfPagesToClose(t *testing.T) {
	d := encodeWidget(t, &pages.ActionButton{
		BaseWidget: pages.BaseWidget{BaseElement: model.BaseElement{ID: "b"}, Name: "b"},
		Action:     &pages.PageClientAction{PageName: "M.Next", NumberOfPagesToClose: "1"},
	})
	if got := bsonnav.DGetString(bsonnav.DGetDoc(d, "Action"), "NumberOfPagesToClose2"); got != "1" {
		t.Errorf("NumberOfPagesToClose2 = %q, want 1", got)
	}
}
