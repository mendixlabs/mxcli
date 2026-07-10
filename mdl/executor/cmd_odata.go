// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"crypto/sha256"
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
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// outputJavadoc writes a javadoc-style comment block.
func outputJavadoc(w io.Writer, text string) {
	outputJavadocIndented(w, text, "")
}

// outputJavadocIndented writes a javadoc-style comment block with an indent prefix.
func outputJavadocIndented(w io.Writer, text string, indent string) {
	lines := strings.Split(text, "\n")
	fmt.Fprintf(w, "%s/**\n", indent)
	for _, line := range lines {
		fmt.Fprintf(w, "%s * %s\n", indent, line)
	}
	fmt.Fprintf(w, "%s */\n", indent)
}

// listODataClients handles SHOW ODATA CLIENTS [IN module] command.
func listODataClients(ctx *ExecContext, moduleName string) error {

	services, err := ctx.Backend.ListConsumedODataServices()
	if err != nil {
		return mdlerrors.NewBackend("list consumed OData services", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	type row struct {
		module        string
		qualifiedName string
		version       string
		odataVer      string
		url           string
		validated     string
	}
	var rows []row

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName != "" && !strings.EqualFold(modName, moduleName) {
			continue
		}

		validated := "No"
		if svc.Validated {
			validated = "Yes"
		}

		url := svc.MetadataUrl
		if len(url) > 60 {
			url = url[:57] + "..."
		}

		qn := modName + "." + svc.Name
		rows = append(rows, row{modName, qn, svc.Version, svc.ODataVersion, url, validated})
	}

	if len(rows) == 0 && ctx.Format != FormatJSON {
		fmt.Fprintln(ctx.Output, "No consumed OData services found.")
		return nil
	}

	// Sort by qualified name
	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].qualifiedName) < strings.ToLower(rows[j].qualifiedName)
	})

	result := &TableResult{
		Columns: []string{"Module", "QualifiedName", "Version", "OData", "MetadataUrl", "Validated"},
		Summary: fmt.Sprintf("(%d OData clients)", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.module, r.qualifiedName, r.version, r.odataVer, r.url, r.validated})
	}
	return writeResult(ctx, result)
}

// describeODataClient handles DESCRIBE ODATA CLIENT command.
func describeODataClient(ctx *ExecContext, name ast.QualifiedName) error {

	services, err := ctx.Backend.ListConsumedODataServices()
	if err != nil {
		return mdlerrors.NewBackend("list consumed OData services", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)
		if strings.EqualFold(modName, name.Module) && strings.EqualFold(svc.Name, name.Name) {
			folderPath := h.BuildFolderPath(svc.ContainerID)
			return outputConsumedODataServiceMDL(ctx, svc, modName, folderPath)
		}
	}

	return mdlerrors.NewNotFoundMsg("consumed OData service", fmt.Sprint(name), fmt.Sprintf("consumed OData service not found: %s", name))
}

// outputConsumedODataServiceMDL outputs a consumed OData service in MDL format.
func outputConsumedODataServiceMDL(ctx *ExecContext, svc *model.ConsumedODataService, moduleName string, folderPath string) error {
	// Use Description for javadoc (the user-visible API description)
	if svc.Description != "" {
		outputJavadoc(ctx.Output, svc.Description)
	}

	fmt.Fprintf(ctx.Output, "create odata client %s.%s (\n", moduleName, svc.Name)

	var props []string
	if folderPath != "" {
		props = append(props, fmt.Sprintf("  Folder: '%s'", folderPath))
	}
	if svc.Version != "" {
		props = append(props, fmt.Sprintf("  Version: '%s'", svc.Version))
	}
	if svc.ODataVersion != "" {
		props = append(props, fmt.Sprintf("  ODataVersion: %s", svc.ODataVersion))
	}
	if svc.MetadataUrl != "" {
		props = append(props, fmt.Sprintf("  MetadataUrl: '%s'", svc.MetadataUrl))
	}
	if svc.TimeoutExpression != "" {
		props = append(props, fmt.Sprintf("  Timeout: %s", svc.TimeoutExpression))
	}
	if svc.ProxyType != "" && svc.ProxyType != "DefaultProxy" {
		props = append(props, fmt.Sprintf("  ProxyType: %s", svc.ProxyType))
	}

	// HTTP configuration
	if cfg := svc.HttpConfiguration; cfg != nil {
		if cfg.OverrideLocation && cfg.CustomLocation != "" {
			props = append(props, fmt.Sprintf("  ServiceUrl: %s", formatExprValue(cfg.CustomLocation)))
		}
		if cfg.UseAuthentication {
			props = append(props, "  UseAuthentication: Yes")
			if cfg.Username != "" {
				props = append(props, fmt.Sprintf("  HttpUsername: %s", formatExprValue(cfg.Username)))
			}
			if cfg.Password != "" {
				props = append(props, fmt.Sprintf("  HttpPassword: %s", formatExprValue(cfg.Password)))
			}
		}
		if cfg.ClientCertificate != "" {
			props = append(props, fmt.Sprintf("  ClientCertificate: '%s'", cfg.ClientCertificate))
		}
	}

	// Microflow references. The "Configuration microflow" and "Headers
	// microflow" dropdown options are distinct storage slots (Mendix >= 11.10),
	// so DESCRIBE emits the matching MDL keyword for each.
	if svc.ConfigurationMicroflow != "" {
		props = append(props, fmt.Sprintf("  ConfigurationMicroflow: microflow %s", svc.ConfigurationMicroflow))
	}
	if svc.HeadersMicroflow != "" {
		props = append(props, fmt.Sprintf("  HeadersMicroflow: microflow %s", svc.HeadersMicroflow))
	}
	if svc.ErrorHandlingMicroflow != "" {
		props = append(props, fmt.Sprintf("  ErrorHandlingMicroflow: microflow %s", svc.ErrorHandlingMicroflow))
	}

	// Proxy constant references
	if svc.ProxyHost != "" {
		props = append(props, fmt.Sprintf("  ProxyHost: %s", svc.ProxyHost))
	}
	if svc.ProxyPort != "" {
		props = append(props, fmt.Sprintf("  ProxyPort: %s", svc.ProxyPort))
	}
	if svc.ProxyUsername != "" {
		props = append(props, fmt.Sprintf("  ProxyUsername: %s", svc.ProxyUsername))
	}
	if svc.ProxyPassword != "" {
		props = append(props, fmt.Sprintf("  ProxyPassword: %s", svc.ProxyPassword))
	}

	fmt.Fprintln(ctx.Output, strings.Join(props, ",\n"))

	// Custom HTTP headers (between property block close and semicolon)
	if cfg := svc.HttpConfiguration; cfg != nil && len(cfg.HeaderEntries) > 0 {
		fmt.Fprintln(ctx.Output, ")")
		fmt.Fprintln(ctx.Output, "headers (")
		for i, h := range cfg.HeaderEntries {
			comma := ","
			if i == len(cfg.HeaderEntries)-1 {
				comma = ""
			}
			fmt.Fprintf(ctx.Output, "  '%s': %s%s\n", h.Key, formatExprValue(h.Value), comma)
		}
		fmt.Fprintln(ctx.Output, ");")
	} else {
		fmt.Fprintln(ctx.Output, ");")
	}

	fmt.Fprintln(ctx.Output, "/")

	return nil
}

