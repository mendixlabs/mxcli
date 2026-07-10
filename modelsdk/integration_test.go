package modelsdk_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/domainmodels"
	"go.mongodb.org/mongo-driver/v2/bson"

	_ "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/domainmodels"
)

// findTestMPR searches well-known locations for a test .mpr file.
func findTestMPR(t *testing.T) string {
	t.Helper()

	patterns := []string{
		"mdl-examples/doctype-tests/app.mpr",
		"reference/test-projects/*/app.mpr",
		"mx-test-projects/*/app.mpr",
		"mx-test-projects/*/*.mpr",
		"testdata/corpus-a/app.mpr",
		"testdata/*/app.mpr",
	}
	// All patterns are relative to the repo root. The test binary runs in the
	// package directory (modelsdk/), so we go one level up.
	root := filepath.Join("..", "")
	for _, p := range patterns {
		matches, _ := filepath.Glob(filepath.Join(root, p))
		if len(matches) > 0 {
			return matches[0]
		}
	}
	return ""
}

// extractTypeName pulls the $Type string from raw BSON without the full Decoder.
func extractTypeName(raw bson.Raw) string {
	val, err := raw.LookupErr("$Type")
	if err != nil {
		return ""
	}
	s, _ := val.StringValueOK()
	return s
}

func TestRoundtrip(t *testing.T) {
	mprPath := findTestMPR(t)
	if mprPath == "" {
		t.Skip("no test MPR found — skipping integration test")
	}
	t.Logf("using MPR: %s", mprPath)

	store, err := codec.Open(mprPath)
	if err != nil {
		t.Fatalf("codec.Open: %v", err)
	}
	defer store.Close()

	units := store.ListUnits()
	if len(units) == 0 {
		t.Fatal("ListUnits returned 0 units")
	}
	t.Logf("total units: %d", len(units))

	// ── Find and decode a DomainModel unit ──────────────────────────────
	var dmRaw bson.Raw
	var dmID string

	// Pick the largest DomainModel unit — it's most likely to have entities.
	var bestSize int
	for _, u := range units {
		raw, err := store.LoadUnit(u.ID)
		if err != nil {
			continue
		}
		if extractTypeName(raw) == "DomainModels$DomainModel" {
			if len(raw) > bestSize {
				dmRaw = raw
				dmID = string(u.ID)
				bestSize = len(raw)
			}
		}
	}
	if dmRaw == nil {
		t.Skip("no DomainModels$DomainModel unit found in MPR — skipping")
	}
	t.Logf("found DomainModel unit: %s (%d bytes)", dmID, len(dmRaw))

	t.Run("DecodeDomainModel", func(t *testing.T) {
		dec := codec.NewDecoder(codec.DefaultRegistry)
		elem, err := dec.Decode(dmRaw)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if elem.TypeName() != "DomainModels$DomainModel" {
			t.Errorf("TypeName = %q, want %q", elem.TypeName(), "DomainModels$DomainModel")
		}
		if elem.ID() == "" {
			t.Error("ID() is empty")
		}
		t.Logf("decoded element: type=%s id=%s", elem.TypeName(), elem.ID())

		// Verify it decoded as the concrete DomainModel type, not a bare Base.
		dm, ok := elem.(*domainmodels.DomainModel)
		if !ok {
			t.Fatalf("decoded element is %T, want *domainmodels.DomainModel", elem)
		}

		// Properties slice should be populated by the factory.
		if len(dm.Properties()) == 0 {
			t.Error("Properties() is empty — factory did not wire properties")
		}
		t.Logf("  properties: %d", len(dm.Properties()))
	})

	t.Run("EncodeRoundtrip", func(t *testing.T) {
		dec := codec.NewDecoder(codec.DefaultRegistry)
		elem, err := dec.Decode(dmRaw)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}

		enc := &codec.Encoder{}
		encoded, err := enc.Encode(elem)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}

		if len(encoded) != len(dmRaw) {
			t.Errorf("encoded length %d != original length %d", len(encoded), len(dmRaw))
		}
		if !bytes.Equal(encoded, []byte(dmRaw)) {
			t.Error("encoded bytes differ from original — passthrough failed")
		}
		t.Logf("roundtrip OK: %d bytes", len(encoded))
	})

	t.Run("DecodeEntities", func(t *testing.T) {
		// Look for an Entities array inside the raw DomainModel BSON.
		entitiesVal, err := dmRaw.LookupErr("Entities")
		if err != nil {
			t.Skip("no Entities array in DomainModel BSON — skipping entity decode test")
		}

		arr, ok := entitiesVal.ArrayOK()
		if !ok {
			t.Skip("Entities field is not an array — skipping")
		}

		elems, err := arr.Values()
		if err != nil || len(elems) == 0 {
			t.Skip("Entities array is empty — skipping")
		}

		t.Logf("found %d entity elements in DomainModel", len(elems))

		dec := codec.NewDecoder(codec.DefaultRegistry)
		decoded := 0
		for i, el := range elems {
			doc, ok := el.DocumentOK()
			if !ok {
				continue // skip non-document elements
			}
			typeName := extractTypeName(bson.Raw(doc))
			if typeName == "" {
				continue
			}

			entity, err := dec.Decode(bson.Raw(doc))
			if err != nil {
				t.Errorf("entity[%d] Decode error: %v", i, err)
				continue
			}

			if !strings.HasPrefix(entity.TypeName(), "DomainModels$") {
				t.Errorf("entity[%d] unexpected type: %s", i, entity.TypeName())
				continue
			}

			// If it's an Entity, try reading the Name property.
			if ent, ok := entity.(*domainmodels.Entity); ok {
				name := ent.Name()
				t.Logf("  entity[%d]: %s (name=%q)", i, entity.TypeName(), name)
			} else {
				t.Logf("  entity[%d]: %s (not *Entity, type=%T)", i, entity.TypeName(), entity)
			}
			decoded++
		}

		if decoded == 0 {
			t.Error("no entities were decoded successfully")
		}
		t.Logf("successfully decoded %d/%d entities", decoded, len(elems))
	})
}

