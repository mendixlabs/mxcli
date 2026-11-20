// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"slices"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// validateShowPageParams checks that each argument of a show page names a
// parameter of the page. One that does not is CE1613 "The selected page
// parameter 'M.P.X' no longer exists" (mxbuild 11.12.5), and exec writes it as
// given. The reported case was `show page M.P (NumberOfPagesToClose: 1)`,
// which reads as an argument for a page parameter of that name rather than as
// the action's setting (mendixlabs/mxcli#1617). A page that does not resolve is
// the not-found check's to report.
func validateShowPageParams(ctx *ExecContext, calls []codeActionCallRef, sc *scriptContext) []string {
	var stored map[string][]string
	var errs []string
	for _, call := range calls {
		key := strings.ToLower(call.name)
		declared, ok := sc.pageParamNames[key]
		if !ok {
			if stored == nil {
				stored = buildPageParamNames(ctx)
			}
			declared, ok = stored[key]
		}
		if !ok {
			continue
		}
		declared = applyPageParamEdits(declared, sc.pageParamEdits[key])
		for _, msg := range validateCodeActionParams("page", call, declared) {
			for _, written := range call.argNames {
				if strings.EqualFold(written, "NumberOfPagesToClose") && strings.Contains(msg, `"`+written+`"`) {
					msg += " NumberOfPagesToClose is the action's setting, not a page parameter: " +
						"write `show page " + call.name + " with NumberOfPagesToClose = 1`."
				}
			}
			errs = append(errs, msg)
		}
	}
	return errs
}

// showPageArgNames is the parameter names a show page's arguments name.
func showPageArgNames(args []ast.ShowPageArg) []string {
	var names []string
	for _, a := range args {
		if a.ParamName != "" {
			names = append(names, a.ParamName)
		}
	}
	return names
}

// buildPageParamNames maps each stored page (lower-cased qualified name) to
// its parameter names. A page with no parameters is present with an empty,
// non-nil list: known to take none, so any argument is the CE1613.
func buildPageParamNames(ctx *ExecContext) map[string][]string {
	out := map[string][]string{}
	h, err := getHierarchy(ctx)
	if err != nil || h == nil {
		return out
	}
	all, err := projectPages(ctx)
	if err != nil {
		return out
	}
	for _, p := range liveFirst(all, pageExcluded) {
		if p == nil {
			continue
		}
		key := strings.ToLower(h.GetQualifiedName(p.ContainerID, p.Name))
		if _, seen := out[key]; seen {
			continue // an excluded twin of a live page (#1928)
		}
		names := []string{}
		for _, param := range p.Parameters {
			if param != nil {
				names = append(names, param.Name)
			}
		}
		out[key] = names
	}
	return out
}

// recordPageParamNames stores a script-declared page's parameter names, for
// the show page argument check: the page and the flow opening it are usually
// one script.
func (sc *scriptContext) recordPageParamNames(qualifiedName string, params []ast.PageParameter) {
	names := make([]string, 0, len(params))
	for _, p := range params {
		names = append(names, p.Name)
	}
	sc.pageParamNames[strings.ToLower(qualifiedName)] = names
}

// recordPageParamEdits keeps an alter page's add/drop parameter operations, so
// a show page later in the script is checked against the page as the script
// leaves it, not as it is stored (ako/mxcli#1940): adding a parameter to a page
// and passing it from the flow that opens the page is one change.
func (sc *scriptContext) recordPageParamEdits(qualifiedName string, ops []ast.AlterPageOperation) {
	key := strings.ToLower(qualifiedName)
	for _, op := range ops {
		switch op.(type) {
		case *ast.AddParameterOp, *ast.DropParameterOp:
			sc.pageParamEdits[key] = append(sc.pageParamEdits[key], op)
		}
	}
}

// applyPageParamEdits returns declared with the edits applied in order. It
// never changes declared, which may be the stored page's list.
func applyPageParamEdits(declared []string, edits []ast.AlterPageOperation) []string {
	if len(edits) == 0 {
		return declared
	}
	out := append([]string{}, declared...)
	for _, op := range edits {
		switch o := op.(type) {
		case *ast.AddParameterOp:
			if !slices.Contains(out, o.Parameter.Name) {
				out = append(out, o.Parameter.Name)
			}
		case *ast.DropParameterOp:
			out = slices.DeleteFunc(out, func(n string) bool { return n == o.ParameterName })
		}
	}
	return out
}
