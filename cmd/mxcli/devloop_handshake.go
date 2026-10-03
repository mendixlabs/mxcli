// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

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

func removeDevLoopHandshake(projectPath string) {
	_ = os.Remove(devLoopHandshakePath(projectPath))
}

// readDevLoopHandshake returns the handshake for a project, or an error a user
// can act on.
//
// A handshake whose process is gone is refused rather than used: the ports in it
// may since have been taken by something else, and sending a configuration
// change to the wrong process is worse than saying no dev loop is running.
func readDevLoopHandshake(projectPath string) (devLoopHandshake, error) {
	var h devLoopHandshake
	path := devLoopHandshakePath(projectPath)
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return h, fmt.Errorf("no 'mxcli run --local' is serving this project\n" +
			"  start one in another terminal, or drop --apply to only record the value")
	}
	if err != nil {
		return h, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := json.Unmarshal(body, &h); err != nil {
		return h, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	if !procalive.Alive(h.PID) {
		return h, fmt.Errorf("%s refers to process %d, which is no longer running\n"+
			"  (the dev loop was stopped without cleaning up; start a new one)", path, h.PID)
	}
	return h, nil
}
