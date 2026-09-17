// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"database/sql"
	"fmt"
	"iter"
	"strings"
	"sync"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/security"
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

	// queryErrors collects catalog queries that failed, so an iterator that
	// could not run is distinguishable from one that legitimately found
	// nothing. See QueryError.
	queryErrors []QueryError
	queryErrMu  sync.Mutex
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
		rows, err := ctx.db.Query(fmt.Sprintf(`
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
			WHERE %s
			ORDER BY e.ModuleName, e.Name
		`, notPlatformModule("m")))
		if err != nil {
			ctx.recordQueryError("Entities", err)
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
				ctx.recordQueryError("Entities (row scan)", err)
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
			ctx.recordQueryError("AttributesFor", err)
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
				ctx.recordQueryError("AttributesFor (row scan)", err)
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
			ctx.recordQueryError("PermissionsFor", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var p Permission
			var memberName, xpathConstraint, moduleName sql.NullString
			err := rows.Scan(&p.ModuleRoleName, &p.EntityName, &memberName, &p.AccessType, &xpathConstraint, &moduleName)
			if err != nil {
				ctx.recordQueryError("PermissionsFor (row scan)", err)
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
			ctx.recordQueryError("Permissions", err)
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
			ctx.recordQueryError("RoleMappings", err)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var rm RoleMappingInfo
			if err := rows.Scan(&rm.UserRoleName, &rm.ModuleRoleName, &rm.ModuleName); err != nil {
				ctx.recordQueryError("RoleMappings (row scan)", err)
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
			ctx.recordQueryError("ModuleRoles", err)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var mr ModuleRoleInfo
			if err := rows.Scan(&mr.Name, &mr.ModuleName); err != nil {
				ctx.recordQueryError("ModuleRoles (row scan)", err)
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
	MicroflowType  string // "MICROFLOW", "NANOFLOW", "RULE" (as stored by the catalog)
	Description    string
	ReturnType     string
	ParameterCount int
	ActivityCount  int
	Complexity     int // McCabe cyclomatic complexity
}

// DocumentNoun is what to call this document in a lint message and in
// Location.DocumentType.
//
// Microflows() yields all three flow flavours, because they share one catalog
// table — so a rule that hardcodes "microflow" reports a nanoflow or a rule
// under the wrong doctype. That is how MPR002 came to say "Microflow 'Rule1'
// has no activities" about a rule, and the same about a nanoflow.
//
// An unrecognised type falls back to "microflow": a lint finding is worth more
// with an imprecise noun than not at all.
func (m Microflow) DocumentNoun() string {
	switch m.MicroflowType {
	case "NANOFLOW":
		return "nanoflow"
	case "RULE":
		return "rule"
	default:
		return "microflow"
	}
}

// DocumentNounTitle is DocumentNoun capitalised, for a message that opens with it.
func (m Microflow) DocumentNounTitle() string {
	n := m.DocumentNoun()
	return strings.ToUpper(n[:1]) + n[1:]
}

// Microflows returns an iterator over all microflows (excluding system modules).
func (ctx *LintContext) Microflows() iter.Seq[Microflow] {
	return func(yield func(Microflow) bool) {
		rows, err := ctx.db.Query(fmt.Sprintf(`
			SELECT mf.Id, mf.Name, mf.QualifiedName, mf.ModuleName, mf.Folder,
			       mf.MicroflowType, mf.Description, mf.ReturnType,
			       mf.ParameterCount, mf.ActivityCount, mf.Complexity
			FROM microflows mf
			LEFT JOIN modules m ON mf.ModuleName = m.Name
			WHERE %s
			ORDER BY mf.ModuleName, mf.Name
		`, notPlatformModule("m")))
		if err != nil {
			ctx.recordQueryError("Microflows", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var mf Microflow
			var desc, retType, folder sql.NullString
			err := rows.Scan(&mf.ID, &mf.Name, &mf.QualifiedName, &mf.ModuleName, &folder,
				&mf.MicroflowType, &desc, &retType, &mf.ParameterCount, &mf.ActivityCount, &mf.Complexity)
			if err != nil {
				ctx.recordQueryError("Microflows (row scan)", err)
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
		rows, err := ctx.db.Query(fmt.Sprintf(`
			SELECT p.Id, p.Name, p.QualifiedName, p.ModuleName, p.Folder,
			       p.Title, p.URL, p.Description, p.WidgetCount
			FROM pages p
			LEFT JOIN modules m ON p.ModuleName = m.Name
			WHERE %s
			ORDER BY p.ModuleName, p.Name
		`, notPlatformModule("m")))
		if err != nil {
			ctx.recordQueryError("Pages", err)
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
				ctx.recordQueryError("Pages (row scan)", err)
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
		rows, err := ctx.db.Query(fmt.Sprintf(`
			SELECT en.Id, en.Name, en.QualifiedName, en.ModuleName, en.Folder,
			       en.Description, en.ValueCount
			FROM enumerations en
			LEFT JOIN modules m ON en.ModuleName = m.Name
			WHERE %s
			ORDER BY en.ModuleName, en.Name
		`, notPlatformModule("m")))
		if err != nil {
			ctx.recordQueryError("Enumerations", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var en Enumeration
			var desc, folder sql.NullString
			err := rows.Scan(&en.ID, &en.Name, &en.QualifiedName, &en.ModuleName, &folder,
				&desc, &en.ValueCount)
			if err != nil {
				ctx.recordQueryError("Enumerations (row scan)", err)
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
		rows, err := ctx.db.Query(fmt.Sprintf(`
			SELECT c.Id, c.Name, c.QualifiedName, c.ModuleName, c.Folder,
			       c.Description, c.DefaultValue, c.ExposedToClient
			FROM constants c
			LEFT JOIN modules m ON c.ModuleName = m.Name
			WHERE %s
			ORDER BY c.ModuleName, c.Name
		`, notPlatformModule("m")))
		if err != nil {
			ctx.recordQueryError("Constants", err)
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
				ctx.recordQueryError("Constants (row scan)", err)
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
	MicroflowRef           string // Qualified name of an action/datasource microflow, if any
	NanoflowRef            string // Qualified name of an action/datasource nanoflow, if any
}

// Widgets returns an iterator over all widgets (excluding system modules).
func (ctx *LintContext) Widgets() iter.Seq[Widget] {
	return func(yield func(Widget) bool) {
		rows, err := ctx.db.Query(fmt.Sprintf(`
			SELECT w.Id, w.Name, w.WidgetType, w.ContainerId, w.ContainerQualifiedName,
			       w.ContainerType, w.ModuleName, w.EntityRef, w.AttributeRef,
			       w.MicroflowRef, w.NanoflowRef
			FROM widgets w
			LEFT JOIN modules m ON w.ModuleName = m.Name
			WHERE %s
			ORDER BY w.ModuleName, w.ContainerQualifiedName, w.Name
		`, notPlatformModule("m")))
		if err != nil {
			ctx.recordQueryError("Widgets", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var w Widget
			var containerID, containerQName, containerType, entityRef, attrRef, mfRef, nfRef sql.NullString
			err := rows.Scan(&w.ID, &w.Name, &w.WidgetType, &containerID, &containerQName,
				&containerType, &w.ModuleName, &entityRef, &attrRef, &mfRef, &nfRef)
			if err != nil {
				ctx.recordQueryError("Widgets (row scan)", err)
				continue
			}
			w.ContainerID = containerID.String
			w.ContainerQualifiedName = containerQName.String
			w.ContainerType = containerType.String
			w.EntityRef = entityRef.String
			w.AttributeRef = attrRef.String
			w.MicroflowRef = mfRef.String
			w.NanoflowRef = nfRef.String

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
		rows, err := ctx.db.Query(fmt.Sprintf(`
			SELECT s.Id, s.Name, s.QualifiedName, s.ModuleName, s.Folder, s.WidgetCount
			FROM snippets s
			LEFT JOIN modules m ON s.ModuleName = m.Name
			WHERE %s
			ORDER BY s.ModuleName, s.Name
		`, notPlatformModule("m")))
		if err != nil {
			ctx.recordQueryError("Snippets", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var s Snippet
			var folder sql.NullString
			var widgetCount sql.NullInt64
			err := rows.Scan(&s.ID, &s.Name, &s.QualifiedName, &s.ModuleName, &folder, &widgetCount)
			if err != nil {
				ctx.recordQueryError("Snippets (row scan)", err)
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
	Name          string
	QualifiedName string
	ModuleName    string
	MicroflowName string // qualified name of the microflow to execute
	// IntervalSeconds is how often the event fires, derived from the Schedule
	// child — NOT from the stored Interval/IntervalType pair, which Studio Pro
	// writes and does not keep in sync with Schedule. Workflow Commons ships an
	// event storing 0/"Minute" beside a DaySchedule of 01:00, so a rule keyed on
	// the legacy pair would read it as "fires every 0 seconds".
	IntervalSeconds int
	// Repeat is the schedule variant (Minute, Hour, Day, Week, MonthDate,
	// MonthWeekday, YearDate, YearWeekday); empty when the event has no
	// Schedule child.
	Repeat string
	// OnOverlap is DelayNext or SkipNext — the event's own concurrency control.
	OnOverlap string
	TimeZone  string
	Enabled   bool
}

// scheduleSeconds is the gap between runs implied by a schedule.
//
// The month and year figures are averages (30 and 365 days): the value is for
// thresholds and ordering ("anything that fires more often than a minute"), not
// for calendar arithmetic.
func scheduleSeconds(s *model.Schedule) int {
	if s == nil {
		return 0
	}
	mult := s.Multiplier
	if mult < 1 {
		mult = 1
	}
	const day = 86400
	switch s.Kind {
	case model.ScheduleMinute:
		return mult * 60
	case model.ScheduleHour:
		return mult * 3600
	case model.ScheduleDay:
		return day
	case model.ScheduleWeek:
		n := 0
		for _, on := range s.Weekdays {
			if on {
				n++
			}
		}
		if n == 0 {
			return 7 * day
		}
		return (7 * day) / n
	case model.ScheduleMonthDate, model.ScheduleMonthWeekday:
		return mult * 30 * day
	case model.ScheduleYearDate, model.ScheduleYearWeekday:
		return 365 * day
	}
	return 0
}

// Queue represents a task queue document.
type Queue struct {
	Name          string
	QualifiedName string
	ModuleName    string
	// Parallelism is an EXPRESSION, not a number — Mendix stores it as a string,
	// so a rule must not assume it parses as an integer.
	Parallelism string
	ClusterWide bool
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
			ctx.recordQueryError("ScheduledEvents", err)
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
			repeat := ""
			if e.Schedule != nil {
				repeat = string(e.Schedule.Kind)
			}
			se := ScheduledEvent{
				Name:            e.Name,
				QualifiedName:   moduleName + "." + e.Name,
				ModuleName:      moduleName,
				MicroflowName:   mfName,
				IntervalSeconds: scheduleSeconds(e.Schedule),
				Repeat:          repeat,
				OnOverlap:       e.OnOverlap,
				TimeZone:        e.TimeZone,
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
			ctx.recordQueryError("XPathExpressions", err)
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
				ctx.recordQueryError("XPathExpressions (row scan)", err)
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
// Queues returns an iterator over all task queues (excluding platform modules).
//
// Backed by the catalog rather than the reader, so no LintReader change is
// needed; the catalog is already built whenever rules run.
func (ctx *LintContext) Queues() iter.Seq[Queue] {
	return func(yield func(Queue) bool) {
		rows, err := ctx.db.Query(fmt.Sprintf(`
			SELECT q.Name, q.QualifiedName, q.ModuleName, q.Parallelism, q.ClusterWide
			FROM queues q
			LEFT JOIN modules m ON q.ModuleName = m.Name
			WHERE %s
			ORDER BY q.ModuleName, q.Name
		`, notPlatformModule("m")))
		if err != nil {
			ctx.recordQueryError("Queues", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var q Queue
			var clusterWide int
			if err := rows.Scan(&q.Name, &q.QualifiedName, &q.ModuleName, &q.Parallelism, &clusterWide); err != nil {
				ctx.recordQueryError("Queues (row scan)", err)
				continue
			}
			q.ClusterWide = clusterWide != 0
			if ctx.IsExcluded(q.ModuleName) {
				continue
			}
			if !yield(q) {
				return
			}
		}
	}
}

func (ctx *LintContext) DatabaseConnections() iter.Seq[DatabaseConnection] {
	return func(yield func(DatabaseConnection) bool) {
		rows, err := ctx.db.Query(fmt.Sprintf(`
			SELECT dc.Id, dc.Name, dc.QualifiedName, dc.ModuleName, dc.Folder,
			       dc.DatabaseType, dc.QueryCount
			FROM database_connections dc
			LEFT JOIN modules m ON dc.ModuleName = m.Name
			WHERE %s
			ORDER BY dc.ModuleName, dc.Name
		`, notPlatformModule("m")))
		if err != nil {
			ctx.recordQueryError("DatabaseConnections", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var dc DatabaseConnection
			var folder sql.NullString
			err := rows.Scan(&dc.ID, &dc.Name, &dc.QualifiedName, &dc.ModuleName, &folder,
				&dc.DatabaseType, &dc.QueryCount)
			if err != nil {
				ctx.recordQueryError("DatabaseConnections (row scan)", err)
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

// RestClient represents a consumed REST service document from the
// rest_clients table.
type RestClient struct {
	ID             string
	Name           string
	QualifiedName  string
	ModuleName     string
	Folder         string
	BaseUrl        string
	AuthScheme     string
	OperationCount int
	Documentation  string
}

// RestClients returns an iterator over all consumed REST services
// (excluding platform modules).
//
// Backed by the catalog rather than the reader, like DatabaseConnections.
func (ctx *LintContext) RestClients() iter.Seq[RestClient] {
	return func(yield func(RestClient) bool) {
		rows, err := ctx.db.Query(fmt.Sprintf(`
			SELECT rc.Id, rc.Name, rc.QualifiedName, rc.ModuleName, rc.Folder,
			       rc.BaseUrl, rc.AuthScheme, rc.OperationCount, rc.Documentation
			FROM rest_clients rc
			LEFT JOIN modules m ON rc.ModuleName = m.Name
			WHERE %s
			ORDER BY rc.ModuleName, rc.Name
		`, notPlatformModule("m")))
		if err != nil {
			ctx.recordQueryError("RestClients", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var rc RestClient
			var folder, baseURL, authScheme, documentation sql.NullString
			err := rows.Scan(&rc.ID, &rc.Name, &rc.QualifiedName, &rc.ModuleName, &folder,
				&baseURL, &authScheme, &rc.OperationCount, &documentation)
			if err != nil {
				ctx.recordQueryError("RestClients (row scan)", err)
				continue
			}
			rc.Folder = folder.String
			rc.BaseUrl = baseURL.String
			rc.AuthScheme = authScheme.String
			rc.Documentation = documentation.String

			if ctx.IsExcluded(rc.ModuleName) {
				continue
			}

			if !yield(rc) {
				return
			}
		}
	}
}

// RestOperation represents one operation on a consumed REST service.
type RestOperation struct {
	ID                   string
	ServiceID            string
	ServiceQualifiedName string
	Name                 string
	HttpMethod           string
	Path                 string
	ParameterCount       int
	HasBody              bool
	ResponseType         string
	// Timeout is the configured timeout in milliseconds; 0 when none is set.
	Timeout    int
	ModuleName string
}

// RestOperations returns an iterator over all consumed REST operations
// (excluding platform modules).
func (ctx *LintContext) RestOperations() iter.Seq[RestOperation] {
	return func(yield func(RestOperation) bool) {
		rows, err := ctx.db.Query(fmt.Sprintf(`
			SELECT ro.Id, ro.ServiceId, ro.ServiceQualifiedName, ro.Name,
			       ro.HttpMethod, ro.Path, ro.ParameterCount, ro.HasBody,
			       ro.ResponseType, ro.Timeout, ro.ModuleName
			FROM rest_operations ro
			LEFT JOIN modules m ON ro.ModuleName = m.Name
			WHERE %s
			ORDER BY ro.ServiceQualifiedName, ro.Name
		`, notPlatformModule("m")))
		if err != nil {
			ctx.recordQueryError("RestOperations", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var ro RestOperation
			var hasBody int
			var httpMethod, path, responseType sql.NullString
			err := rows.Scan(&ro.ID, &ro.ServiceID, &ro.ServiceQualifiedName, &ro.Name,
				&httpMethod, &path, &ro.ParameterCount, &hasBody,
				&responseType, &ro.Timeout, &ro.ModuleName)
			if err != nil {
				ctx.recordQueryError("RestOperations (row scan)", err)
				continue
			}
			ro.HttpMethod = httpMethod.String
			ro.Path = path.String
			ro.ResponseType = responseType.String
			ro.HasBody = hasBody != 0

			if ctx.IsExcluded(ro.ModuleName) {
				continue
			}

			if !yield(ro) {
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
			ctx.recordQueryError("ActivitiesFor", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var a Activity
			var name, caption, actionType, entityRef sql.NullString
			err := rows.Scan(&a.ID, &name, &caption, &a.ActivityType, &actionType,
				&a.MicroflowID, &a.MicroflowQualifiedName, &a.ModuleName, &entityRef)
			if err != nil {
				ctx.recordQueryError("ActivitiesFor (row scan)", err)
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
			ctx.recordQueryError("FindReferences (row scan)", err)
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
		query = fmt.Sprintf(`
			SELECT e.QualifiedName
			FROM entities e
			LEFT JOIN modules m ON e.ModuleName = m.Name
			WHERE %s
			AND e.QualifiedName NOT IN (
				SELECT DISTINCT TargetName FROM refs WHERE TargetType = 'ENTITY'
			)
		`, notPlatformModule("m"))
	case "microflow":
		query = fmt.Sprintf(`
			SELECT mf.QualifiedName
			FROM microflows mf
			LEFT JOIN modules m ON mf.ModuleName = m.Name
			WHERE %s
			AND mf.QualifiedName NOT IN (
				SELECT DISTINCT TargetName FROM refs WHERE TargetType IN ('MICROFLOW', 'NANOFLOW')
			)
		`, notPlatformModule("m"))
	case "page":
		query = fmt.Sprintf(`
			SELECT p.QualifiedName
			FROM pages p
			LEFT JOIN modules m ON p.ModuleName = m.Name
			WHERE %s
			AND p.QualifiedName NOT IN (
				SELECT DISTINCT TargetName FROM refs WHERE TargetType = 'PAGE'
			)
		`, notPlatformModule("m"))
	default:
		return unused
	}

	rows, err := ctx.db.Query(query)
	if err != nil {
		ctx.recordQueryError("FindUnused("+kind+")", err)
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

// JavaActionParameter represents one parameter of a Java action.
type JavaActionParameter struct {
	Name          string
	Description   string
	ParameterType string
	IsRequired    bool
}

// JavaAction represents a Java action from the catalog, with its parameters.
//
// Parameters are carried on the action rather than offered as a separate
// iterator: a rule that reports an undocumented parameter has to name the action
// it belongs to, and pairing them here keeps a rule from having to join.
type JavaAction struct {
	ID            string
	Name          string
	QualifiedName string
	ModuleName    string
	Folder        string
	Documentation string
	ExportLevel   string
	ReturnType    string
	Parameters    []JavaActionParameter
}

// JavaActions returns an iterator over all Java actions (excluding system and
// marketplace modules, as every other document iterator here does — a rule must
// not report undocumented code the user did not write).
func (ctx *LintContext) JavaActions() iter.Seq[JavaAction] {
	return func(yield func(JavaAction) bool) {
		params := ctx.javaActionParameters()

		rows, err := ctx.db.Query(fmt.Sprintf(`
			SELECT ja.Id, ja.Name, ja.QualifiedName, ja.ModuleName, ja.Folder,
			       ja.Documentation, ja.ExportLevel, ja.ReturnType
			FROM java_actions ja
			LEFT JOIN modules m ON ja.ModuleName = m.Name
			WHERE %s
			ORDER BY ja.ModuleName, ja.Name
		`, notPlatformModule("m")))
		if err != nil {
			ctx.recordQueryError("JavaActions", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var ja JavaAction
			var folder, doc, exportLevel, retType sql.NullString
			if err := rows.Scan(&ja.ID, &ja.Name, &ja.QualifiedName, &ja.ModuleName,
				&folder, &doc, &exportLevel, &retType); err != nil {
				continue
			}
			ja.Folder = folder.String
			ja.Documentation = doc.String
			ja.ExportLevel = exportLevel.String
			ja.ReturnType = retType.String
			ja.Parameters = params[ja.ID]

			if ctx.IsExcluded(ja.ModuleName) {
				continue
			}
			if !yield(ja) {
				return
			}
		}
	}
}

// javaActionParameters indexes every parameter by its owning action's ID, in
// declaration order.
func (ctx *LintContext) javaActionParameters() map[string][]JavaActionParameter {
	out := map[string][]JavaActionParameter{}
	rows, err := ctx.db.Query(`
		SELECT JavaActionId, Name, Description, ParameterType, IsRequired
		FROM java_action_parameters
		ORDER BY JavaActionId, Ordinal
	`)
	if err != nil {
		return out
	}
	defer rows.Close()

	for rows.Next() {
		var actionID string
		var p JavaActionParameter
		var desc, ptype sql.NullString
		var required int
		if err := rows.Scan(&actionID, &p.Name, &desc, &ptype, &required); err != nil {
			ctx.recordQueryError("javaActionParameters (row scan)", err)
			continue
		}
		p.Description = desc.String
		p.ParameterType = ptype.String
		p.IsRequired = required != 0
		out[actionID] = append(out[actionID], p)
	}
	return out
}

// QueryError records a catalog query that failed inside an iterator.
//
// The iterators return iter.Seq[T] with no error channel, so a failed query
// used to be indistinguishable from an empty result: `if err != nil { return }`
// yielded nothing and said nothing. That is the worst shape for a linter, whose
// entire output is "here is what I found" — a broken query reads as a clean
// project. Errors are collected here and surfaced by the runner instead.
type QueryError struct {
	Iterator string // the iterator that failed, e.g. "Entities"
	Err      error
}

func (e QueryError) Error() string { return e.Iterator + ": " + e.Err.Error() }

// recordQueryError notes a failed query. Iterators still degrade to "no rows"
// so one broken query cannot take down the whole run, but the failure is no
// longer silent.
//
// The mutex is not currently required (Linter.Run is sequential) but the Linter
// carries a maxWorkers knob, so this stays safe if rules ever run in parallel.
func (ctx *LintContext) recordQueryError(iterator string, err error) {
	if err == nil {
		return
	}
	ctx.queryErrMu.Lock()
	defer ctx.queryErrMu.Unlock()
	// Deduplicate: several rules iterate the same accessor, so one broken view
	// would otherwise be reported once per rule. The fact matters, not how many
	// rules tripped over it.
	for _, existing := range ctx.queryErrors {
		if existing.Iterator == iterator && existing.Err.Error() == err.Error() {
			return
		}
	}
	ctx.queryErrors = append(ctx.queryErrors, QueryError{Iterator: iterator, Err: err})
}

// QueryErrors returns every catalog query that failed during this lint run.
// A non-empty result means the findings are incomplete.
func (ctx *LintContext) QueryErrors() []QueryError {
	ctx.queryErrMu.Lock()
	defer ctx.queryErrMu.Unlock()
	return append([]QueryError(nil), ctx.queryErrors...)
}

// systemModuleID is the fixed sentinel Mendix gives the built-in System module
// (mirrors modelsdk/meta.SystemModuleID, not imported to keep the linter free of
// an engine dependency).
//
// System is NOT distinguishable by modules.Source — that column carries
// "Marketplace …" for downloaded modules and is empty for System exactly as it
// is for the user's own modules. Filtering on Source alone therefore lets every
// System element through, which is how QUAL002 came to report FileDocument,
// HttpRequest and 35 other platform entities as undocumented.
const systemModuleID = "00000000-0000-0000-0000-000000000001"

// notPlatformModule is the WHERE fragment that keeps a query to modules the user
// actually owns: not downloaded from the Marketplace, and not System.
//
// `alias` is the modules-table alias to test.
func notPlatformModule(alias string) string {
	return fmt.Sprintf(
		`COALESCE(%[1]s.Source, '') = '' AND COALESCE(%[1]s.Id, '') <> '%[2]s'`,
		alias, systemModuleID)
}

// Documentable is one model element that can carry documentation, projected
// uniformly across catalog tables so a rule can sweep every document type
// without a query per kind.
type Documentable struct {
	Kind          string // Mendix term: "Page", "Enumeration", "Workflow", …
	Name          string
	QualifiedName string
	ModuleName    string
	Description   string
}

// documentableSource maps a catalog table to the Mendix term for what it holds
// and the column its documentation lives in.
//
// The doc column is NOT uniform — Mendix says "Documentation" for some element
// types and "Description" for others, and the catalog faithfully mirrors that.
// A sweep that assumes one spelling silently reports every element of the other
// half as undocumented.
type documentableSource struct {
	Table  string
	Kind   string
	DocCol string
}

// documentableSources is every document type a user authors and can document.
//
// Deliberately absent, and why:
//   - microflows, java_actions — swept by the rule separately, because they
//     carry exemptions (activity thresholds) and children (parameters) that a
//     uniform projection cannot express.
//   - attributes, java_action_parameters — members of a document, not
//     documents; the rule checks them under their own options.
//   - activities, widgets, widget_definition_properties, xpath_expressions —
//     sub-elements INSIDE a document. Mendix offers no documentation field for
//     most of them, and flagging every widget would drown the rule.
//   - contract_entities — generated from a remote service's $metadata. Not the
//     user's text to write, so not the user's omission to report.
//   - modules — a Mendix module HAS no documentation property, so the rule was
//     asking for something no editor can supply. Measured three ways: the
//     metamodel's ProjectsModule declares none, modelsdk/gen's Module offers no
//     Documentation accessor, and none of a real project's stored
//     Projects$ModuleImpl units contains the key. It reported every module of
//     every project forever, which is what a report of 39 unanswerable warnings
//     looks like from the inside (ako/CapTrackV4 R12).
var documentableSources = []documentableSource{
	{"entities", "Entity", "Description"},
	{"associations", "Association", "Description"},
	{"pages", "Page", "Description"},
	{"snippets", "Snippet", "Description"},
	{"building_blocks", "BuildingBlock", "Description"},
	{"layouts", "Layout", "Description"},
	{"enumerations", "Enumeration", "Description"},
	{"javascript_actions", "JavaScriptAction", "Description"},
	{"image_collections", "ImageCollection", "Description"},
	{"data_transformers", "DataTransformer", "Description"},
	{"workflows", "Workflow", "Description"},
	{"business_event_services", "BusinessEventService", "Documentation"},
	{"rest_clients", "RestClient", "Documentation"},
	{"published_rest_services", "PublishedRestService", "Documentation"},
	{"constants", "Constant", "Description"},
	{"json_structures", "JsonStructure", "Documentation"},
	{"import_mappings", "ImportMapping", "Documentation"},
	{"export_mappings", "ExportMapping", "Documentation"},
}

// DocumentableKinds returns the Kind of every source, for tests and for the
// `mxcli lint` help text to stay in step with the code.
func DocumentableKinds() []string {
	kinds := make([]string, 0, len(documentableSources))
	for _, s := range documentableSources {
		kinds = append(kinds, s.Kind)
	}
	return kinds
}

// DocumentableElements iterates every documentable element across all document
// types, excluding System and Marketplace modules.
//
// Each source is queried separately rather than UNIONed so that a catalog
// missing one table (an older cache, or a Mendix version without that document
// type) loses only that kind instead of the whole sweep.
func (ctx *LintContext) DocumentableElements() iter.Seq[Documentable] {
	return func(yield func(Documentable) bool) {
		for _, src := range documentableSources {
			// The modules table has no module to join to — it IS the module
			// list — so it filters on its own Source column.
			query := fmt.Sprintf(`
				SELECT t.Name, t.QualifiedName, t.ModuleName, COALESCE(t.%s, '')
				FROM %s t
				LEFT JOIN modules m ON t.ModuleName = m.Name
				WHERE %s
				ORDER BY t.ModuleName, t.Name
			`, src.DocCol, src.Table, notPlatformModule("m"))
			if src.Table == "modules" {
				query = fmt.Sprintf(`
					SELECT t.Name, t.QualifiedName, t.Name, COALESCE(t.%s, '')
					FROM %s t
					WHERE %s
					ORDER BY t.Name
				`, src.DocCol, src.Table, notPlatformModule("t"))
			}

			rows, err := ctx.db.Query(query)
			if err != nil {
				// Skip this kind only — one absent table must not take down the
				// whole sweep — but say so, because "this document type has no
				// undocumented elements" and "this document type could not be
				// read" look identical in the output otherwise.
				ctx.recordQueryError("DocumentableElements("+src.Table+")", err)
				continue
			}
			for rows.Next() {
				d := Documentable{Kind: src.Kind}
				var qn, mod sql.NullString
				if err := rows.Scan(&d.Name, &qn, &mod, &d.Description); err != nil {
					ctx.recordQueryError("DocumentableElements("+src.Table+") row scan", err)
					continue
				}
				d.QualifiedName = qn.String
				d.ModuleName = mod.String
				if d.QualifiedName == "" {
					d.QualifiedName = d.Name
				}
				if ctx.IsExcluded(d.ModuleName) {
					continue
				}
				if !yield(d) {
					rows.Close()
					return
				}
			}
			rows.Close()
		}
	}
}

// NavigationTarget is one page a navigation profile sends a user to: the
// profile's home page, a role-specific home page, or a menu item's target.
type NavigationTarget struct {
	Profile string // "Responsive", "Phone", "Tablet", …
	Kind    string // "home", "role_home" or "menu"
	Role    string // user role, for a role_home; "" otherwise
	Caption string // menu item caption, for a menu target; "" otherwise
	Page    string // qualified page name
}

// NavigationTargets iterates every page a navigation profile routes to.
//
// The login page and the not-found page are deliberately NOT targets: the
// platform routes to those itself (/login.html, and the 404 handler), so they
// are not screens a user navigates or links to.
//
// Microflow-valued menu items and home pages are skipped — the microflow decides
// what to open, so there is no page here to say anything about. Targets in
// System and Marketplace modules are not filtered here; a caller that joins to
// Pages() gets that exclusion for free, and one that does not is asking about
// navigation itself.
func (ctx *LintContext) NavigationTargets() iter.Seq[NavigationTarget] {
	return func(yield func(NavigationTarget) bool) {
		rows, err := ctx.db.Query(`
			SELECT ProfileName, 'home', '', '', HomePage
			FROM navigation_profiles
			WHERE HomePageType = 'PAGE' AND COALESCE(HomePage, '') <> ''
			UNION ALL
			SELECT ProfileName, 'role_home', UserRole, '', Page
			FROM navigation_role_homes
			WHERE COALESCE(Page, '') <> ''
			UNION ALL
			SELECT ProfileName, 'menu', '', COALESCE(Caption, ''), TargetPage
			FROM navigation_menu_items
			WHERE COALESCE(TargetPage, '') <> ''
			ORDER BY 1, 2, 5
		`)
		if err != nil {
			ctx.recordQueryError("NavigationTargets", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var t NavigationTarget
			if err := rows.Scan(&t.Profile, &t.Kind, &t.Role, &t.Caption, &t.Page); err != nil {
				ctx.recordQueryError("NavigationTargets row scan", err)
				continue
			}
			if !yield(t) {
				return
			}
		}
	}
}

// Document is one element of the App Explorer tree — what it is, which module
// holds it, and the folder path it sits in ("" for the module root).
//
// Distinct from Documentable above, which projects only what can carry
// DOCUMENTATION and therefore leaves out microflows and Java actions on
// purpose. A rule asking where a document LIVES needs those two most of all:
// they are what fills up an unorganised module.
type Document struct {
	Kind          string // catalog ObjectType: "MICROFLOW", "PAGE", "WORKFLOW", …
	Name          string
	QualifiedName string
	ModuleName    string
	Folder        string // "" = directly in the module root
}

// Documents iterates every element of the catalog's `objects` view that belongs
// to a module, excluding System and Marketplace modules.
//
// It reads the view rather than a list of tables here, so a document type added
// to the catalog is covered without a second list to keep in step — the mistake
// behind #1036, where two keyword lists drifted with nothing comparing them.
//
// MODULE rows are skipped: a module is the container, not something inside one.
// Every other kind is yielded as-is, including kinds the view hardcodes to an
// empty Folder (associations, external entities). Deciding which kinds Studio
// Pro actually lets you file in a folder is left to the caller, where it is
// visible and editable, rather than baked into a second list in here.
func (ctx *LintContext) Documents() iter.Seq[Document] {
	return func(yield func(Document) bool) {
		rows, err := ctx.db.Query(`
			SELECT o.ObjectType, o.Name, o.QualifiedName, o.ModuleName, COALESCE(o.Folder, '')
			FROM objects o
			LEFT JOIN modules m ON o.ModuleName = m.Name
			WHERE o.ObjectType <> 'MODULE'
			  AND COALESCE(o.ModuleName, '') <> ''
			  AND ` + notPlatformModule("m") + `
			ORDER BY o.ModuleName, o.ObjectType, o.Name
		`)
		if err != nil {
			ctx.recordQueryError("Documents(objects)", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var d Document
			var qn sql.NullString
			if err := rows.Scan(&d.Kind, &d.Name, &qn, &d.ModuleName, &d.Folder); err != nil {
				ctx.recordQueryError("Documents(objects) row scan", err)
				continue
			}
			d.QualifiedName = qn.String
			if d.QualifiedName == "" {
				d.QualifiedName = d.Name
			}
			if ctx.IsExcluded(d.ModuleName) {
				continue
			}
			if !yield(d) {
				return
			}
		}
	}
}
