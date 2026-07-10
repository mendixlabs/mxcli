// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/property"
)

func init() {
	// Resources / Operations / operation Parameters serialize with the typed-array
	// marker 2 (populated keyed by child $Type; empty via MandatoryListMarkers). The
	// service's AllowedRoles is a marker-1 reference-string list, AuthenticationTypes
	// and Parameters are empty marker-2 lists, and CorsConfiguration is BSON null.
	codec.RegisterListMarker("Rest$PublishedRestServiceResource", 2)
	codec.RegisterListMarker("Rest$PublishedRestServiceOperation", 2)
	codec.RegisterListMarker("Rest$RestOperationParameter", 2)
	codec.RegisterTypeDefaults("Rest$PublishedRestService", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"AllowedRoles": 1, "AuthenticationTypes": 2, "Parameters": 2},
		NullFields:           []string{"CorsConfiguration"},
	})
	codec.RegisterTypeDefaults("Rest$PublishedRestServiceResource", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Operations": 2},
	})
	codec.RegisterTypeDefaults("Rest$PublishedRestServiceOperation", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Parameters": 2},
	})
}

// CreatePublishedRestService inserts a new Rest$PublishedRestService document
// (resources → operations → path parameters). Mirrors the legacy serializer.
func (b *Backend) CreatePublishedRestService(svc *model.PublishedRestService) error {
	if svc == nil {
		return fmt.Errorf("CreatePublishedRestService: nil service")
	}
	if b.writer == nil {
		return fmt.Errorf("CreatePublishedRestService: not connected for writing")
	}
	if svc.ID == "" {
		svc.ID = model.ID(mmpr.GenerateID())
	}
	svc.TypeName = "Rest$PublishedRestService"
	contents, err := (&codec.Encoder{}).Encode(publishedRestServiceToGen(svc))
	if err != nil {
		return fmt.Errorf("CreatePublishedRestService: encode: %w", err)
	}
	return b.writer.InsertUnit(string(svc.ID), string(svc.ContainerID), "Documents", "Rest$PublishedRestService", contents)
}

// UpdatePublishedRestService rewrites an existing published REST service in place
// (CREATE OR MODIFY / ALTER PUBLISHED REST SERVICE).
func (b *Backend) UpdatePublishedRestService(svc *model.PublishedRestService) error {
	if svc == nil {
		return fmt.Errorf("UpdatePublishedRestService: nil service")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdatePublishedRestService: not connected for writing")
	}
	contents, err := (&codec.Encoder{}).Encode(publishedRestServiceToGen(svc))
	if err != nil {
		return fmt.Errorf("UpdatePublishedRestService: encode: %w", err)
	}
	return b.writer.UpdateRawUnit(string(svc.ID), contents)
}