// listODataServices handles SHOW ODATA SERVICES [IN module] command.
func listODataServices(ctx *ExecContext, moduleName string) error {

	services, err := ctx.Backend.ListPublishedODataServices()
	if err != nil {
		return mdlerrors.NewBackend("list published OData services", err)
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
		odataVer      string
		entitySets    string
		authTypes     string
	}
	var rows []row

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName != "" && !strings.EqualFold(modName, moduleName) {
			continue
		}

		esCount := fmt.Sprintf("%d", len(svc.EntitySets))
		authStr := strings.Join(svc.AuthenticationTypes, ", ")
		if len(authStr) > 30 {
			authStr = authStr[:27] + "..."
		}

		qn := modName + "." + svc.Name
		rows = append(rows, row{modName, qn, svc.Path, svc.Version, svc.ODataVersion, esCount, authStr})
	}

	if len(rows) == 0 && ctx.Format != FormatJSON {
		fmt.Fprintln(ctx.Output, "No published OData services found.")
		return nil
	}

	// Sort by qualified name
	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].qualifiedName) < strings.ToLower(rows[j].qualifiedName)
	})

	result := &TableResult{
		Columns: []string{"Module", "QualifiedName", "Path", "Version", "OData", "EntitySets", "AuthTypes"},
		Summary: fmt.Sprintf("(%d OData services)", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.module, r.qualifiedName, r.path, r.version, r.odataVer, r.entitySets, r.authTypes})
	}
	return writeResult(ctx, result)
}

// describeODataService handles DESCRIBE ODATA SERVICE command.
func describeODataService(ctx *ExecContext, name ast.QualifiedName) error {

	services, err := ctx.Backend.ListPublishedODataServices()
	if err != nil {
		return mdlerrors.NewBackend("list published OData services", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)
		if strings.EqualFold(modName, name.Module) && strings.EqualFold(svc.Name, name.Name) {
			folderPath := h.BuildFolderPath(svc.ContainerID)
			return outputPublishedODataServiceMDL(ctx, svc, modName, folderPath)
		}
	}

	return mdlerrors.NewNotFoundMsg("published OData service", fmt.Sprint(name), fmt.Sprintf("published OData service not found: %s", name))
}

// outputPublishedODataServiceMDL outputs a published OData service in MDL format.
func outputPublishedODataServiceMDL(ctx *ExecContext, svc *model.PublishedODataService, moduleName string, folderPath string) error {
	// Use Description for javadoc (the user-visible API description)
	if svc.Description != "" {
		outputJavadoc(ctx.Output, svc.Description)
	}

	fmt.Fprintf(ctx.Output, "create odata service %s.%s (\n", moduleName, svc.Name)

	var props []string
	if folderPath != "" {
		props = append(props, fmt.Sprintf("  Folder: '%s'", folderPath))
	}
	if svc.Path != "" {
		props = append(props, fmt.Sprintf("  Path: '%s'", svc.Path))
	}
	if svc.Version != "" {
		props = append(props, fmt.Sprintf("  Version: '%s'", svc.Version))
	}
	if svc.ODataVersion != "" {
		props = append(props, fmt.Sprintf("  ODataVersion: %s", svc.ODataVersion))
	}
	if svc.Namespace != "" {
		props = append(props, fmt.Sprintf("  Namespace: '%s'", svc.Namespace))
	}
	if svc.ServiceName != "" {
		props = append(props, fmt.Sprintf("  ServiceName: '%s'", svc.ServiceName))
	}
	if svc.Summary != "" {
		props = append(props, fmt.Sprintf("  Summary: '%s'", svc.Summary))
	}
	if svc.PublishAssociations {
		props = append(props, "  PublishAssociations: Yes")
	}
	fmt.Fprintln(ctx.Output, strings.Join(props, ",\n"))

	fmt.Fprintln(ctx.Output, ")")

	// Authentication types
	if len(svc.AuthenticationTypes) > 0 {
		fmt.Fprintf(ctx.Output, "authentication %s\n", strings.Join(svc.AuthenticationTypes, ", "))
	}
	if svc.AuthMicroflow != "" {
		fmt.Fprintf(ctx.Output, "-- Auth Microflow: %s\n", svc.AuthMicroflow)
	}

	// Published entities block
	if len(svc.EntityTypes) > 0 || len(svc.EntitySets) > 0 {
		fmt.Fprintln(ctx.Output, "{")

		// Build entity set lookup by exposed name and entity type name for merging
		entitySetByExposedName := make(map[string]*model.PublishedEntitySet)
		entitySetByEntityName := make(map[string]*model.PublishedEntitySet)
		for _, es := range svc.EntitySets {
			if es.ExposedName != "" {
				entitySetByExposedName[es.ExposedName] = es
			}
			if es.EntityTypeName != "" {
				entitySetByEntityName[es.EntityTypeName] = es
			}
		}

		for _, et := range svc.EntityTypes {
			// Entity-level javadoc from Summary/Description
			if et.Summary != "" || et.Description != "" {
				doc := et.Summary
				if et.Description != "" {
					if doc != "" {
						doc += "\n\n" + et.Description
					} else {
						doc = et.Description
					}
				}
				outputJavadocIndented(ctx.Output, doc, "  ")
			}

			// Find matching entity set (try exposed name first, then entity reference)
			es := entitySetByExposedName[et.ExposedName]
			if es == nil {
				es = entitySetByEntityName[et.Entity]
			}

			// PUBLISH ENTITY line with modes
			fmt.Fprintf(ctx.Output, "  publish entity %s as '%s'", et.Entity, et.ExposedName)
			if es != nil {
				var modeProps []string
				if es.ReadMode != "" {
					modeProps = append(modeProps, fmt.Sprintf("ReadMode: %s", es.ReadMode))
				}
				if es.InsertMode != "" {
					modeProps = append(modeProps, fmt.Sprintf("InsertMode: %s", es.InsertMode))
				}
				if es.UpdateMode != "" {
					modeProps = append(modeProps, fmt.Sprintf("UpdateMode: %s", es.UpdateMode))
				}
				if es.DeleteMode != "" {
					modeProps = append(modeProps, fmt.Sprintf("DeleteMode: %s", es.DeleteMode))
				}
				if es.UsePaging {
					modeProps = append(modeProps, "UsePaging: Yes")
					modeProps = append(modeProps, fmt.Sprintf("PageSize: %d", es.PageSize))
				}
				if len(modeProps) > 0 {
					fmt.Fprintf(ctx.Output, " (\n    %s\n  )", strings.Join(modeProps, ",\n    "))
				}
			}
			fmt.Fprintln(ctx.Output)

			// EXPOSE members
			if len(et.Members) > 0 {
				fmt.Fprintln(ctx.Output, "  expose (")
				for i, m := range et.Members {
					var modifiers []string
					if m.Filterable {
						modifiers = append(modifiers, "Filterable")
					}
					if m.Sortable {
						modifiers = append(modifiers, "Sortable")
					}
					if m.IsPartOfKey {
						modifiers = append(modifiers, "IsPartOfKey")
					}

					line := fmt.Sprintf("    %s as '%s'", m.Name, m.ExposedName)
					if len(modifiers) > 0 {
						line += fmt.Sprintf(" (%s)", strings.Join(modifiers, ", "))
					}
					if i < len(et.Members)-1 {
						line += ","
					}
					fmt.Fprintln(ctx.Output, line)
				}
				fmt.Fprintln(ctx.Output, "  );")
			}
			fmt.Fprintln(ctx.Output)
		}

		fmt.Fprintln(ctx.Output, "}")
	}

	// Output GRANT statements for allowed module roles
	if len(svc.AllowedModuleRoles) > 0 {
		fmt.Fprintln(ctx.Output)
		fmt.Fprintf(ctx.Output, "grant access on odata service %s.%s to %s;\n",
			moduleName, svc.Name, strings.Join(svc.AllowedModuleRoles, ", "))
	}

	fmt.Fprintln(ctx.Output, "/")

	return nil
}

