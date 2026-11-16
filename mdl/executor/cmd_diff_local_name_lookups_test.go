// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

const lookupFlows = 4

// lookupScript declares lookupFlows microflows, each calling the next, and a
// nanoflow, with text in their bodies.
func lookupScript(verb, text string) string {
	var sb strings.Builder
	for i := 0; i < lookupFlows; i++ {
		fmt.Fprintf(&sb, "%s microflow MyFirstModule.Lookup1873_%d () begin\n  log info node 'L' '%s %d';\n", verb, i, text, i)
		if i > 0 {
			fmt.Fprintf(&sb, "  call microflow MyFirstModule.Lookup1873_%d();\n", i-1)
		}
		sb.WriteString("end;\n")
	}
	fmt.Fprintf(&sb, "%s nanoflow MyFirstModule.Lookup1873_Nano () begin\n  log info node 'L' '%s nano';\nend;\n", verb, text)
	return sb.String()
}

// mendixlabs/mxcli#1873: diff-local renders both sides of every changed flow,
// and each rendering rebuilt the project's ID-to-name maps by decoding every
// microflow of the project. 850 changed microflows on 7,700 units took 21
// minutes. The maps are the same for the whole run and are built once.
//
// The first half is the control for the second: the diff must still render
// every flow, with the calls resolved to names.
func TestDiffLocal_BuildsItsNameLookupsOnce(t *testing.T) {
	before := nameLookupLoads.Load()
	out := diffLocalAfter(t, lookupScript("create", "before"), lookupScript("create or modify", "after"), DiffOptions{})
	loads := nameLookupLoads.Load() - before

	for i := 0; i < lookupFlows; i++ {
		for _, want := range []string{fmt.Sprintf("'before %d'", i), fmt.Sprintf("'after %d'", i)} {
			if !strings.Contains(out, want) {
				t.Fatalf("the diff does not render %s:\n%s", want, out)
			}
		}
	}
	if !strings.Contains(out, "'after nano'") || !strings.Contains(out, "call microflow MyFirstModule.Lookup1873_0") {
		t.Fatalf("the diff does not render the nanoflow or resolve a call:\n%s", out)
	}
	if !strings.Contains(out, fmt.Sprintf("%d modified", lookupFlows+1)) {
		t.Fatalf("the diff did not find %d modified flows:\n%s", lookupFlows+1, out)
	}
	// Ten renderings (two sides of five flows), one build.
	if loads != 1 {
		t.Fatalf("the name lookups were built %d times for %d changed flows, want once", loads, lookupFlows+1)
	}
}

// The maps now come from the flows' headers. They must be the maps decoding
// every flow gave, and outside a diff-local run nothing is kept.
func TestLoadNameLookups_FromHeadersIsWhatDecodingGave(t *testing.T) {
	var out bytes.Buffer
	x := New(&out)
	x.SetQuiet(true)
	x.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	t.Cleanup(func() { x.Close() })
	run(t, x, "CONNECT LOCAL '"+visitor.QuoteString(projectFixture(t))+"'")
	run(t, x, lookupScript("create", "x")+"@excluded\ncreate microflow MyFirstModule.Lookup1873_Excluded () begin end;\n")
	ctx := x.newExecContext(t.Context())

	entities, flows := loadNameLookups(ctx)
	wantEntities, wantFlows := loadNameLookupsDecoding(ctx)
	if !reflect.DeepEqual(entities, wantEntities) || !reflect.DeepEqual(flows, wantFlows) {
		t.Fatalf("the lookups differ: %d entities and %d flows from headers, %d and %d by decoding", len(entities), len(flows), len(wantEntities), len(wantFlows))
	}
	names := map[string]bool{}
	for _, qn := range flows {
		names[qn] = true
	}
	for _, want := range []string{"MyFirstModule.Lookup1873_0", "MyFirstModule.Lookup1873_Nano", "MyFirstModule.Lookup1873_Excluded"} {
		if !names[want] {
			t.Errorf("the lookups do not hold %s", want)
		}
	}
	if len(entities) == 0 {
		t.Error("no entity names: the comparison was of nothing")
	}

	before := nameLookupLoads.Load()
	buildNameLookups(ctx)
	buildNameLookups(ctx)
	if got := nameLookupLoads.Load() - before; got != 2 {
		t.Errorf("outside a diff-local run the lookups were built %d times for 2 calls: something is kept that a write could make stale", got)
	}
}
