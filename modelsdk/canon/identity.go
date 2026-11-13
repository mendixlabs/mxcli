// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"bytes"
	"fmt"
	"os"

	"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Reconcile is ADR-0008 decision 1, as a single decision both storage engines
// share: given the bytes a write wants to store and the bytes already stored, it
// returns what should actually be written and whether the write can be skipped.
//
// Why a *canonical* comparison rather than comparing bytes: the rebuilt bytes
// always differ, because every sub-element in a rebuild gets a freshly random
// $ID. Byte comparison would skip nothing.
//
// Skipping is strictly safer than writing, not merely cheaper. Canonical
// equality means the two documents differ only in which IDs they picked, and the
// stored ones are the IDs every pointer inside that unit already agrees with —
// so keeping them cannot dangle a reference, whereas rewriting them is how
// PR #125 made projects unopenable. Nothing outside the unit can observe them:
// of 9,910 binary $ID pointers in a real project, 0 cross a unit boundary.
//
// Every failure path falls through to writing. A false "different" costs a
// redundant write, which is the behaviour that existed before this function; a
// false "equal" would silently discard the user's intent.
//
// Callers pass stored == nil for a unit that does not exist yet.
// Option adjusts what Reconcile carries. Variadic so every existing caller —
// the common rebuild case — is unaffected.
type Option func(*reconcileOpts)

type reconcileOpts struct {
	contentsOwnTranslations bool
	contentsOwnStorageGUIDs bool
	contentsOwnElementIDs   bool
}

// ContentsOwnElementIDs tells Reconcile that the write is a PATCH of the stored
// document, not a rebuild: every element that survived kept its stored $ID, and
// every new element carries a fresh one on purpose. So the structural transplant
// must not run.
//
// The transplant exists for rebuilds, whose elements all arrive with random
// $IDs and are paired back by type and position. On a patch that pairing is not
// a no-op but a hazard: drop one sequence flow and every flow after it pairs with
// its predecessor, taking that flow's $ID — identities move onto other nodes,
// which is what ADR-0012 measured the microflow rebuild doing (51 of 161). The
// graph splice of `alter microflow` (mfmutator) is the caller; it has its own
// guard that no reference is left dangling. Elision still applies.
func ContentsOwnElementIDs() Option {
	return func(o *reconcileOpts) { o.contentsOwnElementIDs = true }
}

// ContentsOwnTranslations tells Reconcile that the write already accounts for
// every translation in the document, so it must not carry the stored ones.
//
// The distinction is real and only the caller can make it. A REBUILD loses
// translations by accident — MDL carries one string per text, so the other
// languages were never expressible and carrying them back restores what the
// statement could not say. A targeted PATCH of the stored bytes is authoritative:
// it started from the stored translations and any that are missing are missing
// on purpose. Carrying there would undo a deliberate deletion — measured:
// `create or replace translations` reported removing 22 translations and every
// one of them was silently put back.
func ContentsOwnTranslations() Option {
	return func(o *reconcileOpts) { o.contentsOwnTranslations = true }
}

// ContentsOwnStorageGUIDs tells the write path that this write is AUTHORITATIVE
// about the storage GUIDs in the document, so the #1119 guard must not refuse it.
//
// Exactly one caller is: the marketplace module update transplants the stored
// GUIDs onto a module's replacement documents (marketplace.ApplyIdentities),
// which is the same thing Studio Pro's own update does and the reason an update
// does not destroy that module's data. That write's whole purpose is to move
// GUIDs onto elements that kept their $ID — the pattern the guard is built to
// catch — so it has to be able to say so.
//
// It is a narrow exemption on purpose. Every other write either leaves a GUID
// alone or has no business changing one, and "the guard was in the way" is not a
// reason to pass this: a write that trips the guard without being a deliberate
// identity transplant is the #1119 defect, and silencing it there would lose
// production data.
func ContentsOwnStorageGUIDs() Option {
	return func(o *reconcileOpts) { o.contentsOwnStorageGUIDs = true }
}

