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
	"path/filepath"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	genPr "github.com/mendixlabs/mxcli/modelsdk/gen/projects"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/modelsdk/mprread"
)

// Compile-time guarantee that the backend satisfies the whole interface (via the
// embedded `unimplemented` for every method it doesn't override).
var _ backend.FullBackend = (*Backend)(nil)
var _ backend.WriteStatsReporter = (*Backend)(nil)
var _ backend.SourceFileReporter = (*Backend)(nil)
var _ backend.DiskChangeSyncer = (*Backend)(nil)

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

	// domainModelListings counts ListDomainModels calls (DomainModelListings).
	domainModelListings int64

	// fileWrites counts writes that do not go through unit storage — the
	// generated .java/.js source of a code action, whose body lives in
	// javasource/ or javascriptsource/ rather than in its unit. They are folded
	// into WriteStats so a body-only edit is not reported as "Unchanged".
	fileWrites backend.WriteStats

	// sourceFiles lists the files and folders beside the model that writes
	// changed since the executor last took them (TakeSourceFileChanges).
	sourceFiles []backend.SourceFileChange

	// unitScope, on a view made by WithUnitScope, is the units its document
	// lists hold; nil lists every unit.
	unitScope func(unitID string) bool
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

// Readers' names for what the files beside the model hold.
const (
	javaActionBody       = "Java action body"
	javaScriptActionBody = "JavaScript action body"
)

// noteSourceFile records a file or folder beside the model that a write just
// changed, by its absolute path (from, for a move, likewise; empty otherwise).
// Call it only once the change is on disk.
func (b *Backend) noteSourceFile(path, from string, removed bool, what string) {
	c := backend.SourceFileChange{Path: b.projectRelative(path), Removed: removed, What: what}
	if from != "" {
		c.From = b.projectRelative(from)
	}
	b.sourceFiles = append(b.sourceFiles, c)
}

// projectRelative turns an absolute path under the project directory into the
// slash-separated path a reader would stage. A folder's trailing separator is
// kept, as "/".
func (b *Backend) projectRelative(path string) string {
	folder := strings.HasSuffix(path, string(filepath.Separator))
	if rel, err := filepath.Rel(filepath.Dir(b.path), path); err == nil {
		path = rel
	}
	path = filepath.ToSlash(path)
	if folder && !strings.HasSuffix(path, "/") {
		path += "/"
	}
	return path
}

