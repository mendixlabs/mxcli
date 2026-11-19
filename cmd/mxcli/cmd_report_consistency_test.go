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

// mx check --json output, as recorded in docker/testdata/mxcheck.
func mxCheckJSON(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("docker", "testdata", "mxcheck", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

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

type mxCheckCall struct {
	opts  docker.CheckOptions
	kinds docker.CheckKinds
}

func stubMxCheck(t *testing.T, rep docker.CheckReport, err error) *mxCheckCall {
	t.Helper()
	orig := reportMxCheck
	t.Cleanup(func() { reportMxCheck = orig })
	got := &mxCheckCall{}
	reportMxCheck = func(opts docker.CheckOptions, kinds docker.CheckKinds) (docker.CheckReport, error) {
		got.opts, got.kinds = opts, kinds
		return rep, err
	}
	return got
}

func parsedCheck(t *testing.T, name string) docker.CheckReport {
	t.Helper()
	rep, err := docker.ParseCheckJSON(mxCheckJSON(t, name))
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

// ako/mxcli#2423: the scored report runs mx check, warnings and deprecations
// included, and says whether the project builds.
func TestReportConsistencyRunsMxCheck(t *testing.T) {
	call := stubMxCheck(t, parsedCheck(t, "f05_undef_all_kinds.json"), errors.New("exit status 1"))
	c, err := reportConsistency(consistencyCmd(t, map[string]string{"mxbuild-path": "/opt/mxbuild"}), "app.mpr", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Ran || c.Errors != 1 || c.Warnings != 16 || c.Deprecations != 1 || c.Builds() || c.Incomplete != "" {
		t.Errorf("errors found: ran=%v %d/%d/%d incomplete=%q", c.Ran, c.Errors, c.Warnings, c.Deprecations, c.Incomplete)
	}
	if call.opts.ProjectPath != "app.mpr" || call.opts.MxBuildPath != "/opt/mxbuild" || !call.kinds.Warnings || !call.kinds.Deprecations {
		t.Errorf("mx check got %+v", *call)
	}
	if f := c.Findings[0]; f.Module == "" || !strings.Contains(f.Document, "'") {
		t.Errorf("a finding keeps its module and document: %+v", f)
	}

	stubMxCheck(t, parsedCheck(t, "f10_clean.json"), nil)
	c, _ = reportConsistency(consistencyCmd(t, nil), "app.mpr", io.Discard)
	if !c.Builds() || c.Verdict() != "yes" {
		t.Errorf("clean project: %+v", c)
	}
}

// No mx is a report that says the check did not run and how to run it, never a
// clean one; and findings that disagree with mx's declared total are not
// "builds".
func TestReportConsistencyNeverCleanWhenUnknown(t *testing.T) {
	stubMxCheck(t, docker.CheckReport{}, errors.New("mx not found; specify --mxbuild-path"))
	c, err := reportConsistency(consistencyCmd(t, nil), "app.mpr", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if c.Ran || c.Builds() || !strings.Contains(c.NotRun, "mx not found") || !strings.Contains(c.NotRun, "mxcli setup mxbuild -p app.mpr") {
		t.Errorf("no mx: %+v", c)
	}

	short := docker.CheckReport{Declared: 2, HasDeclared: true}
	stubMxCheck(t, short, errors.New("exit status 1"))
	c, _ = reportConsistency(consistencyCmd(t, nil), "app.mpr", io.Discard)
	if c.Builds() || c.Incomplete == "" || !strings.HasPrefix(c.Verdict(), "unknown") {
		t.Errorf("a total the findings do not add up to must not read as building: %+v", c)
	}
}

func TestReportConsistencyResultsFileAndSkip(t *testing.T) {
	called := false
	orig := reportMxCheck
	t.Cleanup(func() { reportMxCheck = orig })
	reportMxCheck = func(docker.CheckOptions, docker.CheckKinds) (docker.CheckReport, error) {
		called = true
		return docker.CheckReport{}, nil
	}

	path := filepath.Join(t.TempDir(), "check.json")
	if err := os.WriteFile(path, mxCheckJSON(t, "f01_dupvar.json"), 0644); err != nil {
		t.Fatal(err)
	}
	c, err := reportConsistency(consistencyCmd(t, map[string]string{"mx-check-results": path}), "app.mpr", io.Discard)
	if err != nil || c.Errors != 1 || !strings.Contains(c.Source, path) {
		t.Errorf("results file: %+v %v", c, err)
	}
	if _, err := reportConsistency(consistencyCmd(t, map[string]string{"mx-check-results": path + ".missing"}), "app.mpr", io.Discard); err == nil {
		t.Error("a results file that cannot be read is an error, not a clean check")
	}
	text := filepath.Join(t.TempDir(), "check.txt")
	_ = os.WriteFile(text, []byte("[error] [CE0117] \"x\" at Microflow 'A'\nThe app contains: 1 error.\n"), 0644)
	if _, err := reportConsistency(consistencyCmd(t, map[string]string{"mx-check-results": text}), "app.mpr", io.Discard); err == nil || !strings.Contains(err.Error(), "-j") {
		t.Errorf("printed mx check output is refused, saying which file it takes: %v", err)
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
