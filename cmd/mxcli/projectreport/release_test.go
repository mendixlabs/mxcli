// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func releaseBaseline() *ReleaseModel {
	return &ReleaseModel{
		Entities: []RelEntity{
			{GUID: "e-order", Name: "Shop.Order", Module: "Shop", Stored: true, Attributes: []RelAttr{
				{GUID: "a-num", Name: "Number", Type: "Integer"},
				{GUID: "a-note", Name: "Note", Type: "String", Length: 200},
				{GUID: "a-total", Name: "Total", Type: "Decimal"},
				{GUID: "a-code", Name: "Code", Type: "String", Length: 50},
				{GUID: "a-calc", Name: "Label", Type: "String", Calculated: true},
				{GUID: "a-keep", Name: "Kept", Type: "Boolean"},
			}},
			{GUID: "e-cust", Name: "Shop.Customer", Module: "Shop", Stored: true, Attributes: []RelAttr{{GUID: "a-name", Name: "Name", Type: "String", Length: 100}}},
			{GUID: "e-acct", Name: "Admin.Account", Module: "Admin", Marketplace: true, Stored: true, Generalization: "System.User"},
			{GUID: "e-gone", Name: "Shop.Legacy", Module: "Shop", Stored: true},
			{GUID: "e-np", Name: "Shop.Filter", Module: "Shop", Stored: false},
			{GUID: "e-log", Name: "Shop.Log", Module: "Shop", Stored: true},
		},
		Associations: []RelAssoc{
			{GUID: "s-oc", Name: "Shop.Order_Customer", From: "e-order", To: "e-cust", FromName: "Shop.Order", ToName: "Shop.Customer", Type: "Reference", StorageFormat: "Column"},
			{GUID: "s-ol", Name: "Shop.Order_Legacy", From: "e-order", To: "e-gone", FromName: "Shop.Order", ToName: "Shop.Legacy", Type: "Reference"},
			{GUID: "s-lo", Name: "Shop.Log_Order", From: "e-log", To: "e-order", FromName: "Shop.Log", ToName: "Shop.Order", Type: "Reference"},
		},
	}
}

// clone deep-copies a model through JSON.
func clone(t *testing.T, m *ReleaseModel) *ReleaseModel {
	t.Helper()
	b, _ := json.Marshal(m)
	var out ReleaseModel
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return &out
}

func entity(m *ReleaseModel, name string) *RelEntity {
	for i := range m.Entities {
		if m.Entities[i].Name == name {
			return &m.Entities[i]
		}
	}
	return nil
}

func attr(e *RelEntity, name string) *RelAttr {
	for i := range e.Attributes {
		if e.Attributes[i].Name == name {
			return &e.Attributes[i]
		}
	}
	return nil
}

func changes(d *ReleaseData) map[string]ReleaseChange {
	out := map[string]ReleaseChange{}
	for _, c := range d.Changes {
		out[c.Kind+" "+c.Name] = c
	}
	return out
}

// The control: an unchanged model, and one renamed and moved everywhere under
// the same GUIDs, destroy nothing — names do not identify data, GUIDs do.
func TestReleaseSameGUIDsDestroyNothing(t *testing.T) {
	old := releaseBaseline()
	if d := CompareReleases(old, clone(t, old)); len(d.Changes) != 0 || d.Tables != 5 {
		t.Fatalf("unchanged model: tables %d, changes %+v", d.Tables, d.Changes)
	}
	next := clone(t, old)
	o := entity(next, "Shop.Order")
	o.Name, o.Module = "Sales.Order", "Sales"
	attr(o, "Total").Name = "Amount"
	next.Associations[0].Name = "Sales.Order_Customer"
	d := CompareReleases(old, next)
	if d.Count(EffectDestroys) != 0 {
		t.Fatalf("renames under the same GUID destroy nothing: %+v", d.Changes)
	}
	cs := changes(d)
	if c := cs["entity Sales.Order"]; c.Effect != EffectMigrates || c.Was != "Shop.Order" || !strings.Contains(c.What, "moved to module Sales") {
		t.Errorf("moved entity: %+v", c)
	}
	if c := cs["attribute Sales.Order.Amount"]; c.Effect != EffectMigrates || c.Was != "Shop.Order.Total" {
		t.Errorf("renamed attribute: %+v", c)
	}
	if c := cs["association Sales.Order_Customer"]; c.Effect != EffectMigrates {
		t.Errorf("renamed association: %+v", c)
	}
}

