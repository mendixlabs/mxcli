// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// TestSystemDomainModel_UniqueAttrIDs guards the catalog PK invariant: the
// virtual System module's attributes must each have a unique, non-empty ID.
// Empty IDs collide on attributes_data.Id and break every catalog query.
func TestSystemDomainModel_UniqueAttrIDs(t *testing.T) {
	dm := buildSystemDomainModel(nil)
	seen := map[string]string{}
	for _, e := range dm.Entities {
		for _, a := range e.Attributes {
			id := string(a.ID)
			if id == "" {
				t.Errorf("System.%s.%s has empty ID", e.Name, a.Name)
				continue
			}
			if prev, ok := seen[id]; ok {
				t.Errorf("duplicate System attr ID %q: %s and System.%s.%s", id, prev, e.Name, a.Name)
			}
			seen[id] = "System." + e.Name + "." + a.Name
		}
	}
}

// TestSystemDomainModel_StringLengthsReachTheModel is ako/mxcli#584 at the layer
// the reporter met it: the length has to survive meta.SystemAttrDef ->
// domainmodel.StringAttributeType, because that is what `describe entity`
// prints and what the view-entity pass-through rule (MDL031) compares a
// declaration against.
//
// Before #584 every one of these read as 0 — "unlimited" — so `describe entity
// System.User` said `Name: String(unlimited)` while mxbuild builds the table at
// String(100), and a view entity selecting u.Name could not be declared in any
// way that both passed check and built.
func TestSystemDomainModel_StringLengthsReachTheModel(t *testing.T) {
	// Measured on the 11.14.0 deployed model; see
	// modelsdk/meta/testdata/system_string_lengths.txt.
	want := map[string]int{
		"User.Name":                 100,
		"FileDocument.Name":         400,
		"Image.PublicThumbnailPath": 500,
		"Language.Code":             20,
		"HttpMessage.HttpVersion":   10,
		// 0 here is Mendix's "unlimited", and is measured too — it must not be
		// mistaken for the unset value the whole table used to carry.
		"Error.Message": 0,
	}

	dm := buildSystemDomainModel(nil)
	seen := map[string]bool{}
	for _, e := range dm.Entities {
		for _, a := range e.Attributes {
			key := e.Name + "." + a.Name
			expected, ok := want[key]
			if !ok {
				continue
			}
			seen[key] = true
			st, isString := a.Type.(*domainmodel.StringAttributeType)
			if !isString {
				t.Errorf("System.%s: want a string attribute, got %T", key, a.Type)
				continue
			}
			if st.Length != expected {
				t.Errorf("System.%s: read as String(%d), mxbuild builds it as String(%d)", key, st.Length, expected)
			}
		}
	}
	for key := range want {
		if !seen[key] {
			t.Errorf("System.%s is not in the domain model at all", key)
		}
	}
}

// TestSystemDomainModel_SystemMemberFlagsReachTheModel: the measured
// owner/changedBy/createdDate/changedDate flags must survive into the virtual
// domain model, because that is what the executor's XPath checks read. Missing,
// System.FileDocument read as storing no owner and `[System.owner = …]` on any
// of its specializations was refused at check time while mxbuild built it.
func TestSystemDomainModel_SystemMemberFlagsReachTheModel(t *testing.T) {
	dm := buildSystemDomainModel(nil)
	byName := map[string]*domainmodel.Entity{}
	for _, e := range dm.Entities {
		byName[e.Name] = e
	}
	fd := byName["FileDocument"]
	if fd == nil || !fd.HasOwner || !fd.HasChangedBy || !fd.HasCreatedDate || !fd.HasChangedDate {
		t.Errorf("System.FileDocument should store all four system members, got %+v", fd)
	}
	if wf := byName["Workflow"]; wf == nil || !wf.HasOwner || wf.HasChangedBy {
		t.Errorf("System.Workflow should store owner only, got %+v", wf)
	}
	if ur := byName["UserRole"]; ur == nil || ur.HasOwner || ur.HasCreatedDate {
		t.Errorf("System.UserRole stores no system members, got %+v", ur)
	}
}

// The workflow sub-process members exist from Mendix 11.8 (measured from each
// version's modeler assembly). TestApp (11.14.0) reads
// System.WorkflowActivityRecord_WorkflowSubProcess/Caption, which could not
// resolve while the System module lacked them: the binding was written as an
// expression, mx check CE0117. Before 11.8 the names must stay absent, or a
// check accepts what the build then refuses as CE1613.
func TestSystemDomainModel_WorkflowSubProcessByVersion(t *testing.T) {
	atLeast := func(major, minor int) func(int, int) bool {
		return func(ma, mi int) bool { return major > ma || major == ma && minor >= mi }
	}
	has := func(dm *domainmodel.DomainModel) (entity, assoc bool) {
		for _, e := range dm.Entities {
			entity = entity || e.Name == "WorkflowSubProcess"
		}
		for _, a := range dm.Associations {
			assoc = assoc || a.Name == "WorkflowActivityRecord_WorkflowSubProcess"
		}
		return
	}
	if e, a := has(buildSystemDomainModel(atLeast(11, 14))); !e || !a {
		t.Errorf("11.14: entity %v, association %v; want both", e, a)
	}
	if e, a := has(buildSystemDomainModel(atLeast(11, 8))); !e || !a {
		t.Errorf("11.8: entity %v, association %v; want both", e, a)
	}
	if e, a := has(buildSystemDomainModel(atLeast(11, 7))); e || a {
		t.Errorf("11.7: entity %v, association %v; want neither", e, a)
	}
}
