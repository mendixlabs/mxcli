// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/security"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// CatalogReader defines the read-only backend surface used by the catalog builder.
// Both *mpr.Reader and backend.FullBackend satisfy this interface.
type CatalogReader interface {
	// Project path — used to locate widgets/*.mpk and .mxcli/widgets/*.def.json
	// when building the widget_definitions catalog table.
	Path() string

	// Infrastructure
	GetRawUnit(id model.ID) (map[string]any, error)
	ListRawUnitsByType(typePrefix string) ([]*types.RawUnit, error)
	ListUnits() ([]*types.UnitInfo, error)
	ListFolders() ([]*types.FolderInfo, error)

	// Modules
	ListModules() ([]*model.Module, error)

	// Settings & security
	GetProjectSettings() (*model.ProjectSettings, error)
	GetProjectSecurity() (*security.ProjectSecurity, error)
	GetNavigation() (*types.NavigationDocument, error)

	// Domain models & enumerations
	ListDomainModels() ([]*domainmodel.DomainModel, error)
	ListEnumerations() ([]*model.Enumeration, error)
	ListConstants() ([]*model.Constant, error)

	// Microflows & nanoflows
	ListMicroflows() ([]*microflows.Microflow, error)
	ListNanoflows() ([]*microflows.Nanoflow, error)
	ListRules() ([]*microflows.Rule, error)

	// Pages, layouts & snippets
	ListPages() ([]*pages.Page, error)
	ListLayouts() ([]*pages.Layout, error)
	ListSnippets() ([]*pages.Snippet, error)
	ListBuildingBlocks() ([]*pages.BuildingBlock, error)

	// Workflows
	ListWorkflows() ([]*workflows.Workflow, error)

	// Java actions
	ListJavaActionsFull() ([]*javaactions.JavaAction, error)

	// Services
	ListConsumedODataServices() ([]*model.ConsumedODataService, error)
	ListPublishedODataServices() ([]*model.PublishedODataService, error)
	ListConsumedRestServices() ([]*model.ConsumedRestService, error)
	ListPublishedRestServices() ([]*model.PublishedRestService, error)
	ListBusinessEventServices() ([]*model.BusinessEventService, error)
	ListDatabaseConnections() ([]*model.DatabaseConnection, error)
	ListModuleSettings() ([]*types.ModuleSettings, error)

	// Mappings & JSON structures
	ListImportMappings() ([]*model.ImportMapping, error)
	ListExportMappings() ([]*model.ExportMapping, error)
	ListJsonStructures() ([]*types.JsonStructure, error)
}

// DescribeFunc generates MDL source for a given object type and qualified name.
// id is the stored document's ID and pins the describe to it: two documents
// may share a qualified name when one is excluded (#914), and describing by
// name alone renders the live one for both (#1185).
type DescribeFunc func(objectType string, qualifiedName string, id string) (string, error)

