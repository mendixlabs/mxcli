// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// listPublishedRestServices handles SHOW PUBLISHED REST SERVICES [IN module] command.
func listPublishedRestServices(ctx *ExecContext, moduleName string) error {

	services, err := ctx.Backend.ListPublishedRestServices()
	if err != nil {
		return mdlerrors.NewBackend("list published rest services", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	type row struct {
		module        string
		qualifiedName string
		path          string
		version       string
		resources     int
		operations    int
	}
	var rows []row

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName != "" && !strings.EqualFold(modName, moduleName) {
			continue
		}

		qn := modName + "." + svc.Name
		opCount := 0
		for _, res := range svc.Resources {
			opCount += len(res.Operations)
		}

		path := svc.Path
		if len(path) > 50 {
			path = path[:47] + "..."
		}

		rows = append(rows, row{modName, qn, path, svc.Version, len(svc.Resources), opCount})
	}

	if len(rows) == 0 && ctx.Format != FormatJSON {
		fmt.Fprintln(ctx.Output, "No published rest services found.")
		return nil
	}

	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].qualifiedName) < strings.ToLower(rows[j].qualifiedName)
	})

	result := &TableResult{
		Columns: []string{"Module", "QualifiedName", "Path", "Version", "Resources", "Operations"},
		Summary: fmt.Sprintf("(%d published rest services)", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.module, r.qualifiedName, r.path, r.version, r.resources, r.operations})
	}
	return writeResult(ctx, result)
}

// describePublishedRestService handles DESCRIBE PUBLISHED REST SERVICE command.
func describePublishedRestService(ctx *ExecContext, name ast.QualifiedName) error {

	services, err := ctx.Backend.ListPublishedRestServices()
	if err != nil {
		return mdlerrors.NewBackend("list published rest services", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)
		qualifiedName := modName + "." + svc.Name

		if !strings.EqualFold(modName, name.Module) || !strings.EqualFold(svc.Name, name.Name) {
			continue
		}

		// Output as re-executable MDL
		// The folder is a clause after the name (R9); `Folder:` is its alias.
		folder := ""
		if folderPath := h.BuildFolderPath(svc.ContainerID); folderPath != "" {
			folder = " folder " + mdlQuote(ctx, folderPath)
		}
		writeDocumentDocComment(ctx.Output, svc.Documentation) // mendixlabs/mxcli#1510
		fmt.Fprintf(ctx.Output, "create or modify published rest service %s%s (\n", qualifiedName, folder)
		fmt.Fprintf(ctx.Output, "  Path: %s", mdlQuoted(svc.Path))
		if svc.Version != "" {
			fmt.Fprintf(ctx.Output, ",\n  Version: %s", mdlQuoted(svc.Version))
		}
		if svc.ServiceName != "" {
			fmt.Fprintf(ctx.Output, ",\n  ServiceName: %s", mdlQuoted(svc.ServiceName))
		}
		var authMf *microflows.Microflow
		if len(svc.Parameters) > 0 && svc.AuthenticationMicroflow != "" {
			authMf = liveMicroflowsByQualifiedName(ctx)[svc.AuthenticationMicroflow]
		}
		authProp, authNote, hasAuth := publishedRestAuthenticationProperty(svc, authMf)
		if hasAuth {
			fmt.Fprintf(ctx.Output, ",\n  %s", authProp)
		}
		fmt.Fprintln(ctx.Output, "\n)")
		if authNote != "" {
			fmt.Fprintf(ctx.Output, "-- Authentication is stored as %s, which MDL cannot state; executing this keeps it.\n", authNote)
		}
		if hasAuth {
			for _, note := range publishedRestAuthParameterNotes(svc) {
				fmt.Fprintf(ctx.Output, "-- %s\n", note)
			}
		}

		if len(svc.Resources) > 0 {
			fmt.Fprintln(ctx.Output, "{")
			for _, res := range svc.Resources {
				fmt.Fprintf(ctx.Output, "  resource %s {\n", mdlQuoted(res.Name))
				for _, op := range res.Operations {
					deprecated := ""
					if op.Deprecated {
						deprecated = " deprecated"
					}
					mf := ""
					if op.Microflow != "" {
						mf = fmt.Sprintf(" microflow %s", op.Microflow)
					}
					summary := ""
					if op.Summary != "" {
						summary = fmt.Sprintf(" -- %s", op.Summary)
					}
					opPath := ""
					if op.Path != "" {
						opPath = " " + mdlQuoted(op.Path)
					}
					fmt.Fprintf(ctx.Output, "    %s%s%s%s%s;%s\n",
						strings.ToLower(op.HTTPMethod), opPath, mf, deprecated, publishedRestBindingClauses(op), summary)
					for _, note := range publishedRestParameterNotes(op) {
						fmt.Fprintf(ctx.Output, "    -- %s\n", note)
					}
				}
				fmt.Fprintln(ctx.Output, "  }")
			}
			fmt.Fprintln(ctx.Output, "};")
		} else {
			// The resource block is not optional in the grammar: a service
			// with no resources still needs an empty one to re-parse (#744).
			fmt.Fprintln(ctx.Output, "{\n};")
		}

		// Emit GRANT statements for any module roles with access.
		if len(svc.AllowedRoles) > 0 {
			fmt.Fprintf(ctx.Output, "\ngrant access on published rest service %s.%s to %s;\n",
				modName, svc.Name, strings.Join(svc.AllowedRoles, ", "))
		}

		return nil
	}

	return mdlerrors.NewNotFound("published rest service", name.String())
}

