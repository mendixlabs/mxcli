// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
)

// listContractEntities handles SHOW CONTRACT ENTITIES FROM Module.Service.
func listContractEntities(ctx *ExecContext, name *ast.QualifiedName) error {
	if name == nil {
		return mdlerrors.NewValidation("service name required: show contract entities from Module.Service")
	}

	doc, svcQN, err := parseServiceContract(ctx, *name)
	if err != nil {
		return err
	}

	type row struct {
		entitySet  string
		entityType string
		key        string
		props      int
		navs       int
		summary    string
	}

	// Build entity set lookup
	esMap := make(map[string]string) // entity type qualified name → entity set name
	for _, es := range doc.EntitySets {
		esMap[es.EntityType] = es.Name
	}

	var rows []row

	for _, s := range doc.Schemas {
		for _, et := range s.EntityTypes {
			entitySetName := esMap[s.Namespace+"."+et.Name]
			key := strings.Join(et.KeyProperties, ", ")
			summary := et.Summary
			if len(summary) > 60 {
				summary = summary[:57] + "..."
			}

			rows = append(rows, row{entitySetName, et.Name, key, len(et.Properties), len(et.NavigationProperties), summary})
		}
	}

	if len(rows) == 0 && ctx.Format != FormatJSON {
		fmt.Fprintf(ctx.Output, "No entity types found in contract for %s.\n", svcQN)
		return nil
	}

	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].entityType) < strings.ToLower(rows[j].entityType)
	})

	result := &TableResult{
		Columns: []string{"EntitySet", "EntityType", "Key", "Props", "Navs", "Summary"},
		Summary: fmt.Sprintf("(%d entity types in %s contract)", len(rows), svcQN),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.entitySet, r.entityType, r.key, r.props, r.navs, r.summary})
	}
	return writeResult(ctx, result)
}

// listContractActions handles SHOW CONTRACT ACTIONS FROM Module.Service.
func listContractActions(ctx *ExecContext, name *ast.QualifiedName) error {
	if name == nil {
		return mdlerrors.NewValidation("service name required: show contract actions from Module.Service")
	}

	doc, svcQN, err := parseServiceContract(ctx, *name)
	if err != nil {
		return err
	}

	if len(doc.Actions) == 0 && ctx.Format != FormatJSON {
		fmt.Fprintf(ctx.Output, "No actions/functions found in contract for %s.\n", svcQN)
		return nil
	}

	type row struct {
		name       string
		params     int
		returnType string
		bound      string
	}

	var rows []row

	for _, a := range doc.Actions {
		ret := a.ReturnType
		if ret == "" {
			ret = "(void)"
		}
		// Shorten namespace prefix
		if idx := strings.LastIndex(ret, "."); idx >= 0 {
			ret = ret[idx+1:]
		}
		if strings.HasPrefix(ret, "Collection(") {
			inner := ret[len("Collection(") : len(ret)-1]
			if idx := strings.LastIndex(inner, "."); idx >= 0 {
				inner = inner[idx+1:]
			}
			ret = "Collection(" + inner + ")"
		}

		bound := "No"
		if a.IsBound {
			bound = "Yes"
		}

		rows = append(rows, row{a.Name, len(a.Parameters), ret, bound})
	}

	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].name) < strings.ToLower(rows[j].name)
	})

	result := &TableResult{
		Columns: []string{"Action", "Params", "ReturnType", "Bound"},
		Summary: fmt.Sprintf("(%d actions/functions in %s contract)", len(rows), svcQN),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.name, r.params, r.returnType, r.bound})
	}
	return writeResult(ctx, result)
}

// describeContractEntity handles DESCRIBE CONTRACT ENTITY Service.EntityName [FORMAT mdl].
func describeContractEntity(ctx *ExecContext, name ast.QualifiedName, format string) error {
	// Name is Module.Service.EntityName — split into service ref and entity name
	// or Module.Service (list all) — but DESCRIBE should have a specific entity
	svcName, entityName, err := splitContractRef(name)
	if err != nil {
		return err
	}

	doc, svcQN, err := parseServiceContract(ctx, svcName)
	if err != nil {
		return err
	}

	et := doc.FindEntityType(entityName)
	if et == nil {
		return mdlerrors.NewNotFoundMsg("entity type", entityName, fmt.Sprintf("entity type %q not found in contract for %s", entityName, svcQN))
	}

	if strings.EqualFold(format, "mdl") {
		return outputContractEntityMDL(ctx, et, svcQN, doc)
	}

	// Default: human-readable format
	fmt.Fprintf(ctx.Output, "%s (Key: %s)\n", et.Name, strings.Join(et.KeyProperties, ", "))
	if et.Summary != "" {
		fmt.Fprintf(ctx.Output, "  Summary: %s\n", et.Summary)
	}
	if et.Description != "" {
		fmt.Fprintf(ctx.Output, "  Description: %s\n", et.Description)
	}
	fmt.Fprintln(ctx.Output)

	// Properties
	nameWidth := len("Property")
	typeWidth := len("Type")
	for _, p := range et.Properties {
		if len(p.Name) > nameWidth {
			nameWidth = len(p.Name)
		}
		typeStr := formatEdmType(p)
		if len(typeStr) > typeWidth {
			typeWidth = len(typeStr)
		}
	}

	fmt.Fprintf(ctx.Output, "  %-*s  %-*s  %s\n", nameWidth, "Property", typeWidth, "Type", "Nullable")
	fmt.Fprintf(ctx.Output, "  %s  %s  %s\n", strings.Repeat("-", nameWidth), strings.Repeat("-", typeWidth), "--------")
	for _, p := range et.Properties {
		nullable := "Yes"
		if p.Nullable != nil && !*p.Nullable {
			nullable = "No"
		}
		fmt.Fprintf(ctx.Output, "  %-*s  %-*s  %s\n", nameWidth, p.Name, typeWidth, formatEdmType(p), nullable)
	}

	// Navigation properties
	if len(et.NavigationProperties) > 0 {
		fmt.Fprintln(ctx.Output)
		fmt.Fprintln(ctx.Output, "  Navigation Properties:")
		for _, nav := range et.NavigationProperties {
			multiplicity := "0..1"
			if nav.IsMany {
				multiplicity = "*"
			}
			target := nav.TargetType
			if target == "" && nav.ToRole != "" {
				target = nav.ToRole
			}
			fmt.Fprintf(ctx.Output, "    → %-20s  (%s %s)\n", nav.Name, target, multiplicity)
		}
	}

	return nil
}

