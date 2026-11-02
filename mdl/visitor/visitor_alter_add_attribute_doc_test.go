// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#1863 (mendixlabs/mxcli#1705): a doc comment above an `alter
// entity … add attribute` documents the attribute it adds, as it would above a
// `create entity` attribute. The statement's comment was read by nothing — the
// alter only looked inside itself, before `add` — so exec reported success and
// the text was lost, with only a later check warning MDL089.
func TestAlterAddAttributeStatementDocComment(t *testing.T) {
	addedDocs := func(prog *ast.Program) []string {
		var out []string
		for _, s := range prog.Statements {
			switch st := s.(type) {
			case *ast.AlterEntityStmt:
				if st.Attribute != nil {
					out = append(out, st.Attribute.Documentation)
				}
			case *ast.AlterEntitiesStmt:
				for _, a := range st.Actions {
					if a.Attribute != nil {
						out = append(out, a.Attribute.Documentation)
					}
				}
			}
		}
		return out
	}
	for _, tc := range []struct {
		name     string
		src      string
		docs     []string
		detached int
	}{{
		name: "alter entity",
		src:  "/** Why */\nalter entity M.E add attribute X: String(200);\n",
		docs: []string{"Why"},
	}, {
		name: "alter entities",
		src:  "/** Why */\nalter entities in M add attribute X: String(200);\n",
		docs: []string{"Why"},
	}, {
		// The attribute's own comment wins; the statement's is still lost.
		name:     "control: attribute already documented",
		src:      "/** Outer */\nalter entity M.E add attribute /** Inner */ X: String(200);\n",
		docs:     []string{"Inner"},
		detached: 1,
	}, {
		// Two attributes: which one the comment is for is a guess, so it
		// stays a check warning rather than being attached to either.
		name:     "control: two attributes added",
		src:      "/** Which */\nalter entity M.E add attribute X: String(200) add attribute Y: Integer;\n",
		docs:     []string{"", ""},
		detached: 1,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			prog, errs := Build(tc.src)
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}
			got := addedDocs(prog)
			if len(got) != len(tc.docs) {
				t.Fatalf("added attributes' docs = %q, want %q", got, tc.docs)
			}
			for i := range got {
				if got[i] != tc.docs[i] {
					t.Errorf("attribute %d documentation = %q, want %q", i, got[i], tc.docs[i])
				}
			}
			if n := len(prog.DetachedDocComments); n != tc.detached {
				t.Errorf("detached doc comments = %+v, want %d", prog.DetachedDocComments, tc.detached)
			}
		})
	}
}