// listExternalEntities handles SHOW EXTERNAL ENTITIES [IN module] command.
func listExternalEntities(ctx *ExecContext, moduleName string) error {

	domainModels, err := ctx.Backend.ListDomainModels()
	if err != nil {
		return mdlerrors.NewBackend("list domain models", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	type row struct {
		module        string
		qualifiedName string
		service       string
		entitySet     string
		remoteName    string
		countable     string
	}
	var rows []row

	for _, dm := range domainModels {
		modID := h.FindModuleID(dm.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName != "" && !strings.EqualFold(modName, moduleName) {
			continue
		}

		for _, entity := range dm.Entities {
			if entity.Source != "Rest$ODataRemoteEntitySource" {
				continue
			}

			countable := "No"
			if entity.Countable {
				countable = "Yes"
			}

			qn := modName + "." + entity.Name
			rows = append(rows, row{modName, qn, entity.RemoteServiceName, entity.RemoteEntitySet, entity.RemoteEntityName, countable})
		}
	}

	if len(rows) == 0 && ctx.Format != FormatJSON {
		fmt.Fprintln(ctx.Output, "No external entities found.")
		return nil
	}

	// Sort by qualified name
	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].qualifiedName) < strings.ToLower(rows[j].qualifiedName)
	})

	result := &TableResult{
		Columns: []string{"Module", "QualifiedName", "Service", "EntitySet", "RemoteName", "Countable"},
		Summary: fmt.Sprintf("(%d external entities)", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.module, r.qualifiedName, r.service, r.entitySet, r.remoteName, r.countable})
	}
	return writeResult(ctx, result)
}

// listExternalActions handles SHOW EXTERNAL ACTIONS [IN module] command.
// It scans all microflows and nanoflows for CallExternalAction activities
// and displays the unique actions grouped by consumed OData service.
func listExternalActions(ctx *ExecContext, moduleName string) error {

	mfs, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return mdlerrors.NewBackend("list microflows", err)
	}
	nfs, err := ctx.Backend.ListNanoflows()
	if err != nil {
		return mdlerrors.NewBackend("list nanoflows", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Collect unique actions: key = service + "." + action name
	type actionInfo struct {
		service    string // Consumed OData service qualified name
		actionName string // External action name
		params     []string
		callers    []string // Microflow/nanoflow qualified names that call this action
	}
	actionMap := make(map[string]*actionInfo) // key = service.actionName

	// Helper to extract actions from a microflow object collection
	extractActions := func(oc *microflows.MicroflowObjectCollection, flowModule, flowName string) {
		if oc == nil {
			return
		}
		for _, obj := range oc.Objects {
			act, ok := obj.(*microflows.ActionActivity)
			if !ok || act.Action == nil {
				continue
			}
			cea, ok := act.Action.(*microflows.CallExternalAction)
			if !ok {
				continue
			}

			key := cea.ConsumedODataService + "." + cea.Name
			info, exists := actionMap[key]
			if !exists {
				var params []string
				for _, pm := range cea.ParameterMappings {
					params = append(params, pm.ParameterName)
				}
				info = &actionInfo{
					service:    cea.ConsumedODataService,
					actionName: cea.Name,
					params:     params,
				}
				actionMap[key] = info
			}
			caller := flowModule + "." + flowName
			// Avoid duplicate caller entries
			found := false
			for _, c := range info.callers {
				if c == caller {
					found = true
					break
				}
			}
			if !found {
				info.callers = append(info.callers, caller)
			}
			// Merge parameter names from different call sites
			if len(cea.ParameterMappings) > len(info.params) {
				info.params = nil
				for _, pm := range cea.ParameterMappings {
					info.params = append(info.params, pm.ParameterName)
				}
			}
		}
	}

	for _, mf := range mfs {
		modID := h.FindModuleID(mf.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName != "" && !strings.EqualFold(modName, moduleName) {
			continue
		}
		extractActions(mf.ObjectCollection, modName, mf.Name)
	}
	for _, nf := range nfs {
		modID := h.FindModuleID(nf.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName != "" && !strings.EqualFold(modName, moduleName) {
			continue
		}
		extractActions(nf.ObjectCollection, modName, nf.Name)
	}

	if len(actionMap) == 0 && ctx.Format != FormatJSON {
		fmt.Fprintln(ctx.Output, "No external actions found.")
		return nil
	}

	// Collect and sort rows
	type row struct {
		service    string
		actionName string
		params     string
		usedBy     string
	}
	var rows []row

	for _, info := range actionMap {
		params := strings.Join(info.params, ", ")
		usedBy := strings.Join(info.callers, ", ")
		rows = append(rows, row{info.service, info.actionName, params, usedBy})
	}

	// Sort by service, then action name
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].service != rows[j].service {
			return strings.ToLower(rows[i].service) < strings.ToLower(rows[j].service)
		}
		return strings.ToLower(rows[i].actionName) < strings.ToLower(rows[j].actionName)
	})

	result := &TableResult{
		Columns: []string{"Service", "Action", "Parameters", "UsedBy"},
		Summary: fmt.Sprintf("(%d external actions)", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.service, r.actionName, r.params, r.usedBy})
	}
	return writeResult(ctx, result)
}

