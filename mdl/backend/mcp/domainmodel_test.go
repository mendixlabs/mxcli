// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

func newPersistentEntity(name string, attrs ...*domainmodel.Attribute) *domainmodel.Entity {
	return &domainmodel.Entity{
		Name:        name,
		Persistable: true,
		Location:    model.Point{X: 100, Y: 100},
		Attributes:  attrs,
	}
}

func attr(name string, t domainmodel.AttributeType) *domainmodel.Attribute {
	return &domainmodel.Attribute{Name: name, Type: t}
}

func TestBuildEntityValue_PlainEntity(t *testing.T) {
	b := &Backend{}
	e := newPersistentEntity("Order",
		attr("Total", &domainmodel.DecimalAttributeType{}),
		attr("Title", &domainmodel.StringAttributeType{Length: 200}),
		attr("Count", &domainmodel.IntegerAttributeType{}),
	)
	v, err := b.buildEntityValue(e)
	if err != nil {
		t.Fatalf("buildEntityValue: %v", err)
	}
	if v.SType != "DomainModels$Entity" || v.Name != "Order" {
		t.Fatalf("unexpected entity header: %+v", v)
	}
	if v.Location == nil || v.Location.X != 100 || v.Location.Y != 100 {
		t.Fatalf("unexpected location: %+v", v.Location)
	}
	want := []struct{ name, typ string }{
		{"Total", "Decimal"}, {"Title", "String"}, {"Count", "Integer"},
	}
	if len(v.Attributes) != len(want) {
		t.Fatalf("got %d attributes, want %d", len(v.Attributes), len(want))
	}
	for i, w := range want {
		got := v.Attributes[i]
		if got.SType != "DomainModels$Attribute" || got.Name != w.name || got.Type != w.typ {
			t.Errorf("attr[%d] = %+v, want name=%s type=%s", i, got, w.name, w.typ)
		}
	}

	// The serialized value must use the PED keys $Type/name/type.
	raw, _ := json.Marshal(v)
	for _, key := range []string{`"$Type":"DomainModels$Entity"`, `"name":"Order"`, `"type":"Decimal"`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("serialized entity missing %s: %s", key, raw)
		}
	}
}

func TestBuildEntityValue_EnumerationRef(t *testing.T) {
	b := &Backend{}
	e := newPersistentEntity("Ticket",
		attr("Status", &domainmodel.EnumerationAttributeType{EnumerationRef: "MyFirstModule.StatusEnum"}),
	)
	v, err := b.buildEntityValue(e)
	if err != nil {
		t.Fatalf("buildEntityValue: %v", err)
	}
	a := v.Attributes[0]
	if a.Type != "Enumeration" || a.EnumerationName != "MyFirstModule.StatusEnum" {
		t.Fatalf("enum attr mapped wrong: %+v", a)
	}
}

func TestBuildEntityValue_BooleanFalseDefaultAllowed(t *testing.T) {
	b := &Backend{}
	boolAttr := attr("IsPaid", &domainmodel.BooleanAttributeType{})
	boolAttr.Value = &domainmodel.AttributeValue{DefaultValue: "false"} // auto-added by the executor
	e := newPersistentEntity("Invoice", boolAttr)
	if _, err := b.buildEntityValue(e); err != nil {
		t.Fatalf("Boolean false default should be allowed (dropped), got: %v", err)
	}
}

func TestBuildEntityValue_RejectsUnsupportedFeatures(t *testing.T) {
	b := &Backend{}
	cases := map[string]func(*domainmodel.Entity){
		// Validation rules (NOT NULL / UNIQUE) are NOT in this list anymore — they
		// are authored at create time via addValidationRules, not rejected here.
		"non-persistent": func(e *domainmodel.Entity) { e.Persistable = false },
		"indexes":        func(e *domainmodel.Entity) { e.Indexes = []*domainmodel.Index{{}} },
		"event handler":  func(e *domainmodel.Entity) { e.EventHandlers = []*domainmodel.EventHandler{{}} },
		"system owner":   func(e *domainmodel.Entity) { e.HasOwner = true },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			e := newPersistentEntity("X", attr("A", &domainmodel.StringAttributeType{}))
			mutate(e)
			if _, err := b.buildEntityValue(e); err == nil {
				t.Fatalf("expected error for %s, got nil", name)
			}
		})
	}
}