// Builder populates catalog tables from MPR data.
type Builder struct {
	catalog         *Catalog
	reader          CatalogReader
	snapshot        *Snapshot
	progress        ProgressFunc
	routine         ProgressFunc // routine counts; nil sends them to progress (SetRoutineProgress)
	hierarchy       *hierarchy
	tx              *builderTx // Transaction for batched inserts; records row owners
	fullMode        bool       // If true, do full parsing (activities/widgets)
	sourceMode      bool       // If true, build source FTS table (implies full)
	communitiesMode bool       // If true, run the graph-analysis pass (reads refs)
	resolution      float64    // Leiden resolution for the graph-analysis pass
	describeFunc    DescribeFunc

	// prevSource is the last source build's rows and basis, and
	// sourceFingerprint the mxcli build writing this one: together they let
	// unchanged documents keep their rows (builder_source_reuse.go).
	prevSource        *PreviousSource
	sourceFingerprint string
	entityDM          map[string]string // entity ID -> its domain model's, from entityBasis

	// The incremental build (builder_incremental.go): the cached catalog to
	// carry rows from, the mxcli build writing this one, every unit's basis
	// and type, the units this build rebuilds (nil: all of them), the row
	// owners it records, and how it went.
	prevCatalogPath string
	fingerprint     string
	basis           map[string]string
	unitTypes       map[string]string
	scope           map[string]bool
	rows            *rowOwners
	stats           BuildStats
	prevOwned       map[string]bool              // the previous build's per-unit passes' units
	deps            map[string]map[string]string // traced reads: unit -> name -> lookup (builder_coverage.go)

	// list is what the per-unit passes list documents from: the reader, or
	// in an incremental build a view of it holding only the units in scope,
	// so the others are neither read nor decoded. scopedReader makes that
	// view; without one the passes list everything and skip what is out of
	// scope.
	list         CatalogReader
	scopedReader func(keep func(unitID string) bool) CatalogReader

	// The project's DefaultLanguageCode, resolved once per build by
	// defaultLanguage(). Texts (enumeration value captions today) are read in
	// this language: see language.go.
	defaultLang       string
	defaultLangLoaded bool

	// Scheduled event → microflow edges, collected while cataloguing the events
	// and emitted by buildReferences (a later pass). Carried on the Builder
	// rather than re-queried because CatalogTx has no Query.
	scheduledEventRefs []scheduledEventRef
	// agentDocRefs are the decoded agent-editor documents, whose edges
	// buildReferences writes once every table their names resolve against is
	// filled (ako/mxcli#2074).
	agentDocRefs []agentDocRefs

	// Entity → regular-expression edges from attribute validation rules,
	// collected while cataloguing regexes and emitted by buildReferences.
	regexRuleRefs []regexRuleRef

	// Published REST operation → microflow edges, collected while cataloguing
	// the services and emitted by buildReferences. Same arrangement as
	// scheduledEventRefs above, and for the same reason: the operation is an
	// entry point, so nothing in the model calls the microflow it runs.
	publishedRestRefs []publishedRestRef

	// Entity → microflow edges from entity event handlers, collected while
	// cataloguing the handlers and emitted by buildReferences.
	eventHandlerRefs []eventHandlerRef

	// Integration service → microflow edges (published OData resources,
	// actions and authentication; published REST authentication; business
	// event handlers; consumed OData configuration microflows), collected by
	// the service builders and emitted by buildReferences (ako/mxcli#1639).
	serviceMicroflowRefs []serviceMicroflowRef

	// Page/snippet → document edges a widget makes beyond its single
	// widgets_data columns (a data grid 2's datasource AND on-click
	// microflow), collected by buildPages/buildSnippets and emitted by
	// buildReferences (mendixlabs/mxcli#1513).
	widgetExtraRefs []widgetExtraRef

	// Built-in widget definitions supplied by the caller — used to populate
	// the widget_definitions catalog table alongside project widgets/.
	builtinWidgetMetas []WidgetDefinitionMeta

	// Document caches — avoid redundant BSON parsing across builder phases.
	// Each List* call on the reader re-parses ALL documents from BSON.
	// By caching results, we parse each document type exactly once.
	microflowCache          []*microflows.Microflow
	nanoflowCache           []*microflows.Nanoflow
	ruleCache               []*microflows.Rule
	pageCache               []*pages.Page
	domainModelCache        []*domainmodel.DomainModel
	enumerationCache        []*model.Enumeration
	workflowCache           []*workflows.Workflow
	businessEventCache      []*model.BusinessEventService
	databaseConnectionCache []*model.DatabaseConnection
	moduleSettingsCache     []*types.ModuleSettings
}

// SetFullMode enables or disables full parsing mode.
// Full mode includes activities and widgets but is much slower.
func (b *Builder) SetFullMode(full bool) {
	b.fullMode = full
}

// SetSourceMode enables source FTS table building (implies full mode).
func (b *Builder) SetSourceMode(source bool) {
	b.sourceMode = source
	if source {
		b.fullMode = true
	}
}

// SetCommunitiesMode enables the graph-analysis pass (communities/cycles/layers/
// centrality) at the given Leiden resolution. Implies full mode (it reads refs).
func (b *Builder) SetCommunitiesMode(on bool, resolution float64) {
	b.communitiesMode = on
	b.resolution = resolution
	if on {
		b.fullMode = true
	}
}

