// SPDX-License-Identifier: Apache-2.0

// Package canon answers one question: would writing this document change
// anything, or only which UUIDs it happened to mint?
//
// mxcli's write paths do not reconcile. `create or replace` rebuilds a document
// from the MDL and every sub-element in that rebuild is a new object whose ID
// comes from a random UUID, so the output is a function of the script *and a
// random source*. Two runs of the same script produce different bytes for
// identical content, which is why re-running an idempotent script still shows up
// as a change in version control.
//
// A canonical form removes exactly that noise: every element $ID is replaced by
// its index in a deterministic containment walk. Two documents are canonically
// equal when they differ only in the *choice* of element IDs — which is exactly
// the condition under which the write could have been skipped.
//
// The trick that makes this implementable today is that you do not need to know
// which *properties* hold references. You only need the set of element IDs,
// which a containment walk yields, and then every occurrence of one of those IDs
// anywhere in the document is a reference by definition.
//
// This is deliberately conservative. Anything it cannot render — a malformed
// document, an unmarshal failure — is reported as an error, and callers are
// expected to treat an error as "different" and write. A false *different* costs
// a redundant write, which is the behaviour that existed before; a false *equal*
// silently discards the user's intent.
//
// See ADR-0008 (docs/13-decisions/0008-identity-and-idempotence.md).
package canon

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Equal reports whether two raw BSON documents are canonically equal. An error
// from either side is returned rather than swallowed, so the caller decides —
// the write path treats any error as "not equal".
func Equal(a, b []byte) (bool, error) {
	da, err := Digest(a)
	if err != nil {
		return false, fmt.Errorf("canonicalise left: %w", err)
	}
	db, err := Digest(b)
	if err != nil {
		return false, fmt.Errorf("canonicalise right: %w", err)
	}
	return da == db, nil
}

// Digest returns the canonical digest of a raw BSON document.
func Digest(raw []byte) (string, error) { return DigestMasking(raw, nil) }

// DigestMasking is Digest with named properties replaced by a constant wherever
// they appear, for measuring what equality *would* look like if a field stopped
// being regenerated. The write path does not mask: a field that reaches storage
// is content, and the way to stop it churning is to stop regenerating it, not to
// stop looking at it.
func DigestMasking(raw []byte, mask map[string]bool) (string, error) {
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		return "", fmt.Errorf("unmarshal BSON (%d bytes): %w", len(raw), err)
	}
	return digestOfDoc(doc, mask), nil
}

// digestOfDoc is DigestMasking for a document already parsed. It only reads
// doc.
func digestOfDoc(doc bson.D, mask map[string]bool) string {
	ids := collectElementIDs(doc)
	var b strings.Builder
	renderValue(&b, doc, ids, mask)
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// collectElementIDs walks containment in a deterministic order and numbers every
// $ID by first appearance. The ordering is what makes the numbering comparable
// between two documents: the same shape yields the same numbering.
func collectElementIDs(v any) map[string]int {
	ids := make(map[string]int)
	var walk func(any)
	walk = func(v any) {
		if d, ok := asDoc(v); ok {
			if id, ok := elementID(d); ok {
				if _, seen := ids[id]; !seen {
					ids[id] = len(ids)
				}
			}
			for _, k := range sortedKeys(d) {
				walk(d[k])
			}
			return
		}
		if s, ok := asSlice(v); ok {
			for _, e := range s {
				walk(e)
			}
		}
	}
	walk(v)
	return ids
}

// renderValue writes a deterministic textual form. Keys are sorted so BSON field
// order cannot masquerade as a semantic difference.
func renderValue(b *strings.Builder, v any, ids map[string]int, mask map[string]bool) {
	if d, ok := asDoc(v); ok {
		b.WriteByte('{')
		for _, k := range sortedKeys(d) {
			b.WriteString(k)
			b.WriteByte(':')
			if mask[k] {
				b.WriteString("<masked>")
			} else {
				renderValue(b, d[k], ids, mask)
			}
			b.WriteByte(',')
		}
		b.WriteByte('}')
		return
	}
	if s, ok := asSlice(v); ok {
		b.WriteByte('[')
		for _, e := range s {
			renderValue(b, e, ids, mask)
			b.WriteByte(',')
		}
		b.WriteByte(']')
		return
	}
	b.WriteString(renderScalar(v, ids))
}

// renderScalar replaces any value that is one of this document's element IDs
// with its index. A UUID that is *not* an element ID of this document is left
// alone — it refers to something else (a GUID, a cross-unit id), and a
// difference in it is a real difference.
func renderScalar(v any, ids map[string]int) string {
	switch t := v.(type) {
	case bson.Binary:
		if len(t.Data) == 16 {
			if n, ok := ids[blobToUUID(t.Data)]; ok {
				return fmt.Sprintf("#%d", n)
			}
		}
		return fmt.Sprintf("bin%d:%x", t.Subtype, t.Data)
	case string:
		if n, ok := ids[t]; ok {
			return fmt.Sprintf("#%d", n)
		}
		return "s:" + t
	case nil:
		return "nil"
	default:
		return fmt.Sprintf("%v", v)
	}
}

func elementID(d map[string]any) (string, bool) {
	switch t := d["$ID"].(type) {
	case bson.Binary:
		if len(t.Data) != 16 {
			return "", false
		}
		return blobToUUID(t.Data), true
	case string:
		return t, true
	}
	return "", false
}

// asDoc flattens the several shapes BSON unmarshalling can produce for a
// subdocument into one map.
func asDoc(v any) (map[string]any, bool) {
	switch t := v.(type) {
	case bson.D:
		out := make(map[string]any, len(t))
		for _, e := range t {
			out[e.Key] = e.Value
		}
		return out, true
	case bson.M:
		return t, true
	case map[string]any:
		return t, true
	}
	return nil, false
}

func asSlice(v any) ([]any, bool) {
	switch t := v.(type) {
	case bson.A:
		return t, true
	case []any:
		return t, true
	}
	return nil, false
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// blobToUUID renders a 16-byte Mendix GUID blob (Microsoft layout: first three
// groups little-endian) as a canonical UUID string.
//
// Deliberately a local copy rather than an import: this package sits underneath
// the storage layer that owns the equivalent helper, so importing it would be a
// cycle. Only the *stability* of the mapping matters here, not its agreement
// with any other renderer — an $ID and a reference to it are both run through
// this same function.
func blobToUUID(b []byte) string {
	if len(b) != 16 {
		return hex.EncodeToString(b)
	}
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		b[3], b[2], b[1], b[0],
		b[5], b[4],
		b[7], b[6],
		b[8], b[9],
		b[10], b[11], b[12], b[13], b[14], b[15])
}
