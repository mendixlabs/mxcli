// SPDX-License-Identifier: Apache-2.0

// Package editorconfig runs a pluggable widget's compiled editorConfig.js to
// find out which of its properties the editor hides for a given configuration.
//
// Studio Pro calls the bundle's `getProperties(values, defaultProperties,
// target)` and shows only the properties left in the returned tree; mxbuild
// then requires every hidden property to hold its default (a hidden text
// template: null), or reports CE0463. Reading that logic from the source, one
// JavaScript shape at a time, leaves most widgets partly covered and cannot see
// a guard on `target` at all (ako/mxcli#1776). Running the function gives
// Studio Pro's own answer.
//
// The bundle is third-party code. It runs in goja, which is pure Go and gives a
// script no file, network or process access; each call gets a fresh runtime, a
// time limit, and a recovered panic, so a broken or hostile bundle costs an
// error and never a crash.
package editorconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"

	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

// Target is the `target` argument Studio Pro passes. Bundles branch on it —
// TreeNode, Maps and every chart hide one set in Studio ("web") and another in
// Studio Pro ("desktop") — and only the desktop answer is what mxbuild checks.
const Target = "desktop"

// Timeout bounds one getProperties call.
const Timeout = time.Second

// Evaluator runs one widget's getProperties.
type Evaluator struct {
	program *goja.Program
	props   []mpk.PropertyDef
	byLower map[string]*mpk.PropertyDef

	mu    sync.Mutex
	cache map[string]map[string]bool
}

// New compiles a widget's editorConfig.js against its declared properties.
func New(js string, props []mpk.PropertyDef) (*Evaluator, error) {
	program, err := goja.Compile("editorConfig.js", js, false)
	if err != nil {
		return nil, fmt.Errorf("compile editorConfig.js: %w", err)
	}
	e := &Evaluator{program: program, props: props, byLower: map[string]*mpk.PropertyDef{}, cache: map[string]map[string]bool{}}
	for i := range props {
		e.byLower[strings.ToLower(props[i].Key)] = &props[i]
	}
	return e, nil
}

type loadKey struct {
	path     string
	widgetID string
	modTime  time.Time
	size     int64
}

type loaded struct {
	ev  *Evaluator
	err error
}

var (
	loadMu    sync.Mutex
	loadCache = map[loadKey]loaded{}
)

// ErrNoEditorConfig is returned when the package ships no editorConfig.js for
// the widget: there is nothing to run, and the caller keeps what it had.
var ErrNoEditorConfig = errors.New("widget ships no editorConfig.js")

// Load returns the evaluator for a widget in an .mpk, compiled once per
// package file (keyed on its size and modification time, so a replaced
// package is read again).
func Load(mpkPath, widgetID string) (*Evaluator, error) {
	st, err := os.Stat(mpkPath)
	if err != nil {
		return nil, err
	}
	key := loadKey{path: mpkPath, widgetID: widgetID, modTime: st.ModTime(), size: st.Size()}
	loadMu.Lock()
	if l, ok := loadCache[key]; ok {
		loadMu.Unlock()
		return l.ev, l.err
	}
	loadMu.Unlock()

	ev, err := load(mpkPath, widgetID)
	loadMu.Lock()
	loadCache[key] = loaded{ev: ev, err: err}
	loadMu.Unlock()
	return ev, err
}

func load(mpkPath, widgetID string) (*Evaluator, error) {
	js, err := mpk.ReadEditorConfig(mpkPath, widgetID)
	if err != nil {
		return nil, err
	}
	if js == "" {
		return nil, ErrNoEditorConfig
	}
	def, err := mpk.ParseMPKForWidget(mpkPath, widgetID)
	if err != nil {
		return nil, err
	}
	if def == nil {
		return nil, fmt.Errorf("widget %s not found in %s", widgetID, mpkPath)
	}
	return New(js, def.Properties)
}

// Hidden returns the top-level properties getProperties removes for the given
// values, keyed by the property key as the widget declares it.
//
// values is keyed by property key, compared case-insensitively, each value in
// the string form mxcli carries it in; a property it does not name takes its
// declared default. The result is cached per configuration.
func (e *Evaluator) Hidden(values map[string]string) (map[string]bool, error) {
	in := e.valuesObject(values)
	keyJSON, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	key := string(keyJSON)
	e.mu.Lock()
	if h, ok := e.cache[key]; ok {
		e.mu.Unlock()
		return h, nil
	}
	e.mu.Unlock()

	hidden, err := e.run(key)
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	e.cache[key] = hidden
	e.mu.Unlock()
	return hidden, nil
}

// collectKeys lists every property key left in a property-group tree, at any
// depth, but not those inside an object list's items.
const collectKeys = `(function collect(groups, out) {
  (groups || []).forEach(function (g) {
    (g.properties || []).forEach(function (p) { out.push(p.key) });
    collect(g.propertyGroups, out);
  });
  return out;
})`

