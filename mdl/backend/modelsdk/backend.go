// SPDX-License-Identifier: Apache-2.0

// Package modelsdkbackend is the codec-engine implementation of
// backend.FullBackend, and the only local engine there is: the legacy
// mdl/backend/mpr package it was written to replace has been deleted
// (docs/plans/2026-09-14-retire-legacy-engine.md). Reads and writes are both
// complete; --engine and MXCLI_ENGINE survive as a warning-only no-op so that
// scripts pinning the old engine keep working.
//
// It embeds the generated `unimplemented` (gen_unimplemented.go) so the whole
// FullBackend surface is satisfied, and every method it has not ported fails
// loudly with errUnimplemented rather than silently returning a zero value. The
// set that still falls through is pinned by
// unimplemented_reachability_test.go, which is worth reading as a map: each
// entry is there because some caller reaches that method while holding a
// concrete sdk/mpr reader instead of a backend value, so the list shrinks by
// closing a bypass rather than by deleting interface surface.
package modelsdkbackend

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	genPr "github.com/mendixlabs/mxcli/modelsdk/gen/projects"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
)

// Compile-time guarantee that the backend satisfies the whole interface (via the
// embedded `unimplemented` for every method it doesn't override).
var _ backend.FullBackend = (*Backend)(nil)
var _ backend.WriteStatsReporter = (*Backend)(nil)

// Backend reads and writes a Mendix project through the modelsdk codec engine.
// It embeds `unimplemented` (generated, see gen_unimplemented.go) so any
// FullBackend method it has not yet ported fails loudly with errUnimplemented
// rather than silently no-op'ing — ADR-0005 "guard, don't silently drop". As
// real methods are added on *Backend they shadow the embedded stubs.
type Backend struct {
	unimplemented
	reader *mmpr.Reader
	writer *mmpr.Writer
	path   string

	// fileWrites counts writes that do not go through unit storage — the
	// generated .java/.js source of a code action, whose body lives in
	// javasource/ or javascriptsource/ rather than in its unit. They are folded
	// into WriteStats so a body-only edit is not reported as "Unchanged".
	fileWrites backend.WriteStats
}

// New constructs a modelsdk backend.
func New() *Backend {
	return &Backend{}
}

// errUnimplemented is the error every not-yet-ported FullBackend method returns
// (via the generated unimplemented embed). Loud failure beats the silent no-op
// the embedded mock used to give — see ADR-0005 "guard, don't silently drop".
//
// It used to end "rerun with MXCLI_ENGINE=legacy". That engine is gone, so the
// message now asks for a report instead of naming a fallback that does not
// exist: a user told to rerun on a deleted engine learns nothing and gets a
// second failure. Reaching this at all is a bug rather than a known gap — the
// set of methods that can is pinned by unimplemented_reachability_test.go and
// measured to have no caller through a backend value.
func errUnimplemented(method string) error {
	return fmt.Errorf("mxcli: %s is not implemented on the model engine. "+
		"This should be unreachable — please report it at "+
		"https://github.com/mendixlabs/mxcli/issues with the command you ran", method)
}

// WriteStats reports how many unit writes reached storage versus how many were
// elided as no-ops (ADR-0008). Zero before Connect, and after Disconnect the
// writer is gone with its counters — a caller sampling across a statement holds
// the connection open for both reads.
func (b *Backend) WriteStats() backend.WriteStats {
	if b.writer == nil {
		return b.fileWrites
	}
	offered, written := b.writer.WriteStats()
	return backend.WriteStats{
		Offered: offered + b.fileWrites.Offered,
		Written: written + b.fileWrites.Written,
	}
}

// noteFileWrite records a non-unit write and whether it changed anything.
func (b *Backend) noteFileWrite(changed bool) {
	b.fileWrites.Offered++
	if changed {
		b.fileWrites.Written++
	}
}

// --- ConnectionBackend ---

// Connect opens the project read-write through the modelsdk reader/writer
// (matching legacy mprbackend, which also opens read-write for all operations).
// The writer shares the reader so cache invalidation after a write is seen by
// subsequent reads on the same connection.
func (b *Backend) Connect(path string) error {
	r, err := mmpr.OpenWithOptions(path, mmpr.OpenOptions{ReadOnly: false})
	if err != nil {
		return err
	}
	b.reader = r
	b.writer = mmpr.NewWriterWithReader(r)
	b.path = path
	return nil
}

