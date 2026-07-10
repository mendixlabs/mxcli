// SPDX-License-Identifier: Apache-2.0

// Package mprread — gen-typed document reader functions.
//
// These functions live here (not on *mpr.Reader) because the codec → mpr import
// edge already exists for write/binary helpers, so the mpr package itself
// cannot import codec. mprread sits one level above both, depending on each.
//
// Each function follows the same shape: list units of a given BSON $Type
// prefix, decode via codec.DefaultRegistry (through ListUnitsByType[T]), and
// return the gen-typed pointers.
package mprread

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	genBE "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/businessevents"
	genConst "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/constants"
	genDBC "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/databaseconnector"
	genDT "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/datatransformers"
	genDM "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/domainmodels"
	genEnum "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/enumerations"
	genExpMap "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/exportmappings"
	genImg "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/images"
	genImpMap "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/importmappings"
	genJA "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/javaactions"
	genJSA "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/javascriptactions"
	genJson "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/jsonstructures"
	genNav "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/navigation"
	genODataPub "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/odatapublish"
	genPg "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/pages"
	genProj "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/projects"
	genRest "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/rest"
	genSched "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/scheduledevents"
	genSec "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/security"
	genSet "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/settings"
	genWf "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/workflows"
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
)

// decodeOne fetches and decodes a single unit by ID into the requested gen type.
// Returns nil if the bytes cannot be read or do not decode into T.
func decodeOne[T element.Element](r *mmpr.Reader, unitID string) (T, error) {
	var zero T
	raw, err := r.GetRawUnitBytes(unitID)
	if err != nil {
		return zero, fmt.Errorf("read unit %s: %w", unitID, err)
	}
	dec := codec.NewDecoder(codec.DefaultRegistry)
	elem, err := dec.Decode(bson.Raw(raw))
	if err != nil {
		return zero, fmt.Errorf("decode unit %s: %w", unitID, err)
	}
	typed, ok := elem.(T)
	if !ok {
		return zero, fmt.Errorf("unit %s decoded as %T, want %T", unitID, elem, zero)
	}
	return typed, nil
}

// matchesQualified reports whether `qualified` (either a bare local name or a
// fully qualified Module.Name) targets an element whose local name is `local`.
// The plan-defined matching scheme accepts both forms uniformly across the
// GetXByQualifiedName helpers.
func matchesQualified(qualified, local string) bool {
	if local == "" {
		return false
	}
	return qualified == local || strings.HasSuffix(qualified, "."+local)
}

// ---------------------------------------------------------------------------
// Enumerations
// ---------------------------------------------------------------------------

// ListEnumerations decodes every Enumerations$Enumeration unit in the project.
func ListEnumerations(r *mmpr.Reader) ([]*genEnum.Enumeration, error) {
	return ListUnitsByType[*genEnum.Enumeration](r)
}

