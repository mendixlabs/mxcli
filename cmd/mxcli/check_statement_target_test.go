// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#1821: `check --references` resolved the target document of alter
// entity and alter page only. drop, rename and move of a document that does not
// exist — and alter enumeration / alter microflow — passed check and were then
// refused by exec. Every form is checked against PedApp: a missing target is
// reported, and the same statement on a target the project has is not.
func TestCheck_StatementTargetMustExist(t *testing.T) {
	src := filepath.Join("..", "..", "testdata", "pedapp")
	if _, err := os.Stat(filepath.Join(src, "PedApp.mpr")); err != nil {
		t.Skipf("PedApp fixture not found: %v", err)
	}
	dir := t.TempDir()
	if err := copyTree(src, dir); err != nil {
		t.Fatal(err)
	}
	_ = checkCmd.InheritedFlags()
	_ = rootCmd.PersistentFlags().Set("project", filepath.Join(dir, "PedApp.mpr"))
	defer func() {
		_ = rootCmd.PersistentFlags().Set("project", "")
		rootCmd.PersistentFlags().Lookup("project").Changed = false
	}()
	if f := checkCmd.Flags().Lookup("format"); f != nil {
		_ = f.Value.Set(f.DefValue)
	}
	scripts := t.TempDir()
	run := func(name, body string) (int, string) {
		path := writeScript(t, scripts, name+".mdl", "mdl 1;\n"+body+"\n")
		var code int
		out := captureStd(t, func() { code = runCheckFiles(checkCmd, []string{path}) })
		return code, out
	}

	cases := []struct {
		name, missing, want, existing string
	}{
		{"drop microflow", "drop microflow MyFirstModule.NoSuchFlow;", "microflow not found: MyFirstModule.NoSuchFlow",
			"drop microflow MyFirstModule.MyFirstLogic;"},
		{"drop nanoflow", "drop nanoflow Atlas_Web_Content.NoSuchNano;", "nanoflow not found: Atlas_Web_Content.NoSuchNano",
			"drop nanoflow Atlas_Web_Content.ACT_Login;"},
		{"drop page", "drop page MyFirstModule.NoSuchPage;", "page not found: MyFirstModule.NoSuchPage",
			"drop page MyFirstModule.Home_Web;"},
		{"drop snippet", "drop snippet Administration.NoSuchSnippet;", "snippet not found: Administration.NoSuchSnippet",
			"drop snippet Administration.ReadMe;"},
		{"drop entity", "drop entity Administration.NoSuchEntity;", "entity not found: Administration.NoSuchEntity",
			"drop entity Administration.AccountPasswordData;"},
		{"drop enumeration", "drop enumeration FeedbackModule.NoSuchEnum;", "enumeration not found: FeedbackModule.NoSuchEnum",
			"drop enumeration FeedbackModule.LogNodes;"},
		{"drop association", "drop association Administration.NoSuchAssoc;", "association not found: Administration.NoSuchAssoc",
			"drop association Administration.AccountPasswordData_Account;"},
		{"drop constant", "drop constant FeedbackModule.NoSuchConst;", "constant not found: FeedbackModule.NoSuchConst",
			"drop constant FeedbackModule.LocalStorageKey;"},
		{"rename microflow", "rename microflow MyFirstModule.NoSuchFlow to Other;", "microflow not found: MyFirstModule.NoSuchFlow",
			"rename microflow MyFirstModule.MyFirstLogic to MyRenamedLogic;"},
		{"move microflow", "move microflow MyFirstModule.NoSuchFlow to folder 'X';", "microflow not found: MyFirstModule.NoSuchFlow",
			"move microflow MyFirstModule.MyFirstLogic to folder 'X';"},
		{"alter enumeration", "alter enumeration FeedbackModule.NoSuchEnum add value V caption 'V';", "enumeration not found: FeedbackModule.NoSuchEnum",
			"alter enumeration FeedbackModule.LogNodes add value if not exists V1821 caption 'V';"},
		{"alter microflow", "alter microflow MyFirstModule.NoSuchFlow { drop $X; };", "microflow not found: MyFirstModule.NoSuchFlow",
			""},
		{"alter nanoflow", "alter nanoflow Atlas_Web_Content.NoSuchNano { drop $X; };", "nanoflow not found: Atlas_Web_Content.NoSuchNano",
			""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out := run(strings.ReplaceAll(tc.name, " ", "_"), tc.missing)
			if code == 0 || !strings.Contains(out, tc.want) {
				t.Errorf("a missing target passed check (exit %d), want %q:\n%s", code, tc.want, out)
			}
			if tc.existing == "" {
				return
			}
			// Control: the same form on a target the project has is not "not found".
			_, out = run(strings.ReplaceAll(tc.name, " ", "_")+"_ok", tc.existing)
			if strings.Contains(out, "not found") {
				t.Errorf("control: an existing target was reported:\n%s", out)
			}
		})
	}

	// Controls the check must not break: drop … if exists, a document the
	// script creates first, and one an earlier rename lands under the name.
	for name, body := range map[string]string{
		"if exists": "drop microflow if exists MyFirstModule.NoSuchFlow;",
		"created in the script": `create microflow MyFirstModule.Temp1821 () begin end;
drop microflow MyFirstModule.Temp1821;`,
		"renamed earlier in the script": `rename microflow MyFirstModule.MyFirstLogic to Renamed1821;
drop microflow MyFirstModule.Renamed1821;`,
		"moved earlier in the script": `move page MyFirstModule.Home_Web to Administration;
drop page Administration.Home_Web;`,
	} {
		t.Run("control: "+name, func(t *testing.T) {
			_, out := run("control_"+strings.ReplaceAll(name, " ", "_"), body)
			if strings.Contains(out, "not found") {
				t.Errorf("reported a target the script provides:\n%s", out)
			}
		})
	}
}
