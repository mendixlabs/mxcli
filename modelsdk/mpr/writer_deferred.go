// SPDX-License-Identifier: Apache-2.0

package mpr

import "github.com/mendixlabs/mxcli/modelsdk/canon"

// Deferred unit writes: a run of statements that each rewrite the same unit is
// judged by where the run ENDS, not by every step on the way.
//
// # Why
//
// Each write is reconciled against what is on disk at the moment it lands
// (ADR-0008). That is the right question for one statement and the wrong one for
// a sequence whose steps undo each other. The measured case is a "reset, then
// authoritative grants" section (ako/mxcli#872, rehearsal W3):
//
//	revoke all on entity M.E from M.R;
//	grant read *, write * on entity M.E to M.R;
//
// The revoke removes the rule and is written; the grant builds the rule afresh
// and is written against a unit that no longer has it, so there is nothing to
// carry the old rule's $IDs from. Net: the domain model is rewritten on every
// run, the rule re-minted, the project's transaction id moved — although the
// rules it ends with are the ones it started with.
//
// # How
//
// While deferral is on, a unit update is held in memory instead of reaching
// storage, and the reader serves the held bytes (GetRawUnitBytes and the unit
// listings alike), so every later read in the run sees the run's own writes. On
// flush each held unit goes through updateUnit once, and is reconciled against
// what was on disk BEFORE the run: a run that nets to nothing is elided like any
// other no-op, and one that changes something carries the stored identities onto
// what it writes.
//
// Only updates are held. An insert or a delete is not something a later
// statement of the run can undo into a no-op, and deleting a unit drops its held
// write. The caller decides what forms a run and must flush before anything that
// writes through another path (see executor.accessRuleRun).
type deferredWrite struct {
	contents []byte
	opts     []canon.Option
	// judged is the write as a judged run compares it: normalised for storage,
	// as the stored unit it is first compared with was. Nil outside such a run.
	judged []byte
}

// DeferUnitWrites starts holding unit updates until FlushDeferredWrites. It is
// idempotent; a run already open stays open.
func (w *Writer) DeferUnitWrites() {
	if w.deferred == nil {
		w.deferred = map[string]deferredWrite{}
	}
}

// DeferUnitWritesJudged is DeferUnitWrites for a run whose statements each
// report whether they changed anything, as they do when each writes for itself.
//
// A script that creates one entity after another rewrote its module's whole
// domain model for every statement, and reconciled each rewrite against the
// stored unit: half of the time such a script took (ako/mxcli#2160). Held, the
// unit is reconciled against storage once, when the run ends. What the
// statements still need is the answer the reconcile gave them, "did this
// change anything", so each held write is judged as it is held and counted in
// WriteStats exactly as a write that reached storage is: offered, and landed
// unless it changed nothing.
//
// The first write of a unit in the run is judged against the stored unit, with
// the comparison a real write gets. Every later one is judged against the
// write the run already holds for it. Both are what the builder produced, so
// they differ exactly where the statement changed something, and comparing
// them needs none of what a comparison with stored bytes does first.
//
// The flush does not count again: its one write per unit is the sum of
// statements already counted.
func (w *Writer) DeferUnitWritesJudged() {
	w.DeferUnitWrites()
	w.deferredJudged = true
}

// judgeHeld counts a write a judged run is about to hold, and returns the form
// the next write of the unit is compared with.
//
// Both sides are normalised for storage first. The writer completes what a
// builder leaves out (a validation rule's empty message, among others), so a
// unit read back from storage and rebuilt has those and a freshly built
// element does not: compared raw, a statement that re-declares an entity
// exactly as it is reads as a change.
func (w *Writer) judgeHeld(unitID string, contents []byte, opts []canon.Option) []byte {
	w.writesOffered++
	normalized, err := w.normalizeForStorage(unitID, contents)
	if err != nil {
		// Not comparable; the flush will report what is wrong with it.
		w.writesLanded++
		return nil
	}
	changed := true
	if prev, seen := w.deferred[unitID]; seen {
		if prev.judged != nil {
			carried := canon.CarryIdentity(normalized, prev.judged)
			if equal, err := canon.Equal(carried, prev.judged); err == nil && equal {
				changed = false
			}
		}
	} else if stored, err := w.reader.GetRawUnitBytes(unitID); err == nil {
		w.reconciles++
		_, unchanged := canon.Reconcile(normalized, stored, opts...)
		changed = !unchanged
	}
	if changed {
		w.writesLanded++
	}
	return normalized
}

// HoldsWrites reports whether a deferred run or a simulation is open, holding
// unit bytes that are not on disk.
func (w *Writer) HoldsWrites() bool {
	return w.deferred != nil || w.simulating
}

// FlushDeferredWrites ends the run: every held unit is written once, through the
// ordinary path (reconciliation, elision, the storage-GUID guard), in the order
// it was first written. The first error is returned; the remaining units are
// still attempted, since each is a complete document of its own.
func (w *Writer) FlushDeferredWrites() error {
	if w.simulating {
		// A simulation is discarded by whoever opened it, never written.
		return ErrSimulatedWrite
	}
	held, order := w.deferred, w.deferredOrder
	judged := w.deferredJudged
	w.deferred, w.deferredOrder, w.deferredJudged = nil, nil, false
	// A judged run counted each write as it was held.
	offered, landed := w.writesOffered, w.writesLanded
	if judged {
		defer func() { w.writesOffered, w.writesLanded = offered, landed }()
	}
	var first error
	for _, id := range order {
		d, ok := held[id]
		if !ok {
			continue // deleted during the run
		}
		w.reader.ClearOverlay(id)
		if err := w.updateUnit(id, d.contents, d.opts...); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// holdDeferred records an update while a run is open and reports whether it did.
// The latest bytes of a unit win, with the options of the write that produced
// them.
func (w *Writer) holdDeferred(unitID string, contents []byte, opts []canon.Option) bool {
	if w.deferred == nil {
		return false
	}
	var judged []byte
	if w.deferredJudged {
		judged = w.judgeHeld(unitID, contents, opts)
	}
	if _, seen := w.deferred[unitID]; !seen {
		w.deferredOrder = append(w.deferredOrder, unitID)
	}
	held := append([]byte(nil), contents...)
	w.deferred[unitID] = deferredWrite{contents: held, opts: opts, judged: judged}
	w.reader.SetOverlay(unitID, held)
	return true
}

// dropDeferred forgets a held update for a unit that is being deleted.
func (w *Writer) dropDeferred(unitID string) {
	if w.deferred == nil {
		return
	}
	if _, ok := w.deferred[unitID]; ok {
		delete(w.deferred, unitID)
		w.reader.ClearOverlay(unitID)
	}
}
