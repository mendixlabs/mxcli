// SPDX-License-Identifier: Apache-2.0

package pages

import "strings"

// The formatting an input widget stores in its Forms$FormattingInfo, shared by
// the CREATE PAGE builder and the ALTER PAGE mutator so the two cannot accept
// different values (ako/mxcli#968).

// dateFormats maps a lowercase DateFormat value to the stored enum value.
var dateFormats = map[string]string{
	"date": "Date", "time": "Time", "datetime": "DateTime", "custom": "Custom",
}

// CanonicalDateFormat returns the stored spelling of a FormattingInfo
// DateFormat value (Date, Time, DateTime, Custom), matched case-insensitively.
func CanonicalDateFormat(s string) (string, bool) {
	v, ok := dateFormats[strings.ToLower(s)]
	return v, ok
}

// enumFormats maps a lowercase EnumFormat value to the stored enum value.
var enumFormats = map[string]string{"text": "Text", "image": "Image"}

// CanonicalEnumFormat returns the stored spelling of a FormattingInfo
// EnumFormat value (Text, Image), matched case-insensitively.
func CanonicalEnumFormat(s string) (string, bool) {
	v, ok := enumFormats[strings.ToLower(s)]
	return v, ok
}

// formattingProperties lists, per widget storage type, the FormattingInfo
// fields MDL sets on it, in canonical spelling and describe order.
//
// A date picker's format is its mode: DateFormat Time or DateTime is what makes
// it a time or date-time picker. A text box binds only string and numeric
// attributes (CE2421 for a date), so of its FormattingInfo only the numeric
// fields mean anything.
var formattingProperties = map[string][]string{
	"Forms$DatePicker": {"DateFormat", "CustomDateFormat"},
	"Forms$TextBox":    {"DecimalPrecision", "GroupDigits"},
}

// FormattingProperties returns the formatting properties an input widget of the
// given storage type ($Type) carries, or nil for one MDL gives none.
func FormattingProperties(storageType string) []string {
	return formattingProperties[storageType]
}
