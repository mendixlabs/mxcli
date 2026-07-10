// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/JordtenBulte-OLC/mxcli/internal/pathutil"
	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/mdl/openapi"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// safeIdent returns an identifier safe for MDL output. Always double-quotes
// the name to avoid clashes with the 600+ MDL keywords/tokens. This guarantees
// DESCRIBE output round-trips through the parser regardless of what JSON field
// names the external API returns (e.g., "Host", "Data", "Method").
func safeIdent(name string) string {
	return `"` + name + `"`
}

// listRestClients handles SHOW REST CLIENTS [IN module] command.
func listRestClients(ctx *ExecContext, moduleName string) error {

	services, err := ctx.Backend.ListConsumedRestServices()
	if err != nil {
		return mdlerrors.NewBackend("list consumed rest services", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	type row struct {
		module        string
		qualifiedName string
		baseUrl       string
		auth          string
		ops           int
	}
	var rows []row

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName != "" && !strings.EqualFold(modName, moduleName) {
			continue
		}

		auth := "none"
		if svc.Authentication != nil {
			auth = strings.ToUpper(svc.Authentication.Scheme)
		}

		baseUrl := svc.BaseUrl
		if len(baseUrl) > 60 {
			baseUrl = baseUrl[:57] + "..."
		}

		qn := modName + "." + svc.Name
		rows = append(rows, row{modName, qn, baseUrl, auth, len(svc.Operations)})
	}

	if len(rows) == 0 && ctx.Format != FormatJSON {
		fmt.Fprintln(ctx.Output, "No consumed rest services found.")
		return nil
	}

	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].qualifiedName) < strings.ToLower(rows[j].qualifiedName)
	})

	result := &TableResult{
		Columns: []string{"Module", "QualifiedName", "BaseURL", "Auth", "Operations"},
		Summary: fmt.Sprintf("(%d rest clients)", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.module, r.qualifiedName, r.baseUrl, r.auth, r.ops})
	}
	return writeResult(ctx, result)
}

// describeRestClient handles DESCRIBE REST CLIENT command.
func describeRestClient(ctx *ExecContext, name ast.QualifiedName) error {

	services, err := ctx.Backend.ListConsumedRestServices()
	if err != nil {
		return mdlerrors.NewBackend("list consumed rest services", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)
		if strings.EqualFold(modName, name.Module) && strings.EqualFold(svc.Name, name.Name) {
			return outputConsumedRestServiceMDL(ctx, svc, modName)
		}
	}

	return mdlerrors.NewNotFound("consumed rest service", name.String())
}

// outputConsumedRestServiceMDL outputs a consumed REST service in the property-based { } format.
func outputConsumedRestServiceMDL(ctx *ExecContext, svc *model.ConsumedRestService, moduleName string) error {
	w := ctx.Output

	if svc.Documentation != "" {
		outputJavadoc(w, svc.Documentation)
	}

	fmt.Fprintf(w, "create rest client %s.%s (\n", moduleName, svc.Name)
	fmt.Fprintf(w, "  BaseUrl: '%s',\n", svc.BaseUrl)
	if svc.Authentication == nil {
		fmt.Fprintln(w, "  Authentication: none")
	} else {
		username := resolveAndFormatRestAuthValue(ctx, svc.Authentication.Username)
		password := resolveAndFormatRestAuthValue(ctx, svc.Authentication.Password)
		fmt.Fprintf(w, "  Authentication: basic (Username: %s, Password: %s)\n",
			username, password)
	}
	fmt.Fprintln(w, ")")
	fmt.Fprintln(w, "{")

	for i, op := range svc.Operations {
		if i > 0 {
			fmt.Fprintln(w)
		}
		outputRestOperation(w, op)
	}

	fmt.Fprintln(w, "};")
	return nil
}

