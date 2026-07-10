// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"database/sql"
	"iter"

	"github.com/JordtenBulte-OLC/mxcli/mdl/catalog"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
	"github.com/JordtenBulte-OLC/mxcli/sdk/security"
)

// LintReader provides read access to MPR document data needed by lint rules.
// Implemented by MprBackend (and any backend satisfying these signatures).
type LintReader interface {
	GetMicroflow(id model.ID) (*microflows.Microflow, error)
	ListMicroflows() ([]*microflows.Microflow, error)
	GetProjectSecurity() (*security.ProjectSecurity, error)
	GetNavigation() (*types.NavigationDocument, error)
	ListPages() ([]*pages.Page, error)
	ListModules() ([]*model.Module, error)
	ListFolders() ([]*types.FolderInfo, error)
	GetRawUnit(id model.ID) (map[string]any, error)
	ListScheduledEvents() ([]*model.ScheduledEvent, error)
}

// LintContext wraps a catalog and provides rule-friendly APIs.
type LintContext struct {
	catalog  *catalog.Catalog
	db       catalog.CatalogDB
	excluded map[string]bool
	included map[string]bool // when non-empty, only these modules are linted
	reader   LintReader

	// fullMFCache memoizes every fully-parsed microflow for the lifetime of the
	// lint run (see FullMicroflow). Populated lazily on first FullMicroflow call.
	fullMFCache  map[model.ID]*microflows.Microflow
	fullMFErr    error
	fullMFLoaded bool
}

// FullMicroflow returns the fully-parsed microflow (with its object collection)
// for id, or nil if absent. It loads and caches ALL microflows once on the first
// call and serves subsequent lookups from the cache.
//
// Rules that inspect microflow bodies MUST use this instead of calling
// Reader().GetMicroflow(id) inside a `for … range ctx.Microflows()` loop: on the
// modelsdk backend GetMicroflow re-lists and re-BSON-decodes EVERY microflow unit
// on each call, so per-microflow calls are O(N^2) full decodes. On a large project
// that made `mxcli report` appear to hang for many minutes (issue #720). The cache
// is safe because the lint run is read-only and short-lived.
func (ctx *LintContext) FullMicroflow(id model.ID) (*microflows.Microflow, error) {
	if !ctx.fullMFLoaded {
		ctx.fullMFLoaded = true
		if ctx.reader != nil {
			mfs, err := ctx.reader.ListMicroflows()
			if err != nil {
				ctx.fullMFErr = err
			} else {
				ctx.fullMFCache = make(map[model.ID]*microflows.Microflow, len(mfs))
				for _, mf := range mfs {
					ctx.fullMFCache[mf.ID] = mf
				}
			}
		}
	}
	if ctx.fullMFErr != nil {
		return nil, ctx.fullMFErr
	}
	return ctx.fullMFCache[id], nil
}

// Reader returns the LintReader, or nil if not set.
func (ctx *LintContext) Reader() LintReader {
	return ctx.reader
}

// NewLintContext creates a new LintContext from a catalog and an optional reader.
// reader may be nil; rules that require backend access must check Reader() != nil.
func NewLintContext(cat *catalog.Catalog, reader LintReader) *LintContext {
	return &LintContext{
		catalog:  cat,
		db:       cat.CatalogDB(),
		excluded: make(map[string]bool),
		reader:   reader,
	}
}

// NewLintContextFromDB creates a new LintContext from a CatalogDB.
// Used in tests to provide an in-memory database with test data.
func NewLintContextFromDB(db catalog.CatalogDB) *LintContext {
	return &LintContext{
		db:       db,
		excluded: make(map[string]bool),
	}
}

// SetExcludedModules sets the list of modules to exclude from linting.
func (ctx *LintContext) SetExcludedModules(modules []string) {
	ctx.excluded = make(map[string]bool)
	for _, m := range modules {
		ctx.excluded[m] = true
	}
}

// SetIncludedModules sets an allowlist of modules to lint. When non-empty,
// only modules in this list are linted (modules not in the list are skipped).
func (ctx *LintContext) SetIncludedModules(modules []string) {
	ctx.included = make(map[string]bool)
	for _, m := range modules {
		ctx.included[m] = true
	}
}

// IsExcluded returns true if the module should be skipped during linting.
// A module is skipped when it is explicitly excluded, or when an inclusion
// filter is active and the module is not in it.
func (ctx *LintContext) IsExcluded(moduleName string) bool {
	if ctx.excluded[moduleName] {
		return true
	}
	if len(ctx.included) > 0 && !ctx.included[moduleName] {
		return true
	}
	return false
}

