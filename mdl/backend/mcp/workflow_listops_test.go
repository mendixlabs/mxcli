// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// pedListSim applies ped_update_document batches to the lists it holds the way
// Studio Pro 11.14 does. The rule is the one that fits every raw-PED measurement
// on a probe workflow (TestPEDListSim_ReproducesMeasurements): a batch's ops run
// highest index first, and at one index the adds go in as a block, in op order,
// before the removes — so a remove at an index where something was just added
// takes the added element.
//
// Reads of a held list return it; anything else is left to the test's handler.
type pedListSim struct {
	lists map[string][]map[string]any
	// mixed counts batches that both add to and remove from a list.
	mixed int
}

func newPEDListSim(lists map[string][]map[string]any) *pedListSim {
	return &pedListSim{lists: lists}
}

func named(names ...string) []map[string]any {
	out := make([]map[string]any, len(names))
	for i, n := range names {
		out[i] = map[string]any{"$Type": "Workflows$WaitForNotificationActivity", "name": n}
	}
	return out
}

func (s *pedListSim) handle(name string, args map[string]any) (string, bool) {
	switch name {
	case "ped_read_document":
		p, _ := args["paths"].([]any)[0].(string)
		l, ok := s.lists[p]
		if !ok {
			return "", false
		}
		raw, _ := json.Marshal(l)
		if l == nil {
			raw = []byte("[]")
		}
		return fmt.Sprintf(`{"results":[{"path":%q,"result":%s}]}`, p, raw), true
	case "ped_update_document":
		type listOp struct {
			add       bool
			indexless bool
			at        int
			v         map[string]any
		}
		byPath := map[string][]listOp{}
		var paths []string
		for _, o := range args["operations"].([]any) {
			entry := o.(map[string]any)
			p := entry["path"].(string)
			if _, ok := s.lists[p]; !ok {
				continue
			}
			op := entry["operation"].(map[string]any)
			at := len(s.lists[p])
			idx, hasIndex := op["index"].(float64)
			if hasIndex {
				at = int(idx)
			}
			if _, seen := byPath[p]; !seen {
				paths = append(paths, p)
			}
			v, _ := op["value"].(map[string]any)
			byPath[p] = append(byPath[p], listOp{add: op["type"] == "add", indexless: !hasIndex, at: at, v: v})
		}
		for _, p := range paths {
			ops := byPath[p]
			var adds, removes bool
			for _, o := range ops {
				adds, removes = adds || o.add, removes || !o.add
			}
			if adds && removes {
				s.mixed++
			}
			// Highest index first; at one index the adds (in op order) before the removes.
			sort.SliceStable(ops, func(i, j int) bool {
				if ops[i].at != ops[j].at {
					return ops[i].at > ops[j].at
				}
				return ops[i].add && !ops[j].add
			})
			l := s.lists[p]
			run := 0
			for i, o := range ops {
				// Adds at one explicit index keep op order; index-less adds each go in
				// at the same end position, so a run of them comes out reversed.
				if o.add && !o.indexless && i > 0 && ops[i-1].add && ops[i-1].at == o.at {
					run++
				} else {
					run = 0
				}
				at := o.at + run
				switch {
				case o.add && at <= len(l):
					l = append(l[:at], append([]map[string]any{o.v}, l[at:]...)...)
				case !o.add && at < len(l):
					l = append(l[:at:at], l[at+1:]...)
				default:
					return fmt.Sprintf("ERROR: index %d out of range at %s (length %d)", at, p, len(l)), true
				}
			}
			s.lists[p] = l
		}
		return "SUCCESS", true
	}
	return "", false
}

func (s *pedListSim) field(path, key string) []any {
	var out []any
	for _, e := range s.lists[path] {
		out = append(out, e[key])
	}
	return out
}

