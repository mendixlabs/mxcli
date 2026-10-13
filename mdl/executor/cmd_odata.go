// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/internal/pathutil"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/dmlayout"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
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

	// `create or modify`: the rewrite carries what describe cannot print (the icon,
	// UseQuerySegment, the catalog and proxy keys, see carryStoredConsumedODataService),
	// proven on ako/TestApp's clients by the round-trip harness (#743).
	// The folder is a clause after the name (R9); `Folder:` is its alias.
	folder := ""
	if folderPath != "" {
		folder = " folder " + mdlQuote(ctx, folderPath)
	}
	fmt.Fprintf(ctx.Output, "create or modify consumed odata service %s.%s%s (\n", moduleName, svc.Name, folder)

	var props []string
	if svc.Version != "" {
		props = append(props, fmt.Sprintf("  Version: %s", mdlQuote(ctx, svc.Version)))
	}
	if svc.ODataVersion != "" {
		props = append(props, fmt.Sprintf("  ODataVersion: %s", svc.ODataVersion))
	}
	if svc.MetadataUrl != "" {
		props = append(props, fmt.Sprintf("  MetadataUrl: %s", mdlQuote(ctx, svc.MetadataUrl)))
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
			// The constant's bare name, as ProxyHost prints (serviceURLConstant).
			// A stored value that is not `@Module.Name` keeps the quoted form.
			if ref := strings.TrimPrefix(cfg.CustomLocation, "@"); strings.HasPrefix(cfg.CustomLocation, "@") && qualifiedConstantName.MatchString(ref) {
				props = append(props, fmt.Sprintf("  ServiceUrl: %s", ref))
			} else {
				props = append(props, fmt.Sprintf("  ServiceUrl: %s", formatExprValue(ctx, cfg.CustomLocation)))
			}
		}
		// HttpUsername / HttpPassword / ClientCertificate and header values are
		// Mendix expressions, and MDL writes an expression as-is: the stored
		// `'abc'` prints as `'abc'`, the string (PROPOSAL_first_class_expressions.md
		// §6.4). ServiceUrl above still takes the older quoted form.
		if cfg.UseAuthentication {
			props = append(props, "  UseAuthentication: Yes")
			if cfg.Username != "" {
				props = append(props, fmt.Sprintf("  HttpUsername: %s", cfg.Username))
			}
			if cfg.Password != "" {
				props = append(props, fmt.Sprintf("  HttpPassword: %s", cfg.Password))
			}
		}
		if cfg.ClientCertificate != "" {
			props = append(props, fmt.Sprintf("  ClientCertificate: %s", cfg.ClientCertificate))
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
			fmt.Fprintf(ctx.Output, "  %s: %s%s\n", mdlQuote(ctx, h.Key), h.Value, comma)
		}
		fmt.Fprintln(ctx.Output, ");")
	} else {
		fmt.Fprintln(ctx.Output, ");")
	}

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

	// `create or modify`: the rewrite carries what this output cannot print
	// (ExportLevel, PageSize without paging, entity-set order, CanBeEmpty), and
	// TestTestAppRoundTrip holds it to both round-trip laws on the Studio
	// Pro-authored services of ako/TestApp (#743).
	// The folder is a clause after the name (R9); `Folder:` is its alias.
	folder := ""
	if folderPath != "" {
		folder = " folder " + mdlQuote(ctx, folderPath)
	}
	fmt.Fprintf(ctx.Output, "create or modify published odata service %s.%s%s (\n", moduleName, svc.Name, folder)

	var props []string
	if svc.Path != "" {
		props = append(props, "  Path: "+mdlQuoted(svc.Path))
	}
	if svc.Version != "" {
		props = append(props, "  Version: "+mdlQuoted(svc.Version))
	}
	if svc.ODataVersion != "" {
		props = append(props, fmt.Sprintf("  ODataVersion: %s", svc.ODataVersion))
	}
	if svc.Namespace != "" {
		props = append(props, "  Namespace: "+mdlQuoted(svc.Namespace))
	}
	if svc.ServiceName != "" {
		props = append(props, "  ServiceName: "+mdlQuoted(svc.ServiceName))
	}
	if svc.Summary != "" {
		props = append(props, "  Summary: "+mdlQuoted(svc.Summary))
	}
	if svc.PublishAssociations {
		props = append(props, "  PublishAssociations: Yes")
	}
	// Only emitted when on: false is what every service was before the property
	// existed, so printing it on each of them would be noise in every describe.
	if svc.SupportsGraphQL {
		props = append(props, "  SupportsGraphQL: Yes")
	}
	// Authentication is a property (R9; the trailing clause is MDL-DEPR139),
	// with the custom-authentication microflow inside it: emitted as a comment
	// the output looked complete but replayed into a service Mendix rejects with
	// CE0333 (mxcli-formula1 §40).
	authProp, authNote, hasAuth := odataAuthenticationProperty(svc)
	if hasAuth {
		props = append(props, "  "+authProp)
	}
	fmt.Fprintln(ctx.Output, strings.Join(props, ",\n"))

	// The statement ends with `;` after the property list or the entity block
	// (ADR-0010 R11).
	hasBlock := len(svc.EntityTypes) > 0 || len(svc.EntitySets) > 0 || len(svc.Microflows) > 0
	if hasBlock {
		fmt.Fprintln(ctx.Output, ")")
	} else {
		fmt.Fprintln(ctx.Output, ");")
	}
	if authNote != "" {
		fmt.Fprintf(ctx.Output, "-- Authentication is stored as %s, which MDL cannot state; executing this keeps it.\n", authNote)
	}

	// Published entities block
	if hasBlock {
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

			// PUBLISH ENTITY line with modes.
			//
			// `AS '<name>'` is the ENTITY SET's exposed name — that is what the
			// served $metadata calls the set, and what re-executing this output
			// must reproduce. Printing the entity TYPE's exposed name here
			// silently renamed the set on a describe -> exec round trip
			// (mxcli-formula1 findings #10.5).
			exposedName := et.ExposedName
			if es != nil && es.ExposedName != "" {
				exposedName = es.ExposedName
			}
			fmt.Fprintf(ctx.Output, "  publish entity %s as %s", et.Entity, mdlQuoted(exposedName))
			if es != nil {
				var modeProps []string
				if es.ReadMode != "" {
					modeProps = append(modeProps, fmt.Sprintf("ReadMode: %s", odataModeToMDL(es.ReadMode)))
				}
				if es.InsertMode != "" {
					modeProps = append(modeProps, fmt.Sprintf("InsertMode: %s", odataModeToMDL(es.InsertMode)))
				}
				if es.UpdateMode != "" {
					modeProps = append(modeProps, fmt.Sprintf("UpdateMode: %s", odataModeToMDL(es.UpdateMode)))
				}
				if es.DeleteMode != "" {
					modeProps = append(modeProps, fmt.Sprintf("DeleteMode: %s", odataModeToMDL(es.DeleteMode)))
				}
				if es.UsePaging {
					modeProps = append(modeProps, "UsePaging: Yes")
					modeProps = append(modeProps, fmt.Sprintf("PageSize: %d", es.PageSize))
				}
				// Only a turned-off query option is worth printing; true is the
				// default and would be noise on every resource.
				if es.Countable != nil && !*es.Countable {
					modeProps = append(modeProps, "Countable: No")
				}
				if es.SkipSupported != nil && !*es.SkipSupported {
					modeProps = append(modeProps, "SkipSupported: No")
				}
				if es.TopSupported != nil && !*es.TopSupported {
					modeProps = append(modeProps, "TopSupported: No")
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
						// KEY is the spelling the syntax help documents;
						// IsPartOfKey parses too, but only one belongs in
						// output meant to be re-executed.
						modifiers = append(modifiers, "KEY")
					}

					// The member is stored fully qualified
					// (Module.Entity.Member), while `expose (...)` takes a bare
					// member name — so emitting the stored form produced MDL
					// that does not parse (mxcli-formula1 findings #10.5).
					line := fmt.Sprintf("    %s as %s", bareMemberName(m.Name), mdlQuoted(m.ExposedName))
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

		// OData actions. Emitted as part of the block, not as a comment beside
		// it: a comment reads as complete and replays into a service without the
		// action (mxcli-formula1 §47.1).
		for _, pm := range svc.Microflows {
			printPublishedMicroflowMDL(ctx.Output, pm)
		}

		fmt.Fprintln(ctx.Output, "};")
	}

	// Output GRANT statements for allowed module roles
	if len(svc.AllowedModuleRoles) > 0 {
		fmt.Fprintln(ctx.Output)
		fmt.Fprintf(ctx.Output, "grant access on published odata service %s.%s to %s;\n",
			moduleName, svc.Name, strings.Join(svc.AllowedModuleRoles, ", "))
	}

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

	// `create or modify`: the rewrite keeps each stored attribute's identity and
	// OData mapping by name (#743), proven on ako/TestApp by the round-trip harness.
	fmt.Fprintf(ctx.Output, "create or modify external entity %s.%s\n", moduleName, entity.Name)
	fmt.Fprintf(ctx.Output, "from consumed odata service %s\n", entity.RemoteServiceName)
	fmt.Fprintln(ctx.Output, "(")

	var props []string
	if entity.RemoteEntitySet != "" {
		props = append(props, "  EntitySet: "+mdlQuoted(entity.RemoteEntitySet))
	}
	if entity.RemoteEntityName != "" {
		props = append(props, "  RemoteName: "+mdlQuoted(entity.RemoteEntityName))
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

	// Output attributes. Without them the statement ends at the property list,
	// and still ends with `;` (ADR-0010 R11).
	if len(entity.Attributes) == 0 {
		fmt.Fprintln(ctx.Output, ");")
		return nil
	}
	fmt.Fprintln(ctx.Output, ")")
	fmt.Fprintln(ctx.Output, "(")
	for i, attr := range entity.Attributes {
		// formatAttributeType, not GetTypeName: the bare `String` executes as
		// unlimited and rewrote a stored String(36) (#743).
		typeName := "Unknown"
		if attr.Type != nil {
			typeName = formatAttributeType(attr.Type) + localizeClause(attr.Type)
		}
		comma := ","
		if i == len(entity.Attributes)-1 {
			comma = ""
		}
		fmt.Fprintf(ctx.Output, "  %s: %s%s\n", attr.Name, typeName, comma)
	}
	fmt.Fprintln(ctx.Output, ");")
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
		return mdlerrors.NewValidation("module name required: use create external entity Module.Name from consumed odata service ...")
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
			// The rebuilt attributes keep each stored attribute's identity and
			// what the statement cannot spell (the OData mapping; LocalizeDate when
			// unstated, #1373),
			// by name; a fresh $ID and a plain StoredValue made the rewrite an
			// unmapped attribute (#743).
			for _, a := range attrs {
				for _, old := range existingEntity.Attributes {
					if old.Name == a.Name {
						a.ID = old.ID
						break
					}
				}
			}
			if err := checkRemoteTypes(ctx, s.Name.String(), existingEntity, attrs); err != nil {
				return err
			}
			carryStoredAttributeState(existingEntity, attrs, statedLocalize(s.Attributes))
			existingEntity.Attributes = attrs
		}
		// A rewrite that carried no doc comment keeps the stored one; an
		// explicitly empty `/** */` clears it (#1018). The `!= ""` form this
		// replaces preserved but could not clear.
		existingEntity.Documentation = carriedDocumentation(
			s.DocumentationSet, s.Documentation, existingEntity.Documentation)
		if err := ctx.Backend.UpdateEntity(dm.ID, existingEntity); err != nil {
			return mdlerrors.NewBackend("update external entity", err)
		}
		ctx.ReportMutation("Modified", "external entity: %s.%s", s.Name.Module, s.Name.Name)
		return nil
	}

	// Initial create: EntitySet is required (no prior value to preserve).
	if s.EntitySet == nil || *s.EntitySet == "" {
		return mdlerrors.NewValidation("EntitySet property is required when creating an external entity")
	}

	// No @Position exists for an external entity: take the next grid slot, as
	// CREATE ENTITY does.
	location := dmlayout.GridSlot(len(dm.Entities))

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
		return mdlerrors.NewValidation("module name required: use create consumed odata service Module.Name (...)")
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
					// svc is the STORED service being mutated, so its own value is
					// what a silent rewrite preserves (#1018).
					svc.Documentation = carriedDocumentation(
						stmt.DocumentationSet, stmt.Documentation, svc.Documentation)
					if stmt.Version != "" {
						svc.Version = stmt.Version
					}
					if stmt.ODataVersion != "" {
						svc.ODataVersion = stmt.ODataVersion
					}
					if stmt.MetadataUrl != "" {
						normalized, nerr := normalizeMetadataURL(ctx, stmt.MetadataUrl)
						if nerr != nil {
							return fmt.Errorf("failed to normalize MetadataUrl: %w", nerr)
						}
						svc.MetadataUrl = normalized
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
						svc.ProxyHost = extractConstantRef(stmt.ProxyHost)
					}
					if stmt.ProxyPort != "" {
						svc.ProxyPort = extractConstantRef(stmt.ProxyPort)
					}
					if stmt.ProxyUsername != "" {
						svc.ProxyUsername = extractConstantRef(stmt.ProxyUsername)
					}
					if stmt.ProxyPassword != "" {
						svc.ProxyPassword = extractConstantRef(stmt.ProxyPassword)
					}
					// Update HTTP configuration
					if stmt.ServiceUrl != "" || stmt.UseAuthentication || stmt.HttpUsername != "" ||
						stmt.HttpPassword != "" || stmt.ClientCertificate != "" || len(stmt.Headers) > 0 {
						if svc.HttpConfiguration == nil {
							svc.HttpConfiguration = &model.HttpConfiguration{}
						}
						if stmt.ServiceUrl != "" {
							location, err := serviceURLConstant(stmt.ServiceUrl)
							if err != nil {
								return err
							}
							svc.HttpConfiguration.OverrideLocation = true
							svc.HttpConfiguration.CustomLocation = location
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
					// Re-fetch $metadata. The cached contract is a snapshot taken
					// when the client was created; leaving it alone here made
					// CREATE OR MODIFY converge every property except the one that
					// changes most often. A service that gains entity sets then
					// reports "Unchanged OData client", and the CREATE EXTERNAL
					// ENTITIES that follows imports the old shape silently
					// (mxcli-formula1: F1LiveNowApi gained Trace and Stints, and
					// the only way out was DROP + recreate, which invalidates the
					// client ID the external entities point at).
					refreshed := refreshCachedMetadata(ctx, svc, stmt)

					if err := ctx.Backend.UpdateConsumedODataService(svc); err != nil {
						return mdlerrors.NewBackend("update OData client", err)
					}
					target, terr := resolveRequestedFolder(ctx, module.ID, stmt.Folder)
					if terr != nil {
						return terr
					}
					if _, err := applyDocumentFolder(ctx, svc.ID, svc.ContainerID, target); err != nil {
						return err
					}
					invalidateHierarchy(ctx)
					ctx.ReportMutation("Modified", "OData client: %s.%s", modName, svc.Name)
					if refreshed {
						if sum := contractSummary(svc.Metadata); sum != "" {
							fmt.Fprintf(ctx.Output, "  Refreshed $metadata: %s\n", sum)
						}
					}
					return nil
				}
				// The rewrite carries what describe cannot print (#743), so it is
				// the advice again, as for every other document type.
				return mdlerrors.NewAlreadyExistsMsg("OData client", modName+"."+svc.Name, fmt.Sprintf(
					"OData client already exists: %s.%s (use create or modify to update it, or "+
						"'alter consumed odata service %s.%s set ...' to change one property)", modName, svc.Name, modName, svc.Name))
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
		ProxyHost:              extractConstantRef(stmt.ProxyHost),
		ProxyPort:              extractConstantRef(stmt.ProxyPort),
		ProxyUsername:          extractConstantRef(stmt.ProxyUsername),
		ProxyPassword:          extractConstantRef(stmt.ProxyPassword),
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
			location, err := serviceURLConstant(stmt.ServiceUrl)
			if err != nil {
				return fmt.Errorf(`ServiceUrl must name a constant (e.g., Module.ApiLocation) — Studio Pro CE6825.
Create a constant first:
  CREATE CONSTANT Module.ApiLocation ( Type: String, DefaultValue: 'https://api.example.com/' );
Then reference it:
  ServiceUrl: Module.ApiLocation
Got: %s`, stmt.ServiceUrl)
			}
			cfg.OverrideLocation = true
			cfg.CustomLocation = location
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
		// Normalize MetadataUrl: convert relative paths to absolute file:// URLs
		normalizedUrl, err := normalizeMetadataURL(ctx, newSvc.MetadataUrl)
		if err != nil {
			return fmt.Errorf("failed to normalize MetadataUrl: %w", err)
		}
		newSvc.MetadataUrl = normalizedUrl

		auth := metadataAuthFromStmt(ctx, stmt)
		metadata, hash, err := fetchODataMetadata(normalizedUrl, auth)
		if err != nil {
			fmt.Fprintf(ctx.Output, "Warning: could not fetch $metadata: %v\n", err)
			for _, hint := range auth.hints() {
				fmt.Fprintf(ctx.Output, "  %s\n", hint)
			}
			fmt.Fprintf(ctx.Output, "  The client is created with no cached entity types, so a following\n")
			fmt.Fprintf(ctx.Output, "  'create external entities from %s.%s' will import nothing.\n", stmt.Name.Module, stmt.Name.Name)
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
	if sum := contractSummary(newSvc.Metadata); sum != "" {
		fmt.Fprintf(ctx.Output, "  Cached $metadata: %s\n", sum)
	}
	return nil
}

// normalizeMetadataURL resolves a MetadataUrl against the project directory,
// turning a relative path into the absolute file:// URL that Studio Pro stores
// and that fetchODataMetadata expects. An empty URL stays empty.
func normalizeMetadataURL(ctx *ExecContext, rawURL string) (string, error) {
	if rawURL == "" {
		return "", nil
	}
	mprDir := ""
	if ctx != nil && ctx.MprPath != "" {
		mprDir = filepath.Dir(ctx.MprPath)
	}
	return pathutil.NormalizeURL(rawURL, mprDir)
}

// contractSummary renders a cached $metadata document as the counts worth
// printing, or "" when there is nothing to parse.
func contractSummary(metadata string) string {
	if metadata == "" {
		return ""
	}
	doc, err := types.ParseEdmx(metadata)
	if err != nil {
		return ""
	}
	entityCount := 0
	for _, s := range doc.Schemas {
		entityCount += len(s.EntityTypes)
	}
	return fmt.Sprintf("%d entity types, %d actions", entityCount, len(doc.Actions))
}

// refreshCachedMetadata re-reads the contract at svc.MetadataUrl into
// svc.Metadata / svc.MetadataHash, reporting whether it actually changed.
//
// A fetch that fails is a warning, not an error, and leaves the cached contract
// in place: losing a contract that was merely unreachable is worse than serving
// a stale one, and the caller has nothing else to fall back on. An unchanged
// contract leaves svc untouched, so the write is elided and the statement
// reports "Unchanged" rather than churning the document.
func refreshCachedMetadata(ctx *ExecContext, svc *model.ConsumedODataService, stmt *ast.CreateODataClientStmt) bool {
	if svc == nil || svc.MetadataUrl == "" {
		return false
	}
	auth := metadataAuthFromStmt(ctx, stmt)
	metadata, hash, err := fetchODataMetadata(svc.MetadataUrl, auth)
	if err != nil {
		fmt.Fprintf(ctx.Output, "Warning: could not refresh $metadata: %v\n", err)
		for _, hint := range auth.hints() {
			fmt.Fprintf(ctx.Output, "  %s\n", hint)
		}
		fmt.Fprintf(ctx.Output, "  The client keeps the contract cached earlier, which may be out of date.\n")
		return false
	}
	if metadata == "" || hash == svc.MetadataHash {
		return false
	}
	svc.Metadata = metadata
	svc.MetadataHash = hash
	svc.Validated = true
	return true
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
					location, err := serviceURLConstant(strVal)
					if err != nil {
						return err
					}
					if svc.HttpConfiguration == nil {
						svc.HttpConfiguration = &model.HttpConfiguration{}
					}
					svc.HttpConfiguration.OverrideLocation = true
					svc.HttpConfiguration.CustomLocation = location
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
					svc.ProxyHost = extractConstantRef(strVal)
				case "proxyport":
					svc.ProxyPort = extractConstantRef(strVal)
				case "proxyusername":
					svc.ProxyUsername = extractConstantRef(strVal)
				case "proxypassword":
					svc.ProxyPassword = extractConstantRef(strVal)
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
		return mdlerrors.NewValidation("module name required: use create published odata service Module.Name (...)")
	}

	// Gate before the write, not after: a property the project's Mendix version
	// does not have is not a build error, it is a document Studio Pro refuses to
	// open (InvalidOperationException at MprProperty.cs). Only checked when the
	// author asked for it, so nothing changes for the services that do not.
	// GraphQL has no representation for an associated object id, so Mendix
	// refuses the combination outright: CE8055 "A service that supports GraphQL
	// must publish associations as a link." Refused here rather than warned
	// about, because unlike PublishAssociations: No on its own — which is a
	// legitimate mode for a service whose key is arranged in Studio Pro — this
	// pair can never build, whatever else the author does.
	if stmt.SupportsGraphQL && stmt.PublishAssociationsSet && !stmt.PublishAssociations {
		return mdlerrors.NewValidation(
			"SupportsGraphQL: Yes with PublishAssociations: No cannot build — a GraphQL service " +
				"must publish associations as a link (CE8055). Remove PublishAssociations to take " +
				"the default (Yes), or drop SupportsGraphQL.")
	}

	if stmt.SupportsGraphQL {
		if err := checkFeature(ctx, "integration", "odata_graphql",
			"SupportsGraphQL on a published OData service",
			"Publishing a service as GraphQL as well arrived in Studio Pro 10.14. "+
				"Remove SupportsGraphQL to publish OData only."); err != nil {
			return err
		}
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
					// Snapshot the grants before anything below can clear them.
					existingRoles := append([]string(nil), svc.AllowedModuleRoles...)
					// svc is the STORED service being mutated, so its own value
					// is what a silent rewrite preserves (#1018). There are two
					// update paths for this doctype and patching only the first
					// left the defect fully intact — the test caught it.
					svc.Documentation = carriedDocumentation(
						stmt.DocumentationSet, stmt.Documentation, svc.Documentation)
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
					} else if svc.ServiceName == "" {
						// Heal a service written before ServiceName was defaulted:
						// re-running the script repairs a model that cannot build.
						svc.ServiceName = svc.Name
					}
					if stmt.Summary != "" {
						svc.Summary = stmt.Summary
					}
					if stmt.Description != "" {
						svc.Description = stmt.Description
					}
					if stmt.PublishAssociationsSet {
						svc.PublishAssociations = stmt.PublishAssociations
					}
					if stmt.SupportsGraphQLSet {
						svc.SupportsGraphQL = stmt.SupportsGraphQL
					}
					if len(stmt.Microflows) > 0 {
						published, mfErr := astMicroflowDefsToModel(ctx, stmt.Microflows)
						if mfErr != nil {
							return mfErr
						}
						svc.Microflows = published
					}
					if stmt.AuthenticationSet {
						svc.AuthenticationTypes = stmt.AuthenticationTypes
						// Restated auth replaces the microflow too, including
						// clearing it when the methods do not include
						// `microflow`, and `none` clears both.
						svc.AuthMicroflow = stmt.AuthMicroflow
					}
					// Published entities are replaced wholesale when the statement
					// supplies any. Previously the modify branch ignored them
					// entirely: editing a `publish entity` block and re-running
					// left the served $metadata unchanged, so `Filterable` or
					// `Countable` changes appeared to do nothing and the only
					// thing that worked was drop + create. Replacing rather than
					// merging is what makes the script the description of the
					// service — a member removed from the script is removed from
					// the service, which merging could never express.
					if len(stmt.Entities) > 0 {
						storedTypes, storedSets := svc.EntityTypes, svc.EntitySets
						svc.EntityTypes = nil
						svc.EntitySets = nil
						for _, entityDef := range stmt.Entities {
							entityType, entitySet := astEntityDefToModel(ctx, entityDef)
							svc.EntityTypes = append(svc.EntityTypes, entityType)
							svc.EntitySets = append(svc.EntitySets, entitySet)
						}
						carryPublishedEntityState(storedTypes, storedSets, svc)
					}
					// AllowedModuleRoles is granted by a separate statement
					// (`grant access on odata service …`) and cannot be expressed
					// here, so a modify must carry it through or the build fails
					// with "At least one allowed role must be selected for the
					// published OData service to be accessible."
					//
					// The reported loss (mxcli-formula1 §26) was real, and this
					// carry-through was never what fixed it: the grants were read
					// and carried correctly, then dropped one layer down, because
					// serializePublishedODataService did not write the field at
					// all. Looking for the loss at model level and concluding "does
					// not reproduce" was the mistake — the round trip to BSON is
					// where a wholesale re-serialization deletes what it omits.
					// Kept as a guard for a caller that clears the slice.
					if len(svc.AllowedModuleRoles) == 0 && len(existingRoles) > 0 {
						svc.AllowedModuleRoles = existingRoles
					}
					if err := ctx.Backend.UpdatePublishedODataService(svc); err != nil {
						return mdlerrors.NewBackend("update OData service", err)
					}
					target, terr := resolveRequestedFolder(ctx, module.ID, stmt.Folder)
					if terr != nil {
						return terr
					}
					if _, err := applyDocumentFolder(ctx, svc.ID, svc.ContainerID, target); err != nil {
						return err
					}
					invalidateHierarchy(ctx)
					ctx.ReportMutation("Modified", "OData service: %s.%s", modName, svc.Name)
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

	// Name (the document) and ServiceName (the name in the OData metadata
	// document) are different properties, and Mendix requires the second to be
	// non-empty — an empty one fails the build with CE0729 "The service name
	// should not be empty", which `mxcli check` cannot see. Default it to the
	// document name, exactly as the CONSUMED path does for CE0339 above.
	// (mxcli-formula1 findings #10.1.)
	serviceName := stmt.ServiceName
	if serviceName == "" {
		serviceName = stmt.Name.Name
	}

	newSvc := &model.PublishedODataService{
		ContainerID:         containerID,
		Name:                stmt.Name.Name,
		Documentation:       stmt.Documentation,
		Path:                stmt.Path,
		Version:             stmt.Version,
		ODataVersion:        stmt.ODataVersion,
		Namespace:           stmt.Namespace,
		ServiceName:         serviceName,
		Summary:             stmt.Summary,
		Description:         stmt.Description,
		PublishAssociations: publishAssociationsFor(stmt),
		SupportsGraphQL:     stmt.SupportsGraphQL,
		AuthenticationTypes: stmt.AuthenticationTypes,
		AuthMicroflow:       stmt.AuthMicroflow,
	}

	// OData actions. Resolved against the project's microflows so the parameter
	// types and return type come off the microflow rather than being restated.
	publishedMFs, mfErr := astMicroflowDefsToModel(ctx, stmt.Microflows)
	if mfErr != nil {
		return mfErr
	}
	newSvc.Microflows = publishedMFs

	// Map AST entity definitions to model entity types and entity sets.
	// Pass ctx so the executor can resolve exposed members against the
	// entity's actual attributes and (for navigation properties) the
	// module's associations.
	for _, entityDef := range stmt.Entities {
		entityType, entitySet := astEntityDefToModel(ctx, entityDef)
		newSvc.EntityTypes = append(newSvc.EntityTypes, entityType)
		newSvc.EntitySets = append(newSvc.EntitySets, entitySet)
	}

	// An explicit false on a non-persistable entity is unbuildable whatever the
	// key is: object-id mode needs a published ID, and Mendix forbids publishing
	// the ID of a non-persistable entity. Say so rather than let CE7375 be the
	// first anyone hears of it.
	if !newSvc.PublishAssociations {
		if nonPersistable := nonPersistablePublishedEntities(ctx, stmt.Entities); len(nonPersistable) > 0 {
			fmt.Fprintf(ctx.Output,
				"  Warning: PublishAssociations is false, but %s %s non-persistable — associations-as-object-id requires a published ID, which Mendix forbids there. The build will fail with CE7375; remove the property to get the default (true).\n",
				strings.Join(nonPersistable, ", "), pluralIsAre(len(nonPersistable)))
		}
	}

	if err := ctx.Backend.CreatePublishedODataService(newSvc); err != nil {
		return mdlerrors.NewBackend("create OData service", err)
	}
	invalidateHierarchy(ctx)
	fmt.Fprintf(ctx.Output, "Created OData service: %s.%s\n", stmt.Name.Module, stmt.Name.Name)
	return nil
}

