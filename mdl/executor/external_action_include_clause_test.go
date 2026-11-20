// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#1914: `include associations ( … )` on a call external action
// argument writes the argument's Associations to include. The tree Studio Pro
// stored on ako/TestApp's Odata.IncludeAssociationsMF (11.16.0) is Quantity
// ticked, Unit ticked under it, both IsParent — each written from its FROM end.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func TestIncludeAssociations_BuildsStudioProTree(t *testing.T) {
	calls := rewriteExternalActionFlow(t, nil, `call external action Odata.Bug1914.CreateBuffer(
  command = $Cmd include associations (Odata.Quantity (Odata.Unit)),
  additional = empty);`)
	cmd := mappingNamed(calls[0], "command")
	if got, want := includedAssociationsShape(cmd.IncludedAssociations),
		includedAssociationsShape(storedIncludedAssociations()); got != want {
		t.Fatalf("tree = %s, want Studio Pro's %s", got, want)
	}
	for _, ia := range []*microflows.IncludedAssociation{cmd.IncludedAssociations[0], cmd.IncludedAssociations[0].IncludedAssociations[0]} {
		if ia.ID == "" {
			t.Errorf("%s has no $ID", ia.Association)
		}
	}
	if add := mappingNamed(calls[0], "additional"); len(add.IncludedAssociations) != 0 {
		t.Errorf("an argument without the clause got a tree: %+v", add.IncludedAssociations)
	}
}

// IsParent is false when the object is the association's TO end: Owner is from
// QuantityParameterType to CreateBufferCommand, and Unit is then reached from
// QuantityParameterType, Owner's far end.
func TestIncludeAssociations_IsParentFromTheToEnd(t *testing.T) {
	calls := rewriteExternalActionFlow(t, nil,
		`call external action Odata.Bug1914.CreateBuffer(command = $Cmd include associations (Odata.Owner (Odata.Unit)));`)
	tree := mappingNamed(calls[0], "command").IncludedAssociations
	if got := includedAssociationsShape(tree); got != "(Odata.Owner false (Odata.Unit true ))" {
		t.Errorf("tree = %s, want Owner from its TO end, Unit from QuantityParameterType", got)
	}
}

