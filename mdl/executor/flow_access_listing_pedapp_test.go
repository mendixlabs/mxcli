// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// ListFlowAccess reads a flow's name, Excluded flag and allowed module roles
// from the stored document's top level, where the flow listings decode and
// convert the whole flow for them. It must list what they list, in their
// order: compared on every microflow and nanoflow of the fixture, after a
// script has given one flow roles and excluded another, so that neither value
// is compared only in its empty form.
func TestListFlowAccess_ListsWhatTheFlowListingsList(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	const setup = `mdl 1;
create module role MyFirstModule.FlowAccessProbe;
create microflow MyFirstModule.FlowAccessGranted () returns boolean begin return true; end;
grant execute on microflow MyFirstModule.FlowAccessGranted to MyFirstModule.FlowAccessProbe;
@excluded
create microflow MyFirstModule.FlowAccessExcluded () returns boolean begin return true; end;
`
	if err := runScript(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	lister, ok := exec.Backend().(flowAccessLister)
	if !ok {
		t.Fatalf("backend %T does not list flow access", exec.Backend())
	}
	got, err := lister.ListFlowAccess()
	if err != nil {
		t.Fatalf("ListFlowAccess: %v", err)
	}

	var want []types.FlowAccess
	mfs, err := exec.Backend().ListMicroflows()
	if err != nil {
		t.Fatalf("ListMicroflows: %v", err)
	}
	for _, mf := range mfs {
		want = append(want, types.FlowAccess{Kind: "microflow", ContainerID: mf.ContainerID, Name: mf.Name, Excluded: mf.Excluded, AllowedModuleRoles: mf.AllowedModuleRoles})
	}
	nfs, err := exec.Backend().ListNanoflows()
	if err != nil {
		t.Fatalf("ListNanoflows: %v", err)
	}
	for _, nf := range nfs {
		want = append(want, types.FlowAccess{Kind: "nanoflow", ContainerID: nf.ContainerID, Name: nf.Name, Excluded: nf.Excluded, AllowedModuleRoles: nf.AllowedModuleRoles})
	}

	withRoles, excluded := 0, 0
	for _, f := range want {
		if len(f.AllowedModuleRoles) > 0 {
			withRoles++
		}
		if f.Excluded {
			excluded++
		}
	}
	t.Logf("%d microflows, %d nanoflows; %d with roles, %d excluded", len(mfs), len(nfs), withRoles, excluded)
	if len(mfs) == 0 || len(nfs) == 0 || withRoles == 0 || excluded == 0 {
		t.Fatalf("control: the fixture must hold microflows, nanoflows, a flow with roles and an excluded one")
	}
	if len(got) != len(want) {
		t.Fatalf("ListFlowAccess lists %d flows, the flow listings %d", len(got), len(want))
	}
	for i := range want {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("flow %d: ListFlowAccess gives %+v, the flow listing %+v", i, got[i], want[i])
		}
	}
}