// GetEnumeration retrieves a single enumeration by unit ID.
func GetEnumeration(r *mmpr.Reader, id model.ID) (*genEnum.Enumeration, error) {
	enums, err := ListEnumerations(r)
	if err != nil {
		return nil, err
	}
	for _, e := range enums {
		if element.ID(id) == e.ID() {
			return e, nil
		}
	}
	return nil, fmt.Errorf("enumeration not found: %s", id)
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

// ListConstants decodes every Constants$Constant unit in the project.
func ListConstants(r *mmpr.Reader) ([]*genConst.Constant, error) {
	return ListUnitsByType[*genConst.Constant](r)
}

// GetConstant retrieves a single constant by unit ID.
func GetConstant(r *mmpr.Reader, id model.ID) (*genConst.Constant, error) {
	consts, err := ListConstants(r)
	if err != nil {
		return nil, err
	}
	for _, c := range consts {
		if element.ID(id) == c.ID() {
			return c, nil
		}
	}
	return nil, fmt.Errorf("constant not found: %s", id)
}

// ---------------------------------------------------------------------------
// Scheduled events
// ---------------------------------------------------------------------------

// ListScheduledEvents decodes every ScheduledEvents$ScheduledEvent unit.
func ListScheduledEvents(r *mmpr.Reader) ([]*genSched.ScheduledEvent, error) {
	return ListUnitsByType[*genSched.ScheduledEvent](r)
}

// GetScheduledEvent retrieves a single scheduled event by unit ID.
func GetScheduledEvent(r *mmpr.Reader, id model.ID) (*genSched.ScheduledEvent, error) {
	events, err := ListScheduledEvents(r)
	if err != nil {
		return nil, err
	}
	for _, s := range events {
		if element.ID(id) == s.ID() {
			return s, nil
		}
	}
	return nil, fmt.Errorf("scheduled event not found: %s", id)
}

// ---------------------------------------------------------------------------
// Import mappings
// ---------------------------------------------------------------------------

// ListImportMappings decodes every ImportMappings$ImportMapping unit.
func ListImportMappings(r *mmpr.Reader) ([]*genImpMap.ImportMapping, error) {
	return ListUnitsByType[*genImpMap.ImportMapping](r)
}

// GetImportMappingByQualifiedName retrieves an import mapping by its qualified
// name (Module.Name) or by its local name alone.
func GetImportMappingByQualifiedName(r *mmpr.Reader, qualifiedName string) (*genImpMap.ImportMapping, error) {
	all, err := ListImportMappings(r)
	if err != nil {
		return nil, err
	}
	for _, m := range all {
		if matchesQualified(qualifiedName, m.Name()) {
			return m, nil
		}
	}
	return nil, fmt.Errorf("import mapping not found: %s", qualifiedName)
}

// ---------------------------------------------------------------------------
// Export mappings
// ---------------------------------------------------------------------------

// ListExportMappings decodes every ExportMappings$ExportMapping unit.
func ListExportMappings(r *mmpr.Reader) ([]*genExpMap.ExportMapping, error) {
	return ListUnitsByType[*genExpMap.ExportMapping](r)
}

// GetExportMappingByQualifiedName retrieves an export mapping by qualified name.
func GetExportMappingByQualifiedName(r *mmpr.Reader, qualifiedName string) (*genExpMap.ExportMapping, error) {
	all, err := ListExportMappings(r)
	if err != nil {
		return nil, err
	}
	for _, m := range all {
		if matchesQualified(qualifiedName, m.Name()) {
			return m, nil
		}
	}
	return nil, fmt.Errorf("export mapping not found: %s", qualifiedName)
}

// ---------------------------------------------------------------------------
// JSON structures
// ---------------------------------------------------------------------------

// ListJsonStructures decodes every JsonStructures$JsonStructure unit.
func ListJsonStructures(r *mmpr.Reader) ([]*genJson.JsonStructure, error) {
	return ListUnitsByType[*genJson.JsonStructure](r)
}

// GetJsonStructureByQualifiedName retrieves a JSON structure by qualified name.
func GetJsonStructureByQualifiedName(r *mmpr.Reader, qualifiedName string) (*genJson.JsonStructure, error) {
	all, err := ListJsonStructures(r)
	if err != nil {
		return nil, err
	}
	for _, m := range all {
		if matchesQualified(qualifiedName, m.Name()) {
			return m, nil
		}
	}
	return nil, fmt.Errorf("json structure not found: %s", qualifiedName)
}

// ---------------------------------------------------------------------------
// Web services & related document types
// ---------------------------------------------------------------------------

// ListBusinessEventServices decodes every BusinessEvents$BusinessEventService unit.
func ListBusinessEventServices(r *mmpr.Reader) ([]*genBE.BusinessEventService, error) {
	return ListUnitsByType[*genBE.BusinessEventService](r)
}

// ListDatabaseConnections decodes every DatabaseConnector$DatabaseConnection unit.
func ListDatabaseConnections(r *mmpr.Reader) ([]*genDBC.DatabaseConnection, error) {
	return ListUnitsByType[*genDBC.DatabaseConnection](r)
}

// ListDataTransformers decodes every DataTransformers$DataTransformer unit.
func ListDataTransformers(r *mmpr.Reader) ([]*genDT.DataTransformer, error) {
	return ListUnitsByType[*genDT.DataTransformer](r)
}

// ListImageCollections decodes every Images$ImageCollection unit.
func ListImageCollections(r *mmpr.Reader) ([]*genImg.ImageCollection, error) {
	return ListUnitsByType[*genImg.ImageCollection](r)
}

// ListConsumedODataServices decodes every Rest$ConsumedODataService unit.
//
// Both ConsumedOData and PublishedRest services live under the `Rest` BSON
// namespace; only PublishedOData uses the legacy `ODataPublish$...Service2`
// type name (see ListPublishedODataServices).
func ListConsumedODataServices(r *mmpr.Reader) ([]*genRest.ConsumedODataService, error) {
	return ListUnitsByType[*genRest.ConsumedODataService](r)
}

// ListPublishedODataServices decodes every ODataPublish$PublishedODataService2
// unit. The trailing `2` is part of the BSON $Type, matching the v2 schema
// generation; the Go type drops it from the documentation but keeps it in the
// struct name (PublishedODataService2).
func ListPublishedODataServices(r *mmpr.Reader) ([]*genODataPub.PublishedODataService2, error) {
	return ListUnitsByType[*genODataPub.PublishedODataService2](r)
}

// ListConsumedRestServices decodes every Rest$ConsumedRestService unit.
func ListConsumedRestServices(r *mmpr.Reader) ([]*genRest.ConsumedRestService, error) {
	return ListUnitsByType[*genRest.ConsumedRestService](r)
}

// ListPublishedRestServices decodes every Rest$PublishedRestService unit.
func ListPublishedRestServices(r *mmpr.Reader) ([]*genRest.PublishedRestService, error) {
	return ListUnitsByType[*genRest.PublishedRestService](r)
}

// ---------------------------------------------------------------------------
// Navigation
// ---------------------------------------------------------------------------

// ListNavigationDocuments decodes every Navigation$NavigationDocument unit.
func ListNavigationDocuments(r *mmpr.Reader) ([]*genNav.NavigationDocument, error) {
	return ListUnitsByType[*genNav.NavigationDocument](r)
}

// GetNavigation returns the first NavigationDocument in the project, which is
// the singleton navigation root that every Mendix project carries.
func GetNavigation(r *mmpr.Reader) (*genNav.NavigationDocument, error) {
	docs, err := ListNavigationDocuments(r)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("navigation document not found")
	}
	return docs[0], nil
}

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