// Catalog returns the underlying catalog.
func (ctx *LintContext) Catalog() *catalog.Catalog {
	return ctx.catalog
}

// CatalogDB returns the underlying database abstraction for advanced queries.
func (ctx *LintContext) CatalogDB() catalog.CatalogDB {
	return ctx.db
}

// Query executes a SQL query and returns rows.
func (ctx *LintContext) Query(query string, args ...any) (*sql.Rows, error) {
	return ctx.db.Query(query, args...)
}

// Entity represents an entity from the catalog.
type Entity struct {
	ID                  string
	Name                string
	QualifiedName       string
	ModuleName          string
	Folder              string
	EntityType          string // "Persistent", "NonPersistent", "View"
	Description         string
	Generalization      string
	AttributeCount      int
	AccessRuleCount     int
	ValidationRuleCount int
	HasEventHandlers    bool
	IsExternal          bool
}

// Entities returns an iterator over all entities (excluding system modules).
func (ctx *LintContext) Entities() iter.Seq[Entity] {
	return func(yield func(Entity) bool) {
		rows, err := ctx.db.Query(`
			SELECT e.Id, e.Name, e.QualifiedName, e.ModuleName, e.Folder,
			       CASE e.EntityType
			           WHEN 'PERSISTENT' THEN 'Persistent'
			           WHEN 'NON_PERSISTENT' THEN 'NonPersistent'
			           WHEN 'VIEW' THEN 'View'
			           ELSE e.EntityType
			       END,
			       e.Description, e.Generalization, e.AttributeCount,
			       e.AccessRuleCount, e.ValidationRuleCount,
			       e.HasEventHandlers, e.IsExternal
			FROM entities e
			LEFT JOIN modules m ON e.ModuleName = m.Name
			WHERE COALESCE(m.Source, '') = ''
			ORDER BY e.ModuleName, e.Name
		`)
		if err != nil {
			return
		}
		defer rows.Close()

		for rows.Next() {
			var e Entity
			var desc, gen, folder sql.NullString
			var hasEventHandlers, isExternal int
			err := rows.Scan(&e.ID, &e.Name, &e.QualifiedName, &e.ModuleName, &folder,
				&e.EntityType, &desc, &gen, &e.AttributeCount,
				&e.AccessRuleCount, &e.ValidationRuleCount,
				&hasEventHandlers, &isExternal)
			if err != nil {
				continue
			}
			e.Folder = folder.String
			e.Description = desc.String
			e.Generalization = gen.String
			e.HasEventHandlers = hasEventHandlers == 1
			e.IsExternal = isExternal == 1

			if ctx.IsExcluded(e.ModuleName) {
				continue
			}

			if !yield(e) {
				return
			}
		}
	}
}

// Attribute represents an attribute of an entity from the catalog.
type Attribute struct {
	ID                  string
	Name                string
	EntityID            string
	EntityQualifiedName string
	ModuleName          string
	DataType            string
	Length              int
	IsUnique            bool
	IsRequired          bool
	DefaultValue        string
	IsCalculated        bool
	Description         string
}

// AttributesFor returns an iterator over all attributes for a given entity.
func (ctx *LintContext) AttributesFor(entityQualifiedName string) iter.Seq[Attribute] {
	return func(yield func(Attribute) bool) {
		rows, err := ctx.db.Query(`
			SELECT Id, Name, EntityId, EntityQualifiedName, ModuleName,
			       DataType, Length, IsUnique, IsRequired, DefaultValue,
			       IsCalculated, Description
			FROM attributes
			WHERE EntityQualifiedName = ?
			ORDER BY Name
		`, entityQualifiedName)
		if err != nil {
			return
		}
		defer rows.Close()

		for rows.Next() {
			var a Attribute
			var dataType, defaultVal, desc sql.NullString
			var length sql.NullInt64
			var isUnique, isRequired, isCalculated int
			err := rows.Scan(&a.ID, &a.Name, &a.EntityID, &a.EntityQualifiedName,
				&a.ModuleName, &dataType, &length, &isUnique, &isRequired, &defaultVal,
				&isCalculated, &desc)
			if err != nil {
				continue
			}
			a.DataType = dataType.String
			a.Length = int(length.Int64)
			a.IsUnique = isUnique == 1
			a.IsRequired = isRequired == 1
			a.DefaultValue = defaultVal.String
			a.IsCalculated = isCalculated == 1
			a.Description = desc.String

			if !yield(a) {
				return
			}
		}
	}
}

