// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// secretFlagCmd is a command with the secret run flags, parsed from args.
func secretFlagCmd(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "run"}
	for flag := range secretRunFlags {
		cmd.Flags().String(strings.TrimPrefix(flag, "--"), "", "")
	}
	if err := cmd.Flags().Parse(args); err != nil {
		t.Fatal(err)
	}
	return cmd
}

// Every secret flag the redaction knows is a string flag of `run`, so none is
// redacted without also being resolved (and the reverse).
func TestSecretRunFlagsAreRunStringFlags(t *testing.T) {
	for flag := range secretRunFlags {
		f := runCmd.Flags().Lookup(strings.TrimPrefix(flag, "--"))
		if f == nil || f.Value.Type() != "string" {
			t.Errorf("%s is not a string flag of run", flag)
			continue
		}
		if !strings.Contains(f.Usage, "@env:VAR") {
			t.Errorf("%s does not document @env:/@file:", flag)
		}
	}
}

func TestResolveRunSecrets(t *testing.T) {
	t.Setenv("MXCLI_TEST_DBPW", "pw-from-env")
	file := filepath.Join(t.TempDir(), "hub")
	if err := os.WriteFile(file, []byte("u:p\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := resolveRunSecrets(secretFlagCmd(t,
		"--db-password", "@env:MXCLI_TEST_DBPW",
		"--hub-secret=@file:"+file,
		"--debug-pass", "literal",
		"--screenshot-password", "@@at"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"db-password": "pw-from-env", "hub-secret": "u:p", "debug-pass": "literal",
		"screenshot-password": "@at", "db-admin-password": ""}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("resolveRunSecrets = %v, want %v", got, want)
	}

	os.Unsetenv("MXCLI_TEST_UNSET_PW")
	_, err = resolveRunSecrets(secretFlagCmd(t, "--db-password", "@env:MXCLI_TEST_UNSET_PW"))
	if err == nil || !strings.Contains(err.Error(), "--db-password") || !strings.Contains(err.Error(), "MXCLI_TEST_UNSET_PW") {
		t.Errorf("unset variable: err = %v, want a refusal naming the flag and the variable", err)
	}
	if _, err := resolveRunSecrets(secretFlagCmd(t, "--db-admin-password", "@dmin")); err == nil {
		t.Error("a value starting with a bare @ was accepted")
	}
}

// A reference is stored as it is, so restart can resolve it again; a literal is
// redacted, so restart refuses.
func TestRedactArgs_KeepsSecretReferences(t *testing.T) {
	in := []string{"run", "--local", "--db-password", "@env:PW", "--hub-secret=@file:/s/hub",
		"--debug-pass", "plain", "--screenshot-password=plain2"}
	got, changed := redactArgs(in)
	want := []string{"run", "--local", "--db-password", "@env:PW", "--hub-secret=@file:/s/hub",
		"--debug-pass", redactedValue, "--screenshot-password=" + redactedValue}
	if !changed || !reflect.DeepEqual(got, want) {
		t.Errorf("redactArgs = %q (%v), want %q", got, changed, want)
	}
	refsOnly, changed := redactArgs([]string{"run", "--db-password", "@env:PW", "--db-admin-password=@file:/s/a"})
	if changed || hasRedactedArgs(refsOnly) {
		t.Errorf("references alone were redacted: %q", refsOnly)
	}
}

func TestDetachArgs_SecretFileReferenceMadeAbsolute(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	got := detachArgs([]string{"run", "--local", "--db-password", "@file:pw", "--hub-secret=@file:sub/hub",
		"--debug-pass", "@file:/abs/x", "--db-user", "@file:notasecret"}, "/abs/app.mpr")
	want := []string{"run", "--local", "--db-password", "@file:" + filepath.Join(dir, "pw"),
		"--hub-secret=@file:" + filepath.Join(dir, "sub", "hub"), "--debug-pass", "@file:/abs/x",
		"--db-user", "@file:notasecret", "-p", "/abs/app.mpr"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("detachArgs = %q\nwant %q", got, want)
	}
}

// A flag's value that looks like -p is the value, not the project flag
// (mendixlabs/mxcli#1761).
func TestDetachArgs_KeepsAFlagValueThatLooksLikeAFlag(t *testing.T) {
	got := detachArgs([]string{"run", "--local", "--db-password", "-pX", "--db-name", "--project", "--watch", "-p", "app.mpr"}, "/abs/app.mpr")
	want := []string{"run", "--local", "--db-password", "-pX", "--db-name", "--project", "--watch", "-p", "/abs/app.mpr"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("detachArgs = %q\nwant %q", got, want)
	}
}

// The refusal names a way that works: the runtime's database password does not
// read PGPASSWORD, so suggesting it gave a run with the default password.
func TestRestartSecretRefusal_SuggestsAReference(t *testing.T) {
	msg := restartSecretRefusal("/abs/app.mpr")
	if strings.Contains(msg, "PGPASSWORD") {
		t.Errorf("the refusal still suggests PGPASSWORD:\n%s", msg)
	}
	if !strings.Contains(msg, "@env:") || !strings.Contains(msg, "mxcli run stop -p /abs/app.mpr") {
		t.Errorf("the refusal does not say how to restart with a reference:\n%s", msg)
	}
}
