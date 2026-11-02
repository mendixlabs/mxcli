// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"
	"os"
	"time"

	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// Backend executes domain-model writes against a live Studio Pro via its MCP
// ("PED") server, while serving reads from the local mounted .mpr file.
//
// It is a hybrid: PED has no way to enumerate modules or run the catalog, so
// reads (ListModules, GetDomainModel, SHOW/DESCRIBE) come from a read-only
// local reader, and writes (CreateEntity, …) go over MCP so Studio Pro stays
// the authoritative serializer and the project can remain open. See
// docs/11-proposals/PROPOSAL_mcp_backend.md.
//
// The embedded unsupportedBackend makes every FullBackend method error by
// default; Backend overrides only the operations in the entity slice. This is
// deliberate: an un-overridden write must NOT fall through to the local .mpr
// (that would edit the file Studio Pro holds in memory) — it errors instead.
type Backend struct {
	unsupportedBackend

	mcpURL string
	dial   string

	// tracer, when set, reports PED tool calls for --mcp-verbose / --mcp-trace.
	// Wired onto the clients in Connect. nil is a no-op.
	tracer *backend.Tracer

	client *Client
	reader backend.FullBackend
	path   string
	server ServerInfo

	// On-Disconnect actions. checkOnExit runs a project-wide ped_check_errors
	// (--mcp-check); runOnExit starts the app through the mendix_app handler and
	// prints its URL (--mcp-run).
	checkOnExit bool
	runOnExit   bool
	// handlers caches query_handlers getDetails answers by handler name.
	handlers map[string]bool

	// schemaFetched records element types already fetched via ped_get_schema
	// this session (the contract asks for a schema fetch before create/add).
	schemaFetched map[string]bool

	// workflowCtorContext caches whether the server's Workflows$Workflow
	// constructor takes the context entity as `context` (Studio Pro 11.14) rather
	// than a `parameter` element; nil until probed. See
	// workflowConstructorTakesContext.
	workflowCtorContext *bool
	// workflowCtorTitle records, from the same probe, that the constructor names
	// the workflow title `title` (11.15+) rather than `caption`.
	workflowCtorTitle bool

	// microflowCtorSkeleton caches whether the server's Microflows$Microflow
	// constructor takes only the canvas (Studio Pro 11.14), with behaviour set by a
	// follow-up update; nil until probed. See microflowConstructorTakesSkeleton.
	microflowCtorSkeleton *bool

	// multiTaskPageCtor caches whether the server's MultiUserTaskActivity
	// constructor takes the page as a `taskPage` element (Studio Pro 11.14)
	// rather than a bare `pageReference`; nil until probed.
	multiTaskPageCtor *bool

	// capsCache memoizes the session's resolved capability set. Every authoring
	// gate consults it and resolution costs a tools/list round-trip, so it is
	// computed once per connection.
	capsCache *Capabilities

	// settleDelay / settleWindow pace every ped_check_errors call against Studio
	// Pro's lagging error list; see pedCheckDocument. Both zero means "ask once,
	// now", which is what the unit tests want. New sets them.
	settleDelay  time.Duration
	settleWindow time.Duration

	// dirty holds module names whose live (in-memory) domain model has diverged
	// from the on-disk .mpr because of writes this session. Reads of a dirty
	// module are reconstructed from MCP instead of the stale local reader —
	// this is the dirty-set read router that closes the consistency hole.
	dirty map[string]bool

	// synthetic maps the synthetic IDs handed out by reconstructed reads back to
	// the PED-addressable element name (entity or association name). PED never
	// exposes real $IDs, so reconstructed elements get synthetic IDs; the write
	// helpers resolve those back to names through this map before falling back
	// to the local reader.
	synthetic map[model.ID]string

	// sessionEnums holds enumerations created over MCP this session. They are
	// not on disk yet, so ListEnumerations/GetEnumeration merge them in — this
	// is what lets "create enumeration X; create entity (a: X)" work in one run.
	// Enumerations are create-only via PED (no delete tool), so a registry is
	// enough; no live reconstruction is needed.
	sessionEnums []*model.Enumeration

	// sessionConstants holds constants created over MCP this session, merged into
	// ListConstants/GetConstant (same rationale as sessionEnums: duplicate
	// detection for CREATE OR MODIFY, and resolving a just-created constant for DROP).
	sessionConstants []*model.Constant

	// sessionMicroflows holds microflows created over MCP this session, merged
	// into ListMicroflows/GetMicroflow for the same reason as sessionEnums
	// (duplicate detection and create-then-reference within one run).
	sessionMicroflows []*microflows.Microflow

	// sessionFolders holds folders the executor asked to create this session. PED
	// can't create an empty folder, so these are pending until a document is created
	// into one (which auto-creates the path); they are merged into ListFolders and
	// drive folder-path resolution for foldered document creates.
	sessionFolders []*types.FolderInfo

	// sessionPages holds pages created over MCP this session, merged into
	// ListPages (the executor's duplicate/role checks read it).
	sessionPages []*pages.Page

	// customWidgets holds the high-level pg `object` recorded for each pluggable
	// widget built this session, keyed by the CustomWidget's ID. The pluggable
	// widget engine builds widgets via LoadWidgetTemplate → an mcpWidgetBuilder
	// that records semantic property ops here instead of mutating BSON;
	// mapPageWidget then looks the object up to emit CustomWidgets$CustomWidget.
	customWidgets map[model.ID]*mcpCustomWidget

	// sessionModules holds modules created over MCP this session, merged into
	// ListModules/GetModule(ByName) so that a module-dependent op later in the
	// same run (e.g. "create module X; create enumeration X.Y") can resolve the
	// freshly created module, which the local reader does not yet know about.
	sessionModules []*model.Module

	// sessionWorkflows holds workflows created over MCP this session, merged into
	// ListWorkflows/GetWorkflow (duplicate detection + create-then-reference).
	sessionWorkflows []*workflows.Workflow
}

