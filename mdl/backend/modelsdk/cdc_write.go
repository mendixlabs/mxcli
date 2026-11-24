// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/canon"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genCdc "github.com/mendixlabs/mxcli/modelsdk/gen/changedatacapture"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// zeroGUID is what Studio Pro stores in a VersionedProperty's
// AssociatedEntityGuid for an attribute: the property is always present, and
// the all-zero value means "no associated entity" rather than "unset". Writing
// the key out is therefore required, not optional (CLAUDE.md rule 4,
// canon.CompletePropertySets).
const zeroGUID = "00000000-0000-0000-0000-000000000000"

func init() {
	// The two sides of the document use DIFFERENT list markers, measured on the
	// Studio Pro fixture: the live side 3, the baseline 2. Nothing about the
	// shapes suggests that, and a writer that used one marker for both produces
	// a document Studio Pro rejects (the ako/mxcli#1640 class).
	codec.RegisterTypeDefaults("ChangeDataCapture$EntityChangeDataCaptureService", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Sources": 3},
	})
	codec.RegisterTypeDefaults("ChangeDataCapture$EntityChangeSource", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Properties": 3},
	})
	codec.RegisterTypeDefaults("ChangeDataCapture$VersionedEntityChangeDataCaptureService", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Sources": 2},
	})
	codec.RegisterTypeDefaults("ChangeDataCapture$VersionedEntityChangeSource", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Properties": 2},
	})
	codec.RegisterListMarker("ChangeDataCapture$EntityChangeSource", 3)
	codec.RegisterListMarker("ChangeDataCapture$AttributeProperty", 3)
	codec.RegisterListMarker("ChangeDataCapture$AssociationProperty", 3)
	codec.RegisterListMarker("ChangeDataCapture$VersionedEntityChangeSource", 2)
	codec.RegisterListMarker("ChangeDataCapture$VersionedProperty", 2)
}

// cdcTypeForAttribute is the CDC type of a Mendix attribute.
//
// Taken from Studio Pro's own change-data-capture editor, which mxbuild ships
// as readable JavaScript (modeler/ide-client/change-data-capture-editor/
// editor.js) and which carries the mapping as an explicit switch on the
// attribute type's $Type. Those cases are every Mendix attribute type, so the
// mapping is total and the editor's `default: throw` is unreachable for a valid
// model. This answers §9.3 of PROPOSAL_kafka_event_integration.md, which had
// expected to need a Studio Pro export.
//
// Date is mxcli's own spelling, not a Mendix type: it stores as
// DomainModels$DateTimeAttributeType ("Mendix has no date-only type",
// domainmodel_write.go), so it is a DateTime here too.
func cdcTypeForAttribute(a *domainmodel.Attribute) (string, error) {
	if a == nil || a.Type == nil {
		return "", fmt.Errorf("attribute has no type")
	}
	switch a.Type.GetTypeName() {
	case "Binary":
		return "Binary", nil
	case "Boolean":
		return "Boolean", nil
	case "DateTime", "Date":
		return "DateTime", nil
	case "Decimal":
		return "Decimal", nil
	case "Enumeration":
		return "Enumeration", nil
	case "Integer":
		return "Int32", nil
	case "Long", "AutoNumber":
		return "Int64", nil
	case "String", "HashedString":
		return "String", nil
	default:
		return "", fmt.Errorf("attribute type %s has no change data capture type", a.Type.GetTypeName())
	}
}

// resolvedMember is one tracked member with both identities: the qualified name
// the live side stores, and the GUID the baseline stores.
type resolvedMember struct {
	qualified   string // Module.Entity.Attribute, or Module.Association
	exposedName string
	isAssoc     bool
	guid        string // the member's own GUID
	assocGUID   string // an association's TO entity GUID; zeroGUID for an attribute
	cdcType     string
}

// resolvedSource is one tracked entity, resolved against the domain model.
type resolvedSource struct {
	entityQN    string
	exposedName string
	entityGUID  string
	members     []resolvedMember
}

