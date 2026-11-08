// SPDX-License-Identifier: Apache-2.0

package ast

// ============================================================================
// Query Statements
// ============================================================================

// ShowStmt represents various SHOW commands.
type ShowStmt struct {
	ObjectType ShowObjectType
	InModule   string         // Optional module filter
	Name       *QualifiedName // Optional specific object name
	Like       string         // For SHOW GLYPHS LIKE 'pattern' — a name substring
	Transitive bool           // For SHOW CALLERS/CALLEES TRANSITIVE
	Depth      int            // For SHOW CONTEXT/STRUCTURE DEPTH N (default 2)
	All        bool           // For SHOW STRUCTURE ALL (include system modules)
}

func (s *ShowStmt) isStatement() {}

// ShowObjectType represents what to show.
type ShowObjectType int

const (
	ShowModules ShowObjectType = iota
	ShowEnumerations
	ShowFolders
	ShowConstants
	ShowEntities
	ShowEntity
	ShowAssociations
	ShowAssociation
	ShowMicroflows
	ShowNanoflows
	ShowRules
	ShowPages
	ShowSnippets
	ShowLayouts
	ShowJavaActions
	ShowJavaScriptActions
	ShowVersion
	ShowCatalogTables
	ShowCatalogStatus
	ShowCallers    // SHOW CALLERS OF Module.Microflow
	ShowCallees    // SHOW CALLEES OF Module.Microflow
	ShowReferences // SHOW REFERENCES TO Module.Entity
	ShowImpact     // SHOW IMPACT OF Module.Entity
	ShowContext    // SHOW CONTEXT OF Module.Microflow [DEPTH N]
	ShowWidgets    // SHOW WIDGETS [WHERE ...] [IN module]

	ShowCommunities      // SHOW COMMUNITIES
	ShowCommunity        // SHOW COMMUNITY OF Module.Asset
	ShowCommunityMembers // SHOW COMMUNITY MEMBERS OF Module.Asset

	// Security show types
	ShowProjectSecurity   // SHOW PROJECT SECURITY
	ShowModuleRoles       // SHOW MODULE ROLES [IN module]
	ShowUserRoles         // SHOW USER ROLES
	ShowDemoUsers         // SHOW DEMO USERS
	ShowAccessOn          // SHOW ACCESS ON Module.Entity
	ShowAccessOnMicroflow // SHOW ACCESS ON MICROFLOW Module.MF
	ShowAccessOnPage      // SHOW ACCESS ON PAGE Module.Page
	ShowAccessOnWorkflow  // SHOW ACCESS ON WORKFLOW Module.WF
	ShowAccessOnNanoflow  // SHOW ACCESS ON NANOFLOW Module.NF
	ShowSecurityMatrix    // SHOW SECURITY MATRIX [IN module]

	// OData show types
	ShowODataClients     // SHOW ODATA CLIENTS [IN module]
	ShowODataServices    // SHOW ODATA SERVICES [IN module]
	ShowExternalEntities // SHOW EXTERNAL ENTITIES [IN module]
	ShowExternalActions  // SHOW EXTERNAL ACTIONS [IN module]

	// Navigation show types
	ShowNavigation      // SHOW NAVIGATION
	ShowNavigationMenu  // SHOW NAVIGATION MENU [profile]
	ShowNavigationHomes // SHOW NAVIGATION HOMES

	ShowStructure                    // SHOW STRUCTURE [DEPTH n] [IN module] [ALL]
	ShowWorkflows                    // SHOW WORKFLOWS [IN module]
	ShowBusinessEventServices        // SHOW BUSINESS EVENT SERVICES [IN module]
	ShowBusinessEventClients         // SHOW BUSINESS EVENT CLIENTS [IN module]
	ShowBusinessEvents               // SHOW BUSINESS EVENTS [IN module] (individual messages)
	ShowSettings                     // SHOW SETTINGS
	ShowFragments                    // SHOW FRAGMENTS
	ShowDatabaseConnections          // SHOW DATABASE CONNECTIONS [IN module]
	ShowImageCollections             // SHOW IMAGE COLLECTIONS [IN module]
	ShowIconCollections              // SHOW ICON COLLECTIONS [IN module]
	ShowGlyphs                       // SHOW GLYPHS [LIKE 'pattern']
	ShowRestClients                  // SHOW REST CLIENTS [IN module]
	ShowPublishedRestServices        // SHOW PUBLISHED REST SERVICES [IN module]
	ShowDataTransformers             // LIST DATA TRANSFORMERS [IN module]
	ShowConstantValues               // SHOW CONSTANT VALUES [IN module]
	ShowContractEntities             // SHOW CONTRACT ENTITIES FROM Module.Service
	ShowContractActions              // SHOW CONTRACT ACTIONS FROM Module.Service
	ShowContractChannels             // SHOW CONTRACT CHANNELS FROM Module.Service (AsyncAPI)
	ShowContractMessages             // SHOW CONTRACT MESSAGES FROM Module.Service (AsyncAPI)
	ShowLanguages                    // SHOW LANGUAGES
	ShowWorkflowGroups               // SHOW WORKFLOW GROUPS (project settings, not a module listing)
	ShowJsonStructures               // SHOW JSON STRUCTURES [IN module]
	ShowMessageDefinitionCollections // SHOW MESSAGE DEFINITION COLLECTIONS [IN module]
	ShowMessageDefinitions           // SHOW MESSAGE DEFINITIONS [IN module] (11.15 documents)
	ShowImportMappings               // SHOW IMPORT MAPPINGS [IN module]
	ShowExportMappings               // SHOW EXPORT MAPPINGS [IN module]
	ShowModels                       // SHOW MODELS [IN module] (agent-editor Model documents)
	ShowAgents                       // SHOW AGENTS [IN module] (agent-editor Agent documents)
	ShowKnowledgeBases               // SHOW KNOWLEDGE BASES [IN module] (agent-editor KB documents)
	ShowConsumedMCPServices          // SHOW CONSUMED MCP SERVICES [IN module] (agent-editor MCP documents)
	ShowJarDependencies              // LIST JAR DEPENDENCIES [IN module]
	ShowBuildingBlocks               // SHOW BUILDING BLOCKS [IN module]
	ShowConnections                  // SHOW CONNECTIONS (open external SQL connections in this session)
	// ShowAnnotations lists a domain model's canvas notes.
	ShowAnnotations
	ShowPublishedMCPServices // LIST PUBLISHED MCP SERVICES [IN module] (agent-editor, MCP Server 5.x)
)

