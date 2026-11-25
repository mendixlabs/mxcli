// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// #2384 §5.1's seven check rules for a CONSUMED business event service. Every
// one of them is something mxbuild does NOT report (measured, §6), so the only
// thing standing between the script and a silent runtime failure is this pass.
//
// Each test has a CONTROL: the same fixture with the defect removed must be
// clean of that rule. Without one the test passes against a build that never
// had the check (CLAUDE.md, "Any test asserting nothing changed must include a
// control") — here, against a pass that reports everything.

// beConsumerContract is the structure of the Studio Pro export
// (mdl/businessevents/testdata/asyncapi-OrdersEventService.yaml), trimmed to
// what these rules read: one channel, one message, two Long payload properties.
const beConsumerContract = `asyncapi: 2.2.0
info:
  title: "OrdersEventService"
  version: "1.0.0"
channels:
  f2477dc90f714d69bb12df0df1ee9cce:
    subscribe:
      operationId: receiveOrderPlaceEventEvents
      message:
        $ref: '#/components/messages/OrderPlaceEvent'
components:
  messages:
    OrderPlaceEvent:
      name: OrderPlaceEvent
      payload:
        $ref: '#/components/schemas/OrderPlaceEventPayload'
  schemas:
    OrderPlaceEventPayload:
      type: object
      properties:
        OrderId:
          type: integer
          format: int64
        CustomerId:
          type: integer
          format: int64
`

// beConsumerFixture is a project that satisfies all seven rules, which every
// test then breaks in exactly one place. tweak may edit the fixture before the
// backend is built.
type beConsumerFixture struct {
	modules      []*model.Module
	entities     map[model.ID][]*domainmodel.Entity // by module id
	microflows   []*microflows.Microflow
	constants    []*model.Constant
	afterStartup string
}

func newBEConsumerFixture() *beConsumerFixture {
	be := mkModule(businessEventsModule)
	shop := mkModule("Shop")

	attr := func(name string, t domainmodel.AttributeType) *domainmodel.Attribute {
		a := &domainmodel.Attribute{Name: name, Type: t}
		a.ID = nextID("attr" + name)
		return a
	}
	cbe := &domainmodel.Entity{Name: "ConsumedBusinessEvent", Persistable: true}
	cbe.ID = nextID("cbe")
	cbe.Attributes = []*domainmodel.Attribute{attr("EventId", &domainmodel.StringAttributeType{})}

	order := &domainmodel.Entity{
		Name:              "CBE_OrderPlaceEvent",
		Persistable:       true,
		GeneralizationRef: consumedBusinessEvent,
	}
	order.ID = nextID("order")
	order.Attributes = []*domainmodel.Attribute{
		attr("OrderId", &domainmodel.LongAttributeType{}),
		attr("CustomerId", &domainmodel.LongAttributeType{}),
	}

	return &beConsumerFixture{
		modules: []*model.Module{be, shop},
		entities: map[model.ID][]*domainmodel.Entity{
			be.ID:   {cbe},
			shop.ID: {order},
		},
		microflows: []*microflows.Microflow{
			beFlow(be.ID, "StartupBusinessEvents", nil, nil),
			beFlow(shop.ID, "HandleBE_OrderPlaceEvent",
				[]*microflows.MicroflowParameter{beObjectParam("Event", "Shop.CBE_OrderPlaceEvent")}, nil),
			beFlow(shop.ID, "ASU_Startup", nil, []string{startupBusinessEvents}),
		},
		constants: []*model.Constant{
			beConstant(be.ID, "ServerUrl", "kafka:9092"),
			beConstant(be.ID, "ChannelName", "default"),
		},
		afterStartup: "Shop.ASU_Startup",
	}
}

func (f *beConsumerFixture) module(name string) *model.Module {
	for _, m := range f.modules {
		if m.Name == name {
			return m
		}
	}
	return nil
}

// dropModule removes a module and its entities, for BE05's case.
func (f *beConsumerFixture) dropModule(name string) {
	var kept []*model.Module
	for _, m := range f.modules {
		if m.Name == name {
			delete(f.entities, m.ID)
			continue
		}
		kept = append(kept, m)
	}
	f.modules = kept
}

func (f *beConsumerFixture) entity(name string) *domainmodel.Entity {
	for _, ents := range f.entities {
		for _, e := range ents {
			if e.Name == name {
				return e
			}
		}
	}
	return nil
}