// TakeSourceFileChanges returns the files and folders beside the model that
// writes changed since the previous call, and forgets them.
func (b *Backend) TakeSourceFileChanges() []backend.SourceFileChange {
	changes := b.sourceFiles
	b.sourceFiles = nil
	return changes
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

// Disconnect closes the modelsdk reader, writing first any unit update a
// deferred run still holds.
func (b *Backend) Disconnect() error {
	if b.reader == nil {
		return nil
	}
	flushErr := b.FlushDeferredWrites()
	err := b.reader.Close()
	b.reader = nil
	if err != nil {
		return err
	}
	return flushErr
}

// SyncWithDisk reports whether another process changed the project since this
// connection last looked, having first dropped every view of it the
// connection keeps, so the next read sees the disk (ako/mxcli#2121). A session
// calls it before each statement: one PRAGMA and one stat, not a re-read.
//
// When the .mpr was replaced by a new file (a git checkout renames one into
// place) the connection still holds the old one open, so the project is
// reopened. Not while a deferred run or a simulation holds unwritten units:
// reopening would write them into, or drop them from, a project they were not
// made against, so that is refused with an error instead.
func (b *Backend) SyncWithDisk() (changed bool, err error) {
	if b.reader == nil {
		return false, nil
	}
	switch b.reader.SyncWithDisk() {
	case mmpr.DiskUnchanged:
		return false, nil
	case mmpr.DiskChanged:
		return true, nil
	}
	if b.writer != nil && b.writer.HoldsWrites() {
		return true, fmt.Errorf("%s was replaced on disk while unwritten changes were held; reconnect to read the new file", b.path)
	}
	readOnly := b.writer == nil
	path := b.path
	if err := b.reader.Close(); err != nil {
		return true, fmt.Errorf("close the replaced project: %w", err)
	}
	b.reader, b.writer = nil, nil
	if readOnly {
		err = b.ConnectReadOnly(path)
	} else {
		err = b.Connect(path)
	}
	if err != nil {
		return true, fmt.Errorf("reopen %s after it was replaced on disk: %w", path, err)
	}
	return true, nil
}

// DeferUnitWrites holds unit updates in memory until FlushDeferredWrites, so a
// run of statements is written once and judged by its net result
// (mmpr.Writer.DeferUnitWrites, ako/mxcli#872). Reads during the run see the
// held bytes. A no-op on a read-only connection.
func (b *Backend) DeferUnitWrites() {
	if b.writer != nil {
		b.writer.DeferUnitWrites()
	}
}

// DeferUnitWritesJudged is DeferUnitWrites for a run whose statements report
// for themselves: each held write is counted in WriteStats as it is held
// (mmpr.Writer.DeferUnitWritesJudged, ako/mxcli#2160).
func (b *Backend) DeferUnitWritesJudged() {
	if b.writer != nil {
		b.writer.DeferUnitWritesJudged()
	}
}

// UnitReconciles is how many times this backend's writer has compared a write
// with the stored unit. For tests.
func (b *Backend) UnitReconciles() int {
	if b.writer == nil {
		return 0
	}
	return b.writer.Reconciles()
}

// FlushDeferredWrites writes what a deferred run holds, each unit once and
// reconciled against what was stored before the run.
func (b *Backend) FlushDeferredWrites() error {
	if b.writer == nil {
		return nil
	}
	return b.writer.FlushDeferredWrites()
}

// SimulateWrites holds unit updates in memory, read back but never written,
// until the returned function discards them; any other write is refused
// (mmpr.Writer.SimulateWrites). ok is false — and nothing is simulated — on a
// read-only connection or while a deferred run is open.
func (b *Backend) SimulateWrites() (discard func(), ok bool) {
	if b.writer == nil {
		return func() {}, false
	}
	return b.writer.SimulateWrites()
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

// CacheUnitReads holds the unit contents this backend reads in memory until
// release is called: for a pass that writes nothing and looks documents up
// once per statement (mendixlabs/mxcli#1272). See mpr.Reader.CacheUnitReads.
func (b *Backend) CacheUnitReads() (release func()) {
	if b.reader == nil {
		return func() {}
	}
	return b.reader.CacheUnitReads()
}

// UnitFileReads is how many unit files this backend's reader has read from
// mprcontents/ — what CacheUnitReads saves. For tests.
func (b *Backend) UnitFileReads() int64 {
	if b.reader == nil {
		return 0
	}
	return b.reader.UnitFileReads()
}

// UnitIndexBuilds is how many times this backend's reader has scanned the Unit
// table to rebuild its list of units. For tests.
func (b *Backend) UnitIndexBuilds() int64 {
	if b.reader == nil {
		return 0
	}
	return b.reader.UnitIndexBuilds()
}

// UnitStoreHits is how many unit reads this backend's reader served from the
// persisted unit store instead of a file. With UnitFileReads it is every read
// that was not served from memory. For tests.
func (b *Backend) UnitStoreHits() int64 {
	if b.reader == nil {
		return 0
	}
	return b.reader.UnitStoreHits()
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

// GetProjectRootID returns the $ID of the Projects$Project root unit — the
// container every module points to, and the value Business Events uses as the
// app id (mendixlabs/mxcli#1526). A copy of the model shares it.
func (b *Backend) GetProjectRootID() (string, error) {
	if b.reader == nil {
		return "", fmt.Errorf("GetProjectRootID: not connected")
	}
	return b.reader.GetProjectRootID()
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
	if pm, ok := b.reader.PackagedModuleOf(string(m.ID)); ok {
		m.PackageFile = pm.File
	}
	// Through the decode cache: a module is looked up for every statement of a
	// script, and decoding its unit afresh each time was a full decode per
	// module per listing (ako/mxcli#2160).
	u, ok, err := mprread.UnitByIDAs[*genPr.Module](b.reader, m.ID)
	if err != nil || !ok {
		return
	}
	gm := u.Element
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
