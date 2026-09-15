// SPDX-License-Identifier: Apache-2.0

// `mx rename-design-properties` fixes something only Mendix can fix (CE6087) and
// rewrites an MPR v2 project as v1 while doing it — measured on 11.12.1, 1,865
// .mxunit files to 0. The snapshot-and-restore that protects `update-widgets`
// (#808) cannot be reused, because that one is allowed to throw the tool's output
// away once the check has run, while these renames have to persist.
//
// So RunToolPreservingFormat harvests: it lets the tool convert the project,
// reads every unit back out, restores the v2 storage, and writes the changed
// units into it. These tests pin the three things that makes load-bearing — the
// tool's change survives, the format survives, and a failure anywhere leaves the
// project as it was rather than as a half-converted v1 file.
package docker

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"go.mongodb.org/mongo-driver/bson"
)

// stubMxTool swaps the mx invocation for the duration of a test.
func stubMxTool(t *testing.T, fn func(mxPath string, args []string, w, stderr io.Writer) error) {
	t.Helper()
	prev := mxToolCmd
	mxToolCmd = fn
	t.Cleanup(func() { mxToolCmd = prev })
}

// firstUnitID returns some unit of the project, for a test to mutate.
func firstUnitID(t *testing.T, mprPath string) string {
	t.Helper()
	reader, err := openReadOnly(mprPath)
	if err != nil {
		t.Fatalf("open %s: %v", mprPath, err)
	}
	defer reader.Disconnect()
	units, err := reader.ListUnits()
	if err != nil || len(units) == 0 {
		t.Fatalf("fixture has no units (err=%v)", err)
	}
	return string(units[0].ID)
}

// markUnit is what the tool "doing its job" looks like: a content change to one
// unit. A canonical comparison must see it — it is a property, not an $ID.
func markUnit(t *testing.T, mprPath, unitID, marker string) {
	t.Helper()
	reader, err := openReadOnly(mprPath)
	if err != nil {
		t.Fatalf("open for marking: %v", err)
	}
	raw, err := reader.GetRawUnitBytes(model.ID(unitID))
	_ = reader.Disconnect()
	if err != nil {
		t.Fatalf("read unit %s: %v", unitID, err)
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode unit %s: %v", unitID, err)
	}
	doc = append(doc, bson.E{Key: "HarvestTestMarker", Value: marker})
	encoded, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("encode unit %s: %v", unitID, err)
	}
	writer, err := openForWriting(mprPath)
	if err != nil {
		t.Fatalf("open for writing: %v", err)
	}
	defer writer.Disconnect()
	if err := writer.UpdateRawUnit(unitID, encoded); err != nil {
		t.Fatalf("write unit %s: %v", unitID, err)
	}
}

// unitMarker reads back what markUnit wrote, or "" when absent.
func unitMarker(t *testing.T, mprPath, unitID string) string {
	t.Helper()
	reader, err := openReadOnly(mprPath)
	if err != nil {
		t.Fatalf("open for reading marker: %v", err)
	}
	defer reader.Disconnect()
	raw, err := reader.GetRawUnitBytes(model.ID(unitID))
	if err != nil {
		t.Fatalf("read unit %s: %v", unitID, err)
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode unit %s: %v", unitID, err)
	}
	for _, e := range doc {
		if e.Key == "HarvestTestMarker" {
			s, _ := e.Value.(string)
			return s
		}
	}
	return ""
}

// The core property: the tool's change lands in the project even though the v2
// storage is restored from a snapshot taken before the tool ran. Get this wrong
// and the command reports success while silently discarding the fix — which is
// exactly what would happen if runUpdateWidgets' snapshot/restore were reused
// here unchanged.
func TestRunToolPreservingFormat_CarriesTheToolsChangeBack(t *testing.T) {
	mprPath := v2Fixture(t)
	unitID := firstUnitID(t, mprPath)

	stubMxTool(t, func(_ string, _ []string, _, _ io.Writer) error {
		markUnit(t, mprPath, unitID, "renamed")
		return nil
	})

	res, err := RunToolPreservingFormat("mx", mprPath, "rename-design-properties", io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("RunToolPreservingFormat: %v", err)
	}
	if got := unitMarker(t, mprPath, unitID); got != "renamed" {
		t.Errorf("the tool's change did not survive the restore: marker=%q, want %q", got, "renamed")
	}
	if res.UnitsWritten != 1 {
		t.Errorf("UnitsWritten = %d, want 1 — only the unit the tool touched should be rewritten", res.UnitsWritten)
	}
	if !res.Harvested || !res.StillV2 {
		t.Errorf("Harvested=%v StillV2=%v, want both true", res.Harvested, res.StillV2)
	}
	if v := storageVersion(t, mprPath); v != types.MPRVersionV2 {
		t.Errorf("project is %v afterwards, want MPRv2 — the format is the whole point", v)
	}
}

// The negative half. A mechanism that rewrites every unit would also pass the
// test above; ADR-0008 requires that a unit whose content did not change is not
// written, or a fix run against an in-sync project churns the whole model.
func TestRunToolPreservingFormat_WritesNothingWhenTheToolChangedNothing(t *testing.T) {
	mprPath := v2Fixture(t)

	stubMxTool(t, func(_ string, _ []string, _, _ io.Writer) error { return nil })

	res, err := RunToolPreservingFormat("mx", mprPath, "update-widgets", io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("RunToolPreservingFormat: %v", err)
	}
	if res.UnitsWritten != 0 {
		t.Errorf("UnitsWritten = %d, want 0 — a no-op tool run must not rewrite the model", res.UnitsWritten)
	}
	if res.StorageFiles == 0 || res.StorageFiles != res.StorageFilesBefore {
		t.Errorf("storage went %d -> %d; a v2 -> v1 collapse shows up exactly here",
			res.StorageFilesBefore, res.StorageFiles)
	}
}