// describeExternalEntity handles DESCRIBE EXTERNAL ENTITY command.
func describeExternalEntity(ctx *ExecContext, name ast.QualifiedName) error {

	domainModels, err := ctx.Backend.ListDomainModels()
	if err != nil {
		return mdlerrors.NewBackend("list domain models", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, dm := range domainModels {
		modID := h.FindModuleID(dm.ContainerID)
		modName := h.GetModuleName(modID)
		if !strings.EqualFold(modName, name.Module) {
			continue
		}

		for _, entity := range dm.Entities {
			if !strings.EqualFold(entity.Name, name.Name) {
				continue
			}

			if entity.Source != "Rest$ODataRemoteEntitySource" {
				return mdlerrors.NewValidationf("%s.%s is not an external entity (source: %s)", modName, entity.Name, entity.Source)
			}

			return outputExternalEntityMDL(ctx, entity, modName)
		}
	}

	return mdlerrors.NewNotFoundMsg("external entity", fmt.Sprint(name), fmt.Sprintf("external entity not found: %s", name))
}

// outputExternalEntityMDL outputs an external entity in MDL format.
func outputExternalEntityMDL(ctx *ExecContext, entity *domainmodel.Entity, moduleName string) error {
	if entity.Documentation != "" {
		outputJavadoc(ctx.Output, entity.Documentation)
	}

	fmt.Fprintf(ctx.Output, "create external entity %s.%s\n", moduleName, entity.Name)
	fmt.Fprintf(ctx.Output, "from odata client %s\n", entity.RemoteServiceName)
	fmt.Fprintln(ctx.Output, "(")

	var props []string
	if entity.RemoteEntitySet != "" {
		props = append(props, fmt.Sprintf("  EntitySet: '%s'", entity.RemoteEntitySet))
	}
	if entity.RemoteEntityName != "" {
		props = append(props, fmt.Sprintf("  RemoteName: '%s'", entity.RemoteEntityName))
	}
	boolStr := func(b bool) string {
		if b {
			return "Yes"
		}
		return "No"
	}
	props = append(props, fmt.Sprintf("  Countable: %s", boolStr(entity.Countable)))
	props = append(props, fmt.Sprintf("  Creatable: %s", boolStr(entity.Creatable)))
	props = append(props, fmt.Sprintf("  Deletable: %s", boolStr(entity.Deletable)))
	props = append(props, fmt.Sprintf("  Updatable: %s", boolStr(entity.Updatable)))
	props = append(props, fmt.Sprintf("  AllowCreateChangeLocally: %s", boolStr(entity.CreateChangeLocally)))
	fmt.Fprintln(ctx.Output, strings.Join(props, ",\n"))

	fmt.Fprintln(ctx.Output, ")")

	// Output attributes
	if len(entity.Attributes) > 0 {
		fmt.Fprintln(ctx.Output, "(")
		for i, attr := range entity.Attributes {
			typeName := "Unknown"
			if attr.Type != nil {
				typeName = attr.Type.GetTypeName()
			}
			comma := ","
			if i == len(entity.Attributes)-1 {
				comma = ""
			}
			fmt.Fprintf(ctx.Output, "  %s: %s%s\n", attr.Name, typeName, comma)
		}
		fmt.Fprintln(ctx.Output, ");")
	}

	fmt.Fprintln(ctx.Output, "/")

	return nil
}

// ============================================================================
// CREATE EXTERNAL ENTITY
// ============================================================================

// execCreateExternalEntity handles CREATE [OR MODIFY] EXTERNAL ENTITY statements.
func execCreateExternalEntity(ctx *ExecContext, s *ast.CreateExternalEntityStmt) error {

	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	if s.Name.Module == "" {
		return mdlerrors.NewValidation("module name required: use create external entity Module.Name from odata client ...")
	}

	// Find module
	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	// Validate that the referenced OData client exists.
	if err := validateODataClientExists(ctx, s.ServiceRef); err != nil {
		return err
	}

	// Get domain model
	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	// Check if entity already exists
	var existingEntity *domainmodel.Entity
	for _, entity := range dm.Entities {
		if entity.Name == s.Name.Name {
			existingEntity = entity
			break
		}
	}

	if existingEntity != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExistsMsg("entity", s.Name.Module+"."+s.Name.Name, fmt.Sprintf("entity already exists: %s.%s (use create or modify to update)", s.Name.Module, s.Name.Name))
	}

	// Build attributes
	var attrs []*domainmodel.Attribute
	for _, a := range s.Attributes {
		attr := &domainmodel.Attribute{
			Name: a.Name,
			Type: convertDataType(a.Type),
		}
		attr.ID = model.ID(types.GenerateID())
		attrs = append(attrs, attr)
	}

	// Service reference as qualified name
	serviceRef := s.ServiceRef.String()

	if existingEntity != nil {
		// Update existing entity. Issue #594: only assign fields that the
		// MDL statement explicitly set. Omitted (nil) fields preserve the
		// prior value — previously the executor unconditionally wrote zero
		// values, wiping fields like RemoteName and triggering Studio Pro
		// NREs (e.g. ODataRemoteEntitySource.RemoteId on empty RemoteName).
		existingEntity.Source = "Rest$ODataRemoteEntitySource"
		existingEntity.RemoteServiceName = serviceRef
		if s.EntitySet != nil {
			existingEntity.RemoteEntitySet = *s.EntitySet
		}
		if s.RemoteName != nil {
			existingEntity.RemoteEntityName = *s.RemoteName
		}
		if s.Countable != nil {
			existingEntity.Countable = *s.Countable
		}
		if s.Creatable != nil {
			existingEntity.Creatable = *s.Creatable
		}
		if s.Deletable != nil {
			existingEntity.Deletable = *s.Deletable
		}
		if s.Updatable != nil {
			existingEntity.Updatable = *s.Updatable
		}
		if s.AllowCreateChangeLocally != nil {
			existingEntity.CreateChangeLocally = *s.AllowCreateChangeLocally
		}
		if len(attrs) > 0 {
			existingEntity.Attributes = attrs
		}
		if s.Documentation != "" {
			existingEntity.Documentation = s.Documentation
		}
		if err := ctx.Backend.UpdateEntity(dm.ID, existingEntity); err != nil {
			return mdlerrors.NewBackend("update external entity", err)
		}
		fmt.Fprintf(ctx.Output, "Modified external entity: %s.%s\n", s.Name.Module, s.Name.Name)
		return nil
	}

	// Initial create: EntitySet is required (no prior value to preserve).
	if s.EntitySet == nil || *s.EntitySet == "" {
		return mdlerrors.NewValidation("EntitySet property is required when creating an external entity")
	}

	// Auto-position based on existing entities
	location := model.Point{X: 100 + len(dm.Entities)*150, Y: 100}

	newEntity := &domainmodel.Entity{
		Name:                s.Name.Name,
		Documentation:       s.Documentation,
		Persistable:         false, // External entities are not persistable
		Location:            location,
		Attributes:          attrs,
		Source:              "Rest$ODataRemoteEntitySource",
		RemoteServiceName:   serviceRef,
		RemoteEntitySet:     *s.EntitySet,
		RemoteEntityName:    derefString(s.RemoteName),
		Countable:           derefBool(s.Countable),
		Creatable:           derefBool(s.Creatable),
		Deletable:           derefBool(s.Deletable),
		Updatable:           derefBool(s.Updatable),
		CreateChangeLocally: derefBool(s.AllowCreateChangeLocally),
	}
	newEntity.ID = model.ID(types.GenerateID())

	if err := ctx.Backend.CreateEntity(dm.ID, newEntity); err != nil {
		return mdlerrors.NewBackend("create external entity", err)
	}
	fmt.Fprintf(ctx.Output, "Created external entity: %s.%s\n", s.Name.Module, s.Name.Name)
	return nil
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefBool(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}

// ============================================================================
// OData Write Handlers (CREATE / ALTER / DROP)
// ============================================================================

// createODataClient handles CREATE ODATA CLIENT command.
func createODataClient(ctx *ExecContext, stmt *ast.CreateODataClientStmt) error {

	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	if stmt.Name.Module == "" {
		return mdlerrors.NewValidation("module name required: use create odata client Module.Name (...)")
	}

	if err := validateMetadataURL(stmt.MetadataUrl); err != nil {
		return err
	}

	module, err := findModule(ctx, stmt.Name.Module)
	if err != nil {
		return err
	}

	// Check if client already exists
	services, err := ctx.Backend.ListConsumedODataServices()
	if err == nil {
		h, _ := getHierarchy(ctx)
		for _, svc := range services {
			modID := h.FindModuleID(svc.ContainerID)
			modName := h.GetModuleName(modID)
			if strings.EqualFold(modName, stmt.Name.Module) && strings.EqualFold(svc.Name, stmt.Name.Name) {
				if stmt.CreateOrModify {
					svc.Documentation = stmt.Documentation
					if stmt.Version != "" {
						svc.Version = stmt.Version
					}
					if stmt.ODataVersion != "" {
						svc.ODataVersion = stmt.ODataVersion
					}
					if stmt.MetadataUrl != "" {
						svc.MetadataUrl = stmt.MetadataUrl
					}
					if stmt.TimeoutExpression != "" {
						svc.TimeoutExpression = stmt.TimeoutExpression
					}
					if stmt.ProxyType != "" {
						svc.ProxyType = stmt.ProxyType
					}
					if stmt.Description != "" {
						svc.Description = stmt.Description
					}
					if stmt.ConfigurationMicroflow != "" {
						svc.ConfigurationMicroflow = extractMicroflowRef(stmt.ConfigurationMicroflow)
					}
					if stmt.HeadersMicroflow != "" {
						svc.HeadersMicroflow = extractMicroflowRef(stmt.HeadersMicroflow)
					}
					if stmt.ErrorHandlingMicroflow != "" {
						svc.ErrorHandlingMicroflow = extractMicroflowRef(stmt.ErrorHandlingMicroflow)
					}
					if stmt.ProxyHost != "" {
						svc.ProxyHost = stmt.ProxyHost
					}
					if stmt.ProxyPort != "" {
						svc.ProxyPort = stmt.ProxyPort
					}
					if stmt.ProxyUsername != "" {
						svc.ProxyUsername = stmt.ProxyUsername
					}
					if stmt.ProxyPassword != "" {
						svc.ProxyPassword = stmt.ProxyPassword
					}
					// Update HTTP configuration
					if stmt.ServiceUrl != "" || stmt.UseAuthentication || stmt.HttpUsername != "" ||
						stmt.HttpPassword != "" || stmt.ClientCertificate != "" || len(stmt.Headers) > 0 {
						if svc.HttpConfiguration == nil {
							svc.HttpConfiguration = &model.HttpConfiguration{}
						}
						if stmt.ServiceUrl != "" {
							if err := validateServiceURL(stmt.ServiceUrl); err != nil {
								return err
							}
							svc.HttpConfiguration.OverrideLocation = true
							svc.HttpConfiguration.CustomLocation = stmt.ServiceUrl
						}
						svc.HttpConfiguration.UseAuthentication = stmt.UseAuthentication
						if stmt.HttpUsername != "" {
							svc.HttpConfiguration.Username = stmt.HttpUsername
						}
						if stmt.HttpPassword != "" {
							svc.HttpConfiguration.Password = stmt.HttpPassword
						}
						if stmt.ClientCertificate != "" {
							svc.HttpConfiguration.ClientCertificate = stmt.ClientCertificate
						}
						if len(stmt.Headers) > 0 {
							svc.HttpConfiguration.HeaderEntries = nil
							for _, h := range stmt.Headers {
								svc.HttpConfiguration.HeaderEntries = append(svc.HttpConfiguration.HeaderEntries, &model.HttpHeaderEntry{
									Key:   h.Key,
									Value: h.Value,
								})
							}
						}
					}
					if err := ctx.Backend.UpdateConsumedODataService(svc); err != nil {
						return mdlerrors.NewBackend("update OData client", err)
					}
					invalidateHierarchy(ctx)
					fmt.Fprintf(ctx.Output, "Modified OData client: %s.%s\n", modName, svc.Name)
					return nil
				}
				return mdlerrors.NewAlreadyExistsMsg("OData client", modName+"."+svc.Name, fmt.Sprintf("OData client already exists: %s.%s (use create or modify to update)", modName, svc.Name))
			}
		}
	}

	// Resolve folder if specified
	containerID := module.ID
	if stmt.Folder != "" {
		folderID, err := resolveFolder(ctx, module.ID, stmt.Folder)
		if err != nil {
			return mdlerrors.NewBackend(fmt.Sprintf("resolve folder %s", stmt.Folder), err)
		}
		containerID = folderID
	}

	timeout := stmt.TimeoutExpression
	if timeout == "" {
		timeout = "300" // Mendix requires a non-empty Timeout (CE6893)
	}

	newSvc := &model.ConsumedODataService{
		ContainerID:            containerID,
		Name:                   stmt.Name.Name,
		ServiceName:            stmt.Name.Name, // Default ServiceName to document name (CE0339)
		Documentation:          stmt.Documentation,
		Version:                stmt.Version,
		ODataVersion:           stmt.ODataVersion,
		MetadataUrl:            stmt.MetadataUrl,
		TimeoutExpression:      timeout,
		ProxyType:              stmt.ProxyType,
		Description:            stmt.Description,
		ConfigurationMicroflow: extractMicroflowRef(stmt.ConfigurationMicroflow),
		HeadersMicroflow:       extractMicroflowRef(stmt.HeadersMicroflow),
		ErrorHandlingMicroflow: extractMicroflowRef(stmt.ErrorHandlingMicroflow),
		ProxyHost:              stmt.ProxyHost,
		ProxyPort:              stmt.ProxyPort,
		ProxyUsername:          stmt.ProxyUsername,
		ProxyPassword:          stmt.ProxyPassword,
	}

	// Build HTTP configuration if any HTTP-level properties are set
	if stmt.ServiceUrl != "" || stmt.UseAuthentication || stmt.HttpUsername != "" ||
		stmt.HttpPassword != "" || stmt.ClientCertificate != "" || len(stmt.Headers) > 0 {
		cfg := &model.HttpConfiguration{
			UseAuthentication: stmt.UseAuthentication,
			Username:          stmt.HttpUsername,
			Password:          stmt.HttpPassword,
			ClientCertificate: stmt.ClientCertificate,
		}
		if stmt.ServiceUrl != "" {
			// ServiceUrl must be a constant reference (e.g., @Module.ConstantName)
			if !strings.HasPrefix(stmt.ServiceUrl, "@") {
				return fmt.Errorf(`ServiceUrl must now be a constant reference (e.g., '@Module.ApiLocation').
Previously literal URLs were allowed; this enforces the Mendix best practice of externalizing configuration.
Create a constant first:
  CREATE CONSTANT Module.ApiLocation TYPE String DEFAULT 'https://api.example.com/';
Then reference it:
  ServiceUrl: '@Module.ApiLocation'
Got: %s`, stmt.ServiceUrl)
			}
			cfg.OverrideLocation = true
			cfg.CustomLocation = stmt.ServiceUrl
		}
		for _, h := range stmt.Headers {
			cfg.HeaderEntries = append(cfg.HeaderEntries, &model.HttpHeaderEntry{
				Key:   h.Key,
				Value: h.Value,
			})
		}
		newSvc.HttpConfiguration = cfg
	}

	// Fetch and cache $metadata from the service URL
	// Normalize local file paths to absolute file:// URLs for Studio Pro compatibility
	if newSvc.MetadataUrl != "" {
		mprDir := ""
		if ctx.MprPath != "" {
			mprDir = filepath.Dir(ctx.MprPath)
		}

		// Normalize MetadataUrl: convert relative paths to absolute file:// URLs
		normalizedUrl, err := pathutil.NormalizeURL(newSvc.MetadataUrl, mprDir)
		if err != nil {
			return fmt.Errorf("failed to normalize MetadataUrl: %w", err)
		}
		newSvc.MetadataUrl = normalizedUrl

		metadata, hash, err := fetchODataMetadata(normalizedUrl)
		if err != nil {
			fmt.Fprintf(ctx.Output, "Warning: could not fetch $metadata: %v\n", err)
		} else if metadata != "" {
			newSvc.Metadata = metadata
			newSvc.MetadataHash = hash
			newSvc.Validated = true
		}
	}

	if err := ctx.Backend.CreateConsumedODataService(newSvc); err != nil {
		return mdlerrors.NewBackend("create OData client", err)
	}
	invalidateHierarchy(ctx)
	fmt.Fprintf(ctx.Output, "Created OData client: %s.%s\n", stmt.Name.Module, stmt.Name.Name)
	if newSvc.Metadata != "" {
		// Parse to show summary
		if doc, err := types.ParseEdmx(newSvc.Metadata); err == nil {
			entityCount := 0
			actionCount := 0
			for _, s := range doc.Schemas {
				entityCount += len(s.EntityTypes)
			}
			actionCount = len(doc.Actions)
			fmt.Fprintf(ctx.Output, "  Cached $metadata: %d entity types, %d actions\n", entityCount, actionCount)
		}
	}
	return nil
}

// alterODataClient handles ALTER ODATA CLIENT command.
func alterODataClient(ctx *ExecContext, stmt *ast.AlterODataClientStmt) error {

	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	services, err := ctx.Backend.ListConsumedODataServices()
	if err != nil {
		return mdlerrors.NewBackend("list consumed OData services", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)
		if strings.EqualFold(modName, stmt.Name.Module) && strings.EqualFold(svc.Name, stmt.Name.Name) {
			for key, val := range stmt.Changes {
				strVal := fmt.Sprintf("%v", val)
				switch strings.ToLower(key) {
				case "version":
					svc.Version = strVal
				case "odataversion":
					svc.ODataVersion = strVal
				case "metadataurl":
					svc.MetadataUrl = strVal
				case "timeout":
					svc.TimeoutExpression = strVal
				case "proxytype":
					svc.ProxyType = strVal
				case "description":
					svc.Description = strVal
				case "serviceurl":
					if err := validateServiceURL(strVal); err != nil {
						return err
					}
					if svc.HttpConfiguration == nil {
						svc.HttpConfiguration = &model.HttpConfiguration{}
					}
					svc.HttpConfiguration.OverrideLocation = true
					svc.HttpConfiguration.CustomLocation = strVal
				case "useauthentication":
					if svc.HttpConfiguration == nil {
						svc.HttpConfiguration = &model.HttpConfiguration{}
					}
					svc.HttpConfiguration.UseAuthentication = strings.EqualFold(strVal, "true") || strings.EqualFold(strVal, "yes")
				case "httpusername":
					if svc.HttpConfiguration == nil {
						svc.HttpConfiguration = &model.HttpConfiguration{}
					}
					svc.HttpConfiguration.Username = strVal
				case "httppassword":
					if svc.HttpConfiguration == nil {
						svc.HttpConfiguration = &model.HttpConfiguration{}
					}
					svc.HttpConfiguration.Password = strVal
				case "clientcertificate":
					if svc.HttpConfiguration == nil {
						svc.HttpConfiguration = &model.HttpConfiguration{}
					}
					svc.HttpConfiguration.ClientCertificate = strVal
				case "configurationmicroflow":
					svc.ConfigurationMicroflow = extractMicroflowRef(strVal)
				case "headersmicroflow":
					svc.HeadersMicroflow = extractMicroflowRef(strVal)
				case "errorhandlingmicroflow":
					svc.ErrorHandlingMicroflow = extractMicroflowRef(strVal)
				case "proxyhost":
					svc.ProxyHost = strVal
				case "proxyport":
					svc.ProxyPort = strVal
				case "proxyusername":
					svc.ProxyUsername = strVal
				case "proxypassword":
					svc.ProxyPassword = strVal
				default:
					return mdlerrors.NewUnsupported(fmt.Sprintf("unknown OData client property: %s", key))
				}
			}
			if err := ctx.Backend.UpdateConsumedODataService(svc); err != nil {
				return mdlerrors.NewBackend("alter OData client", err)
			}
			invalidateHierarchy(ctx)
			fmt.Fprintf(ctx.Output, "Altered OData client: %s.%s\n", modName, svc.Name)
			return nil
		}
	}

	return mdlerrors.NewNotFoundMsg("OData client", fmt.Sprint(stmt.Name), fmt.Sprintf("OData client not found: %s", stmt.Name))
}

// dropODataClient handles DROP ODATA CLIENT command.
func dropODataClient(ctx *ExecContext, stmt *ast.DropODataClientStmt) error {

	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	services, err := ctx.Backend.ListConsumedODataServices()
	if err != nil {
		return mdlerrors.NewBackend("list consumed OData services", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)
		if strings.EqualFold(modName, stmt.Name.Module) && strings.EqualFold(svc.Name, stmt.Name.Name) {
			// Cascade: delete external entities belonging to this service so that
			// DeleteEntity can clean up any associations referencing them.
			serviceRef := modName + "." + svc.Name
			module, findErr := findModule(ctx, stmt.Name.Module)
			if findErr != nil {
				return findErr
			}
			dm, dmErr := ctx.Backend.GetDomainModel(module.ID)
			if dmErr != nil {
				return mdlerrors.NewBackend("get domain model for cascade", dmErr)
			}
			var externalEntityIDs []model.ID
			for _, entity := range dm.Entities {
				if strings.EqualFold(entity.RemoteServiceName, serviceRef) {
					externalEntityIDs = append(externalEntityIDs, entity.ID)
				}
			}
			for _, entityID := range externalEntityIDs {
				if err := ctx.Backend.DeleteEntity(dm.ID, entityID); err != nil {
					return mdlerrors.NewBackend("cascade delete external entity", err)
				}
			}

			if err := ctx.Backend.DeleteConsumedODataService(svc.ID); err != nil {
				return mdlerrors.NewBackend("drop OData client", err)
			}
			invalidateHierarchy(ctx)
			fmt.Fprintf(ctx.Output, "Dropped OData client: %s.%s\n", modName, svc.Name)
			return nil
		}
	}

	return mdlerrors.NewNotFoundMsg("OData client", fmt.Sprint(stmt.Name), fmt.Sprintf("OData client not found: %s", stmt.Name))
}

// createODataService handles CREATE ODATA SERVICE command.
func createODataService(ctx *ExecContext, stmt *ast.CreateODataServiceStmt) error {

	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	if stmt.Name.Module == "" {
		return mdlerrors.NewValidation("module name required: use create odata service Module.Name (...)")
	}

	module, err := findModule(ctx, stmt.Name.Module)
	if err != nil {
		return err
	}

	// Check if service already exists
	services, err := ctx.Backend.ListPublishedODataServices()
	if err == nil {
		h, _ := getHierarchy(ctx)
		for _, svc := range services {
			modID := h.FindModuleID(svc.ContainerID)
			modName := h.GetModuleName(modID)
			if strings.EqualFold(modName, stmt.Name.Module) && strings.EqualFold(svc.Name, stmt.Name.Name) {
				if stmt.CreateOrModify {
					svc.Documentation = stmt.Documentation
					if stmt.Path != "" {
						svc.Path = stmt.Path
					}
					if stmt.Version != "" {
						svc.Version = stmt.Version
					}
					if stmt.ODataVersion != "" {
						svc.ODataVersion = stmt.ODataVersion
					}
					if stmt.Namespace != "" {
						svc.Namespace = stmt.Namespace
					}
					if stmt.ServiceName != "" {
						svc.ServiceName = stmt.ServiceName
					}
					if stmt.Summary != "" {
						svc.Summary = stmt.Summary
					}
					if stmt.Description != "" {
						svc.Description = stmt.Description
					}
					svc.PublishAssociations = stmt.PublishAssociations
					if len(stmt.AuthenticationTypes) > 0 {
						svc.AuthenticationTypes = stmt.AuthenticationTypes
					}
					if err := ctx.Backend.UpdatePublishedODataService(svc); err != nil {
						return mdlerrors.NewBackend("update OData service", err)
					}
					invalidateHierarchy(ctx)
					fmt.Fprintf(ctx.Output, "Modified OData service: %s.%s\n", modName, svc.Name)
					return nil
				}
				return mdlerrors.NewAlreadyExistsMsg("OData service", modName+"."+svc.Name, fmt.Sprintf("OData service already exists: %s.%s (use create or modify to update)", modName, svc.Name))
			}
		}
	}

	// Resolve folder if specified
	containerID := module.ID
	if stmt.Folder != "" {
		folderID, err := resolveFolder(ctx, module.ID, stmt.Folder)
		if err != nil {
			return mdlerrors.NewBackend(fmt.Sprintf("resolve folder %s", stmt.Folder), err)
		}
		containerID = folderID
	}

	newSvc := &model.PublishedODataService{
		ContainerID:         containerID,
		Name:                stmt.Name.Name,
		Documentation:       stmt.Documentation,
		Path:                stmt.Path,
		Version:             stmt.Version,
		ODataVersion:        stmt.ODataVersion,
		Namespace:           stmt.Namespace,
		ServiceName:         stmt.ServiceName,
		Summary:             stmt.Summary,
		Description:         stmt.Description,
		PublishAssociations: stmt.PublishAssociations,
		AuthenticationTypes: stmt.AuthenticationTypes,
	}

	// Map AST entity definitions to model entity types and entity sets.
	// Pass ctx so the executor can resolve exposed members against the
	// entity's actual attributes and (for navigation properties) the
	// module's associations.
	for _, entityDef := range stmt.Entities {
		entityType, entitySet := astEntityDefToModel(ctx, entityDef)
		newSvc.EntityTypes = append(newSvc.EntityTypes, entityType)
		newSvc.EntitySets = append(newSvc.EntitySets, entitySet)
	}

	if err := ctx.Backend.CreatePublishedODataService(newSvc); err != nil {
		return mdlerrors.NewBackend("create OData service", err)
	}
	invalidateHierarchy(ctx)
	fmt.Fprintf(ctx.Output, "Created OData service: %s.%s\n", stmt.Name.Module, stmt.Name.Name)
	return nil
}

// alterODataService handles ALTER ODATA SERVICE command.
func alterODataService(ctx *ExecContext, stmt *ast.AlterODataServiceStmt) error {

	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	services, err := ctx.Backend.ListPublishedODataServices()
	if err != nil {
		return mdlerrors.NewBackend("list published OData services", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)
		if strings.EqualFold(modName, stmt.Name.Module) && strings.EqualFold(svc.Name, stmt.Name.Name) {
			for key, val := range stmt.Changes {
				strVal := fmt.Sprintf("%v", val)
				switch strings.ToLower(key) {
				case "path":
					svc.Path = strVal
				case "version":
					svc.Version = strVal
				case "odataversion":
					svc.ODataVersion = strVal
				case "namespace":
					svc.Namespace = strVal
				case "servicename":
					svc.ServiceName = strVal
				case "summary":
					svc.Summary = strVal
				case "description":
					svc.Description = strVal
				case "publishassociations":
					svc.PublishAssociations = strings.EqualFold(strVal, "true") || strings.EqualFold(strVal, "yes")
				default:
					return mdlerrors.NewUnsupported(fmt.Sprintf("unknown OData service property: %s", key))
				}
			}
			if err := ctx.Backend.UpdatePublishedODataService(svc); err != nil {
				return mdlerrors.NewBackend("alter OData service", err)
			}
			invalidateHierarchy(ctx)
			fmt.Fprintf(ctx.Output, "Altered OData service: %s.%s\n", modName, svc.Name)
			return nil
		}
	}

	return mdlerrors.NewNotFoundMsg("OData service", fmt.Sprint(stmt.Name), fmt.Sprintf("OData service not found: %s", stmt.Name))
}

// dropODataService handles DROP ODATA SERVICE command.
func dropODataService(ctx *ExecContext, stmt *ast.DropODataServiceStmt) error {

	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	services, err := ctx.Backend.ListPublishedODataServices()
	if err != nil {
		return mdlerrors.NewBackend("list published OData services", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)
		if strings.EqualFold(modName, stmt.Name.Module) && strings.EqualFold(svc.Name, stmt.Name.Name) {
			if err := ctx.Backend.DeletePublishedODataService(svc.ID); err != nil {
				return mdlerrors.NewBackend("drop OData service", err)
			}
			invalidateHierarchy(ctx)
			fmt.Fprintf(ctx.Output, "Dropped OData service: %s.%s\n", modName, svc.Name)
			return nil
		}
	}

	return mdlerrors.NewNotFoundMsg("OData service", fmt.Sprint(stmt.Name), fmt.Sprintf("OData service not found: %s", stmt.Name))
}

// validateServiceURL returns an error if url is not a constant reference (@Module.Name).
// CE6825: Studio Pro requires the Service URL to be a constant, not a string literal.
func validateServiceURL(url string) error {
	if !strings.HasPrefix(url, "@") {
		return mdlerrors.NewValidation("ServiceUrl must be a constant reference (e.g., @Module.ServiceUrlConstant) — Studio Pro CE6825: 'Service url' must be a constant")
	}
	return nil
}

// validateMetadataURL returns an error if the MetadataUrl is obviously malformed.
// A valid value must be an http/https URL, a file:// URL, or a path that contains
// at least one path separator or dot (indicating an extension or subdirectory).
// Bare words like "not-a-url" are rejected to prevent silently creating broken OData client configurations.
func validateMetadataURL(rawURL string) error {
	if rawURL == "" {
		return nil
	}
	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		return nil
	}
	if strings.HasPrefix(rawURL, "file://") {
		return nil
	}
	if strings.ContainsAny(rawURL, "/\\.") {
		return nil
	}
	return mdlerrors.NewValidationf("MetadataUrl %q is not a valid URL or file path: use an http/https URL, a file:// URL, or a relative path (e.g. './service/$metadata.xml')", rawURL)
}

// validateODataClientExists returns an error if no consumed OData service matching
// the given qualified name exists in the project.
func validateODataClientExists(ctx *ExecContext, ref ast.QualifiedName) error {
	services, err := ctx.Backend.ListConsumedODataServices()
	if err != nil {
		return mdlerrors.NewBackend("list consumed OData services", err)
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}
	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)
		if strings.EqualFold(modName, ref.Module) && strings.EqualFold(svc.Name, ref.Name) {
			return nil
		}
	}
	return mdlerrors.NewNotFoundMsg("odata client", ref.String(), fmt.Sprintf("odata client not found: %s", ref))
}

