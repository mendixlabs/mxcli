// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"os"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// listBusinessEventServices displays a table of all business event service documents.
func listBusinessEventServices(ctx *ExecContext, inModule string) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	services, err := ctx.Backend.ListBusinessEventServices()
	if err != nil {
		return mdlerrors.NewBackend("list business event services", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}

	var filtered []*model.BusinessEventService
	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		moduleName := h.GetModuleName(modID)
		if inModule != "" && !strings.EqualFold(moduleName, inModule) {
			continue
		}
		filtered = append(filtered, svc)
	}

	if len(filtered) == 0 && ctx.Format != FormatJSON {
		if inModule != "" {
			fmt.Fprintf(ctx.Output, "No business event services found in module %s\n", inModule)
		} else {
			fmt.Fprintln(ctx.Output, "No business event services found")
		}
		return nil
	}

	type row struct {
		module, qualifiedName, name, kind      string
		msgCount, publishCount, subscribeCount int
	}
	var rows []row

	for _, svc := range filtered {
		modID := h.FindModuleID(svc.ContainerID)
		moduleName := h.GetModuleName(modID)
		qn := moduleName + "." + svc.Name
		r := row{module: moduleName, qualifiedName: qn, name: svc.Name, kind: businessEventServiceKind(svc)}

		if svc.Definition != nil {
			for _, ch := range svc.Definition.Channels {
				r.msgCount += len(ch.Messages)
			}
		}
		for _, op := range svc.OperationImplementations {
			switch op.Operation {
			case "publish":
				r.publishCount++
			case "subscribe":
				r.subscribeCount++
			}
		}

		rows = append(rows, r)
	}

	result := &TableResult{
		Columns: []string{"Module", "QualifiedName", "Service", "Kind", "Messages", "Publish", "Subscribe"},
		Summary: fmt.Sprintf("(%d business event services)", len(filtered)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.module, r.qualifiedName, r.name, r.kind, r.msgCount, r.publishCount, r.subscribeCount})
	}
	return writeResult(ctx, result)
}

// businessEventServiceKind tells the two roles of
// BusinessEvents$BusinessEventService apart. Studio Pro stores a *published*
// service as a Definition with an empty Document, and a *consumed* one the other
// way round: no Definition, and the producing app's AsyncAPI contract in
// Document. Without this the list reads as a contradiction — a consumer shows 0
// messages and 1 subscribe, because its messages live in the contract and never
// in a Definition.
func businessEventServiceKind(svc *model.BusinessEventService) string {
	hasContract := strings.TrimSpace(svc.Document) != ""
	switch {
	case hasContract && svc.Definition == nil:
		return "consumed"
	case !hasContract && svc.Definition != nil:
		return "published"
	case hasContract:
		// Both: not a shape Studio Pro writes. Say so rather than pick one.
		return "mixed"
	default:
		return "empty"
	}
}

// listBusinessEventClients displays a table of all business event client documents.
func listBusinessEventClients(ctx *ExecContext, inModule string) error {
	fmt.Fprintln(ctx.Output, "Business event clients are not yet implemented.")
	return nil
}

