// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// webclient_watch.go runs mxbuild's rollup client bundler as a long-lived
// incremental watcher — the client-side mirror of `mxbuild --serve`. The runner
// (rollup-runner.mjs, non-production branch) keeps rollup's module graph loaded
// and rebuilds only what changed, so after the serve Deploy target rewrites the
// web client source, re-bundling web/dist is ~3-4s instead of a ~7s cold build.
//
// The runner emits status lines on stdout using the modern-web-bundler protocol:
//
//	{"protocol":"mx-modern-web-bundler","type":"status","payload":{"kind":"start"}}
//	{"protocol":"mx-modern-web-bundler","type":"status","payload":{"kind":"success"}}
//	{"protocol":"mx-modern-web-bundler","type":"status","payload":{"kind":"error","error":{"message":"..."}}}
//
// WebClientWatcher parses those, counting successful bundles as a generation so
// callers can wait for the rebuild triggered by a given source change.
//
// Container filesystems: rollup's chokidar file watcher relies on inotify, which
// does not fire on overlay/bind-mount filesystems (devcontainers). Without a fix
// change detection takes tens of seconds; CHOKIDAR_USEPOLLING makes it ~1s.

const bundlerProtocolName = "mx-modern-web-bundler"

// bundlerStatus is a parsed protocol status line.
type bundlerStatus struct {
	kind    string // "start" | "success" | "error"
	message string // populated for kind == "error"
}

