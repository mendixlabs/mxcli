// SPDX-License-Identifier: Apache-2.0

package javaactions

import (
	"strings"
	"testing"
)

// mxbuild regenerates a JavaScript action's signature and JSDoc from the model
// and keeps only the user code and EXTRA CODE, so the parameter names the user
// code is written against are the names mxbuild generates, not the declared
// ones. Measured with mxbuild 11.12.5 on an action declaring these parameters.
func TestJavaScriptParameterName_MatchesMxbuild(t *testing.T) {
	for _, c := range []struct{ declared, generated string }{
		{"URL", "uRL"},
		{"MyID", "myID"},
		{"Some_Name", "some_Name"},
		{"A", "a"},
		{"XMLData", "xMLData"},
		{"_Lead", "_Lead"},
		{"Obj2Name", "obj2Name"},
		{"Eid", "eid"},
		// Parameters of Studio Pro-authored actions and their generated .js.
		{"PhoneNumber", "phoneNumber"},
		{"Address", "address"},
		{"GeocodingProvider", "geocodingProvider"},
		{"ProviderApiKey", "providerApiKey"},
		// Already lower-first, and the degenerate input.
		{"latitude", "latitude"},
		{"", ""},
		// First rune, not first byte. Defensive: Mendix names are ASCII, so these
		// two rows are not measured against mxbuild.
		{"Édith", "édith"},
		{"Ärger", "ärger"},
	} {
		if got := JavaScriptParameterName(c.declared); got != c.generated {
			t.Errorf("JavaScriptParameterName(%q) = %q, want %q", c.declared, got, c.generated)
		}
	}
}

func TestGenerateJavaScriptSource_ParameterNamesAreLowerFirst(t *testing.T) {
	src := GenerateJavaScriptSource("JS_Open", "return Url;",
		[]*JavaActionParameter{param("Url", &StringType{}), param("ReadOnly", &BooleanType{})},
		&StringType{}, nil, "")

	for _, want := range []string{
		" * @param {string} url\n",
		" * @param {boolean} readOnly\n",
		"export async function JS_Open(url, readOnly) {\n",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated source lacks %q:\n%s", want, src)
		}
	}
	for _, bad := range []string{"Url,", " Url\n", "ReadOnly)", " ReadOnly\n"} {
		if strings.Contains(src, bad) {
			t.Errorf("generated source still declares a capitalised parameter (%q):\n%s", bad, src)
		}
	}
	// The user code is the user's: it is never rewritten.
	if !strings.Contains(src, "\treturn Url;\n") {
		t.Errorf("user code was altered:\n%s", src)
	}
}
