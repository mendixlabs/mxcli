// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#904: DESCRIBE of an AccessibilityHelper did not re-execute.
//
// Two defects were reported. (1) describe emitted `content { … }` for the
// widget's child slot, which exec refused with MDL-WIDGET26 "`content` is not a
// container of `accessibilityhelper` — it declares: attr, template". That half
// was fixed by mendixlabs/mxcli#1400 (describe derives the slot keyword with the
// definition's function); TestAccessibilityHelper_SlotKeywordIsTemplate pins it
// for this widget. (2) The widget's attributesList item has a property literally
// named `attribute` (the HTML attribute's name, a string), and `check
// --references` — which exec runs first — read it as an attribute BINDING:
//
//	widget '': Attribute 'aria-label' is bound but there is no enclosing data
//	container providing entity context
//
// so the describe output of any AccessibilityHelper with an attribute refused to
// execute. Measured on 11.12.1: exec --no-check of the same script writes the
// item and describes it back unchanged.

const accessibilityHelperID = "com.mendix.widget.web.accessibilityhelper.AccessibilityHelper"

// accessibilityHelperRegistry is the shape of the widget's .def.json: an
// attributesList object list (container `attr`) and a `content` child slot
// (container `template`). In memory, as chartRegistry is, because the embedded
// definitions declare no object list.
func accessibilityHelperRegistry() *WidgetRegistry {
	def := &WidgetDefinition{
		WidgetID: accessibilityHelperID,
		MDLName:  "accessibilityhelper",
		ObjectLists: []ObjectListMapping{
			{PropertyKey: "attributesList", MDLContainer: deriveObjectListKeyword("attributesList")},
		},
		ChildSlots: []ChildSlotMapping{
			{PropertyKey: "content", MDLContainer: mdlContainerForWidgetSlot(accessibilityHelperID, "content")},
		},
	}
	return &WidgetRegistry{
		byMDLName:  map[string]*WidgetDefinition{"ACCESSIBILITYHELPER": def},
		byWidgetID: map[string]*WidgetDefinition{def.WidgetID: def},
	}
}

// describedAccessibilityHelper is what `describe page` prints for one, measured.
const describedAccessibilityHelper = `create or modify page Probe904.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  accessibilityhelper acc1 (targetSelector: '.mx-name-acc1') {
    attr (attribute: 'aria-label', valueSourceType: 'text', valueText: 'Hello', attributeCondition: 'true')
    template {
      dynamictext accContent (Content: 'Accessible content area')
    }
  }
};`

func pageWidgetsOf(t *testing.T, src string) []*ast.WidgetV3 {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	return prog.Statements[0].(*ast.CreatePageStmtV3).Widgets
}

func ctxWithRegistry(r *WidgetRegistry) *ExecContext {
	return &ExecContext{widgetRegistry: r, widgetRegistryLoaded: true}
}

func TestAccessibilityHelper_SlotKeywordIsTemplate(t *testing.T) {
	if kw := mdlContainerForWidgetSlot(accessibilityHelperID, "content"); kw != "TEMPLATE" {
		t.Errorf("content slot keyword = %q, want TEMPLATE (what the definition declares)", kw)
	}
	if kw := types.WidgetSlotKeyword(accessibilityHelperID, "content"); kw != "TEMPLATE" {
		t.Errorf("describe's slot keyword = %q, want TEMPLATE", kw)
	}
}

func TestAccessibilityHelper_ItemAttributeIsNotABinding(t *testing.T) {
	errs := validatePageContextTree(ctxWithRegistry(accessibilityHelperRegistry()), nil,
		pageWidgetsOf(t, describedAccessibilityHelper))
	if len(errs) != 0 {
		t.Errorf("describe output of an AccessibilityHelper refused: %v", errs)
	}
}

// Control: a real attribute binding outside every data container is still
// reported, beside the helper — the rule has not gone quiet.
func TestAccessibilityHelper_RealUnboundAttributeStillReported(t *testing.T) {
	src := strings.Replace(describedAccessibilityHelper,
		"  accessibilityhelper acc1",
		"  textbox txtLoose (Attribute: Name)\n  accessibilityhelper acc1", 1)
	errs := validatePageContextTree(ctxWithRegistry(accessibilityHelperRegistry()), nil, pageWidgetsOf(t, src))
	if len(errs) != 1 || !strings.Contains(errs[0], "txtLoose") {
		t.Errorf("got %v, want exactly the unbound textbox txtLoose", errs)
	}
}
