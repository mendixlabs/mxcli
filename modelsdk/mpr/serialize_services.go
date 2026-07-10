// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// SerializeImageCollection returns BSON bytes for an image collection unit.
// Ported from sdk/mpr/writer_imagecollection.go — same logic, no Writer dependency.
func SerializeImageCollection(ic *types.ImageCollection) ([]byte, error) {
	if ic.ID == "" {
		ic.ID = model.ID(generateUUID())
	}
	if ic.ExportLevel == "" {
		ic.ExportLevel = "Hidden"
	}

	// Images array always starts with the array marker int32(3).
	images := bson.A{int32(3)}
	for i := range ic.Images {
		img := &ic.Images[i]
		if img.ID == "" {
			img.ID = model.ID(generateUUID())
		}
		images = append(images, bson.D{
			{Key: "$ID", Value: idToBsonBinary(string(img.ID))},
			{Key: "$Type", Value: "Images$Image"},
			{Key: "Image", Value: bson.Binary{Subtype: 0, Data: img.Data}},
			{Key: "ImageFormat", Value: img.Format},
			{Key: "Name", Value: img.Name},
		})
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(ic.ID))},
		{Key: "$Type", Value: "Images$ImageCollection"},
		{Key: "Documentation", Value: ic.Documentation},
		{Key: "Excluded", Value: false},
		{Key: "ExportLevel", Value: ic.ExportLevel},
		{Key: "Images", Value: images},
		{Key: "Name", Value: ic.Name},
	}

	return bson.Marshal(doc)
}

// SerializeDataTransformer returns BSON bytes for a data transformer unit.
// Ported from sdk/mpr/writer_datatransformer.go — same logic, no Writer dependency.
func SerializeDataTransformer(dt *model.DataTransformer) ([]byte, error) {
	if dt.ID == "" {
		dt.ID = model.ID(generateUUID())
	}

	// Root element
	rootElemID := generateUUID()
	rootElement := bson.D{
		{Key: "$ID", Value: idToBsonBinary(rootElemID)},
		{Key: "$Type", Value: "DataTransformers$StructureObject"},
		{Key: "Attributes", Value: bson.A{int32(2)}},
	}

	// Source
	var source bson.D
	switch strings.ToUpper(dt.SourceType) {
	case "XML":
		source = bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "DataTransformers$XmlSource"},
			{Key: "Content", Value: dt.SourceJSON},
		}
	default: // JSON
		source = bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "DataTransformers$JsonSource"},
			{Key: "Content", Value: dt.SourceJSON},
		}
	}

	// Steps (versioned array prefix int32(2))
	steps := bson.A{int32(2)}
	for _, step := range dt.Steps {
		var action bson.D
		switch strings.ToUpper(step.Technology) {
		case "JSLT":
			action = bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "DataTransformers$JsltAction"},
				{Key: "Jslt", Value: step.Expression},
			}
		case "XSLT":
			action = bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "DataTransformers$XsltAction"},
				{Key: "Xslt", Value: step.Expression},
			}
		default:
			action = bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "DataTransformers$JsltAction"},
				{Key: "Jslt", Value: step.Expression},
			}
		}

		steps = append(steps, bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "DataTransformers$Step"},
			{Key: "Action", Value: action},
			{Key: "InputElementPointer", Value: idToBsonBinary(rootElemID)},
			{Key: "OutputElementPointer", Value: idToBsonBinary(rootElemID)},
		})
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(dt.ID))},
		{Key: "$Type", Value: "DataTransformers$DataTransformer"},
		{Key: "Name", Value: dt.Name},
		{Key: "Documentation", Value: ""},
		{Key: "Excluded", Value: dt.Excluded},
		{Key: "ExportLevel", Value: "Hidden"},
		{Key: "Source", Value: source},
		{Key: "Elements", Value: bson.A{int32(2), rootElement}},
		{Key: "RootElementPointer", Value: idToBsonBinary(rootElemID)},
		{Key: "Steps", Value: steps},
	}

	return bson.Marshal(doc)
}

// SerializeProjectSettings returns BSON bytes for the project settings unit.
// Ported from sdk/mpr/writer_settings.go — same logic, no Writer dependency.
func SerializeProjectSettings(ps *model.ProjectSettings) ([]byte, error) {
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(ps.ID))},
		{Key: "$Type", Value: "Settings$ProjectSettings"},
	}

	// Rebuild the Settings array from RawParts, overwriting modified parts.
	settings := bson.A{int32(2)} // versioned array prefix

	for _, rawPart := range ps.RawParts {
		typeName, _ := rawPart["$Type"].(string)
		switch typeName {
		case "Settings$ModelSettings":
			if ps.Model != nil {
				settings = append(settings, serPSModelSettings(ps.Model, rawPart))
			} else {
				settings = append(settings, rawPart)
			}
		case "Settings$ConfigurationSettings":
			if ps.Configuration != nil {
				settings = append(settings, serPSConfigurationSettings(ps.Configuration, rawPart))
			} else {
				settings = append(settings, rawPart)
			}
		case "Settings$LanguageSettings":
			if ps.Language != nil {
				settings = append(settings, serPSLanguageSettings(ps.Language, rawPart))
			} else {
				settings = append(settings, rawPart)
			}
		case "Settings$WorkflowsProjectSettingsPart":
			if ps.Workflows != nil {
				settings = append(settings, serPSWorkflowsSettings(ps.Workflows, rawPart))
			} else {
				settings = append(settings, rawPart)
			}
		default:
			// Preserve raw part as-is (WebUI, Integration, Certificate, JarDeployment, Distribution, Convention)
			settings = append(settings, rawPart)
		}
	}

	doc = append(doc, bson.E{Key: "Settings", Value: settings})
	return bson.Marshal(doc)
}

