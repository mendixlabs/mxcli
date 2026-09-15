// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// wfHandlerCtx is a project shaped like the handler spike (mxbuild 11.13.0): a
// context entity Ctx and a Base <- Sub pair, an on-created microflow and an
// event handler microflow per signature shape, and one stored workflow for ALTER.
func wfHandlerCtx(t *testing.T, major, minor, patch int) *ExecContext {
	t.Helper()
	mod := &model.Module{Name: "M"}
	mod.ID = "mod1"
	ctxEnt := &domainmodel.Entity{Name: "Ctx"}
	base := &domainmodel.Entity{Name: "Base"}
	sub := &domainmodel.Entity{Name: "Sub", GeneralizationRef: "M.Base"}
	for _, e := range []*domainmodel.Entity{ctxEnt, base, sub} {
		e.ContainerID = "dm1"
	}
	dm := &domainmodel.DomainModel{Entities: []*domainmodel.Entity{ctxEnt, base, sub}}
	dm.ID = "dm1"
	dm.ContainerID = "mod1"

	obj := func(qn string) microflows.DataType { return &microflows.ObjectType{EntityQualifiedName: qn} }
	mf := func(name string, types ...microflows.DataType) *microflows.Microflow {
		m := &microflows.Microflow{Name: name}
		m.ContainerID = "mod1"
		for i, dt := range types {
			m.Parameters = append(m.Parameters, &microflows.MicroflowParameter{Name: "p" + string(rune('a'+i)), Type: dt})
		}
		return m
	}
	stored := &workflows.Workflow{Name: "Stored", Parameter: &workflows.WorkflowParameter{EntityRef: "M.Ctx"}}
	stored.ContainerID = "mod1"

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ProjectVersionFunc: func() *types.ProjectVersion {
			return &types.ProjectVersion{MajorVersion: major, MinorVersion: minor, PatchVersion: patch}
		},
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if strings.EqualFold(name, "M") {
				return mod, nil
			}
			return nil, nil
		},
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return []*microflows.Microflow{
				mf("OC_Good", obj("System.WorkflowUserTask"), obj("M.Ctx")),
				mf("OC_Reversed", obj("M.Ctx"), obj("System.WorkflowUserTask")),
				mf("OC_NoCtx", obj("System.WorkflowUserTask")),
				mf("OC_None"),
				mf("OC_Extra", obj("System.WorkflowUserTask"), obj("M.Ctx"), &microflows.StringType{}),
				mf("OC_TakesBase", obj("System.WorkflowUserTask"), obj("M.Base")),
				mf("OC_TakesSub", obj("System.WorkflowUserTask"), obj("M.Sub")),
				mf("EH_Good", obj("System.WorkflowEvent"), obj("System.WorkflowRecord"), obj("System.WorkflowActivityRecord")),
				mf("EH_Reversed", obj("System.WorkflowActivityRecord"), obj("System.WorkflowRecord"), obj("System.WorkflowEvent")),
				mf("EH_EventOnly", obj("System.WorkflowEvent")),
				mf("EH_None"),
				mf("EH_Extra", obj("System.WorkflowEvent"), obj("System.WorkflowRecord"), obj("System.WorkflowActivityRecord"), &microflows.StringType{}),
				mf("EH_Ctx", obj("System.WorkflowEvent"), obj("M.Ctx")),
			}, nil
		},
		ListWorkflowsFunc: func() ([]*workflows.Workflow, error) { return []*workflows.Workflow{stored}, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	return ctx
}

// Each row is a shape from the mxbuild spike (11.13.0); want is the CE code the
// build reported, "" where it built at 0 errors.
func TestWorkflowHandlers_OnCreatedSignature(t *testing.T) {
	wf := func(ctxEntity, mf string) string {
		return `create workflow M.W parameter $C: ` + ctxEntity +
			` begin user task T 'c' on created microflow ` + mf + ` outcomes 'a' { }; end workflow;`
	}
	cases := []struct{ name, src, want string }{
		{"(WorkflowUserTask, Ctx)", wf("M.Ctx", "M.OC_Good"), ""},
		{"(Ctx, WorkflowUserTask) reversed", wf("M.Ctx", "M.OC_Reversed"), ""},
		{"(WorkflowUserTask) only", wf("M.Ctx", "M.OC_NoCtx"), "CE6683"},
		{"no parameters", wf("M.Ctx", "M.OC_None"), "CE6683"},
		{"extra parameter", wf("M.Ctx", "M.OC_Extra"), "CE6683"},
		{"specialization of the context", wf("M.Base", "M.OC_TakesSub"), "CE6683"},
		{"generalization of the context is not refused", wf("M.Sub", "M.OC_TakesBase"), ""},
		{"multi user task", `create workflow M.W parameter $C: M.Ctx begin multi user task T 'c' on created microflow M.OC_None outcomes 'a' { }; end workflow;`, "CE6683"},
		{"nested in an outcome", `create workflow M.W parameter $C: M.Ctx begin user task T 'c' outcomes 'a' { user task U 'u' on created microflow M.OC_None outcomes 'x' { }; }; end workflow;`, "CE6683"},
		{"script microflow returning a value", `create microflow M.ScriptOC ( $t: System.WorkflowUserTask, $c: M.Ctx ) returns boolean as $r begin return true; end;
` + wf("M.Ctx", "M.ScriptOC"), "CE5012"},
		{"script microflow returning nothing", `create microflow M.ScriptOC ( $t: System.WorkflowUserTask, $c: M.Ctx ) begin end;
` + wf("M.Ctx", "M.ScriptOC"), ""},
		{"unknown microflow", wf("M.Ctx", "M.Nope"), "microflow not found: M.Nope"},
		{"alter insert", `alter workflow M.Stored insert after T user task U 'u' on created microflow M.OC_NoCtx outcomes 'a' { };`, "CE6683"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertHandlerVerdict(t, checkScript(t, wfHandlerCtx(t, 11, 13, 0), c.src), c.want)
		})
	}
}