func (f *beConsumerFixture) microflow(name string) *microflows.Microflow {
	for _, mf := range f.microflows {
		if mf.Name == name {
			return mf
		}
	}
	return nil
}

func (f *beConsumerFixture) ctx(t *testing.T) *ExecContext {
	t.Helper()
	h := mkHierarchy(f.modules...)
	ps := &model.ProjectSettings{Model: &model.ModelSettings{AfterStartupMicroflow: f.afterStartup}}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return f.modules, nil },
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if m := f.module(name); m != nil {
				return m, nil
			}
			return nil, fmt.Errorf("no module %q", name)
		},
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			ents, ok := f.entities[id]
			if !ok {
				return nil, fmt.Errorf("no domain model %s", id)
			}
			dm := &domainmodel.DomainModel{ContainerID: id, Entities: ents}
			dm.ID = nextID("dm")
			return dm, nil
		},
		ListMicroflowsFunc:     func() ([]*microflows.Microflow, error) { return f.microflows, nil },
		ListConstantsFunc:      func() ([]*model.Constant, error) { return f.constants, nil },
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) { return ps, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx
}

func beFlow(container model.ID, name string, params []*microflows.MicroflowParameter, calls []string) *microflows.Microflow {
	mf := &microflows.Microflow{Name: name, ContainerID: container, Parameters: params}
	mf.ID = nextID("mf" + name)
	if len(calls) > 0 {
		oc := &microflows.MicroflowObjectCollection{}
		for _, target := range calls {
			act := &microflows.ActionActivity{
				Action: &microflows.MicroflowCallAction{
					MicroflowCall: &microflows.MicroflowCall{Microflow: target},
				},
			}
			act.ID = nextID("act")
			oc.Objects = append(oc.Objects, act)
		}
		mf.ObjectCollection = oc
	}
	return mf
}

func beObjectParam(name, entity string) *microflows.MicroflowParameter {
	p := &microflows.MicroflowParameter{Name: name, Type: &microflows.ObjectType{EntityQualifiedName: entity}}
	p.ID = nextID("param" + name)
	return p
}

func beConstant(container model.ID, name, value string) *model.Constant {
	c := &model.Constant{Name: name, ContainerID: container, DefaultValue: value}
	c.ID = nextID("const" + name)
	return c
}

// beConsumerStmt is the statement §5.1 documents: a contract plus one consume.
func beConsumerStmt(contract, message, entity, handler string) *ast.CreateBusinessEventServiceStmt {
	return &ast.CreateBusinessEventServiceStmt{
		Name:        ast.QualifiedName{Module: "Shop", Name: "OrdersEventClient"},
		Contract:    contract,
		ContractSet: true,
		Consumes: []*ast.BusinessEventConsumeDef{
			{MessageName: message, Entity: entity, Microflow: handler},
		},
	}
}

func beDefaultStmt() *ast.CreateBusinessEventServiceStmt {
	return beConsumerStmt(beConsumerContract, "OrderPlaceEvent",
		"Shop.CBE_OrderPlaceEvent", "Shop.HandleBE_OrderPlaceEvent")
}

// runBEConsumerCheck returns the errors and the warnings the pass produces.
func runBEConsumerCheck(ctx *ExecContext, stmts ...ast.Statement) ([]error, []string) {
	prog := &ast.Program{Statements: stmts}
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	errs := validateBusinessEventConsumers(ctx, prog, sc)
	return errs, sc.warnings
}

// findingWith returns the first error or warning mentioning rule, or "".
func findingWith(errs []error, warnings []string, rule string) string {
	for _, e := range errs {
		if strings.Contains(e.Error(), rule) {
			return "ERROR " + e.Error()
		}
	}
	for _, w := range warnings {
		if strings.Contains(w, rule) {
			return "WARN " + w
		}
	}
	return ""
}

// CONTROL for every rule at once: the fixture satisfies all seven, so the pass
// must be completely silent on it. A pass that reported anything here would
// make each test below pass for the wrong reason.
func TestBEConsumer_CleanFixtureIsSilent(t *testing.T) {
	f := newBEConsumerFixture()
	errs, warnings := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
	if len(errs) != 0 || len(warnings) != 0 {
		t.Fatalf("clean fixture: want no finding, got errors %v, warnings %v", errs, warnings)
	}
}