// carryPublishedEntityState carries onto a service's rebuilt entity types and
// sets what the `publish entity` block has no spelling for, from the stored
// ones (#743):
//   - an entity set's PageSize when the statement does not page (describe prints
//     PageSize only with UsePaging, and Studio Pro stores 10000 either way);
//   - the stored order of the entity sets, which describe prints in entity-type
//     order; sets the stored service did not have follow, in statement order;
//   - each member's CanBeEmpty, matched by entity and member name, while the
//     member's key status is unchanged.
func carryPublishedEntityState(storedTypes []*model.PublishedEntityType, storedSets []*model.PublishedEntitySet, svc *model.PublishedODataService) {
	setIndex := make(map[string]int, len(storedSets))
	setByEntity := make(map[string]*model.PublishedEntitySet, len(storedSets))
	for i, es := range storedSets {
		setIndex[es.EntityTypeName] = i
		setByEntity[es.EntityTypeName] = es
	}
	for _, es := range svc.EntitySets {
		if old, ok := setByEntity[es.EntityTypeName]; ok && !es.UsePaging && es.PageSize == 0 {
			es.PageSize = old.PageSize
		}
	}
	sort.SliceStable(svc.EntitySets, func(i, j int) bool {
		a, aok := setIndex[svc.EntitySets[i].EntityTypeName]
		b, bok := setIndex[svc.EntitySets[j].EntityTypeName]
		switch {
		case aok && bok:
			return a < b
		default:
			return aok && !bok
		}
	})

	members := map[string]*model.PublishedMember{} // entity + "/" + bare member name
	for _, et := range storedTypes {
		for _, m := range et.Members {
			members[et.Entity+"/"+bareMemberName(m.Name)] = m
		}
	}
	for _, et := range svc.EntityTypes {
		for _, m := range et.Members {
			// Only while the key status is the one it was stored with: a member
			// that becomes a key would otherwise carry CanBeEmpty true onto a
			// key, which mxbuild refuses (CE0309). On a change, the derived
			// value (!IsPartOfKey) applies.
			if old, ok := members[et.Entity+"/"+bareMemberName(m.Name)]; ok && old.CanBeEmpty != nil && old.IsPartOfKey == m.IsPartOfKey {
				v := *old.CanBeEmpty
				m.CanBeEmpty = &v
			}
		}
	}
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
				case "supportsgraphql":
					svc.SupportsGraphQL = strings.EqualFold(strVal, "true") || strings.EqualFold(strVal, "yes")
				default:
					return mdlerrors.NewUnsupported(fmt.Sprintf("unknown OData service property: %s", key))
				}
			}
			if stmt.AuthenticationSet {
				svc.AuthenticationTypes = stmt.AuthenticationTypes
				svc.AuthMicroflow = stmt.AuthMicroflow
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

// serviceURLConstant turns a ServiceUrl value into what Studio Pro stores in
// HttpConfiguration.CustomLocation: `@Module.Name`.
//
// ServiceUrl names a constant, like ProxyHost — Studio Pro picks the service URL
// as a constant (CE6825: "'Service url' must be a constant") and stores the
// reference as `@Module.Name` (ako/TestApp Odata.Bug1073:
// "@Odata.Bug1073_Location"). So it takes the proxy references' spellings: the
// bare name, `@Module.Name` and the quoted `'@Module.Name'`. Anything that is
// not a constant's name — a literal URL — is refused.
func serviceURLConstant(value string) (string, error) {
	ref := extractConstantRef(value)
	if !qualifiedConstantName.MatchString(ref) {
		return "", mdlerrors.NewValidation(fmt.Sprintf(
			"ServiceUrl must name a constant (e.g., ServiceUrl: Module.ApiLocation) — Studio Pro CE6825: 'Service url' must be a constant; got %s", value))
	}
	return "@" + ref, nil
}

// qualifiedConstantName matches `Module.Name` (or deeper), without an @.
var qualifiedConstantName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)+$`)

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

// formatExprValue formats a stored Mendix expression value for MDL output.
//
// It always quotes, even a value that already starts and ends with a quote. The
// stored text IS the expression, and the visitor unquotes the MDL string, so
// Studio Pro's literal credential `'abc'` has to print with its own quotes
// doubled inside a second pair. Passing an already-quoted value through
// unchanged made a re-exec of DESCRIBE store `abc` — an identifier, not a
// string (ako/TestApp Odata.Bug1073). mdlQuote is the inverse of the visitor's
// unquoteString, backslashes included.
func formatExprValue(ctx *ExecContext, val string) string {
	return mdlQuote(ctx, val)
}

// extractConstantRef strips a leading "@" from a constant reference. The proxy
// properties are BY_NAME references to a constant, and Studio Pro stores the bare
// qualified name (measured: ako/TestApp Odata.Bug1073, `ProxyHost:
// "Odata.Bug1073_ProxyHost"`). `@Module.Const` is MDL's spelling of a constant
// everywhere else, and it was written through verbatim — `"@Module.Const"` names
// nothing, so the proxy resolved to no constant. Accepts the bare, `@` and
// quoted-`@` spellings alike.
func extractConstantRef(ref string) string {
	return strings.TrimPrefix(ref, "@")
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
	QN       string                      // the association's own qualified name (its module, not necessarily the entity's)
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
// edmToDomainModelAttrType.
//
// Every case here has been adjudicated by mxbuild rather than assumed: an
// attribute published as the wrong EDM type is CE5016 ("has type Integer, but is
// published as Edm.Int32"), one error per attribute. Verified on 11.12.1 by
// publishing one attribute of each type and reading the errors.
func mendixAttrTypeToEdm(t domainmodel.AttributeType) string {
	if t == nil {
		return ""
	}
	switch t.GetTypeName() {
	case "String", "HashedString":
		return "Edm.String"
	case "Integer", "Long", "AutoNumber":
		// Mendix publishes Integer as Int64 too, not Int32 — an Integer is
		// 64-bit in the Mendix type system, and Int32 is CE5016 on every whole
		// number in the service.
		return "Edm.Int64"
	case "Decimal":
		return "Edm.Decimal"
	case "Boolean":
		return "Edm.Boolean"
	case "DateTime", "Date":
		return "Edm.DateTimeOffset"
	case "Binary":
		// Kept so the mapping is total, but a Binary attribute cannot actually
		// be exposed over OData — Mendix rejects it outright with CE5013
		// regardless of the published type.
		return "Edm.Binary"
	case "Enumeration":
		// Paired with EnumerationAsString=true (see enumPublishedAsString).
		// Edm.String with that flag false is rejected: Mendix then wants the
		// enumeration published in the service and typed as its own EDM enum.
		return "Edm.String"
	default:
		return "Edm.String"
	}
}

// publishedAttrType is how one Mendix attribute publishes over OData: the EDM
// type, plus whether it is an enumeration flattened to a string. The two travel
// together because Edm.String alone is ambiguous — a String and an
// EnumerationAsString enum both carry it, and only the flag tells them apart.
type publishedAttrType struct {
	Edm      string
	AsString bool
}

// enumPublishedAsString reports whether a published attribute needs the
// EnumerationAsString flag — i.e. whether it is an enumeration at all. The flag
// and the Edm.String type are a matched pair; see mendixAttrTypeToEdm.
func enumPublishedAsString(t domainmodel.AttributeType) bool {
	return t != nil && t.GetTypeName() == "Enumeration"
}

func lookupEntityMembers(ctx *ExecContext, entityQN ast.QualifiedName) (map[string]publishedAttrType, map[string]*assocMembership) {
	attrs := make(map[string]publishedAttrType) // attr name -> how it publishes
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
			attrs[a.Name] = publishedAttrType{
				Edm:      mendixAttrTypeToEdm(a.Type),
				AsString: enumPublishedAsString(a.Type),
			}
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
		assocs[a.Name] = &assocMembership{QN: entityQN.Module + "." + a.Name, ParentQN: parentQN, ChildQN: childQN, Type: a.Type}
	}
	// A cross-module association is a DomainModels$CrossAssociation, kept apart
	// from dm.Associations and stored in the FROM entity's module, with the TO
	// entity by name (mendixlabs/mxcli#1335). Missing it here sent the member to
	// the attribute fallback: CE1613 on a PublishedAttribute named after it.
	for _, ca := range dm.CrossAssociations {
		if parentQN := entityIDToQN[ca.ParentID]; parentQN == entityQN.String() {
			assocs[ca.Name] = &assocMembership{QN: entityQN.Module + "." + ca.Name, ParentQN: parentQN, ChildQN: ca.ChildRef, Type: ca.Type}
		}
	}
	// Published from the TO side, the association lives in another module's
	// domain model, so the published entity's own one cannot find it.
	addCrossAssociationsTo(ctx, entityQN, assocs)
	return attrs, assocs
}

