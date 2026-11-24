// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"fmt"
	"sort"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// An element's GUID is the DATABASE's identity for it, and it is not
// interchangeable with its $ID. The runtime keys mendixsystem$entity.id and
// mendixsystem$attribute.id on the GUID verbatim, so changing only an
// attribute's GUID — same name, same type, same entity — makes the synchroniser
// treat it as an attribute deleted and a new one added, and DROP its column on
// the next deploy (CLAUDE.md, "A GUID Is the Database's Identity").
//
// That is a data-loss bug with no diagnostic behind it. The model stays
// perfectly valid, so mx check is clean and the build passes; mxcli's own reader
// never surfaces the GUID, so DESCRIBE is byte-identical before and after. It
// only becomes visible when the package meets a database that already holds
// data — which is to say, in production. Issue #1119 lost 28 attributes of 607
// rows that way, from one ALTER of one entity.
//
// Worse, the damage does not repeat: the codec writes GUID = $ID, and the $ID is
// held stable by TransplantIDs, so the SECOND identical write produces
// byte-identical bytes and is elided. The corruption happens exactly once, on
// the first write, and every check afterwards — including a re-run of the same
// script — reports "Unchanged".
//
// So the three guards that already stand at this choke point cannot see it:
//
//   - No-op elision compares a document with itself and finds it stable.
//   - TestFreshGUIDFieldsHaveAnIdentityDecision sees only codec FreshGUIDFields.
//     A GUID derived from the $ID is registered as EmitGUID, a different
//     mechanism, so it was never in that guard's view — the same blind spot that
//     let Workflows$*.PersistentId through in #949.
//   - identityFields/CarryIdentity reach only top-level properties of the
//     document root, and these GUIDs sit on elements nested inside it.
//
// Hence a guard here, in the same spirit as DuplicateElementIDError: cheap, at
// the moment the bytes would land, and phrased as the message the user would
// otherwise never get. It converts silent data loss into one refusal naming the
// element.
//
// It is deliberately NOT a repair. Carrying the stored GUID here would mean
// trusting the structural pairing TransplantIDs is built on, whose correctness
// bar is explicitly low — a wrong $ID match only makes a diff bigger, but a
// wrong GUID match makes the runtime adopt another column's data under a new
// name and type. The fix belongs where the write knows which element is which:
// see carryChildIdentity in mdl/backend/modelsdk, which pairs on the $ID the
// executor tracked through the statement.
//
// The check runs AFTER TransplantIDs, because before it a rebuilt element carries
// a freshly minted $ID that appears in no stored document and there would be
// nothing to compare against.
//
// But a shared $ID after the transplant does NOT by itself mean the same member,
// and an earlier version of this comment claimed it did ("unambiguously"). The
// transplant pairs STRUCTURALLY — by $Type and shape, LCS-anchored — and its own
// correctness bar is low on purpose, because a wrong $ID match only makes a diff
// bigger. Feed that pairing to a guard and a wrong match becomes a refusal.
//
// Measured: `CREATE OR MODIFY PERSISTENT ENTITY BusinessEvents.PublishedBusinessEvent
// (EventId: long)` against the marketplace module, whose entity has six
// differently-named attributes. The statement drops all six and adds one; the
// transplant paired the NEW attribute with one of the REMOVED ones and handed it that
// stored $ID; the codec had written GUID = $ID, and the transplant substitutes over
// every 16-byte binary, so the GUID followed. The guard then saw a stored $ID whose
// GUID had "changed" and refused a write that corrupts nothing — blocking a documented
// statement on a doctype script that had been passing for months.
//
// So the pairing here is $ID **plus the member's identity**: the same Name where the
// element has one, and the same $Type where it has none (see sameMember).
//
// The $Type is deliberately not compared for a named element. An earlier version
// did, and it went blind to the one conversion mxcli performs: MOVE ENTITY re-types
// an Association as a CrossAssociation in place, under the same $ID and Name — the
// very arm that exposed ako/mxcli#503. The transplant never pairs across a $Type, so
// comparing it guarded against nothing the transplant can produce.
//
// What the guard does NOT cover, each checked instead where it is decidable:
//
//   - A RENAME that re-minted a GUID, because the name is what changed. The carry
//     tests in mdl/backend/modelsdk cover it (TestIssue1119_AlterPreservesAttributeGUIDs,
//     RenameAttribute case).
//   - Any element that changes UNIT — MOVE ENTITY's entity and attributes, or the
//     cross-association when the FROM side moves. The guard compares one unit's
//     written bytes with that same unit's stored bytes, and in the target unit the
//     moved $ID pairs with nothing. TestIssue503_MovePreservesStorageGUIDs covers it.
//   - A pair where only one side has a Name. Nothing converts between those shapes,
//     so calling them one member would be a guess.
//
// A backstop that refuses correct writes is worse than a backstop with a hole: the
// first makes the tool unusable for work the user is entitled to do, and this one had
// already done so. The holes are listed so nobody mistakes a quiet guard for coverage.

// GUIDChange is one element that kept its $ID across a write while its GUID
// changed — the shape of the #1119 defect.
type GUIDChange struct {
	ElementID string
	Type      string
	Stored    string
	Written   string
}