// String returns the human-readable name of the show object type.
func (t ShowObjectType) String() string {
	switch t {
	case ShowModules:
		return "MODULES"
	case ShowEnumerations:
		return "ENUMERATIONS"
	case ShowFolders:
		return "FOLDERS"
	case ShowConstants:
		return "CONSTANTS"
	case ShowEntities:
		return "ENTITIES"
	case ShowEntity:
		return "ENTITY"
	case ShowAssociations:
		return "ASSOCIATIONS"
	case ShowAnnotations:
		return "ANNOTATIONS"
	case ShowAssociation:
		return "ASSOCIATION"
	case ShowMicroflows:
		return "MICROFLOWS"
	case ShowNanoflows:
		return "NANOFLOWS"
	case ShowRules:
		return "RULES"
	case ShowPages:
		return "PAGES"
	case ShowSnippets:
		return "SNIPPETS"
	case ShowLayouts:
		return "LAYOUTS"
	case ShowJavaActions:
		return "JAVA ACTIONS"
	case ShowJavaScriptActions:
		return "JAVASCRIPT ACTIONS"
	case ShowVersion:
		return "VERSION"
	case ShowCatalogTables:
		return "CATALOG TABLES"
	case ShowCatalogStatus:
		return "CATALOG STATUS"
	case ShowCallers:
		return "CALLERS"
	case ShowCallees:
		return "CALLEES"
	case ShowReferences:
		return "REFERENCES"
	case ShowImpact:
		return "IMPACT"
	case ShowContext:
		return "CONTEXT"
	case ShowWidgets:
		return "WIDGETS"
	case ShowProjectSecurity:
		return "PROJECT SECURITY"
	case ShowModuleRoles:
		return "MODULE ROLES"
	case ShowUserRoles:
		return "USER ROLES"
	case ShowDemoUsers:
		return "DEMO USERS"
	case ShowAccessOn:
		return "ACCESS ON ENTITY"
	case ShowAccessOnMicroflow:
		return "ACCESS ON MICROFLOW"
	case ShowAccessOnPage:
		return "ACCESS ON PAGE"
	case ShowAccessOnWorkflow:
		return "ACCESS ON WORKFLOW"
	case ShowAccessOnNanoflow:
		return "ACCESS ON NANOFLOW"
	case ShowSecurityMatrix:
		return "SECURITY MATRIX"
	case ShowODataClients:
		return "ODATA CLIENTS"
	case ShowODataServices:
		return "ODATA SERVICES"
	case ShowExternalEntities:
		return "EXTERNAL ENTITIES"
	case ShowExternalActions:
		return "EXTERNAL ACTIONS"
	case ShowNavigation:
		return "NAVIGATION"
	case ShowNavigationMenu:
		return "NAVIGATION MENU"
	case ShowNavigationHomes:
		return "NAVIGATION HOMES"
	case ShowStructure:
		return "STRUCTURE"
	case ShowWorkflows:
		return "WORKFLOWS"
	case ShowBusinessEventServices:
		return "BUSINESS EVENT SERVICES"
	case ShowBusinessEventClients:
		return "BUSINESS EVENT CLIENTS"
	case ShowBusinessEvents:
		return "BUSINESS EVENTS"
	case ShowSettings:
		return "SETTINGS"
	case ShowFragments:
		return "FRAGMENTS"
	case ShowDatabaseConnections:
		return "DATABASE CONNECTIONS"
	case ShowImageCollections:
		return "IMAGE COLLECTIONS"
	case ShowIconCollections:
		return "ICON COLLECTIONS"
	case ShowGlyphs:
		return "GLYPHS"
	case ShowRestClients:
		return "REST CLIENTS"
	case ShowPublishedRestServices:
		return "PUBLISHED REST SERVICES"
	case ShowDataTransformers:
		return "DATA TRANSFORMERS"
	case ShowConstantValues:
		return "CONSTANT VALUES"
	case ShowContractEntities:
		return "CONTRACT ENTITIES"
	case ShowContractActions:
		return "CONTRACT ACTIONS"
	case ShowContractChannels:
		return "CONTRACT CHANNELS"
	case ShowContractMessages:
		return "CONTRACT MESSAGES"
	case ShowLanguages:
		return "LANGUAGES"
	case ShowWorkflowGroups:
		return "WORKFLOW GROUPS"
	case ShowJsonStructures:
		return "JSON STRUCTURES"
	case ShowMessageDefinitionCollections:
		return "MESSAGE DEFINITION COLLECTIONS"
	case ShowMessageDefinitions:
		return "MESSAGE DEFINITIONS"
	case ShowImportMappings:
		return "IMPORT MAPPINGS"
	case ShowExportMappings:
		return "EXPORT MAPPINGS"
	case ShowModels:
		return "MODELS"
	case ShowAgents:
		return "AGENTS"
	case ShowKnowledgeBases:
		return "KNOWLEDGE BASES"
	case ShowConsumedMCPServices:
		return "CONSUMED MCP SERVICES"
	case ShowPublishedMCPServices:
		return "PUBLISHED MCP SERVICES"
	case ShowJarDependencies:
		return "JAR DEPENDENCIES"
	case ShowBuildingBlocks:
		return "BUILDING BLOCKS"
	case ShowConnections:
		return "CONNECTIONS"
	default:
		return "UNKNOWN"
	}
}

