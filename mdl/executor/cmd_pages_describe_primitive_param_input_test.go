// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// An input bound directly to a primitive parameter stores no AttributeRef and
// names the parameter in its SourceVariable — a snippet's in SnippetParameter
// (Studio Pro 11.17, ako/TestApp Snippet.SnippetPrimitiveParameter:
// radioButtons1 → SnippetParameter "par1"), a page's in PageParameter. describe
// printed it with no Attribute at all, so describe → exec wrote an input bound
// to nothing. It prints as `$par1`, the spelling the builder takes back.
func TestExtractInputAttribute_PrimitiveParameterBinding(t *testing.T) {
	ctx, _ := newMockCtx(t)
	sv := func(slot, name string) map[string]any {
		m := map[string]any{"$Type": "Forms$PageVariable", "LocalVariable": "", "PageParameter": "",
			"SnippetParameter": "", "SubKey": "", "UseAllPages": false, "Widget": ""}
		m[slot] = name
		return m
	}
	for _, c := range []struct {
		name string
		w    map[string]any
		want string
	}{
		{"snippet parameter", map[string]any{"AttributeRef": nil, "SourceVariable": sv("SnippetParameter", "par1")}, "$par1"},
		{"page parameter", map[string]any{"AttributeRef": nil, "SourceVariable": sv("PageParameter", "h2")}, "$h2"},
		// Control: the page-variable form that already worked (#1235).
		{"page variable", map[string]any{"AttributeRef": nil, "SourceVariable": sv("LocalVariable", "ShowAll")}, "$ShowAll"},
		// Control: an attribute of an entity parameter keeps `$Param.Attr`.
		{"entity parameter attribute", map[string]any{
			"AttributeRef":   map[string]any{"Attribute": "M.Order.Number"},
			"SourceVariable": sv("PageParameter", "Order"),
		}, "$Order.Number"},
	} {
		if got := extractInputAttribute(ctx, c.w); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
