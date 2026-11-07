// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"database/sql"
	"fmt"
	"iter"
	"strings"
)

// Typed iterators over catalog tables that existed before any Starlark builtin
// read them (mendixlabs/mxcli#1265). Each one is a projection with stable,
// documented field names rather than the raw columns: catalog column names
// change between schema versions, a rule's field names must not.
//
// Every iterator filters like its neighbours: System and Marketplace modules
// are left out in SQL (notPlatformModule), --modules / --exclude-modules in Go
// (IsExcluded), and a document-scoped iterator honours --document
// (documentFilterSQL). A failed query is recorded, never swallowed.

// Association is one association, same-module or cross-module.
//
// FromEntity / ToEntity are qualified names in MDL's FROM / TO sense. The
// delete behaviours are the raw Mendix values, named by the same ends:
// ToDeleteBehavior is Mendix's ChildDeleteBehavior (the end MDL's
// `on delete` clause sets), FromDeleteBehavior its ParentDeleteBehavior — the
// pointer names are inverted relative to MDL (see CLAUDE.md).
type Association struct {
	Name                   string
	QualifiedName          string
	ModuleName             string
	FromEntity             string
	ToEntity               string
	Type                   string // "Reference" or "ReferenceSet"
	Owner                  string // "Default" or "Both"
	StorageFormat          string // "Column" or "Table"
	Description            string
	ToDeleteBehavior       string
	FromDeleteBehavior     string
	ToDeleteErrorMessage   string
	FromDeleteErrorMessage string
}

