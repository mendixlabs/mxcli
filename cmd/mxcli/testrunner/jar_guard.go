// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// jarGuard keeps the running runtime's project.jar readable across an
// --attach build (ako/mxcli#2640, upstream mendixlabs/mxcli#1973).
//
// The runtime opens deployment/model/bundles/project.jar once and reads each
// class's bytes at the offsets its central directory gave it. The build an
// attach drives rewrites the file in place — same size, new bytes — so the
// next class the runtime had not loaded yet failed with NoClassDefFoundError,
// caused by "ZipFile invalid LOC header (bad signature)". The test microflows
// an attach injects add no Java, so the rebuilt jar holds the same classes:
// putting the original bytes back leaves the runtime reading what it opened.
type jarGuard struct {
	path string
	data []byte
}

// projectJarPath is where a `run --local` deployment keeps the compiled Java.
func projectJarPath(projectPath string) string {
	return filepath.Join(filepath.Dir(projectPath), "deployment", "model", "bundles", "project.jar")
}

// guardProjectJar reads the jar as it is now; nil when there is none.
func guardProjectJar(projectPath string) *jarGuard {
	path := projectJarPath(projectPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return &jarGuard{path: path, data: data}
}

// settle puts the guarded bytes back when a build rewrote the jar, and
// reports whether the classes in it changed. A jar whose entries and their
// checksums are the ones it had is the same Java, so the original bytes go
// back and the runtime keeps reading the file it opened; a jar with other
// classes is Java the running runtime cannot load without a restart, so the
// original bytes go back too and changed is true.
func (g *jarGuard) settle() (changed bool, err error) {
	if g == nil {
		return false, nil
	}
	now, err := os.ReadFile(g.path)
	if err != nil || bytes.Equal(now, g.data) {
		return false, nil
	}
	same, err := sameJarEntries(g.data, now)
	if err != nil {
		return false, fmt.Errorf("comparing %s with the one the app runs: %w", filepath.Base(g.path), err)
	}
	if err := os.WriteFile(g.path, g.data, 0o644); err != nil {
		return !same, fmt.Errorf("restoring %s for the running app: %w", filepath.Base(g.path), err)
	}
	return !same, nil
}

// sameJarEntries reports whether two jars hold the same entries with the same
// contents (by CRC-32 and size), whatever order or timestamps they were written
// with.
func sameJarEntries(a, b []byte) (bool, error) {
	ea, err := jarEntries(a)
	if err != nil {
		return false, err
	}
	eb, err := jarEntries(b)
	if err != nil {
		return false, err
	}
	if len(ea) != len(eb) {
		return false, nil
	}
	for name, sum := range ea {
		if eb[name] != sum {
			return false, nil
		}
	}
	return true, nil
}

type jarEntrySum struct {
	crc  uint32
	size uint64
}

func jarEntries(data []byte) (map[string]jarEntrySum, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	out := make(map[string]jarEntrySum, len(r.File))
	for _, f := range r.File {
		out[f.Name] = jarEntrySum{f.CRC32, f.UncompressedSize64}
	}
	return out, nil
}
