// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"os"
	"testing"
)

func TestSetHomeRedirectsUserHomeDir(t *testing.T) {
	dir := t.TempDir()
	SetHome(t, dir)
	got, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	if got != dir {
		t.Errorf("os.UserHomeDir() = %q, want %q", got, dir)
	}
}