// ── Private helpers (prefixed serPS* to avoid naming conflicts) ───────────────

func serPSInt64(v int) int64 { return int64(v) }

func serPSModelSettings(ms *model.ModelSettings, raw map[string]any) map[string]any {
	raw["AfterStartupMicroflow"] = ms.AfterStartupMicroflow
	raw["BeforeShutdownMicroflow"] = ms.BeforeShutdownMicroflow
	raw["HealthCheckMicroflow"] = ms.HealthCheckMicroflow
	raw["AllowUserMultipleSessions"] = ms.AllowUserMultipleSessions
	raw["HashAlgorithm"] = ms.HashAlgorithm
	raw["BcryptCost"] = serPSInt64(ms.BcryptCost)
	raw["JavaVersion"] = ms.JavaVersion
	raw["RoundingMode"] = ms.RoundingMode
	raw["ScheduledEventTimeZoneCode"] = ms.ScheduledEventTimeZoneCode
	raw["FirstDayOfWeek"] = ms.FirstDayOfWeek
	raw["DecimalScale"] = serPSInt64(ms.DecimalScale)
	raw["EnableDataStorageOptimisticLocking"] = ms.EnableDataStorageOptimisticLocking
	raw["UseDatabaseForeignKeyConstraints"] = ms.UseDatabaseForeignKeyConstraints
	return raw
}

func serPSConfigurationSettings(cs *model.ConfigurationSettings, raw map[string]any) map[string]any {
	configs := bson.A{int32(2)} // versioned array prefix
	for _, cfg := range cs.Configurations {
		configs = append(configs, serPSServerConfiguration(cfg))
	}
	raw["Configurations"] = configs
	return raw
}

func serPSServerConfiguration(cfg *model.ServerConfiguration) bson.D {
	cfgID := string(cfg.ID)
	if cfgID == "" {
		cfgID = generateUUID()
	}
	cfgDoc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(cfgID)},
		{Key: "$Type", Value: "Settings$ServerConfiguration"},
		{Key: "Name", Value: cfg.Name},
		{Key: "DatabaseType", Value: cfg.DatabaseType},
		{Key: "DatabaseUrl", Value: cfg.DatabaseUrl},
		{Key: "DatabaseName", Value: cfg.DatabaseName},
		{Key: "DatabaseUserName", Value: cfg.DatabaseUserName},
		{Key: "DatabasePassword", Value: cfg.DatabasePassword},
		{Key: "DatabaseUseIntegratedSecurity", Value: cfg.DatabaseUseIntegratedSecurity},
		{Key: "HttpPortNumber", Value: serPSInt64(cfg.HttpPortNumber)},
		{Key: "ServerPortNumber", Value: serPSInt64(cfg.ServerPortNumber)},
		{Key: "ApplicationRootUrl", Value: cfg.ApplicationRootUrl},
		{Key: "MaxJavaHeapSize", Value: serPSInt64(cfg.MaxJavaHeapSize)},
		{Key: "ExtraJvmParameters", Value: cfg.ExtraJvmParameters},
		{Key: "OpenAdminPort", Value: cfg.OpenAdminPort},
		{Key: "OpenHttpPort", Value: cfg.OpenHttpPort},
		{Key: "CustomSettings", Value: bson.A{int32(2)}},
		{Key: "Tracing", Value: nil},
	}

	// Serialize ConstantValues (versioned array prefix)
	cvArr := bson.A{int32(2)}
	for _, cv := range cfg.ConstantValues {
		cvArr = append(cvArr, serPSConstantValue(cv))
	}
	cfgDoc = append(cfgDoc, bson.E{Key: "ConstantValues", Value: cvArr})

	return cfgDoc
}

func serPSConstantValue(cv *model.ConstantValue) bson.D {
	cvID := string(cv.ID)
	if cvID == "" {
		cvID = generateUUID()
	}
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(cvID)},
		{Key: "$Type", Value: "Settings$ConstantValue"},
		{Key: "ConstantId", Value: cv.ConstantId},
		{Key: "Value", Value: cv.Value},
	}
}

func serPSLanguageSettings(ls *model.LanguageSettings, raw map[string]any) map[string]any {
	raw["DefaultLanguageCode"] = ls.DefaultLanguageCode
	return raw
}

func serPSWorkflowsSettings(ws *model.WorkflowsSettings, raw map[string]any) map[string]any {
	raw["UserEntity"] = ws.UserEntity
	raw["DefaultTaskParallelism"] = serPSInt64(ws.DefaultTaskParallelism)
	raw["WorkflowEngineParallelism"] = serPSInt64(ws.WorkflowEngineParallelism)
	return raw
}
