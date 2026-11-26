// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Mendix 11.17 records five more facts in a change data capture service's
// baseline (ako/mxcli#2612): the project's DecimalScale on the baseline, and
// per member AssociationMultiplicity, IsNullable and MaxLength. Studio Pro's CDC
// editor (createBaseline in change-data-capture-editor/editor.js, mxbuild
// 11.17.0) derives them from the domain model. A baseline without them differs
// from the one the editor computes, and mx check reports CE1585 "There are
// changes in your … document": a converted 11.17 project failed after one
// `create or modify change data capture service` that changed nothing.

// The rules, from the editor: an attribute can be empty unless a required rule
// covers it or it is a Boolean, and an AutoNumber cannot; MaxLength is a
// String's length and 0 for anything else; an association's multiplicity is
// ManyToMany for a reference set, OneToOne when both sides own it, else
// OneToMany.
func TestCdcMemberFacts_2612(t *testing.T) {
	str := &domainmodel.Attribute{Name: "Number", Type: &domainmodel.StringAttributeType{Length: 20}}
	req := &domainmodel.Attribute{BaseElement: model.BaseElement{ID: "req"}, Name: "Code", Type: &domainmodel.StringAttributeType{Length: 5}}
	boolean := &domainmodel.Attribute{Name: "Active", Type: &domainmodel.BooleanAttributeType{}}
	auto := &domainmodel.Attribute{Name: "Seq", Type: &domainmodel.AutoNumberAttributeType{}}
	dec := &domainmodel.Attribute{Name: "Total", Type: &domainmodel.DecimalAttributeType{}}
	// The reader names a rule's attribute by QUALIFIED name; the first version
	// of this matched the $ID only and put IsNullable true on a required
	// attribute, which mx check reported as CE1585 on a real 11.17 project.
	byName := &domainmodel.Attribute{Name: "Ref", Type: &domainmodel.StringAttributeType{Length: 9}}
	ent := &domainmodel.Entity{ValidationRules: []*domainmodel.ValidationRule{
		{AttributeID: "req", Type: "Required"}, {AttributeID: "Shop.Order.Ref", Type: "Required"},
		{AttributeID: "Shop.Order.Number", Type: "Unique"}}}

	for _, tc := range []struct {
		attr     *domainmodel.Attribute
		nullable bool
		length   int32
	}{{str, true, 20}, {req, false, 5}, {boolean, false, 0}, {auto, false, 0}, {dec, true, 0}, {byName, false, 9}} {
		if got := cdcAttributeNullable(tc.attr, ent, "Shop.Order."+tc.attr.Name); got != tc.nullable {
			t.Errorf("%s nullable = %v, want %v", tc.attr.Name, got, tc.nullable)
		}
		if got := cdcMaxLength(tc.attr); got != tc.length {
			t.Errorf("%s max length = %d, want %d", tc.attr.Name, got, tc.length)
		}
	}
	for _, tc := range []struct {
		a    *domainmodel.Association
		want string
	}{
		{&domainmodel.Association{Type: domainmodel.AssociationTypeReference, Owner: domainmodel.AssociationOwnerDefault}, "OneToMany"},
		{&domainmodel.Association{Type: domainmodel.AssociationTypeReference, Owner: domainmodel.AssociationOwnerBoth}, "OneToOne"},
		{&domainmodel.Association{Type: domainmodel.AssociationTypeReferenceSet, Owner: domainmodel.AssociationOwnerDefault}, "ManyToMany"},
		{&domainmodel.Association{Type: domainmodel.AssociationTypeReferenceSet, Owner: domainmodel.AssociationOwnerBoth}, "ManyToMany"},
	} {
		if got := cdcAssociationMultiplicity(tc.a); got != tc.want {
			t.Errorf("%s/%s multiplicity = %q, want %q", tc.a.Type, tc.a.Owner, got, tc.want)
		}
	}
}

