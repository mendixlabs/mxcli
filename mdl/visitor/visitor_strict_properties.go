// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/suggest"
)

// ADR-0010 R11: an unknown property key, or a value whose shape its key does
// not take, is an error. The grammar accepts any `Key: value` in these lists,
// and the visitors read the keys they know and drop the rest, so a typo such
// as `pathh: '/users'` parsed, checked and executed with the path missing.
// A value was also read by its shape rather than its key: `Response: json from
// $X` set the request body.
//
// Both are new rejections, tied to the header (ADR-0011): under mdl 0 the list
// is read as before and each property warns. The OData lists already refuse
// an unknown key (MDL-ODATA01); these are the lists that did not (#732).

var unknownPropertyKey = langver.Change{
	Code:  "MDL-V1-PROP",
	Since: langver.V1,
	Old:   "an unknown property key is accepted and ignored",
	New:   "an error",
}

var misshapedPropertyValue = langver.Change{
	Code:  "MDL-V1-PROPVALUE",
	Since: langver.V1,
	Old:   "a value its key does not take is accepted, and ignored or read by its shape",
	New:   "an error",
}

// propShape is one shape a property value can have, in the words an error
// message uses for it.
type propShape string

const (
	shapeString   propShape = "a string ('…')"
	shapeDollar   propShape = "a $$…$$ block"
	shapeName     propShape = "a name"
	shapeQName    propShape = "a qualified name (Module.Name)"
	shapeNumber   propShape = "a number"
	shapeBool     propShape = "true or false"
	shapeVarList  propShape = `a variable list ("Key": String, …)`
	shapeVariable propShape = "a $variable"
	shapeConstant propShape = "a constant (@Module.Name)"
	shapeNone     propShape = "none"
	shapeBasic    propShape = "basic (Username: …, Password: …)"
	// shapeAuthMethods is a published REST service's authentication methods.
	shapeAuthMethods propShape = "a method list (basic, session, microflow Module.Name)"
	shapeMethod      propShape = "an HTTP method (get, post, put, patch, delete)"
	shapeParamList   propShape = "a parameter list ($name: Type, …)"
	shapeHeaderList  propShape = "a header list ('Name' = value, …)"
	shapeTemplate    propShape = "template '…'"
	shapeMapping     propShape = "mapping Module.Entity { … }"
	shapeJSONFrom    propShape = "json from $var"
	shapeFileFrom    propShape = "file from $var"
	shapeJSONAs      propShape = "json as $var"
	shapeStringAs    propShape = "string as $var"
	shapeFileAs      propShape = "file as $var"
	shapeStatusAs    propShape = "status as $var"
	shapeJSON        propShape = "json"
	shapeStringKind  propShape = "string"
	shapeFileKind    propShape = "file"
	shapeStatusKind  propShape = "status"
	shapeOther       propShape = "some other value"
)

// propKey is one key a property list reads, and the shapes it takes.
type propKey struct {
	name   string // the spelling the syntax help and describe use
	shapes []propShape
}

// propSchema is the keys one property list reads.
type propSchema struct {
	on   string // what the list belongs to, for the message: "a REST client operation"
	keys []propKey
}

func (s *propSchema) lookup(key string) (propKey, bool) {
	for _, k := range s.keys {
		if strings.EqualFold(k.name, key) {
			return k, true
		}
	}
	return propKey{}, false
}

func (s *propSchema) names() []string {
	out := make([]string, len(s.keys))
	for i, k := range s.keys {
		out[i] = k.name
	}
	return out
}

// checkProperty reports a property whose key s does not read, or whose value
// has a shape the key does not take: an error under mdl 1, a warning under
// mdl 0. It reports nothing for a well-formed property. The caller reads the
// property as it always has either way; under mdl 1 the error stops the script.
func (b *Builder) checkProperty(at antlr.ParserRuleContext, s *propSchema, key string, shape propShape) {
	k, ok := s.lookup(key)
	if !ok {
		detail := fmt.Sprintf("unknown property '%s' on %s", key, s.on)
		if near := suggest.Closest(key, s.names()); near != "" {
			detail += fmt.Sprintf(" — did you mean '%s'?", near)
		} else {
			detail += "."
		}
		detail += " Known properties: " + strings.Join(s.names(), ", ")
		b.strict(unknownPropertyKey, at, detail)
		return
	}
	for _, want := range k.shapes {
		if want == shape {
			return
		}
	}
	takes := make([]string, len(k.shapes))
	for i, sh := range k.shapes {
		takes[i] = string(sh)
	}
	b.strict(misshapedPropertyValue, at, fmt.Sprintf("property '%s' on %s takes %s, not %s",
		k.name, s.on, strings.Join(takes, " or "), shape))
}

