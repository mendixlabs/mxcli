// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"fmt"
	"io"
	"os"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// HostedEndpoint is a test endpoint installed into a project for the lifetime of
// a dev loop, so `mxcli test --attach` can run tests against it without booting
// its own runtime.
//
// It exists because the endpoint cannot be added to a running app: the handler
// is registered by the after-startup microflow, which only runs at boot, and its
// token comes from the runtime's environment. Whoever boots the app therefore
// has to opt in — which is also the right place for the decision, since hosting
// it means the developer's own app carries a microflow-executing endpoint and
// runs tests against the database they are looking at.
type HostedEndpoint struct {
	// Token the runtime must be given as MXCLI_TEST_TOKEN.
	Token string
	// Env is the entry to add to the runtime process environment.
	Env []string
	// AllowExec switches on the endpoint's exec route (--allow-call).
	AllowExec bool

	projectPath string
	state       projectState
	out         io.Writer
	removed     bool
}

// InstallHostedEndpoint injects the test endpoint into the project and returns
// what the caller needs to boot with it. The caller must Remove it on shutdown.
//
// The project's own after-startup microflow is chained rather than displaced, so
// the app still boots the way the developer expects.
//
// allowExec switches on the exec route (--allow-call): the runtime is given
// endpointAllowExecEnv, and the handshake says so. Without it the route answers
// 403, whatever a caller sends.
func InstallHostedEndpoint(projectPath string, allowExec bool, w io.Writer) (*HostedEndpoint, error) {
	token, err := newEndpointToken()
	if err != nil {
		return nil, err
	}

	// An endpoint an earlier run could not remove (a Windows `run stop`, a crash)
	// comes out first: captured as the project's own state, its after-startup
	// would be chained to and its module kept as the user's.
	if _, err := RestoreAbandonedEndpoint(projectPath, w); err != nil {
		return nil, err
	}

	state, err := captureProjectState(projectPath)
	if err != nil {
		return nil, fmt.Errorf("capturing project state: %w", err)
	}
	// Written before anything is installed, so a run that is killed from here on
	// can still be undone by `run stop` (ako/mxcli#1917).
	if err := writeRestoreRecord(projectPath, os.Getpid(), state); err != nil {
		fmt.Fprintf(w, "Warning: could not record how to remove the test endpoint (%v); if this run is killed, `run stop` cannot remove it\n", err)
	}

	h := &HostedEndpoint{
		Token:       token,
		Env:         []string{endpointTokenEnv + "=" + token},
		AllowExec:   allowExec,
		projectPath: projectPath,
		state:       state,
		out:         w,
	}

	if allowExec {
		h.Env = append(h.Env, endpointAllowExecEnv+"=1")
	}

	fmt.Fprintln(w, "Installing the mxcli test endpoint (for 'mxcli test --attach')...")
	if err := execMDLScript(projectPath, GenerateEndpointMDL(state.afterStartup), "mxtest-endpoint-*.mdl"); err != nil {
		h.Remove()
		return nil, fmt.Errorf("injecting the test endpoint: %w", err)
	}
	if err := execMxcliCmd(projectPath, "ALTER SETTINGS RUNTIME (AfterStartupMicroflow: "+quoteMDLString(endpointStartupFlow)+")"); err != nil {
		h.Remove()
		return nil, fmt.Errorf("pointing after-startup at the endpoint: %w", err)
	}
	if state.afterStartup != "" {
		fmt.Fprintf(w, "  after-startup chained: %s runs, then %s\n", endpointStartupFlow, state.afterStartup)
	}
	return h, nil
}

// Publish writes the handshake that `mxcli test --attach` reads. Called once the
// app is actually serving, so an attach never finds a handshake for a runtime
// that has not come up.
func (h *HostedEndpoint) Publish(info docker.LocalAppInfo) error {
	if h == nil {
		return nil
	}
	err := WriteHandshake(h.projectPath, Handshake{
		Project:   h.projectPath,
		PID:       os.Getpid(),
		AppPort:   info.AppPort,
		AdminPort: info.AdminPort,
		ServePort: info.ServePort,
		AdminPass: info.AdminPass,
		Token:     h.Token,
		AllowExec: h.AllowExec,
		Started:   nowFunc(),
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(h.out, "  test endpoint ready — run 'mxcli test <files> -p %s --attach' from another terminal\n", h.projectPath)
	return nil
}

// Remove withdraws the handshake and restores the project.
//
// Best-effort by design: this runs on the dev loop's shutdown path, where the
// user is trying to stop the app, and a hard failure there helps nobody. What it
// must not do is stay silent — a project left carrying the endpoint is something
// the developer has to know about before they commit.
func (h *HostedEndpoint) Remove() {
	// Idempotent and nil-safe: the caller both defers this and calls it on the
	// os.Exit path (which skips defers), so it must tolerate running twice.
	if h == nil || h.removed {
		return
	}
	h.removed = true
	RemoveHandshake(h.projectPath)
	if err := cleanupEndpoint(h.projectPath, h.state, &TestSuite{}, h.out); err != nil {
		fmt.Fprintf(h.out, "\nERROR: could not remove the test endpoint — the project has been left modified:\n%v\n", err)
		fmt.Fprintf(h.out, "Check the after-startup microflow and the %s module before committing.\n", mxTestModule)
		return
	}
	removeGeneratedJavaSource(h.projectPath, h.out)
	// Only past the error return above, so this never claims a project is
	// unchanged while an injection is still in it.
	restoreProjectFile(h.state, nil, h.out)
	// Kept on the error path above, so `run stop` tries the removal again.
	removeRestoreRecord(h.projectPath)
	fmt.Fprintln(h.out, "  test endpoint removed; project restored")
}