func cdcTestSources() []resolvedSource {
	return []resolvedSource{{
		entityQN: "Shop.Order", exposedName: "Order", entityGUID: "11111111-1111-1111-1111-111111111111",
		members: []resolvedMember{
			{qualified: "Shop.Order.Number", exposedName: "Number", guid: "22222222-2222-2222-2222-222222222222",
				assocGUID: zeroGUID, cdcType: "String", maxLength: 20, nullable: true},
			{qualified: "Shop.Order.Total", exposedName: "Total", guid: "33333333-3333-3333-3333-333333333333",
				assocGUID: zeroGUID, cdcType: "Decimal", nullable: true},
			{qualified: "Shop.Order_Customer", exposedName: "Customer", isAssoc: true, guid: "44444444-4444-4444-4444-444444444444",
				assocGUID: "55555555-5555-5555-5555-555555555555", cdcType: "ArrayOfInt64", nullable: true, multiplicity: "OneToMany"},
		},
	}}
}

// baselineOf encodes a service and returns its baseline and member documents.
func baselineOf(t *testing.T, facts *cdcBaselineFacts) (bson.Raw, []bson.Raw) {
	t.Helper()
	srcs := cdcTestSources()
	g := buildCdcService(element.ID("66666666-6666-6666-6666-666666666666"), "OrdersCdc", "OrdersCdc", "App", "", "Hidden", false,
		srcs, freshCdcVersions(srcs), facts)
	raw, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		t.Fatal(err)
	}
	base := bson.Raw(raw).Lookup("Baseline").Document()
	var props []bson.Raw
	srcVals, _ := base.Lookup("Sources").Array().Values()
	for _, sv := range srcVals[1:] {
		pv, _ := sv.Document().Lookup("Properties").Array().Values()
		for _, p := range pv[1:] {
			props = append(props, p.Document())
		}
	}
	return base, props
}

// The document Mendix 11.17.0 stores, measured on the issue's fixture after mx
// convert: the five fields, with these values.
func TestBuildCdcService_WritesThe1117BaselineFacts_2612(t *testing.T) {
	base, props := baselineOf(t, &cdcBaselineFacts{decimalScale: 8})
	// Mendix stores the integers as 64-bit, as it does Version.Major.
	if v, ok := base.Lookup("DecimalScale").Int64OK(); !ok || v != 8 {
		t.Errorf("baseline DecimalScale = %v, want 8", base.Lookup("DecimalScale"))
	}
	want := []struct {
		mult     string
		nullable bool
		length   int32
	}{{"", true, 20}, {"", true, 0}, {"OneToMany", true, 0}}
	if len(props) != len(want) {
		t.Fatalf("%d properties", len(props))
	}
	for i, w := range want {
		p := props[i]
		name := p.Lookup("ExposedName").StringValue()
		if v, ok := p.Lookup("AssociationMultiplicity").StringValueOK(); !ok || v != w.mult {
			t.Errorf("%s AssociationMultiplicity = %v, want %q", name, p.Lookup("AssociationMultiplicity"), w.mult)
		}
		if v, ok := p.Lookup("IsNullable").BooleanOK(); !ok || v != w.nullable {
			t.Errorf("%s IsNullable = %v, want %v", name, p.Lookup("IsNullable"), w.nullable)
		}
		if v, ok := p.Lookup("MaxLength").Int64OK(); !ok || v != int64(w.length) {
			t.Errorf("%s MaxLength = %v, want %d", name, p.Lookup("MaxLength"), w.length)
		}
	}
}

// Below 11.17 the keys do not exist in the metamodel: writing them is the
// opposite defect, so none of them appears.
func TestBuildCdcService_WritesNo1117FactsBelow1117_2612(t *testing.T) {
	base, props := baselineOf(t, nil)
	docs := append([]bson.Raw{base}, props...)
	for _, d := range docs {
		for _, k := range []string{"DecimalScale", "AssociationMultiplicity", "IsNullable", "MaxLength"} {
			if _, err := d.LookupErr(k); err == nil {
				t.Errorf("%s written below 11.17: %s", k, strings.SplitN(d.String(), ",", 3)[1])
			}
		}
	}
}