// parseBundlerStatus extracts a status from one runner stdout line. ok is false
// for non-protocol lines (plain logging), which are ignored.
func parseBundlerStatus(line string) (bundlerStatus, bool) {
	var m struct {
		Protocol string `json:"protocol"`
		Type     string `json:"type"`
		Payload  struct {
			Kind  string `json:"kind"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		return bundlerStatus{}, false
	}
	if m.Protocol != bundlerProtocolName || m.Type != "status" || m.Payload.Kind == "" {
		return bundlerStatus{}, false
	}
	s := bundlerStatus{kind: m.Payload.Kind}
	if m.Payload.Error != nil {
		s.message = m.Payload.Error.Message
	}
	return s, true
}

// WebClientWatcher is a running incremental rollup bundler for a deployment's
// web/ directory.
type WebClientWatcher struct {
	cmd *exec.Cmd
	log *syncBuffer

	mu       sync.Mutex
	gen      int           // number of successful bundles
	building bool          // a bundle is currently in progress
	lastErr  string        // message of the most recent failed bundle
	exited   bool          // the runner process has exited
	updated  chan struct{} // closed+replaced on every state change (broadcast)
}

// applyStatus folds a status line into the watcher state and broadcasts.
func (wc *WebClientWatcher) applyStatus(s bundlerStatus) {
	wc.mu.Lock()
	switch s.kind {
	case "start":
		wc.building = true
		wc.lastErr = ""
	case "success":
		wc.building = false
		wc.gen++
	case "error":
		wc.building = false
		wc.lastErr = s.message
		if wc.lastErr == "" {
			wc.lastErr = "unknown bundler error"
		}
	}
	wc.broadcastLocked()
	wc.mu.Unlock()
}

// broadcastLocked wakes all waiters. Caller holds wc.mu.
func (wc *WebClientWatcher) broadcastLocked() {
	close(wc.updated)
	wc.updated = make(chan struct{})
}

func (wc *WebClientWatcher) snapshot() (gen int, building, exited bool, lastErr string, ch chan struct{}) {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	return wc.gen, wc.building, wc.exited, wc.lastErr, wc.updated
}

// Generation returns the number of successful bundles so far. Capture it before
// triggering a source change, then pass it to WaitForBundle.
func (wc *WebClientWatcher) Generation() int {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	return wc.gen
}

// StartWebClientWatch launches the incremental bundler and blocks until the first
// bundle completes (so web/dist exists before the app boots).
func StartWebClientWatch(opts WebClientOptions) (*WebClientWatcher, error) {
	webDir := filepath.Join(opts.DeployDir, "web")
	if fi, err := os.Stat(filepath.Join(webDir, "rollup.config.mjs")); err != nil || fi.IsDir() {
		return nil, fmt.Errorf("no rollup.config.mjs in %s (run a serve Deploy build first)", webDir)
	}
	nodeBin, runner, err := resolveNodeTooling(opts.MxBuildPath)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(nodeBin, runner)
	cmd.Dir = webDir
	// NODE_ENV must be a defined string (the config's esbuild plugin injects it as
	// a `define`), but NOT "production" — that would take the runner's one-shot
	// branch instead of watch(). "development" gives incremental watch mode.
	// CHOKIDAR_USEPOLLING makes file-change detection work on container overlay
	// filesystems where inotify is silent (otherwise detection takes ~tens of s).
	cmd.Env = append(os.Environ(),
		"NODE_ENV=development",
		"CHOKIDAR_USEPOLLING=true",
		"CHOKIDAR_INTERVAL=300",
		"MX_WEB_CLIENT_BUILD_LOG="+filepath.Join(opts.DeployDir, "log", "web-client-build.log"),
	)
	log := &syncBuffer{}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("web client watcher stdout pipe: %w", err)
	}
	cmd.Stderr = log

	wc := &WebClientWatcher{cmd: cmd, log: log, updated: make(chan struct{})}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("launching web client watcher: %w", err)
	}
	go wc.readLoop(stdout)
	go func() {
		_ = cmd.Wait()
		wc.mu.Lock()
		wc.exited = true
		wc.broadcastLocked()
		wc.mu.Unlock()
	}()

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	if err := wc.waitForFirstBuild(timeout); err != nil {
		_ = wc.Stop()
		return nil, err
	}
	dist := filepath.Join(webDir, "dist", "index.js")
	if _, err := os.Stat(dist); err != nil {
		_ = wc.Stop()
		return nil, fmt.Errorf("web client watcher reported success but %s is missing:\n%s", dist, wc.log.String())
	}
	return wc, nil
}

// readLoop parses the runner's stdout and folds each status into state. It also
// tees the raw lines into the log buffer for diagnostics.
func (wc *WebClientWatcher) readLoop(stdout io.Reader) {
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		_, _ = wc.log.Write([]byte(line + "\n"))
		if s, ok := parseBundlerStatus(line); ok {
			wc.applyStatus(s)
		}
	}
}

// waitForFirstBuild blocks until the first successful bundle, or an error/exit.
func (wc *WebClientWatcher) waitForFirstBuild(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		gen, _, exited, lastErr, ch := wc.snapshot()
		if gen >= 1 {
			return nil
		}
		if lastErr != "" {
			return fmt.Errorf("initial web client build failed: %s", lastErr)
		}
		if exited {
			return fmt.Errorf("web client watcher exited before the first build:\n%s", wc.log.String())
		}
		wait := time.Until(deadline)
		if wait <= 0 {
			return fmt.Errorf("web client watcher timed out after %s", timeout)
		}
		select {
		case <-ch:
		case <-time.After(wait):
		}
	}
}

// WaitForRebuild waits for the incremental bundle triggered by a source change.
// It returns rebuilt=true once a successful bundle beyond sinceGen lands. If no
// bundle starts within settle, it returns rebuilt=false — the change didn't
// affect rollup's module graph (e.g. it touched a web/ file that isn't a bundle
// input), so there is nothing to wait for. This never hangs: settle bounds the
// "did a rebuild even start" wait, and buildTimeout bounds one that has.
//
// Reliable because file-change detection is fast (CHOKIDAR_USEPOLLING ~1s), so a
// rebuild that is going to happen starts well within a few-second settle window.
func (wc *WebClientWatcher) WaitForRebuild(sinceGen int, settle, buildTimeout time.Duration) (bool, error) {
	settleDeadline := time.Now().Add(settle)
	sawBuild := false
	for {
		gen, building, exited, lastErr, ch := wc.snapshot()
		if gen > sinceGen {
			return true, nil
		}
		if building {
			sawBuild = true
		}
		if sawBuild && !building && lastErr != "" {
			return false, fmt.Errorf("web client build failed: %s", lastErr)
		}
		if exited {
			return false, fmt.Errorf("web client watcher exited:\n%s", wc.log.String())
		}
		var wait time.Duration
		if sawBuild || building {
			wait = time.Until(settleDeadline.Add(buildTimeout))
		} else {
			wait = time.Until(settleDeadline)
		}
		if wait <= 0 {
			return false, nil // no rebuild materialized
		}
		select {
		case <-ch:
		case <-time.After(wait):
			if !sawBuild && !building {
				return false, nil
			}
		}
	}
}

// Log returns the captured watcher output.
func (wc *WebClientWatcher) Log() string { return wc.log.String() }

// Stop terminates the watcher process.
func (wc *WebClientWatcher) Stop() error {
	if wc.cmd == nil || wc.cmd.Process == nil {
		return nil
	}
	_ = wc.cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- wc.cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = wc.cmd.Process.Kill()
		<-done
	}
	return nil
}
