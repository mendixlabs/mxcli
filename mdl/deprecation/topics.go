// SPDX-License-Identifier: Apache-2.0

package deprecation

// topics files every entry under the `mxcli syntax` topics whose statements
// the old spelling appears in, so `mxcli syntax <topic> --deprecated` can list
// the old spellings for that topic. A topic is a syntax registry path; an entry
// is listed for a query that names the path or a prefix of it (`page` lists the
// entries filed under `page.action`).
//
// TestDeprecatedTopicsResolve (cmd/mxcli/syntax) requires every entry to have
// at least one topic and every topic to be a registered path, so an entry added
// without one, or filed under a topic that is renamed, fails there.
var topics = map[string][]string{
	CreateOrReplace:             {"create-modifiers"},
	Show:                        {"page.show", "snippet.show", "layout.show", "navigation.show", "settings.show", "odata.show", "workflow.show", "fragment.show", "buildingblock.show"},
	ListOperationFunctionForm:   {"microflow.list-operations"},
	AggregateFunctionForm:       {"microflow.list-operations"},
	UnstoredWidgetName:          {"page.widgets", "page.column"},
	ConsumedRestService:         {"rest.consumed"},
	ConsumedODataService:        {"odata.consume"},
	PublishedODataService:       {"odata.publish"},
	ODataAuthenticationClause:   {"odata.publish"},
	TaskQueue:                   {"queue"},
	AppSecurity:                 {"security.project-security"},
	SettingsRuntime:             {"settings.alter"},
	ReversedEntityGrant:         {"security.entity-access"},
	QuotedTargetingXPath:        {"workflow.user-task"},
	OnErrorBraces:               {"microflow.error-handling"},
	DollarArgumentName:          {"microflow.call"},
	ColonArgument:               {"microflow.show-page", "page.action", "microflow.call"},
	WorkflowStringArgument:      {"workflow.call-microflow"},
	PositionalTemplateArguments: {"microflow.logging", "microflow.validation"},
	PageActionWord:              {"page.action"},
	ErrorMessageKeyword:         {"domain-model.entity", "validation-rule"},
	DeleteBehaviorClause:        {"domain-model.association"},
	ReferenceSetUnderscore:      {"domain-model.association"},
	ReturnsNone:                 {"rest.call"},
	DocumentationClause:         {"domain-model.constant", "domain-model.association", "json-structure", "image-collection"},
	WorkflowCommentCaption:      {"workflow"},
	FolderProperty:              {"page.create", "snippet.create", "rest.consumed", "odata", "document-folder"},
	DocumentationProperty:       {"regular-expression", "queue", "scheduled-event"},
	ShowSingleThing:             {"page.show", "security.project-security", "structure"},
	UserRoleRemove:              {"security.user-role"},
	SettingsRemove:              {"settings.alter"},
	ColumnForAttribute:          {"domain-model.entity"},
	RestCall:                    {"rest.call"},
	RestCallClauses:             {"rest.call"},
	DescribeWidgetType:          {"page.widget-describe"},
	DefineFragment:              {"fragment.define"},
	"MDL-DEPR130":               {"image-collection", "icon-collection", "message-definition"},
	"MDL-DEPR131":               {"agents.model"},
	"MDL-DEPR132":               {"json-structure"},
	"MDL-DEPR133":               {"security.project-security", "security.guest-access"},
	"MDL-DEPR134":               {"domain-model.constant", "snippet.create"},
	"MDL-DEPR135":               {"domain-model"},
	"MDL-DEPR136":               {"domain-model.constant"},
	"MDL-DEPR137":               {"security.demo-user"},
	"MDL-DEPR138":               {"domain-model.constant"},
	"MDL-DEPR140":               {"workflow.alter"},
	"MDL-DEPR141":               {"workflow.alter"},
	"MDL-DEPR142":               {"workflow.alter"},
	"MDL-DEPR143":               {"workflow.alter"},
	"MDL-DEPR144":               {"workflow.alter"},
	"MDL-DEPR145":               {"workflow.alter"},
	"MDL-DEPR146":               {"workflow.alter", "workflow.parallel-split"},
	"MDL-DEPR147":               {"workflow.alter", "workflow.decision"},
	"MDL-DEPR148":               {"workflow.alter", "workflow.boundary-event"},
	"MDL-DEPR149":               {"workflow.alter"},
	"MDL-DEPR160":               {"domain-model.types"},
	RegexExportLevelPublic:      {"regular-expression"},
	"MDL-DEPR080":               {"workflow.decision", "workflow.boundary-event", "workflow.user-task"},
	"MDL-DEPR081":               {"page.widgets"},
	"MDL-DEPR082":               {"security.entity-access"},
	"MDL-DEPR083":               {"rest.consumed"},
	"MDL-DEPR084":               {"agents.model"},
	"MDL-DEPR085":               {"settings.alter", "domain-model.constant"},
	"MDL-DEPR086":               {"page.create", "snippet.create"},
	"MDL-DEPR070":               {"rest.consumed"},
	"MDL-DEPR071":               {"agents.agent", "agents.mcp-service", "agents.knowledge-base"},
	"MDL-DEPR072":               {"image-collection"},
	"MDL-DEPR075":               {"business-events.create"},
	"MDL-DEPR076":               {"business-events.create"},
	"MDL-DEPR073":               {"message-definition"},
	"MDL-DEPR074":               {"microflow.alter"},
	"MDL-DEPR120":               {"rest.consumed"},
	"MDL-DEPR121":               {"navigation"},
	"MDL-DEPR122":               {"navigation.menu-document"},
	"MDL-DEPR123":               {"page.create", "snippet.create"},
	"MDL-DEPR124":               {"page.widgets"},
	"MDL-DEPR125":               {"page.styling"},
	"MDL-DEPR126":               {"snippet", "page.widgets"},
	"MDL-DEPR127":               {"database-connection"},
	"MDL-DEPR101":               {"page.alter", "snippet.alter"},
	"MDL-DEPR102":               {"page.alter", "snippet.alter"},
	"MDL-DEPR103":               {"page.alter", "snippet.alter"},
	"MDL-DEPR060":               {"settings.alter"},
	"MDL-DEPR061":               {"odata"},
	"MDL-DEPR062":               {"page.styling"},
	"MDL-DEPR063":               {"domain-model.entity"},
	"MDL-DEPR064":               {"domain-model.association"},
	"MDL-DEPR065":               {"domain-model.entity"},
	"MDL-DEPR710":               {"security.user-role"},
	"MDL-DEPR711":               {"rest.consumed"},
}

// Topics returns the `mxcli syntax` topic paths e's old spelling belongs to.
func (e Entry) Topics() []string { return topics[e.Code] }

// ForTopic returns the entries filed under path or under a topic below it, in
// registry order. An empty path returns every entry.
func ForTopic(path string) []Entry {
	var out []Entry
	for _, e := range entries {
		if path == "" || underTopic(e.Topics(), path) {
			out = append(out, e)
		}
	}
	return out
}

func underTopic(ts []string, path string) bool {
	for _, t := range ts {
		if t == path || len(t) > len(path) && t[:len(path)] == path && t[len(path)] == '.' {
			return true
		}
	}
	return false
}