// A PUBLISHED service is not a consumer and none of these rules touch it.
func TestBEConsumer_PublisherUntouched(t *testing.T) {
	f := newBEConsumerFixture()
	f.dropModule(businessEventsModule) // would be BE05 for a consumer
	pub := &ast.CreateBusinessEventServiceStmt{
		Name:        ast.QualifiedName{Module: "Shop", Name: "OrdersEventService"},
		ServiceName: "OrdersEventService",
		Messages:    []*ast.BusinessEventMessageDef{{MessageName: "OrderPlaceEvent", Operation: "publish"}},
	}
	errs, warnings := runBEConsumerCheck(f.ctx(t), pub)
	if len(errs) != 0 || len(warnings) != 0 {
		t.Fatalf("publisher: want no finding, got errors %v, warnings %v", errs, warnings)
	}
}

func TestBEConsumer_BE01_Contract(t *testing.T) {
	cases := []struct {
		name     string
		contract string
		want     string
	}{
		{"empty", "", "business events service should have definitions"},
		{"not yaml", "asyncapi: 2.2.0\n  channels: [oops\n", "not valid YAML"},
		{"wrong version", strings.Replace(beConsumerContract, "2.2.0", "3.0.0", 1), "AsyncAPI 2.x"},
		{"no asyncapi key", "info:\n  title: x\nchannels:\n  abc: {}\n", "no asyncapi: key at all"},
		{"no channels", "asyncapi: 2.2.0\ninfo:\n  title: x\n", "declares no channels"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newBEConsumerFixture()
			errs, warnings := runBEConsumerCheck(f.ctx(t),
				beConsumerStmt(c.contract, "OrderPlaceEvent", "Shop.CBE_OrderPlaceEvent", "Shop.HandleBE_OrderPlaceEvent"))
			got := findingWith(errs, warnings, beContractParsesRule)
			if got == "" || !strings.HasPrefix(got, "ERROR") {
				t.Fatalf("want a %s error, got errors %v warnings %v", beContractParsesRule, errs, warnings)
			}
			if !strings.Contains(got, c.want) {
				t.Errorf("error should explain %q: %s", c.want, got)
			}
		})
	}
}

// An unreadable contract cannot be judged, so BE02-BE04 are not reported on top
// of BE01: one finding, not four.
func TestBEConsumer_BE01_StopsAtTheContract(t *testing.T) {
	f := newBEConsumerFixture()
	errs, _ := runBEConsumerCheck(f.ctx(t),
		beConsumerStmt("asyncapi: 2.2.0\n", "OrderPlaceEvent", "Shop.Nope", "Shop.Nope"))
	if len(errs) != 1 {
		t.Fatalf("want exactly one error, got %d: %v", len(errs), errs)
	}
}

func TestBEConsumer_BE02_MessageNotInContract(t *testing.T) {
	f := newBEConsumerFixture()
	errs, warnings := runBEConsumerCheck(f.ctx(t),
		beConsumerStmt(beConsumerContract, "OrderPlacedEvent", "Shop.CBE_OrderPlaceEvent", "Shop.HandleBE_OrderPlaceEvent"))
	got := findingWith(errs, warnings, beMessageExistsRule)
	if !strings.HasPrefix(got, "ERROR") {
		t.Fatalf("want a %s error, got errors %v warnings %v", beMessageExistsRule, errs, warnings)
	}
	// The message names what the contract DOES define — the typo is a letter.
	if !strings.Contains(got, "OrderPlaceEvent") {
		t.Errorf("error should list the contract's messages: %s", got)
	}
	// Case matters: Mendix resolves the $ref literally.
	errs, _ = runBEConsumerCheck(f.ctx(t),
		beConsumerStmt(beConsumerContract, "orderplaceevent", "Shop.CBE_OrderPlaceEvent", "Shop.HandleBE_OrderPlaceEvent"))
	if findingWith(errs, nil, beMessageExistsRule) == "" {
		t.Errorf("a case-differing message name should be reported: %v", errs)
	}
}

