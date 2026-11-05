// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1419: an image collection created earlier in the same script
// was invisible to the reference pre-check, so
//
//	create image collection ModA.Icons (image pix from file '…');
//	create page … { image img1 (Image: 'ModA.Icons.pix') }
//
// was refused by check with
//
//	image collection not found: ModA.Icons (referenced as ModA.Icons.pix)
//
// and by exec with "Refusing to execute: 1 unresolved reference(s) above".
// Split into two scripts, the same statements passed.

func scriptWithImageCollection(module, name string, images ...string) *scriptContext {
	stmt := &ast.CreateImageCollectionStmt{Name: ast.QualifiedName{Module: module, Name: name}}
	for _, img := range images {
		stmt.Images = append(stmt.Images, ast.ImageItem{Name: img})
	}
	sc := newScriptContext()
	sc.collectSingle(stmt)
	return sc
}

func TestWidgetRefs_ImageInScriptCreatedCollectionIsResolved(t *testing.T) {
	ctx := imageCollectionCtx(t)
	sc := scriptWithImageCollection("ModA", "Icons", "pix")

	errs := validateWidgetReferences(ctx, imageRefWidget("image", "Image", "ModA.Icons.pix"), sc)
	if len(errs) != 0 {
		t.Fatalf("an image in a collection the script creates was refused: %v", errs)
	}
	// Mendix resolves an image reference case-sensitively (a case difference
	// is CE1613, measured on 11.13.0), so a case difference against the
	// script's own spelling is refused, naming that spelling.
	errs = validateWidgetReferences(ctx, imageRefWidget("staticimage", "Image", "moda.ICONS.Pix"), sc)
	if len(errs) != 1 || !strings.Contains(errs[0], "ModA.Icons") || !strings.Contains(errs[0], "case-sensitive") {
		t.Fatalf("a case difference against a script-created image: want one refusal naming ModA.Icons, got %v", errs)
	}
	// The project's own collections still resolve alongside the script's.
	errs = validateWidgetReferences(ctx, imageRefWidget("image", "Image", "MyFirstModule.Images.gallery"), sc)
	if len(errs) != 0 {
		t.Fatalf("a project image was refused once the script defined a collection: %v", errs)
	}
}

// CONTROL: the script's collection is a real answer, not an exemption — an
// image it does not declare is still reported.
func TestWidgetRefs_MissingImageInScriptCreatedCollectionIsReported(t *testing.T) {
	ctx := imageCollectionCtx(t)
	sc := scriptWithImageCollection("ModA", "Icons", "pix")

	errs := validateWidgetReferences(ctx, imageRefWidget("image", "Image", "ModA.Icons.nope"), sc)
	if len(errs) != 1 || !strings.Contains(errs[0], "image not found: nope") {
		t.Fatalf("an image the script's collection does not hold was accepted; got %v", errs)
	}
}