// SetDescribeFunc sets the callback for generating MDL source.
func (b *Builder) SetDescribeFunc(fn DescribeFunc) {
	b.describeFunc = fn
}

// hierarchy provides module and folder resolution for documents.
type hierarchy struct {
	moduleIDs       map[model.ID]bool
	moduleNames     map[model.ID]string
	containerParent map[model.ID]model.ID
	folderNames     map[model.ID]string
	// unitTypes is each unit's $Type, so the folders pass can count the
	// documents a folder holds directly (ako/mxcli#1436).
	unitTypes map[model.ID]string
}

func (b *Builder) buildHierarchy() error {
	h := &hierarchy{
		moduleIDs:       make(map[model.ID]bool),
		moduleNames:     make(map[model.ID]string),
		containerParent: make(map[model.ID]model.ID),
		folderNames:     make(map[model.ID]string),
		unitTypes:       make(map[model.ID]string),
	}

	// Load modules
	modules, err := b.reader.ListModules()
	if err != nil {
		return err
	}
	for _, m := range modules {
		h.moduleIDs[m.ID] = true
		h.moduleNames[m.ID] = m.Name
	}

	// Load units for container hierarchy
	units, _ := b.reader.ListUnits()
	for _, u := range units {
		h.containerParent[u.ID] = u.ContainerID
		h.unitTypes[u.ID] = u.Type
	}

	// Load folders
	folders, _ := b.reader.ListFolders()
	for _, f := range folders {
		h.folderNames[f.ID] = f.Name
		h.containerParent[f.ID] = f.ContainerID
	}

	b.hierarchy = h
	return nil
}

func (h *hierarchy) findModuleID(containerID model.ID) model.ID {
	current := containerID
	for range 100 {
		if h.moduleIDs[current] {
			return current
		}
		parent, ok := h.containerParent[current]
		if !ok || parent == current {
			return containerID
		}
		current = parent
	}
	return containerID
}

func (h *hierarchy) getModuleName(moduleID model.ID) string {
	return h.moduleNames[moduleID]
}

func (h *hierarchy) buildFolderPath(containerID model.ID) string {
	var parts []string
	current := containerID
	for range 100 {
		if h.moduleIDs[current] {
			break
		}
		if name := h.folderNames[current]; name != "" {
			parts = append([]string{name}, parts...)
		}
		parent, ok := h.containerParent[current]
		if !ok || parent == current {
			break
		}
		current = parent
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "/")
}

// NewBuilder creates a new catalog builder.
func NewBuilder(catalog *Catalog, reader CatalogReader) *Builder {
	return &Builder{
		catalog: catalog,
		reader:  reader,
		list:    reader,
	}
}

// lister is what the per-unit passes list documents from (b.list), or the
// reader for a Builder made without NewBuilder, as a pass run on its own is.
func (b *Builder) lister() CatalogReader {
	if b.list == nil {
		return b.reader
	}
	return b.list
}

// Cached document accessors — each parses from BSON at most once per build.

func (b *Builder) cachedMicroflows() ([]*microflows.Microflow, error) {
	if b.microflowCache == nil {
		var err error
		b.microflowCache, err = b.lister().ListMicroflows()
		if err != nil {
			return nil, err
		}
	}
	return b.microflowCache, nil
}

func (b *Builder) cachedNanoflows() ([]*microflows.Nanoflow, error) {
	if b.nanoflowCache == nil {
		var err error
		b.nanoflowCache, err = b.lister().ListNanoflows()
		if err != nil {
			return nil, err
		}
	}
	return b.nanoflowCache, nil
}

func (b *Builder) cachedRules() ([]*microflows.Rule, error) {
	if b.ruleCache == nil {
		var err error
		b.ruleCache, err = b.lister().ListRules()
		if err != nil {
			return nil, err
		}
	}
	return b.ruleCache, nil
}

func (b *Builder) cachedPages() ([]*pages.Page, error) {
	if b.pageCache == nil {
		var err error
		b.pageCache, err = b.lister().ListPages()
		if err != nil {
			return nil, err
		}
	}
	return b.pageCache, nil
}

