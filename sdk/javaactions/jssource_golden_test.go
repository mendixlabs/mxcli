// SPDX-License-Identifier: Apache-2.0

package javaactions

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The goldens testdata/mxbuild/JS_*.js.golden are the .js files mxbuild 11.12.5
// left behind for two JavaScript actions mxcli had created on a 11.12.5 app,
// after a package build (CRLF converted to LF). mxbuild regenerates a JavaScript
// action's file from the model and keeps only the user code, EXTRA CODE and the
// import list, so the signature in the golden is the one the user code has to be
// written against. JS_Open's stored body uses the declared spelling (Url): that
// is the defect, kept verbatim — mxbuild does not touch a body.
//
// Only the identifiers are compared, not the whole file: mxbuild also adds an
// `import { Big }` line and marks nullable parameters `[name]`, which mxcli does
// not write yet.

var jsMxbuildCases = []struct {
	name   string
	params []*JavaActionParameter
}{
	{"JS_Open", []*JavaActionParameter{param("Url", &StringType{}), param("ReadOnly", &BooleanType{})}},
	{"JS_Cases", []*JavaActionParameter{
		param("URL", &StringType{}), param("MyID", &StringType{}), param("Some_Name", &StringType{}),
		param("A", &StringType{}), param("XMLData", &StringType{}), param("_Lead", &StringType{}),
		param("Obj2Name", &StringType{}), param("Eid", &StringType{}),
	}},
}

var (
	jsSignatureLine = regexp.MustCompile(`(?m)^export async function .*$`)
	jsDocParamName  = regexp.MustCompile(`(?m)^ \* @param \{[^}]*\} \[?([^\]\s]+)\]?$`)
)

func jsIdentifiers(src string) (signature string, docParams []string) {
	for _, m := range jsDocParamName.FindAllStringSubmatch(src, -1) {
		docParams = append(docParams, m[1])
	}
	return jsSignatureLine.FindString(src), docParams
}

// Regenerating over the file mxbuild wrote must give the same signature and
// the same @param names: this is the steady state of an mxcli edit followed by
// a build, and the user code must not lose an identifier it uses.
func TestGenerateJavaScriptSource_SignatureMatchesMxbuild(t *testing.T) {
	for _, c := range jsMxbuildCases {
		t.Run(c.name, func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join("testdata", "mxbuild", c.name+".js.golden"))
			if err != nil {
				t.Fatal(err)
			}
			golden := strings.ReplaceAll(string(b), "\r\n", "\n")
			imports, extra := JavaScriptRetainedSections(golden)
			got := GenerateJavaScriptSource(c.name, "return 1;", c.params, &StringType{}, imports, extra)

			wantSig, wantDoc := jsIdentifiers(golden)
			gotSig, gotDoc := jsIdentifiers(got)
			if wantSig == "" || len(wantDoc) != len(c.params) {
				t.Fatalf("golden is not what the test expects (signature %q, @param %v)", wantSig, wantDoc)
			}
			if gotSig != wantSig {
				t.Errorf("signature differs from mxbuild's:\n  mxbuild: %s\n  mxcli:   %s", wantSig, gotSig)
			}
			if strings.Join(gotDoc, ",") != strings.Join(wantDoc, ",") {
				t.Errorf("@param names differ from mxbuild's:\n  mxbuild: %v\n  mxcli:   %v", wantDoc, gotDoc)
			}
		})
	}
}
