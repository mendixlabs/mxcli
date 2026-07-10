// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/catalog"
	"github.com/JordtenBulte-OLC/mxcli/mdl/linter"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// countingMFReader records how the lint layer fetches microflows. It embeds
// minimalReader for the rest of the LintReader surface.
type countingMFReader struct {
	minimalReader
	mfs       []*microflows.Microflow
	listCalls int
	getCalls  int
}

func (c *countingMFReader) ListMicroflows() ([]*microflows.Microflow, error) {
	c.listCalls++
	return c.mfs, nil
}

func (c *countingMFReader) GetMicroflow(id model.ID) (*microflows.Microflow, error) {
	c.getCalls++
	for _, m := range c.mfs {
		if m.ID == id {
			return m, nil
		}
	}
	return nil, nil
}

// TestFullMicroflow_LoadsAllOnce guards issue #720: rules that inspect microflow
// bodies must fetch them through LintContext.FullMicroflow, which loads every
// microflow ONCE via ListMicroflows and serves lookups from cache. The previous
// per-microflow reader.GetMicroflow(id) path re-listed and re-BSON-decoded every
// unit on each call — O(N^2) — which made `mxcli report` hang for many minutes on
// large projects.
func TestFullMicroflow_LoadsAllOnce(t *testing.T) {
	mfA := &microflows.Microflow{}
	mfA.ID = "A"
	mfB := &microflows.Microflow{}
	mfB.ID = "B"
	cr := &countingMFReader{mfs: []*microflows.Microflow{mfA, mfB}}

	cat, err := catalog.New()
	if err != nil {
		t.Fatalf("catalog.New: %v", err)
	}
	defer cat.Close()
	ctx := linter.NewLintContext(cat, cr)

	// Simulate the hot path: many lookups (N rules × N microflows).
	for range 25 {
		got, err := ctx.FullMicroflow("A")
		if err != nil {
			t.Fatalf("FullMicroflow(A): %v", err)
		}
		if got == nil || got.ID != "A" {
			t.Fatalf("FullMicroflow(A) = %v, want microflow A", got)
		}
	}
	if got, _ := ctx.FullMicroflow("missing"); got != nil {
		t.Errorf("FullMicroflow(missing) = %v, want nil", got)
	}

	// The whole point: the reader is queried in bulk exactly once, never per-ID.
	if cr.listCalls != 1 {
		t.Errorf("ListMicroflows called %d times, want 1 (must load once and cache)", cr.listCalls)
	}
	if cr.getCalls != 0 {
		t.Errorf("GetMicroflow called %d times, want 0 (per-ID re-decode is the O(N^2) bug)", cr.getCalls)
	}
}