// GetProjectSettings returns the project-level Settings$ProjectSettings doc.
// There is at most one per project.
func GetProjectSettings(r *mmpr.Reader) (*genSet.ProjectSettings, error) {
	settings, err := ListUnitsByType[*genSet.ProjectSettings](r)
	if err != nil {
		return nil, err
	}
	if len(settings) == 0 {
		return nil, fmt.Errorf("project settings not found")
	}
	return settings[0], nil
}

// ListModuleSettings decodes every Projects$ModuleSettings unit. ModuleSettings
// lives in the `projects` gen package even though its BSON $Type is namespaced
// under `Projects` — the gen layer preserves the historical type name.
func ListModuleSettings(r *mmpr.Reader) ([]*genProj.ModuleSettings, error) {
	return ListUnitsByType[*genProj.ModuleSettings](r)
}

// GetModuleSettings returns the ModuleSettings whose container is the given
// module ID. Uses the raw unit ContainerID for matching because the decoded
// element drops container linkage by default.
func GetModuleSettings(r *mmpr.Reader, moduleID model.ID) (*genProj.ModuleSettings, error) {
	units, err := ListUnitsWithContainer[*genProj.ModuleSettings](r)
	if err != nil {
		return nil, err
	}
	for _, u := range units {
		if u.ContainerID == moduleID {
			return u.Element, nil
		}
	}
	return nil, fmt.Errorf("module settings not found for module: %s", moduleID)
}

// ---------------------------------------------------------------------------
// Security
// ---------------------------------------------------------------------------