// ConnectReadOnly opens a project for reading only, leaving the writer nil.
//
// For a caller that must not take a lock on a file something else owns — the MCP
// backend reads the local .mpr while Studio Pro has it open, and sends its writes
// to Studio Pro rather than to disk. Every write method here already guards on a
// nil writer, so a write attempted through a read-only backend is refused with
// "not connected for writing" rather than silently locking the project.
func (b *Backend) ConnectReadOnly(path string) error {
	r, err := mmpr.OpenWithOptions(path, mmpr.OpenOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	b.reader = r
	b.writer = nil
	b.path = path
	return nil
}

// Disconnect closes the modelsdk reader.
func (b *Backend) Disconnect() error {
	if b.reader == nil {
		return nil
	}
	err := b.reader.Close()
	b.reader = nil
	return err
}

// Commit is a no-op for the read-only slice.
func (b *Backend) Commit() error { return nil }

func (b *Backend) IsConnected() bool { return b.reader != nil }

func (b *Backend) Path() string { return b.path }

// ContentsDir is the mprcontents/ directory of an MPR v2 project, empty for v1.
func (b *Backend) ContentsDir() string {
	if b.reader == nil {
		return ""
	}
	return b.reader.ContentsDir()
}

// InvalidateCache drops the reader's unit cache. Nothing routes through the
// backend interface to reach it today, but the generated stub for a method with
// no results at all is a panic, so leaving it unimplemented parks a crash in the
// default engine against the day something does.
func (b *Backend) InvalidateCache() {
	if b.reader == nil {
		return
	}
	b.reader.InvalidateCache()
}

func (b *Backend) Version() types.MPRVersion {
	if b.reader == nil {
		return 0
	}
	return types.MPRVersion(b.reader.Version())
}

func (b *Backend) ProjectVersion() *types.ProjectVersion {
	if b.reader == nil {
		return nil
	}
	pv := b.reader.ProjectVersion()
	if pv == nil {
		return nil
	}
	return &types.ProjectVersion{
		ProductVersion: pv.ProductVersion,
		BuildVersion:   pv.BuildVersion,
		FormatVersion:  pv.FormatVersion,
		SchemaHash:     pv.SchemaHash,
		MajorVersion:   pv.MajorVersion,
		MinorVersion:   pv.MinorVersion,
		PatchVersion:   pv.PatchVersion,
	}
}

func (b *Backend) GetMendixVersion() (string, error) {
	if b.reader == nil {
		return "", nil
	}
	return b.reader.GetMendixVersion()
}

// --- ModuleBackend (read only) ---

func (b *Backend) ListModules() ([]*model.Module, error) {
	infos, err := b.reader.ListModules()
	if err != nil {
		return nil, err
	}
	out := make([]*model.Module, 0, len(infos))
	for _, mi := range infos {
		m := moduleFromInfo(mi)
		b.enrichModule(m)
		out = append(out, m)
	}
	return out, nil
}

// enrichModule fills in the Marketplace metadata by decoding the module unit.
// The reader returns only ID+Name; FromAppStore/AppStoreVersion (the SHOW
// MODULES "Source" column) and AppStoreGuid live on the gen Module.
//
// Called from every module lookup, not just the listing: a caller that reaches
// a module by name and then branches on FromAppStore — the marketplace guard in
// CREATE LAYOUT does — would otherwise read false for every module and never
// fire.
func (b *Backend) enrichModule(m *model.Module) {
	if m == nil || m.ID == "" {
		return
	}
	raw, err := b.reader.GetRawUnitBytes(string(m.ID))
	if err != nil || len(raw) == 0 {
		return
	}
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		return
	}
	gm, ok := el.(*genPr.Module)
	if !ok {
		return
	}
	m.FromAppStore = gm.FromAppStore()
	m.AppStoreVersion = gm.AppStoreVersion()
	m.AppStoreGuid = gm.AppStoreGuid()
}

func (b *Backend) GetModuleByName(name string) (*model.Module, error) {
	mi, err := b.reader.GetModuleByName(name)
	if err != nil || mi == nil {
		return nil, err
	}
	m := moduleFromInfo(mi)
	b.enrichModule(m)
	return m, nil
}

func (b *Backend) GetModule(id model.ID) (*model.Module, error) {
	mi, err := b.reader.GetModule(string(id))
	if err != nil || mi == nil {
		return nil, err
	}
	m := moduleFromInfo(mi)
	b.enrichModule(m)
	return m, nil
}

// moduleFromInfo converts the modelsdk ModuleInfo (ID + Name) into our
// model.Module. Richer fields (FromAppStore, version, contained documents)
// need a full gen.Module decode and are deferred to a later phase.
func moduleFromInfo(mi *mmpr.ModuleInfo) *model.Module {
	m := &model.Module{Name: mi.Name}
	m.ID = model.ID(mi.ID)
	return m
}