// describeContractAction handles DESCRIBE CONTRACT ACTION Service.ActionName [FORMAT mdl].
func describeContractAction(ctx *ExecContext, name ast.QualifiedName, format string) error {
	svcName, actionName, err := splitContractRef(name)
	if err != nil {
		return err
	}

	doc, svcQN, err := parseServiceContract(ctx, svcName)
	if err != nil {
		return err
	}

	var action *types.EdmAction
	for _, a := range doc.Actions {
		if strings.EqualFold(a.Name, actionName) {
			action = a
			break
		}
	}
	if action == nil {
		return mdlerrors.NewNotFoundMsg("action", actionName, fmt.Sprintf("action %q not found in contract for %s", actionName, svcQN))
	}

	fmt.Fprintf(ctx.Output, "%s\n", action.Name)
	if action.IsBound {
		fmt.Fprintln(ctx.Output, "  Bound: Yes")
	}

	if len(action.Parameters) > 0 {
		fmt.Fprintln(ctx.Output, "  Parameters:")
		for _, p := range action.Parameters {
			nullable := ""
			if p.Nullable != nil && !*p.Nullable {
				nullable = " not null"
			}
			fmt.Fprintf(ctx.Output, "    %-20s  %s%s\n", p.Name, shortenEdmType(p.Type), nullable)
		}
	}

	if action.ReturnType != "" {
		fmt.Fprintf(ctx.Output, "  Returns: %s\n", shortenEdmType(action.ReturnType))
	} else {
		fmt.Fprintln(ctx.Output, "  Returns: (void)")
	}

	return nil
}

// outputContractEntityMDL outputs a CREATE EXTERNAL ENTITY statement from contract metadata.
func outputContractEntityMDL(ctx *ExecContext, et *types.EdmEntityType, svcQN string, doc *types.EdmxDocument) error {
	// Find entity set name
	entitySetName := et.Name + "s" // fallback
	for _, es := range doc.EntitySets {
		if strings.HasSuffix(es.EntityType, "."+et.Name) || es.EntityType == et.Name {
			entitySetName = es.Name
			break
		}
	}

	// Extract module from service qualified name
	module := svcQN
	if idx := strings.Index(svcQN, "."); idx >= 0 {
		module = svcQN[:idx]
	}

	fmt.Fprintf(ctx.Output, "create external entity %s.%s\n", module, et.Name)
	fmt.Fprintf(ctx.Output, "from odata client %s (\n", svcQN)
	fmt.Fprintf(ctx.Output, "    EntitySet: '%s',\n", entitySetName)
	fmt.Fprintf(ctx.Output, "    RemoteName: '%s',\n", et.Name)
	fmt.Fprintf(ctx.Output, "    Countable: Yes\n")
	fmt.Fprintln(ctx.Output, ")")
	fmt.Fprintln(ctx.Output, "(")

	for i, p := range et.Properties {
		// Skip ID properties that are not real attributes
		isKey := false
		for _, k := range et.KeyProperties {
			if p.Name == k {
				isKey = true
				break
			}
		}
		// Key-only virtual ID properties (e.g. Salesforce "ID") have no
		// corresponding external attribute — skip them.
		if isKey && strings.EqualFold(p.Name, "ID") && len(et.KeyProperties) == 1 {
			continue
		}

		attrName := attrNameForOData(p.Name, et.Name)
		mendixType := edmToMendixType(p)
		comma := ","
		if i == len(et.Properties)-1 {
			comma = ""
		}
		// When the OData property name was renamed (reserved word), show the
		// original OData name as a comment so the user knows the mapping.
		if attrName != p.Name {
			fmt.Fprintf(ctx.Output, "    -- OData property: %s\n", p.Name)
		}
		fmt.Fprintf(ctx.Output, "    %s: %s%s\n", attrName, mendixType, comma)
	}

	fmt.Fprintln(ctx.Output, ");")
	fmt.Fprintln(ctx.Output, "/")

	return nil
}

// parseServiceContract finds a consumed OData service by name and parses its cached $metadata.
func parseServiceContract(ctx *ExecContext, name ast.QualifiedName) (*types.EdmxDocument, string, error) {
	services, err := ctx.Backend.ListConsumedODataServices()
	if err != nil {
		return nil, "", mdlerrors.NewBackend("list consumed OData services", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return nil, "", mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)

		if !strings.EqualFold(modName, name.Module) || !strings.EqualFold(svc.Name, name.Name) {
			continue
		}

		svcQN := modName + "." + svc.Name

		if svc.Metadata == "" {
			return nil, svcQN, mdlerrors.NewValidationf("no cached contract metadata for %s (MetadataUrl: %s). The service metadata has not been downloaded yet", svcQN, svc.MetadataUrl)
		}

		doc, err := types.ParseEdmx(svc.Metadata)
		if err != nil {
			return nil, svcQN, mdlerrors.NewBackend(fmt.Sprintf("parse contract metadata for %s", svcQN), err)
		}

		return doc, svcQN, nil
	}

	return nil, "", mdlerrors.NewNotFound("consumed OData service", name.Module+"."+name.Name)
}

// splitContractRef splits Module.Service.EntityName into (Module.Service, EntityName).
// For a 3-part name like Module.Service.Entity, it returns (Module.Service, Entity).
// For a 2-part name, it returns the name as-is and empty entity name.
func splitContractRef(name ast.QualifiedName) (ast.QualifiedName, string, error) {
	// The qualified name from the parser has Module and Name parts.
	// For "Module.Service.Entity", the parser gives Module="Module", Name="Service.Entity"
	// We need to split Name into service name and entity name.
	parts := strings.SplitN(name.Name, ".", 2)
	if len(parts) != 2 {
		return name, "", mdlerrors.NewValidationf("expected Module.Service.EntityName, got %s.%s", name.Module, name.Name)
	}

	svcName := ast.QualifiedName{
		Module: name.Module,
		Name:   parts[0],
	}
	return svcName, parts[1], nil
}

// formatEdmType returns a human-readable type string for a property.
func formatEdmType(p *types.EdmProperty) string {
	t := p.Type
	if p.MaxLength != "" {
		t += "(" + p.MaxLength + ")"
	}
	if p.Scale != "" {
		t += " Scale=" + p.Scale
	}
	return t
}

// shortenEdmType removes namespace prefix from a type name.
func shortenEdmType(t string) string {
	if strings.HasPrefix(t, "Collection(") {
		inner := t[len("Collection(") : len(t)-1]
		if idx := strings.LastIndex(inner, "."); idx >= 0 {
			inner = inner[idx+1:]
		}
		return "Collection(" + inner + ")"
	}
	if idx := strings.LastIndex(t, "."); idx >= 0 {
		return t[idx+1:]
	}
	return t
}

