// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/version"
)

// Mendix 11.17 moved a workflow activity's boundary events into containers
// (ako/mxcli#2616). Measured with mx convert 11.16.1 -> 11.17.0 on a workflow
// holding every kind, and in the 11.17 metamodel (registry main.js):
//
//	11.16: Activity.BoundaryEvents = [ X$BoundaryEvent{Annotation, Caption,
//	         FirstExecutionTime, Flow{Activities: [path…]}, Name (notification),
//	         PersistentId, Recurrence (non-interrupting timer)} ]
//	11.17: Activity.BoundaryEventContainers = [ Workflows$BoundaryEventContainer{
//	         Flow{Activities: [X$BoundaryEventActivity{Annotation, Caption,
//	         FirstExecutionTime, Name, PersistentId, Recurrence,
//	         RelativeMiddlePoint "0;0", Size "0;0"}, path…]}, PersistentId} ]
//
// The container and the event activity both carry the old event's
// PersistentId. A notification event keeps its Name; a timer, which had none,
// is named boundaryEvent1, boundaryEvent2, … in document order. A microflow's
// Workflows$NotifyNotificationBoundaryEventTarget renames BoundaryEvent to
// Activity, same qualified name.
//
// The writers and readers keep the 11.16 shape: RaiseBoundaryEventContainers
// runs on every write to an 11.17+ project, before canon's identity carry, so
// the comparison with the stored unit is like for like; LowerBoundaryEventContainers
// runs where a stored unit is decoded or walked, so nothing else needs to know
// the new shape. Lowering keeps a timer's Name, which the reader carries, so a
// rewrite gives it back rather than renumbering.

var boundaryContainersSince = version.Version{Major: 11, Minor: 17, Patch: 0}

const (
	boundaryEventsKey      = "BoundaryEvents"
	boundaryContainersKey  = "BoundaryEventContainers"
	boundaryContainerType  = "Workflows$BoundaryEventContainer"
	notifyBoundaryTarget   = "Workflows$NotifyNotificationBoundaryEventTarget"
	boundaryEventSuffix    = "BoundaryEvent"
	boundaryActivitySuffix = "BoundaryEventActivity"
	timerNamePrefix        = "boundaryEvent"
)

// RaiseBoundaryEventContainers rewrites a unit in the 11.16 boundary-event
// shape into the one a project of version pv stores. Below 11.17, or for a unit
// with neither shape, it returns contents unchanged.
func RaiseBoundaryEventContainers(contents []byte, pv *version.Version) ([]byte, error) {
	if pv == nil || pv.IsZero() || pv.Compare(boundaryContainersSince) < 0 {
		return contents, nil
	}
	if !bytes.Contains(contents, []byte(boundaryEventsKey)) && !bytes.Contains(contents, []byte(notifyBoundaryTarget)) {
		return contents, nil
	}
	var doc bson.D
	if err := bson.Unmarshal(contents, &doc); err != nil {
		return contents, nil
	}
	r := &boundaryRaise{root: doc, taken: map[string]bool{}}
	r.collectNames(doc)
	out, changed := r.walk(doc)
	if !changed {
		return contents, nil
	}
	b, err := bson.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("raise boundary events: %w", err)
	}
	return b, nil
}

type boundaryRaise struct {
	root  bson.D
	used  map[string]bool // element $IDs, for derived ids
	taken map[string]bool // names in the document
	next  int
}

func (r *boundaryRaise) collectNames(v any) {
	switch n := v.(type) {
	case bson.D:
		if s, ok := shapeString(n, "Name"); ok && s != "" {
			r.taken[s] = true
		}
		for _, e := range n {
			r.collectNames(e.Value)
		}
	case bson.A:
		for _, e := range n {
			r.collectNames(e)
		}
	}
}

// timerName is the next boundaryEventN no element of the document has.
func (r *boundaryRaise) timerName() string {
	for {
		r.next++
		n := timerNamePrefix + strconv.Itoa(r.next)
		if !r.taken[n] {
			r.taken[n] = true
			return n
		}
	}
}

