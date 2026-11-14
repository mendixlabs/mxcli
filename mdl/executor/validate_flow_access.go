// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"github.com/mendixlabs/mxcli/mdl/types"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// flowAccessRule is the check-time counterpart of MxBuild's CE0106:
//
//	"At least one allowed role must be selected if the microflow is used from
//	 navigation, a page, a nanoflow or a published service."
//
// What triggers it was measured with mxbuild 11.14.0 on a fresh project, one
// flow per kind of use, each with no allowed roles:
//
//	page button / page data source / snippet / navigation menu item /
//	menu document / nanoflow call (even from an unused nanoflow,
//	snippet or menu document)                      -> CE0106
//	a nanoflow with no roles called from a page    -> CE0106 ("if the nanoflow…")
//	published REST operation                       -> nothing
//	called only from another microflow / unused    -> nothing
//	referenced only from an EXCLUDED page          -> nothing
//
// at security level Prototype and Production alike, and nothing at all at Off.
//
// The reported way to get there: `drop microflow` in one exec run and
// `create microflow` in a later one. Within one run exec carries the dropped
// flow's roles over (consumeDroppedMicroflow) and `create or modify` keeps them,
// but a create in a later run is a NEW flow, and a new flow gets no allowed roles
// (#1474) — while the page that calls it is still there.
const flowAccessRule = "MDL-SEC21"

// flowRefSourceTypes are the document types whose references make a flow need
// an allowed role. A published REST service is deliberately absent: measured,
// it does not.
var flowRefSourceTypes = []string{
	"Forms$Page",
	"Forms$Snippet",
	"Forms$Layout",
	"Microflows$Nanoflow",
	"Menus$MenuDocument",
	"Navigation$NavigationDocument",
}

// flowRef is a reference to a flow: kind is "microflow" or "nanoflow".
type flowRef struct {
	kind string
	name string
}

func (r flowRef) key() string { return r.kind + ":" + r.name }

// flowAccessState is one flow's simulated access.
type flowAccessState struct {
	ref      flowRef
	exists   bool
	excluded bool
	roles    map[string]bool
	// cause says, for the message, how the script left it without roles.
	cause string
}

func (s *flowAccessState) clone() *flowAccessState {
	c := *s
	c.roles = make(map[string]bool, len(s.roles))
	for r := range s.roles {
		c.roles[r] = true
	}
	return &c
}

// moduleRoleState is a module's simulated role list.
type moduleRoleState struct {
	roles []string // role names, in order
}

// CheckFlowAccess reports MDL-SEC21 for the script.
func (e *Executor) CheckFlowAccess(prog *ast.Program) []linter.Violation {
	if e == nil {
		return nil
	}
	return CheckFlowAccess(e.newExecContext(context.Background()), prog)
}

