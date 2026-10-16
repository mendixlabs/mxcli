// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func TestIsValidMemberIdentifier(t *testing.T) {
	valid := []string{"Resource", "SkillProfile_Resource", "BuildScheduling.SkillProfile_Resource", "_Under", "A1.B2"}
	for _, v := range valid {
		if !isValidMemberIdentifier(v) {
			t.Errorf("isValidMemberIdentifier(%q) = false, want true", v)
		}
	}
	invalid := []string{
		"",
		`BuildScheduling."SkillProfile_Resource"`, // quoted segment
		`"Price"`,          // quoted bare
		"Has Space",        // space
		"1Leading",         // digit-first segment
		"BuildScheduling.", // empty trailing segment
		".Leading",         // empty leading segment
		"Two..Dots",        // empty middle segment
		"Weird-Dash",       // illegal char
	}
	for _, v := range invalid {
		if isValidMemberIdentifier(v) {
			t.Errorf("isValidMemberIdentifier(%q) = true, want false", v)
		}
	}
}

// TestResolveMemberChange_RejectsQuotedMember is the defense-in-depth guard: if a
// quoted member identifier ever reaches the builder, it must be rejected with a
// clear error rather than serialized into a corrupt .mpr.
func TestResolveMemberChange_RejectsQuotedMember(t *testing.T) {
	fb := &flowBuilder{}
	mc := &microflows.MemberChange{}
	fb.resolveMemberChange(mc, `BuildScheduling."SkillProfile_Resource"`, "BuildScheduling.SkillProfile")

	if mc.AttributeQualifiedName != "" || mc.AssociationQualifiedName != "" {
		t.Errorf("quoted member must not serialize: attr=%q assoc=%q",
			mc.AttributeQualifiedName, mc.AssociationQualifiedName)
	}
	if len(fb.errors) == 0 {
		t.Fatal("expected a validation error for the quoted member, got none")
	}
	if !strings.Contains(fb.errors[0], "invalid member name") {
		t.Errorf("unexpected error: %q", fb.errors[0])
	}
}

// TestResolveMemberChange_UnquotedAssociationResolves confirms the normalized
// (unquoted) association name resolves into the Association slot — the outcome
// the visitor fix produces end-to-end for `SET $x/Module."Assoc" = $y`.
func TestResolveMemberChange_UnquotedAssociationResolves(t *testing.T) {
	moduleID := model.ID("build-scheduling")
	backend := &mock.MockBackend{
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if name == "BuildScheduling" {
				return &model.Module{BaseElement: model.BaseElement{ID: moduleID}, Name: name}, nil
			}
			return nil, nil
		},
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			if id == moduleID {
				return &domainmodel.DomainModel{
					ContainerID: moduleID,
					Associations: []*domainmodel.Association{
						{Name: "SkillProfile_Resource", Type: domainmodel.AssociationTypeReference},
					},
				}, nil
			}
			return nil, nil
		},
	}
	fb := &flowBuilder{backend: backend}
	mc := &microflows.MemberChange{}
	fb.resolveMemberChange(mc, "BuildScheduling.SkillProfile_Resource", "BuildScheduling.SkillProfile")

	if mc.AssociationQualifiedName != "BuildScheduling.SkillProfile_Resource" {
		t.Errorf("AssociationQualifiedName = %q, want %q", mc.AssociationQualifiedName, "BuildScheduling.SkillProfile_Resource")
	}
	if mc.AttributeQualifiedName != "" {
		t.Errorf("AttributeQualifiedName = %q, want empty", mc.AttributeQualifiedName)
	}
	if len(fb.errors) != 0 {
		t.Errorf("unexpected errors: %v", fb.errors)
	}
}

// TestResolveMemberChange_RejectsUnknownAssociation guards FINDINGS #51: a
// one-qualifier member (`Module.Name`) that is not a known association cannot be an
// attribute either, so it must be rejected rather than serialized as an invalid
// Attribute (which yields an unloadable .mpr: "... is not a valid AttributeIdentifier").
func TestResolveMemberChange_RejectsUnknownAssociation(t *testing.T) {
	moduleID := model.ID("m")
	backend := &mock.MockBackend{
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if name == "M" {
				return &model.Module{BaseElement: model.BaseElement{ID: moduleID}, Name: name}, nil
			}
			return nil, nil
		},
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			// Domain model exists but has NO associations — the referenced one is absent.
			return &domainmodel.DomainModel{ContainerID: moduleID}, nil
		},
	}
	fb := &flowBuilder{backend: backend}
	mc := &microflows.MemberChange{}
	fb.resolveMemberChange(mc, "M.Nonexistent_Assoc", "M.Child")

	if mc.AttributeQualifiedName != "" || mc.AssociationQualifiedName != "" {
		t.Errorf("unknown association must not serialize: attr=%q assoc=%q",
			mc.AttributeQualifiedName, mc.AssociationQualifiedName)
	}
	if len(fb.errors) == 0 {
		t.Fatal("expected a validation error for the unknown association, got none")
	}
	if !strings.Contains(fb.errors[0], "not a known association") {
		t.Errorf("unexpected error: %q", fb.errors[0])
	}
}

// TestResolveMemberChange_RejectsAssociationMemberPath guards the through-
// association change member (`Assoc.Attr` — the pre-dot segment is an
// association name, not a module): it is neither `Module.Assoc` (that shape
// needs a real module qualifier) nor an attribute of the changed entity, and
// Mendix's ChangeActionItem cannot represent it at all — mxbuild rejects the
// written form with CE1613, after check and exec both reported success
// (docs/12-bug-reports/2026-10-16-change-member-through-association.md).
func TestResolveMemberChange_RejectsAssociationMemberPath(t *testing.T) {
	backend := &mock.MockBackend{
		// The module list answers the qualifier question: "A_B" is not in it,
		// so the pre-dot segment is confidently not a module.
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{{BaseElement: model.BaseElement{ID: model.ID("hop")}, Name: "Hop"}}, nil
		},
	}
	fb := &flowBuilder{backend: backend}
	mc := &microflows.MemberChange{}
	fb.resolveMemberChange(mc, "A_B.Title", "Hop.A")

	if mc.AttributeQualifiedName != "" || mc.AssociationQualifiedName != "" {
		t.Errorf("association member path must not serialize: attr=%q assoc=%q",
			mc.AttributeQualifiedName, mc.AssociationQualifiedName)
	}
	if len(fb.errors) == 0 {
		t.Fatal("expected a validation error for the association member path, got none")
	}
	for _, want := range []string{"A_B.Title", "retrieve"} {
		if !strings.Contains(fb.errors[0], want) {
			t.Errorf("error should mention %q: %s", want, fb.errors[0])
		}
	}
}
