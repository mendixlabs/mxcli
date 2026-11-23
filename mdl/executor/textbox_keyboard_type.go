// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// A text box's keyboard type (ako/mxcli#1209).
//
// Forms$TextBox.KeyboardType is the keyboard a native app opens for the box —
// Default, NumberPad, DecimalPad, EmailAddress, PhonePad or URL (mendixmodelsdk,
// declared from 8.0). The writer wrote "Default" on every text box and describe
// printed nothing, so a rewrite reset the keyboard of every native text box
// Studio Pro had set one on. The MDL spelling is the stored property and enum
// member:
//
//	textbox txtEmail (Attribute: Email, KeyboardType: EmailAddress)
//
// It is a text box's only, so it is kept out of staticWidgetKnownProps (the
// union every widget type shares) and exempted per type here, the arrangement
// of the submit keys in input_submit_width.go — which is also why the read and
// the emit live in this file rather than a builder or describe-output file.

// isKeyboardTypeProp reports whether key is the keyboard type of the MDL widget
// type widgetType.
func isKeyboardTypeProp(widgetType, key string) bool {
	return strings.EqualFold(widgetType, "textbox") && strings.EqualFold(key, "KeyboardType")
}

// keyboardTypeProblem validates a text box's KeyboardType; "" means valid or
// absent. The builder refuses with the same message, so check and exec agree.
func keyboardTypeProblem(w *ast.WidgetV3) string {
	if w == nil || !isKeyboardTypeProp(w.Type, "KeyboardType") {
		return ""
	}
	v, ok := lookupPropCI(w, "KeyboardType")
	if !ok {
		return ""
	}
	if s, isStr := v.(string); isStr {
		if _, valid := pages.CanonicalKeyboardType(s); valid {
			return ""
		}
	}
	return fmt.Sprintf("KeyboardType must be one of %s, got %v", strings.Join(pages.KeyboardTypes, ", "), v)
}

// textBoxKeyboardType reads a text box's KeyboardType as Mendix spells it, ""
// when unset (the writer then writes Default).
func textBoxKeyboardType(w *ast.WidgetV3) (string, error) {
	if msg := keyboardTypeProblem(w); msg != "" {
		return "", mdlerrors.NewValidationf("textbox %q: %s", w.Name, msg)
	}
	v, _ := lookupPropCI(w, "KeyboardType")
	s, _ := v.(string)
	kt, _ := pages.CanonicalKeyboardType(s)
	return kt, nil
}

// describeKeyboardType appends a text box's KeyboardType when it is not the
// default.
func describeKeyboardType(props []string, w rawWidget) []string {
	if w.AutoFocus {
		// Only when set: false is every ordinary text box (ako/mxcli#2519).
		props = append(props, "AutoFocus: true")
	}
	if w.KeyboardType != "" && w.KeyboardType != "Default" {
		props = append(props, "KeyboardType: "+w.KeyboardType)
	}
	return props
}