// formatExprValue formats a Mendix expression value for MDL output.
// If the value is already a quoted string literal (starts/ends with '), it's output as-is.
// Otherwise, it's wrapped in single quotes for round-trip compatibility.
func formatExprValue(val string) string {
	if len(val) >= 2 && val[0] == '\'' && val[len(val)-1] == '\'' {
		return val // Already a quoted Mendix expression string literal
	}
	// Wrap in quotes, escaping internal single quotes
	return "'" + strings.ReplaceAll(val, "'", "''") + "'"
}

// extractMicroflowRef strips a leading "microflow " keyword (any case) from a
// microflow reference string. The visitor emits uppercase `"MICROFLOW " + qn`
// for `microflow Module.Name` property values (see visitor_odata.go); both
// that form and a bare `Module.Name` are accepted. Issue #573.
func extractMicroflowRef(ref string) string {
	const prefix = "microflow "
	if len(ref) >= len(prefix) && strings.EqualFold(ref[:len(prefix)], prefix) {
		return ref[len(prefix):]
	}
	return ref
}

// assocMembership records both endpoints of an association by qualified
// entity name, so callers can resolve "what's on the other side of
// AssocName from Entity X".
type assocMembership struct {
	ParentQN string                      // FROM entity (CLAUDE.md: ParentPointer holds the FROM/FK side)
	ChildQN  string                      // TO entity (ChildPointer holds the TO/referenced side)
	Type     domainmodel.AssociationType // Reference (to-one) vs ReferenceSet (to-many)
}

