// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"database/sql"
	"fmt"
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

	// Pages, layouts & snippets
	ListPages() ([]*pages.Page, error)
	ListLayouts() ([]*pages.Layout, error)
	ListSnippets() ([]*pages.Snippet, error)

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

	// Mappings & JSON structures
	ListImportMappings() ([]*model.ImportMapping, error)
	ListExportMappings() ([]*model.ExportMapping, error)
	ListJsonStructures() ([]*types.JsonStructure, error)
}

// DescribeFunc generates MDL source for a given object type and qualified name.
type DescribeFunc func(objectType string, qualifiedName string) (string, error)

// Builder populates catalog tables from MPR data.
type Builder struct {
	catalog      *Catalog
	reader       CatalogReader
	snapshot     *Snapshot
	progress     ProgressFunc
	hierarchy    *hierarchy
	tx           *sql.Tx // Transaction for batched inserts
	fullMode     bool    // If true, do full parsing (activities/widgets)
	sourceMode   bool    // If true, build source FTS table (implies full)
	describeFunc DescribeFunc

	// Document caches — avoid redundant BSON parsing across builder phases.
	// Each List* call on the reader re-parses ALL documents from BSON.
	// By caching results, we parse each document type exactly once.
	microflowCache          []*microflows.Microflow
	nanoflowCache           []*microflows.Nanoflow
	pageCache               []*pages.Page
	domainModelCache        []*domainmodel.DomainModel
	enumerationCache        []*model.Enumeration
	workflowCache           []*workflows.Workflow
	businessEventCache      []*model.BusinessEventService
	databaseConnectionCache []*model.DatabaseConnection
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
}

func (b *Builder) buildHierarchy() error {
	h := &hierarchy{
		moduleIDs:       make(map[model.ID]bool),
		moduleNames:     make(map[model.ID]string),
		containerParent: make(map[model.ID]model.ID),
		folderNames:     make(map[model.ID]string),
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
	}
}

// Cached document accessors — each parses from BSON at most once per build.

func (b *Builder) cachedMicroflows() ([]*microflows.Microflow, error) {
	if b.microflowCache == nil {
		var err error
		b.microflowCache, err = b.reader.ListMicroflows()
		if err != nil {
			return nil, err
		}
	}
	return b.microflowCache, nil
}

func (b *Builder) cachedNanoflows() ([]*microflows.Nanoflow, error) {
	if b.nanoflowCache == nil {
		var err error
		b.nanoflowCache, err = b.reader.ListNanoflows()
		if err != nil {
			return nil, err
		}
	}
	return b.nanoflowCache, nil
}

func (b *Builder) cachedPages() ([]*pages.Page, error) {
	if b.pageCache == nil {
		var err error
		b.pageCache, err = b.reader.ListPages()
		if err != nil {
			return nil, err
		}
	}
	return b.pageCache, nil
}

func (b *Builder) cachedDomainModels() ([]*domainmodel.DomainModel, error) {
	if b.domainModelCache == nil {
		var err error
		b.domainModelCache, err = b.reader.ListDomainModels()
		if err != nil {
			return nil, err
		}
	}
	return b.domainModelCache, nil
}

func (b *Builder) cachedEnumerations() ([]*model.Enumeration, error) {
	if b.enumerationCache == nil {
		var err error
		b.enumerationCache, err = b.reader.ListEnumerations()
		if err != nil {
			return nil, err
		}
	}
	return b.enumerationCache, nil
}

func (b *Builder) cachedWorkflows() ([]*workflows.Workflow, error) {
	if b.workflowCache == nil {
		var err error
		b.workflowCache, err = b.reader.ListWorkflows()
		if err != nil {
			return nil, err
		}
	}
	return b.workflowCache, nil
}

func (b *Builder) cachedBusinessEventServices() ([]*model.BusinessEventService, error) {
	if b.businessEventCache == nil {
		var err error
		b.businessEventCache, err = b.reader.ListBusinessEventServices()
		if err != nil {
			return nil, err
		}
	}
	return b.businessEventCache, nil
}

func (b *Builder) cachedDatabaseConnections() ([]*model.DatabaseConnection, error) {
	if b.databaseConnectionCache == nil {
		var err error
		b.databaseConnectionCache, err = b.reader.ListDatabaseConnections()
		if err != nil {
			return nil, err
		}
	}
	return b.databaseConnectionCache, nil
}

