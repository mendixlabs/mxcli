// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/canon"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	"github.com/mendixlabs/mxcli/modelsdk/version"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#2256: `mx convert --in-place` from 11.16.0 to 11.16.1 rewrote units
// mxcli had written, ADDING a key to three element types. Each is a property
// name Studio Pro always writes and mxcli left out — the mendixlabs/mxcli#1373
// class, which makes the merge engine throw once Studio Pro saves the document.
// The key sets and values below are the converter's own output for the same
// elements in the reporting project (11.16.1).

// storedForm encodes el with the engine's encoder and gives it the form the mpr
// writer stores it in (canon.CompleteForStorage, as normalizeForStorage does).
func storedForm(t *testing.T, el element.Element, pv string) bson.D {
	t.Helper()
	raw, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	v := version.Parse(pv)
	out, err := canon.CompleteForStorage(raw, &v, true)
	if err != nil {
		t.Fatalf("CompleteForStorage: %v", err)
	}
	var doc bson.D
	if err := bson.Unmarshal(out, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return doc
}

func i2256Keys(d bson.D) string {
	keys := make([]string, 0, len(d))
	for _, e := range d {
		keys = append(keys, e.Key)
	}
	sort.Strings(keys)
	return strings.Join(keys, " ")
}

func i2256Lookup(d bson.D, key string) (any, bool) {
	for _, e := range d {
		if e.Key == key {
			return e.Value, true
		}
	}
	return nil, false
}

// findType returns the first element of $Type typeName in v.
func i2256Find(v any, typeName string) bson.D {
	switch x := v.(type) {
	case bson.D:
		if s, _ := i2256Lookup(x, "$Type"); s == typeName {
			return x
		}
		for _, e := range x {
			if d := i2256Find(e.Value, typeName); d != nil {
				return d
			}
		}
	case bson.A:
		for _, e := range x {
			if d := i2256Find(e, typeName); d != nil {
				return d
			}
		}
	}
	return nil
}

func TestIssue2256_ValidationFeedbackOnAssociationWritesEmptyAttribute(t *testing.T) {
	a := &microflows.ValidationFeedbackAction{
		ObjectVariable:  "BomLine",
		AssociationName: "ServiceDesk.BomLine_Part",
		Template:        &model.Text{Translations: map[string]string{"en_US": "Choose a part."}},
	}
	a.ID = "aaaaaaaa-2256-4000-8000-000000000001"
	doc := storedForm(t, microflowActionToGen(a), "11.16.1")

	const converter = "$ID $Type Association Attribute ErrorHandlingType FeedbackTemplate ValidationVariableName"
	if got := i2256Keys(doc); got != converter {
		t.Errorf("keys = %s\nwant the converter's %s", got, converter)
	}
	if v, ok := i2256Lookup(doc, "Attribute"); !ok || v != "" {
		t.Errorf("Attribute = %#v (present %v), want the converter's empty string", v, ok)
	}
	if v, _ := i2256Lookup(doc, "Association"); v != "ServiceDesk.BomLine_Part" {
		t.Errorf("Association = %#v", v)
	}
}

// Control: a feedback on an attribute keeps it; the completion only adds.
func TestIssue2256_ValidationFeedbackOnAttributeKeepsIt(t *testing.T) {
	a := &microflows.ValidationFeedbackAction{
		ObjectVariable: "BomLine",
		AttributeName:  "ServiceDesk.BomLine.Quantity",
		Template:       &model.Text{Translations: map[string]string{"en_US": "At least one."}},
	}
	a.ID = "aaaaaaaa-2256-4000-8000-000000000002"
	doc := storedForm(t, microflowActionToGen(a), "11.16.1")
	if v, _ := i2256Lookup(doc, "Attribute"); v != "ServiceDesk.BomLine.Quantity" {
		t.Errorf("Attribute = %#v, want the authored attribute", v)
	}
	if v, ok := i2256Lookup(doc, "Association"); !ok || v != "" {
		t.Errorf("Association = %#v (present %v), want empty string", v, ok)
	}
}

func retrieveWithRange(t *testing.T, r *microflows.Range) bson.D {
	t.Helper()
	src := &microflows.DatabaseRetrieveSource{EntityQualifiedName: "ServiceDesk.WikiPage", Range: r}
	src.ID = "aaaaaaaa-2256-4000-8000-000000000010"
	act := &microflows.RetrieveAction{OutputVariable: "Pages", Source: src}
	act.ID = "aaaaaaaa-2256-4000-8000-000000000011"
	doc := storedForm(t, microflowActionToGen(act), "11.16.1")
	rng := i2256Find(doc, "Microflows$CustomRange")
	if rng == nil {
		t.Fatalf("no Microflows$CustomRange in %v", doc)
	}
	return rng
}

func TestIssue2256_RetrieveLimitWithoutOffsetWritesEmptyOffset(t *testing.T) {
	rng := retrieveWithRange(t, &microflows.Range{RangeType: microflows.RangeTypeCustom, Limit: "5"})
	const converter = "$ID $Type LimitExpression OffsetExpression"
	if got := i2256Keys(rng); got != converter {
		t.Errorf("keys = %s\nwant the converter's %s", got, converter)
	}
	if v, ok := i2256Lookup(rng, "OffsetExpression"); !ok || v != "" {
		t.Errorf("OffsetExpression = %#v (present %v), want the converter's empty string", v, ok)
	}
	if v, _ := i2256Lookup(rng, "LimitExpression"); v != "5" {
		t.Errorf("LimitExpression = %#v", v)
	}
}

// Control: an authored offset is kept.
func TestIssue2256_RetrieveWithOffsetKeepsIt(t *testing.T) {
	rng := retrieveWithRange(t, &microflows.Range{RangeType: microflows.RangeTypeCustom, Limit: "5", Offset: "10"})
	if v, _ := i2256Lookup(rng, "OffsetExpression"); v != "10" {
		t.Errorf("OffsetExpression = %#v, want the authored 10", v)
	}
}

func TestIssue2256_AccessRuleWithoutMembersWritesEmptyMemberAccesses(t *testing.T) {
	ar := &domainmodel.AccessRule{
		ModuleRoleNames:           []string{"ServiceDesk.User"},
		DefaultMemberAccessRights: "ReadWrite",
	}
	doc := storedForm(t, accessRuleToGen(ar), "11.16.1")
	const converter = "$ID $Type AllowCreate AllowDelete AllowedModuleRoles DefaultMemberAccessRights Documentation MemberAccesses XPathConstraint XPathConstraintCaption"
	if got := i2256Keys(doc); got != converter {
		t.Errorf("keys = %s\nwant the converter's %s", got, converter)
	}
	v, ok := i2256Lookup(doc, "MemberAccesses")
	arr, isArr := v.(bson.A)
	if !ok || !isArr || len(arr) != 1 || arr[0] != int32(3) {
		t.Errorf("MemberAccesses = %#v (present %v), want the converter's [3] — an empty list with its marker", v, ok)
	}
}

// Control: a rule's member entries are kept, not replaced by the empty list.
func TestIssue2256_AccessRuleWithMembersKeepsThem(t *testing.T) {
	ar := &domainmodel.AccessRule{
		ModuleRoleNames:           []string{"ServiceDesk.User"},
		DefaultMemberAccessRights: "None",
		MemberAccesses: []*domainmodel.MemberAccess{
			{AttributeName: "ServiceDesk.PlaceList.Name", AccessRights: "ReadOnly"},
		},
	}
	doc := storedForm(t, accessRuleToGen(ar), "11.16.1")
	v, _ := i2256Lookup(doc, "MemberAccesses")
	arr, _ := v.(bson.A)
	if len(arr) != 2 || arr[0] != int32(3) {
		t.Fatalf("MemberAccesses = %#v, want the marker and the one authored entry", v)
	}
	ma, _ := arr[1].(bson.D)
	if attr, _ := i2256Lookup(ma, "Attribute"); attr != "ServiceDesk.PlaceList.Name" {
		t.Errorf("member entry = %v, want the authored attribute", ma)
	}
}