// CheckFlowAccess simulates the script's effect on flow access and flow uses,
// and reports every flow the script leaves with no allowed role while something
// that needs one — a page, snippet, layout, nanoflow, menu or navigation
// profile, stored or written by the script — names it.
//
// Only what the script CAUSES is reported: a flow already in that state before
// the script, and left alone by it, is the project's existing problem and not a
// verdict on this script.
func CheckFlowAccess(ctx *ExecContext, prog *ast.Program) []linter.Violation {
	if ctx == nil || prog == nil || !ctx.Connected() || !scriptTouchesFlowAccess(prog) {
		return nil
	}
	h, err := getHierarchy(ctx)
	if err != nil || h == nil {
		return nil
	}

	sim := &flowAccessSim{
		ctx:      ctx,
		flows:    map[string]*flowAccessState{},
		modules:  map[string]*moduleRoleState{},
		dropped:  map[string][]string{},
		usesBy:   map[string][]flowRef{},
		storedPg: map[string]bool{},
	}
	if ps, err := ctx.Backend.GetProjectSecurity(); err == nil && ps != nil {
		sim.level = ps.SecurityLevel
	}
	sim.loadStoredFlows(h)
	sim.loadStoredUses(h)

	before := sim.violating()
	for _, stmt := range prog.Statements {
		sim.apply(stmt)
	}
	after := sim.violating()

	severity := linter.SeverityError
	levelNote := "at security level " + security.SecurityLevelDisplay(sim.level)
	if sim.level == "" || sim.level == security.SecurityLevelOff {
		severity = linter.SeverityWarning
		levelNote = "once security is Prototype or Production (this project is at Off, where it is not checked)"
	}

	var keys []string
	for k := range after {
		if !before[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	var out []linter.Violation
	for _, k := range keys {
		st := sim.flows[k]
		users := sim.usersOf(st.ref)
		qn := splitQualifiedName(st.ref.name)
		cause := st.cause
		if cause == "" {
			cause = "this script makes it a use of a flow that has no allowed role"
		}
		out = append(out, linter.Violation{
			RuleID:   flowAccessRule,
			Severity: severity,
			Message: fmt.Sprintf(
				"%s %s has no allowed role but is used from %s — MxBuild reports CE0106 \"At least one allowed role must be selected if the %s is used from navigation, a page, a nanoflow or a published service\" %s. %s",
				st.ref.kind, st.ref.name, strings.Join(users, ", "), st.ref.kind, levelNote, cause),
			Suggestion: sim.suggestion(st),
			Location:   linter.Location{Module: qn.Module, DocumentType: st.ref.kind, DocumentName: qn.Name},
		})
	}
	return out
}

// scriptTouchesFlowAccess is the cheap gate: a script that writes no flow,
// document, grant or role cannot change the answer, so nothing is loaded.
func scriptTouchesFlowAccess(prog *ast.Program) bool {
	for _, stmt := range prog.Statements {
		switch stmt.(type) {
		case *ast.CreateMicroflowStmt, *ast.CreateNanoflowStmt, *ast.DropMicroflowStmt, *ast.DropNanoflowStmt,
			*ast.RevokeMicroflowAccessStmt, *ast.RevokeNanoflowAccessStmt,
			*ast.CreatePageStmtV3, *ast.CreateSnippetStmtV3, *ast.AlterPageStmt,
			*ast.CreateMenuStmt, *ast.CreateNavigationStmt, *ast.DropModuleRoleStmt,
			*ast.AlterNavigationStmt, *ast.AlterMenuStmt:
			return true
		}
	}
	return false
}

type flowAccessSim struct {
	ctx     *ExecContext
	level   string
	flows   map[string]*flowAccessState
	modules map[string]*moduleRoleState
	// dropped holds the roles of flows the script dropped, which a later create
	// of the same name in the same run carries over.
	dropped map[string][]string
	// usesBy maps a using document ("page Mod.P", "navigation Responsive", …)
	// to the flows it names. A document the script rewrites replaces its entry.
	usesBy map[string][]flowRef
	// storedPg is the pages and snippets the project holds, by "page Mod.P".
	storedPg map[string]bool
}

func (s *flowAccessSim) loadStoredFlows(h *ContainerHierarchy) {
	put := func(kind, qn string, excluded bool, roles []model.ID) {
		key := flowRef{kind, qn}.key()
		if prev, ok := s.flows[key]; ok && !prev.excluded {
			return // a live namesake wins over excluded ones (#914)
		}
		st := &flowAccessState{ref: flowRef{kind, qn}, exists: true, excluded: excluded, roles: map[string]bool{}}
		for _, r := range roles {
			st.roles[string(r)] = true
		}
		s.flows[key] = st
	}
	// A backend that can say what a flow's access takes without decoding the
	// flow is asked that; the listings below decode every flow of the project
	// for three top-level values of each.
	if l, ok := s.ctx.Backend.(flowAccessLister); ok {
		if flows, err := l.ListFlowAccess(); err == nil {
			for _, f := range flows {
				put(f.Kind, h.GetQualifiedName(f.ContainerID, f.Name), f.Excluded, f.AllowedModuleRoles)
			}
			return
		}
	}
	if mfs, err := projectMicroflows(s.ctx); err == nil {
		for _, mf := range mfs {
			put("microflow", h.GetQualifiedName(mf.ContainerID, mf.Name), mf.Excluded, mf.AllowedModuleRoles)
		}
	}
	if nfs, err := projectNanoflows(s.ctx); err == nil {
		for _, nf := range nfs {
			put("nanoflow", h.GetQualifiedName(nf.ContainerID, nf.Name), nf.Excluded, nf.AllowedModuleRoles)
		}
	}
}

// flowAccessLister is a backend that lists every flow's access without
// decoding the flows (modelsdk.Backend.ListFlowAccess).
type flowAccessLister interface {
	ListFlowAccess() ([]types.FlowAccess, error)
}

// loadStoredUses reads every flow reference out of the documents that make a
// flow need a role. The walk is over raw BSON because the references live in
// many shapes (button actions, data sources, menu items, call activities) that
// all store the target's qualified name under a "Microflow" or "Nanoflow" key.
//
// It reads the bytes where they lie rather than decoding each document: a
// page decodes into tens of thousands of values, and decoding every stored
// page was most of what `check` of one microflow cost in a project with
// pages (ako/mxcli#2160). storedUsesDecoded is the decoding form, kept as
// what this one is tested against.
func (s *flowAccessSim) loadStoredUses(h *ContainerHierarchy) {
	for _, typ := range flowRefSourceTypes {
		units, err := s.ctx.Backend.ListRawUnitsByType(typ)
		if err != nil {
			continue
		}
		for _, u := range units {
			if u.Type != typ || len(u.Contents) == 0 {
				continue
			}
			for _, use := range storedUses(typ, u.Contents) {
				src := use.label
				if !use.navigation {
					src = flowUseSourceLabel(typ) + " " + h.GetQualifiedName(u.ContainerID, use.label)
					if typ == "Forms$Page" || typ == "Forms$Snippet" {
						s.storedPg[src] = true
					}
				}
				s.usesBy[src] = use.refs
			}
		}
	}
}

// A storedUse is one source of flow references in a stored document: the
// document itself, or — in the navigation document — one profile. label is
// the document's name, or "navigation <profile>" in full.
type storedUse struct {
	label      string
	navigation bool
	refs       []flowRef
}

// storedUses reads a stored document's flow references from its bytes. It
// returns nothing for an excluded document, and for one that is not BSON.
func storedUses(typ string, contents []byte) []storedUse {
	doc := bsoncore.Document(contents)
	if doc.Validate() != nil {
		return nil
	}
	if b, ok := doc.Lookup("Excluded").BooleanOK(); ok && b {
		return nil
	}
	if typ == "Navigation$NavigationDocument" {
		profiles, ok := doc.Lookup("Profiles").ArrayOK()
		if !ok {
			return nil
		}
		values, err := profiles.Values()
		if err != nil {
			return nil
		}
		var out []storedUse
		for _, v := range values {
			pd, ok := v.DocumentOK()
			if !ok {
				continue
			}
			pn, _ := pd.Lookup("Name").StringValueOK()
			out = append(out, storedUse{label: "navigation " + pn, navigation: true, refs: rawFlowRefs(pd, nil)})
		}
		return out
	}
	name, _ := doc.Lookup("Name").StringValueOK()
	return []storedUse{{label: name, refs: rawFlowRefs(doc, nil)}}
}

// rawFlowRefs appends every qualified name stored under a "Microflow" or
// "Nanoflow" key anywhere in the document, in document order, without
// decoding it. The document has been validated by the caller.
func rawFlowRefs(doc []byte, out []flowRef) []flowRef {
	if len(doc) < 5 {
		return out
	}
	rest := doc[4 : len(doc)-1]
	for len(rest) > 0 {
		elem, after, ok := bsoncore.ReadElement(rest)
		if !ok {
			return out
		}
		rest = after
		value := elem.Value()
		switch value.Type {
		case bsoncore.TypeString:
			kind := ""
			switch elem.Key() {
			case "Microflow":
				kind = "microflow"
			case "Nanoflow":
				kind = "nanoflow"
			}
			if kind != "" {
				if s := value.StringValue(); strings.Contains(s, ".") {
					out = append(out, flowRef{kind, s})
				}
			}
		case bsoncore.TypeEmbeddedDocument, bsoncore.TypeArray:
			out = rawFlowRefs(value.Data, out)
		}
	}
	return out
}

// storedUsesDecoded is storedUses by decoding the whole document: what it
// did before, and what the test holds storedUses to.
func storedUsesDecoded(typ string, contents []byte) []storedUse {
	var doc bson.D
	if err := bson.Unmarshal(contents, &doc); err != nil {
		return nil
	}
	if b, _ := bsonField(doc, "Excluded").(bool); b {
		return nil
	}
	if typ == "Navigation$NavigationDocument" {
		var out []storedUse
		profiles, _ := bsonField(doc, "Profiles").(bson.A)
		for _, p := range profiles {
			pd, ok := p.(bson.D)
			if !ok {
				continue
			}
			pn, _ := bsonField(pd, "Name").(string)
			out = append(out, storedUse{label: "navigation " + pn, navigation: true, refs: collectRawFlowRefs(pd)})
		}
		return out
	}
	name, _ := bsonField(doc, "Name").(string)
	return []storedUse{{label: name, refs: collectRawFlowRefs(doc)}}
}

func flowUseSourceLabel(unitType string) string {
	switch unitType {
	case "Forms$Page":
		return "page"
	case "Forms$Snippet":
		return "snippet"
	case "Forms$Layout":
		return "layout"
	case "Microflows$Nanoflow":
		return "nanoflow"
	case "Menus$MenuDocument":
		return "menu"
	}
	return unitType
}

func bsonField(doc bson.D, key string) any {
	for _, e := range doc {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

// collectRawFlowRefs collects every qualified name stored under a "Microflow"
// or "Nanoflow" key anywhere in the document.
func collectRawFlowRefs(doc bson.D) []flowRef {
	var out []flowRef
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case bson.D:
			for _, e := range t {
				if s, ok := e.Value.(string); ok && strings.Contains(s, ".") {
					switch e.Key {
					case "Microflow":
						out = append(out, flowRef{"microflow", s})
						continue
					case "Nanoflow":
						out = append(out, flowRef{"nanoflow", s})
						continue
					}
				}
				walk(e.Value)
			}
		case bson.A:
			for _, x := range t {
				walk(x)
			}
		}
	}
	walk(doc)
	return out
}

// violating returns the flows that currently need a role and have none.
func (s *flowAccessSim) violating() map[string]bool {
	used := map[string]bool{}
	for _, refs := range s.usesBy {
		for _, r := range refs {
			used[r.key()] = true
		}
	}
	out := map[string]bool{}
	for k, st := range s.flows {
		if st.exists && !st.excluded && len(st.roles) == 0 && used[k] {
			out[k] = true
		}
	}
	return out
}

func (s *flowAccessSim) usersOf(ref flowRef) []string {
	var users []string
	for src, refs := range s.usesBy {
		for _, r := range refs {
			if r == ref {
				users = append(users, src)
				break
			}
		}
	}
	sort.Strings(users)
	if len(users) > 3 {
		users = append(users[:3], fmt.Sprintf("%d more", len(users)-3))
	}
	return users
}

// module returns the simulated role list of a module, loading the stored one on
// first use. A module the project does not hold starts with no roles.
func (s *flowAccessSim) module(name string) *moduleRoleState {
	if m, ok := s.modules[name]; ok {
		return m
	}
	m := &moduleRoleState{}
	if mods, err := s.ctx.Backend.ListModules(); err == nil {
		for _, mod := range mods {
			if mod.Name != name {
				continue
			}
			if ms, err := s.ctx.Backend.GetModuleSecurity(mod.ID); err == nil && ms != nil {
				for _, r := range ms.ModuleRoles {
					m.roles = append(m.roles, r.Name)
				}
			}
			break
		}
	}
	s.modules[name] = m
	return m
}

func (s *flowAccessSim) flow(kind, qn string) *flowAccessState {
	key := flowRef{kind, qn}.key()
	if st, ok := s.flows[key]; ok {
		return st
	}
	st := &flowAccessState{ref: flowRef{kind, qn}, roles: map[string]bool{}}
	s.flows[key] = st
	return st
}

func (s *flowAccessSim) createFlow(kind string, name ast.QualifiedName, orModify, ifNotExists, excluded bool, body []ast.MicroflowStatement) {
	qn := name.String()
	st := s.flow(kind, qn)
	if st.exists {
		if ifNotExists {
			return
		}
		// create or modify (or a plain create exec refuses): the stored roles
		// stay. Exclusion is carried too (#914).
		st.excluded = st.excluded || excluded
	} else {
		st.exists = true
		st.excluded = excluded
		st.roles = map[string]bool{}
		roles, wasDropped := s.dropped[st.ref.key()]
		delete(s.dropped, st.ref.key())
		// A nanoflow carries a dropped namesake's roles only when it had some
		// (buildNanoflowFromStmt); a microflow always does.
		if wasDropped && (kind == "microflow" || len(roles) > 0) {
			for _, r := range roles {
				st.roles[r] = true
			}
			if len(roles) == 0 {
				st.cause = "It had no allowed role when this script dropped it, and the create carries that over."
			}
		} else {
			// A new document starts with no allowed roles (#1474).
			st.cause = fmt.Sprintf(
				"This script creates it as a NEW %s, and a new document starts with no access until it is granted. "+
					"If an earlier run dropped it, its access rules went with it: drop + create in separate runs does not carry them, `create or modify` does.",
				kind)
		}
	}
	if kind == "nanoflow" {
		src := "nanoflow " + qn
		if st.excluded {
			delete(s.usesBy, src)
			return
		}
		c := &flowRefCollector{}
		c.collectFromStatements(body)
		var refs []flowRef
		for _, m := range c.microflows {
			refs = append(refs, flowRef{"microflow", m})
		}
		for _, n := range c.nanoflows {
			refs = append(refs, flowRef{"nanoflow", n})
		}
		s.usesBy[src] = refs
	}
}

func (s *flowAccessSim) dropFlow(kind, qn string) {
	st := s.flow(kind, qn)
	if !st.exists {
		return
	}
	var roles []string
	for r := range st.roles {
		roles = append(roles, r)
	}
	sort.Strings(roles)
	s.dropped[st.ref.key()] = roles
	st.exists = false
	st.roles = map[string]bool{}
	if kind == "nanoflow" {
		delete(s.usesBy, "nanoflow "+qn)
	}
}

func (s *flowAccessSim) grant(kind string, name ast.QualifiedName, roles []ast.QualifiedName) {
	st := s.flow(kind, name.String())
	for _, r := range roles {
		st.roles[r.String()] = true
	}
}

func (s *flowAccessSim) revoke(kind string, name ast.QualifiedName, roles []ast.QualifiedName) {
	st := s.flow(kind, name.String())
	had := len(st.roles) > 0
	for _, r := range roles {
		delete(st.roles, r.String())
	}
	if had && len(st.roles) == 0 {
		st.cause = fmt.Sprintf("This script's `revoke execute on %s %s` removes its last allowed role.", kind, name.String())
	}
}

func widgetFlowRefs(widgets []*ast.WidgetV3) []flowRef {
	c := &widgetRefCollector{}
	c.collectFromWidgets(widgets)
	c.dedupe()
	var refs []flowRef
	for _, m := range c.microflows {
		refs = append(refs, flowRef{"microflow", m})
	}
	for _, n := range c.nanoflows {
		refs = append(refs, flowRef{"nanoflow", n})
	}
	return refs
}

func menuItemFlowRefs(items []ast.NavMenuItemDef) []flowRef {
	var refs []flowRef
	for _, it := range items {
		if it.Microflow != nil && it.Microflow.Module != "" {
			refs = append(refs, flowRef{"microflow", it.Microflow.String()})
		}
		refs = append(refs, menuItemFlowRefs(it.Items)...)
	}
	return refs
}

func (s *flowAccessSim) apply(stmt ast.Statement) {
	switch st := stmt.(type) {
	case *ast.AlterProjectSecurityStmt:
		switch strings.ToLower(st.SecurityLevel) {
		case "off":
			s.level = security.SecurityLevelOff
		case "prototype":
			s.level = security.SecurityLevelPrototype
		case "production":
			s.level = security.SecurityLevelProduction
		}
	case *ast.CreateModuleStmt:
		s.module(st.Name)
	case *ast.CreateModuleRoleStmt:
		m := s.module(st.Name.Module)
		for _, r := range m.roles {
			if strings.EqualFold(r, st.Name.Name) {
				return
			}
		}
		m.roles = append(m.roles, st.Name.Name)
	case *ast.DropModuleRoleStmt:
		m := s.module(st.Name.Module)
		for i, r := range m.roles {
			if strings.EqualFold(r, st.Name.Name) {
				m.roles = append(m.roles[:i], m.roles[i+1:]...)
				break
			}
		}
		// Dropping a role removes it from every document that allowed it.
		for _, f := range s.flows {
			if f.roles[st.Name.String()] {
				delete(f.roles, st.Name.String())
				if len(f.roles) == 0 && f.cause == "" {
					f.cause = fmt.Sprintf("This script drops module role %s, its last allowed role.", st.Name.String())
				}
			}
		}
	case *ast.CreateMicroflowStmt:
		s.createFlow("microflow", st.Name, st.CreateOrModify, st.IfNotExists, st.Excluded, nil)
	case *ast.CreateNanoflowStmt:
		s.createFlow("nanoflow", st.Name, st.CreateOrModify, st.IfNotExists, st.Excluded, st.Body)
	case *ast.DropMicroflowStmt:
		s.dropFlow("microflow", st.Name.String())
	case *ast.DropNanoflowStmt:
		s.dropFlow("nanoflow", st.Name.String())
	case *ast.GrantMicroflowAccessStmt:
		s.grant("microflow", st.Microflow, st.Roles)
	case *ast.RevokeMicroflowAccessStmt:
		s.revoke("microflow", st.Microflow, st.Roles)
	case *ast.GrantNanoflowAccessStmt:
		s.grant("nanoflow", st.Nanoflow, st.Roles)
	case *ast.RevokeNanoflowAccessStmt:
		s.revoke("nanoflow", st.Nanoflow, st.Roles)
	case *ast.CreatePageStmtV3:
		src := "page " + st.Name.String()
		if st.Excluded || carriedExclusion(s.ctx, "page", st.Name, st.IsReplace || st.IsModify) {
			delete(s.usesBy, src)
			return
		}
		_, widgets, _ := documentWidgets(st)
		s.usesBy[src] = widgetFlowRefs(widgets)
		s.storedPg[src] = true
	case *ast.CreateSnippetStmtV3:
		src := "snippet " + st.Name.String()
		if carriedExclusion(s.ctx, "snippet", st.Name, st.IsReplace || st.IsModify) {
			delete(s.usesBy, src)
			return
		}
		s.usesBy[src] = widgetFlowRefs(st.Widgets)
		s.storedPg[src] = true
	case *ast.AlterPageStmt:
		src := strings.ToLower(st.ContainerType) + " " + st.PageName.String()
		for _, op := range st.Operations {
			switch o := op.(type) {
			case *ast.InsertWidgetOp:
				s.usesBy[src] = append(s.usesBy[src], widgetFlowRefs(o.Widgets)...)
			case *ast.ReplaceWidgetOp:
				s.usesBy[src] = append(s.usesBy[src], widgetFlowRefs(o.NewWidgets)...)
			}
		}
	case *ast.DropPageStmt:
		delete(s.usesBy, "page "+st.Name.String())
	case *ast.DropSnippetStmt:
		delete(s.usesBy, "snippet "+st.Name.String())
	case *ast.CreateMenuStmt:
		s.usesBy["menu "+st.Name.String()] = menuItemFlowRefs(st.Items)
	case *ast.DropMenuStmt:
		delete(s.usesBy, "menu "+st.Name.String())
	case *ast.CreateNavigationStmt:
		refs := menuItemFlowRefs(st.MenuItems)
		for _, hp := range st.HomePages {
			if !hp.IsPage && hp.Target.Module != "" {
				refs = append(refs, flowRef{"microflow", hp.Target.String()})
			}
		}
		s.usesBy["navigation "+st.ProfileName] = refs
	case *ast.AlterNavigationStmt, *ast.AlterMenuStmt:
		// An alter adds to what the document uses; the items it does not name
		// keep their flows.
		var key string
		var refs []flowRef
		switch v := menuAlterView(st).(type) {
		case *ast.CreateNavigationStmt:
			key = "navigation " + v.ProfileName
			refs = menuItemFlowRefs(v.MenuItems)
			for _, hp := range v.HomePages {
				if !hp.IsPage && hp.Target.Module != "" {
					refs = append(refs, flowRef{"microflow", hp.Target.String()})
				}
			}
		case *ast.CreateMenuStmt:
			key = "menu " + v.Name.String()
			refs = menuItemFlowRefs(v.Items)
		}
		s.usesBy[key] = append(s.usesBy[key], refs...)
	}
}

// suggestion names the fix: a grant to the module's own roles (a document can
// only be granted to its own module's roles — CE0148), or, for a module with
// none, creating one first.
func (s *flowAccessSim) suggestion(st *flowAccessState) string {
	qn := splitQualifiedName(st.ref.name)
	m := s.module(qn.Module)
	var roles []string
	for _, r := range m.roles {
		roles = append(roles, qn.Module+"."+r)
	}
	grant := fmt.Sprintf("grant execute on %s %s to ", st.ref.kind, st.ref.name)
	var fix string
	if len(roles) == 0 {
		fix = fmt.Sprintf("module %s has no module role to grant: create one (create module role %s.User;), map it into a user role, then %s%s.User;",
			qn.Module, qn.Module, grant, qn.Module)
	} else {
		fix = grant + strings.Join(roles, ", ") + ";"
		if len(roles) > 1 {
			fix += " (or the subset of those roles that should run it)"
		}
	}
	return "add " + fix + " — or, to rebuild a flow that already exists without losing its access, use `create or modify " + st.ref.kind + "` instead of drop + create"
}
