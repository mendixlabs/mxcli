// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mfmutator"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ALTER MICROFLOW / NANOFLOW over MCP (plan item 4.2f).
//
// The splice itself is the shared engine (mfmutator), run on the stored form of
// the flow as the local .mpr holds it — the same decisions, placement and
// refusals as on the modelsdk backend. What differs is the write: Studio Pro
// takes no unit bytes, so Save diffs the spliced document against the stored
// one and sends the difference as ped_update_document path operations on the
// live document: positions set, new objects and flows added, rewired flow ends
// set, removed elements removed — and, in a second update, the new elements
// filled in (flowPatchOps says why). PED addresses list entries by index, and those
// indexes are the stored order — so before anything is sent, the live document
// is read back and compared with the local one, and a mismatch (the .mpr is
// behind Studio Pro) refuses the write rather than patching the wrong element.

// OpenMicroflowForMutation opens a microflow or nanoflow for splicing.
func (b *Backend) OpenMicroflowForMutation(unitID model.ID) (backend.MicroflowMutator, error) {
	raw, err := b.reader.GetRawUnitBytes(unitID)
	if err != nil {
		return nil, fmt.Errorf("alter over MCP reads the stored flow from the local project: %w", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	docType, _ := docValue(d, "$Type").(string)
	name, _ := docValue(d, "Name").(string)
	qn, err := b.flowQualifiedName(unitID, docType, name)
	if err != nil {
		return nil, err
	}
	deps := &mcpFlowDeps{b: b, docType: docType, qn: qn, stored: d, objects: map[string]microflows.MicroflowObject{},
		flows: map[string]*microflows.SequenceFlow{}, annotations: map[string]*microflows.AnnotationFlow{}}
	var work bson.D
	if err := bson.Unmarshal(raw, &work); err != nil {
		return nil, err
	}
	m, err := mfmutator.New(work, unitID, deps)
	if err != nil {
		return nil, err
	}
	return &mcpFlowMutator{Mutator: m, qn: qn}, nil
}

// mcpFlowMutator is the shared splice, limited to what PED applies safely:
// inserts. A PED update that fails is rolled back EXCEPT for its removals,
// which persist (PED's own warning, and measured: a failed update left the
// removed activity gone together with the flows attached to it). A drop or a
// replace needs a removal, so over MCP they are refused rather than risked on
// the live model; run them against the .mpr instead.
type mcpFlowMutator struct {
	*mfmutator.Mutator
	qn string
}

func (m *mcpFlowMutator) Replace(model.ID, *backend.MicroflowFragment) error {
	return fmt.Errorf("replace is not supported by the MCP backend yet: Studio Pro does not roll back a removal when an update fails; run without --mcp to alter %s in the .mpr", m.qn)
}

func (m *mcpFlowMutator) Drop(model.ID) error {
	return fmt.Errorf("drop is not supported by the MCP backend yet: Studio Pro does not roll back a removal when an update fails; run without --mcp to alter %s in the .mpr", m.qn)
}

// ReplaceLoopBody is refused like a replace: it removes the stored body.
func (m *mcpFlowMutator) ReplaceLoopBody(model.ID, *backend.MicroflowFragment) error {
	return fmt.Errorf("replace body is not supported by the MCP backend yet: Studio Pro does not roll back a removal when an update fails; run without --mcp to alter %s in the .mpr", m.qn)
}

// InsertAfter and InsertBefore inside a loop body are refused: the patch Save
// sends addresses top-level objects only, so neither the objects added to the
// body nor a loop box grown to fit them would reach Studio Pro.
func (m *mcpFlowMutator) InsertAfter(target model.ID, frag *backend.MicroflowFragment) error {
	if m.InLoopBody(target) {
		return m.inLoopRefusal()
	}
	return m.Mutator.InsertAfter(target, frag)
}

func (m *mcpFlowMutator) InsertBefore(target model.ID, frag *backend.MicroflowFragment) error {
	if m.InLoopBody(target) {
		return m.inLoopRefusal()
	}
	return m.Mutator.InsertBefore(target, frag)
}

func (m *mcpFlowMutator) inLoopRefusal() error {
	return fmt.Errorf("inserting inside a loop body is not supported by the MCP backend yet: the update it sends addresses top-level objects only; run without --mcp to alter %s in the .mpr", m.qn)
}

func (m *mcpFlowMutator) RemoveNotes(model.ID) error {
	return fmt.Errorf("removing notes is not supported by the MCP backend yet: Studio Pro does not roll back a removal when an update fails; run without --mcp to alter %s in the .mpr", m.qn)
}

// AddNotes is refused like RemoveNotes, which a changed note always comes with.
func (m *mcpFlowMutator) AddNotes(model.ID, *backend.MicroflowFragment) error {
	return fmt.Errorf("changing notes is not supported by the MCP backend yet; run without --mcp to alter %s in the .mpr", m.qn)
}

// SetFreeNotes is refused like RemoveNotes: it removes the stored free notes.
func (m *mcpFlowMutator) SetFreeNotes(*backend.MicroflowFragment) error {
	return fmt.Errorf("changing notes is not supported by the MCP backend yet; run without --mcp to alter %s in the .mpr", m.qn)
}

// SetReturnValue is refused like a replace: the patch Save sends carries
// positions, pointers and added or removed elements only, so a changed value
// would not reach Studio Pro and the edit would be lost without a word.
func (m *mcpFlowMutator) SetReturnValue(model.ID, string) error {
	return fmt.Errorf("changing a return value is not supported by the MCP backend yet; run without --mcp to alter %s in the .mpr", m.qn)
}

// SetCondition is refused like SetReturnValue, for the same reason.
func (m *mcpFlowMutator) SetCondition(model.ID, string, string) error {
	return fmt.Errorf("changing a decision's condition is not supported by the MCP backend yet; run without --mcp to alter %s in the .mpr", m.qn)
}

// SetCaseValues is refused like SetCondition, for the same reason.
func (m *mcpFlowMutator) SetCaseValues(model.ID, []backend.CaseBranchEdit, [][]string) error {
	return fmt.Errorf("changing a case's values is not supported by the MCP backend yet; run without --mcp to alter %s in the .mpr", m.qn)
}

// SetHeader is refused like SetReturnValue: the patch Save sends carries
// positions, pointers and added or removed flow elements only, so a changed
// document property would not reach Studio Pro.
func (m *mcpFlowMutator) SetHeader(any) ([]string, error) {
	return nil, fmt.Errorf("changing a document property or a parameter is not supported by the MCP backend yet; run without --mcp to modify %s in the .mpr", m.qn)
}

func (b *Backend) flowQualifiedName(id model.ID, docType, name string) (string, error) {
	var container model.ID
	switch docType {
	case microflowDocType:
		mf, err := b.GetMicroflow(id)
		if err != nil {
			return "", err
		}
		container = mf.ContainerID
	case "Microflows$Nanoflow":
		nfs, err := b.reader.ListNanoflows()
		if err != nil {
			return "", err
		}
		for _, nf := range nfs {
			if nf.ID == id {
				container = nf.ContainerID
			}
		}
	default:
		return "", fmt.Errorf("unit %s is a %s, not a microflow or nanoflow", id, docType)
	}
	mod, err := b.moduleNameForContainer(container)
	if err != nil {
		return "", err
	}
	return mod + "." + name, nil
}

// mcpFlowDeps is mfmutator.Deps for the MCP backend. Serialize produces only
// what the splice reads (identity, type, geometry, pointers) and keeps the
// domain object, which is what PED is sent; SaveUnit turns the spliced
// document into PED operations.
type mcpFlowDeps struct {
	b       *Backend
	docType string
	qn      string
	stored  bson.D

	objects     map[string]microflows.MicroflowObject
	flows       map[string]*microflows.SequenceFlow
	annotations map[string]*microflows.AnnotationFlow
}

func binaryOf(id model.ID) bson.Binary {
	return bson.Binary{Subtype: 0, Data: types.UUIDToBlob(string(id))}
}

func (d *mcpFlowDeps) SerializeObject(obj microflows.MicroflowObject) (bson.D, error) {
	d.objects[string(obj.GetID())] = obj
	p := obj.GetPosition()
	w, h := 0, 0
	if s, ok := obj.(interface{ GetSize() model.Size }); ok {
		w, h = s.GetSize().Width, s.GetSize().Height
	}
	return bson.D{
		{Key: "$ID", Value: binaryOf(obj.GetID())},
		{Key: "$Type", Value: "mcp-new-object"},
		{Key: "RelativeMiddlePoint", Value: fmt.Sprintf("%d;%d", p.X, p.Y)},
		{Key: "Size", Value: fmt.Sprintf("%d;%d", w, h)},
	}, nil
}

func (d *mcpFlowDeps) SerializeSequenceFlow(f *microflows.SequenceFlow) (bson.D, error) {
	d.flows[string(f.ID)] = f
	return bson.D{
		{Key: "$ID", Value: binaryOf(f.ID)},
		{Key: "$Type", Value: "Microflows$SequenceFlow"},
		{Key: "DestinationConnectionIndex", Value: int32(f.DestinationConnectionIndex)},
		{Key: "DestinationPointer", Value: binaryOf(f.DestinationID)},
		{Key: "IsErrorHandler", Value: f.IsErrorHandler},
		{Key: "Line", Value: bson.D{
			{Key: "$Type", Value: "Microflows$BezierCurve"},
			{Key: "DestinationControlVector", Value: f.DestinationControlVector},
			{Key: "OriginControlVector", Value: f.OriginControlVector},
		}},
		{Key: "OriginConnectionIndex", Value: int32(f.OriginConnectionIndex)},
		{Key: "OriginPointer", Value: binaryOf(f.OriginID)},
	}, nil
}

func (d *mcpFlowDeps) SerializeAnnotationFlow(f *microflows.AnnotationFlow) (bson.D, error) {
	d.annotations[string(f.ID)] = f
	return bson.D{
		{Key: "$ID", Value: binaryOf(f.ID)},
		{Key: "$Type", Value: "Microflows$AnnotationFlow"},
		{Key: "DestinationPointer", Value: binaryOf(f.DestinationID)},
		{Key: "OriginPointer", Value: binaryOf(f.OriginID)},
	}, nil
}

// SerializeDocument is never reached: mcpFlowMutator refuses SetHeader.
func (d *mcpFlowDeps) SerializeDocument(any) (bson.D, error) {
	return nil, fmt.Errorf("%s: document properties are not patched over MCP", d.qn)
}

// SaveUnit sends the difference between the stored and the spliced document.
func (d *mcpFlowDeps) SaveUnit(_ string, contents []byte) error {
	var spliced bson.D
	if err := bson.Unmarshal(contents, &spliced); err != nil {
		return err
	}
	ops, fill, err := d.b.flowPatchOps(d, spliced)
	if err != nil {
		return err
	}
	if len(ops) == 0 {
		return nil
	}
	if err := d.b.checkLiveFlowMatches(d.docType, d.qn, d.stored); err != nil {
		return err
	}
	// Only inserts reach here (mcpFlowMutator refuses the rest), so there is
	// nothing to remove; a removal would mean the splice did something this
	// path was not built for, and PED would not roll it back on a failure.
	for _, op := range ops {
		if op.Operation.Type == "remove" {
			return fmt.Errorf("%s: refusing to send a removal over MCP", d.qn)
		}
	}
	// Two updates, because PED cannot address an element in the update that
	// adds it (see flowPatchOps). The first is all or nothing: the new
	// elements, wired in, and the stored ones moved to make room. The second
	// fills in the new elements. If it fails, the first is not rolled back:
	// the insert is left in Studio Pro without what the second carried, as a
	// create is when setting its activities fails (CreateMicroflow), and
	// ped_check_errors names what is missing.
	if err := d.b.pedUpdateDoc(d.docType, d.qn, ops...); err != nil {
		return err
	}
	if len(fill) > 0 {
		if err := d.b.pedUpdateDoc(d.docType, d.qn, fill...); err != nil {
			return fmt.Errorf("the insert into %s was added in Studio Pro, but setting up its new activities and flows failed (the insert is left in Studio Pro without them): %w", d.qn, err)
		}
	}
	return d.b.pedCheckDocument(d.docType, d.qn)
}

// flowList returns a unit's top-level objects or flows, in stored order, with
// their $IDs.
func flowList(d bson.D, objects bool) (ids []string, docs []bson.D) {
	list := docValue(d, "Flows")
	if objects {
		oc, _ := docValue(d, "ObjectCollection").(bson.D)
		list = docValue(oc, "Objects")
	}
	a, _ := list.(bson.A)
	for i, el := range a {
		if i == 0 {
			if _, marker := el.(int32); marker {
				continue
			}
		}
		e, ok := el.(bson.D)
		if !ok {
			continue
		}
		b, _ := docValue(e, "$ID").(bson.Binary)
		ids = append(ids, types.BlobToUUID(b.Data))
		docs = append(docs, e)
	}
	return ids, docs
}

func docValue(d bson.D, key string) any {
	for _, e := range d {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

func pointerID(d bson.D, key string) string {
	b, _ := docValue(d, key).(bson.Binary)
	return types.BlobToUUID(b.Data)
}

func storedPointToPED(s string) map[string]int {
	x, y, _ := strings.Cut(s, ";")
	px, _ := strconv.Atoi(x)
	py, _ := strconv.Atoi(y)
	return map[string]int{"x": px, "y": py}
}

func pedVector(s string) map[string]int {
	p := storedPointToPED(s)
	return map[string]int{"width": p["x"], "height": p["y"]}
}

// newFlowLineOps sets the control vectors of a sequence flow the insert added,
// at fp. A 0;0 vector is the line's schema default and is left out.
func newFlowLineOps(fp string, f *microflows.SequenceFlow) []pedOpEntry {
	var ops []pedOpEntry
	for _, v := range []struct{ key, vec string }{
		{"originControlVector", f.OriginControlVector},
		{"destinationControlVector", f.DestinationControlVector},
	} {
		if v.vec == "" || v.vec == "0;0" {
			continue
		}
		ops = append(ops, pedOpEntry{Path: fp + "/line/" + v.key, Operation: pedOperation{Type: "set", Value: pedVector(v.vec)}})
	}
	return ops
}

// pedQualifiedFlowType maps the storage $Type of a flow object to the
// qualified name PED reports for it, where the two differ (CLAUDE.md, "BSON
// Storage Names vs Qualified Names").
var pedQualifiedFlowType = map[string]string{
	"Microflows$MicroflowParameter": "Microflows$MicroflowParameterObject",
}

// flowPatchOps derives the PED operations that turn the stored document into
// the spliced one. Order matters, because PED addresses entries by index:
// everything that uses a stored index (positions, rewired flows) and every
// append goes first, while indexes are still the stored ones; removals go
// last, highest index first.
//
// They come in two updates. PED resolves a path against the document as it
// was before the update — its own rule for indexes — so an element the update
// adds cannot be addressed in it (measured on 11.15.0-rc.4). A set below a new
// flow's line fails the whole update with `undefined is not an object
// (evaluating 'u.$Type')`; any other set on a new element — an activity's
// action, its position, a flow's isErrorHandler or whole line — is accepted
// and then missing on read-back, and so is the same property put in the
// constructor value. The same sets land in a later update. So ops adds the new
// elements (constructor properties only, references as $id(), which do
// resolve) and changes the stored ones; fill sets the rest on the new elements,
// for SaveUnit to send once they exist.
//
// Every add carries the array's stored length as its index. Appended without
// one, several elements added to the same array in one update are stored in
// reverse (measured on 11.15.0-rc.4: three objects, two flows), and $id()
// resolves against that final order — so an insert of more than one element
// was wired to the wrong objects. Added at one shared index they are stored as
// listed, which is PED's own rule for adding several elements at a position.
func (b *Backend) flowPatchOps(d *mcpFlowDeps, spliced bson.D) (ops, fill []pedOpEntry, err error) {
	set := func(path string, v any) {
		ops = append(ops, pedOpEntry{Path: path, Operation: pedOperation{Type: "set", Value: v}})
	}
	oldObjIDs, oldObjs := flowList(d.stored, true)
	newObjIDs, newObjs := flowList(spliced, true)
	oldFlowIDs, oldFlows := flowList(d.stored, false)
	newFlowIDs, newFlows := flowList(spliced, false)

	path := map[string]string{}
	oldObjIndex := map[string]int{}
	for i, id := range oldObjIDs {
		oldObjIndex[id] = i
		path[id] = fmt.Sprintf("/objectCollection/objects/%d", i)
	}
	survivingObj := map[string]bool{}
	var added []string
	for i, id := range newObjIDs {
		if idx, ok := oldObjIndex[id]; ok {
			survivingObj[id] = true
			before, _ := docValue(oldObjs[idx], "RelativeMiddlePoint").(string)
			after, _ := docValue(newObjs[i], "RelativeMiddlePoint").(string)
			if before != after {
				set(path[id]+"/relativeMiddlePoint", storedPointToPED(after))
			}
			continue
		}
		added = append(added, id)
	}
	for j, id := range added {
		path[id] = fmt.Sprintf("/objectCollection/objects/%d", len(oldObjIDs)+j)
	}
	objectsEnd, flowsEnd := len(oldObjIDs), len(oldFlowIDs)
	for _, id := range added {
		obj, ok := d.objects[id]
		if !ok {
			return nil, nil, fmt.Errorf("internal: spliced object %s was not serialized", id)
		}
		idPath := map[model.ID]string{}
		m, err := b.mapObjectTree(obj, path[id], idPath)
		if err != nil {
			return nil, nil, err
		}
		skeletonObject(m, path[id], func(p string, v any) {
			fill = append(fill, pedOpEntry{Path: p, Operation: pedOperation{Type: "set", Value: v}})
		})
		ops = append(ops, pedOpEntry{Path: "/objectCollection/objects", Operation: pedOperation{Type: "add", Value: m, Index: &objectsEnd}})
		// The constructor's x/y place it on 11.15; the set is for a server
		// whose skeleton constructor ignores them.
		p := obj.GetPosition()
		fill = append(fill, pedOpEntry{Path: path[id] + "/relativeMiddlePoint", Operation: pedOperation{Type: "set", Value: map[string]int{"x": p.X, "y": p.Y}}})
		for nested, np := range idPath {
			path[string(nested)] = np
		}
	}
	ref := func(id string) (string, error) {
		p, ok := path[id]
		if !ok {
			return "", fmt.Errorf("a flow points at %s, which is not a top-level object PED can address", id)
		}
		return "$id(" + p + ")", nil
	}

	oldFlowIndex := map[string]int{}
	for i, id := range oldFlowIDs {
		oldFlowIndex[id] = i
	}
	survivingFlow := map[string]bool{}
	var addedFlows []string
	for i, id := range newFlowIDs {
		idx, ok := oldFlowIndex[id]
		if !ok {
			addedFlows = append(addedFlows, id)
			continue
		}
		survivingFlow[id] = true
		fp := fmt.Sprintf("/flows/%d", idx)
		o, n := oldFlows[idx], newFlows[i]
		for _, end := range []string{"Origin", "Destination"} {
			key := strings.ToLower(end)
			if pointerID(o, end+"Pointer") != pointerID(n, end+"Pointer") {
				r, err := ref(pointerID(n, end+"Pointer"))
				if err != nil {
					return nil, nil, err
				}
				set(fp+"/"+key, r)
			}
			if fmt.Sprint(docValue(o, end+"ConnectionIndex")) != fmt.Sprint(docValue(n, end+"ConnectionIndex")) {
				set(fp+"/"+key+"ConnectionIndex", docValue(n, end+"ConnectionIndex"))
			}
			ol, _ := docValue(o, "Line").(bson.D)
			nl, _ := docValue(n, "Line").(bson.D)
			if ov, nv := fmt.Sprint(docValue(ol, end+"ControlVector")), fmt.Sprint(docValue(nl, end+"ControlVector")); ov != nv && nv != "" {
				set(fp+"/line/"+key+"ControlVector", pedVector(nv))
			}
		}
	}
	// Sequence flows are added before note lines, the order a stored document
	// has them in, so fill addresses the new sequence flows from the stored
	// length up.
	var addedNotes []*microflows.AnnotationFlow
	next := len(oldFlowIDs)
	for _, id := range addedFlows {
		if af, ok := d.annotations[id]; ok {
			addedNotes = append(addedNotes, af)
			continue
		}
		fp := fmt.Sprintf("/flows/%d", next)
		next++
		f, ok := d.flows[id]
		if !ok {
			return nil, nil, fmt.Errorf("internal: spliced flow %s was not serialized", id)
		}
		o, err := ref(string(f.OriginID))
		if err != nil {
			return nil, nil, err
		}
		dst, err := ref(string(f.DestinationID))
		if err != nil {
			return nil, nil, err
		}
		v := map[string]any{"$Type": "Microflows$SequenceFlow", "originId": o, "destinationId": dst}
		setPEDSides(v, f)
		cv, err := mapCaseValue(f.CaseValue)
		if err != nil {
			return nil, nil, err
		}
		if cv != nil {
			v["caseValue"] = cv
		}
		ops = append(ops, pedOpEntry{Path: "/flows", Operation: pedOperation{Type: "add", Value: v, Index: &flowsEnd}})
		if f.IsErrorHandler {
			fill = append(fill, pedOpEntry{Path: fp + "/isErrorHandler", Operation: pedOperation{Type: "set", Value: true}})
		}
		fill = append(fill, newFlowLineOps(fp, f)...)
	}
	for _, af := range addedNotes {
		o, err := ref(string(af.OriginID))
		if err != nil {
			return nil, nil, err
		}
		dst, err := ref(string(af.DestinationID))
		if err != nil {
			return nil, nil, err
		}
		ops = append(ops, pedOpEntry{Path: "/flows", Operation: pedOperation{Type: "add", Value: pedAnnotationFlow(af, o, dst), Index: &flowsEnd}})
	}

	for i := len(oldFlowIDs) - 1; i >= 0; i-- {
		if !survivingFlow[oldFlowIDs[i]] {
			idx := i
			ops = append(ops, pedOpEntry{Path: "/flows", Operation: pedOperation{Type: "remove", Index: &idx}})
		}
	}
	for i := len(oldObjIDs) - 1; i >= 0; i-- {
		if !survivingObj[oldObjIDs[i]] {
			idx := i
			ops = append(ops, pedOpEntry{Path: "/objectCollection/objects", Operation: pedOperation{Type: "remove", Index: &idx}})
		}
	}
	return ops, fill, nil
}

// checkLiveFlowMatches compares the live document's objects and flows with the
// stored ones the splice ran on: the same count, and each object the same type
// at the same position. PED addresses them by index, so a live document that
// has moved on since the .mpr was saved would have the patch applied to other
// elements than the ones the splice chose.
func (b *Backend) checkLiveFlowMatches(docType, qn string, stored bson.D) error {
	res, err := b.client.CallTool("ped_read_document", map[string]any{
		"documentType": docType,
		"documentName": qn,
		"paths":        []string{"/objectCollection/objects", "/flows"},
	})
	if err != nil {
		return err
	}
	if res.IsError {
		return fmt.Errorf("ped_read_document %s: %s", qn, pedStripReminder(res.Text))
	}
	var body struct {
		Results []struct {
			Path   string          `json:"path"`
			Result json.RawMessage `json:"result"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(pedStripReminder(res.Text)), &body); err != nil {
		return fmt.Errorf("read %s from Studio Pro: %w", qn, err)
	}
	stale := func(what string) error {
		return fmt.Errorf("%s in Studio Pro no longer matches the local project (%s); save in Studio Pro so the .mpr is current, then retry", qn, what)
	}
	_, objs := flowList(stored, true)
	_, flows := flowList(stored, false)
	for _, r := range body.Results {
		var live []map[string]any
		if err := json.Unmarshal(r.Result, &live); err != nil {
			return fmt.Errorf("read %s %s from Studio Pro: %w", qn, r.Path, err)
		}
		switch r.Path {
		case "/flows":
			if len(live) != len(flows) {
				return stale(fmt.Sprintf("%d flows live, %d stored", len(live), len(flows)))
			}
		case "/objectCollection/objects":
			if len(live) != len(objs) {
				return stale(fmt.Sprintf("%d objects live, %d stored", len(live), len(objs)))
			}
			for i, o := range objs {
				typ, _ := docValue(o, "$Type").(string)
				if q, ok := pedQualifiedFlowType[typ]; ok {
					typ = q
				}
				if live[i]["$Type"] != typ {
					return stale(fmt.Sprintf("object %d is a %v live, a %s stored", i, live[i]["$Type"], typ))
				}
				rp, _ := docValue(o, "RelativeMiddlePoint").(string)
				want := storedPointToPED(rp)
				pt, _ := live[i]["relativeMiddlePoint"].(map[string]any)
				if fmt.Sprint(pt["x"]) != strconv.Itoa(want["x"]) || fmt.Sprint(pt["y"]) != strconv.Itoa(want["y"]) {
					return stale(fmt.Sprintf("object %d is at %v live, at %s stored", i, pt, rp))
				}
			}
		}
	}
	return nil
}
