// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"strings"
)

// RuntimeController drives the Mendix runtime admin (M2EE) API for the local dev
// loop: hot-reload the model, run the DB-aware start cycle, and apply a serve
// build result via the restartRequired branch. It orchestrates admin actions;
// the actual runtime process launch/relaunch is the caller's responsibility (a
// domain/view/association change needs a fresh runtime start — see
// docs/11-proposals/PROPOSAL_mxcli_dev_warm_loop.md § Hot-reload scope).
type RuntimeController struct {
	opts M2EEOptions
}

// NewRuntimeController returns a controller for the given admin API connection.
func NewRuntimeController(opts M2EEOptions) *RuntimeController {
	return &RuntimeController{opts: opts}
}

// ApplyAction is the action taken for a build result.
type ApplyAction int

const (
	// ActionReload: hot reload_model (no restart) — page/microflow/text change.
	ActionReload ApplyAction = iota
	// ActionRestart: the runtime must be relaunched — entity/view/association
	// change (the runtime reconciles its metamodel catalog only at startup).
	ActionRestart
)

func (a ApplyAction) String() string {
	if a == ActionReload {
		return "reload"
	}
	return "restart"
}

// DecideApply maps a build's restartRequired flag to the apply action. Kept
// separate so the decision is trivially testable and documented in one place.
func DecideApply(restartRequired bool) ApplyAction {
	if restartRequired {
		return ActionRestart
	}
	return ActionReload
}

// ReloadModel hot-reloads the model into the running runtime (model store +
// microflow engine + i18n), draining in-flight actions first. No process
// restart, no DDL. Use only when the build reported restartRequired=false.
func (c *RuntimeController) ReloadModel() error {
	resp, err := CallM2EE(c.opts, "reload_model", nil)
	if err != nil {
		return err
	}
	if msg := resp.M2EEError(); msg != "" {
		return fmt.Errorf("reload_model failed: %s", msg)
	}
	return nil
}

// Start runs the runtime start sequence, handling an empty or out-of-date
// database: start -> if the runtime reports the schema must change ->
// execute_ddl_commands -> start. Returns the final start response.
func (c *RuntimeController) Start() (*M2EEResponse, error) {
	resp, err := CallM2EE(c.opts, "start", nil)
	if err != nil {
		return nil, err
	}
	if needsDBUpdate(resp) {
		ddl, err := CallM2EE(c.opts, "execute_ddl_commands", nil)
		if err != nil {
			return nil, err
		}
		if msg := ddl.M2EEError(); msg != "" {
			return nil, fmt.Errorf("execute_ddl_commands failed: %s", msg)
		}
		resp, err = CallM2EE(c.opts, "start", nil)
		if err != nil {
			return nil, err
		}
	}
	if msg := resp.M2EEError(); msg != "" {
		return resp, fmt.Errorf("start failed: %s", msg)
	}
	return resp, nil
}

// RuntimeStatus returns the runtime status string (e.g. "running", "starting").
func (c *RuntimeController) RuntimeStatus() (string, error) {
	resp, err := CallM2EE(c.opts, "runtime_status", nil)
	if err != nil {
		return "", err
	}
	fb := resp.Feedback()
	if fb == nil {
		return "", nil
	}
	status, _ := fb["status"].(string)
	return status, nil
}

// ApplyBuild applies a serve build result to the running runtime:
//   - restartRequired=false -> reload_model (hot, done here).
//   - restartRequired=true  -> relaunch (via the caller's restart func) then run
//     the DB-aware Start cycle.
//
// restart may be nil when the caller drives the relaunch itself; in that case a
// restart-required build only returns ActionRestart (no admin calls are made).
func (c *RuntimeController) ApplyBuild(build *BuildResult, restart func() error) (ApplyAction, error) {
	if build == nil {
		return ActionReload, fmt.Errorf("nil build result")
	}
	action := DecideApply(build.RestartRequired)
	if action == ActionReload {
		return action, c.ReloadModel()
	}
	if restart == nil {
		return action, nil
	}
	if err := restart(); err != nil {
		return action, fmt.Errorf("restarting runtime: %w", err)
	}
	if _, err := c.Start(); err != nil {
		return action, err
	}
	return action, nil
}

// needsDBUpdate reports whether a start response indicates the database schema
// must be updated before the runtime can serve (result 3 / "database has to be
// updated" / a synchronizationreason in the feedback).
func needsDBUpdate(resp *M2EEResponse) bool {
	if resp == nil {
		return false
	}
	if resp.Result == 3 {
		return true
	}
	if strings.Contains(strings.ToLower(resp.Message), "database has to be updated") {
		return true
	}
	if fb := resp.Feedback(); fb != nil {
		if _, ok := fb["synchronizationreason"]; ok {
			return true
		}
	}
	return false
}
