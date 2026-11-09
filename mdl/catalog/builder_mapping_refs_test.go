// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#1861 (mendixlabs/mxcli#1703): a flow that applies an import or
// export mapping, and a mapping built on a JSON structure, made no refs row, so
// `refs`, `show callers` and every orphan check answered "not referenced" for a
// mapping or structure in use. ActionRef was empty for the same actions.
func mappingRefsCatalog(t *testing.T) *Catalog {
	t.Helper()
	const mod = model.ID("mod-t")
	flow := func(id, name string, objs ...microflows.MicroflowObject) *microflows.Microflow {
		return &microflows.Microflow{
			BaseElement: model.BaseElement{ID: model.ID(id)}, ContainerID: mod, Name: name,
			ObjectCollection: &microflows.MicroflowObjectCollection{Objects: objs},
		}
	}
	imp := &model.ImportMapping{ContainerID: mod, Name: "IMM_Order", JsonStructure: "T.JSON_Order"}
	imp.ID = "imm-1"
	exp := &model.ExportMapping{ContainerID: mod, Name: "EMM_Order", JsonStructure: "T.JSON_Order"}
	exp.ID = "emm-1"
	xmlImp := &model.ImportMapping{ContainerID: mod, Name: "IMM_Xml", XmlSchema: "T.XSD_Order"} // control: not a JSON structure
	xmlImp.ID = "imm-2"
	js := &types.JsonStructure{ContainerID: mod, Name: "JSON_Order"}
	js.ID = "js-1"

	be := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) { return &model.ProjectSettings{}, nil },
		ListModuleSettingsFunc: func() ([]*types.ModuleSettings, error) { return nil, nil },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{{BaseElement: model.BaseElement{ID: mod}, Name: "T"}}, nil
		},
		ListRulesFunc:          func() ([]*microflows.Rule, error) { return nil, nil },
		ListNanoflowsFunc:      func() ([]*microflows.Nanoflow, error) { return nil, nil },
		GetNavigationFunc:      func() (*types.NavigationDocument, error) { return &types.NavigationDocument{}, nil },
		ListImportMappingsFunc: func() ([]*model.ImportMapping, error) { return []*model.ImportMapping{imp, xmlImp}, nil },
		ListExportMappingsFunc: func() ([]*model.ExportMapping, error) { return []*model.ExportMapping{exp}, nil },
		ListJsonStructuresFunc: func() ([]*types.JsonStructure, error) { return []*types.JsonStructure{js}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return []*microflows.Microflow{
				flow("mf-1", "Import", newAction("a1", &microflows.ImportXmlAction{
					ResultHandling: &microflows.ResultHandlingMapping{MappingID: "T.IMM_Order"}})),
				flow("mf-2", "Export", newAction("a2", &microflows.ExportXmlAction{
					RequestHandling: &microflows.MappingRequestHandling{MappingID: "T.EMM_Order"}})),
				flow("mf-3", "Rest", newAction("a3", &microflows.RestCallAction{
					RequestHandling: &microflows.MappingRequestHandling{MappingID: "T.EMM_Order"},
					ResultHandling:  &microflows.ResultHandlingMapping{MappingID: "T.IMM_Order"}})),
				flow("mf-4", "Soap", newAction("a4", &microflows.WebServiceCallAction{
					SendMappingID: "T.EMM_Order", ReceiveMappingID: "T.IMM_Order"})),
				// Control: a REST call with a string result names no mapping.
				flow("mf-5", "RestString", newAction("a5", &microflows.RestCallAction{
					ResultHandling: &microflows.ResultHandlingString{VariableName: "Body"}})),
			}, nil
		},
	}
	cat, err := New()
	if err != nil {
		t.Fatalf("catalog.New: %v", err)
	}
	t.Cleanup(func() { cat.Close() })
	b := NewBuilder(cat, be)
	b.SetFullMode(true)
	if err := b.Build(nil); err != nil {
		t.Fatalf("catalog build: %v", err)
	}
	return cat
}

func TestMappingRefs_Issue1861(t *testing.T) {
	cat := mappingRefsCatalog(t)
	var got []string
	for _, row := range queryRows(t, cat, `SELECT SourceType, SourceName, RefKind, TargetType, TargetName FROM refs
		WHERE TargetType IN ('IMPORT_MAPPING', 'EXPORT_MAPPING', 'JSON_STRUCTURE')`) {
		parts := make([]string, len(row))
		for i, v := range row {
			parts[i], _ = v.(string)
		}
		got = append(got, strings.Join(parts, " "))
	}
	sort.Strings(got)
	want := []string{
		"EXPORT_MAPPING T.EMM_Order structure JSON_STRUCTURE T.JSON_Order",
		"IMPORT_MAPPING T.IMM_Order structure JSON_STRUCTURE T.JSON_Order",
		"MICROFLOW T.Export mapping_call EXPORT_MAPPING T.EMM_Order",
		"MICROFLOW T.Import mapping_call IMPORT_MAPPING T.IMM_Order",
		"MICROFLOW T.Rest mapping_call EXPORT_MAPPING T.EMM_Order",
		"MICROFLOW T.Rest mapping_call IMPORT_MAPPING T.IMM_Order",
		"MICROFLOW T.Soap mapping_call EXPORT_MAPPING T.EMM_Order",
		"MICROFLOW T.Soap mapping_call IMPORT_MAPPING T.IMM_Order",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("refs rows:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// ActionRef names the mapping an import or export action applies. A REST call
// that names two shows the import (result) mapping, the one that decides what
// the call produces; both are in refs.
func TestMappingActionRef_Issue1861(t *testing.T) {
	cat := mappingRefsCatalog(t)
	got := map[string]string{}
	for _, row := range queryRows(t, cat, `SELECT MicroflowQualifiedName, COALESCE(ActionRef, '') FROM activities
		WHERE MicroflowQualifiedName IN ('T.Import', 'T.Export', 'T.Rest', 'T.RestString')`) {
		got[row[0].(string)], _ = row[1].(string)
	}
	for flow, want := range map[string]string{
		"T.Import":     "T.IMM_Order",
		"T.Export":     "T.EMM_Order",
		"T.Rest":       "T.IMM_Order",
		"T.RestString": "", // control
	} {
		if got[flow] != want {
			t.Errorf("%s ActionRef = %q, want %q (all: %v)", flow, got[flow], want, got)
		}
	}
}
