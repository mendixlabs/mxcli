// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ako/mxcli#2055: describe → exec of TestApp's WorkflowCommons.ManageTaskAssignments
// was refused — "snippet …Snip_TaskAssignment_UserTask_Assignees requires
// parameter $Account". The call sits in a Data grid 2 of System.WorkflowUserTask
// inside a data view of System.User; Studio Pro stores it with NO parameter
// mappings, and `mx check` (11.14) reports 0 errors: a snippet parameter the
// call omits is taken from an enclosing data container, not only the nearest
// one. The builder looked at the nearest only. PedApp has no Data grid 2
// package, so the test nests a list view; the TestApp page round-trips too.

const outerContextSetup = `create module Snip2055;
create persistent entity Snip2055.Item (Name: String(50));
create snippet Snip2055.ItemAndAccount (Params: ( $Item: Snip2055.Item, $Account: Administration.Account )) {
  dynamictext txt1 (Content: 'x')
}
`

func snippetCallPage(name, outerParamEntity string) string {
	return `create page Snip2055.` + name + ` (
  Title: 'x',
  Layout: Atlas_Core.Atlas_Default,
  Params: ( $Outer: ` + outerParamEntity + ` )
) {
  dataview dvOuter (DataSource: $Outer) {
    listview list1 (DataSource: database from Snip2055.Item) {
      snippetcall sc1 (Snippet: Snip2055.ItemAndAccount)
    }
  }
}`
}

func TestSnippetCall_ParameterFromAnEnclosingDataContainer(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	if err := afRun(t, exec, outerContextSetup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	// The enclosing data view supplies $Account itself, and — as in
	// WorkflowCommons — its generalisation System.User.
	for name, outer := range map[string]string{"ByAccount": "Administration.Account", "ByUser": "System.User"} {
		out.Reset()
		if err := afRun(t, exec, snippetCallPage(name, outer)); err != nil {
			t.Errorf("%s: a snippet parameter an enclosing %s data view supplies was refused: %v\n%s", name, outer, err, out.String())
		}
	}

	// Control: nothing around the grid supplies $Account — still refused.
	out.Reset()
	err := afRun(t, exec, snippetCallPage("NoAccount", "Snip2055.Item"))
	if err == nil || !strings.Contains(err.Error()+out.String(), "requires parameter $Account") {
		t.Errorf("control: a call with no context for $Account was accepted: %v\n%s", err, out.String())
	}
}

// The same page then failed mx check once it built: dataView2's source,
// `$currentObject/WorkflowCommons.TaskAssignmentHelper_User`, is stored with
// the DestinationEntity Administration.Account — a specialisation of the
// association's System.User end — and describe printed the single hop without
// it. The rebuild arrived at System.User, and the labels inside, bound to
// Administration.Account's FullName, became CE1613 "System.User.FullName no
// longer exists" (11.14). A single hop whose stored destination is neither end
// of its association prints the destination.
func TestDescribeAssociationSource_KeepsASpecialisedDestination(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	setup := `create module Spec2055;
create persistent entity Spec2055.Holder (Name: String(20));
create association Spec2055.Holder_User from Spec2055.Holder to System.User;
create page Spec2055.HolderPage (
  Title: 'x',
  Layout: Atlas_Core.Atlas_Default,
  Params: ( $Holder: Spec2055.Holder )
) {
  dataview dvHolder (DataSource: $Holder) {
    dataview dvAccount (DataSource: $currentObject/Spec2055.Holder_User/Administration.Account) {
      dynamictext t1 (Content: '{1}', ContentParams: ({1} = FullName))
    }
    dataview dvUser (DataSource: $currentObject/Spec2055.Holder_User) {
      dynamictext t2 (Content: '{1}', ContentParams: ({1} = Name))
    }
  }
}`
	if err := afRun(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	out.Reset()
	if err := afRun(t, exec, "describe page Spec2055.HolderPage;"); err != nil {
		t.Fatalf("describe: %v\n%s", err, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "DataSource: $currentObject/Spec2055.Holder_User/Administration.Account") {
		t.Errorf("the specialised destination was dropped:\n%s", got)
	}
	// Control: a hop that arrives at the association's own end stays short.
	if !strings.Contains(got, "DataSource: $currentObject/Spec2055.Holder_User,") && !strings.Contains(got, "DataSource: $currentObject/Spec2055.Holder_User\n") && !strings.Contains(got, "DataSource: $currentObject/Spec2055.Holder_User)") {
		t.Errorf("control: the plain hop is not printed short:\n%s", got)
	}
}
