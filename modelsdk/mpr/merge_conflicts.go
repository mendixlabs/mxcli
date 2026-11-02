// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"
	"strings"
)

// Merge conflict markers in the unit index.
//
// When Mendix's merge engine (`mx merge`, Studio Pro's Merge Changes Here)
// meets a change it cannot decide, it still writes a result, and marks the
// undecided units in their Unit row: ContentsConflicts holds a JSON list of
// the conflicting changes instead of '' (measured on mx 11.13: every other row
// holds '', never NULL), and TreeConflict is non-zero when the conflict is
// about whether the document exists at all. `mx check` reports CE0137 ("There
// are merge conflicts in this project") for as long as a marker is present.
// The index's other tables carry no conflict state.

// UnitConflict is a unit whose index row carries a merge conflict marker.
type UnitConflict struct {
	UnitID   string // Mendix UUID form
	Contents string // ContentsConflicts as stored ("" when only Tree is set)
	Tree     int64  // TreeConflict as stored (0 when only Contents is set)
}

// UnitConflicts lists the units whose index row carries a merge conflict
// marker. Read-only.
func (r *Reader) UnitConflicts() ([]UnitConflict, error) {
	rows, err := r.db.Query(`SELECT UnitID, COALESCE(ContentsConflicts, ''), COALESCE(TreeConflict, 0) FROM Unit
		WHERE COALESCE(ContentsConflicts, '') != '' OR COALESCE(TreeConflict, 0) != 0`)
	if err != nil {
		return nil, fmt.Errorf("query merge conflict markers: %w", err)
	}
	defer rows.Close()
	var out []UnitConflict
	for rows.Next() {
		var blob []byte
		var c UnitConflict
		if err := rows.Scan(&blob, &c.Contents, &c.Tree); err != nil {
			return nil, fmt.Errorf("scan merge conflict marker: %w", err)
		}
		c.UnitID = blobToUUID(blob)
		out = append(out, c)
	}
	return out, rows.Err()
}

// ClearContentsConflicts empties the ContentsConflicts marker of the given
// units — the step that records a contents conflict as decided, once the
// unit's file holds the chosen content — leaving the row as the engine writes
// every unit it merged without a decision. A TreeConflict is left alone: which
// documents exist is not a contents decision. One SQLite transaction, with
// _Transaction.LastTransactionID bumped as every other write does, under the
// writer's Studio Pro open-project guard. Returns the number of rows cleared.
func (w *Writer) ClearContentsConflicts(unitIDs []string) (int, error) {
	if len(unitIDs) == 0 {
		return 0, nil
	}
	if err := w.guardWrite(); err != nil {
		return 0, err
	}
	tx, err := w.reader.db.Begin()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range unitIDs {
		res, err := tx.Exec(`UPDATE Unit SET ContentsConflicts = '' WHERE UnitID = ? AND COALESCE(ContentsConflicts, '') != ''`, uuidToBlob(id))
		if err != nil {
			_ = tx.Rollback()
			return 0, fmt.Errorf("clear the merge conflict marker of %s: %w", id, err)
		}
		if c, _ := res.RowsAffected(); c > 0 {
			n += int(c)
		}
	}
	if _, err := tx.Exec(`UPDATE _Transaction SET LastTransactionID = ?`, generateUUID()); err != nil &&
		!strings.Contains(err.Error(), "no such table") {
		_ = tx.Rollback()
		return 0, fmt.Errorf("update _Transaction: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	w.reader.InvalidateCache()
	return n, nil
}