// edmToMendixType maps an Edm type to a Mendix attribute type string for MDL output.
func edmToMendixType(p *types.EdmProperty) string {
	switch p.Type {
	case "Edm.String":
		if p.MaxLength != "" && p.MaxLength != "max" {
			return "String(" + p.MaxLength + ")"
		}
		return "String(200)"
	case "Edm.Int32":
		return "Integer"
	case "Edm.Int64":
		return "Long"
	case "Edm.Decimal":
		return "Decimal"
	case "Edm.Boolean":
		return "Boolean"
	case "Edm.DateTime", "Edm.DateTimeOffset":
		return "DateTime"
	case "Edm.Date":
		return "DateTime"
	case "Edm.Binary":
		return "String(200)"
	case "Edm.Guid":
		return "String(36)"
	case "Edm.Double", "Edm.Single":
		return "Decimal"
	case "Edm.Byte", "Edm.SByte", "Edm.Int16":
		return "Integer"
	default:
		return "String(200)"
	}
}

// ============================================================================
// CREATE EXTERNAL ENTITIES (bulk)
// ============================================================================

// reservedEntityAttrNames are Mendix-reserved attribute names that must be
// renamed when imported from an OData property of the same name.
// These names conflict with Mendix system members or runtime internals.
// The check is case-insensitive (see attrNameForOData).
var reservedEntityAttrNames = map[string]bool{
	// Mendix internal identifier
	"id": true,
	// Mendix system-managed attribute for the object name (present on many entities)
	"name": true,
	// System ownership association (HasOwner / System.owner)
	"owner": true,
	// System audit associations (HasChangedBy / System.changedBy)
	"changedby": true,
	// System audit attributes (HasChangedDate / HasCreatedDate)
	"changeddate": true,
	"createddate": true,
	// Runtime type discriminator — conflicts with Mendix's internal type system
	"type": true,
	// Runtime context identifier — conflicts with Mendix OData connector internals
	"context": true,
}