func TestIncludeAssociations_Refusals(t *testing.T) {
	for _, tc := range []struct{ name, clause, want string }{
		{"self-association", "(Odata.Self)", "Odata.Self is a self-association"},
		{"association that does not touch the entity", "(Odata.Elsewhere)",
			"Odata.Elsewhere does not touch Odata.CreateBufferCommand"},
		{"nested association that does not touch the far end", "(Odata.Quantity (Odata.Owner (Odata.Elsewhere)))",
			"Odata.Elsewhere does not touch Odata.CreateBufferCommand"},
		{"unknown association", "(Odata.NoSuch)", "include associations Odata.NoSuch: association not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := runExternalActionFlow(t, nil,
				"call external action Odata.Bug1914.CreateBuffer(command = $Cmd include associations "+tc.clause+");")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

// The clause is the whole tree: it replaces the stored one, `()` clears it,
// and neither warns. Left out, the stored tree is carried and exec warns.
func TestIncludeAssociations_ClauseOverStoredTree(t *testing.T) {
	t.Run("restated with less", func(t *testing.T) {
		calls, out, err := runExternalActionFlow(t, storedCreateBufferCall(),
			`call external action Odata.Bug1914.CreateBuffer(command = $Cmd include associations (Odata.Quantity), additional = empty);`)
		if err != nil {
			t.Fatal(err)
		}
		if got := includedAssociationsShape(mappingNamed(calls[0], "command").IncludedAssociations); got != "(Odata.Quantity true )" {
			t.Errorf("tree = %s, want Quantity alone — Unit removed", got)
		}
		if strings.Contains(out, carriedIncludesRule) {
			t.Errorf("a stated clause warned:\n%s", out)
		}
	})
	t.Run("empty clause clears", func(t *testing.T) {
		calls, out, err := runExternalActionFlow(t, storedCreateBufferCall(),
			`call external action Odata.Bug1914.CreateBuffer(command = $Cmd include associations (), additional = empty);`)
		if err != nil {
			t.Fatal(err)
		}
		if tree := mappingNamed(calls[0], "command").IncludedAssociations; len(tree) != 0 {
			t.Errorf("include associations () kept %s", includedAssociationsShape(tree))
		}
		if strings.Contains(out, carriedIncludesRule) {
			t.Errorf("an empty clause warned:\n%s", out)
		}
	})
	t.Run("omitted clause carries and warns", func(t *testing.T) {
		calls, out, err := runExternalActionFlow(t, storedCreateBufferCall(),
			`call external action Odata.Bug1914.CreateBuffer(command = $Cmd, additional = empty);`)
		if err != nil {
			t.Fatal(err)
		}
		if tree := mappingNamed(calls[0], "command").IncludedAssociations; len(tree) != 1 {
			t.Errorf("the stored tree was not carried: %s", includedAssociationsShape(tree))
		}
		for _, want := range []string{carriedIncludesRule, "Odata.Bug1914.CreateBuffer(command)",
			"(Odata.Quantity (Odata.Unit))", "include associations ()"} {
			if !strings.Contains(out, want) {
				t.Errorf("the warning does not say %q:\n%s", want, out)
			}
		}
	})
	t.Run("control: no stored tree, no clause, no warning", func(t *testing.T) {
		_, out, err := runExternalActionFlow(t, nil,
			`call external action Odata.Bug1914.CreateBuffer(command = $Cmd, additional = empty);`)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out, carriedIncludesRule) {
			t.Errorf("warned with nothing to carry:\n%s", out)
		}
	})
}

// describe prints the stored tree as the clause, so describe -> exec restates
// it; a tree holding a self-association is printed as a comment.
func TestIncludeAssociations_Describe(t *testing.T) {
	mod := mkModule("Odata")
	dm := includedAssocDomainModel(mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))

	got := formatAction(ctx, storedCreateBufferCall().Action, nil, nil)
	if want := "command = $Cmd include associations (Odata.Quantity (Odata.Unit)), additional = empty"; !strings.Contains(got, want) {
		t.Errorf("describe = %s\nwant it to contain %s", got, want)
	}

	// A self-association cannot be written back: printed as a comment, which
	// parses, so the statement still runs and the carry keeps the tree.
	self := storedCreateBufferCall()
	self.Action.(*microflows.CallExternalAction).ParameterMappings[0].IncludedAssociations =
		[]*microflows.IncludedAssociation{{Association: "Odata.Self", IsParent: true}}
	got = formatAction(ctx, self.Action, nil, nil)
	if strings.Contains(got, "$Cmd include associations") ||
		!strings.Contains(got, "/* include associations (Odata.Self): holds the self-association Odata.Self") {
		t.Errorf("describe = %s\nwant the self-association as a comment", got)
	}
	if _, errs := visitor.Build("create microflow Odata.F ($Cmd: Odata.CreateBufferCommand)\nbegin\n" + got + "\nend;"); len(errs) > 0 {
		t.Errorf("the described call does not parse: %v\n%s", errs, got)
	}
}

// A non-empty clause needs Microflows$IncludedAssociation, introduced in
// 10.11.0; `include associations ()` writes nothing and is not gated.
func TestIncludeAssociations_VersionGate(t *testing.T) {
	at := func(major, minor int) *ExecContext {
		ctx, _ := newMockCtx(t, withBackend(&mock.MockBackend{
			IsConnectedFunc: func() bool { return true },
			ProjectVersionFunc: func() *types.ProjectVersion {
				return &types.ProjectVersion{MajorVersion: major, MinorVersion: minor}
			},
		}))
		return ctx
	}
	tree := parseMicroflowStmt(t, "create microflow M.F ($Cmd: M.Cmd) begin\n"+
		"  call external action M.Svc.Act(command = $Cmd include associations (M.Assoc));\nend;")
	none := parseMicroflowStmt(t, "create microflow M.F ($Cmd: M.Cmd) begin\n"+
		"  call external action M.Svc.Act(command = $Cmd include associations ());\nend;")
	if _, err := buildMicroflowFromStmt(at(10, 10), tree, buildFlowOpts{}); err == nil ||
		!strings.Contains(err.Error(), "include associations") || !strings.Contains(err.Error(), "10.11") {
		t.Errorf("10.10: err = %v, want the include associations refusal naming 10.11", err)
	}
	if _, err := buildMicroflowFromStmt(at(10, 11), tree, buildFlowOpts{}); err != nil && strings.Contains(err.Error(), "10.11") {
		t.Errorf("10.11 satisfies the minimum; got: %v", err)
	}
	if _, err := buildMicroflowFromStmt(at(10, 10), none, buildFlowOpts{}); err != nil && strings.Contains(err.Error(), "include associations") {
		t.Errorf("an empty clause writes nothing and must not be gated; got: %v", err)
	}
}