// OwnsStorageGUIDs reports whether these options carry the storage-GUID
// exemption. The guard itself runs at the writer's choke point, where both
// documents are in hand, rather than inside Reconcile — which returns no error —
// so the writer needs to read the option back out.
func OwnsStorageGUIDs(opts ...Option) bool {
	var o reconcileOpts
	for _, fn := range opts {
		fn(&o)
	}
	return o.contentsOwnStorageGUIDs
}

func Reconcile(contents, stored []byte, opts ...Option) (out []byte, unchanged bool) {
	if len(stored) == 0 {
		return contents, false
	}
	var o reconcileOpts
	for _, fn := range opts {
		fn(&o)
	}

	// Identity is carried unconditionally. MXCLI_ALWAYS_WRITE turns off *eliding*
	// the write, not preserving what the document is: a forced write that
	// re-minted StableId would renumber the deployed model's operation ids, which
	// is a change to the app rather than a debugging aid.
	contents = CarryIdentity(contents, stored)

	// The translations the statement had no way to say. MDL carries ONE string
	// per text, so a rebuild drops every other language the document held — a
	// describe→exec round-trip deleted a page's Dutch title from a real project.
	// Runs before the transplant because it is the one carry that changes the
	// document's length; the transplant patches fixed-width binaries in place and
	// must see the final framing.
	if !o.contentsOwnTranslations {
		contents = CarryTranslations(contents, stored)
	}

	// The same reasoning one level down, for the writes that do land: a rebuild
	// mints a fresh $ID for every sub-element, so a two-line change reads in
	// version control as a whole-document replacement. TransplantIDs puts the
	// stored ids back on the elements that still correspond, rewriting every
	// reference with them.
	//
	// The stored document is parsed once here for the two steps that only read
	// it, this pairing and the comparison at the end (parsedStored). The carries
	// in between copy parts of the stored document into the new one and keep a
	// parse of their own.
	storedDoc := parseStored(stored)
	if !o.contentsOwnElementIDs {
		contents = transplantIDs(contents, storedDoc)
	}

	// And the nested identity property the transplant does not cover: every
	// Workflows$* element carries a PersistentId that both engines re-mint on
	// every write, so without this a workflow document never equals itself and
	// no-op elision could never fire for one (issue #949).
	contents = CarryPersistentIDs(contents, stored)

	// A property MDL cannot state: a tab container's default tab. The writer
	// points it at the first tab, so a round trip rewrote a page whose stored
	// default was null and reset any other choice (ako/mxcli#1897). Needs the
	// transplanted $IDs, so it runs after TransplantIDs.
	contents = CarryDefaultTabPages(contents, stored)

	// The rest of what MDL cannot state: an expression's surrounding whitespace
	// and an empty screen-reader label Studio Pro stores as a ClientTemplate
	// (ako/mxcli#1897).
	contents = CarryUnexpressed(contents, stored)

	if alwaysWrite() {
		return contents, false
	}
	eq, err := equalToStored(contents, storedDoc)
	if err != nil {
		return contents, false
	}
	return contents, eq
}

