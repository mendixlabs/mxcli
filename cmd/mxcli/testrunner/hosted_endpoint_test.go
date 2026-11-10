// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"strings"
	"testing"
)

// ako/mxcli#2119: `test --local` while a dev loop hosts the endpoint read the
// dev loop's MxTest.RegisterEndpoint as the project's after-startup microflow,
// chained its own endpoint to it — to itself — and the scratch runtime failed to
// boot with a StackOverflowError. A live host's restore record holds what the
// endpoint replaced, and that is what a test run chains to.
func TestClassifyHostedEndpoint(t *testing.T) {
	alive := func(int) bool { return true }
	dead := func(int) bool { return false }
	rec := &restoreRecord{PID: 77, AfterStartup: "ServiceDesk.ASU_App"}

	hosted, chain, err := classifyHostedEndpoint("/p/App.mpr", endpointStartupFlow, rec, alive)
	if err != nil || !hosted || chain != "ServiceDesk.ASU_App" {
		t.Errorf("live host: hosted=%v chain=%q err=%v, want true, ServiceDesk.ASU_App, nil", hosted, chain, err)
	}
	st := projectState{afterStartup: endpointStartupFlow, hosted: hosted, hostedChain: chain}
	if st.chain() == endpointStartupFlow {
		t.Error("the run would chain its endpoint to itself")
	}

	// The project's own after-startup is chained as before.
	if hosted, _, err := classifyHostedEndpoint("/p/App.mpr", "ServiceDesk.ASU_App", rec, alive); hosted || err != nil {
		t.Errorf("an ordinary after-startup: hosted=%v err=%v", hosted, err)
	}
	if (projectState{afterStartup: "M.ASU"}).chain() != "M.ASU" {
		t.Error("an ordinary after-startup is not chained")
	}

	// The endpoint's flow with no live owner is a leftover nothing may chain to.
	withAfterStartup(t, endpointStartupFlow)
	for _, c := range []struct {
		name  string
		rec   *restoreRecord
		alive func(int) bool
	}{{"dead owner", rec, dead}, {"no record", nil, alive}} {
		if _, _, err := classifyHostedEndpoint("/p/App.mpr", endpointStartupFlow, c.rec, c.alive); err == nil ||
			!strings.Contains(err.Error(), endpointStartupFlow) {
			t.Errorf("%s: err = %v, want a refusal naming %s", c.name, err, endpointStartupFlow)
		}
	}
}

// The cleanup of a run under a hosting dev loop removes its test microflows and
// puts the setting back, but leaves the dev loop's endpoint: dropping it left
// the setting naming a microflow that no longer existed (CE1613).
func TestEndpointCleanupCommands_LeavesAHostedEndpoint(t *testing.T) {
	st := projectState{afterStartup: endpointStartupFlow, hosted: true, hostedChain: "M.ASU"}
	cmds := endpointCleanupCommands(st, []string{testFlowPrefix + "test_1"}, true)
	joined := strings.Join(cmds, "\n")
	if strings.Contains(joined, "DROP MICROFLOW "+endpointStartupFlow) || strings.Contains(joined, "DROP JAVA ACTION") ||
		strings.Contains(joined, "DROP MODULE") {
		t.Errorf("the hosted endpoint is dropped:\n%s", joined)
	}
	if !strings.Contains(joined, "DROP MICROFLOW "+testFlowPrefix+"test_1") ||
		!strings.Contains(joined, "AfterStartupMicroflow: '"+endpointStartupFlow+"'") {
		t.Errorf("the run's own test flows or the setting are not restored:\n%s", joined)
	}

	// Control: an unhosted run with a pre-existing MxTest module still drops
	// its endpoint, as before.
	own := endpointCleanupCommands(projectState{afterStartup: "M.ASU"}, nil, true)
	if !strings.Contains(strings.Join(own, "\n"), "DROP MICROFLOW "+endpointStartupFlow) {
		t.Errorf("an unhosted run no longer removes its endpoint: %v", own)
	}
}