// createExternalEntities handles CREATE [OR MODIFY] EXTERNAL ENTITIES FROM Module.Service [INTO Module] [ENTITIES (...)].
// It reads entity types from the cached $metadata and creates external entities in the domain model,
// populating Source, Key, and per-attribute RemoteName/RemoteType fields so the resulting BSON matches
// what Studio Pro produces.
func createExternalEntities(ctx *ExecContext, s *ast.CreateExternalEntitiesStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	doc, svcQN, err := parseServiceContract(ctx, s.ServiceRef)
	if err != nil {
		return err
	}

	// Build entity set lookup: entity type qualified name → entity set name
	esMap := make(map[string]string)
	esByType := make(map[string]*types.EdmEntitySet)
	for _, es := range doc.EntitySets {
		esMap[es.EntityType] = es.Name
		esByType[es.EntityType] = es
	}

	// Build filter set if entity names specified
	filterSet := make(map[string]bool)
	for _, name := range s.EntityNames {
		filterSet[strings.ToLower(name)] = true
	}

	// Determine target module
	targetModule := s.TargetModule
	if targetModule == "" {
		targetModule = s.ServiceRef.Module
	}

	module, err := findModule(ctx, targetModule)
	if err != nil {
		return err
	}
	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	// Index existing entities by name for upsert
	existing := make(map[string]*domainmodel.Entity)
	for _, ent := range dm.Entities {
		existing[ent.Name] = ent
	}

	// Build a global type lookup so we can resolve BaseType references across schemas.
	typeByQualified := make(map[string]*types.EdmEntityType)
	for _, schema := range doc.Schemas {
		for _, et := range schema.EntityTypes {
			typeByQualified[schema.Namespace+"."+et.Name] = et
		}
	}

	serviceRef := s.ServiceRef.String()
	var created, updated, skipped, failed int

	for _, schema := range doc.Schemas {
		for _, et := range schema.EntityTypes {
			entitySet := esByType[schema.Namespace+"."+et.Name]
			entitySetName := ""
			if entitySet != nil {
				entitySetName = entitySet.Name
			}
			isTopLevel := entitySetName != ""

			// Mendix entity name: entity set name when present (e.g. "People"),
			// otherwise the type name (e.g. "PlanItem", "Flight", "Trip").
			mendixName := et.Name
			if isTopLevel {
				mendixName = entitySetName
			}

			// Apply entity name filter (matched against type name OR entity set name)
			if len(filterSet) > 0 && !filterSet[strings.ToLower(et.Name)] && !filterSet[strings.ToLower(mendixName)] {
				continue
			}

			// Resolve the merged property and key set by walking the BaseType chain.
			mergedProps, keyProps := mergedPropertiesWithKey(et, typeByQualified)

			keyPropSet := make(map[string]bool)
			for _, k := range keyProps {
				keyPropSet[k] = true
			}

			// Build key parts from the resolved key (root entity in the chain)
			var keyParts []*domainmodel.RemoteKeyPart
			for _, keyName := range keyProps {
				var keyProp *types.EdmProperty
				for _, p := range mergedProps {
					if p.Name == keyName {
						keyProp = p
						break
					}
				}
				if keyProp == nil {
					continue
				}
				keyParts = append(keyParts, &domainmodel.RemoteKeyPart{
					Name:       attrNameForOData(keyName, et.Name),
					RemoteName: keyName,
					RemoteType: keyProp.Type,
					Type:       edmToDomainModelAttrType(keyProp, true),
				})
			}

			// Capability defaults follow Mendix's CONSERVATIVE model: an OData
			// entity set with no InsertRestrictions/UpdateRestrictions annotation
			// is treated as read-only (Creatable/Updatable = false). Verified
			// against `mx check` on the TripPin RESTier service — its metadata has
			// zero capability annotations, and Mendix reports the entities as
			// Creatable=False, so the app must match or CE6630 fires. Only an
			// explicit annotation flips a capability on. Non-top-level (contained/
			// derived) types default true — they are mutated via their parent's
			// write flow. (The earlier permissive default regressed this; the
			// service that motivated #729 was a narrower ETag/Concurrency case.)
			defaultCreatable := false
			defaultUpdatable := false
			if !isTopLevel {
				defaultCreatable = true
				defaultUpdatable = true
			}
			if entitySet != nil && entitySet.Insertable != nil {
				defaultCreatable = *entitySet.Insertable
			}
			if entitySet != nil && entitySet.Updatable != nil {
				defaultUpdatable = *entitySet.Updatable
			}
			nonInsertable := make(map[string]bool)
			nonUpdatable := make(map[string]bool)
			if entitySet != nil {
				for _, name := range entitySet.NonInsertableProperties {
					nonInsertable[name] = true
				}
				for _, name := range entitySet.NonUpdatableProperties {
					nonUpdatable[name] = true
				}
			}

			// Build attributes from merged properties
			var attrs []*domainmodel.Attribute
			for _, p := range mergedProps {
				// Drop collection-of-primitive — handled separately as primitive
				// collection NPEs (not yet implemented).
				if strings.HasPrefix(p.Type, "Collection(") {
					continue
				}
				// Drop non-Edm types (complex types and entity refs) — they need
				// to be modelled as NPEs/associations, not implemented yet.
				if !strings.HasPrefix(p.Type, "Edm.") {
					continue
				}
				// Drop Edm.Duration — Mendix has no native duration type and
				// Studio Pro skips these properties.
				if p.Type == "Edm.Duration" {
					continue
				}

				creatable := defaultCreatable
				updatable := defaultUpdatable
				if nonInsertable[p.Name] || p.Computed {
					creatable = false
				}
				if nonUpdatable[p.Name] || p.Computed || p.Immutable {
					updatable = false
				}

				attrName := attrNameForOData(p.Name, et.Name)
				attr := &domainmodel.Attribute{
					Name:       attrName,
					Type:       edmToDomainModelAttrType(p, keyPropSet[p.Name]),
					RemoteName: p.Name,
					RemoteType: p.Type,
					Filterable: true,
					Sortable:   true,
					Creatable:  creatable,
					Updatable:  updatable,
				}
				attr.ID = model.ID(types.GenerateID())
				attrs = append(attrs, attr)
			}

			if existingEntity, ok := existing[mendixName]; ok {
				if !s.CreateOrModify {
					fmt.Fprintf(ctx.Output, "  SKIPPED: %s.%s (already exists; use create or modify to update)\n", targetModule, mendixName)
					skipped++
					continue
				}
				applyExternalEntityFields(existingEntity, et, isTopLevel, serviceRef, entitySet, keyParts, attrs)
				if err := ctx.Backend.UpdateEntity(dm.ID, existingEntity); err != nil {
					fmt.Fprintf(ctx.Output, "  FAILED: %s.%s — %v\n", targetModule, mendixName, err)
					failed++
					continue
				}
				updated++
				continue
			}

			location := model.Point{X: 100 + (created+updated)*150, Y: 100}
			newEntity := &domainmodel.Entity{
				Name:     mendixName,
				Location: location,
			}
			newEntity.ID = model.ID(types.GenerateID())
			applyExternalEntityFields(newEntity, et, isTopLevel, serviceRef, entitySet, keyParts, attrs)
			if err := ctx.Backend.CreateEntity(dm.ID, newEntity); err != nil {
				fmt.Fprintf(ctx.Output, "  FAILED: %s.%s — %v\n", targetModule, mendixName, err)
				failed++
				continue
			}
			created++
		}
	}

	// Second pass: create primitive-collection NPEs (e.g. TripTag for
	// Trip.Tags = Collection(Edm.String)) and the association from the
	// parent entity to each NPE.
	//
	// OData primitive collections are a Mendix 11.0 feature: the backing
	// Rest$ODataMappedPrimitiveCollectionValue / Rest$ODataPrimitiveCollection*
	// storage types do not exist in the 10.x type cache, so writing them makes
	// `mx check` on Mendix 10.x abort the whole load with
	// TypeCacheUnknownTypeException. Pre-11 Studio Pro simply omits these
	// properties when importing an external entity, so mirror that: skip the NPEs
	// on < 11.0 (the rest of the entity imports normally).
	if pv := ctx.Backend.ProjectVersion(); pv != nil && !pv.IsAtLeast(11, 0) {
		fmt.Fprintf(ctx.Output, "Skipped primitive-collection NPEs (OData primitive collections require Mendix 11.0+; project is %s)\n", pv.ProductVersion)
	} else {
		dm, err = ctx.Backend.GetDomainModel(module.ID)
		if err == nil {
			npesCreated := createPrimitiveCollectionNPEs(ctx, dm, doc, typeByQualified, esMap, serviceRef)
			if npesCreated > 0 {
				fmt.Fprintf(ctx.Output, "Created %d primitive-collection NPEs\n", npesCreated)
			}
		}
	}

	// Third pass: walk navigation properties and create associations between
	// the entities we just created. Re-read the domain model so the NPEs
	// from the previous pass are visible.
	dm, err = ctx.Backend.GetDomainModel(module.ID)
	if err == nil {
		assocsCreated := createNavigationAssociations(ctx, dm, doc, typeByQualified, esMap, serviceRef)
		if assocsCreated > 0 {
			fmt.Fprintf(ctx.Output, "Created %d navigation associations\n", assocsCreated)
		}
	}

	fmt.Fprintf(ctx.Output, "\nFrom %s into %s: %d created, %d updated, %d skipped, %d failed\n",
		svcQN, targetModule, created, updated, skipped, failed)

	return nil
}

// assocKey is a (parentEntityName, associationName) pair used to detect
// duplicate associations across passes.
type assocKey struct {
	parent, name string
}

