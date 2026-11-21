// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#2490 part 1. A message the service both publishes and consumes (an
// in-app bus, a request/reply loop) could only be written by declaring it
// twice, once per operation: that stored two messages of one name, and the
// deployment failed — measured with mxbuild 11.12.6 --target=deploy and
// BusinessEvents 3.12.0: "Extension BusinessEvents deployment failed!
// System.ArgumentException: An item with the same key has already been added.
// Key: AgentMessage". The model holds one message with CanPublish and
// CanSubscribe and two operation implementations; `publish subscribe` writes
// exactly that.

const publishSubscribe2490 = `create business event service M.Bus (ServiceName: 'Bus', EventNamePrefix: '') {
  message AgentMessage (TaskId: String(36)) publish subscribe
    entity M.PBE_AgentMessage
    entity M.CBE_AgentMessage
    microflow M.SUB_OnMessage;
};`

func TestPublishSubscribeParses_2490(t *testing.T) {
	stmt := parseBusinessEventService2490(t, publishSubscribe2490)
	m := stmt.Messages[0]
	if !m.PublishAndSubscribe || m.Entity != "M.PBE_AgentMessage" || m.SubscribeEntity != "M.CBE_AgentMessage" || m.Microflow != "M.SUB_OnMessage" {
		t.Errorf("message = %+v, want publish+subscribe with both entities and the handler", m)
	}
	// Control: a single operation keeps its one entity.
	one := parseBusinessEventService2490(t, `create business event service M.Bus (ServiceName: 'Bus', EventNamePrefix: '') {
  message A (X: Long) subscribe entity M.CBE microflow M.H;
};`).Messages[0]
	if one.PublishAndSubscribe || one.Entity != "M.CBE" || one.SubscribeEntity != "" {
		t.Errorf("single operation = %+v", one)
	}
}

func TestPublishSubscribeBuildsOneMessageTwoOperations_2490(t *testing.T) {
	stmt := parseBusinessEventService2490(t, publishSubscribe2490)
	svc, err := businessEventServiceModel(stmt, newBusinessEventIdentityCarry(nil))
	if err != nil {
		t.Fatal(err)
	}
	var msgs []*model.BusinessEventMessage
	for _, ch := range svc.Definition.Channels {
		msgs = append(msgs, ch.Messages...)
	}
	if len(msgs) != 1 || !msgs[0].CanPublish || !msgs[0].CanSubscribe {
		t.Fatalf("messages = %+v, want one with both flags", msgs)
	}
	ops := map[string]*model.ServiceOperation{}
	for _, op := range svc.OperationImplementations {
		ops[op.Operation] = op
	}
	if len(svc.OperationImplementations) != 2 ||
		ops["publish"] == nil || ops["publish"].Entity != "M.PBE_AgentMessage" || ops["publish"].Microflow != "" ||
		ops["subscribe"] == nil || ops["subscribe"].Entity != "M.CBE_AgentMessage" || ops["subscribe"].Microflow != "M.SUB_OnMessage" {
		t.Errorf("operations = %+v, want publish(PBE) and subscribe(CBE, handler)", svc.OperationImplementations)
	}
}

// The double declaration is refused, naming the spelling that works; the
// form's own shape errors are refused too.
func TestBusinessEventMessageShapeProblems_2490(t *testing.T) {
	cases := map[string]string{
		`message A (X: Long) publish entity M.P;
  message A (X: Long) subscribe entity M.C microflow M.H;`: "publish subscribe",
		`message A (X: Long) publish subscribe entity M.P microflow M.H;`: "two entities",
		`message A (X: Long) subscribe entity M.C entity M.D;`:            "one entity",
	}
	for body, want := range cases {
		stmt := parseBusinessEventService2490(t, "create business event service M.Bus (ServiceName: 'Bus', EventNamePrefix: '') {\n  "+body+"\n};")
		probs := businessEventMessageProblems(stmt)
		if len(probs) == 0 || !strings.Contains(strings.Join(probs, "\n"), want) {
			t.Errorf("%s\n  problems = %v, want one mentioning %q", body, probs, want)
		}
	}
	// Control: the valid forms raise nothing.
	for _, body := range []string{
		`message A (X: Long) publish subscribe entity M.P entity M.C microflow M.H;`,
		`message A (X: Long) publish entity M.P;
  message B (X: Long) subscribe entity M.C microflow M.H;`,
	} {
		stmt := parseBusinessEventService2490(t, "create business event service M.Bus (ServiceName: 'Bus', EventNamePrefix: '') {\n  "+body+"\n};")
		if probs := businessEventMessageProblems(stmt); len(probs) != 0 {
			t.Errorf("%s\n  unexpected problems %v", body, probs)
		}
	}
}

// describe writes a stored message with both operations back in the one form,
// and that output re-parses to the same message.
func TestDescribePublishSubscribeRoundTrips_2490(t *testing.T) {
	svc := &model.BusinessEventService{
		BaseElement: model.BaseElement{ID: nextID("bes")},
		Name:        "Bus",
		Definition: &model.BusinessEventDefinition{ServiceName: "Bus", Channels: []*model.BusinessEventChannel{{
			ChannelName: "c1",
			Messages: []*model.BusinessEventMessage{{MessageName: "AgentMessage", CanPublish: true, CanSubscribe: true,
				Attributes: []*model.BusinessEventAttribute{{AttributeName: "TaskId", AttributeType: "String", Length: 36, LengthSet: true}}}},
		}}},
		OperationImplementations: []*model.ServiceOperation{
			{MessageName: "AgentMessage", Operation: "publish", Entity: "M.PBE_AgentMessage"},
			{MessageName: "AgentMessage", Operation: "subscribe", Entity: "M.CBE_AgentMessage", Microflow: "M.SUB_OnMessage"},
		},
	}
	out := describedService2490(t, svc)
	if !strings.Contains(out, "publish subscribe\n    entity M.PBE_AgentMessage\n    entity M.CBE_AgentMessage\n    microflow M.SUB_OnMessage;") {
		t.Fatalf("not described in the publish subscribe form:\n%s", out)
	}
	if strings.Count(out, "message AgentMessage") != 1 {
		t.Errorf("the message is described more than once:\n%s", out)
	}
	m := parseBusinessEventService2490(t, out).Messages[0]
	if !m.PublishAndSubscribe || m.Entity != "M.PBE_AgentMessage" || m.SubscribeEntity != "M.CBE_AgentMessage" || m.Microflow != "M.SUB_OnMessage" {
		t.Errorf("round trip = %+v", m)
	}
	_ = ast.QualifiedName{}
}