// ShowFeaturesStmt represents SHOW FEATURES commands against the version registry.
type ShowFeaturesStmt struct {
	// InArea filters by feature area (e.g., "domain_model", "microflows").
	// Empty means show all areas.
	InArea string

	// ForVersion queries features for a specific version (SHOW FEATURES FOR VERSION x.y).
	// Empty means use the connected project's version.
	ForVersion string

	// AddedSince shows features added after this version (SHOW FEATURES ADDED SINCE x.y).
	// Empty means show all features (not just new ones).
	AddedSince string
}

func (s *ShowFeaturesStmt) isStatement() {}

// SelectStmt represents a SELECT query against catalog tables.
type SelectStmt struct {
	Query string // The raw SQL query
}

func (s *SelectStmt) isStatement() {}

// DescribeStmt represents DESCRIBE commands.
type DescribeStmt struct {
	ObjectType DescribeObjectType
	Name       QualifiedName
	WithAll    bool   // For DESCRIBE MODULE ... WITH ALL
	Format     string // For DESCRIBE CONTRACT ... FORMAT mdl
	// Qualifier carries the subject of a DESCRIBE whose subject is not a
	// qualified name: the 'group:artifact' coordinate of a JAR DEPENDENCY, and
	// the code or name of a GLYPH (a glyph has no qualified name — it is a
	// character code in a font, not an element in the project).
	Qualifier string
	// Normalized selects Mode 3 for DESCRIBE MICROFLOW: an irreducible but
	// recombinable graph is rendered by folding the branch guards into one
	// condition instead of being flattened. Opt-in because the output
	// re-executes to a DIFFERENT graph — equivalent behaviour, fewer nodes —
	// and silently reshaping someone's diagram because they asked to read it
	// is its own guard-don't-drop violation.
	Normalized bool
	// WithHandles makes DESCRIBE MICROFLOW print, above each activity, the
	// content address an `alter microflow` target would use for it (output
	// variable, caption or statement, with an ordinal when needed). The
	// handles are comments, so the output still executes unchanged.
	WithHandles bool
}

