// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"path/filepath"
	"testing"

	"github.com/mendixlabs/mxcli/internal/testutil"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// mendixlabs/mxcli#1410: on a Mendix 9 project every flow write was refused —
//
//	this Mendix 9.24.2 project cannot store Microflows$Microflow.StableId
//	(Mendix 10.2.0+); Microflows$SequenceFlow.CaseValues (Mendix 10.15.0+)
//
// The flow writers emitted the newest shape whatever the project's version:
// a StableId (introduced 10.2.0), the CaseValues list (introduced 10.15.0,
// replacing the single case stored as NewCaseValue) and a BezierCurve Line
// (introduced 10.16.0, replacing Origin/DestinationBezierVector). The property
// floor (canon.StripUndeclaredProperties) is right to refuse a key the version
// does not declare when it holds a value; the writer has to emit the shape the
// version has. The pre-10.15 shape is measured from a Studio Pro 9.24.30
// document (Administration.ChangeMyPassword):
//
//	{"$Type": "Microflows$SequenceFlow", "DestinationBezierVector": "-30;0",
//	 "DestinationConnectionIndex": 3, "DestinationPointer": …, "IsErrorHandler": false,
//	 "NewCaseValue": {"$Type": "Microflows$EnumerationCase", "Value": "true"},
//	 "OriginBezierVector": "15;0", "OriginConnectionIndex": 1, "OriginPointer": …}
//
// and the version floors are the Model SDK's (mendixmodelsdk 4.116.0,
// StructureVersionInfo): Microflow.stableId 10.2.0, SequenceFlow.caseValue
// deleted / caseValues introduced 10.15.0, Flow.line 10.16.0.

// copyV9Fixture copies the Mendix 9.24 v1 project the modelsdk/mpr tests use.
func copyV9Fixture(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	if err := testutil.CopyFixture(dst, "../../../modelsdk/mpr/testdata/v1-project"); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	return filepath.Join(dst, "App.mpr")
}

// decisionFlow builds start -> decision -(true)-> end, -(false)-> end, with a
// hand-drawn curve on the true branch.
func decisionFlow() *microflows.MicroflowObjectCollection {
	obj := func(id string, x int) microflows.BaseMicroflowObject {
		return microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(id)},
			Position:    model.Point{X: x, Y: 100},
			Size:        model.Size{Width: 30, Height: 30},
		}
	}
	start := &microflows.StartEvent{BaseMicroflowObject: obj("a1410000-0000-0000-0000-000000000001", 0)}
	split := &microflows.ExclusiveSplit{
		BaseMicroflowObject: obj("a1410000-0000-0000-0000-000000000002", 100),
		Caption:             "big?",
		SplitCondition:      &microflows.ExpressionSplitCondition{Expression: "1 > 0"},
	}
	endT := &microflows.EndEvent{BaseMicroflowObject: obj("a1410000-0000-0000-0000-000000000003", 200)}
	endF := &microflows.EndEvent{BaseMicroflowObject: obj("a1410000-0000-0000-0000-000000000004", 300)}
	flow := func(id, from, to string, cv microflows.CaseValue) *microflows.SequenceFlow {
		return &microflows.SequenceFlow{
			BaseElement: model.BaseElement{ID: model.ID(id)},
			OriginID:    model.ID(from), DestinationID: model.ID(to),
			OriginConnectionIndex: 1, DestinationConnectionIndex: 3,
			CaseValue: cv,
		}
	}
	curved := flow("a1410000-0000-0000-0000-000000000012", string(split.ID), string(endT.ID), &microflows.EnumerationCase{Value: "true"})
	curved.OriginControlVector, curved.DestinationControlVector = "15;0", "-30;0"
	return &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{start, split, endT, endF},
		Flows: []*microflows.SequenceFlow{
			flow("a1410000-0000-0000-0000-000000000011", string(start.ID), string(split.ID), nil),
			curved,
			flow("a1410000-0000-0000-0000-000000000013", string(split.ID), string(endF.ID), &microflows.EnumerationCase{Value: "false"}),
		},
	}
}