// applyPublishedRestAuthentication sets a service's authentication from the
// statement; nil (not stated) leaves it as it is. Methods and microflow are
// replaced together, so a list without `microflow` clears the microflow, as
// Studio Pro does when Custom is unticked.
//
// The microflow is checked against what Mendix accepts before anything is
// written (MDL-REST04).
//
// The service's authentication parameters (mendixlabs/mxcli#1417) follow the
// microflow: stated, they are the list written with every unlisted parameter
// defaulted to a query parameter; unstated, the stored ones are kept for the
// microflow parameters that still exist (mergeOperationParameters). Either way
// each keeps the stored $ID of the element bound to the same microflow
// parameter. svc.Parameters holds the stored parameters on entry.
func applyPublishedRestAuthentication(ctx *ExecContext, svc *model.PublishedRestService, auth *ast.PublishedRestAuthentication) error {
	if auth == nil {
		if svc.AuthenticationMicroflow != "" {
			if mf, ok := liveMicroflowsByQualifiedName(ctx)[svc.AuthenticationMicroflow]; ok {
				svc.Parameters = mergeOperationParameters(svc.Parameters, publishedRestAuthParametersOf(svc.AuthenticationMicroflow, mf))
			}
		}
		return nil
	}
	if problem := publishedRestAuthListProblem(auth.Parameters); problem != "" {
		return mdlerrors.NewValidationf("MDL-REST04: authentication microflow %s %s", auth.Microflow, problem)
	}
	var params []*model.PublishedRestOperationParameter
	if auth.Microflow != "" {
		mf, ok := liveMicroflowsByQualifiedName(ctx)[auth.Microflow]
		if !ok {
			return mdlerrors.NewValidationf("MDL-REST04: authentication microflow not found: %s", auth.Microflow)
		}
		if problem := publishedRestAuthMicroflowProblem(mf); problem != "" {
			return mdlerrors.NewValidationf("MDL-REST04: authentication microflow %s %s", auth.Microflow, problem)
		}
		var problem string
		params, problem = resolvePublishedRestAuthParameters(auth.Microflow, mf, auth.Parameters)
		if problem != "" {
			return mdlerrors.NewValidationf("MDL-REST04: authentication microflow %s %s", auth.Microflow, problem)
		}
		carryParameterIDs(params, svc.Parameters)
	}
	svc.AuthenticationTypes = append([]string(nil), auth.Methods...)
	svc.AuthenticationMicroflow = auth.Microflow
	svc.Parameters = params
	return nil
}

// publishedRestAuthParametersOf is what a custom-authentication microflow's
// unlisted parameters default to: each one other than the request and the
// response is a query parameter under its own name, with its type, in the
// microflow's order.
func publishedRestAuthParametersOf(mfName string, mf *microflows.Microflow) []*model.PublishedRestOperationParameter {
	var out []*model.PublishedRestOperationParameter
	for _, p := range operationParametersOf(mfName, mf, nil) {
		// An object or a list is no query value (CE0336, refused by
		// publishedRestAuthMicroflowProblem).
		if p.ParameterType == "Query" {
			out = append(out, p)
		}
	}
	return out
}

// publishedRestAuthKinds are the stored ParameterType values a service-level
// parameter can take: the kinds an authentication microflow can read before
// any operation runs.
var publishedRestAuthKinds = map[string]string{"query": "Query", "header": "Header"}

// publishedRestAuthListProblem is what MDL-REST04 refuses in a parameter list
// without looking at the microflow: a kind other than query or header, and a
// parameter listed twice.
func publishedRestAuthListProblem(listed []*ast.PublishedRestAuthParameter) string {
	seen := map[string]bool{}
	for _, l := range listed {
		if _, ok := publishedRestAuthKinds[l.Kind]; !ok {
			return fmt.Sprintf("lists $%s as a %s parameter: an authentication parameter is query or header", l.Name, l.Kind)
		}
		if seen[l.Name] {
			return fmt.Sprintf("lists $%s twice", l.Name)
		}
		seen[l.Name] = true
	}
	return ""
}