// StorageGUIDChanges reports every element that appears in both documents under
// the same $ID but with a different GUID, in a stable order.
//
// Only an element is considered: a sub-document carrying both $ID and $Type.
// A pointer to another element is a primitive property holding the same 16-byte
// shape under a different key, and a containment walk meets plenty of those.
//
// An element present on only one side is not a change: a genuinely new element
// has an $ID no stored element holds, and a deleted one is simply absent. Nor is
// a GUID that only one side carries — an optional property Mendix fills in on
// load must not be invented, and one it stopped writing must not be preserved.
//
// Nor is a pair that is not the same member. Sharing an $ID after the transplant
// does not make two elements the same MEMBER — see the note above the type — so the
// member's own identity has to agree before a GUID difference means anything: its
// Name where it has one (whatever its $Type), its $Type where it has none.
//
// A document that cannot be unmarshalled yields no changes rather than an error.
// This runs on the write path, where failing a write because the guard could not
// read the bytes would be worse than the defect it prevents.
func StorageGUIDChanges(contents, stored []byte) []GUIDChange {
	now := elementGUIDs(contents)
	if len(now) == 0 {
		return nil
	}
	was := elementGUIDs(stored)
	if len(was) == 0 {
		return nil
	}

	ids := make([]string, 0, len(now))
	for id := range now {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var out []GUIDChange
	for _, id := range ids {
		stored, ok := was[id]
		if !ok {
			continue
		}
		written := now[id]
		if written.guid == stored.guid {
			continue
		}
		if !sameMember(stored, written) {
			continue
		}
		out = append(out, GUIDChange{
			ElementID: id,
			Type:      written.typ,
			Stored:    stored.guid,
			Written:   written.guid,
		})
	}
	return out
}

// sameMember reports whether two elements sharing an $ID are the same member, and
// so whether a GUID difference between them is a rewrite rather than the
// transplant having paired two unrelated elements.
//
// A named element is its name. The $Type is deliberately NOT compared there: the
// transplant never pairs across a $Type (pairDoc stops at the mismatch), so two
// types sharing an $ID can only be an element a writer converted in place and
// kept the $ID of — MOVE ENTITY re-typing an Association as a CrossAssociation
// (ako/mxcli#503). That is one member, and its GUID is still the database's
// identity for it.
//
// A nameless element (an index) has nothing but its $Type to go on. And a pair
// where only one side is named is not the same member: nothing converts between
// those shapes, so agreeing on it would be a guess.
func sameMember(a, b elementGUID) bool {
	switch {
	case a.name != "" && b.name != "":
		return a.name == b.name
	case a.name == "" && b.name == "":
		return a.typ == b.typ
	default:
		return false
	}
}

// elementGUID is one GUID-bearing element: its GUID, and the two properties that
// say which member it is. The name is "" for an element that has none.
type elementGUID struct {
	guid string
	typ  string
	name string
}

// ElementGUIDs maps every element's $ID to the GUID stored on it, for one
// unit's raw BSON.
//
// It exists for a writer that must reference an element by the identity the
// DATABASE uses rather than the one the file uses: a CDC service's baseline
// names the entities and members it tracks by GUID, and nothing above raw BSON
// surfaces them -- the generated domain-model types have no GUID accessor, so a
// writer that could not read them here would have to mint them, which is the
// data loss this package exists to prevent.
//
// Reading them here rather than in each writer is deliberate. This package
// already owns the reasoning about what a GUID is and what changing one costs;
// a second reader elsewhere is how the two come to disagree.
//
// Returns nil when raw does not unmarshal. An element with no GUID is absent
// from the map rather than present and empty, so a caller can tell "no GUID
// stored" from "GUID is the zero value" -- the all-zero GUID is a value Studio
// Pro writes deliberately (a VersionedProperty's AssociatedEntityGuid for an
// attribute).
func ElementGUIDs(raw []byte) map[string]string {
	src := elementGUIDs(raw)
	if src == nil {
		return nil
	}
	out := make(map[string]string, len(src))
	for id, eg := range src {
		out[id] = eg.guid
	}
	return out
}

// elementGUIDs maps element $ID -> elementGUID for every element in raw that
// carries both an $ID and a GUID.
func elementGUIDs(raw []byte) map[string]elementGUID {
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		return nil
	}
	out := map[string]elementGUID{}
	var walk func(any)
	walk = func(v any) {
		if doc, ok := asDoc(v); ok {
			if id, ok := elementID(doc); ok && hasType(doc) {
				if g, ok := binary16(doc, "GUID"); ok {
					name, _ := doc["Name"].(string)
					out[id] = elementGUID{guid: blobToUUID(g), typ: typeOf(doc), name: name}
				}
			}
			for _, k := range sortedKeys(doc) {
				walk(doc[k])
			}
			return
		}
		if s, ok := asSlice(v); ok {
			for _, e := range s {
				walk(e)
			}
		}
	}
	walk(d)
	return out
}

// StorageGUIDError returns the error a write should fail with, or nil.
// unitLabel is whatever the caller can name the unit by — an id is enough, a
// qualified name is better.
func StorageGUIDError(unitLabel string, contents, stored []byte) error {
	changes := StorageGUIDChanges(contents, stored)
	if len(changes) == 0 {
		return nil
	}
	const maxReported = 3
	msg := fmt.Sprintf("refusing to write unit %s: %d element(s) kept their $ID but would be "+
		"written with a different GUID. The runtime keys the database on that GUID "+
		"(mendixsystem$entity.id / mendixsystem$attribute.id), so deploying this would make "+
		"the synchroniser drop and recreate the affected columns and lose their data. "+
		"The model would still be valid and `mx check` would still pass",
		unitLabel, len(changes))
	for i, c := range changes {
		if i == maxReported {
			msg += fmt.Sprintf("\n  ... and %d more", len(changes)-maxReported)
			break
		}
		msg += fmt.Sprintf("\n  %s (%s): stored %s, would write %s", c.ElementID, c.Type, c.Stored, c.Written)
	}
	return fmt.Errorf("%s", msg)
}
