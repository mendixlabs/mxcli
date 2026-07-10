// SPDX-License-Identifier: Apache-2.0

// Package modelsdkbackend is the modelsdk-engine implementation of
// backend.FullBackend. It lives at a separate import path from the legacy
// mdl/backend/mpr (mprbackend) package so both engines can be linked at once
// and selected via the MXCLI_ENGINE seam (see cmd/mxcli/engine.go).
//
// Phase 1 (docs/plans/2026-06-05-adopt-modelsdk-engine.md) is a READ slice:
// it embeds *mock.MockBackend so the full 27-interface FullBackend surface is
// satisfied, and overrides only the connection + module read methods to drive
// the real modelsdk codec engine. Un-overridden methods fall through to the
// mock stubs (which return zero/nil and never panic). Write methods are NOT
// implemented yet — callers must not rely on them persisting; the CLI prints a
// read-only warning when this engine is selected.
package modelsdkbackend

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend"
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	genPr "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/projects"
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
)

// Compile-time guarantee that the backend satisfies the whole interface (via the
// embedded `unimplemented` for every method it doesn't override).
var _ backend.FullBackend = (*Backend)(nil)

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
}

// New constructs a modelsdk backend.
func New() *Backend {
	return &Backend{}
}

// errUnimplemented is the error every not-yet-ported FullBackend method returns
// (via the generated unimplemented embed). Loud failure beats the silent no-op
// the embedded mock used to give — see ADR-0005 "guard, don't silently drop".
func errUnimplemented(method string) error {
	return fmt.Errorf("modelsdk engine: %s not implemented yet — rerun with MXCLI_ENGINE=legacy", method)
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
	dec := codec.NewDecoder(codec.DefaultRegistry)
	out := make([]*model.Module, 0, len(infos))
	for _, mi := range infos {
		m := moduleFromInfo(mi)
		// Enrich with Marketplace metadata by decoding the module unit.
		// reader.ListModules returns only ID+Name; FromAppStore/AppStoreVersion
		// (the SHOW MODULES "Source" column) live on the gen Module.
		if raw, rerr := b.reader.GetRawUnitBytes(mi.ID); rerr == nil && len(raw) > 0 {
			if el, derr := dec.Decode(raw); derr == nil {
				if gm, ok := el.(*genPr.Module); ok {
					m.FromAppStore = gm.FromAppStore()
					m.AppStoreVersion = gm.AppStoreVersion()
				}
			}
		}
		out = append(out, m)
	}
	return out, nil
}

func (b *Backend) GetModuleByName(name string) (*model.Module, error) {
	mi, err := b.reader.GetModuleByName(name)
	if err != nil || mi == nil {
		return nil, err
	}
	return moduleFromInfo(mi), nil
}

func (b *Backend) GetModule(id model.ID) (*model.Module, error) {
	mi, err := b.reader.GetModule(string(id))
	if err != nil || mi == nil {
		return nil, err
	}
	return moduleFromInfo(mi), nil
}

// moduleFromInfo converts the modelsdk ModuleInfo (ID + Name) into our
// model.Module. Richer fields (FromAppStore, version, contained documents)
// need a full gen.Module decode and are deferred to a later phase.
func moduleFromInfo(mi *mmpr.ModuleInfo) *model.Module {
	m := &model.Module{Name: mi.Name}
	m.ID = model.ID(mi.ID)
	return m
}