// Permission represents an entity access rule from the catalog.
type Permission struct {
	ModuleRoleName  string
	ModuleName      string // module of the role
	EntityName      string // qualified entity name
	AccessType      string // CREATE, READ, WRITE, DELETE, MEMBER_READ, MEMBER_WRITE
	MemberName      string // populated for MEMBER_READ/MEMBER_WRITE, empty for entity-level
	XPathConstraint string // empty means unconstrained
	IsConstrained   bool   // convenience: XPathConstraint != ""
}

// PermissionsFor returns an iterator over all permissions for a given entity.
func (ctx *LintContext) PermissionsFor(entityQualifiedName string) iter.Seq[Permission] {
	return func(yield func(Permission) bool) {
		rows, err := ctx.db.Query(`
			SELECT ModuleRoleName, ElementName, MemberName, AccessType, XPathConstraint, ModuleName
			FROM permissions
			WHERE ElementType = 'ENTITY' AND ElementName = ?
			ORDER BY ModuleRoleName, AccessType
		`, entityQualifiedName)
		if err != nil {
			return
		}
		defer rows.Close()

		for rows.Next() {
			var p Permission
			var memberName, xpathConstraint, moduleName sql.NullString
			err := rows.Scan(&p.ModuleRoleName, &p.EntityName, &memberName, &p.AccessType, &xpathConstraint, &moduleName)
			if err != nil {
				continue
			}
			p.MemberName = memberName.String
			p.XPathConstraint = xpathConstraint.String
			p.ModuleName = moduleName.String
			p.IsConstrained = p.XPathConstraint != ""

			if !yield(p) {
				return
			}
		}
	}
}

// AllPermission represents a permission from the catalog covering all element types.
type AllPermission struct {
	ModuleRoleName  string
	ElementType     string // ENTITY, MICROFLOW, PAGE, ODATA_SERVICE
	ElementName     string // qualified name of the element
	MemberName      string // populated for MEMBER_READ/MEMBER_WRITE
	AccessType      string // CREATE, READ, WRITE, DELETE, EXECUTE, VIEW, ACCESS, MEMBER_READ, MEMBER_WRITE
	XPathConstraint string
	IsConstrained   bool
	ModuleName      string
}

// Permissions returns an iterator over all permissions in the catalog.
func (ctx *LintContext) Permissions() iter.Seq[AllPermission] {
	return func(yield func(AllPermission) bool) {
		if ctx.db == nil {
			return
		}
		rows, err := ctx.db.Query(`
			SELECT ModuleRoleName, ElementType, ElementName,
				COALESCE(MemberName, ''), AccessType,
				COALESCE(XPathConstraint, ''), COALESCE(ModuleName, '')
			FROM permissions
			ORDER BY ElementType, ElementName, ModuleRoleName, AccessType
		`)
		if err != nil {
			return
		}
		defer rows.Close()

		for rows.Next() {
			var p AllPermission
			if err := rows.Scan(&p.ModuleRoleName, &p.ElementType, &p.ElementName,
				&p.MemberName, &p.AccessType, &p.XPathConstraint, &p.ModuleName); err != nil {
				continue
			}
			p.IsConstrained = p.XPathConstraint != ""
			if !yield(p) {
				return
			}
		}
	}
}

// UserRoleInfo represents a user role from project security.
type UserRoleInfo struct {
	Name        string
	IsAnonymous bool
	ModuleRoles []string
}

// UserRoles returns the user roles from project security.
func (ctx *LintContext) UserRoles() []UserRoleInfo {
	reader := ctx.reader
	if reader == nil {
		return nil
	}
	ps, err := reader.GetProjectSecurity()
	if err != nil || ps == nil {
		return nil
	}

	var roles []UserRoleInfo
	for _, ur := range ps.UserRoles {
		roles = append(roles, UserRoleInfo{
			Name:        ur.Name,
			IsAnonymous: ur.Name == ps.GuestUserRole,
			ModuleRoles: ur.ModuleRoles,
		})
	}
	return roles
}

// RoleMappingInfo represents a user role to module role mapping.
type RoleMappingInfo struct {
	UserRoleName   string
	ModuleRoleName string
	ModuleName     string
}

// RoleMappings returns all user role to module role mappings from the catalog.
func (ctx *LintContext) RoleMappings() iter.Seq[RoleMappingInfo] {
	return func(yield func(RoleMappingInfo) bool) {
		if ctx.db == nil {
			return
		}
		rows, err := ctx.db.Query(`
			SELECT UserRoleName, ModuleRoleName, COALESCE(ModuleName, '')
			FROM role_mappings
			ORDER BY UserRoleName, ModuleRoleName
		`)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var rm RoleMappingInfo
			if err := rows.Scan(&rm.UserRoleName, &rm.ModuleRoleName, &rm.ModuleName); err != nil {
				continue
			}
			if ctx.IsExcluded(rm.ModuleName) {
				continue
			}
			if !yield(rm) {
				return
			}
		}
	}
}

