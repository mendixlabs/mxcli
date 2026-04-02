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
	ShowConstants
	ShowEntities
	ShowEntity
	ShowAssociations
	ShowAssociation
	ShowMicroflows
	ShowNanoflows
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

	// Security show types
	ShowProjectSecurity   // SHOW PROJECT SECURITY
	ShowModuleRoles       // SHOW MODULE ROLES [IN module]
	ShowUserRoles         // SHOW USER ROLES
	ShowDemoUsers         // SHOW DEMO USERS
	ShowAccessOn          // SHOW ACCESS ON Module.Entity
	ShowAccessOnMicroflow // SHOW ACCESS ON MICROFLOW Module.MF
	ShowAccessOnPage      // SHOW ACCESS ON PAGE Module.Page
	ShowAccessOnWorkflow  // SHOW ACCESS ON WORKFLOW Module.WF
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

	ShowStructure             // SHOW STRUCTURE [DEPTH n] [IN module] [ALL]
	ShowWorkflows             // SHOW WORKFLOWS [IN module]
	ShowBusinessEventServices // SHOW BUSINESS EVENT SERVICES [IN module]
	ShowBusinessEventClients  // SHOW BUSINESS EVENT CLIENTS [IN module]
	ShowBusinessEvents        // SHOW BUSINESS EVENTS [IN module] (individual messages)
	ShowSettings              // SHOW SETTINGS
	ShowFragments             // SHOW FRAGMENTS
	ShowDatabaseConnections   // SHOW DATABASE CONNECTIONS [IN module]
	ShowImageCollections      // SHOW IMAGE COLLECTIONS [IN module]
	ShowRestClients           // SHOW REST CLIENTS [IN module]
	ShowPublishedRestServices // SHOW PUBLISHED REST SERVICES [IN module]
	ShowConstantValues        // SHOW CONSTANT VALUES [IN module]
	ShowContractEntities      // SHOW CONTRACT ENTITIES FROM Module.Service
	ShowContractActions       // SHOW CONTRACT ACTIONS FROM Module.Service
	ShowContractChannels      // SHOW CONTRACT CHANNELS FROM Module.Service (AsyncAPI)
	ShowContractMessages      // SHOW CONTRACT MESSAGES FROM Module.Service (AsyncAPI)
	ShowLanguages             // SHOW LANGUAGES
	ShowJsonStructures        // SHOW JSON STRUCTURES [IN module]
)

// String returns the human-readable name of the show object type.
func (t ShowObjectType) String() string {
	switch t {
	case ShowModules:
		return "MODULES"
	case ShowEnumerations:
		return "ENUMERATIONS"
	case ShowConstants:
		return "CONSTANTS"
	case ShowEntities:
		return "ENTITIES"
	case ShowEntity:
		return "ENTITY"
	case ShowAssociations:
		return "ASSOCIATIONS"
	case ShowAssociation:
		return "ASSOCIATION"
	case ShowMicroflows:
		return "MICROFLOWS"
	case ShowNanoflows:
		return "NANOFLOWS"
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
	case ShowRestClients:
		return "REST CLIENTS"
	case ShowPublishedRestServices:
		return "PUBLISHED REST SERVICES"
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
	case ShowJsonStructures:
		return "JSON STRUCTURES"
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
	DescribeJavaScriptAction     // DESCRIBE JAVASCRIPT ACTION Module.Name
	DescribeModuleRole           // DESCRIBE MODULE ROLE Module.RoleName
	DescribeUserRole             // DESCRIBE USER ROLE Name
	DescribeDemoUser             // DESCRIBE DEMO USER 'name'
	DescribeODataClient          // DESCRIBE ODATA CLIENT Module.ServiceName
	DescribeODataService         // DESCRIBE ODATA SERVICE Module.ServiceName
	DescribeExternalEntity       // DESCRIBE EXTERNAL ENTITY Module.EntityName
	DescribeNavigation           // DESCRIBE NAVIGATION [profile]
	DescribeWorkflow             // DESCRIBE WORKFLOW Module.Name
	DescribeBusinessEventService // DESCRIBE BUSINESS EVENT SERVICE Module.Name
	DescribeDatabaseConnection   // DESCRIBE DATABASE CONNECTION Module.Name
	DescribeSettings             // DESCRIBE SETTINGS
	DescribeFragment             // DESCRIBE FRAGMENT Name
	DescribeImageCollection      // DESCRIBE IMAGE COLLECTION Module.Name
	DescribeRestClient           // DESCRIBE REST CLIENT Module.Name
	DescribePublishedRestService // DESCRIBE PUBLISHED REST SERVICE Module.Name
	DescribeContractEntity       // DESCRIBE CONTRACT ENTITY Service.EntityName [FORMAT mdl]
	DescribeContractAction       // DESCRIBE CONTRACT ACTION Service.ActionName [FORMAT mdl]
	DescribeContractMessage      // DESCRIBE CONTRACT MESSAGE Service.MessageName
	DescribeJsonStructure        // DESCRIBE JSON STRUCTURE Module.Name
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
	case DescribeRestClient:
		return "REST CLIENT"
	case DescribePublishedRestService:
		return "PUBLISHED REST SERVICE"
	case DescribeContractEntity:
		return "CONTRACT ENTITY"
	case DescribeContractAction:
		return "CONTRACT ACTION"
	case DescribeContractMessage:
		return "CONTRACT MESSAGE"
	case DescribeJsonStructure:
		return "JSON STRUCTURE"
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
	Full       bool // If true, do full parsing (slow but includes activities/widgets/refs)
	Source     bool // If true, build source FTS table (implies full)
	Force      bool // If true, force rebuild even if cache is valid
	Background bool // If true, run in background and return immediately
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

// HelpStmt represents: HELP or ?
type HelpStmt struct{}

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
