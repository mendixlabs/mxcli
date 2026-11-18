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

	"github.com/mendixlabs/mxcli/internal/pathutil"
	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/openapi"
	"github.com/mendixlabs/mxcli/model"
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

	// The folder is a clause after the name (R9).
	folder := ""
	if h, err := getHierarchy(ctx); err == nil && h != nil {
		if folderPath := h.BuildFolderPath(svc.ContainerID); folderPath != "" {
			folder = " folder " + mdlQuote(ctx, folderPath)
		}
	}
	fmt.Fprintf(w, "create or modify consumed rest service %s.%s%s (\n", moduleName, svc.Name, folder)
	fmt.Fprintf(w, "  BaseUrl: %s,\n", mdlQuoted(svc.BaseUrl))
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

// outputRestOperation writes a single operation, its properties in ( ) (R2).
func outputRestOperation(w io.Writer, op *model.RestClientOperation) {
	if op.Documentation != "" {
		outputJavadocIndented(w, op.Documentation, "  ")
	}

	fmt.Fprintf(w, "  operation %s (\n", op.Name)
	fmt.Fprintf(w, "    Method: %s,\n", strings.ToLower(op.HttpMethod))
	fmt.Fprintf(w, "    Path: %s,\n", mdlQuoted(op.Path))

	// Parameters: ($var: Type, ...)
	if len(op.Parameters) > 0 {
		var params []string
		for _, p := range op.Parameters {
			params = append(params, fmt.Sprintf("$%s: %s", p.Name, restParamTypeOrDefault(p.DataType)))
		}
		fmt.Fprintf(w, "    Parameters: (%s),\n", strings.Join(params, ", "))
	}

	// Query: ($var: Type, ...)
	if len(op.QueryParameters) > 0 {
		var params []string
		for _, q := range op.QueryParameters {
			params = append(params, fmt.Sprintf("$%s: %s", q.Name, restParamTypeOrDefault(q.DataType)))
		}
		fmt.Fprintf(w, "    Query: (%s),\n", strings.Join(params, ", "))
	}

	// Headers: ('Name': 'Value', ...) — a map, `key: value` (R2/R3)
	if len(op.Headers) > 0 {
		var hdrs []string
		for _, h := range op.Headers {
			hdrs = append(hdrs, mdlQuoted(h.Name)+": "+mdlQuoted(h.Value))
		}
		fmt.Fprintf(w, "    Headers: (%s),\n", strings.Join(hdrs, ", "))
	}

	// Body
	if op.BodyType != "" {
		switch strings.ToLower(op.BodyType) {
		case "template":
			fmt.Fprintf(w, "    Body: template %s,\n", mdlQuoted(op.BodyVariable))
		case "export_mapping":
			if op.BodyVariable != "" && len(op.BodyMappings) > 0 {
				fmt.Fprintf(w, "    Body: mapping %s {\n", op.BodyVariable)
				writeExportMappings(w, op.BodyMappings, 6, "(Object)")
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

	// Response. An operation stores no response variable (the call activity
	// names it), so the reader never fills one in and the bare kind is what
	// describe prints (mendixlabs/mxcli#1608).
	switch kind := strings.ToLower(op.ResponseType); kind {
	case "none":
		fmt.Fprintln(w, "    Response: none")
	case "json", "string", "file", "status":
		if op.ResponseVariable != "" {
			fmt.Fprintf(w, "    Response: %s as %s\n", kind, op.ResponseVariable)
		} else {
			fmt.Fprintf(w, "    Response: %s\n", kind)
		}
	case "mapping":
		if op.ResponseEntity != "" && len(op.ResponseMappings) > 0 {
			fmt.Fprintf(w, "    Response: mapping %s {\n", op.ResponseEntity)
			writeResponseMappings(w, op.ResponseMappings, 6, "(Object)")
			fmt.Fprintln(w, "    }")
		} else if op.ResponseEntity != "" {
			fmt.Fprintf(w, "    Response: mapping %s\n", op.ResponseEntity)
		} else {
			fmt.Fprintln(w, "    Response: none")
		}
	default:
		fmt.Fprintln(w, "    Response: none")
	}

	fmt.Fprintln(w, "  )")
}

// restParamTypeOrDefault supplies the type describe prints for a REST parameter.
//
// Rest$QueryParameter has no DataType property — Mendix does not model a type
// for query parameters, so the one written in MDL is dropped at write time and
// there is nothing to read back. The MDL grammar still requires a type
// (`$name: Type`), so emitting the empty string produces `$page: `, which does
// not re-parse. String is the honest stand-in: query parameters travel as text
// in the URL, and it is the type this same describe output round-trips to.
func restParamTypeOrDefault(dataType string) string {
	if dataType == "" {
		return "String"
	}
	return dataType
}

// inlineMemberName is the JSON member DESCRIBE should print for an inline REST
// mapping element: its stored JsonPath rendered relative to the enclosing
// element, not its ExposedName.
//
// The two differ for any MULTI-SEGMENT member. `Title = fields/Title` stores
// JsonPath "(Object)|fields|Title" and ExposedName "Title" (the last segment,
// as Studio Pro does), so printing ExposedName emitted `"Title" = "Title"` —
// output that parses, re-executes, and silently binds the wrong member. Before
// the path fix the two happened to agree, because the whole "fields/Title"
// string was crammed into ExposedName; correcting the path is what made the
// printer wrong, so the two changes belong together.
//
// The generated markers — "(Object)", "(Array)", "(Wrapper)" — are dropped:
// they are levels MDL generates rather than names an author writes, the same
// rule the array forms follow (ako/mxcli#248, #262).
func inlineMemberName(parentPath string, m *model.RestResponseMapping) string {
	if m.JsonPath == "" {
		return m.ExposedName
	}
	rel := m.JsonPath
	if parentPath != "" && strings.HasPrefix(rel, parentPath+"|") {
		rel = rel[len(parentPath)+1:]
	} else if i := strings.LastIndex(rel, "|"); i >= 0 {
		rel = rel[i+1:]
	}
	var segs []string
	for _, seg := range strings.Split(rel, "|") {
		switch seg {
		case "", "(Object)", "(Array)", "(Wrapper)":
			continue
		}
		segs = append(segs, seg)
	}
	if len(segs) == 0 {
		return m.ExposedName
	}
	return strings.Join(segs, "/")
}

// writeResponseMappings writes import-direction mappings (JSON → Entity): EntityAttr = jsonField.
// Matches the import mapping syntax: CREATE Association/Entity = jsonField { ... }.
func writeResponseMappings(w io.Writer, mappings []*model.RestResponseMapping, indent int, parentPath string) {
	pad := strings.Repeat(" ", indent)
	for _, m := range mappings {
		if m.Entity != "" {
			// Object mapping → nested entity via association (import style: CREATE Assoc/Entity = jsonField)
			fmt.Fprintf(w, "%screate %s/%s = %s", pad, m.Association, m.Entity, inlineMemberName(parentPath, m))
			if len(m.Children) > 0 {
				fmt.Fprintln(w, " {")
				writeResponseMappings(w, m.Children, indent+2, m.JsonPath)
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
			fmt.Fprintf(w, "%s%s = %s,%s\n", pad, safeIdent(m.Attribute), safeIdent(inlineMemberName(parentPath, m)), jsonComment)
		}
	}
}

// writeExportMappings writes export-direction mappings (Entity → JSON): jsonField = EntityAttr.
// Matches the export mapping syntax.
func writeExportMappings(w io.Writer, mappings []*model.RestResponseMapping, indent int, parentPath string) {
	pad := strings.Repeat(" ", indent)
	for _, m := range mappings {
		if m.Entity != "" {
			fmt.Fprintf(w, "%s%s/%s = %s", pad, m.Association, m.Entity, inlineMemberName(parentPath, m))
			if len(m.Children) > 0 {
				fmt.Fprintln(w, " {")
				writeExportMappings(w, m.Children, indent+2, m.JsonPath)
				fmt.Fprintf(w, "%s},\n", pad)
			} else {
				fmt.Fprintln(w, " {},")
			}
		} else {
			// Export direction: jsonField = EntityAttr
			fmt.Fprintf(w, "%s%s = %s,\n", pad, safeIdent(inlineMemberName(parentPath, m)), safeIdent(m.Attribute))
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
	if err := checkVersionGates(ctx, stmt); err != nil {
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
	// Where the service currently sits. Its container is re-applied on every
	// statement, and an unset value files a foldered service back into the
	// module root (#932).
	var preservedContainerID model.ID
	var preservedDocumentation string
	var storedOperations []*model.RestClientOperation
	wasModified := false
	for _, existing := range existingServices {
		existModID := h.FindModuleID(existing.ContainerID)
		existModName := h.GetModuleName(existModID)
		if strings.EqualFold(existModName, moduleName) && strings.EqualFold(existing.Name, stmt.Name.Name) {
			if stmt.CreateOrModify {
				// Preserve the existing ID so SEND REST REQUEST references stay valid after replace.
				preservedID = existing.ID
				preservedContainerID = existing.ContainerID
				// A rewrite that carried no doc comment keeps the stored one, so
				// it has to be read before anything replaces the unit (#1018).
				preservedDocumentation = existing.Documentation
				storedOperations = existing.Operations
				wasModified = true
			} else {
				return mdlerrors.NewAlreadyExistsMsg("rest client", moduleName+"."+stmt.Name.Name, fmt.Sprintf("rest client already exists: %s.%s (use create or modify to overwrite)", moduleName, stmt.Name.Name))
			}
		}
	}

	// Resolve folder if specified, else keep where the service already was.
	containerID := module.ID
	if stmt.Folder != "" {
		folderID, err := resolveFolder(ctx, module.ID, stmt.Folder)
		if err != nil {
			return mdlerrors.NewBackend(fmt.Sprintf("resolve folder '%s'", stmt.Folder), err)
		}
		containerID = folderID
	} else if preservedContainerID != "" {
		containerID = preservedContainerID
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
		// A rewrite that carried no doc comment keeps the stored one (#1018).
		svc.Documentation = carriedDocumentation(
			stmt.DocumentationSet, stmt.Documentation, preservedDocumentation)
	}

	if stmt.Authentication != nil {
		auth, err := restAuthFromDef(ctx, stmt.Name, containerID, stmt.Authentication)
		if err != nil {
			return err
		}
		svc.Authentication = auth
	}

	// Operations
	for _, opDef := range stmt.Operations {
		op, err := buildRestClientOperation(opDef)
		if err != nil {
			return fmt.Errorf("operation %q: %w", opDef.Name, err)
		}
		svc.Operations = append(svc.Operations, op)
	}

	// Write to project
	if err := saveConsumedRestService(ctx, svc, preservedID, preservedContainerID); err != nil {
		return mdlerrors.NewBackend("create rest client", err)
	}

	verb := "Created"
	if wasModified {
		verb = "Modified"
	}
	ctx.ReportMutation(verb, "rest client: %s.%s (%d operations)",
		moduleName, stmt.Name.Name, len(svc.Operations))
	if wasModified {
		warnDroppedRestOperations(ctx, stmt.Name, storedOperations, svc.Operations)
	}
	return nil
}

// warnDroppedRestOperations names the stored operations a CREATE OR MODIFY
// removed. The statement declares the service's whole operation list, so one
// that names only the operations to add deletes the rest — often ones made in
// Studio Pro, with headers and mappings — and used to report only
// "Modified rest client: … (2 operations)" (mendixlabs/mxcli#1608). It still
// applies, as `create or modify entity` does for attributes; it says so, and
// names the statement that adds an operation without touching the others.
func warnDroppedRestOperations(ctx *ExecContext, name ast.QualifiedName, stored, written []*model.RestClientOperation) {
	kept := map[string]bool{}
	for _, op := range written {
		kept[strings.ToLower(op.Name)] = true
	}
	var dropped []string
	for _, op := range stored {
		if !kept[strings.ToLower(op.Name)] {
			dropped = append(dropped, op.Name)
		}
	}
	if len(dropped) == 0 {
		return
	}
	fmt.Fprintf(ctx.Output,
		"⚠ create or modify consumed rest service %s drops %d existing operation(s) not listed in this statement: %s\n",
		name, len(dropped), strings.Join(dropped, ", "))
	fmt.Fprintf(ctx.Output,
		"  To add an operation without touching the others, use: alter consumed rest service %s add operation <Name> ( … );\n", name)
}

// restAuthFromDef resolves a statement's basic authentication for service
// name. Mendix requires Rest$ConstantValue for BASIC auth credentials
// (Rest$StringValue causes InvalidCastException in Studio Pro), so a literal
// string gets a constant to hold it, created in containerID; a `$Constant`
// reference is qualified with the service's module.
func restAuthFromDef(ctx *ExecContext, name ast.QualifiedName, containerID model.ID, def *ast.RestAuthDef) (*model.RestAuthentication, error) {
	moduleName := name.Module
	auth := &model.RestAuthentication{
		Scheme: def.Scheme,
	}
	// Username — must use Rest$ConstantValue pointing to a real constant.
	// $Variable refs pass through; literal strings auto-create constants.
	if strings.HasPrefix(def.Username, "$") {
		// Already a $Constant ref — resolve to qualified name for BY_NAME lookup
		n := strings.TrimPrefix(def.Username, "$")
		if !strings.Contains(n, ".") {
			n = moduleName + "." + n
		}
		auth.Username = "$" + n
	} else if def.Username != "" {
		constName := name.Name + "_Username"
		if err := ensureConstant(ctx, moduleName, containerID, constName, def.Username); err != nil {
			return nil, fmt.Errorf("failed to create username constant: %w", err)
		}
		auth.Username = "$" + moduleName + "." + constName
	}
	// Password
	if strings.HasPrefix(def.Password, "$") {
		n := strings.TrimPrefix(def.Password, "$")
		if !strings.Contains(n, ".") {
			n = moduleName + "." + n
		}
		auth.Password = "$" + n
	} else if def.Password != "" {
		constName := name.Name + "_Password"
		if err := ensureConstant(ctx, moduleName, containerID, constName, def.Password); err != nil {
			return nil, fmt.Errorf("failed to create password constant: %w", err)
		}
		auth.Password = "$" + moduleName + "." + constName
	}
	return auth, nil
}

// saveConsumedRestService writes a consumed REST client, rewriting the stored
// unit IN PLACE when the statement is a modification that leaves it where it is.
//
// The obvious implementation — delete the old unit, insert a new one under the
// preserved ID — is what made `create or modify rest client` churn. An insert
// does not pass through the storage layer's reconciliation, so an identical
// re-run rewrote nine element $IDs in the document, dirtied the .mpr, and did it
// again on every run (ako/mxcli#556: 143 bytes of a 1,128-byte unit, all of them
// $IDs). An update does pass through it, and is elided outright when the
// rebuild means the same thing as what is stored.
//
// Delete+create survives for the one thing an in-place rewrite cannot express:
// moving the document to another folder, which lives in the unit's ROW rather
// than in its contents.
//
// Deferring the delete to here is also strictly safer than doing it while
// scanning: building the service from the AST can fail, and the old code had
// already deleted the stored one by then.
func saveConsumedRestService(ctx *ExecContext, svc *model.ConsumedRestService, preservedID, preservedContainerID model.ID) error {
	if preservedID == "" {
		return ctx.Backend.CreateConsumedRestService(svc)
	}
	if svc.ContainerID == preservedContainerID {
		return ctx.Backend.UpdateConsumedRestService(svc)
	}
	if err := ctx.Backend.DeleteConsumedRestService(preservedID); err != nil {
		return fmt.Errorf("delete existing rest client: %w", err)
	}
	return ctx.Backend.CreateConsumedRestService(svc)
}

// buildRestClientOperation converts an AST RestOperationDef to a model RestClientOperation.
func buildRestClientOperation(opDef *ast.RestOperationDef) (*model.RestClientOperation, error) {
	if err := checkInlineMappingBody(opDef); err != nil {
		return nil, err
	}
	if err := checkFileRequestBody(opDef); err != nil {
		return nil, err
	}
	// model.RestClientOperation documents BodyType/ResponseType as upper-case
	// tokens ("JSON", "EXPORT_MAPPING", "MAPPING", ...) and every consumer
	// compares against that spelling — the serializers in both engines, and the
	// REST-call microflow builder. The visitor produces the lower-case source
	// text, so passing it through unchanged silently disabled all of them: a
	// `Response: MAPPING` operation matched no branch and fell back to
	// Rest$NoResponseHandling, dropping the mapping without a warning (#843).
	// Normalize here, at the one place the AST becomes the semantic model.
	op := &model.RestClientOperation{
		Name:             opDef.Name,
		Documentation:    opDef.Documentation,
		HttpMethod:       opDef.Method,
		Path:             opDef.Path,
		BodyType:         strings.ToUpper(opDef.BodyType),
		BodyVariable:     opDef.BodyVariable,
		ResponseType:     strings.ToUpper(opDef.ResponseType),
		ResponseVariable: opDef.ResponseVariable,
		Timeout:          opDef.Timeout,
	}

	// Convert body mapping (export direction: Left=jsonField, Right=entityAttr)
	if opDef.BodyMapping != nil {
		op.BodyType = "EXPORT_MAPPING"
		op.BodyVariable = opDef.BodyMapping.Entity.String()
		op.BodyMappings = convertMappingEntries(opDef.BodyMapping.Entries, false)
	}

	// Convert response mapping (import direction: Left=entityAttr, Right=jsonField)
	if opDef.ResponseMapping != nil {
		op.ResponseType = "MAPPING"
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
		// The value is a template: `{P}` is the operation parameter P, as in
		// the path. `'Bearer ' + $Token` used to store "Bearer " alone, which
		// dropped the token and described back as a literal (ako/mxcli#707);
		// the visitor now builds `Bearer {Token}` from it.
		op.Headers = append(op.Headers, &model.RestClientHeader{Name: h.Name, Value: h.Value})
	}

	return op, nil
}

// checkInlineMappingBody rejects `Body:`/`Response: MAPPING X` written without a
// `{ ... }` body.
//
// The clause names an *entity* and the braces list the JSON fields to map onto
// it; Mendix stores the result inline on the operation as
// Rest$ImplicitMappingResponseHandling / Rest$ImplicitMappingBody. It is not a
// reference to a mapping document — a consumed REST operation has nowhere to put
// one, as Rest$RestOperationResponseHandling has exactly two implementations
// (implicit-mapping and none).
//
// So `Response: MAPPING Mod.IMM_Something` parses, names a mapping document
// where an entity belongs, contributes no field mappings, and used to be written
// out as "no response handling" — accepted in silence, and only noticed at
// runtime when nothing was parsed out of the response body (#843). Refuse it
// instead, and say what to write.
func checkInlineMappingBody(opDef *ast.RestOperationDef) error {
	for _, m := range []struct {
		clause string
		def    *ast.RestMappingDef
		syntax string
	}{
		{"Response", opDef.ResponseMapping, "Response: mapping Module.Entity { Attribute = jsonField, ... }"},
		{"Body", opDef.BodyMapping, "Body: mapping Module.Entity { jsonField = Attribute, ... }"},
	} {
		if m.def == nil || len(m.def.Entries) > 0 {
			continue
		}
		return fmt.Errorf(
			"%s: mapping %s has no mapping body.\n"+
				"  A consumed REST operation cannot reference an import/export mapping document;\n"+
				"  Mendix stores the mapping inline, so the fields must be listed here.\n"+
				"  Name the target entity and its fields:\n"+
				"    %s",
			m.clause, m.def.Entity.String(), m.syntax)
	}
	return nil
}

// checkFileRequestBody rejects `Body: file from $Doc`.
//
// Mendix has no binary request body. The 11.13 metamodel offers exactly three
// implementations of a consumed operation's request body — Rest$JsonBody,
// Rest$StringBody and Rest$ImplicitMappingBody — so there is nowhere for a file
// document to go. Both serializers folded FILE into the TEMPLATE branch and
// wrote a Rest$StringBody holding the *expression text*, which meant the
// request sent the four bytes `$Doc` in place of the document:
//
//	Content-Length: 4
//	data:application/octet-stream;base64,JERvYw==   ->  b'$Doc'
//
// measured against httpbingo with an 8090-byte PNG. `mxcli check` passed,
// `mx check` reported 0 errors and the call returned 200 — nothing anywhere
// said the payload was wrong.
//
// Refusing is the fix, not writing a different type: there is no correct type to
// write, and a silent downgrade that returns 200 is worse than an error. Binary
// upload needs a Java action.
//
// The RESPONSE side is deliberately untouched: `Response: file as $Doc`
// downloads correctly.
func checkFileRequestBody(opDef *ast.RestOperationDef) error {
	if !strings.EqualFold(opDef.BodyType, "FILE") {
		return nil
	}
	target := opDef.BodyVariable
	if target == "" {
		target = "$Doc"
	}
	return fmt.Errorf(
		"Body: file from %s cannot be stored — Mendix has no binary request body.\n"+
			"  A consumed REST operation's body is one of Rest$JsonBody, Rest$StringBody or\n"+
			"  Rest$ImplicitMappingBody, so a file document has nowhere to go. mxcli used to\n"+
			"  write a string body holding the literal text %q, which sends %d bytes and\n"+
			"  still returns 200.\n"+
			"  Binary POST lives on the microflow activity, not the client document:\n"+
			"    call rest service post '<url>' (Headers: ('ContentType': '<type>'), Body: binary %s/Contents)\n"+
			"  (Microflows$BinaryRequestHandling — the shape Studio Pro writes).",
		target, target, len(target), target)
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

	if err := checkVersionGates(ctx, stmt); err != nil {
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
	var openAPIPreservedID, openAPIPreservedContainerID model.ID
	for _, existing := range existingServices {
		existModID := h.FindModuleID(existing.ContainerID)
		existModName := h.GetModuleName(existModID)
		if strings.EqualFold(existModName, moduleName) && strings.EqualFold(existing.Name, stmt.Name.Name) {
			if stmt.CreateOrModify {
				// Reuse the existing ID so microflow references stay valid, and
				// its container so a spec re-import does not file the service
				// back into the module root (#932).
				svc.ID = existing.ID
				if stmt.Folder == "" {
					svc.ContainerID = existing.ContainerID
				}
				openAPIWasModified = true
				openAPIPreservedID = existing.ID
				openAPIPreservedContainerID = existing.ContainerID
			} else {
				return mdlerrors.NewAlreadyExistsMsg("rest client", moduleName+"."+stmt.Name.Name,
					fmt.Sprintf("rest client already exists: %s.%s (use create or modify to overwrite)", moduleName, stmt.Name.Name))
			}
		}
	}

	if err := saveConsumedRestService(ctx, svc, openAPIPreservedID, openAPIPreservedContainerID); err != nil {
		return mdlerrors.NewBackend("create rest client", err)
	}

	openAPIVerb := "Created"
	if openAPIWasModified {
		openAPIVerb = "Modified"
	}
	ctx.ReportMutation(openAPIVerb, "rest client: %s.%s (%d operations from OpenAPI spec)",
		moduleName, stmt.Name.Name, len(svc.Operations))
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
			// No DataType: Rest$QueryParameter has none, so the import stores
			// every query parameter as text, and the preview built from this
			// model has to say what the import writes, not what the spec
			// declares (ako/mxcli#2353).
			modelOp.QueryParameters = append(modelOp.QueryParameters, &model.RestClientParameter{
				Name: q.Name,
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
