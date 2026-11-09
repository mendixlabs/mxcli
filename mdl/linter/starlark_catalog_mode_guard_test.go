// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/catalog"
)

// A Starlark builtin (or a field of the structs it returns) that reads data
// only REFRESH CATALOG FULL writes returns [] / 0 under the default fast build
// — with no warning, so the rule passes having checked nothing. widgets(),
// xpath_expressions(), activities_for(), permissions(), permissions_for() and
// page.widget_count all did exactly that while only refs_to / refs_from raised
// the build depth.
//
// The guard does not trust a hand-kept list: it builds the PedApp fixture's
// catalog twice, fast and full, runs every query builtin against both, and
// requires that anything whose answer differs is auto-detected as needing the
// full build. builtinModes below must classify every predeclared name, so a new
// builtin fails here until someone decides its depth.
type builtinProbe struct {
	mode CatalogMode
	// probe is a Starlark expression evaluating to the builtin's full answer
	// (a list), flattened over the fixture's names for the per-element
	// builtins. "" means the builtin is not probed.
	probe string
}

var builtinModes = map[string]builtinProbe{
	"entities":                  {CatalogFast, "entities()"},
	"microflows":                {CatalogFast, "microflows()"},
	"java_actions":              {CatalogFast, "java_actions()"},
	"documentable_elements":     {CatalogFast, "documentable_elements()"},
	"documents":                 {CatalogFast, "documents()"},
	"navigation_targets":        {CatalogFast, "navigation_targets()"},
	"pages":                     {CatalogFast, "pages()"},
	"enumerations":              {CatalogFast, "enumerations()"},
	"constants":                 {CatalogFast, "constants()"},
	"constant_values":           {CatalogFast, "constant_values()"},
	"snippets":                  {CatalogFast, "snippets()"},
	"scheduled_events":          {CatalogFast, "scheduled_events()"},
	"queues":                    {CatalogFast, "queues()"},
	"database_connections":      {CatalogFast, "database_connections()"},
	"rest_clients":              {CatalogFast, "rest_clients()"},
	"rest_operations":           {CatalogFast, "rest_operations()"},
	"user_roles":                {CatalogFast, "user_roles()"},
	"module_roles":              {CatalogFast, "module_roles()"},
	"role_mappings":             {CatalogFast, "role_mappings()"},
	"project_security":          {CatalogFast, "[project_security()]"},
	"languages":                 {CatalogFast, "languages()"},
	"project_settings":          {CatalogFast, "[project_settings()]"},
	"attributes_for":            {CatalogFast, "[a for n in ENTITY_NAMES for a in attributes_for(n)]"},
	"modules":                   {CatalogFast, "modules()"},
	"folders":                   {CatalogFast, "folders()"},
	"annotations":               {CatalogFast, "annotations()"},
	"associations":              {CatalogFast, "associations()"},
	"entity_event_handlers":     {CatalogFast, "entity_event_handlers()"},
	"navigation_menu_items":     {CatalogFast, "navigation_menu_items()"},
	"jar_dependencies":          {CatalogFast, "jar_dependencies()"},
	"layouts":                   {CatalogFast, "layouts()"},
	"published_rest_operations": {CatalogFast, "published_rest_operations()"},

	"widgets":           {CatalogFull, "widgets()"},
	"xpath_expressions": {CatalogFull, "xpath_expressions()"},
	"activities_for":    {CatalogFull, "[a for n in FLOW_NAMES for a in activities_for(n)]"},
	"permissions":       {CatalogFull, "permissions()"},
	"permissions_for":   {CatalogFull, "[p for n in ENTITY_NAMES for p in permissions_for(n)]"},
	"refs_to":           {CatalogFull, "[r for n in REF_NAMES for r in refs_to(n)]"},
	"refs_from":         {CatalogFull, "[r for n in REF_NAMES for r in refs_from(n)]"},
	"strings":           {CatalogFull, "strings()"},

	// The graph tables are only written by REFRESH CATALOG COMMUNITIES; not
	// probed here (that build is the slow one), only their detection pinned.
	"community_of":        {CatalogCommunities, ""},
	"layer_of":            {CatalogCommunities, ""},
	"cycles":              {CatalogCommunities, ""},
	"module_cycles":       {CatalogCommunities, ""},
	"module_dependencies": {CatalogCommunities, ""},
	"centrality":          {CatalogCommunities, ""},
	"god_nodes":           {CatalogCommunities, ""},
	"integration_surface": {CatalogCommunities, ""},

	// No catalog access.
	"parse_xpath":     {CatalogFast, ""},
	"violation":       {CatalogFast, ""},
	"location":        {CatalogFast, ""},
	"is_pascal_case":  {CatalogFast, ""},
	"is_camel_case":   {CatalogFast, ""},
	"matches":         {CatalogFast, ""},
	"find_all":        {CatalogFast, ""},
	"struct":          {CatalogFast, ""},
	"get_option":      {CatalogFast, ""},
	"get_list_option": {CatalogFast, ""},
}

// fieldsKnownFull are struct fields filled only by a full build, pinned so the
// detection cannot regress for them even where the fixture has no data to show
// it (PedApp has no snippets). Fields the fixture does show are found by the
// probe below.
var fieldsKnownFull = []string{"widget_count"}

