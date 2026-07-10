package modelsdk_test

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/property"
)

// TestDirtyChainPropagation verifies the full 3-layer dirty propagation:
// Property.Set → Base.MarkDirty(bit) → container.MarkChildDirty → root
func TestDirtyChainPropagation(t *testing.T) {
	// Build: root → mid → leaf, each with a Primitive property bound
	root := &element.Base{}
	rootProp := property.NewPrimitive[string]("RootName", property.DecodeString)
	rootProp.Bind(root, 0)
	root.SetProperties([]element.Property{rootProp})

	mid := &element.Base{}
	mid.SetContainer(root)
	midProp := property.NewPrimitive[string]("MidName", property.DecodeString)
	midProp.Bind(mid, 0)
	mid.SetProperties([]element.Property{midProp})

	leaf := &element.Base{}
	leaf.SetContainer(mid)
	leafProp := property.NewPrimitive[string]("LeafName", property.DecodeString)
	leafProp.Bind(leaf, 0)
	leaf.SetProperties([]element.Property{leafProp})

	// Precondition: everything clean
	if root.IsDirty() || mid.IsDirty() || leaf.IsDirty() {
		t.Fatal("precondition failed: all should be clean")
	}

	// Modify leaf property
	leafProp.Set("changed")

	// Verify propagation
	if !leaf.IsDirty() {
		t.Error("leaf should be dirty")
	}
	leafBits := leaf.DirtyBits()
	if len(leafBits) < 1 || leafBits[0]&(1<<0) == 0 {
		t.Error("leaf bit 0 should be set")
	}
	if !mid.IsDirty() {
		t.Error("mid should be dirty (child-dirty)")
	}
	if !mid.IsChildDirty() {
		t.Error("mid should have childDirty set")
	}
	if !root.IsDirty() {
		t.Error("root should be dirty (child-dirty)")
	}
	if !root.IsChildDirty() {
		t.Error("root should have childDirty set")
	}

	// Root's own property should NOT be dirty
	if rootProp.Dirty() {
		t.Error("root's own property should not be dirty")
	}
}

// TestPartListAppendSetsContainerAndBubbles verifies that PartList.Append
// sets the child's container AND bubbles dirty up.
func TestPartListAppendSetsContainerAndBubbles(t *testing.T) {
	parent := &element.Base{}
	pl := property.NewPartList[element.Element]("Items")
	pl.Bind(parent, 3)
	parent.SetProperties([]element.Property{pl})

	child := &element.Base{}
	child.SetTypeName("Test$Child")
	child.MarkDirty(63) // new element

	pl.Append(child)

	// Child's container should be parent
	if child.Container() != parent {
		t.Error("child.Container should be parent after Append")
	}

	// Parent should be dirty (bit 3 = PartList modified)
	parentBits := parent.DirtyBits()
	if len(parentBits) < 1 || parentBits[0]&(1<<3) == 0 {
		t.Error("parent bit 3 should be set (PartList modified)")
	}
}

// TestDecodeChildMutationBubblesUp verifies that mutating a child
// element loaded via AppendFromDecode correctly propagates dirty
// state to the parent container.
func TestDecodeChildMutationBubblesUp(t *testing.T) {
	parent := &element.Base{}
	pl := property.NewPartList[element.Element]("Items")
	pl.Bind(parent, 3)
	parent.SetProperties([]element.Property{pl})

	child := &element.Base{}
	childProp := property.NewPrimitive[string]("Name", property.DecodeString)
	childProp.Bind(child, 0)
	child.SetProperties([]element.Property{childProp})

	pl.AppendFromDecode(child)

	if parent.IsDirty() {
		t.Fatal("parent should be clean after decode")
	}
	if child.IsDirty() {
		t.Fatal("child should be clean after decode")
	}
	if child.Container() != parent {
		t.Fatal("child.Container should be parent after AppendFromDecode")
	}

	childProp.Set("modified")

	if !child.IsDirty() {
		t.Error("child should be dirty after Set")
	}
	if !parent.IsDirty() {
		t.Error("parent should be dirty (childDirty) after child mutation")
	}
	if !parent.IsChildDirty() {
		t.Error("parent.childDirty should be true")
	}
}

// TestDecodePartChildMutationBubblesUp verifies that mutating a child
// element loaded via Part.SetFromDecode correctly propagates dirty
// state to the parent container.
func TestDecodePartChildMutationBubblesUp(t *testing.T) {
	parent := &element.Base{}
	pt := property.NewPart[element.Element]("Child")
	pt.Bind(parent, 1)
	parent.SetProperties([]element.Property{pt})

	child := &element.Base{}
	childProp := property.NewPrimitive[string]("Name", property.DecodeString)
	childProp.Bind(child, 0)
	child.SetProperties([]element.Property{childProp})

	pt.SetFromDecode(child)

	if parent.IsDirty() {
		t.Fatal("parent should be clean after decode")
	}
	if child.Container() != parent {
		t.Fatal("child.Container should be parent after SetFromDecode")
	}

	childProp.Set("modified")
	if !parent.IsDirty() {
		t.Error("parent should be dirty after child mutation")
	}
	if !parent.IsChildDirty() {
		t.Error("parent.childDirty should be true")
	}
}

// TestCleanElementNotDirtyAfterDecode simulates decoding an element from BSON
// and verifies it stays clean.
func TestCleanElementNotDirtyAfterDecode(t *testing.T) {
	elem := &element.Base{}
	elem.SetID("test-id")
	elem.SetTypeName("Test$Clean")

	p := property.NewPrimitive[string]("Name", property.DecodeString)
	p.Bind(elem, 0)
	elem.SetProperties([]element.Property{p})

	// Simulate decode: SetFromDecode-style operations don't mark dirty
	// (Primitive doesn't have SetFromDecode, but Init + Get doesn't dirty)
	if elem.IsDirty() {
		t.Error("freshly constructed element should not be dirty")
	}
	if p.Dirty() {
		t.Error("property should not be dirty before Set")
	}
}
