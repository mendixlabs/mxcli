// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func jarOf(t *testing.T, stamp time.Time, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range []string{"a/A.class", "b/B.class", "c/C.class"} {
		body, ok := files[name]
		if !ok {
			continue
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: stamp})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// An --attach build rewrote the running app's project.jar in place, and the
// runtime then failed to load any class it had not loaded yet: "ZipFile
// invalid LOC header" (ako/mxcli#2640). The same classes rebuilt get the
// original bytes back; other classes are reported as a change the running
// runtime cannot take.
func TestJarGuard_2640(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	jar := projectJarPath(mpr)
	if err := os.MkdirAll(filepath.Dir(jar), 0o755); err != nil {
		t.Fatal(err)
	}
	classes := map[string]string{"a/A.class": "AAAA", "b/B.class": "BBBB"}
	orig := jarOf(t, time.Date(2026, 11, 27, 14, 35, 6, 0, time.UTC), classes)
	if err := os.WriteFile(jar, orig, 0o644); err != nil {
		t.Fatal(err)
	}

	g := guardProjectJar(mpr)
	// The build: the same classes, written again with a new timestamp.
	rebuilt := jarOf(t, time.Date(2026, 11, 27, 14, 35, 51, 0, time.UTC), classes)
	if bytes.Equal(rebuilt, orig) {
		t.Fatal("fixture: the rebuilt jar must differ in bytes")
	}
	_ = os.WriteFile(jar, rebuilt, 0o644)
	changed, err := g.settle()
	if err != nil || changed {
		t.Fatalf("same classes: changed=%v err=%v, want unchanged", changed, err)
	}
	if now, _ := os.ReadFile(jar); !bytes.Equal(now, orig) {
		t.Error("the jar the runtime opened was not put back")
	}

	// Control: a build that changes a class is reported, and the running
	// app's jar is still put back.
	g = guardProjectJar(mpr)
	_ = os.WriteFile(jar, jarOf(t, time.Now(), map[string]string{"a/A.class": "AAAA", "b/B.class": "CHANGED"}), 0o644)
	if changed, err := g.settle(); err != nil || !changed {
		t.Errorf("changed classes: changed=%v err=%v, want changed", changed, err)
	}
	if now, _ := os.ReadFile(jar); !bytes.Equal(now, orig) {
		t.Error("the running app's jar was not put back after a Java change")
	}

	// No jar: nothing to guard.
	if g := guardProjectJar(filepath.Join(t.TempDir(), "X.mpr")); g != nil {
		t.Error("guarded a jar that does not exist")
	}
}
