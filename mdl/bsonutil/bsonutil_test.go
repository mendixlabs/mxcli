// SPDX-License-Identifier: Apache-2.0

package bsonutil

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
)

func TestIDToBsonBinary_ValidUUID(t *testing.T) {
	id := "550e8400-e29b-41d4-a716-446655440000"
	bin := IDToBsonBinary(id)

	if bin.Subtype != 0x00 {
		t.Errorf("expected subtype 0x00, got 0x%02x", bin.Subtype)
	}
	if len(bin.Data) != 16 {
		t.Errorf("expected 16 bytes, got %d", len(bin.Data))
	}
}

func TestIDToBsonBinary_PanicsOnInvalidUUID(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on invalid UUID, got none")
		}
	}()
	IDToBsonBinary("not-a-uuid")
}

func TestIDToBsonBinary_PanicsOnEmptyString(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on empty string, got none")
		}
	}()
	IDToBsonBinary("")
}

func TestBsonBinaryToID_Roundtrip(t *testing.T) {
	id := "550e8400-e29b-41d4-a716-446655440000"
	bin := IDToBsonBinary(id)
	got := BsonBinaryToID(bin)
	if got != id {
		t.Errorf("roundtrip failed: got %q, want %q", got, id)
	}
}

func TestNewIDBsonBinary_ProducesValidUUID(t *testing.T) {
	bin := NewIDBsonBinary()
	if bin.Subtype != 0x00 {
		t.Errorf("expected subtype 0x00, got 0x%02x", bin.Subtype)
	}
	if len(bin.Data) != 16 {
		t.Errorf("expected 16 bytes, got %d", len(bin.Data))
	}

	// Convert back and validate UUID format
	id := BsonBinaryToID(bin)
	if !types.ValidateID(id) {
		t.Errorf("generated ID is not valid UUID format: %q", id)
	}
}

func TestNewIDBsonBinary_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := BsonBinaryToID(NewIDBsonBinary())
		if seen[id] {
			t.Fatalf("duplicate ID generated: %q", id)
		}
		seen[id] = true
	}
}

func TestIDToBsonBinaryErr_ValidUUID(t *testing.T) {
	id := "550e8400-e29b-41d4-a716-446655440000"
	bin, err := IDToBsonBinaryErr(id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bin.Subtype != 0x00 {
		t.Errorf("expected subtype 0x00, got 0x%02x", bin.Subtype)
	}
	if len(bin.Data) != 16 {
		t.Errorf("expected 16 bytes, got %d", len(bin.Data))
	}
	// Roundtrip
	got := BsonBinaryToID(bin)
	if got != id {
		t.Errorf("roundtrip failed: got %q, want %q", got, id)
	}
}

func TestIDToBsonBinaryErr_InvalidUUID(t *testing.T) {
	_, err := IDToBsonBinaryErr("not-a-uuid")
	if err == nil {
		t.Fatal("expected error for invalid UUID, got nil")
	}
}

func TestIDToBsonBinaryErr_EmptyString(t *testing.T) {
	_, err := IDToBsonBinaryErr("")
	if err == nil {
		t.Fatal("expected error for empty string, got nil")
	}
}

// =============================================================================
// String / Bool — unexpected types must not panic
// =============================================================================

// Not parallel-safe: redirects global log output.
func TestStringBool_UnexpectedTypes_NoPanic(t *testing.T) {
	var buf bytes.Buffer
	origOutput := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(origOutput)

	// String with non-string values
	if s := String(42, "test"); s != "" {
		t.Errorf("expected empty string, got %q", s)
	}
	if s := String(nil, "test"); s != "" {
		t.Errorf("expected empty string for nil, got %q", s)
	}
	if s := String(true, "test"); s != "" {
		t.Errorf("expected empty string for bool, got %q", s)
	}

	// Bool with non-bool values
	if b := Bool("true", "test"); b {
		t.Error("expected false for string input")
	}
	if b := Bool(nil, "test"); b {
		t.Error("expected false for nil")
	}
	if b := Bool(42, "test"); b {
		t.Error("expected false for int input")
	}

	// Verify diagnostic warnings were emitted
	logged := buf.String()
	expectedWarnings := []string{
		`expected string for "test", got int`,
		`expected string for "test", got <nil>`,
		`expected string for "test", got bool`,
		`expected bool for "test", got string`,
		`expected bool for "test", got <nil>`,
		`expected bool for "test", got int`,
	}
	for _, w := range expectedWarnings {
		if !strings.Contains(logged, w) {
			t.Errorf("expected warning containing %q in log output", w)
		}
	}
}