// resolvePublishedRestAuthParameters builds the service's authentication
// parameters from the statement's list: each listed entry in the order
// written, then every unlisted one defaulted, in the microflow's order. problem
// names an entry the microflow has no parameter for, or one Mendix supplies
// itself (the request and the response).
func resolvePublishedRestAuthParameters(mfName string, mf *microflows.Microflow, listed []*ast.PublishedRestAuthParameter) ([]*model.PublishedRestOperationParameter, string) {
	derived := publishedRestAuthParametersOf(mfName, mf)
	byName := make(map[string]*model.PublishedRestOperationParameter, len(derived))
	for _, d := range derived {
		byName[d.Name] = d
	}
	if problem := publishedRestAuthListProblem(listed); problem != "" {
		return nil, problem
	}
	used := map[string]bool{}
	var out []*model.PublishedRestOperationParameter
	for _, l := range listed {
		d, ok := byName[l.Name]
		if !ok {
			for _, p := range mf.Parameters {
				if p.Name == l.Name {
					return nil, fmt.Sprintf("lists $%s, which Mendix supplies itself: the request and the response are never listed", l.Name)
				}
			}
			return nil, fmt.Sprintf("lists $%s, which is not one of its parameters", l.Name)
		}
		used[l.Name] = true
		p := *d
		p.ParameterType = publishedRestAuthKinds[l.Kind]
		if l.HTTPName != "" {
			p.Name = l.HTTPName
		}
		p.Description = l.Description
		out = append(out, &p)
	}
	for _, d := range derived {
		if !used[d.Name] {
			out = append(out, d)
		}
	}
	return out, ""
}

// carryParameterIDs gives each parameter the stored $ID of the stored element
// bound to the same microflow parameter (the binding, not the position: a
// reordered list must not swap identities).
func carryParameterIDs(params, stored []*model.PublishedRestOperationParameter) {
	ids := make(map[string]model.ID, len(stored))
	for _, s := range stored {
		if s.MicroflowParameter != "" && s.ID != "" {
			if _, dup := ids[s.MicroflowParameter]; !dup {
				ids[s.MicroflowParameter] = s.ID
			}
		}
	}
	for _, p := range params {
		if id, ok := ids[p.MicroflowParameter]; ok {
			p.ID = id
		}
	}
}

// publishedRestAuthMicroflowProblem says why Mendix would reject mf as a
// published REST service's custom-authentication microflow, or "" when it
// would not. Measured with mx check on 11.14.0 (mendixlabs/mxcli#1331): the
// microflow returns System.User (CE0334), and each parameter is one Mendix
// supplies, matched by type and not by name (CE0336). No parameters is fine.
//
// Mendix supplies an object parameter only as the request or the response; a
// parameter of any other type is a service authentication parameter — a query
// or header value — which the executor lists for it (mendixlabs/mxcli#1417).
func publishedRestAuthMicroflowProblem(mf *microflows.Microflow) string {
	if ot, ok := mf.ReturnType.(*microflows.ObjectType); !ok || ot.EntityQualifiedName != "System.User" {
		return "must return System.User (CE0334)"
	}
	for _, p := range mf.Parameters {
		switch t := p.Type.(type) {
		case *microflows.ObjectType:
			if t.EntityQualifiedName == "System.HttpRequest" || t.EntityQualifiedName == "System.HttpResponse" {
				continue
			}
		case *microflows.ListType:
		default:
			continue
		}
		return fmt.Sprintf("has parameter $%s, which Mendix does not supply to an authentication microflow: "+
			"an object or list parameter can only be a System.HttpRequest or a System.HttpResponse, "+
			"and any other parameter is a query or header parameter of the service (CE0336)", p.Name)
	}
	return ""
}

// publishedRestAuthenticationProperty renders a service's stored
// authentication as the `Authentication:` property, in the stored order. ok is
// false when there is nothing to print (no methods: the creation default) or
// when the stored value has no MDL spelling — a method other than basic,
// session and microflow, or a microflow without the Microflow method or the
// reverse. note then says what is stored, for a comment: leaving the property
// out keeps the stored value when the output is executed.
//
// The microflow entry carries the authentication parameter list when the
// stored parameters are not what defaulting produces from mf (nil when it could
// not be read): see publishedRestAuthParameterList.
func publishedRestAuthenticationProperty(svc *model.PublishedRestService, mf *microflows.Microflow) (prop, note string, ok bool) {
	if len(svc.AuthenticationTypes) == 0 && svc.AuthenticationMicroflow == "" {
		return "", "", false
	}
	parts := make([]string, 0, len(svc.AuthenticationTypes))
	spellable, hasMicroflow := true, false
	for _, t := range svc.AuthenticationTypes {
		switch t {
		case "Basic", "Session":
			parts = append(parts, strings.ToLower(t))
		case "Microflow":
			hasMicroflow = true
			if svc.AuthenticationMicroflow == "" {
				spellable = false
				continue
			}
			parts = append(parts, "microflow "+svc.AuthenticationMicroflow+publishedRestAuthParameterList(svc, mf))
		default:
			spellable = false
		}
	}
	if spellable && hasMicroflow == (svc.AuthenticationMicroflow != "") && len(parts) > 0 {
		return "Authentication: (" + strings.Join(parts, ", ") + ")", "", true
	}
	note = "methods [" + strings.Join(svc.AuthenticationTypes, ", ") + "]"
	if svc.AuthenticationMicroflow != "" {
		note += ", microflow " + svc.AuthenticationMicroflow
	}
	return "", note, false
}

