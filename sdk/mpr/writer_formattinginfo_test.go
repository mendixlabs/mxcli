// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/bson"
)

// TestSerializeClientTemplateParameter_FormattingInfoNoTimeFormat
// guards against the CE0463 "widget definition changed" regression we
// hit on Mendix 11.9: Forms$FormattingInfo's reflection schema does not
// declare a TimeFormat property, but our writer was emitting
// `"TimeFormat": "HoursMinutes"` for every parameter. Studio Pro then
// treated every pluggable widget that embeds FormattingInfo (gallery,
// datagrid2 captions, dynamictext) as having a stale widget definition,
// which cascaded into CE3637 on master-detail pages.
func TestSerializeClientTemplateParameter_FormattingInfoNoTimeFormat(t *testing.T) {
	param := &pages.ClientTemplateParameter{Expression: "'hello'"}
	doc := serializeClientTemplateParameter(param)

	fi, ok := getBSONField(doc, "FormattingInfo").(bson.D)
	if !ok {
		t.Fatalf("FormattingInfo is not bson.D, got %T", getBSONField(doc, "FormattingInfo"))
	}
	for _, e := range fi {
		if e.Key == "TimeFormat" {
			t.Fatalf("FormattingInfo unexpectedly contains TimeFormat=%q — schema only declares CustomDateFormat/DateFormat/DecimalPrecision/EnumFormat/GroupDigits", e.Value)
		}
	}
	// Sanity: the five schema-declared keys are present.
	for _, want := range []string{"CustomDateFormat", "DateFormat", "DecimalPrecision", "EnumFormat", "GroupDigits"} {
		if getBSONField(fi, want) == nil {
			t.Errorf("FormattingInfo missing required field %q", want)
		}
	}
}
