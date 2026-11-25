// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
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
	// mendix is the project's Mendix version; BE06 only speaks where MxBuild is
	// known to wire the startup itself.
	mendix string
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
		// No microflow named StartupBusinessEvents: in Business Events 3.12.1 it
		// is a Java action, and ASU_Startup calls it as one.
		microflows: []*microflows.Microflow{
			beFlow(shop.ID, "HandleBE_OrderPlaceEvent",
				[]*microflows.MicroflowParameter{beObjectParam("Event", "Shop.CBE_OrderPlaceEvent")}, nil),
			// CORRECTLY wired for Mendix 11.16.1: the after-startup microflow does
			// NOT call StartupBusinessEvents, because MxBuild starts the module
			// itself (ako/mxcli#2384 B1).
			beJavaActionFlow(shop.ID, "ASU_Startup", "Shop.SeedData"),
		},
		constants: []*model.Constant{
			beConstant(be.ID, "ServerUrl", "kafka:9092"),
			beConstant(be.ID, "ChannelName", "default"),
		},
		afterStartup: "Shop.ASU_Startup",
		mendix:       "11.16.1",
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
		ProjectVersionFunc: func() *types.ProjectVersion {
			if f.mendix == "" {
				return nil
			}
			var maj, min, pat int
			fmt.Sscanf(f.mendix, "%d.%d.%d", &maj, &min, &pat)
			return &types.ProjectVersion{ProductVersion: f.mendix, MajorVersion: maj, MinorVersion: min, PatchVersion: pat}
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
		mf.ObjectCollection = beCalls(microflowCall, calls...)
	}
	return mf
}

// beCalls builds a flow body of calls of one kind.
func beCalls(kind func(string) microflows.MicroflowAction, targets ...string) *microflows.MicroflowObjectCollection {
	oc := &microflows.MicroflowObjectCollection{}
	for _, target := range targets {
		act := &microflows.ActionActivity{Action: kind(target)}
		act.ID = nextID("act")
		oc.Objects = append(oc.Objects, act)
	}
	return oc
}

func microflowCall(target string) microflows.MicroflowAction {
	return &microflows.MicroflowCallAction{MicroflowCall: &microflows.MicroflowCall{Microflow: target}}
}

// javaActionCall is the module's REAL shape: StartupBusinessEvents is a Java
// action, not a microflow (ako/mxcli#2384 M4). The fixture used a microflow
// call, so the rule's blindness to a Java action call passed every test while
// warning on every correctly wired app.
func javaActionCall(target string) microflows.MicroflowAction {
	return &microflows.JavaActionCallAction{JavaAction: target}
}