// ModuleRoleInfo represents a module role from a specific module.
type ModuleRoleInfo struct {
	Name        string
	ModuleName  string
	Description string
}

// ModuleRoles returns all module roles from the catalog role_mappings table.
// Returns deduplicated module roles derived from role mapping data.
func (ctx *LintContext) ModuleRoles() iter.Seq[ModuleRoleInfo] {
	return func(yield func(ModuleRoleInfo) bool) {
		if ctx.db == nil {
			return
		}
		rows, err := ctx.db.Query(`
			SELECT DISTINCT ModuleRoleName, COALESCE(ModuleName, '')
			FROM role_mappings
			ORDER BY ModuleName, ModuleRoleName
		`)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var mr ModuleRoleInfo
			if err := rows.Scan(&mr.Name, &mr.ModuleName); err != nil {
				continue
			}
			if ctx.IsExcluded(mr.ModuleName) {
				continue
			}
			if !yield(mr) {
				return
			}
		}
	}
}

// Microflow represents a microflow from the catalog.
type Microflow struct {
	ID             string
	Name           string
	QualifiedName  string
	ModuleName     string
	Folder         string
	MicroflowType  string // "Microflow", "Nanoflow"
	Description    string
	ReturnType     string
	ParameterCount int
	ActivityCount  int
	Complexity     int // McCabe cyclomatic complexity
}

// Microflows returns an iterator over all microflows (excluding system modules).
func (ctx *LintContext) Microflows() iter.Seq[Microflow] {
	return func(yield func(Microflow) bool) {
		rows, err := ctx.db.Query(`
			SELECT mf.Id, mf.Name, mf.QualifiedName, mf.ModuleName, mf.Folder,
			       mf.MicroflowType, mf.Description, mf.ReturnType,
			       mf.ParameterCount, mf.ActivityCount, mf.Complexity
			FROM microflows mf
			LEFT JOIN modules m ON mf.ModuleName = m.Name
			WHERE COALESCE(m.Source, '') = ''
			ORDER BY mf.ModuleName, mf.Name
		`)
		if err != nil {
			return
		}
		defer rows.Close()

		for rows.Next() {
			var mf Microflow
			var desc, retType, folder sql.NullString
			err := rows.Scan(&mf.ID, &mf.Name, &mf.QualifiedName, &mf.ModuleName, &folder,
				&mf.MicroflowType, &desc, &retType, &mf.ParameterCount, &mf.ActivityCount, &mf.Complexity)
			if err != nil {
				continue
			}
			mf.Folder = folder.String
			mf.Description = desc.String
			mf.ReturnType = retType.String

			if ctx.IsExcluded(mf.ModuleName) {
				continue
			}

			if !yield(mf) {
				return
			}
		}
	}
}

// Page represents a page from the catalog.
type Page struct {
	ID            string
	Name          string
	QualifiedName string
	ModuleName    string
	Folder        string
	Title         string
	URL           string
	Description   string
	WidgetCount   int
}

// Pages returns an iterator over all pages (excluding system modules).
func (ctx *LintContext) Pages() iter.Seq[Page] {
	return func(yield func(Page) bool) {
		rows, err := ctx.db.Query(`
			SELECT p.Id, p.Name, p.QualifiedName, p.ModuleName, p.Folder,
			       p.Title, p.URL, p.Description, p.WidgetCount
			FROM pages p
			LEFT JOIN modules m ON p.ModuleName = m.Name
			WHERE COALESCE(m.Source, '') = ''
			ORDER BY p.ModuleName, p.Name
		`)
		if err != nil {
			return
		}
		defer rows.Close()

		for rows.Next() {
			var pg Page
			var title, url, desc, folder sql.NullString
			var widgetCount sql.NullInt64
			err := rows.Scan(&pg.ID, &pg.Name, &pg.QualifiedName, &pg.ModuleName, &folder,
				&title, &url, &desc, &widgetCount)
			if err != nil {
				continue
			}
			pg.Folder = folder.String
			pg.Title = title.String
			pg.URL = url.String
			pg.Description = desc.String
			pg.WidgetCount = int(widgetCount.Int64)

			if ctx.IsExcluded(pg.ModuleName) {
				continue
			}

			if !yield(pg) {
				return
			}
		}
	}
}

// Enumeration represents an enumeration from the catalog.
type Enumeration struct {
	ID            string
	Name          string
	QualifiedName string
	ModuleName    string
	Folder        string
	Description   string
	ValueCount    int
}