// publishedRestAuthParameterList prints the service's authentication
// parameters as ` ($Token: header 'X-Api-Token', …)`, or "" when they are what
// executing `microflow M.F` alone writes: each a query parameter under its own
// name with no description, in the microflow's order (when mf is nil the order
// cannot be checked, and is taken as the default). A parameter not bound to a
// parameter of the microflow has no spelling; publishedRestAuthParameterNotes
// names it.
func publishedRestAuthParameterList(svc *model.PublishedRestService, mf *microflows.Microflow) string {
	prefix := svc.AuthenticationMicroflow + "."
	var entries, order []string
	isDefault := true
	for _, p := range svc.Parameters {
		if !strings.HasPrefix(p.MicroflowParameter, prefix) {
			isDefault = false
			continue
		}
		bound := strings.TrimPrefix(p.MicroflowParameter, prefix)
		order = append(order, bound)
		e := "$" + bound + ": " + strings.ToLower(p.ParameterType)
		if p.ParameterType != "Query" {
			isDefault = false
		}
		if p.Name != bound {
			e += " " + mdlQuoted(p.Name)
			isDefault = false
		}
		if p.Description != "" {
			e += " description " + mdlQuoted(p.Description)
			isDefault = false
		}
		entries = append(entries, e)
	}
	if isDefault && mf != nil {
		var want []string
		for _, d := range publishedRestAuthParametersOf(svc.AuthenticationMicroflow, mf) {
			want = append(want, d.Name)
		}
		isDefault = strings.Join(want, "\x00") == strings.Join(order, "\x00")
	}
	if isDefault || len(entries) == 0 {
		return ""
	}
	return " (" + strings.Join(entries, ", ") + ")"
}

// publishedRestAuthParameterNotes names the stored authentication parameters
// the list cannot state: one bound to no parameter of the authentication
// microflow. Executing the output, which states the authentication, drops it.
func publishedRestAuthParameterNotes(svc *model.PublishedRestService) []string {
	prefix := svc.AuthenticationMicroflow + "."
	var notes []string
	for _, p := range svc.Parameters {
		if svc.AuthenticationMicroflow == "" || strings.HasPrefix(p.MicroflowParameter, prefix) {
			continue
		}
		bound := "no microflow parameter"
		if p.MicroflowParameter != "" {
			bound = p.MicroflowParameter
		}
		notes = append(notes, fmt.Sprintf("authentication parameter %s (%s) is bound to %s, which MDL cannot state; executing this drops it",
			mdlQuoted(p.Name), strings.ToLower(p.ParameterType), bound))
	}
	return notes
}

// publishedRestBindingClauses prints an operation's mapping bindings and its
// commit option, in the grammar's order. Commit is printed when it is not
// "Yes", the value exec writes when the statement has no commit clause.
func publishedRestBindingClauses(op *model.PublishedRestOperation) string {
	var b strings.Builder
	if op.ImportMapping != "" {
		b.WriteString(" import mapping " + op.ImportMapping)
	}
	if op.ExportMapping != "" {
		b.WriteString(" export mapping " + op.ExportMapping)
	}
	if op.Commit != "" && op.Commit != "Yes" {
		b.WriteString(" commit " + op.Commit)
	}
	return b.String()
}

// publishedRestParameterNotes names the operation parameters MDL cannot state:
// it derives every parameter from the microflow, so a header or form
// parameter, a parameter renamed away from its microflow parameter, or one
// with a description has no spelling. create or modify on the same project
// keeps them; a fresh create derives the parameter again.
func publishedRestParameterNotes(op *model.PublishedRestOperation) []string {
	var notes []string
	for _, p := range op.OperationParameters {
		bound := p.MicroflowParameter
		if i := strings.LastIndex(bound, "."); i >= 0 {
			bound = bound[i+1:]
		}
		var why []string
		switch p.ParameterType {
		case "Path", "Query", "Body":
		default:
			why = append(why, strings.ToLower(p.ParameterType)+" parameter")
		}
		if bound != p.Name {
			why = append(why, fmt.Sprintf("bound to $%s", bound))
		}
		if p.Description != "" {
			why = append(why, "description "+mdlQuoted(strings.ReplaceAll(p.Description, "\n", " ")))
		}
		if len(why) > 0 {
			notes = append(notes, fmt.Sprintf("parameter %s: %s (not expressible in MDL; kept by create or modify on this project)",
				p.Name, strings.Join(why, ", ")))
		}
	}
	return notes
}