// createPrimitiveCollectionNPEs walks each entity type's properties and, for
// each Collection(Edm.X) property, creates a non-persistent entity to hold
// the values plus an association from the parent entity. This mirrors how
// Studio Pro handles e.g. Trip.Tags = Collection(Edm.String) by creating a
// TripTag NPE and a Trip_TripTag ReferenceSet.
func createPrimitiveCollectionNPEs(
	ctx *ExecContext,
	dm *domainmodel.DomainModel,
	doc *types.EdmxDocument,
	typeByQualified map[string]*types.EdmEntityType,
	esMap map[string]string,
	serviceRef string,
) int {
	// Lookup parent Mendix entity by EDMX type qualified name.
	parentByQN := make(map[string]*domainmodel.Entity)
	for qn, et := range typeByQualified {
		mendixName := et.Name
		if es := esMap[qn]; es != "" {
			mendixName = es
		}
		for _, ent := range dm.Entities {
			if ent.Name == mendixName {
				parentByQN[qn] = ent
				break
			}
		}
	}

	count := 0
	for _, schema := range doc.Schemas {
		for _, et := range schema.EntityTypes {
			parentEnt := parentByQN[schema.Namespace+"."+et.Name]
			if parentEnt == nil {
				continue
			}

			// Studio Pro only creates primitive collection NPEs when the
			// parent entity is non-persistable (Rest$ODataEntityTypeSource).
			// Mendix forbids associations from a persistable to a non-
			// persistable entity (CE0001), so a top-level entity-set entity
			// can't own an NPE child.
			if parentEnt.Persistable {
				continue
			}

			// Collect inherited properties from BaseType chain (only for the
			// derived type itself — base types iterate independently).
			merged, _ := mergedPropertiesWithKey(et, typeByQualified)
			for _, p := range merged {
				if !strings.HasPrefix(p.Type, "Collection(Edm.") {
					continue
				}

				// Skip if a property of the same name was inherited from a
				// base type (the base type's iteration already created the NPE).
				if isInheritedProperty(et, p.Name, typeByQualified) {
					continue
				}

				npeName := parentEnt.Name + singular(p.Name)

				// Skip if NPE already exists (idempotent re-runs)
				if findEntityByName(dm, npeName) != nil {
					continue
				}

				// Build the inner attribute type from the element type
				innerType := p.Type[len("Collection(") : len(p.Type)-1]
				innerProp := &types.EdmProperty{
					Name:      singular(p.Name),
					Type:      innerType,
					MaxLength: p.MaxLength,
					Scale:     p.Scale,
				}

				attr := &domainmodel.Attribute{
					Name:                  singular(p.Name),
					Type:                  edmToDomainModelAttrType(innerProp, false),
					RemoteName:            p.Name,
					RemoteType:            primitiveCollectionRemoteType(innerType, p.Nullable),
					IsPrimitiveCollection: true,
				}
				attr.ID = model.ID(types.GenerateID())

				npe := &domainmodel.Entity{
					Name:              npeName,
					Persistable:       false,
					Location:          model.Point{X: parentEnt.Location.X + 200, Y: parentEnt.Location.Y + 100},
					Attributes:        []*domainmodel.Attribute{attr},
					Source:            "Rest$ODataPrimitiveCollectionEntitySource",
					RemoteServiceName: serviceRef,
				}
				npe.ID = model.ID(types.GenerateID())

				if err := ctx.Backend.CreateEntity(dm.ID, npe); err != nil {
					fmt.Fprintf(ctx.Output, "  NPE FAILED: %s — %v\n", npeName, err)
					continue
				}
				count++

				// Create the association from the parent entity to the NPE.
				// Studio Pro names this <ParentEntityName>_<NPEName> and uses
				// Rest$ODataPrimitiveCollectionAssociationSource (a marker
				// type with no fields, paired with the NPE's source type).
				assocName := parentEnt.Name + "_" + npeName
				assoc := &domainmodel.Association{
					Name:          assocName,
					ParentID:      parentEnt.ID,
					ChildID:       npe.ID,
					Type:          domainmodel.AssociationTypeReferenceSet,
					Owner:         domainmodel.AssociationOwnerDefault,
					StorageFormat: domainmodel.StorageFormatColumn,
					Source:        "Rest$ODataPrimitiveCollectionAssociationSource",
				}
				assoc.ID = model.ID(types.GenerateID())
				if err := ctx.Backend.CreateAssociation(dm.ID, assoc); err != nil {
					fmt.Fprintf(ctx.Output, "  NPE ASSOC FAILED: %s — %v\n", assocName, err)
				}
			}
		}
	}
	return count
}

// isInheritedProperty reports whether a property name comes from one of the
// entity type's base types (rather than being defined on the type itself).
func isInheritedProperty(et *types.EdmEntityType, propName string, byQN map[string]*types.EdmEntityType) bool {
	for _, p := range et.Properties {
		if p.Name == propName {
			return false
		}
	}
	return true
}

// findEntityByName returns a domain model entity by name, or nil if not found.
func findEntityByName(dm *domainmodel.DomainModel, name string) *domainmodel.Entity {
	for _, ent := range dm.Entities {
		if ent.Name == name {
			return ent
		}
	}
	return nil
}

// primitiveCollectionRemoteType formats the OData remote type string the way
// Studio Pro stores it for a Collection(Edm.X) — bracketed with the Nullable
// and Unicode attributes spelled out, e.g.
//
//	Collection([Edm.String Nullable=False Unicode=True])
//	Collection([Edm.Int32 Nullable=False])
func primitiveCollectionRemoteType(innerType string, nullable *bool) string {
	nullableStr := "True"
	if nullable != nil && !*nullable {
		nullableStr = "False"
	}
	if innerType == "Edm.String" {
		return fmt.Sprintf("Collection([%s Nullable=%s Unicode=True])", innerType, nullableStr)
	}
	return fmt.Sprintf("Collection([%s Nullable=%s])", innerType, nullableStr)
}

// singular returns a naive singular form of an English plural by stripping a
// trailing "s". Good enough for OData property names like "Tags" → "Tag".
// Doesn't handle irregular plurals.
func singular(name string) string {
	if strings.HasSuffix(name, "ies") {
		return name[:len(name)-3] + "y"
	}
	if strings.HasSuffix(name, "es") && !strings.HasSuffix(name, "ses") {
		return name[:len(name)-2]
	}
	if strings.HasSuffix(name, "s") {
		return name[:len(name)-1]
	}
	return name
}

