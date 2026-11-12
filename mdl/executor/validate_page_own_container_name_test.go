// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ownContainerFlagged returns the widgets MDL-BUTTON02 names, sorted.
func ownContainerFlagged(t *testing.T, src string) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	var names []string
	for _, v := range ValidatePageButtonContext(prog) {
		if v.RuleID != "MDL-BUTTON02" {
			continue
		}
		start := strings.Index(v.Message, "`")
		end := strings.Index(v.Message[start+1:], "`")
		names = append(names, v.Message[start+1:start+1+end])
	}
	sort.Strings(names)
	return names
}

// mendixlabs/mxcli#1324: a data container's widget-name variable ($dvGate) is
// only in scope for widgets nested in ANOTHER data container below it. From the
// container's own direct context it is not a variable at all, and mxbuild
// 11.14.0 reports `[CE0117] "Error(s) in expression." at Action button 'btnOwn'`
// while check and exec were clean. This is the issue's reproduction verbatim.
func TestValidatePageButtonContext_OwnDataViewName_Issue1324(t *testing.T) {
	src := `create page "G52"."GatePage" (Title: 'Gate', Layout: Atlas_Core.PopupLayout, Params: { $Gate: "G52"."Gate" }) {
  dataview dvGate (DataSource: $Gate) {
    actionbutton btnOwn (Caption: 'Own data view name', Action: microflow "G52"."ACT_Open"(Gate: $dvGate))
    actionbutton btnCur (Caption: 'Current object', Action: microflow "G52"."ACT_Open"(Gate: $currentObject))
    dataview dvInner (DataSource: $Gate) {
      actionbutton btnOuter (Caption: 'Enclosing data view name', Action: microflow "G52"."ACT_Open"(Gate: $dvGate))
    }
  }
};`
	got := ownContainerFlagged(t, src)
	if strings.Join(got, ",") != "btnOwn" {
		t.Fatalf("want only btnOwn flagged (btnCur and btnOuter build clean), got %v", got)
	}
}

// Every verdict below was measured with `mx check` on Mendix 11.14.0: the
// flagged set is exactly the set mxbuild reported CE0117 for. The nearest
// data container decides — a plain container is transparent, a control bar
// takes the context from ABOVE its grid (so a grid's own selection stays
// spellable there), and a row or item of a nested list is a new context.
//
// btnCtlBar is no longer flagged (ako/mxcli#2109). Its CE0117 was an error in
// an EXPRESSION `$dvP`; since #2095 the argument is written as the variable
// pair {Widget: dvP, PageParameter: Gate}, which builds with 0 errors on
// 11.16 (probe page, with a CE1365 positive control in the same build), and
// is what Studio Pro stores for a control-bar button there
// (FactoryManagement OIDC.OIDC_Client_NewEdit actionButton7, which builds).
// A control bar sits inside its grid, so the container above is an outer one.
func TestValidatePageButtonContext_OwnContainerName_MeasuredShapes(t *testing.T) {
	src := `create page G52.Probe (Title: 'Probe', Layout: Atlas_Core.PopupLayout, Params: ( $Gate: G52.Gate )) {
  dataview dvP (DataSource: $Gate) {
    container cWrap {
      actionbutton btnInContainer (Caption: 'c', Action: microflow G52.ACT_Open(Gate = $dvP))
    }
    actionbutton btnOwnPath (Caption: 'p', Action: microflow G52.ACT_Str(S = $dvP/Name))
    actionbutton btnLiteral (Caption: 'l', Action: microflow G52.ACT_Str(S = '$dvP'))
    datagrid dgIn (DataSource: database from G52.Gate) {
      controlbar cb1 {
        actionbutton btnCtlBar (Caption: 'cb', Action: microflow G52.ACT_Open(Gate = $dvP))
      }
      column Name (Attribute: Name) {
        actionbutton btnInColumn (Caption: 'col', Action: microflow G52.ACT_Open(Gate = $dvP))
      }
    }
    listview lvIn (DataSource: database from G52.Gate) {
      actionbutton btnInList (Caption: 'li', Action: microflow G52.ACT_Open(Gate = $dvP))
      actionbutton btnLvOwn (Caption: 'lo', Action: microflow G52.ACT_Open(Gate = $lvIn))
    }
  }
  datagrid dgSelf (DataSource: database from G52.Gate, Selection: Single) {
    controlbar cb2 {
      actionbutton btnSelCtl (Caption: 'sel', Action: microflow G52.ACT_Open(Gate = $dgSelf))
    }
    column Name (Attribute: Name) {
      actionbutton btnDgOwnCol (Caption: 'dg', Action: microflow G52.ACT_Open(Gate = $dgSelf))
    }
  }
  gallery gaSelf (DataSource: database from G52.Gate) {
    template t1 {
      actionbutton btnGaOwn (Caption: 'ga', Action: microflow G52.ACT_Open(Gate = $gaSelf))
    }
  }
};`
	got := strings.Join(ownContainerFlagged(t, src), ",")
	want := "btnDgOwnCol,btnGaOwn,btnInContainer,btnLvOwn,btnOwnPath"
	if got != want {
		t.Fatalf("flagged %q, want %q (mx check 11.14.0's CE0117 set)", got, want)
	}
}