// listBusinessEvents displays a table of individual messages across all business event services.
func listBusinessEvents(ctx *ExecContext, inModule string) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	services, err := ctx.Backend.ListBusinessEventServices()
	if err != nil {
		return mdlerrors.NewBackend("list business event services", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}

	type row struct {
		service, message, channel, operation, entity string
		attrs                                        int
	}
	var rows []row

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		moduleName := h.GetModuleName(modID)
		if inModule != "" && !strings.EqualFold(moduleName, inModule) {
			continue
		}

		svcQN := moduleName + "." + svc.Name

		// Build operation map: messageName -> ServiceOperation
		opMap := make(map[string]*model.ServiceOperation)
		for _, op := range svc.OperationImplementations {
			opMap[op.MessageName] = op
		}

		if svc.Definition != nil {
			for _, ch := range svc.Definition.Channels {
				for _, msg := range ch.Messages {
					opStr := ""
					entityStr := ""
					if op, ok := opMap[msg.MessageName]; ok {
						opStr = strings.ToUpper(op.Operation)
						entityStr = op.Entity
					}
					rows = append(rows, row{
						service: svcQN,
						message: msg.MessageName,
						// The channel name is the topic the broker derives
						// (`businessevents.<channel id>.<space>`), so it is what a
						// subscriber in another app — or a lake's topic config —
						// has to agree on.
						channel:   ch.ChannelName,
						operation: opStr,
						entity:    entityStr,
						attrs:     len(msg.Attributes),
					})
				}
			}
			continue
		}

		// A consumed service has no Definition: its events are the subscriptions,
		// and their channels live in the cached contract. Without this branch the
		// list reported only what the app publishes and silently omitted
		// everything it consumes.
		channels, attrCounts := consumedContractIndex(svc.Document)
		for _, op := range svc.OperationImplementations {
			rows = append(rows, row{
				service:   svcQN,
				message:   op.MessageName,
				channel:   channels[op.MessageName],
				operation: strings.ToUpper(op.Operation),
				entity:    op.Entity,
				attrs:     attrCounts[op.MessageName],
			})
		}
	}

	if len(rows) == 0 && ctx.Format != FormatJSON {
		if inModule != "" {
			fmt.Fprintf(ctx.Output, "No business events found in module %s\n", inModule)
		} else {
			fmt.Fprintln(ctx.Output, "No business events found")
		}
		return nil
	}

	result := &TableResult{
		Columns: []string{"Service", "Message", "Channel", "Operation", "Entity", "Attributes"},
		Summary: fmt.Sprintf("(%d business events)", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.service, r.message, r.channel, r.operation, r.entity, r.attrs})
	}
	return writeResult(ctx, result)
}

// consumedContractIndex reads a consumed service's cached AsyncAPI contract and
// returns, per message name, its channel id and its payload property count. A
// contract that is absent or does not parse yields empty maps: the caller still
// lists the subscriptions, just without the channel, because an unreadable
// contract is no reason to hide what the app subscribes to.
func consumedContractIndex(contract string) (channels map[string]string, attrCounts map[string]int) {
	channels, attrCounts = map[string]string{}, map[string]int{}
	if strings.TrimSpace(contract) == "" {
		return channels, attrCounts
	}
	doc, err := types.ParseAsyncAPI(contract)
	if err != nil {
		return channels, attrCounts
	}
	for _, ch := range doc.Channels {
		if ch.MessageRef != "" {
			if _, seen := channels[ch.MessageRef]; !seen {
				channels[ch.MessageRef] = ch.Name
			}
		}
	}
	for _, m := range doc.Messages {
		attrCounts[m.Name] = len(m.Properties)
	}
	return channels, attrCounts
}

// describeBusinessEventService outputs the full MDL description of a business event service.
func describeBusinessEventService(ctx *ExecContext, name ast.QualifiedName) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	services, err := ctx.Backend.ListBusinessEventServices()
	if err != nil {
		return mdlerrors.NewBackend("list business event services", err)
	}

	// Use hierarchy to resolve container IDs to module names
	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}

	// Find the service by qualified name
	var found *model.BusinessEventService
	var foundModule string
	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		moduleName := h.GetModuleName(modID)
		if strings.EqualFold(moduleName, name.Module) && strings.EqualFold(svc.Name, name.Name) {
			found = svc
			foundModule = moduleName
			break
		}
	}

	if found == nil {
		return mdlerrors.NewNotFound("business event service", name.String())
	}

	// A consumed service has no Definition of its own: it carries the producer's
	// AsyncAPI contract and one subscription per event. MDL cannot author that
	// shape yet, and emitting the statement header alone produced output that
	// does not parse. Describe it as comments instead — a parseable description
	// that states everything, and no statement to re-run (ako/mxcli#2381).
	if found.Definition == nil {
		describeConsumedBusinessEventService(ctx, foundModule, found)
		return nil
	}

	// Output MDL CREATE statement
	if found.Documentation != "" {
		outputJavadoc(ctx.Output, found.Documentation)
	}
	fmt.Fprintf(ctx.Output, "create or modify business event service %s.%s\n", foundModule, found.Name)

	fmt.Fprintf(ctx.Output, "(\n")
	fmt.Fprintf(ctx.Output, "  ServiceName: %s", mdlQuote(ctx, found.Definition.ServiceName))
	fmt.Fprintf(ctx.Output, ",\n  EventNamePrefix: %s", mdlQuote(ctx, found.Definition.EventNamePrefix))
	fmt.Fprintf(ctx.Output, "\n)\n")

	fmt.Fprintf(ctx.Output, "{\n")

	// Build operation map: messageName -> operation info
	opMap := make(map[string]*model.ServiceOperation)
	for _, op := range found.OperationImplementations {
		opMap[op.MessageName] = op
	}

	// Output messages
	for _, ch := range found.Definition.Channels {
		for _, msg := range ch.Messages {
			// Format attributes
			var attrs []string
			for _, a := range msg.Attributes {
				attrs = append(attrs, fmt.Sprintf("%s: %s", a.AttributeName, a.AttributeType))
			}

			// Determine operation from OperationImplementations
			opStr := "publish"
			entityStr := ""
			microflowStr := ""
			if op, ok := opMap[msg.MessageName]; ok {
				if op.Operation == "subscribe" {
					opStr = "subscribe"
				}
				if op.Entity != "" {
					entityStr = fmt.Sprintf("\n    entity %s", op.Entity)
				}
				// The grammar has `MICROFLOW qualifiedName` on the message
				// rule; describe never printed it, so describe → exec
				// unbound the handler (ako/mxcli#2381).
				if op.Microflow != "" {
					microflowStr = fmt.Sprintf("\n    microflow %s", op.Microflow)
				}
			}

			fmt.Fprintf(ctx.Output, "  message %s (%s) %s%s%s;\n",
				msg.MessageName, strings.Join(attrs, ", "), opStr, entityStr, microflowStr)
		}
	}

	fmt.Fprintf(ctx.Output, "};\n")

	return nil
}

