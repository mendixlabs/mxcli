// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#2111: a combo box whose Editable is "based on attribute value" was
// described as a bare `Editable: Conditional` — the condition dropped — and
// check refused that (MDL-WIDGET19). Studio Pro stores the attribute form of
// editability as it stores visibility's: Attribute, one Enumerations$Condition
// per value (EditableVisible marking the editable ones), and SourceVariable.

func TestDescribeEditableWhen_ReadsTheAttributeCondition(t *testing.T) {
	var w rawWidget
	extractConditionalSettings(nil, &w, map[string]any{
		"ConditionalEditabilitySettings": map[string]any{
			"Expression": "",
			"Attribute":  "AgentCommons.Tool.UserAccessApproval",
			"Conditions": []any{int32(2),
				map[string]any{"AttributeValue": "Required", "EditableVisible": false},
				map[string]any{"AttributeValue": "NotRequired", "EditableVisible": true},
				map[string]any{"AttributeValue": "(empty)", "EditableVisible": true},
			},
		},
	})
	if w.EditableAttr != "UserAccessApproval" || strings.Join(w.EditableValues, ",") != "NotRequired,empty" {
		t.Fatalf("read EditableAttr=%q EditableValues=%v, want UserAccessApproval [NotRequired empty]", w.EditableAttr, w.EditableValues)
	}

	w.Editable = "Conditional"
	var editable []string
	for _, p := range appendAppearanceProps(nil, nil, w) {
		if strings.HasPrefix(p, "Editable:") {
			editable = append(editable, p)
		}
	}
	if len(editable) != 1 || editable[0] != "Editable: UserAccessApproval in (NotRequired, empty)" {
		t.Fatalf("describe printed %v, want exactly [Editable: UserAccessApproval in (NotRequired, empty)]", editable)
	}
}

// Control: an expression condition still prints as the expression, and no
// attribute condition is invented for it.
func TestDescribeEditableWhen_ExpressionUnchanged(t *testing.T) {
	var w rawWidget
	extractConditionalSettings(nil, &w, map[string]any{
		"ConditionalEditabilitySettings": map[string]any{"Expression": "$currentObject/Open", "Attribute": ""},
	})
	if w.EditableIf != "$currentObject/Open" || w.EditableAttr != "" {
		t.Fatalf("EditableIf=%q EditableAttr=%q", w.EditableIf, w.EditableAttr)
	}
}

func TestVisitorEditableWhen(t *testing.T) {
	w := parseSnippetWidget(t, `textbox t (Attribute: Title, Editable: Status in (Draft, empty))`)
	ew, ok := w.Properties["EditableWhen"].(*ast.VisibleWhenV3)
	if !ok || ew.Attribute != "Status" || strings.Join(ew.Values, ",") != "Draft,empty" {
		t.Fatalf("EditableWhen = %#v, want Status in (Draft, empty)", w.Properties["EditableWhen"])
	}
	if _, ok := w.Properties["EditableIf"]; ok {
		t.Error("the attribute form was also lowered to an expression")
	}
}
