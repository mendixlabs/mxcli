// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"bytes"
	"crypto/sha256"
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// rebuilt is raw as a rebuild of the same document would encode it: every
// element with a new $ID, and every 16-byte binary that held one of those ids
// (a pointer to the element) following it. The new ids are derived from the
// old, so the result is the same on every run.
func rebuilt(t *testing.T, raw []byte) []byte {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ids := map[string][]byte{}
	var collect func(v any)
	collect = func(v any) {
		switch x := v.(type) {
		case bson.D:
			for _, e := range x {
				if b, ok := e.Value.(bson.Binary); ok && e.Key == "$ID" && len(b.Data) == 16 {
					sum := sha256.Sum256(append([]byte("rebuilt"), b.Data...))
					ids[string(b.Data)] = sum[:16]
				}
				collect(e.Value)
			}
		case bson.A:
			for _, e := range x {
				collect(e)
			}
		}
	}
	collect(doc)
	var remint func(v any) any
	remint = func(v any) any {
		switch x := v.(type) {
		case bson.D:
			for i := range x {
				x[i].Value = remint(x[i].Value)
			}
		case bson.A:
			for i := range x {
				x[i] = remint(x[i])
			}
		case bson.Binary:
			if to, ok := ids[string(x.Data)]; ok {
				return bson.Binary{Subtype: x.Subtype, Data: append([]byte(nil), to...)}
			}
		}
		return v
	}
	out, err := bson.Marshal(remint(doc))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return out
}

// Reconcile is defined as its steps run on the bytes alone, each parsing what
// it reads (reconcileInPasses). It now parses the stored document once for the
// steps that only read it, skips the PersistentId carry for a document without
// the property, and checks the id substitution without parsing. It must return
// the same bytes and the same verdict: compared on every PedApp unit against
// itself, against a rebuild of itself (new ids throughout), against a rebuild
// that left properties out, and the other way round.
func TestReconcile_ReturnsWhatItsStepsReturnOnTheBytesAlone(t *testing.T) {
	units := pedAppUnits(t)
	compared, unchangedCount, changedBytes := 0, 0, 0
	for name, stored := range units {
		pairs := [][2][]byte{
			{stored, stored},
			{rebuilt(t, stored), stored},
			{thinned(t, rebuilt(t, stored), 7), stored},
			{stored, thinned(t, rebuilt(t, stored), 7)},
			{withoutUserRoleGUIDs(t, rebuilt(t, stored)), stored},
		}
		for i, pair := range pairs {
			contents, against := pair[0], pair[1]
			// The translations option is compared on the rebuild, where a carry
			// has something to do.
			optionSets := [][]Option{nil}
			if i == 1 {
				optionSets = append(optionSets, []Option{ContentsOwnTranslations()})
			}
			for _, opts := range optionSets {
				want, wantSame := reconcileInPasses(append([]byte(nil), contents...), against, opts...)
				got, gotSame := Reconcile(append([]byte(nil), contents...), against, opts...)
				compared++
				if gotSame != wantSame {
					t.Fatalf("%s pair %d: unchanged %v, the steps on the bytes alone give %v", name, i, gotSame, wantSame)
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("%s pair %d: %d bytes, the steps on the bytes alone give %d different ones", name, i, len(got), len(want))
				}
				if wantSame {
					unchangedCount++
				}
				if !bytes.Equal(want, contents) {
					changedBytes++
				}
			}
		}
	}
	t.Logf("%d comparisons: %d judged unchanged, %d where the reconcile rewrote the contents", compared, unchangedCount, changedBytes)
	// Controls: unchanged and changed verdicts, and reconciles that carried
	// something onto the contents, must all have been compared.
	if unchangedCount == 0 || unchangedCount == compared || changedBytes == 0 {
		t.Fatalf("the comparison did not cover every outcome: %d unchanged of %d, %d rewritten", unchangedCount, compared, changedBytes)
	}
}

// The fixture holds no workflow, and a workflow's elements are the only ones
// with a PersistentId, so the carry Reconcile now skips for a document without
// the property is compared here on one that has it: a rebuild that minted new
// PersistentIds must get the stored ones back, by both routes.
func TestReconcile_StillCarriesPersistentIDs(t *testing.T) {
	bin := func(b byte) bson.Binary {
		return bson.Binary{Data: bytes.Repeat([]byte{b}, 16)}
	}
	workflow := func(rootID, actID, actPersistent byte) []byte {
		raw, err := bson.Marshal(bson.D{
			{Key: "$ID", Value: bin(rootID)},
			{Key: "$Type", Value: "Workflows$Workflow"},
			{Key: "Name", Value: "W"},
			{Key: "Activities", Value: bson.A{int32(3), bson.D{
				{Key: "$ID", Value: bin(actID)},
				{Key: "$Type", Value: "Workflows$UserTask"},
				{Key: "Name", Value: "Task"},
				{Key: "PersistentId", Value: bin(actPersistent)},
			}}},
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return raw
	}
	stored := workflow(1, 2, 3)
	contents := workflow(11, 12, 13)

	want, wantSame := reconcileInPasses(append([]byte(nil), contents...), stored)
	got, gotSame := Reconcile(append([]byte(nil), contents...), stored)
	if !bytes.Equal(got, want) || gotSame != wantSame {
		t.Fatalf("Reconcile gives %d bytes (unchanged %v), the steps on the bytes alone %d (unchanged %v)", len(got), gotSame, len(want), wantSame)
	}
	if !wantSame {
		t.Fatal("control: a rebuild that differs only in minted ids was not judged unchanged, so the carry did not run")
	}
	if !bytes.Contains(got, bytes.Repeat([]byte{3}, 16)) || bytes.Contains(got, bytes.Repeat([]byte{13}, 16)) {
		t.Error("the stored PersistentId was not carried onto the rebuild")
	}
}

// The ids the substitution check reads where they lie are the ids it read by
// parsing the document.
func TestElementIDSetOf_ReadsTheIDsParsingRead(t *testing.T) {
	units := pedAppUnits(t)
	total := 0
	for name, stored := range units {
		for _, raw := range [][]byte{stored, rebuilt(t, stored), thinned(t, stored, 5)} {
			want, wantOK := elementIDSetByParsing(raw)
			got, gotOK := elementIDSetOf(raw)
			if gotOK != wantOK || !reflect.DeepEqual(got, want) {
				t.Fatalf("%s: read %d ids (ok %v), parsing reads %d (ok %v)", name, len(got), gotOK, len(want), wantOK)
			}
			total += len(want)
		}
	}
	if total == 0 {
		t.Fatal("control: no ids were read, so the comparison is of empty sets")
	}
	for _, junk := range [][]byte{nil, {}, []byte("not a document")} {
		_, wantOK := elementIDSetByParsing(junk)
		if _, gotOK := elementIDSetOf(junk); gotOK != wantOK {
			t.Errorf("%d bytes that are not a document: ok %v, parsing gives ok %v", len(junk), gotOK, wantOK)
		}
	}
}