func (s *DescribeStmt) isStatement() {}

// DescribeObjectType represents what to describe.
type DescribeObjectType int

const (
	DescribeEnumeration DescribeObjectType = iota
	DescribeEntity
	DescribeAssociation
	DescribeMicroflow
	DescribeModule
	DescribePage
	DescribeSnippet
	DescribeLayout
	DescribeConstant
	DescribeJavaAction
	DescribeJavaScriptAction            // DESCRIBE JAVASCRIPT ACTION Module.Name
	DescribeModuleRole                  // DESCRIBE MODULE ROLE Module.RoleName
	DescribeUserRole                    // DESCRIBE USER ROLE Name
	DescribeDemoUser                    // DESCRIBE DEMO USER 'name'
	DescribeODataClient                 // DESCRIBE ODATA CLIENT Module.ServiceName
	DescribeODataService                // DESCRIBE ODATA SERVICE Module.ServiceName
	DescribeExternalEntity              // DESCRIBE EXTERNAL ENTITY Module.EntityName
	DescribeNavigation                  // DESCRIBE NAVIGATION [profile]
	DescribeWorkflow                    // DESCRIBE WORKFLOW Module.Name
	DescribeBusinessEventService        // DESCRIBE BUSINESS EVENT SERVICE Module.Name
	DescribeDatabaseConnection          // DESCRIBE DATABASE CONNECTION Module.Name
	DescribeSettings                    // DESCRIBE SETTINGS
	DescribeFragment                    // DESCRIBE FRAGMENT Name
	DescribeImageCollection             // DESCRIBE IMAGE COLLECTION Module.Name
	DescribeIconCollection              // DESCRIBE ICON COLLECTION Module.Name
	DescribeGlyph                       // DESCRIBE GLYPH 57350 | DESCRIBE GLYPH 'star'
	DescribeRestClient                  // DESCRIBE REST CLIENT Module.Name
	DescribePublishedRestService        // DESCRIBE PUBLISHED REST SERVICE Module.Name
	DescribeDataTransformer             // DESCRIBE DATA TRANSFORMER Module.Name
	DescribeContractEntity              // DESCRIBE CONTRACT ENTITY Service.EntityName [FORMAT mdl]
	DescribeContractAction              // DESCRIBE CONTRACT ACTION Service.ActionName [FORMAT mdl]
	DescribeContractMessage             // DESCRIBE CONTRACT MESSAGE Service.MessageName
	DescribeJsonStructure               // DESCRIBE JSON STRUCTURE Module.Name
	DescribeMessageDefinitionCollection // DESCRIBE MESSAGE DEFINITION COLLECTION Module.Name
	DescribeMessageDefinition           // DESCRIBE MESSAGE DEFINITION Module.Name (11.15 document)
	DescribeNanoflow                    // DESCRIBE NANOFLOW Module.Name
	DescribeRule                        // DESCRIBE RULE Module.Name
	DescribeImportMapping               // DESCRIBE IMPORT MAPPING Module.Name
	DescribeExportMapping               // DESCRIBE EXPORT MAPPING Module.Name
	DescribeModel                       // DESCRIBE MODEL Module.Name (agent-editor Model document)
	DescribeAgent                       // DESCRIBE AGENT Module.Name (agent-editor Agent document)
	DescribeKnowledgeBase               // DESCRIBE KNOWLEDGE BASE Module.Name (agent-editor KB document)
	DescribeConsumedMCPService          // DESCRIBE CONSUMED MCP SERVICE Module.Name (agent-editor MCP document)
	DescribeJarDependency               // DESCRIBE JAR DEPENDENCY ModuleName 'group:artifact'
	DescribeBuildingBlock               // DESCRIBE BUILDING BLOCK Module.Name
	DescribeMenu                        // DESCRIBE MENU Module.Name (standalone Menus$MenuDocument)
	DescribeQueue                       // DESCRIBE QUEUE Module.Name
	DescribeScheduledEvent              // DESCRIBE SCHEDULED EVENT Module.Name
	// DescribeWidget is not a document — it is a widget DEFINITION, named by
	// MDL keyword or widget id. Name carries whichever was written.
	DescribeWidget              // DESCRIBE WIDGET combobox | DESCRIBE WIDGET 'com.mendix.widget.web.combobox.Combobox'
	DescribeRegularExpression   // DESCRIBE REGULAR EXPRESSION Module.Name
	DescribeAuto                // DESCRIBE Module.Name — type auto-detected at execution time
	DescribePublishedMCPService // DESCRIBE PUBLISHED MCP SERVICE Module.Name (agent-editor, MCP Server 5.x)
)