// addCrossAssociationsTo adds the cross-module associations whose TO entity is
// entityQN. Best-effort like lookupEntityMembers: a backend failure leaves assocs
// as it was. An association name already present (local to the module) wins.
func addCrossAssociationsTo(ctx *ExecContext, entityQN ast.QualifiedName, assocs map[string]*assocMembership) {
	modules, err := getModulesFromCache(ctx)
	if err != nil {
		return
	}
	dms, err := ctx.Backend.ListDomainModels()
	if err != nil {
		return
	}
	moduleNames := make(map[model.ID]string, len(modules))
	for _, m := range modules {
		moduleNames[m.ID] = m.Name
	}
	for _, dm := range dms {
		modName := moduleNames[dm.ContainerID]
		if modName == "" || modName == entityQN.Module {
			continue
		}
		for _, ca := range dm.CrossAssociations {
			if ca.ChildRef != entityQN.String() {
				continue
			}
			if _, taken := assocs[ca.Name]; taken {
				continue
			}
			var parentQN string
			for _, e := range dm.Entities {
				if e.ID == ca.ParentID {
					parentQN = modName + "." + e.Name
					break
				}
			}
			if parentQN == "" {
				continue
			}
			assocs[ca.Name] = &assocMembership{QN: modName + "." + ca.Name, ParentQN: parentQN, ChildQN: ca.ChildRef, Type: ca.Type}
		}
	}
}