// lookupEntityMembers returns (set of attribute names, map of association
// name -> assocMembership) for the given entity. Both lookups are
// best-effort — if the backend or domain model can't be resolved we
// return empty collections rather than failing the whole publish.
// mendixAttrTypeToEdm maps a Mendix attribute type to the OData EDM type Studio
// Pro publishes it as (the PublishedAttribute.EdmType field). Inverse of
// edmToDomainModelAttrType. String/Decimal/Boolean/DateTimeOffset verified
// against Studio Pro output; the rest follow the standard OData mapping.
func mendixAttrTypeToEdm(t domainmodel.AttributeType) string {
	if t == nil {
		return ""
	}
	switch t.GetTypeName() {
	case "String", "HashedString":
		return "Edm.String"
	case "Integer":
		return "Edm.Int32"
	case "Long", "AutoNumber":
		return "Edm.Int64"
	case "Decimal":
		return "Edm.Decimal"
	case "Boolean":
		return "Edm.Boolean"
	case "DateTime", "Date":
		return "Edm.DateTimeOffset"
	case "Binary":
		return "Edm.Binary"
	case "Enumeration":
		// Enums exposed as string (EnumerationAsString path). A non-string enum
		// exposure would use the enum's own type — not yet modelled.
		return "Edm.String"
	default:
		return "Edm.String"
	}
}

