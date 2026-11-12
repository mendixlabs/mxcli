// SPDX-License-Identifier: Apache-2.0

//go:build windows

package docker

import (
	"os"
	"path/filepath"
	"testing"
)

// A copy in progress must not stop mxbuild deleting the file (ako/mxcli#2150).
// Control: the same delete fails while the file is open through os.Open.
func TestOpenClientSource_DoesNotBlockDelete(t *testing.T) {
	dir := t.TempDir()
	shared := filepath.Join(dir, "shared.js")
	plain := filepath.Join(dir, "plain.js")
	for _, p := range []string{shared, plain} {
		if err := os.WriteFile(p, []byte("//"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	f, err := openClientSource(shared)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := os.Remove(shared); err != nil {
		t.Fatalf("deleting a file openClientSource holds open must succeed: %v", err)
	}

	g, err := os.Open(plain)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	if err := os.Remove(plain); err == nil {
		t.Fatal("control: deleting a file held through os.Open was expected to fail on Windows")
	}
}