// compile-time guarantee that Backend (and its embedded base) satisfies the
// full backend contract.
var _ backend.FullBackend = (*Backend)(nil)

// New returns an unconnected MCP backend that will issue model writes to the
// MCP server at mcpURL. dial optionally overrides the TCP address actually
// connected to (empty = derive from the URL; localhost maps to
// host.docker.internal from a devcontainer). Call Connect with the local .mpr
// path to open the read side.
func New(mcpURL, dial string) *Backend {
	return &Backend{
		mcpURL:        mcpURL,
		dial:          dial,
		settleDelay:   settleDelay,
		settleWindow:  settleWindow,
		schemaFetched: map[string]bool{},
		dirty:         map[string]bool{},
		synthetic:     map[model.ID]string{},
	}
}

// ExitActions selects what Disconnect runs after the command's writes.
type ExitActions struct {
	Check bool // project-wide ped_check_errors (--mcp-check)
	Run   bool // execute_handler mendix_app run (--mcp-run)
}

// WithTracer attaches a tool-call tracer (--mcp-verbose / --mcp-trace). The same
// tracer should also be given to the executor (SetTracer) so MDL-command headers
// and PED-call lines interleave on one writer. Returns the receiver for chaining.
func (b *Backend) WithTracer(t *backend.Tracer) *Backend {
	b.tracer = t
	return b
}

// WithExitActions sets what Disconnect runs. Returns the receiver for chaining.
func (b *Backend) WithExitActions(a ExitActions) *Backend {
	b.checkOnExit = a.Check
	b.runOnExit = a.Run
	return b
}