func TestBEConsumer_BE03_EntityShape(t *testing.T) {
	t.Run("not persistent", func(t *testing.T) {
		f := newBEConsumerFixture()
		f.entity("CBE_OrderPlaceEvent").Persistable = false
		errs, warnings := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
		if got := findingWith(errs, warnings, beEntityShapeRule); !strings.HasPrefix(got, "ERROR") ||
			!strings.Contains(got, "not a persistent entity") {
			t.Fatalf("want a persistence error, got errors %v warnings %v", errs, warnings)
		}
	})

	// A view entity reports Persistable true — Source is the discriminator.
	t.Run("view entity", func(t *testing.T) {
		f := newBEConsumerFixture()
		f.entity("CBE_OrderPlaceEvent").Source = "OqlViewEntitySource"
		errs, warnings := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
		if got := findingWith(errs, warnings, beEntityShapeRule); !strings.HasPrefix(got, "ERROR") {
			t.Fatalf("want an error for a view entity, got errors %v warnings %v", errs, warnings)
		}
	})

	t.Run("does not extend ConsumedBusinessEvent", func(t *testing.T) {
		f := newBEConsumerFixture()
		f.entity("CBE_OrderPlaceEvent").GeneralizationRef = ""
		errs, warnings := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
		got := findingWith(errs, warnings, beEntityShapeRule)
		if !strings.HasPrefix(got, "ERROR") || !strings.Contains(got, consumedBusinessEvent) {
			t.Fatalf("want a generalization error, got errors %v warnings %v", errs, warnings)
		}
	})

	// A payload property with no attribute WARNS: the contract-to-attribute
	// mapping is not validated at build time, and only Long is measured.
	t.Run("missing attribute warns", func(t *testing.T) {
		f := newBEConsumerFixture()
		e := f.entity("CBE_OrderPlaceEvent")
		e.Attributes = e.Attributes[:1] // drop CustomerId
		errs, warnings := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
		if len(errs) != 0 {
			t.Fatalf("a missing attribute must not block: %v", errs)
		}
		got := findingWith(nil, warnings, beEntityShapeRule)
		if !strings.Contains(got, "CustomerId") {
			t.Fatalf("want a warning naming CustomerId, got %v", warnings)
		}
	})

	// A real mismatch warns: the contract says the property is a string, and
	// the attribute is a Long.
	t.Run("type mismatch warns", func(t *testing.T) {
		f := newBEConsumerFixture()
		contract := strings.Replace(beConsumerContract,
			"          type: integer\n          format: int64", "          type: string", -1)
		_, warnings := runBEConsumerCheck(f.ctx(t),
			beConsumerStmt(contract, "OrderPlaceEvent", "Shop.CBE_OrderPlaceEvent", "Shop.HandleBE_OrderPlaceEvent"))
		got := findingWith(nil, warnings, beEntityShapeRule)
		if !strings.Contains(got, "string") || !strings.Contains(got, "Long") {
			t.Fatalf("want a warning contrasting the contract's string with the Long attribute, got %v", warnings)
		}
	})

	// CONTROL: more than one attribute type legitimately holds an encoding — an
	// enumeration travels as its key, so a `string` property is as well served
	// by an Enumeration attribute as by a String. A check that demanded the one
	// type Mendix RENDERS for `string` would flag this correct model
	// (businessevents.MendixAttributeTypesFor returns a set for exactly this).
	t.Run("an encoding's other valid type is accepted", func(t *testing.T) {
		f := newBEConsumerFixture()
		contract := strings.Replace(beConsumerContract,
			"          type: integer\n          format: int64", "          type: string", -1)
		for _, a := range f.entity("CBE_OrderPlaceEvent").Attributes {
			a.Type = &domainmodel.EnumerationAttributeType{}
		}
		_, warnings := runBEConsumerCheck(f.ctx(t),
			beConsumerStmt(contract, "OrderPlaceEvent", "Shop.CBE_OrderPlaceEvent", "Shop.HandleBE_OrderPlaceEvent"))
		if got := findingWith(nil, warnings, beEntityShapeRule); got != "" {
			t.Fatalf("an Enumeration attribute holds a string property: %s", got)
		}
	})

	// CONTROL: an encoding Mendix never writes is not judged at all — a
	// non-Mendix publisher's contract must not make mxcli invent a verdict.
	t.Run("an unknown encoding is not judged", func(t *testing.T) {
		f := newBEConsumerFixture()
		contract := strings.Replace(beConsumerContract,
			"          type: integer\n          format: int64", "          type: array", -1)
		_, warnings := runBEConsumerCheck(f.ctx(t),
			beConsumerStmt(contract, "OrderPlaceEvent", "Shop.CBE_OrderPlaceEvent", "Shop.HandleBE_OrderPlaceEvent"))
		if got := findingWith(nil, warnings, beEntityShapeRule); got != "" {
			t.Fatalf("an encoding Mendix does not write must not be judged: %s", got)
		}
	})

	// An entity the SCRIPT creates is read from the script: the ordinary shape
	// is one script with the entity, the handler and the service.
	t.Run("entity created in the script", func(t *testing.T) {
		f := newBEConsumerFixture()
		delete(f.entities, f.module("Shop").ID)
		ent := &ast.CreateEntityStmt{
			Name:           ast.QualifiedName{Module: "Shop", Name: "CBE_OrderPlaceEvent"},
			Kind:           ast.EntityNonPersistent,
			Generalization: &ast.QualifiedName{Module: businessEventsModule, Name: "ConsumedBusinessEvent"},
			Attributes:     []ast.Attribute{{Name: "OrderId", Type: ast.DataType{Kind: ast.TypeLong}}},
		}
		errs, warnings := runBEConsumerCheck(f.ctx(t), ent, beDefaultStmt())
		if got := findingWith(errs, warnings, beEntityShapeRule); !strings.Contains(got, "not a persistent entity") {
			t.Fatalf("want the script's non-persistent entity reported, got errors %v warnings %v", errs, warnings)
		}
	})
}