// Associations iterates every association outside System and Marketplace modules.
func (ctx *LintContext) Associations() iter.Seq[Association] {
	return func(yield func(Association) bool) {
		rows, err := ctx.db.Query(fmt.Sprintf(`
			SELECT a.Name, a.QualifiedName, a.ModuleName,
			       COALESCE(a.FromEntity, ''), COALESCE(a.ToEntity, ''),
			       COALESCE(a.AssociationType, ''), COALESCE(a.Owner, ''),
			       COALESCE(a.StorageFormat, ''), COALESCE(a.Description, ''),
			       COALESCE(a.ToDeleteBehavior, ''), COALESCE(a.FromDeleteBehavior, ''),
			       COALESCE(a.ToDeleteErrorMessage, ''), COALESCE(a.FromDeleteErrorMessage, '')
			FROM associations a
			LEFT JOIN modules m ON a.ModuleName = m.Name
			WHERE %s
			ORDER BY a.ModuleName, a.Name
		`, notPlatformModule("m")))
		if err != nil {
			ctx.recordQueryError("Associations", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var a Association
			if err := rows.Scan(&a.Name, &a.QualifiedName, &a.ModuleName,
				&a.FromEntity, &a.ToEntity, &a.Type, &a.Owner, &a.StorageFormat, &a.Description,
				&a.ToDeleteBehavior, &a.FromDeleteBehavior,
				&a.ToDeleteErrorMessage, &a.FromDeleteErrorMessage); err != nil {
				ctx.recordQueryError("Associations (row scan)", err)
				continue
			}
			if ctx.IsExcluded(a.ModuleName) {
				continue
			}
			if !yield(a) {
				return
			}
		}
	}
}

// EntityEventHandler is one before/after event handler on an entity.
type EntityEventHandler struct {
	Entity            string // qualified entity name
	ModuleName        string
	Moment            string // "Before" or "After"
	Event             string // "Create", "Commit", "Delete" or "RollBack" (Mendix's capital B)
	Microflow         string // qualified microflow name
	RaiseErrorOnFalse bool
	PassEventObject   bool
}

// EntityEventHandlers iterates every entity event handler outside System and
// Marketplace modules.
func (ctx *LintContext) EntityEventHandlers() iter.Seq[EntityEventHandler] {
	return func(yield func(EntityEventHandler) bool) {
		rows, err := ctx.db.Query(fmt.Sprintf(`
			SELECT h.EntityQualifiedName, h.ModuleName, COALESCE(h.Moment, ''),
			       COALESCE(h.Event, ''), COALESCE(h.Microflow, ''),
			       COALESCE(h.RaiseErrorOnFalse, 0), COALESCE(h.PassEventObject, 0)
			FROM entity_event_handlers h
			LEFT JOIN modules m ON h.ModuleName = m.Name
			WHERE %s
			ORDER BY h.ModuleName, h.EntityQualifiedName, h.Moment, h.Event
		`, notPlatformModule("m")))
		if err != nil {
			ctx.recordQueryError("EntityEventHandlers", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var h EntityEventHandler
			var raise, pass int64
			if err := rows.Scan(&h.Entity, &h.ModuleName, &h.Moment, &h.Event,
				&h.Microflow, &raise, &pass); err != nil {
				ctx.recordQueryError("EntityEventHandlers (row scan)", err)
				continue
			}
			h.RaiseErrorOnFalse = raise != 0
			h.PassEventObject = pass != 0
			if ctx.IsExcluded(h.ModuleName) {
				continue
			}
			if !yield(h) {
				return
			}
		}
	}
}

// NavigationMenuItem is one item of a navigation profile's menu, at any depth.
type NavigationMenuItem struct {
	Profile         string
	ItemPath        string // "0", "0.2", … — position, dot-separated per level
	Depth           int    // 0 for a top-level item
	Caption         string
	ActionType      string // "PageAction", "MicroflowAction", "SignOutAction", "OpenLinkAction", "NoAction", or the stored $Type
	TargetPage      string // qualified page name, for a PageAction
	TargetMicroflow string // qualified microflow name, for a MicroflowAction
}

// NavigationMenuItems iterates every navigation menu item. Navigation belongs
// to the project, not to a module, so — like NavigationTargets — no module
// filter applies; a rule that cares where the target lives joins it itself.
func (ctx *LintContext) NavigationMenuItems() iter.Seq[NavigationMenuItem] {
	return func(yield func(NavigationMenuItem) bool) {
		rows, err := ctx.db.Query(`
			SELECT ProfileName, ItemPath, COALESCE(Depth, 0), COALESCE(Caption, ''),
			       COALESCE(ActionType, ''), COALESCE(TargetPage, ''), COALESCE(TargetMicroflow, '')
			FROM navigation_menu_items
			ORDER BY ProfileName, Id
		`)
		if err != nil {
			ctx.recordQueryError("NavigationMenuItems", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var n NavigationMenuItem
			if err := rows.Scan(&n.Profile, &n.ItemPath, &n.Depth, &n.Caption,
				&n.ActionType, &n.TargetPage, &n.TargetMicroflow); err != nil {
				ctx.recordQueryError("NavigationMenuItems (row scan)", err)
				continue
			}
			if !yield(n) {
				return
			}
		}
	}
}

// JarDependency is one Maven dependency a module declares.
type JarDependency struct {
	ModuleName string
	GroupID    string
	ArtifactID string
	Version    string
	Coordinate string // "groupId:artifactId" -- no version, so two versions of one library share it
	IsIncluded bool
}

// JarDependencies iterates the JAR dependencies of modules outside System and
// the Marketplace.
func (ctx *LintContext) JarDependencies() iter.Seq[JarDependency] {
	return func(yield func(JarDependency) bool) {
		rows, err := ctx.db.Query(fmt.Sprintf(`
			SELECT j.ModuleName, COALESCE(j.GroupId, ''), COALESCE(j.ArtifactId, ''),
			       COALESCE(j.Version, ''), COALESCE(j.Coordinate, ''), COALESCE(j.IsIncluded, 1)
			FROM jar_dependencies j
			LEFT JOIN modules m ON j.ModuleName = m.Name
			WHERE %s
			ORDER BY j.ModuleName, j.Coordinate
		`, notPlatformModule("m")))
		if err != nil {
			ctx.recordQueryError("JarDependencies", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var j JarDependency
			var included int64
			if err := rows.Scan(&j.ModuleName, &j.GroupID, &j.ArtifactID,
				&j.Version, &j.Coordinate, &included); err != nil {
				ctx.recordQueryError("JarDependencies (row scan)", err)
				continue
			}
			j.IsIncluded = included != 0
			if ctx.IsExcluded(j.ModuleName) {
				continue
			}
			if !yield(j) {
				return
			}
		}
	}
}

// CatalogString is one row of the catalog's full-text `strings` table: a piece
// of user-facing or documentary text and where it lives. Filled only by a FULL
// catalog build.
type CatalogString struct {
	QualifiedName string // the document the text belongs to
	ObjectType    string // catalog object type, upper-case: "PAGE", "MICROFLOW", …
	Value         string
	Context       string // what the text is: "Forms$Page.Title" for translatable text, "documentation", "page_url", …
	Language      string // "" for text that is not translatable
	ElementID     string
	ModuleName    string
}

// Strings iterates the catalog's strings outside System and Marketplace modules,
// narrowed to one language when language is non-empty. A language a text was
// never translated into has no row — the absence, not an empty value, is how
// "missing translation" reads.
func (ctx *LintContext) Strings(language string) iter.Seq[CatalogString] {
	return func(yield func(CatalogString) bool) {
		langFilter := "1=1"
		var args []any
		if language != "" {
			langFilter = "s.Language = ?"
			args = append(args, language)
		}
		rows, err := ctx.db.Query(fmt.Sprintf(`
			SELECT COALESCE(s.QualifiedName, ''), COALESCE(s.ObjectType, ''),
			       COALESCE(s.StringValue, ''), COALESCE(s.StringContext, ''),
			       COALESCE(s.Language, ''), COALESCE(s.ElementId, ''), COALESCE(s.ModuleName, '')
			FROM strings s
			LEFT JOIN modules m ON s.ModuleName = m.Name
			WHERE %s AND %s AND %s
			ORDER BY s.ModuleName, s.QualifiedName
		`, notPlatformModule("m"), ctx.documentFilterSQL("s.QualifiedName"), langFilter), args...)
		if err != nil {
			ctx.recordQueryError("Strings", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var s CatalogString
			if err := rows.Scan(&s.QualifiedName, &s.ObjectType, &s.Value, &s.Context,
				&s.Language, &s.ElementID, &s.ModuleName); err != nil {
				ctx.recordQueryError("Strings (row scan)", err)
				continue
			}
			if ctx.IsExcluded(s.ModuleName) {
				continue
			}
			if !yield(s) {
				return
			}
		}
	}
}

// Layout is one page layout document.
type Layout struct {
	Name          string
	QualifiedName string
	ModuleName    string
	Folder        string
	LayoutType    string // "Responsive", "Phone", "Tablet", "Popup", "ModalPopup", "Default", "Legacy"
	Platform      string // "Web" or "Native" — LayoutType alone cannot tell ("Popup" is native)
	Description   string
	// The layout's own Appearance (full catalog only).
	Class          string
	Style          string
	DynamicClasses string
}

// Layouts iterates every layout outside System and Marketplace modules.
func (ctx *LintContext) Layouts() iter.Seq[Layout] {
	return func(yield func(Layout) bool) {
		rows, err := ctx.db.Query(fmt.Sprintf(`
			SELECT l.Name, l.QualifiedName, l.ModuleName, COALESCE(l.Folder, ''),
			       COALESCE(l.LayoutType, ''), COALESCE(l.Platform, ''), COALESCE(l.Description, ''),
			       COALESCE(l.Class, ''), COALESCE(l.Style, ''), COALESCE(l.DynamicClasses, '')
			FROM layouts l
			LEFT JOIN modules m ON l.ModuleName = m.Name
			WHERE %s AND %s
			ORDER BY l.ModuleName, l.Name
		`, notPlatformModule("m"), ctx.documentFilterSQL("l.QualifiedName")))
		if err != nil {
			ctx.recordQueryError("Layouts", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var l Layout
			if err := rows.Scan(&l.Name, &l.QualifiedName, &l.ModuleName, &l.Folder,
				&l.LayoutType, &l.Platform, &l.Description,
				&l.Class, &l.Style, &l.DynamicClasses); err != nil {
				ctx.recordQueryError("Layouts (row scan)", err)
				continue
			}
			if ctx.IsExcluded(l.ModuleName) {
				continue
			}
			if !yield(l) {
				return
			}
		}
	}
}

// NativePages returns the qualified names of the pages whose layout is a
// native-mobile one, and of the native layouts themselves (whose widgets the
// catalog records too, ako/mxcli#1302). React-client rules (CE0582) do not apply to them: a native
// page is not rendered by the React client, and mxbuild builds a legacy widget
// there clean (ako/mxcli#953).
//
// Unlike Layouts() it does NOT filter Marketplace modules out of the layouts:
// the native layouts almost every app uses live in Atlas_Core, and a page in a
// user module on one of them is the case that matters. The page's layout is
// recorded by a FULL catalog build only (pages.LayoutRef), so a rule calling
// this needs CatalogFull; on a fast catalog it returns an empty set, and the
// rule behaves as it did before.
func (ctx *LintContext) NativePages() map[string]bool {
	out := map[string]bool{}
	rows, err := ctx.db.Query(`
		SELECT p.QualifiedName
		FROM pages p
		JOIN layouts l ON l.QualifiedName = p.LayoutRef
		WHERE l.Platform = 'Native'
		UNION
		SELECT QualifiedName FROM layouts WHERE Platform = 'Native'
	`)
	if err != nil {
		ctx.recordQueryError("NativePages", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var qn string
		if err := rows.Scan(&qn); err != nil {
			ctx.recordQueryError("NativePages (row scan)", err)
			continue
		}
		out[qn] = true
	}
	return out
}

// PublishedRestOperation is one operation of a published REST service.
type PublishedRestOperation struct {
	Service    string // qualified service name
	Resource   string
	HTTPMethod string // as Mendix stores it: "Get", "Post", "Put", "Patch", "Delete", …
	Path       string
	Summary    string
	Microflow  string // qualified name of the microflow that implements it
	Deprecated bool
	ModuleName string
}

// PublishedRestOperations iterates the operations of every published REST
// service outside System and Marketplace modules. --document names the service.
func (ctx *LintContext) PublishedRestOperations() iter.Seq[PublishedRestOperation] {
	return func(yield func(PublishedRestOperation) bool) {
		rows, err := ctx.db.Query(fmt.Sprintf(`
			SELECT COALESCE(o.ServiceQualifiedName, ''), COALESCE(o.ResourceName, ''),
			       COALESCE(o.HttpMethod, ''), COALESCE(o.Path, ''), COALESCE(o.Summary, ''),
			       COALESCE(o.Microflow, ''), COALESCE(o.Deprecated, 0), COALESCE(o.ModuleName, '')
			FROM published_rest_operations o
			LEFT JOIN modules m ON o.ModuleName = m.Name
			WHERE %s AND %s
			ORDER BY o.ModuleName, o.ServiceQualifiedName, o.ResourceName, o.Path, o.HttpMethod
		`, notPlatformModule("m"), ctx.documentFilterSQL("o.ServiceQualifiedName")))
		if err != nil {
			ctx.recordQueryError("PublishedRestOperations", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var o PublishedRestOperation
			var deprecated int64
			if err := rows.Scan(&o.Service, &o.Resource, &o.HTTPMethod, &o.Path, &o.Summary,
				&o.Microflow, &deprecated, &o.ModuleName); err != nil {
				ctx.recordQueryError("PublishedRestOperations (row scan)", err)
				continue
			}
			o.Deprecated = deprecated != 0
			if ctx.IsExcluded(o.ModuleName) {
				continue
			}
			if !yield(o) {
				return
			}
		}
	}
}

// Module is one module the user owns (not System, not from the Marketplace).
type Module struct {
	ID   string
	Name string
	// DomainModelDocumentation is the module's domain model's documentation.
	// A Mendix module has no documentation property of its own; this is the
	// text Studio Pro shows for the domain model with nothing selected.
	DomainModelDocumentation string
}

// Modules iterates the user's modules.
func (ctx *LintContext) Modules() iter.Seq[Module] {
	return func(yield func(Module) bool) {
		rows, err := ctx.db.Query(fmt.Sprintf(`
			SELECT m.Id, m.Name, COALESCE(m.DomainModelDocumentation, '')
			FROM modules m
			WHERE %s
			ORDER BY m.Name
		`, notPlatformModule("m")))
		if err != nil {
			ctx.recordQueryError("Modules", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var mod Module
			var id sql.NullString
			if err := rows.Scan(&id, &mod.Name, &mod.DomainModelDocumentation); err != nil {
				ctx.recordQueryError("Modules (row scan)", err)
				continue
			}
			mod.ID = id.String
			if ctx.IsExcluded(mod.Name) {
				continue
			}
			if !yield(mod) {
				return
			}
		}
	}
}

// ConstantValue is a constant's value in one run configuration
// (Settings → Configurations), which overrides the constant's default there.
type ConstantValue struct {
	Constant      string // Module.Constant
	Configuration string
	Value         string
}

// ConstantValues iterates the constant values the run configurations set, for
// constants outside System and the Marketplace (ako/mxcli#1453). A
// configuration that does not override a constant has no row: it runs on the
// constant's default_value.
func (ctx *LintContext) ConstantValues() iter.Seq[ConstantValue] {
	return func(yield func(ConstantValue) bool) {
		rows, err := ctx.db.Query(fmt.Sprintf(`
			SELECT cv.ConstantName, cv.ConfigurationName, COALESCE(cv.Value, '')
			FROM constant_values cv
			LEFT JOIN modules m ON substr(cv.ConstantName, 1, instr(cv.ConstantName, '.') - 1) = m.Name
			WHERE %s
			ORDER BY cv.ConstantName, cv.ConfigurationName
		`, notPlatformModule("m")))
		if err != nil {
			ctx.recordQueryError("ConstantValues", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var c ConstantValue
			if err := rows.Scan(&c.Constant, &c.Configuration, &c.Value); err != nil {
				ctx.recordQueryError("ConstantValues (row scan)", err)
				continue
			}
			if module, _, ok := strings.Cut(c.Constant, "."); ok && ctx.IsExcluded(module) {
				continue
			}
			if !yield(c) {
				return
			}
		}
	}
}
