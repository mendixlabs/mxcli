// SPDX-License-Identifier: Apache-2.0

package types

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

type fixedStructure struct{ root string }

func (f fixedStructure) GetJsonStructureByQualifiedName(string, string) (*JsonStructure, error) {
	return &JsonStructure{Elements: []*JsonElement{{ElementType: f.root}}}, nil
}

// ako/mxcli#1938 (mendixlabs/mxcli#1734): an array-rooted JSON structure whose
// mapping maps the ARRAY ITSELF to the root entity (root at "(Array)", the
// items a child at "(Array)|(Object)") returns ONE root object; the structure
// fallback answered "list" for any array-rooted structure, and the import
// activity was written as returning a list — mx check 11.14 CE0243 "The mapping
// used to return a value of type 'List of …', but now returns '…'". A root
// whose path is known is decided by the path alone.
func TestJsonMappingRootIsList_ByRootPath(t *testing.T) {
	mapping := func(path string) *model.ImportMapping {
		return &model.ImportMapping{
			JsonStructure: "M.JS",
			Elements:      []*model.ImportMappingElement{{Kind: "Object", Entity: "M.Root", JsonPath: path}},
		}
	}
	array := fixedStructure{root: "Array"}
	for _, tc := range []struct {
		path string
		want bool
	}{
		{"(Array)", false},         // the reporter's: the array is the root object
		{"(Array)|(Object)", true}, // Studio Pro's own array-rooted mappings (#248)
		{"(Object)", false},
		{"(Object)|choices|(Object)|message", true}, // rooted below an array (#267)
	} {
		list, known := JsonMappingRootIsList(array, mapping(tc.path))
		if !known || list != tc.want {
			t.Errorf("root at %q: list=%v known=%v, want list=%v", tc.path, list, known, tc.want)
		}
	}
	// Control: with no stored path the structure still decides.
	if list, known := JsonMappingRootIsList(array, mapping("")); !known || !list {
		t.Errorf("no root path, array structure: list=%v known=%v, want the structure's answer (list)", list, known)
	}
}
