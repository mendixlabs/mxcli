// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// judgedWriteDeferrer is the optional backend capability a run of entity and
// association statements is written through. A backend without it writes each
// statement as it always has.
type judgedWriteDeferrer interface {
	DeferUnitWritesJudged()
	FlushDeferredWrites() error
}

// entityRun writes a run of consecutive CREATE ENTITY and CREATE ASSOCIATION
// statements once, at its end.
//
// Each of those statements rewrites its module's whole domain model, and each
// rewrite was reconciled against the stored unit and written: half the time of
// a script that builds a domain model, and a cost per statement that grows with
// the model it is adding to (ako/mxcli#2160). Held until the run ends, a domain
// model is reconciled and written once however many statements changed it.
//
// Unlike an access-rule run, the statements report as they go. The writer
// judges each held write as it is held (Writer.DeferUnitWritesJudged), so
// "Created entity", "Modified entity" and "Unchanged" mean what they mean when
// a statement writes for itself, and a re-run of a settled script still reports
// every statement unchanged.
//
// What changes is when a write can fail. The reconcile against storage — and
// with it the storage-GUID guard — happens at the end of the run, after the
// statements have reported. If it refuses, the run's error names the domain
// model that was not written, and the statements that changed it did not take
// effect.
//
// The run is only plain entities and associations. A view entity or an
// external entity writes other documents beside the domain model, and any other
// statement may read or write the model another way; each ends the run first.
type entityRun struct {
	open judgedWriteDeferrer
}

func isEntityRunStmt(stmt ast.Statement) bool {
	switch s := stmt.(type) {
	case *ast.CreateEntityStmt:
		return s.Kind == ast.EntityPersistent || s.Kind == ast.EntityNonPersistent
	case *ast.CreateAssociationStmt:
		return true
	}
	return false
}

// step opens the run before an entity or association statement and ends it
// before any other statement.
func (r *entityRun) step(e *Executor, stmt ast.Statement) error {
	if !isEntityRunStmt(stmt) {
		return r.end()
	}
	if r.open != nil || e.backend == nil || !e.backend.IsConnected() {
		return nil
	}
	if d, ok := e.backend.(judgedWriteDeferrer); ok {
		d.DeferUnitWritesJudged()
		r.open = d
	}
	return nil
}

// end writes what the run holds. Safe to call with no run open.
func (r *entityRun) end() error {
	if r.open == nil {
		return nil
	}
	d := r.open
	r.open = nil
	if err := d.FlushDeferredWrites(); err != nil {
		return mdlerrors.NewBackend("write the domain model the statements above changed — they did not take effect", err)
	}
	return nil
}

// stepRuns moves both kinds of run past stmt. The run stmt does not belong to
// is ended before the one it does belong to is opened: both hold their writes
// in the same place, and a run that opened over another's would have its
// writes flushed, and its deferral closed, by the other's end.
func stepRuns(e *Executor, rules *accessRuleRun, entities *entityRun, stmt ast.Statement) error {
	if isEntityRunStmt(stmt) {
		if err := rules.step(e, stmt); err != nil {
			return err
		}
		return entities.step(e, stmt)
	}
	if err := entities.step(e, stmt); err != nil {
		return err
	}
	return rules.step(e, stmt)
}