func (b *Builder) cachedDomainModels() ([]*domainmodel.DomainModel, error) {
	if b.domainModelCache == nil {
		var err error
		b.domainModelCache, err = b.lister().ListDomainModels()
		if err != nil {
			return nil, err
		}
	}
	return b.domainModelCache, nil
}

func (b *Builder) cachedEnumerations() ([]*model.Enumeration, error) {
	if b.enumerationCache == nil {
		var err error
		b.enumerationCache, err = b.lister().ListEnumerations()
		if err != nil {
			return nil, err
		}
	}
	return b.enumerationCache, nil
}

func (b *Builder) cachedWorkflows() ([]*workflows.Workflow, error) {
	if b.workflowCache == nil {
		var err error
		b.workflowCache, err = b.lister().ListWorkflows()
		if err != nil {
			return nil, err
		}
	}
	return b.workflowCache, nil
}

func (b *Builder) cachedBusinessEventServices() ([]*model.BusinessEventService, error) {
	if b.businessEventCache == nil {
		var err error
		b.businessEventCache, err = b.lister().ListBusinessEventServices()
		if err != nil {
			return nil, err
		}
	}
	return b.businessEventCache, nil
}

func (b *Builder) cachedDatabaseConnections() ([]*model.DatabaseConnection, error) {
	if b.databaseConnectionCache == nil {
		var err error
		b.databaseConnectionCache, err = b.lister().ListDatabaseConnections()
		if err != nil {
			return nil, err
		}
	}
	return b.databaseConnectionCache, nil
}

func (b *Builder) cachedModuleSettings() ([]*types.ModuleSettings, error) {
	if b.moduleSettingsCache == nil {
		var err error
		b.moduleSettingsCache, err = b.lister().ListModuleSettings()
		if err != nil {
			return nil, err
		}
	}
	return b.moduleSettingsCache, nil
}

// buildPhase is one pass of a build: the tables it writes, in dependency
// order.
type buildPhase struct {
	name string
	run  func() error
}

func (b *Builder) phases() []buildPhase {
	return []buildPhase{
		{"modules", b.buildModules},
		{"folders", b.buildFolders},
		{"entities", b.buildEntities},
		{"domain model annotations", b.buildDomainModelAnnotations},
		{"entity event handlers", b.buildEntityEventHandlers},
		{"associations", b.buildAssociations},
		{"enumerations", b.buildEnumerations},
		{"constants", b.buildConstants},
		{"constant values", b.buildConstantValues},
		{"java actions", b.buildJavaActions},
		{"javascript actions", func() error {
			return b.buildSimpleNamedDocs("JavaScriptActions$JavaScriptAction", "javascript_actions", "JavaScript Actions")
		}},
		{"image collections", func() error {
			return b.buildSimpleNamedDocs("Images$ImageCollection", "image_collections", "Image Collections")
		}},
		{"icon collections", func() error {
			return b.buildSimpleNamedDocs("CustomIcons$CustomIconCollection", "icon_collections", "Icon Collections")
		}},
		{"menus", func() error { return b.buildSimpleNamedDocs("Menus$MenuDocument", "menus", "Menus") }},
		// Page templates are indexed in their own right rather than as pages. They
		// were previously swept into pages_data by a prefix-matched Forms$Page query;
		// dropping them from the catalog entirely instead would make 46 documents in
		// a stock Atlas project invisible to anything that enumerates a module.
		{"page templates", func() error { return b.buildSimpleNamedDocs("Forms$PageTemplate", "page_templates", "Page Templates") }},
		{"data transformers", func() error {
			return b.buildSimpleNamedDocs("DataTransformers$DataTransformer", "data_transformers", "Data Transformers")
		}},
		{"message definition collections", func() error {
			return b.buildSimpleNamedDocs("MessageDefinitions$MessageDefinitionCollection", "message_definition_collections", "Message Definition Collections")
		}},
		{"message definitions", func() error {
			return b.buildSimpleNamedDocs("MessageDefinitions$MessageDefinition2", "message_definitions", "Message Definitions")
		}},
		{"scheduled events", b.buildScheduledEvents},
		{"queues", b.buildQueues},
		{"regular expressions", b.buildRegularExpressions},
		{"agent-editor documents", b.buildAgentEditorDocs},
		{"microflows", b.buildMicroflows},
		{"pages", b.buildPages},
		{"snippets", b.buildSnippets},
		{"building blocks", b.buildBuildingBlocks},
		{"layouts", b.buildLayouts},
		{"OData clients", b.buildODataClients},
		{"OData services", b.buildODataServices},
		{"workflows", b.buildWorkflows},
		{"business event services", b.buildBusinessEventServices},
		{"widget definitions", b.buildWidgetDefinitions},
		{"database connections", b.buildDatabaseConnections},
		{"jar dependencies", b.buildJarDependencies},
		{"REST clients", b.buildRestClients},
		{"published REST services", b.buildPublishedRestServices},
		{"external entities", b.buildExternalEntities},
		{"external actions", b.buildExternalActions},
		{"business events", b.buildBusinessEvents},
		{"contract entities", b.buildContractEntities},
		{"contract entity usage", b.buildContractEntityUsage},
		{"contract messages", b.buildContractMessages},
		{"navigation", b.buildNavigation},
		{"role mappings", b.buildRoleMappings},
		{"JSON structures", b.buildJsonStructures},
		{"import mappings", b.buildImportMappings},
		{"export mappings", b.buildExportMappings},
		// Before buildReferences, which resolves the attributes and associations
		// each constraint names.
		{"xpath expressions", b.buildXPathExpressions},
		{"references", b.buildReferences},
		{"permissions", b.buildPermissions},
		{"strings", b.buildStrings},
		{"source", b.buildSource},
		// Only for REFRESH CATALOG COMMUNITIES; reads the refs built above.
		{"graph analysis", b.buildGraphAnalysis},
	}
}

