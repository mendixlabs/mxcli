// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#968 (mendixlabs/mxcli#1263): the writer hard-coded the default
// Forms$FormattingInfo on every date picker and text box, so `DateFormat: Time`
// was stored as Date and a date-time picker came back date-only.

func formattingOf(t *testing.T, doc bson.D) map[string]any {
	t.Helper()
	fi, ok := docGet(doc, "FormattingInfo").(bson.D)
	if !ok {
		t.Fatalf("FormattingInfo = %#v, want a Forms$FormattingInfo document", docGet(doc, "FormattingInfo"))
	}
	out := map[string]any{}
	for _, e := range fi {
		out[e.Key] = e.Value
	}
	return out
}

func TestDatePickerFormattingInfoWritten(t *testing.T) {
	for _, tc := range []struct {
		name           string
		fi             *pages.FormattingInfo
		wantDF, wantCD string
	}{
		{"default", nil, "Date", ""},
		{"time", &pages.FormattingInfo{DateFormat: "Time", DecimalPrecision: 2, EnumFormat: "Text"}, "Time", ""},
		{"datetime", &pages.FormattingInfo{DateFormat: "DateTime", DecimalPrecision: 2, EnumFormat: "Text"}, "DateTime", ""},
		{"custom", &pages.FormattingInfo{DateFormat: "Custom", CustomDateFormat: "dd-MM-yyyy H:mm", DecimalPrecision: 2, EnumFormat: "Text"}, "Custom", "dd-MM-yyyy H:mm"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dp := &pages.DatePicker{BaseWidget: pages.BaseWidget{Name: "dp"}, AttributePath: "M.Booking.Moment", FormattingInfo: tc.fi}
			got := formattingOf(t, encodeWidget(t, dp))
			if got["DateFormat"] != tc.wantDF || got["CustomDateFormat"] != tc.wantCD {
				t.Errorf("FormattingInfo DateFormat/CustomDateFormat = %v/%q, want %s/%q",
					got["DateFormat"], got["CustomDateFormat"], tc.wantDF, tc.wantCD)
			}
		})
	}
}

func TestTextBoxFormattingInfoWritten(t *testing.T) {
	tb := &pages.TextBox{BaseWidget: pages.BaseWidget{Name: "tb"}, AttributePath: "M.Booking.Amount",
		FormattingInfo: &pages.FormattingInfo{DateFormat: "Date", DecimalPrecision: 4, GroupDigits: true, EnumFormat: "Text"}}
	got := formattingOf(t, encodeWidget(t, tb))
	// int64, the width Studio Pro stores DecimalPrecision at (4,856 of 4,856
	// across nine Studio Pro-authored projects, ako/mxcli#1264). The comparison
	// is on an interface, so it pins the width as well as the value.
	if got["DecimalPrecision"] != int64(4) || got["GroupDigits"] != true {
		t.Errorf("FormattingInfo DecimalPrecision/GroupDigits = %v/%v, want 4/true", got["DecimalPrecision"], got["GroupDigits"])
	}
	// Control: no FormattingInfo keeps the defaults byte-for-byte.
	def := formattingOf(t, encodeWidget(t, &pages.TextBox{BaseWidget: pages.BaseWidget{Name: "tb"}, AttributePath: "M.Booking.Amount"}))
	if def["DecimalPrecision"] != int64(2) || def["GroupDigits"] != false || def["DateFormat"] != "Date" {
		t.Errorf("default FormattingInfo = %v, want precision 2, no grouping, Date", def)
	}
}
