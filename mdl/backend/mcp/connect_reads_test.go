// SPDX-License-Identifier: Apache-2.0

package mcp

// Connect had no test at all — 190 tests in this package and not one called it —
// so the reader swap underneath it (a concrete sdk/mpr reader to a read-only
// codec backend, Phase 3 of docs/plans/2026-09-14-retire-legacy-engine.md) would
// have landed unverified with the suite green. That is the same shape as the
// api/ integration suite that had only ever skipped.
//
// These exercise the half of the backend that reads the local .mpr. The write
// half goes over MCP to Studio Pro and is covered by the fakePED tests around it.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// localProject copies the committed fixture the codec backend's own tests use,
// so these run everywhere those do.
func localProject(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	if err := os.CopyFS(dst, os.DirFS("../../../testdata/expr-checker")); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	return filepath.Join(dst, "minimal.mpr")
}

// backendFor returns an unconnected Backend pointed at the fake Studio Pro.
//
// The dial address is passed explicitly, as connectClient does. An empty one
// lets defaultDial rewrite 127.0.0.1 to host.docker.internal wherever that name
// resolves — correct for a real Studio Pro on the host, but inside a devcontainer
// it sends the request to the Docker host instead of the httptest listener in
// this container, and Connect fails with "connection refused".
func backendFor(ped *fakePED) *Backend {
	return New(ped.srv.URL+"/mcp", strings.TrimPrefix(ped.srv.URL, "http://"))
}

