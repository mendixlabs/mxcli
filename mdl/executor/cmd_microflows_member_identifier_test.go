// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
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