// findPublishedRestService looks up a published REST service by module and name.
func findPublishedRestService(ctx *ExecContext, moduleName, name string) (*model.PublishedRestService, error) {

	services, err := ctx.Backend.ListPublishedRestServices()
	if err != nil {
		return nil, err
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil, err
	}
	// Prefer the live service over an excluded twin of the same name (#914).
	if svc, ok := pickLive(services,
		func(svc *model.PublishedRestService) bool {
			return h.GetModuleName(h.FindModuleID(svc.ContainerID)) == moduleName && svc.Name == name
		},
		func(svc *model.PublishedRestService) bool { return svc.Excluded },
	); ok {
		return svc, nil
	}
	return nil, mdlerrors.NewNotFound("published rest service", moduleName+"."+name)
}

// execCreatePublishedRestService creates a new published REST service.
func execCreatePublishedRestService(ctx *ExecContext, s *ast.CreatePublishedRestServiceStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	if err := checkVersionGates(ctx, s); err != nil {
		return err
	}

	// Check for existing service
	existing, findErr := findPublishedRestService(ctx, s.Name.Module, s.Name.Name)
	var nfe *mdlerrors.NotFoundError
	if findErr != nil && !errors.As(findErr, &nfe) {
		return mdlerrors.NewBackend("find existing service", findErr)
	}
	if existing != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExistsMsg("published rest service", s.Name.Module+"."+s.Name.Name,
			fmt.Sprintf("published rest service already exists: %s.%s (use create or modify to update)", s.Name.Module, s.Name.Name))
	}
	// The statement is the whole service: say which stored operations it
	// removes, which exec did not (MDL-REST06, ako/mxcli#2449).
	if dropped := droppedPublishedRestOperations(existing, s); len(dropped) > 0 {
		fmt.Fprintf(ctx.progress(), "Warning [%s]: %s. %s\n", publishedRestOperationDropRule,
			publishedRestDropMessage(s.Name.String(), dropped), publishedRestDropSuggestion)
	}

	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return mdlerrors.NewNotFound("module", s.Name.Module)
	}

	containerID := module.ID
	if s.Folder != "" {
		folderID, err := resolveFolder(ctx, module.ID, s.Folder)
		if err != nil {
			return mdlerrors.NewBackend(fmt.Sprintf("resolve folder '%s'", s.Folder), err)
		}
		containerID = folderID
	}

	svc := &model.PublishedRestService{
		ContainerID: containerID,
		Name:        s.Name.Name,
		Path:        s.Path,
		Version:     s.Version,
		ServiceName: s.ServiceName,
		// The doc comment (mendixlabs/mxcli#1510).
		Documentation: s.Documentation,
	}
	if existing != nil {
		// A rewrite that carried no doc comment keeps the stored one (#1018).
		svc.Documentation = carriedDocumentation(s.DocumentationSet, s.Documentation, existing.Documentation)
		svc.ID = existing.ID
		svc.AllowedRoles = existing.AllowedRoles
		// Excluded is model state, not script state (#914).
		svc.Excluded = existing.Excluded
		// An unstated authentication keeps the stored one; stating it below
		// replaces it (mendixlabs/mxcli#1331, ako/mxcli#571).
		svc.AuthenticationTypes = existing.AuthenticationTypes
		svc.AuthenticationMicroflow = existing.AuthenticationMicroflow
		svc.Parameters = existing.Parameters
	}
	if err := applyPublishedRestAuthentication(ctx, svc, s.Authentication); err != nil {
		return err
	}

	for _, resDef := range s.Resources {
		resource, err := astResourceDefToModel(resDef)
		if err != nil {
			return err
		}
		svc.Resources = append(svc.Resources, resource)
	}
	if existing != nil {
		carryStoredOperations(svc, existing)
	}
	deriveOperationParameters(ctx, svc)

	if existing != nil {
		if s.Folder == "" {
			svc.ContainerID = existing.ContainerID
		}
		if err := ctx.Backend.UpdatePublishedRestService(svc); err != nil {
			return mdlerrors.NewBackend("update published rest service", err)
		}
		if _, err := applyDocumentFolder(ctx, svc.ID, existing.ContainerID, svc.ContainerID); err != nil {
			return err
		}
		if !ctx.Quiet {
			ctx.ReportMutation("Modified", "published rest service %s.%s", s.Name.Module, s.Name.Name)
		}
	} else {
		if err := ctx.Backend.CreatePublishedRestService(svc); err != nil {
			return mdlerrors.NewBackend("create published rest service", err)
		}
		if !ctx.Quiet {
			fmt.Fprintf(ctx.Output, "Created published rest service %s.%s\n", s.Name.Module, s.Name.Name)
		}
	}
	return nil
}