// resolveCdcSources turns the authored sources into both sides' worth of data.
//
// Every GUID is READ from the domain model, never minted: the runtime keys
// mendixsystem$entity.id and mendixsystem$attribute.id on these values, so a
// writer that invented one would detach a table from its rows on the next
// deploy, with no build error and nothing in a diff to see (CLAUDE.md, "A GUID
// Is the Database's Identity"). That is the whole reason the baseline is
// computed here rather than authored in MDL.
func (b *Backend) resolveCdcSources(sources []*model.ChangeSource) ([]resolvedSource, error) {
	dms, err := b.ListDomainModels()
	if err != nil {
		return nil, fmt.Errorf("reading domain models: %w", err)
	}
	mods, err := b.ListModules()
	if err != nil {
		return nil, fmt.Errorf("reading modules: %w", err)
	}
	moduleName := map[model.ID]string{}
	for _, m := range mods {
		moduleName[m.ID] = m.Name
	}

	// GUIDs come from the raw unit; nothing above raw BSON exposes them.
	guids := map[model.ID]map[string]string{} // domain model unit -> $ID -> GUID
	guidOf := func(dm *domainmodel.DomainModel, id model.ID) (string, error) {
		m, ok := guids[dm.ID]
		if !ok {
			raw, err := b.GetRawUnitBytes(dm.ID)
			if err != nil {
				return "", fmt.Errorf("reading domain model %s: %w", dm.ID, err)
			}
			m = canon.ElementGUIDs(raw)
			if m == nil {
				return "", fmt.Errorf("domain model %s could not be read for its GUIDs", dm.ID)
			}
			guids[dm.ID] = m
		}
		g, ok := m[string(id)]
		if !ok || g == "" {
			return "", fmt.Errorf("element %s has no stored GUID: change data capture identifies what it tracks by GUID, and minting one would detach the entity from its table", id)
		}
		return g, nil
	}

	var out []resolvedSource
	for _, src := range sources {
		modName, entName, ok := splitQualified(src.Entity)
		if !ok {
			return nil, fmt.Errorf("entity %q is not a qualified name (Module.Entity)", src.Entity)
		}
		var dm *domainmodel.DomainModel
		var ent *domainmodel.Entity
		for _, cand := range dms {
			if !strings.EqualFold(moduleName[cand.ContainerID], modName) {
				continue
			}
			for _, e := range cand.Entities {
				if strings.EqualFold(e.Name, entName) {
					dm, ent = cand, e
				}
			}
		}
		if ent == nil {
			return nil, fmt.Errorf("entity %s not found", src.Entity)
		}
		// A view entity and an external entity both report Persistable = true in
		// the semantic model, so the flag does not tell them apart from a real
		// table -- Source does. Neither has rows of its own to capture: a view is
		// a query over other entities, and an external entity's rows live in
		// another app. Checked here because nothing downstream would say so:
		// mx check passes, and the service simply publishes nothing.
		if ent.Source != "" {
			return nil, fmt.Errorf("entity %s has no table of its own (source %s), so there are no committed changes to capture: change data capture tracks persistable entities", src.Entity, ent.Source)
		}
		if !ent.Persistable {
			return nil, fmt.Errorf("entity %s is not persistable: change data capture publishes committed database changes, so there is nothing for it to observe", src.Entity)
		}

		entGUID, err := guidOf(dm, ent.ID)
		if err != nil {
			return nil, fmt.Errorf("entity %s: %w", src.Entity, err)
		}
		rs := resolvedSource{
			entityQN:    modName + "." + ent.Name,
			exposedName: src.ExposedName,
			entityGUID:  entGUID,
		}
		if rs.exposedName == "" {
			rs.exposedName = ent.Name
		}

		for _, mem := range src.Properties {
			name := mem.Member()
			rm, err := b.resolveCdcMember(dm, ent, modName, name, guidOf)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", src.Entity, err)
			}
			rm.exposedName = mem.ExposedName
			if rm.exposedName == "" {
				rm.exposedName = name
			}
			rs.members = append(rs.members, rm)
		}
		out = append(out, rs)
	}
	return out, nil
}