// astEntityDefToModel converts an AST PublishedEntityDef to model PublishedEntityType
// and PublishedEntitySet. Each PUBLISH ENTITY block maps to both a type (schema) and
// a set (runtime endpoint with CRUD modes).
//
// Member kinds (attribute vs association) are auto-detected against the
// entity's attributes and the module's associations: if a member's name
// matches an attribute on the entity, it's an attribute; otherwise we
// look for an association that involves this entity — same-module, or a
// cross-module one from either side — and emit it as a PublishedAssociationEnd. The user writes the bare
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
		if pub, ok := entityAttrs[m.Name]; ok {
			member.Kind = "attribute"
			member.EdmType = pub.Edm
			member.EnumerationAsString = pub.AsString
		} else if assoc := moduleAssocs[m.Name]; assoc != nil {
			member.Kind = "association"
			// Qualified here, not by the writer: it can only prefix the published
			// entity's module, which is wrong from a cross association's TO side.
			member.Name = assoc.QN
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
		Countable:      def.Countable,
		SkipSupported:  def.SkipSupported,
		TopSupported:   def.TopSupported,
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
// metadataFetchAuth carries the credentials and headers used for the
// design-time $metadata fetch.
//
// Only *literal* values are usable here. MDL stores a quoted literal with its
// quotes ('f1api') and a constant reference bare (Module.ApiUser); at design
// time mxcli has no runtime to resolve a constant against, so a reference is
// reported rather than sent as if it were the value itself.
type metadataFetchAuth struct {
	Username string            // literal, quotes already stripped
	Password string            // literal, quotes already stripped
	Headers  map[string]string // literal values only
	// Unresolved names the caller referenced by constant, for the message.
	Unresolved []string
}

// apply sets basic auth and headers on the request. A nil receiver is a no-op,
// so an unauthenticated fetch needs no special case at the call site.
func (a *metadataFetchAuth) apply(req *http.Request) {
	if a == nil {
		return
	}
	if a.Username != "" || a.Password != "" {
		req.SetBasicAuth(a.Username, a.Password)
	}
	for k, v := range a.Headers {
		req.Header.Set(k, v)
	}
}

// metadataAuthFromStmt collects the statement's own credentials and headers for
// the design-time fetch, keeping only the literals.
func metadataAuthFromStmt(ctx *ExecContext, stmt *ast.CreateODataClientStmt) *metadataFetchAuth {
	auth := &metadataFetchAuth{Headers: map[string]string{}}
	consts := designTimeConstants(ctx)

	if v, ok := resolveCredential(stmt.HttpUsername, stmt.HttpUsernameIsLiteral, consts); ok {
		auth.Username = v
	} else if stmt.HttpUsername != "" {
		auth.Unresolved = append(auth.Unresolved, "HttpUsername ("+stmt.HttpUsername+")")
	}
	if v, ok := resolveCredential(stmt.HttpPassword, stmt.HttpPasswordIsLiteral, consts); ok {
		auth.Password = v
	} else if stmt.HttpPassword != "" {
		// Named, not printed: a constant reference is a name, but the value it
		// resolves to is a secret and this line goes to the console.
		auth.Unresolved = append(auth.Unresolved, "HttpPassword ("+stmt.HttpPassword+")")
	}
	for _, h := range stmt.Headers {
		if v, ok := resolveCredential(h.Value, h.ValueIsLiteral, consts); ok {
			auth.Headers[h.Key] = v
		} else if h.Value != "" {
			auth.Unresolved = append(auth.Unresolved, "header "+h.Key+" ("+h.Value+")")
		}
	}
	sort.Strings(auth.Unresolved)
	return auth
}

// resolveCredential turns an MDL property value into the string to send on the
// design-time fetch.
//
// The value is the Mendix expression the property holds, as written — these
// properties are first-class expressions (PROPOSAL_first_class_expressions.md
// §6.4):
//
//	HttpUsername: 'f1api'            a string: sends f1api
//	HttpUsername: @Module.ApiUser    a constant: sends its design-time default
//	HttpUsername: 'Key ' + @M.C      compound: cannot be evaluated, reported unresolved
//
// The constant reference matters because mxcli requires one for ServiceUrl, so a
// client written the documented way has constants for its credentials too
// (mxcli-formula1 #23 follow-up). The old quoted form `'@Module.ApiUser'` is now
// the literal text and is refused at check time (MDL-ODATA07); the branches
// below still resolve the bare-value inputs older statements produced.
//
// A constant's design-time default is exactly what Studio Pro uses for the same
// fetch, so resolving it here is not a workaround — it is the value.
func resolveCredential(value string, isLiteral bool, consts map[string]string) (string, bool) {
	if value == "" {
		return "", false
	}
	if ref, ok := constantReference(value, isLiteral); ok {
		v, found := consts[strings.ToLower(ref)]
		return v, found && v != ""
	}
	// The value is a Mendix expression. A string literal — Studio Pro's own
	// spelling of a literal credential, `'MxAdmin'` — sends its content; any
	// other expression that starts with a quote (`'Key ' + @M.C`) cannot be
	// evaluated here and is reported unresolved rather than sent as text.
	if strings.HasPrefix(value, "'") {
		s, ok := mendixStringLiteral(value)
		return s, ok && s != ""
	}
	if isLiteral {
		return value, true
	}
	return "", false
}

// mendixStringLiteral reports whether expr is exactly one Mendix string literal
// (`'…'`, a quote inside doubled) and returns its content.
func mendixStringLiteral(expr string) (string, bool) {
	if len(expr) < 2 || expr[0] != '\'' || expr[len(expr)-1] != '\'' {
		return "", false
	}
	inner := expr[1 : len(expr)-1]
	var b strings.Builder
	for i := 0; i < len(inner); i++ {
		if inner[i] == '\'' {
			if i+1 >= len(inner) || inner[i+1] != '\'' {
				return "", false // a lone quote ends the literal early: not one literal
			}
			i++
		}
		b.WriteByte(inner[i])
	}
	return b.String(), true
}

// constantReference reports whether a property value names a constant, and which
// one. A leading @ marks a reference in either spelling; an unquoted qualified
// name is one too, since a bare Module.Name cannot be a credential.
func constantReference(value string, isLiteral bool) (string, bool) {
	if rest, found := strings.CutPrefix(value, "@"); found {
		return rest, true
	}
	if !isLiteral && strings.Contains(value, ".") {
		return value, true
	}
	return "", false
}

// designTimeConstants maps a constant's qualified name (lowercased) to its
// default value. Best-effort: a project that cannot be read yields an empty map,
// and every reference then reports itself unresolved rather than failing the
// statement.
func designTimeConstants(ctx *ExecContext) map[string]string {
	out := map[string]string{}
	if ctx == nil || ctx.Backend == nil {
		return out
	}
	consts, err := ctx.Backend.ListConstants()
	if err != nil {
		return out
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return out
	}
	for _, c := range consts {
		if c == nil {
			continue
		}
		mod := h.GetModuleName(h.FindModuleID(c.ContainerID))
		if mod == "" {
			continue
		}
		out[strings.ToLower(mod+"."+c.Name)] = c.DefaultValue
	}
	return out
}

// hints explains a failed fetch when the reason is credentials mxcli could not
// resolve, and points at the workaround that also happens to be better practice.
func (a *metadataFetchAuth) hints() []string {
	var out []string
	if a == nil {
		return out
	}
	if len(a.Unresolved) > 0 {
		out = append(out, "These are constant references, which only the runtime can resolve, so the fetch went out without them: "+strings.Join(a.Unresolved, ", "))
	}
	out = append(out,
		"Fetch the contract once and point MetadataUrl at the file — it commits, so the",
		"model rebuilds without the service running and a contract change is a reviewable diff.")
	return out
}

func fetchODataMetadata(metadataUrl string, auth *metadataFetchAuth) (metadata string, hash string, err error) {
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
		req, reqErr := http.NewRequest(http.MethodGet, metadataUrl, nil)
		if reqErr != nil {
			return "", "", mdlerrors.NewBackend(fmt.Sprintf("build $metadata request for %s", metadataUrl), reqErr)
		}
		// The credentials and headers already on the statement apply to this
		// fetch too. Without them a service behind `authentication basic`
		// answers 401, the client is created with no cached entity types, and
		// the CREATE EXTERNAL ENTITIES that follows silently imports nothing.
		auth.apply(req)
		resp, err := client.Do(req)
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
// nonPersistablePublishedEntities returns the qualified names of the published
// entities that are non-persistable, in statement order. Entities it cannot
// resolve are treated as persistable: this drives a silent default correction,
// so an unreadable entity must not change what gets written.
func nonPersistablePublishedEntities(ctx *ExecContext, defs []*ast.PublishedEntityDef) []string {
	if ctx == nil || ctx.Backend == nil {
		return nil
	}
	var out []string
	seen := make(map[string]bool)
	for _, def := range defs {
		if def == nil || seen[def.Entity.String()] {
			continue
		}
		seen[def.Entity.String()] = true
		module, err := findModule(ctx, def.Entity.Module)
		if err != nil {
			continue
		}
		dm, err := ctx.Backend.GetDomainModel(module.ID)
		if err != nil {
			continue
		}
		for _, e := range dm.Entities {
			if e.Name == def.Entity.Name && !e.Persistable {
				out = append(out, def.Entity.String())
				break
			}
		}
	}
	return out
}

// pluralIsAre picks the verb for a list of n names.
func pluralIsAre(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}

// publishAssociationsFor picks the PublishAssociations value to store.
//
// false means "expose associations as an associated object id", and Mendix then
// requires the system ID attribute to be published as the entity key — so a
// service whose key is an ordinary attribute (what MDL's `expose (Attr (KEY))`
// writes) fails the build with CE7375, and a non-persistable entity cannot
// satisfy it at all because publishing its ID is forbidden. Verified on 11.12.1:
// the identical service builds 0 errors with true and CE7375 with false, for a
// persistent entity with a unique key.
//
// Defaulting to true therefore does not pick a preference; it picks the value
// that can build from the MDL people actually write. An explicit
// `PublishAssociations: false` is still honoured — that author has published an
// ID key, or wants to know. (mxcli-formula1 findings #10.4.)
func publishAssociationsFor(stmt *ast.CreateODataServiceStmt) bool {
	if !stmt.PublishAssociationsSet {
		return true
	}
	return stmt.PublishAssociations
}

// odataModeToMDL turns a stored Read/Change mode into the MDL spelling that
// parses back. The backend stores a microflow-backed mode as
// "CallMicroflow:Module.Name" (and accepts "MICROFLOW Module.Name" on the way
// in), but a bare `CallMicroflow:Qualified.Name` matches no MDL value — so
// DESCRIBE was emitting something it could not read (mxcli-formula1 #10.5).
func odataModeToMDL(mode string) string {
	for _, prefix := range []string{"CallMicroflow:", "MICROFLOW ", "microflow "} {
		if rest := strings.TrimPrefix(mode, prefix); rest != mode {
			return "microflow " + strings.TrimSpace(rest)
		}
	}
	return mode
}

// bareMemberName strips a Module.Entity. prefix from a published member name,
// leaving the member name `expose (...)` accepts.
func bareMemberName(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i+1:]
	}
	return name
}

