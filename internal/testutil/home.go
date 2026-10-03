// SPDX-License-Identifier: Apache-2.0

// Package testutil holds helpers shared by tests across the module.
//
// The one that matters most is SetHome. os.UserHomeDir reads USERPROFILE on
// Windows and HOME everywhere else, so a test that isolates the home directory
// with t.Setenv("HOME", dir) only does so on Unix. On Windows it silently reads
// and writes the developer's real %USERPROFILE%\.mxcli: a fixture PAT in
// auth.json, fake mxbuild/runtime caches, the marketplace catalog cache
// (mendixlabs/mxcli#1256). TestNoBareHomeSetenv fails the build for any test
// file that does it.
package testutil

import "testing"

// SetHome points os.UserHomeDir at dir for the duration of the test, on every
// OS. It sets both HOME and USERPROFILE rather than switching on runtime.GOOS:
// setting the variable an OS ignores is harmless, and there is no branch to
// get wrong.
func SetHome(t testing.TB, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}
