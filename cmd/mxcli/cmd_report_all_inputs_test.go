// SPDX-License-Identifier: Apache-2.0

package main

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// resetFlags puts flags back to their defaults: report kinds are package
// globals, so a value set by one test would leak into the next.
func resetFlags(t *testing.T, cmd *cobra.Command, names ...string) {
	t.Helper()
	t.Cleanup(func() {
		for _, n := range names {
			f := cmd.Flags().Lookup(n)
			if f == nil {
				continue
			}
			if sv, ok := f.Value.(pflag.SliceValue); ok {
				_ = sv.Replace(nil)
			} else {
				_ = f.Value.Set(f.DefValue)
			}
			f.Changed = false
		}
	})
}

func flagString(t *testing.T, kind, name string) string {
	t.Helper()
	v, err := reportKinds[kind].cmd.Flags().GetString(name)
	if err != nil {
		t.Fatalf("%s --%s: %v", kind, name, err)
	}
	return v
}

func flagSlice(t *testing.T, kind, name string) []string {
	t.Helper()
	v, err := reportKinds[kind].cmd.Flags().GetStringSlice(name)
	if err != nil {
		t.Fatalf("%s --%s: %v", kind, name, err)
	}
	return v
}

// report all hands each report the inputs that are its own (ako/mxcli#2451):
// the functional journey log and the test results go to tests AND
// traceability, and performance gets only a performance log, never the
// functional one, which it would read as one "step" per journey.
func TestReportAllHandsEachReportItsOwnInputs(t *testing.T) {
	evidence := []string{"journeys", "spans", "junit", "tests", "run", "test-arg"}
	resetFlags(t, reportAllCmd, append(evidence, "perf-journeys", "perf-spans")...)
	for _, k := range []string{"tests", "traceability", "performance"} {
		resetFlags(t, reportKinds[k].cmd, evidence...)
	}
	for flag, v := range map[string]string{"journeys": "func.jsonl", "spans": "func-spans.jsonl", "junit": "results.xml", "tests": "tests/"} {
		if err := reportAllCmd.Flags().Set(flag, v); err != nil {
			t.Fatalf("report all --%s: %v", flag, err)
		}
	}
	passThrough(reportAllCmd, "", nil)

	for _, k := range []string{"tests", "traceability"} {
		if got := flagString(t, k, "journeys"); got != "func.jsonl" {
			t.Errorf("%s --journeys = %q, want the functional log", k, got)
		}
		if got := flagSlice(t, k, "spans"); !reflect.DeepEqual(got, []string{"func-spans.jsonl"}) {
			t.Errorf("%s --spans = %v", k, got)
		}
		if got := flagString(t, k, "junit"); got != "results.xml" {
			t.Errorf("%s --junit = %q, want the test results", k, got)
		}
		if got := flagSlice(t, k, "tests"); !reflect.DeepEqual(got, []string{"tests/"}) {
			t.Errorf("%s --tests = %v", k, got)
		}
	}
	if got := flagString(t, "performance", "journeys"); got != "" {
		t.Errorf("performance got the functional log %q: it would read each journey as a step", got)
	}
	if why := needsInput(reportAllCmd, "performance"); why == "" {
		t.Error("without a performance log, performance does not run")
	}

	// With its own log, performance runs on that.
	_ = reportAllCmd.Flags().Set("perf-journeys", "perf.jsonl")
	_ = reportAllCmd.Flags().Set("perf-spans", "perf-spans.jsonl")
	passThrough(reportAllCmd, "", nil)
	if why := needsInput(reportAllCmd, "performance"); why != "" {
		t.Errorf("with --perf-journeys, performance runs: %s", why)
	}
	if got := flagString(t, "performance", "journeys"); got != "perf.jsonl" {
		t.Errorf("performance --journeys = %q, want the performance log", got)
	}
	if got := flagSlice(t, "performance", "spans"); !reflect.DeepEqual(got, []string{"perf-spans.jsonl"}) {
		t.Errorf("performance --spans = %v", got)
	}
}

// --run runs the tests once, in the tests report, into <dir>/junit.xml;
// traceability, which runs after it, reads that file instead of running them
// again.
func TestReportAllRunWritesResultsForTraceability(t *testing.T) {
	evidence := []string{"journeys", "spans", "junit", "tests", "run", "test-arg", "local"}
	resetFlags(t, reportAllCmd, evidence...)
	for _, k := range []string{"tests", "traceability"} {
		resetFlags(t, reportKinds[k].cmd, evidence...)
	}
	_ = reportAllCmd.Flags().Set("run", "true")
	_ = reportAllCmd.Flags().Set("local", "true")
	passThrough(reportAllCmd, "out", nil)
	if local, _ := reportKinds["tests"].cmd.Flags().GetBool("local"); !local {
		t.Error("report all --local reaches the tests report")
	}
	if run, _ := reportKinds["tests"].cmd.Flags().GetBool("run"); !run {
		t.Error("tests runs the microflow tests")
	}
	if run, _ := reportKinds["traceability"].cmd.Flags().GetBool("run"); run {
		t.Error("traceability does not run them a second time")
	}
	for _, k := range []string{"tests", "traceability"} {
		if got := flagString(t, k, "junit"); got != "out/junit.xml" {
			t.Errorf("%s --junit = %q, want out/junit.xml", k, got)
		}
	}
}
