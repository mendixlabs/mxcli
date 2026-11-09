// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Both messages below are real, captured from mxbuild 11.14.0 driven over its
// own HTTP API with no mxcli involved. Which one a build produces depends only
// on how far it gets before it needs something the first build did not leave:
// with the model changed it dies exporting the per-document client, with the
// model untouched it gets to the bundler.
const (
	// Second serve build after an ALTER PAGE edit.
	real1114MissingClientDirs = "One or more errors occurred. " +
		"(Could not find a part of the path '/w/W1114/deployment/web/layouts/Atlas_Core.Atlas_TopBar.js'.) " +
		"(Could not find a part of the path '/w/W1114/deployment/web/pages/MyFirstModule.Home_Web.js'.) " +
		"(Could not find a part of the path '/w/W1114/deployment/web/layouts/Atlas_Core.Atlas_Default.js'.)"

	// Second serve build, byte-identical request, model untouched. App bundler
	// = Rollup (the 11.14 default; EnableRspackBundler false in the model).
	real1114MissingRollupConfig = `{"problems":{"errors":[{"message":"Compilation of the app bundle failed.",` +
		`"details":"Error [ERR_MODULE_NOT_FOUND]: Cannot find module ` +
		`'/w/W1114/deployment/web/rollup.config.mjs' imported from ` +
		`/home/vscode/.mxcli/mxbuild/11.14.0/modeler/tools/node/rollup-runner.mjs"}],"problems":[]},` +
		`"status":"Failure","message":"Compilation of the app bundle failed."}`

	// The same, with App Settings > Runtime > App bundler = Rspack. This is the
	// control that kills "just switch bundlers" as a workaround.
	real1114MissingRspackConfig = `{"problems":{"errors":[{"message":"Compilation of the app bundle failed.",` +
		`"details":"Failed to load Rspack configuration file.\n{\n  \"code\": \"ERR_MODULE_NOT_FOUND\",\n` +
		`  \"url\": \"file:///w/W1114/deployment/web/rspack.config.mjs\"\n}"}],"problems":[]},` +
		`"status":"Failure","message":"Compilation of the app bundle failed."}`

	bundleFailedMessage = "Compilation of the app bundle failed."

	// Second serve build after an ALTER PAGE edit, macOS mxbuild 11.15.0: the
	// same failure, reported by resource key instead of English text.
	real1115MacOSMissingClientDirsByKey = "One or more errors occurred. " +
		"(AggregateException_ctor_DefaultMessage (IO_PathNotFound_Path, " +
		"/w/W1115/deployment/web/pages/MyFirstModule.Home_Web.js)) " +
		"(AggregateException_ctor_DefaultMessage (IO_PathNotFound_Path, " +
		"/w/W1115/deployment/web/layouts/Atlas_Core.Atlas_TopBar.js))"
)

// deploy1114 is the shape a 11.14 first build leaves: web/dist and nothing
// else — no bundler config, no per-document client directories.
func deploy1114(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "web", "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLegacyClientBuildHintExplainsBothRealFailures(t *testing.T) {
	for name, tc := range map[string]struct{ message, raw, want string }{
		"missing client dirs":   {real1114MissingClientDirs, "", "web/pages/"},
		"missing rollup config": {bundleFailedMessage, real1114MissingRollupConfig, "web/rollup.config.mjs"},
		"missing rspack config": {bundleFailedMessage, real1114MissingRspackConfig, "web/rspack.config.mjs"},
	} {
		hint := legacyClientBuildHint(deploy1114(t), tc.message, tc.raw)
		if hint == "" {
			t.Errorf("%s: must be explained", name)
			continue
		}
		// The three things a user acts on: it is not their model, what is
		// actually missing, and a command that works today.
		for _, want := range []string{"not a problem with your model", tc.want, "run --local --screenshot"} {
			if !strings.Contains(hint, want) {
				t.Errorf("%s: hint missing %q:\n%s", name, want, hint)
			}
		}
		// Both obvious wrong moves must be closed off — measured, neither helps.
		for _, want := range []string{"switching bundlers does not help", "Deleting deployment/ does not either"} {
			if !strings.Contains(hint, want) {
				t.Errorf("%s: hint missing %q:\n%s", name, want, hint)
			}
		}
	}
}