// strict is gate for a new rejection whose message names the construct: an
// error from c.Since on, a warning with that detail before it.
func (b *Builder) strict(c langver.Change, at antlr.ParserRuleContext, detail string) {
	line := 0
	if at != nil && at.GetStart() != nil {
		line = at.GetStart().GetLine()
	}
	if c.Applies(b.langVersion) {
		b.addError(fmt.Errorf("line %d: %s", line, detail))
		return
	}
	b.langNotes = append(b.langNotes, ast.LanguageNote{
		Line:    line,
		Code:    c.Code,
		Message: detail + "; " + c.Warning(b.langVersion),
	})
}

// ---------------------------------------------------------------------------
// The schemas. Each lists exactly what its visitor reads, plus what describe
// writes for it, so describe output is accepted under mdl 1.
// ---------------------------------------------------------------------------

var (
	textShapes = []propShape{shapeString, shapeDollar}
	nameShapes = []propShape{shapeName, shapeString}
)

var restClientSchema = propSchema{on: "a REST client", keys: []propKey{
	{"BaseUrl", []propShape{shapeString}},
	{"Authentication", []propShape{shapeNone, shapeBasic}},
	{"OpenApi", []propShape{shapeString}},
	{"Folder", []propShape{shapeString}},
}}

// What `alter consumed rest service … set ( … )` can change. The folder is
// not a property of the document's contents, and the OpenAPI source is
// CREATE's input, not something stored to set.
var restClientAlterSetSchema = propSchema{on: "an ALTER of a REST client", keys: []propKey{
	{"BaseUrl", []propShape{shapeString}},
	{"Authentication", []propShape{shapeNone, shapeBasic}},
}}

var restClientBasicAuthSchema = propSchema{on: "REST client basic authentication", keys: []propKey{
	{"Username", []propShape{shapeString, shapeConstant, shapeVariable}},
	{"Password", []propShape{shapeString, shapeConstant, shapeVariable}},
}}

var restClientOperationSchema = propSchema{on: "a REST client operation", keys: []propKey{
	{"Method", []propShape{shapeMethod}},
	{"Path", []propShape{shapeString}},
	{"Parameters", []propShape{shapeParamList}},
	{"Query", []propShape{shapeParamList}},
	{"Headers", []propShape{shapeHeaderList}},
	{"Body", []propShape{shapeJSONFrom, shapeFileFrom, shapeTemplate, shapeMapping}},
	{"Timeout", []propShape{shapeNumber}},
	{"Response", []propShape{shapeNone, shapeJSON, shapeJSONAs, shapeStringKind, shapeStringAs,
		shapeFileKind, shapeFileAs, shapeStatusKind, shapeStatusAs, shapeMapping}},
}}

var publishedRestSchema = propSchema{on: "a published REST service", keys: []propKey{
	{"Path", []propShape{shapeString}},
	{"Version", []propShape{shapeString}},
	{"ServiceName", []propShape{shapeString}},
	{"Authentication", []propShape{shapeNone, shapeAuthMethods}},
	{"Folder", []propShape{shapeString}},
}}

var businessEventServiceSchema = propSchema{on: "a business event service", keys: []propKey{
	{"ServiceName", []propShape{shapeString}},
	{"EventNamePrefix", []propShape{shapeString}},
	{"Folder", []propShape{shapeString}},
}}

// The model visitor reads each value by one alternative only: a Provider
// written as a string was dropped.
var modelSchema = propSchema{on: "a model", keys: []propKey{
	{"Provider", []propShape{shapeName}},
	{"Key", []propShape{shapeConstant, shapeQName}},
	{"DisplayName", []propShape{shapeString}},
	{"KeyName", []propShape{shapeString}},
	{"KeyId", []propShape{shapeString}},
	{"Environment", []propShape{shapeString}},
	{"ResourceName", []propShape{shapeString}},
	{"DeepLinkURL", []propShape{shapeString}},
}}