// The suggestion is the spelling that works where the button is.
func TestValidatePageButtonContext_OwnContainerName_Suggestion(t *testing.T) {
	prog, errs := visitor.Build(`create page P.X (Title: 'x', Layout: Atlas_Core.PopupLayout, Params: ( $O: P.O )) {
  dataview dvO (DataSource: $O) {
    actionbutton b1 (Caption: 'b', Action: microflow P.F(O = $dvO))
  }
};`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	vs := ValidatePageButtonContext(prog)
	if len(vs) != 1 || vs[0].RuleID != "MDL-BUTTON02" {
		t.Fatalf("want one MDL-BUTTON02, got %+v", vs)
	}
	if !strings.Contains(vs[0].Message, "CE0117") || !strings.Contains(vs[0].Suggestion, "$currentObject") {
		t.Errorf("message should name CE0117 and suggest $currentObject: %+v", vs[0])
	}
}

// The same scope rule holds outside actions. Measured with `mx check` 11.14.0
// (each slot once directly in dvP, once one data view deeper as the control):
// a nested data view's microflow data-source argument, Visible, Editable and
// DynamicClasses are CE0117 and a nested list's XPath `where` is CE0161 when
// they read the container they sit in directly; every control builds clean.
func TestValidatePageButtonContext_OwnContainerName_OutsideActions(t *testing.T) {
	src := `create page G52.Probe3 (Title: 'Probe3', Layout: Atlas_Core.PopupLayout, Params: ( $Gate: G52.Gate )) {
  dataview dvP (DataSource: $Gate) {
    dataview dvDsOwn (DataSource: microflow G52.DS_Gate(Gate = $dvP)) {
      dynamictext dtA (Content: 'a')
    }
    container cVisOwn (Visible: $dvP/Name != '') {
      dynamictext dtB (Content: 'b')
    }
    textbox tbEdOwn (Attribute: Name, Editable: $dvP/Name != '')
    container cClsOwn (DynamicClasses: if $dvP/Name = '' then 'a' else 'b') {
      dynamictext dtE (Content: 'e')
    }
    listview lvXpOwn (DataSource: database from G52.Gate where [Name = $dvP/Name]) {
      dynamictext dtF (Content: 'f')
    }
    dataview dvMid (DataSource: $Gate) {
      dataview dvDsOuter (DataSource: microflow G52.DS_Gate(Gate = $dvP)) {
        dynamictext dtC (Content: 'c')
      }
      container cVisOuter (Visible: $dvP/Name != '') {
        dynamictext dtD (Content: 'd')
      }
      textbox tbEdOuter (Attribute: Name, Editable: $dvP/Name != '')
      container cClsOuter (DynamicClasses: if $dvP/Name = '' then 'a' else 'b') {
        dynamictext dtG (Content: 'g')
      }
      listview lvXpOuter (DataSource: database from G52.Gate where [Name = $dvP/Name]) {
        dynamictext dtH (Content: 'h')
      }
    }
  }
};`
	got := strings.Join(ownContainerFlagged(t, src), ",")
	want := "cClsOwn,cVisOwn,dvDsOwn,lvXpOwn,tbEdOwn"
	if got != want {
		t.Fatalf("flagged %q, want %q (mx check 11.14.0's CE0117/CE0161 set)", got, want)
	}
}
