// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// widgetValueChange names the properties of one pluggable widget whose values
// differ between two states of a project.
//
// `mx update-widgets` clears a CE0463 caused by a value in a property the
// widget's editorConfig hides by resetting the property to its default: the
// stored type stays byte-identical and only the values move (measured on
// 11.15.0, ako/mxcli#2507). So a "stale widget definition" label, and a quiet
// `fix widgets`, dropped the values a script had set. Comparing the values
// before and after the tool is what tells a resync from a reset.
type widgetValueChange struct {
	Document   string   // the unit's Name (a page, snippet or layout)
	Widget     string   // the widget's Name
	Properties []string // property keys, sorted
}

func (c widgetValueChange) String() string {
	return fmt.Sprintf("%s on '%s' (%s)", strings.Join(c.Properties, ", "), c.Widget, c.Document)
}

// widgetValues are a project's pluggable widget property values: widget →
// property key → the value in a form that ignores $IDs and pointers.
type widgetValues map[widgetRef]map[string]string

type widgetRef struct{ unit, document, widget string }

// widgetPropertyValues reads every CustomWidgets$CustomWidget in the units.
func widgetPropertyValues(units map[string]harvestedUnit) widgetValues {
	out := widgetValues{}
	for id, u := range units {
		if !bytes.Contains(u.contents, []byte("CustomWidgets$CustomWidget")) {
			continue
		}
		var doc bson.D
		if err := bson.Unmarshal(u.contents, &doc); err != nil {
			continue
		}
		name, _ := docString(doc, "Name")
		walkDocs(doc, func(d bson.D) {
			if t, _ := docString(d, "$Type"); t != "CustomWidgets$CustomWidget" {
				return
			}
			w, _ := docString(d, "Name")
			out[widgetRef{unit: id, document: name, widget: w}] = customWidgetValues(d)
		})
	}
	return out
}

// customWidgetValues maps each property of the widget's object to its value,
// by the property key its TypePointer names in the widget's own type.
func customWidgetValues(w bson.D) map[string]string {
	keys := map[string]string{}
	if typ, ok := docField(w, "Type").(bson.D); ok {
		walkDocs(typ, func(d bson.D) {
			if t, _ := docString(d, "$Type"); t != "CustomWidgets$WidgetPropertyType" {
				return
			}
			if k, ok := docString(d, "PropertyKey"); ok {
				keys[binaryKey(docField(d, "$ID"))] = k
			}
		})
	}
	vals := map[string]string{}
	obj, _ := docField(w, "Object").(bson.D)
	for _, p := range docArray(obj, "Properties") {
		pd, ok := p.(bson.D)
		if !ok {
			continue
		}
		key := keys[binaryKey(docField(pd, "TypePointer"))]
		if key == "" {
			continue
		}
		vals[key] = canonicalValue(docField(pd, "Value"))
	}
	return vals
}

// widgetValueChanges lists the widgets whose property values differ, in a
// stable order. A widget or property that exists on one side only is not a
// value change and is not listed.
func widgetValueChanges(before, after widgetValues) []widgetValueChange {
	var out []widgetValueChange
	for ref, bv := range before {
		av, ok := after[ref]
		if !ok {
			continue
		}
		var props []string
		for k, v := range bv {
			if a, ok := av[k]; ok && a != v {
				props = append(props, k)
			}
		}
		if len(props) == 0 {
			continue
		}
		sort.Strings(props)
		out = append(out, widgetValueChange{Document: ref.document, Widget: ref.widget, Properties: props})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Document != out[j].Document {
			return out[i].Document < out[j].Document
		}
		return out[i].Widget < out[j].Widget
	})
	return out
}

// canonicalValue renders a value without the parts every re-save renews:
// $IDs, binary fields (pointers to other elements, by $ID), and key order —
// update-widgets writes a DataSource's keys sorted where Studio Pro does not,
// which listed 24 of mxchess's widgets as changed when no value was (11.15.0).
func canonicalValue(v any) string {
	b, err := bson.MarshalExtJSON(bson.D{{Key: "v", Value: stripIdentity(v)}}, true, false)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

func stripIdentity(v any) any {
	switch t := v.(type) {
	case bson.D:
		out := bson.D{}
		for _, e := range t {
			if e.Key == "$ID" {
				continue
			}
			if _, isBin := e.Value.(bson.Binary); isBin {
				continue
			}
			out = append(out, bson.E{Key: e.Key, Value: stripIdentity(e.Value)})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
		return out
	case bson.A:
		out := bson.A{}
		for _, e := range t {
			out = append(out, stripIdentity(e))
		}
		return out
	}
	return v
}

func walkDocs(v any, fn func(bson.D)) {
	switch t := v.(type) {
	case bson.D:
		fn(t)
		for _, e := range t {
			walkDocs(e.Value, fn)
		}
	case bson.A:
		for _, e := range t {
			walkDocs(e, fn)
		}
	}
}

func docField(d bson.D, key string) any {
	for _, e := range d {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

func docString(d bson.D, key string) (string, bool) {
	s, ok := docField(d, key).(string)
	return s, ok
}

func docArray(d bson.D, key string) bson.A {
	a, _ := docField(d, key).(bson.A)
	return a
}

func binaryKey(v any) string {
	if b, ok := v.(bson.Binary); ok {
		return string(b.Data)
	}
	return ""
}