// describeConsumedBusinessEventService writes a consumed service.
//
// Since MDL can author this shape (ako/mxcli#2384 part B) the output is the
// statement, with the contract inline as a $$…$$ block so it needs no side file
// and re-executes anywhere. Two cases have no statement to write, and both fall
// back to comments rather than emit something that would not round-trip:
//
//   - no contract stored — `consume` has nothing to take its event from, and
//     mxbuild throws on a contractless consumer anyway;
//   - a contract containing `$$` — DOLLAR_STRING is `'$$' .*? '$$'`, so the
//     block would end early, and there is no escape for it.
func describeConsumedBusinessEventService(ctx *ExecContext, moduleName string, svc *model.BusinessEventService) {
	contract := svc.Document
	switch {
	case strings.TrimSpace(contract) == "":
		describeConsumedAsComments(ctx, moduleName, svc,
			"It stores no AsyncAPI contract, so there is no statement to write: a",
			"consumed service without one makes mxbuild throw \"business events",
			"service should have definitions\".")
		return
	case strings.Contains(contract, "$$"):
		describeConsumedAsComments(ctx, moduleName, svc,
			"Its AsyncAPI contract contains `$$`, which ends a $$…$$ block and has no",
			"escape, so the contract cannot be written inline. Author this service with",
			"ContractFile: '<path>' instead, using the contract printed below.")
		return
	}

	if svc.Documentation != "" {
		outputJavadoc(ctx.Output, svc.Documentation)
	}
	fmt.Fprintf(ctx.Output, "create or modify business event service %s.%s\n", moduleName, svc.Name)
	// The stored text goes between the delimiters EXACTLY: unquoteDollarString
	// returns everything between them, so a newline added for looks is a
	// character the re-executed contract has and the stored one does not, and
	// the round trip stops being a fixed point — describe → exec reported
	// Modified on an unchanged service until this was byte-exact. The contract
	// normally ends in a newline, which puts the closing $$ on its own line.
	fmt.Fprintf(ctx.Output, "(\n  Contract: $$%s$$\n)\n{\n", svc.Document)
	for _, op := range svc.OperationImplementations {
		fmt.Fprintf(ctx.Output, "  consume %s", op.MessageName)
		if op.Entity != "" {
			fmt.Fprintf(ctx.Output, "\n    entity %s", op.Entity)
		}
		if op.Microflow != "" {
			fmt.Fprintf(ctx.Output, "\n    microflow %s", op.Microflow)
		}
		fmt.Fprintf(ctx.Output, ";\n")
	}
	fmt.Fprintf(ctx.Output, "};\n")
}

