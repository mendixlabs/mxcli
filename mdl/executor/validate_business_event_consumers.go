// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/businessevents"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/versions"
)

// The seven rules of #2384 §5.1, for a CONSUMED business event service — the
// shape `Contract:` / `ContractFile:` plus `consume` writes.
//
// Every one of them is something mxbuild does NOT report. Measured on mxbuild
// 11.16.1 (§6): a consumer whose `consume` names a message the contract does
// not contain builds with 0 errors, and nothing in the build looks at the
// consumed entity, the handler's signature, the marketplace module or the
// broker constants at all. The failure surfaces at runtime instead — the app
// consumes nothing, silently, or does not boot.
//
// Severity follows what has been measured, not what would be tidy:
//
//   - BE01/BE02 are errors. A contract that is not AsyncAPI 2.x with a channel,
//     or a `consume` naming a message that is not in it, can never subscribe to
//     anything: there is no reading of the model under which it works.
//   - BE03's generalization and persistence halves are errors (the module
//     retrieves consumed events by that generalization and commits them); its
//     payload-to-attribute half only WARNS, because the contract-to-Mendix type
//     mapping is measured for Long alone (#2384 §9.1, businessevents.asyncAPIType).
//   - BE04 is an error: a handler is invoked with the one consumed object, so a
//     different arity or an unrelated parameter type cannot be called.
//   - BE05 is an error — without the module there is no consumer at all.
//   - BE06 and BE07 WARN. Both are project configuration a script may legitimately
//     set afterwards (or per environment, which is what `mxcli constant set` and
//     `--constant` are for), so refusing exec over them would block authoring a
//     consumer before the app is wired up.
//
// Where something cannot be read — the contract is not this statement's to
// resolve, a microflow's parameter types are unavailable, the project cannot
// list constants — nothing is reported for that part. A refusal on a guess
// would block a script that works.
const (
	beContractParsesRule = "MDL-BE01"
	beMessageExistsRule  = "MDL-BE02"
	beEntityShapeRule    = "MDL-BE03"
	beHandlerShapeRule   = "MDL-BE04"
	beModulePresentRule  = "MDL-BE05"
	beModuleStartedRule  = "MDL-BE06"
	beBrokerConfigRule   = "MDL-BE07"
)

// businessEventsModule is the marketplace module's name, and the three elements
// of it a consumer depends on. All four are Mendix's, not mxcli's: the module is
// downloaded from the marketplace, and these names are what it ships.
const (
	businessEventsModule  = "BusinessEvents"
	consumedBusinessEvent = "BusinessEvents.ConsumedBusinessEvent"
	startupBusinessEvents = "BusinessEvents.StartupBusinessEvents"
	beServerURLConstant   = "BusinessEvents.ServerUrl"
	beChannelNameConstant = "BusinessEvents.ChannelName"
	beBrokerSpaceConstant = "BusinessEvents.EventBrokerSpace"
)

// validateBusinessEventConsumers runs §5.1's check rules over every consumed
// business event service the script creates. Errors are returned; findings that
// do not block are appended to sc.warnings.
func validateBusinessEventConsumers(ctx *ExecContext, prog *ast.Program, sc *scriptContext) []error {
	if ctx == nil || prog == nil || sc == nil || !ctx.Connected() {
		return nil
	}
	type consumer struct {
		index int
		stmt  *ast.CreateBusinessEventServiceStmt
	}
	var consumers []consumer
	for i, stmt := range prog.Statements {
		if s, ok := stmt.(*ast.CreateBusinessEventServiceStmt); ok && s.IsConsumer() {
			consumers = append(consumers, consumer{i + 1, s})
		}
	}
	if len(consumers) == 0 {
		return nil
	}

	var errs []error
	for _, c := range consumers {
		errs = append(errs, validateOneBusinessEventConsumer(ctx, sc, c.index, c.stmt)...)
	}
	// The project-wide rules are about the app, not the statement, so they are
	// reported once however many consumers the script creates.
	errs = append(errs, businessEventsModuleErrors(ctx, sc)...)
	businessEventsRuntimeWarnings(ctx, prog, sc)
	return errs
}

