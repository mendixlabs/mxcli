// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#1829: a document that names an entity as a type, or navigates to
// one, had no refs edge to it, so `show references`, `impact` and the unused-
// entity check missed the document, and an incremental build did not rebuild it
// when the entity changed. Measured on FactoryManagement: 65 reads of another
// document with no edge. The shapes below are the stored BSON of those reads.
//
// The other half: prose and module-role names were read as if they could be
// references. An annotation that mentions `Mod.Assoc` is not a use of it.

func entityTypeRefsIndex() *memberRefIndex {
	return &memberRefIndex{
		attributes:   map[string]string{"Shop.Order.Total": "", "Shop.Customer.Name": ""},
		associations: map[string]bool{"Shop.Order_Customer": true},
		enumerations: map[string]bool{"Shop.Status": true},
		enumValues:   map[string]bool{"Shop.Status.Open": true},
		entities:     map[string]bool{"Shop.Order": true, "Shop.Customer": true, "Shop.Line": true, "Shop.User": true},
	}
}

func edgesOf(t *testing.T, doc bson.D, sourceType string) []string {
	t.Helper()
	raw, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range memberRefsInUnit(raw, sourceType, entityTypeRefsIndex()) {
		got = append(got, e.RefKind+" "+e.TargetType+" "+e.TargetName)
	}
	sort.Strings(got)
	return got
}

func objs(items ...any) bson.A { return append(bson.A{int32(3)}, items...) }