// execDropPublishedRestService deletes a published REST service.
func execDropPublishedRestService(ctx *ExecContext, s *ast.DropPublishedRestServiceStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	services, err := ctx.Backend.ListPublishedRestServices()
	if err != nil {
		return mdlerrors.NewBackend("list published rest services", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == s.Name.Module && svc.Name == s.Name.Name {
			if err := ctx.Backend.DeletePublishedRestService(svc.ID); err != nil {
				return mdlerrors.NewBackend("drop published rest service", err)
			}
			if !ctx.Quiet {
				fmt.Fprintf(ctx.Output, "Dropped published rest service %s.%s\n", s.Name.Module, s.Name.Name)
			}
			return nil
		}
	}

	return mdlerrors.NewNotFound("published rest service", s.Name.Module+"."+s.Name.Name)
}

// astResourceDefToModel converts an AST PublishedRestResourceDef to the
// runtime model type used by the writer.
func astResourceDefToModel(def *ast.PublishedRestResourceDef) (*model.PublishedRestResource, error) {
	resource := &model.PublishedRestResource{Name: def.Name}
	for _, opDef := range def.Operations {
		commit, err := publishedRestCommit(opDef.Commit)
		if err != nil {
			return nil, mdlerrors.NewValidation(fmt.Sprintf("resource '%s', operation %s %s: %v",
				def.Name, opDef.HTTPMethod, opDef.Path, err))
		}
		resource.Operations = append(resource.Operations, &model.PublishedRestOperation{
			HTTPMethod:    opDef.HTTPMethod,
			Path:          opDef.Path,
			Microflow:     opDef.Microflow.String(),
			Deprecated:    opDef.Deprecated,
			ImportMapping: opDef.ImportMapping,
			ExportMapping: opDef.ExportMapping,
			Commit:        commit,
		})
	}
	return resource, nil
}

// publishedRestCommitValues are the values of Rest$PublishedRestServiceOperation.Commit.
var publishedRestCommitValues = []string{"Yes", "YesWithoutEvents", "No"}

// publishedRestCommit returns the stored spelling of an operation's commit
// clause ("" when the statement has none). Any other value used to parse and
// be thrown away; it is refused, since there is nothing correct to write.
func publishedRestCommit(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	for _, c := range publishedRestCommitValues {
		if strings.EqualFold(v, c) {
			return c, nil
		}
	}
	return "", fmt.Errorf("commit %s is not a commit option (allowed: %s)", v, strings.Join(publishedRestCommitValues, ", "))
}

// carryStoredOperations copies onto each operation a statement declares what
// MDL cannot state about it — summary, documentation, the import mapping's
// object handling, and the stored parameters (their names, kinds and
// descriptions) — from the stored operation it restates. An operation is
// matched by its resource's name, its method and its path, in order; one
// without a match is new and gets the defaults.
func carryStoredOperations(svc, stored *model.PublishedRestService) {
	pool := map[string][]*model.PublishedRestOperation{}
	key := func(res string, op *model.PublishedRestOperation) string {
		return res + "\x00" + strings.ToUpper(op.HTTPMethod) + "\x00" + op.Path
	}
	for _, res := range stored.Resources {
		for _, op := range res.Operations {
			k := key(res.Name, op)
			pool[k] = append(pool[k], op)
		}
	}
	for _, res := range svc.Resources {
		for _, op := range res.Operations {
			k := key(res.Name, op)
			if len(pool[k]) == 0 {
				continue
			}
			old := pool[k][0]
			pool[k] = pool[k][1:]
			op.Summary = old.Summary
			op.Documentation = old.Documentation
			op.ObjectHandlingBackup = old.ObjectHandlingBackup
			if old.Microflow == op.Microflow {
				op.OperationParameters = old.OperationParameters
			}
		}
	}
}

// deriveOperationParameters gives every operation the parameters Studio Pro
// derives from its microflow (ako/mxcli#571, mendixlabs/mxcli#1206): a
// parameter named in the path is a path parameter, an object or a list is the
// body, System.HttpRequest and System.HttpResponse are the request and the
// response themselves, and anything else is a query parameter — each with the
// microflow parameter's type. Without the query and body parameters mx check
// reports CE0350, and a String path parameter bound to an Integer is CE6539.
//
// The operation's stored parameters (op.OperationParameters on entry) win for
// the microflow parameter they bind: Studio Pro lets a parameter be renamed,
// described or turned into a header, and MDL has no spelling for that, so a
// rewrite keeps it. Only the type follows the microflow, and a path parameter
// follows the path.
func deriveOperationParameters(ctx *ExecContext, svc *model.PublishedRestService) {
	var microflowsByName map[string]*microflows.Microflow
	for _, resource := range svc.Resources {
		for _, op := range resource.Operations {
			if op.Microflow == "" {
				continue
			}
			if microflowsByName == nil {
				microflowsByName = liveMicroflowsByQualifiedName(ctx)
			}
			mf := microflowsByName[op.Microflow]
			if mf == nil {
				if len(op.OperationParameters) == 0 && !ctx.Quiet {
					fmt.Fprintf(ctx.Output, "Warning: microflow %s not found, so operation %s %s gets only its path parameters, as String -- "+
						"create the microflow before the service, or its other parameters fail mx check with CE0350\n",
						op.Microflow, strings.ToUpper(op.HTTPMethod), op.Path)
				}
				continue
			}
			op.OperationParameters = mergeOperationParameters(op.OperationParameters, operationParametersOf(op.Microflow, mf, op.PathParameterNames()))
		}
	}
}

// liveMicroflowsByQualifiedName indexes the project's live microflows (an
// excluded twin never shadows the live one, #914).
func liveMicroflowsByQualifiedName(ctx *ExecContext) map[string]*microflows.Microflow {
	out := map[string]*microflows.Microflow{}
	all, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return out
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return out
	}
	for _, mf := range all {
		qn := h.GetQualifiedName(mf.ContainerID, mf.Name)
		if prev, ok := out[qn]; ok && !prev.Excluded {
			continue
		}
		out[qn] = mf
	}
	return out
}

