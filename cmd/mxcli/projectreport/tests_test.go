// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCallsInMDL(t *testing.T) {
	got := CallsInMDL("$a = call microflow Shop.ACT_A($x);\nCALL MICROFLOW Shop.SUB_B();\n$c = call microflow Shop.ACT_A();", []string{"Shop.Setup"})
	if !reflect.DeepEqual(got, []string{"Shop.Setup", "Shop.ACT_A", "Shop.SUB_B"}) {
		t.Errorf("got %v", got)
	}
}

func TestReadJUnitAssertionsAndStatus(t *testing.T) {
	p := filepath.Join(t.TempDir(), "r.xml")
	_ = os.WriteFile(p, []byte(`<testsuites><testsuite name="s">
<testcase name="a" time="0.5"><properties><property name="assertions" value="6"/></properties></testcase>
<testcase name="b"><failure message="expected 2&#10;got 3">x</failure></testcase>
<testcase name="c"><skipped/></testcase></testsuite></testsuites>`), 0o644)
	rs, err := ReadJUnit(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 3 || rs[0].Assertions != 6 || rs[1].Status != "failed" || rs[1].Message != "expected 2" || rs[2].Status != "skipped" {
		t.Errorf("%+v", rs)
	}
}

func TestCoverageCategories(t *testing.T) {
	f := newFixture(t)
	f.module("Shop", "")
	for _, m := range []string{"Shop.Both", "Shop.TestOnly", "Shop.Callee", "Shop.JourneyOnly", "Shop.Startup", "Shop.StartupCallee", "Shop.Scheduled", "Shop.Nothing"} {
		f.microflow(m, true, "")
	}
	f.call("Shop.TestOnly", "Shop.Callee")
	f.call("Shop.Startup", "Shop.StartupCallee")
	f.exec(`INSERT INTO scheduled_events_data (Id, Name, QualifiedName, ModuleName, Microflow) VALUES ('se', 'Nightly', 'Shop.Nightly', 'Shop', 'Shop.Scheduled')`)

	t0 := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	journeys := &JourneyLog{Database: "fresh seed", Records: []JourneyRecord{{Journey: "Order", Start: t0.Format(time.RFC3339Nano),
		End: t0.Add(5 * time.Second).Format(time.RFC3339Nano), Status: "passed", Checks: []string{"order placed"}}}}
	at := func(s int) int64 { return t0.Add(time.Duration(s) * time.Second).UnixNano() }
	spans := []Span{
		{SpanID: "1", Name: "Microflow Shop.Both", Start: at(1), End: at(2)},
		{SpanID: "2", Name: "Microflow Shop.JourneyOnly", Start: at(2), End: at(3)},
		{SpanID: "3", Name: "Microflow Shop.Nothing", Start: at(9), End: at(10)}, // outside the window
	}
	d, err := CollectTests(f.cat, TestsInput{
		Sources:      []TestSource{{Name: "t1", Calls: []string{"Shop.TestOnly", "Shop.Both"}}, {Name: "t2", Calls: []string{"Shop.Renamed"}}},
		Results:      []TestResult{{Name: "t1", Status: "passed", Assertions: 2}},
		Journeys:     journeys,
		Spans:        spans,
		StartupFlows: []string{"Shop.Startup"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"Shop.Both": ReachBoth, "Shop.TestOnly": ReachTests, "Shop.Callee": ReachTests, "Shop.JourneyOnly": ReachJourneys,
		"Shop.Startup": ReachStartup, "Shop.StartupCallee": ReachStartup, "Shop.Scheduled": ReachStartup, "Shop.Nothing": ReachNeither,
	}
	if !reflect.DeepEqual(d.Reach, want) {
		t.Errorf("reach = %v\nwant %v", d.Reach, want)
	}
	if d.Totals.Neither != 1 || len(d.Gaps) != 1 || d.Gaps[0].Microflows[0] != "Shop.Nothing" {
		t.Errorf("gaps = %+v totals = %+v", d.Gaps, d.Totals)
	}
	if !reflect.DeepEqual(d.UnknownCalls, []string{"Shop.Renamed"}) {
		t.Errorf("unknown calls = %v", d.UnknownCalls)
	}
	if d.Tests[1].Status != "not run" {
		t.Errorf("a test with no result is not run, got %q", d.Tests[1].Status)
	}
}

// Unreached microflows are grouped by kind, startup and scheduled work is
// listed apart, and each journey shows what only it runs (ako/mxcli#2443).
func TestGapKindsAndWhatOnlyOneJourneyRuns(t *testing.T) {
	f := newFixture(t)
	f.module("Shop", "")
	for _, m := range []string{"Shop.ACT_Pay", "Shop.SUB_Total", "Shop.ACT_Order", "Shop.Seed_Pizzas", "Shop.Old",
		"Shop.Startup", "Shop.Scheduled", "Shop.Shared", "Shop.OnlyOrder"} {
		f.microflow(m, true, "")
	}
	f.permission("Shop.User", "MICROFLOW", "Shop.ACT_Pay", "EXECUTE", "")
	f.permission("Shop.User", "MICROFLOW", "Shop.ACT_Order", "EXECUTE", "")
	f.call("Shop.ACT_Order", "Shop.SUB_Total")
	f.exec(`INSERT INTO scheduled_events_data (Id, Name, QualifiedName, ModuleName, Microflow) VALUES ('se', 'Nightly', 'Shop.Nightly', 'Shop', 'Shop.Scheduled')`)

	t0 := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	at := func(s int) int64 { return t0.Add(time.Duration(s) * time.Second).UnixNano() }
	win := func(name string, from int) JourneyRecord {
		return JourneyRecord{Journey: name, Start: t0.Add(time.Duration(from) * time.Second).Format(time.RFC3339Nano),
			End: t0.Add(time.Duration(from+5) * time.Second).Format(time.RFC3339Nano), Status: "passed"}
	}
	d, err := CollectTests(f.cat, TestsInput{
		Journeys: &JourneyLog{Records: []JourneyRecord{win("Order", 0), win("Browse", 10)}},
		Spans: []Span{
			{SpanID: "1", Name: "Microflow Shop.Shared", Start: at(1), End: at(2)},
			{SpanID: "2", Name: "Microflow Shop.OnlyOrder", Start: at(2), End: at(3)},
			{SpanID: "3", Name: "Microflow Shop.Shared", Start: at(11), End: at(12)},
		},
		StartupFlows: []string{"Shop.Startup"},
	})
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string][]string{}
	for _, k := range d.GapKinds {
		kinds[k.Kind] = k.Microflows
	}
	want := map[string][]string{
		GapCallable:   {"Shop.ACT_Order", "Shop.ACT_Pay"},
		GapSub:        {"Shop.SUB_Total"},
		GapSeed:       {"Shop.Seed_Pizzas"},
		GapUnreferred: {"Shop.Old"},
	}
	if !reflect.DeepEqual(kinds, want) {
		t.Errorf("gap kinds = %v\nwant %v", kinds, want)
	}
	if !reflect.DeepEqual(d.Startup, []string{"Shop.Startup"}) || !reflect.DeepEqual(d.Scheduled, []string{"Shop.Scheduled"}) {
		t.Errorf("startup %v, scheduled %v", d.Startup, d.Scheduled)
	}

	env, _ := NewEnvelope("tests", "Shop", "now", "test", d, TestsMethod, TestsLimits, nil)
	doc, err := RenderTests(env, nil)
	if err != nil {
		t.Fatal(err)
	}
	var md bytes.Buffer
	_ = RenderMarkdown(&md, doc)
	mustContain(t, md.String(), "| Order | 1 | `Shop.OnlyOrder` |", "| a scheduled event | 1 | `Shop.Scheduled` |",
		"| seed or demo data | 1 | `Shop.Seed_Pizzas` |")
	if strings.Contains(md.String(), "| Browse | 1 | `") {
		t.Error("Browse runs nothing no other journey runs, so it has no row")
	}
}
