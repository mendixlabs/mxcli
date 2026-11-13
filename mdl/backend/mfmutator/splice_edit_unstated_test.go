// SPDX-License-Identifier: Apache-2.0

package mfmutator

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#2169 (mendixlabs/mxcli#1830): an in-place edit of an activity is
// written from the statement, and a property MDL has no spelling for is absent
// from it, so it came back at the writer's default. A REST call whose proxy was
// set to Override lost it — RequestProxyType Override → DefaultProxy and the
// host/port expressions gone — when only its timeout was edited, and the
// activity's documentation was cleared the same way.
//
// The statement cannot state these, so the stored value is the only one there
// is: an edit keeps it.

// docDeps is editDeps writing the activity's Documentation as the codec does
// for a statement, which has none.
type docDeps struct{ editDeps }

func (d *docDeps) SerializeObject(o microflows.MicroflowObject) (bson.D, error) {
	doc, err := d.editDeps.SerializeObject(o)
	if err == nil && doc != nil {
		if _, ok := o.(*microflows.ActionActivity); ok {
			doc = append(doc, bson.E{Key: "Documentation", Value: ""})
		}
	}
	return doc, err
}

func restCall(act, proxyType string, proxy any, timeout string) bson.D {
	return el(act, "Microflows$RestCallAction", kv("ErrorHandlingType", "Rollback"),
		kv("ProxyConfiguration", proxy),
		kv("RequestProxyType", proxyType),
		kv("TimeOutExpression", timeout))
}

func overrideProxy(id string) bson.D {
	return el(id, "Microflows$ProxyConfiguration",
		kv("HostExpression", "$proxyHost"), kv("PortExpression", "$proxyPort"),
		kv("UsernameExpression", "$proxyUser"), kv("PasswordExpression", "$proxyPassword"))
}

func editA(t *testing.T, stored, replacement bson.D, storedDoc string) bson.D {
	t.Helper()
	objs := withAction(stored)
	objs[1] = append(objs[1], bson.E{Key: "Documentation", Value: storedDoc})
	raw, err := bson.Marshal(unit(objs, lineFlows()))
	if err != nil {
		t.Fatal(err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	deps := &docDeps{editDeps{action: replacement}}
	m, err := New(d, "unit", deps)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Replace(model.ID(uid("a")), oneActivity()); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if err := m.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	if !m.EditedInPlace(model.ID(uid("a"))) {
		t.Fatal("fixture: the replace was not an in-place edit")
	}
	var got bson.D
	if err := bson.Unmarshal(deps.saved, &got); err != nil {
		t.Fatal(err)
	}
	m2, _ := New(got, "unit", &fakeDeps{})
	a := m2.graph().nodes[uid("a")]
	if a == nil {
		t.Fatal("a is not in the unit")
	}
	return a.doc
}

func TestSplice_EditKeepsRestCallProxy(t *testing.T) {
	a := editA(t,
		restCall("act", "Override", overrideProxy("px"), "300"),
		restCall("", "DefaultProxy", nil, "301"),
		"")
	action := dDoc(a, "Action")
	if got := dString(action, "TimeOutExpression"); got != "301" {
		t.Errorf("TimeOutExpression = %q, want the edited 301", got)
	}
	if got := dString(action, "RequestProxyType"); got != "Override" {
		t.Errorf("RequestProxyType = %q, want the stored Override", got)
	}
	px := dDoc(action, "ProxyConfiguration")
	if px == nil || dString(px, "HostExpression") != "$proxyHost" || binaryID(dGet(px, "$ID")) != uid("px") {
		t.Errorf("ProxyConfiguration = %v, want the stored one", dGet(action, "ProxyConfiguration"))
	}
}

func TestSplice_EditKeepsNoProxy(t *testing.T) {
	a := editA(t,
		restCall("act", "NoProxy", nil, "300"),
		restCall("", "DefaultProxy", nil, "301"),
		"")
	if got := dString(dDoc(a, "Action"), "RequestProxyType"); got != "NoProxy" {
		t.Errorf("RequestProxyType = %q, want the stored NoProxy", got)
	}
}

func TestSplice_EditKeepsActivityDocumentation(t *testing.T) {
	a := editA(t,
		restCall("act", "DefaultProxy", nil, "300"),
		restCall("", "DefaultProxy", nil, "301"),
		"Calls the warehouse.")
	if got := dString(a, "Documentation"); got != "Calls the warehouse." {
		t.Errorf("Documentation = %q, want the stored text", got)
	}
}