// check -p says what exec will warn: an omitted clause over a stored tree.
func TestIncludeAssociations_CheckWarnsOnCarry(t *testing.T) {
	mod := mkModule("Odata")
	h := mkHierarchy(mod)
	mf := mkMicroflow(mod.ID, "ACT_Buffer")
	mf.ObjectCollection = &microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{storedCreateBufferCall()}}
	ctx, _ := newMockCtx(t, withBackend(&mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
	}), withHierarchy(h))
	run := func(script string) []string {
		prog, errs := visitor.Build(script)
		if len(errs) > 0 {
			t.Fatalf("parse: %v", errs)
		}
		var msgs []string
		for _, v := range CheckCarriedIncludedAssociations(ctx, prog) {
			msgs = append(msgs, v.RuleID+": "+v.Message)
		}
		return msgs
	}
	flow := func(args string) string {
		return "create or modify microflow Odata.ACT_Buffer ($Cmd: Odata.CreateBufferCommand)\nbegin\n" +
			"  call external action Odata.Bug1914.CreateBuffer(" + args + ");\nend;"
	}
	if got := run(flow("command = $Cmd, additional = empty")); len(got) != 1 ||
		!strings.Contains(got[0], carriedIncludesRule) || !strings.Contains(got[0], "(Odata.Quantity (Odata.Unit))") {
		t.Errorf("omitted clause: %q, want one MDL-ODATA10 naming the stored tree", got)
	}
	for name, script := range map[string]string{
		"stated":         flow("command = $Cmd include associations (Odata.Quantity), additional = empty"),
		"stated empty":   flow("command = $Cmd include associations (), additional = empty"),
		"drop + create":  "drop microflow Odata.ACT_Buffer;\n" + strings.Replace(flow("command = $Cmd"), "create or modify", "create", 1),
		"another action": strings.Replace(flow("command = $Other"), "Bug1914.CreateBuffer(", "Bug1914.CreateOther(", 1),
	} {
		if got := run(script); len(got) != 0 {
			t.Errorf("%s: %q, want no warning", name, got)
		}
	}
}

// check refuses what exec's builder refuses, against the same contract and
// domain model — and leaves an association the project does not hold to exec,
// since an earlier statement may create it.
func TestIncludeAssociations_CheckAgreesWithExec(t *testing.T) {
	mod := mkModule("Odata")
	dm := includedAssocDomainModel(mod.ID)
	ctx, _ := newMockCtx(t, withBackend(&mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if name == mod.Name {
				return mod, nil
			}
			return nil, nil
		},
		GetDomainModelFunc:   func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) {
			return []*model.ConsumedODataService{{BaseElement: model.BaseElement{ID: "svc"}, ContainerID: mod.ID,
				Name: "Bug1914", Metadata: includedAssocMetadata}}, nil
		},
	}), withHierarchy(mkHierarchy(mod)))
	check := func(clause string) string {
		prog, errs := visitor.Build("create microflow Odata.F ($Cmd: Odata.CreateBufferCommand)\nbegin\n" +
			"  call external action Odata.Bug1914.CreateBuffer(command = $Cmd include associations " + clause + ");\nend;")
		if len(errs) > 0 {
			t.Fatalf("parse: %v", errs)
		}
		var msgs []string
		for _, e := range validateExternalActionCalls(ctx, prog) {
			msgs = append(msgs, e.Error())
		}
		return strings.Join(msgs, "\n")
	}
	for clause, want := range map[string]string{
		"(Odata.Self)":                    "Odata.Self is a self-association",
		"(Odata.Elsewhere)":               "Odata.Elsewhere does not touch Odata.CreateBufferCommand",
		"(Odata.Quantity (Odata.Owner))":  "", // Owner touches QuantityParameterType from its FROM end
		"(Odata.Quantity (Odata.Unit))":   "",
		"(Odata.CreatedLaterInTheScript)": "", // not in the project: exec's to resolve
	} {
		got := check(clause)
		if want == "" && strings.Contains(got, "include associations") {
			t.Errorf("%s: refused: %s", clause, got)
		}
		if want != "" && !strings.Contains(got, want) {
			t.Errorf("%s: %q, want %q", clause, got, want)
		}
	}
}

// A call in a loop body is built by a nested builder: what it states must
// reach the carry, or `include associations ()` there gets the stored tree back.
func TestIncludeAssociations_StatedInANestedBody(t *testing.T) {
	calls, out, err := runExternalActionFlow(t, storedCreateBufferCall(), `while $Cmd != empty
begin
  call external action Odata.Bug1914.CreateBuffer(command = $Cmd include associations (), additional = empty);
  break;
end while;`)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 0 {
		t.Fatalf("the call is in the loop, not the top level: %d", len(calls))
	}
	if strings.Contains(out, carriedIncludesRule) {
		t.Errorf("a stated clause in a loop body was carried over:\n%s", out)
	}
}