func TestBuildAttributeValue_RejectsCalculated(t *testing.T) {
	b := &Backend{}
	calc := attr("Full", &domainmodel.StringAttributeType{})
	calc.Value = &domainmodel.AttributeValue{Type: "CalculatedValue"}
	if _, err := b.buildAttributeValue(calc); err == nil {
		t.Error("expected calculated attribute to be rejected")
	}

	// A default value is no longer rejected here — the constructor doesn't carry
	// it (it's set via a follow-up path-op), so buildAttributeValue accepts it.
	deflt := attr("Qty", &domainmodel.IntegerAttributeType{})
	deflt.Value = &domainmodel.AttributeValue{DefaultValue: "5"}
	if _, err := b.buildAttributeValue(deflt); err != nil {
		t.Errorf("default value should be accepted by buildAttributeValue now, got: %v", err)
	}
}

// Issue: attribute default values via PED path-op (Studio Pro 11.12+).
func TestPedAttributeDefault(t *testing.T) {
	mk := func(typ domainmodel.AttributeType, def string) *domainmodel.Attribute {
		a := attr("A", typ)
		if def != "" {
			a.Value = &domainmodel.AttributeValue{DefaultValue: def}
		}
		return a
	}
	cases := []struct {
		name    string
		attr    *domainmodel.Attribute
		wantVal string
		wantSet bool
	}{
		{"no value", mk(&domainmodel.StringAttributeType{}, ""), "", false},
		{"string default", mk(&domainmodel.StringAttributeType{}, "hi"), "hi", true},
		{"integer default", mk(&domainmodel.IntegerAttributeType{}, "0"), "0", true},
		{"boolean true", mk(&domainmodel.BooleanAttributeType{}, "true"), "true", true},
		{"boolean false dropped", mk(&domainmodel.BooleanAttributeType{}, "false"), "", false},
		// Enums are stored BARE by PED (verified live on 11.12) — the executor
		// already provides the bare value name, sent as-is.
		{"enum bare value", mk(&domainmodel.EnumerationAttributeType{EnumerationRef: "MES.WorkOrderStatus"}, "Draft"), "Draft", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, ok := pedAttributeDefault(c.attr)
			if ok != c.wantSet || v != c.wantVal {
				t.Errorf("pedAttributeDefault = (%q,%v), want (%q,%v)", v, ok, c.wantVal, c.wantSet)
			}
		})
	}

	// Calculated value carries no settable default.
	calc := attr("C", &domainmodel.StringAttributeType{})
	calc.Value = &domainmodel.AttributeValue{Type: "CalculatedValue"}
	if _, ok := pedAttributeDefault(calc); ok {
		t.Error("calculated value should not be a settable default")
	}
}

func TestAttributeDefaultsSupported(t *testing.T) {
	if attributeDefaultsSupported(nil) {
		t.Error("nil version must not be supported")
	}
	if attributeDefaultsSupported(&types.ProjectVersion{MajorVersion: 11, MinorVersion: 11}) {
		t.Error("11.11 must not be supported")
	}
	if !attributeDefaultsSupported(&types.ProjectVersion{MajorVersion: 11, MinorVersion: 12}) {
		t.Error("11.12 must be supported")
	}
	if !attributeDefaultsSupported(&types.ProjectVersion{MajorVersion: 12, MinorVersion: 0}) {
		t.Error("12.0 must be supported")
	}
}

