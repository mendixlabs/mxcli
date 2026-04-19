// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// showBusinessEventServices displays a table of all business event service documents.
func showBusinessEventServices(ctx *ExecContext, inModule string) error {
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

	if len(filtered) == 0 {
		if inModule != "" {
			fmt.Fprintf(ctx.Output, "No business event services found in module %s\n", inModule)
		} else {
			fmt.Fprintln(ctx.Output, "No business event services found")
		}
		return nil
	}

	type row struct {
		module, qualifiedName, name            string
		msgCount, publishCount, subscribeCount int
	}
	var rows []row

	for _, svc := range filtered {
		modID := h.FindModuleID(svc.ContainerID)
		moduleName := h.GetModuleName(modID)
		qn := moduleName + "." + svc.Name
		r := row{module: moduleName, qualifiedName: qn, name: svc.Name}

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
		Columns: []string{"Module", "QualifiedName", "Service", "Messages", "Publish", "Subscribe"},
		Summary: fmt.Sprintf("(%d business event services)", len(filtered)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.module, r.qualifiedName, r.name, r.msgCount, r.publishCount, r.subscribeCount})
	}
	return writeResult(ctx, result)
}

// showBusinessEventClients displays a table of all business event client documents.
func showBusinessEventClients(ctx *ExecContext, inModule string) error {
	fmt.Fprintln(ctx.Output, "Business event clients are not yet implemented.")
	return nil
}

// showBusinessEvents displays a table of individual messages across all business event services.
func showBusinessEvents(ctx *ExecContext, inModule string) error {
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
		service, message, operation, entity string
		attrs                               int
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
						service:   svcQN,
						message:   msg.MessageName,
						operation: opStr,
						entity:    entityStr,
						attrs:     len(msg.Attributes),
					})
				}
			}
		}
	}

	if len(rows) == 0 {
		if inModule != "" {
			fmt.Fprintf(ctx.Output, "No business events found in module %s\n", inModule)
		} else {
			fmt.Fprintln(ctx.Output, "No business events found")
		}
		return nil
	}

	result := &TableResult{
		Columns: []string{"Service", "Message", "Operation", "Entity", "Attributes"},
		Summary: fmt.Sprintf("(%d business events)", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.service, r.message, r.operation, r.entity, r.attrs})
	}
	return writeResult(ctx, result)
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

	// Output MDL CREATE statement
	if found.Documentation != "" {
		outputJavadoc(ctx.Output, found.Documentation)
	}
	fmt.Fprintf(ctx.Output, "CREATE OR REPLACE BUSINESS EVENT SERVICE %s.%s\n", foundModule, found.Name)

	if found.Definition != nil {
		fmt.Fprintf(ctx.Output, "(\n")
		fmt.Fprintf(ctx.Output, "  ServiceName: '%s'", found.Definition.ServiceName)
		if found.Definition.EventNamePrefix != "" {
			fmt.Fprintf(ctx.Output, ",\n  EventNamePrefix: '%s'", found.Definition.EventNamePrefix)
		} else {
			fmt.Fprintf(ctx.Output, ",\n  EventNamePrefix: ''")
		}
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
				opStr := "PUBLISH"
				entityStr := ""
				if op, ok := opMap[msg.MessageName]; ok {
					if op.Operation == "subscribe" {
						opStr = "SUBSCRIBE"
					}
					if op.Entity != "" {
						entityStr = fmt.Sprintf("\n    ENTITY %s", op.Entity)
					}
				}

				fmt.Fprintf(ctx.Output, "  MESSAGE %s (%s) %s%s;\n",
					msg.MessageName, strings.Join(attrs, ", "), opStr, entityStr)
			}
		}

		fmt.Fprintf(ctx.Output, "};\n")
	}

	return nil
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

	// Check for existing service with same name (if not CREATE OR REPLACE)
	existingServices, _ := ctx.Backend.ListBusinessEventServices()
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, existing := range existingServices {
		existModID := h.FindModuleID(existing.ContainerID)
		existModName := h.GetModuleName(existModID)
		if strings.EqualFold(existModName, moduleName) && strings.EqualFold(existing.Name, stmt.Name.Name) {
			if stmt.CreateOrReplace {
				// Delete existing
				if err := ctx.Backend.DeleteBusinessEventService(existing.ID); err != nil {
					return mdlerrors.NewBackend("delete existing service", err)
				}
			} else {
				return mdlerrors.NewAlreadyExistsMsg("business event service", moduleName+"."+stmt.Name.Name, fmt.Sprintf("business event service already exists: %s.%s (use CREATE OR REPLACE to overwrite)", moduleName, stmt.Name.Name))
			}
		}
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

	// Build the service from AST
	svc := &model.BusinessEventService{
		ContainerID:   containerID,
		Name:          stmt.Name.Name,
		Documentation: stmt.Documentation,
		ExportLevel:   "Hidden",
	}

	// Build definition
	def := &model.BusinessEventDefinition{
		ServiceName:     stmt.ServiceName,
		EventNamePrefix: stmt.EventNamePrefix,
	}
	def.TypeName = "BusinessEvents$BusinessEventDefinition"

	// Create channels (one per message in our simplified model)
	for _, msgDef := range stmt.Messages {
		ch := &model.BusinessEventChannel{
			ChannelName: generateChannelName(),
		}
		ch.TypeName = "BusinessEvents$Channel"

		msg := &model.BusinessEventMessage{
			MessageName: msgDef.MessageName,
		}
		msg.TypeName = "BusinessEvents$Message"

		// Set publish/subscribe based on operation
		switch strings.ToUpper(msgDef.Operation) {
		case "PUBLISH":
			msg.CanSubscribe = true // Service publishes → others subscribe
		case "SUBSCRIBE":
			msg.CanPublish = true // Service subscribes → others publish
		}

		// Build attributes
		for _, attrDef := range msgDef.Attributes {
			attr := &model.BusinessEventAttribute{
				AttributeName: attrDef.Name,
				AttributeType: attrDef.TypeName,
			}
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
		op.TypeName = "BusinessEvents$ServiceOperation"
		svc.OperationImplementations = append(svc.OperationImplementations, op)
	}

	svc.Definition = def

	// Write to project
	if err := ctx.Backend.CreateBusinessEventService(svc); err != nil {
		return mdlerrors.NewBackend("create business event service", err)
	}

	fmt.Fprintf(ctx.Output, "Created business event service: %s.%s\n", moduleName, stmt.Name.Name)
	return nil
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

// generateChannelName generates a hex channel name (similar to Mendix Studio Pro).
func generateChannelName() string {
	// Generate a UUID-like hex string
	uuid := types.GenerateID()
	return strings.ReplaceAll(uuid, "-", "")
}

// Executor wrappers for unmigrated callers.
