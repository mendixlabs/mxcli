// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#2490 part 1. A message a service both publishes and consumes (an
// in-app bus, a request/reply loop) could only be written by declaring it
// twice, once per operation: that stored two messages of one name, and the
// deployment failed — measured with mxbuild 11.12.6 --target=deploy and
// BusinessEvents 3.12.0: "Extension BusinessEvents deployment failed!
// System.ArgumentException: An item with the same key has already been added.
// Key: AgentMessage". The model holds one message with CanPublish and
// CanSubscribe and two operation implementations. MDL now says so the way R2
// says children: the operations are the message's, each with its properties
// in ( ).

const publishSubscribe2490 = `create business event service M.Bus (ServiceName: 'Bus', EventNamePrefix: '') {
  message AgentMessage (TaskId: String(36)) {
    publish (Entity: M.PBE_AgentMessage);
    subscribe (Entity: M.CBE_AgentMessage, Microflow: M.SUB_OnMessage);
  };
};`

func TestPublishSubscribeParses_2490(t *testing.T) {
	stmt := parseBusinessEventService2490(t, publishSubscribe2490)
	m := stmt.Messages[0]
	if !m.PublishAndSubscribe || m.Entity != "M.PBE_AgentMessage" || m.SubscribeEntity != "M.CBE_AgentMessage" || m.Microflow != "M.SUB_OnMessage" {
		t.Errorf("message = %+v, want publish+subscribe with both entities and the handler", m)
	}
	// Control: one operation as a child keeps its one entity and handler.
	one := parseBusinessEventService2490(t, `create business event service M.Bus (ServiceName: 'Bus', EventNamePrefix: '') {
  message A (X: Long) { subscribe (Entity: M.CBE, Microflow: M.H); };
};`).Messages[0]
	if one.PublishAndSubscribe || one.Operation != "SUBSCRIBE" || one.Entity != "M.CBE" || one.Microflow != "M.H" {
		t.Errorf("single operation = %+v", one)
	}
}

// The keyword clauses still parse to the same message, and record their
// deprecation: an older script keeps working and is told the new form.
func TestOldMessageAndConsumeClausesAreAliases_2490(t *testing.T) {
	prog, errs := visitor.Build(`create business event service M.Bus (ServiceName: 'Bus', EventNamePrefix: '') {
  message A (X: Long) subscribe entity M.CBE microflow M.H;
};
create business event service M.Client (ContractFile: 'c.yaml') {
  consume A entity M.CBE microflow M.H;
};`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	codes := map[string]bool{}
	for _, d := range prog.Deprecations {
		codes[d.Code] = true
	}
	if !codes[deprecation.BusinessEventMessageClauses] || !codes[deprecation.BusinessEventConsumeClauses] {
		t.Errorf("deprecations = %v, want %s and %s", prog.Deprecations, deprecation.BusinessEventMessageClauses, deprecation.BusinessEventConsumeClauses)
	}
	m := parseBusinessEventService2490(t, `create business event service M.Bus (ServiceName: 'Bus', EventNamePrefix: '') {
  message A (X: Long) subscribe entity M.CBE microflow M.H;
};`).Messages[0]
	if m.Operation != "SUBSCRIBE" || m.Entity != "M.CBE" || m.Microflow != "M.H" {
		t.Errorf("old form = %+v", m)
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

// The double declaration is refused, naming the spelling that works; a
// publish-and-subscribe message without an entity per operation is refused.
func TestBusinessEventMessageShapeProblems_2490(t *testing.T) {
	cases := map[string]string{
		`message A (X: Long) { publish (Entity: M.P); };
  message A (X: Long) { subscribe (Entity: M.C, Microflow: M.H); };`: "declare it once with both operations",
		`message A (X: Long) publish entity M.P;
  message A (X: Long) subscribe entity M.C microflow M.H;`: "declared twice",
		`message A (X: Long) { publish (); subscribe (Entity: M.C, Microflow: M.H); };`: "each operation needs its entity",
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
		`message A (X: Long) { publish (Entity: M.P); subscribe (Entity: M.C, Microflow: M.H); };`,
		`message A (X: Long) { publish (Entity: M.P); };
  message B (X: Long) { subscribe (Entity: M.C, Microflow: M.H); };`,
	} {
		stmt := parseBusinessEventService2490(t, "create business event service M.Bus (ServiceName: 'Bus', EventNamePrefix: '') {\n  "+body+"\n};")
		if probs := businessEventMessageProblems(stmt); len(probs) != 0 {
			t.Errorf("%s\n  unexpected problems %v", body, probs)
		}
	}
}

// Two operations of one kind, a handler on the publish operation and an
// unknown property are parse errors.
func TestBusinessEventOperationParseErrors_2490(t *testing.T) {
	for src, want := range map[string]string{
		`message A (X: Long) { publish (Entity: M.P); publish (Entity: M.Q); };`:                   "two publish operations",
		`message A (X: Long) { publish (Entity: M.P, Microflow: M.H); subscribe (Entity: M.C); };`: "no handler",
		`message A (X: Long) { subscribe (Entity: M.C, Handler: M.H); };`:                          "unknown property",
		`message A (X: Long) { };`: "no operation",
	} {
		_, errs := visitor.Build("create business event service M.Bus (ServiceName: 'Bus', EventNamePrefix: '') {\n  " + src + "\n};")
		var all []string
		for _, e := range errs {
			all = append(all, e.Error())
		}
		if !strings.Contains(strings.Join(all, "\n"), want) {
			t.Errorf("%s\n  errors = %v, want one mentioning %q", src, all, want)
		}
	}
}

// describe writes a stored message with both operations back as children, and
// that output re-parses to the same message.
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
	want := "  message AgentMessage (TaskId: String(36)) {\n" +
		"    publish (Entity: M.PBE_AgentMessage);\n" +
		"    subscribe (Entity: M.CBE_AgentMessage, Microflow: M.SUB_OnMessage);\n" +
		"  };\n"
	if !strings.Contains(out, want) {
		t.Fatalf("not described as the message's children:\n%s", out)
	}
	prog, errs := visitor.Build(out)
	if len(errs) > 0 || len(prog.Deprecations) > 0 {
		t.Fatalf("describe output must parse with no deprecation: errs %v, deprecations %v\n%s", errs, prog.Deprecations, out)
	}
	m := parseBusinessEventService2490(t, out).Messages[0]
	if !m.PublishAndSubscribe || m.Entity != "M.PBE_AgentMessage" || m.SubscribeEntity != "M.CBE_AgentMessage" || m.Microflow != "M.SUB_OnMessage" {
		t.Errorf("round trip = %+v", m)
	}
}