// createNavigationAssociations walks the navigation properties of every entity
// type in the schema and creates a corresponding Mendix association for each
// one whose target also exists as an entity in this domain model. Inherited
// navigation properties from BaseType chains are walked too.
//
// Each association uses Rest$ODataRemoteAssociationSource so Studio Pro can
// map it back to the OData navigation property. Per-association
// CreatableFromParent / UpdatableFromParent come from the entity set's
// Org.OData.Capabilities.V1.{Insert,Update}Restrictions/Non*NavigationProperties
// annotations.
func createNavigationAssociations(
	ctx *ExecContext,
	dm *domainmodel.DomainModel,
	doc *types.EdmxDocument,
	typeByQualified map[string]*types.EdmEntityType,
	esMap map[string]string,
	serviceRef string,
) int {
	// Build per-entity-type lookup of nav property name → restricted flags,
	// plus a direct entity-set lookup so we can read the base Insertable /
	// Updatable defaults for the FROM entity.
	type navRestrictions struct {
		nonInsertable map[string]bool
		nonUpdatable  map[string]bool
	}
	restrictionsByType := make(map[string]navRestrictions)
	esByType := make(map[string]*types.EdmEntitySet)
	for _, es := range doc.EntitySets {
		r := navRestrictions{
			nonInsertable: make(map[string]bool),
			nonUpdatable:  make(map[string]bool),
		}
		for _, name := range es.NonInsertableNavigationProperties {
			r.nonInsertable[name] = true
		}
		for _, name := range es.NonUpdatableNavigationProperties {
			r.nonUpdatable[name] = true
		}
		restrictionsByType[es.EntityType] = r
		esByType[es.EntityType] = es
	}
	// Build a lookup from EDMX type qualified name → existing Mendix entity.
	// An entity type matches by its EntitySet name when present, otherwise by
	// its bare type name.
	mendixByType := make(map[string]*domainmodel.Entity)
	for qn, et := range typeByQualified {
		entitySetName := esMap[qn]
		mendixName := et.Name
		if entitySetName != "" {
			mendixName = entitySetName
		}
		for _, ent := range dm.Entities {
			if ent.Name == mendixName {
				mendixByType[qn] = ent
				break
			}
		}
	}

	// Track associations we've already created to avoid duplicates from
	// inherited nav properties. existingAssocs is keyed by association name (for
	// uniqueness/renaming); existingNav is keyed by the OData nav-property name
	// so a re-import skips a nav property that already has an association instead
	// of creating a numerically-suffixed duplicate (Friends, Friends2, …). The
	// nav-property key relies on RemoteParentNavigationProperty, which the
	// legacy read preserves; the modelsdk read does not, so the natural
	// association name (== nav-property name) is the fallback skip signal.
	existingAssocs := make(map[assocKey]bool)
	existingNav := make(map[assocKey]bool)
	for _, a := range dm.Associations {
		// Find parent entity name for this association
		for _, ent := range dm.Entities {
			if ent.ID == a.ParentID {
				existingAssocs[assocKey{ent.Name, a.Name}] = true
				if a.RemoteParentNavigationProperty != "" {
					existingNav[assocKey{ent.Name, a.RemoteParentNavigationProperty}] = true
				}
				break
			}
		}
	}

	count := 0
	for _, schema := range doc.Schemas {
		for _, et := range schema.EntityTypes {
			parentQN := schema.Namespace + "." + et.Name
			parentEnt := mendixByType[parentQN]
			if parentEnt == nil {
				continue
			}

			for _, np := range et.NavigationProperties {
				// ContainsTarget=true navigation properties refer to OData
				// contained entities (e.g. Person.Trips). Studio Pro doesn't
				// create an association for these — the contained entities
				// are reached via the parent entity, not by association.
				if np.ContainsTarget {
					continue
				}

				// Resolve target type qualified name
				targetTypeName := np.Type
				isMany := false
				if strings.HasPrefix(targetTypeName, "Collection(") && strings.HasSuffix(targetTypeName, ")") {
					targetTypeName = targetTypeName[len("Collection(") : len(targetTypeName)-1]
					isMany = true
				}
				childEnt := mendixByType[targetTypeName]
				if childEnt == nil {
					continue // target type isn't in our project
				}

				// Mendix forbids associations from a persistable entity to a
				// non-persistable entity (CE0001). Skip these for now —
				// Studio Pro doesn't create them either when the target is
				// stored as Rest$ODataEntityTypeSource (Persistable=false).
				if parentEnt.Persistable && !childEnt.Persistable {
					continue
				}

				// Re-import dedup: if this nav property already has an
				// association on the parent, skip it — otherwise a second import
				// creates a numerically-suffixed duplicate (Friends, Friends2,
				// Friends3 …). Match by the OData nav-property name when the read
				// preserves it (legacy), else by the natural association name
				// (== nav-property name; the modelsdk read drops the nav prop).
				if existingNav[assocKey{parentEnt.Name, np.Name}] ||
					existingAssocs[assocKey{parentEnt.Name, np.Name}] {
					continue
				}

				// An association name must be unique within a module and may
				// not collide with any entity name (CE0065). When the OData
				// nav property name collides with an existing entity, append
				// a numeric suffix.
				assocName := uniqueAssocName(np.Name, dm, existingAssocs)

				assocType := domainmodel.AssociationTypeReference
				if isMany {
					assocType = domainmodel.AssociationTypeReferenceSet
				}

				// Per-association capability defaults. For a top-level FROM
				// entity (has its own entity set), CreatableFromParent /
				// UpdatableFromParent follow the entity set's Insertable /
				// Updatable annotations — silent metadata means the service
				// doesn't advertise write support, so default to false.
				// For contained/derived FROM entities (no entity set), both
				// default to true because the relationship is mutated via
				// the parent's write flow. Per-nav overrides from
				// Non{Insertable,Updatable}NavigationProperties still apply.
				creatable := true
				updatable := true
				if es, ok := esByType[parentQN]; ok {
					creatable = es.Insertable != nil && *es.Insertable
					updatable = es.Updatable != nil && *es.Updatable
				}
				if r, ok := restrictionsByType[parentQN]; ok {
					if r.nonInsertable[np.Name] {
						creatable = false
					}
					if r.nonUpdatable[np.Name] {
						updatable = false
					}
				}

				assoc := &domainmodel.Association{
					Name:                           assocName,
					ParentID:                       parentEnt.ID,
					ChildID:                        childEnt.ID,
					Type:                           assocType,
					Owner:                          domainmodel.AssociationOwnerDefault,
					StorageFormat:                  domainmodel.StorageFormatColumn,
					Source:                         "Rest$ODataRemoteAssociationSource",
					RemoteParentNavigationProperty: np.Name,
					Navigability2:                  "ParentToChild",
					CreatableFromParent:            creatable,
					UpdatableFromParent:            updatable,
				}
				assoc.ID = model.ID(types.GenerateID())

				if err := ctx.Backend.CreateAssociation(dm.ID, assoc); err != nil {
					fmt.Fprintf(ctx.Output, "  ASSOC FAILED: %s.%s — %v\n", parentEnt.Name, assocName, err)
					continue
				}
				existingAssocs[assocKey{parentEnt.Name, assocName}] = true
				existingNav[assocKey{parentEnt.Name, np.Name}] = true
				count++
			}
		}
	}
	return count
}

// uniqueAssocName returns a Mendix-safe association name for an OData nav
// property. If the requested name collides with an existing entity name OR an
// already-created association name, append a numeric suffix.
func uniqueAssocName(base string, dm *domainmodel.DomainModel, existingAssocs map[assocKey]bool) string {
	collides := func(name string) bool {
		for _, ent := range dm.Entities {
			if ent.Name == name {
				return true
			}
		}
		for k := range existingAssocs {
			if k.name == name {
				return true
			}
		}
		return false
	}
	if !collides(base) {
		return base
	}
	for i := 2; i < 100; i++ {
		candidate := fmt.Sprintf("%s_%d", base, i)
		if !collides(candidate) {
			return candidate
		}
	}
	return base
}

