// SPDX-License-Identifier: Apache-2.0

package javaactions

import (
	"os"
	"path/filepath"
	"testing"
)

// ako/mxcli#2357: a re-run of an unchanged script rewrote every JavaScript
// action's .js file, dropping the action's documentation from the JSDoc. The
// file on disk was mxbuild's, and mxcli's generator wrote another shape. Since
// #2131 changed the user code's indentation, every re-run wrote that other
// shape over mxbuild's.
//
// The goldens *_11.12.6.js.golden are the files mxbuild 11.12.6
// (--target=deploy) regenerated, byte for byte (CRLF), for actions mxcli
// created. The generator must write the same bytes, so a build leaves the file
// alone and a re-run finds nothing to write.

func enumValues(qn string) []string {
	if qn == "MyFirstModule.Color" {
		return []string{"Red", "Blue"}
	}
	return nil
}

func requiredParam(name string, t CodeActionParameterType) *JavaActionParameter {
	p := param(name, t)
	p.IsRequired = true
	return p
}

func TestGenerateJavaScriptSource_MatchesMxbuild_Issue2357(t *testing.T) {
	thing := &EntityType{Entity: "MyFirstModule.Thing"}
	cases := []struct {
		golden string
		src    JSSource
	}{
		{"JS_CopyText_11.12.6.js.golden", JSSource{
			Action:        "JS_CopyText",
			Documentation: "Copies text to the clipboard (\"Copy PGN\", \"Copy FEN\").\nSecond line of the documentation.",
			UserCode:      "return true;",
			Params:        []*JavaActionParameter{param("Text", &StringType{}), requiredParam("Count", &IntegerType{})},
			ReturnType:    &BooleanType{},
		}},
		{"JS_Types_11.12.6.js.golden", JSSource{
			Action:   "JS_Types",
			UserCode: "return null;",
			Params: []*JavaActionParameter{
				param("PInt", &IntegerType{}), param("PLong", &LongType{}), param("PDec", &DecimalType{}),
				param("PBool", &BooleanType{}), param("PDate", &DateTimeType{}), param("PStr", &StringType{}),
				param("PEnum", &EnumerationType{Enumeration: "MyFirstModule.Color"}),
				param("PObj", thing), param("PList", &ListType{Entity: "MyFirstModule.Thing"}),
			},
			ReturnType: &DecimalType{},
			EnumValues: enumValues,
		}},
		{"JS_REnum_11.12.6.js.golden", JSSource{Action: "JS_REnum", UserCode: "return null;",
			ReturnType: &EnumerationType{Enumeration: "MyFirstModule.Color"}, EnumValues: enumValues}},
		{"JS_RList_11.12.6.js.golden", JSSource{Action: "JS_RList", UserCode: "return null;",
			ReturnType: &ListType{Entity: "MyFirstModule.Thing"}}},
		{"JS_RVoid_11.12.6.js.golden", JSSource{Action: "JS_RVoid", UserCode: "return;", ReturnType: &VoidType{}}},
	}
	for _, c := range cases {
		t.Run(c.golden, func(t *testing.T) {
			want, err := os.ReadFile(filepath.Join("testdata", "mxbuild", c.golden))
			if err != nil {
				t.Fatal(err)
			}
			if got := c.src.Generate(); got != string(want) {
				t.Errorf("generated source differs from mxbuild's\n got: %q\nwant: %q", got, want)
			}
		})
	}
}

// A regeneration keeps the import list it finds, and adds the Big import only
// when the list lacks it: mxbuild's own file regenerates to itself.
func TestGenerateJavaScriptSource_RegeneratesMxbuildsFile_Issue2357(t *testing.T) {
	want, err := os.ReadFile(filepath.Join("testdata", "mxbuild", "JS_CopyText_11.12.6.js.golden"))
	if err != nil {
		t.Fatal(err)
	}
	imports, extra := JavaScriptRetainedSections(string(want))
	src := JSSource{
		Action:        "JS_CopyText",
		Documentation: "Copies text to the clipboard (\"Copy PGN\", \"Copy FEN\").\nSecond line of the documentation.",
		UserCode:      "return true;",
		Params:        []*JavaActionParameter{param("Text", &StringType{}), requiredParam("Count", &IntegerType{})},
		ReturnType:    &BooleanType{},
		Imports:       imports,
		ExtraCode:     extra,
	}
	if got := src.Generate(); got != string(want) {
		t.Errorf("regenerating mxbuild's file changed it\n got: %q\nwant: %q", got, want)
	}
}
