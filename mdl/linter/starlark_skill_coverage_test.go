// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The write-lint-rules skill is what a Starlark rule gets written from, and it
// has drifted from the API every time the API moved: fields added and never
// documented (the entity audit members, scheduled_event's schedule fields),
// whole query functions missing (java_actions(), queues(), ...), and literals
// the linter never emits (#1164, #1178). A field or function missing from the
// skill is one no rule author will find; a documented one that does not exist
// is a rule that fails at load time or, worse, reads a default.
//
// These tests hold the skill to the API as registered in code: every builtin in
// buildPredeclared, and every struct any builtin returns, with exactly its
// fields. starlark_documented_values_test.go pins the enum literals.

const coverageSkillPath = "../../.claude/skills/mendix/write-lint-rules/SKILL.md"

func TestLintSkillDocumentsEveryBuiltin(t *testing.T) {
	skill := readCoverageSkill(t)
	var names []string
	for name := range (&StarlarkRule{}).buildPredeclared() {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) < 10 {
		t.Fatalf("buildPredeclared returned %d names -- the enumeration is broken", len(names))
	}
	for _, name := range names {
		if !strings.Contains(skill, "`"+name+"(") {
			t.Errorf("builtin %s() is not documented in the write-lint-rules skill", name)
		}
	}
}

// Structs documented somewhere other than their own "### name" table.
var (
	// Built by the rule author; the fields are the helper's parameters.
	helperStructs = map[string]string{"violation": "violation", "location": "location"}
	// Graph facts, documented inline as struct{...} in the function table row.
	inlineStructs = map[string]string{"cycle": "cycles", "module_cycle": "module_cycles"}
	// One table documents both: permissions_for() adds entity_name and drops
	// element_type/element_name, and the table says which rows are which.
	sharedSections = map[string]string{"entity_permission": "permission"}
)

func TestLintSkillDocumentsEveryStructField(t *testing.T) {
	skill := readCoverageSkill(t)
	api := starlarkStructFields(t)
	for _, want := range []string{"entity", "microflow", "project_security", "password_policy", "expr"} {
		if len(api[want]) == 0 {
			t.Fatalf("found no fields for struct %q in the package source -- the scan is broken", want)
		}
	}
	sections := skillSections(skill)

	shared := map[string][]string{}
	for _, name := range sortedKeys(api) {
		fields := api[name]
		switch {
		case helperStructs[name] != "":
			assertFieldSet(t, name+" (parameters of "+name+"())", helperParams(t, skill, helperStructs[name]), fields)
		case inlineStructs[name] != "":
			assertFieldSet(t, name+" (inline in "+inlineStructs[name]+"())", inlineStructFields(t, skill, inlineStructs[name]), fields)
		case name == "expr":
			// One row per node kind; the fields are the backticked names in the
			// "Additional fields" column, plus the kind every node carries.
			doc := []string{"kind"}
			for _, row := range sections["expr"] {
				doc = append(doc, typedFields(row.rest)...)
			}
			assertFieldSet(t, "expr", uniq(doc), fields)
		default:
			section := name
			if s, ok := sharedSections[name]; ok {
				section = s
			}
			if _, ok := sections[section]; !ok {
				t.Errorf("struct %s has no ### %s table in the skill; fields: %v", name, section, fields)
				continue
			}
			shared[section] = uniq(append(shared[section], fields...))
		}
	}
	for section, fields := range shared {
		var doc []string
		for _, row := range sections[section] {
			doc = append(doc, row.name)
		}
		assertFieldSet(t, "### "+section+" table", uniq(doc), fields)
	}
}

// starlarkStructFields returns, for every struct type name the package builds
// with starlarkstruct.FromStringDict(starlark.String("name"), dict), the keys of
// dict -- a StringDict literal, or a local variable assigned one and then
// indexed. Dynamic names (rowToStruct) are skipped: their keys are SQL columns
// and are documented inline in the graph-function table.
func starlarkStructFields(t *testing.T) map[string][]string {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for _, f := range pkgs["linter"].Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			locals := localDictKeys(fn.Body)
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || !isSelector(call.Fun, "starlarkstruct", "FromStringDict") || len(call.Args) != 2 {
					return true
				}
				name, ok := starlarkStringLiteral(call.Args[0])
				if !ok {
					return true
				}
				var keys []string
				switch d := call.Args[1].(type) {
				case *ast.CompositeLit:
					keys = literalKeys(d)
				case *ast.Ident:
					keys = locals[d.Name]
				}
				out[name] = uniq(append(out[name], keys...))
				return true
			})
		}
	}
	return out
}

