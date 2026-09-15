// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"

	modelsdk "github.com/mendixlabs/mxcli"
	"go.mongodb.org/mongo-driver/bson"
)

// packageProjectEntry is the full model of the module being published, carried
// inside every .mpk as a v1 .mpr.
const packageProjectEntry = "project.mpr"

// makeImportable returns a path to a .mpk that `mx module-import` will accept.
//
// mx refuses a theme module outright — "Importing theme module is not
// supported", exit 112 — which takes Atlas_Core, Atlas_Web_Content and
// Conversational UI off the table for every headless path. The refusal is
// gated on a single BSON boolean on the module document inside the package
// (`Projects$ModuleImpl → IsThemeModule`); flipping it and re-importing the
// otherwise identical package, theme files and all, imports and checks cleanly
// (mxcli-formula1 FINDINGS §53, reproduced here).
//
// So when the package declares a theme module, this writes a copy with the flag
// cleared and returns that path; otherwise it returns mpkPath unchanged.
//
// This is only sound because of where it is used. The copy is imported into a
// throwaway reference project that exists to be described once and deleted, so
// the flag's real meaning — whether Studio Pro treats the module as a theme —
// never matters. It would NOT be sound in an install path writing to a user's
// project, which is why this lives beside the reference builder rather than in
// something reusable.
func makeImportable(mpkPath, workDir string) (string, error) {
	// The package's project.mpr MUST be unpacked into a directory of its own.
	//
	// An .mpr is read as MPR v2 when an `mprcontents/` directory sits beside it —
	// that adjacency is the whole format test. workDir already holds the scratch
	// project mx just created, mprcontents/ included, so extracting the package's
	// v1 .mpr straight into it makes the reader treat it as v2 and look for unit
	// contents in the *scratch project's* files. The flag flip then silently
	// writes nothing the package will carry, and the import is refused exactly as
	// if nothing had been done — which is how this was found.
	unpackDir, err := os.MkdirTemp(workDir, "pkg-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(unpackDir)

	mprPath := filepath.Join(unpackDir, packageProjectEntry)
	if err := extractZipEntry(mpkPath, packageProjectEntry, mprPath); err != nil {
		// A package with no project.mpr is not a module package; let mx report it.
		return mpkPath, nil //nolint:nilerr // not our error to diagnose
	}

	cleared, err := clearThemeModuleFlag(mprPath)
	if err != nil {
		return "", err
	}
	if !cleared {
		return mpkPath, nil
	}

	out := filepath.Join(workDir, "importable.mpk")
	if err := replaceZipEntry(mpkPath, packageProjectEntry, mprPath, out); err != nil {
		return "", err
	}
	return out, nil
}

// clearThemeModuleFlag sets IsThemeModule to false on the package's module
// document, reporting whether it had to change anything.
//
// The edit is done on the decoded document and written back whole, because the
// flag is a top-level property of the unit and mxcli's writer is the only thing
// that knows how to store a unit in either MPR format.
func clearThemeModuleFlag(mprPath string) (bool, error) {
	reader, err := modelsdk.Open(mprPath)
	if err != nil {
		return false, fmt.Errorf("open the package's project model: %w", err)
	}
	units, err := reader.ListRawUnitsByType("Projects$ModuleImpl")
	if err != nil {
		_ = reader.Disconnect()
		return false, fmt.Errorf("read the package's module document: %w", err)
	}

	type edit struct {
		id       string
		contents []byte
	}
	var edits []edit
	for _, u := range units {
		// bson.D preserves key order, so the rewritten document differs from the
		// original in exactly one value rather than in its whole layout.
		var doc bson.D
		if err := bson.Unmarshal(u.Contents, &doc); err != nil {
			continue
		}
		changed := false
		for i, e := range doc {
			if e.Key != "IsThemeModule" {
				continue
			}
			if isTheme, ok := e.Value.(bool); ok && isTheme {
				doc[i].Value = false
				changed = true
			}
			break
		}
		if !changed {
			continue
		}
		encoded, err := bson.Marshal(doc)
		if err != nil {
			_ = reader.Disconnect()
			return false, fmt.Errorf("re-encode the module document: %w", err)
		}
		edits = append(edits, edit{id: string(u.ID), contents: encoded})
	}
	_ = reader.Disconnect()

	if len(edits) == 0 {
		return false, nil
	}

	writer, err := modelsdk.OpenForWriting(mprPath)
	if err != nil {
		return false, fmt.Errorf("open the package's project model for writing: %w", err)
	}
	defer writer.Disconnect()
	for _, e := range edits {
		if err := writer.UpdateRawUnit(e.id, e.contents); err != nil {
			return false, fmt.Errorf("clear the theme-module flag: %w", err)
		}
	}
	return true, nil
}

// extractZipEntry writes one entry of a zip to dst.
func extractZipEntry(zipPath, entry, dst string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.Name != entry {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		out, err := os.Create(dst)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, rc)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	}
	return fmt.Errorf("no %s in %s", entry, filepath.Base(zipPath))
}

// replaceZipEntry copies a zip to dst, substituting one entry's content. Every
// other entry is carried across byte-for-byte, so the package mx sees differs
// from the published one only in the module document.
func replaceZipEntry(srcZip, entry, replacement, dst string) error {
	zr, err := zip.OpenReader(srcZip)
	if err != nil {
		return err
	}
	defer zr.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	for _, f := range zr.File {
		header := f.FileHeader
		w, err := zw.CreateHeader(&header)
		if err != nil {
			return err
		}
		var src io.ReadCloser
		if f.Name == entry {
			src, err = os.Open(replacement)
		} else {
			src, err = f.Open()
		}
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(w, src)
		_ = src.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return zw.Close()
}