// Build populates all catalog tables from MPR data. With a previous catalog
// (SetPreviousCatalog) it carries over the rows of every unit nothing has
// changed under and rebuilds the rest (builder_incremental.go).
func (b *Builder) Build(progress ProgressFunc) error {
	b.progress = progress
	b.stats = BuildStats{}

	// Create snapshot
	b.snapshot = b.catalog.CreateSnapshot("", SnapshotSourceUnknown)
	b.snapshot.Date = time.Now()

	// Build hierarchy for module/folder resolution
	if err := b.buildHierarchy(); err != nil {
		return fmt.Errorf("failed to build hierarchy: %w", err)
	}
	if b.fingerprint != "" {
		b.basis, b.unitTypes = b.unitBasis()
	}

	sdb, ok := b.catalog.db.(*SqliteCatalogDB)
	if !ok {
		// A catalog not on SQLite (js/wasm) has no ATTACH to carry rows with.
		tx, err := b.catalog.db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}
		b.stats.Reason = "the catalog is not on SQLite"
		if err := b.buildAttempt(tx, nil, nil); err != nil {
			return err
		}
		return b.markGraphAnalysis()
	}
	if err := b.buildOn(sdb.RawDB()); err != nil {
		return err
	}
	// After the build's connection is back in the pool: an in-memory
	// catalog lives on that one connection.
	return b.markGraphAnalysis()
}

// buildOn builds on one connection of db: an in-memory catalog lives on
// one connection, and an incremental build loads the previous catalog into it.
func (b *Builder) buildOn(db *sql.DB) error {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("failed to open catalog connection: %w", err)
	}
	defer conn.Close()

	plan, reason := (*incrementalPlan)(nil), "no cache to carry from"
	if b.prevCatalogPath != "" && b.fingerprint != "" && b.basis != nil {
		var data []byte
		if data, plan, reason = b.loadPrevious(ctx); plan != nil {
			// Loaded once, and only to build on: a connection whose
			// full-text tables were in use crashed SQLite when it was
			// loaded again (modernc.org/sqlite), so a fallback empties the
			// tables instead (clearCatalog).
			if err := deserializeConn(conn, data); err != nil {
				plan, reason = nil, "cannot load the cache: "+err.Error()
			}
		}
	}
	loaded := plan != nil

	// An incremental attempt whose result shows the plan missed a unit is
	// rolled back and run again with the wider scope, once; after that, or
	// when it fails any other way, the build is a full one.
	for round := 1; plan != nil && round <= 2; round++ {
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}
		b.stats.Rounds = round
		err = b.buildAttempt(tx, plan, func() (map[string]bool, string) { return b.verifyIncremental(plan) })
		if err == nil {
			return nil
		}
		var widen *widenScope
		if !errors.As(err, &widen) {
			var fallback *incrementalFallback
			if !errors.As(err, &fallback) {
				return err
			}
			reason = fallback.reason
			break
		}
		reason = "the plan missed a document that names a changed one"
		for id := range widen.units {
			plan.scope[id] = true
		}
	}
	b.stats = BuildStats{Reason: reason}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	if loaded {
		// The attempts were rolled back to the previous catalog.
		if err := clearCatalog(tx); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to empty the catalog: %w", err)
		}
	}
	return b.buildAttempt(tx, nil, nil)
}