// beJavaActionFlow is a microflow whose body calls Java actions.
func beJavaActionFlow(container model.ID, name string, actions ...string) *microflows.Microflow {
	mf := &microflows.Microflow{Name: name, ContainerID: container}
	mf.ID = nextID("mf" + name)
	mf.ObjectCollection = beCalls(javaActionCall, actions...)
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

// BE06, the other way round (ako/mxcli#2384 B1). From Mendix 11.16.1 MxBuild
// generates BusinessEvents.ASu_BusinessEvents and calls it from the app's
// after-startup microflow, so the module starts without the app asking. An
// explicit call is a SECOND start, which leaks HeartbeatService's thread pool
// and leaves two heartbeat schedules running.
//
// The rule used to demand that call. It therefore warned on every correctly
// wired app and, once the call was removed, told the author to add back the
// thing that causes the double start.
func TestBEConsumer_BE06_DoubleStart(t *testing.T) {
	// The after-startup microflow calls the Java action: a second start.
	t.Run("explicit call in the after-startup microflow", func(t *testing.T) {
		f := newBEConsumerFixture()
		f.microflow("ASU_Startup").ObjectCollection = beCalls(javaActionCall, startupBusinessEvents)
		errs, warnings := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
		if len(errs) != 0 {
			t.Fatalf("BE06 must not block: %v", errs)
		}
		got := findingWith(nil, warnings, beModuleStartedRule)
		for _, want := range []string{"SECOND start", "ASu_BusinessEvents", "Remove the call"} {
			if !strings.Contains(got, want) {
				t.Errorf("warning should mention %q: %s", want, got)
			}
		}
	})

	// Reached through another microflow is the same double start.
	t.Run("explicit call reached transitively", func(t *testing.T) {
		f := newBEConsumerFixture()
		f.microflow("ASU_Startup").ObjectCollection = beCalls(microflowCall, "Shop.StartIntegrations")
		f.microflows = append(f.microflows,
			beJavaActionFlow(f.module("Shop").ID, "StartIntegrations", startupBusinessEvents))
		_, warnings := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
		if got := findingWith(nil, warnings, beModuleStartedRule); !strings.Contains(got, "SECOND start") {
			t.Fatalf("want the double-start warning, got %v", warnings)
		}
	})

	// A module version shipping the startup as a microflow is the same case.
	t.Run("a microflow of that name is also a double start", func(t *testing.T) {
		f := newBEConsumerFixture()
		f.microflow("ASU_Startup").ObjectCollection = beCalls(microflowCall, startupBusinessEvents)
		f.microflows = append(f.microflows,
			beFlow(f.module(businessEventsModule).ID, "StartupBusinessEvents", nil, nil))
		_, warnings := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
		if got := findingWith(nil, warnings, beModuleStartedRule); !strings.Contains(got, "SECOND start") {
			t.Fatalf("want the double-start warning, got %v", warnings)
		}
	})

	// A script that adds the call is reported before it is written.
	t.Run("a script adding the call", func(t *testing.T) {
		f := newBEConsumerFixture()
		wiring := &ast.CreateMicroflowStmt{
			Name: ast.QualifiedName{Module: "Shop", Name: "ASU_New"},
			Body: []ast.MicroflowStatement{&ast.CallJavaActionStmt{
				ActionName: ast.QualifiedName{Module: businessEventsModule, Name: "StartupBusinessEvents"},
			}},
		}
		_, warnings := runBEConsumerCheck(f.ctx(t), wiring, beDefaultStmt())
		if got := findingWith(nil, warnings, beModuleStartedRule); !strings.Contains(got, "SECOND start") {
			t.Fatalf("want the double-start warning for the script's call, got %v", warnings)
		}
	})

	// CONTROL: the fixture is wired the way an 11.16.1 app should be — no
	// explicit call — and must be silent. This is the case the old rule warned
	// on, and the reason it had to change.
	t.Run("no explicit call is correct and silent", func(t *testing.T) {
		f := newBEConsumerFixture()
		_, warnings := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
		if got := findingWith(nil, warnings, beModuleStartedRule); got != "" {
			t.Fatalf("an app that leaves the startup to MxBuild must be silent: %s", got)
		}
	})

	// CONTROL: below the measured version nothing is reported. On a Mendix that
	// does not wire the startup the call is required, and warning against it
	// would be the mirror image of the defect this replaced.
	t.Run("below the measured version, silent", func(t *testing.T) {
		for _, v := range []string{"11.15.0", "10.24.0"} {
			f := newBEConsumerFixture()
			f.mendix = v
			f.microflow("ASU_Startup").ObjectCollection = beCalls(javaActionCall, startupBusinessEvents)
			_, warnings := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
			if got := findingWith(nil, warnings, beModuleStartedRule); got != "" {
				t.Errorf("Mendix %s is not measured as wiring the startup: %s", v, got)
			}
		}
	})

	// CONTROL: a backend that cannot report a version is not guessed at.
	t.Run("no readable version, silent", func(t *testing.T) {
		f := newBEConsumerFixture()
		f.mendix = ""
		f.microflow("ASU_Startup").ObjectCollection = beCalls(javaActionCall, startupBusinessEvents)
		_, warnings := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
		if got := findingWith(nil, warnings, beModuleStartedRule); got != "" {
			t.Fatalf("want nothing without a version: %s", got)
		}
	})

	// CONTROL: an unrelated Java action is not the startup.
	t.Run("another java action is not a start", func(t *testing.T) {
		f := newBEConsumerFixture()
		f.microflow("ASU_Startup").ObjectCollection =
			beCalls(javaActionCall, "BusinessEvents.ShutdownBusinessEvents")
		_, warnings := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
		if got := findingWith(nil, warnings, beModuleStartedRule); got != "" {
			t.Fatalf("an unrelated Java action must not be read as a start: %s", got)
		}
	})

	// CONTROL: a cycle in the call graph must not hang the walk.
	t.Run("call cycle terminates", func(t *testing.T) {
		f := newBEConsumerFixture()
		f.microflow("ASU_Startup").ObjectCollection = beCalls(microflowCall, "Shop.Loop")
		f.microflows = append(f.microflows,
			beFlow(f.module("Shop").ID, "Loop", nil, []string{"Shop.ASU_Startup"}))
		_, warnings := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
		if got := findingWith(nil, warnings, beModuleStartedRule); got != "" {
			t.Fatalf("no startup call in the cycle, so nothing to report: %s", got)
		}
	})

	// CONTROL: nothing is said about a MISSING after-startup microflow. MxBuild
	// injects its call into whichever microflow the app configures; whether it
	// supplies one when the app configures none is not measured, and asserting
	// it would repeat the mistake this rule is recovering from.
	t.Run("no after-startup microflow is not judged", func(t *testing.T) {
		f := newBEConsumerFixture()
		f.afterStartup = ""
		_, warnings := runBEConsumerCheck(f.ctx(t), beDefaultStmt())
		if got := findingWith(nil, warnings, beModuleStartedRule); got != "" {
			t.Fatalf("want nothing for a missing after-startup microflow: %s", got)
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