func TestEntityTypeRefs_Issue1829(t *testing.T) {
	flow := bson.D{
		{Key: "$Type", Value: "Microflows$Microflow"},
		{Key: "Name", Value: "Flow"},
		// A module role that happens to share an entity's name is not the entity.
		{Key: "AllowedModuleRoles", Value: bson.A{int32(1), "Shop.User"}},
		{Key: "ObjectCollection", Value: bson.D{{Key: "Objects", Value: objs(
			// create list of Shop.Line
			bson.D{{Key: "$Type", Value: "Microflows$ActionActivity"}, {Key: "Action", Value: bson.D{
				{Key: "$Type", Value: "Microflows$CreateListAction"}, {Key: "Entity", Value: "Shop.Line"}}}},
			// a call whose result variable is typed as Shop.Order
			bson.D{{Key: "$Type", Value: "Microflows$ActionActivity"}, {Key: "Action", Value: bson.D{
				{Key: "$Type", Value: "Microflows$MicroflowCallAction"},
				{Key: "ResultHandling", Value: bson.D{
					{Key: "$Type", Value: "Microflows$ResultHandling"},
					{Key: "VariableType", Value: bson.D{{Key: "$Type", Value: "DataTypes$ListType"}, {Key: "Entity", Value: "Shop.Order"}}}}},
				{Key: "MicroflowCall", Value: bson.D{{Key: "$Type", Value: "Microflows$MicroflowCall"}, {Key: "ParameterMappings", Value: objs(
					// an association path that ends on an entity in an argument
					bson.D{{Key: "$Type", Value: "Microflows$MicroflowCallParameterMapping"},
						{Key: "Argument", Value: "$Order/Shop.Order_Customer/Shop.Customer"}},
				)}}}}}},
			// an entity-type argument to a Java or JavaScript action
			bson.D{{Key: "$Type", Value: "Microflows$ActionActivity"}, {Key: "Action", Value: bson.D{
				{Key: "$Type", Value: "Microflows$JavaActionCallAction"},
				{Key: "ParameterMappings", Value: objs(
					bson.D{{Key: "$Type", Value: "Microflows$JavaActionParameterMapping"}, {Key: "Value", Value: bson.D{
						{Key: "$Type", Value: "Microflows$EntityTypeCodeActionParameterValue"}, {Key: "Entity", Value: "Shop.User"}}}},
					// a value the enumeration does not have: still a use of the enumeration
					bson.D{{Key: "$Type", Value: "Microflows$JavaActionParameterMapping"}, {Key: "Value", Value: bson.D{
						{Key: "$Type", Value: "Microflows$BasicCodeActionParameterValue"}, {Key: "Argument", Value: "Shop.Status.Closed"}}}},
				)}}}},
			// prose: an annotation naming an attribute path is not a use of it
			bson.D{{Key: "$Type", Value: "Microflows$Annotation"},
				{Key: "Caption", Value: "Sums Shop.Order/Total for each Shop.Order"}},
		)}}},
	}
	want := []string{
		"member ASSOCIATION Shop.Order_Customer",
		"member ENTITY Shop.Customer",
		"type ENTITY Shop.Line",
		"type ENTITY Shop.Order",
		"type ENTITY Shop.User",
		"value ENUMERATION Shop.Status",
	}
	if got := edgesOf(t, flow, RefObjectMicroflow); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("flow edges:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	snippet := bson.D{
		{Key: "$Type", Value: "Forms$Snippet"},
		{Key: "Name", Value: "Snip"},
		{Key: "Parameters", Value: objs(bson.D{
			{Key: "$Type", Value: "Forms$SnippetParameter"}, {Key: "Name", Value: "Order"},
			{Key: "ParameterType", Value: bson.D{{Key: "$Type", Value: "DataTypes$ObjectType"}, {Key: "Entity", Value: "Shop.Order"}}}})},
		{Key: "Widgets", Value: objs(
			// a data view over an association: the step's destination entity
			bson.D{{Key: "$Type", Value: "Forms$DataView"}, {Key: "DataSource", Value: bson.D{
				{Key: "$Type", Value: "Forms$DataViewSource"},
				{Key: "EntityRef", Value: bson.D{{Key: "$Type", Value: "DomainModels$IndirectEntityRef"}, {Key: "Steps", Value: objs(
					bson.D{{Key: "$Type", Value: "DomainModels$EntityRefStep"},
						{Key: "Association", Value: "Shop.Order_Customer"},
						{Key: "DestinationEntity", Value: "Shop.Customer"}})}}}}}},
			// static text naming an entity is prose
			bson.D{{Key: "$Type", Value: "Forms$Text"}, {Key: "Caption", Value: bson.D{
				{Key: "$Type", Value: "Texts$Text"}, {Key: "Items", Value: objs(bson.D{
					{Key: "$Type", Value: "Texts$Translation"}, {Key: "Text", Value: "Shows Shop.Customer/Name of Shop.Line"}})}}}},
		)},
	}
	want = []string{
		"member ASSOCIATION Shop.Order_Customer",
		"member ENTITY Shop.Customer",
		"parameter ENTITY Shop.Order",
	}
	if got := edgesOf(t, snippet, RefObjectSnippet); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("snippet edges:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// Controls: the edges above are not emitted for an entity name anywhere a
// string happens to hold one. A page parameter has its typed `parameter` edge
// already (buildReferences), and a flow parameter its `parameter` edge, so the
// walk must not add a second row for either.
func TestEntityTypeRefs_NoDuplicateOfTypedEdges(t *testing.T) {
	page := bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "Name", Value: "P"},
		{Key: "Parameters", Value: objs(bson.D{
			{Key: "$Type", Value: "Forms$PageParameter"}, {Key: "Name", Value: "Order"},
			{Key: "ParameterType", Value: bson.D{{Key: "$Type", Value: "DataTypes$ObjectType"}, {Key: "Entity", Value: "Shop.Order"}}}})},
	}
	if got := edgesOf(t, page, RefObjectPage); len(got) != 0 {
		t.Errorf("page parameter: want no walk edge (the typed pass emits it), got %v", got)
	}
	flow := bson.D{
		{Key: "$Type", Value: "Microflows$Microflow"},
		{Key: "Name", Value: "F"},
		{Key: "MicroflowReturnType", Value: bson.D{{Key: "$Type", Value: "DataTypes$ObjectType"}, {Key: "Entity", Value: "Shop.Order"}}},
		{Key: "ObjectCollection", Value: bson.D{{Key: "Objects", Value: objs(
			bson.D{{Key: "$Type", Value: "Microflows$MicroflowParameterObject"}, {Key: "Name", Value: "Order"},
				{Key: "VariableType", Value: bson.D{{Key: "$Type", Value: "DataTypes$ObjectType"}, {Key: "Entity", Value: "Shop.Order"}}}},
			// a bare entity name in an expression, not after an association: a cast
			// or a type check, which the split pass and typed passes handle
			bson.D{{Key: "$Type", Value: "Microflows$ExclusiveSplit"}, {Key: "SplitCondition", Value: bson.D{
				{Key: "$Type", Value: "Microflows$ExpressionSplitCondition"}, {Key: "Expression", Value: "$Order = empty or Shop.Status.Open = $S"}}}},
		)}}},
	}
	want := "value ENUMERATION_VALUE Shop.Status.Open"
	if got := edgesOf(t, flow, RefObjectMicroflow); strings.Join(got, "\n") != want {
		t.Errorf("flow: want only %q, got %v", want, got)
	}
}
