// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"os"
	"path/filepath"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/agenteditor"
)

// TestUpdatePaths_KeepStoredExportLevel guards ako/mxcli#816: every backend
// rewrite of an existing document keeps the ExportLevel stored on it, unless
// the statement authored one.
//
// The rewrite converters wrote ExportLevel as a constant — "Hidden" — or the
// semantic model's value where the executor itself filled in "Hidden", and the
// unit is replaced wholesale, so an API document came back Hidden from any
// CREATE OR MODIFY, describe -> exec, or ALTER that rebuilds it.
//
// THE SUBJECT HAS TO BE API. Every document in ako/TestApp is Hidden, the value
// the converters write, so the loss is invisible on the fixture as committed.
// Each case sets the stored level to API first — what a marketplace module's
// public documents carry. A kind TestApp has no document of is created first,
// on the same copy.
//
// Each case hands the Update call the semantic document the way the executor
// does: with ExportLevel "Hidden" where the executor hardcodes it or the kind
// has no MDL spelling, and "" — unauthored — where the statement can spell it.
// A kind with a spelling also checks the other half: an authored level is
// written as authored, over the stored one, in both directions.
func TestUpdatePaths_KeepStoredExportLevel(t *testing.T) {
	src := filepath.Join("..", "..", "..", "testdata", "testapp", "TestApp")
	if _, err := os.Stat(filepath.Join(src, "TestApp.mpr")); err != nil {
		t.Skipf("TestApp submodule not initialised (git submodule update --init testdata/testapp): %v", err)
	}

	for _, tc := range exportLevelCases {
		t.Run(tc.name, func(t *testing.T) {
			proj := copyTestAppFixture(t, src)
			b := New()
			if err := b.Connect(proj); err != nil {
				t.Fatalf("connect: %v", err)
			}
			defer b.Disconnect()

			id := tc.subject(t, b)
			setStoredExportLevel(t, b, id, "API")
			if err := tc.rewrite(t, b, id, ""); err != nil {
				t.Fatalf("rewrite: %v", err)
			}
			if got := storedExportLevel(t, b, id); got != "API" {
				t.Errorf("the rewrite turned ExportLevel API into %q", got)
			}

			if !tc.authorable {
				return
			}
			setStoredExportLevel(t, b, id, "API")
			if err := tc.rewrite(t, b, id, "Hidden"); err != nil {
				t.Fatalf("authored rewrite: %v", err)
			}
			if got := storedExportLevel(t, b, id); got != "Hidden" {
				t.Errorf("the statement authored ExportLevel Hidden, the rewrite wrote %q", got)
			}

			// The other direction: an authored API over a stored Hidden. Hidden
			// over API alone cannot tell a converter that honours the clause from
			// one that writes the "Hidden" constant — workflowToGen did the
			// latter, which made `export level API` a no-op.
			setStoredExportLevel(t, b, id, "Hidden")
			if err := tc.rewrite(t, b, id, "API"); err != nil {
				t.Fatalf("authored rewrite: %v", err)
			}
			if got := storedExportLevel(t, b, id); got != "API" {
				t.Errorf("the statement authored ExportLevel API, the rewrite wrote %q", got)
			}
		})
	}
}

type exportLevelCase struct {
	name string
	// subject returns the unit the case rewrites: the first stored document of
	// the kind, or one it creates when TestApp has none.
	subject func(t *testing.T, b *Backend) model.ID
	// rewrite reads the document back and writes it through the Update path
	// under test. authored is the level the statement spelled ("" for none); a
	// kind without a spelling ignores it and passes what the executor passes.
	rewrite    func(t *testing.T, b *Backend, id model.ID, authored string) error
	authorable bool
}

// orHidden is what an executor that fills in the constant passes.
func orHidden(authored string) string {
	if authored == "" {
		return "Hidden"
	}
	return authored
}