// connected returns a Backend wired to a fake Studio Pro and the local fixture.
func connected(t *testing.T) *Backend {
	t.Helper()
	ped := newFakePED(t, func(string, map[string]any) (string, bool) { return "{}", false })
	b := backendFor(ped)
	if err := b.Connect(localProject(t)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	return b
}

// The reads MCP takes from the local project, through the backend it now
// composes rather than a reader it owns.
func TestConnect_LocalReadsWork(t *testing.T) {
	b := connected(t)

	mods, err := b.ListModules()
	if err != nil {
		t.Fatalf("ListModules: %v", err)
	}
	if len(mods) == 0 {
		t.Fatal("ListModules returned nothing — the local project was not read")
	}

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	if _, err := b.GetDomainModel(mod.ID); err != nil {
		t.Errorf("GetDomainModel: %v", err)
	}
	if _, err := b.ListMicroflows(); err != nil {
		t.Errorf("ListMicroflows: %v", err)
	}
	if _, err := b.ListPages(); err != nil {
		t.Errorf("ListPages: %v", err)
	}
}

// The three methods that existed on FullBackend only because MCP called them on
// its own reader. Each is now served by the composed backend.
func TestConnect_TheThreeFormerBypassReads(t *testing.T) {
	b := connected(t)

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	dm, err := b.GetDomainModel(mod.ID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}

	// GetDomainModelByID — the same document, keyed by its own id rather than
	// by its module's.
	byID, err := b.GetDomainModelByID(dm.ID)
	if err != nil {
		t.Fatalf("GetDomainModelByID: %v", err)
	}
	if byID.ID != dm.ID {
		t.Errorf("GetDomainModelByID returned %s, want %s", byID.ID, dm.ID)
	}

	// ListNavigationDocuments — a project has one, and the list must contain it.
	navs, err := b.ListNavigationDocuments()
	if err != nil {
		t.Fatalf("ListNavigationDocuments: %v", err)
	}
	if len(navs) == 0 {
		t.Error("ListNavigationDocuments returned nothing; the fixture has a navigation document")
	}

	// GetWorkflow — see TestConnect_GetWorkflowReadsASeededWorkflow. The fixture
	// carries none, so asserting over the listing here would pass vacuously.
	if _, err := b.ListWorkflows(); err != nil {
		t.Errorf("ListWorkflows: %v", err)
	}
}

// GetWorkflow against a workflow that actually exists. The committed fixture has
// zero, so a loop over the listing would have passed without calling the method
// at all — the same vacuous-green shape as a suite that only ever skips. One is
// seeded through a read-write backend first, then read back through the
// read-only one MCP composes.
func TestConnect_GetWorkflowReadsASeededWorkflow(t *testing.T) {
	path := localProject(t)

	rw := modelsdkbackend.New()
	if err := rw.Connect(path); err != nil {
		t.Fatalf("Connect (read-write): %v", err)
	}
	mod, err := rw.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	seeded := &workflows.Workflow{ContainerID: mod.ID, Name: "ZzSeededWorkflow"}
	if err := rw.CreateWorkflow(seeded); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := rw.Disconnect(); err != nil {
		t.Fatalf("Disconnect (read-write): %v", err)
	}

	ped := newFakePED(t, func(string, map[string]any) (string, bool) { return "{}", false })
	b := backendFor(ped)
	if err := b.Connect(path); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	wfs, err := b.ListWorkflows()
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	var listed *workflows.Workflow
	for _, w := range wfs {
		if w.Name == "ZzSeededWorkflow" {
			listed = w
		}
	}
	if listed == nil {
		t.Fatalf("the seeded workflow is not in the listing (%d workflows) — the "+
			"rest of this test would be measuring nothing", len(wfs))
	}

	got, err := b.GetWorkflow(listed.ID)
	if err != nil {
		t.Fatalf("GetWorkflow(%s): %v", listed.ID, err)
	}
	if got.Name != listed.Name || got.ID != listed.ID {
		t.Errorf("GetWorkflow = %s/%q, listing says %s/%q — the two decoders disagree",
			got.ID, got.Name, listed.ID, listed.Name)
	}

	// A miss is an error, not (nil, nil): callers branch on the error, and a nil
	// document reads as "exists but is empty".
	if _, err := b.GetWorkflow(model.ID("00000000-0000-0000-0000-00000000dead")); err == nil {
		t.Error("GetWorkflow on an unknown id returned no error")
	}
}

// THE constraint the comment in Connect states: MCP must not lock the file
// Studio Pro owns. The composed backend is connected read-only, so it has no
// writer and a write through it is refused rather than reaching disk.
//
// CreateEntity is the probe because it needs nothing already in the fixture —
// an attribute-level write would skip on MyFirstModule, which has no entities,
// and a test that only ever skips proves nothing (#808).
func TestConnect_LocalBackendIsReadOnly(t *testing.T) {
	b := connected(t)

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	dm, err := b.GetDomainModel(mod.ID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}

	// Reach past the MCP write path to the local backend directly: a write there
	// must fail, because that backend was opened read-only.
	err = b.reader.CreateEntity(dm.ID, &domainmodel.Entity{
		Name: "ZzReadOnlyProbe", Persistable: true,
	})
	if err == nil {
		t.Fatal("a write through the local backend succeeded — it is not read-only, " +
			"and MCP would be locking the project Studio Pro has open")
	}
}

// The control for the test above. Without it, that test passes against a
// CreateEntity that refuses this entity for some unrelated reason — which is
// indistinguishable from the read-only guard working.
func TestConnect_TheSameWriteSucceedsReadWrite(t *testing.T) {
	rw := modelsdkbackend.New()
	if err := rw.Connect(localProject(t)); err != nil {
		t.Fatalf("Connect (read-write): %v", err)
	}
	t.Cleanup(func() { _ = rw.Disconnect() })

	mod, err := rw.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	dm, err := rw.GetDomainModel(mod.ID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	if err := rw.CreateEntity(dm.ID, &domainmodel.Entity{
		Name: "ZzReadOnlyProbe", Persistable: true,
	}); err != nil {
		t.Fatalf("the probe write failed read-write too (%v) — the read-only test "+
			"above is not measuring the read-only guard", err)
	}
}

// A second Disconnect must not panic: Connect/Disconnect are driven by the CLI's
// lifecycle and a double teardown is cheap to get wrong.
func TestConnect_DisconnectIsIdempotent(t *testing.T) {
	ped := newFakePED(t, func(string, map[string]any) (string, bool) { return "{}", false })
	b := backendFor(ped)
	if err := b.Connect(localProject(t)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("first Disconnect: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Errorf("second Disconnect: %v", err)
	}
}