// Build populates all catalog tables from MPR data.
func (b *Builder) Build(progress ProgressFunc) error {
	b.progress = progress

	// Create snapshot
	b.snapshot = b.catalog.CreateSnapshot("", SnapshotSourceUnknown)
	b.snapshot.Date = time.Now()

	// Build hierarchy for module/folder resolution
	if err := b.buildHierarchy(); err != nil {
		return fmt.Errorf("failed to build hierarchy: %w", err)
	}

	// Start transaction for all inserts (major performance boost)
	tx, err := b.catalog.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if tx != nil {
			tx.Rollback()
		}
	}()
	b.tx = tx

	// Build project record
	if err := b.buildProject(); err != nil {
		return fmt.Errorf("failed to build project: %w", err)
	}

	// Build tables in dependency order
	if err := b.buildModules(); err != nil {
		return fmt.Errorf("failed to build modules: %w", err)
	}

	if err := b.buildEntities(); err != nil {
		return fmt.Errorf("failed to build entities: %w", err)
	}

	if err := b.buildEnumerations(); err != nil {
		return fmt.Errorf("failed to build enumerations: %w", err)
	}

	if err := b.buildConstants(); err != nil {
		return fmt.Errorf("failed to build constants: %w", err)
	}

	if err := b.buildConstantValues(); err != nil {
		return fmt.Errorf("failed to build constant values: %w", err)
	}

	if err := b.buildJavaActions(); err != nil {
		return fmt.Errorf("failed to build java actions: %w", err)
	}

	if err := b.buildMicroflows(); err != nil {
		return fmt.Errorf("failed to build microflows: %w", err)
	}

	if err := b.buildPages(); err != nil {
		return fmt.Errorf("failed to build pages: %w", err)
	}

	if err := b.buildSnippets(); err != nil {
		return fmt.Errorf("failed to build snippets: %w", err)
	}

	if err := b.buildLayouts(); err != nil {
		return fmt.Errorf("failed to build layouts: %w", err)
	}

	if err := b.buildODataClients(); err != nil {
		return fmt.Errorf("failed to build OData clients: %w", err)
	}

	if err := b.buildODataServices(); err != nil {
		return fmt.Errorf("failed to build OData services: %w", err)
	}

	if err := b.buildWorkflows(); err != nil {
		return fmt.Errorf("failed to build workflows: %w", err)
	}

	if err := b.buildBusinessEventServices(); err != nil {
		return fmt.Errorf("failed to build business event services: %w", err)
	}

	if err := b.buildDatabaseConnections(); err != nil {
		return fmt.Errorf("failed to build database connections: %w", err)
	}

	if err := b.buildRestClients(); err != nil {
		return fmt.Errorf("failed to build REST clients: %w", err)
	}

	if err := b.buildPublishedRestServices(); err != nil {
		return fmt.Errorf("failed to build published REST services: %w", err)
	}

	if err := b.buildExternalEntities(); err != nil {
		return fmt.Errorf("failed to build external entities: %w", err)
	}

	if err := b.buildExternalActions(); err != nil {
		return fmt.Errorf("failed to build external actions: %w", err)
	}

	if err := b.buildBusinessEvents(); err != nil {
		return fmt.Errorf("failed to build business events: %w", err)
	}

	if err := b.buildContractEntities(); err != nil {
		return fmt.Errorf("failed to build contract entities: %w", err)
	}

	if err := b.buildContractMessages(); err != nil {
		return fmt.Errorf("failed to build contract messages: %w", err)
	}

	if err := b.buildNavigation(); err != nil {
		return fmt.Errorf("failed to build navigation: %w", err)
	}

	if err := b.buildRoleMappings(); err != nil {
		return fmt.Errorf("failed to build role mappings: %w", err)
	}

	if err := b.buildJsonStructures(); err != nil {
		return fmt.Errorf("failed to build JSON structures: %w", err)
	}

	if err := b.buildImportMappings(); err != nil {
		return fmt.Errorf("failed to build import mappings: %w", err)
	}

	if err := b.buildExportMappings(); err != nil {
		return fmt.Errorf("failed to build export mappings: %w", err)
	}

	// Build cross-references (only in full mode)
	if err := b.buildReferences(); err != nil {
		return fmt.Errorf("failed to build references: %w", err)
	}

	// Build permissions (only in full mode)
	if err := b.buildPermissions(); err != nil {
		return fmt.Errorf("failed to build permissions: %w", err)
	}

	// Build XPath expressions table (full mode only)
	if err := b.buildXPathExpressions(); err != nil {
		return fmt.Errorf("failed to build xpath expressions: %w", err)
	}

	// Build strings FTS table (full mode only)
	if err := b.buildStrings(); err != nil {
		return fmt.Errorf("failed to build strings: %w", err)
	}

	// Build source FTS table (source mode only)
	if err := b.buildSource(); err != nil {
		return fmt.Errorf("failed to build source: %w", err)
	}

	// Update snapshot with object count
	b.updateSnapshotCount()

	// Build snapshot record
	if err := b.buildSnapshotRecord(); err != nil {
		return fmt.Errorf("failed to build snapshot: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	tx = nil // Prevent rollback in defer
	b.tx = nil

	return nil
}

func (b *Builder) report(table string, count int) {
	if b.progress != nil {
		b.progress(table, count)
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

// snapshotMeta returns common snapshot metadata for inserts.
func (b *Builder) snapshotMeta() (projectID, projectName, snapshotID, snapshotDate, snapshotSource, sourceID, sourceBranch, sourceRevision string) {
	return b.catalog.projectID,
		b.catalog.projectName,
		b.snapshot.ID,
		b.snapshot.Date.Format("2006-01-02 15:04:05"),
		string(b.snapshot.Source),
		b.snapshot.SourceID,
		b.snapshot.Branch,
		b.snapshot.Revision
}