// GetProjectSecurity returns the project-level Security$ProjectSecurity doc.
func GetProjectSecurity(r *mmpr.Reader) (*genSec.ProjectSecurity, error) {
	secs, err := ListUnitsByType[*genSec.ProjectSecurity](r)
	if err != nil {
		return nil, err
	}
	if len(secs) == 0 {
		return nil, fmt.Errorf("project security not found")
	}
	return secs[0], nil
}

// ListModuleSecurity decodes every Security$ModuleSecurity unit.
func ListModuleSecurity(r *mmpr.Reader) ([]*genSec.ModuleSecurity, error) {
	return ListUnitsByType[*genSec.ModuleSecurity](r)
}

// ---------------------------------------------------------------------------
// Agent-editor documents (CustomBlobDocument wrapper)
// ---------------------------------------------------------------------------
//
// All four document types created by the Studio Pro Agent Editor extension
// (Agent, Model, Knowledge Base, Consumed MCP Service) share the same outer
// CustomBlobDocument BSON wrapper and are discriminated by the
// CustomDocumentType field. The wrapper's Contents field is a JSON string
// containing the actual payload.
//
// This mirrors the structure in sdk/mpr/parser_customblob.go — kept in sync
// by hand since neither side is generated. If you add a new payload field to
// mdl/types.{Model,KnowledgeBase,ConsumedMCPService,Agent}, update the JSON
// struct here and in sdk/mpr/parser_customblob.go.

const customBlobDocBsonType = "CustomBlobDocuments$CustomBlobDocument"

// customBlobWrapper holds the fields shared by every agent-editor document.
type customBlobWrapper struct {
	Name               string
	Documentation      string
	Excluded           bool
	ExportLevel        string
	CustomDocumentType string
	Contents           string // JSON payload
}

// parseCustomBlobWrapper decodes the outer CustomBlobDocument BSON.
func parseCustomBlobWrapper(contents []byte) (*customBlobWrapper, error) {
	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal CustomBlobDocument BSON: %w", err)
	}
	out := &customBlobWrapper{}
	if v, ok := raw["Name"].(string); ok {
		out.Name = v
	}
	if v, ok := raw["Documentation"].(string); ok {
		out.Documentation = v
	}
	if v, ok := raw["Excluded"].(bool); ok {
		out.Excluded = v
	}
	if v, ok := raw["ExportLevel"].(string); ok {
		out.ExportLevel = v
	}
	if v, ok := raw["CustomDocumentType"].(string); ok {
		out.CustomDocumentType = v
	}
	if v, ok := raw["Contents"].(string); ok {
		out.Contents = v
	}
	return out, nil
}

// listAgentEditorDocsByType filters CustomBlobDocument units by their
// CustomDocumentType discriminator and hands each matching unit to `decode`.
// `decode` returns the typed document or an error.
func listAgentEditorDocsByType[T any](r *mmpr.Reader, customType string, decode func(unitID, containerID string, wrap *customBlobWrapper) (*T, error)) ([]*T, error) {
	refs, err := r.ListUnitsByType(customBlobDocBsonType)
	if err != nil {
		return nil, err
	}
	var result []*T
	for _, ref := range refs {
		wrap, err := parseCustomBlobWrapper(ref.Contents)
		if err != nil {
			continue
		}
		if wrap.CustomDocumentType != customType {
			continue
		}
		doc, err := decode(ref.ID, ref.ContainerID, wrap)
		if err != nil {
			return nil, fmt.Errorf("decode agent-editor %s %s: %w", customType, ref.ID, err)
		}
		result = append(result, doc)
	}
	return result, nil
}

// ListAgentEditorModels decodes every CustomBlobDocument whose
// CustomDocumentType == "agenteditor.model" into a *types.Model.
func ListAgentEditorModels(r *mmpr.Reader) ([]*types.Model, error) {
	return listAgentEditorDocsByType(r, types.CustomTypeModel, decodeAgentEditorModel)
}