var knowledgeBaseSchema = propSchema{on: "a knowledge base", keys: []propKey{
	{"Provider", nameShapes},
	{"Key", []propShape{shapeConstant, shapeQName}},
	{"ModelDisplayName", []propShape{shapeString}},
	{"ModelName", []propShape{shapeString}},
	{"KeyName", []propShape{shapeString}},
	{"KeyId", []propShape{shapeString}},
	{"Environment", []propShape{shapeString}},
	{"DeepLinkURL", []propShape{shapeString}},
}}

var consumedMCPServiceSchema = propSchema{on: "a consumed MCP service", keys: []propKey{
	{"ProtocolVersion", nameShapes},
	{"Version", []propShape{shapeString}},
	{"ConnectionTimeoutSeconds", []propShape{shapeNumber}},
	{"Documentation", textShapes},
}}

// ServerName is stored but not in Studio Pro's editor: a new document gets its
// own name, a duplicate keeps the original's.
var publishedMCPServiceSchema = propSchema{on: "a published MCP service", keys: []propKey{
	{"ServerName", []propShape{shapeString}},
	{"EndpointPath", []propShape{shapeString}},
	{"Version", []propShape{shapeString}},
	{"ProtocolVersion", nameShapes},
	{"AuthenticationEnabled", []propShape{shapeBool}},
	{"AuthenticationMicroflow", []propShape{shapeQName}},
	{"AuthorizationServerUrl", []propShape{shapeString}},
	{"Scopes", []propShape{shapeString}},
	{"Documentation", textShapes},
}}

var publishedMCPToolSchema = propSchema{on: "a published MCP service's tool block", keys: []propKey{
	{"Microflow", []propShape{shapeQName}},
	{"Description", textShapes},
	{"Title", []propShape{shapeString}},
	{"Schema", textShapes},
}}

var agentSchema = propSchema{on: "an agent", keys: []propKey{
	{"UsageType", nameShapes},
	{"Description", textShapes},
	{"Model", []propShape{shapeQName}},
	{"Entity", []propShape{shapeQName}},
	{"Variables", []propShape{shapeVarList}},
	{"MaxTokens", []propShape{shapeNumber}},
	{"ToolChoice", nameShapes},
	{"Temperature", []propShape{shapeNumber}},
	{"TopP", []propShape{shapeNumber}},
	{"SystemPrompt", textShapes},
	{"UserPrompt", textShapes},
	{"EnableBackendStreaming", []propShape{shapeBool}},
	{"EnableUIStreaming", []propShape{shapeBool}},
	{"SelectedModel", []propShape{shapeString}},
	{"SelectedModelName", []propShape{shapeString}},
}}

var agentMCPServiceSchema = propSchema{on: "an agent's mcp service block", keys: []propKey{
	{"Enabled", []propShape{shapeBool}},
	{"Description", textShapes},
	{"UserAccessApproval", []propShape{shapeName}},
	{"DisplayTitle", textShapes},
	{"DisplayDescription", textShapes},
}}

var agentToolSchema = propSchema{on: "an agent's tool block", keys: []propKey{
	{"Enabled", []propShape{shapeBool}},
	{"Description", textShapes},
	{"ToolType", []propShape{shapeName}},
	{"Document", []propShape{shapeQName}},
	{"UserAccessApproval", []propShape{shapeName}},
	{"DisplayTitle", textShapes},
	{"DisplayDescription", textShapes},
}}

var agentKnowledgeBaseSchema = propSchema{on: "an agent's knowledge base block", keys: []propKey{
	{"Source", []propShape{shapeQName}},
	{"Collection", []propShape{shapeString}},
	{"Description", textShapes},
	{"MaxResults", []propShape{shapeNumber}},
	{"Enabled", []propShape{shapeBool}},
	{"MinSimilarity", []propShape{shapeNumber}},
	{"IsVisible", []propShape{shapeBool}},
}}

// ---------------------------------------------------------------------------
// Value shapes, read off the parse tree.
// ---------------------------------------------------------------------------