func localDictKeys(body *ast.BlockStmt) map[string][]string {
	out := map[string][]string{}
	ast.Inspect(body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
			return true
		}
		switch l := as.Lhs[0].(type) {
		case *ast.Ident:
			if lit, ok := as.Rhs[0].(*ast.CompositeLit); ok && isSelector(lit.Type, "starlark", "StringDict") {
				out[l.Name] = append(out[l.Name], literalKeys(lit)...)
			}
		case *ast.IndexExpr:
			id, ok := l.X.(*ast.Ident)
			key, ok2 := l.Index.(*ast.BasicLit)
			if ok && ok2 && key.Kind == token.STRING {
				k, _ := strconv.Unquote(key.Value)
				out[id.Name] = append(out[id.Name], k)
			}
		}
		return true
	})
	return out
}

func literalKeys(lit *ast.CompositeLit) []string {
	var keys []string
	for _, e := range lit.Elts {
		if kv, ok := e.(*ast.KeyValueExpr); ok {
			if b, ok := kv.Key.(*ast.BasicLit); ok && b.Kind == token.STRING {
				k, _ := strconv.Unquote(b.Value)
				keys = append(keys, k)
			}
		}
	}
	return keys
}

func starlarkStringLiteral(e ast.Expr) (string, bool) {
	call, ok := e.(*ast.CallExpr)
	if !ok || !isSelector(call.Fun, "starlark", "String") || len(call.Args) != 1 {
		return "", false
	}
	b, ok := call.Args[0].(*ast.BasicLit)
	if !ok || b.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(b.Value)
	return s, err == nil
}

func isSelector(e ast.Expr, pkg, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == pkg
}

type skillRow struct{ name, rest string }

// skillSections maps each "### name" / "#### name ..." heading to the table
// rows under it whose first cell is a backticked identifier.
func skillSections(skill string) map[string][]skillRow {
	heading := regexp.MustCompile(`^#{3,4} (\w+)`)
	row := regexp.MustCompile("^\\|\\s*`\"?(\\w+)\"?`\\s*\\|(.*)$")
	out := map[string][]skillRow{}
	current := ""
	for _, line := range strings.Split(skill, "\n") {
		if strings.HasPrefix(line, "#") {
			current = ""
			if m := heading.FindStringSubmatch(line); m != nil {
				current = m[1]
				out[current] = nil
			}
			continue
		}
		if m := row.FindStringSubmatch(line); m != nil && current != "" {
			out[current] = append(out[current], skillRow{m[1], m[2]})
		}
	}
	return out
}

// helperParams returns the parameter names in the helper table's
// `name(a, b, c?)` signature.
func helperParams(t *testing.T, skill, fn string) []string {
	t.Helper()
	m := regexp.MustCompile("`" + fn + `\(([^)]*)\)` + "`").FindStringSubmatch(skill)
	if m == nil {
		t.Errorf("skill has no `%s(...)` signature", fn)
		return nil
	}
	var out []string
	for _, p := range strings.Split(m[1], ",") {
		out = append(out, strings.TrimSuffix(strings.TrimSpace(p), "?"))
	}
	return out
}

// inlineStructFields returns the names in `struct{a, b, c}` on the function
// table row for fn().
func inlineStructFields(t *testing.T, skill, fn string) []string {
	t.Helper()
	for _, line := range strings.Split(skill, "\n") {
		if !strings.HasPrefix(line, "| `"+fn+"(") {
			continue
		}
		m := regexp.MustCompile(`struct\{([^}]*)\}`).FindStringSubmatch(line)
		if m == nil {
			t.Errorf("%s() row documents no struct{...}: %s", fn, line)
			return nil
		}
		var out []string
		for _, f := range strings.Split(m[1], ",") {
			out = append(out, strings.TrimSpace(f))
		}
		return out
	}
	t.Errorf("skill has no function-table row for %s()", fn)
	return nil
}

// typedFields returns the names in "`name` (type)" pairs, skipping backticked
// operators and keywords in the description column.
func typedFields(s string) []string {
	var out []string
	for _, m := range regexp.MustCompile("`(\\w+)` \\(").FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	return out
}

func assertFieldSet(t *testing.T, what string, doc, api []string) {
	t.Helper()
	var missing, phantom []string
	for _, f := range api {
		if !slices.Contains(doc, f) {
			missing = append(missing, f)
		}
	}
	for _, f := range doc {
		if !slices.Contains(api, f) {
			phantom = append(phantom, f)
		}
	}
	if len(missing) > 0 || len(phantom) > 0 {
		t.Errorf("%s: undocumented %v, documented but not in the API %v", what, missing, phantom)
	}
}

func readCoverageSkill(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(coverageSkillPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func uniq(s []string) []string {
	out := slices.Clone(s)
	sort.Strings(out)
	return slices.Compact(out)
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