var exportLevelCases = []exportLevelCase{
	{
		// `create enumeration … export level api` (mendixlabs/mxcli#1498).
		name:       "Enumeration",
		authorable: true,
		subject: func(t *testing.T, b *Backend) model.ID {
			all, err := b.ListEnumerations()
			return firstID(t, err, len(all), func() model.ID { return all[0].ID })
		},
		rewrite: func(t *testing.T, b *Backend, id model.ID, authored string) error {
			e, err := b.GetEnumeration(id)
			if err != nil {
				return err
			}
			e.ExportLevel = authored
			return b.UpdateEnumeration(e)
		},
	},
	{
		name: "Rule",
		subject: func(t *testing.T, b *Backend) model.ID {
			all, err := b.ListRules()
			return firstID(t, err, len(all), func() model.ID { return all[0].ID })
		},
		rewrite: func(t *testing.T, b *Backend, id model.ID, _ string) error {
			r, err := b.GetRule(id)
			if err != nil {
				return err
			}
			return b.UpdateRule(r)
		},
	},
	{
		name: "Page",
		subject: func(t *testing.T, b *Backend) model.ID {
			all, err := b.ListPages()
			return firstID(t, err, len(all), func() model.ID { return all[0].ID })
		},
		rewrite: func(t *testing.T, b *Backend, id model.ID, _ string) error {
			p, err := b.GetPage(id)
			if err != nil {
				return err
			}
			return b.UpdatePage(p)
		},
	},
	{
		// TestApp's layouts are all in Atlas_Core, and the read side does not
		// carry a layout's widget tree, so one is created; its stored level is
		// then set to API like every other subject's.
		name: "Layout",
		subject: func(t *testing.T, b *Backend) model.ID {
			l := minimalLayout()
			l.Name = "Issue816Layout"
			l.ContainerID = firstModuleID(t, b)
			if err := b.CreateLayout(l); err != nil {
				t.Fatalf("create layout: %v", err)
			}
			return l.ID
		},
		rewrite: func(t *testing.T, b *Backend, id model.ID, _ string) error {
			l := minimalLayout()
			l.ID = id
			l.Name = "Issue816Layout"
			l.ContainerID = firstModuleID(t, b)
			return b.UpdateLayout(l)
		},
	},
	{
		name: "ViewEntitySourceDocument",
		subject: func(t *testing.T, b *Backend) model.ID {
			all, err := b.ListRawUnitsByType("DomainModels$ViewEntitySourceDocument")
			return firstID(t, err, len(all), func() model.ID { return all[0].ID })
		},
		rewrite: func(t *testing.T, b *Backend, id model.ID, _ string) error {
			raw, err := b.GetRawUnitBytes(id)
			if err != nil {
				return err
			}
			var d struct {
				Name, Oql, Documentation string
			}
			if err := bson.Unmarshal(raw, &d); err != nil {
				return err
			}
			mod := b.moduleNameFor(id)
			m, err := b.GetModuleByName(mod)
			if err != nil || m == nil {
				t.Fatalf("module %q of the source document: %v", mod, err)
			}
			_, err = b.WriteViewEntitySourceDocument(m.ID, mod, d.Name, d.Oql+" ", d.Documentation)
			return err
		},
	},
	{
		name: "ImportMapping",
		subject: func(t *testing.T, b *Backend) model.ID {
			all, err := b.ListImportMappings()
			return firstID(t, err, len(all), func() model.ID { return all[0].ID })
		},
		rewrite: func(t *testing.T, b *Backend, id model.ID, authored string) error {
			all, err := b.ListImportMappings()
			if err != nil {
				return err
			}
			for _, im := range all {
				if im.ID == id {
					im.ExportLevel = orHidden(authored) // cmd_import_mappings hardcodes it
					return b.UpdateImportMapping(im)
				}
			}
			t.Fatalf("import mapping %s not listed", id)
			return nil
		},
	},
	{
		name: "ExportMapping",
		subject: func(t *testing.T, b *Backend) model.ID {
			all, err := b.ListExportMappings()
			return firstID(t, err, len(all), func() model.ID { return all[0].ID })
		},
		rewrite: func(t *testing.T, b *Backend, id model.ID, authored string) error {
			all, err := b.ListExportMappings()
			if err != nil {
				return err
			}
			for _, em := range all {
				if em.ID == id {
					em.ExportLevel = orHidden(authored) // cmd_export_mappings hardcodes it
					return b.UpdateExportMapping(em)
				}
			}
			t.Fatalf("export mapping %s not listed", id)
			return nil
		},
	},
	{
		name: "JsonStructure",
		subject: func(t *testing.T, b *Backend) model.ID {
			all, err := b.ListJsonStructures()
			return firstID(t, err, len(all), func() model.ID { return all[0].ID })
		},
		rewrite: func(t *testing.T, b *Backend, id model.ID, authored string) error {
			all, err := b.ListJsonStructures()
			if err != nil {
				return err
			}
			for _, js := range all {
				if js.ID == id {
					js.ExportLevel = orHidden(authored)
					return b.UpdateJsonStructure(js)
				}
			}
			t.Fatalf("json structure %s not listed", id)
			return nil
		},
	},
	{
		name: "PublishedRestService",
		subject: func(t *testing.T, b *Backend) model.ID {
			all, err := b.ListPublishedRestServices()
			return firstID(t, err, len(all), func() model.ID { return all[0].ID })
		},
		rewrite: func(t *testing.T, b *Backend, id model.ID, _ string) error {
			all, err := b.ListPublishedRestServices()
			if err != nil {
				return err
			}
			for _, s := range all {
				if s.ID == id {
					return b.UpdatePublishedRestService(s)
				}
			}
			t.Fatalf("published rest service %s not listed", id)
			return nil
		},
	},
	{
		name: "ConsumedRestService",
		subject: func(t *testing.T, b *Backend) model.ID {
			all, err := b.ListConsumedRestServices()
			return firstID(t, err, len(all), func() model.ID { return all[0].ID })
		},
		rewrite: func(t *testing.T, b *Backend, id model.ID, _ string) error {
			all, err := b.ListConsumedRestServices()
			if err != nil {
				return err
			}
			for _, s := range all {
				if s.ID == id {
					return b.UpdateConsumedRestService(s)
				}
			}
			t.Fatalf("consumed rest service %s not listed", id)
			return nil
		},
	},
	{
		name:       "ScheduledEvent",
		authorable: true,
		subject: func(t *testing.T, b *Backend) model.ID {
			all, err := b.ListScheduledEvents()
			return firstID(t, err, len(all), func() model.ID { return all[0].ID })
		},
		rewrite: func(t *testing.T, b *Backend, id model.ID, authored string) error {
			ev, err := b.GetScheduledEvent(id)
			if err != nil {
				return err
			}
			ev.ExportLevel = authored
			return b.UpdateScheduledEvent(ev)
		},
	},
	{
		name:       "Workflow",
		authorable: true,
		subject: func(t *testing.T, b *Backend) model.ID {
			all, err := b.ListWorkflows()
			return firstID(t, err, len(all), func() model.ID { return all[0].ID })
		},
		rewrite: func(t *testing.T, b *Backend, id model.ID, authored string) error {
			wf, err := b.GetWorkflow(id)
			if err != nil {
				return err
			}
			wf.ExportLevel = authored
			return b.UpdateWorkflow(wf)
		},
	},
	{
		name:       "Queue",
		authorable: true,
		subject: func(t *testing.T, b *Backend) model.ID {
			q := &types.Queue{ContainerID: firstModuleID(t, b), Name: "Issue816Queue"}
			if err := b.CreateQueue(q); err != nil {
				t.Fatalf("create queue: %v", err)
			}
			return q.ID
		},
		rewrite: func(t *testing.T, b *Backend, id model.ID, authored string) error {
			return b.UpdateQueue(&types.Queue{BaseElement: model.BaseElement{ID: id}, ContainerID: firstModuleID(t, b), Name: "Issue816Queue", ExportLevel: authored})
		},
	},
	{
		name:       "RegularExpression",
		authorable: true,
		subject: func(t *testing.T, b *Backend) model.ID {
			re := &model.RegularExpression{ContainerID: firstModuleID(t, b), Name: "Issue816Regex", Expression: "[a-z]+"}
			if err := b.CreateRegularExpression(re); err != nil {
				t.Fatalf("create regular expression: %v", err)
			}
			return re.ID
		},
		rewrite: func(t *testing.T, b *Backend, id model.ID, authored string) error {
			return b.UpdateRegularExpression(&model.RegularExpression{BaseElement: model.BaseElement{ID: id}, ContainerID: firstModuleID(t, b), Name: "Issue816Regex", Expression: "[a-z]+", ExportLevel: authored})
		},
	},
	{
		name: "DatabaseConnection",
		subject: func(t *testing.T, b *Backend) model.ID {
			c := &model.DatabaseConnection{ContainerID: firstModuleID(t, b), Name: "Issue816Db", DatabaseType: "PostgreSQL", ExportLevel: "Hidden"}
			if err := b.CreateDatabaseConnection(c); err != nil {
				t.Fatalf("create database connection: %v", err)
			}
			return c.ID
		},
		rewrite: func(t *testing.T, b *Backend, id model.ID, _ string) error {
			// cmd_dbconnection hardcodes Hidden.
			return b.UpdateDatabaseConnection(&model.DatabaseConnection{BaseElement: model.BaseElement{ID: id}, ContainerID: firstModuleID(t, b), Name: "Issue816Db", DatabaseType: "PostgreSQL", ExportLevel: "Hidden"})
		},
	},
	{
		name: "BusinessEventService",
		subject: func(t *testing.T, b *Backend) model.ID {
			s := &model.BusinessEventService{ContainerID: firstModuleID(t, b), Name: "Issue816Events", ExportLevel: "Hidden"}
			if err := b.CreateBusinessEventService(s); err != nil {
				t.Fatalf("create business event service: %v", err)
			}
			return s.ID
		},
		rewrite: func(t *testing.T, b *Backend, id model.ID, _ string) error {
			// cmd_businessevents hardcodes Hidden.
			return b.UpdateBusinessEventService(&model.BusinessEventService{BaseElement: model.BaseElement{ID: id}, ContainerID: firstModuleID(t, b), Name: "Issue816Events", ExportLevel: "Hidden"})
		},
	},
	{
		name: "DataTransformer",
		subject: func(t *testing.T, b *Backend) model.ID {
			dt := &model.DataTransformer{ContainerID: firstModuleID(t, b), Name: "Issue816Transformer", SourceType: "JSON", SourceJSON: "{}"}
			if err := b.CreateDataTransformer(dt); err != nil {
				t.Fatalf("create data transformer: %v", err)
			}
			return dt.ID
		},
		rewrite: func(t *testing.T, b *Backend, id model.ID, _ string) error {
			return b.UpdateDataTransformer(&model.DataTransformer{BaseElement: model.BaseElement{ID: id}, ContainerID: firstModuleID(t, b), Name: "Issue816Transformer", SourceType: "JSON", SourceJSON: "{}"})
		},
	},
	{
		// Constants have no MDL spelling for their export level, and the
		// executor builds the model without one (ako/mxcli#1980).
		name: "Constant",
		subject: func(t *testing.T, b *Backend) model.ID {
			c := issue1980Constant(firstModuleID(t, b), "")
			if err := b.CreateConstant(c); err != nil {
				t.Fatalf("create constant: %v", err)
			}
			return c.ID
		},
		rewrite: func(t *testing.T, b *Backend, id model.ID, _ string) error {
			c := issue1980Constant(firstModuleID(t, b), id)
			return b.UpdateConstant(c)
		},
	},
	{
		name: "AgentEditorModel",
		subject: func(t *testing.T, b *Backend) model.ID {
			m := &agenteditor.Model{ContainerID: firstModuleID(t, b), Name: "Issue816Model"}
			if err := b.CreateAgentEditorModel(m); err != nil {
				t.Fatalf("create agent editor model: %v", err)
			}
			return m.ID
		},
		rewrite: func(t *testing.T, b *Backend, id model.ID, _ string) error {
			return b.UpdateAgentEditorModel(&agenteditor.Model{BaseElement: model.BaseElement{ID: id}, ContainerID: firstModuleID(t, b), Name: "Issue816Model"})
		},
	},
}