func TestModelAPI(t *testing.T) {
	mprPath := findTestMPR(t)
	if mprPath == "" {
		t.Skip("no test MPR found")
	}

	m, err := modelsdk.Open(mprPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer m.Close()

	t.Run("Units", func(t *testing.T) {
		units := m.Units()
		if len(units) == 0 {
			t.Fatal("no units")
		}
		t.Logf("total units: %d", len(units))
	})

	t.Run("AllOfType", func(t *testing.T) {
		dms := m.AllOfType("DomainModels$DomainModel")
		if len(dms) == 0 {
			t.Skip("no DomainModel units")
		}
		t.Logf("found %d DomainModel units", len(dms))

		for _, dm := range dms {
			if dm.TypeName() != "DomainModels$DomainModel" {
				t.Errorf("unexpected type: %s", dm.TypeName())
			}
			if _, ok := dm.(*domainmodels.DomainModel); !ok {
				t.Errorf("expected *domainmodels.DomainModel, got %T", dm)
			}
		}
	})

	t.Run("LoadUnit", func(t *testing.T) {
		units := m.Units()
		if len(units) == 0 {
			t.Skip("no units")
		}
		elem, err := m.LoadUnit(units[0].ID)
		if err != nil {
			t.Fatalf("LoadUnit: %v", err)
		}
		if elem.TypeName() == "" {
			t.Error("TypeName empty")
		}
		t.Logf("loaded: %s", elem.TypeName())

		// Load again — should be cached.
		elem2, _ := m.LoadUnit(units[0].ID)
		if elem != elem2 {
			t.Error("expected cached element, got different pointer")
		}
	})

	t.Run("EncodeModified", func(t *testing.T) {
		dms := m.AllOfType("DomainModels$DomainModel")
		if len(dms) == 0 {
			t.Skip("no DomainModel units")
		}
		dm := dms[0].(*domainmodels.DomainModel)
		original := dm.Documentation()

		dm.SetDocumentation("test-modified")
		dm.MarkDirty(0)

		out, err := m.Encode(dm)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}

		// Verify the encoded output has the modified value.
		var doc bson.D
		if err := bson.Unmarshal(out, &doc); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		for _, e := range doc {
			if e.Key == "documentation" {
				if e.Value != "test-modified" {
					t.Errorf("documentation = %v, want 'test-modified'", e.Value)
				}
			}
		}

		// Restore original.
		dm.SetDocumentation(original)
		t.Logf("encode of modified element OK (%d bytes)", len(out))
	})
}

func TestWriteRoundtrip(t *testing.T) {
	mprPath := findTestMPR(t)
	if mprPath == "" {
		t.Skip("no test MPR found")
	}

	// Copy MPR to temp dir so we don't modify the original.
	tmpDir := t.TempDir()
	tmpMPR := filepath.Join(tmpDir, "test.mpr")
	copyFile(t, mprPath, tmpMPR)

	// Also copy mprcontents/ if it exists (v2 format).
	srcDir := filepath.Dir(mprPath)
	srcContents := filepath.Join(srcDir, "mprcontents")
	if info, err := os.Stat(srcContents); err == nil && info.IsDir() {
		dstContents := filepath.Join(tmpDir, "mprcontents")
		copyDir(t, srcContents, dstContents)
	}

	// Open for writing, modify, flush, close.
	m, err := modelsdk.OpenForWriting(tmpMPR)
	if err != nil {
		t.Fatalf("OpenForWriting: %v", err)
	}

	dms := m.AllOfType("DomainModels$DomainModel")
	if len(dms) == 0 {
		m.Close()
		t.Skip("no DomainModel units")
	}

	dm := dms[0].(*domainmodels.DomainModel)
	dm.SetDocumentation("modelsdk-write-test")
	dm.MarkDirty(0)

	if _, err := m.Flush(); err != nil {
		m.Close()
		t.Fatalf("Flush: %v", err)
	}
	m.Close()

	// Reopen and verify the change persisted.
	m2, err := modelsdk.Open(tmpMPR)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer m2.Close()

	dms2 := m2.AllOfType("DomainModels$DomainModel")
	if len(dms2) == 0 {
		t.Fatal("no DomainModel after reopen")
	}
	dm2 := dms2[0].(*domainmodels.DomainModel)
	if dm2.Documentation() != "modelsdk-write-test" {
		t.Errorf("after reopen, Documentation = %q, want 'modelsdk-write-test'", dm2.Documentation())
	}
	t.Logf("write roundtrip OK: Documentation persisted as %q", dm2.Documentation())
}

func TestNewEntityFactory(t *testing.T) {
	entity := domainmodels.NewEntity()
	if entity == nil {
		t.Fatal("NewEntity returned nil")
	}
	entity.SetName("TestCustomer")
	if entity.Name() != "TestCustomer" {
		t.Errorf("Name = %q", entity.Name())
	}
	if entity.TypeName() == "" {
		t.Error("TypeName empty")
	}
	t.Logf("NewEntity: type=%s name=%s props=%d", entity.TypeName(), entity.Name(), len(entity.Properties()))
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		copyFile(t, path, target)
		return nil
	})
}
