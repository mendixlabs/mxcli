// SPDX-License-Identifier: Apache-2.0

// Regression test for the "change with association loses member name" drift.
//
// Scenario: a microflow iterates a list whose element type cannot be determined
// at build time (e.g., the list came from a java action whose return type
// isn't registered in varTypes). The CHANGE statement inside the loop receives
// `entityQN == ""` in resolveMemberChange — the old code returned early,
// leaving both AssociationQualifiedName and AttributeQualifiedName empty.
// The writer then emitted `"Association": ""` / no `"Attribute"` at all, and
// the describer re-rendered the change as `change $var ( = $value)` — invalid
// MDL that won't re-execute.
//
// Fix: when the entity type is unknown, fall back to the dot-contains heuristic
// so the qualified name the user authored is preserved.
package executor

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func TestResolveMemberChange_UnknownEntityPreservesQualifiedAssociationName(t *testing.T) {
	fb := &flowBuilder{
		// backend is nil AND entityQN is empty — this is the exact case
		// hit when the change target's variable type wasn't registered.
	}
	mc := &microflows.MemberChange{}
	fb.resolveMemberChange(mc, "MxKafka.Message_MessageOverview", "")

	if mc.AssociationQualifiedName != "MxKafka.Message_MessageOverview" {
		t.Errorf("expected association name preserved, got %q", mc.AssociationQualifiedName)
	}
	if mc.AttributeQualifiedName != "" {
		t.Errorf("expected empty attribute, got %q", mc.AttributeQualifiedName)
	}
}

func TestResolveMemberChange_UnknownEntityPreservesBareAttributeName(t *testing.T) {
	fb := &flowBuilder{}
	mc := &microflows.MemberChange{}
	// Bare member name with unknown entity type — should be treated as attribute
	// (bare names are attributes in MDL; associations require a Module. prefix).
	fb.resolveMemberChange(mc, "Offset", "")

	if mc.AttributeQualifiedName != "Offset" {
		t.Errorf("expected attribute name preserved, got %q", mc.AttributeQualifiedName)
	}
	if mc.AssociationQualifiedName != "" {
		t.Errorf("expected empty association, got %q", mc.AssociationQualifiedName)
	}
}

func TestResolveMemberChange_UnknownEntityEmptyMemberName(t *testing.T) {
	// Defensive: empty member name stays empty on both fields — better to emit
	// nothing than fabricate a phantom attribute. The caller is responsible for
	// not invoking us with empty names, but we keep the guard.
	fb := &flowBuilder{}
	mc := &microflows.MemberChange{}
	fb.resolveMemberChange(mc, "", "")

	if mc.AttributeQualifiedName != "" || mc.AssociationQualifiedName != "" {
		t.Errorf("expected both empty on empty input, got attr=%q assoc=%q",
			mc.AttributeQualifiedName, mc.AssociationQualifiedName)
	}
}

// TestResolveMemberChange_UnknownEntityQualifiedAttributeStaysAttribute covers
// the codex review finding: a name with two or more dots is a qualified
// attribute (`Module.Entity.Attribute`), not an association. MDL association
// names always have exactly one dot (`Module.AssociationName`) because they
// are qualified by module only; any additional dot indicates an
// entity.attribute path.
func TestResolveMemberChange_UnknownEntityQualifiedAttributeStaysAttribute(t *testing.T) {
	fb := &flowBuilder{}
	mc := &microflows.MemberChange{}
	// Authored shape: `change $x (MyModule.MyEntity.Offset = 1)` with
	// entityQN unknown (variable type not registered).
	fb.resolveMemberChange(mc, "MyModule.MyEntity.Offset", "")

	if mc.AttributeQualifiedName != "MyModule.MyEntity.Offset" {
		t.Errorf("qualified attribute: got %q, want %q",
			mc.AttributeQualifiedName, "MyModule.MyEntity.Offset")
	}
	if mc.AssociationQualifiedName != "" {
		t.Errorf("qualified attribute was mistakenly classified as association: %q",
			mc.AssociationQualifiedName)
	}
}