// Enumerations returns an iterator over all enumerations (excluding system modules).
func (ctx *LintContext) Enumerations() iter.Seq[Enumeration] {
	return func(yield func(Enumeration) bool) {
		rows, err := ctx.db.Query(`
			SELECT en.Id, en.Name, en.QualifiedName, en.ModuleName, en.Folder,
			       en.Description, en.ValueCount
			FROM enumerations en
			LEFT JOIN modules m ON en.ModuleName = m.Name
			WHERE COALESCE(m.Source, '') = ''
			ORDER BY en.ModuleName, en.Name
		`)
		if err != nil {
			return
		}
		defer rows.Close()

		for rows.Next() {
			var en Enumeration
			var desc, folder sql.NullString
			err := rows.Scan(&en.ID, &en.Name, &en.QualifiedName, &en.ModuleName, &folder,
				&desc, &en.ValueCount)
			if err != nil {
				continue
			}
			en.Folder = folder.String
			en.Description = desc.String

			if ctx.IsExcluded(en.ModuleName) {
				continue
			}

			if !yield(en) {
				return
			}
		}
	}
}

// LintConstant represents a constant from the catalog.
type LintConstant struct {
	ID              string
	Name            string
	QualifiedName   string
	ModuleName      string
	Folder          string
	Description     string
	DefaultValue    string
	ExposedToClient bool
}

// Constants returns an iterator over all constants (excluding system modules).
func (ctx *LintContext) Constants() iter.Seq[LintConstant] {
	return func(yield func(LintConstant) bool) {
		rows, err := ctx.db.Query(`
			SELECT c.Id, c.Name, c.QualifiedName, c.ModuleName, c.Folder,
			       c.Description, c.DefaultValue, c.ExposedToClient
			FROM constants c
			LEFT JOIN modules m ON c.ModuleName = m.Name
			WHERE COALESCE(m.Source, '') = ''
			ORDER BY c.ModuleName, c.Name
		`)
		if err != nil {
			return
		}
		defer rows.Close()

		for rows.Next() {
			var c LintConstant
			var folder, desc, defaultVal sql.NullString
			var exposedToClient int
			err := rows.Scan(&c.ID, &c.Name, &c.QualifiedName, &c.ModuleName,
				&folder, &desc, &defaultVal, &exposedToClient)
			if err != nil {
				continue
			}
			c.Folder = folder.String
			c.Description = desc.String
			c.DefaultValue = defaultVal.String
			c.ExposedToClient = exposedToClient == 1

			if ctx.excluded[c.ModuleName] {
				continue
			}

			if !yield(c) {
				return
			}
		}
	}
}

// Widget represents a widget from the catalog.
type Widget struct {
	ID                     string
	Name                   string
	WidgetType             string
	ContainerID            string
	ContainerQualifiedName string
	ContainerType          string
	ModuleName             string
	EntityRef              string // Qualified name of referenced entity (e.g., "OtherModule.Customer")
	AttributeRef           string
}

// Widgets returns an iterator over all widgets (excluding system modules).
func (ctx *LintContext) Widgets() iter.Seq[Widget] {
	return func(yield func(Widget) bool) {
		rows, err := ctx.db.Query(`
			SELECT w.Id, w.Name, w.WidgetType, w.ContainerId, w.ContainerQualifiedName,
			       w.ContainerType, w.ModuleName, w.EntityRef, w.AttributeRef
			FROM widgets w
			LEFT JOIN modules m ON w.ModuleName = m.Name
			WHERE COALESCE(m.Source, '') = ''
			ORDER BY w.ModuleName, w.ContainerQualifiedName, w.Name
		`)
		if err != nil {
			return
		}
		defer rows.Close()

		for rows.Next() {
			var w Widget
			var containerID, containerQName, containerType, entityRef, attrRef sql.NullString
			err := rows.Scan(&w.ID, &w.Name, &w.WidgetType, &containerID, &containerQName,
				&containerType, &w.ModuleName, &entityRef, &attrRef)
			if err != nil {
				continue
			}
			w.ContainerID = containerID.String
			w.ContainerQualifiedName = containerQName.String
			w.ContainerType = containerType.String
			w.EntityRef = entityRef.String
			w.AttributeRef = attrRef.String

			if ctx.IsExcluded(w.ModuleName) {
				continue
			}

			if !yield(w) {
				return
			}
		}
	}
}

// Snippet represents a snippet from the catalog.
type Snippet struct {
	ID            string
	Name          string
	QualifiedName string
	ModuleName    string
	Folder        string
	WidgetCount   int
}

