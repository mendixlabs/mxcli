// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// prefixWidgetNames prepends prefix to every widget name in the tree — the
// `as <prefix>` of `use building block` / `use fragment` — and, in the same
// pass, to every reference to one of those names inside the tree.
//
// A block's widgets name each other: Atlas's Master_Detail holds
// `dataview dataView7 (DataSource: selection gallery2)`. Renaming the widgets
// alone left that pointing at a name the page no longer has, and exec failed
// with "widget not found: gallery2" (ako/mxcli#560). Only names DECLARED in the
// tree are rewritten; a reference to a widget elsewhere on the page keeps its
// name.
//
// The tree's property values may be shared with a fragment definition
// (cloneWidget copies the property map, not the values in it), so references
// are rewritten copy-on-write: a value that changes is replaced in this tree's
// map, never mutated in place.
func prefixWidgetNames(widgets []*ast.WidgetV3, prefix string) {
	if prefix == "" {
		return
	}
	declared := map[string]bool{}
	collectWidgetNames(widgets, declared)
	renameWidgetNames(widgets, prefix)
	if len(declared) == 0 {
		return
	}
	r := newWidgetRefRenamer(declared, prefix)
	r.rewriteTree(widgets)
}

func renameWidgetNames(widgets []*ast.WidgetV3, prefix string) {
	for _, w := range widgets {
		if w.Name != "" {
			w.Name = prefix + w.Name
		}
		renameWidgetNames(w.Children, prefix)
	}
}

// widgetRefRenamer rewrites references to a set of widget names.
//
// A widget name is referenced in two spellings:
//   - bare, in the fields that hold a name and nothing else: a `selection`
//     datasource's Reference, an association source's ContextVariable, a
//     `delete $w` action's SourceVariable, an `open link $w/Attr` LinkVariable;
//   - as a `$name` variable token anywhere text is stored — a flow argument
//     (`Item = $gallery1`), a template parameter or attribute reference
//     (`$dataView1.Name`), a visibility or editability expression
//     (`$dataView1/Active`), a `where` constraint.
//
// The second is matched as a whole token (`$gal1` is not touched in
// `$gal10`), so it is safe across every string a property value holds; a
// building block has no parameters of its own, so a `$name` matching one of its
// widget names can only be that widget.
type widgetRefRenamer struct {
	names  map[string]bool
	prefix string
	token  *regexp.Regexp
}

func newWidgetRefRenamer(names map[string]bool, prefix string) *widgetRefRenamer {
	list := make([]string, 0, len(names))
	for n := range names {
		list = append(list, regexp.QuoteMeta(n))
	}
	// Longest first, so an alternation never stops at a shorter name that is a
	// prefix of the longer one.
	sort.Slice(list, func(i, j int) bool { return len(list[i]) > len(list[j]) })
	return &widgetRefRenamer{
		names:  names,
		prefix: prefix,
		token:  regexp.MustCompile(`(^|[^A-Za-z0-9_$])\$(` + strings.Join(list, "|") + `)\b`),
	}
}

func (r *widgetRefRenamer) rewriteTree(widgets []*ast.WidgetV3) {
	for _, w := range widgets {
		if len(w.Properties) > 0 {
			props := make(map[string]any, len(w.Properties))
			for k, v := range w.Properties {
				props[k] = r.rewriteAny(v)
			}
			w.Properties = props
		}
		if len(w.ValueSource) > 0 {
			vs := make(map[string]string, len(w.ValueSource))
			for k, v := range w.ValueSource {
				vs[k] = r.text(v)
			}
			w.ValueSource = vs
		}
		r.rewriteTree(w.Children)
	}
}

// text rewrites `$name` tokens.
func (r *widgetRefRenamer) text(s string) string {
	if !strings.Contains(s, "$") {
		return s
	}
	return r.token.ReplaceAllString(s, "${1}$$"+r.prefix+"${2}")
}

// bare rewrites a field that holds a widget name on its own.
func (r *widgetRefRenamer) bare(s string) string {
	if r.names[s] {
		return r.prefix + s
	}
	return s
}

func (r *widgetRefRenamer) rewriteAny(v any) any {
	if v == nil {
		return nil
	}
	nv := r.rewriteValue(reflect.ValueOf(v))
	if !nv.IsValid() {
		return v
	}
	return nv.Interface()
}

// rewriteValue returns v with every reference rewritten, as a new value
// wherever anything changed underneath; v itself is never modified.
func (r *widgetRefRenamer) rewriteValue(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.String:
		s := r.text(v.String())
		if s == v.String() {
			return v
		}
		return reflect.ValueOf(s).Convert(v.Type())
	case reflect.Interface:
		if v.IsNil() {
			return v
		}
		inner := r.rewriteValue(v.Elem())
		out := reflect.New(v.Type()).Elem()
		out.Set(inner)
		return out
	case reflect.Ptr:
		if v.IsNil() {
			return v
		}
		elem := r.rewriteValue(v.Elem())
		out := reflect.New(v.Type().Elem())
		out.Elem().Set(elem)
		return out
	case reflect.Struct:
		out := reflect.New(v.Type()).Elem()
		out.Set(v)
		for i := 0; i < v.NumField(); i++ {
			if !out.Field(i).CanSet() {
				continue
			}
			out.Field(i).Set(r.rewriteValue(v.Field(i)))
		}
		r.rewriteBareFields(out)
		return out
	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			out.Index(i).Set(r.rewriteValue(v.Index(i)))
		}
		return out
	case reflect.Map:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeMapWithSize(v.Type(), v.Len())
		iter := v.MapRange()
		for iter.Next() {
			out.SetMapIndex(iter.Key(), r.rewriteValue(iter.Value()))
		}
		return out
	}
	return v
}

// rewriteBareFields renames the fields that hold a widget name without a `$`.
// They are listed by type because the same field holds other things too: a
// datasource's Reference is an entity, a flow or a `$Param` unless the source
// is a selection.
func (r *widgetRefRenamer) rewriteBareFields(s reflect.Value) {
	switch x := s.Addr().Interface().(type) {
	case *ast.DataSourceV3:
		if x.Type == "selection" {
			x.Reference = r.bare(x.Reference)
		}
		x.ContextVariable = r.bare(x.ContextVariable)
	case *ast.ActionV3:
		x.SourceVariable = r.bare(x.SourceVariable)
		x.LinkVariable = r.bare(x.LinkVariable)
	}
}
