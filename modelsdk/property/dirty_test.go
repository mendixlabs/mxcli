package property

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
)

func TestPrimitiveBindPropagates(t *testing.T) {
	owner := &element.Base{}
	p := NewPrimitive[string]("Name", DecodeString)
	p.Bind(owner, 0)
	p.Set("hello")
	if !owner.IsDirty() {
		t.Error("owner should be dirty")
	}
	bits := owner.DirtyBits()
	if len(bits) < 1 || bits[0]&(1<<0) == 0 {
		t.Error("bit 0 should be set")
	}
}

func TestPartBindPropagates(t *testing.T) {
	owner := &element.Base{}
	p := NewPart[element.Element]("Gen")
	p.Bind(owner, 3)
	p.Set(&element.Base{})
	if !owner.IsDirty() {
		t.Error("owner should be dirty")
	}
}

func TestPartListAppendSetsContainer(t *testing.T) {
	owner := &element.Base{}
	pl := NewPartList[element.Element]("Attrs")
	pl.Bind(owner, 5)
	child := &element.Base{}
	pl.Append(child)
	if !owner.IsDirty() {
		t.Error("owner should be dirty")
	}
	if child.Container() != owner {
		t.Error("child container should be owner")
	}
}

func TestEnumBindPropagates(t *testing.T) {
	owner := &element.Base{}
	e := NewEnum[string]("Level")
	e.Bind(owner, 7)
	e.Set("Hidden")
	bits2 := owner.DirtyBits()
	if len(bits2) < 1 || bits2[0]&(1<<7) == 0 {
		t.Error("bit 7 should be set")
	}
}

func TestByNameRefBindPropagates(t *testing.T) {
	owner := &element.Base{}
	r := NewByNameRef[element.Element]("Image", "Images$Image")
	r.Bind(owner, 10)
	r.SetQualifiedName("Mod.Img")
	bits3 := owner.DirtyBits()
	if len(bits3) < 1 || bits3[0]&(1<<10) == 0 {
		t.Error("bit 10 should be set")
	}
}

func TestByIdRefBindPropagates(t *testing.T) {
	owner := &element.Base{}
	r := NewByIdRef[element.Element]("child")
	r.Bind(owner, 2)
	r.SetID("some-id")
	bits4 := owner.DirtyBits()
	if len(bits4) < 1 || bits4[0]&(1<<2) == 0 {
		t.Error("bit 2 should be set")
	}
}
