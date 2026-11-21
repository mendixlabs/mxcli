// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#2490. (2) `TaskId: String(36)` and `Text: String(unlimited)` in a
// business event message passed check and were stored with no Length, so the
// build applied its 200 and mx check 11.12.6 reported "The max length of the
// attribute 'TaskId' is different between the business event and the entity"
// for every one. (3) describe paired operations with messages by name, so a
// message declared once per operation printed both as the later one.

func parseBusinessEventService2490(t *testing.T, src string) *ast.CreateBusinessEventServiceStmt {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	for _, s := range prog.Statements {
		if c, ok := s.(*ast.CreateBusinessEventServiceStmt); ok {
			return c
		}
	}
	t.Fatal("no business event service statement")
	return nil
}

func TestBusinessEventMessageStringLengthsParsed_2490(t *testing.T) {
	stmt := parseBusinessEventService2490(t, `create business event service M.Bus (ServiceName: 'Bus', EventNamePrefix: '') {
  message AgentMessage (TaskId: String(36), Text: String(unlimited), Note: String, N: Integer) { publish (Entity: M.PBE); };
};`)
	got := map[string][2]int{}
	for _, a := range stmt.Messages[0].Attributes {
		set := 0
		if a.LengthSet {
			set = 1
		}
		got[a.Name] = [2]int{a.Length, set}
	}
	want := map[string][2]int{"TaskId": {36, 1}, "Text": {0, 1}, "Note": {0, 0}, "N": {0, 0}}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s: (Length, set) = %v, want %v", k, got[k], w)
		}
	}
}

func describedService2490(t *testing.T, svc *model.BusinessEventService) string {
	t.Helper()
	mod := mkModule("M")
	svc.ContainerID = mod.ID
	h := mkHierarchy(mod)
	withContainer(h, svc.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListBusinessEventServicesFunc: func() ([]*model.BusinessEventService, error) {
			return []*model.BusinessEventService{svc}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeBusinessEventService(ctx, ast.QualifiedName{Module: "M", Name: svc.Name}))
	return buf.String()
}

func TestDescribeBusinessEventLengthsAndOperations_2490(t *testing.T) {
	msg := func() *model.BusinessEventMessage {
		return &model.BusinessEventMessage{MessageName: "AgentMessage", Attributes: []*model.BusinessEventAttribute{
			{AttributeName: "TaskId", AttributeType: "String", Length: 36, LengthSet: true},
			{AttributeName: "Text", AttributeType: "String", Length: 0, LengthSet: true},
			{AttributeName: "Note", AttributeType: "String"},
		}}
	}
	svc := &model.BusinessEventService{
		BaseElement: model.BaseElement{ID: nextID("bes")},
		Name:        "Bus",
		Definition: &model.BusinessEventDefinition{ServiceName: "Bus", Channels: []*model.BusinessEventChannel{
			{ChannelName: "c1", Messages: []*model.BusinessEventMessage{msg()}},
			{ChannelName: "c2", Messages: []*model.BusinessEventMessage{msg()}},
		}},
		OperationImplementations: []*model.ServiceOperation{
			{MessageName: "AgentMessage", Operation: "publish", Entity: "M.PBE_AgentMessage"},
			{MessageName: "AgentMessage", Operation: "subscribe", Entity: "M.CBE_AgentMessage", Microflow: "M.SUB_OnMessage"},
		},
	}
	out := describedService2490(t, svc)
	if !strings.Contains(out, "TaskId: String(36)") || !strings.Contains(out, "Text: String(unlimited)") || !strings.Contains(out, "Note: String,") && !strings.Contains(out, "Note: String)") {
		t.Errorf("lengths not described as stored:\n%s", out)
	}
	if strings.Contains(out, "Note: String(") {
		t.Errorf("a string stored with no length was given one:\n%s", out)
	}
	pub := strings.Index(out, "publish (Entity: M.PBE_AgentMessage);")
	sub := strings.Index(out, "subscribe (Entity: M.CBE_AgentMessage, Microflow: M.SUB_OnMessage);")
	if pub < 0 || sub < 0 {
		t.Errorf("each message must be described with its own operation:\n%s", out)
	}

	// The describe output re-parses to the stated lengths.
	stmt := parseBusinessEventService2490(t, out)
	for _, a := range stmt.Messages[0].Attributes {
		if a.Name == "TaskId" && (!a.LengthSet || a.Length != 36) {
			t.Errorf("round trip: TaskId = (%d, %v)", a.Length, a.LengthSet)
		}
	}
}
