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

// businessEventsStartupWarning is BE06: BusinessEvents.StartupBusinessEvents has
// to be reachable from the after-startup microflow, or the app consumes nothing
// and every other check passes.
//
// It says nothing when the script itself touches the wiring — it creates a
// microflow that calls StartupBusinessEvents, or sets the after-startup
// microflow. What the project will look like afterwards is then the script's
// business, and warning from the stored state would be wrong.
func businessEventsStartupWarning(ctx *ExecContext, prog *ast.Program) string {
	if scriptWiresBusinessEventsStartup(prog) {
		return ""
	}
	ps, err := ctx.Backend.GetProjectSettings()
	if err != nil || ps == nil || ps.Model == nil {
		return "" // cannot tell
	}
	after := ps.Model.AfterStartupMicroflow
	if after == "" {
		return fmt.Sprintf(
			"%s: this app has no after-startup microflow, so %s never runs and no event is ever consumed. "+
				"Point App Settings > Runtime > After startup at a microflow that calls it "+
				"(`alter settings model set AfterStartupMicroflow '<Module.Microflow>'`)",
			beModuleStartedRule, startupBusinessEvents)
	}
	if microflowReaches(ctx, after, startupBusinessEvents) {
		return ""
	}
	return fmt.Sprintf(
		"%s: the after-startup microflow %s does not reach %s, so the consumer is never started and no "+
			"event is consumed — silently, with every other check green. Add a call to it there",
		beModuleStartedRule, after, startupBusinessEvents)
}

// scriptWiresBusinessEventsStartup reports whether the script itself arranges
// the startup call, in which case BE06 keeps quiet.
func scriptWiresBusinessEventsStartup(prog *ast.Program) bool {
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.AlterSettingsStmt:
			if strings.EqualFold(s.Section, "model") {
				if _, ok := s.Properties["AfterStartupMicroflow"]; ok {
					return true
				}
			}
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

// flowCallsStartupBusinessEvents reports a `call microflow
// BusinessEvents.StartupBusinessEvents` anywhere in an MDL flow body.
func flowCallsStartupBusinessEvents(body []ast.MicroflowStatement) bool {
	found := false
	walkFlowStatements(body, func(s ast.MicroflowStatement) {
		if c, ok := s.(*ast.CallMicroflowStmt); ok &&
			strings.EqualFold(c.MicroflowName.String(), startupBusinessEvents) {
			found = true
		}
	})
	return found
}

// microflowReaches reports whether target is called from fromQN, directly or
// through other microflows. Nanoflows are not followed: the after-startup
// microflow runs server-side and cannot call one.
func microflowReaches(ctx *ExecContext, fromQN, target string) bool {
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
		if strings.EqualFold(qn, target) {
			return true
		}
		mf := findMicroflowByQualifiedName(ctx, qn)
		if mf == nil {
			continue
		}
		queue = append(queue, calledMicroflows(mf.ObjectCollection)...)
	}
	return false
}

// calledMicroflows names the microflows a flow body calls, following loops.
func calledMicroflows(oc *microflows.MicroflowObjectCollection) []string {
	var out []string
	var walk func(*microflows.MicroflowObjectCollection)
	walk = func(c *microflows.MicroflowObjectCollection) {
		if c == nil {
			return
		}
		for _, obj := range c.Objects {
			switch o := obj.(type) {
			case *microflows.ActionActivity:
				if a, ok := o.Action.(*microflows.MicroflowCallAction); ok && a != nil && a.MicroflowCall != nil {
					if a.MicroflowCall.Microflow != "" {
						out = append(out, a.MicroflowCall.Microflow)
					}
				}
			case *microflows.LoopedActivity:
				walk(o.ObjectCollection)
			}
		}
	}
	walk(oc)
	return out
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
