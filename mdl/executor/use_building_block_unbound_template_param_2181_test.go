// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// rawTemplateParam is a stored Forms$ClientTemplateParameter. Studio Pro stores
// a parameter no one has bound yet with all three binding slots empty:
// AttributeRef null, Expression "", SourceVariable null.
func rawTemplateParam(expr string) map[string]any {
	return map[string]any{
		"$Type": "Forms$ClientTemplateParameter", "AttributeRef": nil,
		"Expression": expr, "SourceVariable": nil,
	}
}

// rawDynamicTextWithParams is a dynamic text over `{1} · {2}`: the first
// parameter unbound, the second the literal expression 'x'.
func rawDynamicTextWithParams() map[string]any {
	return map[string]any{
		"$Type": "Forms$DynamicText", "Name": "galleryTitle",
		"Content": map[string]any{"$Type": "Forms$ClientTemplate",
			"Template": map[string]any{"$Type": "Texts$Text", "Items": []any{int32(3),
				map[string]any{"$Type": "Texts$Translation", "LanguageCode": "en_US", "Text": "{1} · {2}"}}},
			"Parameters": []any{int32(2), rawTemplateParam(""), rawTemplateParam("'x'")},
		},
	}
}

// TestBuildingBlockWithUnboundTemplateParamExpands is ako/mxcli#2181
// (mendixlabs/mxcli#1834): `use building block PlatformDesignSystem.Gallery_List_Divider`
// failed with "could not expand widget tree: extraneous input '<'". A building
// block is a template, so its text-template parameters are usually unbound;
// the describer prints one as `{1} = <unbound>` and the expansion parses that
// text back. The block must be placed with the parameter still unbound.
func TestBuildingBlockWithUnboundTemplateParamExpands(t *testing.T) {
	raw := map[string]any{"Widgets": []any{rawDynamicTextWithParams()}}
	pb, _ := pdsBlockCtx(t, raw)

	got, err := pb.expandIfFragment(&ast.WidgetV3{
		Type: "USE_BUILDING_BLOCK", Name: "PlatformDesignSystem.Button",
		Properties: map[string]any{},
	})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	txt := findWidget(got, "galleryTitle")
	if txt == nil {
		t.Fatalf("dynamic text not expanded: %+v", got)
	}
	params := txt.GetContentParams()
	if len(params) != 2 {
		t.Fatalf("ContentParams = %+v, want two parameters", params)
	}
	if !params[0].Unbound || params[0].Value != nil {
		t.Errorf("{1} = %+v, want an unbound parameter", params[0])
	}
	// Control: the bound parameter beside it is read as before.
	if params[1].Unbound || params[1].Value != "'x'" {
		t.Errorf("{2} = %+v, want the expression 'x'", params[1])
	}

	// Written back: the unbound parameter stays a parameter, in its position,
	// with every binding slot empty — what Studio Pro stores for it — so `{1}`
	// does not shift onto the next binding.
	written := pb.buildClientTemplateParams(params)
	if len(written) != 2 {
		t.Fatalf("wrote %d parameters, want 2", len(written))
	}
	if p := written[0]; p.Expression != "" || p.AttributeRef != "" || p.SourceVariable != "" || p.SourceWidget != "" {
		t.Errorf("unbound parameter written with a binding: %+v", p)
	}
	if p := written[1]; p.Expression != "'x'" {
		t.Errorf("bound parameter written as %+v, want Expression 'x'", p)
	}
}

// TestDescribeUnboundTemplateParamReparses: `describe page` prints the same
// spelling for a page whose dynamic text has an unbound parameter, and
// describe -> exec is the invariant — the printed text must parse back into
// the same parameter list.
func TestDescribeUnboundTemplateParamReparses(t *testing.T) {
	described := describeWidget(t, rawDynamicTextWithParams())
	if !strings.Contains(described, "{1} = <unbound>") || !strings.Contains(described, "{2} = 'x'") {
		t.Fatalf("describe output lacks the parameters:\n%s", described)
	}
	prog, errs := visitor.Build("create page M.P (Title: 'x', Layout: A.L) {\n" + described + "}\n")
	if len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs, described)
	}
	stmt := prog.Statements[0].(*ast.CreatePageStmtV3)
	params := findWidget(stmt.Widgets, "galleryTitle").GetContentParams()
	if len(params) != 2 || !params[0].Unbound || params[1].Unbound || params[1].Value != "'x'" {
		t.Errorf("reparsed parameters = %+v, want [unbound, 'x']", params)
	}
}

// TestUnboundTemplateParamOnPluggableCompanion: the reported lines are a
// pluggable widget's `<Name>Params` companion (`ImageUrlParams: ({1} = <unbound>)`)
// with a format block possibly following, which go through the same rule.
func TestUnboundTemplateParamOnPluggableCompanion(t *testing.T) {
	src := "create page M.P (Title: 'x', Layout: A.L) {\n" +
		"  image img1 (ImageUrl: '{1}', ImageUrlParams: ({1} = <unbound>))\n" +
		"  dynamictext t1 (Content: '{1} {2}', ContentParams: ({1} = <unbound> format (decimalPrecision: 0), {2} = <unbound>))\n" +
		"}\n"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("does not parse: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreatePageStmtV3)
	img := findWidget(stmt.Widgets, "img1")
	if p, ok := img.Properties["ImageUrlParams"].([]ast.ParamAssignmentV3); !ok || len(p) != 1 || !p[0].Unbound {
		t.Errorf("ImageUrlParams = %#v, want one unbound parameter", img.Properties["ImageUrlParams"])
	}
	params := findWidget(stmt.Widgets, "t1").GetContentParams()
	if len(params) != 2 || !params[0].Unbound || params[0].Format == nil || !params[1].Unbound {
		t.Errorf("ContentParams = %+v, want two unbound parameters, the first formatted", params)
	}

	// Only the placeholder word `unbound` is the spelling; anything else in
	// angle brackets is still refused rather than read as unbound.
	if _, errs := visitor.Build("create page M.P (Title: 'x', Layout: A.L) {\n" +
		"  dynamictext t1 (Content: '{1}', ContentParams: ({1} = <bound>))\n}\n"); len(errs) == 0 {
		t.Error("`<bound>` parsed; only `<unbound>` names an unbound parameter")
	}
}