// operationParametersOf maps a microflow's parameters to operation parameters
// by the rule Studio Pro applies.
func operationParametersOf(mfName string, mf *microflows.Microflow, pathNames []string) []*model.PublishedRestOperationParameter {
	inPath := make(map[string]bool, len(pathNames))
	for _, name := range pathNames {
		inPath[name] = true
	}
	var params []*model.PublishedRestOperationParameter
	for _, p := range mf.Parameters {
		param := &model.PublishedRestOperationParameter{
			Name:               p.Name,
			ParameterType:      "Query",
			MicroflowParameter: mfName + "." + p.Name,
		}
		if p.Type != nil {
			param.DataType = p.Type.GetTypeName()
		}
		switch t := p.Type.(type) {
		case *microflows.ObjectType:
			if t.EntityQualifiedName == "System.HttpRequest" || t.EntityQualifiedName == "System.HttpResponse" {
				continue
			}
			param.ParameterType, param.DataType, param.QualifiedName = "Body", "Object", t.EntityQualifiedName
		case *microflows.ListType:
			param.ParameterType, param.DataType, param.QualifiedName = "Body", "List", t.EntityQualifiedName
		case *microflows.EnumerationType:
			param.DataType, param.QualifiedName = "Enumeration", t.EnumerationQualifiedName
		}
		if inPath[p.Name] {
			param.ParameterType = "Path"
		}
		params = append(params, param)
	}
	return params
}

// mergeOperationParameters keeps each stored parameter whose microflow
// parameter the microflow still has — in stored order, with the derived type,
// and the derived kind where either side is a path parameter — drops the ones
// it no longer has, keeps an unbound one as stored, and appends the derived
// parameters no stored one binds.
func mergeOperationParameters(stored, derived []*model.PublishedRestOperationParameter) []*model.PublishedRestOperationParameter {
	byBinding := make(map[string]*model.PublishedRestOperationParameter, len(derived))
	for _, d := range derived {
		byBinding[d.MicroflowParameter] = d
	}
	used := map[string]bool{}
	var out []*model.PublishedRestOperationParameter
	for _, sp := range stored {
		if sp.MicroflowParameter == "" {
			out = append(out, sp)
			continue
		}
		d, ok := byBinding[sp.MicroflowParameter]
		if !ok || used[sp.MicroflowParameter] {
			continue
		}
		used[sp.MicroflowParameter] = true
		kept := *sp
		if d.DataType != "" { // "" is a type the reader could not read: keep the stored one
			kept.DataType, kept.QualifiedName = d.DataType, d.QualifiedName
		}
		if d.ParameterType == "Path" || sp.ParameterType == "Path" {
			kept.Name, kept.ParameterType = d.Name, d.ParameterType
		}
		out = append(out, &kept)
	}
	for _, d := range derived {
		if !used[d.MicroflowParameter] {
			out = append(out, d)
		}
	}
	return out
}

