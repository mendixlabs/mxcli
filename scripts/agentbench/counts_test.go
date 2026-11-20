// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func hits(n int64) *int64 { return &n }

// A re-read of a unit is served from the persisted store, so it is the store
// hits that tell a command reading every unit twice from one reading it once
// (ako/mxcli#2160). They are summed over a command's stats lines, and stay
// unreported when no line reports them — a baseline older than the count.
func TestReadCounts_SumsStoreHitsAndKeepsTheirAbsence(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	got := readCounts(write("new.jsonl", `{"unit_file_reads":3,"unit_store_hits":40,"writes_offered":1,"writes_written":1}
{"unit_file_reads":2,"unit_store_hits":0,"writes_offered":0,"writes_written":0}
`))
	if got.UnitFileReads != 5 || got.UnitStoreHits == nil || *got.UnitStoreHits != 40 || got.WritesOffered != 1 {
		t.Fatalf("got %+v (hits %v)", got, got.UnitStoreHits)
	}
	// Zero reported is not the same as not reported.
	zero := readCounts(write("zero.jsonl", `{"unit_file_reads":2,"unit_store_hits":0,"writes_offered":0,"writes_written":0}`+"\n"))
	if zero.UnitStoreHits == nil || *zero.UnitStoreHits != 0 {
		t.Fatalf("a reported zero was lost: %v", zero.UnitStoreHits)
	}
	old := readCounts(write("old.jsonl", `{"unit_file_reads":5,"writes_offered":1,"writes_written":1}`+"\n"))
	if old.UnitStoreHits != nil {
		t.Fatalf("a binary that reports no store hits was given %d", *old.UnitStoreHits)
	}
}

func TestCounts_SameComparesStoreHitsOnlyWhereBothReportThem(t *testing.T) {
	base := counts{UnitFileReads: 5, WritesOffered: 1, WritesWritten: 1}
	with := func(n int64) counts { c := base; c.UnitStoreHits = hits(n); return c }
	if !with(40).same(with(40)) {
		t.Error("equal counts differ")
	}
	// The case the count exists for: the same files, more reads.
	if with(40).same(with(90)) {
		t.Error("90 store hits against 40 were not a difference")
	}
	// Against a baseline too old to count them, their absence is not one.
	if !with(40).same(base) || !base.same(with(40)) {
		t.Error("an unreported count was compared")
	}
	other := with(40)
	other.UnitFileReads = 6
	if with(40).same(other) {
		t.Error("different file reads were the same")
	}
}

func TestReads_ShowsBothCountsWhereReported(t *testing.T) {
	if got := reads(&counts{UnitFileReads: 7}); got != "7" {
		t.Errorf("got %q", got)
	}
	if got := reads(&counts{UnitFileReads: 7, UnitStoreHits: hits(271)}); got != "7 + 271" {
		t.Errorf("got %q", got)
	}
	if got := reads(nil); got != "n/a" {
		t.Errorf("got %q", got)
	}
}
