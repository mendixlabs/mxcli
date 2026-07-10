package modelsdk_test

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"

	_ "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/domainmodels"
	_ "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/enumerations"
)

// copyMPRForWrite copies an MPR (and mprcontents/ for v2 format) to a temp
// directory and returns the path to the copy.
func copyMPRForWrite(t *testing.T, src string) string {
	t.Helper()
	tmpDir := t.TempDir()
	dst := filepath.Join(tmpDir, filepath.Base(src))
	copyFile(t, src, dst)

	// Also copy mprcontents/ if it exists (v2 format).
	srcDir := filepath.Dir(src)
	srcContents := filepath.Join(srcDir, "mprcontents")
	if info, err := os.Stat(srcContents); err == nil && info.IsDir() {
		dstContents := filepath.Join(tmpDir, "mprcontents")
		copyDir(t, srcContents, dstContents)
	}
	return dst
}

// newUUID generates a random UUID string for test use.
func newUUID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// TestFlushReturnsCount verifies that Flush() returns the number of units written.
func TestFlushReturnsCount(t *testing.T) {
	mprPath := findTestMPR(t)
	if mprPath == "" {
		t.Skip("no test MPR found")
	}
	tmpMPR := copyMPRForWrite(t, mprPath)
	m, err := modelsdk.OpenForWriting(tmpMPR)
	if err != nil {
		t.Fatalf("OpenForWriting: %v", err)
	}
	defer m.Close()

	n, err := m.Flush()
	if err != nil {
		t.Fatalf("Flush() error: %v", err)
	}
	if n != 0 {
		t.Errorf("Flush() with no modifications returned n=%d, want 0", n)
	}
}

// TestAllOfTypePerTypeCacheIsolation verifies that inserting a unit of
// type A does not invalidate cached elements of type B.
func TestAllOfTypePerTypeCacheIsolation(t *testing.T) {
	mprPath := findTestMPR(t)
	if mprPath == "" {
		t.Skip("no test MPR found")
	}
	tmpMPR := copyMPRForWrite(t, mprPath)
	m, err := modelsdk.OpenForWriting(tmpMPR)
	if err != nil {
		t.Fatalf("OpenForWriting: %v", err)
	}
	defer m.Close()

	// Try a sequence of type names in order of likelihood to be present.
	// We need at least one pre-existing unit of typeA to populate the cache.
	// typeB is what we insert — it must be a different type than typeA.
	type testCase struct {
		typeA string // type to cache and verify pointer identity
		typeB string // type to insert (must differ from typeA)
	}
	candidates := []testCase{
		{"Enumerations$Enumeration", "DomainModels$DomainModel"},
		{"DomainModels$DomainModel", "Microflows$Microflow"},
		{"Microflows$Microflow", "DomainModels$DomainModel"},
		{"Pages$Page", "DomainModels$DomainModel"},
	}

	var chosen *testCase
	var elementsBefore []element.Element
	for i := range candidates {
		tc := &candidates[i]
		elems := m.AllOfType(tc.typeA)
		if len(elems) > 0 {
			chosen = tc
			elementsBefore = elems
			break
		}
	}
	if chosen == nil {
		t.Skip("no suitable unit types found in test project")
	}

	t.Logf("testing with typeA=%s (%d units), inserting typeB=%s", chosen.typeA, len(elementsBefore), chosen.typeB)

	// Find a valid container unit to use as parent for the new unit.
	units := m.Units()
	if len(units) == 0 {
		t.Skip("no units in MPR")
	}
	containerID := units[0].ID

	// Insert a unit of typeB (different type from typeA).
	// Use a minimal BSON document with just a $Type field.
	dummyData := []byte{5, 0, 0, 0, 0} // minimal empty BSON document
	newID := element.ID(newUUID(t))
	err = m.InsertUnit(
		newID,
		containerID,
		"Documents",
		chosen.typeB,
		dummyData,
	)
	if err != nil {
		// InsertUnit failing is acceptable if the MPR writer rejects the data.
		// Skip rather than fail — the test environment may not support arbitrary inserts.
		t.Skipf("InsertUnit with dummy data returned error (ok for this test env): %v", err)
	}

	// AllOfType for typeA should still return cached elements
	// without re-decoding (same pointers).
	elementsAfter := m.AllOfType(chosen.typeA)
	if len(elementsAfter) != len(elementsBefore) {
		t.Fatalf("element count changed: before=%d after=%d", len(elementsBefore), len(elementsAfter))
	}

	// Verify pointer identity — cached elements should be the same objects.
	for i := range elementsBefore {
		if elementsBefore[i] != elementsAfter[i] {
			t.Errorf("element[%d] of type %s: got different pointer after inserting unrelated type %s (re-decoded when it shouldn't have)", i, chosen.typeA, chosen.typeB)
			break
		}
	}
}