// execAlterPublishedRestService applies SET / ADD RESOURCE / DROP RESOURCE
// actions to an existing published REST service.
func execAlterPublishedRestService(ctx *ExecContext, s *ast.AlterPublishedRestServiceStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	if err := checkVersionGates(ctx, s); err != nil {
		return err
	}

	svc, err := findPublishedRestService(ctx, s.Name.Module, s.Name.Name)
	if err != nil {
		return err
	}

	for _, action := range s.Actions {
		switch a := action.(type) {
		case *ast.PublishedRestSetAction:
			for key, val := range a.Changes {
				switch strings.ToLower(key) {
				case "path":
					svc.Path = val
				case "version":
					svc.Version = val
				case "servicename":
					svc.ServiceName = val
				default:
					return mdlerrors.NewUnsupported(fmt.Sprintf("unknown published rest service property: %s (allowed: Path, Version, ServiceName, Authentication)", key))
				}
			}
			if err := applyPublishedRestAuthentication(ctx, svc, a.Authentication); err != nil {
				return err
			}

		case *ast.PublishedRestAddResourceAction:
			// Reject duplicate resource names
			for _, existing := range svc.Resources {
				if existing.Name == a.Resource.Name {
					return mdlerrors.NewAlreadyExistsMsg("resource", a.Resource.Name, fmt.Sprintf("resource '%s' already exists on %s.%s", a.Resource.Name, s.Name.Module, s.Name.Name))
				}
			}
			resource, err := astResourceDefToModel(a.Resource)
			if err != nil {
				return err
			}
			svc.Resources = append(svc.Resources, resource)

		case *ast.PublishedRestAddOperationsAction:
			// One operation of a stored resource; the resource's other
			// operations are written back as stored (ako/mxcli#2449).
			res := publishedRestResourceNamed(svc, a.Resource)
			if res == nil {
				return mdlerrors.NewNotFoundMsg("resource", a.Resource, fmt.Sprintf(
					"resource %s not found on %s.%s: `add operation in resource` adds to a stored resource; a new one is `add resource %s { … }`",
					mdlQuoted(a.Resource), s.Name.Module, s.Name.Name, mdlQuoted(a.Resource)))
			}
			added, err := astResourceDefToModel(&ast.PublishedRestResourceDef{Name: a.Resource, Operations: a.Operations})
			if err != nil {
				return err
			}
			for _, op := range added.Operations {
				key := publishedRestOperationKey(a.Resource, op.HTTPMethod, op.Path)
				for _, stored := range res.Operations {
					if publishedRestOperationKey(a.Resource, stored.HTTPMethod, stored.Path) == key {
						return mdlerrors.NewAlreadyExistsMsg("operation", key, fmt.Sprintf(
							"operation %s already exists on %s.%s: `add operation` adds a new one; to change it, "+
								"`drop operation %s in resource %s` first and add it back", key, s.Name.Module, s.Name.Name,
							publishedRestOperationSpelling(op.HTTPMethod, op.Path), mdlQuoted(a.Resource)))
					}
				}
				res.Operations = append(res.Operations, op)
			}

		case *ast.PublishedRestDropOperationAction:
			res := publishedRestResourceNamed(svc, a.Resource)
			if res == nil {
				return mdlerrors.NewNotFoundMsg("resource", a.Resource, fmt.Sprintf("resource '%s' not found on %s.%s", a.Resource, s.Name.Module, s.Name.Name))
			}
			key := publishedRestOperationKey(a.Resource, a.HTTPMethod, a.Path)
			idx := -1
			for i, op := range res.Operations {
				if publishedRestOperationKey(a.Resource, op.HTTPMethod, op.Path) == key {
					idx = i
					break
				}
			}
			if idx == -1 {
				return mdlerrors.NewNotFoundMsg("operation", key, fmt.Sprintf("operation %s not found on %s.%s", key, s.Name.Module, s.Name.Name))
			}
			res.Operations = append(res.Operations[:idx], res.Operations[idx+1:]...)

		case *ast.PublishedRestDropResourceAction:
			idx := -1
			for i, existing := range svc.Resources {
				if existing.Name == a.Name {
					idx = i
					break
				}
			}
			if idx == -1 {
				return mdlerrors.NewNotFoundMsg("resource", a.Name, fmt.Sprintf("resource '%s' not found on %s.%s", a.Name, s.Name.Module, s.Name.Name))
			}
			svc.Resources = append(svc.Resources[:idx], svc.Resources[idx+1:]...)

		default:
			return mdlerrors.NewUnsupported(fmt.Sprintf("unsupported alter action: %T", action))
		}
	}

	deriveOperationParameters(ctx, svc)
	if err := ctx.Backend.UpdatePublishedRestService(svc); err != nil {
		return mdlerrors.NewBackend("alter published rest service", err)
	}

	if !ctx.Quiet {
		fmt.Fprintf(ctx.Output, "Altered published rest service %s.%s\n", s.Name.Module, s.Name.Name)
	}
	return nil
}

// Executor wrappers for unmigrated callers.

// publishedRestResourceNamed is the service's resource of that name, or nil.
func publishedRestResourceNamed(svc *model.PublishedRestService, name string) *model.PublishedRestResource {
	for _, r := range svc.Resources {
		if r.Name == name {
			return r
		}
	}
	return nil
}

// publishedRestOperationSpelling is an operation as MDL writes it: method and,
// when there is one, the quoted path.
func publishedRestOperationSpelling(method, path string) string {
	m := strings.ToLower(method)
	if p := strings.Trim(path, "/"); p != "" {
		return m + " " + mdlQuoted(p)
	}
	return m
}