// ako/mxcli#2424: each way a deploy drops data, found by GUID.
func TestReleaseFindsWhatDestroysData(t *testing.T) {
	old := releaseBaseline()
	next := clone(t, old)
	entity(next, "Admin.Account").GUID = "e-acct-reminted" // same name, new GUID
	o := entity(next, "Shop.Order")
	attr(o, "Code").GUID = "a-code-reminted" // same name, new GUID
	attr(o, "Kept").Calculated = true        // now calculated
	var attrs []RelAttr
	for _, a := range o.Attributes {
		if a.Name != "Note" {
			attrs = append(attrs, a)
		}
	}
	o.Attributes = attrs                    // Note removed
	entity(next, "Shop.Log").Stored = false // no longer persistent
	var ents []RelEntity
	for _, e := range next.Entities {
		if e.Name != "Shop.Legacy" {
			ents = append(ents, e)
		}
	}
	next.Entities = ents // Legacy removed
	next.Associations[0].GUID = "s-oc-reminted"

	d := CompareReleases(old, next)
	cs := changes(d)
	for key, want := range map[string]string{
		"entity Admin.Account":            "GUID changed under the same name",
		"entity Shop.Legacy":              "removed",
		"entity Shop.Log":                 "no longer persistent",
		"attribute Shop.Order.Code":       "GUID changed under the same name",
		"attribute Shop.Order.Note":       "removed",
		"attribute Shop.Order.Kept":       "now calculated",
		"association Shop.Order_Customer": "GUID changed under the same name",
	} {
		c, ok := cs[key]
		if !ok || c.Effect != EffectDestroys || !strings.Contains(c.What, want) {
			t.Errorf("%s: want destroys %q, got %+v", key, want, c)
		}
	}
	// The re-minted entity is not also reported as a new table, nor its
	// re-minted attribute as a new column.
	for _, c := range d.Changes {
		if c.Effect == EffectAdds {
			t.Errorf("a re-mint is one destroy, not a destroy plus an add: %+v", c)
		}
	}
	// An association whose end's table is dropped is not listed again.
	if _, ok := cs["association Shop.Order_Legacy"]; ok {
		t.Error("Order_Legacy goes with Legacy's table; listing it again is noise")
	}
	if c := cs["entity Admin.Account"]; !c.Marketplace || !strings.Contains(c.Evidence, "e-acct") {
		t.Errorf("evidence and Marketplace flag: %+v", c)
	}
	// A calculated attribute and a non-persistent entity never had storage.
	for _, key := range []string{"attribute Shop.Order.Label", "entity Shop.Filter"} {
		if _, ok := cs[key]; ok {
			t.Errorf("%s has no storage and must not be listed", key)
		}
	}
}

func TestReleaseVerifyAndAdds(t *testing.T) {
	old := releaseBaseline()
	next := clone(t, old)
	o := entity(next, "Shop.Order")
	attr(o, "Number").Type = "String"
	attr(o, "Code").Length = 10
	attr(o, "Note").Length = 0 // unlimited: longer, not shorter
	o.Attributes = append(o.Attributes, RelAttr{GUID: "a-status", Name: "Status", Type: "String", Length: 30})
	entity(next, "Shop.Customer").Generalization = "Shop.Party"
	next.Associations[0].Type = "ReferenceSet"
	next.Entities = append(next.Entities, RelEntity{GUID: "e-inv", Name: "Shop.Invoice", Module: "Shop", Stored: true})
	entity(next, "Shop.Filter").Stored = true

	cs := changes(CompareReleases(old, next))
	for key, want := range map[string]string{
		"attribute Shop.Order.Number":     "type changed from Integer to String",
		"attribute Shop.Order.Code":       "length lowered from 50 to 10",
		"entity Shop.Customer":            "generalization changed",
		"association Shop.Order_Customer": "Reference, stored as column to ReferenceSet",
	} {
		if c := cs[key]; c.Effect != EffectVerify || !strings.Contains(c.What, want) {
			t.Errorf("%s: want verify %q, got %+v", key, want, c)
		}
	}
	if _, ok := cs["attribute Shop.Order.Note"]; ok {
		t.Error("a longer string limit is not a risk")
	}
	for key, want := range map[string]string{
		"attribute Shop.Order.Status": "new column",
		"entity Shop.Invoice":         "new entity",
		"entity Shop.Filter":          "now persistent",
	} {
		if c := cs[key]; c.Effect != EffectAdds || !strings.Contains(c.What, want) {
			t.Errorf("%s: want adds %q, got %+v", key, want, c)
		}
	}
}

// The page leads with the verdict, and an accepted drop is shown as accepted.
func TestRenderReleaseVerdictAndAcceptance(t *testing.T) {
	old := releaseBaseline()
	next := clone(t, old)
	var ents []RelEntity
	for _, e := range next.Entities {
		if e.Name != "Shop.Legacy" && e.Name != "Shop.Log" {
			ents = append(ents, e)
		}
	}
	next.Entities = ents
	d := CompareReleases(old, next)
	d.Against = "git v1"
	env, _ := NewEnvelope("release", "Shop", "now", "t", d, ReleaseMethod, ReleaseLimits, nil)
	render := func(ann *Annotations) string {
		doc, err := RenderRelease(env, ann)
		if err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		_ = RenderMarkdown(&b, doc)
		return b.String()
	}
	mustContain(t, render(nil), "This release destroys stored data in 2 place(s). Compared with git v1.", "`Shop.Legacy`", "removed: the table and all its rows are dropped")
	md := render(&Annotations{Items: map[string]Note{"release:entity:Shop.Legacy": {Status: "accepted", Note: "archived to S3 first"}}})
	mustContain(t, md, "destroys stored data in 1 place(s) (and 1 more that are accepted)", "accepted: archived to S3 first")

	same := CompareReleases(old, clone(t, old))
	same.Against = "git v1"
	env, _ = NewEnvelope("release", "Shop", "now", "t", same, ReleaseMethod, ReleaseLimits, nil)
	mustContain(t, render(nil), "No stored data is destroyed.")
}