// odataAuthenticationProperty renders a published service's stored
// authentication as the `Authentication:` property, in the stored order, with
// the microflow attached to the Microflow method. ok is false when there is
// nothing to print (no methods: the creation default) or when the stored value
// has no MDL spelling: a method MDL has no keyword for, or a microflow without
// the Microflow method or the reverse. note then says what is stored, for a
// comment; leaving the property out keeps the stored value when the output is
// executed. Printing `microflow M.F` for a microflow stored without the method
// used to add the method on replay.
func odataAuthenticationProperty(svc *model.PublishedODataService) (prop, note string, ok bool) {
	if len(svc.AuthenticationTypes) == 0 && svc.AuthMicroflow == "" {
		return "", "", false
	}
	parts := make([]string, 0, len(svc.AuthenticationTypes))
	spellable, hasMicroflow := true, false
	for _, t := range svc.AuthenticationTypes {
		switch t {
		case "Basic", "Session", "Guest":
			parts = append(parts, strings.ToLower(t))
		case "Microflow":
			hasMicroflow = true
			if svc.AuthMicroflow == "" {
				spellable = false
				continue
			}
			parts = append(parts, "microflow "+svc.AuthMicroflow)
		default:
			spellable = false
		}
	}
	if spellable && hasMicroflow == (svc.AuthMicroflow != "") && len(parts) > 0 {
		return "Authentication: (" + strings.Join(parts, ", ") + ")", "", true
	}
	note = "methods [" + strings.Join(svc.AuthenticationTypes, ", ") + "]"
	if svc.AuthMicroflow != "" {
		note += ", microflow " + svc.AuthMicroflow
	}
	return "", note, false
}

