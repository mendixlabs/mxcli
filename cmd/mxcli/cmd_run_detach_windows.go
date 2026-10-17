// SPDX-License-Identifier: Apache-2.0

//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

const (
	createNewProcessGroup = 0x00000200
	createNoWindow        = 0x08000000
)

// setDetachAttrs starts the background run with a console of its own that has
// no window, in a process group of its own, so closing the calling console does
// not end it.
//
// The run must have a console. With DETACHED_PROCESS it has none, and Windows
// then gives every console program it starts (java, mxbuild, gradle) a new
// console with a visible window (mendixlabs/mxcli#1484). CREATE_NO_WINDOW gives
// the run one hidden console that those children inherit, so none appears.
// Windows ignores CREATE_NO_WINDOW when it is combined with DETACHED_PROCESS,
// so the two must not be set together.
func setDetachAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup | createNoWindow, HideWindow: true}
}

// terminateProcess ends the run and its whole process tree. Windows has no
// SIGTERM for a console-less process, so this is not a graceful stop: the
// run's own teardown (removing --test-endpoint) does not run.
func terminateProcess(pid int) error {
	return exec.Command("taskkill", "/T", "/F", "/PID", fmt.Sprint(pid)).Run()
}

// killProcess force-kills a process.
func killProcess(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}
