// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#2033: while `run --test-endpoint` hosts the endpoint,
// javasource/mxtest/actions/RegisterTestEndpoint.java is in the working tree,
// and a `git add -A` in that window committed it. The folder is git-ignored
// from the moment the endpoint is installed — creating the project's
// .gitignore when it has none — and by the .gitignore `mxcli init` writes.

func TestTestEndpointIgnored_CreatesGitignore(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if err := ensureTestEndpointIgnored(mpr); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("no .gitignore created: %v", err)
	}
	for _, want := range []string{"/javasource/mxtest/", ".mxcli/", "/deployment/"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("created .gitignore lacks %q:\n%s", want, body)
		}
	}
}

func TestTestEndpointIgnored_AppendsOnce(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	gi := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(gi, []byte("/deployment/\n# mine\n*.log"), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := ensureTestEndpointIgnored(mpr); err != nil {
			t.Fatal(err)
		}
	}
	body, _ := os.ReadFile(gi)
	s := string(body)
	if !strings.HasPrefix(s, "/deployment/\n# mine\n*.log\n") {
		t.Errorf("the project's own lines were not kept as they were:\n%s", s)
	}
	if n := strings.Count(s, "/javasource/mxtest/"); n != 1 {
		t.Errorf("/javasource/mxtest/ appears %d times, want 1:\n%s", n, s)
	}
	if strings.Count(s, ".mxcli/") != 1 {
		t.Errorf(".mxcli/ (the handshake and its token) not ensured once:\n%s", s)
	}
}

// A project that already ignores the folder in its own spelling is left alone.
func TestTestEndpointIgnored_KeepsAnExistingEntry(t *testing.T) {
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	orig := "javasource/mxtest/\n.mxcli/\n"
	if err := os.WriteFile(gi, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureTestEndpointIgnored(filepath.Join(dir, "App.mpr")); err != nil {
		t.Fatal(err)
	}
	if body, _ := os.ReadFile(gi); string(body) != orig {
		t.Errorf(".gitignore changed:\n%s", body)
	}
}

func TestInitGitignoreTemplateIgnoresTestEndpoint(t *testing.T) {
	if !strings.Contains(mendixGitignore, "/javasource/mxtest/") {
		t.Error("the .gitignore mxcli init writes does not ignore /javasource/mxtest/")
	}
}
