// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The test run removes MxTest and its Java, but the registrar mxbuild
// generated while MxTest existed still registers its action: a `run --local
// --watch` dev loop's next warm build then failed to compile
// ("package mxtest.actions does not exist") and served a blank page until a
// restart (ako/mxcli#2639, upstream mendixlabs/mxcli#1972).
func TestRemoveGeneratedJavaSourceScrubsTheRegistrar_2639(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	sys := filepath.Join(dir, "javasource", "system")
	if err := os.MkdirAll(sys, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "javasource", "mxtest", "actions"), 0o755); err != nil {
		t.Fatal(err)
	}
	registrar := filepath.Join(sys, "UserActionsRegistrar.java")
	src := `package system;

public class UserActionsRegistrar
{
  public void registerActions(IActionRegistrator registrator)
  {
    registrator.registerUserAction(myfirstmodule.actions.UseHelper.class);
    registrator.registerUserAction(mxtest.actions.RegisterTestEndpoint.class);
    registrator.registerUserAction(system.actions.VerifyPassword.class);
  }
}
`
	if err := os.WriteFile(registrar, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	removeGeneratedJavaSource(mpr, io.Discard)

	got, err := os.ReadFile(registrar)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "mxtest.") {
		t.Errorf("the registrar still registers MxTest's action:\n%s", got)
	}
	// Control: every other registration stays, byte for byte.
	if string(got) != strings.Replace(src, "    registrator.registerUserAction(mxtest.actions.RegisterTestEndpoint.class);\n", "", 1) {
		t.Errorf("other lines changed:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "javasource", "mxtest")); !os.IsNotExist(err) {
		t.Error("javasource/mxtest is still there")
	}
}
