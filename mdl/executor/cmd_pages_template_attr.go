// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// templateAttrBinding is the #823 change to a text-template parameter bound to
// a non-String attribute of a parameter or data view, `{1} = $Order.Total`
// (ako/mxcli#721 L3; decided as option C on ako/mxcli#714).
//
// mxcli used to store it as the Expression `toString($Order/Total)`. It is now
// stored as Studio Pro stores it: an attribute reference, which the runtime
// renders through the parameter's formatting (a Decimal's precision and digit
// grouping, a date's format, an enumeration's caption). That applies under
// every language version: the old write was not what Studio Pro stores, and a
// snippet's copy of it failed mx check. An mdl 0 script whose page now renders
// differently gets a warning naming the expression that keeps the old output,
// so the change is never silent.
//
// It is a langver.Change so the code is registered with the other language
// changes, but it gates only the warning: under mdl 1 there is nothing to
// report, because the reference is the language's meaning.
var templateAttrBinding = langver.Change{
	Code:  "MDL-V1-TEMPLATEATTR",
	Since: langver.V1,
	Old: "a text-template parameter bound to a non-String attribute (`{1} = $Order.Total`) was stored as " +
		"`toString($Order/Total)` by earlier mxcli releases",
	New: "an attribute reference, rendered with the attribute's formatting, as Studio Pro stores it",
}

// warnTemplateAttrBinding reports, under mdl 0, a `$name.Attr` template
// parameter that earlier releases wrapped in toString(): name is a parameter or
// data container whose entity is known, and Attr's type is known and not String.
// An attribute whose type cannot be found was not wrapped (the old check failed
// open), so it is not reported.
func (pb *pageBuilder) warnTemplateAttrBinding(name, attr string) {
	if pb.ctx == nil || templateAttrBinding.Applies(pb.ctx.LanguageVersion) {
		return
	}
	entity, ok := pb.paramEntityNames[name]
	if !ok {
		entity, ok = pb.paramEntityNames["$"+name]
	}
	if !ok {
		return
	}
	attrType := pb.findAttributeType(entity + "." + attr)
	if attrType == nil {
		return
	}
	if _, isString := attrType.(*domainmodel.StringAttributeType); isString {
		return
	}
	fmt.Fprintf(pb.ctx.progress(),
		"Warning [%s]: text-template parameter $%s.%s is bound to the attribute: it renders with the "+
			"attribute's formatting, as in Studio Pro. mxcli releases before #823 stored toString($%s/%s); "+
			"write `{n} = toString($%s/%s)` to keep that output. %s\n",
		templateAttrBinding.Code, name, attr, name, attr, name, attr, langver.HelpHint(templateAttrBinding.Code))
}

// templateAttributeRefRe matches what a text-template parameter binds as an
// attribute: `$Param.Attr`, `$localVar`, `Attr`, `Module.Entity.Attr`,
// `Assoc/Attr`, `$currentObject/Module.Assoc/Attr`. Anything else — a call, an
// operator, a number, a literal — is an expression.
var templateAttributeRefRe = regexp.MustCompile(`^\$?[A-Za-z_][A-Za-z0-9_]*(?:[./][A-Za-z_][A-Za-z0-9_]*)*$`)

// isTemplateExpression reports whether a text-template parameter value is an
// expression the parameter stores as written, rather than an attribute to
// bind. `{1} = toString($Order/Total)` used to be resolved as an attribute
// path and stored as a bogus attribute name.
func isTemplateExpression(v string) bool {
	v = strings.TrimSpace(v)
	return strings.HasPrefix(v, "'") || strings.HasPrefix(v, "\"") || !templateAttributeRefRe.MatchString(v) ||
		isVariableRootedTemplatePath(v)
}

// isTemplateExpression is isTemplateExpression for the document being built:
// a `$Root/Assoc/…/Attr` path that resolves from one of its parameters or an
// enclosing data view is a binding, as `$currentObject/Assoc/Attr` is. Studio
// Pro stores it as the steps beside a SourceVariable naming the root
// (TestApp WorkflowCommons.Snip_Workflow_Header), and the expression form of
// the same path is mx check CE0117 "Error(s) in expression." (ako/mxcli#833).
// Anything else is judged as before, so `$Customer/Name` stays the expression
// Studio Pro stores (mendixlabs/mxcli#1406).
func (pb *pageBuilder) isTemplateExpression(v string) bool {
	if _, _, _, ok := pb.resolveRootedAssociationPath(strings.TrimSpace(v)); ok {
		return false
	}
	return isTemplateExpression(v)
}

// isVariableRootedTemplatePath reports a `$var/…` member path, which is Mendix
// expression syntax rather than one of MDL's binding spellings: `$Customer/Name`,
// `$currentObject/Name`. Studio Pro stores such a parameter as an Expression, and
// describe prints it back as written (mendixlabs/mxcli#1406), so it is stored as
// the expression it is. Bound instead, the writer kept the whole string as the
// attribute name — CE1613 "The selected attribute 'M.E.$Customer/Name' no longer
// exists" (#1046).
//
// `$currentObject/Assoc/Attr` is the one exception, and stays a binding: the
// association resolver strips that prefix (resolveAssociationAttributePath), and
// #1046 measured it building clean as an attribute over the association.
func isVariableRootedTemplatePath(v string) bool {
	if !strings.HasPrefix(v, "$") {
		return false
	}
	slash := strings.Index(v, "/")
	if slash < 0 || slash == len(v)-1 {
		return false
	}
	variable, rest := v[1:slash], v[slash+1:]
	return variable != "currentObject" || !strings.Contains(rest, "/")
}