func decodeAgentEditorModel(unitID, containerID string, wrap *customBlobWrapper) (*types.Model, error) {
	m := &types.Model{}
	m.ID = model.ID(unitID)
	m.TypeName = customBlobDocBsonType
	m.ContainerID = model.ID(containerID)
	m.Name = wrap.Name
	m.Documentation = wrap.Documentation
	m.Excluded = wrap.Excluded
	m.ExportLevel = wrap.ExportLevel

	if wrap.Contents == "" {
		return m, nil
	}
	var payload struct {
		Type           string `json:"type"`
		Name           string `json:"name"`
		DisplayName    string `json:"displayName"`
		Provider       string `json:"provider"`
		ProviderFields struct {
			Environment  string             `json:"environment"`
			DeepLinkURL  string             `json:"deepLinkURL"`
			KeyID        string             `json:"keyId"`
			KeyName      string             `json:"keyName"`
			ResourceName string             `json:"resourceName"`
			Key          *types.ConstantRef `json:"key"`
		} `json:"providerFields"`
	}
	if err := json.Unmarshal([]byte(wrap.Contents), &payload); err != nil {
		return nil, fmt.Errorf("unmarshal Model Contents JSON: %w", err)
	}
	m.Type = payload.Type
	m.InnerName = payload.Name
	m.DisplayName = payload.DisplayName
	m.Provider = payload.Provider
	m.Environment = payload.ProviderFields.Environment
	m.DeepLinkURL = payload.ProviderFields.DeepLinkURL
	m.KeyID = payload.ProviderFields.KeyID
	m.KeyName = payload.ProviderFields.KeyName
	m.ResourceName = payload.ProviderFields.ResourceName
	m.Key = payload.ProviderFields.Key
	return m, nil
}

// ListAgentEditorKnowledgeBases decodes CustomDocumentType ==
// "agenteditor.knowledgebase" entries into *types.KnowledgeBase.
func ListAgentEditorKnowledgeBases(r *mmpr.Reader) ([]*types.KnowledgeBase, error) {
	return listAgentEditorDocsByType(r, types.CustomTypeKnowledgeBase, decodeAgentEditorKnowledgeBase)
}

func decodeAgentEditorKnowledgeBase(unitID, containerID string, wrap *customBlobWrapper) (*types.KnowledgeBase, error) {
	k := &types.KnowledgeBase{}
	k.ID = model.ID(unitID)
	k.TypeName = customBlobDocBsonType
	k.ContainerID = model.ID(containerID)
	k.Name = wrap.Name
	k.Documentation = wrap.Documentation
	k.Excluded = wrap.Excluded
	k.ExportLevel = wrap.ExportLevel

	if wrap.Contents == "" {
		return k, nil
	}
	var payload struct {
		Name           string `json:"name"`
		Provider       string `json:"provider"`
		ProviderFields struct {
			Environment      string             `json:"environment"`
			DeepLinkURL      string             `json:"deepLinkURL"`
			KeyID            string             `json:"keyId"`
			KeyName          string             `json:"keyName"`
			ModelDisplayName string             `json:"modelDisplayName"`
			ModelName        string             `json:"modelName"`
			Key              *types.ConstantRef `json:"key"`
		} `json:"providerFields"`
	}
	if err := json.Unmarshal([]byte(wrap.Contents), &payload); err != nil {
		return nil, fmt.Errorf("unmarshal KnowledgeBase Contents JSON: %w", err)
	}
	k.Provider = payload.Provider
	k.Environment = payload.ProviderFields.Environment
	k.DeepLinkURL = payload.ProviderFields.DeepLinkURL
	k.KeyID = payload.ProviderFields.KeyID
	k.KeyName = payload.ProviderFields.KeyName
	k.ModelDisplayName = payload.ProviderFields.ModelDisplayName
	k.ModelName = payload.ProviderFields.ModelName
	k.Key = payload.ProviderFields.Key
	return k, nil
}

// ListAgentEditorConsumedMCPServices decodes CustomDocumentType ==
// "agenteditor.consumedMCPService" entries into *types.ConsumedMCPService.
func ListAgentEditorConsumedMCPServices(r *mmpr.Reader) ([]*types.ConsumedMCPService, error) {
	return listAgentEditorDocsByType(r, types.CustomTypeConsumedMCPService, decodeAgentEditorConsumedMCPService)
}

