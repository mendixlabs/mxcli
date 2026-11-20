// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#2132 (mendixlabs/mxcli#1813): a dynamic text whose template
// parameter is a String page parameter (`{1} = $IpAddress`) was stored as
// AttributePath "$IpAddress" with no AttributeRef and no SourceVariable, so
// check and exec passed and mxbuild reported CE0402 "No value specified." at
// Text 'txtIp'. A primitive page parameter binds the way a page variable does:
// a Forms$PageVariable naming it, no attribute.
func TestTemplateParamPrimitivePageParameterBindsTheParameter(t *testing.T) {
	pb := builderWithParams("Item")
	pb.paramEntityNames = map[string]string{"Item": "M.Item"}
	pb.primitiveParams = map[string]bool{"IpAddress": true}

	param := &pages.ClientTemplateParameter{}
	pb.resolveTemplateAttributePathFull("$IpAddress", param)
	if param.SourceVariable != "IpAddress" || param.SourceVariableKind != "" || param.AttributeRef != "" || param.SourceWidget != "" {
		t.Errorf("$IpAddress bound as {SourceVariable %q, Kind %q, AttributeRef %q, Widget %q}, want the PageParameter slot naming IpAddress and no attribute",
			param.SourceVariable, param.SourceVariableKind, param.AttributeRef, param.SourceWidget)
	}

	// Control: an entity parameter's attribute is bound as before.
	param = &pages.ClientTemplateParameter{}
	pb.resolveTemplateAttributePathFull("$Item.Name", param)
	if param.SourceVariable != "Item" || param.AttributeRef != "M.Item.Name" {
		t.Errorf("$Item.Name bound as {SourceVariable %q, AttributeRef %q}, want Item / M.Item.Name", param.SourceVariable, param.AttributeRef)
	}
}

// The read half: the stored PageVariable with no attribute describes as the
// bare parameter, not `<unbound>`, so describe -> exec writes it back.
func TestDescribeTemplateParamPrimitivePageParameter(t *testing.T) {
	tpl := map[string]any{"Parameters": []any{int32(2), map[string]any{
		"$Type": "Forms$ClientTemplateParameter", "AttributeRef": nil, "Expression": "",
		"SourceVariable": map[string]any{
			"$Type": "Forms$PageVariable", "LocalVariable": "", "PageParameter": "IpAddress",
			"SnippetParameter": "", "Widget": "",
		},
	}}}
	got := extractTextTemplateParameters(nil, tpl)
	if len(got) != 1 || got[0] != "$IpAddress" {
		t.Errorf("described %v, want [$IpAddress]", got)
	}
}
