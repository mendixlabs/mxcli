// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	"github.com/spf13/cobra"
)

const mxCheckWithErrors = `Checking app...
[error] [CE0117] "Error(s) in expression." at Microflow 'ACT_X'
[warning] [CW0702] "Unused variable." at Microflow 'ACT_Y'
[deprecation] [CD0001] "Deprecated widget." at Page 'Home'
The app contains: 1 error.
`

const mxCheckClean = "Checking app...\nThe app contains: 0 errors.\n"

func consistencyCmd(t *testing.T, flags map[string]string) *cobra.Command {
	t.Helper()
	c := &cobra.Command{}
	c.Flags().Bool("no-mx-check", false, "")
	c.Flags().String("mx-check-results", "", "")
	c.Flags().String("mxbuild-path", "", "")
	for k, v := range flags {
		if err := c.Flags().Set(k, v); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

func stubMxCheck(t *testing.T, output string, err error) *docker.CheckOptions {
	t.Helper()
	orig := reportMxCheck
	t.Cleanup(func() { reportMxCheck = orig })
	var got docker.CheckOptions
	reportMxCheck = func(opts docker.CheckOptions) (docker.CheckReport, error) {
		got = opts
		if output == "" {
			return docker.CheckReport{}, err
		}
		return docker.ParseCheckReport(output), err
	}
	return &got
}

// ako/mxcli#2423: the scored report runs mx check and says whether the
// project builds.
func TestReportConsistencyRunsMxCheck(t *testing.T) {
	opts := stubMxCheck(t, mxCheckWithErrors, errors.New("exit status 1"))
	c, err := reportConsistency(consistencyCmd(t, map[string]string{"mxbuild-path": "/opt/mxbuild"}), "app.mpr", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Ran || c.Errors != 1 || c.Warnings != 1 || c.Deprecations != 1 || c.Builds() || c.Incomplete != "" {
		t.Errorf("errors found: %+v", c)
	}
	if opts.ProjectPath != "app.mpr" || opts.MxBuildPath != "/opt/mxbuild" {
		t.Errorf("mx check got %+v", *opts)
	}

	stubMxCheck(t, mxCheckClean, nil)
	c, _ = reportConsistency(consistencyCmd(t, nil), "app.mpr", io.Discard)
	if !c.Builds() || c.Verdict() != "yes" {
		t.Errorf("clean project: %+v", c)
	}
}

// No mx is a report that says the check did not run and how to run it, never a
// clean one; and output whose error count disagrees with mx's own is not
// "builds".
func TestReportConsistencyNeverCleanWhenUnknown(t *testing.T) {
	stubMxCheck(t, "", errors.New("mx not found; specify --mxbuild-path"))
	c, err := reportConsistency(consistencyCmd(t, nil), "app.mpr", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if c.Ran || c.Builds() || !strings.Contains(c.NotRun, "mx not found") || !strings.Contains(c.NotRun, "mxcli setup mxbuild -p app.mpr") {
		t.Errorf("no mx: %+v", c)
	}

	// mx stopped before its closing count: one error seen, the rest unknown.
	stubMxCheck(t, `[error] [CE0117] "x" at Microflow 'A'`+"\n", errors.New("signal: killed"))
	c, _ = reportConsistency(consistencyCmd(t, nil), "app.mpr", io.Discard)
	if !c.Ran || c.Errors != 1 || c.Incomplete == "" {
		t.Errorf("cut short: %+v", c)
	}
	stubMxCheck(t, "The app contains: 2 errors.\n", errors.New("exit status 1"))
	c, _ = reportConsistency(consistencyCmd(t, nil), "app.mpr", io.Discard)
	if c.Builds() || c.Incomplete == "" || !strings.HasPrefix(c.Verdict(), "unknown") {
		t.Errorf("count mismatch must not read as building: %+v", c)
	}
}

func TestReportConsistencyResultsFileAndSkip(t *testing.T) {
	called := false
	orig := reportMxCheck
	t.Cleanup(func() { reportMxCheck = orig })
	reportMxCheck = func(docker.CheckOptions) (docker.CheckReport, error) { called = true; return docker.CheckReport{}, nil }

	path := filepath.Join(t.TempDir(), "check.txt")
	if err := os.WriteFile(path, []byte("Using mx: /x/mx\n"+mxCheckWithErrors), 0644); err != nil {
		t.Fatal(err)
	}
	c, err := reportConsistency(consistencyCmd(t, map[string]string{"mx-check-results": path}), "app.mpr", io.Discard)
	if err != nil || c.Errors != 1 || !strings.Contains(c.Source, path) {
		t.Errorf("results file: %+v %v", c, err)
	}
	if _, err := reportConsistency(consistencyCmd(t, map[string]string{"mx-check-results": path + ".missing"}), "app.mpr", io.Discard); err == nil {
		t.Error("a results file that cannot be read is an error, not a clean check")
	}
	c, _ = reportConsistency(consistencyCmd(t, map[string]string{"no-mx-check": "true"}), "app.mpr", io.Discard)
	if c.Ran || !strings.Contains(c.NotRun, "--no-mx-check") {
		t.Errorf("skipped: %+v", c)
	}
	if called {
		t.Error("neither a results file nor --no-mx-check runs mx")
	}
	for _, f := range []string{"no-mx-check", "mx-check-results", "mxbuild-path"} {
		if reportCmd.Flags().Lookup(f) == nil {
			t.Errorf("mxcli report has no --%s", f)
		}
	}
}