func (e *Evaluator) run(valuesJSON string) (hidden map[string]bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			hidden, err = nil, fmt.Errorf("editorConfig.js: %v", r)
		}
	}()
	vm := goja.New()
	timer := time.AfterFunc(Timeout, func() { vm.Interrupt("getProperties did not return within " + Timeout.String()) })
	defer timer.Stop()

	module := vm.NewObject()
	exports := vm.NewObject()
	if err := module.Set("exports", exports); err != nil {
		return nil, err
	}
	for name, v := range map[string]any{"module": module, "exports": exports} {
		if err := vm.Set(name, v); err != nil {
			return nil, err
		}
	}
	if err := vm.Set("require", func(call goja.FunctionCall) goja.Value {
		panic(vm.NewTypeError("require(%s) is not available to an editorConfig", call.Argument(0).String()))
	}); err != nil {
		return nil, err
	}
	if _, err := vm.RunProgram(e.program); err != nil {
		return nil, fmt.Errorf("load editorConfig.js: %w", err)
	}
	getProperties, ok := goja.AssertFunction(module.Get("exports").ToObject(vm).Get("getProperties"))
	if !ok {
		// No getProperties: the editor shows every property.
		return map[string]bool{}, nil
	}

	treeJSON, err := json.Marshal(e.defaultProperties())
	if err != nil {
		return nil, err
	}
	parse, _ := goja.AssertFunction(vm.Get("JSON").ToObject(vm).Get("parse"))
	// Real JS arrays and objects, not Go values wrapped: the helpers filter and
	// splice the tree in place, which a wrapped Go slice does not survive.
	tree, err := parse(goja.Undefined(), vm.ToValue(string(treeJSON)))
	if err != nil {
		return nil, err
	}
	values, err := parse(goja.Undefined(), vm.ToValue(valuesJSON))
	if err != nil {
		return nil, err
	}
	result, err := getProperties(goja.Undefined(), values, tree, vm.ToValue(Target))
	if err != nil {
		return nil, fmt.Errorf("getProperties: %w", err)
	}
	collectV, err := vm.RunString(collectKeys)
	if err != nil {
		return nil, err
	}
	collect, _ := goja.AssertFunction(collectV)
	left, err := collect(goja.Undefined(), result, vm.NewArray())
	if err != nil {
		return nil, fmt.Errorf("read getProperties result: %w", err)
	}
	kept := map[string]bool{}
	if arr, ok := left.Export().([]any); ok {
		for _, k := range arr {
			kept[fmt.Sprint(k)] = true
		}
	}
	hidden = map[string]bool{}
	for _, p := range e.props {
		if !kept[p.Key] {
			hidden[p.Key] = true
		}
	}
	return hidden, nil
}

// valuesObject is the `values` argument: every declared property, in the shape
// Studio Pro hands an editorConfig — a boolean as a boolean, a number as a
// number (null when unset), an enumeration or text as a string, an object list
// as an array, and an unset action, data source, icon or image as null.
func (e *Evaluator) valuesObject(values map[string]string) map[string]any {
	lower := make(map[string]string, len(values))
	for k, v := range values {
		lower[strings.ToLower(k)] = v
	}
	out := make(map[string]any, len(e.props))
	for _, p := range e.props {
		v, set := lower[strings.ToLower(p.Key)]
		out[p.Key] = jsValue(p, v, set)
	}
	return out
}

func jsValue(p mpk.PropertyDef, v string, set bool) any {
	if !set {
		v = p.DefaultValue
	}
	switch strings.ToLower(p.Type) {
	case "boolean":
		return strings.EqualFold(v, "true")
	case "integer", "decimal":
		if v == "" {
			return nil
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil
		}
		return f
	case "object":
		return []any{}
	case "action", "datasource", "icon", "image", "file":
		if v == "" {
			return nil
		}
		return map[string]any{"type": v}
	case "widgets":
		n := 0
		if v != "" {
			n = 1
		}
		return map[string]any{"widgetCount": n}
	case "selection":
		if v == "" {
			return "None"
		}
		return v
	default:
		return v
	}
}

// defaultProperties is the `defaultProperties` tree: tabs holding property
// groups holding properties, from the categories the widget XML declares
// ("Tab::Group").
func (e *Evaluator) defaultProperties() []any {
	type group struct {
		caption string
		props   []any
	}
	var tabs []string
	byTab := map[string][]*group{}
	for _, p := range e.props {
		tab, name := "General", "General"
		parts := strings.SplitN(p.Category, "::", 2)
		if parts[0] != "" {
			tab = parts[0]
		}
		if len(parts) > 1 && parts[1] != "" {
			name = parts[1]
		}
		if _, ok := byTab[tab]; !ok {
			tabs = append(tabs, tab)
			byTab[tab] = nil
		}
		var g *group
		for _, x := range byTab[tab] {
			if x.caption == name {
				g = x
			}
		}
		if g == nil {
			g = &group{caption: name}
			byTab[tab] = append(byTab[tab], g)
		}
		node := map[string]any{"key": p.Key, "caption": p.Caption, "description": p.Description}
		if strings.EqualFold(p.Type, "object") {
			node["objectHeaders"] = []any{}
			node["objects"] = []any{}
		}
		g.props = append(g.props, node)
	}
	out := make([]any, 0, len(tabs))
	for _, t := range tabs {
		groups := make([]any, 0, len(byTab[t]))
		for _, g := range byTab[t] {
			groups = append(groups, map[string]any{"caption": g.caption, "properties": g.props})
		}
		out = append(out, map[string]any{"caption": t, "propertyGroups": groups})
	}
	return out
}

// SortedKeys returns a hidden set's keys in order, for messages.
func SortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k, v := range set {
		if v {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