// A fact the baseline recorded that the domain model no longer matches is a
// Major change, as Studio Pro's editor rates it; a baseline that recorded none
// — written before 11.17, or by an mxcli that dropped them — is no change, or
// the first rewrite after an upgrade would start a new topic.
func TestBumpCdcVersions_Facts_2612(t *testing.T) {
	stored := func(facts *model.ChangePropertyFacts) *model.ChangeBaseline {
		return &model.ChangeBaseline{Version: &model.ChangeVersion{Major: 1}, Sources: []*model.VersionedChangeSource{{
			ExposedName: "Order", Version: &model.ChangeVersion{Major: 1},
			Properties: []*model.VersionedChangeProperty{{ExposedName: "Number", Type: "String", Facts: facts}},
		}}}
	}
	now := []resolvedSource{{exposedName: "Order", members: []resolvedMember{
		{exposedName: "Number", cdcType: "String", maxLength: 40, nullable: true}}}}
	facts := &cdcBaselineFacts{decimalScale: 8}
	ver := func(v cdcVersions) string { s := v.source["Order"]; return s.String() }

	if got := bumpCdcVersions(stored(&model.ChangePropertyFacts{IsNullable: true, MaxLength: 20}), now, facts); ver(got) != "2.0" {
		t.Errorf("a recorded length 20 -> 40: source %s, want a major bump", ver(got))
	}
	if got := bumpCdcVersions(stored(&model.ChangePropertyFacts{IsNullable: true, MaxLength: 40}), now, facts); ver(got) != "1.0" {
		t.Errorf("recorded facts unchanged: source %s, want 1.0", ver(got))
	}
	if got := bumpCdcVersions(stored(nil), now, facts); ver(got) != "1.0" {
		t.Errorf("no recorded facts: source %s, want 1.0", ver(got))
	}
}

// An association both sides own (owner Both) is tracked from its TO entity as
// well, pointing back at the FROM entity (ako/mxcli#2613): Studio Pro offers it
// on both entities, and mxbuild reports CE1583 until both track it. An owner
// Default association stays FROM-side only.
func TestResolveCdcMember_OwnerBothFromTheToSide_2613(t *testing.T) {
	order := &domainmodel.Entity{BaseElement: model.BaseElement{ID: "order"}, Name: "Order"}
	tag := &domainmodel.Entity{BaseElement: model.BaseElement{ID: "tag"}, Name: "Tag"}
	both := &domainmodel.Association{BaseElement: model.BaseElement{ID: "both"}, Name: "Order_Primary",
		ParentID: "order", ChildID: "tag", Type: domainmodel.AssociationTypeReference, Owner: domainmodel.AssociationOwnerBoth}
	def := &domainmodel.Association{BaseElement: model.BaseElement{ID: "def"}, Name: "Order_Tag",
		ParentID: "order", ChildID: "tag", Type: domainmodel.AssociationTypeReference, Owner: domainmodel.AssociationOwnerDefault}
	dm := &domainmodel.DomainModel{Entities: []*domainmodel.Entity{order, tag}, Associations: []*domainmodel.Association{both, def}}
	guid := func(_ *domainmodel.DomainModel, id model.ID) (string, error) { return "g-" + string(id), nil }
	b := New()

	from, err := b.resolveCdcMember(dm, order, "Shop", "Order_Primary", guid)
	if err != nil || from.assocGUID != "g-tag" || from.multiplicity != "OneToOne" {
		t.Errorf("from the FROM side: %+v, %v; want it to point at Tag", from, err)
	}
	to, err := b.resolveCdcMember(dm, tag, "Shop", "Order_Primary", guid)
	if err != nil || to.assocGUID != "g-order" || to.guid != "g-both" || to.qualified != "Shop.Order_Primary" || !to.isAssoc {
		t.Errorf("from the TO side: %+v, %v; want the same association pointing back at Order", to, err)
	}
	if _, err := b.resolveCdcMember(dm, tag, "Shop", "Order_Tag", guid); err == nil {
		t.Error("an owner Default association was accepted from its TO side")
	}
}