func (r *boundaryRaise) walk(v any) (any, bool) {
	switch n := v.(type) {
	case bson.D:
		changed := false
		if t, _ := shapeString(n, "$Type"); t == notifyBoundaryTarget {
			if i := shapeIndex(n, "BoundaryEvent"); i >= 0 {
				n[i].Key = "Activity"
				changed = true
			}
		}
		// An activity's own events are named before anything nested in it,
		// as mx convert numbers them.
		if i := shapeIndex(n, boundaryEventsKey); i >= 0 {
			n[i] = bson.E{Key: boundaryContainersKey, Value: r.containers(n[i].Value)}
			changed = true
		}
		for i := range n {
			if nv, c := r.walk(n[i].Value); c {
				n[i].Value = nv
				changed = true
			}
		}
		return n, changed
	case bson.A:
		changed := false
		for i := range n {
			if nv, c := r.walk(n[i]); c {
				n[i] = nv
				changed = true
			}
		}
		return n, changed
	}
	return v, false
}

// containers turns a BoundaryEvents list into BoundaryEventContainers. The
// events' own flows are walked afterwards, by walk, once they sit inside the
// containers.
func (r *boundaryRaise) containers(v any) bson.A {
	out := bson.A{int32(2)}
	for _, item := range listItems(v) {
		ev, ok := item.(bson.D)
		if !ok {
			continue
		}
		out = append(out, r.container(ev))
	}
	return out
}

func (r *boundaryRaise) container(ev bson.D) bson.D {
	typ, _ := shapeString(ev, "$Type")
	persistentID, _ := shapeValue(ev, "PersistentId")
	act := bson.D{}
	if id, ok := shapeValue(ev, "$ID"); ok {
		act = append(act, bson.E{Key: "$ID", Value: id})
	}
	act = append(act, bson.E{Key: "$Type", Value: strings.TrimSuffix(typ, boundaryEventSuffix) + boundaryActivitySuffix})
	// Annotation is on every event activity mx convert writes (null for
	// none); the 11.16 event got it from the property-set table, which has no
	// row for the 11.17 types.
	annotation, _ := shapeValue(ev, "Annotation")
	act = append(act, bson.E{Key: "Annotation", Value: annotation})
	for _, k := range []string{"Caption", "FirstExecutionTime"} {
		if v, ok := shapeValue(ev, k); ok {
			act = append(act, bson.E{Key: k, Value: v})
		}
	}
	name, _ := shapeString(ev, "Name")
	if name == "" {
		name = r.timerName()
	}
	act = append(act, bson.E{Key: "Name", Value: name})
	if persistentID != nil {
		act = append(act, bson.E{Key: "PersistentId", Value: persistentID})
	}
	if v, ok := shapeValue(ev, "Recurrence"); ok {
		act = append(act, bson.E{Key: "Recurrence", Value: v})
	}
	act = append(act, bson.E{Key: "RelativeMiddlePoint", Value: "0;0"}, bson.E{Key: "Size", Value: "0;0"})

	flow := bson.D{}
	path := bson.A{int32(3), act}
	if f, ok := shapeValue(ev, "Flow"); ok {
		if fd, ok := f.(bson.D); ok {
			for _, e := range fd {
				if e.Key != "Activities" {
					flow = append(flow, e)
				}
			}
			path = append(path, listItems(shapeOr(fd, "Activities"))...)
		}
	}
	if shapeIndex(flow, "$ID") < 0 {
		flow = append(bson.D{{Key: "$ID", Value: r.derivedID(act, "Flow")}}, flow...)
	}
	if shapeIndex(flow, "$Type") < 0 {
		flow = append(flow, bson.E{Key: "$Type", Value: "Workflows$Flow"})
	}
	flow = append(flow, bson.E{Key: "Activities", Value: path})

	c := bson.D{
		{Key: "$ID", Value: r.derivedID(act, "Container")},
		{Key: "$Type", Value: boundaryContainerType},
		{Key: "Flow", Value: flow},
	}
	if persistentID != nil {
		c = append(c, bson.E{Key: "PersistentId", Value: persistentID})
	}
	return c
}