// applyExternalEntityFields stamps the Source/RemoteServiceName/Key/Attributes
// fields on a domain model entity, choosing the right BSON source type based on
// whether the entity has its own entity set.
//
// Top-level entities (have an entity set) → Rest$ODataRemoteEntitySource.
// Derived/abstract/contained types → Rest$ODataEntityTypeSource.
//
// When entitySet is non-nil, its parsed capability annotations
// (InsertRestrictions/UpdateRestrictions/DeleteRestrictions) override the
// optimistic defaults.
func applyExternalEntityFields(
	ent *domainmodel.Entity,
	et *types.EdmEntityType,
	isTopLevel bool,
	serviceRef string,
	entitySet *types.EdmEntitySet,
	keyParts []*domainmodel.RemoteKeyPart,
	attrs []*domainmodel.Attribute,
) {
	ent.RemoteServiceName = serviceRef
	ent.RemoteEntityName = et.Name
	ent.RemoteKeyParts = keyParts
	ent.Attributes = attrs

	if isTopLevel {
		ent.Source = "Rest$ODataRemoteEntitySource"
		ent.Persistable = true
		ent.RemoteEntitySet = entitySet.Name
		ent.Countable = true
		// Capabilities default to false (Mendix's conservative read-only default)
		// when the entity set has no Insert/Delete restriction annotation — an
		// unannotated service is treated as read-only, and the app must match or
		// mx check reports CE6630 (verified against the TripPin RESTier service,
		// whose metadata has zero capability annotations). Only an explicit
		// annotation turns a capability on.
		// Rest$ODataRemoteEntitySource has no Updatable field; updatability
		// is expressed per attribute via Rest$ODataMappedValue.
		ent.Creatable = entitySet.Insertable != nil && *entitySet.Insertable
		ent.Deletable = entitySet.Deletable != nil && *entitySet.Deletable
		ent.Updatable = false
		ent.SkipSupported = true
		ent.TopSupported = true
		ent.CreateChangeLocally = false
		return
	}

	// Derived / abstract / contained-target entity (no entity set)
	ent.Source = "Rest$ODataEntityTypeSource"
	ent.Persistable = false
	ent.IsOpen = et.IsOpen
	ent.RemoteEntitySet = ""
	// CRUD/skip/top fields are not used for entity-type sources; clear them
	// in case we're updating an existing entity that previously had them.
	ent.Countable = false
	ent.Creatable = false
	ent.Deletable = false
	ent.Updatable = false
	ent.SkipSupported = false
	ent.TopSupported = false
	ent.CreateChangeLocally = false
}

// mergedPropertiesWithKey walks the BaseType chain of an entity type and
// returns the merged property list (base properties first, then derived) along
// with the key property names from the root of the chain.
func mergedPropertiesWithKey(et *types.EdmEntityType, byQualified map[string]*types.EdmEntityType) ([]*types.EdmProperty, []string) {
	// Walk to the root, collecting types in order from base → derived.
	chain := []*types.EdmEntityType{et}
	current := et
	for current.BaseType != "" {
		parent := byQualified[current.BaseType]
		if parent == nil {
			break
		}
		chain = append([]*types.EdmEntityType{parent}, chain...)
		current = parent
	}

	var merged []*types.EdmProperty
	seen := make(map[string]bool)
	for _, t := range chain {
		for _, p := range t.Properties {
			if seen[p.Name] {
				continue
			}
			seen[p.Name] = true
			merged = append(merged, p)
		}
	}

	// The key always comes from the root of the chain.
	keyProps := chain[0].KeyProperties
	return merged, keyProps
}

// attrNameForOData returns a Mendix-safe attribute name for an OData property.
// Reserved names like Id and Name collide with Mendix's built-in entity members,
// so they get prefixed with the entity name (e.g. "Id" → "PhotoId").
func attrNameForOData(propName, entityName string) string {
	if reservedEntityAttrNames[strings.ToLower(propName)] {
		return entityName + propName
	}
	return propName
}

// edmToDomainModelAttrType converts an EDM property to a domainmodel attribute type.
// isKey forces a non-zero length for string keys: Mendix forbids unlimited
// strings as part of an external entity key (CE6121).
func edmToDomainModelAttrType(p *types.EdmProperty, isKey bool) domainmodel.AttributeType {
	switch p.Type {
	case "Edm.String":
		// Studio Pro stores Length=0 (unlimited) for OData strings without MaxLength.
		length := 0
		if p.MaxLength != "" && p.MaxLength != "max" {
			fmt.Sscanf(p.MaxLength, "%d", &length)
		}
		if isKey && length == 0 {
			length = 100 // Mendix requires bounded length for key attributes
		}
		return &domainmodel.StringAttributeType{Length: length}
	case "Edm.Int32", "Edm.Int16", "Edm.Byte", "Edm.SByte":
		return &domainmodel.IntegerAttributeType{}
	case "Edm.Int64":
		return &domainmodel.LongAttributeType{}
	case "Edm.Decimal", "Edm.Double", "Edm.Single":
		return &domainmodel.DecimalAttributeType{}
	case "Edm.Boolean":
		return &domainmodel.BooleanAttributeType{}
	case "Edm.DateTime", "Edm.DateTimeOffset", "Edm.Date":
		return &domainmodel.DateTimeAttributeType{}
	case "Edm.Guid":
		return &domainmodel.StringAttributeType{Length: 36}
	case "Edm.Binary":
		return &domainmodel.BinaryAttributeType{}
	default:
		return &domainmodel.StringAttributeType{Length: 0}
	}
}

// edmToAstDataType converts an Edm property to an AST data type.
func edmToAstDataType(p *types.EdmProperty) ast.DataType {
	switch p.Type {
	case "Edm.String":
		length := 200
		if p.MaxLength != "" && p.MaxLength != "max" {
			if n, err := fmt.Sscanf(p.MaxLength, "%d", &length); n == 0 || err != nil {
				length = 200
			}
		}
		return ast.DataType{Kind: ast.TypeString, Length: length}
	case "Edm.Int32":
		return ast.DataType{Kind: ast.TypeInteger}
	case "Edm.Int64":
		return ast.DataType{Kind: ast.TypeLong}
	case "Edm.Decimal":
		return ast.DataType{Kind: ast.TypeDecimal}
	case "Edm.Boolean":
		return ast.DataType{Kind: ast.TypeBoolean}
	case "Edm.DateTime", "Edm.DateTimeOffset", "Edm.Date":
		return ast.DataType{Kind: ast.TypeDateTime}
	case "Edm.Double", "Edm.Single":
		return ast.DataType{Kind: ast.TypeDecimal}
	case "Edm.Byte", "Edm.SByte", "Edm.Int16":
		return ast.DataType{Kind: ast.TypeInteger}
	case "Edm.Guid":
		return ast.DataType{Kind: ast.TypeString, Length: 36}
	case "Edm.Binary":
		return ast.DataType{Kind: ast.TypeString, Length: 200}
	default:
		return ast.DataType{Kind: ast.TypeString, Length: 200}
	}
}