func TestBEConsumer_BE04_HandlerShape(t *testing.T) {
	t.Run("no parameters", func(t *testing.T) {
		f := newBEConsumerFixture()
		f.microflow("HandleBE_OrderPlaceEvent").Parameters = nil
		errs, warnings := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
		if got := findingWith(errs, warnings, beHandlerShapeRule); !strings.HasPrefix(got, "ERROR") ||
			!strings.Contains(got, "takes 0 parameters") {
			t.Fatalf("want an arity error, got errors %v warnings %v", errs, warnings)
		}
	})

	t.Run("two parameters", func(t *testing.T) {
		f := newBEConsumerFixture()
		mf := f.microflow("HandleBE_OrderPlaceEvent")
		mf.Parameters = append(mf.Parameters, beObjectParam("Extra", "Shop.CBE_OrderPlaceEvent"))
		errs, _ := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
		if got := findingWith(errs, nil, beHandlerShapeRule); !strings.Contains(got, "takes 2 parameters") {
			t.Fatalf("want an arity error, got %v", errs)
		}
	})

	t.Run("not an object", func(t *testing.T) {
		f := newBEConsumerFixture()
		p := &microflows.MicroflowParameter{Name: "EventId", Type: &microflows.StringType{}}
		p.ID = nextID("param")
		f.microflow("HandleBE_OrderPlaceEvent").Parameters = []*microflows.MicroflowParameter{p}
		errs, _ := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
		if got := findingWith(errs, nil, beHandlerShapeRule); !strings.Contains(got, "not a single object") {
			t.Fatalf("want a parameter-type error, got %v", errs)
		}
	})

	t.Run("a list, not an object", func(t *testing.T) {
		f := newBEConsumerFixture()
		p := &microflows.MicroflowParameter{Name: "Events",
			Type: &microflows.ListType{EntityQualifiedName: "Shop.CBE_OrderPlaceEvent"}}
		p.ID = nextID("param")
		f.microflow("HandleBE_OrderPlaceEvent").Parameters = []*microflows.MicroflowParameter{p}
		errs, _ := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
		if got := findingWith(errs, nil, beHandlerShapeRule); !strings.Contains(got, "not a single object") {
			t.Fatalf("want a parameter-type error for a list, got %v", errs)
		}
	})

	t.Run("unrelated entity", func(t *testing.T) {
		f := newBEConsumerFixture()
		f.microflow("HandleBE_OrderPlaceEvent").Parameters =
			[]*microflows.MicroflowParameter{beObjectParam("Event", "Shop.Something")}
		errs, _ := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
		if got := findingWith(errs, nil, beHandlerShapeRule); !strings.Contains(got, "Shop.Something") {
			t.Fatalf("want an entity mismatch error, got %v", errs)
		}
	})

	// CONTROL: a parameter typed as an ANCESTOR of the consumed entity holds the
	// object perfectly well and is not reported.
	t.Run("ancestor parameter is accepted", func(t *testing.T) {
		f := newBEConsumerFixture()
		f.microflow("HandleBE_OrderPlaceEvent").Parameters =
			[]*microflows.MicroflowParameter{beObjectParam("Event", consumedBusinessEvent)}
		errs, warnings := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
		if got := findingWith(errs, warnings, beHandlerShapeRule); got != "" {
			t.Fatalf("an ancestor parameter must be accepted: %s", got)
		}
	})

	// CONTROL: a handler the project does not have is the reference checks' to
	// report, not this rule's — it must not guess a shape it cannot see.
	t.Run("unresolvable handler says nothing", func(t *testing.T) {
		f := newBEConsumerFixture()
		errs, warnings := runBEConsumerCheck(f.ctx(t),
			beConsumerStmt(beConsumerContract, "OrderPlaceEvent", "Shop.CBE_OrderPlaceEvent", "Shop.NoSuchFlow"))
		if got := findingWith(errs, warnings, beHandlerShapeRule); got != "" {
			t.Fatalf("want nothing for an unresolvable handler: %s", got)
		}
	})
}

