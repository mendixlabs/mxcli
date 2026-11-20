// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mendixlabs/mxcli/internal/pathutil"
	"github.com/mendixlabs/mxcli/internal/procalive"
)

// devLoopHandshakeName is what `mxcli run --local` publishes so another mxcli
// process can reach the app it is serving.
//
// It is the sibling of testrunner's test-endpoint.json, and exists for the same
// reason: a second process cannot otherwise know which ports this dev loop chose
// or what configuration its runtime was booted with. The admin API has no
// read-back, so "what was it booted with" cannot be asked — only remembered.
const devLoopHandshakeName = "run-local.json"

// devLoopHandshake is the contract between a running `mxcli run --local` and a
// command that wants to change something about the app it is serving.
//
// It carries a live credential and the runtime's database password, so it is
// written 0600 into the gitignored .mxcli directory and removed when the loop
// exits. It is not a secret store: the values only work against a loopback
// admin port on this machine, and only while that runtime is up.
type devLoopHandshake struct {
	// Project is the .mpr this loop is serving, so a caller can refuse a
	// handshake left behind by a different project.
	Project string `json:"project"`
	// PID of the hosting process, used to detect a stale file.
	PID int `json:"pid"`
	// AppPort is where the app itself is reachable.
	AppPort int `json:"appPort"`
	// AdminPort/AdminPass reach the M2EE admin API.
	AdminPort int    `json:"adminPort"`
	AdminPass string `json:"adminPass"`
	// BootConfig is the update_configuration payload the runtime was started
	// with. A caller changing one setting re-sends this with that key replaced —
	// the only way to avoid guessing at everything it does not want to change.
	BootConfig map[string]any `json:"bootConfig"`
	// Started is when this was published, for a clearer stale message.
	Started time.Time `json:"started"`
}

// bootConfigForHandshake is the boot payload without MicroflowConstants. Every
// resolved constant is in there, secrets included, and nothing reads them back:
// docker.ApplyConstants replaces that key with the freshly resolved chain
// before it re-sends the payload.
func bootConfigForHandshake(boot map[string]any) map[string]any {
	if _, ok := boot["MicroflowConstants"]; !ok {
		return boot
	}
	out := make(map[string]any, len(boot))
	for k, v := range boot {
		if k != "MicroflowConstants" {
			out[k] = v
		}
	}
	return out
}

func devLoopHandshakePath(projectPath string) string {
	return filepath.Join(filepath.Dir(projectPath), ".mxcli", devLoopHandshakeName)
}

// writeDevLoopHandshake publishes the handshake, replacing any existing one.
func writeDevLoopHandshake(projectPath string, h devLoopHandshake) error {
	path := devLoopHandshakePath(projectPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	body, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("publishing %s: %w", path, err)
	}
	return nil
}

// withdrawDevLoopHandshake removes the handshake the run with pid published,
// and nothing else.
//
// Removal used to be unconditional, and every `mxcli run --local` deferred it —
// including `--setup`, which never publishes one and which the SessionStart
// hook runs on every session start, resume and compaction included. So while a
// detached run was serving the app, the hook deleted ITS handshake: oql, log
// and constant --apply fell back to default credentials, met HTTP 403 from a
// runtime that was up, and the caller blamed the password (ako/mxcli#1542).
// A file that cannot be read is left alone too: it is not provably ours.
func withdrawDevLoopHandshake(projectPath string, pid int) {
	path := devLoopHandshakePath(projectPath)
	body, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var h devLoopHandshake
	if json.Unmarshal(body, &h) != nil || h.PID != pid {
		return
	}
	_ = os.Remove(path)
}

// readDevLoopHandshake returns the handshake for a project, or an error a user
// can act on.
//
// A handshake whose process is gone is refused rather than used: the ports in it
// may since have been taken by something else, and sending a configuration
// change to the wrong process is worse than saying no dev loop is running.
//
// Both refusals quote `run status`, which knows what became of the run — the
// licence limit, a crash — where the missing file alone reads like a
// credentials problem (ako/mxcli#1542).
func readDevLoopHandshake(projectPath string) (devLoopHandshake, error) {
	var h devLoopHandshake
	path := devLoopHandshakePath(projectPath)
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return h, fmt.Errorf("%w\n"+
			"  start one in another terminal, or drop --apply to only record the value%s",
			errNoDevLoopHandshake, runStatusNote(projectPath))
	}
	if err != nil {
		return h, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := json.Unmarshal(body, &h); err != nil {
		return h, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	// A copied app folder carries the original's handshake. Its ports and
	// password belong to the original's runtime, so it is no run of this project.
	if !pathutil.SameProject(h.Project, projectPath) {
		return devLoopHandshake{}, fmt.Errorf("%w\n"+
			"  (%s was left by the run of %s, not this project; ignoring it)%s",
			errNoDevLoopHandshake, path, h.Project, runStatusNote(projectPath))
	}
	if !procalive.Alive(h.PID) {
		return h, fmt.Errorf("%s refers to process %d, which is %w\n"+
			"  (the dev loop was stopped without cleaning up; start a new one)%s",
			path, h.PID, errStaleDevLoopHandshake, runStatusNote(projectPath))
	}
	return h, nil
}

var (
	// errNoDevLoopHandshake: no run-local.json — no run is serving, or it has
	// ended and withdrawn it.
	errNoDevLoopHandshake = errors.New("no 'mxcli run --local' is serving this project")
	// errStaleDevLoopHandshake: run-local.json names a process that is gone.
	errStaleDevLoopHandshake = errors.New("no longer running")
)

// runStatusNote is "\n  mxcli run status: <line>" for a project with a
// recorded run, or "".
func runStatusNote(projectPath string) string {
	if line := runStatusSummary(projectPath); line != "" {
		return "\n  mxcli run status: " + line
	}
	return ""
}