// Connect opens the local .mpr read-only (for reads/enumeration) and completes
// the MCP handshake with Studio Pro (for writes).
func (b *Backend) Connect(path string) error {
	// Read-only: never lock the file Studio Pro owns. This is the codec engine
	// rather than a concrete sdk/mpr reader — MCP holding one of those was the
	// reason GetDomainModelByID, GetWorkflow and ListNavigationDocuments sat on
	// FullBackend with no caller through a backend value. See Phase 3 of
	// docs/plans/2026-09-14-retire-legacy-engine.md.
	r := modelsdkbackend.New()
	if err := r.ConnectReadOnly(path); err != nil {
		return fmt.Errorf("open local project %q: %w", path, err)
	}
	c, err := NewClient(ClientOptions{URL: b.mcpURL, Dial: b.dial})
	if err != nil {
		_ = r.Disconnect()
		return err
	}
	c.trace = b.tracer
	si, err := c.Initialize()
	if err != nil {
		_ = r.Disconnect()
		return fmt.Errorf("connect to MCP server %q: %w", b.mcpURL, err)
	}
	b.reader = r
	b.client = c
	b.server = si
	b.path = path

	return nil
}

// ServerInfo returns the connected MCP server identity (after Connect).
func (b *Backend) ServerInfo() ServerInfo { return b.server }

// ---------------------------------------------------------------------------
// ConnectionBackend
// ---------------------------------------------------------------------------

func (b *Backend) Disconnect() error {
	// PED persists every write to disk as it lands (measured on 11.12.0 and
	// 11.16.0), so there is nothing to save here.
	//
	// Validate the model after writes if requested. Diagnostic, so it prints to
	// stderr and never blocks teardown.
	if b.checkOnExit && b.client != nil {
		if problems, err := b.CheckProject(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: --mcp-check failed: %v\n", err)
		} else {
			writeCheckReport(os.Stderr, problems)
		}
	}
	// Start the app (build + deploy current model) and report its URL, for a
	// change-then-run loop.
	if b.runOnExit && b.client != nil {
		if s, err := b.RunApp(0); err != nil {
			fmt.Fprintf(os.Stderr, "warning: --mcp-run failed: %v\n", err)
		} else {
			writeAppStatus(os.Stderr, s)
		}
	}
	if b.reader != nil {
		err := b.reader.Disconnect()
		b.reader = nil
		b.client = nil
		b.path = ""
		return err
	}
	return nil
}

// Commit is a no-op: PED applies each operation to Studio Pro's in-memory model
// immediately. There is no MCP flush-to-disk tool — the user saves in Studio
// Pro. See the "consistency hole" section of the proposal.
func (b *Backend) Commit() error { return nil }

func (b *Backend) IsConnected() bool { return b.client != nil && b.reader != nil }
func (b *Backend) Path() string      { return b.path }

func (b *Backend) Version() types.MPRVersion             { return types.MPRVersion(b.reader.Version()) }
func (b *Backend) ProjectVersion() *types.ProjectVersion { return b.reader.ProjectVersion() }
func (b *Backend) GetMendixVersion() (string, error)     { return b.reader.GetMendixVersion() }

// ---------------------------------------------------------------------------
// Reads — delegated to the local read-only reader (hybrid model).
//
// Caveat: these reflect the last-saved on-disk state. Edits applied via MCP
// this session that Studio Pro keeps in memory are not visible here until the
// user saves (the "consistency hole"). For the entity slice this is acceptable;
// a dirty-set read router is future work.
// ---------------------------------------------------------------------------

func (b *Backend) ListModules() ([]*model.Module, error) {
	local, err := b.reader.ListModules()
	if err != nil {
		return nil, err
	}
	if len(b.sessionModules) == 0 {
		return local, nil
	}
	seen := make(map[string]bool, len(b.sessionModules))
	out := make([]*model.Module, 0, len(local)+len(b.sessionModules))
	for _, m := range b.sessionModules {
		seen[m.Name] = true
		out = append(out, m)
	}
	for _, m := range local {
		if !seen[m.Name] {
			out = append(out, m)
		}
	}
	return out, nil
}