// describeConsumedAsComments states a consumed service that cannot be written
// as a statement: why not, its subscriptions, and the contract verbatim. The
// contract is the irreplaceable part — its channel keys are the topics a
// subscriber binds to, and nothing else in the model records them — so it is
// printed in full rather than summarised.
func describeConsumedAsComments(ctx *ExecContext, moduleName string, svc *model.BusinessEventService, why ...string) {
	comment := func(format string, args ...any) {
		line := fmt.Sprintf(format, args...)
		if line == "" {
			fmt.Fprintln(ctx.Output, "--")
			return
		}
		fmt.Fprintf(ctx.Output, "-- %s\n", line)
	}

	comment("%s.%s is a CONSUMED business event service.", moduleName, svc.Name)
	comment("")
	for _, line := range why {
		comment("%s", line)
	}

	if svc.Documentation != "" {
		comment("")
		comment("Documentation:")
		for _, line := range strings.Split(strings.ReplaceAll(svc.Documentation, "\r\n", "\n"), "\n") {
			comment("  %s", line)
		}
	}

	comment("")
	if len(svc.OperationImplementations) == 0 {
		comment("Subscriptions: none.")
	} else {
		comment("Subscriptions:")
		for _, op := range svc.OperationImplementations {
			comment("  %s (%s)", op.MessageName, orPlaceholder(op.Operation, "subscribe"))
			comment("    entity     %s", orPlaceholder(op.Entity, "(none)"))
			comment("    microflow  %s", orPlaceholder(op.Microflow, "(none)"))
		}
	}

	comment("")
	if strings.TrimSpace(svc.Document) == "" {
		comment("Contract: none stored.")
		return
	}
	comment("Contract (AsyncAPI, verbatim). Each key under `channels:` is the channel")
	comment("id the producer minted; the Kafka topic is derived from it, so this is the")
	comment("only record of what a subscriber binds to:")
	for _, line := range strings.Split(strings.ReplaceAll(strings.TrimRight(svc.Document, "\n"), "\r\n", "\n"), "\n") {
		comment("  %s", line)
	}
}

// orPlaceholder is value, or placeholder when value is empty.
func orPlaceholder(value, placeholder string) string {
	if value == "" {
		return placeholder
	}
	return value
}

