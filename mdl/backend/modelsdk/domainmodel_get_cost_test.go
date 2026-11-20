// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/internal/testutil"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// mendixlabs/mxcli#1616: GetDomainModel(moduleID) listed and decoded the domain
// model of every module and then kept the one whose container matched. The
// microflow builder calls it once per member assignment of a `create` action,
// so 200 creates of 8 members went from 14 s to 294 s when 30 modules were
// added. It now reads and decodes only the module's own domain model.

func TestGetDomainModel_ReadsOnlyItsModulesDomainModel(t *testing.T) {
	b := New()
	if err := b.Connect(copyPedApp(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	all, err := b.ListDomainModels()
	if err != nil {
		t.Fatalf("list domain models: %v", err)
	}
	stored := 0
	for _, dm := range all {
		if dm.ContainerID != buildSystemDomainModel(nil).ContainerID {
			stored++
		}
	}
	if stored < 4 {
		t.Fatalf("control: the fixture has %d stored domain models; the bound below would prove nothing", stored)
	}

	mod, err := b.GetModuleByName("Administration")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName(Administration) = %v, %v", mod, err)
	}
	b.GetDomainModel(mod.ID) //nolint:errcheck // warm the unit-type index
	before := b.UnitFileReads()
	if _, err := b.GetDomainModel(mod.ID); err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	if reads := b.UnitFileReads() - before; reads != 1 {
		t.Errorf("GetDomainModel of one module read %d unit files; the project has %d domain models, and it needs one", reads, stored)
	}
}

// The narrowing must not change the answer: every module's domain model is
// what the full listing — which still decodes them all — says it is,
// including the persistability a specialization inherits across modules
// (mendixlabs/mxcli#1600) and view entities' OQL.
func TestGetDomainModel_AgreesWithTheFullListing(t *testing.T) {
	for _, fixture := range []struct {
		name string
		path func(*testing.T) string
	}{
		{"pedapp", copyPedApp},
		{"expr-checker", copyFixture},
		{"v1", copyV1Project}, // the container filter on MPR v1's listing
	} {
		t.Run(fixture.name, func(t *testing.T) {
			b := New()
			if err := b.Connect(fixture.path(t)); err != nil {
				t.Fatalf("connect: %v", err)
			}
			t.Cleanup(func() { _ = b.Disconnect() })
			all, err := b.ListDomainModels()
			if err != nil {
				t.Fatalf("list domain models: %v", err)
			}
			if len(all) < 2 {
				t.Fatalf("control: the fixture has no stored domain model, only System's")
			}
			specializations := 0
			for _, want := range all {
				got, err := b.GetDomainModel(want.ContainerID)
				if err != nil {
					t.Fatalf("GetDomainModel(%s): %v", want.ContainerID, err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("GetDomainModel(%s) differs from the listing's domain model for that module", want.ContainerID)
				}
				for _, e := range want.Entities {
					if e.GeneralizationRef != "" {
						specializations++
					}
				}
			}
			if fixture.name == "pedapp" && specializations == 0 {
				t.Errorf("control: no specializations in the fixture; inherited persistability is not exercised")
			}
		})
	}
}

func copyV1Project(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	if err := testutil.CopyFixture(dst, "../../../modelsdk/mpr/testdata/v1-project"); err != nil {
		t.Fatalf("copy v1 fixture: %v", err)
	}
	return filepath.Join(dst, "App.mpr")
}

// A generalization chain into another stored module is the one case where the
// narrowed lookup must still read a second domain model: the root's flag is
// stored there.
func TestGetDomainModel_InheritsPersistabilityAcrossModules(t *testing.T) {
	b := New()
	if err := b.Connect(copyPedApp(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	dmOf := func(module string) (*domainmodel.DomainModel, model.ID) {
		t.Helper()
		mod, err := b.GetModuleByName(module)
		if err != nil || mod == nil {
			t.Fatalf("GetModuleByName(%s) = %v, %v", module, mod, err)
		}
		dm, err := b.GetDomainModel(mod.ID)
		if err != nil {
			t.Fatalf("GetDomainModel(%s): %v", module, err)
		}
		return dm, mod.ID
	}
	base, _ := dmOf("Administration")
	if err := b.CreateEntity(base.ID, &domainmodel.Entity{Name: "ZzNpBase", Persistable: false}); err != nil {
		t.Fatalf("create base: %v", err)
	}
	child, childModule := dmOf("MyFirstModule")
	// Declared persistent: a reader that does not follow the chain reads that.
	if err := b.CreateEntity(child.ID, &domainmodel.Entity{Name: "ZzNpChild", Persistable: true, GeneralizationRef: "Administration.ZzNpBase"}); err != nil {
		t.Fatalf("create child: %v", err)
	}

	got, err := b.GetDomainModel(childModule)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	e := got.FindEntityByName("ZzNpChild")
	if e == nil {
		t.Fatal("ZzNpChild not found")
	}
	if e.Persistable {
		t.Errorf("MyFirstModule.ZzNpChild reads persistent; its root Administration.ZzNpBase is not (mendixlabs/mxcli#1600)")
	}
}
