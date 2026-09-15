// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

// FindCustomWidgetType was the last method keeping cmd/mxcli on sdk/mpr: it was
// generated into unimplemented_gen.go, so `mxcli extract-templates` reached the
// stored widget only by holding a concrete *sdk/mpr.Reader
// (docs/plans/2026-09-14-retire-legacy-engine.md, Phase 4a).
//
// The implementation was never missing — modelsdk/mpr.Reader has had it, with
// FindAllCustomWidgetTypes and the collectCustomWidgets walker behind it. Only
// the wiring was absent, which is why the error it used to return said "this
// should be unreachable": nothing on the backend path could reach it.

import (
	bsonv2 "go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// FindCustomWidgetType returns the stored Type and Object of the first
// CustomWidget carrying widgetID, or nil with no error when the project has
// none — the not-found contract both readers already had, which callers branch
// on rather than treating as failure.
//
// THE CONVERSION IS THE POINT. types.RawCustomWidgetType documents RawType and
// RawObject as "bson.D in sdk/mpr", and every caller asserts that — but
// modelsdk/mpr builds them with the **v2** driver, so a straight delegation
// hands back a bson.D from the other BSON package. The two are unrelated types
// with the same name, which makes the failure read
//
//	widget type is bson.D, want bson.D
//
// and a cast written to silence that error would panic instead. Converting here
// keeps one currency at the backend boundary, as widget_pluggable_write.go
// already does on the write side.
func (b *Backend) FindCustomWidgetType(widgetID string) (*types.RawCustomWidgetType, error) {
	w, err := b.reader.FindCustomWidgetType(widgetID)
	if err != nil || w == nil {
		return nil, err
	}
	out := *w
	if d, ok := w.RawType.(bsonv2.D); ok {
		out.RawType = v2ToV1BSON(d)
	}
	if d, ok := w.RawObject.(bsonv2.D); ok {
		out.RawObject = v2ToV1BSON(d)
	}
	return &out, nil
}