// validateOneBusinessEventConsumer is BE01-BE04, the rules a single statement
// decides.
func validateOneBusinessEventConsumer(ctx *ExecContext, sc *scriptContext, index int, s *ast.CreateBusinessEventServiceStmt) []error {
	where := fmt.Sprintf("statement %d: business event service %s", index, s.Name.String())

	text, err := resolveBusinessEventContract(ctx, s)
	if err != nil {
		// The path is the statement's own and exec would fail on it too; report
		// it here, where it is cheap to fix.
		return []error{mdlerrors.NewValidationf("%s: %s: %v", where, beContractParsesRule, err)}
	}
	contract, perr := parseBusinessEventContract(text)
	if perr != nil {
		return []error{mdlerrors.NewValidationf("%s: %s: %v", where, beContractParsesRule, perr)}
	}

	var errs []error
	for _, cons := range s.Consumes {
		msg, ok := contract.message(cons.MessageName)
		if !ok {
			errs = append(errs, mdlerrors.NewValidationf(
				"%s: %s: `consume %s` names a message the contract does not define — it declares %s. "+
					"mxbuild reports nothing (measured, #2384 §6): the service builds and subscribes to no such "+
					"event, so the handler never runs. Message names are case-sensitive",
				where, beMessageExistsRule, cons.MessageName, contract.messageList()))
			continue
		}
		errs = append(errs, consumedEntityErrors(ctx, sc, where, cons, contract.payloadProperties(msg))...)
		errs = append(errs, consumedHandlerErrors(ctx, sc, where, cons)...)
	}
	return errs
}

// ---------------------------------------------------------------------------
// BE01: the contract
// ---------------------------------------------------------------------------

// beContract is the part of an AsyncAPI 2.x document these rules read. Anything
// else in it is left alone: the document is stored verbatim, and mxcli is not
// an AsyncAPI validator.
type beContract struct {
	AsyncAPI   string         `yaml:"asyncapi"`
	Channels   map[string]any `yaml:"channels"`
	Components struct {
		Messages map[string]struct {
			Payload beContractSchema `yaml:"payload"`
		} `yaml:"messages"`
		Schemas map[string]beContractSchema `yaml:"schemas"`
	} `yaml:"components"`
}

// beContractSchema is a schema or a $ref to one.
type beContractSchema struct {
	Ref        string                        `yaml:"$ref"`
	Type       string                        `yaml:"type"`
	Properties map[string]beContractProperty `yaml:"properties"`
}

// beContractProperty is one payload property: its JSON Schema type and format,
// which together say which Mendix attribute type the event maps onto.
type beContractProperty struct {
	Type   string `yaml:"type"`
	Format string `yaml:"format"`
}

// parseBusinessEventContract reads the contract and refuses one that cannot
// describe a subscription.
//
// An empty document is what mxbuild throws on — `System.Exception: business
// events service should have definitions` (measured, §6) — and a document that
// is not AsyncAPI 2.x, or carries no channel, leaves the module with nothing to
// subscribe to.
func parseBusinessEventContract(text string) (*beContract, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("the contract is empty: mxbuild throws " +
			"\"business events service should have definitions\" on a consumed service without one. " +
			"Give it Contract: $$…$$ or ContractFile: '<path>'")
	}
	var c beContract
	if err := yaml.Unmarshal([]byte(text), &c); err != nil {
		return nil, fmt.Errorf("the contract is not valid YAML: %v. Export it from Studio Pro "+
			"(App Explorer, the published service, Export contract) or render it with "+
			"`mxcli export contract`", err)
	}
	major := strings.SplitN(c.AsyncAPI, ".", 2)[0]
	if major != "2" {
		stated := c.AsyncAPI
		if stated == "" {
			stated = "no asyncapi: key at all"
		}
		return nil, fmt.Errorf("the contract states %q: business events reads AsyncAPI 2.x, "+
			"which is what Studio Pro exports (asyncapi: 2.2.0)", stated)
	}
	if len(c.Channels) == 0 {
		return nil, fmt.Errorf("the contract declares no channels: the channel id is the Kafka topic " +
			"the consumer subscribes to, so without one it subscribes to nothing")
	}
	return &c, nil
}

