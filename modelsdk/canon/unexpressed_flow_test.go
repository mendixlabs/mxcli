// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#2178 (mendixlabs/mxcli#1831): `drop microflow` + `create` in one
// script rebuilt the flow in place — the writer carries the deleted unit's
// bytes to the re-insert, and the transplant kept every element $ID — but each
// property MDL has no spelling for came back at the writer's default:
//
//	Microflow MarkAsUsed           true                     → false
//	Parameter Documentation        "Parameter documentation" → ""
//	Activity Documentation         "Activity documentation"  → ""
//	Loop ErrorHandlingType         Continue                 → Rollback
//
// The last one changes behaviour: one failing iteration now aborts the loop.

type flowProps struct {
	markAsUsed                      bool
	paramDoc, activityDoc, loopDoc  string
	splitDoc, loopErrors, proxyType string
	proxy                           any
	logText                         string
}

func flowUnit(p flowProps) []byte {
	param := bson.D{{Key: "$ID", Value: id16(0x10)}, {Key: "$Type", Value: "Microflows$MicroflowParameter"},
		{Key: "Documentation", Value: p.paramDoc}, {Key: "Name", Value: "Count"}}
	activity := bson.D{{Key: "$ID", Value: id16(0x11)}, {Key: "$Type", Value: "Microflows$ActionActivity"},
		{Key: "Action", Value: bson.D{{Key: "$ID", Value: id16(0x12)}, {Key: "$Type", Value: "Microflows$RestCallAction"},
			{Key: "ProxyConfiguration", Value: p.proxy}, {Key: "RequestProxyType", Value: p.proxyType}}},
		{Key: "Documentation", Value: p.activityDoc}}
	loop := bson.D{{Key: "$ID", Value: id16(0x13)}, {Key: "$Type", Value: "Microflows$LoopedActivity"},
		{Key: "Documentation", Value: p.loopDoc}, {Key: "ErrorHandlingType", Value: p.loopErrors}}
	split := bson.D{{Key: "$ID", Value: id16(0x14)}, {Key: "$Type", Value: "Microflows$ExclusiveSplit"},
		{Key: "Caption", Value: p.logText}, {Key: "Documentation", Value: p.splitDoc}}
	b, _ := bson.Marshal(bson.D{
		{Key: "$ID", Value: id16(0xAA)}, {Key: "$Type", Value: "Microflows$Microflow"},
		{Key: "MarkAsUsed", Value: p.markAsUsed},
		{Key: "ObjectCollection", Value: bson.D{{Key: "$ID", Value: id16(0xAB)}, {Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
			{Key: "Objects", Value: bson.A{int32(3), param, activity, loop, split}}}},
	})
	return b
}

func overrideProxyDoc() bson.D {
	return bson.D{{Key: "$ID", Value: id16(0x20)}, {Key: "$Type", Value: "Microflows$ProxyConfiguration"},
		{Key: "HostExpression", Value: "'proxy.example'"}, {Key: "PortExpression", Value: "3128"}}
}

func storedFlow() flowProps {
	return flowProps{markAsUsed: true, paramDoc: "Parameter documentation", activityDoc: "Activity documentation",
		loopDoc: "Loop documentation", splitDoc: "Split documentation", loopErrors: "Continue",
		proxyType: "Override", proxy: overrideProxyDoc(), logText: "ok?"}
}

// What a rebuild from the statement writes: every unstatable property at the
// writer's default.
func rebuiltFlow() flowProps {
	return flowProps{loopErrors: "Rollback", proxyType: "DefaultProxy", logText: "ok?"}
}

func TestUnexpressed_FlowRebuildKeepsWhatMDLCannotState(t *testing.T) {
	stored := flowUnit(storedFlow())
	written := flowUnit(rebuiltFlow())
	if eq, _ := Equal(written, stored); eq {
		t.Fatal("control: the documents should differ before the carries")
	}
	out, unchanged := Reconcile(written, stored)
	if !unchanged {
		var got bson.D
		_ = bson.Unmarshal(out, &got)
		t.Errorf("a rebuild that stated nothing new was not elided; it wrote\n%v", got)
	}
}

// Control: a property MDL does state (here the decision's caption) still lands
// on the same rebuild, and the unstatable ones are carried beside it.
func TestUnexpressed_FlowRebuildStillLandsAStatedChange(t *testing.T) {
	stored := flowUnit(storedFlow())
	p := rebuiltFlow()
	p.logText = "really ok?"
	out, unchanged := Reconcile(flowUnit(p), stored)
	if unchanged {
		t.Fatal("a changed caption was elided")
	}
	want := storedFlow()
	want.logText = "really ok?"
	if eq, _ := Equal(out, flowUnit(want)); !eq {
		var got bson.D
		_ = bson.Unmarshal(out, &got)
		t.Errorf("want the stored unstatable values with the new caption, got\n%v", got)
	}
}

// Control: a value that differs from the writer's default came from the
// statement, and is not overwritten by the stored one.
func TestUnexpressed_FlowCarryLeavesANonDefaultValue(t *testing.T) {
	stored := flowUnit(storedFlow())
	p := rebuiltFlow()
	p.loopErrors = "Abort"
	out, _ := Reconcile(flowUnit(p), stored)
	var got bson.D
	_ = bson.Unmarshal(out, &got)
	var loopErrors string
	walkElements(got, func(d bson.D) {
		if docLookup(d, "$Type") == "Microflows$LoopedActivity" {
			loopErrors, _ = docLookup(d, "ErrorHandlingType").(string)
		}
	})
	if loopErrors != "Abort" {
		t.Errorf("loop ErrorHandlingType = %q, want the written Abort", loopErrors)
	}
}
