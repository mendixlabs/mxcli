// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// kindResolver resolves each anchor to the kind named in its element:
// @Core.Role_Admin is a module role, @Core.ACT_Map a microflow.
type kindResolver struct{}

func (kindResolver) Resolve(a Anchor) (Resolution, error) {
	switch {
	case strings.HasPrefix(a.Element, "Role_"):
		return Resolution{State: Resolved, Module: a.Module, Kind: "module role"}, nil
	case strings.HasPrefix(a.Element, "ENUM_"):
		return Resolution{State: Resolved, Module: a.Module, Kind: "enumeration"}, nil
	case strings.HasPrefix(a.Element, "Missing"):
		return Resolution{State: NotFound}, nil
	}
	return Resolution{State: Resolved, Module: a.Module, Kind: "microflow"}, nil
}

// ako/mxcli#2303: "Four roles" anchored only on module roles is built and can
// never be proven by a test; brain check now says so — and only for that one.
func TestCheckWarnsOnUnprovableRequirements(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	add := func(title string, anchors ...string) Entry {
		e, err := NewRequirement(title, anchors, "01-access", now)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Promote(e, PlanShard("01-access")); err != nil {
			t.Fatal(err)
		}
		return e
	}
	roles := add("Four roles", "@Core.Role_Admin", "@Core.Role_Planner")
	add("Roles mapped from Entra groups", "@Core.Role_Admin", "@Core.ACT_MapGroups") // a microflow too: provable
	add("Leave types", "@Core.ENUM_LeaveType", "@Core.MissingThing")                 // not built: not judged
	add("Unanchored wish")

	shards, _ := s.ListShards()
	rep, err := Check(s, kindResolver{}, shards)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Unprovable) != 1 || rep.Unprovable[0].EntryID != roles.ID || !reflect.DeepEqual(rep.Unprovable[0].Kinds, []string{"module role"}) {
		t.Fatalf("unprovable = %+v", rep.Unprovable)
	}
	if rep.Failed() {
		t.Error("an unprovable requirement is a warning; only --strict fails on it")
	}
	if h := UnprovableHint(roles.ID, rep.Unprovable[0].Kinds); !strings.Contains(h, "anchored only on module role") {
		t.Errorf("hint = %q", h)
	}
}

func TestProvableAndUnprovableKindsDoNotOverlap(t *testing.T) {
	for _, k := range ProvableKinds() {
		if UnprovableKind(k) {
			t.Errorf("%s is in both lists", k)
		}
	}
	if !UnprovableKind("Module Role") || !ProvableKind("Java Action") || UnprovableKind("Rest$SomethingNew") {
		t.Error("kinds are compared case- and space-insensitively, and an unknown kind is not judged")
	}
}