// printPublishedMicroflowMDL writes a `publish microflow` block. Parameter data
// types and the return type are deliberately absent: they are read off the
// microflow at execution time, so restating them here would let the emitted
// script and the microflow drift apart.
func printPublishedMicroflowMDL(w io.Writer, pm *model.PublishedMicroflow) {
	head := "  publish microflow " + pm.Microflow
	if pm.ExposedName != "" {
		head += " as " + mdlQuoted(pm.ExposedName)
	}
	if len(pm.Parameters) == 0 {
		fmt.Fprintln(w, head+";")
		return
	}
	fmt.Fprintln(w, head)
	parts := make([]string, 0, len(pm.Parameters))
	for _, p := range pm.Parameters {
		// The stored ref is Module.Microflow.Param; MDL names the parameter
		// alone, so take the last segment.
		name := p.MicroflowParameter
		if i := strings.LastIndex(name, "."); i >= 0 {
			name = name[i+1:]
		}
		part := name
		if p.ExposedName != "" && p.ExposedName != name {
			part += " as " + mdlQuoted(p.ExposedName)
		}
		if p.CanBeEmpty {
			part += " (CanBeEmpty)"
		}
		parts = append(parts, part)
	}
	fmt.Fprintf(w, "    expose ( %s );\n", strings.Join(parts, ", "))
}