func TestBEConsumer_BE05_ModulePresent(t *testing.T) {
	f := newBEConsumerFixture()
	f.dropModule(businessEventsModule)
	errs, _ := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
	got := findingWith(errs, nil, beModulePresentRule)
	if !strings.HasPrefix(got, "ERROR") || !strings.Contains(got, "Marketplace") {
		t.Fatalf("want a %s error naming the marketplace, got %v", beModulePresentRule, errs)
	}
	// Reported once, however many consumers the script creates.
	errs, _ = runBEConsumerCheck(f.ctx(t), beDefaultStmt(), beDefaultStmt())
	n := 0
	for _, e := range errs {
		if strings.Contains(e.Error(), beModulePresentRule) {
			n++
		}
	}
	if n != 1 {
		t.Errorf("want one %s for two consumers, got %d: %v", beModulePresentRule, n, errs)
	}
}

func TestBEConsumer_BE06_ModuleStarted(t *testing.T) {
	t.Run("no after-startup microflow", func(t *testing.T) {
		f := newBEConsumerFixture()
		f.afterStartup = ""
		errs, warnings := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
		if len(errs) != 0 {
			t.Fatalf("BE06 must not block: %v", errs)
		}
		if got := findingWith(nil, warnings, beModuleStartedRule); !strings.Contains(got, "no after-startup microflow") {
			t.Fatalf("want a %s warning, got %v", beModuleStartedRule, warnings)
		}
	})

	t.Run("after-startup does not reach the startup microflow", func(t *testing.T) {
		f := newBEConsumerFixture()
		f.microflow("ASU_Startup").ObjectCollection = nil
		_, warnings := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
		if got := findingWith(nil, warnings, beModuleStartedRule); !strings.Contains(got, "does not reach") {
			t.Fatalf("want a %s warning, got %v", beModuleStartedRule, warnings)
		}
	})

	// CONTROL: reached through another microflow, which is how a real app wires
	// it — an ASU_ flow calling several startup flows.
	t.Run("reached transitively", func(t *testing.T) {
		f := newBEConsumerFixture()
		f.microflow("ASU_Startup").ObjectCollection =
			beFlow(f.module("Shop").ID, "x", nil, []string{"Shop.StartIntegrations"}).ObjectCollection
		f.microflows = append(f.microflows,
			beFlow(f.module("Shop").ID, "StartIntegrations", nil, []string{startupBusinessEvents}))
		_, warnings := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
		if got := findingWith(nil, warnings, beModuleStartedRule); got != "" {
			t.Fatalf("a transitive call must satisfy the rule: %s", got)
		}
	})

	// CONTROL: a cycle in the call graph must not hang the walk.
	t.Run("call cycle terminates", func(t *testing.T) {
		f := newBEConsumerFixture()
		f.microflow("ASU_Startup").ObjectCollection =
			beFlow(f.module("Shop").ID, "x", nil, []string{"Shop.Loop"}).ObjectCollection
		f.microflows = append(f.microflows,
			beFlow(f.module("Shop").ID, "Loop", nil, []string{"Shop.ASU_Startup"}))
		_, warnings := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
		if got := findingWith(nil, warnings, beModuleStartedRule); !strings.Contains(got, "does not reach") {
			t.Fatalf("want the warning, not a hang: %v", warnings)
		}
	})

	// CONTROL: a script that wires the startup itself is judged on what it will
	// produce, not on what is stored now.
	t.Run("script wires it", func(t *testing.T) {
		for _, name := range []string{"create microflow", "alter settings"} {
			f := newBEConsumerFixture()
			f.afterStartup = ""
			var wiring ast.Statement
			if name == "create microflow" {
				wiring = &ast.CreateMicroflowStmt{
					Name: ast.QualifiedName{Module: "Shop", Name: "ASU_New"},
					Body: []ast.MicroflowStatement{&ast.CallMicroflowStmt{
						MicroflowName: ast.QualifiedName{Module: businessEventsModule, Name: "StartupBusinessEvents"},
					}},
				}
			} else {
				wiring = &ast.AlterSettingsStmt{
					Section:    "model",
					Properties: map[string]any{"AfterStartupMicroflow": "Shop.ASU_New"},
				}
			}
			_, warnings := runBEConsumerCheck(f.ctx(t), wiring, beDefaultStmt())
			if got := findingWith(nil, warnings, beModuleStartedRule); got != "" {
				t.Errorf("%s: the script wires startup, so the rule must keep quiet: %s", name, got)
			}
		}
	})
}

