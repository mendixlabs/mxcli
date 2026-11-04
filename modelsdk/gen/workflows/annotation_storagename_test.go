// SPDX-License-Identifier: Apache-2.0

package workflows

import (
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
)

// ako/mxcli#1784: the generated constructor named a workflow annotation
// Workflows$AnnotationActivity, the writer overrode it to Workflows$Annotation,
// and the decoder registered both. Measured, one workflow with a start and an
// activity annotation per project: Workflows$Annotation is 0 errors on
// mxbuild 9.24.30 and 11.12.6, and Workflows$AnnotationActivity is
// TypeCacheUnknownTypeException on both — no project can store it. So the
// constructor must produce the name Mendix loads, and the unknown name must not
// be decodable as if it were one.
func TestAnnotationStorageName(t *testing.T) {
	if got := NewAnnotation().TypeName(); got != "Workflows$Annotation" {
		t.Errorf("NewAnnotation().TypeName() = %q, want Workflows$Annotation (AnnotationActivity is TypeCacheUnknownTypeException)", got)
	}
	if _, ok := codec.DefaultRegistry.Lookup("Workflows$Annotation"); !ok {
		t.Error("Workflows$Annotation must decode")
	}
	if _, ok := codec.DefaultRegistry.Lookup("Workflows$AnnotationActivity"); ok {
		t.Error("Workflows$AnnotationActivity is not a Mendix storage name; registering it hides a wrong write")
	}
}
