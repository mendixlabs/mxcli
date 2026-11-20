// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// RuntimeController drives the Mendix runtime admin (M2EE) API for the local dev
// loop: hot-reload the model, run the DB-aware start cycle, and apply a serve
// build result via the restartRequired branch. It orchestrates admin actions;
// the actual runtime process launch/relaunch is the caller's responsibility (a
// domain/view/association change needs a fresh runtime start — see
// docs/11-proposals/PROPOSAL_mxcli_dev_warm_loop.md § Hot-reload scope).
type RuntimeController struct {
	opts M2EEOptions
	// LogSubscriberFile, when non-empty, makes Start attach a Mendix "file" log
	// subscriber writing the application log (microflow LOG output, server-side
	// stack traces) to this file after a successful start. A standalone runtime
	// attaches no log subscriber by default — unlike a Studio Pro / m2ee run,
	// which configures one after start — so without this the application log is
	// never written anywhere and a page-action error shows only the generic
	// Mendix dialog (findings #25). The runtime is told never to rotate the file
	// (max_rotate 0) because run --local also tees the JVM's own stdout/stderr to
	// this same path, and a rotate-rename would detach that handle.
	LogSubscriberFile string
	// Stdout receives a non-fatal warning if the log subscriber cannot be
	// attached (the app is already up at that point). nil is silent.
	Stdout io.Writer
	// StartTimeout bounds the start and execute_ddl_commands actions, which run
	// the after-startup microflow and the schema update. Zero falls back to
	// $MXCLI_START_TIMEOUT, then 15m (see runtimeStartTimeout). Every other
	// admin action keeps the connection's own Timeout.
	StartTimeout time.Duration
}

// defaultStartTimeout bounds one start (or schema update) when nothing
// overrides it. It was the admin connection's 150 s, which an app's
// after-startup microflow outran on every start — and mxcli then shut the
// runtime down mid-startup (mendixlabs/mxcli#1818). A start that fails reports
// its failure in the response; this only has to outlast a slow successful one.
const defaultStartTimeout = 15 * time.Minute

// startTimeoutEnv overrides defaultStartTimeout. The --start-timeout flag of
// `run --local` wins over it.
const startTimeoutEnv = "MXCLI_START_TIMEOUT"

func runtimeStartTimeout(explicit time.Duration) time.Duration {
	return durationSetting(explicit, startTimeoutEnv, defaultStartTimeout)
}

// callStartPhase runs a start-phase admin action under the start timeout, and
// says how to raise it when it runs out.
func (c *RuntimeController) callStartPhase(action string) (*M2EEResponse, error) {
	opts := c.opts
	opts.Timeout = runtimeStartTimeout(c.StartTimeout)
	resp, err := CallM2EE(opts, action, nil)
	if err != nil && isAdminTimeout(err) {
		return nil, fmt.Errorf("%w (the runtime was still starting -- an after-startup microflow or a schema "+
			"update can take longer; raise the limit with --start-timeout or $%s)", err, startTimeoutEnv)
	}
	return resp, err
}

// logSubscriberName identifies the file log subscriber run --local attaches. A
// stable name means a restart's re-attach replaces rather than duplicates it.
const logSubscriberName = "mxcli-run-local"

// maxRuntimeLogSize bounds the file subscriber. With max_rotate 0 the runtime
// never rotates, so keep this generous — it is a per-session dev log, not a
// production sink.
const maxRuntimeLogSize = 1 << 30 // 1 GiB

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
	resp, err := c.callStartPhase("start")
	if err != nil {
		return nil, err
	}
	schemaAttempted := false
	if needsDBUpdate(resp) {
		ddl, err := c.callStartPhase("execute_ddl_commands")
		if err != nil {
			return nil, err
		}
		if msg := ddl.M2EEError(); msg != "" {
			return nil, fmt.Errorf("execute_ddl_commands failed: %s", msg)
		}
		schemaAttempted = true
		resp, err = c.callStartPhase("start")
		if err != nil {
			return nil, err
		}
	}
	if msg := resp.M2EEError(); msg != "" {
		if schemaAttempted {
			return resp, fmt.Errorf("start failed after creating or updating the database schema: %s", msg)
		}
		return resp, fmt.Errorf("start failed: %s", msg)
	}
	// The runtime is up; wire the application log to a file (best-effort — a
	// logging hiccup must not fail an otherwise-good start). Re-run on every
	// Start so a restart's fresh JVM re-attaches the subscriber (findings #25).
	if err := c.configureRuntimeLogging(); err != nil && c.Stdout != nil {
		fmt.Fprintf(c.Stdout, "  (runtime application log not attached: %v)\n", err)
	}
	return resp, nil
}

// configureRuntimeLogging wires the runtime's application log to LogSubscriberFile
// (a no-op when the field is empty). Two admin actions are needed, in order:
//
//  1. create_log_subscriber — register a "file" subscriber that autosubscribes to
//     every log node at INFO or above (mirrors m2ee-tools' post-start setup).
//  2. start_logging — a standalone runtime boots with logging NOT started, so the
//     registered subscriber sits inert and receives nothing until logging is
//     activated. Without this, runtime.log holds only the JVM banner and no
//     microflow LOG output or server stack traces ever appear (findings #25).
//
// start_logging is idempotent from our side: Start can run again on a
// still-running JVM (e.g. the DB-update retry path), so an "already started"
// response is treated as success rather than an error.
func (c *RuntimeController) configureRuntimeLogging() error {
	if c.LogSubscriberFile == "" {
		return nil
	}
	params := map[string]any{
		"type":          "file",
		"name":          logSubscriberName,
		"autosubscribe": "INFO",
		"filename":      c.LogSubscriberFile,
		"max_size":      maxRuntimeLogSize,
		"max_rotate":    0,
	}
	resp, err := CallM2EE(c.opts, "create_log_subscriber", params)
	if err != nil {
		return err
	}
	if msg := resp.M2EEError(); msg != "" {
		return fmt.Errorf("create_log_subscriber: %s", msg)
	}

	lg, err := CallM2EE(c.opts, "start_logging", nil)
	if err != nil {
		return err
	}
	if msg := lg.M2EEError(); msg != "" && !strings.Contains(strings.ToLower(msg), "already") {
		return fmt.Errorf("start_logging: %s", msg)
	}
	return nil
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

// needsDBUpdate reports whether a start response indicates the database must be
// created or updated before the runtime can serve.
//
// Two results mean that, and both need the same response (execute_ddl_commands
// then start again):
//
//   - result 3, "the database has to be updated" — the schema is out of date.
//   - result 2, "the database to be used does not exist" — there is no schema at
//     all. This is the normal first boot of the built-in HSQLDB database, whose
//     JDBC URL carries ifexists=true and therefore never creates the file.
//
// Measured on Mendix 11.12.2: a fresh built-in HSQLDB project answers start
// with Result 2 and Message "The database to be used does not exist.".
//
// The synchronizationreason feedback is a third signal the runtime sometimes
// sends instead of the message.
func needsDBUpdate(resp *M2EEResponse) bool {
	if resp == nil {
		return false
	}
	if resp.Result == 2 || resp.Result == 3 {
		return true
	}
	lower := strings.ToLower(resp.Message)
	if strings.Contains(lower, "database has to be updated") ||
		strings.Contains(lower, "database to be used does not exist") {
		return true
	}
	if fb := resp.Feedback(); fb != nil {
		if _, ok := fb["synchronizationreason"]; ok {
			return true
		}
	}
	return false
}
