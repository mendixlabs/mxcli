// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"io"
	"net"
	"path/filepath"
)

// AttachedServe builds a project through the mxbuild --serve of a dev loop this
// process does not own — `mxcli test --attach` against `run --local
// --test-endpoint` — with the recovery the dev loop's own builds get.
//
// The attach's builds are never a serve process's first, and on the
// prebuilt-client Mendix versions only the first is complete: any later one
// leaves web/dist without its page modules, and the running app answers 404 on
// every page (mendixlabs/mxcli#1580). The dev loop recovers by restarting its
// serve process. The attach must not: that process belongs to the other mxcli
// and is the one its next build will use. So the attach's "restart" starts a
// serve process of its own on a free port, builds there, and stops it on Close
// (ako/mxcli#1917, mendixlabs/mxcli#1723).
type AttachedServe struct {
	rec  *serveRecovery
	proc *borrowedServe
}

// NewAttachedServe builds through host, falling back to a private serve process
// for projectPath when a build on host leaves the client bundle incomplete.
func NewAttachedServe(host *ServeServer, projectPath string, out io.Writer) *AttachedServe {
	proc := &borrowedServe{
		host:  host,
		start: func() (ownedServe, error) { return startPrivateServe(projectPath, out) },
		out:   out,
	}
	return &AttachedServe{
		rec:  &serveRecovery{serve: proc, deployDir: filepath.Join(filepath.Dir(projectPath), "deployment"), out: out},
		proc: proc,
	}
}

// Build runs one build, recovering as the dev loop's own builds do.
func (a *AttachedServe) Build(req BuildRequest) (*BuildResult, error) { return a.rec.Build(req) }

// Close stops the private serve process, if one was started. The host's is
// never touched.
func (a *AttachedServe) Close() {
	if a.proc.own != nil {
		_ = a.proc.own.Stop()
		a.proc.own = nil
	}
}

// ownedServe is the part of *ServeServer a private serve process needs — a seam
// so the borrowing is testable without an mxbuild.
type ownedServe interface {
	serveProcess
	Stop() error
}

// borrowedServe is the serveProcess under AttachedServe's recovery: builds go to
// the host until a restart is asked for, and from then on to a process of its
// own, which a later restart replaces.
type borrowedServe struct {
	host  serveProcess
	own   ownedServe
	start func() (ownedServe, error)
	out   io.Writer
}

func (b *borrowedServe) Build(req BuildRequest) (*BuildResult, error) {
	if b.own != nil {
		return b.own.Build(req)
	}
	return b.host.Build(req)
}

func (b *borrowedServe) Restart() error {
	if b.own != nil {
		return b.own.Restart()
	}
	fmt.Fprintln(b.out, "  starting a separate mxbuild --serve for this run (the app's own is left as it is)...")
	own, err := b.start()
	if err != nil {
		return err
	}
	b.own = own
	return nil
}

// startPrivateServe starts mxbuild --serve for projectPath on a free loopback
// port, resolved the way `run --local` resolves its own.
func startPrivateServe(projectPath string, out io.Writer) (ownedServe, error) {
	reader, err := openReadOnly(projectPath)
	if err != nil {
		return nil, fmt.Errorf("reading the project version: %w", err)
	}
	pv := reader.ProjectVersion()
	reader.Disconnect()
	if pv == nil {
		return nil, fmt.Errorf("the project has no version")
	}
	mxbuildPath, err := ResolveMxBuildForLocal("", pv.ProductVersion, out)
	if err != nil {
		return nil, err
	}
	port, err := freeLoopbackPort()
	if err != nil {
		return nil, err
	}
	javaMajor, _ := ProjectJavaMajor(projectPath)
	return StartServe(serveOptionsFor(mxbuildPath, pv.ProductVersion, javaMajor, port))
}

func freeLoopbackPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("finding a free port for mxbuild --serve: %w", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