// Snippets returns an iterator over all snippets (excluding system modules).
func (ctx *LintContext) Snippets() iter.Seq[Snippet] {
	return func(yield func(Snippet) bool) {
		rows, err := ctx.db.Query(`
			SELECT s.Id, s.Name, s.QualifiedName, s.ModuleName, s.Folder, s.WidgetCount
			FROM snippets s
			LEFT JOIN modules m ON s.ModuleName = m.Name
			WHERE COALESCE(m.Source, '') = ''
			ORDER BY s.ModuleName, s.Name
		`)
		if err != nil {
			return
		}
		defer rows.Close()

		for rows.Next() {
			var s Snippet
			var folder sql.NullString
			var widgetCount sql.NullInt64
			err := rows.Scan(&s.ID, &s.Name, &s.QualifiedName, &s.ModuleName, &folder, &widgetCount)
			if err != nil {
				continue
			}
			s.Folder = folder.String
			s.WidgetCount = int(widgetCount.Int64)

			if ctx.IsExcluded(s.ModuleName) {
				continue
			}

			if !yield(s) {
				return
			}
		}
	}
}

// ScheduledEvent represents a scheduled event document.
type ScheduledEvent struct {
	Name            string
	QualifiedName   string
	ModuleName      string
	MicroflowName   string // qualified name of the microflow to execute
	IntervalSeconds int
	Enabled         bool
}

// intervalToSeconds converts a Mendix interval value and type to seconds.
// Returns 0 for unrecognised interval types (treated as "not convertible").
func intervalToSeconds(interval int, intervalType string) int {
	multipliers := map[string]int{
		"Second": 1,
		"Minute": 60,
		"Hour":   3600,
		"Day":    86400,
		"Week":   604800,
		"Month":  2592000,
		"Year":   31536000,
	}
	if mult, ok := multipliers[intervalType]; ok {
		return interval * mult
	}
	return 0
}

// ScheduledEvents returns an iterator over all scheduled events (excluding system modules).
// Returns an empty iterator if no reader is available.
func (ctx *LintContext) ScheduledEvents() iter.Seq[ScheduledEvent] {
	return func(yield func(ScheduledEvent) bool) {
		if ctx.reader == nil {
			return
		}

		// Build module ID → name map from catalog.
		moduleNames := map[model.ID]string{}
		// Build microflow UUID → qualified name map from catalog.
		// Falls back to the raw UUID when the catalog has not been built yet.
		microflowNames := map[string]string{}
		if ctx.db != nil {
			if rows, err := ctx.db.Query(`SELECT Id, Name FROM modules`); err == nil {
				defer rows.Close()
				for rows.Next() {
					var id, name string
					if rows.Scan(&id, &name) == nil {
						moduleNames[model.ID(id)] = name
					}
				}
			}
			if rows, err := ctx.db.Query(`SELECT Id, QualifiedName FROM microflows`); err == nil {
				defer rows.Close()
				for rows.Next() {
					var id, qname string
					if rows.Scan(&id, &qname) == nil {
						microflowNames[id] = qname
					}
				}
			}
		}

		events, err := ctx.reader.ListScheduledEvents()
		if err != nil {
			return
		}

		for _, e := range events {
			moduleName := moduleNames[e.ContainerID]
			if ctx.IsExcluded(moduleName) {
				continue
			}
			mfName := microflowNames[string(e.MicroflowID)]
			if mfName == "" {
				mfName = string(e.MicroflowID)
			}
			se := ScheduledEvent{
				Name:            e.Name,
				QualifiedName:   moduleName + "." + e.Name,
				ModuleName:      moduleName,
				MicroflowName:   mfName,
				IntervalSeconds: intervalToSeconds(e.Interval, e.IntervalType),
				Enabled:         e.Enabled,
			}
			if !yield(se) {
				return
			}
		}
	}
}

// XPathExpressionEntry represents an XPath constraint expression from the catalog.
type XPathExpressionEntry struct {
	ID                    string
	DocumentType          string // MICROFLOW, NANOFLOW, DOMAIN_MODEL, PAGE, SNIPPET
	DocumentID            string
	DocumentQualifiedName string
	ComponentType         string // RETRIEVE_ACTION, ACCESS_RULE, WIDGET
	ComponentID           string
	ComponentName         string
	XPathExpression       string // raw XPath string, may include outer [ ]
	TargetEntity          string // qualified name of entity being queried
	ReferencedEntities    string // comma-separated qualified names
	IsParameterized       bool   // true when XPath contains $variable references
	UsageType             string // RETRIEVE, SECURITY, DATASOURCE
	ModuleName            string
}