// widenScope rolls an incremental attempt back to run it again with more
// units in scope.
type widenScope struct{ units map[string]bool }

func (w *widenScope) Error() string { return fmt.Sprintf("%d more units to rebuild", len(w.units)) }

// incrementalFallback rolls an incremental attempt back to run a full build.
type incrementalFallback struct{ reason string }

func (f *incrementalFallback) Error() string { return f.reason }

// buildAttempt runs every phase in tx and commits. With a plan the catalog
// holds the previous build's rows: it first drops those of the units in
// scope and the project-wide ones, and verify checks the result before the
// commit.
func (b *Builder) buildAttempt(tx CatalogTx, plan *incrementalPlan, verify func() (map[string]bool, string)) error {
	defer func() {
		if tx != nil {
			tx.Rollback()
		}
	}()
	b.tx = newBuilderTx(tx, b)
	b.rows = newRowOwners()
	b.resetCollected()
	if b.deps != nil {
		b.deps = map[string]map[string]string{}
	}
	b.scope = nil
	b.list = b.reader
	if plan != nil {
		b.scope = plan.scope
		if b.scopedReader != nil {
			scope := plan.scope
			b.list = b.scopedReader(func(id string) bool { return scope[id] })
		}
		carried, err := b.dropRebuiltRows()
		if err != nil {
			return &incrementalFallback{"cannot reuse the cached rows: " + err.Error()}
		}
		b.stats.Incremental = true
		b.stats.Carried = carried
		b.stats.Rebuilt = len(plan.scope)
	}

	// Build project record
	if err := b.buildProject(); err != nil {
		return fmt.Errorf("failed to build project: %w", err)
	}

	for _, p := range b.phases() {
		b.disown()
		if err := p.run(); err != nil {
			return fmt.Errorf("failed to build %s: %w", p.name, err)
		}
	}
	b.disown()
	if b.rows.err != nil {
		if plan == nil {
			return fmt.Errorf("failed to build catalog: %w", b.rows.err)
		}
		return &incrementalFallback{b.rows.err.Error()}
	}

	// Update snapshot with object count
	b.updateSnapshotCount()

	// Build snapshot record
	if err := b.buildSnapshotRecord(); err != nil {
		return fmt.Errorf("failed to build snapshot: %w", err)
	}

	// Last, so the ranges include every row written above.
	if err := b.persistOwners(); err != nil {
		return fmt.Errorf("failed to record row owners: %w", err)
	}
	if verify != nil {
		if more, reason := verify(); reason != "" {
			return &incrementalFallback{reason}
		} else if len(more) > 0 {
			return &widenScope{more}
		}
	}
	if plan != nil {
		b.say(incrementalSummary(b.stats), b.stats.Rebuilt)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	tx = nil // Prevent rollback in defer
	b.tx = nil
	return nil
}

// markGraphAnalysis records that the build ran the graph pass: the same flag
// AddGraphAnalysis writes, for the build-it-in-one-pass route. Both routes
// must set it or an empty cycles table stays ambiguous down whichever one
// the caller took (mendixlabs/mxcli#1060).
func (b *Builder) markGraphAnalysis() error {
	if !b.communitiesMode {
		return nil
	}
	if err := b.catalog.SetMeta(MetaGraphAnalysis, time.Now().Format(time.RFC3339)); err != nil {
		return err
	}
	return b.catalog.SetMeta(MetaGraphResolution,
		strconv.FormatFloat(effectiveResolution(b.resolution), 'g', -1, 64))
}

// resetCollected clears what the passes collect for later ones and the
// document lists, so a second attempt does not inherit the first one's.
func (b *Builder) resetCollected() {
	b.microflowCache = nil
	b.nanoflowCache = nil
	b.ruleCache = nil
	b.pageCache = nil
	b.domainModelCache = nil
	b.enumerationCache = nil
	b.workflowCache = nil
	b.businessEventCache = nil
	b.databaseConnectionCache = nil
	b.moduleSettingsCache = nil
	b.scheduledEventRefs = nil
	b.agentDocRefs = nil
	b.regexRuleRefs = nil
	b.publishedRestRefs = nil
	b.eventHandlerRefs = nil
	b.serviceMicroflowRefs = nil
	b.widgetExtraRefs = nil
}

// report is a pass's progress line: the rows it wrote to a table. An
// incremental build writes only the rebuilt units' rows, so a count would
// read as the table's size and is left out; say reports regardless.
func (b *Builder) report(table string, count int) {
	if b.scope == nil {
		b.count(table, count)
	}
}

// SetRoutineProgress sends the routine lines — a table's row count, the
// source pass's periodic "source n/m documents" — to fn instead of to the
// Build callback. Those are about fifty lines a build that say nothing but
// "still going", so a caller writing to a log or an agent's context can
// throttle them, while the lines that carry information (the incremental
// summary, documents that could not be described) still reach Build's
// callback (ako/mxcli#1860).
func (b *Builder) SetRoutineProgress(fn ProgressFunc) {
	b.routine = fn
}

// count reports a routine line in every build.
func (b *Builder) count(line string, n int) {
	if b.routine != nil {
		b.routine(line, n)
		return
	}
	b.say(line, n)
}

// say reports a progress line in every build.
func (b *Builder) say(line string, count int) {
	if b.progress != nil {
		b.progress(line, count)
	}
}

func (b *Builder) buildProject() error {
	_, err := b.tx.Exec(`
		INSERT INTO projects (ProjectId, ProjectName, MendixVersion, CreatedDate, SnapshotCount)
		VALUES (?, ?, ?, ?, ?)
	`,
		b.catalog.projectID,
		b.catalog.projectName,
		b.catalog.mendixVersion,
		time.Now().Format("2006-01-02 15:04:05"),
		1,
	)
	return err
}

func (b *Builder) buildSnapshotRecord() error {
	_, err := b.tx.Exec(`
		INSERT INTO snapshots (SnapshotId, SnapshotName, ProjectId, ProjectName, SnapshotDate,
			SnapshotSource, SourceId, SourceBranch, SourceRevision, ObjectCount, IsActive)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		b.snapshot.ID,
		b.snapshot.Name,
		b.catalog.projectID,
		b.catalog.projectName,
		b.snapshot.Date.Format("2006-01-02 15:04:05"),
		string(b.snapshot.Source),
		b.snapshot.SourceID,
		b.snapshot.Branch,
		b.snapshot.Revision,
		b.snapshot.ObjectCount,
		1,
	)
	return err
}

func (b *Builder) updateSnapshotCount() {
	// Count total objects
	var count int
	row := b.catalog.db.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT Id FROM modules
			UNION ALL SELECT Id FROM entities
			UNION ALL SELECT Id FROM microflows
			UNION ALL SELECT Id FROM pages
			UNION ALL SELECT Id FROM snippets
			UNION ALL SELECT Id FROM enumerations
			UNION ALL SELECT Id FROM java_actions
			UNION ALL SELECT Id FROM workflows
		)
	`)
	row.Scan(&count)
	b.snapshot.ObjectCount = count
}

// snapshotMeta returns the foreign-key columns that every domain table row
// carries: the project ID and the snapshot ID. Project name, snapshot date,
// source, branch, and revision are no longer copied per-row — they live on
// the projects/snapshots lookup tables and are surfaced via the per-table
// views (see tables.go and issue #576).
func (b *Builder) snapshotMeta() (projectID, snapshotID string) {
	return b.catalog.projectID, b.snapshot.ID
}