// String returns the human-readable name of the describe object type.
func (t DescribeObjectType) String() string {
	switch t {
	case DescribeEnumeration:
		return "ENUMERATION"
	case DescribeEntity:
		return "ENTITY"
	case DescribeAssociation:
		return "ASSOCIATION"
	case DescribeMicroflow:
		return "MICROFLOW"
	case DescribeModule:
		return "MODULE"
	case DescribePage:
		return "PAGE"
	case DescribeSnippet:
		return "SNIPPET"
	case DescribeLayout:
		return "LAYOUT"
	case DescribeConstant:
		return "CONSTANT"
	case DescribeJavaAction:
		return "JAVA ACTION"
	case DescribeJavaScriptAction:
		return "JAVASCRIPT ACTION"
	case DescribeModuleRole:
		return "MODULE ROLE"
	case DescribeUserRole:
		return "USER ROLE"
	case DescribeDemoUser:
		return "DEMO USER"
	case DescribeODataClient:
		return "ODATA CLIENT"
	case DescribeODataService:
		return "ODATA SERVICE"
	case DescribeExternalEntity:
		return "EXTERNAL ENTITY"
	case DescribeNavigation:
		return "NAVIGATION"
	case DescribeWorkflow:
		return "WORKFLOW"
	case DescribeBusinessEventService:
		return "BUSINESS EVENT SERVICE"
	case DescribeDatabaseConnection:
		return "DATABASE CONNECTION"
	case DescribeSettings:
		return "SETTINGS"
	case DescribeFragment:
		return "FRAGMENT"
	case DescribeImageCollection:
		return "IMAGE COLLECTION"
	case DescribeIconCollection:
		return "ICON COLLECTION"
	case DescribeGlyph:
		return "GLYPH"
	case DescribeRestClient:
		return "REST CLIENT"
	case DescribePublishedRestService:
		return "PUBLISHED REST SERVICE"
	case DescribeDataTransformer:
		return "DATA TRANSFORMER"
	case DescribeContractEntity:
		return "CONTRACT ENTITY"
	case DescribeContractAction:
		return "CONTRACT ACTION"
	case DescribeContractMessage:
		return "CONTRACT MESSAGE"
	case DescribeMessageDefinitionCollection:
		return "MESSAGE DEFINITION COLLECTION"
	case DescribeMessageDefinition:
		return "MESSAGE DEFINITION"
	case DescribeJsonStructure:
		return "JSON STRUCTURE"
	case DescribeNanoflow:
		return "NANOFLOW"
	case DescribeRule:
		return "RULE"
	case DescribeImportMapping:
		return "IMPORT MAPPING"
	case DescribeExportMapping:
		return "EXPORT MAPPING"
	case DescribeModel:
		return "MODEL"
	case DescribeAgent:
		return "AGENT"
	case DescribeKnowledgeBase:
		return "KNOWLEDGE BASE"
	case DescribeConsumedMCPService:
		return "CONSUMED MCP SERVICE"
	case DescribePublishedMCPService:
		return "PUBLISHED MCP SERVICE"
	case DescribeJarDependency:
		return "JAR DEPENDENCY"
	case DescribeBuildingBlock:
		return "BUILDING BLOCK"
	case DescribeMenu:
		return "MENU"
	case DescribeAuto:
		return "AUTO"
	default:
		return "UNKNOWN"
	}
}