// message resolves a message name against components.messages, case-sensitively
// — the $ref the channel carries is a literal key.
func (c *beContract) message(name string) (beContractSchema, bool) {
	m, ok := c.Components.Messages[name]
	if !ok {
		return beContractSchema{}, false
	}
	return m.Payload, true
}

// messageList names the messages the contract defines, for the BE02 message.
func (c *beContract) messageList() string {
	if len(c.Components.Messages) == 0 {
		return "none"
	}
	names := make([]string, 0, len(c.Components.Messages))
	for n := range c.Components.Messages {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// payloadProperties is the message payload's properties, following a $ref into
// components.schemas once. An unresolvable ref yields nothing, which makes BE03
// say nothing about attributes rather than claim they are all missing.
func (c *beContract) payloadProperties(payload beContractSchema) map[string]beContractProperty {
	if payload.Ref != "" {
		name := payload.Ref[strings.LastIndex(payload.Ref, "/")+1:]
		resolved, ok := c.Components.Schemas[name]
		if !ok {
			return nil
		}
		payload = resolved
	}
	return payload.Properties
}

// contractTypeLabel renders a payload property's encoding the way the contract
// writes it, for the warning: "integer/int64", or just "string" with no format.
func contractTypeLabel(p beContractProperty) string {
	if p.Format == "" {
		return p.Type
	}
	return p.Type + "/" + p.Format
}

// containsFold reports whether want is in list, ignoring case.
func containsFold(list []string, want string) bool {
	for _, s := range list {
		if strings.EqualFold(s, want) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// BE03: the consumed entity
// ---------------------------------------------------------------------------

// consumedEntityErrors is BE03: the entity a `consume` binds must be
// persistent, must extend BusinessEvents.ConsumedBusinessEvent, and should
// carry an attribute per payload property.
func consumedEntityErrors(ctx *ExecContext, sc *scriptContext, where string, cons *ast.BusinessEventConsumeDef,
	props map[string]beContractProperty) []error {
	if cons.Entity == "" {
		return nil
	}
	shape, known := consumedEntityShape(ctx, sc, cons.Entity)
	if !known {
		return nil // the reference checks report an entity that does not exist
	}

	var errs []error
	if !shape.persistent {
		errs = append(errs, mdlerrors.NewValidationf(
			"%s: %s: `consume %s` binds %s, which is not a persistent entity — the BusinessEvents module "+
				"commits each consumed event, so a non-persistent (or view) entity cannot hold one. "+
				"Declare it `create persistent entity %s extends %s (…)`",
			where, beEntityShapeRule, cons.MessageName, cons.Entity, cons.Entity, consumedBusinessEvent))
	}
	if !shape.extendsConsumed {
		chain := "nothing"
		if len(shape.ancestors) > 0 {
			chain = strings.Join(shape.ancestors, " -> ")
		}
		errs = append(errs, mdlerrors.NewValidationf(
			"%s: %s: `consume %s` binds %s, which does not extend %s (it extends %s) — the module retrieves "+
				"consumed events by that generalization and stores the event metadata on it, so an entity "+
				"outside the chain is never filled. This is the `CBE_` entity Studio Pro generates",
			where, beEntityShapeRule, cons.MessageName, cons.Entity, consumedBusinessEvent, chain))
	}

	for _, name := range sortedPropertyNames(props) {
		attrType, hasAttr := shape.attributes[strings.ToLower(name)]
		if !hasAttr {
			sc.warnings = append(sc.warnings, fmt.Sprintf(
				"%s: %s: the contract's %s payload carries %s, and %s declares no such attribute — the "+
					"property is dropped on consumption. mxbuild reports nothing; the mapping from a "+
					"contract to attributes is not validated at build time",
				where, beEntityShapeRule, cons.MessageName, name, cons.Entity))
			continue
		}
		accepted, known := businessevents.MendixAttributeTypesFor(props[name].Type, props[name].Format)
		if known && attrType != "" && !containsFold(accepted, attrType) {
			sc.warnings = append(sc.warnings, fmt.Sprintf(
				"%s: %s: the contract types %s.%s as %s, which a %s attribute holds, but %s.%s is %s",
				where, beEntityShapeRule, cons.MessageName, name,
				contractTypeLabel(props[name]), strings.Join(accepted, " or "),
				cons.Entity, name, attrType))
		}
	}
	return errs
}

// entityShape is what BE03 needs to know about a consumed entity: whether it is
// persistent, whether ConsumedBusinessEvent is on its generalization chain, the
// chain itself (for the message), and every attribute it has including inherited
// ones, lower-cased, mapped to the type name where that is known.
type entityShape struct {
	persistent      bool
	extendsConsumed bool
	ancestors       []string
	attributes      map[string]string
}

// consumedEntityShape reads the entity from the script when the script declares
// it, otherwise from the project. known is false when it resolves in neither.
//
// The generalization chain is matched by NAME, so the verdict holds whether or
// not the BusinessEvents module is in the project — a missing module is BE05's
// to report, once, and not a reason to go quiet about every entity.
func consumedEntityShape(ctx *ExecContext, sc *scriptContext, entityQN string) (entityShape, bool) {
	b, _ := ctx.Backend.(entityLookupBackend)
	shape := entityShape{attributes: map[string]string{}}

	decl, scripted := scriptEntityDecl(sc, entityQN)
	switch {
	case scripted:
		shape.persistent = decl.Kind == ast.EntityPersistent
		for _, a := range decl.Attributes {
			shape.attributes[strings.ToLower(a.Name)] = a.Type.Kind.String()
		}
	default:
		e, found := findEntityByQN(b, entityQN)
		if !found {
			return shape, false
		}
		// A view entity reports Persistable true (measured while building the
		// CDC write path): Source is the discriminator.
		shape.persistent = e.Persistable && e.Source == ""
		for _, a := range e.Attributes {
			name := strings.ToLower(a.Name)
			if a.Type != nil {
				shape.attributes[name] = a.Type.GetTypeName()
			} else {
				shape.attributes[name] = ""
			}
		}
	}

	// Walk up, collecting the chain and the inherited attributes. The walk stops
	// at the first ancestor that resolves in neither the script nor the project,
	// exactly as inheritedAttributeOwners does.
	seen := map[string]bool{strings.ToLower(entityQN): true}
	parent, ok := entityParent(b, sc, entityQN)
	for ok && parent != "" && !seen[strings.ToLower(parent)] {
		seen[strings.ToLower(parent)] = true
		shape.ancestors = append(shape.ancestors, parent)
		if strings.EqualFold(parent, consumedBusinessEvent) {
			shape.extendsConsumed = true
		}
		names, found := entityOwnAttributeNames(b, sc, parent)
		if !found {
			break
		}
		for _, n := range names {
			if _, dup := shape.attributes[strings.ToLower(n)]; !dup {
				// The ancestor's type is not read here: an inherited attribute
				// satisfies the name, and BE03 only judges a type it can see on
				// the entity itself.
				shape.attributes[strings.ToLower(n)] = ""
			}
		}
		parent, ok = entityParent(b, sc, parent)
	}
	return shape, true
}

// scriptEntityDecl is the script's own declaration of entityQN, if it has one.
func scriptEntityDecl(sc *scriptContext, entityQN string) (*ast.CreateEntityStmt, bool) {
	if sc == nil {
		return nil, false
	}
	for qn, decl := range sc.entityDecls {
		if strings.EqualFold(qn, entityQN) && decl != nil {
			return decl, true
		}
	}
	return nil, false
}

func sortedPropertyNames(props map[string]beContractProperty) []string {
	names := make([]string, 0, len(props))
	for n := range props {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ---------------------------------------------------------------------------
// BE04: the handler
// ---------------------------------------------------------------------------

// consumedHandlerErrors is BE04: the handler takes exactly one parameter, and
// that parameter accepts the consumed object.
//
// A parameter typed as an ANCESTOR of the consumed entity is accepted — it
// legitimately holds the object — so only an arity other than one, a
// non-object parameter, or an unrelated entity is reported.
func consumedHandlerErrors(ctx *ExecContext, sc *scriptContext, where string, cons *ast.BusinessEventConsumeDef) []error {
	if cons.Microflow == "" || cons.Entity == "" {
		return nil
	}
	params, known := handlerParameters(ctx, sc, cons.Microflow)
	if !known {
		return nil // not resolvable, or its parameter types are unavailable
	}
	if len(params) != 1 {
		return []error{mdlerrors.NewValidationf(
			"%s: %s: handler %s takes %d parameters, and the BusinessEvents module calls it with exactly "+
				"one — the consumed %s object. Declare it `create microflow %s ($Event: %s) { … }`",
			where, beHandlerShapeRule, cons.Microflow, len(params), cons.Entity,
			cons.Microflow, cons.Entity)}
	}
	p := params[0]
	if !p.Object || p.List {
		return []error{mdlerrors.NewValidationf(
			"%s: %s: handler %s takes $%s, which is not a single object of %s — the module calls the "+
				"handler with the one consumed event",
			where, beHandlerShapeRule, cons.Microflow, p.Name, cons.Entity)}
	}
	if p.Entity == "" {
		return nil // an object parameter whose entity could not be read
	}
	if acceptsConsumedEntity(ctx, sc, p.Entity, cons.Entity) {
		return nil
	}
	return []error{mdlerrors.NewValidationf(
		"%s: %s: handler %s takes $%s of %s, but `consume %s` binds %s — the module calls the handler "+
			"with that object, and %s is not on its generalization chain",
		where, beHandlerShapeRule, cons.Microflow, p.Name, p.Entity, cons.MessageName, cons.Entity, p.Entity)}
}

// handlerParameters is the microflow's parameters, from the script when the
// script declares it, otherwise from the project. known is false when the flow
// resolves in neither.
func handlerParameters(ctx *ExecContext, sc *scriptContext, flowQN string) ([]flowParam, bool) {
	if sc != nil && sc.microflows[flowQN] {
		sig, ok := sc.flowParams[strings.ToLower(flowQN)]
		if !ok || sig == nil {
			return nil, false // created here, signature not recorded
		}
		return sig.Params, true
	}
	mf := findMicroflowByQualifiedName(ctx, flowQN)
	if mf == nil {
		return nil, false
	}
	out := make([]flowParam, 0, len(mf.Parameters))
	for _, p := range mf.Parameters {
		fp := flowParam{Name: p.Name}
		switch t := p.Type.(type) {
		case *microflows.ObjectType:
			fp.Object, fp.Entity = true, t.EntityQualifiedName
		case *microflows.ListType:
			fp.Object, fp.List, fp.Entity = true, true, t.EntityQualifiedName
		}
		out = append(out, fp)
	}
	return out, true
}

// acceptsConsumedEntity reports whether a parameter of paramEntity can hold an
// object of consumedEntity: the same entity, or one of its ancestors.
func acceptsConsumedEntity(ctx *ExecContext, sc *scriptContext, paramEntity, consumedEntity string) bool {
	if strings.EqualFold(paramEntity, consumedEntity) {
		return true
	}
	b, _ := ctx.Backend.(entityLookupBackend)
	seen := map[string]bool{strings.ToLower(consumedEntity): true}
	parent, ok := entityParent(b, sc, consumedEntity)
	for ok && parent != "" && !seen[strings.ToLower(parent)] {
		if strings.EqualFold(parent, paramEntity) {
			return true
		}
		seen[strings.ToLower(parent)] = true
		parent, ok = entityParent(b, sc, parent)
	}
	return false
}

// ---------------------------------------------------------------------------
// BE05-BE07: the app around the service
// ---------------------------------------------------------------------------

// businessEventsModuleErrors is BE05: the marketplace module has to be there.
func businessEventsModuleErrors(ctx *ExecContext, sc *scriptContext) []error {
	if sc != nil && sc.modules[businessEventsModule] {
		return nil
	}
	mods, err := ctx.Backend.ListModules()
	if err != nil || len(mods) == 0 {
		return nil // cannot tell
	}
	for _, m := range mods {
		if strings.EqualFold(m.Name, businessEventsModule) {
			return nil
		}
	}
	return []error{mdlerrors.NewValidationf(
		"%s: the %s module is not in this project, and a consumed business event service is nothing "+
			"without it: it ships %s (the generalization the consumed entity needs) and "+
			"%s (what starts the Kafka consumer). Import Business Events from the Mendix Marketplace",
		beModulePresentRule, businessEventsModule, consumedBusinessEvent, startupBusinessEvents)}
}

// businessEventsRuntimeWarnings is BE06 and BE07: the module has to be started,
// and the broker has to be configured. Both warn — see the rule table above.
func businessEventsRuntimeWarnings(ctx *ExecContext, prog *ast.Program, sc *scriptContext) {
	if w := businessEventsStartupWarning(ctx, prog); w != "" {
		sc.warnings = append(sc.warnings, w)
	}
	sc.warnings = append(sc.warnings, businessEventsBrokerWarnings(ctx)...)
}

// mxbuildWiresStartupFrom is the Mendix version from which MxBuild starts the
// Business Events module itself, so an app that calls StartupBusinessEvents
// starts it TWICE.
//
// MEASURED on Mendix 11.16.1 with module 3.12.1 (ako/mxcli#2384 B1). MxBuild
// writes a microflow `BusinessEvents.ASu_BusinessEvents` into the compiled model
// (deployment/model/model.mdp) that exists neither in the project nor in the
// module package, and injects a call to it into the app's after-startup
// microflow. An OpenTelemetry trace of one boot shows the whole chain on one
// thread:
//
//	Microflow Fulfilment.ASU_Startup
//	  Microflow BusinessEvents.ASu_BusinessEvents
//	    Java action BusinessEvents.ConsumeBusinessEvent    (registers consumers)
//	    Java action BusinessEvents.StartupBusinessEvents   <- first start
//	  Java action BusinessEvents.StartupBusinessEvents     <- the app's call, second
//
// Which earlier versions do this is NOT measured, so the gate sits at the
// version that was. Below it nothing is reported: on a version that does not
// wire the startup the call is required, and warning against it would be the
// mirror image of the defect this replaced.
var mxbuildWiresStartupFrom = versions.SemVer{Major: 11, Minor: 16, Patch: 1}

// mxbuildWiresStartup reports whether this project's Mendix version is one where
// MxBuild starts the module itself. False when the version cannot be read — a
// warning is for something known, never for something assumed.
func mxbuildWiresStartup(ctx *ExecContext) bool {
	rpv := ctx.Backend.ProjectVersion()
	if rpv == nil {
		return false
	}
	pv := versions.SemVer{Major: rpv.MajorVersion, Minor: rpv.MinorVersion, Patch: rpv.PatchVersion}
	return pv.AtLeast(mxbuildWiresStartupFrom)
}

// businessEventsStartupWarning is BE06, now the other way round: it reports an
// app that starts the Business Events module a SECOND time.
//
// It used to demand the call, on the premise that nothing else starts the
// module. That premise was wrong on 11.16.1 (see mxbuildWiresStartupFrom), and
// the rule warned on correctly wired apps and then, once the call was removed,
// told them to add back the thing that causes the double start.
//
// The second start is not cosmetic. StartupService.init initialises
// EventRateMetricsService, LogCollectorService and HeartbeatService, and
// HeartbeatService.init assigns a fresh ScheduledExecutorService to its static
// pulseThread and schedules a new fixed-rate task without shutting down the pool
// already there. So a second start leaks a thread pool and leaves two heartbeat
// schedules running, of which ShutdownBusinessEvents stops only the last.
//
// Nothing is reported about a MISSING after-startup microflow. MxBuild injects
// its call into whichever microflow the app configures, and whether it supplies
// one when the app configures none has not been measured — asserting it would
// repeat the mistake this rule is recovering from.
func businessEventsStartupWarning(ctx *ExecContext, prog *ast.Program) string {
	if !mxbuildWiresStartup(ctx) {
		return ""
	}
	if scriptAddsBusinessEventsStartupCall(prog) {
		return fmt.Sprintf(
			"%s: this script calls %s, and from Mendix %s MxBuild already starts the module itself "+
				"(it generates BusinessEvents.ASu_BusinessEvents and calls it from the after-startup "+
				"microflow). The explicit call is a SECOND start: it leaks HeartbeatService's thread pool "+
				"and leaves two heartbeat schedules running. Remove it",
			beModuleStartedRule, startupBusinessEvents, mxbuildWiresStartupFrom.String())
	}
	ps, err := ctx.Backend.GetProjectSettings()
	if err != nil || ps == nil || ps.Model == nil {
		return "" // cannot tell
	}
	after := ps.Model.AfterStartupMicroflow
	if after == "" || !microflowStartsBusinessEvents(ctx, after) {
		return ""
	}
	return fmt.Sprintf(
		"%s: the after-startup microflow %s reaches %s, and from Mendix %s MxBuild already starts the "+
			"module itself (it generates BusinessEvents.ASu_BusinessEvents and calls it from the "+
			"after-startup microflow). That is a SECOND start: it leaks HeartbeatService's thread pool and "+
			"leaves two heartbeat schedules running, of which ShutdownBusinessEvents stops only the last. "+
			"Remove the call",
		beModuleStartedRule, after, startupBusinessEvents, mxbuildWiresStartupFrom.String())
}

// scriptAddsBusinessEventsStartupCall reports a script that calls
// StartupBusinessEvents in a flow it creates or alters. On a version where
// MxBuild wires the startup there is no reason to call it, so the call is
// reported wherever the script puts it.
func scriptAddsBusinessEventsStartupCall(prog *ast.Program) bool {
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateMicroflowStmt:
			if flowCallsStartupBusinessEvents(s.Body) {
				return true
			}
		case *ast.AlterFlowStmt:
			if s.Nanoflow {
				continue
			}
			for _, op := range s.Operations {
				if flowCallsStartupBusinessEvents(op.Body) {
					return true
				}
			}
		}
	}
	return false
}

// flowCallsStartupBusinessEvents reports a call to
// BusinessEvents.StartupBusinessEvents anywhere in an MDL flow body.
//
// `call java action` is the spelling that matters — the module ships the
// startup as a Java action (see microflowStartsBusinessEvents) — and
// `call microflow` is accepted for a version that wraps it.
func flowCallsStartupBusinessEvents(body []ast.MicroflowStatement) bool {
	found := false
	walkFlowStatements(body, func(s ast.MicroflowStatement) {
		switch c := s.(type) {
		case *ast.CallJavaActionStmt:
			if strings.EqualFold(c.ActionName.String(), startupBusinessEvents) {
				found = true
			}
		case *ast.CallMicroflowStmt:
			if strings.EqualFold(c.MicroflowName.String(), startupBusinessEvents) {
				found = true
			}
		}
	})
	return found
}

// microflowStartsBusinessEvents reports whether fromQN, or any microflow it
// calls, starts the Business Events module.
//
// In Business Events 3.12.1 `StartupBusinessEvents` is a **Java action**
// (`Private/Java_Actions`, javasource/businessevents/actions/
// StartupBusinessEvents.java); the module ships no microflow of that name. The
// first version of this walk followed microflow calls only, so it warned on
// every correctly wired consumer and told the author to add a call that was
// already there (ako/mxcli#2384 M4). A Java action call is what to look for.
//
// A microflow of that name is accepted too, rather than replacing one check
// with the other: the module is a marketplace dependency whose shape mxcli does
// not control, and an older or newer version wrapping the action in a microflow
// would otherwise bring the false warning straight back.
//
// Nanoflows are not followed: the after-startup microflow runs server-side and
// cannot call one.
func microflowStartsBusinessEvents(ctx *ExecContext, fromQN string) bool {
	seen := map[string]bool{}
	queue := []string{fromQN}
	for len(queue) > 0 {
		qn := queue[0]
		queue = queue[1:]
		key := strings.ToLower(qn)
		if seen[key] {
			continue
		}
		seen[key] = true
		// A microflow BY that name, for a module version that ships one.
		if strings.EqualFold(qn, startupBusinessEvents) {
			return true
		}
		mf := findMicroflowByQualifiedName(ctx, qn)
		if mf == nil {
			continue
		}
		calls, javaActions := flowCallTargets(mf.ObjectCollection)
		for _, ja := range javaActions {
			if strings.EqualFold(ja, startupBusinessEvents) {
				return true
			}
		}
		queue = append(queue, calls...)
	}
	return false
}

// flowCallTargets names the microflows and the Java actions a flow body calls,
// following loops. Two lists because they are resolved differently: a microflow
// is followed, a Java action is a leaf.
func flowCallTargets(oc *microflows.MicroflowObjectCollection) (microflowCalls, javaActionCalls []string) {
	var walk func(*microflows.MicroflowObjectCollection)
	walk = func(c *microflows.MicroflowObjectCollection) {
		if c == nil {
			return
		}
		for _, obj := range c.Objects {
			switch o := obj.(type) {
			case *microflows.ActionActivity:
				switch a := o.Action.(type) {
				case *microflows.MicroflowCallAction:
					if a != nil && a.MicroflowCall != nil && a.MicroflowCall.Microflow != "" {
						microflowCalls = append(microflowCalls, a.MicroflowCall.Microflow)
					}
				case *microflows.JavaActionCallAction:
					if a != nil && a.JavaAction != "" {
						javaActionCalls = append(javaActionCalls, a.JavaAction)
					}
				}
			case *microflows.LoopedActivity:
				walk(o.ObjectCollection)
			}
		}
	}
	walk(oc)
	return microflowCalls, javaActionCalls
}

// businessEventsBrokerWarnings is BE07: ServerUrl non-blank, and at least one of
// ChannelName / EventBrokerSpace set.
//
// MEASURED on Mendix 11.16.1: with either missing the app does not merely
// consume nothing, it fails to boot — StartupBusinessEvents throws in the
// after-startup action and the runtime shuts down ("Resolved value for constant:
// BusinessEvents.ServerUrl was not found or blank", then "Constant
// BusinessEvents.ChannelName or BusinessEvents.EventBrokerSpace should be set").
//
// Both are per-environment values, so an empty DEFAULT is a warning, not an
// error: `mxcli constant set` and `--constant` supply them for a dev run.
func businessEventsBrokerWarnings(ctx *ExecContext) []string {
	defaults, ok := businessEventConstantDefaults(ctx)
	if !ok {
		return nil // cannot tell
	}
	var out []string
	if strings.TrimSpace(defaults[strings.ToLower(beServerURLConstant)]) == "" {
		out = append(out, fmt.Sprintf(
			"%s: %s has no default value. Measured on 11.16.1, the app does not boot without it: "+
				"\"Resolved value for constant: %s was not found or blank\". "+
				"Set it per environment, or for a dev run with `mxcli constant set %s '<broker>:9092'`",
			beBrokerConfigRule, beServerURLConstant, beServerURLConstant, beServerURLConstant))
	}
	channel := strings.TrimSpace(defaults[strings.ToLower(beChannelNameConstant)])
	space := strings.TrimSpace(defaults[strings.ToLower(beBrokerSpaceConstant)])
	if channel == "" && space == "" {
		out = append(out, fmt.Sprintf(
			"%s: neither %s nor %s has a default value. Measured on 11.16.1, the app does not boot "+
				"without one: \"Constant %s or %s should be set\". Either names the Kafka topic's last "+
				"segment, so the consumer cannot resolve a topic without it",
			beBrokerConfigRule, beChannelNameConstant, beBrokerSpaceConstant,
			beChannelNameConstant, beBrokerSpaceConstant))
	}
	return out
}

// businessEventConstantDefaults maps the lower-cased qualified name of every
// constant in the BusinessEvents module to its default value. ok is false when
// the project cannot be asked — an empty map would otherwise read as "every
// constant is blank".
func businessEventConstantDefaults(ctx *ExecContext) (map[string]string, bool) {
	consts, err := ctx.Backend.ListConstants()
	if err != nil {
		return nil, false
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil, false
	}
	out := map[string]string{}
	for _, c := range consts {
		if c == nil {
			continue
		}
		qn := h.GetQualifiedName(c.ContainerID, c.Name)
		out[strings.ToLower(qn)] = c.DefaultValue
	}
	return out, true
}
