// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// The reference graph had no ATTRIBUTE, ENUMERATION or ENUMERATION_VALUE
// targets at all, no microflow -> workflow edge, no page -> association edge and
// no mapping -> entity edge. `impact Module.Entity.Attr` therefore answered
// "(no impact - element is not referenced)" for an attribute a microflow writes
// and a page displays — measured on Evora Factory Management, where
// DigitalTwin.Machine.NumberOfIncidents is set by a change activity and bound on
// DigitalTwin.Machine_Details. An agent acting on that answer deletes a live
// attribute.
//
// The fixture runs the two real passes over documents shaped like the stored
// BSON (key names checked against Evora's units): buildXPathExpressions, then
// buildReferences, and reads what landed in refs.

const memberRefsModuleID = model.ID("mod-shop")

func memberRefsFixture(t *testing.T) *Catalog {
	t.Helper()

	cat, err := New()
	if err != nil {
		t.Fatalf("new catalog: %v", err)
	}
	t.Cleanup(func() { cat.Close() })

	tx, err := cat.db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}

	// The tables buildReferences reads its name sets from. buildEntities,
	// buildAssociations and buildEnumerations fill them in a real build.
	seed := []string{
		`INSERT INTO entities_data (Id, Name, QualifiedName, ModuleName, Generalization) VALUES
			('e1', 'Order', 'Shop.Order', 'Shop', ''),
			('e2', 'Customer', 'Shop.Customer', 'Shop', ''),
			('e3', 'SpecialOrder', 'Shop.SpecialOrder', 'Shop', 'Shop.Order')`,
		`INSERT INTO attributes_data (Id, Name, EntityId, EntityQualifiedName, ModuleName, DataType, EnumerationQualifiedName) VALUES
			('a1', 'Status', 'e1', 'Shop.Order', 'Shop', 'Enumeration', 'Shop.OrderStatus'),
			('a2', 'Total', 'e1', 'Shop.Order', 'Shop', 'Decimal', ''),
			('a3', 'Code', 'e1', 'Shop.Order', 'Shop', 'String', ''),
			('a4', 'Name', 'e2', 'Shop.Customer', 'Shop', 'String', ''),
			('a5', 'Unused', 'e2', 'Shop.Customer', 'Shop', 'String', '')`,
		`INSERT INTO associations_data (Id, Name, QualifiedName, ModuleName, FromEntity, ToEntity) VALUES
			('as1', 'Order_Customer', 'Shop.Order_Customer', 'Shop', 'Shop.Order', 'Shop.Customer')`,
		`INSERT INTO enumerations_data (Id, Name, QualifiedName, ModuleName) VALUES
			('en1', 'OrderStatus', 'Shop.OrderStatus', 'Shop')`,
		`INSERT INTO enumeration_values_data (Id, EnumerationId, EnumerationQualifiedName, ModuleName, Name) VALUES
			('v1', 'en1', 'Shop.OrderStatus', 'Shop', 'Open'),
			('v2', 'en1', 'Shop.OrderStatus', 'Shop', 'Closed')`,
	}
	for _, s := range seed {
		if _, err := tx.Exec(s); err != nil {
			t.Fatalf("seed: %v\n%s", err, s)
		}
	}

	// The typed microflow: a call-workflow activity and two XPath retrieves.
	mf := &microflows.Microflow{
		BaseElement: model.BaseElement{ID: "mf-1"},
		ContainerID: memberRefsModuleID,
		Name:        "ACT_Start",
		ObjectCollection: &microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{
			newAction("act-wf", &microflows.WorkflowCallAction{Workflow: "Shop.WF_Approve"}),
			newAction("act-r1", &microflows.RetrieveAction{Source: &microflows.DatabaseRetrieveSource{
				EntityQualifiedName: "Shop.Order",
				XPathConstraint:     "[Shop.Order_Customer/Shop.Customer/Name = $n and Status = 'Open' and Code != empty]",
			}}),
			newAction("act-r2", &microflows.RetrieveAction{Source: &microflows.DatabaseRetrieveSource{
				EntityQualifiedName: "Shop.SpecialOrder",
				XPathConstraint:     "[Total > 5]",
			}}),
		}},
	}

	// The same microflow as stored: the member refs are read from the raw
	// document, so a site no typed struct models is still covered.
	mfRaw := bson.D{
		{Key: "$Type", Value: "Microflows$Microflow"},
		{Key: "Name", Value: "ACT_Start"},
		// Prose is not a reference: a name mentioned here must not become an edge.
		{Key: "Documentation", Value: "Shop.Order.Code"},
		{Key: "ObjectCollection", Value: bson.D{
			{Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
			{Key: "Objects", Value: bson.A{int32(3),
				bson.D{
					{Key: "$Type", Value: "Microflows$ActionActivity"},
					{Key: "Action", Value: bson.D{
						{Key: "$Type", Value: "Microflows$ChangeAction"},
						{Key: "Items", Value: bson.A{int32(3),
							bson.D{
								{Key: "$Type", Value: "Microflows$MemberChange"},
								{Key: "Association", Value: ""},
								{Key: "Attribute", Value: "Shop.Order.Total"},
								{Key: "Value", Value: "$Total"},
							},
							bson.D{
								{Key: "$Type", Value: "Microflows$MemberChange"},
								{Key: "Association", Value: "Shop.Order_Customer"},
								{Key: "Attribute", Value: ""},
								{Key: "Value", Value: "$Customer"},
							},
						}},
					}},
				},
				bson.D{
					{Key: "$Type", Value: "Microflows$ExclusiveSplit"},
					{Key: "SplitCondition", Value: bson.D{
						{Key: "$Type", Value: "Microflows$ExpressionSplitCondition"},
						{Key: "Expression", Value: "$Order/Status = Shop.OrderStatus.Closed"},
					}},
				},
				bson.D{
					{Key: "$Type", Value: "Microflows$ActionActivity"},
					{Key: "Action", Value: bson.D{
						{Key: "$Type", Value: "Microflows$CreateVariableAction"},
						{Key: "VariableType", Value: bson.D{
							{Key: "$Type", Value: "DataTypes$EnumerationType"},
							{Key: "Enumeration", Value: "Shop.OrderStatus"},
						}},
					}},
				},
			}},
		}},
	}

	// A page binding an attribute in a text template and navigating an
	// association in a data source — neither is a widget's own AttributeRef.
	pageRaw := bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "Name", Value: "Order_Edit"},
		{Key: "Widgets", Value: bson.A{int32(3),
			bson.D{
				{Key: "$Type", Value: "Forms$DynamicText"},
				{Key: "Content", Value: bson.D{
					{Key: "$Type", Value: "Forms$ClientTemplate"},
					{Key: "Parameters", Value: bson.A{int32(2),
						bson.D{
							{Key: "$Type", Value: "Forms$ClientTemplateParameter"},
							{Key: "AttributeRef", Value: bson.D{
								{Key: "$Type", Value: "DomainModels$AttributeRef"},
								{Key: "Attribute", Value: "Shop.Order.Code"},
							}},
						},
					}},
				}},
			},
			bson.D{
				{Key: "$Type", Value: "Forms$ListView"},
				{Key: "DataSource", Value: bson.D{
					{Key: "$Type", Value: "Forms$AssociationSource"},
					{Key: "EntityRef", Value: bson.D{
						{Key: "$Type", Value: "DomainModels$IndirectEntityRef"},
						{Key: "Steps", Value: bson.A{int32(2),
							bson.D{
								{Key: "$Type", Value: "DomainModels$EntityRefStep"},
								{Key: "Association", Value: "Shop.Order_Customer"},
								{Key: "DestinationEntity", Value: "Shop.Customer"},
							},
						}},
					}},
				}},
			},
		}},
	}

	mappingRaw := bson.D{
		{Key: "$Type", Value: "ImportMappings$ImportMapping"},
		{Key: "Name", Value: "IMM_Order"},
		{Key: "Elements", Value: bson.A{int32(2),
			bson.D{
				{Key: "$Type", Value: "ImportMappings$ObjectMappingElement"},
				{Key: "Entity", Value: "Shop.Order"},
				{Key: "Children", Value: bson.A{int32(2),
					bson.D{
						{Key: "$Type", Value: "ImportMappings$ValueMappingElement"},
						{Key: "Attribute", Value: "Shop.Order.Total"},
					},
				}},
			},
		}},
	}

	// A message definition exposes an entity, its attributes and an
	// association, each by qualified name (#1781). Two storage shapes: before
	// 11.15 the definitions sit inside a MessageDefinitionCollection; from 11.15
	// each is a MessageDefinition2 document of its own.
	exposedOrder := bson.D{
		{Key: "$Type", Value: "MessageDefinitions$ExposedEntity"},
		{Key: "Entity", Value: "Shop.Order"},
		{Key: "Children", Value: bson.A{int32(2),
			bson.D{
				{Key: "$Type", Value: "MessageDefinitions$ExposedAttribute"},
				{Key: "Attribute", Value: "Shop.Order.Total"},
			},
			bson.D{
				{Key: "$Type", Value: "MessageDefinitions$ExposedAssociation"},
				{Key: "Association", Value: "Shop.Order_Customer"},
				{Key: "Entity", Value: "Shop.Customer"},
			},
		}},
	}
	mdCollectionRaw := bson.D{
		{Key: "$Type", Value: "MessageDefinitions$MessageDefinitionCollection"},
		{Key: "Name", Value: "MD_Order"},
		{Key: "MessageDefinitions", Value: bson.A{int32(2),
			bson.D{
				{Key: "$Type", Value: "MessageDefinitions$EntityMessageDefinition"},
				{Key: "Name", Value: "Order"},
				{Key: "ExposedEntity", Value: exposedOrder},
			},
		}},
	}
	mdDocumentRaw := bson.D{
		{Key: "$Type", Value: "MessageDefinitions$MessageDefinition2"},
		{Key: "Name", Value: "MD2_Order"},
		{Key: "ExposedEntity", Value: exposedOrder},
	}

	rawUnit := func(id, typ string, doc bson.D) *types.RawUnit {
		b, err := bson.Marshal(doc)
		if err != nil {
			t.Fatalf("marshal %s: %v", id, err)
		}
		return &types.RawUnit{ID: model.ID(id), ContainerID: memberRefsModuleID, Type: typ, Contents: b}
	}
	units := []*types.RawUnit{
		rawUnit("mf-1", "Microflows$Microflow", mfRaw),
		rawUnit("pg-1", "Forms$Page", pageRaw),
		rawUnit("im-1", "ImportMappings$ImportMapping", mappingRaw),
		rawUnit("mdc-1", "MessageDefinitions$MessageDefinitionCollection", mdCollectionRaw),
		rawUnit("md2-1", "MessageDefinitions$MessageDefinition2", mdDocumentRaw),
	}

	b := &Builder{
		catalog: cat,
		reader: &mock.MockBackend{
			ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
				return []*microflows.Microflow{mf}, nil
			},
			ListRawUnitsByTypeFunc: func(prefix string) ([]*types.RawUnit, error) {
				var out []*types.RawUnit
				for _, u := range units {
					if prefix == "" || u.Type == prefix || len(u.Type) >= len(prefix) && u.Type[:len(prefix)] == prefix {
						out = append(out, u)
					}
				}
				return out, nil
			},
			GetNavigationFunc: func() (*types.NavigationDocument, error) {
				return &types.NavigationDocument{}, nil
			},
		},
		snapshot: &Snapshot{ID: "snap-1"},
		hierarchy: &hierarchy{
			moduleIDs:       map[model.ID]bool{memberRefsModuleID: true},
			moduleNames:     map[model.ID]string{memberRefsModuleID: "Shop"},
			containerParent: map[model.ID]model.ID{},
			folderNames:     map[model.ID]string{},
		},
		tx:       wrapTx(tx),
		fullMode: true,
	}

	if err := b.buildXPathExpressions(); err != nil {
		t.Fatalf("buildXPathExpressions: %v", err)
	}
	if err := b.buildReferences(); err != nil {
		t.Fatalf("buildReferences: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return cat
}

type refRow struct{ sourceType, sourceName, targetType, targetName, kind string }

func refsTo(t *testing.T, cat *Catalog, target string) map[refRow]int {
	t.Helper()
	rows, err := cat.db.Query(`SELECT SourceType, SourceName, TargetType, TargetName, RefKind FROM refs WHERE TargetName = ?`, target)
	if err != nil {
		t.Fatalf("query refs: %v", err)
	}
	defer rows.Close()
	got := map[refRow]int{}
	for rows.Next() {
		var r refRow
		if err := rows.Scan(&r.sourceType, &r.sourceName, &r.targetType, &r.targetName, &r.kind); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[r]++
	}
	return got
}

func TestReferencesReachMembersEnumerationsWorkflowsAndMappings(t *testing.T) {
	cat := memberRefsFixture(t)

	want := []refRow{
		// microflow -> workflow: the call-workflow activity.
		{"MICROFLOW", "Shop.ACT_Start", "WORKFLOW", "Shop.WF_Approve", "call"},
		// Attributes, from every kind of site.
		{"MICROFLOW", "Shop.ACT_Start", "ATTRIBUTE", "Shop.Order.Total", "member"},               // change activity
		{"PAGE", "Shop.Order_Edit", "ATTRIBUTE", "Shop.Order.Code", "member"},                    // text template parameter
		{"IMPORT_MAPPING", "Shop.IMM_Order", "ATTRIBUTE", "Shop.Order.Total", "member"},          // value mapping element
		{"MICROFLOW", "Shop.ACT_Start", "ATTRIBUTE", "Shop.Customer.Name", "xpath"},              // path segment after an entity
		{"MICROFLOW", "Shop.ACT_Start", "ATTRIBUTE", "Shop.Order.Status", "xpath"},               // bare name on the retrieved entity
		{"MICROFLOW", "Shop.ACT_Start", "ATTRIBUTE", "Shop.Order.Code", "xpath"},                 // compared to empty
		{"MICROFLOW", "Shop.ACT_Start", "ATTRIBUTE", "Shop.Order.Total", "xpath"},                // inherited: retrieve of the specialization
		{"MICROFLOW", "Shop.ACT_Start", "ASSOCIATION", "Shop.Order_Customer", "member"},          // change activity
		{"MICROFLOW", "Shop.ACT_Start", "ASSOCIATION", "Shop.Order_Customer", "xpath"},           // xpath path
		{"PAGE", "Shop.Order_Edit", "ASSOCIATION", "Shop.Order_Customer", "member"},              // widget -> association
		{"IMPORT_MAPPING", "Shop.IMM_Order", "ENTITY", "Shop.Order", "mapping"},                  // mapping -> entity
		{"ENTITY", "Shop.Order", "ENUMERATION", "Shop.OrderStatus", "type"},                      // attribute type
		{"MICROFLOW", "Shop.ACT_Start", "ENUMERATION", "Shop.OrderStatus", "type"},               // variable type
		{"MICROFLOW", "Shop.ACT_Start", "ENUMERATION_VALUE", "Shop.OrderStatus.Closed", "value"}, // expression
		{"MICROFLOW", "Shop.ACT_Start", "ENUMERATION_VALUE", "Shop.OrderStatus.Open", "xpath"},   // enum attribute compared to a literal
	}

	for _, w := range want {
		got := refsTo(t, cat, w.targetName)
		if got[w] == 0 {
			t.Errorf("missing edge %s %s -> %s %s (%s); refs to %s: %v",
				w.sourceType, w.sourceName, w.targetType, w.targetName, w.kind, w.targetName, got)
		}
		if got[w] > 1 {
			t.Errorf("edge %v emitted %d times; one document using a member twice is one edge", w, got[w])
		}
	}

	// Prose in Documentation is not a use, so the only microflow edge to
	// Shop.Order.Code is the XPath one.
	for r := range refsTo(t, cat, "Shop.Order.Code") {
		if r.sourceType == "MICROFLOW" && r.kind != "xpath" {
			t.Errorf("documentation text produced an edge: %v", r)
		}
	}
	// An attribute nothing uses stays unreferenced — the control that shows
	// the walk matches names rather than emitting every attribute it knows.
	if got := refsTo(t, cat, "Shop.Customer.Unused"); len(got) != 0 {
		t.Errorf("unused attribute has references: %v", got)
	}
}

// A message definition that exposes an entity, an attribute and an
// association is a use of all three (#1781): without the edges, renaming or
// deleting any of them looked safe while the definition still depended on it.
// Both storage shapes, the collection and the 11.15+ document.
func TestReferencesReachMessageDefinitions(t *testing.T) {
	cat := memberRefsFixture(t)

	for _, src := range []refRow{
		{sourceType: "MESSAGE_DEFINITION_COLLECTION", sourceName: "Shop.MD_Order"},
		{sourceType: "MESSAGE_DEFINITION", sourceName: "Shop.MD2_Order"},
	} {
		for _, w := range []refRow{
			{src.sourceType, src.sourceName, "ENTITY", "Shop.Order", "mapping"},
			{src.sourceType, src.sourceName, "ENTITY", "Shop.Customer", "mapping"},
			{src.sourceType, src.sourceName, "ATTRIBUTE", "Shop.Order.Total", "member"},
			{src.sourceType, src.sourceName, "ASSOCIATION", "Shop.Order_Customer", "member"},
		} {
			if got := refsTo(t, cat, w.targetName); got[w] != 1 {
				t.Errorf("edge %s %s -> %s %s (%s): %d, want 1; refs to %s: %v",
					w.sourceType, w.sourceName, w.targetType, w.targetName, w.kind, got[w], w.targetName, got)
			}
		}
	}

	// Control: what no definition exposes has no message-definition edge, so
	// the edges above come from the exposure, not from every name the walk knows.
	for r := range refsTo(t, cat, "Shop.Order.Code") {
		if strings.HasPrefix(r.sourceType, "MESSAGE_DEFINITION") {
			t.Errorf("unexposed attribute has a message-definition edge: %v", r)
		}
	}
}

// scanPaths is the resolver both XPath constraints and expressions go through.
// Each case pins one way a bare word could be mistaken for a member, or a real
// member missed.
func TestScanPaths(t *testing.T) {
	idx := &memberRefIndex{
		attributes: map[string]string{
			"Shop.Order.Status": "Shop.OrderStatus", "Shop.Order.Code": "",
			"Shop.Order.empty": "", "Shop.Order.contains": "", "Shop.Customer.Name": "",
		},
		associations:   map[string]bool{"Shop.Order_Customer": true},
		enumValues:     map[string]bool{"Shop.OrderStatus.Open": true},
		entities:       map[string]bool{"Shop.Order": true, "Shop.Customer": true},
		generalization: map[string]string{},
	}
	cases := []struct {
		name, text, context string
		want                []string
	}{
		{"bare attribute on the context entity", "[Code = 'x']", "Shop.Order", []string{"ATTRIBUTE Shop.Order.Code"}},
		{"no context entity resolves nothing bare", "[Code = 'x']", "", nil},
		{"keyword that is also an attribute name", "[Code != empty]", "Shop.Order", []string{"ATTRIBUTE Shop.Order.Code"}},
		{"function name that is also an attribute name", "[contains(Code, 'x')]", "Shop.Order", []string{"ATTRIBUTE Shop.Order.Code"}},
		{"XPath token between percent signs", "[Code = '[%CurrentUser%]' or Code = %CurrentDateTime%]", "Shop.Order", []string{"ATTRIBUTE Shop.Order.Code"}},
		{"predicate after a path is evaluated on the path's entity",
			"[Shop.Order_Customer/Shop.Customer[Name = 'x']]", "Shop.Order",
			[]string{"ASSOCIATION Shop.Order_Customer", "ATTRIBUTE Shop.Customer.Name", "ENTITY Shop.Customer"}},
		{"enumeration attribute compared to a literal names the value", "[Status = 'Open']", "Shop.Order",
			[]string{"ATTRIBUTE Shop.Order.Status", "ENUMERATION_VALUE Shop.OrderStatus.Open"}},
		{"a literal not compared to the enum attribute names nothing", "[Status = Code and Code = 'Open']", "Shop.Order",
			[]string{"ATTRIBUTE Shop.Order.Code", "ATTRIBUTE Shop.Order.Status"}},
		{"expression: association path from a variable", "$o/Shop.Order_Customer/Shop.Customer/Name", "",
			[]string{"ASSOCIATION Shop.Order_Customer", "ATTRIBUTE Shop.Customer.Name", "ENTITY Shop.Customer"}},
		{"expression: bare member of a variable is not resolvable", "$o/Code", "", nil},
		{"expression: qualified enumeration value", "if $o/Status = Shop.OrderStatus.Open then 1 else 2", "",
			[]string{"ENUMERATION_VALUE Shop.OrderStatus.Open"}},
		{"a qualified name inside a string literal is text", "'Shop.OrderStatus.Open'", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var set edgeSet
			scanPaths(tc.text, tc.context, idx, func(tt, n string) { set.add(tt, n, "") })
			var got []string
			for _, e := range set.sorted() {
				got = append(got, e.TargetType+" "+e.TargetName)
			}
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Errorf("scanPaths(%q, %q) = %v, want %v", tc.text, tc.context, got, tc.want)
			}
		})
	}
}
