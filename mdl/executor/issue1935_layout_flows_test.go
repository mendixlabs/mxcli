// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// A loop written by an mxcli that sized its box from the body's absolute canvas
// coordinates keeps that wrong Size in the stored unit (#1935). `layout flows`
// rebuilds the flow, so it must heal the stored box rather than report a move
// that leaves it untouched - and heal it once, not on every run.
//
// The reads go through a fresh connection: a second `mxcli layout flows` is a
// new process, and this is the bug where the first run reported `moved` while
// the stored Size stayed put.
func TestLayoutFlow_RecomputesStaleLoopBox(t *testing.T) {
	path := projectFixture(t)
	exec := newLayoutFlowExecutor(t, path)
	run(t, exec, `create microflow MyFirstModule.LoopFlow (
  $Items: List of System.User
)
begin
  @start(60, 200)
  @position(600, 200)
  loop $Item in $Items
  begin
    @position(600, 300)
    log info node 'Repro' $Item/Name;
  end loop;
  @position(1300, 200)
  return;
end;`)

	name := flowQN("MyFirstModule.LoopFlow")
	g, err := storedFlow(exec.newExecContext(t.Context()), "microflow", name)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := exec.backend.GetRawUnitBytes(g.id)
	if err != nil {
		t.Fatal(err)
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	loop := findLoopD(doc)
	if loop == nil {
		t.Fatal("the created flow has no LoopedActivity")
	}
	if !setDString(loop, "Size", "710;480") {
		t.Fatal("the loop has no Size to make stale")
	}
	stale, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := exec.backend.UpdateRawUnit(string(g.id), stale); err != nil {
		t.Fatal(err)
	}
	if got := loopSizeOnDisk(t, newLayoutFlowExecutor(t, path), g); got != "710;480" {
		t.Fatalf("control: the stale size did not land, got %q", got)
	}

	res, err := exec.LayoutFlow("microflow", name, false)
	if err != nil || res.Refused != "" {
		t.Fatalf("layout: %v %s", err, res.Refused)
	}
	if got := loopSizeOnDisk(t, newLayoutFlowExecutor(t, path), g); got == "710;480" {
		t.Fatalf("layout flows reported moved %d but left the stale loop Size %q", res.Moved, got)
	}

	second, err := newLayoutFlowExecutor(t, path).LayoutFlow("microflow", name, false)
	if err != nil {
		t.Fatal(err)
	}
	if second.Changed() {
		t.Errorf("a second run moved %d objects and %d flows after the box was healed", second.Moved, second.Flows)
	}
}

func newLayoutFlowExecutor(t *testing.T, path string) *Executor {
	t.Helper()
	exec := New(&bytes.Buffer{})
	exec.SetQuiet(true)
	exec.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	t.Cleanup(func() { exec.Close() })
	run(t, exec, "CONNECT LOCAL '"+visitor.QuoteString(path)+"'")
	return exec
}

func findLoopD(node any) bson.D {
	switch v := node.(type) {
	case bson.D:
		for _, e := range v {
			if e.Key == "$Type" {
				if s, ok := e.Value.(string); ok && s == "Microflows$LoopedActivity" {
					return v
				}
			}
		}
		for _, e := range v {
			if loop := findLoopD(e.Value); loop != nil {
				return loop
			}
		}
	case bson.A:
		for _, item := range v {
			if loop := findLoopD(item); loop != nil {
				return loop
			}
		}
	}
	return nil
}

func setDString(d bson.D, key, value string) bool {
	for i, e := range d {
		if e.Key != key {
			continue
		}
		d[i].Value = value
		return true
	}
	return false
}

func getDString(d bson.D, key string) string {
	for _, e := range d {
		if e.Key == key {
			s, _ := e.Value.(string)
			return s
		}
	}
	return ""
}

func loopSizeOnDisk(t *testing.T, exec *Executor, g *layoutGraph) string {
	t.Helper()
	raw, err := exec.backend.GetRawUnitBytes(g.id)
	if err != nil {
		t.Fatal(err)
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	loop := findLoopD(doc)
	if loop == nil {
		t.Fatal("the flow lost its LoopedActivity")
	}
	return getDString(loop, "Size")
}
