// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ValidateProgram runs every semantic check mxcli can make from the parsed
// script alone, plus the few that additionally consult a project when one is
// given. It is the single definition of "what `mxcli check` checks".
//
// It exists as one function because it has two callers that must not diverge:
// `mxcli check`, which reports the violations, and `mxcli exec`, which refuses
// to apply a script whose violations include an error. Those were previously
// unconnected — `check` held this list inline and `exec` ran none of it — so a
// script `check` rejected was applied by `exec` anyway (mxcli-banking findings,
// slice 2: "a page with an invalid widget property was written to the model").
// Keeping the list in one place is what makes "check before exec" enforceable
// rather than a convention.
//
// projectPath may be empty; the checks that need a project skip themselves.
// Parse errors are the caller's business — this operates on a built program.
func ValidateProgram(prog *ast.Program, projectPath string) []linter.Violation {
	// Statement-level checks that need no project connection.
	var violations []linter.Violation
	securityEnabled := programEnablesSecurity(prog)
	for _, stmt := range prog.Statements {
		// Check enumeration values for reserved words
		if enumStmt, ok := stmt.(*ast.CreateEnumerationStmt); ok {
			violations = append(violations, ValidateEnumeration(enumStmt)...)
		}
		// Check entity attributes for reserved system names
		if entityStmt, ok := stmt.(*ast.CreateEntityStmt); ok {
			violations = append(violations, ValidateEntity(entityStmt)...)
		}
		// Apply the same per-attribute checks to ALTER ENTITY ADD ATTRIBUTE
		if alterStmt, ok := stmt.(*ast.AlterEntityStmt); ok {
			violations = append(violations, ValidateAlterEntity(alterStmt)...)
		}
		// An association carries the same pair of contradictory guards (MDL067),
		// and its FROM entity must live in the module it is declared in (MDL070) —
		// the remote-parent form writes a project that cannot be opened.
		if assocStmt, ok := stmt.(*ast.CreateAssociationStmt); ok {
			violations = append(violations, validateIdempotencyGuard(
				assocStmt.CreateOrModify, assocStmt.IfNotExists, "association", assocStmt.Name.String())...)
			violations = append(violations, ValidateAssociationModules(assocStmt)...)
		}
		// A user role with no System module role cannot sign in (CE0156) — but
		// only once security is on, which the script may say itself.
		if roleStmt, ok := stmt.(*ast.CreateUserRoleStmt); ok {
			violations = append(violations, ValidateUserRoleSystemModuleRole(roleStmt, securityEnabled)...)
		}
		// A navigation menu item with no icon is unreadable once the sidebar is
		// collapsed to its icon rail (MDL077). Covers both statements that carry
		// menu items, which share one AST node so they cannot diverge.
		violations = append(violations, validateMenuItemIcons(stmt)...)
		violations = append(violations, validateGlyphCodes(stmt)...)
		// A layout must declare exactly one placeholder named `Main`, with unique
		// names (MDL081/MDL082). mxbuild validates this — CE0848/CE0849/CE0495 —
		// and nothing before it resolves a placeholder name, so the reported
		// script passed check AND exec and failed a build later
		// (mendixlabs/mxcli#1063).
		violations = append(violations, validateLayoutPlaceholders(stmt)...)
		// A page with parameters and a Url must name each parameter in it (CE5601).
		if pageStmt, ok := stmt.(*ast.CreatePageStmtV3); ok {
			violations = append(violations, ValidatePageURLParameters(pageStmt)...)
		}
		// Check microflow body for common issues
		if mfStmt, ok := stmt.(*ast.CreateMicroflowStmt); ok {
			violations = append(violations, ValidateMicroflow(mfStmt)...)
			violations = append(violations,
				ValidateFlowParameterAnnotations("microflow '"+mfStmt.Name.String()+"'", mfStmt.Parameters)...)
		}
		// Parameter annotations for the two flow flavours that do not go through
		// ValidateMicroflow but share the parameter grammar.
		if nfStmt, ok := stmt.(*ast.CreateNanoflowStmt); ok {
			violations = append(violations,
				ValidateFlowParameterAnnotations("nanoflow '"+nfStmt.Name.String()+"'", nfStmt.Parameters)...)
		}
		if ruleStmt, ok := stmt.(*ast.CreateRuleStmt); ok {
			violations = append(violations,
				ValidateFlowParameterAnnotations("rule '"+ruleStmt.Name.String()+"'", ruleStmt.Parameters)...)
		}
		// Check workflow for constructs MxBuild rejects (missing page,
		// single-outcome-with-activities, invalid decision outcome names)
		if wfStmt, ok := stmt.(*ast.CreateWorkflowStmt); ok {
			violations = append(violations, ValidateWorkflow(wfStmt)...)
		}
		// ALTER WORKFLOW … INSERT BRANCH writes the same outcome value, so it
		// carries the same load-time trap (MDL-WF03); an ALTER that inserts or
		// replaces an activity reaches the same build errors as a CREATE body, so
		// MDL-WF06 is checked over what it introduces.
		if awfStmt, ok := stmt.(*ast.AlterWorkflowStmt); ok {
			violations = append(violations, ValidateAlterWorkflow(awfStmt)...)
		}
		// Check GRANT for member rights Mendix cannot store
		if grantStmt, ok := stmt.(*ast.GrantEntityAccessStmt); ok {
			violations = append(violations, ValidateGrantEntityAccess(grantStmt)...)
		}
		// Check typed ALTER SETTINGS / CREATE CONFIGURATION property values
		if setStmt, ok := stmt.(*ast.AlterSettingsStmt); ok {
			violations = append(violations, ValidateSettings(setStmt)...)
		}
		if cfgStmt, ok := stmt.(*ast.CreateConfigurationStmt); ok {
			violations = append(violations, ValidateCreateConfiguration(cfgStmt)...)
		}
		// Check database connection credentials: a literal where Mendix
		// stores a constant reference writes an UNOPENABLE project.
		if dbStmt, ok := stmt.(*ast.CreateDatabaseConnectionStmt); ok {
			violations = append(violations, ValidateDatabaseConnection(dbStmt)...)
		}
		// Check view entity OQL
		if viewStmt, ok := stmt.(*ast.CreateViewEntityStmt); ok {
			if viewStmt.Query.RawQuery != "" {
				violations = append(violations, ValidateOQLSyntax(viewStmt.Query.RawQuery)...)
				violations = append(violations, ValidateOQLTypes(viewStmt.Query.RawQuery, viewStmt.Attributes)...)
				violations = append(violations, ValidateViewAttributeDeclarations(viewStmt.Query.RawQuery, viewStmt.Attributes)...)
			}
		}
	}

	// Check for intra-script duplicate definitions (CREATE X … CREATE X without DROP)
	violations = append(violations, CheckScriptDuplicates(prog)...)

	// Validate design properties against the project's theme registry
	// (themesource design-properties.json) — flags unknown keys and invalid
	// option values, listing the allowed values. Only runs with --project.
	violations = append(violations, ValidateDesignProperties(prog, projectPath)...)

	// Validate pluggable widget properties against widget definitions —
	// catches typos in property keys before MxBuild does. Uses built-in
	// definitions alone when no project is given; with --project, also
	// loads project-installed .def.json files for full coverage.
	violations = append(violations, ValidateWidgetProperties(prog, projectPath)...)

	// Warn (MPR010) when an edit/new form (a parameter-bound DataView) is not
	// wrapped in a layout grid — its label/input widths only render correctly
	// inside a layoutgrid. Same rule as the MPR010 lint rule, surfaced at
	// authoring time on the AST.
	violations = append(violations, ValidatePageLayoutGrid(prog)...)

	// Warn (MDL-OFFLINE01) when a page binds an attribute across more than one
	// association in a project that has an offline navigation profile. Mendix
	// rejects those with CE6206 on any page an offline profile can reach, and
	// the error appears far from the statement that caused it — adding the
	// PROFILE is what invalidates pages written earlier.
	violations = append(violations, ValidateOfflineAttributePaths(prog, projectPath)...)

	// Warn (MDL-WORKFLOW10) when a microflow completes a user task it never
	// assigned. TARGETING decides who may SEE a task; it does not assign it, and
	// completing an unassigned one fails only at runtime, with the button
	// appearing to do nothing.
	violations = append(violations, ValidateTaskClaims(prog)...)

	// Flag control-bar buttons that pass $currentObject — a control bar is
	// not row-scoped, so the argument is unbound (CE1571) at build time.
	violations = append(violations, ValidatePageButtonContext(prog)...)

	// Flag a database-connection TYPE Studio Pro does not offer. mxcli writes
	// the string through and mxbuild does not check it, so a wrong value
	// builds green and simply does not connect.
	violations = append(violations, ValidateDatabaseConnectionType(prog)...)

	// Flag OData property names nothing below will act on. The grammar takes
	// any `name: value` pair, so a typo used to be discarded in silence and
	// the model quietly lacked what the author asked for.
	violations = append(violations, ValidateODataProperties(prog)...)

	// Flag a microflow-backed OData resource whose read microflow cannot keep
	// the promises the service makes for it. A read microflow has no
	// System.HttpResponse parameter, so it cannot answer 400 — its contract
	// has to be declared correctly up front, and nothing else checks that.
	violations = append(violations, ValidateODataReadContract(prog)...)

	// Flag `authentication microflow` with no microflow named. The grammar
	// makes the name optional, so this parses and executes into a service
	// Mendix refuses to build (CE0333).
	violations = append(violations, ValidateODataAuth(prog)...)

	// Flag two service shapes mxbuild rejects — a Path that breaks its
	// slash rules, and the PublishAssociations mode whose name invites
	// exactly the wrong value. A Path with no slash at all is the reason
	// this is worth a check: mxbuild throws out of its own validator with
	// no error code, so there is nothing to look up.
	violations = append(violations, ValidateODataServiceShape(prog)...)

	// Flag a page whose widgets point at a page created further down the same
	// script. `exec` resolves page references in statement order and is not
	// transactional, so this fails after earlier statements are already
	// written. --references catches it too, but the ordering needs no project
	// when the target is created by a plain CREATE (#9).
	violations = append(violations, ValidateScriptPageOrder(prog)...)

	// Flag the same ordering mistake for the other reference kinds the executor
	// resolves at write time — a flow's parameter and return types, an entity
	// attribute's enumeration, an association's endpoints, a CALL, and a GRANT.
	// `exec` already produces the diagnosis once a statement has failed; this
	// says it before the first write (#955).
	violations = append(violations, ValidateScriptDefinitionOrder(prog)...)

	// Flag a document-access GRANT naming a role from another module — Mendix
	// rejects it with CE0148. Needs no project, so it runs here rather than
	// under --references, where it would only fire with -p (#836).
	violations = append(violations, ValidateGrantRoles(prog)...)

	// Flag an after-startup microflow that does not return Boolean (CE0142).
	// The reference RESOLVES — the name exists — so #274's existence check has
	// nothing to say; the constraint is on the thing the setting names. When the
	// script creates the microflow itself, which is the usual shape, the answer
	// is in the script and needs no project (CapTrackV2 FINDINGS §6).
	violations = append(violations, ValidateAfterStartupReturnType(prog)...)

	// Flag an export mapping value whose member is a nested path — an export has
	// to produce the intermediate node, so Mendix rejects it with CE5015. The
	// answer is in the statement, so it runs here rather than under --references
	// (#927).
	violations = append(violations, ValidateExportMappingMembers(prog)...)

	// Flag a template parameter written as a `$variable/` path where the
	// position takes an attribute of the widget's context object. The writer
	// keeps the prefix as part of the attribute name, so the model names an
	// attribute that cannot exist (mendixlabs/mxcli#1046). The answer is in the
	// statement, so it runs here rather than under --references — otherwise
	// `mxcli check page.mdl` would stay silent on a mistake it can see.
	violations = append(violations, ValidateWidgetParamPaths(prog)...)

	// Flag a CREATE whose target name has no module. `exec` refuses it and
	// `check` passed it, so a script stopped partway through with the earlier
	// statements already applied (mendixlabs/mxcli#1050).
	violations = append(violations, ValidateCreateIsQualified(prog)...)

	// Flag `RETURNS void AS $x`. The alias names the returned variable, so
	// pairing it with void is a contradiction — and mxcli believed the alias,
	// writing `return $x` into a flow with no such variable (CE0109,
	// mendixlabs/mxcli#1041).
	violations = append(violations, ValidateVoidReturnAlias(prog)...)

	// Flag a CUSTOM NAME MAP entry that matches nothing in the snippet. Silence
	// there made a typo indistinguishable from not writing the entry, which is
	// how #272's missing `item of` stayed hidden (ako/mxcli#272).
	violations = append(violations, ValidateJsonStructureNames(prog)...)

	// Flag an import mapping element that searches for an object without a key
	// (CE0250), or over an entity that is not persistable (CE0251). The key half
	// is decidable from the statement and runs with or without a project; the
	// persistability half needs the domain model and skips itself without one
	// (ako/mxcli#253).
	violations = append(violations, ValidateImportMappingFind(prog, projectPath)...)

	// Flag a REST client operation whose Body/Response mapping clause has no
	// `{ ... }` body — Mendix cannot reference a mapping document from an
	// operation, so the mapping would be dropped in silence (#843).
	violations = append(violations, ValidateRestClientMappings(prog)...)

	// Flag a scheduled event whose Repeat and fields disagree (a Multiplier on
	// a Daily repeat, an HourOfDay of 99). Decidable from the statement, so it
	// runs here rather than at exec, where the script would already have
	// passed check.
	violations = append(violations, ValidateScheduledEvents(prog)...)

	// Flag an annotation written before a CREATE that the document does not
	// read — a typo, or one on the wrong document kind. The grammar accepts an
	// annotation on every create statement while only six read one, so these
	// parsed and did nothing (MDL059, the same rule statements already have).
	violations = append(violations, ValidateDocumentAnnotations(prog)...)

	return violations
}
