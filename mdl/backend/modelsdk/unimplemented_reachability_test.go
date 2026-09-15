// SPDX-License-Identifier: Apache-2.0

// Can a run still hit errUnimplemented?
//
// Counting unimplemented methods answers that wrongly: 19 of FullBackend's 276
// methods were not declared on *Backend, but 17 of them were interface surface
// nothing calls through a backend value, so their stub could never fire.
// Measured with scripts/backend-reachability.sh, which removes one method from
// its interface at a time and rebuilds — grep cannot tell `b.reader.X()` inside
// a backend from `ctx.Backend.X()` in the executor.
//
// WHY A METHOD IS UNREACHABLE — three causes, and they want opposite fixes.
// The reason column records which, because reading them all as one is how this
// list gets mismanaged:
//
//  1. BYPASS — a caller wants the method but holds a concrete sdk/mpr reader or
//     writer instead of a backend value. The fix is to route that caller through
//     the abstraction, which makes the method reachable and therefore worth
//     implementing. AddAttribute and UpdateAttribute left the list this way when
//     api/ was ported; GetDomainModelByID, GetWorkflow and ListNavigationDocuments
//     when the MCP backend composed this one for its reads; ExportJSON when the
//     root package (modelsdk.go) stopped aliasing sdk/mpr and examples/
//     read_project moved onto a backend value (Phase 4a).
//  2. ORPHAN — nothing anywhere calls it, under any type. There is no bypass to
//     close, so the fix is to DELETE it from the interface.
//  3. DUPLICATE — callers exist, but through a narrower package-local interface
//     with a different signature, so the FullBackend copy is vestigial. Also a
//     delete, and the easiest of the three to mistake for a bypass: the name has
//     plenty of call sites.
//
// docs/plans/2026-09-14-retire-legacy-engine.md's Phase 3 write-up asserted the
// list "shrinks by closing a bypass, never by deleting methods". That holds for
// (1) and is wrong for (2) and (3): the whole WidgetSerializationBackend
// interface (SerializeWidget, SerializeClientAction, SerializeDataSource,
// SerializeWorkflowActivity), GetUnitTypes and UpdateLayout were measured DEAD
// with either no caller at all or callers using a different signature —
// SerializeWidget/DataSource superseded by WidgetBuilderBackend's *ToOpaque
// pair, whose own comment says so, and SerializeWorkflowActivity by wfmutator's
// deps. They were deleted rather than ported, taking the list from 11 to 6.
//
// So: establish the cause before acting on an entry. The probe says DEAD for all
// three, which is exactly why it cannot tell them apart — grep for callers under
// ANY type is what separates (1) from (2) and (3).
//
// This test does not repeat the probe (a build per method takes minutes). It
// pins its OUTPUT: the set of methods *Backend leaves to the stub must be
// exactly the set measured unreachable. A new stub, or a rename that drops an
// implementation, fails here and is a prompt to re-run the script rather than to
// extend the list on faith.
package modelsdkbackend

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
)

// unreachableUnimplemented are the FullBackend methods *Backend does not
// implement, each measured to have no caller through a backend value.
//
// The reason column is what the probe found, not a guess: for all but one it is
// that only the MPR backend's own delegation mentions the name, plus callers
// that hold a concrete *mpr.Reader / *mpr.Writer (the api/ package, examples/,
// and cmd/mxcli commands that open a reader directly) — none of which route
// through this engine.
// Every remaining entry is a BYPASS, and all five are the same kind of caller:
// a raw-unit debugging or export command that holds a concrete reader on
// purpose. ACCEPTED as deliberate rather than queued for porting (2026-09-15,
// closing Phase 3 of docs/plans/2026-09-14-retire-legacy-engine.md) — routing a
// raw-BSON dumper through an interface that speaks the semantic model would
// either widen that interface with raw accessors or make the tools worse at the
// one thing they exist for.
//
// So this map is now a STANDING RECORD, not a to-do list. A new entry is still
// a finding: it means a NEW bypass appeared, or a method was added to
// FullBackend that nothing calls — establish which (see the three causes above)
// rather than adding a row to match the failure.
var unreachableUnimplemented = map[string]string{
	"FindAllCustomWidgetTypes": "BYPASS, accepted: reached only via the reader, inside modelsdk/mpr itself",
	"GetProjectRootID":         "BYPASS, accepted: callers hold a reader; this package uses b.reader.GetProjectRootID directly",
	"ListAllUnitIDs":           "BYPASS, accepted: cmd/mxcli/diag.go holds a concrete reader; infrastructure_write.go uses b.reader",
	"ListRawUnits":             "BYPASS, accepted: the bson dump/discover/describe commands hold a concrete reader",
}

func TestNoReachableUnimplementedBackendMethods(t *testing.T) {
	declared := methodsDeclaredOnBackend(t)
	iface := reflect.TypeOf((*backend.FullBackend)(nil)).Elem()

	var missing []string
	for i := 0; i < iface.NumMethod(); i++ {
		if name := iface.Method(i).Name; !declared[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 && len(unreachableUnimplemented) > 0 {
		t.Fatalf("*Backend now implements everything, but %d methods are still listed as "+
			"unreachable — delete unreachableUnimplemented rather than leave it stale",
			len(unreachableUnimplemented))
	}
	sort.Strings(missing)

	var unexpected []string
	for _, name := range missing {
		if unreachableUnimplemented[name] == "" {
			unexpected = append(unexpected, name)
		}
	}
	if len(unexpected) > 0 {
		t.Errorf("these FullBackend methods fall through to the errUnimplemented stub and are not\n"+
			"recorded as unreachable:\n  %s\n"+
			"Run `scripts/backend-reachability.sh %s`. If it says LIVE, implement the method —\n"+
			"a default-engine run can reach it and will be told to rerun on an engine that is\n"+
			"being retired. If it says DEAD, add it to unreachableUnimplemented with that reason.",
			strings.Join(unexpected, "\n  "), strings.Join(unexpected, " "))
	}

	present := map[string]bool{}
	for _, name := range missing {
		present[name] = true
	}
	var stale []string
	for name := range unreachableUnimplemented {
		if !present[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("unreachableUnimplemented lists %v, which *Backend now implements — "+
			"strike them off, or the list stops meaning anything", stale)
	}
}

// TestTheTwoReachableMethodsAreImplemented is the control for the list above.
//
// Without it the test would pass just as well against a build where nothing is
// implemented and everything is listed as unreachable. These two are the ones
// the probe found LIVE, called from mdl/executor/cmd_microflows_builder.go, and
// they must never go back on the list.
func TestTheTwoReachableMethodsAreImplemented(t *testing.T) {
	declared := methodsDeclaredOnBackend(t)
	for _, name := range []string{"GetRawUnitByName", "ParseMicroflowBSON"} {
		if !declared[name] {
			t.Errorf("%s is reachable from the executor but not implemented — a default-engine "+
				"run hits errUnimplemented and falls back to an O(n) module walk", name)
		}
		if unreachableUnimplemented[name] != "" {
			t.Errorf("%s is listed as unreachable; the probe found four call sites in "+
				"mdl/executor/cmd_microflows_builder.go", name)
		}
	}
}