// The simulator reproduces every batch measured against Studio Pro 11.14 — the
// control that makes the order tests below mean something. Among them the batch
// UpdateWorkflow used to send, and the ones ALTER's insert and replace sent.
func TestPEDListSim_ReproducesMeasurements(t *testing.T) {
	add := func(name string, at int) any {
		return map[string]any{"path": "/l", "operation": map[string]any{"type": "add", "index": float64(at), "value": map[string]any{"name": name}}}
	}
	addEnd := func(name string) any {
		return map[string]any{"path": "/l", "operation": map[string]any{"type": "add", "value": map[string]any{"name": name}}}
	}
	rm := func(at int) any {
		return map[string]any{"path": "/l", "operation": map[string]any{"type": "remove", "index": float64(at)}}
	}
	cases := []struct {
		name        string
		before, got []string
		ops         []any
	}{
		{"old UpdateWorkflow: removes, then the middles in reverse at 1",
			[]string{"S", "a", "b", "E"}, []string{"S", "B", "A", "a", "E"}, []any{rm(2), rm(1), add("C", 1), add("B", 1), add("A", 1)}},
		{"adds at one index keep op order",
			[]string{"S", "w2", "w3", "E"}, []string{"S", "P", "Q", "w2", "w3", "E"}, []any{add("P", 1), add("Q", 1)}},
		{"adds at different indices",
			[]string{"S", "P", "Q", "w2", "E"}, []string{"S", "R", "P", "S2", "Q", "w2", "E"}, []any{add("R", 1), add("S2", 2)}},
		{"old InsertAfterActivity: incrementing indices",
			[]string{"S", "R", "P", "S2", "E"}, []string{"S", "R", "I1", "P", "I2", "S2", "E"}, []any{add("I1", 2), add("I2", 3)}},
		{"old ReplaceActivity: remove then adds at k, k+1",
			[]string{"S", "I1", "P", "I2", "E"}, []string{"S", "I1", "P", "K2", "I2", "E"}, []any{rm(2), add("K1", 2), add("K2", 3)}},
		{"remove then adds at one index",
			[]string{"S", "I2", "S2", "Q", "E"}, []string{"S", "I2", "L2", "S2", "Q", "E"}, []any{rm(2), add("L1", 2), add("L2", 2)}},
		{"removes only, high to low",
			[]string{"S", "w1", "Z", "w2", "E"}, []string{"S", "w2", "E"}, []any{rm(2), rm(1)}},
		{"event sub-processes at different indices",
			[]string{"C", "D", "B", "A"}, []string{"C", "E", "D", "B", "F", "A"}, []any{add("E", 1), add("F", 3)}},
		{"index-less event sub-process adds",
			[]string{}, []string{"B", "A"}, []any{addEnd("A"), addEnd("B")}},
		{"index-less boundary event adds",
			[]string{}, []string{"NI-B", "NI-A", "I"}, []any{addEnd("I"), addEnd("NI-A"), addEnd("NI-B")}},
		{"one index-less add appends",
			[]string{"C", "A"}, []string{"C", "A", "G"}, []any{addEnd("G")}},
	}
	for _, c := range cases {
		list := make([]map[string]any, len(c.before))
		for i, n := range c.before {
			list[i] = map[string]any{"name": n}
		}
		sim := newPEDListSim(map[string][]map[string]any{"/l": list})
		sim.handle("ped_update_document", map[string]any{"operations": c.ops})
		want := make([]any, len(c.got))
		for i, n := range c.got {
			want[i] = n
		}
		if got := sim.field("/l", "name"); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: simulated %v, measured %v", c.name, got, want)
		}
	}
}