func firstID(t *testing.T, err error, n int, first func() model.ID) model.ID {
	t.Helper()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if n == 0 {
		t.Fatal("TestApp has no document of this kind — create one in the case instead")
	}
	return first()
}

// firstModuleID is the module a created subject is placed in: MyFirstModule,
// the one app module every TestApp copy has.
func firstModuleID(t *testing.T, b *Backend) model.ID {
	t.Helper()
	m, err := b.GetModuleByName("MyFirstModule")
	if err != nil || m == nil {
		t.Fatalf("module MyFirstModule: %v", err)
	}
	return m.ID
}

func storedExportLevel(t *testing.T, b *Backend, id model.ID) string {
	t.Helper()
	raw, err := b.reader.GetRawUnitBytes(string(id))
	if err != nil {
		t.Fatalf("GetRawUnitBytes: %v", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, e := range d {
		if e.Key == "ExportLevel" {
			s, _ := e.Value.(string)
			return s
		}
	}
	t.Fatalf("unit %s has no ExportLevel", id)
	return ""
}

// setStoredExportLevel rewrites the unit's ExportLevel, standing in for a
// document Studio Pro exported as part of its module's API.
func setStoredExportLevel(t *testing.T, b *Backend, id model.ID, lvl string) {
	t.Helper()
	raw, err := b.reader.GetRawUnitBytes(string(id))
	if err != nil {
		t.Fatalf("GetRawUnitBytes: %v", err)
	}
	out, err := withExportLevel(raw, lvl)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.writer.UpdateRawUnit(string(id), out); err != nil {
		t.Fatalf("UpdateRawUnit: %v", err)
	}
	if got := storedExportLevel(t, b, id); got != lvl {
		t.Fatalf("precondition: ExportLevel %q after setting it to %q — the unit stores no ExportLevel", got, lvl)
	}
}

// TestWithExportLevel_ReplacesOnlyTheValue: the carry rewrites the value of the
// top-level ExportLevel and copies every other element verbatim, in order — a
// nested ExportLevel (an attribute's, an annotation's) is not the document's —
// and leaves a document without the key, or already holding the value, as is.
func TestWithExportLevel_ReplacesOnlyTheValue(t *testing.T) {
	doc, err := bson.Marshal(bson.D{
		{Key: "$Type", Value: "Enumerations$Enumeration"},
		{Key: "CanvasWidth", Value: int64(1198)},
		{Key: "ExportLevel", Value: "Hidden"},
		{Key: "Child", Value: bson.D{{Key: "ExportLevel", Value: "Hidden"}}},
		{Key: "Name", Value: "E"},
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := withExportLevel(doc, "API")
	if err != nil {
		t.Fatal(err)
	}
	var got bson.D
	if err := bson.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	want := bson.D{
		{Key: "$Type", Value: "Enumerations$Enumeration"},
		{Key: "CanvasWidth", Value: int64(1198)},
		{Key: "ExportLevel", Value: "API"},
		{Key: "Child", Value: bson.D{{Key: "ExportLevel", Value: "Hidden"}}},
		{Key: "Name", Value: "E"},
	}
	wantBytes, _ := bson.Marshal(want)
	if string(out) != string(wantBytes) {
		t.Errorf("withExportLevel:\n got %v\nwant %v", got, want)
	}

	same, err := withExportLevel(out, "API")
	if err != nil || string(same) != string(out) {
		t.Errorf("already API: want the input unchanged, err=%v", err)
	}
	noKey, _ := bson.Marshal(bson.D{{Key: "Name", Value: "E"}})
	kept, err := withExportLevel(noKey, "API")
	if err != nil || string(kept) != string(noKey) {
		t.Errorf("no ExportLevel key: want the input unchanged (no key added), err=%v", err)
	}
}

func issue1980Constant(container, id model.ID) *model.Constant {
	return &model.Constant{
		BaseElement:  model.BaseElement{ID: id},
		ContainerID:  container,
		Name:         "Issue1980Constant",
		Type:         model.ConstantDataType{Kind: "Boolean"},
		DefaultValue: "true",
	}
}

// ako/mxcli#1980 (mendixlabs/mxcli#1749): `create constant` wrote ExportLevel ""
// where Studio Pro writes "Hidden". mx check accepts it, but mx diff crashes on
// a pair of units holding "" and "Hidden", and git merge's report goes with it.
// A rewrite also repairs a unit an earlier version left at "".
func TestConstant_ExportLevelDefaultsToHidden(t *testing.T) {
	src := filepath.Join("..", "..", "..", "testdata", "testapp", "TestApp")
	if _, err := os.Stat(filepath.Join(src, "TestApp.mpr")); err != nil {
		t.Skipf("TestApp submodule not initialised: %v", err)
	}
	proj := copyTestAppFixture(t, src)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer b.Disconnect()

	c := issue1980Constant(firstModuleID(t, b), "")
	if err := b.CreateConstant(c); err != nil {
		t.Fatal(err)
	}
	if got := storedExportLevel(t, b, c.ID); got != "Hidden" {
		t.Fatalf("create constant wrote ExportLevel %q, want Hidden", got)
	}
	setStoredExportLevel(t, b, c.ID, "")
	if err := b.UpdateConstant(issue1980Constant(firstModuleID(t, b), c.ID)); err != nil {
		t.Fatal(err)
	}
	if got := storedExportLevel(t, b, c.ID); got != "Hidden" {
		t.Errorf("rewriting a constant stored with ExportLevel \"\" wrote %q, want Hidden", got)
	}
}
