// SPDX-License-Identifier: Apache-2.0

package main

// Canonical-form digests: the normalisation needed to answer "did this write
// change anything, or only which UUIDs it happened to mint?"
//
// This is the opposite of the rest of this tool. Elsewhere the $ID is the entire
// signal; here it is the entire noise. Two documents are canonically equal when
// they differ only in the *choice* of element IDs, not in structure or values —
// which is exactly the condition under which a write could have been skipped.
//
// The trick that makes this implementable today: you do not need to know which
// *properties* hold references. You only need the set of element IDs, which a
// plain containment walk yields, and then every occurrence of one of those IDs
// anywhere in the document is a reference by definition. Each is replaced by the
// index at which its target was first seen, so two documents built with
// different random UUIDs but identical shape render identically.
//
// Two digests are emitted per unit:
//
//	canon   — IDs canonicalised, everything else verbatim.
//	masked  — the same, plus fields mxcli deliberately regenerates on every
//	          write (see volatileFields) replaced by a constant.
//
// The pair is the diagnostic. canon differing while masked matches means the
// only thing that changed is a field mxcli mints fresh by policy — today that is
// Microflows$Microflow.StableId, registered as a FreshGUIDField in
// microflow_write.go. That distinction is what decides whether "skip the write
// when nothing changed" is reachable, or blocked behind a policy change first.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/types"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// volatileFields are regenerated on every write regardless of content, so they
// make a document differ from itself. Masking them measures what idempotence
// would look like if the policy changed.
//
// StableId is not incidental churn. Mendix declares it IsIdentifier, seeds it
// once via a one-time conversion, transplants it across a module update
// (PackageUtils.RescueStableIDs), and derives each client-callable microflow's
// operation id from it — operationId == base64(uuid5(projectId, StableId)).
// See ADR-0008, "What StableId is".
var volatileFields = map[string]bool{
	"StableId": true,
}

// canonicalDigests renders a unit twice and returns (canon, masked).
func canonicalDigests(doc map[string]any) (string, string) {
	ids := collectElementIDs(doc)
	return digestOf(doc, ids, false), digestOf(doc, ids, true)
}

func digestOf(doc map[string]any, ids map[string]int, mask bool) string {
	var b strings.Builder
	renderValue(&b, doc, ids, mask)
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])[:16]
}

// collectElementIDs walks containment in a deterministic order and numbers every
// $ID by first appearance. Order is what makes the numbering comparable between
// two documents: the same shape yields the same numbering.
func collectElementIDs(doc map[string]any) map[string]int {
	ids := make(map[string]int)
	var walk func(v any)
	walk = func(v any) {
		if m, ok := asMap(v); ok {
			if id, ok := elementID(m); ok {
				if _, seen := ids[id]; !seen {
					ids[id] = len(ids)
				}
			}
			for _, k := range sortedKeys(m) {
				walk(m[k])
			}
			return
		}
		if s, ok := asSlice(v); ok {
			for _, e := range s {
				walk(e)
			}
		}
	}
	walk(doc)
	return ids
}

// renderValue writes a deterministic textual form. Keys are sorted so that BSON
// field order cannot masquerade as a semantic difference.
func renderValue(b *strings.Builder, v any, ids map[string]int, mask bool) {
	if m, ok := asMap(v); ok {
		b.WriteByte('{')
		for _, k := range sortedKeys(m) {
			b.WriteString(k)
			b.WriteByte(':')
			if mask && volatileFields[k] {
				b.WriteString("<volatile>")
			} else {
				renderValue(b, m[k], ids, mask)
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
// with its index. A UUID that is not an element ID in this document is left
// alone — it refers to something else (a GUID, a cross-unit id) and a difference
// in it is a real difference.
func renderScalar(v any, ids map[string]int) string {
	switch t := v.(type) {
	case primitive.Binary:
		if len(t.Data) == 16 {
			if n, ok := ids[types.BlobToUUID(t.Data)]; ok {
				return fmt.Sprintf("#%d", n)
			}
		}
		return fmt.Sprintf("bin:%x", t.Data)
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

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
