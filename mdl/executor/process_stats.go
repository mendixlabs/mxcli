// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"encoding/json"
	"os"

	"github.com/mendixlabs/mxcli/mdl/backend"
)

// StatsFileEnv names a file to which every project connection appends one
// JSON line when it closes: the unit files it read and the unit writes it
// offered and landed. Unset, nothing is written.
//
// These are the counts the read- and decode-budget tests assert on in process
// (unitFileReads, backend.WriteStats). The agent dev-loop benchmark needs them
// from outside, per command, because it measures whole processes; a count does
// not depend on the machine, so unlike a duration it can be compared exactly
// between two runs (docs/11-proposals/PROPOSAL_agent_devloop_benchmark.md).
//
// One line per connection rather than one per process: a command that
// reconnects, or starts child mxcli processes that inherit the variable,
// appends several, and the reader sums them.
const StatsFileEnv = "MXCLI_STATS_FILE"

// ProcessStats is one line of the stats file.
type ProcessStats struct {
	UnitFileReads int64 `json:"unit_file_reads"`
	// UnitStoreHits is the reads served from the persisted unit store. With
	// UnitFileReads it is every read not served from memory, so a command
	// that reads a unit twice shows here even when no file was opened twice.
	// Always written, zero included: a reader tells a command that made none
	// from a binary too old to count them by whether the key is there.
	UnitStoreHits int64 `json:"unit_store_hits"`
	WritesOffered int   `json:"writes_offered"`
	WritesWritten int   `json:"writes_written"`
}

// disconnectBackend closes b's project connection and, when StatsFileEnv is
// set, appends what the connection did. Every disconnect goes through here so
// no connection's counts are lost.
//
// The reads are taken before the disconnect, which drops the reader that
// counts them; the writes after it, because the disconnect is what flushes
// deferred writes.
func disconnectBackend(b backend.FullBackend) error {
	path := os.Getenv(StatsFileEnv)
	if path == "" {
		return b.Disconnect()
	}
	var stats ProcessStats
	if r, ok := b.(interface{ UnitFileReads() int64 }); ok {
		stats.UnitFileReads = r.UnitFileReads()
	}
	if r, ok := b.(interface{ UnitStoreHits() int64 }); ok {
		stats.UnitStoreHits = r.UnitStoreHits()
	}
	err := b.Disconnect()
	ws := currentWriteStats(b)
	stats.WritesOffered, stats.WritesWritten = ws.Offered, ws.Written
	appendProcessStats(path, stats)
	return err
}

// appendProcessStats writes one line. A failure is swallowed: the file is a
// measurement aid, and a command must not fail because it could not be
// measured.
func appendProcessStats(path string, stats ProcessStats) {
	line, err := json.Marshal(stats)
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}
