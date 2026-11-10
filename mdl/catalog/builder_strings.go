// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"sort"
	"strings"
	"unicode"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/translations"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// buildStrings extracts string literals from documents into the FTS5 strings table.
// Only runs in full mode.
func (b *Builder) buildStrings() error {
	if !b.fullMode {
		return nil
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO strings (QualifiedName, ObjectType, StringValue, StringContext, Language, ElementId, ModuleName)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	widgetStmt, err := b.tx.Prepare(`
		INSERT INTO strings (QualifiedName, ObjectType, StringValue, StringContext, Language, ElementId, ModuleName, WidgetId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer widgetStmt.Close()

	count := 0
	insert := func(qn, objType, value, ctx, lang, elementID, module string) {
		if value == "" {
			return
		}
		stmt.Exec(qn, objType, value, ctx, lang, elementID, module)
		count++
	}
	// The translatable walk knows the widget around each text (ako/mxcli#1303).
	insertText := func(r stringRow) {
		if r.StringValue == "" {
			return
		}
		widgetStmt.Exec(r.QualifiedName, r.ObjectType, r.StringValue, r.StringContext, r.Language, r.ElementID, r.ModuleName, r.WidgetID)
		count++
	}

	// Every TRANSLATABLE string comes from the walk below, not from here. What
	// remains in the typed extractions is the strings that are not Texts$Text
	// and so are invisible to it: URLs, log node names, REST paths,
	// documentation, and the workflow templates (Microflows$StringTemplate,
	// which holds a plain Text and cannot carry a translation).

	// Page URL (no language)
	pageList, err := b.cachedPages()
	if err == nil {
		for _, pg := range pageList {
			if !b.own(string(pg.ID)) {
				continue
			}
			if pg.URL == "" {
				continue
			}
			moduleID := b.hierarchy.findModuleID(pg.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			insert(moduleName+"."+pg.Name, "PAGE", pg.URL, "page_url", "", string(pg.ID), moduleName)
		}
		b.disown()
	}

	// Extract from microflows — using cached list
	mfList, err := b.cachedMicroflows()
	if err == nil {
		for _, mf := range mfList {
			if !b.own(string(mf.ID)) {
				continue
			}
			moduleID := b.hierarchy.findModuleID(mf.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			qn := moduleName + "." + mf.Name

			mfID := string(mf.ID)

			// Documentation (no language). A log message is a plain-text
			// StringTemplate and is indexed with its node name below; the
			// other message templates are Texts$Text and come from the walk.
			if mf.Documentation != "" {
				insert(qn, "MICROFLOW", mf.Documentation, "documentation", "", mfID, moduleName)
			}
			extractLogNodeNames(mf.ObjectCollection, qn, "MICROFLOW", moduleName, insert)
		}
		b.disown()
	}

	// Extract from workflows — using cached list
	wfList, err := b.cachedWorkflows()
	if err == nil {
		for _, wf := range wfList {
			if !b.own(string(wf.ID)) {
				continue
			}
			moduleID := b.hierarchy.findModuleID(wf.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			qn := moduleName + "." + wf.Name

			wfID := string(wf.ID)
			if wf.WorkflowName != "" {
				insert(qn, "WORKFLOW", wf.WorkflowName, "workflow_name", "", wfID, moduleName)
			}
			if wf.WorkflowDescription != "" {
				insert(qn, "WORKFLOW", wf.WorkflowDescription, "workflow_description", "", wfID, moduleName)
			}
			if wf.Documentation != "" {
				insert(qn, "WORKFLOW", wf.Documentation, "documentation", "", wfID, moduleName)
			}

			if wf.Flow != nil {
				extractWorkflowFlowStrings(wf.Flow, qn, moduleName, insert)
			}
		}
		b.disown()
	}

	// Extract from published REST services
	prsServices, err := b.lister().ListPublishedRestServices()
	if err == nil {
		for _, svc := range prsServices {
			if !b.own(string(svc.ID)) {
				continue
			}
			moduleID := b.hierarchy.findModuleID(svc.ContainerID)
			moduleName := b.hierarchy.getModuleName(moduleID)
			qn := moduleName + "." + svc.Name
			svcID := string(svc.ID)

			insert(qn, "PUBLISHED_REST_SERVICE", svc.Path, "rest_path", "", svcID, moduleName)
			insert(qn, "PUBLISHED_REST_SERVICE", svc.ServiceName, "service_name", "", svcID, moduleName)
			insert(qn, "PUBLISHED_REST_SERVICE", svc.Version, "version", "", svcID, moduleName)

			for _, res := range svc.Resources {
				insert(qn, "PUBLISHED_REST_SERVICE", res.Name, "resource_name", "", svcID, moduleName)
				for _, op := range res.Operations {
					if op.Path != "" {
						insert(qn, "PUBLISHED_REST_SERVICE", op.Path, "operation_path", "", svcID, moduleName)
					}
					if op.Summary != "" {
						insert(qn, "PUBLISHED_REST_SERVICE", op.Summary, "operation_summary", "", svcID, moduleName)
					}
				}
			}
		}
		b.disown()
	}

	b.buildTranslatableStrings(insertText)

	docRows, err := b.buildDocumentationStrings()
	if err != nil {
		return err
	}
	count += docRows

	b.report("strings", count)
	return nil
}

// buildDocumentationStrings indexes the documentation of every element whose
// catalog table already holds it: entities, attributes, associations,
// enumerations, constants, pages, snippets, layouts, nanoflows, Java actions,
// REST services, mappings and the rest of the `objects` view, plus the
// parameters of microflows and Java actions and a module's domain model
// documentation.
//
// Documentation is a plain string, not a Texts$Text, so neither the translatable
// walk nor the typed extractions above reach it. It is read back from the tables
// the builders above just filled rather than walked from the model a second
// time. Each row names its element (QualifiedName, ObjectType) and its field
// (StringContext), the same labels the typed microflow and workflow
// documentation rows use.
//
// Left out on purpose:
//   - MICROFLOW and WORKFLOW — their documentation is indexed by the typed
//     extractions in buildStrings, and a second row would make every hit appear
//     twice.
//   - CONTRACT_ENTITY — its Description is a remote service's $metadata summary,
//     not text the project's author wrote.
//   - enumeration values — the metamodel has no documentation property on them.
//
// A module is its own module: the MODULE row of the objects view carries an
// empty ModuleName, so the module's documentation is stored under its Name.
// Without that, a filter on ModuleName (module-scoped searches, excluding
// Marketplace modules) never matches the module's own text.
//
// Rows are limited to the snapshot being built, so a catalog that also holds
// another project's snapshot does not index that project's text under this one.
func (b *Builder) buildDocumentationStrings() (int, error) {
	_, snapshotID := b.snapshotMeta()

	res, err := b.tx.Exec(`
		INSERT INTO strings (QualifiedName, ObjectType, StringValue, StringContext, Language, ElementId, ModuleName)
		SELECT QualifiedName, ObjectType, Description, 'documentation', '', Id,
		       CASE WHEN ObjectType = 'MODULE' THEN Name ELSE ModuleName END
		FROM objects
		WHERE Description <> '' AND SnapshotId = ?1
		  AND ObjectType NOT IN ('MICROFLOW', 'WORKFLOW', 'CONTRACT_ENTITY')
		UNION ALL
		SELECT EntityQualifiedName || '.' || Name, 'ATTRIBUTE', Description, 'documentation', '', Id, ModuleName
		FROM attributes_data
		WHERE Description <> '' AND SnapshotId = ?1
		UNION ALL
		SELECT MicroflowQualifiedName || '.' || Name, 'MICROFLOW_PARAMETER', Description, 'documentation', '', Id, ModuleName
		FROM microflow_parameters_data
		WHERE Description <> '' AND SnapshotId = ?1
		UNION ALL
		SELECT QualifiedName, 'JAVA_ACTION_PARAMETER', Description, 'documentation', '', Id, ModuleName
		FROM java_action_parameters_data
		WHERE Description <> '' AND SnapshotId = ?1
		UNION ALL
		SELECT QualifiedName, 'MODULE', DomainModelDocumentation, 'domain_model_documentation', '', Id, Name
		FROM modules_data
		WHERE DomainModelDocumentation <> '' AND SnapshotId = ?1
	`, snapshotID)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// buildTranslatableStrings indexes every Texts$Text in the project.
//
// It walks the RAW units rather than the typed readers, deliberately. The typed
// path reached five sites because each was hand-written, and a sixth cost
// another case; this reaches all of them — 17 distinct sites in a stock 11.13
// app — with no per-type code, and covers document types mxcli cannot otherwise
// round-trip. Measured on that app, the typed path indexed ~69 of 3265 texts and
// saw 8 of 9 languages, so `ar_DZ` was invisible to SHOW LANGUAGES and to
// QUAL005 rather than merely undercounted.
//
// Atlas design templates (Forms$PageTemplate, Forms$BuildingBlock) are ~70% of
// the corpus and their captions never render in a running app. They are indexed
// anyway, with ObjectType naming the document type so a consumer can filter:
// DESCRIBE TRANSLATIONS reaches them and CREATE TRANSLATIONS writes them, so a
// SHOW LANGUAGES that excluded them would disagree with the statement that
// changes them — the same split this is closing.
// The caller's insert closure counts the rows it writes, so nothing is counted
// here — doing both reported twice the rows the table actually holds.
func (b *Builder) buildTranslatableStrings(insert func(stringRow)) {
	units, err := b.lister().ListRawUnitsByType("")
	if err != nil {
		return
	}

	for _, u := range units {
		if !b.ownRows(string(u.ID)) {
			continue
		}
		if len(u.Contents) == 0 {
			continue
		}
		var named struct {
			Name string `bson:"Name"`
		}
		_ = bson.Unmarshal(u.Contents, &named)

		moduleName := b.hierarchy.getModuleName(b.hierarchy.findModuleID(u.ContainerID))
		qn := named.Name
		if moduleName != "" && qn != "" {
			qn = moduleName + "." + qn
		}

		for _, r := range translatableRows(u.Type, qn, moduleName, u.Contents) {
			insert(r)
		}
	}
	b.disown()
}

// stringRow is one row of the strings index.
type stringRow struct {
	QualifiedName string
	ObjectType    string
	StringValue   string
	StringContext string
	Language      string
	ElementID     string
	ModuleName    string
	WidgetID      string // the widget the text belongs to; "" outside a widget
}

// translatableRows turns one unit's stored bytes into index rows, one per
// (text, language). A language present with an empty string is a text that is
// not translated yet and is skipped: indexing it would make the language look
// present everywhere it is not, which is the opposite of what QUAL005 asks.
func translatableRows(unitType, qualifiedName, moduleName string, raw []byte) []stringRow {
	sites, err := translations.SitesInUnit(raw)
	if err != nil {
		return nil
	}
	objType := catalogObjectType(unitType)

	var out []stringRow
	for _, site := range sites {
		ctx := site.OwnerType + "." + site.Property
		for _, lang := range sortedLangs(site.Targets) {
			if site.Targets[lang] == "" {
				continue
			}
			out = append(out, stringRow{
				QualifiedName: qualifiedName,
				ObjectType:    objType,
				StringValue:   site.Targets[lang],
				StringContext: ctx,
				Language:      lang,
				ElementID:     site.ElementID,
				ModuleName:    moduleName,
				WidgetID:      site.WidgetID,
			})
		}
	}
	return out
}

// sortedLangs keeps row order deterministic — a map iteration here would make
// the catalog's bytes differ between two builds of an unchanged project.
func sortedLangs(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// catalogObjectType turns a unit's stored $Type into the catalog's object-type
// vocabulary — "Forms$PageTemplate" to "PAGE_TEMPLATE". Derived rather than
// looked up in a table, so a document type Mendix adds later is named correctly
// without anyone maintaining a list. It agrees with the hand-written values on
// every type they both cover (PAGE, MICROFLOW, ENUMERATION).
func catalogObjectType(unitType string) string {
	name := unitType
	if i := strings.LastIndex(name, "$"); i >= 0 {
		name = name[i+1:]
	}
	var b strings.Builder
	for i, r := range name {
		if unicode.IsUpper(r) && i > 0 {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToUpper(r))
	}
	return b.String()
}

// extractWorkflowFlowStrings extracts strings from workflow activities recursively.
func extractWorkflowFlowStrings(flow *workflows.Flow, qn, moduleName string, insert func(string, string, string, string, string, string, string)) {
	for _, act := range flow.Activities {
		actID := string(act.GetID())
		if act.GetCaption() != "" {
			insert(qn, "WORKFLOW", act.GetCaption(), "activity_caption", "", actID, moduleName)
		}

		switch a := act.(type) {
		case *workflows.UserTask:
			if a.TaskName != "" {
				insert(qn, "WORKFLOW", a.TaskName, "task_name", "", actID, moduleName)
			}
			if a.TaskDescription != "" {
				insert(qn, "WORKFLOW", a.TaskDescription, "task_description", "", actID, moduleName)
			}
			for _, outcome := range a.Outcomes {
				if outcome.Caption != "" {
					insert(qn, "WORKFLOW", outcome.Caption, "outcome_caption", "", actID, moduleName)
				}
				if outcome.Flow != nil {
					extractWorkflowFlowStrings(outcome.Flow, qn, moduleName, insert)
				}
			}
		case *workflows.SystemTask:
			for _, outcome := range a.Outcomes {
				if f := outcome.GetFlow(); f != nil {
					extractWorkflowFlowStrings(f, qn, moduleName, insert)
				}
			}
		case *workflows.CallMicroflowTask:
			for _, outcome := range a.Outcomes {
				if f := outcome.GetFlow(); f != nil {
					extractWorkflowFlowStrings(f, qn, moduleName, insert)
				}
			}
		case *workflows.ExclusiveSplitActivity:
			for _, outcome := range a.Outcomes {
				if f := outcome.GetFlow(); f != nil {
					extractWorkflowFlowStrings(f, qn, moduleName, insert)
				}
			}
		case *workflows.ParallelSplitActivity:
			for _, outcome := range a.Outcomes {
				if outcome.Flow != nil {
					extractWorkflowFlowStrings(outcome.Flow, qn, moduleName, insert)
				}
			}
		}
	}
}

// extractLogNodeNames indexes the two microflow-activity strings that are NOT a
// Texts$Text: a log node's name and a log message's text. A log message is a
// Microflows$StringTemplate, whose Text is a plain string, so the translatable
// walk in buildTranslatableStrings never sees it (measured on a Mendix 11.12
// project: a word present only in a log message was not found by SEARCH). The
// other message templates — show message, validation feedback — are Texts$Text
// and come from the walk, which also reaches the ones this never listed.
func extractLogNodeNames(oc *microflows.MicroflowObjectCollection, qn, objType, moduleName string, insert func(string, string, string, string, string, string, string)) {
	if oc == nil {
		return
	}
	for _, obj := range oc.Objects {
		act, ok := obj.(*microflows.ActionActivity)
		if !ok || act.Action == nil {
			continue
		}
		a, ok := act.Action.(*microflows.LogMessageAction)
		if !ok {
			continue
		}
		if a.LogNodeName != "" {
			insert(qn, objType, a.LogNodeName, "log_node", "", string(act.ID), moduleName)
		}
		if msg := textValue(a.MessageTemplate); msg != "" {
			insert(qn, objType, msg, "log_message", "", string(act.ID), moduleName)
		}
	}
}
