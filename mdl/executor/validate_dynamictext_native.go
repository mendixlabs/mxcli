// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// webOnlyRules are widget notes whose reasoning is about the web client's
// HTML, so they say nothing about a native page.
//
// The adjacent inline dynamic texts run together because each is an
// HTML <span>. On a native page each dynamic text is its own React Native Text
// element (ako/mxcli#2562). Whether native texts touch is not measured, so the
// note is dropped rather than replaced by a claim about native rendering.
var webOnlyRules = map[string]bool{consecutiveDynamicTextRule: true}

// withoutWebOnlyNotesOnNativePages drops the web-only notes raised on a page
// whose layout is native — one the script creates with a native layout type,
// or one the project holds. A page whose layout neither resolves keeps them.
func withoutWebOnlyNotesOnNativePages(vs []linter.Violation, prog *ast.Program, project func() backend.FullBackend) []linter.Violation {
	if len(vs) == 0 {
		return vs
	}
	layouts := map[string]string{}
	for _, stmt := range prog.Statements {
		if p, ok := stmt.(*ast.CreatePageStmtV3); ok && p.Layout != "" {
			layouts[strings.ToLower(p.Name.String())] = p.Layout
		}
	}
	var resolve func(string) (bool, bool)
	out := vs[:0]
	for _, v := range vs {
		if webOnlyRules[v.RuleID] && v.Location.DocumentType == "page" {
			if layout, ok := layouts[strings.ToLower(v.Location.Module+"."+v.Location.DocumentName)]; ok {
				if resolve == nil {
					resolve = layoutPlatformResolver(prog, project)
				}
				if native, known := resolve(layout); known && native {
					continue
				}
			}
		}
		out = append(out, v)
	}
	return out
}
