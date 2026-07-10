// SPDX-License-Identifier: Apache-2.0

// Package unitstore defines the BufferedUnitStore and its persistence contract.
// All writes during an import session are held in memory and flushed to disk
// as a single batch at each file checkpoint, eliminating per-statement
// SQLite transactions and fsync overhead.
package unitstore

import "github.com/JordtenBulte-OLC/mxcli/model"

// UnitReader is the read-only face of the unit buffer.
// Reads check the in-memory pending/loaded maps before going to disk.
type UnitReader interface {
	Read(id model.ID) ([]byte, error)
}

// UnitWriter adds write and lifecycle methods to UnitReader.
type UnitWriter interface {
	UnitReader
	// Write stores data in the in-memory pending set. No disk I/O.
	Write(id model.ID, data []byte) error
	// Flush batches all pending writes to disk in a single transaction,
	// promotes them to the loaded cache, and clears the pending set.
	Flush() error
	// Discard drops all pending writes without writing to disk.
	Discard()
}

// UnitPersistence is the storage abstraction that BufferedUnitStore delegates
// to for actual I/O. Implement MprUnitPersistence for production use;
// use a stub for testing.
type UnitPersistence interface {
	// Load reads raw BSON bytes for a single unit from disk.
	// Called lazily — only when a unit is first read and not yet in cache.
	Load(id model.ID) ([]byte, error)
	// BatchStore writes all units in a single SQLite transaction.
	BatchStore(units map[model.ID][]byte) error
	// BatchHash returns a SHA-256 hex string per unit (used for @cache: markers).
	BatchHash(units map[model.ID][]byte) (map[model.ID]string, error)
}