func TestBEConsumer_BE07_BrokerConfigured(t *testing.T) {
	t.Run("blank ServerUrl", func(t *testing.T) {
		f := newBEConsumerFixture()
		f.constants[0].DefaultValue = "  "
		errs, warnings := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
		if len(errs) != 0 {
			t.Fatalf("BE07 must not block — it is a per-environment value: %v", errs)
		}
		if got := findingWith(nil, warnings, beBrokerConfigRule); !strings.Contains(got, beServerURLConstant) {
			t.Fatalf("want a %s warning, got %v", beBrokerConfigRule, warnings)
		}
	})

	t.Run("neither channel nor space", func(t *testing.T) {
		f := newBEConsumerFixture()
		f.constants = f.constants[:1] // ServerUrl only
		_, warnings := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
		got := findingWith(nil, warnings, beBrokerConfigRule)
		if !strings.Contains(got, beChannelNameConstant) || !strings.Contains(got, beBrokerSpaceConstant) {
			t.Fatalf("want the warning to name both constants, got %v", warnings)
		}
	})

	// CONTROL: EventBrokerSpace alone satisfies it — the rule is "at least one".
	t.Run("space alone is enough", func(t *testing.T) {
		f := newBEConsumerFixture()
		f.constants = []*model.Constant{
			beConstant(f.module(businessEventsModule).ID, "ServerUrl", "kafka:9092"),
			beConstant(f.module(businessEventsModule).ID, "EventBrokerSpace", "prod"),
		}
		_, warnings := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
		if got := findingWith(nil, warnings, beBrokerConfigRule); got != "" {
			t.Fatalf("EventBrokerSpace alone must satisfy the rule: %s", got)
		}
	})
}

// Nothing is reported without a project: `mxcli check script.mdl` with no -p
// cannot see any of this, and must not pretend to.
func TestBEConsumer_SilentWithoutAProject(t *testing.T) {
	mb := &mock.MockBackend{IsConnectedFunc: func() bool { return false }}
	ctx, _ := newMockCtx(t, withBackend(mb))
	errs, warnings := runBEConsumerCheck(ctx,
		beConsumerStmt("not a contract at all", "Nope", "Shop.Nope", "Shop.Nope"))
	if len(errs) != 0 || len(warnings) != 0 {
		t.Fatalf("want nothing without a project, got errors %v warnings %v", errs, warnings)
	}
}

// The rules run in the pass `check --references` and exec's pre-flight use.
func TestBEConsumer_RunsInReferencePass(t *testing.T) {
	f := newBEConsumerFixture()
	errs, warnings := validateProgramWithWarnings(f.ctx(t), &ast.Program{Statements: []ast.Statement{
		beConsumerStmt(beConsumerContract, "NotInContract", "Shop.CBE_OrderPlaceEvent", "Shop.HandleBE_OrderPlaceEvent"),
	}})
	if findingWith(errs, warnings, beMessageExistsRule) == "" {
		t.Fatalf("the reference pass did not report %s: errors %v warnings %v", beMessageExistsRule, errs, warnings)
	}
}
