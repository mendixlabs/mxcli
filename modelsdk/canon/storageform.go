// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"sync/atomic"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/version"
)

// storageFormParses counts the times CompleteForStorage, or the passes it
// stands for, parsed a whole document. For tests.
var storageFormParses atomic.Int64

// StorageFormParses is how many whole-document parses the storage-form passes
// have made in this process. For tests.
func StorageFormParses() int64 { return storageFormParses.Load() }

// parseForStorageForm is bson.Unmarshal into a document, counted.
func parseForStorageForm(contents []byte, doc *bson.D) error {
	storageFormParses.Add(1)
	return bson.Unmarshal(contents, doc)
}

// CompleteForStorage gives a unit the form it is stored in, for a project of
// version pv: StripUndeclaredProperties, CompletePropertySets,
// FillMissingUserRoleGUIDs and — when listMarkers is set — NormalizeListMarkers,
// then UndeclaredTypeError on the result. It returns exactly what those return
// when called one after the other, and contents itself when nothing changes.
//
// Called one after the other, each parses the whole document and each that
// changes it encodes it again: up to four parses and four encodings for every
// unit written, and for every statement of a run that holds its writes, which
// normalises the domain model it rewrites to compare it (ako/mxcli#2160). The
// passes do not need the bytes in between. Each works on the parsed document,
// so this parses once and encodes once, and only when something changed.
//
// pv must not be nil; the caller that has no version has nothing to complete.
func CompleteForStorage(contents []byte, pv *version.Version, listMarkers bool) ([]byte, error) {
	if pv == nil || len(contents) == 0 {
		return completeForStorageInPasses(contents, pv, listMarkers)
	}
	var doc bson.D
	if err := parseForStorageForm(contents, &doc); err != nil {
		return completeForStorageInPasses(contents, pv, listMarkers)
	}
	s := &stripper{pv: *pv}
	changed := s.doc(&doc)
	if len(s.refused) > 0 {
		return nil, s.err()
	}
	if defs := propertyDefaults(); len(defs) > 0 {
		if completeValue(&doc, defs, *pv, elementIDs(doc)) {
			changed = true
		}
	}
	if fillUserRoleGUIDs(&doc) {
		changed = true
	}
	// The type floor reads $Type values, which the list markers do not touch,
	// so it is judged on the bytes after them: the same verdict as before them.
	if listMarkers && len(propertyListMarkers) > 0 && normalizeListMarkersIn(doc) {
		changed = true
	}
	out := contents
	if changed {
		encoded, err := bson.Marshal(doc)
		if err != nil {
			return completeForStorageInPasses(contents, pv, listMarkers)
		}
		out = encoded
	}
	if err := UndeclaredTypeError(out, pv); err != nil {
		return nil, err
	}
	return out, nil
}

// completeForStorageInPasses is CompleteForStorage as it was before the passes
// shared a parse: each pass on the bytes the one before it returned. It is what
// CompleteForStorage is defined by, what it falls back to for a document it
// cannot parse or encode, and what its test compares it with.
func completeForStorageInPasses(contents []byte, pv *version.Version, listMarkers bool) ([]byte, error) {
	stripped, err := StripUndeclaredProperties(contents, pv)
	if err != nil {
		return nil, err
	}
	out := FillMissingUserRoleGUIDs(CompletePropertySets(stripped, pv))
	if err := UndeclaredTypeError(out, pv); err != nil {
		return nil, err
	}
	if listMarkers {
		out = NormalizeListMarkers(out)
	}
	return out, nil
}