func lookupEntityMembers(ctx *ExecContext, entityQN ast.QualifiedName) (map[string]string, map[string]*assocMembership) {
	attrs := make(map[string]string) // attr name -> OData EDM type (for the published EdmType)
	assocs := make(map[string]*assocMembership)
	if ctx == nil || ctx.Backend == nil {
		return attrs, assocs
	}
	module, err := findModule(ctx, entityQN.Module)
	if err != nil {
		return attrs, assocs
	}
	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return attrs, assocs
	}
	// Build entity-id -> qualified-name map so we can resolve association
	// Parent/Child IDs back to qualified names.
	entityIDToQN := make(map[model.ID]string)
	var thisEntity *domainmodel.Entity
	for _, e := range dm.Entities {
		entityIDToQN[e.ID] = entityQN.Module + "." + e.Name
		if e.Name == entityQN.Name {
			thisEntity = e
		}
	}
	if thisEntity != nil {
		for _, a := range thisEntity.Attributes {
			attrs[a.Name] = mendixAttrTypeToEdm(a.Type)
		}
	}
	for _, a := range dm.Associations {
		parentQN := entityIDToQN[a.ParentID]
		childQN := entityIDToQN[a.ChildID]
		if parentQN == "" || childQN == "" {
			continue
		}
		// Only include associations where this entity is on one side.
		if parentQN != entityQN.String() && childQN != entityQN.String() {
			continue
		}
		assocs[a.Name] = &assocMembership{ParentQN: parentQN, ChildQN: childQN, Type: a.Type}
	}
	return attrs, assocs
}

