// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"path/filepath"
	"testing"

	"github.com/mendixlabs/mxcli/internal/testutil"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// copyPedApp copies the Studio Pro-authored PedApp fixture into a temp dir. Its
// elements have GUID != $ID, which is the only subject on which a re-minted
// storage GUID is observable (CLAUDE.md, "A GUID Is the Database's Identity").
func copyPedApp(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	if err := testutil.CopyFixture(dst, "../../../testdata/pedapp"); err != nil {
		t.Fatalf("copy PedApp fixture: %v", err)
	}
	return filepath.Join(dst, "PedApp.mpr")
}

// TestIssue627_UpdateAttributePreservesGUID guards ako/mxcli#627: the
// AttributeModifier path of the fluent API (Backend.UpdateAttribute) rebuilt the
// attribute from the semantic model with raw == nil, so the codec minted
// GUID = $ID — the #1119 data-loss class, the column dropped on the next deploy.
// The #1119 write guard refused it, which left the API unusable on any Studio
// Pro-authored attribute.
//
// The subject is a PedApp string attribute whose stored GUID differs from its
// $ID; the comparison is against the GUID read before the write, never against a
// previous run (a re-mint is stable, so a second write is elided).
func TestIssue627_UpdateAttributePreservesGUID(t *testing.T) {
	proj := copyPedApp(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}

	dmID, ent, attr := studioProStringAttribute(t, b)
	before := attributeGUIDs(t, b, dmID, ent.ID)
	ids := attributeIDs(t, b, dmID, ent.ID)
	want := before[attr.Name]
	if want == "" || want == ids[attr.Name] {
		t.Fatalf("subject %s.%s: GUID %q, $ID %q — need a stored GUID that differs from $ID",
			ent.Name, attr.Name, want, ids[attr.Name])
	}

	st := attr.Type.(*domainmodel.StringAttributeType)
	st.Length += 7
	if err := b.UpdateAttribute(dmID, ent.ID, attr); err != nil {
		t.Fatalf("UpdateAttribute: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	after := attributeGUIDs(t, b2, dmID, ent.ID)
	if got := after[attr.Name]; got != want {
		t.Errorf("attribute %s.%s: storage GUID changed %s -> %s", ent.Name, attr.Name, want, got)
	}
	// The siblings pass through as stored bytes; assert it so a future rebuild
	// of the whole list is caught here too.
	for name, g := range before {
		if after[name] != g {
			t.Errorf("sibling attribute %s: storage GUID changed %s -> %s", name, g, after[name])
		}
	}
	// And the edit itself landed — otherwise "GUID unchanged" proves nothing.
	dm, err := b2.GetDomainModel(moduleOfDM(t, b2, dmID))
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	var gotLen int
	for _, e := range dm.Entities {
		if e.ID != ent.ID {
			continue
		}
		for _, a := range e.Attributes {
			if a.Name == attr.Name {
				if s, ok := a.Type.(*domainmodel.StringAttributeType); ok {
					gotLen = s.Length
				}
			}
		}
	}
	if gotLen != st.Length {
		t.Errorf("attribute %s.%s: length = %d after UpdateAttribute, want %d", ent.Name, attr.Name, gotLen, st.Length)
	}
}

// studioProStringAttribute picks the first string attribute with a non-zero
// length in a loadable, non-System domain model.
func studioProStringAttribute(t *testing.T, b *Backend) (model.ID, *domainmodel.Entity, *domainmodel.Attribute) {
	t.Helper()
	dms, err := b.ListDomainModels()
	if err != nil {
		t.Fatalf("ListDomainModels: %v", err)
	}
	for _, d := range dms {
		if _, err := b.loadDomainModelGen(d.ID); err != nil {
			continue
		}
		for _, e := range d.Entities {
			for _, a := range e.Attributes {
				if s, ok := a.Type.(*domainmodel.StringAttributeType); ok && s.Length > 0 {
					return d.ID, e, a
				}
			}
		}
	}
	t.Fatal("no string attribute in a loadable PedApp domain model")
	return "", nil, nil
}

func moduleOfDM(t *testing.T, b *Backend, dmID model.ID) model.ID {
	t.Helper()
	dms, err := b.ListDomainModels()
	if err != nil {
		t.Fatalf("ListDomainModels: %v", err)
	}
	for _, d := range dms {
		if d.ID == dmID {
			return d.ContainerID
		}
	}
	t.Fatalf("domain model %s not found", dmID)
	return ""
}