// resolveCdcMember finds a member by name on one entity. Attributes are looked
// at first, as Studio Pro's editor does, and a name that is both is an error
// rather than a silent choice.
func (b *Backend) resolveCdcMember(
	dm *domainmodel.DomainModel, ent *domainmodel.Entity, modName, name string,
	guidOf func(*domainmodel.DomainModel, model.ID) (string, error),
) (resolvedMember, error) {
	var attr *domainmodel.Attribute
	for _, a := range ent.Attributes {
		if strings.EqualFold(a.Name, name) {
			attr = a
		}
	}
	// An association belongs to its FROM entity — the one stored in
	// ParentPointer (CLAUDE.md, "Association Parent/Child Pointer Semantics").
	// Tracking it from the TO side is not a thing the model can express.
	var assoc *domainmodel.Association
	for _, a := range dm.Associations {
		if strings.EqualFold(a.Name, name) && a.ParentID == ent.ID {
			assoc = a
		}
	}
	switch {
	case attr != nil && assoc != nil:
		return resolvedMember{}, fmt.Errorf("%q is both an attribute and an association of %s: rename one, or the tracked member is ambiguous", name, ent.Name)
	case attr != nil:
		t, err := cdcTypeForAttribute(attr)
		if err != nil {
			return resolvedMember{}, fmt.Errorf("attribute %q: %w", name, err)
		}
		g, err := guidOf(dm, attr.ID)
		if err != nil {
			return resolvedMember{}, fmt.Errorf("attribute %q: %w", name, err)
		}
		return resolvedMember{
			qualified: modName + "." + ent.Name + "." + attr.Name,
			guid:      g,
			assocGUID: zeroGUID,
			cdcType:   t,
		}, nil
	case assoc != nil:
		g, err := guidOf(dm, assoc.ID)
		if err != nil {
			return resolvedMember{}, fmt.Errorf("association %q: %w", name, err)
		}
		// The baseline records the TO entity's GUID, which is the association's
		// Child end.
		var toGUID string
		for _, e := range dm.Entities {
			if e.ID == assoc.ChildID {
				if toGUID, err = guidOf(dm, e.ID); err != nil {
					return resolvedMember{}, fmt.Errorf("association %q target: %w", name, err)
				}
			}
		}
		if toGUID == "" {
			return resolvedMember{}, fmt.Errorf("association %q points outside this module: a cross-module association is not supported here yet", name)
		}
		return resolvedMember{
			qualified: modName + "." + assoc.Name,
			isAssoc:   true,
			guid:      g,
			assocGUID: toGUID,
			cdcType:   "ArrayOfInt64",
		}, nil
	default:
		return resolvedMember{}, fmt.Errorf("%q is not an attribute or an association of %s", name, ent.Name)
	}
}

func splitQualified(qn string) (module, name string, ok bool) {
	i := strings.Index(qn, ".")
	if i <= 0 || i == len(qn)-1 {
		return "", "", false
	}
	return qn[:i], qn[i+1:], true
}