func decodeAgentEditorConsumedMCPService(unitID, containerID string, wrap *customBlobWrapper) (*types.ConsumedMCPService, error) {
	c := &types.ConsumedMCPService{}
	c.ID = model.ID(unitID)
	c.TypeName = customBlobDocBsonType
	c.ContainerID = model.ID(containerID)
	c.Name = wrap.Name
	c.Documentation = wrap.Documentation
	c.Excluded = wrap.Excluded
	c.ExportLevel = wrap.ExportLevel

	if wrap.Contents == "" {
		return c, nil
	}
	var payload struct {
		ProtocolVersion          string `json:"protocolVersion"`
		Documentation            string `json:"documentation"`
		Version                  string `json:"version"`
		ConnectionTimeoutSeconds int    `json:"connectionTimeoutSeconds"`
	}
	if err := json.Unmarshal([]byte(wrap.Contents), &payload); err != nil {
		return nil, fmt.Errorf("unmarshal ConsumedMCPService Contents JSON: %w", err)
	}
	c.ProtocolVersion = payload.ProtocolVersion
	c.InnerDocumentation = payload.Documentation
	c.Version = payload.Version
	c.ConnectionTimeoutSeconds = payload.ConnectionTimeoutSeconds
	return c, nil
}

// ListAgentEditorAgents decodes CustomDocumentType == "agenteditor.agent"
// entries into *types.Agent.
func ListAgentEditorAgents(r *mmpr.Reader) ([]*types.Agent, error) {
	return listAgentEditorDocsByType(r, types.CustomTypeAgent, decodeAgentEditorAgent)
}

func decodeAgentEditorAgent(unitID, containerID string, wrap *customBlobWrapper) (*types.Agent, error) {
	a := &types.Agent{}
	a.ID = model.ID(unitID)
	a.TypeName = customBlobDocBsonType
	a.ContainerID = model.ID(containerID)
	a.Name = wrap.Name
	a.Documentation = wrap.Documentation
	a.Excluded = wrap.Excluded
	a.ExportLevel = wrap.ExportLevel

	if wrap.Contents == "" {
		return a, nil
	}
	var payload struct {
		Description        string              `json:"description"`
		SystemPrompt       string              `json:"systemPrompt"`
		UserPrompt         string              `json:"userPrompt"`
		UsageType          string              `json:"usageType"`
		Variables          []types.AgentVar    `json:"variables"`
		Tools              []types.AgentTool   `json:"tools"`
		KnowledgebaseTools []types.AgentKBTool `json:"knowledgebaseTools"`
		Model              *types.DocRef       `json:"model"`
		Entity             *types.DocRef       `json:"entity"`
		MaxTokens          *int                `json:"maxTokens"`
		ToolChoice         string              `json:"toolChoice"`
		Temperature        *float64            `json:"temperature"`
		TopP               *float64            `json:"topP"`
	}
	if err := json.Unmarshal([]byte(wrap.Contents), &payload); err != nil {
		return nil, fmt.Errorf("unmarshal Agent Contents JSON: %w", err)
	}
	a.Description = payload.Description
	a.SystemPrompt = payload.SystemPrompt
	a.UserPrompt = payload.UserPrompt
	a.UsageType = payload.UsageType
	a.Variables = payload.Variables
	a.Tools = payload.Tools
	a.KBTools = payload.KnowledgebaseTools
	a.Model = payload.Model
	a.Entity = payload.Entity
	a.MaxTokens = payload.MaxTokens
	a.ToolChoice = payload.ToolChoice
	a.Temperature = payload.Temperature
	a.TopP = payload.TopP
	return a, nil
}

// ---------------------------------------------------------------------------
// Pages, Snippets, Layouts, BuildingBlocks, PageTemplates (Forms domain)
// ---------------------------------------------------------------------------

// ListPages decodes every Forms$Page unit in the project.
func ListPages(r *mmpr.Reader) ([]*genPg.Page, error) {
	return ListUnitsByType[*genPg.Page](r)
}

// ListSnippets decodes every Forms$Snippet unit in the project.
func ListSnippets(r *mmpr.Reader) ([]*genPg.Snippet, error) {
	return ListUnitsByType[*genPg.Snippet](r)
}