func (r *boundaryRaise) derivedID(owner bson.D, key string) bson.Binary {
	if r.used == nil {
		r.used = elementIDs(r.root)
	}
	return completionID(elementIDBytes(owner), "RaiseBoundaryEventContainers/"+key, r.used)
}

func shapeOr(d bson.D, key string) any {
	v, _ := shapeValue(d, key)
	return v
}

// LowerBoundaryEventContainers rewrites a stored unit in the 11.17
// boundary-event shape into the 11.16 one the readers decode. A unit without
// containers or a renamed notify target is returned unchanged.
func LowerBoundaryEventContainers(contents []byte) []byte {
	if !bytes.Contains(contents, []byte(boundaryContainersKey)) && !bytes.Contains(contents, []byte(notifyBoundaryTarget)) {
		return contents
	}
	var doc bson.D
	if err := bson.Unmarshal(contents, &doc); err != nil {
		return contents
	}
	out, changed := lowerBoundaryWalk(doc)
	if !changed {
		return contents
	}
	b, err := bson.Marshal(out)
	if err != nil {
		return contents
	}
	return b
}

func lowerBoundaryWalk(v any) (any, bool) {
	switch n := v.(type) {
	case bson.D:
		changed := false
		if t, _ := shapeString(n, "$Type"); t == notifyBoundaryTarget {
			if i := shapeIndex(n, "Activity"); i >= 0 {
				n[i].Key = "BoundaryEvent"
				changed = true
			}
		}
		if i := shapeIndex(n, boundaryContainersKey); i >= 0 {
			events := bson.A{int32(2)}
			for _, item := range listItems(n[i].Value) {
				if c, ok := item.(bson.D); ok {
					if ev := lowerContainer(c); ev != nil {
						events = append(events, ev)
					}
				}
			}
			n[i] = bson.E{Key: boundaryEventsKey, Value: events}
			changed = true
		}
		for i := range n {
			if nv, c := lowerBoundaryWalk(n[i].Value); c {
				n[i].Value = nv
				changed = true
			}
		}
		return n, changed
	case bson.A:
		changed := false
		for i := range n {
			if nv, c := lowerBoundaryWalk(n[i]); c {
				n[i] = nv
				changed = true
			}
		}
		return n, changed
	}
	return v, false
}

// lowerContainer is the 11.16 event a container holds: its flow's first
// activity, with the rest of the flow as the event's path. nil when the
// container's flow does not start with a boundary event activity.
func lowerContainer(c bson.D) bson.D {
	fd, ok := shapeOr(c, "Flow").(bson.D)
	if !ok {
		return nil
	}
	acts := listItems(shapeOr(fd, "Activities"))
	if len(acts) == 0 {
		return nil
	}
	act, ok := acts[0].(bson.D)
	if !ok {
		return nil
	}
	typ, _ := shapeString(act, "$Type")
	if !strings.HasSuffix(typ, boundaryActivitySuffix) {
		return nil
	}
	ev := bson.D{}
	if id, ok := shapeValue(act, "$ID"); ok {
		ev = append(ev, bson.E{Key: "$ID", Value: id})
	}
	ev = append(ev, bson.E{Key: "$Type", Value: strings.TrimSuffix(typ, boundaryActivitySuffix) + boundaryEventSuffix})
	for _, k := range []string{"Annotation", "Caption", "FirstExecutionTime"} {
		if v, ok := shapeValue(act, k); ok {
			ev = append(ev, bson.E{Key: k, Value: v})
		}
	}
	flow := bson.D{}
	for _, e := range fd {
		if e.Key != "Activities" {
			flow = append(flow, e)
		}
	}
	flow = append(flow, bson.E{Key: "Activities", Value: append(bson.A{int32(3)}, acts[1:]...)})
	ev = append(ev, bson.E{Key: "Flow", Value: flow})
	for _, k := range []string{"Name", "PersistentId", "Recurrence"} {
		if v, ok := shapeValue(act, k); ok {
			ev = append(ev, bson.E{Key: k, Value: v})
		}
	}
	return ev
}