func (b *Backend) GetModule(id model.ID) (*model.Module, error) {
	for _, m := range b.sessionModules {
		if m.ID == id {
			return m, nil
		}
	}
	if b.reader == nil {
		return nil, fmt.Errorf("module %s not found", id)
	}
	return b.reader.GetModule(id)
}

func (b *Backend) GetModuleByName(name string) (*model.Module, error) {
	for _, m := range b.sessionModules {
		if m.Name == name {
			return m, nil
		}
	}
	return b.reader.GetModuleByName(name)
}

// sessionDMPrefix prefixes the synthetic domain-model ID handed out for a
// session-created module; the suffix is the module name.
const sessionDMPrefix = "mcp~dm~"

// effectiveDomainModel is the SINGLE seam for reading a module's domain model:
// the live view that accounts for writes made this session. Every domain-model
// read funnels through it, so none can forget the dirty routing — the omission
// that made ListDomainModels return stale entities and broke CREATE ASSOCIATION
// for an existing module.
//
// Routing, in order:
//   - session-created module (no on-disk domain model): reconstruct from Studio
//     Pro with a synthetic DM ID encoding the module name (so
//     moduleNameForDomainModel can resolve it back);
//   - dirty existing module (written this session): reconstruct from Studio Pro;
//   - otherwise: the on-disk reader copy (last-saved state).
func (b *Backend) effectiveDomainModel(moduleID model.ID) (*domainmodel.DomainModel, error) {
	for _, m := range b.sessionModules {
		if m.ID == moduleID {
			return b.reconstructDomainModelFromPED(m.Name, model.ID(sessionDMPrefix+m.Name), moduleID)
		}
	}
	localDM, err := b.reader.GetDomainModel(moduleID)
	if err != nil {
		return nil, err
	}
	if mod, merr := b.reader.GetModule(moduleID); merr == nil && b.dirty[mod.Name] {
		if recon, rerr := b.reconstructDomainModelFromPED(mod.Name, localDM.ID, localDM.ContainerID); rerr == nil {
			return recon, nil // best-effort: fall back to the local copy on failure
		}
	}
	return localDM, nil
}

// ListDomainModels returns every module's live domain model (dirty routing via
// effectiveDomainModel), plus session-created modules that have no on-disk copy.
func (b *Backend) ListDomainModels() ([]*domainmodel.DomainModel, error) {
	local, err := b.reader.ListDomainModels()
	if err != nil {
		return nil, err
	}
	out := make([]*domainmodel.DomainModel, 0, len(local)+len(b.sessionModules))
	for _, dm := range local {
		eff, eerr := b.effectiveDomainModel(dm.ContainerID)
		if eerr != nil {
			eff = dm // best-effort: keep the on-disk copy if the live view fails
		}
		out = append(out, eff)
	}
	for _, m := range b.sessionModules {
		if dm, derr := b.effectiveDomainModel(m.ID); derr == nil {
			out = append(out, dm)
		}
	}
	return out, nil
}

// GetDomainModel returns a module's live domain model (see effectiveDomainModel).
func (b *Backend) GetDomainModel(moduleID model.ID) (*domainmodel.DomainModel, error) {
	return b.effectiveDomainModel(moduleID)
}

// GetDomainModelByID mirrors GetDomainModel but is keyed by the domain model's
// own ID; it resolves the owning module and applies the same dirty routing.
func (b *Backend) GetDomainModelByID(id model.ID) (*domainmodel.DomainModel, error) {
	localDM, err := b.reader.GetDomainModelByID(id)
	if err != nil {
		return nil, err
	}
	return b.effectiveDomainModel(localDM.ContainerID)
}

// ReconcileMemberAccesses is a no-op for the MCP backend. It is the executor's
// finalize-time sync that keeps entity/association member-access rules
// consistent on disk. Studio Pro maintains that consistency itself for PED
// edits, and this slice never pushes access rules over PED, so there is
// nothing to reconcile.
func (b *Backend) ReconcileMemberAccesses(_ model.ID, _ string) (int, error) {
	return 0, nil
}