// XPathExpressions returns an iterator over all XPath expression entries in the catalog.
func (ctx *LintContext) XPathExpressions() iter.Seq[XPathExpressionEntry] {
	return func(yield func(XPathExpressionEntry) bool) {
		rows, err := ctx.db.Query(`
			SELECT Id, DocumentType, DocumentId, DocumentQualifiedName,
			       ComponentType, ComponentId, ComponentName,
			       XPathExpression, TargetEntity, ReferencedEntities,
			       IsParameterized, UsageType, ModuleName
			FROM xpath_expressions
			ORDER BY ModuleName, DocumentQualifiedName
		`)
		if err != nil {
			return
		}
		defer rows.Close()

		for rows.Next() {
			var e XPathExpressionEntry
			var componentName, targetEntity, refEntities, moduleName sql.NullString
			var isParam int
			err := rows.Scan(
				&e.ID, &e.DocumentType, &e.DocumentID, &e.DocumentQualifiedName,
				&e.ComponentType, &e.ComponentID, &componentName,
				&e.XPathExpression, &targetEntity, &refEntities,
				&isParam, &e.UsageType, &moduleName,
			)
			if err != nil {
				continue
			}
			e.ComponentName = componentName.String
			e.TargetEntity = targetEntity.String
			e.ReferencedEntities = refEntities.String
			e.ModuleName = moduleName.String
			e.IsParameterized = isParam == 1

			if ctx.IsExcluded(e.ModuleName) {
				continue
			}

			if !yield(e) {
				return
			}
		}
	}
}

// DatabaseConnection represents a database connection from the catalog.
type DatabaseConnection struct {
	ID            string
	Name          string
	QualifiedName string
	ModuleName    string
	Folder        string
	DatabaseType  string
	QueryCount    int
}

// DatabaseConnections returns an iterator over all database connections (excluding system modules).
func (ctx *LintContext) DatabaseConnections() iter.Seq[DatabaseConnection] {
	return func(yield func(DatabaseConnection) bool) {
		rows, err := ctx.db.Query(`
			SELECT dc.Id, dc.Name, dc.QualifiedName, dc.ModuleName, dc.Folder,
			       dc.DatabaseType, dc.QueryCount
			FROM database_connections dc
			LEFT JOIN modules m ON dc.ModuleName = m.Name
			WHERE COALESCE(m.Source, '') = ''
			ORDER BY dc.ModuleName, dc.Name
		`)
		if err != nil {
			return
		}
		defer rows.Close()

		for rows.Next() {
			var dc DatabaseConnection
			var folder sql.NullString
			err := rows.Scan(&dc.ID, &dc.Name, &dc.QualifiedName, &dc.ModuleName, &folder,
				&dc.DatabaseType, &dc.QueryCount)
			if err != nil {
				continue
			}
			dc.Folder = folder.String

			if ctx.IsExcluded(dc.ModuleName) {
				continue
			}

			if !yield(dc) {
				return
			}
		}
	}
}

// Activity represents an activity from the activities table (FULL catalog mode).
type Activity struct {
	ID                     string
	Name                   string
	Caption                string
	ActivityType           string
	ActionType             string
	MicroflowID            string
	MicroflowQualifiedName string
	ModuleName             string
	EntityRef              string
}

// ActivitiesFor returns an iterator over all activities for a given microflow.
func (ctx *LintContext) ActivitiesFor(microflowQualifiedName string) iter.Seq[Activity] {
	return func(yield func(Activity) bool) {
		rows, err := ctx.db.Query(`
			SELECT Id, Name, Caption, ActivityType, ActionType,
			       MicroflowId, MicroflowQualifiedName, ModuleName, EntityRef
			FROM activities
			WHERE MicroflowQualifiedName = ?
			ORDER BY Sequence
		`, microflowQualifiedName)
		if err != nil {
			return
		}
		defer rows.Close()

		for rows.Next() {
			var a Activity
			var name, caption, actionType, entityRef sql.NullString
			err := rows.Scan(&a.ID, &name, &caption, &a.ActivityType, &actionType,
				&a.MicroflowID, &a.MicroflowQualifiedName, &a.ModuleName, &entityRef)
			if err != nil {
				continue
			}
			a.Name = name.String
			a.Caption = caption.String
			a.ActionType = actionType.String
			a.EntityRef = entityRef.String

			if !yield(a) {
				return
			}
		}
	}
}

// Reference represents a reference from the refs table.
type Reference struct {
	SourceType string
	SourceID   string
	SourceName string
	TargetType string
	TargetID   string
	TargetName string
	RefKind    string
	ModuleName string
}

// HasRefsTable checks if the refs table has been populated.
func (ctx *LintContext) HasRefsTable() bool {
	var count int
	err := ctx.db.QueryRow("SELECT COUNT(*) FROM refs").Scan(&count)
	return err == nil && count > 0
}

