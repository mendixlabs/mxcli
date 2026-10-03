// SPDX-License-Identifier: Apache-2.0

//go:build windows

package docker

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"github.com/mendixlabs/mxcli/internal/procalive"
)

// procgroup_windows.go provides the Windows implementations of the process
// helpers. Windows has no POSIX process groups, so the "group" operations are
// implemented by terminating the whole process tree with taskkill instead.

// setProcessGroup is a no-op on Windows: there is no PGID to set. Tree teardown
// is handled by signalProcessGroup / killProcessGroup below.
func setProcessGroup(cmd *exec.Cmd) {}

// signalProcessGroup terminates the process tree led by p.
//
// Windows has no signal semantics — os.Process.Signal only supports Kill and
// returns EWINDOWS for everything else — so a "graceful signal" cannot be
// delivered and the process would otherwise linger until the caller's grace
// period expired. Terminating the tree here keeps shutdown prompt.
func signalProcessGroup(p *os.Process, sig syscall.Signal) error {
	if p == nil {
		return nil
	}
	if err := killProcessTree(p.Pid); err == nil {
		return nil
	}
	return p.Signal(sig)
}

// killProcessGroup force-terminates the process tree led by p.
//
// The tree, not just p: mxbuild.exe is a wrapper that launches a Deno web-ext
// worker (modeler/tools/deno/win-x64/deno.exe) which inherits the stdout/stderr
// pipe this package hands to exec.Cmd. Killing only the wrapper leaves that
// worker alive holding the pipe, so cmd.Wait() never sees EOF and blocks
// forever — which is why `mxcli run --local` used to hang at "Starting mxbuild
// --serve..." with no error. taskkill /T reaps the grandchild too.
//
// The direct p.Kill() is only a fallback for when taskkill is unavailable: once
// the tree kill has terminated p, calling Kill on it again returns "invalid
// argument", which must not be reported as a teardown failure.
func killProcessGroup(p *os.Process) error {
	if p == nil {
		return nil
	}
	if err := killProcessTree(p.Pid); err == nil {
		return nil
	}
	return p.Kill()
}

// killProcessTree force-terminates pid and all of its descendants, returning the
// taskkill error (or nil when pid <= 0, which is nothing to do).
func killProcessTree(pid int) error {
	if pid <= 0 {
		return nil
	}
	cmd := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}

// processAlive reports whether p is still running.
//
// os.Process.Signal(syscall.Signal(0)) is NOT a liveness test on Windows: Go
// returns EWINDOWS ("not supported by windows") for every signal except Kill, so
// it reports a live process as dead. waitReady() relied on it, concluded mxbuild
// had exited the instant it was launched, and the local loop never got past
// "Starting mxbuild --serve...". WaitForSingleObject on a SYNCHRONIZE handle is
// the real check: it returns WAIT_TIMEOUT while the process runs and
// WAIT_OBJECT_0 once it has terminated (even before it is reaped). The check
// itself now lives in procalive.Alive, shared with the handshake readers.
func processAlive(p *os.Process) bool {
	if p == nil {
		return false
	}
	return procalive.Alive(p.Pid)
}