// macOS (and Linux) mxbuild report the identical first-build-only failure by
// resource key, not English text (measured on 11.15.0). The English-text match
// alone would leave --watch reporting "could not be applied" forever on these
// platforms, since missingClientDirs would never recognize the failure.
func TestLegacyClientBuildHintRecognizesMacOSKeyForm(t *testing.T) {
	hint := legacyClientBuildHint(deploy1114(t), real1115MacOSMissingClientDirsByKey, "")
	if hint == "" {
		t.Fatal("must be explained: this is the same shape as the English-text message")
	}
	for _, want := range []string{"not a problem with your model", "web/pages/", "run --local --screenshot"} {
		if !strings.Contains(hint, want) {
			t.Errorf("hint missing %q:\n%s", want, hint)
		}
	}
}

// The key form names a FILE ("…/web/pages/X.js"), not the directory. When an
// earlier serve build left the directory behind (e.g. mid-session after one
// successful incremental build), a plain directory-exists check would wrongly
// conclude nothing is missing and the hint would stay silent on a build that
// is failing for exactly this reason.
func TestLegacyClientBuildHintMacOSKeyForm_DirExistsFileMissing(t *testing.T) {
	dir := deploy1114(t)
	if err := os.MkdirAll(filepath.Join(dir, "web", "pages"), 0o755); err != nil {
		t.Fatal(err)
	}
	hint := legacyClientBuildHint(dir, real1115MacOSMissingClientDirsByKey, "")
	if hint == "" {
		t.Fatal("must still be explained: the directory exists but the file mxbuild wants does not")
	}
	if !strings.Contains(hint, "web/pages/ files") {
		t.Errorf("hint must name the missing files, not just the directory:\n%s", hint)
	}
}

func TestLegacyClientBuildHintStaysSilentOnOtherFailures(t *testing.T) {
	dir := deploy1114(t)
	// Every one of these is a real failure with a different cause. A hint on
	// any of them sends the user chasing a Mendix-version story that has
	// nothing to do with their build.
	for name, tc := range map[string]struct{ message, raw string }{
		"model errors":  {"1 error occurred. CE0142: The microflow must return a Boolean.", ""},
		"scss":          {"Expected expression. _custom.scss 180:35", ""},
		"widget export": {"Deployment failed during export of pluggable widgets.", ""},
		"other path":    {"Could not find a part of the path '/w/W1114/userlib/missing.jar'.", ""},
		"other module":  {bundleFailedMessage, `{"details":"Error [ERR_MODULE_NOT_FOUND]: Cannot find module '/w/node_modules/left-pad'"}`},
		"empty":         {"", ""},
	} {
		if hint := legacyClientBuildHint(dir, tc.message, tc.raw); hint != "" {
			t.Errorf("%s must not be explained as the 11.14 serve shape:\n%s", name, hint)
		}
	}
}

func TestLegacyClientBuildHintStaysSilentWhenTheArtifactsExist(t *testing.T) {
	// The pre-11.14 shape: the first build made these, so a failure naming one
	// is a real failure and not this. Without the stat the hint would fire on
	// any 11.13 build that lost a page file or a config, a different bug.
	dir := deploy1114(t)
	for _, d := range legacyClientDirs {
		if err := os.MkdirAll(filepath.Join(dir, "web", d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range bundlerConfigs {
		if err := os.WriteFile(filepath.Join(dir, "web", f), []byte("export default {}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for name, tc := range map[string]struct{ message, raw string }{
		"client dirs":   {real1114MissingClientDirs, ""},
		"rollup config": {bundleFailedMessage, real1114MissingRollupConfig},
		"rspack config": {bundleFailedMessage, real1114MissingRspackConfig},
	} {
		if hint := legacyClientBuildHint(dir, tc.message, tc.raw); hint != "" {
			t.Errorf("%s: must not fire when the artifact exists:\n%s", name, hint)
		}
	}
}
