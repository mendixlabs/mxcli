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
		fmt.Fprintf(ctx.Output, "create or modify published rest service %s (\n", qualifiedName)
		fmt.Fprintf(ctx.Output, "  Path: '%s'", svc.Path)
		if svc.Version != "" {
			fmt.Fprintf(ctx.Output, ",\n  Version: '%s'", svc.Version)
		}
		if svc.ServiceName != "" {
			fmt.Fprintf(ctx.Output, ",\n  ServiceName: '%s'", svc.ServiceName)
		}
		folderPath := h.BuildFolderPath(svc.ContainerID)
		if folderPath != "" {
			fmt.Fprintf(ctx.Output, ",\n  Folder: '%s'", folderPath)
		}
		fmt.Fprintln(ctx.Output, "\n)")

		if len(svc.Resources) > 0 {
			fmt.Fprintln(ctx.Output, "{")
			for _, res := range svc.Resources {
				fmt.Fprintf(ctx.Output, "  resource '%s' {\n", res.Name)
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
						opPath = fmt.Sprintf(" '%s'", op.Path)
					}
					fmt.Fprintf(ctx.Output, "    %s%s%s%s;%s\n",
						strings.ToUpper(op.HTTPMethod), opPath, mf, deprecated, summary)
				}
				fmt.Fprintln(ctx.Output, "  }")
			}
			fmt.Fprintln(ctx.Output, "};")
		} else {
			fmt.Fprintln(ctx.Output, ";")
		}
		fmt.Fprintln(ctx.Output, "/")

		// Emit GRANT statements for any module roles with access.
		if len(svc.AllowedRoles) > 0 {
			fmt.Fprintf(ctx.Output, "\ngrant access on published rest service %s.%s to %s;\n",
				modName, svc.Name, strings.Join(svc.AllowedRoles, ", "))
		}

		return nil
	}

	return mdlerrors.NewNotFound("published rest service", name.String())
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

	if err := checkFeature(ctx, "integration", "published_rest_service",
		"create published rest service",
		"upgrade your project to 10.0+"); err != nil {
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
	}
	if existing != nil {
		svc.ID = existing.ID
		svc.AllowedRoles = existing.AllowedRoles
		// Excluded is model state, not script state (#914).
		svc.Excluded = existing.Excluded
	}

	for _, resDef := range s.Resources {
		resource := &model.PublishedRestResource{
			Name: resDef.Name,
		}
		for _, opDef := range resDef.Operations {
			op := &model.PublishedRestOperation{
				HTTPMethod: opDef.HTTPMethod,
				Path:       opDef.Path,
				Microflow:  opDef.Microflow.String(),
				Summary:    "",
				Deprecated: opDef.Deprecated,
			}
			resource.Operations = append(resource.Operations, op)
		}
		svc.Resources = append(svc.Resources, resource)
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

// deriveOperationParameters gives every operation the parameters Studio Pro derives from
// its microflow: a parameter named in the path is a path parameter, an object or a list is
// the body, and any other parameter is a query parameter. System.HttpRequest and
// System.HttpResponse parameters are the request and response themselves, not operation
// parameters. Without the query and body parameters mx check reports CE0350 for every
// microflow parameter the path does not name.
func deriveOperationParameters(ctx *ExecContext, svc *model.PublishedRestService) {
	for _, resource := range svc.Resources {
		for _, op := range resource.Operations {
			op.OperationParameters = nil
			if op.Microflow == "" {
				continue
			}
			mf := findMicroflowByQualifiedName(ctx, op.Microflow)
			if mf == nil {
				fmt.Fprintf(ctx.Output, "Warning: microflow %s not found, so operation %s %s gets only its path parameters -- "+
					"create the microflow before the service, or its other parameters fail mx check with CE0350\n",
					op.Microflow, strings.ToUpper(op.HTTPMethod), op.Path)
				continue
			}
			op.OperationParameters = operationParametersOf(mf, op.PathParameterNames())
		}
	}
}

// operationParametersOf maps a microflow's parameters to operation parameters.
func operationParametersOf(mf *microflows.Microflow, pathNames []string) []*model.PublishedRestOperationParameter {
	inPath := make(map[string]bool, len(pathNames))
	for _, name := range pathNames {
		inPath[name] = true
	}
	var params []*model.PublishedRestOperationParameter
	for _, p := range mf.Parameters {
		param := &model.PublishedRestOperationParameter{Name: p.Name, ParameterType: "Query"}
		if p.Type != nil {
			param.DataType = p.Type.GetTypeName()
		}
		switch t := p.Type.(type) {
		case *microflows.ObjectType:
			if t.EntityQualifiedName == "System.HttpRequest" || t.EntityQualifiedName == "System.HttpResponse" {
				continue
			}
			param.ParameterType, param.QualifiedName = "Body", t.EntityQualifiedName
		case *microflows.ListType:
			param.ParameterType, param.QualifiedName = "Body", t.EntityQualifiedName
		case *microflows.EnumerationType:
			param.QualifiedName = t.EnumerationQualifiedName
		}
		if inPath[p.Name] {
			param.ParameterType = "Path"
		}
		params = append(params, param)
	}
	return params
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
func astResourceDefToModel(def *ast.PublishedRestResourceDef) *model.PublishedRestResource {
	resource := &model.PublishedRestResource{Name: def.Name}
	for _, opDef := range def.Operations {
		resource.Operations = append(resource.Operations, &model.PublishedRestOperation{
			HTTPMethod: opDef.HTTPMethod,
			Path:       opDef.Path,
			Microflow:  opDef.Microflow.String(),
			Deprecated: opDef.Deprecated,
		})
	}
	return resource
}

// execAlterPublishedRestService applies SET / ADD RESOURCE / DROP RESOURCE
// actions to an existing published REST service.
func execAlterPublishedRestService(ctx *ExecContext, s *ast.AlterPublishedRestServiceStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	if err := checkFeature(ctx, "integration", "published_rest_alter",
		"alter published rest service",
		"upgrade your project to 10.0+"); err != nil {
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
					return mdlerrors.NewUnsupported(fmt.Sprintf("unknown published rest service property: %s (allowed: Path, Version, ServiceName)", key))
				}
			}

		case *ast.PublishedRestAddResourceAction:
			// Reject duplicate resource names
			for _, existing := range svc.Resources {
				if existing.Name == a.Resource.Name {
					return mdlerrors.NewAlreadyExistsMsg("resource", a.Resource.Name, fmt.Sprintf("resource '%s' already exists on %s.%s", a.Resource.Name, s.Name.Module, s.Name.Name))
				}
			}
			svc.Resources = append(svc.Resources, astResourceDefToModel(a.Resource))

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