// The control ADR-0008 requires for any "nothing changed" assertion: with
// elision off, the same run writes everything. Without this, the test above also
// passes a harvest that read no units at all and had nothing to offer.
func TestRunToolPreservingFormat_AlwaysWriteProvesUnitsWereRead(t *testing.T) {
	mprPath := v2Fixture(t)
	t.Setenv("MXCLI_ALWAYS_WRITE", "1")

	stubMxTool(t, func(_ string, _ []string, _, _ io.Writer) error { return nil })

	res, err := RunToolPreservingFormat("mx", mprPath, "update-widgets", io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("RunToolPreservingFormat: %v", err)
	}
	if res.UnitsWritten == 0 {
		t.Fatal("MXCLI_ALWAYS_WRITE wrote 0 units — the harvest is reading nothing, " +
			"so the 0 in the elision test means 'no data', not 'no change'")
	}
	if res.UnitsWritten != res.StorageFiles {
		t.Errorf("wrote %d of %d units under MXCLI_ALWAYS_WRITE; every unit should be offered",
			res.UnitsWritten, res.StorageFiles)
	}
}

// A tool that fails after converting the project must not leave it converted.
// This is the difference between a failed command and a broken repository.
func TestRunToolPreservingFormat_RestoresFormatWhenTheToolFails(t *testing.T) {
	mprPath := v2Fixture(t)
	contentsDir := filepath.Join(filepath.Dir(mprPath), "mprcontents")

	stubMxTool(t, func(_ string, _ []string, _, _ io.Writer) error {
		convertToV1(t, mprPath)
		return errors.New("mx exploded")
	})

	if _, err := RunToolPreservingFormat("mx", mprPath, "rename-design-properties", io.Discard, io.Discard); err == nil {
		t.Fatal("expected the tool's failure to surface")
	}
	if _, err := os.Stat(contentsDir); err != nil {
		t.Fatalf("mprcontents/ was not restored after a failed tool run: %v", err)
	}
	if v := storageVersion(t, mprPath); v != types.MPRVersionV2 {
		t.Errorf("project left as %v after a failed run, want MPRv2", v)
	}
}

// Same, one step later: the tool succeeds but leaves something the reader cannot
// open. The harvest has to give up *and* put the format back.
func TestRunToolPreservingFormat_RestoresFormatWhenTheOutputIsUnreadable(t *testing.T) {
	mprPath := v2Fixture(t)

	stubMxTool(t, func(_ string, _ []string, _, _ io.Writer) error {
		convertToV1(t, mprPath) // writes a .mpr that is not a database at all
		return nil
	})

	if _, err := RunToolPreservingFormat("mx", mprPath, "rename-design-properties", io.Discard, io.Discard); err == nil {
		t.Fatal("expected unreadable tool output to be an error, not a silent no-op")
	}
	if v := storageVersion(t, mprPath); v != types.MPRVersionV2 {
		t.Errorf("project left as %v after an unreadable harvest, want MPRv2", v)
	}
}

// An MPR v1 project needs no protection: these tools write v1 natively. Snapshot
// and harvest would be pure cost, and the result must say so rather than claim a
// format was preserved that was never at risk.
func TestRunToolPreservingFormat_PassesV1StraightThrough(t *testing.T) {
	mprPath := v1Fixture(t)

	ran := false
	stubMxTool(t, func(_ string, args []string, _, _ io.Writer) error {
		ran = true
		if len(args) == 0 || args[0] != "update-widgets" {
			t.Errorf("args = %v, want the subcommand first", args)
		}
		if !filepath.IsAbs(args[len(args)-1]) {
			t.Errorf("project path %q is not absolute; MxToolset skips the step on a bare filename", args[len(args)-1])
		}
		return nil
	})

	res, err := RunToolPreservingFormat("mx", mprPath, "update-widgets", io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("RunToolPreservingFormat: %v", err)
	}
	if !ran {
		t.Fatal("the tool was not run on an MPRv1 project")
	}
	if res.Harvested {
		t.Error("Harvested = true on an MPRv1 project; nothing needed harvesting")
	}
}

// The reported storage count must match what the rest of mxcli calls a unit.
// mprcontents/ also holds an `mprname` metadata file; counting it makes every
// report one too high, which reads as a dropped unit.
func TestUnitCount_CountsOnlyMxunitFiles(t *testing.T) {
	mprPath := v2Fixture(t)
	contentsDir := filepath.Join(filepath.Dir(mprPath), "mprcontents")

	reader, err := openReadOnly(mprPath)
	if err != nil {
		t.Fatal(err)
	}
	units, err := reader.ListUnits()
	_ = reader.Disconnect()
	if err != nil {
		t.Fatal(err)
	}

	if got := unitCount(contentsDir); got != len(units) {
		var all bytes.Buffer
		_ = filepath.WalkDir(contentsDir, func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && filepath.Ext(p) != ".mxunit" {
				all.WriteString(" " + filepath.Base(p))
			}
			return nil
		})
		t.Errorf("unitCount = %d but the project has %d units; non-.mxunit files present:%s",
			got, len(units), all.String())
	}
}