// createBusinessEventService creates a new business event service from an AST statement.
func createBusinessEventService(ctx *ExecContext, stmt *ast.CreateBusinessEventServiceStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	moduleName := stmt.Name.Module
	module, err := findModule(ctx, moduleName)
	if err != nil {
		return mdlerrors.NewNotFound("module", moduleName)
	}

	// Check for existing service with same name
	existingServices, _ := ctx.Backend.ListBusinessEventServices()
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	var existingID model.ID
	// Target the live service and carry its exclusion forward (#914).
	existingExcluded := false
	// Placement is carried forward the same way when the statement is silent
	// about folders (#932).
	var existingContainerID model.ID
	var existingDocumentation string
	var existingSvc *model.BusinessEventService
	haveExistingSvc := false
	if existing, ok := pickLive(existingServices,
		func(svc *model.BusinessEventService) bool {
			return strings.EqualFold(h.GetModuleName(h.FindModuleID(svc.ContainerID)), moduleName) &&
				strings.EqualFold(svc.Name, stmt.Name.Name)
		},
		func(svc *model.BusinessEventService) bool { return svc.Excluded },
	); ok {
		if !stmt.CreateOrModify {
			return mdlerrors.NewAlreadyExistsMsg("business event service", moduleName+"."+stmt.Name.Name, fmt.Sprintf("business event service already exists: %s.%s (use create or modify to update)", moduleName, stmt.Name.Name))
		}
		// Guard, don't drop (ADR-0005): a stored AsyncAPI contract marks a
		// *consumed* service, and a published-shaped statement would null it and
		// every handler microflow with it — the contract being the only record of
		// the channel id a subscriber binds to (ako/mxcli#2381).
		//
		// A guard has a lifecycle: now that MDL can express a consumer, a
		// statement that names a contract is restating the document, not
		// destroying it, and must pass. Only the published shape still refuses.
		if strings.TrimSpace(existing.Document) != "" && !stmt.IsConsumer() {
			return mdlerrors.NewValidationf(
				"%s.%s stores an AsyncAPI contract (%d bytes) — the shape of a consumed "+
					"business event service, which MDL cannot author yet. That contract holds the "+
					"channel id its subscribers bind to, and this statement can only build a "+
					"published service, so applying it would destroy the contract and the handler "+
					"microflow of every subscription, with no way to recover either from the model. "+
					"Use `describe business event service %s.%s` to see what is stored; "+
					"`drop business event service %s.%s` first if you do mean to replace it with a published one.",
				moduleName, stmt.Name.Name, len(existing.Document),
				moduleName, stmt.Name.Name, moduleName, stmt.Name.Name)
		}
		existingID = existing.ID
		existingExcluded = existing.Excluded
		existingContainerID = existing.ContainerID
		existingDocumentation = existing.Documentation
		existingSvc = existing
		haveExistingSvc = true
	}

	// Resolve folder if specified
	containerID := module.ID
	if stmt.Folder != "" {
		folderID, err := resolveFolder(ctx, module.ID, stmt.Folder)
		if err != nil {
			return mdlerrors.NewBackend(fmt.Sprintf("resolve folder '%s'", stmt.Folder), err)
		}
		containerID = folderID
	}

	// Build the service from AST, preserving existing ID on OR MODIFY
	svc := &model.BusinessEventService{
		ContainerID:   containerID,
		Name:          stmt.Name.Name,
		Documentation: stmt.Documentation,
		ExportLevel:   "Hidden",
		Excluded:      existingExcluded,
	}
	// A rewrite that carried no doc comment keeps the stored one (#1018).
	if haveExistingSvc {
		svc.Documentation = carriedDocumentation(stmt.DocumentationSet, stmt.Documentation, existingDocumentation)
	}
	if existingID != "" {
		svc.ID = existingID
	}

	// Build definition. On a rewrite every identity the stored service holds is
	// carried by name (mendixlabs/mxcli#1525): the ChannelName is the topic
	// subscribers listen on, so minting a new one per run moved every topic of a
	// deployed app and made an unchanged re-run a write.
	carry := newBusinessEventIdentityCarry(existingSvc)

	// A consumed service: the contract plus one subscription per `consume`.
	// It has no Definition of its own — that is what tells the two roles apart
	// in storage — so it is built and written here rather than below.
	if stmt.IsConsumer() {
		contract, err := resolveBusinessEventContract(ctx, stmt)
		if err != nil {
			return err
		}
		svc.Document = contract
		svc.Definition = nil
		for _, cons := range stmt.Consumes {
			op := &model.ServiceOperation{
				MessageName: cons.MessageName,
				Operation:   "subscribe",
				Entity:      cons.Entity,
				Microflow:   cons.Microflow,
			}
			if storedOp := carry.operationFor(cons.MessageName); storedOp != nil {
				op.ID = storedOp.ID
				if op.Microflow == "" {
					op.Microflow = storedOp.Microflow
				}
				if op.Entity == "" {
					op.Entity = storedOp.Entity
				}
			}
			op.TypeName = "BusinessEvents$ServiceOperation"
			svc.OperationImplementations = append(svc.OperationImplementations, op)
		}
		return writeBusinessEventService(ctx, svc, stmt, moduleName, existingID, existingContainerID)
	}
	def := &model.BusinessEventDefinition{
		ServiceName:     stmt.ServiceName,
		EventNamePrefix: stmt.EventNamePrefix,
	}
	def.ID = carry.definitionID()
	def.TypeName = "BusinessEvents$BusinessEventDefinition"

	// Create channels (one per message in our simplified model)
	for _, msgDef := range stmt.Messages {
		ch := &model.BusinessEventChannel{}
		ch.TypeName = "BusinessEvents$Channel"

		msg := &model.BusinessEventMessage{
			MessageName: msgDef.MessageName,
		}
		msg.TypeName = "BusinessEvents$Message"
		storedMsg := carry.channelFor(msgDef.MessageName, ch, msg)

		// Set publish/subscribe based on operation
		switch strings.ToLower(msgDef.Operation) {
		case "publish":
			msg.CanSubscribe = true // Service publishes → others subscribe
		case "subscribe":
			msg.CanPublish = true // Service subscribes → others publish
		}

		// Build attributes
		for _, attrDef := range msgDef.Attributes {
			attr := &model.BusinessEventAttribute{
				AttributeName: attrDef.Name,
				AttributeType: attrDef.TypeName,
			}
			attr.ID = storedAttributeID(storedMsg, attrDef.Name)
			attr.TypeName = "BusinessEvents$MessageAttribute"
			msg.Attributes = append(msg.Attributes, attr)
		}

		ch.Messages = append(ch.Messages, msg)
		def.Channels = append(def.Channels, ch)

		// Create operation implementation
		op := &model.ServiceOperation{
			MessageName: msgDef.MessageName,
			Operation:   strings.ToLower(msgDef.Operation),
			Entity:      msgDef.Entity,
			Microflow:   msgDef.Microflow,
		}
		if storedOp := carry.operationFor(msgDef.MessageName); storedOp != nil {
			op.ID = storedOp.ID
			// A statement silent about the handler keeps the stored one, the
			// way a silent doc comment does (#1018).
			if op.Microflow == "" {
				op.Microflow = storedOp.Microflow
			}
		}
		op.TypeName = "BusinessEvents$ServiceOperation"
		svc.OperationImplementations = append(svc.OperationImplementations, op)
	}

	svc.Definition = def

	return writeBusinessEventService(ctx, svc, stmt, moduleName, existingID, existingContainerID)
}