// HasGraphTables reports whether the catalog carries the graph analysis data
// populated by REFRESH CATALOG COMMUNITIES (needed by the graph_* Starlark
// builtins). Like HasRefsTable, it returns false when the tables are absent or
// empty.
func (ctx *LintContext) HasGraphTables() bool {
	var count int
	err := ctx.db.QueryRow("SELECT COUNT(*) FROM graph_layers_data").Scan(&count)
	return err == nil && count > 0
}

// SatisfiesCatalogMode reports whether the catalog carries the data a rule
// declared it needs (mode). It is the safety net: even with auto-upgrade, a
// reused fast catalog that can't be rebuilt, or a project with no refs/graph
// rows, should surface a diagnostic rather than silently under-report.
func (ctx *LintContext) SatisfiesCatalogMode(mode CatalogMode) bool {
	switch mode {
	case CatalogCommunities:
		return ctx.HasGraphTables()
	case CatalogFull:
		return ctx.HasRefsTable()
	default:
		return true
	}
}

// FindReferences finds all references to a given element.
func (ctx *LintContext) FindReferences(targetName string) []Reference {
	var refs []Reference
	rows, err := ctx.db.Query(`
		SELECT SourceType, SourceId, SourceName, TargetType, TargetId, TargetName, RefKind, ModuleName
		FROM refs
		WHERE TargetName = ?
	`, targetName)
	if err != nil {
		return refs
	}
	defer rows.Close()

	for rows.Next() {
		var r Reference
		var srcID, tgtID sql.NullString
		err := rows.Scan(&r.SourceType, &srcID, &r.SourceName, &r.TargetType,
			&tgtID, &r.TargetName, &r.RefKind, &r.ModuleName)
		if err != nil {
			continue
		}
		r.SourceID = srcID.String
		r.TargetID = tgtID.String
		refs = append(refs, r)
	}
	return refs
}

// FindUnused finds elements with no incoming references.
func (ctx *LintContext) FindUnused(kind string) []string {
	var unused []string

	var query string
	switch kind {
	case "entity":
		query = `
			SELECT e.QualifiedName
			FROM entities e
			LEFT JOIN modules m ON e.ModuleName = m.Name
			WHERE COALESCE(m.Source, '') = ''
			AND e.QualifiedName NOT IN (
				SELECT DISTINCT TargetName FROM refs WHERE TargetType = 'ENTITY'
			)
		`
	case "microflow":
		query = `
			SELECT mf.QualifiedName
			FROM microflows mf
			LEFT JOIN modules m ON mf.ModuleName = m.Name
			WHERE COALESCE(m.Source, '') = ''
			AND mf.QualifiedName NOT IN (
				SELECT DISTINCT TargetName FROM refs WHERE TargetType IN ('MICROFLOW', 'NANOFLOW')
			)
		`
	case "page":
		query = `
			SELECT p.QualifiedName
			FROM pages p
			LEFT JOIN modules m ON p.ModuleName = m.Name
			WHERE COALESCE(m.Source, '') = ''
			AND p.QualifiedName NOT IN (
				SELECT DISTINCT TargetName FROM refs WHERE TargetType = 'PAGE'
			)
		`
	default:
		return unused
	}

	rows, err := ctx.db.Query(query)
	if err != nil {
		return unused
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			unused = append(unused, name)
		}
	}
	return unused
}

// ModuleDependencies returns a map of module -> modules it depends on.
func (ctx *LintContext) ModuleDependencies() map[string][]string {
	deps := make(map[string][]string)

	rows, err := ctx.db.Query(`
		SELECT DISTINCT ModuleName,
		       CASE
		           WHEN INSTR(TargetName, '.') > 0
		           THEN SUBSTR(TargetName, 1, INSTR(TargetName, '.') - 1)
		           ELSE ''
		       END as TargetModule
		FROM refs
		WHERE TargetName != '' AND ModuleName != ''
	`)
	if err != nil {
		return deps
	}
	defer rows.Close()

	for rows.Next() {
		var srcModule, tgtModule string
		if err := rows.Scan(&srcModule, &tgtModule); err != nil || tgtModule == "" || srcModule == tgtModule {
			continue
		}
		deps[srcModule] = append(deps[srcModule], tgtModule)
	}

	// Deduplicate
	for mod, targets := range deps {
		seen := make(map[string]bool)
		unique := []string{}
		for _, t := range targets {
			if !seen[t] {
				seen[t] = true
				unique = append(unique, t)
			}
		}
		deps[mod] = unique
	}

	return deps
}
