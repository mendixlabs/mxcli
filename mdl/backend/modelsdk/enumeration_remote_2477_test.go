// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"bytes"
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#2477: an external entity's enum attribute must use an enumeration
// generated from the contract's EnumType — mx check 11.12.6 refused a plain one
// with CE4597 "should have an enumeration type that was generated from
// 'Vendor.Sales.OrderStatus' in service 'MyFirstModule.Sales'", and accepted
// one with Rest$ODataRemoteEnumerationSource and a
// Rest$ODataRemoteEnumerationValue per value (0 errors). The model had no
// place for either, so the writer could not produce one, and rebuilding a
// Studio Pro-generated enumeration dropped its source.
func TestRemoteEnumerationRoundTrip_2477(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	remote := &model.Enumeration{ContainerID: mod.ID, Name: "OrderStatus2477",
		RemoteService: "MyFirstModule.Sales", RemoteName: "Vendor.Sales.OrderStatus",
		Values: []model.EnumerationValue{{Name: "Open", RemoteName: "Open"}, {Name: "Shipped", RemoteName: "Shipped"}}}
	plain := &model.Enumeration{ContainerID: mod.ID, Name: "Plain2477", Values: []model.EnumerationValue{{Name: "A"}}}
	for _, e := range []*model.Enumeration{remote, plain} {
		if err := b.CreateEnumeration(e); err != nil {
			t.Fatalf("create %s: %v", e.Name, err)
		}
	}

	read := func(id model.ID) *model.Enumeration {
		t.Helper()
		e, err := b.GetEnumeration(id)
		if err != nil || e == nil {
			t.Fatalf("read %s: %v", id, err)
		}
		return e
	}
	got := read(remote.ID)
	if got.RemoteService != "MyFirstModule.Sales" || got.RemoteName != "Vendor.Sales.OrderStatus" ||
		len(got.Values) != 2 || got.Values[0].RemoteName != "Open" || got.Values[1].RemoteName != "Shipped" {
		t.Fatalf("read back = %+v", got)
	}
	// A rebuild from what was read keeps the source.
	got.Documentation = "touched"
	if err := b.UpdateEnumeration(got); err != nil {
		t.Fatal(err)
	}
	raw, _ := b.GetRawUnitBytes(remote.ID)
	for _, want := range []string{"Rest$ODataRemoteEnumerationSource", "Rest$ODataRemoteEnumerationValue", "Vendor.Sales.OrderStatus", "MyFirstModule.Sales"} {
		if !bytes.Contains(raw, []byte(want)) {
			t.Errorf("after update the unit has no %q", want)
		}
	}
	if again := read(remote.ID); again.RemoteName != "Vendor.Sales.OrderStatus" || again.Values[0].RemoteName != "Open" {
		t.Errorf("after update = %+v", again)
	}

	// Control: an ordinary enumeration stores no remote source.
	if p := read(plain.ID); p.RemoteName != "" || p.RemoteService != "" || p.Values[0].RemoteName != "" {
		t.Errorf("plain enumeration = %+v", p)
	}
	rawPlain, _ := b.GetRawUnitBytes(plain.ID)
	if bytes.Contains(rawPlain, []byte("ODataRemoteEnumeration")) {
		t.Error("an ordinary enumeration was given a remote source")
	}
}