// writeBusinessEventService stores a built service, created or rewritten. Both
// roles share it, so a consumer gets the same folder handling and the same
// Created/Modified reporting as a publisher.
func writeBusinessEventService(ctx *ExecContext, svc *model.BusinessEventService, stmt *ast.CreateBusinessEventServiceStmt, moduleName string, existingID, existingContainerID model.ID) error {
	if existingID != "" {
		if stmt.Folder == "" {
			svc.ContainerID = existingContainerID
		}
		if err := ctx.Backend.UpdateBusinessEventService(svc); err != nil {
			return mdlerrors.NewBackend("update business event service", err)
		}
		if _, err := applyDocumentFolder(ctx, svc.ID, existingContainerID, svc.ContainerID); err != nil {
			return err
		}
		ctx.ReportMutation("Modified", "business event service: %s.%s", moduleName, stmt.Name.Name)
		return nil
	}
	if err := ctx.Backend.CreateBusinessEventService(svc); err != nil {
		return mdlerrors.NewBackend("create business event service", err)
	}
	fmt.Fprintf(ctx.Output, "Created business event service: %s.%s\n", moduleName, stmt.Name.Name)
	return nil
}

// resolveBusinessEventContract returns the AsyncAPI text the statement names,
// whether written inline or pointed at by a path.
//
// The text is what gets stored, so a ContractFile is read now and nothing
// afterwards depends on the file: the same service re-executed on another
// machine, or described and re-run, carries its own contract. An unreadable
// file is an error rather than an empty contract — mxbuild throws
// "business events service should have definitions" on one of those, which is a
// far worse place to find out.
func resolveBusinessEventContract(ctx *ExecContext, stmt *ast.CreateBusinessEventServiceStmt) (string, error) {
	if stmt.ContractFile == "" {
		return stmt.Contract, nil
	}
	path, err := ctx.ResolveScriptRelative(stmt.ContractFile)
	if err != nil {
		return "", mdlerrors.NewValidationf("ContractFile %q: %v", stmt.ContractFile, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", mdlerrors.NewValidationf("ContractFile %q: %v", stmt.ContractFile, err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return "", mdlerrors.NewValidationf("ContractFile %q is empty: a consumed business event service without a contract makes mxbuild throw \"business events service should have definitions\"", stmt.ContractFile)
	}
	return string(data), nil
}