// On an unknown/old version the gate rejects a default up-front; without a
// default it passes regardless of version.
func TestGateAttributeDefaults(t *testing.T) {
	b := &Backend{} // nil reader → version unknown → treated as < 11.12
	withDefault := attr("Qty", &domainmodel.IntegerAttributeType{})
	withDefault.Value = &domainmodel.AttributeValue{DefaultValue: "5"}
	if err := b.gateAttributeDefaults([]*domainmodel.Attribute{withDefault}); err == nil {
		t.Error("expected gate to reject a default on an unknown/old version")
	}
	plain := attr("Name", &domainmodel.StringAttributeType{})
	if err := b.gateAttributeDefaults([]*domainmodel.Attribute{plain}); err != nil {
		t.Errorf("plain attribute should pass the gate, got: %v", err)
	}
}

func TestAssociationMultiplicity(t *testing.T) {
	cases := []struct {
		typ   domainmodel.AssociationType
		owner domainmodel.AssociationOwner
		want  string
	}{
		{domainmodel.AssociationTypeReference, domainmodel.AssociationOwnerDefault, "one_to_many"},
		{domainmodel.AssociationTypeReference, domainmodel.AssociationOwnerBoth, "one_to_one"},
		{domainmodel.AssociationTypeReferenceSet, domainmodel.AssociationOwnerBoth, "many_to_many"},
	}
	for _, c := range cases {
		got, err := associationMultiplicity(&domainmodel.Association{Name: "A", Type: c.typ, Owner: c.owner})
		if err != nil || got != c.want {
			t.Errorf("type=%s owner=%s => %q,%v; want %q", c.typ, c.owner, got, err, c.want)
		}
	}
}

func TestGuardAssociationFeatures_RejectsCustomDeleteBehavior(t *testing.T) {
	a := &domainmodel.Association{
		Name:                "A",
		ChildDeleteBehavior: &domainmodel.DeleteBehavior{Type: domainmodel.DeleteBehaviorTypeDeleteMeAndReferences},
	}
	if err := guardAssociationFeatures(a); err == nil {
		t.Error("expected custom (cascade) delete behavior to be rejected")
	}
	// default keep-references is allowed
	a.ChildDeleteBehavior.Type = domainmodel.DeleteBehaviorTypeDeleteMeButKeepReferences
	if err := guardAssociationFeatures(a); err != nil {
		t.Errorf("keep-references should be allowed, got: %v", err)
	}
}

func TestPedUpdate_SendsAssociationGUIDs(t *testing.T) {
	f := newFakePED(t, func(string, map[string]any) (string, bool) { return "SUCCESS", false })
	b := &Backend{client: f.connectClient(t)}

	err := b.pedUpdate("ObjListV10", pedOpEntry{
		Path: "/associations",
		Operation: pedOperation{Type: "add", Value: pedAssociation{
			SType: "DomainModels$Association", Name: "SalesData_Location",
			ParentEntity: "1f5aa90f-6b84-46d1-86cc-84e5a7ba8311",
			ChildEntity:  "ec764fad-d840-45f5-b595-038662842e51",
			Multiplicity: "one_to_many",
		}},
	})
	if err != nil {
		t.Fatalf("pedUpdate: %v", err)
	}
	call, _ := f.callByName("ped_update_document")
	raw, _ := json.Marshal(call.Args["operations"])
	for _, want := range []string{
		`"path":"/associations"`,
		`"parentEntity":"1f5aa90f-6b84-46d1-86cc-84e5a7ba8311"`,
		`"childEntity":"ec764fad-d840-45f5-b595-038662842e51"`,
		`"multiplicity":"one_to_many"`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("association op missing %s: %s", want, raw)
		}
	}
}