// reconcileInPasses is Reconcile as it was before its read-only steps shared a
// parse of the stored document: every step on the bytes alone. It is what
// Reconcile is defined by and what its test compares it with.
func reconcileInPasses(contents, stored []byte, opts ...Option) (out []byte, unchanged bool) {
	if len(stored) == 0 {
		return contents, false
	}
	var o reconcileOpts
	for _, fn := range opts {
		fn(&o)
	}

	// Identity is carried unconditionally. MXCLI_ALWAYS_WRITE turns off *eliding*
	// the write, not preserving what the document is: a forced write that
	// re-minted StableId would renumber the deployed model's operation ids, which
	// is a change to the app rather than a debugging aid.
	contents = CarryIdentity(contents, stored)

	// The translations the statement had no way to say. MDL carries ONE string
	// per text, so a rebuild drops every other language the document held — a
	// describe→exec round-trip deleted a page's Dutch title from a real project.
	// Runs before the transplant because it is the one carry that changes the
	// document's length; the transplant patches fixed-width binaries in place and
	// must see the final framing.
	if !o.contentsOwnTranslations {
		contents = CarryTranslations(contents, stored)
	}

	// The same reasoning one level down, for the writes that do land: a rebuild
	// mints a fresh $ID for every sub-element, so a two-line change reads in
	// version control as a whole-document replacement. TransplantIDs puts the
	// stored ids back on the elements that still correspond, rewriting every
	// reference with them.
	if !o.contentsOwnElementIDs {
		contents = transplantIDsInPasses(contents, stored)
	}

	// And the nested identity property the transplant does not cover: every
	// Workflows$* element carries a PersistentId that both engines re-mint on
	// every write, so without this a workflow document never equals itself and
	// no-op elision could never fire for one (issue #949).
	contents = carryPersistentIDsUnfiltered(contents, stored)

	// A property MDL cannot state: a tab container's default tab. The writer
	// points it at the first tab, so a round trip rewrote a page whose stored
	// default was null and reset any other choice (ako/mxcli#1897). Needs the
	// transplanted $IDs, so it runs after TransplantIDs.
	contents = CarryDefaultTabPages(contents, stored)

	// The rest of what MDL cannot state: an expression's surrounding whitespace
	// and an empty screen-reader label Studio Pro stores as a ClientTemplate
	// (ako/mxcli#1897).
	contents = CarryUnexpressed(contents, stored)

	if alwaysWrite() {
		return contents, false
	}
	eq, err := Equal(contents, stored)
	if err != nil {
		return contents, false
	}
	return contents, eq
}

// parsedStored is the stored document parsed for the steps of Reconcile that
// only read it. ok is false for bytes that are not a document, which those
// steps treat as they treat a failed parse of their own.
type parsedStored struct {
	doc bson.D
	ok  bool
}

func parseStored(stored []byte) parsedStored {
	var doc bson.D
	if err := bson.Unmarshal(stored, &doc); err != nil {
		return parsedStored{}
	}
	return parsedStored{doc: doc, ok: true}
}

// equalToStored is Equal(contents, stored) with the stored side already parsed.
func equalToStored(contents []byte, stored parsedStored) (bool, error) {
	da, err := Digest(contents)
	if err != nil {
		return false, fmt.Errorf("canonicalise left: %w", err)
	}
	if !stored.ok {
		return false, fmt.Errorf("canonicalise right: not a document")
	}
	return da == digestOfDoc(stored.doc, nil), nil
}

// alwaysWrite disables no-op elision. An escape hatch for bisecting a suspected
// elision bug ("does it still reproduce if every write lands?") without building
// a patched binary — not a supported knob.
//
// Read per call rather than cached at init so a test can toggle it: the control
// that proves elision is what stopped the churn has to be able to turn elision
// off in-process.
func alwaysWrite() bool { return os.Getenv("MXCLI_ALWAYS_WRITE") != "" }

// identityFields lists, per document $Type, the top-level properties that carry
// *identity* rather than content: values a rebuild has no business re-minting,
// because something outside this document remembers them.
//
// Today that is exactly one field. Microflows$Microflow.StableId is declared by
// Mendix as ModelPropertyAttribute("StableId", RetentionType.DesignTime) with
// IsIdentifier = true. It is seeded once by a one-time conversion, transplanted
// across a marketplace module update by PackageUtils.RescueStableIDs, and — the
// part that makes it load-bearing rather than decorative — the build derives
// every client-callable microflow's operation id from it:
//
//	operationId == base64(uuid5(projectId, StableId).bytes_le)
//
// verified against deployment/model/operations.json. Regenerating it on every
// write renames those operations. See ADR-0008, "What StableId is".
//
// Adding a row here is a claim that Mendix treats the property as an identity,
// which should be established the same way rather than assumed.
var identityFields = map[string][]string{
	"Microflows$Microflow": {"StableId"},
}

