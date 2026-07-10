// SPDX-License-Identifier: Apache-2.0

package mprread_test

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/mprread"
)

// fixturePath is the canonical Stage 4 read fixture: a v2 MPR with
// mprcontents/ folder, 16 Microflows$Microflow units, and at least one
// Microflows$Nanoflow.
const fixturePath = "../../testdata/expr-checker/minimal.mpr"

// openTestReader copies the canonical fixture into a per-test temp dir
// and opens it as a *mmpr.Reader.
func openTestReader(t *testing.T) *mmpr.Reader {
	t.Helper()
	dst := copyMPRTree(t, fixturePath, t.TempDir())
	r, err := mmpr.Open(dst)
	if err != nil {
		t.Fatalf("Open(%s): %v", dst, err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

// TestListMicroflows_FixtureSurface asserts the read-only helper returns
// gen-typed microflows from the canonical fixture.
func TestListMicroflows_FixtureSurface(t *testing.T) {
	r := openTestReader(t)
	mfs, err := mprread.ListMicroflows(r)
	if err != nil {
		t.Fatalf("ListMicroflows: %v", err)
	}
	if len(mfs) == 0 {
		t.Fatal("ListMicroflows: got 0 microflows, expected non-zero")
	}
	for _, mf := range mfs {
		if mf == nil {
			t.Fatal("ListMicroflows: nil entry in result")
		}
		if mf.TypeName() != "Microflows$Microflow" {
			t.Errorf("TypeName = %q, want %q", mf.TypeName(), "Microflows$Microflow")
		}
		if mf.ID() == "" {
			t.Errorf("microflow %q has empty ID", mf.Name())
		}
		// Compile-time check: every gen Microflow must satisfy the
		// element.Element interface so callers can compose with codec.
		var _ element.Element = mf
	}
}

// TestListNanoflows_FixtureSurface asserts the helper returns gen-typed
// Nanoflows. Fixture may have zero, but the function must succeed and
// produce a valid (possibly empty) slice; entries (if any) must be
// non-nil with the canonical $Type.
func TestListNanoflows_FixtureSurface(t *testing.T) {
	r := openTestReader(t)
	nfs, err := mprread.ListNanoflows(r)
	if err != nil {
		t.Fatalf("ListNanoflows: %v", err)
	}
	for _, nf := range nfs {
		if nf == nil {
			t.Fatal("ListNanoflows: nil entry in result")
		}
		if nf.TypeName() != "Microflows$Nanoflow" {
			t.Errorf("TypeName = %q, want %q", nf.TypeName(), "Microflows$Nanoflow")
		}
		if nf.ID() == "" {
			t.Errorf("nanoflow %q has empty ID", nf.Name())
		}
		var _ element.Element = nf
	}
}

func copyMPRTree(t *testing.T, srcMPR, dstDir string) string {
	t.Helper()
	dstMPR := filepath.Join(dstDir, filepath.Base(srcMPR))
	if err := copyOneFile(srcMPR, dstMPR); err != nil {
		t.Fatalf("copy %s -> %s: %v", srcMPR, dstMPR, err)
	}
	srcContents := filepath.Join(filepath.Dir(srcMPR), "mprcontents")
	if info, err := os.Stat(srcContents); err == nil && info.IsDir() {
		dstContents := filepath.Join(dstDir, "mprcontents")
		if err := copyDir(srcContents, dstContents); err != nil {
			t.Fatalf("copy contents %s -> %s: %v", srcContents, dstContents, err)
		}
	}
	return dstMPR
}

func copyOneFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyOneFile(p, target)
	})
}
