// SPDX-License-Identifier: Apache-2.0

// Deciding what cleanup has to remove.
//
// The generated test microflows are named positionally — MxTest.Test_test_1,
// _2, … from the test's index in its file — so every test file reuses the same
// names. Cleaning up the CURRENT suite's names is therefore wrong in both
// directions, and both were reported as mendixlabs/mxcli#1104:
//
//   - A run with fewer tests than the last one leaves the surplus behind, and a
//     leftover that does not compile fails the build of every later run of any
//     file. Cleanup never looks at it again, because it is not in any suite.
//   - An injection that failed part-way created only some of them, so DROPping
//     the whole suite fails on the rest and cleanup reports the project left
//     modified when it had just been cleaned.
//
// What is in the project is the authority on what to remove. The suite is only
// the fallback for when the project cannot be read.
package testrunner

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// generatedTestFlowNames qualifies the microflow names mxcli generated, and only
// those.
//
// A pre-existing MxTest module is the user's — `mxcli test` adds documents to it
// and takes those documents back out, never the module and never anything else
// in it. The prefix is the whole of that distinction, so it is applied in one
// place.
func generatedTestFlowNames(names []string) []string {
	bare := strings.TrimPrefix(testFlowPrefix, mxTestModule+".")
	out := make([]string, 0, len(names))
	for _, n := range names {
		if strings.HasPrefix(n, bare) {
			out = append(out, mxTestModule+"."+n)
		}
	}
	return out
}

// listGeneratedTestFlows asks the project which generated test microflows it
// currently holds.
func listGeneratedTestFlows(projectPath string) ([]string, error) {
	mxcliPath, err := findMxcli()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(mxcliPath, "-p", projectPath, "-c", "SHOW MICROFLOWS IN "+mxTestModule, "--json")
	cmd.Env = append(os.Environ(), "MXCLI_QUIET=1")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var flows []struct {
		Name string `json:"Name"`
	}
	if err := json.Unmarshal(output, &flows); err != nil {
		return nil, fmt.Errorf("parsing microflow list: %w", err)
	}
	names := make([]string, 0, len(flows))
	for _, f := range flows {
		names = append(names, f.Name)
	}
	return generatedTestFlowNames(names), nil
}

// testFlowsToDrop returns the generated microflows cleanup should remove.
//
// Discovery can fail — no MxTest module yet, an unreadable project, no mxcli on
// PATH — and a cleanup that removes nothing is worse than one that tries the
// names it knows. So the suite is the fallback, which is exactly the old
// behaviour and no worse than it.
func testFlowsToDrop(projectPath string, suite *TestSuite) []string {
	if projectPath == "" {
		return suiteTestFlowNames(suite)
	}
	if flows, err := listGeneratedTestFlows(projectPath); err == nil {
		return flows
	}
	return suiteTestFlowNames(suite)
}

// suiteTestFlowNames is the fallback: the names this run would have created.
func suiteTestFlowNames(suite *TestSuite) []string {
	if suite == nil {
		return nil
	}
	names := make([]string, 0, len(suite.Tests))
	for _, tc := range suite.Tests {
		names = append(names, testFlowName(tc))
	}
	if len(names) > 0 {
		names = append(names, errorMessageFlow)
	}
	return names
}

// survivingTestFlows reports what is still in the project after a failed
// cleanup, best effort. Nothing is reported rather than something guessed: a
// list the reader cannot trust is worse than no list, because the whole point
// is to save them the hunt.
func survivingTestFlows(projectPath string) []string {
	if projectPath == "" {
		return nil
	}
	flows, err := listGeneratedTestFlows(projectPath)
	if err != nil {
		return nil
	}
	return flows
}
