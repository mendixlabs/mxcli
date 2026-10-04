// SPDX-License-Identifier: Apache-2.0

//go:build windows

package docker

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// errnoPrivilegeNotHeld is ERROR_PRIVILEGE_NOT_HELD: what CreateSymbolicLink
// reports when the caller lacks SeCreateSymbolicLinkPrivilege.
const errnoPrivilegeNotHeld = syscall.Errno(1314)

// linkDir makes link a directory link to target.
//
// An ordinary Windows user may create a symlink only with Developer Mode on or
// an elevated token (SeCreateSymbolicLinkPrivilege); without either,
// os.Symlink fails with "A required privilege is not held by the client", which
// stopped the first `run --local` / `test --local` of every new Mendix version
// (mendixlabs/mxcli#1286). A directory junction needs no privilege and serves
// the same purpose here, so on that one error linkDir falls back to a junction.
//
// The junction is made with `cmd /c mklink /J`, the same shell-out style as
// killProcessTree, rather than FSCTL_SET_REPARSE_POINT through golang.org/x/sys,
// which is only an indirect dependency of this module and would become a direct
// one. Go reports a junction as os.ModeIrregular, not os.ModeSymlink, so
// os.RemoveAll deletes the junction itself and never follows it into target.
//
// A junction must name an absolute local path, so target is made absolute
// (the callers already pass absolute cache paths). Any other symlink failure,
// such as the link already existing, is returned unchanged.
func linkDir(target, link string) error {
	symErr := os.Symlink(target, link)
	if symErr == nil || !errors.Is(symErr, errnoPrivilegeNotHeld) {
		return symErr
	}

	absTarget, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("%w (junction fallback: resolving target: %v)", symErr, err)
	}
	cmd := exec.Command("cmd", "/c", "mklink", "/J", link, absTarget)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w; junction fallback `mklink /J` failed: %v: %s",
			symErr, err, strings.TrimSpace(string(out)))
	}
	return nil
}
