// SPDX-License-Identifier: Apache-2.0

package unitstore

import (
	"fmt"
	"sync"

	"github.com/JordtenBulte-OLC/mxcli/model"
)

// BufferedUnitStore holds pending writes in memory and flushes them to disk
// as a single batch. Reads check the pending set and a lazy-loaded cache
// before going to the persistence layer.
type BufferedUnitStore struct {
	persistence UnitPersistence

	mu      sync.RWMutex
	pending map[model.ID][]byte
	loaded  map[model.ID][]byte
}

// New creates a BufferedUnitStore backed by the given persistence layer.
func New(p UnitPersistence) *BufferedUnitStore {
	return &BufferedUnitStore{
		persistence: p,
		pending:     make(map[model.ID][]byte),
		loaded:      make(map[model.ID][]byte),
	}
}

// Read returns unit bytes. Priority: pending > loaded cache > disk (lazy).
func (b *BufferedUnitStore) Read(id model.ID) ([]byte, error) {
	b.mu.RLock()
	if data, ok := b.pending[id]; ok {
		b.mu.RUnlock()
		return data, nil
	}
	if data, ok := b.loaded[id]; ok {
		b.mu.RUnlock()
		return data, nil
	}
	b.mu.RUnlock()

	data, err := b.persistence.Load(id)
	if err != nil {
		return nil, fmt.Errorf("load unit %s: %w", id, err)
	}
	b.mu.Lock()
	b.loaded[id] = data
	b.mu.Unlock()
	return data, nil
}

// Write stores data in the pending set. No disk I/O occurs.
func (b *BufferedUnitStore) Write(id model.ID, data []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pending[id] = data
	return nil
}

// Flush writes all pending units to disk in one batch, promotes them to the
// loaded cache, and clears the pending set.
func (b *BufferedUnitStore) Flush() error {
	b.mu.Lock()
	pending := make(map[model.ID][]byte, len(b.pending))
	for k, v := range b.pending {
		pending[k] = v
	}
	b.mu.Unlock()

	if len(pending) == 0 {
		return nil
	}

	if _, err := b.persistence.BatchHash(pending); err != nil {
		return fmt.Errorf("batch hash: %w", err)
	}
	if err := b.persistence.BatchStore(pending); err != nil {
		return fmt.Errorf("batch store: %w", err)
	}

	b.mu.Lock()
	for id, data := range pending {
		b.loaded[id] = data
	}
	b.pending = make(map[model.ID][]byte)
	b.mu.Unlock()
	return nil
}

// Discard drops all pending writes. The loaded cache is preserved.
func (b *BufferedUnitStore) Discard() {
	b.mu.Lock()
	b.pending = make(map[model.ID][]byte)
	b.mu.Unlock()
}

// PendingCount returns the number of units waiting to be flushed.
func (b *BufferedUnitStore) PendingCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.pending)
}