// outputRestOperation writes a single operation in the new { Key: Value } format.
func outputRestOperation(w io.Writer, op *model.RestClientOperation) {
	if op.Documentation != "" {
		outputJavadocIndented(w, op.Documentation, "  ")
	}

	fmt.Fprintf(w, "  operation %s {\n", op.Name)
	fmt.Fprintf(w, "    Method: %s,\n", strings.ToLower(op.HttpMethod))
	fmt.Fprintf(w, "    Path: '%s',\n", op.Path)

	// Parameters: ($var: Type, ...)
	if len(op.Parameters) > 0 {
		var params []string
		for _, p := range op.Parameters {
			params = append(params, fmt.Sprintf("$%s: %s", p.Name, p.DataType))
		}
		fmt.Fprintf(w, "    Parameters: (%s),\n", strings.Join(params, ", "))
	}

	// Query: ($var: Type, ...)
	if len(op.QueryParameters) > 0 {
		var params []string
		for _, q := range op.QueryParameters {
			params = append(params, fmt.Sprintf("$%s: %s", q.Name, q.DataType))
		}
		fmt.Fprintf(w, "    Query: (%s),\n", strings.Join(params, ", "))
	}

	// Headers: ('Name' = 'Value', ...)
	if len(op.Headers) > 0 {
		var hdrs []string
		for _, h := range op.Headers {
			hdrs = append(hdrs, fmt.Sprintf("'%s' = '%s'", h.Name, h.Value))
		}
		fmt.Fprintf(w, "    Headers: (%s),\n", strings.Join(hdrs, ", "))
	}

	// Body
	if op.BodyType != "" {
		switch strings.ToLower(op.BodyType) {
		case "template":
			fmt.Fprintf(w, "    Body: template '%s',\n", strings.ReplaceAll(op.BodyVariable, "'", "''"))
		case "export_mapping":
			if op.BodyVariable != "" && len(op.BodyMappings) > 0 {
				fmt.Fprintf(w, "    Body: mapping %s {\n", op.BodyVariable)
				writeExportMappings(w, op.BodyMappings, 6)
				fmt.Fprintln(w, "    },")
			} else if op.BodyVariable != "" {
				fmt.Fprintf(w, "    Body: mapping %s,\n", op.BodyVariable)
			}
		default:
			fmt.Fprintf(w, "    Body: %s from %s,\n", strings.ToLower(op.BodyType), op.BodyVariable)
		}
	}

	// Timeout
	if op.Timeout > 0 {
		fmt.Fprintf(w, "    Timeout: %d,\n", op.Timeout)
	}

	// Response
	switch strings.ToLower(op.ResponseType) {
	case "none":
		fmt.Fprintln(w, "    Response: none")
	case "json":
		if op.ResponseVariable != "" {
			fmt.Fprintf(w, "    Response: json as %s\n", op.ResponseVariable)
		} else {
			fmt.Fprintln(w, "    Response: json")
		}
	case "string":
		fmt.Fprintf(w, "    Response: string as %s\n", op.ResponseVariable)
	case "file":
		fmt.Fprintf(w, "    Response: file as %s\n", op.ResponseVariable)
	case "status":
		fmt.Fprintf(w, "    Response: status as %s\n", op.ResponseVariable)
	case "mapping":
		if op.ResponseEntity != "" && len(op.ResponseMappings) > 0 {
			fmt.Fprintf(w, "    Response: mapping %s {\n", op.ResponseEntity)
			writeResponseMappings(w, op.ResponseMappings, 6)
			fmt.Fprintln(w, "    }")
		} else if op.ResponseEntity != "" {
			fmt.Fprintf(w, "    Response: mapping %s\n", op.ResponseEntity)
		} else {
			fmt.Fprintln(w, "    Response: none")
		}
	default:
		fmt.Fprintln(w, "    Response: none")
	}

	fmt.Fprintln(w, "  }")
}

// writeResponseMappings writes import-direction mappings (JSON → Entity): EntityAttr = jsonField.
// Matches the import mapping syntax: CREATE Association/Entity = jsonField { ... }.
func writeResponseMappings(w io.Writer, mappings []*model.RestResponseMapping, indent int) {
	pad := strings.Repeat(" ", indent)
	for _, m := range mappings {
		if m.Entity != "" {
			// Object mapping → nested entity via association (import style: CREATE Assoc/Entity = jsonField)
			fmt.Fprintf(w, "%screate %s/%s = %s", pad, m.Association, m.Entity, m.ExposedName)
			if len(m.Children) > 0 {
				fmt.Fprintln(w, " {")
				writeResponseMappings(w, m.Children, indent+2)
				fmt.Fprintf(w, "%s},\n", pad)
			} else {
				fmt.Fprintln(w, " {},")
			}
		} else {
			// Value mapping: EntityAttr = jsonField (import direction)
			jsonComment := ""
			if m.JsonPath != "" {
				jsonComment = fmt.Sprintf("  -- %s", m.JsonPath)
			}
			fmt.Fprintf(w, "%s%s = %s,%s\n", pad, safeIdent(m.Attribute), safeIdent(m.ExposedName), jsonComment)
		}
	}
}