// ============================================================================
// AsyncAPI Contract Commands
// ============================================================================

// listContractChannels handles SHOW CONTRACT CHANNELS FROM Module.Service.
func listContractChannels(ctx *ExecContext, name *ast.QualifiedName) error {
	if name == nil {
		return mdlerrors.NewValidation("service name required: show contract channels from Module.Service")
	}

	doc, svcQN, err := parseAsyncAPIContract(ctx, *name)
	if err != nil {
		return err
	}

	if len(doc.Channels) == 0 && ctx.Format != FormatJSON {
		fmt.Fprintf(ctx.Output, "No channels found in contract for %s.\n", svcQN)
		return nil
	}

	type row struct {
		channel   string
		operation string
		opID      string
		message   string
	}

	var rows []row

	for _, ch := range doc.Channels {
		rows = append(rows, row{ch.Name, ch.OperationType, ch.OperationID, ch.MessageRef})
	}

	result := &TableResult{
		Columns: []string{"Channel", "Operation", "OperationID", "Message"},
		Summary: fmt.Sprintf("(%d channels in %s contract)", len(rows), svcQN),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.channel, r.operation, r.opID, r.message})
	}
	return writeResult(ctx, result)
}

// listContractMessages handles SHOW CONTRACT MESSAGES FROM Module.Service.
func listContractMessages(ctx *ExecContext, name *ast.QualifiedName) error {
	if name == nil {
		return mdlerrors.NewValidation("service name required: show contract messages from Module.Service")
	}

	doc, svcQN, err := parseAsyncAPIContract(ctx, *name)
	if err != nil {
		return err
	}

	if len(doc.Messages) == 0 && ctx.Format != FormatJSON {
		fmt.Fprintf(ctx.Output, "No messages found in contract for %s.\n", svcQN)
		return nil
	}

	type row struct {
		name        string
		title       string
		contentType string
		props       int
	}

	var rows []row

	for _, msg := range doc.Messages {
		rows = append(rows, row{msg.Name, msg.Title, msg.ContentType, len(msg.Properties)})
	}

	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].name) < strings.ToLower(rows[j].name)
	})

	result := &TableResult{
		Columns: []string{"Message", "Title", "ContentType", "Props"},
		Summary: fmt.Sprintf("(%d messages in %s contract)", len(rows), svcQN),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.name, r.title, r.contentType, r.props})
	}
	return writeResult(ctx, result)
}

// describeContractMessage handles DESCRIBE CONTRACT MESSAGE Module.Service.MessageName.
func describeContractMessage(ctx *ExecContext, name ast.QualifiedName) error {
	svcName, msgName, err := splitContractRef(name)
	if err != nil {
		return err
	}

	doc, svcQN, err := parseAsyncAPIContract(ctx, svcName)
	if err != nil {
		return err
	}

	msg := doc.FindMessage(msgName)
	if msg == nil {
		return mdlerrors.NewNotFoundMsg("message", msgName, fmt.Sprintf("message %q not found in contract for %s", msgName, svcQN))
	}

	fmt.Fprintf(ctx.Output, "%s\n", msg.Name)
	if msg.Title != "" {
		fmt.Fprintf(ctx.Output, "  Title: %s\n", msg.Title)
	}
	if msg.Description != "" {
		fmt.Fprintf(ctx.Output, "  Description: %s\n", msg.Description)
	}
	if msg.ContentType != "" {
		fmt.Fprintf(ctx.Output, "  ContentType: %s\n", msg.ContentType)
	}

	if len(msg.Properties) > 0 {
		fmt.Fprintln(ctx.Output)
		nameWidth := len("Property")
		typeWidth := len("Type")
		for _, p := range msg.Properties {
			if len(p.Name) > nameWidth {
				nameWidth = len(p.Name)
			}
			t := asyncTypeString(p)
			if len(t) > typeWidth {
				typeWidth = len(t)
			}
		}

		fmt.Fprintf(ctx.Output, "  %-*s  %-*s\n", nameWidth, "Property", typeWidth, "Type")
		fmt.Fprintf(ctx.Output, "  %s  %s\n", strings.Repeat("-", nameWidth), strings.Repeat("-", typeWidth))
		for _, p := range msg.Properties {
			fmt.Fprintf(ctx.Output, "  %-*s  %-*s\n", nameWidth, p.Name, typeWidth, asyncTypeString(p))
		}
	}

	return nil
}

// parseAsyncAPIContract finds a business event service by name and parses its cached AsyncAPI document.
func parseAsyncAPIContract(ctx *ExecContext, name ast.QualifiedName) (*types.AsyncAPIDocument, string, error) {
	services, err := ctx.Backend.ListBusinessEventServices()
	if err != nil {
		return nil, "", mdlerrors.NewBackend("list business event services", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return nil, "", mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)

		if !strings.EqualFold(modName, name.Module) || !strings.EqualFold(svc.Name, name.Name) {
			continue
		}

		svcQN := modName + "." + svc.Name

		if svc.Document == "" {
			return nil, svcQN, mdlerrors.NewValidationf("no cached AsyncAPI contract for %s. This service has no Document field (it may be a publisher, not a consumer)", svcQN)
		}

		doc, err := types.ParseAsyncAPI(svc.Document)
		if err != nil {
			return nil, svcQN, mdlerrors.NewBackend(fmt.Sprintf("parse AsyncAPI contract for %s", svcQN), err)
		}

		return doc, svcQN, nil
	}

	return nil, "", mdlerrors.NewNotFound("business event service", name.Module+"."+name.Name)
}

// asyncTypeString formats an AsyncAPI property type for display.
func asyncTypeString(p *types.AsyncAPIProperty) string {
	if p.Format != "" {
		return p.Type + " (" + p.Format + ")"
	}
	return p.Type
}

// --- Executor method wrappers for backward compatibility ---