func TestPedAttributeType(t *testing.T) {
	ok := map[string]string{
		"String": "String", "Integer": "Integer", "Boolean": "Boolean",
		"DateTime": "DateTime", "Date": "DateTime", "Enumeration": "Enumeration",
		"AutoNumber": "AutoNumber", "Long": "Long", "Decimal": "Decimal",
		"Binary": "Binary", "HashedString": "HashedString",
	}
	for in, want := range ok {
		got, err := pedAttributeType(in)
		if err != nil || got != want {
			t.Errorf("pedAttributeType(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := pedAttributeType("Reference"); err == nil {
		t.Error("expected unsupported type to error")
	}
}

// --- ALTER ENTITY in-place (rename + documentation; type-change rejection) ---

func TestAttributeTypeFromPED(t *testing.T) {
	cases := map[string]string{
		`{"$Type":"DomainModels$StringAttributeType","length":200}`:                 "String",
		`{"$Type":"DomainModels$IntegerAttributeType"}`:                             "Integer",
		`{"$Type":"DomainModels$LongAttributeType"}`:                                "Long",
		`{"$Type":"DomainModels$DecimalAttributeType"}`:                             "Decimal",
		`{"$Type":"DomainModels$BooleanAttributeType"}`:                             "Boolean",
		`{"$Type":"DomainModels$DateTimeAttributeType"}`:                            "DateTime",
		`{"$Type":"DomainModels$EnumerationAttributeType","enumeration":"M.Color"}`: "Enumeration",
	}
	for raw, want := range cases {
		got := attributeTypeFromPED(json.RawMessage(raw))
		if got == nil || got.GetTypeName() != want {
			t.Errorf("attributeTypeFromPED(%s) = %v, want %s", raw, got, want)
		}
	}
	if attributeTypeFromPED(json.RawMessage(`{"$Type":"DomainModels$WhoKnows"}`)) != nil {
		t.Error("unknown constructor should map to nil")
	}
	if attributeTypeFromPED(nil) != nil {
		t.Error("absent type should map to nil")
	}
	// String length and enum ref are carried through.
	if st, ok := attributeTypeFromPED(json.RawMessage(`{"$Type":"DomainModels$StringAttributeType","length":50}`)).(*domainmodel.StringAttributeType); !ok || st.Length != 50 {
		t.Error("string length not captured")
	}
	if et, ok := attributeTypeFromPED(json.RawMessage(`{"$Type":"DomainModels$EnumerationAttributeType","enumeration":"M.Color"}`)).(*domainmodel.EnumerationAttributeType); !ok || et.EnumerationRef != "M.Color" {
		t.Error("enum ref not captured")
	}
}

func TestSameAttributeType(t *testing.T) {
	s := &domainmodel.StringAttributeType{}
	i := &domainmodel.IntegerAttributeType{}
	if sameAttributeType(s, i) {
		t.Error("String vs Integer should differ")
	}
	if !sameAttributeType(i, &domainmodel.IntegerAttributeType{}) {
		t.Error("Integer vs Integer should match")
	}
	if !sameAttributeType(nil, i) || !sameAttributeType(s, nil) {
		t.Error("an unknown (nil) type must never manufacture a type-change")
	}
	// Date normalises to DateTime, so they must compare equal.
	if !sameAttributeType(&domainmodel.DateAttributeType{}, &domainmodel.DateTimeAttributeType{}) {
		t.Error("Date and DateTime should match")
	}
	a := &domainmodel.EnumerationAttributeType{EnumerationRef: "M.A"}
	if sameAttributeType(a, &domainmodel.EnumerationAttributeType{EnumerationRef: "M.B"}) {
		t.Error("different enum refs should differ")
	}
	if !sameAttributeType(a, &domainmodel.EnumerationAttributeType{EnumerationRef: "M.A"}) {
		t.Error("same enum ref should match")
	}
}

// pedReadResponder scripts a fakePED: ped_read_document returns the given
// path->raw-JSON results; every other tool succeeds.
func pedReadResponder(values map[string]string) func(string, map[string]any) (string, bool) {
	return func(name string, args map[string]any) (string, bool) {
		if name == "ped_check_errors" {
			return "No errors found.", false
		}
		if name != "ped_read_document" {
			return "SUCCESS", false
		}
		paths, _ := args["paths"].([]any)
		var sb strings.Builder
		sb.WriteString(`{"results":[`)
		for idx, p := range paths {
			ps, _ := p.(string)
			v, ok := values[ps]
			if !ok {
				v = "null"
			}
			if idx > 0 {
				sb.WriteString(",")
			}
			fmt.Fprintf(&sb, `{"path":%q,"result":%s}`, ps, v)
		}
		sb.WriteString(`]}`)
		return sb.String(), false
	}
}

func TestRenameAttribute_SetsNameLeaf(t *testing.T) {
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		if name == "ped_check_errors" {
			return "No errors found.", false
		}
		return "SUCCESS", false
	})
	b := &Backend{client: f.connectClient(t)}
	if err := b.renameAttribute("M", 0, []string{"Total", "Name"}, "Total", "Amount"); err != nil {
		t.Fatalf("renameAttribute: %v", err)
	}
	call, ok := f.callByName("ped_update_document")
	if !ok {
		t.Fatal("no ped_update_document sent")
	}
	raw, _ := json.Marshal(call.Args["operations"])
	for _, want := range []string{`"path":"/entities/0/attributes/0/name"`, `"type":"set"`, `"value":"Amount"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("rename op missing %s: %s", want, raw)
		}
	}
}

func TestApplyInPlace_RejectsTypeChange(t *testing.T) {
	f := newFakePED(t, pedReadResponder(map[string]string{
		"/entities/0/documentation":              `""`,
		"/entities/0/attributes/0/type":          `{"$Type":"DomainModels$IntegerAttributeType"}`,
		"/entities/0/attributes/0/documentation": `""`,
	}))
	b := &Backend{client: f.connectClient(t)}
	e := newPersistentEntity("Order", attr("Amount", &domainmodel.DecimalAttributeType{}))
	err := b.applyInPlaceEntityChanges("M", 0, e, []string{"Amount"})
	if err == nil || !strings.Contains(err.Error(), "changing an attribute's type") {
		t.Fatalf("want type-change rejection, got %v", err)
	}
	if _, sent := f.callByName("ped_update_document"); sent {
		t.Error("a rejected type change must not write")
	}
}

func TestApplyInPlace_SetsEntityDocumentation(t *testing.T) {
	f := newFakePED(t, pedReadResponder(map[string]string{
		"/entities/0/documentation":              `""`,
		"/entities/0/attributes/0/type":          `{"$Type":"DomainModels$StringAttributeType","length":200}`,
		"/entities/0/attributes/0/documentation": `""`,
	}))
	b := &Backend{client: f.connectClient(t)}
	e := newPersistentEntity("Order", attr("Name", &domainmodel.StringAttributeType{Length: 200}))
	e.Documentation = "An order"
	if err := b.applyInPlaceEntityChanges("M", 0, e, []string{"Name"}); err != nil {
		t.Fatalf("applyInPlaceEntityChanges: %v", err)
	}
	call, ok := f.callByName("ped_update_document")
	if !ok {
		t.Fatal("expected an entity-documentation write")
	}
	raw, _ := json.Marshal(call.Args["operations"])
	if !strings.Contains(string(raw), `"path":"/entities/0/documentation"`) || !strings.Contains(string(raw), `"value":"An order"`) {
		t.Errorf("entity doc op wrong: %s", raw)
	}
}

func TestApplyInPlace_NoChangeIsNoOp(t *testing.T) {
	f := newFakePED(t, pedReadResponder(map[string]string{
		"/entities/0/documentation":              `"An order"`,
		"/entities/0/attributes/0/type":          `{"$Type":"DomainModels$StringAttributeType","length":200}`,
		"/entities/0/attributes/0/documentation": `""`,
	}))
	b := &Backend{client: f.connectClient(t)}
	e := newPersistentEntity("Order", attr("Name", &domainmodel.StringAttributeType{Length: 200}))
	e.Documentation = "An order"
	if err := b.applyInPlaceEntityChanges("M", 0, e, []string{"Name"}); err != nil {
		t.Fatalf("applyInPlaceEntityChanges: %v", err)
	}
	if _, sent := f.callByName("ped_update_document"); sent {
		t.Error("an identical-state ALTER must not write")
	}
}
