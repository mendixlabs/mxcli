// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mendixlabs/mxcli/internal/procalive"
)

// HandshakeFile is where a `run --local --test-endpoint` session publishes what
// `mxcli test --attach` needs to reach it. It lives beside the project rather
// than in a shared temp directory so two projects cannot collide.
const handshakeName = "test-endpoint.json"

// Handshake is the contract between a dev loop hosting the test endpoint and a
// test run attaching to it.
//
// It carries a live credential, so it is written 0600 and removed when the dev
// loop exits. It is not a secret store: the token it holds only works against a
// loopback endpoint on this machine, and only until that runtime stops.
type Handshake struct {
	// Project is the .mpr the dev loop is serving, so an attach can refuse a
	// handshake left behind by a different project.
	Project string `json:"project"`
	// PID of the hosting `mxcli run --local` process, used to detect a stale file.
	PID int `json:"pid"`
	// AppPort is where the test endpoint is reachable.
	AppPort int `json:"appPort"`
	// AdminPort is the M2EE admin API, used to reload the model after injecting
	// test microflows.
	AdminPort int `json:"adminPort"`
	// AdminPass authenticates against that admin API. It is NOT the endpoint
	// token: the two are different secrets, and using one for the other fails
	// with "Authentication failed" at the first reload.
	AdminPass string `json:"adminPass"`
	// ServePort is the mxbuild serve API, used to rebuild after injecting.
	ServePort int `json:"servePort"`
	// Token authenticates against the endpoint.
	Token string `json:"token"`
	// Started is when the dev loop published this, for a clearer stale message.
	Started time.Time `json:"started"`
}

// HandshakePath is the handshake file's location for a project.
func HandshakePath(projectPath string) string {
	return filepath.Join(filepath.Dir(projectPath), ".mxcli", handshakeName)
}

// WriteHandshake publishes the handshake, replacing any existing one.
//
// Written via a temp file and renamed so an attach can never read a
// half-written file, and created 0600 because it carries the endpoint token.
func WriteHandshake(projectPath string, h Handshake) error {
	path := HandshakePath(projectPath)
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

// RemoveHandshake deletes the handshake. Safe when it is not there.
func RemoveHandshake(projectPath string) {
	os.Remove(HandshakePath(projectPath))
}

// ReadHandshake loads the handshake for a project and rejects a stale one.
//
// Staleness matters more than it looks: a dev loop killed with SIGKILL leaves
// the file behind, and attaching to a dead runtime would fail with a confusing
// connection error several steps later. Checking the recorded PID turns that
// into one clear message at the start.
func ReadHandshake(projectPath string) (*Handshake, error) {
	path := HandshakePath(projectPath)
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no test endpoint is being hosted for this project\n"+
				"  --attach needs an app already running with the endpoint. Start one with:\n"+
				"      mxcli run --local --test-endpoint -p %s\n"+
				"  (expected the handshake at %s)", projectPath, path)
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var h Handshake
	if err := json.Unmarshal(body, &h); err != nil {
		return nil, fmt.Errorf("%s is not readable as a handshake: %w", path, err)
	}
	if h.Token == "" || h.AppPort == 0 {
		return nil, fmt.Errorf("%s is incomplete; stop and restart the hosting 'mxcli run --local --test-endpoint'", path)
	}
	if !procalive.Alive(h.PID) {
		return nil, fmt.Errorf("the app that published %s (pid %d, started %s) is no longer running\n"+
			"  Start one with: mxcli run --local --test-endpoint -p %s",
			path, h.PID, h.Started.Format(time.RFC3339), projectPath)
	}
	return &h, nil
}

// nowFunc is time.Now, indirected so a test can pin the timestamp.
var nowFunc = time.Now
