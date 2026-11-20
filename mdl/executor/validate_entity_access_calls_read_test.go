// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// pedAppFlowUnits is the stored bytes of every microflow and rule of the
// PedApp fixture.
func pedAppFlowUnits(t *testing.T) [][]byte {
	t.Helper()
	root := filepath.Join("..", "..", "testdata", "pedapp", "mprcontents")
	var out [][]byte
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".mxunit" {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		switch typ, _ := bson.Raw(raw).Lookup("$Type").StringValueOK(); typ {
		case "Microflows$Microflow", "Microflows$Rule":
			out = append(out, raw)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read the PedApp units: %v", err)
	}
	if len(out) < 10 {
		t.Fatalf("found %d microflows and rules in the fixture, expected its whole set", len(out))
	}
	return out
}

// storedEAFlow reads the name and flags where they lie and decodes a document
// only when its bytes name a call type. It must read what decoding every
// document read: compared on every microflow and rule of the fixture, with
// and without the calls, and on bytes that are not a document.
func TestStoredEAFlow_ReadsWhatDecodingTheDocumentRead(t *testing.T) {
	units := pedAppFlowUnits(t)
	withCalls, without := 0, 0
	before := eaDocumentsDecoded.Load()
	for i, raw := range units {
		for _, calls := range []bool{true, false} {
			want, wantOK := storedEAFlowDecoded(raw, calls)
			got, gotOK := storedEAFlow(raw, calls)
			if gotOK != wantOK || !reflect.DeepEqual(got, want) {
				t.Fatalf("unit %d (calls %v): read %+v (ok %v), decoding the document reads %+v (ok %v)", i, calls, got, gotOK, want, wantOK)
			}
			if want.name == "" {
				t.Fatalf("control: unit %d has no name, so the comparison is of empty values", i)
			}
			if calls {
				if len(want.calls) > 0 {
					withCalls++
				} else {
					without++
				}
			}
		}
	}
	decoded := eaDocumentsDecoded.Load() - before
	t.Logf("%d documents: %d with calls, %d without; %d decoded in full", len(units), withCalls, without, decoded)
	// Controls: both kinds must be in the fixture, and the ones without a
	// call must not have been decoded.
	if withCalls == 0 || without == 0 {
		t.Fatalf("the fixture does not hold both kinds: %d with calls, %d without", withCalls, without)
	}
	if decoded > int64(withCalls)+int64(len(units))/10 {
		t.Errorf("%d documents decoded in full for %d that hold a call", decoded, withCalls)
	}
	for _, junk := range [][]byte{nil, {}, []byte("not a document"), units[0][:len(units[0])/2]} {
		_, wantOK := storedEAFlowDecoded(junk, true)
		if _, gotOK := storedEAFlow(junk, true); gotOK != wantOK {
			t.Errorf("%d bytes that are not a document: ok %v, decoding gives ok %v", len(junk), gotOK, wantOK)
		}
	}
}
