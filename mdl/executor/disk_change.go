// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/mendixlabs/mxcli/mdl/backend"
)

// syncWithDisk is run before each statement: when another process — a second
// mxcli, Studio Pro, a git checkout — changed the project since the session
// last looked, the session drops what it kept of it and reads it afresh
// (ako/mxcli#2121). Without it a long-lived session went on answering from the
// project as it first read it: `show` left out documents made elsewhere,
// `describe` said "not found" for them, and on MPR v2 a `create` of a name
// another process had taken wrote a second document with that name.
//
// The probe is one PRAGMA and one stat (backend.DiskChangeSyncer); the
// session's own writes do not trip it. It is skipped while an access-rule run
// holds deferred writes: the run's statements are one write, and the probe
// runs again before the statement after it.
func (e *Executor) syncWithDisk() error {
	if e.backend == nil || e.accessRun != nil || !e.backend.IsConnected() {
		return nil
	}
	syncer, ok := e.backend.(backend.DiskChangeSyncer)
	if !ok {
		return nil
	}
	changed, err := syncer.SyncWithDisk()
	if !changed {
		return err
	}
	if e.cache != nil {
		e.cache.dropProjectReads()
	}
	if e.logger != nil {
		e.logger.Info("project changed on disk by another process; re-reading", "path", e.mprPath)
	}
	return err
}

// dropProjectReads forgets what the cache read from the project, keeping what
// it records about this session's own writes (created and dropped documents,
// domain models awaiting finalization), which another process's write does not
// undo.
func (c *executorCache) dropProjectReads() {
	c.modulesMu.Lock()
	c.modules = nil
	c.modulesMu.Unlock()
	c.units = nil
	c.folders = nil
	c.domainModels = nil
	c.enumerations = nil
	c.hierarchy = nil
	c.defaultLang, c.defaultLangLoaded = "", false
	c.entityNames = nil
	c.microflowNames = nil
	c.pageNames = nil
}
