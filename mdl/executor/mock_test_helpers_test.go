// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/domainmodel"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
	"github.com/JordtenBulte-OLC/mxcli/sdk/workflows"
)

// --- Context construction ---

type mockCtxOption func(*ExecContext)

// newMockCtx creates an ExecContext backed by a MockBackend with a bytes.Buffer
// as output. Returns the context and the buffer for output assertions.
// The default format is FormatTable. Pass options to override.
func newMockCtx(t *testing.T, opts ...mockCtxOption) (*ExecContext, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	ctx := &ExecContext{
		Context: context.Background(),
		Backend: &mock.MockBackend{
			IsConnectedFunc: func() bool { return true },
		},
		Output: &buf,
		Format: FormatTable,
	}
	for _, opt := range opts {
		opt(ctx)
	}
	return ctx, &buf
}

func withBackend(b *mock.MockBackend) mockCtxOption {
	return func(ctx *ExecContext) { ctx.Backend = b }
}

func withFormat(f OutputFormat) mockCtxOption {
	return func(ctx *ExecContext) { ctx.Format = f }
}

func withQuiet() mockCtxOption {
	return func(ctx *ExecContext) { ctx.Quiet = true }
}

func withCache(c *executorCache) mockCtxOption {
	return func(ctx *ExecContext) { ctx.Cache = c }
}

func withHierarchy(h *ContainerHierarchy) mockCtxOption {
	return func(ctx *ExecContext) {
		if ctx.Cache == nil {
			ctx.Cache = &executorCache{}
		}
		ctx.Cache.hierarchy = h
	}
}

func withMprPath(p string) mockCtxOption {
	return func(ctx *ExecContext) { ctx.MprPath = p }
}

func withSettings(s map[string]any) mockCtxOption {
	return func(ctx *ExecContext) { ctx.Settings = s }
}

// --- Hierarchy construction ---

// mkHierarchy builds a ContainerHierarchy from modules. After creation, use
// withContainer to register container-parent links for documents (entities,
// enumerations, etc.) so that FindModuleID can walk up to the owning module.
func mkHierarchy(modules ...*model.Module) *ContainerHierarchy {
	h := &ContainerHierarchy{
		moduleIDs:       make(map[model.ID]bool),
		moduleNames:     make(map[model.ID]string),
		containerParent: make(map[model.ID]model.ID),
		folderNames:     make(map[model.ID]string),
	}
	for _, m := range modules {
		h.moduleIDs[m.ID] = true
		h.moduleNames[m.ID] = m.Name
	}
	return h
}

// withContainer registers a container-parent link in the hierarchy so that
// ContainerHierarchy.FindModuleID can walk from containerID up to a module.
// In production, FindModuleID is always called with an element's ContainerID
// field (not the element's own ID). For elements whose ContainerID is already
// a module ID, this call is technically redundant (the module is found directly
// in moduleIDs), but it keeps test setup explicit about parentage. For
// intermediate containers (folders, units) this call is required.
func withContainer(h *ContainerHierarchy, containerID, parentContainerID model.ID) {
	h.containerParent[containerID] = parentContainerID
}

// --- Model factories ---

// idCounter generates unique IDs across all tests in the package. IDs are
// non-deterministic across runs (depend on test execution order), which is
// fine for string-contains assertions but would break exact-value assertions.
var idCounter atomic.Int64

func nextID(prefix string) model.ID {
	n := idCounter.Add(1)
	return model.ID(prefix + "-" + strconv.FormatInt(n, 10))
}

func mkModule(name string) *model.Module {
	return &model.Module{
		BaseElement: model.BaseElement{ID: nextID("mod")},
		Name:        name,
	}
}

func mkEnumeration(containerID model.ID, name string, values ...string) *model.Enumeration {
	e := &model.Enumeration{
		BaseElement: model.BaseElement{ID: nextID("enum")},
		ContainerID: containerID,
		Name:        name,
	}
	for _, v := range values {
		e.Values = append(e.Values, model.EnumerationValue{
			BaseElement: model.BaseElement{ID: nextID("ev")},
			Name:        v,
		})
	}
	return e
}

func mkConstant(containerID model.ID, name string, typ string, defaultVal string) *model.Constant {
	return &model.Constant{
		BaseElement:  model.BaseElement{ID: nextID("const")},
		ContainerID:  containerID,
		Name:         name,
		Type:         model.ConstantDataType{Kind: typ},
		DefaultValue: defaultVal,
	}
}

func mkEntity(containerID model.ID, name string) *domainmodel.Entity {
	return &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: nextID("ent")},
		ContainerID: containerID,
		Name:        name,
		Persistable: true,
	}
}

func mkDomainModel(containerID model.ID, entities ...*domainmodel.Entity) *domainmodel.DomainModel {
	return &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm")},
		ContainerID: containerID,
		Entities:    entities,
	}
}

func mkAssociation(containerID model.ID, name string, parentID, childID model.ID) *domainmodel.Association {
	return &domainmodel.Association{
		BaseElement: model.BaseElement{ID: nextID("assoc")},
		ContainerID: containerID,
		Name:        name,
		ParentID:    parentID,
		ChildID:     childID,
		Type:        "Reference",
		Owner:       "Default",
	}
}

func mkMicroflow(containerID model.ID, name string) *microflows.Microflow {
	return &microflows.Microflow{
		BaseElement: model.BaseElement{ID: nextID("mf")},
		ContainerID: containerID,
		Name:        name,
	}
}

func mkNanoflow(containerID model.ID, name string) *microflows.Nanoflow {
	return &microflows.Nanoflow{
		BaseElement: model.BaseElement{ID: nextID("nf")},
		ContainerID: containerID,
		Name:        name,
	}
}

func mkPage(containerID model.ID, name string) *pages.Page {
	return &pages.Page{
		BaseElement: model.BaseElement{ID: nextID("pg")},
		ContainerID: containerID,
		Name:        name,
	}
}

func mkSnippet(containerID model.ID, name string) *pages.Snippet {
	return &pages.Snippet{
		BaseElement: model.BaseElement{ID: nextID("snp")},
		ContainerID: containerID,
		Name:        name,
	}
}

func mkLayout(containerID model.ID, name string) *pages.Layout {
	return &pages.Layout{
		BaseElement: model.BaseElement{ID: nextID("lay")},
		ContainerID: containerID,
		Name:        name,
	}
}

func mkWorkflow(containerID model.ID, name string) *workflows.Workflow {
	return &workflows.Workflow{
		BaseElement: model.BaseElement{ID: nextID("wf")},
		ContainerID: containerID,
		Name:        name,
	}
}

// --- Assertion helpers ---

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func assertContainsStr(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("output should contain %q, got:\n%s", want, got)
	}
}

func assertNotContainsStr(t *testing.T, got, unwanted string) {
	t.Helper()
	if strings.Contains(got, unwanted) {
		t.Errorf("output should not contain %q, got:\n%s", unwanted, got)
	}
}

// assertValidJSON checks that s is valid JSON starting with '{' or '['.
// Unlike json.Valid alone, this rejects scalar JSON values (true, 123, null)
// which would not be valid handler output.
func assertValidJSON(t *testing.T, s string) {
	t.Helper()
	trimmed := strings.TrimSpace(s)
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
		t.Errorf("expected json array or object, got:\n%s", s)
		return
	}
	if !json.Valid([]byte(trimmed)) {
		t.Errorf("expected valid json, got:\n%s", s)
	}
}