func TestBuiltinModesCoverEveryPredeclared(t *testing.T) {
	for name := range (&StarlarkRule{}).buildPredeclared() {
		if _, ok := builtinModes[name]; !ok {
			t.Errorf("builtin %s() is not classified in builtinModes: decide whether it "+
				"needs a full catalog (and add it to fullBuiltins) before shipping it", name)
		}
	}
}

func TestDetectedModeMatchesBuiltinModes(t *testing.T) {
	for name, bp := range builtinModes {
		if got := detectRequiredCatalogMode("x = " + name + "(y)"); got != bp.mode {
			t.Errorf("a rule calling %s() is built at %s, needs %s", name, got, bp.mode)
		}
	}
	for _, f := range fieldsKnownFull {
		if got := detectRequiredCatalogMode("n = p." + f); got < CatalogFull {
			t.Errorf("a rule reading .%s is built at %s, needs full", f, got)
		}
	}
}

// TestFullOnlyDataIsAutoDetected is the empirical half: whatever answers
// differently under a fast and a full build must be detected as full.
func TestFullOnlyDataIsAutoDetected(t *testing.T) {
	mpr := filepath.Join("..", "..", "testdata", "pedapp", "PedApp.mpr")
	if _, err := os.Stat(mpr); err != nil {
		t.Skipf("PedApp fixture not found: %v", err)
	}
	be := modelsdkbackend.New()
	if err := be.ConnectReadOnly(mpr); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer be.Disconnect()

	build := func(full bool) *catalog.Catalog {
		cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cat.Close() })
		b := catalog.NewBuilder(cat, be)
		b.SetFullMode(full)
		if err := b.Build(nil); err != nil {
			t.Fatalf("build (full=%v): %v", full, err)
		}
		return cat
	}
	fast, full := build(false), build(true)

	// Names for the per-element builtins come from the FULL catalog, so the
	// fast run is asked about exactly the same elements.
	names := func(q string) *starlark.List {
		res, err := full.Query(q)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		var vs []starlark.Value
		for _, row := range res.Rows {
			if s, ok := row[0].(string); ok {
				vs = append(vs, starlark.String(s))
			}
		}
		return starlark.NewList(vs)
	}
	globals := starlark.StringDict{
		"ENTITY_NAMES": names(`SELECT DISTINCT ElementName FROM permissions WHERE ElementType = 'ENTITY'
			UNION SELECT QualifiedName FROM entities`),
		"FLOW_NAMES": names(`SELECT QualifiedName FROM microflows`),
		"REF_NAMES":  names(`SELECT TargetName FROM refs UNION SELECT SourceName FROM refs`),
	}

	run := func(cat *catalog.Catalog, expr string) []starlark.Value {
		r := &StarlarkRule{ctx: NewLintContext(cat, be)}
		env := r.buildPredeclared()
		for k, v := range globals {
			env[k] = v
		}
		v, err := starlark.Eval(&starlark.Thread{Name: "probe"}, "probe", expr, env)
		if err != nil {
			t.Fatalf("%s: %v", expr, err)
		}
		list, ok := v.(*starlark.List)
		if !ok {
			t.Fatalf("%s: not a list: %s", expr, v.Type())
		}
		out := make([]starlark.Value, list.Len())
		for i := range out {
			out[i] = list.Index(i)
		}
		return out
	}

	differingFields := map[string][]string{} // field -> builtins it was seen on
	for name, bp := range builtinModes {
		if bp.probe == "" {
			continue
		}
		f, u := run(fast, bp.probe), run(full, bp.probe)
		if len(f) != len(u) {
			if detectRequiredCatalogMode(name+"(") < CatalogFull {
				t.Errorf("%s() answers %d rows under a fast catalog and %d under a full one, "+
					"but a rule calling it is not built full", name, len(f), len(u))
			}
			continue
		}
		if len(u) == 0 {
			continue
		}
		for _, field := range structFields(u[0]) {
			if fieldValues(f, field) != fieldValues(u, field) {
				differingFields[field] = append(differingFields[field], name)
			}
		}
	}
	for field, from := range differingFields {
		if detectRequiredCatalogMode("n = p."+field) < CatalogFull {
			t.Errorf(".%s (on %v) differs between a fast and a full catalog, but a rule "+
				"reading it is not built full", field, from)
		}
	}
	// The fixture must exercise the full-only paths, or the test is vacuous: a
	// full build with no widgets would agree with the fast one.
	for _, expr := range []string{"widgets()", "permissions()", "xpath_expressions()"} {
		if len(run(full, expr)) == 0 {
			t.Errorf("fixture has no %s under a full build; the guard checks nothing there", expr)
		}
	}
}

func structFields(v starlark.Value) []string {
	s, ok := v.(*starlarkstruct.Struct)
	if !ok {
		return nil
	}
	return s.AttrNames()
}

// fieldValues is the sorted multiset of one field across rows, so row order
// does not matter.
func fieldValues(rows []starlark.Value, field string) string {
	vals := make([]string, 0, len(rows))
	for _, r := range rows {
		s, ok := r.(*starlarkstruct.Struct)
		if !ok {
			continue
		}
		v, err := s.Attr(field)
		if err != nil {
			continue
		}
		vals = append(vals, v.String())
	}
	sort.Strings(vals)
	return strings.Join(vals, "\x00")
}