// writeExportMappings writes export-direction mappings (Entity → JSON): jsonField = EntityAttr.
// Matches the export mapping syntax.
func writeExportMappings(w io.Writer, mappings []*model.RestResponseMapping, indent int) {
	pad := strings.Repeat(" ", indent)
	for _, m := range mappings {
		if m.Entity != "" {
			fmt.Fprintf(w, "%s%s/%s = %s", pad, m.Association, m.Entity, m.ExposedName)
			if len(m.Children) > 0 {
				fmt.Fprintln(w, " {")
				writeExportMappings(w, m.Children, indent+2)
				fmt.Fprintf(w, "%s},\n", pad)
			} else {
				fmt.Fprintln(w, " {},")
			}
		} else {
			// Export direction: jsonField = EntityAttr
			fmt.Fprintf(w, "%s%s = %s,\n", pad, safeIdent(m.ExposedName), safeIdent(m.Attribute))
		}
	}
}

// createRestClient handles CREATE REST CLIENT statement.
func createRestClient(ctx *ExecContext, stmt *ast.CreateRestClientStmt) error {
	// Spec-driven path: OpenAPI: property present
	if stmt.OpenApiPath != "" {
		return createRestClientFromSpec(ctx, stmt)
	}

	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	// Version pre-check: REST clients require 10.1+
	if err := checkFeature(ctx, "integration", "rest_client_basic",
		"create rest client",
		"upgrade your project to 10.1+"); err != nil {
		return err
	}

	moduleName := stmt.Name.Module
	module, err := findModule(ctx, moduleName)
	if err != nil {
		return mdlerrors.NewNotFound("module", moduleName)
	}

	// Check for existing service with same name
	existingServices, err := ctx.Backend.ListConsumedRestServices()
	if err != nil {
		return mdlerrors.NewBackend("list rest clients", err)
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	var preservedID model.ID
	wasModified := false
	for _, existing := range existingServices {
		existModID := h.FindModuleID(existing.ContainerID)
		existModName := h.GetModuleName(existModID)
		if strings.EqualFold(existModName, moduleName) && strings.EqualFold(existing.Name, stmt.Name.Name) {
			if stmt.CreateOrModify {
				// Preserve the existing ID so SEND REST REQUEST references stay valid after replace.
				preservedID = existing.ID
				wasModified = true
				if err := ctx.Backend.DeleteConsumedRestService(existing.ID); err != nil {
					return mdlerrors.NewBackend("delete existing rest client", err)
				}
			} else {
				return mdlerrors.NewAlreadyExistsMsg("rest client", moduleName+"."+stmt.Name.Name, fmt.Sprintf("rest client already exists: %s.%s (use create or modify to overwrite)", moduleName, stmt.Name.Name))
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

	// Build the model from AST
	svc := &model.ConsumedRestService{
		ContainerID:   containerID,
		Name:          stmt.Name.Name,
		Documentation: stmt.Documentation,
		BaseUrl:       stmt.BaseUrl,
	}
	// Preserve the existing ID on OR MODIFY so SEND REST REQUEST references stay valid.
	if preservedID != "" {
		svc.ID = preservedID
	}

	// Authentication — Mendix requires Rest$ConstantValue for BASIC auth credentials
	// (Rest$StringValue causes InvalidCastException in Studio Pro). When literal
	// strings are provided, auto-create constants to hold them.
	if stmt.Authentication != nil {
		auth := &model.RestAuthentication{
			Scheme: stmt.Authentication.Scheme,
		}
		// Username — must use Rest$ConstantValue pointing to a real constant.
		// $Variable refs pass through; literal strings auto-create constants.
		if strings.HasPrefix(stmt.Authentication.Username, "$") {
			// Already a $Constant ref — resolve to qualified name for BY_NAME lookup
			name := strings.TrimPrefix(stmt.Authentication.Username, "$")
			if !strings.Contains(name, ".") {
				name = moduleName + "." + name
			}
			auth.Username = "$" + name
		} else if stmt.Authentication.Username != "" {
			constName := stmt.Name.Name + "_Username"
			if err := ensureConstant(ctx, moduleName, containerID, constName, stmt.Authentication.Username); err != nil {
				return fmt.Errorf("failed to create username constant: %w", err)
			}
			auth.Username = "$" + moduleName + "." + constName
		}
		// Password
		if strings.HasPrefix(stmt.Authentication.Password, "$") {
			name := strings.TrimPrefix(stmt.Authentication.Password, "$")
			if !strings.Contains(name, ".") {
				name = moduleName + "." + name
			}
			auth.Password = "$" + name
		} else if stmt.Authentication.Password != "" {
			constName := stmt.Name.Name + "_Password"
			if err := ensureConstant(ctx, moduleName, containerID, constName, stmt.Authentication.Password); err != nil {
				return fmt.Errorf("failed to create password constant: %w", err)
			}
			auth.Password = "$" + moduleName + "." + constName
		}
		svc.Authentication = auth
	}

	// Operations
	for _, opDef := range stmt.Operations {
		op := buildRestClientOperation(opDef)
		svc.Operations = append(svc.Operations, op)
	}

	// Write to project
	if err := ctx.Backend.CreateConsumedRestService(svc); err != nil {
		return mdlerrors.NewBackend("create rest client", err)
	}

	verb := "Created"
	if wasModified {
		verb = "Modified"
	}
	fmt.Fprintf(ctx.Output, "%s rest client: %s.%s (%d operations)\n", verb, moduleName, stmt.Name.Name, len(svc.Operations))
	return nil
}

// buildRestClientOperation converts an AST RestOperationDef to a model RestClientOperation.
func buildRestClientOperation(opDef *ast.RestOperationDef) *model.RestClientOperation {
	op := &model.RestClientOperation{
		Name:             opDef.Name,
		Documentation:    opDef.Documentation,
		HttpMethod:       opDef.Method,
		Path:             opDef.Path,
		BodyType:         opDef.BodyType,
		BodyVariable:     opDef.BodyVariable,
		ResponseType:     opDef.ResponseType,
		ResponseVariable: opDef.ResponseVariable,
		Timeout:          opDef.Timeout,
	}

	// Convert body mapping (export direction: Left=jsonField, Right=entityAttr)
	if opDef.BodyMapping != nil {
		op.BodyType = "export_mapping"
		op.BodyVariable = opDef.BodyMapping.Entity.String()
		op.BodyMappings = convertMappingEntries(opDef.BodyMapping.Entries, false)
	}

	// Convert response mapping (import direction: Left=entityAttr, Right=jsonField)
	if opDef.ResponseMapping != nil {
		op.ResponseType = "mapping"
		op.ResponseEntity = opDef.ResponseMapping.Entity.String()
		op.ResponseMappings = convertMappingEntries(opDef.ResponseMapping.Entries, true)
	}

	// Path parameters
	for _, p := range opDef.Parameters {
		name := strings.TrimPrefix(p.Name, "$")
		op.Parameters = append(op.Parameters, &model.RestClientParameter{
			Name:     name,
			DataType: p.DataType,
		})
	}

	// Query parameters
	for _, q := range opDef.QueryParameters {
		name := strings.TrimPrefix(q.Name, "$")
		op.QueryParameters = append(op.QueryParameters, &model.RestClientParameter{
			Name:     name,
			DataType: q.DataType,
		})
	}

	// Headers
	for _, h := range opDef.Headers {
		header := &model.RestClientHeader{
			Name: h.Name,
		}
		if h.Variable != "" {
			// Dynamic headers: store static prefix only.
			// Mendix consumed REST services don't support dynamic header values
			// in the service definition; dynamic values must be set through the
			// calling microflow.
			header.Value = h.Prefix
		} else {
			header.Value = h.Value
		}
		op.Headers = append(op.Headers, header)
	}

	return op
}

// convertMappingEntries converts AST RestMappingEntry slices to model RestResponseMapping slices.
// importDirection=true: Left=entityAttr, Right=jsonField (import/response)
// importDirection=false: Left=jsonField, Right=entityAttr (export/body)
func convertMappingEntries(entries []ast.RestMappingEntry, importDirection bool) []*model.RestResponseMapping {
	var result []*model.RestResponseMapping
	for _, e := range entries {
		if e.Entity.Name != "" {
			// Object mapping
			result = append(result, &model.RestResponseMapping{
				Entity:      e.Entity.String(),
				Association: e.Association.String(),
				ExposedName: e.ExposedName,
				Children:    convertMappingEntries(e.Children, importDirection),
			})
		} else {
			// Value mapping — direction determines which side is attribute vs JSON field
			m := &model.RestResponseMapping{}
			if importDirection {
				m.Attribute = e.Left // EntityAttr = jsonField
				m.ExposedName = e.Right
			} else {
				m.Attribute = e.Right // jsonField = EntityAttr
				m.ExposedName = e.Left
			}
			result = append(result, m)
		}
	}
	return result
}

// ensureConstant creates a string constant if it doesn't already exist.
func ensureConstant(ctx *ExecContext, moduleName string, containerID model.ID, constName, value string) error {
	// Check if constant already exists
	constants, _ := ctx.Backend.ListConstants()
	h, _ := getHierarchy(ctx)
	for _, c := range constants {
		modID := h.FindModuleID(c.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == moduleName && c.Name == constName {
			return nil // already exists
		}
	}

	// Create the constant
	constant := &model.Constant{
		ContainerID:  containerID,
		Name:         constName,
		Type:         model.ConstantDataType{Kind: "String"},
		DefaultValue: value,
		ExportLevel:  "Hidden",
	}
	return ctx.Backend.CreateConstant(constant)
}

// dropRestClient handles DROP REST CLIENT statement.
func dropRestClient(ctx *ExecContext, stmt *ast.DropRestClientStmt) error {

	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	services, err := ctx.Backend.ListConsumedRestServices()
	if err != nil {
		return mdlerrors.NewBackend("list consumed rest services", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		moduleName := h.GetModuleName(modID)
		if strings.EqualFold(moduleName, stmt.Name.Module) && strings.EqualFold(svc.Name, stmt.Name.Name) {
			if err := ctx.Backend.DeleteConsumedRestService(svc.ID); err != nil {
				return mdlerrors.NewBackend("delete rest client", err)
			}
			fmt.Fprintf(ctx.Output, "Dropped rest client: %s.%s\n", moduleName, svc.Name)
			return nil
		}
	}

	return mdlerrors.NewNotFound("rest client", stmt.Name.String())
}

// formatRestAuthValue formats an authentication value for MDL output.
// Constant references (stored with $ prefix internally) are emitted as @Module.Constant.
func formatRestAuthValue(value string) string {
	if strings.HasPrefix(value, "$") {
		return "@" + strings.TrimPrefix(value, "$")
	}
	return "'" + value + "'"
}

// resolveAndFormatRestAuthValue resolves a constant reference to its literal DefaultValue
// for DESCRIBE output. Falls back to @Module.Constant notation when resolution fails.
func resolveAndFormatRestAuthValue(ctx *ExecContext, value string) string {
	if !strings.HasPrefix(value, "$") {
		return "'" + value + "'"
	}
	qualifiedName := strings.TrimPrefix(value, "$")
	if ctx != nil && ctx.Backend != nil {
		parts := strings.SplitN(qualifiedName, ".", 2)
		if len(parts) == 2 {
			moduleName, constName := parts[0], parts[1]
			if constants, err := ctx.Backend.ListConstants(); err == nil {
				for _, c := range constants {
					if !strings.EqualFold(c.Name, constName) {
						continue
					}
					if mod, err := ctx.Backend.GetModule(c.ContainerID); err == nil &&
						strings.EqualFold(mod.Name, moduleName) {
						return "'" + c.DefaultValue + "'"
					}
				}
			}
		}
	}
	return "@" + qualifiedName
}

// createRestClientFromSpec handles CREATE REST CLIENT (OpenAPI: '...') by parsing the spec
// and generating operations from it automatically.
func createRestClientFromSpec(ctx *ExecContext, stmt *ast.CreateRestClientStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	if err := checkFeature(ctx, "integration", "rest_client_basic",
		"create rest client",
		"upgrade your project to 10.1+"); err != nil {
		return err
	}

	mprDir := ""
	if ctx.MprPath != "" {
		mprDir = filepath.Dir(ctx.MprPath)
	}
	specBytes, rawURL, err := fetchSpecBytes(stmt.OpenApiPath, mprDir)
	if err != nil {
		return fmt.Errorf("failed to read OpenAPI spec: %w", err)
	}

	spec, err := openapi.ParseSpecFromURL(specBytes, rawURL)
	if err != nil {
		return fmt.Errorf("failed to parse OpenAPI spec: %w", err)
	}

	parsed, warnings, err := openapi.ToRestClientModel(spec, stmt.Name.Name, stmt.BaseUrl)
	if err != nil {
		return fmt.Errorf("failed to convert OpenAPI spec: %w", err)
	}
	for _, w := range warnings {
		fmt.Fprintf(ctx.Output, "Warning: %s\n", w)
	}

	// Resolve container (module / folder)
	moduleName := stmt.Name.Module
	module, err := findModule(ctx, moduleName)
	if err != nil {
		return mdlerrors.NewNotFound("module", moduleName)
	}
	containerID := module.ID
	if stmt.Folder != "" {
		folderID, err := resolveFolder(ctx, module.ID, stmt.Folder)
		if err != nil {
			return mdlerrors.NewBackend(fmt.Sprintf("resolve folder '%s'", stmt.Folder), err)
		}
		containerID = folderID
	}

	// Convert openapi intermediate types to model types
	svc := convertOpenAPIToModel(parsed, containerID)

	// Store raw spec content so Studio Pro can show "View OpenAPI"
	svc.OpenApiContent = string(specBytes)

	// MDL statement properties override spec-derived values
	if stmt.Documentation != "" {
		svc.Documentation = stmt.Documentation
	}
	if stmt.Authentication != nil {
		svc.Authentication = &model.RestAuthentication{Scheme: stmt.Authentication.Scheme}
	}

	// Handle OR MODIFY: delete existing if present, preserving UnitID so any
	// SEND REST REQUEST microflows that reference this service by ID remain valid.
	existingServices, err := ctx.Backend.ListConsumedRestServices()
	if err != nil {
		return mdlerrors.NewBackend("list rest clients", err)
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}
	openAPIWasModified := false
	for _, existing := range existingServices {
		existModID := h.FindModuleID(existing.ContainerID)
		existModName := h.GetModuleName(existModID)
		if strings.EqualFold(existModName, moduleName) && strings.EqualFold(existing.Name, stmt.Name.Name) {
			if stmt.CreateOrModify {
				// Reuse the existing ID so microflow references stay valid.
				svc.ID = existing.ID
				openAPIWasModified = true
				if err := ctx.Backend.DeleteConsumedRestService(existing.ID); err != nil {
					return mdlerrors.NewBackend("delete existing rest client", err)
				}
			} else {
				return mdlerrors.NewAlreadyExistsMsg("rest client", moduleName+"."+stmt.Name.Name,
					fmt.Sprintf("rest client already exists: %s.%s (use create or modify to overwrite)", moduleName, stmt.Name.Name))
			}
		}
	}

	if err := ctx.Backend.CreateConsumedRestService(svc); err != nil {
		return mdlerrors.NewBackend("create rest client", err)
	}

	openAPIVerb := "Created"
	if openAPIWasModified {
		openAPIVerb = "Modified"
	}
	fmt.Fprintf(ctx.Output, "%s rest client: %s.%s (%d operations from OpenAPI spec)\n",
		openAPIVerb, moduleName, stmt.Name.Name, len(svc.Operations))
	return nil
}

// describeContractFromOpenAPI handles DESCRIBE CONTRACT OPERATION FROM OPENAPI '/path/to/spec.json'.
// No project connection required — outputs the MDL that would be generated from the spec.
func describeContractFromOpenAPI(ctx *ExecContext, stmt *ast.DescribeContractFromOpenAPIStmt) error {
	mprDir := ""
	if ctx.MprPath != "" {
		mprDir = filepath.Dir(ctx.MprPath)
	}
	specBytes, rawURL, err := fetchSpecBytes(stmt.SpecPath, mprDir)
	if err != nil {
		return fmt.Errorf("failed to read OpenAPI spec: %w", err)
	}

	spec, err := openapi.ParseSpecFromURL(specBytes, rawURL)
	if err != nil {
		return fmt.Errorf("failed to parse OpenAPI spec: %w", err)
	}

	serviceName := sanitizeModuleName(spec.Info.Title)
	parsed, warnings, err := openapi.ToRestClientModel(spec, serviceName, "")
	if err != nil {
		return fmt.Errorf("failed to convert OpenAPI spec: %w", err)
	}
	for _, w := range warnings {
		fmt.Fprintf(ctx.Output, "-- Warning: %s\n", w)
	}

	svc := convertOpenAPIToModel(parsed, "")
	return outputConsumedRestServiceMDL(ctx, svc, "MyModule")
}

// convertOpenAPIToModel converts an openapi.ConsumedRestService to a model.ConsumedRestService.
func convertOpenAPIToModel(parsed *openapi.ConsumedRestService, containerID model.ID) *model.ConsumedRestService {
	svc := &model.ConsumedRestService{
		ContainerID:   containerID,
		Name:          parsed.Name,
		Documentation: parsed.Documentation,
		BaseUrl:       parsed.BaseUrl,
	}
	if parsed.Authentication != nil {
		svc.Authentication = &model.RestAuthentication{Scheme: parsed.Authentication.Scheme}
	}
	for _, op := range parsed.Operations {
		modelOp := &model.RestClientOperation{
			Name:          op.Name,
			Documentation: op.Documentation,
			HttpMethod:    op.HttpMethod,
			Path:          op.Path,
			Tags:          op.Tags,
			BodyType:      op.BodyType,
			BodyVariable:  op.BodyVariable,
			ResponseType:  op.ResponseType,
		}
		for _, p := range op.Parameters {
			modelOp.Parameters = append(modelOp.Parameters, &model.RestClientParameter{
				Name:     p.Name,
				DataType: p.DataType,
			})
		}
		for _, q := range op.QueryParameters {
			modelOp.QueryParameters = append(modelOp.QueryParameters, &model.RestClientParameter{
				Name:     q.Name,
				DataType: q.DataType,
			})
		}
		for _, h := range op.Headers {
			modelOp.Headers = append(modelOp.Headers, &model.RestClientHeader{
				Name:  h.Name,
				Value: h.Value,
			})
		}
		svc.Operations = append(svc.Operations, modelOp)
	}
	return svc
}

// sanitizeModuleName converts a spec title to a PascalCase MDL identifier suitable as a service/module name.
// This is intentionally different from openapi.sanitizeIdent (which produces snake_case for operation names):
// service names follow Mendix PascalCase convention while operation names follow snake_case.
func sanitizeModuleName(title string) string {
	// Replace spaces and non-alphanumeric with nothing (PascalCase-ish)
	var b strings.Builder
	upper := true
	for _, r := range title {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			if upper {
				if r >= 'a' && r <= 'z' {
					r -= 32
				}
				upper = false
			}
			b.WriteRune(r)
		} else {
			upper = true
		}
	}
	name := b.String()
	if name == "" {
		name = "Service"
	}
	return name
}

// fetchSpecBytes retrieves raw spec bytes from a local path or HTTP(S) URL.
// baseDir is used to resolve relative paths (pass filepath.Dir(ctx.MprPath) when available).
// Returns the bytes, the resolved URL (for format detection), and any error.
func fetchSpecBytes(specPath, baseDir string) ([]byte, string, error) {
	normalised, err := pathutil.NormalizeURL(specPath, baseDir)
	if err != nil {
		return nil, "", fmt.Errorf("invalid spec source %q: %w", specPath, err)
	}

	filePath := pathutil.PathFromURL(normalised)
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, normalised, fmt.Errorf("read spec file %s: %w", filePath, err)
		}
		return data, normalised, nil
	}

	// HTTP(S) fetch
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(normalised)
	if err != nil {
		return nil, normalised, fmt.Errorf("fetch spec from %s: %w", normalised, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, normalised, fmt.Errorf("spec fetch returned HTTP %d from %s", resp.StatusCode, normalised)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10 MB cap
	if err != nil {
		return nil, normalised, fmt.Errorf("read spec response: %w", err)
	}
	return data, normalised, nil
}

// Executor wrappers for unmigrated callers.