// DeletePublishedRestService removes a published REST service unit by ID.
func (b *Backend) DeletePublishedRestService(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeletePublishedRestService: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

// UpdatePublishedRestServiceRoles patches just the AllowedRoles field (marker-1
// reference-string array) on an existing service, preserving the rest of the
// document. Used by GRANT/REVOKE on a published REST service.
func (b *Backend) UpdatePublishedRestServiceRoles(unitID model.ID, roles []string) error {
	if b.writer == nil {
		return fmt.Errorf("UpdatePublishedRestServiceRoles: not connected for writing")
	}
	raw, err := b.reader.GetRawUnitBytes(string(unitID))
	if err != nil {
		return fmt.Errorf("UpdatePublishedRestServiceRoles: load unit: %w", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		return fmt.Errorf("UpdatePublishedRestServiceRoles: unmarshal: %w", err)
	}
	arr := bson.A{int32(1)}
	for _, r := range roles {
		arr = append(arr, r)
	}
	set := false
	for i := range d {
		if d[i].Key == "AllowedRoles" {
			d[i].Value = arr
			set = true
			break
		}
	}
	if !set {
		d = append(d, bson.E{Key: "AllowedRoles", Value: arr})
	}
	out, err := bson.Marshal(d)
	if err != nil {
		return fmt.Errorf("UpdatePublishedRestServiceRoles: marshal: %w", err)
	}
	return b.writer.UpdateRawUnit(string(unitID), out)
}

func publishedRestServiceToGen(svc *model.PublishedRestService) element.Element {
	g := newElem("Rest$PublishedRestService", string(svc.ID))
	addStr(g, "Name", svc.Name)
	addStr(g, "Documentation", "")
	addBool(g, "Excluded", svc.Excluded)
	addStr(g, "ExportLevel", "Hidden")
	addStr(g, "Path", svc.Path)
	addStr(g, "Version", svc.Version)
	addStr(g, "ServiceName", svc.ServiceName)
	if len(svc.AllowedRoles) > 0 {
		addByNameRefList(g, "AllowedRoles", "Security$ModuleRole", svc.AllowedRoles)
	}
	// AllowedRoles (empty), AuthenticationTypes, Parameters, CorsConfiguration:
	// emitted via the registered TypeDefaults.
	addStr(g, "AuthenticationMicroflow", "")

	resources := make([]element.Element, 0, len(svc.Resources))
	for _, res := range svc.Resources {
		r := newElem("Rest$PublishedRestServiceResource", string(res.ID))
		addStr(r, "Name", res.Name)
		addStr(r, "Documentation", "")
		ops := make([]element.Element, 0, len(res.Operations))
		for _, op := range res.Operations {
			ops = append(ops, publishedRestOperationToGen(op))
		}
		if len(ops) > 0 {
			addPartList(r, "Operations", ops)
		}
		resources = append(resources, r)
	}
	if len(resources) > 0 {
		addPartList(g, "Resources", resources)
	}
	return g
}

func publishedRestOperationToGen(op *model.PublishedRestOperation) element.Element {
	g := newElem("Rest$PublishedRestServiceOperation", string(op.ID))
	addStr(g, "HttpMethod", httpMethodToMendix(op.HTTPMethod))
	addStr(g, "Path", op.Path)
	addStr(g, "Microflow", op.Microflow)
	addStr(g, "Summary", op.Summary)
	addBool(g, "Deprecated", op.Deprecated)
	addStr(g, "Commit", "Yes")
	addStr(g, "Documentation", "")
	addStr(g, "ExportMapping", "")
	addStr(g, "ImportMapping", "")
	addStr(g, "ObjectHandlingBackup", "Create")
	// Path parameters are auto-extracted from {name} placeholders and wired to the
	// matching microflow parameter (Module.Microflow.name) — without that wiring
	// mx check raises CE6538 / CE0350.
	params := make([]element.Element, 0)
	for _, name := range extractPathParams(op.Path) {
		p := newElem("Rest$RestOperationParameter", "")
		addStr(p, "Name", name)
		addPart(p, "Type", newElem("DataTypes$StringType", ""))
		addStr(p, "ParameterType", "Path")
		mfParam := ""
		if op.Microflow != "" {
			mfParam = op.Microflow + "." + name
		}
		addStr(p, "MicroflowParameter", mfParam)
		addStr(p, "Description", "")
		params = append(params, p)
	}
	if len(params) > 0 {
		addPartList(g, "Parameters", params)
	}
	return g
}

// addByNameRefList adds a marker-1 reference-string list property (qualified
// names), the form Mendix uses for AllowedRoles / AllowedModuleRoles.
func addByNameRefList(b *element.Base, name, targetType string, qnames []string) {
	p := property.NewByNameRefList[element.Element](name, targetType)
	b.AddProperty(p, uint(len(b.Properties())))
	for _, qn := range qnames {
		p.Append(qn)
	}
}

// extractPathParams returns parameter names from {param} placeholders in a path.
func extractPathParams(path string) []string {
	var names []string
	for {
		start := strings.Index(path, "{")
		if start < 0 {
			break
		}
		end := strings.Index(path[start:], "}")
		if end < 0 {
			break
		}
		names = append(names, path[start+1:start+end])
		path = path[start+end+1:]
	}
	return names
}

// httpMethodToMendix converts an HTTP method name to Mendix casing.
func httpMethodToMendix(method string) string {
	switch strings.ToUpper(method) {
	case "GET":
		return "Get"
	case "POST":
		return "Post"
	case "PUT":
		return "Put"
	case "PATCH":
		return "Patch"
	case "DELETE":
		return "Delete"
	case "HEAD":
		return "Head"
	case "OPTIONS":
		return "Options"
	default:
		return method
	}
}