// dropBusinessEventService deletes a business event service.
func dropBusinessEventService(ctx *ExecContext, stmt *ast.DropBusinessEventServiceStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	services, err := ctx.Backend.ListBusinessEventServices()
	if err != nil {
		return mdlerrors.NewBackend("list business event services", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		moduleName := h.GetModuleName(modID)
		if strings.EqualFold(moduleName, stmt.Name.Module) && strings.EqualFold(svc.Name, stmt.Name.Name) {
			if err := ctx.Backend.DeleteBusinessEventService(svc.ID); err != nil {
				return mdlerrors.NewBackend("delete business event service", err)
			}
			fmt.Fprintf(ctx.Output, "Dropped business event service: %s.%s\n", moduleName, svc.Name)
			return nil
		}
	}

	return mdlerrors.NewNotFound("business event service", stmt.Name.String())
}

// businessEventIdentityCarry hands the identities of a stored business event
// service to the rewrite that replaces it, keyed by message name (and attribute
// name), never by position: a reordered script must not swap two topics.
//
// The ChannelName is what makes this more than $ID hygiene. It is the channel's
// identity outside the model — the topic name publishers and subscribers in
// other apps agree on — so a rewrite that re-mints it silently disconnects every
// subscriber on the next deploy, with a perfectly valid model. Element $IDs are
// carried alongside so an unchanged rewrite equals the stored unit and is elided
// (ADR-0008).
type businessEventIdentityCarry struct {
	stored   *model.BusinessEventService
	messages map[string]storedBusinessEventMessage
	claimed  map[*model.BusinessEventChannel]bool
	ops      map[string]*model.ServiceOperation
}

type storedBusinessEventMessage struct {
	channel *model.BusinessEventChannel
	message *model.BusinessEventMessage
}

func newBusinessEventIdentityCarry(stored *model.BusinessEventService) *businessEventIdentityCarry {
	c := &businessEventIdentityCarry{
		stored:   stored,
		messages: map[string]storedBusinessEventMessage{},
		claimed:  map[*model.BusinessEventChannel]bool{},
		ops:      map[string]*model.ServiceOperation{},
	}
	if stored == nil {
		return c
	}
	if stored.Definition != nil {
		for _, ch := range stored.Definition.Channels {
			for _, m := range ch.Messages {
				key := strings.ToLower(m.MessageName)
				if _, dup := c.messages[key]; !dup {
					c.messages[key] = storedBusinessEventMessage{channel: ch, message: m}
				}
			}
		}
	}
	for _, op := range stored.OperationImplementations {
		key := strings.ToLower(op.MessageName)
		if _, dup := c.ops[key]; !dup {
			c.ops[key] = op
		}
	}
	return c
}

func (c *businessEventIdentityCarry) definitionID() model.ID {
	if c.stored == nil || c.stored.Definition == nil {
		return ""
	}
	return c.stored.Definition.ID
}

// channelFor fills ch and msg with the stored identities of the message named
// name and returns the stored message (nil for a new one). A new message gets a
// freshly generated ChannelName. A stored channel is handed out once: Studio Pro
// can put several messages in one channel, MDL writes one channel per message,
// and two channels sharing a ChannelName is a duplicate topic (PR #533).
func (c *businessEventIdentityCarry) channelFor(name string, ch *model.BusinessEventChannel, msg *model.BusinessEventMessage) *model.BusinessEventMessage {
	key := strings.ToLower(name)
	s, ok := c.messages[key]
	if !ok {
		ch.ChannelName = generateChannelName()
		return nil
	}
	delete(c.messages, key)
	msg.ID = s.message.ID
	if c.claimed[s.channel] || s.channel.ChannelName == "" {
		ch.ChannelName = generateChannelName()
	} else {
		c.claimed[s.channel] = true
		ch.ID = s.channel.ID
		ch.ChannelName = s.channel.ChannelName
	}
	return s.message
}

// operationFor is the stored operation implementation for the named message,
// handed out once, or nil. The caller takes its $ID, and its Microflow when the
// statement is silent: MDL has `MICROFLOW qualifiedName` on the message rule, so
// a rewrite that omits it is a partial statement, not a request to unbind the
// handler (ako/mxcli#2381).
func (c *businessEventIdentityCarry) operationFor(messageName string) *model.ServiceOperation {
	key := strings.ToLower(messageName)
	op, ok := c.ops[key]
	if !ok {
		return nil
	}
	delete(c.ops, key)
	return op
}

// storedAttributeID is the $ID of the stored attribute named name, or "".
func storedAttributeID(stored *model.BusinessEventMessage, name string) model.ID {
	if stored == nil {
		return ""
	}
	for _, a := range stored.Attributes {
		if strings.EqualFold(a.AttributeName, name) {
			return a.ID
		}
	}
	return ""
}

// generateChannelName generates a hex channel name (similar to Mendix Studio Pro).
func generateChannelName() string {
	// Generate a UUID-like hex string
	uuid := types.GenerateID()
	return strings.ReplaceAll(uuid, "-", "")
}

// Executor wrappers for unmigrated callers.