// IdentityFields returns the identity properties known for a $Type, or nil.
//
// This table is hand-maintained and cannot currently be generated: Mendix's
// `IsIdentifier` flag lives only in the modeler assemblies, not in the reflection
// data `generated/metamodel` is built from. A new document type with an identity
// property therefore needs a row here, and nothing about adding that type will
// remind you — which is why the backend package carries a drift guard that fails
// when a property is minted fresh on every write without a decision recorded here.
func IdentityFields(typeName string) []string {
	return identityFields[typeName]
}

// CarryIdentity returns contents with each identity field of its $Type
// replaced by the value the stored document already holds.
//
// Two rules from the overlay-write guidance (CLAUDE.md, ADR-0005) apply and are
// both enforced by the shape of the patch:
//
//   - Only a key the document *already carries* is written. If either side is
//     missing the field, or the two disagree about its type or width, nothing is
//     carried. An absent optional property is filled in on load; a wrongly-typed
//     one makes the document unopenable in Studio Pro.
//   - The dispatch is on the stored $Type, not on what the caller believes it is
//     writing.
//
// The patch is byte-for-byte in place: a BSON binary value is fixed-width, so
// overwriting the payload of an equal-length binary cannot disturb any length
// prefix. Nothing is re-marshalled, so a document the codec produced reaches
// storage exactly as the codec produced it apart from these bytes.
func CarryIdentity(contents, stored []byte) []byte {
	typeName, ok := topLevelString(stored, "$Type")
	if !ok {
		return contents
	}
	fields := identityFields[typeName]
	if len(fields) == 0 {
		return contents
	}

	out := contents
	patched := false
	for _, field := range fields {
		want, ok := topLevelBinary(stored, field)
		if !ok {
			continue
		}
		if !patched {
			// Copy lazily: the overwhelming majority of writes are of types with
			// no identity fields, and those must not pay for a copy.
			out = append([]byte(nil), contents...)
			patched = true
		}
		if !overwriteTopLevelBinary(out, field, want) {
			// The new document lacks the field, or its shape differs. Leave it
			// alone rather than inventing or reshaping a stored property.
			continue
		}
	}
	return out
}

// topLevelString reads a top-level string property.
func topLevelString(raw []byte, key string) (string, bool) {
	v, err := bsoncore.Document(raw).LookupErr(key)
	if err != nil {
		return "", false
	}
	s, ok := v.StringValueOK()
	return s, ok
}

// topLevelBinary reads a top-level binary property, returning its subtype and
// payload.
func topLevelBinary(raw []byte, key string) (binaryValue, bool) {
	v, err := bsoncore.Document(raw).LookupErr(key)
	if err != nil {
		return binaryValue{}, false
	}
	subtype, data, ok := v.BinaryOK()
	if !ok {
		return binaryValue{}, false
	}
	return binaryValue{subtype: subtype, data: data}, true
}

type binaryValue struct {
	subtype byte
	data    []byte
}

// overwriteTopLevelBinary replaces the payload of a top-level binary property in
// place, and reports whether it did. It refuses unless the target is a binary of
// the same subtype and the same length, so the document's framing is untouched.
//
// The patch relies on the looked-up value aliasing raw rather than copying out
// of it, which is a property of the BSON library and not of this package. Rather
// than assume it, the write is read back: if the library ever stops aliasing,
// this reports false and the caller carries nothing — a lost preservation, not a
// silently corrupt document.
func overwriteTopLevelBinary(raw []byte, key string, want binaryValue) bool {
	v, err := bsoncore.Document(raw).LookupErr(key)
	if err != nil {
		return false
	}
	subtype, data, ok := v.BinaryOK()
	if !ok || subtype != want.subtype || len(data) != len(want.data) {
		return false
	}
	copy(data, want.data)

	got, ok := topLevelBinary(raw, key)
	return ok && got.subtype == want.subtype && bytes.Equal(got.data, want.data)
}
