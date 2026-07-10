// SPDX-License-Identifier: Apache-2.0
package unitstore_test

import (
	"fmt"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/unitstore"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

type stubPersistence struct {
	disk    map[model.ID][]byte
	stored  map[model.ID][]byte
	loadCnt int
}

func newStub(disk map[model.ID][]byte) *stubPersistence {
	if disk == nil {
		disk = make(map[model.ID][]byte)
	}
	return &stubPersistence{disk: disk, stored: make(map[model.ID][]byte)}
}
func (s *stubPersistence) Load(id model.ID) ([]byte, error) {
	s.loadCnt++
	if d, ok := s.disk[id]; ok {
		return d, nil
	}
	return nil, fmt.Errorf("not found: %s", id)
}
func (s *stubPersistence) BatchStore(units map[model.ID][]byte) error {
	for id, data := range units {
		s.stored[id] = data
		s.disk[id] = data
	}
	return nil
}
func (s *stubPersistence) BatchHash(units map[model.ID][]byte) (map[model.ID]string, error) {
	out := make(map[model.ID]string, len(units))
	for id := range units {
		out[id] = "hash-" + string(id)
	}
	return out, nil
}

func TestBufferedUnitStore_WriteStaysInMemory(t *testing.T) {
	p := newStub(nil)
	buf := unitstore.New(p)
	id := model.ID("unit-1")
	if err := buf.Write(id, []byte("bson-bytes")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if len(p.stored) != 0 {
		t.Errorf("expected no disk writes before Flush, got %d", len(p.stored))
	}
}

func TestBufferedUnitStore_ReadReturnsWrittenData(t *testing.T) {
	p := newStub(nil)
	buf := unitstore.New(p)
	id := model.ID("unit-1")
	data := []byte("bson-bytes")
	_ = buf.Write(id, data)
	got, err := buf.Read(id)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("got %q, want %q", got, data)
	}
	if p.loadCnt != 0 {
		t.Errorf("expected no Load calls, got %d", p.loadCnt)
	}
}

func TestBufferedUnitStore_ReadLazyLoadsFromDisk(t *testing.T) {
	id := model.ID("unit-1")
	p := newStub(map[model.ID][]byte{id: []byte("disk-data")})
	buf := unitstore.New(p)
	got, err := buf.Read(id)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != "disk-data" {
		t.Errorf("got %q, want disk-data", got)
	}
	if p.loadCnt != 1 {
		t.Errorf("expected 1 Load call, got %d", p.loadCnt)
	}
	_, _ = buf.Read(id)
	if p.loadCnt != 1 {
		t.Errorf("expected cache hit on second read, got %d Load calls", p.loadCnt)
	}
}

func TestBufferedUnitStore_FlushWritesToDisk(t *testing.T) {
	p := newStub(nil)
	buf := unitstore.New(p)
	id := model.ID("unit-1")
	_ = buf.Write(id, []byte("data"))
	if err := buf.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if _, ok := p.stored[id]; !ok {
		t.Errorf("expected unit in stored after Flush")
	}
	got, _ := buf.Read(id)
	if string(got) != "data" {
		t.Errorf("expected data after Flush, got %q", got)
	}
	p.stored = make(map[model.ID][]byte)
	if err := buf.Flush(); err != nil {
		t.Fatalf("second Flush: %v", err)
	}
	if len(p.stored) != 0 {
		t.Errorf("expected no second BatchStore on empty pending")
	}
}

func TestBufferedUnitStore_DiscardClearsPending(t *testing.T) {
	p := newStub(nil)
	buf := unitstore.New(p)
	_ = buf.Write(model.ID("unit-1"), []byte("data"))
	buf.Discard()
	if len(p.stored) != 0 {
		t.Errorf("expected no disk writes after Discard")
	}
	_, err := buf.Read(model.ID("unit-1"))
	if err == nil {
		t.Errorf("expected error reading discarded unit from empty disk")
	}
}