// ListLayouts decodes every Forms$Layout unit in the project.
func ListLayouts(r *mmpr.Reader) ([]*genPg.Layout, error) {
	return ListUnitsByType[*genPg.Layout](r)
}

// ListBuildingBlocks decodes every Forms$BuildingBlock unit in the project.
func ListBuildingBlocks(r *mmpr.Reader) ([]*genPg.BuildingBlock, error) {
	return ListUnitsByType[*genPg.BuildingBlock](r)
}

// ListPageTemplates decodes every Forms$PageTemplate unit in the project.
func ListPageTemplates(r *mmpr.Reader) ([]*genPg.PageTemplate, error) {
	return ListUnitsByType[*genPg.PageTemplate](r)
}

// ---------------------------------------------------------------------------
// Domain models
// ---------------------------------------------------------------------------

// ListDomainModels decodes every DomainModels$DomainModel unit in the project.
func ListDomainModels(r *mmpr.Reader) ([]*genDM.DomainModel, error) {
	return ListUnitsByType[*genDM.DomainModel](r)
}

// ---------------------------------------------------------------------------
// Workflows
// ---------------------------------------------------------------------------

// ListWorkflows decodes every Workflows$Workflow unit in the project.
func ListWorkflows(r *mmpr.Reader) ([]*genWf.Workflow, error) {
	return ListUnitsByType[*genWf.Workflow](r)
}

// ---------------------------------------------------------------------------
// Java and JavaScript actions
// ---------------------------------------------------------------------------

// ListJavaActions decodes every JavaActions$JavaAction unit in the project.
func ListJavaActions(r *mmpr.Reader) ([]*genJA.JavaAction, error) {
	return ListUnitsByType[*genJA.JavaAction](r)
}

// ListJavaScriptActions decodes every JavaScriptActions$JavaScriptAction unit.
func ListJavaScriptActions(r *mmpr.Reader) ([]*genJSA.JavaScriptAction, error) {
	return ListUnitsByType[*genJSA.JavaScriptAction](r)
}

// ---------------------------------------------------------------------------
// Modules and Folders
// ---------------------------------------------------------------------------
//
// Modules are stored as Projects$ModuleImpl units. The codec registers
// *genProj.Module under the canonical "Projects$Module" name plus
// "Projects$ModuleImpl" as an alias, so the generic ListUnitsByType[T] type
// resolver cannot disambiguate (it returns the canonical name, which then
// fails the strict ref.Type match against the actual ModuleImpl rows). These
// helpers therefore call r.ListUnitsByType("Projects$ModuleImpl") directly
// and decode via the codec.

// ListModules decodes every Projects$ModuleImpl unit in the project.
// Returns each module paired with its container (project root) ID.
func ListModules(r *mmpr.Reader) ([]Unit[*genProj.Module], error) {
	const typeName = "Projects$ModuleImpl"
	refs, err := r.ListUnitsByType(typeName)
	if err != nil {
		return nil, err
	}
	dec := codec.NewDecoder(codec.DefaultRegistry)
	out := make([]Unit[*genProj.Module], 0, len(refs))
	for _, ref := range refs {
		if ref.Type != typeName {
			continue
		}
		elem, err := dec.Decode(bson.Raw(ref.Contents))
		if err != nil {
			return nil, fmt.Errorf("decode module %s: %w", ref.ID, err)
		}
		typed, ok := elem.(*genProj.Module)
		if !ok {
			return nil, fmt.Errorf("module %s decoded as %T, want *genProj.Module", ref.ID, elem)
		}
		typed.SetID(element.ID(ref.ID))
		out = append(out, Unit[*genProj.Module]{Element: typed, ContainerID: model.ID(ref.ContainerID)})
	}
	return out, nil
}

// ListFolders decodes every Projects$Folder unit in the project.
// Folders are container-scoped: ContainerID identifies the parent Module or
// parent Folder.
func ListFolders(r *mmpr.Reader) ([]Unit[*genProj.Folder], error) {
	return ListUnitsWithContainer[*genProj.Folder](r)
}