// appNameForProject is the topic's second segment on a NEW service.
//
// INFERRED, not measured: the fixture's AppName is "TestApp" and its project is
// TestApp.mpr, which is consistent with the project file's name but does not
// distinguish that from a settings value that happens to match. It is preserved
// verbatim on modify, and `describe` prints the resulting topic, so a wrong
// value here is visible rather than silent. MDL cannot set it (the visitor
// refuses AppName) precisely because changing it re-points every topic.
func (b *Backend) appNameForProject() string {
	base := filepath.Base(b.path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// buildCdcService builds the document from resolved sources: the live side, the
// baseline that mirrors it, and the pointers between them.
//
// The baseline is a mirror by construction rather than by copying: mxbuild
// reports CE1585 when the two sides disagree, and Studio Pro's "accept changes"
// is exactly this rebuild. Building it from the same resolved data is what makes
// the two agree without comparing them.
func buildCdcService(
	id element.ID, name, serviceName, appName, documentation, exportLevel string, excluded bool,
	sources []resolvedSource, versions cdcVersions,
) element.Element {
	svc := genCdc.NewEntityChangeDataCaptureService()
	svc.SetID(id)
	svc.SetName(name)
	svc.SetServiceName(serviceName)
	svc.SetAppName(appName)
	svc.SetDocumentation(documentation)
	svc.SetExportLevel(exportLevel)
	// Studio Pro writes Excluded on the service, so it is written here even at
	// its default: merge compares property NAMES, and an absent key is a
	// different document from one holding false (CLAUDE.md rule 4,
	// canon.CompletePropertySets, mendixlabs/mxcli#1373). The shape test caught
	// this one rather than a reviewer.
	svc.SetExcluded(excluded)

	baseline := genCdc.NewVersionedEntityChangeDataCaptureService()
	baseline.SetID(element.ID(mmpr.GenerateID()))
	baseline.SetVersion(newCdcVersion(versions.service))

	for _, rs := range sources {
		// The baseline element is built first so the live side has an $ID to
		// point at. Pointers are primitive properties holding an element.ID, so
		// nothing walks them for us (CLAUDE.md rule 1).
		vSrc := genCdc.NewVersionedEntityChangeSource()
		vSrc.SetID(element.ID(mmpr.GenerateID()))
		vSrc.SetEntityGuid(rs.entityGUID)
		vSrc.SetExposedName(rs.exposedName)
		vSrc.SetVersion(newCdcVersion(versions.source[rs.exposedName]))

		src := genCdc.NewEntityChangeSource()
		src.SetID(element.ID(mmpr.GenerateID()))
		src.SetEntityQualifiedName(rs.entityQN)
		src.SetExposedName(rs.exposedName)
		src.SetVersionedSourceID(vSrc.ID())

		for _, rm := range rs.members {
			vProp := genCdc.NewVersionedProperty()
			vProp.SetID(element.ID(mmpr.GenerateID()))
			vProp.SetExposedName(rm.exposedName)
			vProp.SetPropertyGuid(rm.guid)
			vProp.SetAssociatedEntityGuid(rm.assocGUID)
			vProp.SetType(rm.cdcType)
			vSrc.AddProperties(vProp)

			if rm.isAssoc {
				p := genCdc.NewAssociationProperty()
				p.SetID(element.ID(mmpr.GenerateID()))
				p.SetAssociationQualifiedName(rm.qualified)
				p.SetExposedName(rm.exposedName)
				p.SetVersionedPropertyID(vProp.ID())
				src.AddProperties(p)
				continue
			}
			p := genCdc.NewAttributeProperty()
			p.SetID(element.ID(mmpr.GenerateID()))
			p.SetAttributeQualifiedName(rm.qualified)
			p.SetExposedName(rm.exposedName)
			p.SetVersionedPropertyID(vProp.ID())
			src.AddProperties(p)
		}

		baseline.AddSources(vSrc)
		svc.AddSources(src)
	}

	svc.SetBaseline(baseline)
	return svc
}

func newCdcVersion(v model.ChangeVersion) element.Element {
	g := genCdc.NewVersion()
	g.SetID(element.ID(mmpr.GenerateID()))
	g.SetMajor(int32(v.Major))
	g.SetMinor(int32(v.Minor))
	return g
}

// cdcVersions is the revision each source and the service carry.
//
// §5.2's policy, which the fixture is consistent with (Order 1.1 after an
// association was added, Customer 1.0, service 1.1) but which is INFERRED from
// that one document: §9.2 still wants a Studio Pro session that adds, renames
// and removes a property and records what it picks. mxbuild ignores the numbers
// (§6), so a wrong rule is not a build error — it is a contract signal to
// consumers, and a wrong MAJOR silently starts a new topic.
type cdcVersions struct {
	service model.ChangeVersion
	source  map[string]model.ChangeVersion // keyed by exposed name
}

// freshCdcVersions is what a newly created service carries: 1.0 throughout.
func freshCdcVersions(sources []resolvedSource) cdcVersions {
	v := cdcVersions{
		service: model.ChangeVersion{Major: 1, Minor: 0},
		source:  map[string]model.ChangeVersion{},
	}
	for _, rs := range sources {
		v.source[rs.exposedName] = model.ChangeVersion{Major: 1, Minor: 0}
	}
	return v
}

// CreateChangeDataCaptureService inserts a new
// ChangeDataCapture$EntityChangeDataCaptureService document.
func (b *Backend) CreateChangeDataCaptureService(svc *model.ChangeDataCaptureService) error {
	if svc == nil {
		return fmt.Errorf("CreateChangeDataCaptureService: nil service")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateChangeDataCaptureService: not connected for writing")
	}
	sources, err := b.resolveCdcSources(svc.Sources)
	if err != nil {
		return fmt.Errorf("CreateChangeDataCaptureService: %w", err)
	}
	if svc.ID == "" {
		svc.ID = model.ID(mmpr.GenerateID())
	}
	serviceName := svc.ServiceName
	if serviceName == "" {
		serviceName = svc.Name
	}
	appName := svc.AppName
	if appName == "" {
		appName = b.appNameForProject()
	}
	g := buildCdcService(element.ID(svc.ID), svc.Name, serviceName, appName,
		svc.Documentation, svc.ExportLevel, svc.Excluded, sources, freshCdcVersions(sources))
	contents, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		return fmt.Errorf("CreateChangeDataCaptureService: encode: %w", err)
	}
	return b.writer.InsertUnit(string(svc.ID), string(svc.ContainerID), "Documents",
		"ChangeDataCapture$EntityChangeDataCaptureService", contents)
}

// DeleteChangeDataCaptureService removes the document by ID.
func (b *Backend) DeleteChangeDataCaptureService(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteChangeDataCaptureService: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

// bumpCdcVersions is the revision each source carries after a modify.
//
// §5.2's policy, INFERRED from one Studio Pro document (§9.2 wants a session
// that adds, renames and removes a property and records what it picks):
//
//   - a source whose member set is unchanged keeps its stored version;
//   - a source that only gained members takes minor+1;
//   - a source that lost a member, renamed an exposed name or changed a type
//     takes major+1, minor 0;
//   - a new source starts at 1.0;
//   - the service takes the largest bump any source took.
//
// mxbuild ignores the numbers (§6), so a wrong rule is not a build error. It is
// a contract signal to consumers, and a wrong MAJOR is worse than cosmetic: the
// major is a topic segment, so bumping it when nothing needed it starts a new
// topic and, downstream, a new table with no history.
func bumpCdcVersions(stored *model.ChangeBaseline, sources []resolvedSource) cdcVersions {
	out := cdcVersions{source: map[string]model.ChangeVersion{}}
	if stored == nil {
		return freshCdcVersions(sources)
	}
	type storedSource struct {
		version model.ChangeVersion
		members map[string]string // exposed name -> CDC type
	}
	was := map[string]storedSource{}
	for _, s := range stored.Sources {
		ss := storedSource{members: map[string]string{}}
		if s.Version != nil {
			ss.version = *s.Version
		}
		for _, p := range s.Properties {
			ss.members[p.ExposedName] = p.Type
		}
		was[s.ExposedName] = ss
	}

	major, minor := false, false
	for _, rs := range sources {
		prev, existed := was[rs.exposedName]
		if !existed {
			// A new source is 1.0 and does not bump the service: it publishes
			// to a topic of its own, so no existing consumer is affected.
			out.source[rs.exposedName] = model.ChangeVersion{Major: 1, Minor: 0}
			continue
		}
		added, breaking := false, false
		now := map[string]string{}
		for _, rm := range rs.members {
			now[rm.exposedName] = rm.cdcType
			t, had := prev.members[rm.exposedName]
			switch {
			case !had:
				added = true
			case t != rm.cdcType:
				breaking = true // a type change is not something a consumer can absorb
			}
		}
		for name := range prev.members {
			if _, still := now[name]; !still {
				breaking = true // removed, or an exposed name renamed
			}
		}
		switch {
		case breaking:
			out.source[rs.exposedName] = model.ChangeVersion{Major: prev.version.Major + 1, Minor: 0}
			major = true
		case added:
			out.source[rs.exposedName] = model.ChangeVersion{Major: prev.version.Major, Minor: prev.version.Minor + 1}
			minor = true
		default:
			out.source[rs.exposedName] = prev.version
		}
	}

	svc := model.ChangeVersion{Major: 1, Minor: 0}
	if stored.Version != nil {
		svc = *stored.Version
	}
	switch {
	case major:
		out.service = model.ChangeVersion{Major: svc.Major + 1, Minor: 0}
	case minor:
		out.service = model.ChangeVersion{Major: svc.Major, Minor: svc.Minor + 1}
	default:
		out.service = svc
	}
	return out
}

// UpdateChangeDataCaptureService rewrites an existing service (CREATE OR MODIFY).
//
// The whole document is rebuilt rather than patched, because the baseline must
// mirror the live side and the pointers must be internally consistent: a
// partial edit is how an $ID comes to point at nothing (CLAUDE.md rule 1, "a
// unit is rewritten wholesale or not at all"). Storage elides the write when
// the result is semantically equal to what is stored (ADR-0008), so rebuilding
// an unchanged service is not a churn.
//
// AppName is preserved from the stored document, never recomputed: it is the
// topic's second segment, and re-deriving it would re-point every topic if the
// project file were ever renamed.
func (b *Backend) UpdateChangeDataCaptureService(svc *model.ChangeDataCaptureService) error {
	if svc == nil {
		return fmt.Errorf("UpdateChangeDataCaptureService: nil service")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateChangeDataCaptureService: not connected for writing")
	}
	sources, err := b.resolveCdcSources(svc.Sources)
	if err != nil {
		return fmt.Errorf("UpdateChangeDataCaptureService: %w", err)
	}

	var stored *model.ChangeDataCaptureService
	existing, err := b.ListChangeDataCaptureServices()
	if err != nil {
		return fmt.Errorf("UpdateChangeDataCaptureService: reading the stored service: %w", err)
	}
	for _, s := range existing {
		if s.ID == svc.ID {
			stored = s
		}
	}
	if stored == nil {
		return fmt.Errorf("UpdateChangeDataCaptureService: %s is not in the project", svc.Name)
	}

	serviceName := svc.ServiceName
	if serviceName == "" {
		serviceName = stored.ServiceName
	}
	if serviceName == "" {
		serviceName = svc.Name
	}
	appName := stored.AppName
	if appName == "" {
		appName = b.appNameForProject()
	}
	// Documentation arrives already resolved: the executor decides between
	// preserving the stored value and clearing it (carriedDocumentation,
	// mendixlabs/mxcli#1018), so the backend must not second-guess an empty one.
	g := buildCdcService(element.ID(svc.ID), svc.Name, serviceName, appName,
		svc.Documentation, stored.ExportLevel, stored.Excluded,
		sources, bumpCdcVersions(stored.Baseline, sources))
	contents, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		return fmt.Errorf("UpdateChangeDataCaptureService: encode: %w", err)
	}
	return b.writer.UpdateRawUnit(string(svc.ID), contents)
}