// modelPropertyShape is the shape of a `Key: value` in an agent-editor list.
func modelPropertyShape(pc *parser.ModelPropertyContext) propShape {
	idents := pc.AllIdentifierOrKeyword()
	switch {
	case pc.AT() != nil:
		return shapeConstant
	case pc.QualifiedName() != nil:
		return shapeQName
	case pc.STRING_LITERAL() != nil:
		return shapeString
	case pc.DOLLAR_STRING() != nil:
		return shapeDollar
	case pc.NUMBER_LITERAL() != nil:
		return shapeNumber
	case pc.BooleanLiteral() != nil:
		return shapeBool
	case pc.VariableDefList() != nil:
		return shapeVarList
	case len(idents) > 1:
		// `Enabled: true` can reach the name alternative, and reads as true.
		if t := strings.ToLower(idents[1].GetText()); t == "true" || t == "false" {
			return shapeBool
		}
		return shapeName
	}
	return shapeOther
}

// checkModelProperties checks every property of an agent-editor list.
func (b *Builder) checkModelProperties(props []parser.IModelPropertyContext, s *propSchema) {
	for _, p := range props {
		pc, ok := p.(*parser.ModelPropertyContext)
		if !ok || pc == nil {
			continue
		}
		idents := pc.AllIdentifierOrKeyword()
		if len(idents) == 0 {
			continue
		}
		b.checkProperty(pc, s, idents[0].GetText(), modelPropertyShape(pc))
	}
}

// checkAgentBodyBlock checks a TOOL, MCP SERVICE or KNOWLEDGE BASE block of
// an agent, in create agent and in alter agent … add.
func (b *Builder) checkAgentBodyBlock(blk *parser.AgentBodyBlockContext) {
	switch {
	case blk.MCP() != nil && blk.SERVICE() != nil:
		b.checkModelProperties(blk.AllModelProperty(), &agentMCPServiceSchema)
	case blk.KNOWLEDGE() != nil && blk.BASE() != nil:
		b.checkModelProperties(blk.AllModelProperty(), &agentKnowledgeBaseSchema)
	case blk.TOOL() != nil:
		b.checkModelProperties(blk.AllModelProperty(), &agentToolSchema)
	}
}

// restClientPropertyShape is the shape of a REST client's `Key: value`.
func restClientPropertyShape(pc *parser.RestClientPropertyContext) propShape {
	switch {
	case pc.BASIC() != nil:
		return shapeBasic
	case pc.NONE() != nil:
		return shapeNone
	case pc.STRING_LITERAL() != nil:
		return shapeString
	case pc.VARIABLE() != nil:
		return shapeVariable
	case pc.AT() != nil:
		return shapeConstant
	}
	return shapeOther
}

// restClientOpPropShape is the shape of a REST client operation's `Key: value`.
func restClientOpPropShape(pc *parser.RestClientOpPropContext) propShape {
	switch {
	case pc.RestHttpMethod() != nil:
		return shapeMethod
	case pc.MAPPING() != nil:
		return shapeMapping
	case pc.TEMPLATE() != nil:
		return shapeTemplate
	case pc.VARIABLE() != nil:
		kind := "json"
		switch {
		case pc.FILE_KW() != nil:
			kind = "file"
		case pc.STRING_TYPE() != nil:
			kind = "string"
		case pc.STATUS() != nil:
			kind = "status"
		}
		if pc.FROM() != nil {
			return propShape(kind + " from $var")
		}
		return propShape(kind + " as $var")
	case pc.JSON() != nil:
		return shapeJSON
	case pc.STRING_TYPE() != nil:
		return shapeStringKind
	case pc.FILE_KW() != nil:
		return shapeFileKind
	case pc.STATUS() != nil:
		return shapeStatusKind
	case pc.NONE() != nil:
		return shapeNone
	case len(pc.AllRestClientParamItem()) > 0:
		return shapeParamList
	case len(pc.AllRestClientHeaderItem()) > 0:
		return shapeHeaderList
	case pc.STRING_LITERAL() != nil:
		return shapeString
	case pc.NUMBER_LITERAL() != nil:
		return shapeNumber
	}
	return shapeOther
}