// assertPre1015Flows checks every sequence flow of a stored flow document has
// the Mendix 9 shape: a single NewCaseValue, bezier vectors, no Line.
func assertPre1015Flows(t *testing.T, raw bson.Raw, wantCases map[string]string) {
	t.Helper()
	flows, ok := raw.Lookup("Flows").ArrayOK()
	if !ok {
		t.Fatalf("no Flows list")
	}
	vals, _ := flows.Values()
	n := 0
	for _, v := range vals {
		d, ok := v.DocumentOK()
		if !ok {
			continue
		}
		if typ, _ := d.Lookup("$Type").StringValueOK(); typ != "Microflows$SequenceFlow" {
			continue
		}
		n++
		for _, key := range []string{"CaseValues", "Line", "LineType"} {
			if _, err := d.LookupErr(key); err == nil {
				t.Errorf("sequence flow carries %s, which Mendix 9 does not declare", key)
			}
		}
		cv, ok := d.Lookup("NewCaseValue").DocumentOK()
		if !ok {
			t.Errorf("sequence flow has no NewCaseValue document — the case Mendix 9 stores")
			continue
		}
		for _, key := range []string{"OriginBezierVector", "DestinationBezierVector"} {
			if _, ok := d.Lookup(key).StringValueOK(); !ok {
				t.Errorf("sequence flow has no %s", key)
			}
		}
		id := blobID(d.Lookup("$ID"))
		typ, _ := cv.Lookup("$Type").StringValueOK()
		val, _ := cv.Lookup("Value").StringValueOK()
		if want, ok := wantCases[id]; ok {
			if typ != "Microflows$EnumerationCase" || val != want {
				t.Errorf("flow %s case = %s %q, want EnumerationCase %q", id, typ, val, want)
			}
			if want == "true" {
				ov, _ := d.Lookup("OriginBezierVector").StringValueOK()
				dv, _ := d.Lookup("DestinationBezierVector").StringValueOK()
				if ov != "15;0" || dv != "-30;0" {
					t.Errorf("curved flow vectors = %q/%q, want 15;0/-30;0", ov, dv)
				}
			}
		} else if typ != "Microflows$NoCase" {
			t.Errorf("flow %s case = %s, want NoCase", id, typ)
		}
	}
	if n != 3 {
		t.Errorf("stored %d sequence flows, want 3", n)
	}
}

func blobID(v bson.RawValue) string {
	_, data, ok := v.BinaryOK()
	if !ok {
		return ""
	}
	return types.BlobToUUID(data)
}

func TestFlowWritesOnMendix9_1410(t *testing.T) {
	b := New()
	if err := b.Connect(copyV9Fixture(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	if pv := b.ProjectVersion(); pv == nil || pv.MajorVersion != 9 {
		t.Fatalf("fixture version = %+v, want a Mendix 9 project", pv)
	}
	mod, err := b.GetModuleByName("NanoflowCommons")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	cases := map[string]string{
		"a1410000-0000-0000-0000-000000000012": "true",
		"a1410000-0000-0000-0000-000000000013": "false",
	}

	t.Run("create microflow", func(t *testing.T) {
		mf := &microflows.Microflow{Name: "MF_Decide1410", ObjectCollection: decisionFlow()}
		mf.ContainerID = mod.ID
		if err := b.CreateMicroflow(mf); err != nil {
			t.Fatalf("CreateMicroflow on Mendix 9: %v", err)
		}
		raw, err := b.GetRawUnitBytes(mf.ID)
		if err != nil {
			t.Fatalf("GetRawUnitBytes: %v", err)
		}
		if _, err := bson.Raw(raw).LookupErr("StableId"); err == nil {
			t.Errorf("microflow carries StableId, introduced in Mendix 10.2.0")
		}
		assertPre1015Flows(t, raw, cases)

		// It reads back as written: the case and the curve.
		got, err := b.GetMicroflow(mf.ID)
		if err != nil {
			t.Fatalf("GetMicroflow: %v", err)
		}
		for _, f := range got.ObjectCollection.Flows {
			want, ok := cases[string(f.ID)]
			if !ok {
				continue
			}
			ec, isEnum := f.CaseValue.(*microflows.EnumerationCase)
			if !isEnum || ec.Value != want {
				t.Errorf("flow %s reads back with case %#v, want %q", f.ID, f.CaseValue, want)
			}
			if want == "true" && (f.OriginControlVector != "15;0" || f.DestinationControlVector != "-30;0") {
				t.Errorf("curved flow reads back as %q/%q, want 15;0/-30;0", f.OriginControlVector, f.DestinationControlVector)
			}
		}
	})

	t.Run("create nanoflow", func(t *testing.T) {
		nf := &microflows.Nanoflow{Name: "NF_Decide1410", ObjectCollection: decisionFlow()}
		nf.ContainerID = mod.ID
		if err := b.CreateNanoflow(nf); err != nil {
			t.Fatalf("CreateNanoflow on Mendix 9: %v", err)
		}
		raw, err := b.GetRawUnitBytes(nf.ID)
		if err != nil {
			t.Fatalf("GetRawUnitBytes: %v", err)
		}
		assertPre1015Flows(t, raw, cases)
	})

	t.Run("alter splices a Mendix 9 flow", func(t *testing.T) {
		// The splice encodes each new flow with this, and setCase moves the
		// case of a stored flow under the key this writes.
		d, err := codecMicroflowDeps{b: b}.SerializeSequenceFlow(&microflows.SequenceFlow{
			BaseElement: model.BaseElement{ID: model.ID("a1410000-0000-0000-0000-000000000021")},
			CaseValue:   &microflows.EnumerationCase{Value: "true"},
		})
		if err != nil {
			t.Fatalf("SerializeSequenceFlow: %v", err)
		}
		keys := map[string]bool{}
		for _, e := range d {
			keys[e.Key] = true
		}
		if keys["CaseValues"] || keys["Line"] || !keys["NewCaseValue"] || !keys["OriginBezierVector"] {
			t.Errorf("spliced flow keys = %v, want NewCaseValue and the bezier vectors, no CaseValues/Line", keys)
		}
	})
}
