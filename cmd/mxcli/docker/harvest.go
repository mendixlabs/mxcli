// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/modelsdk/canon"
)

// HarvestResult reports what a model-fixing mx tool changed.
type HarvestResult struct {
	// Harvested is true when the tool's output had to be carried back into an
	// MPR v2 project; false when the project was already v1 and the tool's write
	// landed directly.
	Harvested bool
	// UnitsWritten counts units whose content actually changed. Units the tool
	// rewrote without changing anything (a fresh $ID per sub-element, which every
	// rebuild mints) are elided by canon.Reconcile and not counted — ADR-0008.
	UnitsWritten int
	// Added and Removed name units the tool created or deleted, as
	// "<type> <id>". These are reported rather than applied: the tools this runs
	// rewrite documents in place, so a non-empty list means the tool did
	// something this mechanism was not built for and the caller should say so.
	Added   []string
	Removed []string
	// StorageFiles is the number of .mxunit files on disk after the run, and
	// StorageFilesBefore what it was before. They are the assertion this whole
	// mechanism exists for: a v2 -> v1 conversion takes the count to zero, so a
	// caller that prints both cannot report success on a collapsed project.
	StorageFilesBefore int
	StorageFiles       int
	// StillV2 is false if the project is not in the v2 layout afterwards. It
	// should never be false when Harvested is true; the commands check it rather
	// than assume it.
	StillV2 bool
}

// mxToolCmd runs the mx invocation. A package variable so tests can substitute a
// stub that simulates the v2 -> v1 conversion without needing mx on the box.
var mxToolCmd = func(mxPath string, args []string, w, stderr io.Writer) error {
	cmd := exec.Command(mxPath, args...)
	cmd.Stdout = w
	cmd.Stderr = stderr
	PrepareMxCommand(cmd)
	return cmd.Run()
}

// RunToolPreservingFormat runs an mx model-fixing tool (`rename-design-properties`,
// `update-widgets`, ...) and lands its result in the project **without** leaving
// the project converted to MPR v1.
//
// Three of Mendix's own commands rewrite an MPR v2 project as v1 as a side
// effect of doing their job — `module-import`, `update-widgets` and
// `rename-design-properties`. Measured on 11.12.1, `rename-design-properties`
// turned 1,865 `.mxunit` files into 0 and a 249,856-byte index into 39,895,040
// bytes, while renaming 149 design properties across 41 documents. The renames
// are real work that only Mendix can do; the conversion is collateral.
//
// `runUpdateWidgets` solves its half of this by snapshotting the v2 storage and
// restoring it afterwards, which is correct **there** because the widget resync
// only has to hold for the duration of the check that follows. That trick does
// not transfer to a fix whose result must persist: restoring the snapshot would
// restore the un-renamed model too.
//
// So this harvests instead. The tool is allowed to convert the project, every
// unit is read back out of the converted file, the v2 storage is restored, and
// the changed units are written into it through mxcli's own writer — which is
// also what keeps the write honest, because that writer is the choke point where
// canon.Reconcile preserves identity fields and elides units that did not really
// change (ADR-0008). Copying whole units is safe for the same reason a module
// transplant is: no binary `$ID` pointer crosses a unit boundary.
//
// An MPR v1 project needs none of this and is handed straight to the tool.
//
// On any failure after the tool has run, the v2 storage is restored before
// returning, so a failed harvest leaves the project as it was rather than as a
// half-converted v1 file.
func RunToolPreservingFormat(mxPath, projectPath, subcommand string, w, stderr io.Writer) (*HarvestResult, error) {
	abs, err := filepath.Abs(projectPath)
	if err != nil {
		abs = projectPath
	}
	args := []string{subcommand, abs}

	reader, err := openReadOnly(projectPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", projectPath, err)
	}
	isV2 := reader.Version() == types.MPRVersionV2
	contentsDir := reader.ContentsDir()
	_ = reader.Disconnect()

	if !isV2 {
		// v1 is what these tools already produce; nothing to protect.
		if err := mxToolCmd(mxPath, args, w, stderr); err != nil {
			return nil, fmt.Errorf("mx %s: %w", subcommand, err)
		}
		return &HarvestResult{}, nil
	}
	before := unitCount(contentsDir)

	_, restore, err := snapshotStorageFormat(projectPath, contentsDir)
	if err != nil {
		// Without a snapshot the conversion would be unrecoverable. Refuse rather
		// than trade the storage format for the fix.
		return nil, fmt.Errorf("snapshot MPR v2 storage: %w\n"+
			"  refusing to run 'mx %s', which would convert the project to MPR v1 with no way back", err, subcommand)
	}

	if err := mxToolCmd(mxPath, args, w, stderr); err != nil {
		restore()
		return nil, fmt.Errorf("mx %s: %w", subcommand, err)
	}

	fixed, order, err := readAllUnits(projectPath)
	if err != nil {
		restore()
		return nil, fmt.Errorf("read back what 'mx %s' produced: %w", subcommand, err)
	}

	// Back to v2, pre-fix, with the tool's output held in memory.
	restore()

	res, err := applyHarvest(projectPath, fixed, order)
	if err != nil {
		return nil, err
	}
	res.StorageFilesBefore = before
	res.StorageFiles = unitCount(contentsDir)
	res.StillV2 = verifyStillV2(projectPath)
	return res, nil
}