// astEntityDefToModel converts an AST PublishedEntityDef to model PublishedEntityType
// and PublishedEntitySet. Each PUBLISH ENTITY block maps to both a type (schema) and
// a set (runtime endpoint with CRUD modes).
//
// Member kinds (attribute vs association) are auto-detected against the
// entity's attributes and the module's associations: if a member's name
// matches an attribute on the entity, it's an attribute; otherwise we
// look for an association in the same module that involves this entity
// and emit it as a PublishedAssociationEnd. The user writes the bare
// association name (e.g. `Order_Customer as 'Orders'`) and the executor
// fills in the target entity and qualified names from the domain model.
func astEntityDefToModel(ctx *ExecContext, def *ast.PublishedEntityDef) (*model.PublishedEntityType, *model.PublishedEntitySet) {
	// EntitySet ExposedName comes from the user's `AS 'X'` (typically plural,
	// e.g. 'Customers'). EntityType ExposedName must differ — Studio Pro
	// convention is the singular form (the entity's local name, e.g.
	// 'Customer'). Sharing one name for both causes Mendix to fail
	// resolving the second entity's key in a multi-entity service (CE6585).
	entitySetExposedName := def.ExposedName
	if entitySetExposedName == "" {
		entitySetExposedName = def.Entity.Name
	}
	entityTypeExposedName := def.Entity.Name

	entityType := &model.PublishedEntityType{
		Entity:      def.Entity.String(),
		ExposedName: entityTypeExposedName,
	}

	// Look up the entity's attributes and the module's associations so we
	// can distinguish attribute exposures from association navigation
	// properties. Failures here downgrade to "treat everything as
	// attribute" — Mendix will then emit a missing-member error which is
	// the right user-facing signal.
	entityAttrs, moduleAssocs := lookupEntityMembers(ctx, def.Entity)

	for _, m := range def.Members {
		member := &model.PublishedMember{
			Name:        m.Name,
			ExposedName: m.ExposedName,
			Filterable:  m.Filterable,
			Sortable:    m.Sortable,
			IsPartOfKey: m.IsPartOfKey,
		}
		if member.ExposedName == "" {
			member.ExposedName = member.Name
		}
		// Auto-detect kind: attribute first, association as fallback.
		if edmType, ok := entityAttrs[m.Name]; ok {
			member.Kind = "attribute"
			member.EdmType = edmType
		} else if assoc := moduleAssocs[m.Name]; assoc != nil {
			member.Kind = "association"
			member.ExposedAssociationName = m.Name
			// Target entity = the OTHER side of the association. Multiplicity
			// (IsMany) of the exposed navigation: a ReferenceSet is to-many
			// from either side; a Reference is to-many only when exposed from
			// the TO/Child side (Customer→Orders), to-one from the FROM/Parent
			// side (Order→Customer). Without IsMany, mx check reports CE5022.
			if assoc.ParentQN == def.Entity.String() {
				member.AssociationTargetEntity = assoc.ChildQN
				member.IsMany = assoc.Type == domainmodel.AssociationTypeReferenceSet
			} else {
				member.AssociationTargetEntity = assoc.ParentQN
				member.IsMany = true // reverse of a Reference is to-many; ReferenceSet is too
			}
			// AssociationEnd has no Filterable/Sortable/IsPartOfKey
			// concept — clear them so the writer's bson.M doesn't
			// leak stale attribute-shaped fields.
			member.Filterable = false
			member.Sortable = false
			member.IsPartOfKey = false
		} else {
			member.Kind = "attribute"
		}
		entityType.Members = append(entityType.Members, member)
	}

	entitySet := &model.PublishedEntitySet{
		ExposedName:    entitySetExposedName,
		EntityTypeName: def.Entity.String(),
		ReadMode:       def.ReadMode,
		InsertMode:     def.InsertMode,
		UpdateMode:     def.UpdateMode,
		DeleteMode:     def.DeleteMode,
		UsePaging:      def.UsePaging,
		PageSize:       def.PageSize,
	}

	return entityType, entitySet
}

// fetchODataMetadata downloads or reads the $metadata document.
// Supports:
//   - https://... or http://... (HTTP fetch)
//   - file:///abs/path (local absolute path from normalized URL)
//
// Returns the metadata XML and its SHA-256 hash, or empty strings if the fetch fails.
// Note: metadataUrl is expected to be already normalized by NormalizeURL() in createODataClient,
// so all relative paths have been converted to absolute file:// URLs.
func fetchODataMetadata(metadataUrl string) (metadata string, hash string, err error) {
	if metadataUrl == "" {
		return "", "", nil
	}

	var body []byte

	// At this point, metadataUrl is already normalized by NormalizeURL() in createODataClient:
	// - Relative paths have been converted to absolute file:// URLs
	// - HTTP(S) URLs are unchanged
	// So we only need to distinguish file:// vs HTTP(S)

	filePath := pathutil.PathFromURL(metadataUrl)
	if filePath != "" {
		// Local file - read directly (path is already absolute)
		body, err = os.ReadFile(filePath)
		if err != nil {
			return "", "", mdlerrors.NewBackend(fmt.Sprintf("read local metadata file %s", filePath), err)
		}
	} else {
		// HTTP(S) fetch
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(metadataUrl)
		if err != nil {
			return "", "", mdlerrors.NewBackend(fmt.Sprintf("fetch $metadata from %s", metadataUrl), err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return "", "", mdlerrors.NewValidationf("$metadata fetch returned HTTP %d from %s", resp.StatusCode, metadataUrl)
		}

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return "", "", mdlerrors.NewBackend("read $metadata response", err)
		}
	}

	// Hash calculation (same for both HTTP and local file)
	metadata = string(body)
	h := sha256.Sum256(body)
	hash = fmt.Sprintf("%x", h)
	return metadata, hash, nil
}

// Executor wrappers for unmigrated callers.
