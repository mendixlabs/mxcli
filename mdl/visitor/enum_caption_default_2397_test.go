// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#2397 (mendixlabs/mxcli#1893): an enumeration value written without
// a caption was stored with an empty one — `create enumeration M.Color (Red)`
// and `alter enumeration M.Color add value Blue` both gave a drop-down option
// with no text, and mx check said nothing. Studio Pro fills a new value's
// caption from its name; so does mxcli. A caption written as '' stays empty.
func TestEnumValueCaptionDefaultsToName_Issue2397(t *testing.T) {
	prog, errs := Build(`create enumeration M.Color ( Red, Green 'Green!', Grey '' );
alter enumeration M.Color add value Blue;
alter enumeration M.Color add value Pink caption 'Rose';`)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	ce := prog.Statements[0].(*ast.CreateEnumerationStmt)
	want := map[string]string{"Red": "Red", "Green": "Green!", "Grey": ""}
	for _, v := range ce.Values {
		if v.Caption != want[v.Name] {
			t.Errorf("create: %s caption = %q, want %q", v.Name, v.Caption, want[v.Name])
		}
	}
	if a := prog.Statements[1].(*ast.AlterEnumerationStmt); a.Caption != "Blue" {
		t.Errorf("add value Blue: caption = %q, want Blue", a.Caption)
	}
	if a := prog.Statements[2].(*ast.AlterEnumerationStmt); a.Caption != "Rose" {
		t.Errorf("add value Pink caption 'Rose': caption = %q, want Rose", a.Caption)
	}
}