// harvestedUnit is a unit as the tool left it, with the placement metadata an
// insert needs.
type harvestedUnit struct {
	contents        []byte
	containerID     string
	containmentName string
	unitType        string
}

// readAllUnits reads every unit out of a project, returning them by ID plus a
// stable ordering. The order is the reader's, which is the storage order — not
// map order, which would make the write sequence (and any failure point)
// unreproducible.
func readAllUnits(projectPath string) (map[string]harvestedUnit, []string, error) {
	reader, err := openReadOnly(projectPath)
	if err != nil {
		return nil, nil, err
	}
	defer reader.Disconnect()

	units, err := reader.ListUnits()
	if err != nil {
		return nil, nil, err
	}

	out := make(map[string]harvestedUnit, len(units))
	order := make([]string, 0, len(units))
	for _, u := range units {
		raw, rerr := reader.GetRawUnitBytes(u.ID)
		if rerr != nil || len(raw) == 0 {
			// A unit that cannot be read cannot be carried over. Skipping it leaves
			// the stored version in place, which is the safe direction.
			continue
		}
		id := string(u.ID)
		out[id] = harvestedUnit{
			contents:        append([]byte{}, raw...),
			containerID:     string(u.ContainerID),
			containmentName: u.ContainmentName,
			unitType:        u.Type,
		}
		order = append(order, id)
	}
	return out, order, nil
}

// applyHarvest writes the tool's units into the restored v2 project, skipping
// the ones that did not really change.
func applyHarvest(projectPath string, fixed map[string]harvestedUnit, order []string) (*HarvestResult, error) {
	res := &HarvestResult{Harvested: true}

	writer, err := openForWriting(projectPath)
	if err != nil {
		return nil, fmt.Errorf("open %s for writing: %w", projectPath, err)
	}
	defer writer.Disconnect()

	stored, _, err := readAllUnits(projectPath)
	if err != nil {
		return nil, fmt.Errorf("read the restored project: %w", err)
	}

	for _, id := range order {
		h := fixed[id]
		prev, ok := stored[id]
		if !ok {
			res.Added = append(res.Added, fmt.Sprintf("%s %s", h.unitType, id))
			continue
		}
		out, unchanged := canon.Reconcile(h.contents, prev.contents)
		if unchanged {
			continue
		}
		if err := writer.UpdateRawUnit(id, out); err != nil {
			return nil, fmt.Errorf("write unit %s: %w", id, err)
		}
		res.UnitsWritten++
	}

	for id, s := range stored {
		if _, ok := fixed[id]; !ok {
			res.Removed = append(res.Removed, fmt.Sprintf("%s %s", s.unitType, id))
		}
	}
	sort.Strings(res.Added)
	sort.Strings(res.Removed)
	return res, nil
}

// verifyStillV2 reports whether the project is on disk in the MPR v2 layout. The
// commands built on RunToolPreservingFormat assert this after they run: the
// whole point is that the format survives, and an assertion is cheaper than a
// bug report.
func verifyStillV2(projectPath string) bool {
	reader, err := openReadOnly(projectPath)
	if err != nil {
		return false
	}
	defer reader.Disconnect()
	return reader.Version() == types.MPRVersionV2
}

// unitCount is used by the commands to report the storage size before and after,
// which is the number a v2 -> v1 conversion moves to zero.
//
// Only `.mxunit` files are counted. `mprcontents/` also holds an `mprname`
// metadata file, and counting it makes the reported storage size one higher than
// the unit count every other part of mxcli prints — which reads as a unit having
// been dropped, and costs whoever notices an investigation to find out it was
// not.
func unitCount(contentsDir string) int {
	n := 0
	_ = filepath.WalkDir(contentsDir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && filepath.Ext(path) == ".mxunit" {
			n++
		}
		return nil
	})
	return n
}