// odataPropertyShape is the shape of a `Key: value` in an OData-style list,
// which the business event service shares.
func odataPropertyShape(pc *parser.OdataPropertyAssignmentContext) propShape {
	v, ok := pc.OdataPropertyValue().(*parser.OdataPropertyValueContext)
	if !ok || v == nil {
		return shapeOther
	}
	switch {
	case v.STRING_LITERAL() != nil:
		return shapeString
	case v.NUMBER_LITERAL() != nil:
		return shapeNumber
	case v.TRUE() != nil || v.FALSE() != nil:
		return shapeBool
	case v.AT() != nil:
		return shapeConstant
	case v.QualifiedName() != nil && v.MICROFLOW() == nil:
		return shapeQName
	}
	return shapeOther
}

// showPageWithSchema is what a show page's `with` list reads. The list
// ignored every other key, so `with NumberOfPagesToClose = 1` was dropped
// with check and exec clean before the key existed (mendixlabs/mxcli#1617).
var showPageWithSchema = propSchema{on: "a show page action", keys: []propKey{
	{"title", []propShape{shapeString}},
	{"NumberOfPagesToClose", nil}, // any expression: refusePagesToCloseLiteral
}}

// ExitShowPageStatement checks the `with` list against showPageWithSchema.
// location and modal are read and never written: Mendix deleted a Show page
// action's location in 8.0 — the page's own layout decides how it opens —
// and the action never had a modal flag.
func (b *Builder) ExitShowPageStatement(ctx *parser.ShowPageStatementContext) {
	list, ok := ctx.MemberAssignmentList().(*parser.MemberAssignmentListContext)
	if !ok || list == nil {
		return
	}
	for _, m := range list.AllMemberAssignment() {
		assign, ok := m.(*parser.MemberAssignmentContext)
		if !ok || assign.MemberAttributeName() == nil {
			continue
		}
		key := memberAttributeNameText(assign.MemberAttributeName())
		switch strings.ToLower(key) {
		case "numberofpagestoclose":
			b.refusePagesToCloseLiteral(assign)
		case "location", "modal", "modalform":
			b.strict(unknownPropertyKey, assign, fmt.Sprintf(
				"'%s' on a show page action is not stored: the action has no location or modal setting "+
					"(Mendix removed its location in 8.0), the page's own layout decides how it opens. Known properties: %s",
				key, strings.Join(showPageWithSchema.names(), ", ")))
		default:
			b.checkProperty(assign, &showPageWithSchema, key, expressionShape(assign.Expression()))
		}
	}
}

// refusePagesToCloseLiteral refuses a NumberOfPagesToClose literal that is
// not a whole number. The setting is an Integer expression, and mxbuild
// 11.12.5 fails 1.5 and a Boolean with CE0117 "Error(s) in expression" at the
// activity (0, -1 and `$Count + 1` pass). The key was dropped until #1617, so
// no script relied on it and the refusal is not tied to the header — the same
// reasoning as `close page 1.5` (ExitClosePageStatement).
func (b *Builder) refusePagesToCloseLiteral(assign *parser.MemberAssignmentContext) {
	lit, ok := buildExpression(assign.Expression()).(*ast.LiteralExpr)
	if !ok || lit.Kind == ast.LiteralInteger {
		return
	}
	b.addErrorWithExample(
		fmt.Sprintf("%s: `NumberOfPagesToClose = %s`: the number of pages to close is an Integer expression — "+
			"a whole number or an expression of type Integer (Mendix build fails CE0117 otherwise)",
			ctxPos(assign), expressionSourceText(assign.Expression())),
		"  show page M.P with NumberOfPagesToClose = 1;          -- close the current page first\n"+
			"  show page M.P with NumberOfPagesToClose = $Count;     -- an Integer expression")
}

// expressionShape is the shape of an expression value, in the propShape
// vocabulary.
func expressionShape(expr parser.IExpressionContext) propShape {
	if expr == nil {
		return shapeOther
	}
	switch e := buildExpression(expr).(type) {
	case *ast.LiteralExpr:
		switch e.Kind {
		case ast.LiteralString:
			return shapeString
		case ast.LiteralInteger, ast.LiteralDecimal:
			return shapeNumber
		case ast.LiteralBoolean:
			return shapeBool
		}
	case *ast.VariableExpr:
		return shapeVariable
	}
	return shapeOther
}