// DescribeCatalogTableStmt represents DESCRIBE CATALOG.tablename.
type DescribeCatalogTableStmt struct {
	TableName string // lowercase table name, e.g. "widgets", "entities"
}

func (s *DescribeCatalogTableStmt) isStatement() {}

// ============================================================================
// Repository Statements
// ============================================================================

// UpdateStmt represents: UPDATE
type UpdateStmt struct{}

func (s *UpdateStmt) isStatement() {}

// RefreshStmt represents: REFRESH
type RefreshStmt struct{}

func (s *RefreshStmt) isStatement() {}

// RefreshCatalogStmt represents: REFRESH CATALOG [FULL] [SOURCE] [FORCE] [BACKGROUND]
type RefreshCatalogStmt struct {
	Full        bool    // If true, do full parsing (slow but includes activities/widgets/refs)
	Source      bool    // If true, build source FTS table (implies full)
	Force       bool    // If true, force rebuild even if cache is valid
	Background  bool    // If true, run in background and return immediately
	Communities bool    // If true, run graph analysis (communities/cycles/layers/centrality); implies full
	Resolution  float64 // Leiden resolution for the graph-analysis pass (0 = default 1.0)
}

func (s *RefreshCatalogStmt) isStatement() {}

// ============================================================================
// Session Statements
// ============================================================================

// SetStmt represents: SET key = value
type SetStmt struct {
	Key   string
	Value any // string, int64, or bool
}

func (s *SetStmt) isStatement() {}

// HelpStmt represents: HELP [topic words...]
type HelpStmt struct {
	Topic []string // e.g., ["workflow", "user-task"] for HELP WORKFLOW USER-TASK
}

func (s *HelpStmt) isStatement() {}

// ExitStmt represents: EXIT or QUIT
type ExitStmt struct{}

func (s *ExitStmt) isStatement() {}

// SearchStmt represents: SEARCH 'query'
type SearchStmt struct {
	Query string
}

func (s *SearchStmt) isStatement() {}

// ExecuteScriptStmt represents: EXECUTE SCRIPT 'path'
type ExecuteScriptStmt struct {
	Path string
}

func (s *ExecuteScriptStmt) isStatement() {}