func TestWorkflowHandlers_EventHandlerSignature(t *testing.T) {
	wf := func(mf string) string {
		return `create workflow M.W parameter $C: M.Ctx on workflow events (UserTaskStarted) microflow ` + mf +
			` as 'h' begin user task T 'c' outcomes 'a' { }; end workflow;`
	}
	cases := []struct{ name, src, want string }{
		{"the three records", wf("M.EH_Good"), ""},
		{"the three records reversed", wf("M.EH_Reversed"), ""},
		{"WorkflowEvent only", wf("M.EH_EventOnly"), "CE6691"},
		{"no parameters", wf("M.EH_None"), "CE6691"},
		{"extra parameter", wf("M.EH_Extra"), "CE6691"},
		{"context entity instead", wf("M.EH_Ctx"), "CE6691"},
		{"unknown microflow", wf("M.Nope"), "microflow not found: M.Nope"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertHandlerVerdict(t, checkScript(t, wfHandlerCtx(t, 11, 13, 0), c.src), c.want)
		})
	}
}

// Event types are version-dependent, and mxbuild does not check them.
func TestWorkflowHandlers_EventTypesPerVersion(t *testing.T) {
	named := `create workflow M.W parameter $C: M.Ctx on workflow events (NotificationStarted) microflow M.EH_Good begin user task T 'c' outcomes 'a' { }; end workflow;`
	anyEvent := `create workflow M.W parameter $C: M.Ctx on any workflow event microflow M.EH_Good begin user task T 'c' outcomes 'a' { }; end workflow;`
	cases := []struct {
		name                string
		major, minor, patch int
		src, want           string
	}{
		{"type present", 11, 13, 0, named, ""},
		{"type measured absent", 11, 10, 0, named, "does not exist in Mendix 11.10.0"},
		{"type between points is let through", 11, 12, 0, named, ""},
		{"any, measured version", 11, 13, 0, anyEvent, ""},
		{"any, below every measured point", 10, 24, 0, anyEvent, "not known to mxcli"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertHandlerVerdict(t, checkScript(t, wfHandlerCtx(t, c.major, c.minor, c.patch), c.src), c.want)
		})
	}
}

// The signature rules are measured on 11.13 only — the MDL-WF07 precedent.
func TestWorkflowHandlers_SignaturesNotAppliedBeforeMendix11(t *testing.T) {
	src := `create workflow M.W parameter $C: M.Ctx on workflow events (UserTaskStarted) microflow M.EH_None begin user task T 'c' on created microflow M.OC_None outcomes 'a' { }; end workflow;`
	assertHandlerVerdict(t, checkScript(t, wfHandlerCtx(t, 10, 24, 0), src), "")
}

// A misspelt type builds at 0 errors and never fires, so it is a syntax-phase
// error that needs no project.
func TestWorkflowHandlers_UnknownEventTypeIsMDLWF12(t *testing.T) {
	stmt := parseWorkflowStmt(t, `create workflow M.W on workflow events (UserTaskStart, usertaskended) microflow M.EH_Good begin end workflow;`)
	vs := ValidateWorkflow(stmt)
	var hits []string
	for _, v := range vs {
		if v.RuleID == "MDL-WF12" {
			hits = append(hits, v.Message)
		}
	}
	if len(hits) != 1 || !strings.Contains(hits[0], "UserTaskStart") {
		t.Errorf("want one MDL-WF12 naming UserTaskStart (the lower-case spelling is valid), got %v", hits)
	}
}

func assertHandlerVerdict(t *testing.T, got, want string) {
	t.Helper()
	if want != "" {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q, got:\n%s", want, got)
		}
		return
	}
	for _, code := range []string{"CE6683", "CE6691", "CE5012", "not found", "does not exist", "not known"} {
		if strings.Contains(got, code) {
			t.Errorf("expected no handler error, got %q:\n%s", code, got)
		}
	}
}
