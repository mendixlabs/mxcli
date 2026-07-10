// SPDX-License-Identifier: Apache-2.0

// Tests that concurrent describe calls do not race on the describer's flow-map
// state. The describer used to install package-level currentFlowsByOrigin /
// currentFlowsByDest maps for the duration of a describe; two goroutines
// running formatMicroflowActivities in parallel would clobber each other's
// maps mid-traversal.
//
// This test drives formatMicroflowActivities from many goroutines at once.
// It is primarily useful under `go test -race`, where the previous global-
// state implementation reported a data race on currentFlowsByOrigin /
// currentFlowsByDest. After threading the maps as explicit parameters the
// race disappears; the test also asserts per-goroutine output integrity.
package executor

import (
	"strings"
	"sync"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

func TestFormatMicroflowActivities_Concurrent_NoRace(t *testing.T) {
	// Build two distinct microflows whose activities are anchored to
	// different sides. If the two describe calls share state, one will
	// emit the other's anchor keyword.
	mfA := mkRaceMicroflow("mfa-start", "mfa-log", "mfa-end", AnchorTop)
	mfB := mkRaceMicroflow("mfb-start", "mfb-log", "mfb-end", AnchorBottom)

	e := newTestExecutor()
	ctx := e.newExecContext(t.Context())

	const workersPerFlow = 12

	var wg sync.WaitGroup
	resultsA := make([]string, workersPerFlow)
	resultsB := make([]string, workersPerFlow)
	for i := 0; i < workersPerFlow; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			resultsA[i] = strings.Join(formatMicroflowActivities(ctx, mfA, nil, nil), "\n")
		}(i)
		go func(i int) {
			defer wg.Done()
			resultsB[i] = strings.Join(formatMicroflowActivities(ctx, mfB, nil, nil), "\n")
		}(i)
	}
	wg.Wait()

	wantA := "@anchor(from: top)"
	wantB := "@anchor(from: bottom)"
	for i, got := range resultsA {
		if !strings.Contains(got, wantA) {
			t.Errorf("worker %d (A) missing %q in output:\n%s", i, wantA, got)
		}
		if strings.Contains(got, wantB) {
			t.Errorf("worker %d (A) unexpectedly contains %q from flow B:\n%s", i, wantB, got)
		}
	}
	for i, got := range resultsB {
		if !strings.Contains(got, wantB) {
			t.Errorf("worker %d (B) missing %q in output:\n%s", i, wantB, got)
		}
		if strings.Contains(got, wantA) {
			t.Errorf("worker %d (B) unexpectedly contains %q from flow A:\n%s", i, wantA, got)
		}
	}
}

// mkRaceMicroflow builds a minimal microflow (Start → Log → End) whose
// sequence-flow origin index matches the given anchorSide, so concurrent
// describe outputs are distinguishable by their @anchor line.
func mkRaceMicroflow(startID, logID, endID string, originSide int) *microflows.Microflow {
	start := &microflows.StartEvent{BaseMicroflowObject: mkObj(startID)}
	log := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj(logID)},
		Action: &microflows.LogMessageAction{
			LogLevel:    "Info",
			LogNodeName: "'App'",
			MessageTemplate: &model.Text{
				Translations: map[string]string{"en_US": "hi"},
			},
		},
	}
	end := &microflows.EndEvent{BaseMicroflowObject: mkObj(endID)}

	// Flow start→log carries the distinguishing originSide on the LOG side
	// (where @anchor(from: ...) is emitted).
	flowSL := &microflows.SequenceFlow{
		OriginID:                   mkID(startID),
		DestinationID:              mkID(logID),
		OriginConnectionIndex:      AnchorRight,
		DestinationConnectionIndex: AnchorLeft,
	}
	flowLE := &microflows.SequenceFlow{
		OriginID:                   mkID(logID),
		DestinationID:              mkID(endID),
		OriginConnectionIndex:      originSide,
		DestinationConnectionIndex: AnchorLeft,
	}

	return &microflows.Microflow{
		Name: "MF_" + logID,
		ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{start, log, end},
			Flows:   []*microflows.SequenceFlow{flowSL, flowLE},
		},
	}
}
