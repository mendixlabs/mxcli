// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

// testAppKnownFailures is the allowlist for ako/TestApp (testdata/testapp), kept
// apart from PedApp's so each fixture's list can only shrink on its own. The
// rules are those of knownFailures in allowlist_test.go.
//
// Measured on 2026-09-27 at TestApp 11a8fca, when TestApp joined the harness
// (#743): 142 of 775 documents keep both laws. Everything listed is the #721
// classes on a larger fixture and has not been triaged per document. The #743
// entries (workflows, OData clients and external entities, whose describe
// printed a plain `create`) were struck when their rewrites learned to carry
// what describe cannot print, and the false generalization flags #721 B
// dropped struck 27 entity and association entries with them.
//
// ako/mxcli#801 struck 79 more. Executing an association's describe output goes
// through the whole-domain-model rewrite, which rebuilt every entity and
// association in the module: API entities came back Hidden and every access
// rule lost the empty member-access key Studio Pro writes. Untouched elements
// now pass through as stored and a rebuilt member access writes both keys.
// #721 L2 (client action settings: confirmations, progress bars,
// DisabledDuringExecution) struck four pages whose only loss it was.
// #721 L3 (a snippet's own parameter re-bound as a page parameter, in dynamic-text
// parameters and association data sources of WorkflowCommons snippets) struck
// seven snippets whose only loss it was.
// #721 L5 (a List View's database source over an association re-typed as an
// association source, losing its sort and search) struck two snippets and a
// page whose only loss it was, with the list markers and the null
// SourceVariable key Studio Pro writes on every list view. Four more snippets
// stopped failing exec with it — their list view is typed Administration.Account
// again, so a nested snippet call finds its $Account — and now break only
// GetPut, on the snippet call's null visibility key (C3).
// ako/mxcli#950 struck three: Rules.Entity_Menu (navigation-list items described
// as the legacy `show_page 'M.P'`, and unnamed items refused by exec) and two
// WorkflowCommons microflows whose `return if … then … else …` was described
// with a `$` in front.
// ako/mxcli#980 struck Atlas_Core.Phone_Menu and Tablet_Menu: the menu document
// writer left out each item's null AlternativeText and empty Items list, which
// Studio Pro stores on every menu item. Navigation profiles joined the harness
// with it, and round-trip from the start. It also struck
// WorkflowCommons.ConflictedWorkflowDefinitionView_ActionConfirmation, whose only
// loss was the empty OutputMappings Studio Pro stores on every nanoflow call.
// The static open-link address's null AttributeRef (ako/mxcli-ledger FINDINGS
// #170) struck WorkflowCommons.ConflictedWorkflowDefinitionView and
// WorkflowCommons.Readme, whose only loss it was.
// Keying every Widgets list's marker 2 on the property struck
// WorkflowCommons.Snip_TaskDashboard_Numbers, whose only loss it was.
// Writing an empty Widgets list as [2] on a new owner, where it had been left
// out, struck Atlas_Core.FeedbackWidget.
// mendixlabs/mxcli#1627 struck the 16 view-entity associations, whose describe
// printed a plain `create or modify association` that exec refuses (CE6771) —
// the view's OQL column declares them, so describe now gives them no statement —
// and WorkflowCommons.ConflictedWorkflowDefinitionView, whose only loss was the
// stored OQL text describe → exec re-indented.
// ako/mxcli#1631 struck the three Clients.Soap* mappings: gen read their
// imported web service under the SDK name instead of WsdlFile, so the #365
// refusal could not see it and the rewrite dropped the binding (GetPut). Read
// under the storage key, the rewrite is refused and nothing is written.
// ako/mxcli#1560 struck WorkflowCommons.User_WorkflowGroup_Select and
// WorkflowGroup_User_Select: their Save button passes comboBox1's selection, a
// PageVariable naming only the widget, which describe dropped and exec could
// not write back — the rebuilt button called its microflow unbound (CE1571).
var testAppKnownFailures = map[string]knownFailure{
	"building block Atlas_Web_Content.Alert":                              {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.AlertIcon":                          {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.AlertIcon_WithAction":               {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Alert_WithAction":                   {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Breadcrumb":                         {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Breadcrumb_Underline":               {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Card":                               {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Card_Action":                        {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Card_ActionWithImage":               {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Card_Background":                    {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Card_WithImage":                     {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Form_Horizontal":                    {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Form_Horizontal_WithAction":         {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Form_Horizontal_WithTitle":          {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Form_Vertical":                      {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Form_Vertical_WithAction":           {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Form_Vertical_WithTitle":            {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Heroheader":                         {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Heroheader_Background":              {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Heroheader_WithAction":              {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.ListItem_DoubleLine":                {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.ListItem_SingleLine":                {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.ListItem_WithImage":                 {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.List_Cards":                         {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.List_WithImage":                     {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Master_Detail":                      {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Pageheader":                         {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.PageheaderImage":                    {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.PageheaderImage_WithBack":           {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.PageheaderImage_WithControls":       {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Pageheader_WithBack":                {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Pageheader_WithControls":            {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Pageheader_WithSearch":              {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Timeline":                           {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Timeline_WithImage":                 {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Wizard_Arrow":                       {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Wizard_Arrow_Step":                  {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Wizard_Circle":                      {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block Atlas_Web_Content.Wizard_Circle_Step":                 {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"building block FeedbackModule.FeedbackWidget":                        {laws: []law{lawParse}, issue: "#721", why: "building block: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"entity WorkflowCommons.ConflictedWorkflowView":                       {laws: []law{lawExec}, issue: "#721", why: "entity: breaks exec on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"entity WorkflowCommons.MyInitiatedWorkflowView":                      {laws: []law{lawExec}, issue: "#721", why: "entity: breaks exec on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"entity WorkflowCommons.UserTaskView":                                 {laws: []law{lawGetPut, lawPutGet}, issue: "#721", why: "entity: executes since a Studio Pro index keeps its GUID on CREATE OR MODIFY (mendixlabs/mxcli#1470); re-executing adds a fourth access rule (the XPath-scoped grant is not matched to its stored rule) and the documentation's trailing whitespace drifts — not yet triaged into #721's classes"},
	"entity WorkflowCommons.WorkflowAttachment":                           {laws: []law{lawGetPut, lawPutGet}, issue: "#721", why: "entity: breaks getput and putget on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"entity WorkflowCommons.WorkflowComment":                              {laws: []law{lawGetPut, lawPutGet}, issue: "#721", why: "entity: breaks getput and putget on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"entity WorkflowCommons.WorkflowGroupView":                            {laws: []law{lawParse}, issue: "#721", why: "entity: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"entity WorkflowCommons.WorkflowUserTaskView":                         {laws: []law{lawExec}, issue: "#721", why: "entity: breaks exec on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"entity WorkflowCommons.WorkflowView":                                 {laws: []law{lawGetPut, lawPutGet}, issue: "#721", why: "entity: executes since a Studio Pro index keeps its GUID on CREATE OR MODIFY (mendixlabs/mxcli#1470); re-executing adds a fourth access rule (the XPath-scoped grant is not matched to its stored rule) and the documentation's trailing whitespace drifts — not yet triaged into #721's classes"},
	"enumeration WorkflowCommons.Enum_NotificationArea_RenderAs":          {laws: []law{lawGetPut}, issue: "#721", why: "enumeration: breaks getput on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"export mapping Services.OrdersExportMapping":                         {laws: []law{lawGetPut}, issue: "#721", why: "export mapping: breaks getput on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"page WorkflowCommons.WorkflowAttachment_View":                        {laws: []law{lawExec}, issue: "#721", why: "page: breaks exec on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"page WorkflowCommons.WorkflowDefinition_View":                        {laws: []law{lawExec}, issue: "#721", why: "page: breaks exec on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"page WorkflowCommons.WorkflowSubProcess_View":                        {laws: []law{lawExec}, issue: "#721", why: "page: breaks exec on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"rest client Clients.OrdersRestClient":                                {laws: []law{lawParse}, issue: "#721", why: "rest client: breaks parse on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"rule Rules.Rule1":                                                    {laws: []law{lawGetPut}, issue: "#721", why: "rule: breaks getput on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"rule Rules.Rule2":                                                    {laws: []law{lawGetPut}, issue: "#721", why: "rule: breaks getput on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"scheduled event WorkflowCommons.SE_WorkflowAuditTrailRecord_CleanUp": {laws: []law{lawGetPut}, issue: "#721", why: "scheduled event: breaks getput on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"snippet WorkflowCommons.Snip_Assignee_Search":                        {laws: []law{lawGetPut}, issue: "#721", why: "snippet: breaks exec on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes; executes since a widget bound to a snippet parameter is described as $Param.Attr — getput not yet triaged"},
	"snippet WorkflowCommons.Snip_TaskAssignmentHelper_Assignee_Search":   {laws: []law{lawGetPut}, issue: "#721", why: "snippet: breaks exec on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes; executes since a widget bound to a snippet parameter is described as $Param.Attr — getput not yet triaged"},
	"snippet WorkflowCommons.Snip_TaskAssignmentHelper_TargetUser_Search": {laws: []law{lawGetPut}, issue: "#721", why: "snippet: breaks exec on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes; executes since a widget bound to a snippet parameter is described as $Param.Attr — getput not yet triaged"},
	"snippet WorkflowCommons.Snip_TimelineViewer_Timeline_Full":           {laws: []law{lawExec}, issue: "#721", why: "snippet: breaks exec on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"snippet WorkflowCommons.Snip_TimelineViewer_Timeline_Tasks":          {laws: []law{lawExec}, issue: "#721", why: "snippet: breaks exec on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"snippet WorkflowCommons.Snip_UserTask_NameColumn":                    {laws: []law{lawGetPut, lawPutGet}, issue: "#721 #826", why: "executes since describe prints snippet call Params (#826); an association-path template parameter read from a snippet parameter loses its AttributeRef and SourceVariable (#721)"},
	"snippet WorkflowCommons.Snip_WorkflowActivityRecord_ActivityIcon":    {laws: []law{lawExec}, issue: "#721", why: "snippet: breaks exec on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"snippet WorkflowCommons.Snip_WorkflowActivityRecord_Details":         {laws: []law{lawGetPut}, issue: "#721", why: "snippet: breaks exec on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes; executes since a widget bound to a snippet parameter is described as $Param.Attr — getput not yet triaged"},
	"snippet WorkflowCommons.Snip_WorkflowActivityRecord_Header":          {laws: []law{lawGetPut, lawPutGet}, issue: "#721 #826", why: "executes since describe prints snippet call Params (#826); association-path template parameters and an expression parameter drift (#721)"},
	"snippet WorkflowCommons.Snip_WorkflowActivity_Details_ActivityType":  {laws: []law{lawExec}, issue: "#721", why: "snippet: breaks exec on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"snippet WorkflowCommons.Snip_WorkflowSubProcess_Details":             {laws: []law{lawExec}, issue: "#721", why: "snippet: breaks exec on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"snippet WorkflowCommons.Snip_WorkflowSubProcess_IncompatibleWarning": {laws: []law{lawExec}, issue: "#721", why: "snippet: breaks exec on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"snippet WorkflowCommons.Snip_WorkflowSubProcess_State":               {laws: []law{lawExec}, issue: "#721", why: "snippet: breaks exec on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"snippet WorkflowCommons.Snip_WorkflowUserTaskView_Details":           {laws: []law{lawExec}, issue: "#721", why: "snippet: breaks exec on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"snippet WorkflowCommons.Snip_WorkflowUserTaskView_NameColumn":        {laws: []law{lawGetPut, lawPutGet}, issue: "#721 #826", why: "executes since describe prints snippet call Params (#826); an association-path template parameter read from a snippet parameter loses its AttributeRef and SourceVariable (#721)"},
	"snippet WorkflowCommons.Snip_Workflow_CommentsAndAttachments":        {laws: []law{lawExec}, issue: "#721", why: "snippet: breaks exec on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"snippet WorkflowCommons.Snip_Workflow_CommentsAndAttachments_Admin":  {laws: []law{lawExec}, issue: "#721", why: "snippet: breaks exec on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
	"snippet WorkflowCommons.Snip_Workflow_Header":                        {laws: []law{lawGetPut, lawPutGet}, issue: "#721 #826", why: "executes since describe prints snippet call Params (#826); an association-path caption parameter read from a snippet parameter loses its AttributeRef and SourceVariable (#721)"},
	"snippet WorkflowCommons.Snip_Workflow_SubProcessDetails":             {laws: []law{lawExec}, issue: "#721", why: "snippet: breaks exec on TestApp, measured when it joined the harness (#743); not yet triaged into #721's classes"},
}

// createOrModifyProbeKnownFailures is the allowlist of
// TestTestAppCreateOrModifyProbe. It may only shrink; a kind switches its
// describe verb once none of its documents is listed here.
var createOrModifyProbeKnownFailures = map[string]knownFailure{}