// CREATE OR MODIFY over MCP stores the flow, the event sub-processes and the event
// handlers in the statement's order, with the stored ones gone — and never sends
// adds and removes for a list in one batch.
func TestUpdateWorkflow_ReplacesInStatementOrder(t *testing.T) {
	sim := newPEDListSim(map[string][]map[string]any{
		"/flow/activities":   named("Start", "old1", "old2", "End"),
		"/eventSubProcesses": {{"$Type": "Workflows$EventSubProcess", "name": "OldEsp"}},
		"/onWorkflowEvent":   {{"$Type": "Workflows$WorkflowEventHandler", "description": "old"}},
	})
	f := newFakePED(t, func(name string, args map[string]any) (string, bool) {
		if text, ok := sim.handle(name, args); ok {
			return text, false
		}
		if name == "ped_check_errors" {
			return "No errors found.", false
		}
		return "SUCCESS", false
	})
	mod := &model.Module{Name: "M"}
	mod.ID = "mod1"
	b := &Backend{client: f.connectClient(t), sessionModules: []*model.Module{mod}}
	esp := func(name string) *workflows.EventSubProcess {
		return &workflows.EventSubProcess{Name: name, Flow: timerPath(
			&workflows.EventSubProcessStartActivity{BaseWorkflowActivity: wfBase(name+"Start", name), Interrupting: true},
			timerEnd(name+"End"))}
	}
	wf := &workflows.Workflow{
		Name: "WF",
		Flow: timerPath(
			&workflows.StartWorkflowActivity{BaseWorkflowActivity: wfBase("Start", "Start")},
			waitWith("A"), waitWith("B"), waitWith("C"),
			&workflows.EndWorkflowActivity{BaseWorkflowActivity: wfBase("End", "End")},
		),
		EventSubProcesses: []*workflows.EventSubProcess{esp("Esp1"), esp("Esp2")},
		EventHandlers: []*workflows.WorkflowEventHandler{
			{Description: "one", Microflow: "M.One"}, {Description: "two", Microflow: "M.Two"},
		},
	}
	wf.ContainerID = "mod1"
	wf.ID = "wfid"
	if err := b.UpdateWorkflow(wf); err != nil {
		t.Fatalf("UpdateWorkflow: %v", err)
	}
	for path, want := range map[string][]any{
		"/flow/activities":   {"Start", "A", "B", "C", "End"},
		"/eventSubProcesses": {"Esp1", "Esp2"},
	} {
		if got := sim.field(path, "name"); !reflect.DeepEqual(got, want) {
			t.Errorf("%s stored %v, want %v", path, got, want)
		}
	}
	if got, want := sim.field("/onWorkflowEvent", "description"), []any{"one", "two"}; !reflect.DeepEqual(got, want) {
		t.Errorf("/onWorkflowEvent stored %v, want %v", got, want)
	}
	if sim.mixed != 0 {
		t.Errorf("%d batch(es) both added to and removed from one list", sim.mixed)
	}
}

func listMutator(t *testing.T, flow []map[string]any) (*pedListSim, *mcpWorkflowMutator) {
	t.Helper()
	sim := newPEDListSim(map[string][]map[string]any{"/flow/activities": flow})
	f := newFakePED(t, func(name string, args map[string]any) (string, bool) {
		if text, ok := sim.handle(name, args); ok {
			return text, false
		}
		if name == "ped_read_document" {
			p := args["paths"].([]any)[0].(string)
			return fmt.Sprintf(`{"results":[{"path":%q,"result":null}]}`, p), false
		}
		return "SUCCESS", false
	})
	return sim, &mcpWorkflowMutator{backend: &Backend{client: f.connectClient(t)}, moduleName: "M", workflowName: "WF"}
}

// INSERT … AFTER X with several activities stores them together, in order, right
// after X. Measured: the incrementing indices it used to send (I1@2, I2@3 after
// R) stored R, I1, P, I2.
func TestWFInsertAfterActivity_KeepsOrder(t *testing.T) {
	sim, m := listMutator(t, named("Start", "R", "P", "End"))
	if err := m.InsertAfterActivity("R", 0, []workflows.WorkflowActivity{waitWith("I1"), waitWith("I2")}); err != nil {
		t.Fatal(err)
	}
	if got, want := sim.field("/flow/activities", "name"), []any{"Start", "R", "I1", "I2", "P", "End"}; !reflect.DeepEqual(got, want) {
		t.Errorf("stored %v, want %v", got, want)
	}
}

// REPLACE X replaces it in place, with one activity or several. Measured: the
// single batch it used to send (remove @k, adds at k, k+1) removed the first
// added activity and kept X.
func TestWFReplaceActivity_ReplacesInPlace(t *testing.T) {
	sim, m := listMutator(t, named("Start", "R", "P", "End"))
	if err := m.ReplaceActivity("P", 0, []workflows.WorkflowActivity{waitWith("K1"), waitWith("K2")}); err != nil {
		t.Fatal(err)
	}
	if err := m.ReplaceActivity("R", 0, []workflows.WorkflowActivity{waitWith("R2")}); err != nil {
		t.Fatal(err)
	}
	if got, want := sim.field("/flow/activities", "name"), []any{"Start", "R2", "K1", "K2", "End"}; !reflect.DeepEqual(got, want) {
		t.Errorf("stored %v, want %v", got, want)
	}
	if sim.mixed != 0 {
		t.Errorf("%d batch(es) both added to and removed from the flow", sim.mixed)
	}
}
