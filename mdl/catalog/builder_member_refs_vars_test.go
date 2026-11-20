// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"sort"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#2468: an attribute used through a document's own parameter,
// `$Card/Title`, was in no reference — `show references to Shop.Card.Title` was
// empty for a snippet, a page and a microflow alike — so MDL108 passed a drop that
// `mx check` then rejected (CE0117 for the expression). The stored expression names
// the attribute bare, and resolving it needs the variable's entity, which the
// parameter declares.
func varsIndex() *memberRefIndex {
	return &memberRefIndex{
		attributes:     map[string]string{"Shop.Card.Title": "", "Shop.Card.Note": "", "Shop.Other.Title": ""},
		associations:   map[string]bool{"Shop.Card_Other": true},
		enumerations:   map[string]bool{},
		enumValues:     map[string]bool{},
		entities:       map[string]bool{"Shop.Card": true, "Shop.Other": true},
		generalization: map[string]string{},
	}
}

func objectParam(typ, nameKey, typeKey, name, entity string) bson.D {
	return bson.D{
		{Key: "$Type", Value: typ},
		{Key: "Name", Value: name},
		{Key: typeKey, Value: bson.D{
			{Key: "$Type", Value: "DataTypes$ObjectType"},
			{Key: "Entity", Value: entity},
		}},
	}
}

func edgeNames(edges []refEdge) []string {
	var out []string
	for _, e := range edges {
		out = append(out, e.TargetType+" "+e.TargetName+" "+e.RefKind)
	}
	sort.Strings(out)
	return out
}

func marshalDoc(t *testing.T, d bson.D) []byte {
	t.Helper()
	b, err := bson.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestMemberRefs_ResolveVariablesFromDocumentParameters(t *testing.T) {
	text := func(expr string) bson.D {
		return bson.D{
			{Key: "$Type", Value: "Forms$ClientTemplateParameter"},
			{Key: "Expression", Value: expr},
		}
	}
	cases := []struct {
		name   string
		unit   string // sourceType
		doc    bson.D
		want   []string
		reject []string
	}{
		{
			name: "snippet parameter in a text template expression (the issue)",
			unit: RefObjectSnippet,
			doc: bson.D{{Key: "$Type", Value: "Forms$Snippet"}, {Key: "Name", Value: "SNIP"},
				{Key: "Parameters", Value: bson.A{int32(3), objectParam("Forms$SnippetParameter", "Name", "ParameterType", "Card", "Shop.Card")}},
				{Key: "Widgets", Value: bson.A{int32(2), text("$Card/Title")}}},
			want: []string{"ATTRIBUTE Shop.Card.Title member", "ENTITY Shop.Card parameter"}, // the parameter edge was already there
			// Shop.Other.Title has the same bare name: the variable's entity decides.
			reject: []string{"ATTRIBUTE Shop.Other.Title member"},
		},
		{
			name: "page parameter",
			unit: RefObjectPage,
			doc: bson.D{{Key: "$Type", Value: "Forms$Page"}, {Key: "Name", Value: "PG"},
				{Key: "Parameters", Value: bson.A{int32(3), objectParam("Forms$PageParameter", "Name", "ParameterType", "Card", "Shop.Card")}},
				{Key: "Widgets", Value: bson.A{int32(2), text("$Card/Note = 'x'")}}},
			want: []string{"ATTRIBUTE Shop.Card.Note member"},
		},
		{
			name: "microflow parameter in a split condition",
			unit: RefObjectMicroflow,
			doc: bson.D{{Key: "$Type", Value: "Microflows$Microflow"}, {Key: "Name", Value: "MF"},
				{Key: "ObjectCollection", Value: bson.D{{Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
					{Key: "Objects", Value: bson.A{int32(3),
						objectParam("Microflows$MicroflowParameter", "Name", "VariableType", "P", "Shop.Card"),
						bson.D{{Key: "$Type", Value: "Microflows$ExpressionSplitCondition"}, {Key: "Expression", Value: "$P/Title = 'x'"}},
					}}}}},
			want: []string{"ATTRIBUTE Shop.Card.Title member"},
		},
		{
			name: "nanoflow parameter",
			unit: RefObjectNanoflow,
			doc: bson.D{{Key: "$Type", Value: "Microflows$Nanoflow"}, {Key: "Name", Value: "NF"},
				{Key: "ObjectCollection", Value: bson.D{{Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
					{Key: "Objects", Value: bson.A{int32(3),
						objectParam("Microflows$MicroflowParameter", "Name", "VariableType", "P", "Shop.Card"),
						bson.D{{Key: "$Type", Value: "Microflows$ExpressionSplitCondition"}, {Key: "Expression", Value: "$P/Note != empty"}},
					}}}}},
			want: []string{"ATTRIBUTE Shop.Card.Note member"},
		},
		{
			name: "an association step off the parameter, then an attribute of its entity",
			unit: RefObjectPage,
			doc: bson.D{{Key: "$Type", Value: "Forms$Page"}, {Key: "Name", Value: "PG"},
				{Key: "Parameters", Value: bson.A{int32(3), objectParam("Forms$PageParameter", "Name", "ParameterType", "Card", "Shop.Card")}},
				{Key: "Widgets", Value: bson.A{int32(2), text("$Card/Shop.Card_Other/Shop.Other/Title")}}},
			want: []string{"ASSOCIATION Shop.Card_Other member", "ATTRIBUTE Shop.Other.Title member", "ENTITY Shop.Other member"},
		},
		{
			name: "a variable that is not a parameter is still unresolved (control)",
			unit: RefObjectPage,
			doc: bson.D{{Key: "$Type", Value: "Forms$Page"}, {Key: "Name", Value: "PG"},
				{Key: "Parameters", Value: bson.A{int32(3), objectParam("Forms$PageParameter", "Name", "ParameterType", "Card", "Shop.Card")}},
				{Key: "Widgets", Value: bson.A{int32(2), text("$Other/Title")}}},
			want: nil,
		},
		{
			name: "an attribute the parameter's entity does not have (control)",
			unit: RefObjectPage,
			doc: bson.D{{Key: "$Type", Value: "Forms$Page"}, {Key: "Name", Value: "PG"},
				{Key: "Parameters", Value: bson.A{int32(3), objectParam("Forms$PageParameter", "Name", "ParameterType", "Card", "Shop.Card")}},
				{Key: "Widgets", Value: bson.A{int32(2), text("$Card/Missing")}}},
			want: nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := edgeNames(memberRefsInUnit(marshalDoc(t, c.doc), c.unit, varsIndex()))
			if len(got) != len(c.want) {
				t.Fatalf("edges = %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					// want lists are sorted by construction
					t.Fatalf("edges = %v, want %v", got, c.want)
				}
			}
			for _, r := range c.reject {
				for _, g := range got {
					if g == r {
						t.Errorf("edge %s must not be emitted", r)
					}
				}
			}
		})
	}
}
